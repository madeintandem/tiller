package mutate

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

func transferReplaceEdge(edges []string, oldID, newID string, aliases ...string) []string {
	out := make([]string, 0, len(edges))
	seen := make(map[string]bool, len(edges))
	for _, edge := range edges {
		if edge == oldID || slices.Contains(aliases, edge) {
			edge = newID
		}
		if !seen[edge] {
			out = append(out, edge)
			seen[edge] = true
		}
	}
	return out
}

func (p *transferPlan) relationships(original, replacement store.Node, res *TransferResult) error {
	oldID, newID := original.NodeID(), replacement.NodeID()
	oldKey := deref(original.NodeKey())
	for _, tid := range sortedMapKeys(p.tree.Tasks) {
		task := p.tree.Tasks[tid]
		if (!slices.Contains(task.DependsOn, oldID) && !slices.Contains(task.DependsOn, oldKey)) || transferTerminal(task.Status) {
			continue
		}
		if task.Started || reviewGrainTaskInFlight(task.Status) {
			return transferBlocked("transfer_started_dependent", "task %s (%s) has already started and depends on %s; finish it or seek a human-reviewed structural change before transferring", deref(task.Key), task.ID, oldID)
		}
		before := append([]string(nil), task.DependsOn...)
		task.DependsOn = transferReplaceEdge(task.DependsOn, oldID, newID, oldKey)
		res.Relationships = append(res.Relationships, TransferRelationship{Noun: store.NounTask, ID: task.ID, Relation: "depends_on", Before: before, After: task.DependsOn})
		parent := p.tree.Features[task.FeatureID]
		if err := p.put(filepath.Join(store.TaskRelDir(parent.EpicKey, localKey(deref(parent.Key)), localKey(deref(task.Key))), store.TaskStateFile), task); err != nil {
			return err
		}
	}
	for _, noun := range store.ShelfNouns() {
		for _, work := range p.tree.WorkNodesOf(noun) {
			w := work.Body()
			if transferTerminal(w.Status) {
				continue
			}
			dependent := slices.Contains(w.DependsOn, oldID) || slices.Contains(w.DependsOn, oldKey)
			carried := w.ResolvedBy == oldID
			if !dependent && !carried {
				continue
			}
			if w.Started || reviewGrainTaskInFlight(w.Status) {
				return transferBlocked("transfer_started_dependent", "%s %s (%s) has already started and depends on or is carried by %s; finish it or seek a human-reviewed structural change before transferring", noun, deref(w.Key), w.ID, oldID)
			}
			if dependent {
				before := append([]string(nil), w.DependsOn...)
				w.DependsOn = transferReplaceEdge(w.DependsOn, oldID, newID, oldKey)
				res.Relationships = append(res.Relationships, TransferRelationship{Noun: noun, ID: w.ID, Relation: "depends_on", Before: before, After: w.DependsOn})
			}
			if carried {
				if replacement.Noun() != store.NounTask {
					return transferBlocked("transfer_carrier_unsafe", "%s %s has unsupported shelf carrier %s; a human must review tiller resolve before transfer", noun, deref(w.Key), oldID)
				}
				w.ResolvedBy = newID
				res.Relationships = append(res.Relationships, TransferRelationship{Noun: noun, ID: w.ID, Relation: "resolved_by", Before: []string{oldID}, After: []string{newID}})
			}
			if err := p.put(filepath.Join(workItemRelDir(work), store.ShelfStateFile(noun)), work); err != nil {
				return err
			}
		}
	}
	questionsChanged := false
	for _, q := range p.questions.Questions {
		if !slices.Contains(q.Blocks, oldID) && !slices.Contains(q.Blocks, oldKey) {
			continue
		}
		before := append([]string(nil), q.Blocks...)
		if !slices.Contains(q.Blocks, newID) {
			q.Blocks = append(q.Blocks, newID)
		}
		res.Relationships = append(res.Relationships, TransferRelationship{Noun: store.NounQuestion, ID: q.ID, Relation: "blocks", Before: before, After: q.Blocks})
		questionsChanged = true
	}
	if questionsChanged {
		return p.put(filepath.FromSlash(questionsRelPath), &p.questions)
	}
	return nil
}

func (p *transferPlan) shelfOrder(noun, oldID, newID string, res *TransferResult) error {
	rel := store.ShelfOrderRelPath(noun)
	raw, err := p.read(rel)
	if err != nil {
		return err
	}
	var order store.ShelfOrder
	if len(raw) != 0 {
		if err := yaml.Unmarshal(raw, &order); err != nil {
			return err
		}
	}
	if !equalStringLists(order.Order, p.tree.ShelfOrderOf(noun)) {
		return removalStale(rel)
	}
	before := append([]string(nil), order.Order...)
	// Materialize the effective order, including previously unlisted nodes:
	// a successor's new key must not move the work among otherwise tied peers.
	ordered := gen.OrderWorkNodes(p.tree, p.tree.WorkNodesOf(noun), order.Order)
	order.Order = make([]string, 0, len(ordered))
	for _, node := range ordered {
		order.Order = append(order.Order, node.NodeID())
	}
	order.Order = transferReplaceEdge(order.Order, oldID, newID)
	res.Relationships = append(res.Relationships, TransferRelationship{Noun: noun, ID: fmt.Sprintf("%s/order", store.ShelfDir(noun)), Relation: "order", Before: before, After: order.Order})
	return p.put(rel, &order)
}
