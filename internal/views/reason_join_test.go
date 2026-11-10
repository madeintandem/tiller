package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// Workflow reasons are sentences ending in a period, so nothing glues the
// next clause onto one with "; ".

func TestResolvedReviewCheckFollowsItsReasonSentenceWithTheCommand(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	green := store.GatesBlock{Gates: map[string]store.Gate{"tests": {Required: true, Status: "pass", Evidence: []store.Evidence{{Kind: "command_output", Ref: "test.txt"}}}}}
	task := tree.Tasks["T-aaaaa0"]
	task.ReviewRef, task.GatesBlock = &store.ReviewRef{Branch: "implementation"}, green
	advice := workflow.AdviceOf(tree, f, in)
	require.NotEmpty(t, advice.Command)
	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc", Workflow: in})
	got := checkLabels(r)["Resolved review "+gen.Ref(tree, f)]
	require.Equal(t, advice.Reason+" `"+advice.Command+"`", got)
}

func mergeFeatureProse(tree *store.Tree, in workflow.Input) string {
	for _, e := range views.QueueOfInput(tree, views.QueueInput{Workflow: in}).Entries {
		if e.Kind == "merge_feature" {
			return e.Prose
		}
	}
	return ""
}

func TestMergeCardLeadsWithThePRAsItsOwnSentence(t *testing.T) {
	tree, in := reviewPhaseTree(t, store.ReviewGrainFeature)
	f := tree.Features["F-aaaaa0"]
	advice := workflow.AdviceOf(tree, f, in)
	require.Equal(t, "PR #42. "+advice.Reason, mergeFeatureProse(tree, in))

	f.PR, f.ReviewRef = nil, &store.ReviewRef{Branch: "tiller/reviews/x"}
	advice = workflow.AdviceOf(tree, f, in)
	require.Equal(t, "Branch tiller/reviews/x. "+advice.Reason, mergeFeatureProse(tree, in))
}

func TestBlockedFeatureReasonClosesTheProblemBeforeTheReviewGuidance(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	in.Eligibility = map[string]string{f.ID: "current accepted basis drift"}
	route := topMove(t, tree, "jc", in)
	require.Equal(t, "resolve_blocker", route.Kind)
	require.NotContains(t, route.Reason, "; Aggregate")
	require.Contains(t, route.Reason, "drift. "+route.Workflow.ReviewGuidance)
}
