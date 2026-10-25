package mutate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func ownershipMigrationFixture(t *testing.T) string {
	t.Helper()
	root := writeSchema2Repo(t)
	writeMigrationFile(t, root, "tiller.yml", "# retain human configuration\nschema_version: 5\nschema_epoch: initial\nproject: {name: X, key: x, kind: greenfield}\nowners: [{id: jc, name: JC}, {id: sam, name: Sam}, {id: lee, name: Lee}]\nclaiming:\n  policy: single-active-owner\n  scope: feature\ninputs: {docs: [design/]}\ncustom_setting: {preserve: this}\n")
	writeMigrationLineage(t, root,
		"status: ready\nowner: lee\nbranch: tiller/reviews/epics/pay\npr: 51\n",
		"status: ready\nowner: jc\nbranch: tiller/reviews/features/pay/bill\npr: 52\ncustom_history: {keep: [a, b]}\n",
		"status: in_review\nowner: jc\nbranch: tiller/pay/bill/send\npr: 53\nreview_ref: {branch: tiller/pay/bill/send, head: {kind: git_commit, value: 'git:abcdef'}}\naccepted_against: {spec: {kind: git_blob, value: 'git:123456', path: epics/pay/features/bill/tasks/send/task.md}}\ngates: {tests: {status: pass}}\nevidence: [{kind: test, ref: test.log}]\ncustom_history: {original: true}\n")
	writeMigrationFile(t, root, "epics/pay/features/bill/tasks/reserved/task.yml", "id: T-bbbbbb\nkey: pay/bill/reserved\nstatus: ready\nowner: lee\n")
	writeMigrationFile(t, root, "bugs/fix/bug.yml", "id: B-aaaaaa\nkey: bugs/fix\nstatus: check\nowner: sam\nbranch: tiller/bugs/fix\nattached_to: F-aaaaaa\n")
	writeMigrationFile(t, root, "chores/update/chore.yml", "id: CH-aaaaaa\nkey: chores/update\nstatus: in_progress\nowner: lee\nbranch: tiller/chores/update\n")
	gitCommitAllM(t, root, "schema five active work")
	return root
}

func reviewedOwnership(t *testing.T, root string) *mutate.OwnershipReview {
	t.Helper()
	inspection, err := mutate.InspectMigration(root, mutate.Deps{})
	require.NoError(t, err)
	review := inspection.OwnershipReview
	require.NotNil(t, review)
	for i := range review.Active {
		review.Active[i].ConfirmedOwner = review.Active[i].ObservedOwner
	}
	return review
}

func TestSchemaFiveOwnershipReviewPreservesOpenReviewsAndCorrections(t *testing.T) {
	t.Parallel()
	root := ownershipMigrationFixture(t)
	before := snapshotDir(t, root)
	inspection, err := mutate.InspectMigration(root, mutate.Deps{Mint: func(string) string { t.Fatal("inspection minted an event"); return "" }})
	require.NoError(t, err)
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, []mutate.OwnershipAssignment{
		{ID: "B-aaaaaa", Noun: "bug", Ref: "bugs/fix", ObservedOwner: "sam"},
		{ID: "CH-aaaaaa", Noun: "chore", Ref: "chores/update", ObservedOwner: "lee"},
		{ID: "T-aaaaaa", Noun: "task", Ref: "pay/bill/send", ObservedOwner: "jc"},
	}, inspection.OwnershipReview.Active)
	require.Contains(t, strings.Join(inspection.Notes, "\n"), "ready to active")
	_, err = mutate.Migrate(root, testDeps())
	var refusal *store.EnvError
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, store.ReasonOwnershipReviewRequired, refusal.Reason)
	requireNoWrites(t, before, snapshotDir(t, root))
	review := reviewedOwnership(t, root)
	for i := range review.Active {
		if review.Active[i].ID == "T-aaaaaa" {
			review.Active[i].ConfirmedOwner = "sam"
		}
	}
	result, err := mutate.MigrateWithCommand(root, testDeps(), "upgrade --project-only", review)
	require.NoError(t, err)
	require.Equal(t, store.CurrentSchemaVersion, result.To)
	tree := load(t, root)
	task := tree.Tasks["T-aaaaaa"]
	require.True(t, task.Started)
	require.Equal(t, "sam", task.Owner)
	require.Equal(t, "in_review", task.Status)
	require.Equal(t, "tiller/pay/bill/send", task.Branch)
	require.Equal(t, 53, *task.PR)
	require.Equal(t, "git:abcdef", task.ReviewRef.Head.Value)
	require.Equal(t, "git:123456", task.AcceptedAgainst.Spec.Value)
	require.Equal(t, "pass", task.Gates["tests"].Status)
	require.Equal(t, "test.log", task.Evidence[0].Ref)
	require.Equal(t, "active", tree.Features["F-aaaaaa"].Status)
	require.Equal(t, 52, *tree.Features["F-aaaaaa"].PR)
	require.Equal(t, "tiller/reviews/features/pay/bill", tree.Features["F-aaaaaa"].Branch)
	require.Equal(t, 51, *tree.Epics["E-aaaaaa"].PR)
	require.False(t, tree.Tasks["T-bbbbbb"].Started)
	require.Equal(t, "lee", tree.Tasks["T-bbbbbb"].Owner)
	require.True(t, tree.Bugs["B-aaaaaa"].Started)
	require.True(t, tree.Chores["CH-aaaaaa"].Started)
	require.Equal(t, []store.Contributor{{ID: "sam", Name: "Sam"}}, tree.Contributors["F-aaaaaa"])
	for _, rel := range []string{"epics/pay/epic.yml", "epics/pay/features/bill/feature.yml"} {
		data, err := os.ReadFile(filepath.Join(root, ".tiller", rel))
		require.NoError(t, err)
		var fields map[string]any
		require.NoError(t, yaml.Unmarshal(data, &fields))
		require.NotContains(t, fields, "owner")
	}
	manifest, err := os.ReadFile(filepath.Join(root, ".tiller/tiller.yml"))
	require.NoError(t, err)
	require.Contains(t, string(manifest), "custom_setting: {preserve: this}")
	require.NotContains(t, string(manifest), "claiming:")
	evs, err := events.List(tree)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	require.Equal(t, "upgrade --project-only", evs[0].Command)
	require.Equal(t, map[string]any{"E-aaaaaa": "lee", "F-aaaaaa": "jc"}, evs[0].Before["ancestor_owners"])
	require.Contains(t, evs[0].After, "owner_corrections")
	require.Contains(t, evs[0].After, "ownership_review")
}

func TestOwnershipReviewRejectsIncompleteDuplicateAndConflictingInput(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"unconfirmed", "incomplete", "duplicate", "unknown owner", "observed owner", "noun", "source schema", "snapshot", "source file"} {
		t.Run(kind, func(t *testing.T) {
			root := ownershipMigrationFixture(t)
			review := reviewedOwnership(t, root)
			switch kind {
			case "unconfirmed":
				review.Active[0].ConfirmedOwner = ""
			case "incomplete":
				review.Active = review.Active[:len(review.Active)-1]
			case "duplicate":
				review.Active[1] = review.Active[0]
			case "unknown owner":
				review.Active[0].ConfirmedOwner = "outsider"
			case "observed owner":
				review.Active[0].ObservedOwner = "lee"
			case "noun":
				review.Active[0].Noun = "task"
			case "source schema":
				review.SourceSchema = 4
			case "snapshot":
				review.Snapshot = "other"
			case "source file":
				review.Sources[0].SHA256 = "other"
			}
			before := snapshotDir(t, root)
			_, err := mutate.MigrateWithCommand(root, testDeps(), "migrate", review)
			var refusal *store.EnvError
			require.ErrorAs(t, err, &refusal)
			require.Equal(t, store.ReasonOwnershipReviewInvalid, refusal.Reason)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestOwnershipReviewPinsRosterInventoryAndState(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"roster", "manifest", "owner", "status", "added task", "added bug", "history"} {
		t.Run(kind, func(t *testing.T) {
			root := ownershipMigrationFixture(t)
			review := reviewedOwnership(t, root)
			switch kind {
			case "roster", "manifest":
				path := filepath.Join(root, ".tiller/tiller.yml")
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				if kind == "roster" {
					data = []byte(strings.Replace(string(data), "name: Sam", "name: Samuel", 1))
				} else {
					data = append(data, []byte("# new configuration review\n")...)
				}
				require.NoError(t, os.WriteFile(path, data, 0o644))
			case "owner", "status":
				path := filepath.Join(root, ".tiller/bugs/fix/bug.yml")
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				if kind == "owner" {
					data = []byte(strings.Replace(string(data), "owner: sam", "owner: jc", 1))
				} else {
					data = []byte(strings.Replace(string(data), "status: check", "status: in_progress", 1))
				}
				require.NoError(t, os.WriteFile(path, data, 0o644))
			case "added task":
				writeMigrationFile(t, root, "epics/pay/features/bill/tasks/new/task.yml", "id: T-cccccc\nkey: pay/bill/new\nstatus: in_progress\nowner: lee\n")
			case "added bug":
				writeMigrationFile(t, root, "bugs/new/bug.yml", "id: B-bbbbbb\nkey: bugs/new\nstatus: in_progress\nowner: jc\n")
			case "history":
				writeMigrationFile(t, root, "events/2026/09/19/EV-zzzzzz.yml", "id: EV-zzzzzz\ntype: start.task\ntargets: [T-aaaaaa]\n")
			}
			gitCommitAllM(t, root, "changed reviewed snapshot")
			before := snapshotDir(t, root)
			_, err := mutate.MigrateWithCommand(root, testDeps(), "migrate", review)
			var refusal *store.EnvError
			require.ErrorAs(t, err, &refusal)
			require.Equal(t, store.ReasonOwnershipReviewInvalid, refusal.Reason)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestOwnershipMigrationKeepsHistoricalGapsAndMalformedEvents(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	writeMigrationFile(t, root, "tiller.yml", "schema_version: 5\nowners: [{id: jc, name: JC}]\n")
	writeMigrationLineage(t, root, "status: ready\nowner: jc\n", "status: active\nowner: jc\n", "status: done\ncustom_history: {unrecognized: true}\n")
	for _, row := range []struct{ key, id, suffix string }{
		{"never", "T-bbbbbb", ""}, {"event", "T-cccccc", "owner: retired\n"}, {"branch", "T-dddddd", "branch: retained/task\n"},
	} {
		writeMigrationFile(t, root, "epics/pay/features/bill/tasks/"+row.key+"/task.yml", fmt.Sprintf("id: %s\nkey: pay/bill/%s\nstatus: cancelled\n%s", row.id, row.key, row.suffix))
	}
	writeMigrationFile(t, root, "bugs/carried/bug.yml", "id: B-aaaaaa\nkey: bugs/carried\nstatus: done\ndone_basis: carried\nresolved_by: T-aaaaaa\n")
	writeMigrationFile(t, root, "events/2026/09/18/EV-aaaaaa.yml", "id: EV-aaaaaa\ntype: start.task\ntargets: [T-cccccc]\n")
	malformed := "id: [this history cannot be parsed\n"
	writeMigrationFile(t, root, "events/2026/09/18/EV-bbbbbb.yml", malformed)
	gitCommitAllM(t, root, "legacy history with gaps")
	_, err := mutate.Migrate(root, testDeps())
	require.NoError(t, err)
	tree := load(t, root)
	require.True(t, tree.Tasks["T-aaaaaa"].Started)
	require.Empty(t, tree.Tasks["T-aaaaaa"].Owner)
	require.False(t, tree.Tasks["T-bbbbbb"].Started)
	require.True(t, tree.Tasks["T-cccccc"].Started)
	require.True(t, tree.Tasks["T-dddddd"].Started)
	require.False(t, tree.Bugs["B-aaaaaa"].Started)
	require.Equal(t, []store.Contributor{{ID: "retired", Name: "retired"}}, tree.Contributors["F-aaaaaa"])
	data, err := os.ReadFile(filepath.Join(root, ".tiller/events/2026/09/18/EV-bbbbbb.yml"))
	require.NoError(t, err)
	require.Equal(t, malformed, string(data))
	data, err = os.ReadFile(filepath.Join(root, ".tiller/epics/pay/features/bill/tasks/send/task.yml"))
	require.NoError(t, err)
	var state map[string]any
	require.NoError(t, yaml.Unmarshal(data, &state))
	require.Equal(t, map[string]any{"unrecognized": true}, state["custom_history"])
}

func TestOwnershipMigrationGuardsNewShelfWorkAfterReview(t *testing.T) {
	t.Parallel()
	root := ownershipMigrationFixture(t)
	review := reviewedOwnership(t, root)
	deps := testDeps()
	var raced map[string]string
	deps.Mint = func(prefix string) string {
		writeMigrationFile(t, root, "chores/new/chore.yml", "id: CH-bbbbbb\nkey: chores/new\nstatus: in_progress\nowner: jc\n")
		raced = snapshotDir(t, root)
		return fixedMint(prefix)
	}
	_, err := mutate.MigrateWithCommand(root, deps, "migrate", review)
	require.Error(t, err)
	require.NotNil(t, raced)
	requireNoWrites(t, raced, snapshotDir(t, root))
}

func TestOwnershipMigrationPreservesAliasedHistoricalOwners(t *testing.T) {
	t.Parallel()
	root := ownershipMigrationFixture(t)
	featurePath := filepath.Join(root, ".tiller/epics/pay/features/bill/feature.yml")
	taskPath := filepath.Join(root, ".tiller/epics/pay/features/bill/tasks/send/task.yml")
	for _, path := range []string{featurePath, taskPath} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		data = []byte(strings.Replace(string(data), "owner: jc", "owner: &original jc", 1) + "aliased_history: {previous_owner: *original}\n")
		require.NoError(t, os.WriteFile(path, data, 0o644))
	}
	gitCommitAllM(t, root, "aliased historical ownership")
	review := reviewedOwnership(t, root)
	for i := range review.Active {
		if review.Active[i].ID == "T-aaaaaa" {
			review.Active[i].ConfirmedOwner = "sam"
		}
	}
	_, err := mutate.MigrateWithCommand(root, testDeps(), "migrate", review)
	require.NoError(t, err)
	for _, path := range []string{featurePath, taskPath} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var state map[string]any
		require.NoError(t, yaml.Unmarshal(data, &state))
		require.Equal(t, map[string]any{"previous_owner": "jc"}, state["aliased_history"])
	}
	require.Equal(t, "sam", load(t, root).Tasks["T-aaaaaa"].Owner)
}

func TestMigrationInspectionRefusesIncompatibleInactiveLineage(t *testing.T) {
	t.Parallel()
	root := writeSchema2Repo(t)
	writeMigrationFile(t, root, "tiller.yml", "schema_version: 5\nowners: [{id: jc, name: JC}]\n")
	writeMigrationLineage(t, root, "status: ready\n", "status: active\n",
		"status: cancelled\nsuperseded_by: T-bbbbbb\n")
	gitCommitAllM(t, root, "broken historical replacement")
	before := snapshotDir(t, root)
	_, err := mutate.InspectMigration(root, testDeps())
	require.ErrorContains(t, err, "successor")
	requireNoWrites(t, before, snapshotDir(t, root))
}
