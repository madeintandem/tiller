package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	syncFirstTaskBranch  = "tiller/form-engine/certification-form-loading/transcribe-the-schema"
	syncSecondTaskBranch = "tiller/form-engine/certification-form-loading/validate-the-schema"
	syncAggregateBranch  = "tiller/reviews/features/form-engine/certification-form-loading"
)

func independentSyncBranches(t *testing.T) string {
	t.Helper()
	root := layTaskGrainGit(t)
	gitCommitAllM(t, root, "record task fixture")
	for i, branch := range []string{syncFirstTaskBranch, syncSecondTaskBranch} {
		gitRunM(t, root, "checkout", "-b", branch, "main")
		name := []string{"first.txt", "second.txt"}[i]
		commitFile(t, root, name, branch)
		gitRunM(t, root, "checkout", "main")
	}
	stageSyncReview(t, root, "T-aaaaaa", syncFirstTaskBranch)
	stageSyncReview(t, root, "T-aaaaab", syncSecondTaskBranch)
	return root
}

func mergeSyncBranch(t *testing.T, root, branch, shape string) {
	t.Helper()
	if shape == "squash" {
		gitRunM(t, root, "merge", "--squash", branch)
		// State fixtures are intentionally not part of the implementation patch.
		gitRunM(t, root, "commit", "-m", "squash implementation")
		return
	}
	gitRunM(t, root, "merge", "--no-ff", "-m", "merge implementation", branch)
}

func recordSyncAggregate(t *testing.T, root, grain string, recorded bool) string {
	t.Helper()
	branch := syncAggregateBranch
	featurePath := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml")
	data, err := os.ReadFile(featurePath)
	require.NoError(t, err)
	var f store.Feature
	require.NoError(t, yaml.Unmarshal(data, &f))
	f.PR, f.ReviewRef = nil, nil
	if grain == store.ReviewGrainEpic {
		branch = "tiller/reviews/epics/form-engine"
		f.ReviewGrain = ""
		p := filepath.Join(root, ".tiller", "epics", "form-engine", "epic.yml")
		data, err = os.ReadFile(p)
		require.NoError(t, err)
		var e store.Epic
		require.NoError(t, yaml.Unmarshal(data, &e))
		e.ReviewGrain, e.Branch = grain, branch
		e.PR, e.ReviewRef = nil, nil
		if recorded {
			e.ReviewRef = syncReviewRef(t, root, branch)
		}
		data, err = yaml.Marshal(&e)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(p, data, 0o644))
	} else {
		f.ReviewGrain = grain
		if recorded {
			f.ReviewRef = syncReviewRef(t, root, branch)
		}
	}
	f.Branch = branch
	data, err = yaml.Marshal(&f)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(featurePath, data, 0o644))
	rideSyncAggregate(t, filepath.Dir(featurePath), grain)
	return branch
}

// rideSyncAggregate records grain as each fixture task's started grain. The
// fixture starts its tasks before the grain is hand-set, and a task that kept
// started_grain: task would take its own PR rather than ride the aggregate
// (issue #146).
func rideSyncAggregate(t *testing.T, featureDir, grain string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(featureDir, "tasks", "*", "task.yml"))
	require.NoError(t, err)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var task store.Task
		require.NoError(t, yaml.Unmarshal(data, &task))
		if task.StartedGrain == "" {
			continue
		}
		task.StartedGrain = grain
		data, err = yaml.Marshal(&task)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o644))
	}
}

func syncReviewRef(t *testing.T, root, branch string) *store.ReviewRef {
	t.Helper()
	return &store.ReviewRef{Branch: branch, Head: store.RevisionRef{
		Kind: store.KindGitCommit, Value: "git:" + gitOutM(t, root, "rev-parse", "refs/heads/"+branch),
	}}
}

func TestSyncIndependentTaskBranchMergeLeavesSiblingInReview(t *testing.T) {
	t.Parallel()
	for _, shape := range []string{"merge", "squash"} {
		t.Run(shape, func(t *testing.T) {
			root := independentSyncBranches(t)
			mergeSyncBranch(t, root, syncFirstTaskBranch, shape)
			res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Equal(t, []string{"T-aaaaaa"}, crossedIDs(res))
			tree := load(t, root)
			require.Equal(t, "done", tree.Tasks["T-aaaaaa"].Status)
			require.Equal(t, "in_review", tree.Tasks["T-aaaaab"].Status)
			require.Equal(t, "active", tree.Features["F-aaaaaa"].Status)
			require.Nil(t, tree.Tasks["T-aaaaaa"].PR, "offline review needs no host PR")
			require.Len(t, res.Awaiting, 1)
			require.Equal(t, syncSecondTaskBranch, res.Awaiting[0].Branch)
		})
	}
}

func TestSyncTaskFastForwardAssertionIsTaskScoped(t *testing.T) {
	t.Parallel()
	root := independentSyncBranches(t)
	gitRunM(t, root, "merge", "--ff-only", syncFirstTaskBranch)
	declareIntegrationBranch(t, root)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{"T-aaaaaa"}})
	require.NoError(t, err)
	require.Equal(t, []string{"T-aaaaaa"}, crossedIDs(res))
	require.Equal(t, "human_attested", taskDoneBasis(t, root, "T-aaaaaa"))
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaab"].Status)
}

func TestSyncCoarseMergeRequiresEachTaskHeadAndCurrentTip(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		for _, shape := range []string{"merge", "squash"} {
			t.Run(grain+"/"+shape, func(t *testing.T) {
				root := independentSyncBranches(t)
				branch := syncAggregateBranch
				if grain == store.ReviewGrainEpic {
					branch = "tiller/reviews/epics/form-engine"
				}
				gitRunM(t, root, "checkout", "-b", branch, "main")
				gitRunM(t, root, "merge", "--no-ff", "-m", "assemble first task", syncFirstTaskBranch)
				gitRunM(t, root, "checkout", "main")
				recordSyncAggregate(t, root, grain, true)

				// Assembly is agent work, not the human integration merge.
				res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
				require.NoError(t, err)
				require.Empty(t, res.Crossings)
				mergeSyncBranch(t, root, branch, shape)
				res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
				require.NoError(t, err)
				require.Equal(t, []string{"T-aaaaaa"}, crossedIDs(res))
				require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaab"].Status)
				require.Len(t, res.Awaiting, 1)
				require.True(t, res.Awaiting[0].AggregateIncomplete)
			})
		}
	}
}

func TestSyncCoarseMergeRejectsStaleTaskInclusion(t *testing.T) {
	t.Parallel()
	for _, missing := range []string{"submitted_head", "current_tip"} {
		t.Run(missing, func(t *testing.T) {
			root := independentSyncBranches(t)
			gitRunM(t, root, "checkout", "-b", syncAggregateBranch, "main")
			gitRunM(t, root, "merge", "--no-ff", "-m", "assemble first task", syncFirstTaskBranch)
			gitRunM(t, root, "checkout", "main")
			if missing == "current_tip" {
				gitRunM(t, root, "checkout", syncFirstTaskBranch)
				// Commit only implementation: the review head must remain the old one.
				require.NoError(t, os.WriteFile(filepath.Join(root, "late.txt"), []byte("submission state\n"), 0o644))
				gitRunM(t, root, "add", "late.txt")
				gitRunM(t, root, "commit", "-m", "late state commit")
				gitRunM(t, root, "checkout", "main")
			} else {
				p := taskYMLPath(t, root, "T-aaaaaa")
				data, err := os.ReadFile(p)
				require.NoError(t, err)
				var tk store.Task
				require.NoError(t, yaml.Unmarshal(data, &tk))
				tk.ReviewRef.Head = syncReviewRef(t, root, syncSecondTaskBranch).Head
				data, err = yaml.Marshal(&tk)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(p, data, 0o644))
			}
			recordSyncAggregate(t, root, store.ReviewGrainFeature, true)
			mergeSyncBranch(t, root, syncAggregateBranch, "merge")
			res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Empty(t, res.Crossings)
			require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
			for _, a := range res.Awaiting {
				require.True(t, a.AggregateIncomplete)
			}
		})
	}
}

func TestSyncCoarseBranchWithoutRecordedReviewNeverCrosses(t *testing.T) {
	t.Parallel()
	root := independentSyncBranches(t)
	gitRunM(t, root, "checkout", "-b", syncAggregateBranch, "main")
	gitRunM(t, root, "merge", "--no-ff", "-m", "assemble task", syncFirstTaskBranch)
	gitRunM(t, root, "checkout", "main")
	recordSyncAggregate(t, root, store.ReviewGrainFeature, false)
	mergeSyncBranch(t, root, syncAggregateBranch, "merge")
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings)
	for _, a := range res.Awaiting {
		require.True(t, a.ReviewMissing)
	}
}

func TestSyncMissingAggregateWaitsWithoutCrossing(t *testing.T) {
	t.Parallel()
	root := independentSyncBranches(t)
	gitRunM(t, root, "branch", syncAggregateBranch, syncFirstTaskBranch)
	recordSyncAggregate(t, root, store.ReviewGrainFeature, true)
	gitRunM(t, root, "branch", "-D", syncAggregateBranch)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings)
	require.Len(t, res.Awaiting, 2)
	for _, awaiting := range res.Awaiting {
		require.True(t, awaiting.BranchMissing)
	}
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaab"].Status)
}

func TestSyncRejectsTaskAndAggregateAsIntegration(t *testing.T) {
	t.Parallel()
	root := independentSyncBranches(t)
	for _, branch := range []string{syncFirstTaskBranch, syncAggregateBranch} {
		if branch == syncAggregateBranch {
			recordSyncAggregate(t, root, store.ReviewGrainFeature, false)
		}
		_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{Into: branch})
		var refused *mutate.IntoIsFeatureBranchError
		require.ErrorAs(t, err, &refused)
		require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
	}
}

func TestSyncTaskWithoutBranchNeverFallsBackToFeature(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)
	stageSyncReview(t, root, "T-aaaaaa", "")
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings)
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
	require.Len(t, res.Awaiting, 1)
	require.True(t, res.Awaiting[0].BranchMissing)
	require.Empty(t, res.Awaiting[0].Branch)
}

func TestSyncCoarseFastForwardAssertionTargetsReviewedAggregate(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			root := independentSyncBranches(t)
			branch, id := syncAggregateBranch, "F-aaaaaa"
			if grain == store.ReviewGrainEpic {
				branch, id = "tiller/reviews/epics/form-engine", "E-aaaaaa"
			}
			gitRunM(t, root, "checkout", "-b", branch, "main")
			gitRunM(t, root, "merge", "--no-ff", "-m", "assemble task", syncFirstTaskBranch)
			gitRunM(t, root, "checkout", "main")
			recordSyncAggregate(t, root, grain, true)
			gitRunM(t, root, "merge", "--ff-only", branch)
			declareIntegrationBranch(t, root)
			_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
				AcceptFastForward: []string{"T-aaaaaa"},
			})
			var refused *mutate.AttestNotEligibleError
			require.ErrorAs(t, err, &refused)
			require.Equal(t, mutate.AttestReasonNothingToCross, refused.Nodes[0].Reason)
			res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
				AcceptFastForward: []string{id},
			})
			require.NoError(t, err)
			require.Equal(t, []string{"T-aaaaaa"}, crossedIDs(res))
			require.Equal(t, "human_attested", taskDoneBasis(t, root, "T-aaaaaa"))
			require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaab"].Status)
		})
	}
}
