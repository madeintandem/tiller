package views

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// routableShelfNodes lists the owner's uncarried shelf work still to do,
// checked work first, then the rest, each in bug-before-chore shelf order:
// checked work anywhere on the shelves still needs its handoff first.
func routableShelfNodes(t *store.Tree, owner string) []store.WorkNode {
	if owner == "" {
		return nil
	}
	var checked, rest []store.WorkNode
	for _, noun := range store.ShelfNouns() {
		for _, work := range orderedShelfNodes(t, noun) {
			body := work.Body()
			if body.Owner != owner || (body.Status != "check" && body.Status != "ready" && body.Status != "in_progress") {
				continue
			}
			if gen.WorkLeftQueue(t, work) {
				continue
			}
			if body.Status == "check" {
				checked = append(checked, work)
			} else {
				rest = append(rest, work)
			}
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

// shelfMergeWaits lists the uncarried shelf work sitting in review, waiting
// on a human merge, in shelf order.
func shelfMergeWaits(t *store.Tree) []store.WorkNode {
	var out []store.WorkNode
	for _, noun := range store.ShelfNouns() {
		for _, work := range orderedShelfNodes(t, noun) {
			if work.Body().Status == "in_review" && !gen.WorkLeftQueue(t, work) {
				out = append(out, work)
			}
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
	return "queue: " + strings.Join(parts, ", ") + " ready"
}
