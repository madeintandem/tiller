package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// observedRouteTree lays two ready, unclaimed tasks in one ready epic, first
// before second in recorded order (issue #149).
func observedRouteTree(t *testing.T) (tree *store.Tree, first, second *store.Task) {
	t.Helper()
	tree = treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f, first := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	second = &store.Task{ID: "T-bbbbb0", Key: strPtr("epic-0/work/second"), Status: "ready", FeatureID: f.ID}
	tree.Tasks[second.ID] = second
	tree.FeatureTasks[f.ID] = append(tree.FeatureTasks[f.ID], second)
	f.Order = []string{first.ID, second.ID}
	return tree, first, second
}

func observedOn(id, branch, owner string) workflow.Input {
	return workflow.Input{Observed: map[string]workflow.Observation{id: {Branch: branch, Owner: owner}}}
}

func TestReadersOwnObservedTaskContinuesOnItsBranch(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	next := nextFor(tree, "jc", observedOn(task.ID, "tiller/epic-0/work/first", "jc"))
	require.Len(t, next.Candidates, 1, "the task in flight is not also a claim")
	top := next.Candidates[0]
	require.Equal(t, "continue", top.Kind)
	require.Equal(t, task.ID, top.Node.ID)
	require.Equal(t, "tiller/epic-0/work/first", top.Branch)
	require.Empty(t, top.Command, "it continues on its branch, not from this checkout")
	require.Empty(t, top.Skill)
	require.Nil(t, top.Workflow)
	require.Equal(t, "your task, in flight on its branch", top.Reason)
}

// The parallel claim the retired note offered is the next claim_task.
func TestReadersOwnObservedTaskLeavesTheNextClaimAsAMove(t *testing.T) {
	tree, first, second := observedRouteTree(t)
	next := nextFor(tree, "jc", observedOn(first.ID, "tiller/epic-0/work/first", "jc"))
	require.Equal(t, []string{"continue", "claim_task"}, candidateKinds(next.Candidates))
	require.Equal(t, []string{first.ID, second.ID}, candidateNodeIDs(next.Candidates))
	require.Equal(t, "tiller claim task epic-0/work/second", next.Candidates[1].Command)
}

func TestOwnObservedTaskReachedThroughTheOwnedPassContinuesOnItsBranch(t *testing.T) {
	tree, first, second := observedRouteTree(t)
	first.Owner, first.Status, first.Started = "jc", "in_progress", true
	next := nextFor(tree, "jc", observedOn(first.ID, "tiller/epic-0/work/first", "jc"))
	require.Equal(t, []string{"continue", "claim_task"}, candidateKinds(next.Candidates))
	require.Equal(t, []string{first.ID, second.ID}, candidateNodeIDs(next.Candidates))
	require.Equal(t, "tiller/epic-0/work/first", next.Candidates[0].Branch)
}

func TestObservedTaskOwnedByOtherIsNoMove(t *testing.T) {
	tree, first, second := observedRouteTree(t)
	next := nextFor(tree, "jc", observedOn(first.ID, "tiller/epic-0/work/first", "robin"))
	require.Equal(t, []string{"claim_task"}, candidateKinds(next.Candidates))
	require.Equal(t, []string{second.ID}, candidateNodeIDs(next.Candidates), "robin's task in flight is passed over")
}

func TestObservedTaskWithEmptyOwnerIsNoMove(t *testing.T) {
	tree, first, second := observedRouteTree(t)
	for _, reader := range []string{"jc", ""} {
		next := nextFor(tree, reader, observedOn(first.ID, "tiller/epic-0/work/first", ""))
		require.Equal(t, []string{"claim_task"}, candidateKinds(next.Candidates), "an unset observed owner is never the reader's")
		require.Equal(t, []string{second.ID}, candidateNodeIDs(next.Candidates))
	}
}

func TestObservedOwnTaskOutranksAnEarlierClaim(t *testing.T) {
	tree, first, second := observedRouteTree(t)
	next := nextFor(tree, "jc", observedOn(second.ID, "tiller/epic-0/work/second", "jc"))
	require.Equal(t, []string{"continue", "claim_task"}, candidateKinds(next.Candidates))
	require.Equal(t, []string{second.ID, first.ID}, candidateNodeIDs(next.Candidates))
}

func TestEveryTaskObservedElsewhereLeavesLaterMoves(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	draftFeatureUnder(t, tree, "F-aaaaa1", "epic-0/later", "epic-0")
	next := nextFor(tree, "jc", observedOn(task.ID, "tiller/epic-0/work/first", "robin"))
	require.Equal(t, []string{"specify"}, candidateKinds(next.Candidates))
	require.Equal(t, []string{"F-aaaaa1"}, candidateNodeIDs(next.Candidates))
}

// questionHeldSiblingTree reproduces issue #139's shape: epic-0 orders first
// and holds a ready unclaimed task; the reader closed out a task in
// epic-1/approval, whose first remaining sibling is question-held and whose
// second is eligible.
func questionHeldSiblingTree(t *testing.T) (tree *store.Tree, elsewhere, held, sibling *store.Task) {
	t.Helper()
	tree = treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"
	tree.ProjectOrder.Epics = []string{"E-aaaaa0", "E-aaaaa1"}
	_, elsewhere = readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/records", "epic-0")
	f, closed := readyFeatureWithTask(t, tree, "F-aaaaa1", "epic-1/approval", "epic-1")
	f.Status = "active"
	closed.Status, closed.Started, closed.Owner = "done", true, "reader"
	held = &store.Task{ID: "T-bbbbb1", Key: strPtr("epic-1/approval/decisions"), Status: "ready", FeatureID: f.ID}
	sibling = &store.Task{ID: "T-ccccc1", Key: strPtr("epic-1/approval/correction"), Status: "ready", FeatureID: f.ID}
	tree.Tasks[held.ID], tree.Tasks[sibling.ID] = held, sibling
	tree.FeatureTasks[f.ID] = append(tree.FeatureTasks[f.ID], held, sibling)
	f.Order = []string{closed.ID, held.ID, sibling.ID}
	holdClaimableNode(tree, "Q-gsssu6", held.ID)
	return tree, elsewhere, held, sibling
}

// An observation elsewhere and a question hold in the same feature each
// remove their task from the moves; the holding question stays one.
func TestObservationsAndQuestionHoldsBothRemoveTheirTasks(t *testing.T) {
	tree, elsewhere, held, sibling := questionHeldSiblingTree(t)
	next := nextFor(tree, "reader", observedOn(sibling.ID, "tiller/epic-1/approval/correction", "robin"))
	require.Equal(t, []string{"claim_task", "answer_question"}, candidateKinds(next.Candidates))
	require.Equal(t, []string{elsewhere.ID, "Q-gsssu6"}, candidateNodeIDs(next.Candidates))
	require.Empty(t, candidatesFor(next.Candidates, held.ID))
	require.Empty(t, candidatesFor(next.Candidates, sibling.ID))
}

// The caught-up pool signpost leaves a task observed in flight under someone
// else out of its count, so it never misdiagnoses that task as held (R4), and
// it reads in the singular for one remaining task.
func TestPoolSignpostLeavesOthersObservedTaskOut(t *testing.T) {
	tree, first, second := observedRouteTree(t)
	second.Blocked = true
	in := observedOn(first.ID, "tiller/epic-0/work/first", "robin")
	order := gen.DependencyOrder(tree)
	require.True(t, nextFor(tree, "jc", in).CaughtUp)
	require.Equal(t, "There is 1 unclaimed task; the first in dependency order, epic-0/work/second (T-bbbbb0), is manually blocked",
		views.PoolSignpost(tree, order, in))

	readout := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{Contributor: "jc", Workflow: in})
	require.True(t, readout.Next.CaughtUp)
	require.Equal(t, "There is 1 unclaimed task; the first in dependency order, epic-0/work/second (T-bbbbb0), is manually blocked", readout.Next.Hint)

	second.Blocked = false
	second.Status = "done"
	require.Empty(t, views.PoolSignpost(tree, order, in), "nothing else remains")
	readout = views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{Contributor: "jc", Workflow: in})
	require.True(t, readout.Next.CaughtUp)
	require.Empty(t, readout.Next.Hint)
}
