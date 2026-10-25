package mutate

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// applyShelfOrder is the shelf grain of SetOrder: a read-modify-write of
// .tiller/<shelf>/order.yml, whole-list replace or clear, one
// order.<shelf> event (docs/plans/2026-08-28-bugs-and-chores-design.md,
// Queue, routing, and dependencies). The shelf has no parent to name.
func applyShelfOrder(tree *store.Tree, deps Deps, owner store.Contributor, noun string, ids []string) (SetOrderResult, error) {
	relYml := store.ShelfOrderRelPath(noun)
	var doc store.ShelfOrder
	before, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, relYml), &doc)
	if errors.Is(err, fs.ErrNotExist) {
		before, err = store.AbsentSnapshot(), nil
	}
	if err != nil {
		return SetOrderResult{}, err
	}
	changed := !slices.Equal(doc.Order, ids)
	res := SetOrderResult{Noun: noun, Order: ids, Changed: changed}
	if !changed {
		res.Order = doc.Order
		return res, nil
	}
	prior := doc.Order
	doc.Order = ids
	data, err := yaml.Marshal(doc)
	if err != nil {
		return SetOrderResult{}, err
	}
	ev := events.NewEvent(deps.clock(), shelfOrderEvent(noun), owner.ID, orderCommand(noun, ids), ids...)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"order": prior}
	ev.After = map[string]any{"order": ids}
	ev.Files = []events.FileChange{{Path: relYml}}
	if err := commitMutation(tree, "order "+noun, []store.PlannedWrite{
		store.PlanWrite(filepath.Join(tree.Root, relYml), before, data),
	}, []events.Event{ev}); err != nil {
		return SetOrderResult{}, err
	}

	return res, nil
}
