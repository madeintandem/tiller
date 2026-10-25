package gen

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func bug(id, key, status string, deps ...string) *store.Bug {
	k := "bugs/" + key
	return &store.Bug{WorkItem: store.WorkItem{ID: id, Key: &k, Status: status, DependsOn: deps}}
}

func TestOrderWorkNodesDependencyThenSoftThenKey(t *testing.T) {
	a := bug("B-aaaaaa", "alpha", "ready", "B-cccccc")
	b := bug("B-bbbbbb", "bravo", "ready")
	c := bug("B-cccccc", "charlie", "ready")
	tree := &store.Tree{Bugs: map[string]*store.Bug{a.ID: a, b.ID: b, c.ID: c}}
	got := OrderWorkNodes(tree, []store.WorkNode{a, b, c}, []string{"B-bbbbbb"})
	ids := []string{got[0].NodeID(), got[1].NodeID(), got[2].NodeID()}
	// charlie must precede alpha (edge); bravo leads the free layer (soft order).
	require.Equal(t, []string{"B-bbbbbb", "B-cccccc", "B-aaaaaa"}, ids)
}

func TestOrderWorkNodesNormalizesLegacyShelfKeys(t *testing.T) {
	alpha := bug("B-alpha", "alpha", "ready", "bugs/zulu")
	zulu := bug("B-zulu", "zulu", "ready")
	tree := &store.Tree{Bugs: map[string]*store.Bug{alpha.ID: alpha, zulu.ID: zulu}}
	legacy := OrderWorkNodes(tree, []store.WorkNode{alpha, zulu}, []string{"B-alpha"})
	require.Equal(t, []store.WorkNode{zulu, alpha}, legacy)
	alpha.DependsOn = []string{"B-zulu"}
	require.Equal(t, legacy, OrderWorkNodes(tree, []store.WorkNode{alpha, zulu}, []string{"B-alpha"}))
}

// carrierTree is a tree holding one task, the carrier a shelf node's
// resolved_by names, at the given status.
func carrierTree(status string) *store.Tree {
	k := "form-engine/loading/wire"
	return &store.Tree{Tasks: map[string]*store.Task{
		"T-k3p9w6": {ID: "T-k3p9w6", Key: &k, Status: status},
	}}
}

func TestWorkActivityCarried(t *testing.T) {
	w := bug("B-aaaaaa", "alpha", "ready")
	tree := carrierTree("in_progress")
	require.Equal(t, "", WorkActivity(tree, w))
	w.ResolvedBy = "T-k3p9w6"
	require.Equal(t, ActivityCarried, WorkActivity(tree, w))
	w.Status = "done"
	require.Equal(t, "", WorkActivity(tree, w))
}

// TestWorkActivityRecomputesOnTheCarriersFate pins the third outcome: carried
// is derived, not stored, so a carrier that is cancelled, archived, or gone
// from the tree returns the node to the queue under its own phase rather than
// leaving it displayed carried forever.
func TestWorkActivityRecomputesOnTheCarriersFate(t *testing.T) {
	w := bug("B-aaaaaa", "alpha", "ready")
	w.ResolvedBy = "T-k3p9w6"

	require.Equal(t, ActivityCarrierCancelled, WorkActivity(carrierTree("cancelled"), w))
	require.Equal(t, ActivityCarrierCancelled, WorkActivity(carrierTree("archived"), w))
	require.Equal(t, ActivityCarrierCancelled, WorkActivity(&store.Tree{}, w),
		"an id the tree no longer holds cannot carry anything either")
	require.Equal(t, ActivityCarried, WorkActivity(carrierTree("done"), w),
		"a done carrier is still the carrier until sync crosses the node")
}

func TestProsePathForShelfNodes(t *testing.T) {
	tree := &store.Tree{}
	p, ok := ProsePath(tree, bug("B-aaaaaa", "alpha", "ready"))
	require.True(t, ok)
	require.Equal(t, ".tiller/bugs/alpha/bug.md", p)
	ck := "chores/bump"
	p, ok = ProsePath(tree, &store.Chore{WorkItem: store.WorkItem{ID: "CH-aaaaaa", Key: &ck}})
	require.True(t, ok)
	require.Equal(t, ".tiller/chores/bump/chore.md", p)
}

func TestTweakProsePath(t *testing.T) {
	key := "tweaks/button-spacing"
	node := &store.Tweak{WorkItem: store.WorkItem{ID: "TW-aaaaaa", Key: &key}}
	p, ok := ProsePath(nil, node)
	require.True(t, ok)
	require.Equal(t, ".tiller/tweaks/button-spacing/tweak.md", p)
}
