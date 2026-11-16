package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// fpLayDependentFeature lays an epic-grain epic whose feature beta depends
// on its sibling alpha, with beta's only task approved.
func fpLayDependentFeature(t *testing.T, deps mutate.Deps) (root string) {
	t.Helper()
	root = rgLayEpicWithTwoFeatures(t, deps, "pay")
	_, err := mutate.Link(load(t, root), deps, mutate.LinkInput{Noun: store.NounFeature, Node: "pay/beta", DependsOn: []string{"pay/alpha"}})
	require.NoError(t, err)
	driveWith(t, root, deps, store.NounTask, "pay/beta/only-task", mutate.TransitionInput{Verb: "approve"})
	return root
}

func TestFeaturePrerequisiteInTheSameEpicAggregateStartsFromTheAggregate(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := fpLayDependentFeature(t, deps)
	prerequisite := featureByKey(load(t, root), "pay/alpha")
	pending := branchTask(t, root, "pay/alpha/only-task")

	err := tryTransition(t, root, deps, store.NounTask, "pay/beta/only-task", mutate.TransitionInput{Verb: "start"})
	var blocked *mutate.TaskDependencyBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, prerequisite.ID, blocked.DependencyID)
	require.Equal(t, pending.ID, blocked.PendingTaskID)
	require.Equal(t, "tiller/reviews/epics/pay", blocked.Branch)
	require.Contains(t, blocked.Error(), "submit its task pay/alpha/only-task")
	require.Contains(t, blocked.Error(), "merge")
	require.Empty(t, epicByKey(load(t, root), "pay").Branch, "a refused start writes nothing")

	rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 1)
	pending = branchTask(t, root, "pay/alpha/only-task")
	aggregate := epicByKey(load(t, root), "pay").Branch
	gitRunM(t, root, "checkout", pending.Branch)
	require.NoError(t, os.WriteFile(filepath.Join(root, "state.txt"), []byte("not assembled\n"), 0o644))
	gitCommitAllM(t, root, "pending state")
	gitRunM(t, root, "checkout", aggregate)
	err = tryTransition(t, root, deps, store.NounTask, "pay/beta/only-task", mutate.TransitionInput{Verb: "start"})
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, pending.ID, blocked.PendingTaskID)
	require.Contains(t, blocked.Error(), aggregate)
	require.Contains(t, blocked.Error(), "merge")
	require.Equal(t, "ready", branchTask(t, root, "pay/beta/only-task").Status)

	gitRunM(t, root, "merge", "--no-ff", "-m", "assemble prerequisite state", pending.Branch)
	started := driveWith(t, root, deps, store.NounTask, "pay/beta/only-task", mutate.TransitionInput{Verb: "start"})
	require.Equal(t, aggregate, started.BaseBranch)
	require.Equal(t, aggregate, started.AggregateBranch)
	require.Equal(t, "active", featureByKey(load(t, root), "pay/alpha").Status, "the prerequisite completes only with the epic's merge")
}

// A feature that overrides to its own review never enters the epic's
// aggregate, so its dependents still wait for its integration.
func TestFeaturePrerequisiteWithItsOwnReviewStillWaitsForIntegration(t *testing.T) {
	t.Parallel()
	for _, override := range []string{"pay/alpha", "pay/beta"} {
		t.Run(override, func(t *testing.T) {
			deps := seqDeps()
			root := fpLayDependentFeature(t, deps)
			_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: store.NounFeature, Key: override, ReviewGrain: "feature"})
			require.NoError(t, err)
			rgDriveTaskToInReview(t, root, deps, "pay", "alpha", "only-task", 1)

			err = tryTransition(t, root, deps, store.NounTask, "pay/beta/only-task", mutate.TransitionInput{Verb: "start"})
			var blocked *mutate.TaskDependencyBlockedError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, featureByKey(load(t, root), "pay/alpha").ID, blocked.DependencyID)
			require.Empty(t, blocked.Branch)
			require.Empty(t, blocked.PendingTaskID)
			require.Contains(t, blocked.Error(), "wait")
			require.NotContains(t, blocked.Error(), "aggregate")
		})
	}
}
