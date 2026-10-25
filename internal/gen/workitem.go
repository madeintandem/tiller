package gen

import (
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
)

// ActivityCarried is the derived display phase of a shelf node whose
// resolved_by names a live carrier: it has left the queue and sync will
// cross it when the carrier lands (docs/plans/2026-08-28-bugs-and-chores-
// design.md, Resolution). Derived on read, never stored, like
// FeatureActivity's phases.
const ActivityCarried = "carried"

// ActivityCarrierCancelled is the same node once its carrier can no longer
// carry it: cancelled, archived, or gone from the tree. The design says
// carried "recomputes on the carrier's fate the way a rollup done
// recomputes", so the phase is recomputed here rather than frozen at the
// moment resolve ran: the node is back in the queue, waiting for its own
// work or for a human to re-point resolved_by, and this phase is what says
// so out loud. The pointer itself stays on the record, because clearing or
// re-pointing it is a judgment (`tiller resolve --clear` or `--by`), not
// something a read derives.
const ActivityCarrierCancelled = "carrier_cancelled"

// WorkActivity is a shelf node's derived display phase, recomputed on every
// read against the carrier's current fate:
//
//   - ActivityCarried while resolved_by names a live epic, feature, or task;
//   - ActivityCarrierCancelled when that carrier is cancelled, archived, or
//     absent from the tree;
//   - "" when the node carries no resolved_by at all, or is itself terminal,
//     where a display phase would say nothing the status does not.
//
// A done carrier still reads carried: it is the crossing sync is about to
// record, not a carrier that failed.
func WorkActivity(t *store.Tree, w store.WorkNode) string {
	b := w.Body()
	if b.ResolvedBy == "" || b.Status == "done" || b.Status == "cancelled" {
		return ""
	}
	if carrierLive(t, b.ResolvedBy) {
		return ActivityCarried
	}
	return ActivityCarrierCancelled
}

// carrierLive reports whether id names an epic, feature, or task the tree
// still holds in a state whose merge can come. Only the three tree grains
// answer: resolve accepts nothing else as a carrier, so a resolved_by naming
// anything else is as unresolvable as one naming nothing.
func carrierLive(t *store.Tree, id string) bool {
	if t == nil {
		return false
	}
	status := ""
	switch {
	case t.Epics[id] != nil:
		status = t.Epics[id].Status
	case t.Features[id] != nil:
		status = t.Features[id].Status
	case t.Tasks[id] != nil:
		status = t.Tasks[id].Status
	default:
		return false
	}
	return status != "cancelled" && status != "archived"
}

// OrderWorkNodes ranks one shelf's nodes the way OrderTasks ranks siblings:
// recorded depends_on edges first, then the shelf's soft order, then
// canonical key ascending (the design's Queue, routing, and dependencies).
func OrderWorkNodes(tree *store.Tree, nodes []store.WorkNode, soft []string) []store.WorkNode {
	dependencyNodes := make([]store.Node, len(nodes))
	for n, node := range nodes {
		dependencyNodes[n] = node
	}
	index := relationships.BuildDependencies(tree, dependencyNodes)
	byID := make(map[string]store.WorkNode, len(nodes))
	ids := make([]string, 0, len(nodes))
	deps := make(map[string][]string, len(nodes))
	for _, w := range nodes {
		if _, known := byID[w.NodeID()]; known {
			continue
		}
		ids = append(ids, w.NodeID())
		byID[w.NodeID()] = w
		deps[w.NodeID()] = dependencyTargets(index, w.NodeID())
	}
	keyOf := func(id string) string { return keyStr(byID[id].NodeKey()) }
	tie := softTie(soft, keyTie(keyOf))
	ranked, _ := layeredTopoOrderTied(ids, deps, keyOf, tie, tie)
	out := make([]store.WorkNode, 0, len(ranked))
	for _, id := range ranked {
		out = append(out, byID[id])
	}
	return out
}
