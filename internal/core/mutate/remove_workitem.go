package mutate

import (
	"path/filepath"
	"reflect"

	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// planRemovalShelfReferences scrubs every surviving shelf node: removed ids
// leave depends_on, and attached_to, resolved_by, and duplicate_of clear to
// absent when they named a removed node, since unapproved proposal material
// never shipped and nothing was found in it (docs/spec/cli-surface.md). It
// returns the ids whose provenance fields were cleared so the caller can say
// so.
func planRemovalShelfReferences(tree *store.Tree, removed map[string]struct{}, changed map[string]struct{}) ([]store.PlannedWrite, []string, error) {
	var writes []store.PlannedWrite
	var detached []string
	for _, noun := range store.ShelfNouns() {
		for _, w := range tree.WorkNodesOf(noun) {
			if _, gone := removed[w.NodeID()]; gone {
				continue
			}
			path := filepath.Join(tree.Root, workItemRelDir(w), store.ShelfStateFile(noun))
			disk, err := store.NewWorkNode(noun)
			if err != nil {
				return nil, nil, err
			}
			snapshot, err := readRemovalYAML(tree.Root, path, disk)
			if err != nil {
				return nil, nil, err
			}
			if !reflect.DeepEqual(*disk.Body(), *w.Body()) {
				return nil, nil, removalStale(path)
			}
			b := disk.Body()
			b.DependsOn, _ = withoutRemoved(b.DependsOn, removed)
			cleared := false
			for _, field := range []*string{&b.AttachedTo, &b.ResolvedBy, &b.DuplicateOf} {
				if _, gone := removed[*field]; gone && *field != "" {
					*field = ""
					cleared = true
				}
			}
			if cleared {
				detached = append(detached, w.NodeID())
			}
			write, err := removalStateWrite(tree.Root, path, snapshot, *w.Body(), *b, changed)
			if err != nil {
				return nil, nil, err
			}
			writes = append(writes, write)
		}
	}
	return writes, detached, nil
}

// planRemovalShelfOrder scrubs one shelf's order.yml, the twin of
// planRemovalProjectOrder for the root list.
func planRemovalShelfOrder(tree *store.Tree, noun string, removed map[string]struct{}, changed map[string]struct{}) (store.PlannedWrite, error) {
	path := filepath.Join(tree.Root, store.ShelfOrderRelPath(noun))
	data, snapshot, err := store.ReadGuardedFileSnapshot(tree.Root, path)
	if err != nil {
		return store.PlannedWrite{}, err
	}
	recorded := tree.ShelfOrderOf(noun)
	if !snapshot.Exists() {
		if len(recorded) != 0 {
			return store.PlannedWrite{}, removalStale(path)
		}
		return store.PlanReadGuard(path, snapshot), nil
	}
	var disk store.ShelfOrder
	if err := yaml.Unmarshal(data, &disk); err != nil {
		return store.PlannedWrite{}, err
	}
	if !reflect.DeepEqual(disk.Order, recorded) {
		return store.PlannedWrite{}, removalStale(path)
	}
	var changedAny bool
	disk.Order, changedAny = withoutRemoved(disk.Order, removed)
	if !changedAny {
		return store.PlanReadGuard(path, snapshot), nil
	}
	out, err := yaml.Marshal(disk)
	if err != nil {
		return store.PlannedWrite{}, err
	}
	changed[relativeSlash(tree.Root, path)] = struct{}{}
	return store.PlanWrite(path, snapshot, out), nil
}
