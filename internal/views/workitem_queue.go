package views

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

func orderedShelfNodes(t *store.Tree, noun string) []store.WorkNode {
	return gen.OrderWorkNodes(t, t.WorkNodesOf(noun), t.ShelfOrderOf(noun))
}

func queueApproveShelf(t *store.Tree, noun string) []QueueEntry {
	kind := "approve_" + noun
	var out []QueueEntry
	for _, work := range orderedShelfNodes(t, noun) {
		body := work.Body()
		if body.Status != "proposed" || gen.WorkActivity(t, work) == gen.ActivityCarried {
			continue
		}
		out = append(out, QueueEntry{
			Kind: kind, Noun: noun, ID: body.ID, Key: Key(work),
			Ref: gen.Ref(t, work), Prose: body.Title,
			Command: fmt.Sprintf("tiller approve %s %s", noun, queueAddr(work)),
		})
	}
	return out
}

func queueCarrierCancelled(t *store.Tree) []QueueEntry {
	var out []QueueEntry
	for _, noun := range store.ShelfNouns() {
		for _, work := range orderedShelfNodes(t, noun) {
			if gen.WorkActivity(t, work) != gen.ActivityCarrierCancelled {
				continue
			}
			body := work.Body()
			out = append(out, QueueEntry{
				Kind: "carrier_cancelled", Noun: noun, ID: body.ID, Key: Key(work),
				Ref: gen.Ref(t, work), Prose: body.Title,
				Command: fmt.Sprintf("tiller resolve %s %s --clear", noun, queueAddr(work)),
			})
		}
	}
	return out
}
