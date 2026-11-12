package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func legacyMaintenanceFixture(t *testing.T, schema int) string {
	t.Helper()
	root := maintenanceFixture(t)
	path := filepath.Join(root, ".tiller", "tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	legacy, err := store.RenderSchemaVersion([]byte(strings.Replace(string(data), "contributors:", "owners:", 1)), schema)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, legacy, 0644))
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-qm", "legacy baseline")
	return root
}

func TestMigrateCommandStampsForwardThenNoOps(t *testing.T) {
	root := legacyMaintenanceFixture(t, 2)
	out, _, code := execCLI(t, "migrate", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	var e struct {
		OK   bool        `json:"ok"`
		Data migrateView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &e))
	require.True(t, e.OK)
	require.Equal(t, 2, e.Data.From)
	require.Equal(t, store.CurrentSchemaVersion, e.Data.To)
	require.Equal(t, "migrated", string(e.Data.Outcome))
	require.Equal(t, "ran", e.Data.Refresh)
	_, err := os.Stat(filepath.Join(root, ".tiller", "identity.yml"))
	require.True(t, os.IsNotExist(err))
	require.Equal(t, gen.Orientation(), readFile(t, root, ".tiller", "orientation.md"))
	out, _, code = execCLI(t, "migrate", "--json")
	require.Equal(t, 0, code, out)
	e.Data = migrateView{}
	require.NoError(t, json.Unmarshal([]byte(out), &e))
	require.Equal(t, "current", string(e.Data.Outcome))
	require.Empty(t, e.Data.Files)
	require.Empty(t, e.Data.Project.Artifacts.Files)
	require.Nil(t, e.Data.Project.Actor)
}

func TestMigrateRefreshesCurrentUnjoinedTreeWithoutActor(t *testing.T) {
	root := maintenanceFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "orientation.md"), []byte("stale\n"), 0644))
	out, _, code := execCLI(t, "migrate", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, "current", jsonData(t, out)["outcome"])
	require.Equal(t, "ran", jsonData(t, out)["refresh"])
	require.Equal(t, gen.Orientation(), readFile(t, root, ".tiller", "orientation.md"))
}

func TestMigrationActorAndDirtyInputRefusalsPreserveSchema(t *testing.T) {
	root := legacyMaintenanceFixture(t, 4)
	before := readFile(t, root, ".tiller", "tiller.yml")
	out, _, code := execCLI(t, "upgrade", "--project-only", "--json")
	require.Equal(t, 3, code, out)
	require.Equal(t, "contributor_unset", readMaintenanceEnvelope(t, out).Error.Reason)
	out, _, code = execCLI(t, "upgrade", "--project-only", "--contributor", "missing", "--json")
	require.Equal(t, 3, code, out)
	require.Equal(t, "contributor_unknown", readMaintenanceEnvelope(t, out).Error.Reason)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(before+"\n# human work\n"), 0644))
	out, _, code = execCLI(t, "upgrade", "--project-only", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	e := readMaintenanceEnvelope(t, out)
	require.NotNil(t, e.Data)
	require.False(t, e.Data.Complete)
	require.Equal(t, "migration_inputs_dirty", e.Data.Failure.Reason)
	require.Equal(t, 4, e.Data.Project.SchemaAfter)
}

func TestMigrateRefreshFailureIsNonzeroWithPartialResult(t *testing.T) {
	root := maintenanceFixture(t)
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller", "orientation.md")))
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "outside"), filepath.Join(root, ".tiller", "orientation.md")))
	out, _, code := execCLI(t, "migrate", "--json")
	require.Equal(t, 3, code, out)
	e := readMaintenanceEnvelope(t, out)
	require.NotNil(t, e.Error.PartialResult)
	require.False(t, e.Error.PartialResult.Complete)
	require.Equal(t, "failed", e.Error.PartialResult.Project.Artifacts.Status)
}

func TestMigrationRefusesHiddenDirtyInputsWithoutChangingIndex(t *testing.T) {
	for _, flag := range []string{"--assume-unchanged", "--skip-worktree"} {
		t.Run(flag, func(t *testing.T) {
			root := legacyMaintenanceFixture(t, 4)
			gitRun(t, root, "update-index", flag, ".tiller/tiller.yml")
			path := filepath.Join(root, ".tiller", "tiller.yml")
			content := readFile(t, path) + "\n# hidden human edit\n"
			require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
			index := readFile(t, root, ".git", "index")
			out, _, code := execCLI(t, "upgrade", "--project-only", "--contributor", "jc", "--json")
			require.Equal(t, 1, code, out)
			require.Equal(t, "migration_inputs_dirty", readMaintenanceEnvelope(t, out).Data.Failure.Reason)
			require.Equal(t, content, readFile(t, path))
			require.Equal(t, index, readFile(t, root, ".git", "index"))
		})
	}
}

func TestProjectOnlyPreservesMigrationAfterRefreshRollback(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses filesystem permissions")
	}
	root := legacyMaintenanceFixture(t, 4)
	directory := filepath.Join(root, ".claude", "skills")
	require.NoError(t, os.MkdirAll(directory, 0o755))
	require.NoError(t, os.Chmod(directory, 0o555))
	t.Cleanup(func() { _ = os.Chmod(directory, 0o755) })
	out, _, code := execCLI(t, "upgrade", "--project-only", "--contributor", "jc", "--json")
	require.Equal(t, 3, code, out)
	partial := readMaintenanceEnvelope(t, out).Error.PartialResult
	require.Equal(t, store.CurrentSchemaVersion, partial.Project.SchemaAfter)
	require.Equal(t, "migrated", partial.Project.Migration.Status)
	require.Equal(t, "failed", partial.Project.Artifacts.Status)
	require.Empty(t, partial.Project.ShimsAdded)
	require.NoFileExists(t, filepath.Join(root, ".tiller", ".cache", "journal.yml"))
	require.NoError(t, os.Chmod(directory, 0o755))
	out, _, code = execCLI(t, "upgrade", "--project-only", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, "current", readMaintenanceEnvelope(t, out).Data.Project.Migration.Status)
}

func TestMaintenanceReportsRecoveryRequiredWithoutClearingJournal(t *testing.T) {
	root := maintenanceFixture(t)
	path := filepath.Join(root, ".tiller", ".cache", "journal.yml")
	require.NoError(t, os.WriteFile(path, []byte("retained recovery journal\n"), 0o644))
	out, _, code := execCLI(t, "upgrade", "--project-only", "--json")
	require.Equal(t, 3, code, out)
	partial := readMaintenanceEnvelope(t, out).Error.PartialResult
	require.Equal(t, "mutation_recovery_needed", partial.Failure.Reason)
	require.Equal(t, "recovery_required", partial.Project.Artifacts.Status)
	require.Equal(t, "retained recovery journal\n", readFile(t, path))
}

func TestMigrationUsesAndRewritesLegacyCheckoutIdentity(t *testing.T) {
	root := legacyMaintenanceFixture(t, 8)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	identityPath := filepath.Join(root, ".tiller", "identity.yml")
	require.NoError(t, os.WriteFile(identityPath, []byte("owner: jc\n"), 0o644))
	out, _, code := execCLI(t, "migrate", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, readFile(t, identityPath), "contributor: jc")
	require.NotContains(t, readFile(t, identityPath), "owner:")
	id, err := store.ReadLocalIdentity(root)
	require.NoError(t, err)
	require.Equal(t, "jc", id)
	require.Contains(t, readFile(t, root, ".tiller", "tiller.yml"), "contributors:")
}

// identity.yml is gitignored, so a schema-9 migration in one checkout cannot
// rename the legacy owner key in a teammate's clone. The teammate's documented
// refresh, `tiller upgrade --project-only`, renames it after the pull.
func TestProjectOnlyRenamesTeammateLegacyIdentityAfterPull(t *testing.T) {
	origin := legacyMaintenanceFixture(t, 8)
	teammate := filepath.Join(t.TempDir(), "teammate")
	gitRun(t, origin, "clone", "-q", origin, teammate)
	gitRun(t, teammate, "config", "user.email", "teammate@example.com")
	gitRun(t, teammate, "config", "user.name", "Teammate")
	gitRun(t, teammate, "config", "commit.gpgsign", "false")
	identityPath := filepath.Join(teammate, ".tiller", "identity.yml")
	legacy := "# teammate checkout\nowner: jc # me\n"
	require.NoError(t, os.WriteFile(identityPath, []byte(legacy), 0o644))

	out, _, code := execCLI(t, "migrate", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	gitRun(t, origin, "add", ".")
	gitRun(t, origin, "commit", "-qm", "migrate to schema 9")
	gitRun(t, teammate, "pull", "-q", "--ff-only")
	t.Chdir(teammate)

	out, _, code = execCLI(t, "add", "epic", "example", "--json")
	require.Equal(t, 3, code, out)
	require.Contains(t, out, store.ReasonIdentityUnreadable)
	require.Contains(t, out, "tiller upgrade --project-only", "the refusal names the remedy")

	out, _, code = execCLI(t, "upgrade", "--project-only", "--inspect", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, readMaintenanceEnvelope(t, out).Data.Project.PlannedSourceFiles, ".tiller/identity.yml")
	require.Equal(t, legacy, readFile(t, identityPath), "inspection never writes")

	out, _, code = execCLI(t, "upgrade", "--project-only", "--json")
	require.Equal(t, 0, code, out)
	project := readMaintenanceEnvelope(t, out).Data.Project
	require.Equal(t, "current", project.Migration.Status)
	require.Contains(t, project.Migration.Files, ".tiller/identity.yml")
	require.Equal(t, "# teammate checkout\ncontributor: jc # me\n", readFile(t, identityPath), "only the key is renamed")

	out, _, code = execCLI(t, "add", "epic", "example", "--json")
	require.Equal(t, 0, code, out)
}

func TestProjectOnlyRefusesLegacyIdentityItCannotRename(t *testing.T) {
	root := maintenanceFixture(t)
	identityPath := filepath.Join(root, ".tiller", "identity.yml")
	for _, body := range []string{"owner: missing\n", "owner: jc\ncontributor: jc\n"} {
		require.NoError(t, os.WriteFile(identityPath, []byte(body), 0o644))
		out, _, code := execCLI(t, "upgrade", "--project-only", "--json")
		require.Equal(t, 3, code, out)
		require.Equal(t, body, readFile(t, identityPath))
	}
}

// Only a legacy owner key the refresh cannot rename blocks it. A malformed or
// symlinked identity file holds no renameable key, so the refresh completes as
// it did before the rename existed and leaves the file for ordinary commands
// to refuse.
func TestProjectOnlyCompletesPastAnUnrenameableIdentityFile(t *testing.T) {
	root := maintenanceFixture(t)
	identityPath := filepath.Join(root, ".tiller", "identity.yml")

	require.NoError(t, os.WriteFile(identityPath, []byte("jc\n"), 0o644))
	out, _, code := execCLI(t, "upgrade", "--project-only", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, "jc\n", readFile(t, identityPath))

	require.NoError(t, os.Remove(identityPath))
	target := filepath.Join(t.TempDir(), "identity.yml")
	require.NoError(t, os.WriteFile(target, []byte("owner: jc\n"), 0o644))
	require.NoError(t, os.Symlink(target, identityPath))
	out, _, code = execCLI(t, "upgrade", "--project-only", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, "owner: jc\n", readFile(t, target), "the refresh never follows the link")
}

// A legacy id missing from the roster cannot be renamed into a usable identity,
// so the refusal names both ways out: select a listed contributor, or register
// the legacy id.
func TestProjectOnlyNamesTheRemedyForAnUnlistedLegacyIdentity(t *testing.T) {
	root := maintenanceFixture(t)
	identityPath := filepath.Join(root, ".tiller", "identity.yml")
	require.NoError(t, os.WriteFile(identityPath, []byte("owner: ghost\n"), 0o644))
	_, stderr, code := execCLI(t, "upgrade", "--project-only")
	require.Equal(t, 3, code, stderr)
	require.Contains(t, stderr, `.tiller/identity.yml names contributor "ghost", who is not in the project contributors: `+
		"run `tiller join --as <id>` to select a listed contributor, or `tiller add contributor ghost --name <name>` to register them")
}

// thirteenFixture is a committed schema 13 checkout, the last schema before
// event history compacts on upgrade.
func thirteenFixture(t *testing.T) string {
	t.Helper()
	root := maintenanceFixture(t)
	path := filepath.Join(root, ".tiller", "tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	old, err := store.RenderSchemaVersion(data, 13)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, old, 0o644))
	return root
}

func commitAll(t *testing.T, root, message string) {
	t.Helper()
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", message)
}

func migratedManifestSchema(t *testing.T, root string) int {
	t.Helper()
	m, err := store.ParseManifest(root, []byte(readFile(t, root, ".tiller", "tiller.yml")))
	require.NoError(t, err)
	return m.SchemaVersion
}

func looseEventPaths(t *testing.T, root string) []string {
	t.Helper()
	loose, err := filepath.Glob(filepath.Join(root, ".tiller", "events", "*", "*", "*", "*.yml"))
	require.NoError(t, err)
	return loose
}

func daysAgo(n int) string {
	return time.Now().UTC().AddDate(0, 0, -n).Format(time.RFC3339)
}

func TestMigrateFromThirteenCompactsOldEvents(t *testing.T) {
	root := thirteenFixture(t)
	old := layLooseEvent(t, root, "EV-oldold", daysAgo(10))
	today := layLooseEvent(t, root, "EV-newnew", daysAgo(0))
	commitAll(t, root, "schema thirteen with history")
	looseBefore := looseEventPaths(t, root)

	out, _, code := execCLI(t, "migrate", "--contributor", "jc", "--json")

	require.Equal(t, 0, code, out)
	require.Equal(t, 14, migratedManifestSchema(t, root))
	require.NoFileExists(t, old)
	require.FileExists(t, today)
	segments, err := filepath.Glob(filepath.Join(filepath.Dir(filepath.Dir(old)), "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, segments, 1)
	var e struct {
		Data struct {
			Project struct {
				Compaction *struct {
					Days []json.RawMessage `json:"days"`
				} `json:"compaction"`
			} `json:"project"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &e))
	require.NotNil(t, e.Data.Project.Compaction)
	require.Len(t, e.Data.Project.Compaction.Days, 1)
	require.Len(t, looseEventPaths(t, root), len(looseBefore), "the old event left and the migrate.schema event arrived")
}

// TestMigrateHumanOutputCarriesTheStageCommand pins that a migration that
// compacted hands over the same git add line sync and compact print, so the
// deletions and the new segments are staged together, and the skipped line.
func TestMigrateHumanOutputCarriesTheStageCommand(t *testing.T) {
	root := thirteenFixture(t)
	layLooseEvent(t, root, "EV-oldold", daysAgo(10))
	layUncarriableEvent(t, root, "EV-oddodd", daysAgo(10))
	commitAll(t, root, "schema thirteen with history")

	out, _, code := execCLI(t, "migrate", "--contributor", "jc")

	require.Equal(t, 0, code, out)
	require.Contains(t, out, "Compacted 1 day of events (1 event) into 1 file. Stage them with:\n  git add -A -- .tiller/events/")
	require.Contains(t, out, "1 event could not be compacted and stays as its own file.")
	line := stageLine(t, out)
	git := exec.Command("git", strings.Fields(line)[1:]...)
	git.Dir = root
	output, err := git.CombinedOutput()
	require.NoError(t, err, "%s\n%s", line, output)
}

// TestMigrateSaysNothingAboutCompactionWhenNoDayFolded pins that a migration
// whose only eligible event cannot be carried reports no compaction at all,
// in human output or in JSON.
func TestMigrateSaysNothingAboutCompactionWhenNoDayFolded(t *testing.T) {
	root := thirteenFixture(t)
	layUncarriableEvent(t, root, "EV-oddodd", daysAgo(10))
	commitAll(t, root, "schema thirteen with an uncarriable event")

	out, _, code := execCLI(t, "migrate", "--contributor", "jc", "--json")

	require.Equal(t, 0, code, out)
	require.Equal(t, 14, migratedManifestSchema(t, root))
	require.NotContains(t, out, `"compaction"`)
	require.NotContains(t, out, "could not be compacted")
}

func TestMigrateHumanOutputSaysNothingAboutCompactionWhenNoDayFolded(t *testing.T) {
	root := thirteenFixture(t)
	layUncarriableEvent(t, root, "EV-oddodd", daysAgo(10))
	commitAll(t, root, "schema thirteen with an uncarriable event")

	out, _, code := execCLI(t, "migrate", "--contributor", "jc")

	require.Equal(t, 0, code, out)
	require.NotContains(t, out, "Compacted")
	require.NotContains(t, out, "could not be compacted")
	require.NotContains(t, out, "git add -A --")
}

func TestMigrateOnACurrentTreeDoesNotCompact(t *testing.T) {
	root := maintenanceFixture(t)
	old := layLooseEvent(t, root, "EV-oldold", daysAgo(10))
	commitAll(t, root, "current with history")

	out, _, code := execCLI(t, "migrate", "--contributor", "jc", "--json")

	require.Equal(t, 0, code, out)
	require.FileExists(t, old)
	require.NotContains(t, out, `"compaction"`)
}

func TestMigrateSurvivesACompactionFailure(t *testing.T) {
	root := thirteenFixture(t)
	when := daysAgo(10)
	loose := layLooseEvent(t, root, "EV-oldold", when)
	at, err := time.Parse(time.RFC3339, when)
	require.NoError(t, err)
	held := events.Event{ID: "EV-oldold", Type: "start.task", At: at, Contributor: "jc", Command: "tiller start", Targets: []string{"T-aaaaaa"}}
	raw, err := events.EncodeSegment([]events.Event{held})
	require.NoError(t, err)
	segment := filepath.Join(root, events.SegmentRelPath(at.UTC(), raw))
	require.NoError(t, os.WriteFile(segment, raw, 0o644))
	commitAll(t, root, "schema thirteen with disagreeing history")

	out, _, code := execCLI(t, "migrate", "--contributor", "jc", "--json")

	require.Equal(t, 0, code, out)
	require.Equal(t, 14, migratedManifestSchema(t, root))
	require.FileExists(t, loose)
	require.Contains(t, out, "Event history was not fully compacted:")
}

// TestMigrateSurvivesACompactionPanic pins that a panic inside the upgrade's
// compaction is the same warning a failure is, and maintenance goes on to
// the identity rename and the artifact refresh.
func TestMigrateSurvivesACompactionPanic(t *testing.T) {
	root := thirteenFixture(t)
	loose := layLooseEvent(t, root, "EV-oldold", daysAgo(10))
	commitAll(t, root, "schema thirteen with history")
	panickingCompaction(t)

	out, _, code := execCLI(t, "migrate", "--contributor", "jc", "--json")

	require.Equal(t, 0, code, out)
	require.Equal(t, 14, migratedManifestSchema(t, root))
	require.FileExists(t, loose)
	require.Contains(t, out, "Event history was not fully compacted: segment reader exploded.")
	var e struct {
		Data struct {
			Complete bool `json:"complete"`
			Project  struct {
				Status    string `json:"status"`
				Artifacts struct {
					Status string `json:"status"`
				} `json:"artifacts"`
			} `json:"project"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &e))
	require.True(t, e.Data.Complete, out)
	require.Equal(t, "complete", e.Data.Project.Status)
	require.Contains(t, []string{"refreshed", "current"}, e.Data.Project.Artifacts.Status)
}
