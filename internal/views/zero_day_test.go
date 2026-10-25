package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// confirmedBrownfield is the trial's day-one state: one confirmed as-is doc,
// one still pending, no epics, no to-be docs.
func confirmedBrownfield(t *testing.T) *store.Tree {
	tree := treeWithDraftEpics(t, 0)
	tree.Manifest.Project.Kind = store.KindBrownfield
	tree.Baseline.Docs = []store.BaselineDoc{
		{Path: "as-is/app.md", Describes: []string{"."}, Confirmed: baselineConfirmation()},
		{Path: "as-is/store.md", Describes: []string{"."}},
	}
	return tree
}

// rankedForJC is the ranked next list for jc, with no observations.
func rankedForJC(tree *store.Tree) views.Next {
	return nextFor(tree, "jc", workflow.Input{})
}

// The confirmed baseline's pending doc is the one move left, and the hint
// says how work enters.
func TestConfirmedBaselineWithNoToBeDocIsUnplanned(t *testing.T) {
	n := rankedForJC(confirmedBrownfield(t))
	require.True(t, n.Unplanned)
	require.Equal(t, []string{"confirm_design"}, candidateKinds(n.Candidates))
	require.Equal(t, "no work is planned yet (baseline established): bring work in through intake (a document making a case for change), capture (a bug, chore, or tweak), or a to-be design under .tiller/design/ (which makes decompose due)", n.Hint)
}

func TestLegacyMigrationUsesTheExistingCodeWording(t *testing.T) {
	tree := confirmedBrownfield(t)
	tree.Manifest.Project.Kind = store.KindLegacyMigration
	require.Contains(t, rankedForJC(tree).Hint, "(baseline established)")
}

func TestUnplannedNamesTheOpenQuestionCount(t *testing.T) {
	tree := confirmedBrownfield(t)
	tree.Questions = map[string]*store.Question{
		"Q-aaaaaa": {ID: "Q-aaaaaa", Status: "open"},
	}
	require.Contains(t, rankedForJC(tree).Hint, "(baseline established, 1 open question)")
	tree.Questions["Q-bbbbbb"] = &store.Question{ID: "Q-bbbbbb", Status: "open"}
	require.Contains(t, rankedForJC(tree).Hint, "(baseline established, 2 open questions)")
	tree.Questions["Q-bbbbbb"].Status = "answered"
	require.Contains(t, rankedForJC(tree).Hint, "(baseline established, 1 open question)")
}

func TestOneToBeDocMakesDecomposeDue(t *testing.T) {
	tree := confirmedBrownfield(t)
	tree.Baseline.Docs[1].Confirmed = baselineConfirmation()
	tree.ToBeDesign = []string{"sync.md"}
	n := rankedForJC(tree)
	require.False(t, n.CaughtUp)
	require.False(t, n.Unplanned)
	require.Len(t, n.Candidates, 1)
	top := n.Candidates[0]
	require.Equal(t, "decompose", top.Kind)
	require.Equal(t, "decompose", top.Skill)
	require.Nil(t, top.Node)
	require.Equal(t, "no epics recorded yet; 1 to-be design doc awaits decomposition, starting with .tiller/design/sync.md", top.Reason)

	tree.ToBeDesign = []string{"a.md", "b.md"}
	require.Equal(t, "no epics recorded yet; 2 to-be design docs await decomposition, starting with .tiller/design/a.md", topMove(t, tree, "jc", workflow.Input{}).Reason)
}

func TestGreenfieldWithNoDesignIsUnplanned(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.Manifest.Project.Kind = store.KindGreenfield
	n := rankedForJC(tree)
	require.True(t, n.CaughtUp)
	require.True(t, n.Unplanned)
	require.Equal(t, "no work is planned yet: add design inputs under .tiller/design/ for decompose to turn into a draft tree", n.Hint)

	tree.Questions = map[string]*store.Question{"Q-aaaaaa": {ID: "Q-aaaaaa", Status: "open"}}
	require.Equal(t, "no work is planned yet (1 open question): add design inputs under .tiller/design/ for decompose to turn into a draft tree", rankedForJC(tree).Hint)
}

// A non-empty tree never consults ToBeDesign: with epics, decompose is no
// move and the end state never appears.
func TestATreeWithEpicsIgnoresToBeDesign(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.ToBeDesign = []string{"sync.md"}
	n := rankedForJC(tree)
	require.Empty(t, candidatesOfKind(n.Candidates, "decompose"))
	require.False(t, n.Unplanned)
}

func TestUnplannedNextIsTheHumansReason(t *testing.T) {
	tree := confirmedBrownfield(t)
	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{Contributor: "jc"})
	require.True(t, r.Next.Unplanned)
	require.Equal(t, rankedForJC(tree).Hint, r.Next.Hint)
}
