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
	"gopkg.in/yaml.v3"
)

func writeSchema2Repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tdir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(tdir, 0o755))
	manifest := "# hand comment\nschema_version: 2\nschema_epoch: initial\n\nproject:\n  name: \"X\"\n  key: x\n  kind: greenfield\n\nowners:\n  - {\"id\": \"jc\", \"name\": \"JC\"}\n"
	require.NoError(t, os.WriteFile(filepath.Join(tdir, "tiller.yml"), []byte(manifest), 0o644))
	gitInitM(t, dir)
	gitCommitAllM(t, dir, "init")
	return dir
}

func TestMigrateStampsTwoToCurrentPreservingEveryOtherByte(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	before, err := os.ReadFile(filepath.Join(root, ".tiller", "tiller.yml"))
	require.NoError(t, err)
	res, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, 2, res.From)
	require.Equal(t, store.CurrentSchemaVersion, res.To)
	require.Equal(t, mutate.MigrateOutcomeMigrated, res.Outcome)
	require.Contains(t, res.Files, ".tiller/tiller.yml")
	require.Len(t, res.Files, 2, "transaction paths include the newly created migration event")
	after, err := os.ReadFile(filepath.Join(root, ".tiller", "tiller.yml"))
	require.NoError(t, err)
	require.Equal(t,
		strings.Replace(strings.Replace(string(before), "schema_version: 2", fmt.Sprintf("schema_version: %d", store.CurrentSchemaVersion), 1), "\nowners:", "\ncontributors:", 1),
		string(after), "only the stamp and roster key moved")
	// The tree now loads normally.
	_, err = store.Load(root)
	require.NoError(t, err)
}

func writeSchema3Repo(t *testing.T) string {
	t.Helper()
	root := writeSchema2Repo(t)
	p := filepath.Join(root, ".tiller", "tiller.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, []byte(strings.Replace(string(data), "schema_version: 2", "schema_version: 3", 1)), 0o644))
	gitCommitAllM(t, root, "stamp 3")
	return root
}

func TestMigrateStampsThreeToCurrentPreservingEveryOtherByte(t *testing.T) {
	t.Parallel()
	root := writeSchema3Repo(t)
	before, err := os.ReadFile(filepath.Join(root, ".tiller", "tiller.yml"))
	require.NoError(t, err)
	res, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, 3, res.From)
	require.Equal(t, store.CurrentSchemaVersion, res.To)
	after, err := os.ReadFile(filepath.Join(root, ".tiller", "tiller.yml"))
	require.NoError(t, err)
	require.Equal(t, strings.Replace(strings.Replace(string(before), "schema_version: 3", fmt.Sprintf("schema_version: %d", store.CurrentSchemaVersion), 1), "\nowners:", "\ncontributors:", 1), string(after))
	_, err = store.Load(root)
	require.NoError(t, err)
}

func TestMigrateCarriesTwoToCurrentThroughAllSteps(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	res, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, 2, res.From)
	require.Equal(t, store.CurrentSchemaVersion, res.To)
}

func TestMigrateOnACurrentTreeIsANoOp(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	_, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	res, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, mutate.MigrateOutcomeCurrent, res.Outcome)
	require.Equal(t, store.CurrentSchemaVersion, res.From)
}

func TestMigrateRecordsOneEvent(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	_, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	tree, err := store.Load(root)
	require.NoError(t, err)
	evs, err := events.List(tree)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	require.Equal(t, "migrate.schema", evs[0].Type)
	require.Equal(t, map[string]any{"schema_version": 2}, evs[0].Before)
	require.Equal(t, map[string]any{"schema_version": store.CurrentSchemaVersion}, evs[0].After)
}

func TestMigrateSixToEightPreservesWorkAndApprovedHistory(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	manifestPath := filepath.Join(root, ".tiller", "tiller.yml")
	manifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	manifest, err = store.RenderSchemaVersion(manifest, 6)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, manifest, 0o644))
	writeMigrationLineage(t, root, "status: ready\n", "status: active\n", "status: in_progress\nstarted: true\nowner: jc\nbranch: tiller/pay/bill/send\n")
	preserved := map[string]string{
		"epics/pay/features/bill/feature.md":         "# Billing\n\nKeep the original accepted contract.\n",
		"epics/pay/features/bill/tasks/send/task.md": "# Send\n\nKeep task acceptance unchanged.\n",
		"amendments/A-aaaaaa.yml":                    "id: A-aaaaaa\nstatus: approved\nreason: Original scope\noperations: []\n",
		"amendments/A-aaaaaa.md":                     "# Original amendment\n\nApproved history stays byte-for-byte intact.\n",
	}
	for path, content := range preserved {
		writeMigrationFile(t, root, path, content)
	}
	gitCommitAllM(t, root, "schema six with accepted work and history")
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	inspection, err := mutate.InspectMigration(root, testDeps())
	require.NoError(t, err)
	require.Nil(t, inspection.OwnershipReview, "schema-six work must not repeat reviewed ownership adoption")
	require.Empty(t, inspection.Blockers, "stamp-only migration does not require draining started work")
	requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))

	result, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, mutate.MigrateOutcomeMigrated, result.Outcome)
	require.Equal(t, 6, result.From)
	require.Equal(t, store.CurrentSchemaVersion, result.To)
	require.Len(t, result.Files, 2, "only manifest and migration event belong to the source transaction")
	after, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(strings.Replace(string(manifest), "schema_version: 6", fmt.Sprintf("schema_version: %d", store.CurrentSchemaVersion), 1), "\nowners:", "\ncontributors:", 1), string(after))
	preservedAfter := snapshotDir(t, filepath.Join(root, ".tiller"))
	for path, content := range before {
		if path == "tiller.yml" || strings.HasPrefix(path, ".cache/") {
			continue
		}
		require.Equal(t, content, preservedAfter[path], path)
	}
	tree, err := store.Load(root)
	require.NoError(t, err)
	recorded, err := events.List(tree)
	require.NoError(t, err)
	require.Len(t, recorded, 1)
	require.Equal(t, "migrate.schema", recorded[0].Type)
	require.Equal(t, map[string]any{"schema_version": 6}, recorded[0].Before)
	require.Equal(t, map[string]any{"schema_version": store.CurrentSchemaVersion}, recorded[0].After)
}

func TestMigrateSevenToEightPreservesWorkAndApprovedHistory(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	manifestPath := filepath.Join(root, ".tiller", "tiller.yml")
	manifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	manifest, err = store.RenderSchemaVersion(manifest, 7)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, manifest, 0o644))
	writeMigrationLineage(t, root, "status: ready\n", "status: active\n", "status: in_progress\nstarted: true\nowner: jc\nbranch: tiller/pay/bill/send\n")
	preserved := map[string]string{
		"bugs/legacy/bug.yml":                        "id: B-aaaaaa\nkey: bugs/legacy\nstatus: done\nstarted: true\nowner: jc\ndone_basis: ancestry\n",
		"chores/legacy/chore.yml":                    "id: CH-aaaaaa\nkey: chores/legacy\nstatus: in_progress\nstarted: true\nowner: jc\n",
		"epics/pay/features/bill/feature.md":         "# Billing\n\nKeep the original accepted contract.\n",
		"epics/pay/features/bill/tasks/send/task.md": "# Send\n\nKeep task acceptance unchanged.\n",
		"amendments/A-aaaaaa.yml":                    "id: A-aaaaaa\nstatus: approved\nreason: Original scope\noperations: []\n",
		"amendments/A-aaaaaa.md":                     "# Original amendment\n\nApproved history stays byte-for-byte intact.\n",
	}
	for path, content := range preserved {
		writeMigrationFile(t, root, path, content)
	}
	gitCommitAllM(t, root, "schema seven with accepted work and history")
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	inspection, err := mutate.InspectMigration(root, testDeps())
	require.NoError(t, err)
	require.Nil(t, inspection.OwnershipReview, "schema-seven work must not repeat reviewed ownership adoption")
	require.Empty(t, inspection.Blockers, "stamp-only migration does not require draining started work")
	requireNoWrites(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))

	result, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, mutate.MigrateOutcomeMigrated, result.Outcome)
	require.Equal(t, 7, result.From)
	require.Equal(t, store.CurrentSchemaVersion, result.To)
	require.Len(t, result.Files, 2, "only manifest and migration event belong to the source transaction")
	after, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(strings.Replace(string(manifest), "schema_version: 7", fmt.Sprintf("schema_version: %d", store.CurrentSchemaVersion), 1), "\nowners:", "\ncontributors:", 1), string(after))
	preservedAfter := snapshotDir(t, filepath.Join(root, ".tiller"))
	for path, content := range before {
		if path == "tiller.yml" || strings.HasPrefix(path, ".cache/") {
			continue
		}
		require.Equal(t, content, preservedAfter[path], path)
	}
	tree, err := store.Load(root)
	require.NoError(t, err)
	recorded, err := events.List(tree)
	require.NoError(t, err)
	require.Len(t, recorded, 1)
	require.Equal(t, "migrate.schema", recorded[0].Type)
	require.Equal(t, map[string]any{"schema_version": 7}, recorded[0].Before)
	require.Equal(t, map[string]any{"schema_version": store.CurrentSchemaVersion}, recorded[0].After)
}

// Serial: t.Setenv clears TILLER_CONTRIBUTOR for the whole process.
func TestMigrateRefusesBelowTheFloorAboveCurrentAndUnsetOwner(t *testing.T) {
	root := writeSchema2Repo(t)
	stamp := func(v string) {
		p := filepath.Join(root, ".tiller", "tiller.yml")
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(p, []byte(strings.Replace(string(data), "schema_version: 2", "schema_version: "+v, 1)), 0o644))
	}
	stamp("1")
	_, err := mutate.Migrate(root, testDeps())
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonSchemaVersionUnsupported, env.Reason)

	stamp2 := func(v string) { // reset then restamp
		p := filepath.Join(root, ".tiller", "tiller.yml")
		data, _ := os.ReadFile(p)
		os.WriteFile(p, []byte(strings.Replace(string(data), "schema_version: 1", "schema_version: "+v, 1)), 0o644)
	}
	stamp2(fmt.Sprint(store.CurrentSchemaVersion + 1))
	_, err = mutate.Migrate(root, testDeps())
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonSchemaVersionUnsupported, env.Reason)

	// Owner refusal: deps with no owner and no identity file.
	p := filepath.Join(root, ".tiller", "tiller.yml")
	data, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(data), fmt.Sprintf("schema_version: %d", store.CurrentSchemaVersion+1), "schema_version: 2", 1)), 0o644)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	deps := testDeps()
	deps.Contributor = ""
	_, err = mutate.Migrate(root, deps)
	require.Error(t, err, "a migration is a mutation and attributes to an owner")
}

// TestMigrateSucceedsOnATreeWithPriorMutationHistory guards against Migrate
// committing against a Tree whose generation does not match what is actually
// on disk. A brand-new fixture has no .tiller/.cache/generation file, so a
// zero-value generation happens to match; any real tree that has had even one
// mutation since init carries a non-empty on-disk generation, and Migrate
// must read and honor that real value rather than assume "".
func TestMigrateSucceedsOnATreeWithPriorMutationHistory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tdir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(tdir, 0o755))
	manifest := fmt.Sprintf("# hand comment\nschema_version: %d\nschema_epoch: initial\n\nproject:\n  name: \"X\"\n  key: x\n  kind: greenfield\n\ncontributors:\n  - {\"id\": \"jc\", \"name\": \"JC\"}\n", store.CurrentSchemaVersion)
	require.NoError(t, os.WriteFile(filepath.Join(tdir, "tiller.yml"), []byte(manifest), 0o644))
	gitInitM(t, dir)
	gitCommitAllM(t, dir, "init")

	// One real mutation against the current-schema tree, so
	// .tiller/.cache/generation exists and is non-empty, the same as any
	// project that has done real work since init.
	_, err := mutate.AddContributor(load(t, dir), testDeps(), store.Contributor{ID: "ada", Name: "Ada"})
	require.NoError(t, err)

	// Downgrade the stamp by hand, standing in for a tree whose manifest
	// predates the migration that would bring it to the current schema, even
	// though its mutation cache already carries history.
	p := filepath.Join(dir, ".tiller", "tiller.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	downgraded, err := store.RenderSchemaVersion(data, 2)
	require.NoError(t, err)
	downgraded = []byte(strings.Replace(string(downgraded), "\ncontributors:", "\nowners:", 1))
	require.NoError(t, os.WriteFile(p, downgraded, 0o644))
	gitCommitAllM(t, dir, "legacy state with prior mutation history")

	res, err := mutate.Migrate(dir, testDeps())
	require.NoError(t, err)
	require.Equal(t, mutate.MigrateOutcomeMigrated, res.Outcome)

	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Contains(t, string(after), fmt.Sprintf("schema_version: %d", store.CurrentSchemaVersion))
}

func writeMigrationFile(t *testing.T, root, rel, data string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
}

func writeMigrationLineage(t *testing.T, root, epic, feature, task string) {
	t.Helper()
	writeMigrationFile(t, root, "epics/pay/epic.yml", "id: E-aaaaaa\nkey: pay\n"+epic)
	writeMigrationFile(t, root, "epics/pay/features/bill/feature.yml", "id: F-aaaaaa\nkey: pay/bill\n"+feature)
	writeMigrationFile(t, root, "epics/pay/features/bill/tasks/send/task.yml", "id: T-aaaaaa\nkey: pay/bill/send\n"+task)
}

func TestMigrateRefusesSharedBranchWorkWithoutAnyWrites(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, epic, feature, task string
	}{
		{"task in progress", "status: ready\n", "status: active\n", "status: in_progress\n"},
		{"task checked", "status: ready\n", "status: active\n", "status: check\n"},
		{"task in review", "status: ready\n", "status: active\n", "status: in_review\n"},
		{"feature PR", "status: ready\n", "status: active\npr: 42\n", "status: done\n"},
		{"feature review", "status: ready\n", "status: active\nreview_ref: {branch: tiller/pay/bill, head: {kind: git_commit, value: 'git:abc'}}\n", "status: done\n"},
		{"epic review", "status: ready\nreview_ref: {branch: tiller/pay, head: {kind: git_commit, value: 'git:abc'}}\n", "status: done\n", "status: done\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, version := range []int{2, 3, 4} {
				t.Run(fmt.Sprint(version), func(t *testing.T) {
					root := writeSchema2Repo(t)
					path := filepath.Join(root, ".tiller", "tiller.yml")
					data, err := os.ReadFile(path)
					require.NoError(t, err)
					data, err = store.RenderSchemaVersion(data, version)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(path, data, 0o644))
					writeMigrationLineage(t, root, tc.epic, tc.feature, tc.task)
					before := snapshotDir(t, root)
					_, err = mutate.Migrate(root, testDeps())
					var env *store.EnvError
					require.ErrorAs(t, err, &env)
					require.Equal(t, store.ReasonMigrationWorkInFlight, env.Reason)
					requireNoWrites(t, before, snapshotDir(t, root))
				})
			}
		})
	}
}

func TestMigrateDrainedActiveFeaturePreservesRemainingWorkAndHistory(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	writeMigrationFile(t, root, "tiller.yml",
		"schema_version: 4\nschema_epoch: initial\nproject: {name: X, key: x, kind: greenfield}\nowners: [{id: jc, name: JC}, {id: sam, name: Sam}]\nclaiming: {policy: single-active-owner, scope: feature}\n")
	writeMigrationLineage(t, root,
		"status: ready\nowner: jc\n",
		"status: active\nowner: jc\nbranch: tiller/pay/bill\naccepted_against: {spec: {kind: git_blob, value: 'git:abc'}}\ngates: {build: {status: pass}}\ncustom_history: {keep: true}\n",
		"status: done\nowner: sam\npr: 41\ndone_basis: ancestry\nreview_ref: {branch: tiller/pay/bill, head: {kind: git_commit, value: 'git:def'}}\n")
	writeMigrationFile(t, root, "epics/pay/features/bill/tasks/next/task.yml",
		"id: T-bbbbbb\nkey: pay/bill/next\nstatus: ready\nowner: jc\ndepends_on: [T-aaaaaa]\naccepted_against: {spec: {kind: git_blob, value: 'git:fed'}}\ncustom_history: {keep: true}\n")
	eventRel := "events/2026/06/27/EV-bbbbbb.yml"
	writeMigrationFile(t, root, eventRel, "id: EV-bbbbbb\ntype: done.task\nowner: sam\ntargets: [T-aaaaaa]\n")
	gitCommitAllM(t, root, "drained tasks under unfinished feature")
	gitRunM(t, root, "branch", "tiller/pay/bill")
	before := snapshotDir(t, root)
	inspection, err := mutate.InspectMigration(root, mutate.Deps{})
	require.NoError(t, err)
	require.Empty(t, inspection.Blockers)
	require.Empty(t, inspection.OwnershipReview.Active)
	requireNoWrites(t, before, snapshotDir(t, root))
	warnings, err := mutate.MigrationExecutionWarnings(root)
	require.NoError(t, err)
	require.Len(t, warnings, 1, "the retained legacy Git branch still conflicts with the next task branch")
	require.Contains(t, warnings[0], "tiller/pay/bill")
	requireNoWrites(t, before, snapshotDir(t, root))

	paths := []string{
		"epics/pay/features/bill/feature.yml",
		"epics/pay/features/bill/tasks/send/task.yml",
		"epics/pay/features/bill/tasks/next/task.yml",
	}
	states := make([]map[string]any, len(paths))
	for i, rel := range paths {
		data, err := os.ReadFile(filepath.Join(root, ".tiller", rel))
		require.NoError(t, err)
		require.NoError(t, yaml.Unmarshal(data, &states[i]))
	}
	gitBefore := snapshotDir(t, filepath.Join(root, ".git"))
	result, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, store.CurrentSchemaVersion, result.To)
	for i, rel := range paths {
		expected := states[i]
		if i == 0 {
			delete(expected, "owner")
			delete(expected, "branch")
		} else {
			expected["started"] = i == 1
		}
		data, err := os.ReadFile(filepath.Join(root, ".tiller", rel))
		require.NoError(t, err)
		var actual map[string]any
		require.NoError(t, yaml.Unmarshal(data, &actual))
		require.Equal(t, expected, actual, rel)
	}
	tree := load(t, root)
	require.Equal(t, "active", tree.Features["F-aaaaaa"].Status)
	require.Equal(t, []store.Contributor{{ID: "sam", Name: "Sam"}}, tree.Contributors["F-aaaaaa"])
	require.Equal(t, "jc", tree.Tasks["T-bbbbbb"].Owner)
	require.False(t, tree.Tasks["T-bbbbbb"].Started)
	requireNoWrites(t, gitBefore, snapshotDir(t, filepath.Join(root, ".git")))
	event, err := os.ReadFile(filepath.Join(root, ".tiller", eventRel))
	require.NoError(t, err)
	require.Equal(t, "id: EV-bbbbbb\ntype: done.task\nowner: sam\ntargets: [T-aaaaaa]\n", string(event))
}

func TestMigrateDrainedHistoryAndGitRefsSurviveDormantBranchCleanup(t *testing.T) {
	t.Parallel()
	root := writeSchema3Repo(t)
	path := filepath.Join(root, ".tiller", "tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data, err = store.RenderSchemaVersion(data, 4)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	review := "branch: tiller/pay\npr: 42\nreview_ref: {branch: tiller/pay, head: {kind: git_commit, value: 'git:abc'}}\n"
	writeMigrationLineage(t, root, "status: archived\n"+review, "status: done\n"+review,
		"status: done\npr: 41\nreview_ref: {branch: tiller/pay, head: {kind: git_commit, value: 'git:def'}}\ndone_basis: ancestry\n")
	writeMigrationFile(t, root, "epics/next/epic.yml", "id: E-bbbbbb\nkey: next\nstatus: ready\nbranch: tiller/next\n")
	writeMigrationFile(t, root, "epics/next/features/new/feature.yml",
		"id: F-bbbbbb\nkey: next/new\nstatus: ready\nbranch: tiller/next/new\ncustom_history: {reviewed: true}\n")
	writeMigrationFile(t, root, "epics/next/features/new/tasks/retired/task.yml",
		"id: T-bbbbbb\nkey: next/new/retired\nstatus: cancelled\npr: 43\nreview_ref: {branch: tiller/retired, head: {kind: git_commit, value: 'git:fed'}}\n")
	eventRel := "events/2026/06/27/EV-bbbbbb.yml"
	writeMigrationFile(t, root, eventRel, "id: EV-bbbbbb\ntype: done.task\nowner: jc\n")
	gitCommitAllM(t, root, "drained legacy history")
	gitRunM(t, root, "branch", "tiller/pay")
	gitRunM(t, root, "branch", "tiller/next/new")
	gitRunM(t, root, "tag", "prior-review")
	gitBefore := snapshotDir(t, filepath.Join(root, ".git"))
	history := map[string][]byte{}
	for _, rel := range []string{
		"epics/pay/epic.yml", "epics/pay/features/bill/feature.yml",
		"epics/pay/features/bill/tasks/send/task.yml",
		"epics/next/features/new/tasks/retired/task.yml", eventRel,
	} {
		history[rel], err = os.ReadFile(filepath.Join(root, ".tiller", rel))
		require.NoError(t, err)
	}

	res, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, 4, res.From)
	require.Equal(t, store.CurrentSchemaVersion, res.To)
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	var migrationEvent string
	for _, ev := range evs {
		if ev.Type == "migrate.schema" {
			eventPath, _, err := events.File(load(t, root), ev)
			require.NoError(t, err)
			migrationEvent, err = filepath.Rel(root, eventPath)
			require.NoError(t, err)
		}
	}
	require.NotEmpty(t, migrationEvent)
	require.ElementsMatch(t, []string{".tiller/tiller.yml", ".tiller/epics/next/epic.yml",
		".tiller/epics/next/features/new/feature.yml", ".tiller/epics/pay/features/bill/tasks/send/task.yml",
		".tiller/epics/next/features/new/tasks/retired/task.yml", filepath.ToSlash(migrationEvent)}, res.Files)
	for rel, want := range history {
		got, err := os.ReadFile(filepath.Join(root, ".tiller", rel))
		require.NoError(t, err)
		if strings.HasSuffix(rel, "task.yml") {
			var oldState, newState map[string]any
			require.NoError(t, yaml.Unmarshal(want, &oldState))
			require.NoError(t, yaml.Unmarshal(got, &newState))
			require.Equal(t, true, newState["started"])
			delete(newState, "started")
			require.Equal(t, oldState, newState, rel)
		} else {
			require.Equal(t, want, got, rel)
		}
	}
	requireNoWrites(t, gitBefore, snapshotDir(t, filepath.Join(root, ".git")))
	tree, err := store.Load(root)
	require.NoError(t, err)
	require.Empty(t, tree.Epics["E-bbbbbb"].Branch)
	require.Empty(t, tree.Features["F-bbbbbb"].Branch)
	require.Equal(t, "tiller/pay", tree.Features["F-aaaaaa"].Branch)
	require.Empty(t, tree.Tasks["T-aaaaaa"].Branch, "migration must not invent historical task branches")
	var dormant map[string]any
	data, err = os.ReadFile(filepath.Join(root, ".tiller/epics/next/features/new/feature.yml"))
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &dormant))
	require.Equal(t, map[string]any{"reviewed": true}, dormant["custom_history"])
}

func TestMigrateGuardsDrainDecisionAgainstConcurrentTaskChanges(t *testing.T) {
	t.Parallel()
	for _, added := range []bool{false, true} {
		t.Run(fmt.Sprintf("added=%t", added), func(t *testing.T) {
			root := writeSchema2Repo(t)
			writeMigrationLineage(t, root, "status: ready\n", "status: active\nbranch: tiller/pay/bill\n", "status: ready\n")
			gitCommitAllM(t, root, "drained tasks under unfinished feature")
			deps := testDeps()
			var before map[string]string
			deps.Mint = func(prefix string) string {
				if added {
					writeMigrationFile(t, root, "epics/pay/features/bill/tasks/another/task.yml",
						"id: T-bbbbbb\nkey: pay/bill/another\nstatus: in_progress\n")
				} else {
					writeMigrationFile(t, root, "epics/pay/features/bill/tasks/send/task.yml",
						"id: T-aaaaaa\nkey: pay/bill/send\nstatus: in_progress\n")
				}
				before = snapshotDir(t, root)
				return fixedMint(prefix)
			}
			_, err := mutate.Migrate(root, deps)
			require.Error(t, err)
			require.NotNil(t, before, "concurrent edit runs after the drain decision")
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestInspectMigrationCollectsEveryDrainBlockerWithoutWritesOrEventMint(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	writeMigrationLineage(t, root,
		"status: ready\nbranch: tiller/pay\npr: 42\n",
		"status: active\nbranch: tiller/pay/bill\n",
		"status: in_review\nreview_ref: {branch: tiller/pay/bill, head: {kind: git_commit, value: 'git:abc'}}\n")
	deps := testDeps()
	deps.Contributor = ""
	deps.Mint = func(string) string {
		t.Fatal("inspection must not mint event identities")
		return ""
	}
	before := snapshotDir(t, root)
	inspection, err := mutate.InspectMigration(root, deps)
	var refused *store.EnvError
	require.ErrorAs(t, err, &refused)
	require.Equal(t, store.ReasonMigrationWorkInFlight, refused.Reason)
	require.Equal(t, 2, inspection.From)
	require.Equal(t, store.CurrentSchemaVersion, inspection.To)
	require.Len(t, inspection.Blockers, 2)
	refs := make([]string, 0, len(inspection.Blockers))
	for _, blocker := range inspection.Blockers {
		refs = append(refs, blocker.Ref)
		require.FileExists(t, filepath.Join(root, blocker.Path))
	}
	require.Equal(t, []string{"pay", "pay/bill/send"}, refs)
	require.Equal(t, "E-aaaaaa", inspection.Blockers[0].ID)
	require.Equal(t, "ready", inspection.Blockers[0].Status)
	require.Equal(t, 42, *inspection.Blockers[0].PR)
	require.Equal(t, "tiller/pay", inspection.Blockers[0].Branch)
	require.Equal(t, "in_review", inspection.Blockers[1].Status)
	require.Equal(t, "git:abc", inspection.Blockers[1].ReviewRef.Head.Value)
	applied, err := mutate.MigrateWithCommand(root, testDeps(), "upgrade --project-only")
	require.ErrorAs(t, err, &refused)
	require.Equal(t, inspection.Blockers, applied.Blockers)
	require.Equal(t, 2, applied.To, "refusal must not claim the target schema landed")
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestInspectMigrationDoesNotRequireMutationIdentity(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	deps := testDeps()
	deps.Contributor = ""
	before := snapshotDir(t, root)
	inspection, err := mutate.InspectMigration(root, deps)
	require.NoError(t, err)
	require.Equal(t, 2, inspection.From)
	require.Equal(t, store.CurrentSchemaVersion, inspection.To)
	require.Equal(t, []string{".tiller/tiller.yml"}, inspection.Files)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestMigrateProtectsOnlyExistingTransformationInputs(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"unstaged manifest", "staged manifest", "uncommitted dormant feature", "unrelated dirty files"} {
		t.Run(kind, func(t *testing.T) {
			root := writeSchema2Repo(t)
			switch kind {
			case "unstaged manifest", "staged manifest":
				path := filepath.Join(root, ".tiller/tiller.yml")
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, append(data, []byte("# pending human edit\n")...), 0o644))
				if kind == "staged manifest" {
					gitRunM(t, root, "add", ".tiller/tiller.yml")
				}
			case "uncommitted dormant feature":
				writeMigrationLineage(t, root, "status: ready\n", "status: ready\nbranch: tiller/pay/bill\n", "status: ready\n")
			case "unrelated dirty files":
				require.NoError(t, os.WriteFile(filepath.Join(root, "app.go"), []byte("human edits\n"), 0o644))
				writeMigrationFile(t, root, "events/2026/06/26/EV-dddddd.yml", "id: EV-dddddd\ntype: done.task\n")
			}
			before := snapshotDir(t, root)
			inspection, inspectErr := mutate.InspectMigration(root, testDeps())
			requireNoWrites(t, before, snapshotDir(t, root))
			res, err := mutate.Migrate(root, testDeps())
			if kind == "unrelated dirty files" {
				require.NoError(t, inspectErr)
				require.NoError(t, err)
				require.Equal(t, mutate.MigrateOutcomeMigrated, res.Outcome)
				after := snapshotDir(t, root)
				require.Equal(t, before["app.go"], after["app.go"])
				require.Equal(t, before[".tiller/events/2026/06/26/EV-dddddd.yml"], after[".tiller/events/2026/06/26/EV-dddddd.yml"])
				require.Len(t, res.Files, len(inspection.Files)+1, "absent event output needs no committed baseline")
			} else {
				for _, refusal := range []error{inspectErr, err} {
					var env *store.EnvError
					require.ErrorAs(t, refusal, &env)
					require.Equal(t, 1, env.ExitCode())
					require.Equal(t, "migration_inputs_dirty", env.Reason)
				}
				require.Equal(t, 2, res.To)
				requireNoWrites(t, before, snapshotDir(t, root))
			}
		})
	}
}

func TestMigrateCommandProvenanceAndCurrentInspectionAreIdempotent(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	deps := testDeps()
	deps.Mint = func(string) string {
		t.Fatal("inspection minted an event")
		return ""
	}
	before := snapshotDir(t, root)
	_, err := mutate.InspectMigration(root, deps)
	require.NoError(t, err)
	requireNoWrites(t, before, snapshotDir(t, root))
	res, err := mutate.MigrateWithCommand(root, testDeps(), "upgrade --project-only")
	require.NoError(t, err)
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, evs, 1)
	require.Equal(t, "upgrade --project-only", evs[0].Command)
	require.Equal(t, "jc", evs[0].Contributor)
	eventPath, _, err := events.File(load(t, root), evs[0])
	require.NoError(t, err)
	eventRel, err := filepath.Rel(root, eventPath)
	require.NoError(t, err)
	require.Contains(t, res.Files, filepath.ToSlash(eventRel))
	before = snapshotDir(t, root)
	deps.Contributor = ""
	inspection, err := mutate.InspectMigration(root, deps)
	require.NoError(t, err, "current schema has no actor prerequisite")
	require.Equal(t, store.CurrentSchemaVersion, inspection.From)
	require.Empty(t, inspection.Files)
	res, err = mutate.MigrateWithCommand(root, deps, "upgrade --project-only")
	require.NoError(t, err)
	require.Equal(t, mutate.MigrateOutcomeCurrent, res.Outcome)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestMigrateApplyReplansAfterInspection(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	writeMigrationLineage(t, root, "status: ready\n", "status: active\n", "status: ready\n")
	gitCommitAllM(t, root, "drained lineage")
	_, err := mutate.InspectMigration(root, testDeps())
	require.NoError(t, err)
	writeMigrationFile(t, root, "epics/pay/features/bill/tasks/send/task.yml",
		"id: T-aaaaaa\nkey: pay/bill/send\nstatus: in_progress\n")
	before := snapshotDir(t, root)
	res, err := mutate.MigrateWithCommand(root, testDeps(), "upgrade --project-only")
	var refused *store.EnvError
	require.ErrorAs(t, err, &refused)
	require.Equal(t, store.ReasonMigrationWorkInFlight, refused.Reason)
	require.Equal(t, "pay/bill/send", res.Blockers[0].Ref)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestMigrateRefusesEventCollisionWithoutOverwritingHistory(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	writeMigrationFile(t, root, "events/2026/06/27/EV-cccccc.yml", "id: EV-cccccc\ntype: done.task\n")
	deps := testDeps()
	deps.Mint = func(string) string { return "EV-cccccc" }
	before := snapshotDir(t, root)
	res, err := mutate.Migrate(root, deps)
	var refused *store.EnvError
	require.ErrorAs(t, err, &refused)
	require.Equal(t, store.ReasonMutationStale, refused.Reason)
	require.Contains(t, res.Files, ".tiller/events/2026/06/27/EV-cccccc.yml")
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestMigrateRevalidatesGitIndexBeforeSourceCommit(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	deps := testDeps()
	var before map[string]string
	deps.Mint = func(prefix string) string {
		path := filepath.Join(root, ".tiller/tiller.yml")
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, append(append([]byte(nil), data...), []byte("# concurrent index edit\n")...), 0o644))
		gitRunM(t, root, "add", ".tiller/tiller.yml")
		require.NoError(t, os.WriteFile(path, data, 0o644))
		before = snapshotDir(t, root)
		return fixedMint(prefix)
	}
	_, err := mutate.Migrate(root, deps)
	var refused *store.EnvError
	require.ErrorAs(t, err, &refused)
	require.Equal(t, "migration_inputs_dirty", refused.Reason)
	require.NotNil(t, before)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestMigrationExecutionWarningsPreserveLegacyRefsAndState(t *testing.T) {
	t.Parallel()
	for _, schema := range []int{4, store.CurrentSchemaVersion} {
		t.Run(fmt.Sprint(schema), func(t *testing.T) {
			root := writeSchema2Repo(t)
			path := filepath.Join(root, ".tiller/tiller.yml")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			data, err = store.RenderSchemaVersion(data, schema)
			require.NoError(t, err)
			if schema == store.CurrentSchemaVersion {
				data = []byte(strings.Replace(string(data), "\nowners:", "\ncontributors:", 1))
			}
			require.NoError(t, os.WriteFile(path, data, 0o644))
			writeMigrationLineage(t, root, "status: ready\n", "status: ready\n", "status: proposed\n")
			gitRunM(t, root, "branch", "tiller/pay/bill")
			before := snapshotDir(t, root)
			warnings, err := mutate.MigrationExecutionWarnings(root)
			require.NoError(t, err)
			require.Len(t, warnings, 1)
			require.Contains(t, warnings[0], "pay/bill/send")
			require.Contains(t, warnings[0], "tiller/pay/bill")
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestMigrateRefusesUnresolvedGitConflictsOutsideItsInputs(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	app := filepath.Join(root, "application.txt")
	require.NoError(t, os.WriteFile(app, []byte("base\n"), 0o644))
	gitCommitAllM(t, root, "application baseline")
	gitRunM(t, root, "checkout", "-b", "migration-left")
	require.NoError(t, os.WriteFile(app, []byte("left\n"), 0o644))
	gitCommitAllM(t, root, "left edit")
	gitRunM(t, root, "checkout", "-b", "migration-right", "HEAD~1")
	require.NoError(t, os.WriteFile(app, []byte("right\n"), 0o644))
	gitCommitAllM(t, root, "right edit")
	merge := exec.Command("git", "merge", "migration-left")
	merge.Dir = root
	_, err := merge.CombinedOutput()
	require.Error(t, err, "the fixture must retain an unresolved conflict")
	before := snapshotDir(t, root)
	_, inspectErr := mutate.InspectMigration(root, testDeps())
	_, applyErr := mutate.Migrate(root, testDeps())
	for _, err := range []error{inspectErr, applyErr} {
		var refused *store.EnvError
		require.ErrorAs(t, err, &refused)
		require.Equal(t, "migration_git_conflicts", refused.Reason)
		require.Equal(t, 1, refused.ExitCode())
	}
	requireNoWrites(t, before, snapshotDir(t, root))
}

// An older binary could create an epic keyed by a shelf folder that a later
// schema reserves. Carrying it forward would make its features read as shelf
// refs, so migration refuses and names the rename only the older binary can do.
func TestMigrateRefusesAnEpicKeyedByAReservedShelf(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status, remedy string
	}{
		{"draft", "rename epic tweaks --key <new-key>"},
		{"ready", "no tiller command can rename an epic past draft"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			root := writeSchema2Repo(t)
			path := filepath.Join(root, ".tiller", "tiller.yml")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			data, err = store.RenderSchemaVersion(data, 7)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0o644))
			writeMigrationFile(t, root, "epics/tweaks/epic.yml", "id: E-aaaaaa\nkey: tweaks\nstatus: "+tc.status+"\n")
			writeMigrationFile(t, root, "epics/tweaks/features/foo/feature.yml", "id: F-aaaaaa\nkey: tweaks/foo\nstatus: draft\n")
			gitCommitAllM(t, root, "schema seven with a tweaks epic")
			before := snapshotDir(t, root)
			_, inspectErr := mutate.InspectMigration(root, testDeps())
			_, applyErr := mutate.Migrate(root, testDeps())
			for _, err := range []error{inspectErr, applyErr} {
				var refused *store.EnvError
				require.ErrorAs(t, err, &refused)
				require.Equal(t, store.ReasonMigrationReservedEpicKey, refused.Reason)
				require.Equal(t, 3, refused.ExitCode())
				require.Contains(t, refused.Msg, "epic tweaks (E-aaaaaa)")
				require.Contains(t, refused.Msg, tc.remedy)
			}
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}
