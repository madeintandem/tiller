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

func branchTask(t *testing.T, root, key string) *store.Task {
	t.Helper()
	return resolveNode(t, load(t, root), store.NounTask, key).(*store.Task)
}

func writeBranchTask(t *testing.T, root string, task *store.Task) {
	t.Helper()
	data, err := yaml.Marshal(task)
	require.NoError(t, err)
	feature := load(t, root).Features[task.FeatureID]
	path := filepath.Join(root, ".tiller", "epics", feature.EpicKey, "features", filepath.Base(*feature.Key), "tasks", filepath.Base(*task.Key), "task.yml")
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func setTaskBranchPattern(t *testing.T, root, pattern string) {
	t.Helper()
	manifest := load(t, root).Manifest
	manifest.Git.BranchPattern = pattern
	data, err := yaml.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), data, 0o644))
}

func TestTaskStartRecordsDistinctBranchesAtEveryReviewGrain(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{"task", "feature", "epic"} {
		t.Run(grain, func(t *testing.T) {
			deps := seqDeps()
			root := rgLayFeature(t, deps, "pay", "checkout", "alfa", "bravo")
			_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: store.NounEpic, Key: "pay", ReviewGrain: grain})
			require.NoError(t, err)
			before, err := vcs.New(root).LocalBranches()
			require.NoError(t, err)
			a := driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})
			b := driveWith(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start"})
			require.Equal(t, "tiller/pay/checkout/alfa", a.Branch)
			require.Equal(t, "tiller/pay/checkout/bravo", b.Branch)
			require.Equal(t, "main", a.BaseBranch)
			require.Equal(t, "main", b.BaseBranch)
			require.Equal(t, a.Branch, branchTask(t, root, "pay/checkout/alfa").Branch)
			switch grain {
			case "task":
				require.Empty(t, a.AggregateBranch)
				require.Empty(t, featureByKey(load(t, root), "pay/checkout").Branch)
			case "feature":
				require.Equal(t, "tiller/reviews/features/pay/checkout", a.AggregateBranch)
			case "epic":
				require.Equal(t, "tiller/reviews/epics/pay", a.AggregateBranch)
				require.Equal(t, a.AggregateBranch, epicByKey(load(t, root), "pay").Branch)
			}
			after, err := vcs.New(root).LocalBranches()
			require.NoError(t, err)
			require.Equal(t, before, after, "recording names never creates or changes a Git ref")
		})
	}
}

func TestTaskStartRefusesUnsafePatternsAndExistingPrefixesWithoutWrites(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, pattern, existing, reason string }{
		{"constant", "work", "", "branch_invalid"},
		{"invalid ref", "work/../${node.key}", "", "branch_invalid"},
		{"ancestor ref", "tiller/${node.key}", "tiller/pay/checkout", "branch_ref_conflict"},
		{"descendant ref", "tiller/${node.key}", "tiller/pay/checkout/alfa/old", "branch_ref_conflict"},
		{"exact ref", "tiller/${node.key}", "tiller/pay/checkout/alfa", "branch_ref_conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := seqDeps()
			root := rgLayFeature(t, deps, "pay", "checkout", "alfa", "bravo")
			setTaskBranchPattern(t, root, tc.pattern)
			if tc.existing != "" {
				gitRunM(t, root, "branch", tc.existing)
			}
			before, err := vcs.New(root).LocalBranches()
			require.NoError(t, err)
			eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))
			err = tryTransition(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})
			var blocked *mutate.BranchSafetyError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, tc.reason, blocked.Reason)
			task := branchTask(t, root, "pay/checkout/alfa")
			require.Equal(t, "ready", task.Status)
			require.Empty(t, task.Branch)
			require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")))
			after, err := vcs.New(root).LocalBranches()
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestTaskSubmitRejectsSiblingBranchAndResumeKeepsIdentity(t *testing.T) {
	t.Parallel()
	root := layCheckedTask(t)
	task := load(t, root).Tasks["T-aaaaaa"]
	gitRunM(t, root, "checkout", "-b", "unrelated-sibling")
	err := tryTransition(t, root, testDeps(), store.NounTask, task.ID, mutate.TransitionInput{Verb: "submit", PR: intPtr(7)})
	var blocked *mutate.BranchSafetyError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "branch_mismatch", blocked.Reason)
	require.Equal(t, "check", load(t, root).Tasks[task.ID].Status)
	gitRunM(t, root, "checkout", task.Branch)
	head, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	err = tryTransition(t, root, testDeps(), store.NounTask, task.ID, mutate.TransitionInput{Verb: "submit", ReviewRef: &store.ReviewRef{Branch: "unrelated-sibling", Head: head}})
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "branch_mismatch", blocked.Reason)
	driveTo(t, root, store.NounTask, task.ID, mutate.TransitionInput{Verb: "submit", PR: intPtr(7)})
	setTaskBranchPattern(t, root, "other/${node.key}")
	resumed := driveTo(t, root, store.NounTask, task.ID, mutate.TransitionInput{Verb: "resume", Reason: "review changes"})
	require.Equal(t, task.Branch, resumed.Branch)
	require.Equal(t, 7, *resumed.PR)
	require.Equal(t, task.Branch, load(t, root).Tasks[task.ID].Branch)
	driveTo(t, root, store.NounTask, task.ID, mutate.TransitionInput{Verb: "check"})
	err = tryTransition(t, root, testDeps(), store.NounTask, task.ID, mutate.TransitionInput{Verb: "submit", PR: intPtr(8)})
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "branch_review_identity", blocked.Reason)
}

func TestAggregateSubmitRequiresLatestTaskTipAndKeepsSamePR(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "feature"})
	require.NoError(t, err)
	rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "alfa", 1)
	rgCheckFeatureGreen(t, root, deps, "pay", "checkout")
	gitCommitAllM(t, root, "aggregate gates")
	aggregate := featureByKey(load(t, root), "pay/checkout").Branch
	task := branchTask(t, root, "pay/checkout/alfa")
	gitRunM(t, root, "checkout", task.Branch)
	require.NoError(t, os.WriteFile(filepath.Join(root, "submission-state.txt"), []byte("additional durable task state\n"), 0o644))
	gitCommitAllM(t, root, "later task state")
	gitRunM(t, root, "checkout", aggregate)
	before := countFiles(t, filepath.Join(root, ".tiller", "events"))
	_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(7)})
	var missing *mutate.AggregateContentMissingError
	require.ErrorAs(t, err, &missing)
	require.Equal(t, task.ID, missing.TaskID)
	require.Nil(t, featureByKey(load(t, root), "pay/checkout").PR)
	require.Equal(t, before, countFiles(t, filepath.Join(root, ".tiller", "events")))
	gitRunM(t, root, "merge", "--no-ff", "-m", "assemble remaining state", task.Branch)
	_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(7)})
	require.NoError(t, err)
	again, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(7)})
	require.NoError(t, err)
	require.False(t, again.Changed)
	require.Equal(t, 7, *again.PR)
}

func TestTaskDependencyStartsFromAggregateOnlyAfterPrerequisiteAssembly(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{"task", "feature"} {
		t.Run(grain, func(t *testing.T) {
			deps := seqDeps()
			root := rgLayFeature(t, deps, "pay", "checkout", "alfa", "bravo")
			_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: grain})
			require.NoError(t, err)
			first := branchTask(t, root, "pay/checkout/alfa")
			next := branchTask(t, root, "pay/checkout/bravo")
			next.DependsOn = []string{first.ID}
			writeBranchTask(t, root, next)
			err = tryTransition(t, root, deps, store.NounTask, *next.Key, mutate.TransitionInput{Verb: "start"})
			var blocked *mutate.TaskDependencyBlockedError
			require.ErrorAs(t, err, &blocked)
			rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "alfa", 1)
			if grain == "task" {
				err = tryTransition(t, root, deps, store.NounTask, *next.Key, mutate.TransitionInput{Verb: "start"})
				require.ErrorAs(t, err, &blocked)
				require.NotContains(t, blocked.Error(), "aggregate")
				return
			}
			first = branchTask(t, root, *first.Key)
			aggregate := featureByKey(load(t, root), "pay/checkout").Branch
			gitRunM(t, root, "checkout", first.Branch)
			require.NoError(t, os.WriteFile(filepath.Join(root, "state.txt"), []byte("not assembled\n"), 0o644))
			gitCommitAllM(t, root, "pending state")
			gitRunM(t, root, "checkout", aggregate)
			err = tryTransition(t, root, deps, store.NounTask, *next.Key, mutate.TransitionInput{Verb: "start"})
			require.ErrorAs(t, err, &blocked)
			require.Contains(t, blocked.Error(), aggregate)
			require.Contains(t, blocked.Error(), "merge")
			gitRunM(t, root, "merge", "--no-ff", "-m", "assemble prerequisite state", first.Branch)
			started := driveWith(t, root, deps, store.NounTask, *next.Key, mutate.TransitionInput{Verb: "start"})
			require.Equal(t, aggregate, started.BaseBranch)
			require.Equal(t, "tiller/pay/checkout/bravo", started.Branch)
		})
	}
}

func TestTaskStartRotatesCompletedAggregateWithoutChangingOldRefs(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			deps := seqDeps()
			root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
			noun, key := store.NounFeature, "pay/checkout"
			if grain == store.ReviewGrainEpic {
				noun, key = store.NounEpic, "pay"
			}
			_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: noun, Key: key, ReviewGrain: grain})
			require.NoError(t, err)
			rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "alfa", 1)
			rgCheckFeatureGreen(t, root, deps, "pay", "checkout")
			gitCommitAllM(t, root, "aggregate gates")
			pr := 9
			_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{Noun: noun, Key: key, PR: &pr})
			require.NoError(t, err)
			gitCommitAllM(t, root, "aggregate submission")
			old := featureByKey(load(t, root), "pay/checkout").Branch
			oldHead, err := vcs.New(root).ShortSHA(vcs.BranchRef(old))
			require.NoError(t, err)
			first := branchTask(t, root, "pay/checkout/alfa")
			gitRunM(t, root, "checkout", "main")
			gitRunM(t, root, "merge", "--no-ff", "-m", "human final merge", old)
			_, err = mutate.Sync(load(t, root), deps, vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Equal(t, "done", featureByKey(load(t, root), "pay/checkout").Status)
			rgAddTask(t, root, deps, "pay", "checkout", "bravo")
			driveWith(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "approve"})
			err = tryTransition(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start"})
			var blocked *mutate.BranchSafetyError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, "aggregate_round_closed", blocked.Reason)
			require.Equal(t, "ready", branchTask(t, root, "pay/checkout/bravo").Status)
			_, err = mutate.Sync(load(t, root), deps, vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			started := driveWith(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start"})
			require.NotEqual(t, old, started.AggregateBranch)
			require.Equal(t, started.AggregateBranch, featureByKey(load(t, root), "pay/checkout").Branch)
			require.Equal(t, first.Branch, branchTask(t, root, "pay/checkout/alfa").Branch)
			afterHead, err := vcs.New(root).ShortSHA(vcs.BranchRef(old))
			require.NoError(t, err)
			require.Equal(t, oldHead, afterHead)
			exists, err := vcs.New(root).RefExists(vcs.BranchRef(started.AggregateBranch))
			require.NoError(t, err)
			require.False(t, exists, "the agent creates the newly recorded aggregate, never Tiller")
		})
	}
}

func TestTaskStartIgnoresHistoricalFeatureBranchButStillChecksGitRefs(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
	feature := featureByKey(load(t, root), "pay/checkout")
	feature.Branch = "tiller/pay/checkout"
	data, err := yaml.Marshal(feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "feature.yml"), data, 0o644))
	gitRunM(t, root, "branch", feature.Branch)
	err = tryTransition(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})
	var blocked *mutate.BranchSafetyError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "branch_ref_conflict", blocked.Reason)
	gitRunM(t, root, "branch", "-m", feature.Branch, "history/checkout")
	started := driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})
	require.Equal(t, "tiller/pay/checkout/alfa", started.Branch)
	require.Equal(t, feature.Branch, featureByKey(load(t, root), "pay/checkout").Branch, "historical metadata survives")
}

func TestIdempotentTaskStartOnRecordedBranchNeedsNoNewBase(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	started := driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	gitRunM(t, root, "checkout", "-b", started.Branch)
	reentered := driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	require.False(t, reentered.Changed)
	require.Equal(t, started.Branch, reentered.Branch)
	require.Empty(t, reentered.BaseBranch, "an existing branch is not recreated from a recomputed base")
}

func TestAggregateSubmissionAlsoRequiresRecordedReviewHead(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "feature"})
	require.NoError(t, err)
	rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "alfa", 1)
	rgCheckFeatureGreen(t, root, deps, "pay", "checkout")
	gitCommitAllM(t, root, "aggregate gates")
	aggregate := featureByKey(load(t, root), "pay/checkout").Branch
	gitRunM(t, root, "checkout", "-b", "unassembled-review")
	require.NoError(t, os.WriteFile(filepath.Join(root, "reviewed.txt"), []byte("submitted work no longer on task tip\n"), 0o644))
	gitCommitAllM(t, root, "recorded review work")
	head, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	gitRunM(t, root, "checkout", aggregate)
	task := branchTask(t, root, "pay/checkout/alfa")
	task.ReviewRef.Head = head
	writeBranchTask(t, root, task)
	_, err = mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{Noun: store.NounFeature, Key: "pay/checkout", PR: intPtr(7)})
	var missing *mutate.AggregateContentMissingError
	require.ErrorAs(t, err, &missing)
	require.Equal(t, task.ID, missing.TaskID)
}

func TestTaskStartAncestorDependencyRefusalNamesNoBranch(t *testing.T) {
	t.Parallel()
	for _, ancestor := range []string{"epic", "feature"} {
		t.Run(ancestor, func(t *testing.T) {
			deps := seqDeps()
			root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
			_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "base", Title: "Epic base"})
			require.NoError(t, err)
			tree := load(t, root)
			dependency := epicByKey(tree, "base")
			var node any
			var path string
			if ancestor == "epic" {
				epic := epicByKey(tree, "pay")
				epic.DependsOn = []string{dependency.ID}
				node, path = epic, ".tiller/epics/pay/epic.yml"
			} else {
				feature := featureByKey(tree, "pay/checkout")
				feature.DependsOn = []string{dependency.ID}
				node, path = feature, ".tiller/epics/pay/features/checkout/feature.yml"
			}
			data, err := yaml.Marshal(node)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, path), data, 0o644))
			driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "approve"})
			err = tryTransition(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})
			var blocked *mutate.TaskDependencyBlockedError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, dependency.ID, blocked.DependencyID)
			require.Empty(t, blocked.Branch)
			require.Contains(t, blocked.Error(), "wait")
			require.NotContains(t, blocked.Error(), "aggregate")
			require.NotContains(t, blocked.Error(), "merge")
			require.Equal(t, "ready", branchTask(t, root, "pay/checkout/alfa").Status)
		})
	}
}
