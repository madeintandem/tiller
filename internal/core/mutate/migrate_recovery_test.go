package mutate_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func schemaNineRecoveryFixture(t *testing.T) string {
	t.Helper()
	root := schemaEightContributorFixture(t)
	path := filepath.Join(root, ".tiller/tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data, err = store.RenderContributorMigration(data)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	gitCommitAllM(t, root, "schema nine with active aggregate reviews")
	writeMigrationFile(t, root, "identity.yml", "# real checkout actor\ncontributor: sam # preserve local identity\n")
	return root
}

func TestSchemaNineRecoveryMigrationOnlyStampsManifestAndAppendsEvent(t *testing.T) {
	t.Parallel()
	root := schemaNineRecoveryFixture(t)
	manifestPath := filepath.Join(root, ".tiller/tiller.yml")
	manifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	refs := func() []byte {
		cmd := exec.Command("git", "show-ref")
		cmd.Dir = root
		data, err := cmd.Output()
		require.NoError(t, err)
		return data
	}
	beforeRefs := refs()
	inspection, err := mutate.InspectMigration(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, 9, inspection.From)
	require.Equal(t, store.CurrentSchemaVersion, inspection.To)
	require.Empty(t, inspection.Blockers)
	require.Nil(t, inspection.OwnershipReview)
	require.Equal(t, []string{".tiller/tiller.yml"}, inspection.Files)
	require.Equal(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
	result, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, mutate.MigrateOutcomeMigrated, result.Outcome)
	require.Len(t, result.Files, 2)
	after := snapshotDir(t, filepath.Join(root, ".tiller"))
	for path, content := range before {
		if path == "tiller.yml" || strings.HasPrefix(path, ".cache/") {
			continue
		}
		require.Equal(t, content, after[path], "work, existing event, and local identity bytes: %s", path)
	}
	written, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(string(manifest), "schema_version: 9", fmt.Sprintf("schema_version: %d", store.CurrentSchemaVersion), 1), string(written))
	require.Equal(t, beforeRefs, refs())
	tree, err := store.Load(root)
	require.NoError(t, err)
	require.Equal(t, "in_review", tree.Tasks["T-aaaaaa"].Status)
	require.Equal(t, 71, *tree.Epics["E-aaaaaa"].PR)
	require.Equal(t, 72, *tree.Features["F-aaaaaa"].PR)
	require.Nil(t, tree.Epics["E-aaaaaa"].RecoveredReview)
	require.Nil(t, tree.Features["F-aaaaaa"].RecoveredReview)
	recorded, err := events.List(tree)
	require.NoError(t, err)
	require.Len(t, recorded, 2)
	var migration *events.Event
	for i := range recorded {
		if recorded[i].Type == "migrate.schema" {
			migration = &recorded[i]
		}
	}
	require.NotNil(t, migration)
	require.Equal(t, map[string]any{"schema_version": 9}, migration.Before)
	require.Equal(t, map[string]any{"schema_version": store.CurrentSchemaVersion}, migration.After)
	secondBefore := snapshotDir(t, filepath.Join(root, ".tiller"))
	result, err = mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, mutate.MigrateOutcomeCurrent, result.Outcome)
	require.Equal(t, secondBefore, snapshotDir(t, filepath.Join(root, ".tiller")))
}

func TestRecoveryMigrationRefusesNewerSchemaWithoutWrites(t *testing.T) {
	t.Parallel()
	root := schemaNineRecoveryFixture(t)
	path := filepath.Join(root, ".tiller/tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data, err = store.RenderSchemaVersion(data, store.CurrentSchemaVersion+1)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	_, err = mutate.Migrate(root, testDeps())
	var refusal *store.EnvError
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, store.ReasonSchemaVersionUnsupported, refusal.Reason)
	require.Equal(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
}

func TestSchemaNineRecoveryMigrationMaintainsTeammateLegacyIdentity(t *testing.T) {
	t.Parallel()
	root := schemaNineRecoveryFixture(t)
	identityPath := filepath.Join(root, ".tiller/identity.yml")
	legacy := "# teammate pulled schema nine before refreshing local identity\nowner: 'sam' # selected identity\ncustom: {owner: preserve}\n"
	require.NoError(t, os.WriteFile(identityPath, []byte(legacy), 0o644))
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	selected, err := store.ReadLocalIdentityForMigration(root)
	require.NoError(t, err, "maintenance must resolve the teammate's retained legacy identity")
	require.Equal(t, "sam", selected)
	deps := testDeps()
	deps.Contributor = selected
	inspection, err := mutate.InspectMigration(root, deps)
	require.NoError(t, err)
	require.Contains(t, inspection.Files, ".tiller/identity.yml")
	require.Equal(t, before, snapshotDir(t, filepath.Join(root, ".tiller")), "inspection never writes")
	result, err := mutate.Migrate(root, deps)
	require.NoError(t, err)
	require.Equal(t, store.CurrentSchemaVersion, result.To)
	require.Equal(t, "sam", result.Contributor)
	require.Len(t, result.Files, 3, "manifest, required legacy local identity conversion, migration event")
	actual, err := os.ReadFile(identityPath)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(legacy, "\nowner:", "\ncontributor:", 1), string(actual))
	selected, err = store.ReadLocalIdentity(root)
	require.NoError(t, err)
	require.Equal(t, "sam", selected)
	after := snapshotDir(t, filepath.Join(root, ".tiller"))
	for path, content := range before {
		if path == "tiller.yml" || path == "identity.yml" || strings.HasPrefix(path, ".cache/") {
			continue
		}
		require.Equal(t, content, after[path], path)
	}
}

func TestMigrate10To11StampsOnly(t *testing.T) {
	t.Parallel()
	root := schemaNineRecoveryFixture(t)
	path := filepath.Join(root, ".tiller/tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data, err = store.RenderSchemaVersion(data, 10)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	gitCommitAllM(t, root, "schema ten")
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	result, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, mutate.MigrateOutcomeMigrated, result.Outcome)
	require.Equal(t, 10, result.From)
	require.Equal(t, store.CurrentSchemaVersion, result.To)
	after := snapshotDir(t, filepath.Join(root, ".tiller"))
	for name, content := range before {
		if name == "tiller.yml" || strings.HasPrefix(name, ".cache/") {
			continue
		}
		require.Equal(t, content, after[name], "work files stay byte-identical: %s", name)
	}
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(string(data), "schema_version: 10", fmt.Sprintf("schema_version: %d", store.CurrentSchemaVersion), 1), string(written))
}

func TestMigrate11To12StampsOnly(t *testing.T) {
	root := schemaNineRecoveryFixture(t)
	path := filepath.Join(root, ".tiller/tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data, err = store.RenderSchemaVersion(data, 11)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	gitCommitAllM(t, root, "schema eleven")
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	result, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, mutate.MigrateOutcomeMigrated, result.Outcome)
	require.Equal(t, 11, result.From)
	require.Equal(t, 12, result.To)
	after := snapshotDir(t, filepath.Join(root, ".tiller"))
	for name, content := range before {
		if name == "tiller.yml" || strings.HasPrefix(name, ".cache/") {
			continue
		}
		require.Equal(t, content, after[name], "work files stay byte-identical: %s", name)
	}
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(string(data), "schema_version: 11", "schema_version: 12", 1), string(written))
}
