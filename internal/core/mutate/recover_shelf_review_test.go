package mutate_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

const handNamedBranch = "fix/several-bugs"

// layMergedUnrecordedBugs lays the accident shelf recovery exists for: two
// approved bugs fixed together on a hand-named branch, with no start, check,
// or submit recorded, and that branch then merged into main the given way
// ("merge", "squash", or "contained"). The input names the reviewed head and
// the integration result; the caller sets the noun and key.
func layMergedUnrecordedBugs(t *testing.T, how string) (string, mutate.Deps, mutate.RecoverReviewInput) {
	t.Helper()
	root := writeGatedRepo(t, "true", "true")
	setIntegrationBranch(t, root, "main")
	deps := seqDeps()
	for _, key := range []string{"alpha", "beta"} {
		_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: key, Title: "Bug " + key, Severity: "medium"})
		require.NoError(t, err)
		writeBugContract(t, root, key, "## Reproduction\n\nx\n\n## Fixed when\n\ny\n")
	}
	gitCommitAllM(t, root, "capture two bugs")
	for _, key := range []string{"alpha", "beta"} {
		driveTo(t, root, store.NounBug, key, mutate.TransitionInput{Verb: "approve"})
	}
	gitCommitAllM(t, root, "approve two bugs")

	gitRunM(t, root, "checkout", "-b", handNamedBranch)
	require.NoError(t, os.WriteFile(filepath.Join(root, "fix.txt"), []byte("both fixes\n"), 0o644))
	gitCommitAllM(t, root, "fix both bugs")
	head := gitOutM(t, root, "rev-parse", "HEAD")
	gitRunM(t, root, "checkout", "main")
	switch how {
	case "squash":
		gitRunM(t, root, "merge", "--squash", handNamedBranch)
		gitCommitAllM(t, root, "human squash")
	case "contained":
		gitRunM(t, root, "merge", "--ff-only", handNamedBranch)
	default:
		gitRunM(t, root, "merge", "--no-ff", "-m", "human merge", handNamedBranch)
	}
	return root, deps, mutate.RecoverReviewInput{
		Noun: store.NounBug, Head: head, MergeCommit: gitOutM(t, root, "rev-parse", "HEAD"),
		Reason: "merged before any submission was recorded",
	}
}

func bugByKey(t *testing.T, root, key string) *store.Bug {
	t.Helper()
	return resolveNode(t, load(t, root), store.NounBug, key).(*store.Bug)
}

func TestRecoverShelfReviewClosesSeveralBugsFromOneMerge(t *testing.T) {
	t.Parallel()
	root, deps, in := layMergedUnrecordedBugs(t, "merge")
	gitRunM(t, root, "branch", "-D", handNamedBranch)

	for _, key := range []string{"alpha", "beta"} {
		in.Key = key
		r, err := mutate.RecoverReview(load(t, root), deps, in)
		require.NoError(t, err)
		require.True(t, r.Changed)
		require.Equal(t, store.NounBug, r.Noun)
		require.Equal(t, "ancestry", r.RecoveredReview.Basis)
		require.Equal(t, []string{"build", "tests"}, r.MissingGates, "recovery approves no gate and names the ones with no recorded pass")

		bug := bugByKey(t, root, key)
		require.Equal(t, "ready", bug.Status, "recovery moves no lifecycle")
		require.NotNil(t, bug.RecoveredReview)
		require.Equal(t, "git:"+in.Head, bug.RecoveredReview.Head.Value)
		require.Equal(t, "git:"+in.MergeCommit, bug.RecoveredReview.MergeCommit.Value)
		require.Equal(t, in.Reason, bug.RecoveredReview.Reason)
		require.Empty(t, bug.RecoveredReview.Branch, "the fix never ran on a branch Tiller recorded")
		require.Empty(t, bug.Gates, "no gate is approved")
		require.Equal(t, "recover_review.bug", lastEvent(t, root, "recover_review.bug").Type)
	}
	gitCommitAllM(t, root, "record recovery")

	res, err := mutate.Sync(load(t, root), deps, vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 2)
	require.Empty(t, res.Awaiting)
	for _, c := range res.Crossings {
		require.Equal(t, store.NounBug, c.Noun)
		require.Equal(t, "done", c.Status)
		require.Equal(t, "recovered", c.Basis)
		require.Equal(t, []string{"build", "tests"}, c.PolicyViolations, "missing gate evidence is a recorded warning")
	}
	for _, key := range []string{"alpha", "beta"} {
		bug := bugByKey(t, root, key)
		require.Equal(t, "done", bug.Status)
		require.Equal(t, "recovered", bug.DoneBasis)
		require.NotNil(t, bug.RecoveredReview, "the record stays as the audit trail")
		require.Len(t, bug.PolicyViolations, 2)
	}
	ev := lastEvent(t, root, "sync.bug")
	require.Equal(t, "recovered", ev.After["basis"])
	require.Equal(t, "ancestry", ev.After["evidence"])

	// A retry after the crossing writes nothing and keeps the original record.
	snapshot := snapshotDir(t, filepath.Join(root, ".tiller"))
	in.Key, in.Reason = "alpha", "a different explanation"
	retry, err := mutate.RecoverReview(load(t, root), deps, in)
	require.NoError(t, err)
	require.False(t, retry.Changed)
	requireNoWrites(t, snapshot, snapshotDir(t, filepath.Join(root, ".tiller")))
}

func TestRecoverShelfReviewMatchesAnExactSquash(t *testing.T) {
	t.Parallel()
	root, deps, in := layMergedUnrecordedBugs(t, "squash")
	in.Key = "alpha"
	r, err := mutate.RecoverReview(load(t, root), deps, in)
	require.NoError(t, err)
	require.Equal(t, "content", r.RecoveredReview.Basis)
	gitCommitAllM(t, root, "record recovery")
	res, err := mutate.Sync(load(t, root), deps, vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 1)
	require.Equal(t, "recovered", res.Crossings[0].Basis)
	require.Equal(t, "git:"+in.MergeCommit, res.Crossings[0].MatchedCommit)
	require.Equal(t, "content", lastEvent(t, root, "sync.bug").After["evidence"])
}

func TestRecoverShelfReviewContainedHeadAwaitsTheHumanAssertion(t *testing.T) {
	t.Parallel()
	root, deps, in := layMergedUnrecordedBugs(t, "contained")
	in.Key = "alpha"
	r, err := mutate.RecoverReview(load(t, root), deps, in)
	require.NoError(t, err)
	require.Equal(t, "contained", r.RecoveredReview.Basis)
	gitCommitAllM(t, root, "record recovery")

	res, err := mutate.Sync(load(t, root), deps, vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "containment is not an observed merge")
	require.Len(t, res.Awaiting, 1)
	require.True(t, res.Awaiting[0].NoMergeWitness)

	id := bugByKey(t, root, "alpha").ID
	res, err = mutate.Sync(load(t, root), deps, vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{id}})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 1)
	require.Equal(t, "recovered", res.Crossings[0].Basis)
	require.Equal(t, "human_attested", lastEvent(t, root, "sync.bug").After["evidence"])
	require.Equal(t, "done", bugByKey(t, root, "alpha").Status)
}

func TestRecoverShelfReviewCoversStartedAndSubmittedNodes(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"in_progress", "check", "in_review"} {
		t.Run(status, func(t *testing.T) {
			root, deps, in := layMergedUnrecordedBugs(t, "merge")
			in.Key = "alpha"
			stageBugStatus(t, root, "alpha", status)
			gitCommitAllM(t, root, "bug is "+status)
			r, err := mutate.RecoverReview(load(t, root), deps, in)
			require.NoError(t, err)
			require.True(t, r.Changed)
			gitCommitAllM(t, root, "record recovery")
			res, err := mutate.Sync(load(t, root), deps, vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Len(t, res.Crossings, 1)
			require.Equal(t, "done", bugByKey(t, root, "alpha").Status)
		})
	}
}

func TestRecoverShelfReviewRefusalsWriteNothing(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		arrange func(t *testing.T, root string, in *mutate.RecoverReviewInput)
		reason  string
	}{
		"proposed node": {reason: "node_not_approved", arrange: func(t *testing.T, root string, _ *mutate.RecoverReviewInput) {
			stageBugStatus(t, root, "alpha", "proposed")
			gitCommitAllM(t, root, "back to proposed")
		}},
		"done node": {reason: "node_terminal", arrange: func(t *testing.T, root string, _ *mutate.RecoverReviewInput) {
			stageBugStatus(t, root, "alpha", "done")
			gitCommitAllM(t, root, "done")
		}},
		"cancelled node": {reason: "node_terminal", arrange: func(t *testing.T, root string, _ *mutate.RecoverReviewInput) {
			stageBugStatus(t, root, "alpha", "cancelled")
			gitCommitAllM(t, root, "cancelled")
		}},
		"head not carried by the result": {reason: "merge_witness_missing", arrange: func(t *testing.T, root string, in *mutate.RecoverReviewInput) {
			gitRunM(t, root, "checkout", "-b", "elsewhere", in.Head)
			require.NoError(t, os.WriteFile(filepath.Join(root, "later.txt"), []byte("unreviewed\n"), 0o644))
			gitCommitAllM(t, root, "work the review never saw")
			in.Head = gitOutM(t, root, "rev-parse", "HEAD")
			gitRunM(t, root, "checkout", "main")
		}},
		"result off integration": {reason: "result_not_on_integration", arrange: func(t *testing.T, root string, in *mutate.RecoverReviewInput) {
			in.MergeCommit = in.Head
		}},
		"not on the integration checkout": {reason: "integration_checkout_required", arrange: func(t *testing.T, root string, _ *mutate.RecoverReviewInput) {
			gitRunM(t, root, "checkout", handNamedBranch)
		}},
		"uncommitted contract": {reason: "relevant_input_uncommitted", arrange: func(t *testing.T, root string, _ *mutate.RecoverReviewInput) {
			writeBugContract(t, root, "alpha", "## Reproduction\n\nedited\n")
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root, deps, in := layMergedUnrecordedBugs(t, "merge")
			in.Key = "alpha"
			tc.arrange(t, root, &in)
			snapshot := snapshotDir(t, filepath.Join(root, ".tiller"))
			_, err := mutate.RecoverReview(load(t, root), deps, in)
			var refusal *mutate.RecoverReviewError
			require.True(t, errors.As(err, &refusal), "%v", err)
			require.Equal(t, tc.reason, refusal.Reason)
			require.Equal(t, store.NounBug, refusal.Noun)
			require.NotEmpty(t, refusal.NodeID)
			requireNoWrites(t, snapshot, snapshotDir(t, filepath.Join(root, ".tiller")))
		})
	}
}

func TestRecoverShelfReviewRefusesANodeWithALiveCarrier(t *testing.T) {
	t.Parallel()
	root := layTaskGrainGit(t)
	deps := seqDeps()
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "alpha", Title: "Bug alpha", Severity: "medium"})
	require.NoError(t, err)
	writeBugContract(t, root, "alpha", "## Reproduction\n\nx\n")
	gitCommitAllM(t, root, "capture")
	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "approve"})
	_, err = mutate.Resolve(load(t, root), deps, mutate.ResolveInput{Noun: store.NounBug, Node: "alpha", By: "T-aaaaaa"})
	require.NoError(t, err)
	gitCommitAllM(t, root, "carried")
	head := gitOutM(t, root, "rev-parse", "HEAD")

	_, err = mutate.RecoverReview(load(t, root), deps, mutate.RecoverReviewInput{Noun: store.NounBug, Key: "alpha", Head: head, MergeCommit: head, Reason: "x"})
	var refusal *mutate.RecoverReviewError
	require.True(t, errors.As(err, &refusal), "%v", err)
	require.Equal(t, "carried_by_live_work", refusal.Reason)
}

func TestRecoverShelfReviewRequiresThePRWhenARemoteIsConfigured(t *testing.T) {
	t.Parallel()
	root, deps, in := layMergedUnrecordedBugs(t, "merge")
	in.Key = "alpha"
	gitRunM(t, root, "remote", "add", "origin", "https://example.invalid/review.git")
	_, err := mutate.RecoverReview(load(t, root), deps, in)
	require.ErrorContains(t, err, "--pr")

	pr := 13
	in.PR = &pr
	r, err := mutate.RecoverReview(load(t, root), deps, in)
	require.NoError(t, err)
	require.Equal(t, 13, *r.PR)
	require.Equal(t, 13, *bugByKey(t, root, "alpha").PR)

	// The record is immutable: another PR or witness cannot replace it.
	other := 14
	in.PR = &other
	_, err = mutate.RecoverReview(load(t, root), deps, in)
	var refusal *mutate.RecoverReviewError
	require.True(t, errors.As(err, &refusal), "%v", err)
	require.Equal(t, "recovery_identity_changed", refusal.Reason)
}

func TestRecoverShelfReviewRefusesWorkLeftOutsideTheReviewedHead(t *testing.T) {
	t.Parallel()
	root, deps, in := layMergedUnrecordedBugs(t, "merge")
	in.Key = "alpha"
	// The bug was started on its own recorded branch, which then advanced
	// with work the merged review never carried.
	gitRunM(t, root, "checkout", "-b", bugBranch)
	require.NoError(t, os.WriteFile(filepath.Join(root, "unmerged.txt"), []byte("later\n"), 0o644))
	gitCommitAllM(t, root, "later work")
	gitRunM(t, root, "checkout", "main")
	stageBugStatus(t, root, "alpha", "in_progress")
	gitCommitAllM(t, root, "bug started")

	_, err := mutate.RecoverReview(load(t, root), deps, in)
	var refusal *mutate.RecoverReviewError
	require.True(t, errors.As(err, &refusal), "%v", err)
	require.Equal(t, "retained_branch_tip_not_included", refusal.Reason)
}

func TestSyncRefusesARecoveredShelfReviewWhoseResultLeftIntegration(t *testing.T) {
	t.Parallel()
	root, deps, in := layMergedUnrecordedBugs(t, "merge")
	in.Key = "alpha"
	_, err := mutate.RecoverReview(load(t, root), deps, in)
	require.NoError(t, err)
	gitCommitAllM(t, root, "record recovery")
	// Integration is rebuilt without the merge the record names.
	recorded := gitOutM(t, root, "rev-parse", "HEAD")
	gitRunM(t, root, "reset", "--hard", in.MergeCommit+"~1")
	gitRunM(t, root, "checkout", recorded, "--", ".tiller")
	gitCommitAllM(t, root, "state without the merge")

	_, err = mutate.Sync(load(t, root), deps, vcs.New(root), mutate.SyncInput{})
	var refusal *mutate.RecoverReviewError
	require.True(t, errors.As(err, &refusal), "%v", err)
	require.Equal(t, "result_not_on_integration", refusal.Reason)
	require.Equal(t, store.NounBug, refusal.Noun)
	require.Equal(t, "ready", bugByKey(t, root, "alpha").Status)
}

func TestOrdinarySubmitCannotReplaceARecoveredShelfReview(t *testing.T) {
	t.Parallel()
	root, deps, in := layMergedUnrecordedBugs(t, "merge")
	in.Key = "alpha"
	stageBugStatus(t, root, "alpha", "check")
	gitCommitAllM(t, root, "bug checked")
	_, err := mutate.RecoverReview(load(t, root), deps, in)
	require.NoError(t, err)
	tree := load(t, root)
	_, err = mutate.Transition(tree, deps, resolveNode(t, tree, store.NounBug, "alpha"), mutate.TransitionInput{Verb: "submit"})
	var refusal *mutate.RecoverReviewError
	require.True(t, errors.As(err, &refusal), "%v", err)
	require.Equal(t, "recovery_already_recorded", refusal.Reason)
}

// stageBugStatus writes a bug's lifecycle state straight to disk, standing in
// for the start, check, and submit a fixture has no need to drive. A started
// status also records the claim and branch those verbs would have.
func stageBugStatus(t *testing.T, root, key, status string) {
	t.Helper()
	path := filepath.Join(root, store.ShelfRelDir(store.NounBug, key), store.ShelfStateFile(store.NounBug))
	bug := bugByKey(t, root, key)
	bug.Status = status
	if status == "in_progress" || status == "check" || status == "in_review" || status == "done" {
		bug.Started, bug.Owner, bug.Branch = true, "jc", "tiller/bugs/"+key
	}
	writeYAMLTest(t, path, bug)
}
