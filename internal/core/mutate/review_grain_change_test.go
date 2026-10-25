package mutate

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestReviewGrainChangeRefusesAffectedWorkInFlight(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"in_progress", "check", "in_review"} {
		t.Run(status, func(t *testing.T) {
			tree, epic, feature := reviewGrainChangeTree(store.ReviewGrainTask)
			taskKey := "pay/checkout/charge-card"
			task := &store.Task{ID: "T-000003", Key: &taskKey, FeatureID: feature.ID, Status: status, Owner: "jc", Started: true}
			tree.Tasks[task.ID] = task
			tree.FeatureTasks[feature.ID] = []*store.Task{task}

			err := evaluateReviewGrainChange(tree, reviewGrainChange{
				Noun: store.NounFeature, Epic: epic, Feature: feature, Desired: store.ReviewGrainFeature,
			})

			var blocked *ReviewGrainChangeBlockedError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, ReasonReviewGrainWorkInFlight, blocked.Reason)
			require.Equal(t, store.NounFeature, blocked.TargetNoun)
			require.Equal(t, feature.ID, blocked.TargetID)
			require.Equal(t, store.NounTask, blocked.BlockerNoun)
			require.Equal(t, task.ID, blocked.BlockerID)
			require.Equal(t, status, blocked.BlockerStatus)
			require.Equal(t, store.ReviewGrainTask, blocked.BeforeGrain)
			require.Equal(t, store.ReviewGrainFeature, blocked.AfterGrain)
		})
	}
}

func TestReviewGrainChangeAllowsTaskToFeatureAfterPriorTasksAreDone(t *testing.T) {
	t.Parallel()
	tree, epic, feature := reviewGrainChangeTree(store.ReviewGrainTask)
	taskKey := "pay/checkout/charge-card"
	task := &store.Task{ID: "T-000003", Key: &taskKey, FeatureID: feature.ID, Status: "done", Owner: "jc", Started: true}
	tree.Tasks[task.ID] = task
	tree.FeatureTasks[feature.ID] = []*store.Task{task}

	err := evaluateReviewGrainChange(tree, reviewGrainChange{
		Noun: store.NounFeature, Epic: epic, Feature: feature, Desired: store.ReviewGrainFeature,
	})

	require.NoError(t, err)
}

func TestProjectChangeIgnoresExplicitlyUnaffectedFeature(t *testing.T) {
	t.Parallel()
	tree, _, feature := reviewGrainChangeTree(store.ReviewGrainTask)
	feature.ReviewGrain = store.ReviewGrainTask
	taskKey := "pay/checkout/charge-card"
	task := &store.Task{ID: "T-000003", Key: &taskKey, FeatureID: feature.ID, Status: "in_review", Owner: "jc", Started: true}
	tree.Tasks[task.ID] = task
	tree.FeatureTasks[feature.ID] = []*store.Task{task}

	err := evaluateReviewGrainChange(tree, reviewGrainChange{Noun: "project", Desired: store.ReviewGrainFeature})

	require.NoError(t, err)
}

func TestAncestorChangeRefusesAffectedDescendantReviewSurface(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*store.Epic) reviewGrainChange
	}{
		{name: "project", change: func(_ *store.Epic) reviewGrainChange {
			return reviewGrainChange{Noun: "project", Desired: store.ReviewGrainFeature}
		}},
		{name: "epic", change: func(epic *store.Epic) reviewGrainChange {
			return reviewGrainChange{Noun: store.NounEpic, Epic: epic, Desired: store.ReviewGrainFeature}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, epic, feature := reviewGrainChangeTree(store.ReviewGrainTask)
			pr := 41
			feature.PR = &pr

			err := evaluateReviewGrainChange(tree, tc.change(epic))

			var blocked *ReviewGrainChangeBlockedError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, ReasonReviewGrainWorkInFlight, blocked.Reason)
			require.Equal(t, store.NounFeature, blocked.BlockerNoun)
			require.Equal(t, feature.ID, blocked.BlockerID)
		})
	}
}

func TestProjectChangeRefusesAffectedEpicReviewSurface(t *testing.T) {
	t.Parallel()
	tree, epic, _ := reviewGrainChangeTree(store.ReviewGrainEpic)
	pr := 77
	epic.PR = &pr
	tree.Features = map[string]*store.Feature{}
	tree.FeatureTasks = map[string][]*store.Task{}

	err := evaluateReviewGrainChange(tree, reviewGrainChange{Noun: "project", Desired: store.ReviewGrainFeature})

	var blocked *ReviewGrainChangeBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, ReasonReviewGrainWorkInFlight, blocked.Reason)
	require.Equal(t, "project", blocked.TargetNoun)
	require.Equal(t, store.NounEpic, blocked.BlockerNoun)
	require.Equal(t, epic.ID, blocked.BlockerID)
	require.Equal(t, store.ReviewGrainEpic, blocked.BeforeGrain)
	require.Equal(t, store.ReviewGrainFeature, blocked.AfterGrain)
}

func TestReviewGrainChangeRefusesCrossingEpicBoundaryAfterBranchRecorded(t *testing.T) {
	t.Parallel()
	tree, _, feature := reviewGrainChangeTree(store.ReviewGrainTask)
	feature.Branch = "tiller/pay/checkout"

	err := evaluateReviewGrainChange(tree, reviewGrainChange{Noun: "project", Desired: store.ReviewGrainEpic})

	var blocked *ReviewGrainChangeBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, ReasonReviewGrainBranchLocked, blocked.Reason)
	require.Equal(t, store.NounFeature, blocked.BlockerNoun)
	require.Equal(t, feature.ID, blocked.BlockerID)
	require.Equal(t, store.ReviewGrainTask, blocked.BeforeGrain)
	require.Equal(t, store.ReviewGrainEpic, blocked.AfterGrain)
}

func TestReviewGrainChangeAllowsFeatureToTaskAfterPriorTasksAreDone(t *testing.T) {
	t.Parallel()
	tree, epic, feature := reviewGrainChangeTree(store.ReviewGrainFeature)
	taskKey := "pay/checkout/charge-card"
	task := &store.Task{ID: "T-000003", Key: &taskKey, FeatureID: feature.ID, Status: "done", Owner: "jc", Started: true}
	tree.Tasks[task.ID] = task
	tree.FeatureTasks[feature.ID] = []*store.Task{task}

	err := evaluateReviewGrainChange(tree, reviewGrainChange{
		Noun: store.NounFeature, Epic: epic, Feature: feature, Desired: store.ReviewGrainTask,
	})

	require.NoError(t, err)
}

func TestReviewGrainChangeCannotReuseFeatureAggregateAtTaskGrain(t *testing.T) {
	t.Parallel()
	tree, epic, feature := reviewGrainChangeTree(store.ReviewGrainFeature)
	feature.Branch = "tiller/reviews/features/pay/checkout"
	err := evaluateReviewGrainChange(tree, reviewGrainChange{
		Noun: store.NounFeature, Epic: epic, Feature: feature, Desired: store.ReviewGrainTask,
	})
	var blocked *ReviewGrainChangeBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, ReasonReviewGrainBranchLocked, blocked.Reason)
	require.Equal(t, feature.ID, blocked.BlockerID)
}

func TestReviewGrainChangeRefusesLeavingEpicAfterBranchRecorded(t *testing.T) {
	t.Parallel()
	tree, _, feature := reviewGrainChangeTree(store.ReviewGrainEpic)
	feature.Branch = "tiller/pay"

	err := evaluateReviewGrainChange(tree, reviewGrainChange{Noun: "project", Desired: store.ReviewGrainFeature})

	var blocked *ReviewGrainChangeBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, ReasonReviewGrainBranchLocked, blocked.Reason)
	require.Equal(t, store.ReviewGrainEpic, blocked.BeforeGrain)
	require.Equal(t, store.ReviewGrainFeature, blocked.AfterGrain)
}

func TestReviewGrainChangeChoosesFeaturesByIDAndTasksByRecordedOrder(t *testing.T) {
	t.Parallel()
	tree, _, laterFeature := reviewGrainChangeTree(store.ReviewGrainTask)
	earlierFeatureKey := "pay/refunds"
	earlierFeature := &store.Feature{
		ID: "F-000001", Key: &earlierFeatureKey, EpicKey: "pay", Status: "active",
		Order: []string{"T-000002", "T-000001"},
	}
	tree.Features[earlierFeature.ID] = earlierFeature

	firstKey := "pay/refunds/first"
	secondRef := "pay/refunds/second"
	first := &store.Task{ID: "T-000001", Key: &firstKey, FeatureID: earlierFeature.ID, Status: "in_review", Owner: "jc", Started: true}
	second := &store.Task{ID: "T-000002", Key: &secondRef, FeatureID: earlierFeature.ID, Status: "in_review", Owner: "jc", Started: true}
	laterKey := "pay/checkout/later"
	later := &store.Task{ID: "T-000000", Key: &laterKey, FeatureID: laterFeature.ID, Status: "in_review", Owner: "jc", Started: true}
	tree.FeatureTasks[earlierFeature.ID] = []*store.Task{first, second}
	tree.FeatureTasks[laterFeature.ID] = []*store.Task{later}

	err := evaluateReviewGrainChange(tree, reviewGrainChange{Noun: "project", Desired: store.ReviewGrainFeature})

	var blocked *ReviewGrainChangeBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, second.ID, blocked.BlockerID,
		"the earlier feature id wins globally, then its recorded task order wins locally")
}

func reviewGrainChangeTree(projectGrain string) (*store.Tree, *store.Epic, *store.Feature) {
	epicKey := "pay"
	featureKey := "pay/checkout"
	epic := &store.Epic{ID: "E-000002", Key: &epicKey, Status: "ready"}
	feature := &store.Feature{ID: "F-000002", Key: &featureKey, EpicKey: epicKey, Status: "active"}
	return &store.Tree{
		Manifest:     &store.Manifest{Execution: store.ExecutionBlock{ReviewGrain: projectGrain}},
		Epics:        map[string]*store.Epic{epic.ID: epic},
		Features:     map[string]*store.Feature{feature.ID: feature},
		Tasks:        map[string]*store.Task{},
		FeatureTasks: map[string][]*store.Task{feature.ID: nil},
	}, epic, feature
}
