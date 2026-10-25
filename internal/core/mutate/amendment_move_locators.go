package mutate

import (
	"path/filepath"
	"sort"

	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

func moveIdentityLocators(tree *store.Tree, feature *store.Feature) map[string]string {
	refs := map[string]string{deref(feature.Key): feature.ID}
	for _, task := range tree.FeatureTasks[feature.ID] {
		refs[deref(task.Key)] = task.ID
	}
	return refs
}

// These are current identity locators, not historical revisions or authored
// provenance. Legacy key-valued pointers become stable IDs before paths move.
func remapMoveNode(node store.Node, refs map[string]string, apply bool) bool {
	changed := false
	field := func(value *string) {
		if id := refs[*value]; id != "" {
			changed = true
			if apply {
				*value = id
			}
		}
	}
	list := func(values []string) {
		for i := range values {
			field(&values[i])
		}
	}
	switch n := node.(type) {
	case *store.Epic:
		list(n.DependsOn)
		list(n.Order)
	case *store.Feature:
		list(n.DependsOn)
		list(n.Supersedes)
		if n.SupersededBy != nil {
			field(n.SupersededBy)
		}
	case *store.Task:
		list(n.DependsOn)
		list(n.Supersedes)
		if n.SupersededBy != nil {
			field(n.SupersededBy)
		}
	case store.WorkNode:
		w := n.Body()
		list(w.DependsOn)
		list(w.Supersedes)
		if w.SupersededBy != nil {
			field(w.SupersededBy)
		}
		field(&w.AttachedTo)
		field(&w.ResolvedBy)
	}
	return changed
}

func moveNodes(tree *store.Tree) []store.Node {
	nodes := make([]store.Node, 0, len(tree.Epics)+len(tree.Features)+len(tree.Tasks)+len(tree.Bugs)+len(tree.Chores)+len(tree.Tweaks))
	for _, n := range tree.Epics {
		nodes = append(nodes, n)
	}
	for _, n := range tree.Features {
		nodes = append(nodes, n)
	}
	for _, n := range tree.Tasks {
		nodes = append(nodes, n)
	}
	for _, n := range tree.Bugs {
		nodes = append(nodes, n)
	}
	for _, n := range tree.Chores {
		nodes = append(nodes, n)
	}
	for _, n := range tree.Tweaks {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeID() < nodes[j].NodeID() })
	return nodes
}

func (b *batch) remapMoveLocators(tree *store.Tree, doc *store.Amendment) error {
	refs := map[string]string{}
	moved := map[string]bool{}
	for _, op := range doc.Operations {
		if op.Op != store.AmendmentOpMoveFeature {
			continue
		}
		f := tree.Features[op.Target]
		for key, id := range moveIdentityLocators(tree, f) {
			refs[key] = id
			moved[id] = true
		}
	}
	if len(refs) == 0 {
		return nil
	}
	for _, node := range moveNodes(tree) {
		if moved[node.NodeID()] || !remapMoveNode(node, refs, false) {
			continue
		}
		rel, ok := relationships.NodePath(tree, node, "yml")
		if !ok {
			return moveRefusal("current locator has no state path: " + node.NodeID())
		}
		edit := b.edits[rel]
		if edit == nil {
			var fresh store.Node
			switch node.(type) {
			case *store.Epic:
				fresh = &store.Epic{}
			case *store.Feature:
				fresh = &store.Feature{}
			case *store.Task:
				fresh = &store.Task{}
			case *store.Bug:
				fresh = &store.Bug{}
			case *store.Chore:
				fresh = &store.Chore{}
			case *store.Tweak:
				fresh = &store.Tweak{}
			}
			data, before, err := store.ReadRegularFileSnapshot(tree.Root, filepath.Join(tree.Root, rel))
			if err != nil {
				return err
			}
			if err := yaml.Unmarshal(data, fresh); err != nil {
				return err
			}
			if fresh.NodeID() != node.NodeID() {
				return moveRefusal("current locator owner changed")
			}
			edit = b.register(&nodeEdit{noun: node.Noun(), rel: rel, doc: fresh, before: before})
		}
		if remapMoveNode(edit.doc, refs, true) {
			b.dirty[rel] = true
		}
	}
	return nil
}
