package views

import (
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
)

// MapNode is one card on the Explore map (docs/plans/2026-08-30-explore-map-
// design.md, "The graph"): the node's identity and state, its container
// (the id the card nests inside; empty for an epic or a shelf node), and
// the count of open questions naming it, which it renders as a badge.
type MapNode struct {
	Noun         string              `json:"noun"`
	ID           string              `json:"id"`
	Key          string              `json:"key,omitempty"`
	Title        string              `json:"title,omitempty"`
	Status       string              `json:"status"`
	Owner        string              `json:"owner,omitempty"`
	Contributors []store.Contributor `json:"contributors,omitzero"`
	Parent       string              `json:"parent,omitempty"` // a feature's epic id, a task's feature id; empty for an epic or a shelf node
	Holds        int                 `json:"holds,omitempty"`  // open questions naming this node itself, not the holds it inherits
}

// MapEdge is one recorded relationship. From is always the node whose record
// carries the field, To its target: "from depends on to", "from was found in
// to", "from is carried by to".
type MapEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // depends_on | attached_to | resolved_by
}

// MapView is GET /api/map. Nodes ride in gen.DependencyOrder order, which the
// client keeps as the sibling order inside a column; the edges are every
// depends_on, attached_to, and resolved_by whose target resolves in the tree.
type MapView struct {
	Nodes         []MapNode         `json:"nodes"`
	Edges         []MapEdge         `json:"edges"`
	Relationships RelationshipGraph `json:"relationships"`
}

const (
	MapEdgeDependsOn  = "depends_on"
	MapEdgeAttachedTo = "attached_to"
	MapEdgeResolvedBy = "resolved_by"
)

// MapOf projects the working tree onto the map payload, excluding cancelled
// epic subtrees from containment while preserving the full relationship graph.
func MapOf(t *store.Tree, index *relationships.Index) MapView {
	if index == nil {
		index = relationships.BuildStructural(t)
	}
	order := gen.DependencyOrder(t)
	nodes := make([]MapNode, 0, len(t.Epics)+len(t.Features)+len(t.Tasks)+len(t.Bugs)+len(t.Chores)+len(t.Tweaks))
	for _, epicID := range order.Epics() {
		e := t.Epics[epicID]
		if e == nil || e.Status == "cancelled" {
			continue
		}
		nodes = append(nodes, MapNode{
			Noun: store.NounEpic, ID: e.ID, Key: Key(e), Title: e.Title, Status: e.Status, Contributors: Contributors(t, e),
			Holds: gen.DirectQuestionHolds(t, e),
		})
		for _, featureID := range order.Features(Key(e)) {
			f := t.Features[featureID]
			if f == nil {
				continue
			}
			nodes = append(nodes, MapNode{
				Noun: store.NounFeature, ID: f.ID, Key: Key(f), Title: f.Title, Status: f.Status, Contributors: Contributors(t, f),
				Parent: e.ID, Holds: gen.DirectQuestionHolds(t, f),
			})
			for _, tk := range order.Tasks(f.ID) {
				nodes = append(nodes, MapNode{
					Noun: store.NounTask, ID: tk.ID, Key: Key(tk), Title: tk.Title, Status: tk.Status, Owner: tk.Owner,
					Parent: f.ID, Holds: gen.DirectQuestionHolds(t, tk),
				})
			}
		}
	}
	for _, w := range mapShelf(t) {
		b := w.Body()
		nodes = append(nodes, MapNode{
			Noun: w.Noun(), ID: b.ID, Key: Key(w), Title: b.Title, Status: b.Status, Owner: b.Owner,
		})
	}
	return MapView{Nodes: nodes, Edges: mapEdges(index, nodes), Relationships: RelationshipGraphOf(index)}
}

// Layout affinity uses only its original three relationship kinds. The full
// semantic graph is carried separately and cannot change display parenting.
func mapEdges(index *relationships.Index, nodes []MapNode) []MapEdge {
	edges := make([]MapEdge, 0)
	present := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		present[node.ID] = true
	}
	for _, node := range nodes {
		for _, kind := range []string{MapEdgeDependsOn, MapEdgeAttachedTo, MapEdgeResolvedBy} {
			for _, edge := range index.Adjacent(node.ID, kind) {
				if present[edge.To] {
					edges = append(edges, MapEdge{From: edge.From, To: edge.To, Kind: edge.Kind})
				}
			}
		}
	}
	return edges
}

// mapShelf keeps the two shelf branches distinct while applying each shelf's
// canonical dependency, recorded soft-order, and key sequence.
func mapShelf(t *store.Tree) []store.WorkNode {
	out := make([]store.WorkNode, 0, len(t.Bugs)+len(t.Chores)+len(t.Tweaks))
	for _, noun := range store.ShelfNouns() {
		out = append(out, gen.OrderWorkNodes(t, t.WorkNodesOf(noun), t.ShelfOrderOf(noun))...)
	}
	return out
}
