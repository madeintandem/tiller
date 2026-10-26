package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func shelfRouteTree() *store.Tree {
	epicKey, featureKey, bugKey, choreKey := "foundation", "foundation/app", "bugs/blank", "chores/bump"
	return &store.Tree{
		Manifest: &store.Manifest{},
		Epics: map[string]*store.Epic{
			"E-aaaaaa": {ID: "E-aaaaaa", Key: &epicKey, Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-aaaaaa": {ID: "F-aaaaaa", Key: &featureKey, EpicKey: epicKey, Status: "ready"},
		},
		Bugs: map[string]*store.Bug{
			"B-aaaaaa": {WorkItem: store.WorkItem{ID: "B-aaaaaa", Key: &bugKey, Title: "Blank", Status: "ready", Owner: "jc"}},
		},
		Chores: map[string]*store.Chore{
			"CH-aaaaaa": {WorkItem: store.WorkItem{ID: "CH-aaaaaa", Key: &choreKey, Title: "Bump", Status: "ready"}},
		},
	}
}

func TestNextContinuesAnOwnedReadyBugAndSkipsAnUnclaimedChore(t *testing.T) {
	tree := shelfRouteTree()
	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"continue"}, candidateKinds(next.Candidates))
	top := next.Candidates[0]
	require.Equal(t, "execute", top.Skill)
	require.Equal(t, "B-aaaaaa", top.Node.ID)
	require.Equal(t, "bugs/blank (B-aaaaaa)", top.Node.Ref)
}

func TestNextKeepsAnOwnedTaskAheadOfOwnedShelfWork(t *testing.T) {
	tree := shelfRouteTree()
	tree.Bugs["B-aaaaaa"].Status = "check"
	taskKey := "foundation/app/route"
	task := &store.Task{ID: "T-aaaaaa", Key: &taskKey, Status: "ready", Owner: "jc", FeatureID: "F-aaaaaa"}
	tree.Tasks = map[string]*store.Task{"T-aaaaaa": task}
	tree.FeatureTasks = map[string][]*store.Task{"F-aaaaaa": {task}}
	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"T-aaaaaa", "B-aaaaaa"}, candidateNodeIDs(candidatesOfKind(next.Candidates, "continue")))
	require.Equal(t, "T-aaaaaa", next.Candidates[0].Node.ID)
}

func TestNextSkipsCarriedAndContinuesCarrierCancelledShelfNodes(t *testing.T) {
	tree := shelfRouteTree()
	tree.Bugs["B-aaaaaa"].ResolvedBy = "F-aaaaaa"
	require.Empty(t, candidatesFor(nextFor(tree, "jc", workflow.Input{}).Candidates, "B-aaaaaa"))

	tree.Features["F-aaaaaa"].Status = "cancelled"
	top := topMove(t, tree, "jc", workflow.Input{})
	require.Equal(t, "continue", top.Kind)
	require.Equal(t, "B-aaaaaa", top.Node.ID)
}

func TestNextNamesAnInReviewBugAsAHumanMergeWait(t *testing.T) {
	tree := shelfRouteTree()
	tree.Bugs["B-aaaaaa"].Status = "in_review"
	top := topMove(t, tree, "jc", workflow.Input{})
	require.Equal(t, "merge_task", top.Kind)
	require.Equal(t, "human", top.Actor)
	require.Empty(t, top.Skill)
	require.Equal(t, "B-aaaaaa", top.Node.ID)
	require.Equal(t, "in review, waiting on your merge", top.Reason)
}

func TestReadyShelfQueueLineCountsReadyButNotCarried(t *testing.T) {
	tree := shelfRouteTree()
	carriedKey := "bugs/carried"
	tree.Bugs["B-carry0"] = &store.Bug{WorkItem: store.WorkItem{ID: "B-carry0", Key: &carriedKey, Status: "ready", ResolvedBy: "F-aaaaaa"}}
	require.Equal(t, "queue: 1 bug, 1 chore, 0 tweaks ready", views.ReadyShelfQueueLine(tree))
}

// epiclessShelfTree is shelfRouteTree with its epic and feature removed: the
// state of a brownfield project the day its baseline is confirmed and a bug
// is captured, before any epic exists.
func epiclessShelfTree() *store.Tree {
	tree := shelfRouteTree()
	tree.Epics = map[string]*store.Epic{}
	tree.Features = map[string]*store.Feature{}
	return tree
}

func TestEpiclessTreeContinuesAnOwnedReadyBugBeforeDecompose(t *testing.T) {
	tree := epiclessShelfTree()
	tree.ToBeDesign = []string{"roadmap.md"}
	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"continue", "decompose"}, candidateKinds(next.Candidates))
	require.Equal(t, "B-aaaaaa", next.Candidates[0].Node.ID)
}

func TestEpiclessTreeContinuesAnOwnedCheckedBugBeforeDecompose(t *testing.T) {
	tree := epiclessShelfTree()
	tree.ToBeDesign = []string{"roadmap.md"}
	tree.Bugs["B-aaaaaa"].Status = "check"
	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"continue", "decompose"}, candidateKinds(next.Candidates))
	top := next.Candidates[0]
	require.Equal(t, "B-aaaaaa", top.Node.ID)
	require.Contains(t, top.Reason, "finish any outstanding checks")
}

func TestEpiclessTreeNamesAnInReviewBugBeforeDecompose(t *testing.T) {
	tree := epiclessShelfTree()
	tree.ToBeDesign = []string{"roadmap.md"}
	tree.Bugs["B-aaaaaa"].Status = "in_review"
	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"merge_task", "decompose"}, candidateKinds(next.Candidates))
	require.Equal(t, "B-aaaaaa", next.Candidates[0].Node.ID)
}

// Unclaimed and other-owned shelf work is never the reader's move, so
// decompose is the only one.
func TestEpiclessTreeWithOnlyOthersShelfWorkStillOffersDecompose(t *testing.T) {
	tree := epiclessShelfTree()
	tree.ToBeDesign = []string{"roadmap.md"}
	tree.Bugs["B-aaaaaa"].Owner = "al"
	next := nextFor(tree, "jc", workflow.Input{})
	require.False(t, next.CaughtUp)
	require.Equal(t, []string{"decompose"}, candidateKinds(next.Candidates))
	require.Contains(t, next.Candidates[0].Reason, "no epics recorded yet")
	require.Nil(t, next.Candidates[0].Node)
}

// A carrier is always an epic, feature, or task, so on a tree with no epics a
// resolved_by pointer can only dangle. gen.WorkActivity reads a carrier the
// tree no longer holds as carrier_cancelled, which returns owned work to its
// own shelf: the bug continues rather than hiding behind decompose.
func TestEpiclessTreeContinuesAnOwnedBugWhoseCarrierIsGone(t *testing.T) {
	tree := epiclessShelfTree()
	tree.Bugs["B-aaaaaa"].ResolvedBy = "F-aaaaaa"
	top := topMove(t, tree, "jc", workflow.Input{})
	require.Equal(t, "continue", top.Kind)
	require.Equal(t, "B-aaaaaa", top.Node.ID)
}
