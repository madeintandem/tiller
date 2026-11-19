package gen

import (
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
)

// ActivityCarried is the derived display phase of a shelf node whose
// resolved_by names a live carrier: it has left the queue and sync will
// cross it when the carrier lands (docs/spec/cli-surface.md).
// Derived on read, never stored, like FeatureActivity's phases.
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

// ActivityRecovered is the derived display phase of an unfinished shelf node
// holding a recovered review: its fix already merged, so it has left the queue
// and the next sync crosses it on the record.
const ActivityRecovered = "recovered"

// ActivityBatched is the derived display phase of a shelf node a batch is
// reviewing: the batch owns its branch, gates, and pull request, and sync
// crosses it when the batch lands. Its stored status stays where triage left
// it.
const ActivityBatched = "batched"

// WorkActivity is a shelf node's derived display phase, recomputed on every
// read against the carrier's current fate:
//
//   - ActivityBatched while the node names a batch that is not cancelled;
//   - ActivityCarried while resolved_by names a live epic, feature, or task;
//   - ActivityRecovered while the node holds a recovered review and no live
//     carrier, the order sync itself reads them in;
//   - ActivityCarrierCancelled when that carrier is cancelled, archived, or
//     absent from the tree;
//   - "" when the node carries neither, or is itself terminal, where a
//     display phase would say nothing the status does not.
//
// A done carrier still reads carried: it is the crossing sync is about to
// record, not a carrier that failed.
func WorkActivity(t *store.Tree, w store.WorkNode) string {
	b := w.Body()
	if b.Status == "done" || b.Status == "cancelled" {
		return ""
	}
	if Batched(t, w) {
		return ActivityBatched
	}
	if b.ResolvedBy != "" && carrierLive(t, b.ResolvedBy) {
		return ActivityCarried
	}
	if b.RecoveredReview != nil {
		return ActivityRecovered
	}
	if b.ResolvedBy == "" {
		return ""
	}
	return ActivityCarrierCancelled
}

// WorkLeftQueue reports a shelf node that no longer waits on its own work:
// reviewed by a batch, carried by live work, or holding a recovered review.
// Sync crosses it each way, so it is nobody's to start, continue, or wait on a
// merge for.
func WorkLeftQueue(t *store.Tree, w store.WorkNode) bool {
	activity := WorkActivity(t, w)
	return activity == ActivityBatched || activity == ActivityCarried || activity == ActivityRecovered
}

// Batched reports an unfinished shelf node whose batch can still cross it: the
// batch exists and is not cancelled. A done batch still counts, because that
// is the crossing sync is about to record. A batch itself is never batched.
func Batched(t *store.Tree, w store.WorkNode) bool {
	if t == nil || w.Noun() == store.NounBatch {
		return false
	}
	b := w.Body()
	if b.Status == "done" || b.Status == "cancelled" {
		return false
	}
	batch := t.Batches[b.BatchID]
	return batch != nil && batch.Status != "cancelled"
}

// BatchSeverity is the highest severity among a batch's members, "" when none
// is graded. A batch stores no severity of its own.
func BatchSeverity(t *store.Tree, batch *store.Batch) string {
	highest := ""
	for _, member := range t.BatchMembers(batch.ID) {
		severity := member.Body().Severity
		if severity != "" && (highest == "" || store.SeverityRank(severity) < store.SeverityRank(highest)) {
			highest = severity
		}
	}
	return highest
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
