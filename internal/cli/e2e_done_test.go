package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These scenarios drive the command flow against real Git repositories. Branch
// identities and accepted bases come from start and approve, never hand-set YAML.

// soleFeatureBranch reads the one feature's recorded branch off disk, so a test
// can assert start recorded it without parsing the YAML by hand.
func soleFeatureBranch(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Len(t, tree.Features, 1)
	for _, f := range tree.Features {
		return f.Branch
	}
	return ""
}

// hasEventCLI reports whether the tree's event stream carries an event of the type.
func hasEventCLI(t *testing.T, tree *store.Tree, typ string) bool {
	t.Helper()
	evs, err := events.List(tree)
	require.NoError(t, err)
	for _, e := range evs {
		if e.Type == typ {
			return true
		}
	}
	return false
}

// TestEndToEndTaskAndFeatureReachDoneCLI drives the whole v2 spine through the
// real CLI at every grain in one run: decompose, the epic gate, the merged
// specify stage, the one plan gate that moves the feature AND its initial tasks,
// the work loop on a task, a local --no-ff merge, and sync. It asserts the three
// properties no single-grain test can hold together:
//
//  1. ONE `approve feature` moves the feature and its initial task and pins a
//     basis on each (cli-surface.md, "Specifying a feature": "it moves the
//     feature to ready, flips every initial proposed task to ready in the same
//     act, and records the active-work contract" at both grains).
//  2. The merge crosses the TASK, and only then does the feature roll up, and
//     only then the epic (cli-surface.md, "The done crossing").
//  3. Every done above a task is arithmetic, recorded as `done_basis: rollup`,
//     while the task's own records the observation that carried it.
//
// The real command flow records task.Branch and accepted_against.
func TestEndToEndTaskAndFeatureReachDoneCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")

	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)

	// The specify stage is one act with one output: the specification, the
	// acceptance contract, and the task explosion, all written before the gate
	// validates the whole of it. No hand-set fixtures.
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")

	_, _, code = execCLI(t, "spec", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)
	require.Equal(t, "proposed", soleTaskStatus(t, tid), "the explosion lands proposed, ahead of the plan gate")

	// The plan gate. Its human output names the tasks it carried, and the tree
	// afterwards is the real proof: one command, two nodes moved, a basis pinned
	// on each. The task assertions are what make this the plan gate rather than a
	// feature-only approve; without them a build that quietly stopped flipping
	// tasks would still pass every line above.
	out, _, code := execCLI(t, "approve", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "1 task(s) in the plan moved to ready with it")

	planned, err := store.Load(dir)
	require.NoError(t, err)
	feat := planned.Features[soleFeatureID(t)]
	require.Equal(t, "ready", feat.Status)
	require.Equal(t, "ready", planned.Tasks[tid].Status, "the initial task rode the feature's own gate to ready")
	require.NotEmpty(t, feat.AcceptedAgainst.Spec.Value, "the gate pinned the feature's spec")
	require.NotEmpty(t, feat.AcceptedAgainst.Eval.Value, "and its acceptance contract")
	require.NotNil(t, planned.Tasks[tid].AcceptedAgainst, "and the task's own implementation spec")
	require.NotEmpty(t, planned.Tasks[tid].AcceptedAgainst.Spec.Value)

	claimTaskAsOwner(t, tid, "jc")

	require.Empty(t, soleFeatureBranch(t), "task grain has no shared feature branch")

	// The work loop on the task the plan gate already made ready.
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	branch := checkoutTaskBranchCLI(t, tid)
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, 0, code)

	// The feature-level gates must be checked for the rollup to fire.
	_, _, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)

	// Nothing above the task has crossed yet, and saying so is the ordering
	// half of property 2: without this the "done" assertions below would hold
	// just as well in a build that crossed the feature and the epic early, on
	// the plan gate or on check, rather than on the merge sync is about to
	// observe.
	beforeMerge, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, "in_review", beforeMerge.Tasks[tid].Status)
	require.Equal(t, "active", beforeMerge.Features[soleFeatureID(t)].Status, "the feature does not roll up before the merge")
	require.Equal(t, "ready", soleEpic(t, beforeMerge).Status, "and neither does the epic")

	// Run sync HERE, on the unmerged tree, and require it to move nothing.
	//
	// The three lines above prove only that nothing has crossed at this moment.
	// They cannot reach the rollup predicates, because the arc's real sync runs
	// after the merge, by which point the children genuinely are done and a
	// predicate that never looked at them answers the same as one that did. This
	// call is the arm that reaches them: the feature's gates are recorded and its
	// one task is still in_review, so featureRollupReady's "every non-cancelled
	// task is done" clause, and epicRollupReady's "every feature is done" clause
	// behind it, are the only things standing between this run and a crossing.
	// The real-world failure is a human running `tiller sync` on the integration
	// branch and getting a feature marked done over work still under review.
	//
	// The assertion is on the crossings the run RECORDED, not on the tree it
	// left behind, and that distinction is load-bearing rather than stylistic. A
	// loosened featureRollupReady crosses the feature here and then the same pass
	// reopens it, because the reopen walk sees the still-in-review task
	// underneath; the tree afterwards reads "active" either way. What the run did
	// is still wrong: it wrote a sync.feature crossing event mid-pass. Only the
	// recorded crossings show it, so that is what this reads.
	preMerge, _, code := execCLI(t, "sync", "--into", "main", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Empty(t, syncCrossings(t, preMerge),
		"with the task still in review and the branch unmerged, sync must record no crossing at any grain")

	stillOpen, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, "in_review", stillOpen.Tasks[tid].Status, "an unmerged branch crosses no task")
	require.Equal(t, "active", stillOpen.Features[soleFeatureID(t)].Status, "and rolls up no feature over it")
	require.Equal(t, "ready", soleEpic(t, stillOpen).Status, "and no epic behind that")

	// Build the local merge topology the agent never performs through Tiller: commit
	// the work, branch, commit on the branch, and merge --no-ff into the default branch.
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "lay tree and task work")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "work.txt"), []byte("work\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "feature work")
	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "merge", "--no-ff", "-m", "merge feature", branch)

	// sync observes the merge by ancestry and records the crossings.
	out, _, code = execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"status\": \"done\"")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, "done", tree.Tasks[tid].Status, "the merged task crosses to done")
	require.Equal(t, "ancestry", tree.Tasks[tid].DoneBasis, "the --no-ff merge commit is what the task's crossing observed")
	require.Equal(t, "done", tree.Features[soleFeatureID(t)].Status, "the parent feature rolls up to done")
	require.Equal(t, "rollup", tree.Features[soleFeatureID(t)].DoneBasis,
		"no merge is observed at feature grain; the crossing is arithmetic over its tasks")

	// The epic rolls up in the same pass its last non-cancelled feature crosses.
	// There is no accept verb and no epic gate: the only route to a done epic is
	// this arithmetic (artifact-schema.md, Epics: States).
	epic := soleEpic(t, tree)
	require.Equal(t, "done", epic.Status, "the epic rolls up behind its last feature")
	require.Equal(t, "rollup", epic.DoneBasis)

	require.True(t, hasEventCLI(t, tree, "sync.task"), "a sync.task crossing event was written")
	require.True(t, hasEventCLI(t, tree, "sync.feature"), "a sync.feature rollup event was written")
	require.True(t, hasEventCLI(t, tree, "sync.epic"), "a sync.epic rollup event was written")
}

// TestEndToEndRollupParkedFeatureIsNamedAndThenCrosses reproduces the Acme
// incident end to end and proves it cannot recur: a real build's agent walked
// start -> submit -> merge -> sync on a feature's last task, and the feature
// silently stranded active because `check feature` never ran. Cloned from
// TestEndToEndTaskAndFeatureReachDoneCLI, with one deliberate difference: after
// the task's local --no-ff merge, sync runs WITHOUT `check feature` ever having
// been called. Where the sibling test runs `check feature` right after submit
// so the rollup fires clean on the merge sync, this test omits that step
// entirely, so the merge sync is the one that used to read as a finished,
// quiet success while the feature was left behind with nothing recorded.
//
// The assertions walk the whole visibility arc this feature adds on top of
// that gap: the sync confirmation names the parked feature by id with the
// exact command to unstick it (Task 4); the status oracle reads the parked
// phase at both its human parenthetical and its JSON activity field;
// `check feature` moves the phase from awaiting_check
// to awaiting_sync, which this test also renders at the human status surface,
// closing a Task 5 review finding that the awaiting-sync parenthetical had no
// CLI-level render test; and only the sync that follows records the rollup,
// with the same done/rollup shape the sibling test asserts for the clean path.
func TestEndToEndRollupParkedFeatureIsNamedAndThenCrosses(t *testing.T) {
	dir := initGatedGitRepo(t, "true")

	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)

	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	_, _, code = execCLI(t, "spec", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)

	_, _, code = execCLI(t, "approve", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)

	claimTaskAsOwner(t, tid, "jc")
	require.Empty(t, soleFeatureBranch(t))

	// The work loop on the task, exactly as the sibling test walks it.
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	branch := checkoutTaskBranchCLI(t, tid)
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, 0, code)

	// The incident, reproduced: no `check feature` here. The agent goes
	// straight from the task's submit to the local merge and sync, exactly as
	// the real build did on its feature's last task.
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "lay tree and task work")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "work.txt"), []byte("work\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "feature work")
	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "merge", "--no-ff", "-m", "merge feature", branch)

	featureID := soleFeatureID(t)

	// 1. The sync that crosses the last task names the parked feature with the
	// command that unsticks it. This is the confirmation block Task 4 added;
	// before it, this exact sync run reported success with no mention that a
	// feature had been left behind.
	syncOut, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, syncOut, "Awaiting feature check: 1 feature(s)")
	require.Contains(t, syncOut, "run: tiller check feature "+featureID)

	// The task really did cross on the merge; only the feature is parked. A
	// build where the rollup silently fired would fail this pair (feature
	// already "done") just as surely as a build where the task never crossed.
	crossed, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, "done", crossed.Tasks[tid].Status, "the task itself still crosses on the observed merge")
	require.Equal(t, "active", crossed.Features[featureID].Status,
		"the feature does not roll up silently; it stays active, parked awaiting its check")

	// 2. The status oracle carries the phase, human and JSON.
	statusOut, _, code := execCLI(t, "status", "feature", cliFeatureKey)
	require.Equal(t, 0, code)
	require.Contains(t, statusOut, "state: active (awaiting check: all tasks done, feature gates unrecorded)")

	statusJSON, _, code := execCLI(t, "status", "feature", cliFeatureKey, "--json")
	require.Equal(t, 0, code)
	require.Contains(t, statusJSON, "\"activity\": \"awaiting_check\"")

	// 4. check feature moves the phase to awaiting_sync, at both the JSON
	// status surface and, per the controller's Task 5 review note, the human
	// one: the awaiting-sync parenthetical (status.go's
	// renderStatusNodeHuman) had no CLI-level render test before this.
	_, _, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)

	statusJSONAfterCheck, _, code := execCLI(t, "status", "feature", cliFeatureKey, "--json")
	require.Equal(t, 0, code)
	require.Contains(t, statusJSONAfterCheck, "\"activity\": \"awaiting_sync\"")

	statusOutAfterCheck, _, code := execCLI(t, "status", "feature", cliFeatureKey)
	require.Equal(t, 0, code)
	require.Contains(t, statusOutAfterCheck, "state: active (awaiting sync: rollup earned, the next sync records it)")

	// 5. The next sync records the rollup: feature done, done_basis rollup, no
	// more checks-due block, and the epic rolls up behind it, mirroring the
	// sibling test's done/rollup assertions on the clean path.
	finalSync, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NotContains(t, finalSync, "Awaiting feature check")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, "done", tree.Features[featureID].Status, "the parked feature now rolls up")
	require.Equal(t, "rollup", tree.Features[featureID].DoneBasis,
		"no merge is observed at feature grain; the crossing is arithmetic over its tasks")
	epic := soleEpic(t, tree)
	require.Equal(t, "done", epic.Status, "the epic rolls up behind its last feature")
	require.Equal(t, "rollup", epic.DoneBasis)
	require.True(t, hasEventCLI(t, tree, "sync.task"), "a sync.task crossing event was written")
	require.True(t, hasEventCLI(t, tree, "sync.feature"), "a sync.feature rollup event was written")
	require.True(t, hasEventCLI(t, tree, "sync.epic"), "a sync.epic rollup event was written")
}

// syncCrossings parses the noun of every crossing one `sync --json` run
// recorded, in envelope order. It reads what the run DID rather than what the
// tree looks like afterwards, which is the only way to see a crossing that the
// same pass went on to reopen.
func syncCrossings(t *testing.T, jsonText string) []string {
	t.Helper()
	var env struct {
		Data struct {
			Crossings []struct {
				Noun string `json:"noun"`
			} `json:"crossings"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonText), &env), "sync --json must parse")
	nouns := make([]string, 0, len(env.Data.Crossings))
	for _, c := range env.Data.Crossings {
		nouns = append(nouns, c.Noun)
	}
	return nouns
}

// soleEpic reads the one epic out of a loaded tree.
func soleEpic(t *testing.T, tree *store.Tree) *store.Epic {
	t.Helper()
	require.Len(t, tree.Epics, 1)
	for _, e := range tree.Epics {
		return e
	}
	return nil
}

// TestEndToEndActiveBasisRefusesUntilRefreshCLI drives the active-basis guard end
// to end with NO commit dance: approve feature records the basis from the current
// feature.md/eval content, then a bare working-tree edit of feature.md makes
// start and check refuse with the exit-1 active_basis blocked answer (ok:false, a
// data block, never an error block). approve feature --refresh-basis re-records
// the basis and start proceeds. The basis is recorded by the real approve flow,
// not hand-set.
//
// It then walks the same arc one grain down: the task's own task.md is pinned by
// the plan gate, an edit of it refuses start, check, and submit in turn, and
// approve task --refresh-basis clears each. start, check, and submit are the
// guard's whole surface (cli-surface.md, "Specifying a feature": "start, check,
// and submit refuse it with the active_basis blocked answer"), and this test is
// the only place all three are refused at the grain that owns the pin THROUGH
// THE CLI. The mutate layer has its own table over the same three verbs on a
// drifted task.md (TestTaskBasisGuardRefusesStartCheckAndSubmit); what this adds
// is the exit code, the JSON shape, and the refresh path a caller actually sees.
func TestEndToEndActiveBasisRefusesUntilRefreshCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")

	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	claimTaskAsOwner(t, cliFeatureKey+"/transcribe-the-schema", "jc")
	_, _, code = execCLI(t, "spec", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)

	// Plain approve records the accepted basis from the current feature.md/eval content.
	_, _, code = execCLI(t, "approve", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)

	// A bare working-tree edit of feature.md, no commit: its blob now differs from
	// the recorded basis, and the guard must notice immediately.
	mdPath := filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("# Certification form loading\n\nRevised: scope changed since acceptance.\n"), 0o644))

	// start refuses with active_basis (exit 1, ok:false + a data block, no error block).
	out, _, code := execCLI(t, "start", "task", cliFeatureKey+"/transcribe-the-schema", "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"reason\": \"active_basis\"")
	require.Contains(t, out, "feature.md")
	require.NotContains(t, out, "\"error\"")

	// check refuses the same way while the basis is drifted, and the reason is
	// asserted rather than the bare exit code: exit 1 is every blocked answer's
	// code, so without the reason this clause would not distinguish the
	// active-basis guard from an open-question blocker or an illegal move.
	out, _, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"reason\": \"active_basis\"")
	require.NotContains(t, out, "\"error\"")

	// The feature did not move.
	require.Equal(t, "ready", soleFeatureStatus(t))

	// Refresh the parent basis after review; the task start below can proceed.
	_, _, code = execCLI(t, "approve", "feature", cliFeatureKey, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, code)

	// The same arc at task grain. The plan gate pinned this task's task.md when
	// it approved the feature, so it starts clean.
	tid := soleTaskID(t)
	taskMD := filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading",
		store.TaskDirName, "transcribe-the-schema", store.TaskProseFile)

	// start is the first of the three verbs the guard covers, and this grain owns
	// the pin: the edit is to the task's own task.md, not the feature's spec, so a
	// guard that only compared one grain up would let this through.
	require.NoError(t, os.WriteFile(taskMD, []byte("# Transcribe the schema\n\n## Approach\n\nRevised before the task ever started.\n"), 0o644))
	out, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"reason\": \"active_basis\"")
	require.Contains(t, out, "\"noun\": \"task\"", "the blocked answer names the task, whose basis moved")
	require.NotContains(t, out, "\"error\"")
	require.Equal(t, "ready", soleTaskStatus(t, tid), "a refused start must not move the task")

	_, _, code = execCLI(t, "approve", "task", tid, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code, "the refreshed pin matches, so start proceeds")
	checkoutTaskBranchCLI(t, tid)

	require.NoError(t, os.WriteFile(taskMD, []byte("# Transcribe the schema\n\n## Approach\n\nRevised after approval.\n"), 0o644))

	out, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"reason\": \"active_basis\"")
	require.Contains(t, out, store.TaskProseFile)
	require.Contains(t, out, "\"noun\": \"task\"", "the blocked answer names the task, whose basis moved")
	require.NotContains(t, out, "\"error\"")
	require.Equal(t, "in_progress", soleTaskStatus(t, tid), "a refused check must not move the task")

	// approve task --refresh-basis is the documented clear at this grain.
	_, _, code = execCLI(t, "approve", "task", tid, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Equal(t, "in_progress", soleTaskStatus(t, tid), "refresh-basis records a revision, it does not move the node")
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code, "the refreshed pin matches, so the gates run")

	// And submit, which carried no basis comparison at all before this arc.
	require.NoError(t, os.WriteFile(taskMD, []byte("# Transcribe the schema\n\n## Approach\n\nRevised once more.\n"), 0o644))
	out, _, code = execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"reason\": \"active_basis\"")
	require.NotContains(t, out, "\"error\"")
	require.Equal(t, "check", soleTaskStatus(t, tid), "a refused submit must not move the task")

	_, _, code = execCLI(t, "approve", "task", tid, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Equal(t, "in_review", soleTaskStatus(t, tid))
}

// soleFeatureStatus reads the one feature's status off disk.
func soleFeatureStatus(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Len(t, tree.Features, 1)
	for _, f := range tree.Features {
		return f.Status
	}
	return ""
}

// TestEndToEndOfflineLoopReachesDoneWithoutPR is the issue #15 offline loop,
// end to end with no PR anywhere: in a repo with no git remote, submit without
// --pr moves the TASK to in_review with the branch and head commit recorded as
// the review ref, a local --no-ff merge of the task's recorded working branch
// is then observed by sync via git ancestry, and the task crosses to done with
// its feature rolling up behind it. The pr field stays absent the whole way; the
// local-ancestry crossing needs no PR to work.
func TestEndToEndOfflineLoopReachesDoneWithoutPR(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	branch, tid, head := driveOfflineLoopToInReviewCLI(t, dir)

	// The human merges locally; Tiller never orchestrates it.
	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "merge", "--no-ff", "-m", "merge feature", branch)

	// sync observes the merge by ancestry and crosses the task to done, with
	// no PR ever recorded.
	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"status\": \"done\"")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	tk := tree.Tasks[tid]
	require.Equal(t, "done", tk.Status, "the locally merged branch crosses its task to done with no PR")
	require.Nil(t, tk.PR, "pr stays absent across the whole offline loop")
	require.NotNil(t, tk.ReviewRef, "the review ref recorded at submit survives the crossing")
	require.Equal(t, branch, tk.ReviewRef.Branch)
	require.Equal(t, "git:"+head, tk.ReviewRef.Head.Value)
	require.Equal(t, "done", tree.Features[soleFeatureID(t)].Status, "the feature rolls up behind it")
	require.Equal(t, "rollup", tree.Features[soleFeatureID(t)].DoneBasis)
	// And the epic behind that. The rollups are arithmetic over what the human
	// merged, so they do not depend on a PR having existed anywhere in the arc.
	require.Equal(t, "done", soleEpic(t, tree).Status)
	require.Equal(t, "rollup", soleEpic(t, tree).DoneBasis)
	require.True(t, hasEventCLI(t, tree, "sync.task"), "a sync.task crossing event was written")
	require.True(t, hasEventCLI(t, tree, "sync.feature"), "a sync.feature rollup event was written")
}

// driveOfflineLoopToInReviewCLI leaves a task in review on its own branch,
// including its submission state commit, ready for the human's merge.
// The returned head is the submitted work revision, before that state commit.
func driveOfflineLoopToInReviewCLI(t *testing.T, dir string) (branch, taskID, head string) {
	t.Helper()
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	_, _, code = execCLI(t, "spec", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)

	// No add and no approve at task grain: the explosion is part of the stage-2
	// output written above, and the feature's own plan gate already flipped it
	// to ready (cli-surface.md, "Specifying a feature").
	taskID = soleTaskID(t)
	require.Equal(t, "ready", soleTaskStatus(t, taskID), "the initial task arrived ready with the approved plan")
	claimTaskAsOwner(t, taskID, "jc")
	_, _, code = execCLI(t, "start", "task", taskID, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "check", "task", taskID, "--contributor", "jc")
	require.Equal(t, 0, code)

	branch = loadTestTree(t).Tasks[taskID].Branch
	require.NotEmpty(t, branch)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "lay tree and gate results")
	gitRun(t, dir, "checkout", "-b", branch)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "work.txt"), []byte("work\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "feature work")
	head = gitOut(t, dir, "rev-parse", "--short", "HEAD")

	// submit without --pr: the repo has no remote, so the branch-ref fallback
	// records the current branch and head as the task's review ref.
	out, _, code := execCLI(t, "submit", "task", taskID, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "\"status\": \"in_review\"")
	require.Contains(t, out, "\"branch\": \""+branch+"\"")
	require.Contains(t, out, "\"head\": \"git:"+head+"\"")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "record task review")
	require.Equal(t, "in_review", soleTaskStatus(t, taskID))
	return branch, taskID, head
}

// soleTaskStatus reads one task's status off disk.
func soleTaskStatus(t *testing.T, id string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Contains(t, tree.Tasks, id)
	return tree.Tasks[id].Status
}

// A sync on the task's own branch refuses rather than inventing integration.
func TestEndToEndSyncRefusesOnTheTaskBranchCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	branch, taskID, _ := driveOfflineLoopToInReviewCLI(t, dir)

	require.Equal(t, branch, gitOut(t, dir, "branch", "--show-current"))
	merged := exec.Command("git", "merge-base", "--is-ancestor", branch, "main")
	merged.Dir = dir
	require.Error(t, merged.Run(), "the branch is genuinely NOT merged into main")

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"reason\": \"into_is_feature_branch\"")
	require.NotContains(t, out, "\"error\"")
	require.Equal(t, "in_review", soleTaskStatus(t, taskID), "a refused sync moves nothing")
}

// TestEndToEndSquashMergeReachesDoneCLI drives the same offline loop, then merges
// the way GitHub's default button does: a squash. Squashing makes ancestry
// permanently false, so before this batch the node sat in in_review forever with
// sync reporting a clean run at exit 0. The content fallback observes it, and the
// crossing records that its evidence was content rather than ancestry.
func TestEndToEndSquashMergeReachesDoneCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	branch, taskID, _ := driveOfflineLoopToInReviewCLI(t, dir)

	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "merge", "--squash", branch)
	gitRun(t, dir, "commit", "-m", "squash merge the feature")
	squash := gitOut(t, dir, "rev-parse", "HEAD")

	notAncestor := exec.Command("git", "merge-base", "--is-ancestor", branch, "main")
	notAncestor.Dir = dir
	require.Error(t, notAncestor.Run(), "a squash leaves ancestry false; only content can see this merge")

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"status\": \"done\"")
	require.Contains(t, out, "\"basis\": \"content\"")
	require.Contains(t, out, "\"matched_commit\": \"git:"+squash+"\"")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, "done", tree.Tasks[taskID].Status)
	require.Equal(t, "done", tree.Features[soleFeatureID(t)].Status)
	require.True(t, hasEventCLI(t, tree, "sync.task"))
	require.True(t, hasEventCLI(t, tree, "sync.feature"))
}

// TestIncidentArcRefusesBeforeCode replays issue #33's field sequence at the CLI
// grain, the half this file can build without a hand-set fixture: add feature,
// cut a task, approve it. The guard must stop the arc right there, before any
// code exists, because a draft feature has no plan pinned to gate against. The
// arc's other hole, a task hand-advanced past the plan gate on a legacy tree
// and then started, needs a fixture that pokes task status onto disk directly,
// which this file's no-hand-set-fixtures rule forbids; that half is proven at
// the mutate grain by TestStartTaskRefusesUnderDraftFeature
// (internal/core/mutate/transition_test.go).
func TestIncidentArcRefusesBeforeCode(t *testing.T) {
	initGatedGitRepo(t, "true")
	execCLI(t, "add", "epic", "foundation", "--title", "Foundation", "--contributor", "jc")
	execCLI(t, "approve", "epic", "foundation", "--contributor", "jc")
	execCLI(t, "add", "feature", "foundation/application-platform", "--title", "Application platform", "--contributor", "jc")
	execCLI(t, "add", "task", "foundation/application-platform/scaffold", "--title", "Scaffold", "--contributor", "jc")

	out, stderr, code := execCLI(t, "approve", "task", "foundation/application-platform/scaffold", "--contributor", "jc")
	require.Equal(t, 1, code, "the incident arc's first crossing now refuses: out=%s stderr=%s", out, stderr)
	require.Contains(t, out+stderr, "Blocked")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	// Net 6 (issue #40): without this, an empty tree.Tasks would make the loop
	// below iterate zero times and pass vacuously, "proving" nothing moved
	// when in fact nothing was ever checked at all.
	require.NotEmpty(t, tree.Tasks, "the fixture must have laid the one task the refusal is about")
	for _, tk := range tree.Tasks {
		require.Equal(t, "proposed", tk.Status, "nothing moved")
	}
}
