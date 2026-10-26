package views_test

// The approve-epic move: right after decompose every epic is
// draft awaiting `tiller approve epic`, and the route used to walk past that
// gate to the frontier and name specify. These tests pin the move, its place
// in plan order among the other moves, and the frontier's matching skip.

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// draftFeatureUnder adds one draft feature under the epic whose key is
// epicKey, the shape decompose leaves every feature in.
func draftFeatureUnder(t *testing.T, tree *store.Tree, id, key, epicKey string) *store.Feature {
	t.Helper()
	f := feat(t, id, key, epicKey)
	f.Status = "draft"
	tree.Features[id] = f
	return f
}

// TestTopMoveIsTheApproveEpicOnAnAllDraftTree pins the move itself: a draft
// epic carrying a live feature is the human's approval, not a specify seam.
// The node is the epic, and the decompose readout's Next names the approve
// command and offers no spec of a feature under the draft epic.
func TestTopMoveIsTheApproveEpicOnAnAllDraftTree(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	draftFeatureUnder(t, tree, "F-aaaaa0", "epic-0/alpha", "epic-0")

	top := topMove(t, tree, "jc", workflow.Input{})
	require.Equal(t, "approve_epic", top.Kind)
	require.Empty(t, top.Skill, "the approve-epic gate is the human's move, not a seam")
	require.Equal(t, "its decomposition awaits your approval", top.Reason)
	require.Equal(t, store.NounEpic, top.Node.Noun)
	require.Equal(t, "E-aaaaa0", top.Node.ID)

	r := views.ReadoutOfInput(tree, views.MomentDecompose, views.ReadoutInput{})
	require.Equal(t, []string{"approve_epic"}, nextKinds(r), "a feature under a draft epic is not yet a spec move")
	top = topNext(t, r)
	require.Equal(t, "E-aaaaa0", top.Node.ID)
	require.Equal(t, "tiller approve epic epic-0", top.Command)
	require.Equal(t, "human", top.Actor, "approving the tree is the human's move")
}

// TestDecomposeNextListsDraftEpicsInSoftOrder pins the several-epics shape:
// Next lists each draft epic's approval in recorded soft order, the same list
// on every moment.
func TestDecomposeNextListsDraftEpicsInSoftOrder(t *testing.T) {
	tree := treeWithDraftEpics(t, 3)
	draftFeatureUnder(t, tree, "F-aaaaa0", "epic-0/alpha", "epic-0")
	draftFeatureUnder(t, tree, "F-aaaaa1", "epic-1/beta", "epic-1")
	// epic-2 stays featureless.
	tree.ProjectOrder.Epics = []string{"E-aaaaa1", "E-aaaaa0"}

	require.Equal(t, "E-aaaaa1", topMove(t, tree, "jc", workflow.Input{}).Node.ID,
		"the soft order, not key order, ranks the first waiting epic")

	decompose := views.ReadoutOfInput(tree, views.MomentDecompose, views.ReadoutInput{})
	// The featureless epic is still a legal approval; it ranks last.
	require.Equal(t, []string{"E-aaaaa1", "E-aaaaa0", "E-aaaaa2"}, nextNodeIDs(decompose), "the soft order leads")
	require.Equal(t, "tiller approve epic epic-1", topNext(t, decompose).Command)

	orient := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	require.Equal(t, decompose.Next, orient.Next, "every moment ends on the same list")
}

// TestApproveEpicIsAMoveWithoutALiveFeature pins the ranked-next ruling that
// replaced the walk's live-feature guard: approving a draft epic is legal and
// the queue lists it, so a draft epic with nothing live under it (no
// features, or only cancelled ones) is still a move.
func TestApproveEpicIsAMoveWithoutALiveFeature(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"approve_epic"}, candidateKinds(next.Candidates))

	cancelled := draftFeatureUnder(t, tree, "F-aaaaa0", "epic-0/alpha", "epic-0")
	cancelled.Status = "cancelled"
	next = nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"approve_epic"}, candidateKinds(next.Candidates), "a cancelled feature neither counts nor blocks")
}

// TestFrontierSkipsADraftFeatureUnderADraftEpic pins the frontier's half of
// the fix: the frontier only ever offers a feature under an approved epic.
// The draft-epic feature sits first in flow order, so without the skip it
// would be the pick; with only it in the tree the frontier is empty rather
// than pointing past the gate. `tiller spec feature` stays legal on it; this
// shapes the suggestion alone.
func TestFrontierSkipsADraftFeatureUnderADraftEpic(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa1"].Status = "ready"
	draftFeatureUnder(t, tree, "F-aaaaa0", "epic-0/alpha", "epic-0")
	draftFeatureUnder(t, tree, "F-aaaaa1", "epic-1/beta", "epic-1")

	require.Equal(t, []string{"F-aaaaa1"}, frontierIDs(tree), "the feature under the approved epic is the frontier")

	delete(tree.Features, "F-aaaaa1")
	require.Empty(t, frontierIDs(tree), "a draft feature under a draft epic is never on the frontier")
}

// TestExecuteOutranksTheApproveEpicWait pins that a task the reader holds is
// still the first move, even while another epic's decomposition waits on the
// human.
func TestExecuteOutranksTheApproveEpicWait(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	draftFeatureUnder(t, tree, "F-aaaaa0", "epic-0/alpha", "epic-0")
	tree.Epics["E-aaaaa1"].Status = "ready"
	ready := feat(t, "F-aaaaa1", "epic-1/beta", "epic-1")
	ready.Status = "ready"
	tree.Features["F-aaaaa1"] = ready
	task := &store.Task{ID: "T-aaaaa1", Key: strPtr("epic-1/beta/step"), Status: "ready", Owner: "jc", FeatureID: "F-aaaaa1"}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{"F-aaaaa1": {task}}

	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"continue", "approve_epic"}, candidateKinds(next.Candidates))
	require.Equal(t, "T-aaaaa1", next.Candidates[0].Node.ID)
}

// TestPlanGateOutranksTheApproveEpicWait pins plan order between the two
// gates: the gate the human already has open on a plan comes before the
// decomposition gate one level up.
func TestPlanGateOutranksTheApproveEpicWait(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	draftFeatureUnder(t, tree, "F-aaaaa0", "epic-0/alpha", "epic-0")
	tree.Epics["E-aaaaa1"].Status = "ready"
	specced := feat(t, "F-aaaaa1", "epic-1/beta", "epic-1")
	specced.Status = "specced"
	tree.Features["F-aaaaa1"] = specced

	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"approve_feature", "approve_epic"}, candidateKinds(next.Candidates))
	require.Equal(t, "F-aaaaa1", next.Candidates[0].Node.ID)
	require.Equal(t, "its feature plan awaits your approval", next.Candidates[0].Reason)
}

// TestApproveEpicWaitOutranksTheFrontier pins plan order above the frontier:
// an open frontier under an approved epic ranks after a decomposition that
// waits, and a draft under the draft epic that is also dependency-gated is no
// move of its own; the approval is (the blind spot in claimable routing).
func TestApproveEpicWaitOutranksTheFrontier(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa1"].Status = "ready"
	inflight := feat(t, "F-nflght", "epic-1/inflight", "epic-1")
	inflight.Status = "active"
	tree.Features["F-nflght"] = inflight
	draftFeatureUnder(t, tree, "F-aaaaa1", "epic-1/open", "epic-1")
	gated := draftFeatureUnder(t, tree, "F-aaaaa0", "epic-0/alpha", "epic-0")
	gated.DependsOn = []string{"F-nflght"}

	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"approve_epic", "specify"}, candidateKinds(next.Candidates))
	require.Equal(t, []string{"E-aaaaa0", "F-aaaaa1"}, candidateNodeIDs(next.Candidates))
}

// TestTopMoveIsTheFrontierOnceTheEpicIsApproved is the arc's second leg: the
// human's approve clears the wait and the same draft feature becomes the
// specify move.
func TestTopMoveIsTheFrontierOnceTheEpicIsApproved(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	draftFeatureUnder(t, tree, "F-aaaaa0", "epic-0/alpha", "epic-0")
	tree.Epics["E-aaaaa0"].Status = "ready"

	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"specify"}, candidateKinds(next.Candidates))
	require.Equal(t, "F-aaaaa0", next.Candidates[0].Node.ID)
}
