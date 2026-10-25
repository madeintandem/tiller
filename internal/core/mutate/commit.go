package mutate

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

func commitMutation(tree *store.Tree, operation string, writes []store.PlannedWrite, evs []events.Event) error {
	appended, err := eventWrites(tree, evs)
	if err != nil {
		return err
	}
	planned := append([]store.PlannedWrite(nil), writes...)
	return tree.Commit(operation, append(planned, appended...))
}

// eventWrites resolves each event to the planned write that appends it, so a
// mutation whose write ORDER matters can place the event files itself rather
// than taking commitMutation's append-at-the-end. Only the amendment's approve
// needs that: its whole recovery contract rests on the amendment's own status
// landing after everything else in the transaction, events included
// (amendment_approve.go's doc comment).
func eventWrites(tree *store.Tree, evs []events.Event) ([]store.PlannedWrite, error) {
	planned := make([]store.PlannedWrite, 0, len(evs))
	for _, ev := range evs {
		path, data, err := events.File(tree, ev)
		if err != nil {
			return nil, err
		}
		planned = append(planned, store.PlanWrite(path, store.AbsentSnapshot(), data))
	}
	return planned, nil
}

func readYAMLSnapshot(root, path string, out any) (store.FileSnapshot, error) {
	data, snapshot, err := store.ReadRegularFileSnapshot(root, path)
	if err != nil {
		return store.FileSnapshot{}, err
	}
	if !snapshot.Exists() {
		return store.FileSnapshot{}, fs.ErrNotExist
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return store.FileSnapshot{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return snapshot, nil
}

// commitTransition marshals the node's write and its event through
// commitNodeWrites as one act. reviewGrainBefore is the node's review grain
// immediately before this call overwrote it, so the event's Before map can
// name what was replaced; pass "" where the caller never touches
// review_grain, since transitionEvent's own guard on in.ReviewGrain leaves it
// unread in that case.
func commitTransition(tree *store.Tree, deps Deps, owner store.Contributor, in TransitionInput,
	noun, targetID, ymlRel string, ydata []byte, snapshot store.FileSnapshot, before, after, reviewGrainBefore string) error {
	return commitTransitionWithGuards(tree, deps, owner, in, noun, targetID, ymlRel, ydata,
		snapshot, before, after, reviewGrainBefore, nil)
}

func commitTransitionWithGuards(tree *store.Tree, deps Deps, owner store.Contributor, in TransitionInput,
	noun, targetID, ymlRel string, ydata []byte, snapshot store.FileSnapshot, before, after, reviewGrainBefore string,
	guards []store.PlannedWrite) error {
	ev := transitionEvent(deps, owner, in, noun, targetID, ymlRel, before, after, reviewGrainBefore)
	return commitNodeWritesWithGuards(tree, in.Verb+"."+noun,
		[]nodeWrite{{rel: ymlRel, data: ydata, before: snapshot, event: ev}}, guards)
}

type nodeWrite struct {
	rel    string
	data   []byte
	before store.FileSnapshot
	event  events.Event
}

func commitNodeWrites(tree *store.Tree, operation string, writes []nodeWrite) error {
	return commitNodeWritesWithGuards(tree, operation, writes, nil)
}

func commitNodeWritesWithGuards(tree *store.Tree, operation string, writes []nodeWrite, guards []store.PlannedWrite) error {
	written := make(map[string]struct{}, len(writes))
	for _, write := range writes {
		written[filepath.Join(tree.Root, write.rel)] = struct{}{}
	}
	planned := make([]store.PlannedWrite, 0, len(guards)+len(writes))
	for _, guard := range guards {
		if _, replaced := written[guard.Path]; !replaced {
			planned = append(planned, guard)
		}
	}
	evs := make([]events.Event, 0, len(writes))
	for _, write := range writes {
		planned = append(planned, store.PlanWrite(filepath.Join(tree.Root, write.rel), write.before, write.data))
		evs = append(evs, write.event)
	}
	return commitMutation(tree, operation, planned, evs)
}
