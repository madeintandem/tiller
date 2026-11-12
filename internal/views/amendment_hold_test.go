package views_test

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// These tests pin the read surfaces that say a draft is held (decision 7): the
// node view, the Build card, the list row, the map card, and the status
// oracle each carry the holding amendment for a held feature and nothing for
// an unheld one. mapTree's F-2 is the held feature, F-3 the unheld one.

func heldFeatureTree() *store.Tree {
	tree := mapTree()
	holdByAmendment(tree, "A-aaaaaa", "F-2")
	return tree
}

func requireHoldView(t *testing.T, got *views.AmendmentHoldView) {
	t.Helper()
	require.NotNil(t, got)
	require.Equal(t, views.AmendmentHoldView{
		ID: "A-aaaaaa", Key: "iphone-only", Ref: "iphone-only (A-aaaaaa)", Title: "iPhone only", Status: "draft",
	}, *got)
}

func TestNodeViewCarriesAmendmentHold(t *testing.T) {
	tree := heldFeatureTree()
	requireHoldView(t, views.Node(tree, tree.Features["F-2"]).AmendmentHold)
	require.Nil(t, views.Node(tree, tree.Features["F-3"]).AmendmentHold)

	raw, err := json.Marshal(views.Node(tree, tree.Features["F-3"]))
	require.NoError(t, err)
	require.NotContains(t, string(raw), "amendment_hold")
	raw, err = json.Marshal(views.Node(tree, tree.Features["F-2"]))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"amendment_hold":{"id":"A-aaaaaa","key":"iphone-only","ref":"iphone-only (A-aaaaaa)","title":"iPhone only","status":"draft"}`)
}

func TestBuildCardCarriesAmendmentHold(t *testing.T) {
	tree := heldFeatureTree()
	cards := map[string]views.BuildFeature{}
	for _, e := range views.BuildBoardOf(tree).Epics {
		for _, f := range e.Features {
			cards[f.ID] = f
		}
	}
	requireHoldView(t, cards["F-2"].AmendmentHold)
	require.Nil(t, cards["F-3"].AmendmentHold)
	// A held draft still counts as the draft it is on the board.
	require.Equal(t, "draft", cards["F-2"].Column)
}

func TestListItemCarriesAmendmentHold(t *testing.T) {
	tree := heldFeatureTree()
	requireHoldView(t, views.Item(tree, tree.Features["F-2"]).AmendmentHold)
	require.Nil(t, views.Item(tree, tree.Features["F-3"]).AmendmentHold)
}

func TestMapNodeCarriesAmendmentHold(t *testing.T) {
	tree := heldFeatureTree()
	got := map[string]views.MapNode{}
	for _, n := range views.MapOf(tree, nil).Nodes {
		got[n.ID] = n
	}
	requireHoldView(t, got["F-2"].AmendmentHold)
	require.Nil(t, got["F-3"].AmendmentHold)
}

func TestStatusOracleOffersNoLifecycleVerbOnAHeldNode(t *testing.T) {
	tree := heldFeatureTree()
	held := views.NodeStatus(tree, tree.Features["F-2"], workflow.Input{})
	requireHoldView(t, held.AmendmentHold)
	require.Empty(t, held.LegalNext, "a held draft takes no lifecycle step")
	require.NotNil(t, held.LegalNext, "legal_next stays an array, never null")

	free := views.NodeStatus(tree, tree.Features["F-3"], workflow.Input{})
	require.Nil(t, free.AmendmentHold)
	require.NotEmpty(t, free.LegalNext)
}
