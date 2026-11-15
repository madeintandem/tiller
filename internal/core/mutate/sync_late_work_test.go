package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// The late-work tests cover the ordering the other reopen tests leave out:
// work added beneath a done rollup and merged with no sync in between, so the
// run that first sees the addition is also the run that crosses it.

const lateTaskKey = "form-engine/certification-form-loading/one-more-thing"

// approveAndStartLateTask adds one task under the fixture's done feature and
// walks it through the real post-gate verbs: approve, claim, start.
func approveAndStartLateTask(t *testing.T, root string) {
	t.Helper()
	addLateTask(t, root, "T-bbbbbb", "one-more-thing", "One more thing")
	writeTaskSpec(t, root, "one-more-thing", "## Approach\n\nLand the one more thing.")
	driveTo(t, root, store.NounTask, "T-bbbbbb", mutate.TransitionInput{Verb: "approve"})
	_, err := mutate.ClaimTask(load(t, root), testDeps(), lateTaskKey, "")
	require.NoError(t, err)
	driveTo(t, root, store.NounTask, "T-bbbbbb", mutate.TransitionInput{Verb: "start"})
}

// mergeLateBranch commits one work file on its own branch, merges it into main
// with a merge commit, and puts the task in review on that branch.
func mergeLateBranch(t *testing.T, root, taskID, branch, file string) {
	t.Helper()
	gitRunM(t, root, "checkout", "-q", "-b", branch)
	require.NoError(t, os.WriteFile(filepath.Join(root, file), []byte("late\n"), 0o644))
	commitPaths(t, root, "late work", file)
	gitRunM(t, root, "checkout", "-q", "main")
	gitRunM(t, root, "merge", "--no-ff", "-m", "merge "+branch, branch)
	stageSyncReview(t, root, taskID, branch)
}

func syncOnce(t *testing.T, root string) mutate.SyncResult {
	t.Helper()
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	return res
}

func reopenedIDs(res mutate.SyncResult) []string {
	ids := make([]string, 0, len(res.Reopened))
	for _, r := range res.Reopened {
		ids = append(ids, r.ID)
	}
	return ids
}

// TestSyncReopensADoneFeatureWhoseLateTaskMergedBeforeTheNextSync is the
// missing ordering. A task is added under a done feature, approved, started,
// and merged before anyone runs sync. None of those verbs reopens the feature,
// so the first sync since the addition crosses the task and then finds every
// task done. The feature's first-round completion record, green gates, and
// criterion results must not stand as its current acceptance: the new work
// landed after all of them were recorded.
func TestSyncReopensADoneFeatureWhoseLateTaskMergedBeforeTheNextSync(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	syncOnce(t, root)
	first := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "done", first.Status)
	require.Equal(t, store.GateStatusPass, first.Gates["build"].Status, "the first crossing was green")
	require.NotEmpty(t, first.VerifiedAgainst.Commit.Value)

	approveAndStartLateTask(t, root)
	started := load(t, root)
	require.Equal(t, "in_progress", started.Tasks["T-bbbbbb"].Status)
	require.Equal(t, "done", started.Features["F-aaaaaa"].Status,
		"add, approve, claim, and start leave the done feature alone: reopening is sync's job")
	require.Equal(t, "done", started.Epics["E-aaaaaa"].Status)

	mergeLateBranch(t, root, "T-bbbbbb", "tiller/late-work", "late.txt")

	res := syncOnce(t, root)
	tasks := crossingsOf(res, store.NounTask)
	require.Len(t, tasks, 1, "the merged late task still crosses: merge-backed done is untouched")
	require.Equal(t, "T-bbbbbb", tasks[0].ID)
	require.ElementsMatch(t, []string{"F-aaaaaa", "E-aaaaaa"}, reopenedIDs(res),
		"the feature and the epic above it reopen in the run that first sees the late work")
	require.Empty(t, crossingsOf(res, store.NounFeature),
		"the run that reopens a feature does not also cross it on the acceptance it just staled")

	tree := load(t, root)
	require.Equal(t, "done", tree.Tasks["T-bbbbbb"].Status)
	f := tree.Features["F-aaaaaa"]
	require.Equal(t, "active", f.Status)
	require.Empty(t, f.DoneBasis)
	require.Zero(t, f.VerifiedAgainst.Commit, "the first round's completion record does not survive as current")
	for name, g := range f.Gates {
		require.Equalf(t, store.GateStatusStale, g.Status, "gate %q predates the late work", name)
	}
	require.Equal(t, store.CriterionStatusStale, f.Gates["evals"].Criteria["C1"].Status,
		"a criterion result from before the late work does not ride across")
	require.Equal(t, "ready", tree.Epics["E-aaaaaa"].Status)

	// The next sync is the later crossing. It reads the staled acceptance, so
	// with no re-check it is recorded with violations instead of as a clean done.
	res = syncOnce(t, root)
	require.Empty(t, res.Reopened)
	features := crossingsOf(res, store.NounFeature)
	require.Len(t, features, 1)
	require.Contains(t, features[0].PolicyViolations, "build")
	require.Len(t, crossingsOf(res, store.NounEpic), 1)
	tree = load(t, root)
	recrossed := tree.Features["F-aaaaaa"]
	require.Equal(t, "done", recrossed.Status)
	require.NotEqual(t, first.VerifiedAgainst.Commit.Value, recrossed.VerifiedAgainst.Commit.Value,
		"the new completion record names the commit that holds the late work")
	require.Equal(t, "done", tree.Epics["E-aaaaaa"].Status)

	// A further sync has nothing left to recompute.
	res = syncOnce(t, root)
	require.Empty(t, res.Reopened)
	require.Empty(t, res.Crossings)
}

// TestSyncNamesAReopenedFeatureWithStaleGatesInChecksDue: the run that reopens
// a feature leaves it with every task done and every required gate stale. That
// feature is due a feature check exactly as one whose gates never ran is, so
// ChecksDue names it. Without the entry a caller reading ChecksDue alone sees
// nothing to do, and the next sync records the crossing with violations.
func TestSyncNamesAReopenedFeatureWithStaleGatesInChecksDue(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	first := syncOnce(t, root)
	require.Empty(t, first.ChecksDue, "a feature that crossed to done is never due a check")

	approveAndStartLateTask(t, root)
	mergeLateBranch(t, root, "T-bbbbbb", "tiller/late-work", "late.txt")

	res := syncOnce(t, root)
	require.Contains(t, reopenedIDs(res), "F-aaaaaa")
	require.Equal(t, []mutate.CheckDue{{
		Noun: store.NounFeature, ID: "F-aaaaaa", Key: "form-engine/certification-form-loading",
	}}, res.ChecksDue, "the reopened feature's gates are stale, so its check is due")

	// The entry names work that is still there to do: the crossing has not
	// been recorded, and the gates the check would rerun are the stale ones.
	f := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "active", f.Status)
	require.Equal(t, store.GateStatusStale, f.Gates["build"].Status)

	// With no check run, the next sync still records the crossing, with
	// violations, and a done feature drops out of ChecksDue.
	res = syncOnce(t, root)
	require.Len(t, crossingsOf(res, store.NounFeature), 1)
	require.Empty(t, res.ChecksDue)
}

// TestSyncReopensWhenOneLateTaskMergedAndAnotherIsStillOpen: the unfinished
// sibling already reopened the feature before this fix. The merged one must not
// change that answer.
func TestSyncReopensWhenOneLateTaskMergedAndAnotherIsStillOpen(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	syncOnce(t, root)
	approveAndStartLateTask(t, root)
	addLateTask(t, root, "T-cccccc", "and-another", "And another")
	mergeLateBranch(t, root, "T-bbbbbb", "tiller/late-work", "late.txt")

	res := syncOnce(t, root)
	require.Len(t, crossingsOf(res, store.NounTask), 1)
	require.ElementsMatch(t, []string{"F-aaaaaa", "E-aaaaaa"}, reopenedIDs(res))
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status)
	require.Empty(t, res.ChecksDue, "stale gates with a task still open are not a check due yet")

	res = syncOnce(t, root)
	require.Empty(t, res.Reopened, "a reopened feature with work still open is not reopened twice")
	require.Empty(t, res.Crossings)
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status)
}

// TestSyncDoesNotReopenADoneFeatureForACancelledLateTask: a late task
// cancelled before any sync saw it landed nothing, so the completion record
// and the gates stand.
func TestSyncDoesNotReopenADoneFeatureForACancelledLateTask(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	syncOnce(t, root)
	done := load(t, root).Features["F-aaaaaa"]

	approveAndStartLateTask(t, root)
	driveTo(t, root, store.NounTask, "T-bbbbbb", mutate.TransitionInput{Verb: "cancel", Reason: "not needed after all"})

	res := syncOnce(t, root)
	require.Empty(t, res.Reopened)
	require.Empty(t, res.Crossings)
	after := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "done", after.Status)
	require.Equal(t, done.VerifiedAgainst, after.VerifiedAgainst)
	require.Equal(t, store.GateStatusPass, after.Gates["build"].Status)
}

// TestSyncReopensForMergedLateWorkAndIgnoresAHeldTaskBesideIt keeps the
// held-draft rule intact beside the new ordering: the held task is ignored,
// and the merged late task alone decides the reopen.
func TestSyncReopensForMergedLateWorkAndIgnoresAHeldTaskBesideIt(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	syncOnce(t, root)
	approveAndStartLateTask(t, root)
	deps, amendmentID := openHeldAmendment(t, root)
	stageHeld(t, root, deps, amendmentID, mutate.StageInput{
		Op: store.AmendmentOpAddTask, Key: "form-engine/certification-form-loading/held-proposal", Title: "Held proposal",
	})
	mergeLateBranch(t, root, "T-bbbbbb", "tiller/late-work", "late.txt")

	res := syncOnce(t, root)
	require.ElementsMatch(t, []string{"F-aaaaaa", "E-aaaaaa"}, reopenedIDs(res))

	// The held draft neither blocks nor counts on the way back to done.
	res = syncOnce(t, root)
	require.Len(t, crossingsOf(res, store.NounFeature), 1)
	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status)
}

// layLateFeature adds a second feature under the fixture's done epic and
// walks it through the real verbs up to a started task: decomposed, specified,
// plan-approved, claimed, started.
func layLateFeature(t *testing.T, root string) {
	t.Helper()
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		switch prefix {
		case "F":
			return "F-bbbbbb"
		case "T":
			return "T-cccccc"
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/late-arrival", Title: "Late arrival", ChangeType: "create",
	})
	require.NoError(t, err)
	dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "late-arrival")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Late arrival\n\nOne more outcome under the finished epic.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte("---\nnode: F-bbbbbb\ncriteria:\n  - id: C1\n    statement: It works\n    polarity: introduce\n    binding: { type: command, run: \"true\" }\n---\n\n# Acceptance\n"), 0o644))
	_, err = mutate.AddTask(load(t, root), deps, mutate.TaskInput{Key: "form-engine/late-arrival/build-it", Title: "Build it"})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), testDeps(), "form-engine/late-arrival/build-it", "")
	require.NoError(t, err)
	writeTaskSpecAt(t, root, "form-engine", "late-arrival", "build-it", "## Approach\n\nBuild the late arrival.")
	driveTo(t, root, store.NounFeature, "form-engine/late-arrival", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/late-arrival", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounTask, "T-cccccc", mutate.TransitionInput{Verb: "start"})
}

// TestSyncReopensADoneEpicWhoseLateFeatureFinishedBeforeTheNextSync is the same
// ordering one grain up. A feature is added under a done epic and carried all
// the way to a merged task before anyone runs sync, so the first sync since the
// addition crosses the task and rolls the feature up, and the epic pass then
// finds every feature done. The epic's completion record still names the commit
// from before the feature existed, so the epic is recomputed: reopened, then
// crossed again against the commit that holds the new work.
func TestSyncReopensADoneEpicWhoseLateFeatureFinishedBeforeTheNextSync(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	syncOnce(t, root)
	first := load(t, root).Epics["E-aaaaaa"]
	require.Equal(t, "done", first.Status)
	require.NotEmpty(t, first.VerifiedAgainst.Commit.Value)

	layLateFeature(t, root)
	require.Equal(t, "done", load(t, root).Epics["E-aaaaaa"].Status,
		"adding, approving, and starting work leaves the done epic alone: reopening is sync's job")

	// The late feature's prose has to be on integration for its completion record.
	commitPaths(t, root, "late feature prose", ".tiller/epics/form-engine/features/late-arrival")
	mergeLateBranch(t, root, "T-cccccc", "tiller/late-feature", "late-feature.txt")
	feat := resolveNode(t, load(t, root), store.NounFeature, "form-engine/late-arrival")
	_, err := mutate.Check(load(t, root), testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)

	res := syncOnce(t, root)
	require.Len(t, crossingsOf(res, store.NounTask), 1)
	features := crossingsOf(res, store.NounFeature)
	require.Len(t, features, 1, "the late feature rolls up in the run that crosses its task")
	require.Equal(t, "F-bbbbbb", features[0].ID)
	require.Equal(t, []string{"E-aaaaaa"}, reopenedIDs(res),
		"the epic's first-round done did not cover the late feature, so it is reported as reopened")
	require.Len(t, crossingsOf(res, store.NounEpic), 1,
		"and with every feature done it crosses again in the same run instead of stranding at ready")

	after := load(t, root).Epics["E-aaaaaa"]
	require.Equal(t, "done", after.Status)
	require.Equal(t, "rollup", after.DoneBasis)
	require.NotEqual(t, first.VerifiedAgainst.Commit.Value, after.VerifiedAgainst.Commit.Value,
		"the completion record names the commit that holds the late feature")

	res = syncOnce(t, root)
	require.Empty(t, res.Reopened)
	require.Empty(t, res.Crossings)
}
