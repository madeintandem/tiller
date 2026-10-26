package views_test

import (
	"fmt"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// queuePlanTree builds a live epic holding one feature in the given status with
// one task per entry in taskStatuses, ids T-plan0, T-plan1, and so on.
func queuePlanTree(featureStatus string, taskStatuses ...string) (*store.Tree, *store.Feature, []*store.Task) {
	epicKey, featKey := "foundation", "foundation/widgets"
	feat := &store.Feature{ID: "F-plan00", Key: &featKey, Status: featureStatus, EpicKey: epicKey}
	tree := &store.Tree{
		Manifest:     &store.Manifest{},
		Epics:        map[string]*store.Epic{"E-plan00": {ID: "E-plan00", Key: &epicKey, Status: "ready"}},
		Features:     map[string]*store.Feature{feat.ID: feat},
		Tasks:        map[string]*store.Task{},
		FeatureTasks: map[string][]*store.Task{},
		Questions:    map[string]*store.Question{},
	}
	var tasks []*store.Task
	for i, status := range taskStatuses {
		key := fmt.Sprintf("foundation/widgets/t%d", i)
		task := &store.Task{ID: fmt.Sprintf("T-plan%d", i), Key: &key, Status: status, FeatureID: feat.ID}
		tree.Tasks[task.ID] = task
		tree.FeatureTasks[feat.ID] = append(tree.FeatureTasks[feat.ID], task)
		tasks = append(tasks, task)
	}
	return tree, feat, tasks
}

func holdQueuePlanNode(tree *store.Tree, questionID, nodeID string) {
	tree.Questions[questionID] = &store.Question{
		ID: questionID, Title: "Which schema?", Statement: "Which schema does this work read?",
		Status: "open", Blocks: []string{nodeID},
	}
}

// TestQueueHidesHeldFeaturePlan extends the suppress-not-exempt rule to a plan
// an open question holds: `tiller approve feature` refuses while any open
// question names the feature, its epic, or one of its live tasks, and a queue
// whose commands fail when copied is worse than silence. The
// question's own answer_question row is what takes the row's place.
func TestQueueHidesHeldFeaturePlan(t *testing.T) {
	tree, _, tasks := queuePlanTree("specced", "proposed", "proposed")
	require.Len(t, entriesOfKind(views.QueueOf(tree), "approve_feature"), 1,
		"an unheld specced feature still queues the row")

	holdQueuePlanNode(tree, "Q-plan00", tasks[1].ID)
	q := views.QueueOf(tree)
	require.Empty(t, entriesOfKind(q, "approve_feature"),
		"a question on one task holds the whole plan, so the row is gone")
	answers := entriesOfKind(q, "answer_question")
	require.Len(t, answers, 1)
	require.Equal(t, "Q-plan00", answers[0].ID)

	tree.Questions["Q-plan00"].Status = "answered"
	require.Len(t, entriesOfKind(views.QueueOf(tree), "approve_feature"), 1,
		"an answered question releases the plan and the row returns")
}

// TestQueueCancelledQuestionReleasesFeaturePlan: a cancelled question holds
// nothing, so the plan row returns and no answer_question row remains.
func TestQueueCancelledQuestionReleasesFeaturePlan(t *testing.T) {
	tree, _, tasks := queuePlanTree("specced", "proposed")
	holdQueuePlanNode(tree, "Q-plan00", tasks[0].ID)
	require.Empty(t, entriesOfKind(views.QueueOf(tree), "approve_feature"))

	tree.Questions["Q-plan00"].Status = "cancelled"
	q := views.QueueOf(tree)
	require.Len(t, entriesOfKind(q, "approve_feature"), 1,
		"a cancelled question releases the plan and the row returns")
	require.Empty(t, entriesOfKind(q, "answer_question"))
}

// TestQueueApproveTasksRowNeedsOneUnheld pins the approve_tasks row against the
// batch it names: `tiller approve task --feature` approves every proposed task
// it can, so the row stays while at least one proposed task is unheld and goes
// only when every proposed task is held.
func TestQueueApproveTasksRowNeedsOneUnheld(t *testing.T) {
	tree, _, tasks := queuePlanTree("ready", "proposed", "proposed")
	require.Len(t, entriesOfKind(views.QueueOf(tree), "approve_tasks"), 1)

	holdQueuePlanNode(tree, "Q-plan00", tasks[0].ID)
	require.Len(t, entriesOfKind(views.QueueOf(tree), "approve_tasks"), 1,
		"one unheld proposed task keeps the row")

	holdQueuePlanNode(tree, "Q-plan01", tasks[1].ID)
	require.Empty(t, entriesOfKind(views.QueueOf(tree), "approve_tasks"),
		"every proposed task held: nothing for the row to approve")
}

// TestQueueApproveTasksRowIgnoresHeldNonProposedTasks pins that a hold on a
// task the batch does not cover (a ready sibling) neither hides nor keeps the
// row: only the proposed tasks count.
func TestQueueApproveTasksRowIgnoresHeldNonProposedTasks(t *testing.T) {
	tree, _, tasks := queuePlanTree("active", "ready", "proposed")
	holdQueuePlanNode(tree, "Q-plan00", tasks[0].ID)
	require.Len(t, entriesOfKind(views.QueueOf(tree), "approve_tasks"), 1,
		"the held task is ready, not proposed, so the proposed one still queues")
}

// TestQueueApproveTasksRowHeldByFeatureQuestion pins that a question naming the
// feature itself holds every task under it, so the row is gone.
func TestQueueApproveTasksRowHeldByFeatureQuestion(t *testing.T) {
	tree, feat, _ := queuePlanTree("ready", "proposed", "proposed")
	holdQueuePlanNode(tree, "Q-plan00", feat.ID)
	require.Empty(t, entriesOfKind(views.QueueOf(tree), "approve_tasks"))
}

// TestEnumerateHeldPlanYieldsTheQuestionNotTheApprove pins the routing effect
// of the queue hiding a plan approval an open question holds: routing reads
// queue entries, so the held specced feature offers no approve-the-spec
// candidate and the question's answer candidate stands in for it. The unheld
// feature is the control.
func TestEnumerateHeldPlanYieldsTheQuestionNotTheApprove(t *testing.T) {
	tree, feat, _ := queuePlanTree("specced")

	approves := candidatesOfKind(enumerate(tree, "jc", workflow.Input{}), "approve_feature")
	require.Len(t, approves, 1, "an unheld specced feature offers its approve")
	require.Equal(t, "approve the spec for", approves[0].Action)

	holdQueuePlanNode(tree, "Q-plan00", feat.ID)
	cands := enumerate(tree, "jc", workflow.Input{})
	require.Empty(t, candidatesOfKind(cands, "approve_feature"),
		"a held plan offers no approve the spec for candidate")
	answers := candidatesOfKind(cands, "answer_question")
	require.Len(t, answers, 1)
	require.Equal(t, "Q-plan00", answers[0].Node.ID)
}
