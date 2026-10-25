package mutate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func schemaEightContributorFixture(t *testing.T) string {
	t.Helper()
	root := writeSchema2Repo(t)
	writeMigrationFile(t, root, "tiller.yml", "# human configuration\nschema_version: 8 # preserve stamp comment\nschema_epoch: adopted-era\nproject: {name: X, key: x, kind: greenfield}\n\n'owners': # preserve roster comment\n  - {id: jc, name: JC}\n  - {id: sam, name: Sam}\ncustom: {owners: [leave, nested, keys]}\n")
	writeMigrationLineage(t, root,
		"status: ready\nbranch: tiller/reviews/epics/pay\npr: 71\n",
		"status: active\nbranch: tiller/reviews/features/pay/bill\npr: 72\n",
		"status: in_review\nstarted: true\nowner: jc\nbranch: tiller/pay/bill/send\npr: 73\nreview_ref: {branch: tiller/pay/bill/send, head: {kind: git_commit, value: 'git:abcdef'}}\naccepted_against: {spec: {kind: git_blob, value: 'git:123456'}}\ngates: {tests: {status: pass}}\nevidence: [{kind: test, ref: test.log}]\n")
	writeMigrationFile(t, root, "epics/pay/features/bill/tasks/finished/task.yml", "id: T-bbbbbb\nkey: pay/bill/finished\nstatus: done\nstarted: true\nowner: sam\nbranch: retained/finished\npr: 74\n")
	writeMigrationFile(t, root, "bugs/fixed/bug.yml", "id: B-aaaaaa\nkey: bugs/fixed\nstatus: done\nstarted: true\nowner: sam\ndone_basis: ancestry\n")
	writeMigrationFile(t, root, "chores/live/chore.yml", "id: CH-aaaaaa\nkey: chores/live\nstatus: check\nstarted: true\nowner: jc\nbranch: tiller/chores/live\n")
	writeMigrationFile(t, root, "tweaks/dropped/tweak.yml", "id: TW-aaaaaa\nkey: tweaks/dropped\nstatus: cancelled\nstarted: true\nowner: sam\n")
	writeMigrationFile(t, root, "events/2026/06/26/EV-eeeeee.yml", "id: EV-eeeeee\ntype: owner.add\nowner: sam\ncommand: owner add sam\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".tiller/identity.yml\n.tiller/.cache/\n"), 0o644))
	gitCommitAllM(t, root, "schema eight active and terminal claims")
	gitRunM(t, root, "branch", "tiller/pay/bill/send")
	return root
}

func TestSchemaEightMigrationPreservesClaimsHistoryAndLocalIdentity(t *testing.T) {
	t.Parallel()
	root := schemaEightContributorFixture(t)
	identity := "# local checkout identity\nowner: 'sam' # keep selected id\ncustom: {owner: untouched}\n"
	writeMigrationFile(t, root, "identity.yml", identity)
	manifestBefore, err := os.ReadFile(filepath.Join(root, ".tiller/tiller.yml"))
	require.NoError(t, err)
	before := snapshotDir(t, root)
	inspection, err := mutate.InspectMigration(root, testDeps())
	require.NoError(t, err)
	require.Empty(t, inspection.Blockers, "the vocabulary rename must not drain active work or open aggregate reviews")
	require.Nil(t, inspection.OwnershipReview)
	require.Contains(t, inspection.Files, ".tiller/identity.yml")
	requireNoWrites(t, before, snapshotDir(t, root))

	result, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, 8, result.From)
	require.Equal(t, store.CurrentSchemaVersion, result.To)
	require.Len(t, result.Files, 3, "only manifest, local identity, and new event change")
	after := snapshotDir(t, root)
	for path, content := range before {
		if path == ".tiller/tiller.yml" || path == ".tiller/identity.yml" || strings.HasPrefix(path, ".tiller/.cache/") {
			continue
		}
		require.Equal(t, content, after[path], path)
	}
	manifestAfter, err := os.ReadFile(filepath.Join(root, ".tiller/tiller.yml"))
	require.NoError(t, err)
	require.Equal(t, strings.Replace(strings.Replace(string(manifestBefore), "schema_version: 8", fmt.Sprintf("schema_version: %d", store.CurrentSchemaVersion), 1), "'owners':", "'contributors':", 1), string(manifestAfter))
	identityAfter, err := os.ReadFile(filepath.Join(root, ".tiller/identity.yml"))
	require.NoError(t, err)
	require.Equal(t, strings.Replace(identity, "\nowner:", "\ncontributor:", 1), string(identityAfter))
	tree, err := store.Load(root)
	require.NoError(t, err)
	require.Equal(t, "adopted-era", tree.Manifest.SchemaEpoch)
	require.Equal(t, []store.Contributor{{ID: "jc", Name: "JC"}, {ID: "sam", Name: "Sam"}}, tree.Manifest.Contributors)
	require.Equal(t, "jc", tree.Tasks["T-aaaaaa"].Owner)
	require.Equal(t, "in_review", tree.Tasks["T-aaaaaa"].Status)
	require.True(t, tree.Tasks["T-aaaaaa"].Started)
	require.Equal(t, "test.log", tree.Tasks["T-aaaaaa"].Evidence[0].Ref)
	require.Equal(t, "sam", tree.Tasks["T-bbbbbb"].Owner)
	require.Equal(t, "sam", tree.Tweaks["TW-aaaaaa"].Owner)
	selected, err := store.ReadLocalIdentity(root)
	require.NoError(t, err)
	require.Equal(t, "sam", selected, "migration actor must not replace the checkout's selected contributor")
}

func TestSchemaEightMigrationRejectsUnsafeIdentityWithoutWrites(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"malformed", "mixed vocabulary", "invalid id shape", "symlink", "dirty manifest", "unknown actor"} {
		t.Run(kind, func(t *testing.T) {
			root := schemaEightContributorFixture(t)
			deps := testDeps()
			identity := "owner: sam\n"
			switch kind {
			case "malformed":
				identity = "owner: [\n"
			case "mixed vocabulary":
				identity = "owner: sam\ncontributor: jc\n"
			case "invalid id shape":
				identity = "owner: [sam]\n"
			case "dirty manifest":
				manifestPath := filepath.Join(root, ".tiller/tiller.yml")
				data, err := os.ReadFile(manifestPath)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(manifestPath, append(data, []byte("# uncommitted edit\n")...), 0o644))
			case "unknown actor":
				deps.Contributor = "unknown"
			}
			if kind == "symlink" {
				target := filepath.Join(t.TempDir(), "identity.yml")
				require.NoError(t, os.WriteFile(target, []byte(identity), 0o644))
				require.NoError(t, os.Symlink(target, filepath.Join(root, ".tiller/identity.yml")))
			} else {
				writeMigrationFile(t, root, "identity.yml", identity)
			}
			before := snapshotDir(t, root)
			_, inspectErr := mutate.InspectMigration(root, deps)
			_, applyErr := mutate.Migrate(root, deps)
			require.Error(t, inspectErr)
			require.Error(t, applyErr)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestSchemaEightMigrationGuardsConcurrentLocalIdentityEdits(t *testing.T) {
	t.Parallel()
	for _, initiallyPresent := range []bool{false, true} {
		t.Run(map[bool]string{false: "appears", true: "changes"}[initiallyPresent], func(t *testing.T) {
			root := schemaEightContributorFixture(t)
			if initiallyPresent {
				writeMigrationFile(t, root, "identity.yml", "owner: sam\n")
			}
			deps := testDeps()
			var before map[string]string
			deps.Mint = func(prefix string) string {
				writeMigrationFile(t, root, "identity.yml", "owner: jc # concurrent selection\n")
				before = snapshotDir(t, root)
				return fixedMint(prefix)
			}
			_, err := mutate.Migrate(root, deps)
			var refusal *store.EnvError
			require.ErrorAs(t, err, &refusal)
			require.Equal(t, store.ReasonMutationStale, refusal.Reason)
			require.NotNil(t, before)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestOlderMigrationChainCarriesRosterAndLocalIdentityToSchemaNine(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	writeMigrationFile(t, root, "identity.yml", "owner: jc # selected before schema nine\n")
	result, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	require.Equal(t, 2, result.From)
	require.Equal(t, store.CurrentSchemaVersion, result.To)
	tree, err := store.Load(root)
	require.NoError(t, err)
	require.Equal(t, []store.Contributor{{ID: "jc", Name: "JC"}}, tree.Manifest.Contributors)
	selected, err := store.ReadLocalIdentity(root)
	require.NoError(t, err)
	require.Equal(t, "jc", selected)
}

func TestCurrentSchemaMigrationRejectsLegacyRosterWithoutWrites(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	writeMigrationFile(t, root, "tiller.yml", "schema_version: 9\nowners: [{id: jc, name: JC}]\n")
	before := snapshotDir(t, root)
	_, err := mutate.Migrate(root, testDeps())
	require.Error(t, err)
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestSchemaEightInspectionReportsNullQuestionEntry covers the older-schema path,
// which loads the full tree: a null question in the ledger is a normal error that
// names the ledger entry, and the failed inspection writes nothing.
func TestSchemaEightInspectionReportsNullQuestionEntry(t *testing.T) {
	t.Parallel()
	root := schemaEightContributorFixture(t)
	writeMigrationFile(t, root, "decisions/open-questions.yml", "questions:\n  - null\n")
	before := snapshotDir(t, root)

	require.NotPanics(t, func() {
		_, err := mutate.InspectMigration(root, testDeps())
		require.Error(t, err)
		require.Contains(t, err.Error(), "open-questions.yml[0]")
	})
	requireNoWrites(t, before, snapshotDir(t, root))
}
