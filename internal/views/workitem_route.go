package views

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// routableShelfNodes lists the owner's shelf work still to do, a batch
// counting as one unit and its members not at all: checked work first, then
// the rest, each batches first and then in bug-before-chore shelf order.
// Checked work anywhere on the shelves still needs its handoff first.
func routableShelfNodes(t *store.Tree, owner string) []store.WorkNode {
	if owner == "" {
		return nil
	}
	var checked, rest []store.WorkNode
	for _, work := range shelfExecutionUnits(t) {
		body := work.Body()
		if body.Owner != owner || (body.Status != "check" && body.Status != "ready" && body.Status != "in_progress") {
			continue
		}
		// A ready batch with an unapproved member cannot start: the move is
		// the human's triage of that member, which the triage rows carry.
		if work.Noun() == store.NounBatch && body.Status == "ready" && !batchStartable(t, work) {
			continue
		}
		if body.Status == "check" {
			checked = append(checked, work)
		} else {
			rest = append(rest, work)
		}
	}
	return append(checked, rest...)
}

// shelfWorkGuidance is what the owner still owes their shelf work before its
// next step, "" when nothing is owed.
func shelfWorkGuidance(work store.WorkNode) string {
	if work.Body().Status == "check" {
		return "finish any outstanding checks, then publish and submit it"
	}
	return ""
}

// shelfMergeWaits lists the shelf work sitting in review on its own review,
// waiting on a human merge: batches, then shelf order.
func shelfMergeWaits(t *store.Tree) []store.WorkNode {
	var out []store.WorkNode
	for _, work := range shelfExecutionUnits(t) {
		if work.Body().Status == "in_review" {
			out = append(out, work)
		}
	}
	return out
}

// shelfRecoveredWaits lists the shelf work holding a recovered review that
// sync has yet to cross, in shelf order.
func shelfRecoveredWaits(t *store.Tree) []store.WorkNode {
	var out []store.WorkNode
	for _, noun := range store.ShelfNouns() {
		for _, work := range orderedShelfNodes(t, noun) {
			if gen.WorkActivity(t, work) == gen.ActivityRecovered {
				out = append(out, work)
			}
		}
	}
	return out
}

func ReadyShelfQueueLine(t *store.Tree) string {
	var parts []string
	for _, shelf := range store.Shelves() {
		ready := 0
		for _, work := range t.WorkNodesOf(shelf.Noun) {
			if work.Body().Status == "ready" && !gen.WorkLeftQueue(t, work) {
				ready++
			}
		}
		parts = append(parts, fmt.Sprintf("%d %s", ready, plural(ready, shelf.Noun, shelf.Dir)))
	}
	// Batches join the line only when one is ready to start, every member
	// approved, so a project that never batches reads exactly as it did.
	ready := 0
	for _, batch := range t.Batches {
		if batch.Status == "ready" && batchStartable(t, batch) {
			ready++
		}
	}
	if ready > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", ready, plural(ready, store.NounBatch, store.ShelfDir(store.NounBatch))))
	}
	return "queue: " + strings.Join(parts, ", ") + " ready"
}
