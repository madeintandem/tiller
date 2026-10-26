package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func TestHistoricalTaskGrainReviewWaitsForMergeUnderCoarseParent(t *testing.T) {
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		for _, owner := range []string{"", "jc"} {
			t.Run(grain+"/"+owner, func(t *testing.T) {
				tree := treeWithDraftEpics(t, 1)
				tree.Epics["E-aaaaa0"].Status = "ready"
				f, prerequisite := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
				f.ReviewGrain = grain
				prerequisite.Status = "in_review"
				prerequisite.Started = true
				prerequisite.StartedGrain = store.ReviewGrainTask
				prerequisite.Branch = "tiller/epic-0/work/first"
				prerequisite.ReviewRef = &store.ReviewRef{Branch: prerequisite.Branch, Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:abc1234"}}
				dependent := &store.Task{ID: "T-bbbbb0", Key: strPtr("epic-0/work/second"), FeatureID: f.ID, Status: "ready", Owner: owner, DependsOn: []string{prerequisite.ID}}
				tree.Tasks[dependent.ID] = dependent
				tree.FeatureTasks[f.ID] = append(tree.FeatureTasks[f.ID], dependent)
				require.False(t, gen.EligibleTask(tree, dependent))
				require.False(t, gen.AssignedTaskEligible(tree, dependent))
				require.Empty(t, candidatesFor(nextFor(tree, "jc", workflow.Input{}).Candidates, dependent.ID))
				require.Nil(t, workflow.ReviewOwnerOf(tree, prerequisite))
				_, missing := workflow.ReviewStateOf(tree, prerequisite)
				require.False(t, missing)
				_, missing = workflow.ReviewStateOf(tree, f)
				require.False(t, missing)
				require.False(t, views.Node(tree, prerequisite).ReviewMissing)
				prerequisite.Status = "done"
				require.True(t, gen.EligibleTask(tree, dependent))
				require.NotEmpty(t, candidatesFor(nextFor(tree, "jc", workflow.Input{}).Candidates, dependent.ID))
			})
		}
	}
}

func TestCoarseStartedOfflineReviewStillRidesAggregate(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	f.ReviewGrain = store.ReviewGrainFeature
	task.Status, task.StartedGrain = "in_review", store.ReviewGrainFeature
	task.ReviewRef = &store.ReviewRef{Branch: "task-branch", Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:abc1234"}}
	require.Equal(t, f, workflow.ReviewOwnerOf(tree, task))
	_, missing := workflow.ReviewStateOf(tree, f)
	require.True(t, missing, "an actual aggregate rider still requires its aggregate review")
}
