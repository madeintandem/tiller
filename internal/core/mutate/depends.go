package mutate

// This file is the dependency write path: depends_on was modeled
// on epics and features (artifact-schema.md), but nothing could record it,
// so decomposition buried the graph
// in epic prose. Two writers fix that: --depends-on at add time (threaded
// through AddEpic, AddFeature, and AddTask via their inputs) and Link for the
// order that becomes clear later. Both go through resolveDependsOn, which
// scopes what a target may be to the grain recording it: an epic or a feature
// names another epic or feature, and a task names its own siblings
// (artifact-schema.md, Tasks: Ordering). Recording is display-shaped above task
// grain, deliberately: whether start should refuse an unmet
// dependency is a separate decision the recorded graph makes possible. At task
// grain the recorded edges are also the rendered sibling order, which
// gen.OrderTasks ranks.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

const (
	eventLinkEpic    = "link.epic"
	eventLinkFeature = "link.feature"
	eventLinkTask    = "link.task"
)

// LinkInput records dependencies, existing-code surfaces, and a shelf node's
// attachment in one planned batch.
type LinkInput struct {
	Noun string
	Node string
	// DependsOn are the targets, addressed by id or key; they resolve
	// all-or-nothing and are stored as ids.
	DependsOn []string
	Surface   []string
	// Attach is the epic, feature, or task a bug, chore, or tweak was found
	// in (id or key); empty leaves the attachment alone.
	Attach string
}

// LinkResult reports the node's resulting fields and whether each requested
// axis changed.
type LinkResult struct {
	Noun                string
	ID                  string
	Key                 string
	DependsOn           []string
	Surface             []string
	AttachedTo          string
	AttachedBefore      string
	DependenciesChanged bool
	SurfaceChanged      bool
	AttachmentChanged   bool
	Changed             bool
}

// ReasonDependencyChangeLocked is the stable blocked-answer discriminator for
// dependency edits attempted after work has begun.
const ReasonDependencyChangeLocked = "dependency_change_locked"

// DependencyChangeLockedError reports that changing a real dependency edge is
// no longer a direct correction. No-op retries never reach this error.
type DependencyChangeLockedError struct {
	Noun   string
	ID     string
	Key    string
	Status string
	Reason string
}

func (e *DependencyChangeLockedError) Error() string {
	ident := e.ID
	if e.Key != "" {
		ident = e.Key
	}
	return fmt.Sprintf("mutate: %s %s is %s; dependency changes are locked once work starts (return ready %s work before it starts; a started task may only add an edge to a done task)",
		e.Noun, ident, e.Status, e.Noun)
}

// Link validates every requested axis before committing one node snapshot
// and a distinct event for each changed axis. Repeats are no-change successes.
// A refusal on any axis writes nothing.
func Link(tree *store.Tree, deps Deps, in LinkInput) (LinkResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return LinkResult{}, err
	}
	if in.Attach != "" {
		if err := requireAttachmentNoun(in.Noun); err != nil {
			return LinkResult{}, err
		}
	}
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Node})
	if err != nil {
		return LinkResult{}, err
	}
	return linkNode(tree, deps, owner, node, in.DependsOn, in.Surface, in.Attach)
}

func linkNode(tree *store.Tree, deps Deps, owner store.Contributor, node store.Node, dependsOn, paths []string, attach string) (LinkResult, error) {
	if len(dependsOn) == 0 && len(paths) == 0 && attach == "" {
		return LinkResult{}, fmt.Errorf("mutate: link needs at least one --depends-on target, --surface path, or --attach target")
	}
	var clean []string
	var err error
	if len(paths) > 0 {
		clean, err = validateSurfacePaths(tree, node, paths)
		if err != nil {
			return LinkResult{}, err
		}
	}
	targets, err := resolveDependsOn(tree, dependsOn, node.NodeID(), siblingScopeOf(node))
	if err != nil {
		return LinkResult{}, err
	}
	var attachment *string
	if attach != "" {
		target, err := resolveAttachRequest(tree, attach)
		if err != nil {
			return LinkResult{}, err
		}
		attachment = &target
	}
	edit, err := editNodeLinks(tree, node, linkChange{
		surface:    clean,
		attachment: attachment,
		dependsOn: func(before []string) (after, removed []string) {
			return unionLinkValues(before, targets), nil
		},
	})
	if err != nil {
		return LinkResult{}, err
	}
	res := LinkResult{
		Noun: node.Noun(), ID: node.NodeID(), Key: deref(node.NodeKey()),
		DependsOn: edit.after, Surface: edit.surface,
		AttachedTo: edit.attachedAfter, AttachedBefore: edit.attachedBefore,
		DependenciesChanged: edit.changed, SurfaceChanged: edit.surfaceChanged,
		AttachmentChanged: edit.attachmentChanged,
		Changed:           edit.changed || edit.surfaceChanged || edit.attachmentChanged,
	}
	if !res.Changed {
		return res, nil
	}
	var guards []store.PlannedWrite
	var evs []events.Event
	if edit.changed {
		guards, err = guardDependencyChangeOpen(tree, node, addedDependencies(edit.before, edit.after))
		if err != nil {
			return LinkResult{}, err
		}
		verb := eventLinkEpic
		switch node.Noun() {
		case store.NounFeature:
			verb = eventLinkFeature
		case store.NounTask:
			verb = eventLinkTask
		default:
			if store.IsWorkNoun(node.Noun()) {
				verb = workItemEventType("link", node.Noun())
			}
		}
		ev := events.NewEvent(deps.clock(), verb, owner.ID, "link "+node.Noun()+" "+node.NodeID(), node.NodeID())
		ev.ID = deps.mintRaw("EV")
		ev.Before = map[string]any{"depends_on": edit.before}
		ev.After = map[string]any{"depends_on": edit.after, "added": addedDependencies(edit.before, edit.after)}
		ev.Files = []events.FileChange{{Path: edit.relYml}}
		evs = append(evs, ev)
	}
	if edit.surfaceChanged {
		evs = append(evs, surfaceEvent(deps, owner, node, edit.relYml, edit.surface))
	}
	if edit.attachmentChanged {
		command := fmt.Sprintf("link %s %s --attach %s", node.Noun(), deref(node.NodeKey()), attach)
		evs = append(evs, attachmentEvent(deps, owner, node, "link", command, edit.relYml, edit.attachedBefore, edit.attachedAfter))
	}
	writes := append(guards, store.PlanWrite(filepath.Join(tree.Root, edit.relYml), edit.snapshot, edit.data))
	if err := commitMutation(tree, "link "+node.Noun(), writes, evs); err != nil {
		return LinkResult{}, err
	}

	return res, nil
}

func unionLinkValues(before, additions []string) []string {
	if len(additions) == 0 {
		return before
	}
	after := append([]string(nil), before...)
	have := make(map[string]bool, len(before)+len(additions))
	for _, value := range before {
		have[value] = true
	}
	for _, value := range additions {
		if !have[value] {
			after = append(after, value)
			have[value] = true
		}
	}
	return after
}

// addedDependencies is the set difference after minus before, in after's
// order: the edges one Link call laid, for its event.
func addedDependencies(before, after []string) []string {
	had := make(map[string]bool, len(before))
	for _, id := range before {
		had[id] = true
	}
	added := []string{}
	for _, id := range after {
		if !had[id] {
			added = append(added, id)
		}
	}
	return added
}

// linkChange is every structural edit one Link or Unlink call plans against a
// node's state file.
type linkChange struct {
	// dependsOn maps the stored depends_on to its new value and the ids it
	// removed.
	dependsOn func(before []string) (after, removed []string)
	// surface paths to union into surface.paths (epics and features only).
	surface []string
	// surfaceRemove paths to subtract from surface.paths (epics and features
	// only), matched literally.
	surfaceRemove []string
	// attachment, when non-nil, is the attached_to value to record; the empty
	// string clears it (shelf nodes only).
	attachment *string
}

type dependsOnEdit struct {
	relYml         string
	snapshot       store.FileSnapshot
	data           []byte
	before         []string
	after          []string
	removed        []string
	changed        bool
	surface        []string
	surfaceBefore  []string
	surfaceRemoved []string
	surfaceChanged bool
	// status and changeType are the fresh node's, for the surface guards.
	status            string
	changeType        string
	attachedBefore    string
	attachedAfter     string
	attachmentChanged bool
}

// editNodeLinks reads and edits the target once, so dependency, surface, and
// attachment changes cannot clobber one another and commit as one write.
func editNodeLinks(tree *store.Tree, node store.Node, change linkChange) (dependsOnEdit, error) {
	var edit dependsOnEdit
	var current *[]string
	var surface *[]string
	var attachedTo *string
	var marshal func() ([]byte, error)

	switch n := node.(type) {
	case *store.Epic:
		edit.relYml = filepath.Join(".tiller", "epics", deref(n.Key), "epic.yml")
		var doc store.Epic
		snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, edit.relYml), &doc)
		if err != nil {
			return dependsOnEdit{}, err
		}
		edit.snapshot = snapshot
		if err := validateFreshNodeIdentity(store.NounEpic, n.ID, edit.relYml, doc.ID, deref(doc.Key), deref(n.Key), ""); err != nil {
			return dependsOnEdit{}, err
		}
		current = &doc.DependsOn
		surface = &doc.Surface.Paths
		edit.status = doc.Status
		marshal = func() ([]byte, error) { return yaml.Marshal(doc) }
	case *store.Feature:
		if n.Key == nil {
			return dependsOnEdit{}, fmt.Errorf("mutate: feature %s has no key in its feature.yml; restore the key line before changing dependencies", n.ID)
		}
		edit.relYml = filepath.Join(".tiller", "epics", n.EpicKey, "features", localKey(*n.Key), "feature.yml")
		var doc store.Feature
		snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, edit.relYml), &doc)
		if err != nil {
			return dependsOnEdit{}, err
		}
		doc.EpicKey = n.EpicKey
		if err := validateFreshNodeIdentity(store.NounFeature, n.ID, edit.relYml, doc.ID, deref(doc.Key), deref(n.Key), n.EpicKey); err != nil {
			return dependsOnEdit{}, err
		}
		edit.snapshot = snapshot
		current = &doc.DependsOn
		surface = &doc.Surface.Paths
		edit.status = doc.Status
		edit.changeType = doc.ChangeType
		marshal = func() ([]byte, error) { return yaml.Marshal(doc) }
	case *store.Task:
		feat, ok := tree.Features[n.FeatureID]
		if !ok || feat.Key == nil {
			return dependsOnEdit{}, fmt.Errorf("mutate: task %s names no addressable parent feature, so its folder cannot be found", n.ID)
		}
		doc, relYml, snapshot, err := loadTaskFile(tree, feat, n)
		if err != nil {
			return dependsOnEdit{}, err
		}
		edit.relYml = relYml
		edit.snapshot = snapshot
		current = &doc.DependsOn
		marshal = func() ([]byte, error) { return yaml.Marshal(doc) }
	default:
		w, ok := store.AsWorkNode(node)
		if !ok {
			carriers := append([]string{"epics", "features", "tasks"}, shelfDirs()...)
			return dependsOnEdit{}, fmt.Errorf("mutate: only %s carry depends_on, not a %s", englishList("and", carriers...), node.Noun())
		}
		doc, relYml, snapshot, err := loadWorkItemFile(tree, w)
		if err != nil {
			return dependsOnEdit{}, err
		}
		edit.relYml = relYml
		edit.snapshot = snapshot
		current = &doc.Body().DependsOn
		attachedTo = &doc.Body().AttachedTo
		marshal = func() ([]byte, error) { return yaml.Marshal(doc) }
	}

	edit.before = append([]string(nil), (*current)...)
	edit.after, edit.removed = change.dependsOn(edit.before)
	edit.changed = !equalStringLists(edit.before, edit.after)
	*current = edit.after
	if surface != nil {
		edit.surfaceBefore = append([]string(nil), (*surface)...)
		if len(change.surface) > 0 {
			after := unionLinkValues(*surface, change.surface)
			edit.surfaceChanged = !equalStringLists(*surface, after)
			*surface = after
		}
		if len(change.surfaceRemove) > 0 {
			drop := make(map[string]bool, len(change.surfaceRemove))
			for _, path := range change.surfaceRemove {
				drop[path] = true
			}
			after := make([]string, 0, len(*surface))
			for _, path := range *surface {
				if drop[path] {
					edit.surfaceRemoved = append(edit.surfaceRemoved, path)
					continue
				}
				after = append(after, path)
			}
			edit.surfaceChanged = edit.surfaceChanged || len(edit.surfaceRemoved) > 0
			*surface = after
		}
		edit.surface = *surface
	} else if len(change.surfaceRemove) > 0 {
		return dependsOnEdit{}, validateSurfaceTarget(node, "unlink")
	}
	if attachedTo != nil {
		edit.attachedBefore = *attachedTo
		edit.attachedAfter = *attachedTo
		if change.attachment != nil {
			edit.attachedAfter = *change.attachment
			edit.attachmentChanged = edit.attachedBefore != edit.attachedAfter
			*attachedTo = edit.attachedAfter
		}
	} else if change.attachment != nil {
		return dependsOnEdit{}, requireAttachmentNoun(node.Noun())
	}
	data, err := marshal()
	if err != nil {
		return dependsOnEdit{}, err
	}
	edit.data = data
	return edit, nil
}

// allDoneTasks reports whether every id names a task whose state file, read
// fresh from disk, says done. Cancelled is not done.
func allDoneTasks(tree *store.Tree, ids []string) bool {
	for _, id := range ids {
		tk := tree.Tasks[id]
		if tk == nil {
			return false
		}
		feat := tree.Features[tk.FeatureID]
		if feat == nil {
			return false
		}
		doc, _, _, err := loadTaskFile(tree, feat, tk)
		if err != nil || doc.Status != "done" {
			return false
		}
	}
	return true
}

func equalStringLists(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// guardDependencyChangeOpen enforces the one direct-correction boundary shared
// by changed Link and Unlink calls. It re-reads typed state from disk. Epic
// checks additionally pin the fresh descendant inventory and bytes into the
// eventual transaction so work cannot start between the check and the write.
// added holds the resolved target ids a Link is adding (Unlink passes nil). A
// started task may add an edge, never remove one, when every added target is a
// task whose fresh status is exactly done.
func guardDependencyChangeOpen(tree *store.Tree, node store.Node, added []string) ([]store.PlannedWrite, error) {
	locked := func(n store.Node) ([]store.PlannedWrite, error) {
		return nil, &DependencyChangeLockedError{
			Noun: n.Noun(), ID: n.NodeID(), Key: deref(n.NodeKey()),
			Status: n.NodeStatus(), Reason: ReasonDependencyChangeLocked,
		}
	}

	switch n := node.(type) {
	case *store.Task:
		feat := tree.Features[n.FeatureID]
		if feat == nil {
			return nil, fmt.Errorf("mutate: task %s names no parent feature", n.ID)
		}
		doc, _, _, err := loadTaskFile(tree, feat, n)
		if err != nil {
			return nil, err
		}
		switch doc.Status {
		case "proposed", "ready":
			return nil, nil
		default:
			if len(added) > 0 && allDoneTasks(tree, added) {
				return nil, nil
			}
			return locked(doc)
		}
	case *store.Feature:
		if n.Key == nil {
			return nil, fmt.Errorf("mutate: feature %s has no key in its feature.yml; restore the key line before changing dependencies", n.ID)
		}
		path := filepath.Join(tree.Root, ".tiller", "epics", n.EpicKey, "features", localKey(*n.Key), "feature.yml")
		var doc store.Feature
		if _, err := readYAMLSnapshot(tree.Root, path, &doc); err != nil {
			return nil, err
		}
		doc.EpicKey = n.EpicKey
		if doc.Branch != "" {
			return locked(&doc)
		}
		switch doc.Status {
		case "draft", "specced", "ready":
			return nil, nil
		default:
			return locked(&doc)
		}
	case *store.Epic:
		if n.Key == nil {
			return nil, fmt.Errorf("mutate: epic %s has no key in its epic.yml; restore the key line before changing dependencies", n.ID)
		}
		path := filepath.Join(tree.Root, ".tiller", "epics", *n.Key, "epic.yml")
		var doc store.Epic
		if _, err := readYAMLSnapshot(tree.Root, path, &doc); err != nil {
			return nil, err
		}
		if doc.Key == nil {
			return nil, fmt.Errorf("mutate: epic %s has no key in its epic.yml; restore the key line before changing dependencies", doc.ID)
		}
		if doc.Branch != "" {
			return locked(&doc)
		}
		switch doc.Status {
		case "draft", "ready":
		default:
			return locked(&doc)
		}

		fresh, guards, err := freshReviewGrainLineage(tree, nil)
		if err != nil {
			return nil, err
		}
		startedTasks, historyGuards, err := startedTaskHistory(tree)
		if err != nil {
			return nil, err
		}
		guards = append(guards, historyGuards...)
		for _, feature := range fresh.Features {
			if feature.EpicKey != *doc.Key {
				continue
			}
			if feature.Branch != "" {
				return locked(&doc)
			}
			switch feature.Status {
			case "draft", "specced", "ready", "cancelled":
			default:
				return locked(&doc)
			}
			for _, task := range fresh.FeatureTasks[feature.ID] {
				switch task.Status {
				case "proposed", "ready":
				case "cancelled":
					if startedTasks[task.ID] {
						return locked(&doc)
					}
				default:
					return locked(&doc)
				}
			}
		}
		return guards, nil
	default:
		w, ok := store.AsWorkNode(node)
		if !ok {
			return nil, fmt.Errorf("mutate: %s carries no depends_on", node.Noun())
		}
		doc, _, _, err := loadWorkItemFile(tree, w)
		if err != nil {
			return nil, err
		}
		switch doc.Body().Status {
		case "proposed", "ready":
			return nil, nil
		default:
			return locked(doc)
		}
	}
}

func startedTaskHistory(tree *store.Tree) (map[string]bool, []store.PlannedWrite, error) {
	history, err := events.List(tree)
	if err != nil {
		return nil, nil, err
	}
	started := make(map[string]bool)
	var guards []store.PlannedWrite
	for _, event := range history {
		if event.Type != "start.task" {
			continue
		}
		path, _, err := events.File(tree, event)
		if err != nil {
			return nil, nil, err
		}
		_, snapshot, err := store.ReadGuardedFileSnapshot(tree.Root, path)
		if err != nil {
			return nil, nil, err
		}
		guards = append(guards, store.PlanReadGuard(path, snapshot))
		for _, target := range event.Targets {
			started[target] = true
		}
	}
	return started, guards, nil
}

// dependsScope names where a node's depends_on targets are allowed to live.
// Sibling reports whether the recording node is a task, which is the fact that
// picks the arm; Feature is the parent whose tasks its siblings are, and is
// empty only for a task the tree could not name a parent for.
//
// The two are separate fields because "this node is not a task" and "this
// task's parent has no id" are different facts, and one string cannot hold
// both: an unparented task collapsed onto the empty scope would take the
// epic-and-feature arm, where a bare sibling key resolves as an EPIC key and a
// three-segment key resolves as nothing, so the refusal a caller gets would be
// about the wrong class of node. Sibling stays true for it, and every target
// then refuses, which is the honest answer for a node with no siblings anyone
// can name.
type dependsScope struct {
	Sibling bool
	Feature string
	// Shelf marks a bug's or chore's depends_on, which names bugs and chores
	// only, never a tree node (the design's Queue, routing, and
	// dependencies).
	Shelf bool
}

// siblingScopeOf reads a recording node's scope: a task is sibling-scoped to
// its own parent feature, a bug or chore is shelf-scoped to the other shelf
// nodes, and every other noun records edges across the tree.
func siblingScopeOf(node store.Node) dependsScope {
	if tk, ok := node.(*store.Task); ok {
		return dependsScope{Sibling: true, Feature: tk.FeatureID}
	}
	if _, ok := store.AsWorkNode(node); ok {
		return dependsScope{Shelf: true}
	}
	return dependsScope{}
}

// taskAddressForm reports whether raw is shaped like a task address at all: an
// id, or the three-segment canonical key a task carries (cli-surface.md, "A
// node is addressed by id or by key"; artifact-schema.md, Keys and paths). A
// non-task id prefix passes, because the resolver's own noun-mismatch refusal
// names that mistake better than a form complaint would.
//
// The near-miss this exists for is the bare local segment. Naming a sibling
// "schema" reads as unambiguous to a human looking at one feature's tasks, but
// it is not an address, and resolving it would report that no task called
// "schema" exists, about a sibling that plainly does.
func taskAddressForm(raw string) bool {
	return hasIDPrefix(raw, "E-", "F-", "T-", "Q-", "A-", "ADR-", "EV-", "AT-") ||
		strings.Count(raw, "/") == 2
}

// resolveDependsOn resolves a depends_on list to stored ids. Which targets are
// admissible depends on the grain that is recording the edge, so the caller
// names the scope:
//
//   - a scope that is neither Sibling nor Shelf is an epic's or a feature's
//     own depends_on, which reaches anywhere in the tree. A dependency names
//     an epic or a feature: an id names its class by prefix, a key with a
//     slash is a feature's canonical form, a bare key is an epic's.
//   - a Sibling scope is a task's depends_on, which names sibling tasks under
//     scope.Feature and nothing else (artifact-schema.md, Tasks). A target
//     resolving to a task under another feature is a resolution failure, not a
//     dropped edge: the cross-feature task edge is not part of this model, and
//     gen.OrderTasks would silently ignore one if it were ever written.
//   - a Shelf scope is a bug's or a chore's depends_on, which names bugs and
//     chores only, never a tree node (the design's Queue, routing, and
//     dependencies): an id names its shelf by prefix (B-, CH-), a shelf key
//     (bugs/<key>, chores/<key>) names it directly.
//
// selfID is the node recording the edges, refused as its own target. Anything
// that does not fit the scope refuses plainly, and every entry must resolve so
// one typo cannot record a dangling edge.
func resolveDependsOn(tree *store.Tree, raws []string, selfID string, scope dependsScope) ([]string, error) {
	if len(raws) == 0 {
		return nil, nil
	}
	r := resolve.NewResolver(tree)
	ids := make([]string, 0, len(raws))
	seen := make(map[string]bool, len(raws))
	for _, raw := range raws {
		var n store.Node
		var err error
		switch {
		case scope.Shelf:
			shelf, ok := store.ShelfOfRef(raw)
			if !ok {
				return nil, notShelfWork(raw)
			}
			n, err = r.Resolve(resolve.Ref{Noun: shelf.Noun, Raw: raw})
		case scope.Sibling:
			if !taskAddressForm(raw) {
				return nil, fmt.Errorf("mutate: depends_on target %q is not a task address; name a sibling task by its id (T-xxxxxx) or by its full key (<epic-key>/<feature-key>/<task-key>)", raw)
			}
			n, err = r.Resolve(resolve.Ref{Noun: store.NounTask, Raw: raw})
		case strings.HasPrefix(raw, "E-"):
			n, err = r.Resolve(resolve.Ref{Noun: store.NounEpic, Raw: raw})
		case strings.HasPrefix(raw, "F-"):
			n, err = r.Resolve(resolve.Ref{Noun: store.NounFeature, Raw: raw})
		case hasIDPrefix(raw, "T-", "Q-", "A-", "ADR-", "EV-", "AT-"), isShelfRef(raw):
			return nil, fmt.Errorf("mutate: depends_on target %q is not a dependency; a dependency names an epic or a feature", raw)
		case strings.Contains(raw, "/"):
			n, err = r.Resolve(resolve.Ref{Noun: store.NounFeature, Raw: raw})
		default:
			n, err = r.Resolve(resolve.Ref{Noun: store.NounEpic, Raw: raw})
		}
		if err != nil {
			return nil, err
		}
		if n.NodeID() == selfID {
			return nil, fmt.Errorf("mutate: %s cannot depend on itself", selfID)
		}
		if scope.Sibling {
			tk, ok := n.(*store.Task)
			if !ok || scope.Feature == "" || tk.FeatureID != scope.Feature {
				return nil, fmt.Errorf("mutate: depends_on target %q is not a sibling task; a task depends on the tasks under its own feature", raw)
			}
		}
		if scope.Shelf {
			if _, ok := n.(store.WorkNode); !ok {
				return nil, fmt.Errorf("mutate: depends_on target %q is not shelf work", raw)
			}
		}
		// The same node named twice (by key and by id, or a repeated flag)
		// records one edge; the graph is a set.
		if !seen[n.NodeID()] {
			seen[n.NodeID()] = true
			ids = append(ids, n.NodeID())
		}
	}
	return ids, nil
}

// notShelfWork refuses a shelf node's depends_on target that is not written
// as shelf work, naming every shelf's id and key form.
func notShelfWork(raw string) error {
	var ids, keys []string
	for _, s := range store.Shelves() {
		ids = append(ids, s.IDPrefix+"-xxxxxx")
		keys = append(keys, s.Dir+"/<key>")
	}
	return fmt.Errorf("mutate: depends_on target %q is not shelf work; shelf work depends only on %s, named by id (%s) or shelf key (%s)",
		raw, englishList("and", shelfDirs()...), strings.Join(ids, ", "), strings.Join(keys, ", "))
}

// hasIDPrefix reports whether raw starts with any of the given id prefixes.
func hasIDPrefix(raw string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(raw, p) {
			return true
		}
	}
	return false
}
