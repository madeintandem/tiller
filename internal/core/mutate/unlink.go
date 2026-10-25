package mutate

import (
	"fmt"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
)

const (
	eventUnlinkEpic    = "unlink.epic"
	eventUnlinkFeature = "unlink.feature"
	eventUnlinkTask    = "unlink.task"
)

// UnlinkInput is a request to remove dependency edges addressed by id or key,
// recorded surface paths, a shelf node's attachment, or a combination in one
// planned batch.
type UnlinkInput struct {
	Noun      string
	Node      string
	DependsOn []string
	// Detach clears a bug's, chore's, or tweak's attached_to.
	Detach bool
	// Surface paths to remove from an epic's or a feature's surface.paths,
	// matched literally after cleaning, so a path the work moved or deleted
	// can still be named.
	Surface []string
}

// UnlinkResult reports the node's complete surviving dependency list and the
// exact stored ids this call removed, both in their prior stable order, plus
// the attachment this call cleared.
type UnlinkResult struct {
	Noun              string
	ID                string
	Key               string
	DependsOn         []string
	Removed           []string
	DetachedFrom      string
	AttachmentChanged bool
	// Surface is the node's surviving surface list and SurfaceRemoved the
	// recorded paths this call removed, both in stored order.
	Surface        []string
	SurfaceRemoved []string
	SurfaceChanged bool
	Changed        bool
}

// Unlink subtracts resolved targets from depends_on and, on request, clears
// the attachment. Every target resolves before the node file is read, and a
// refusal on either axis writes nothing. An absent edge or attachment is an
// idempotent no-op success, and an unchanged dependency list never reaches
// the shared post-start dependency guard.
func Unlink(tree *store.Tree, deps Deps, in UnlinkInput) (UnlinkResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return UnlinkResult{}, err
	}
	if in.Detach {
		if err := requireAttachmentNoun(in.Noun); err != nil {
			return UnlinkResult{}, err
		}
	}
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Node})
	if err != nil {
		return UnlinkResult{}, err
	}
	var surface []string
	if len(in.Surface) > 0 {
		if err := validateSurfaceTarget(node, "unlink"); err != nil {
			return UnlinkResult{}, err
		}
		if surface, err = cleanSurfacePaths(in.Surface); err != nil {
			return UnlinkResult{}, err
		}
	}
	targets, err := resolveDependsOn(tree, in.DependsOn, node.NodeID(), siblingScopeOf(node))
	if err != nil {
		return UnlinkResult{}, err
	}
	if len(targets) == 0 && !in.Detach && len(surface) == 0 {
		return UnlinkResult{}, fmt.Errorf("mutate: unlink needs at least one --depends-on target, --surface path, or --detach")
	}

	remove := make(map[string]bool, len(targets))
	for _, id := range targets {
		remove[id] = true
	}
	change := linkChange{surfaceRemove: surface, dependsOn: func(before []string) (after, removed []string) {
		after = make([]string, 0, len(before))
		for _, id := range before {
			if remove[id] {
				removed = append(removed, id)
				continue
			}
			after = append(after, id)
		}
		return after, removed
	}}
	if in.Detach {
		cleared := ""
		change.attachment = &cleared
	}
	edit, err := editNodeLinks(tree, node, change)
	if err != nil {
		return UnlinkResult{}, err
	}
	res := UnlinkResult{
		Noun: node.Noun(), ID: node.NodeID(), Key: deref(node.NodeKey()),
		DependsOn: edit.after, Removed: edit.removed,
		AttachmentChanged: edit.attachmentChanged,
		Surface:           edit.surface, SurfaceRemoved: edit.surfaceRemoved,
		SurfaceChanged: edit.surfaceChanged,
		Changed:        edit.changed || edit.attachmentChanged || edit.surfaceChanged,
	}
	if edit.attachmentChanged {
		res.DetachedFrom = edit.attachedBefore
	}
	if !res.Changed {
		return res, nil
	}
	if edit.surfaceChanged {
		if err := guardSurfaceRemoval(node, edit); err != nil {
			return UnlinkResult{}, err
		}
	}
	var guards []store.PlannedWrite
	var evs []events.Event
	if edit.changed {
		guards, err = guardDependencyChangeOpen(tree, node, nil)
		if err != nil {
			return UnlinkResult{}, err
		}
		verb := eventUnlinkEpic
		switch node.Noun() {
		case store.NounFeature:
			verb = eventUnlinkFeature
		case store.NounTask:
			verb = eventUnlinkTask
		default:
			if store.IsWorkNoun(node.Noun()) {
				verb = workItemEventType("unlink", node.Noun())
			}
		}
		ev := events.NewEvent(deps.clock(), verb, owner.ID, "unlink "+node.Noun()+" "+node.NodeID(), node.NodeID())
		ev.ID = deps.mintRaw("EV")
		ev.Before = map[string]any{"depends_on": edit.before}
		ev.After = map[string]any{"depends_on": edit.after, "removed": edit.removed}
		ev.Files = []events.FileChange{{Path: edit.relYml}}
		evs = append(evs, ev)
	}
	if edit.surfaceChanged {
		evs = append(evs, unlinkSurfaceEvent(deps, owner, node, edit.relYml, edit.surfaceBefore, edit.surface, edit.surfaceRemoved))
	}
	if edit.attachmentChanged {
		command := fmt.Sprintf("unlink %s %s --detach", node.Noun(), deref(node.NodeKey()))
		evs = append(evs, attachmentEvent(deps, owner, node, "unlink", command, edit.relYml, edit.attachedBefore, ""))
	}

	writes := append(guards, store.PlanWrite(filepath.Join(tree.Root, edit.relYml), edit.snapshot, edit.data))
	if err := commitMutation(tree, "unlink "+node.Noun(), writes, evs); err != nil {
		return UnlinkResult{}, err
	}

	return res, nil
}
