package mutate

// This file owns soft-order writes (docs/spec/cli-surface.md):
// SetOrder replaces explicit intent, while amendment approval may only append
// epics it successfully creates.
//
// A soft order records how siblings should render and route wherever the
// dependency graph leaves the choice open: the human or the agent states the
// intended sequence, and gen's sort authority prefers it wherever depends_on
// does not already decide. It never refuses a legal action and never blocks
// work taken out of sequence; every refusal SetOrder raises below is about
// the request's shape, an unresolvable or duplicate reference, a mixed
// parent, a noun outside the ordered grains, an empty list with no
// --clear, and never about a node's lifecycle state or its place in someone
// else's order.
//
// Storage is machine-owned at three tree grains plus two shelf grains: the
// epic sequence lives at the project root (.tiller/order.yml, absent until
// first written), a feature sequence lives on its owning epic's order field
// (epic.yml), a task sequence lives on its owning feature's order field
// (feature.yml), and a bug or chore sequence lives on its own shelf's
// order.yml (.tiller/bugs/order.yml, .tiller/chores/order.yml), which names
// no parent, the same as epic grain. A rerun replaces the parent's recorded
// list wholesale, because the list is a statement of current intent; its
// history lives in git and, at every grain but epic, in the event this write
// appends beside it. Epic grain records no event: v1 has no project-grain
// event stream, and order.yml under git is the record.

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// Event type discriminators for the tree grains that carry one. Epic grain
// writes no event (this file's own doc comment); a shelf writes
// shelfOrderEvent's order.<shelf dir>.
const (
	eventOrderFeatures = "order.features"
	eventOrderTasks    = "order.tasks"
)

// SetOrderInput is a request to record, or clear, the soft order at one
// parent: the project root (epic grain), one epic's features, or one
// feature's tasks.
type SetOrderInput struct {
	Noun  string   // noun of the listed siblings: epic, feature, or task
	Nodes []string // refs, key or id; all-or-nothing; empty only with Clear
	Clear bool
	// Parent scopes a --clear, which names no siblings to infer from:
	// ParentEpic for feature grain, ParentFeature for task grain, both
	// resolvable refs. Refused, not ignored, alongside a non-empty Nodes
	// list (SetOrder's own guard): a parent flag has nothing left to scope
	// once nodes are given.
	ParentEpic    string
	ParentFeature string
}

// SetOrderResult is the outcome: the grain, the parent node id that now
// carries the list (empty at epic grain, the root), the list after the
// write, and whether anything changed.
type SetOrderResult struct {
	Noun    string
	Parent  string
	Order   []string
	Changed bool
}

// SetOrder replaces the recorded soft order at one parent, or clears it. Every
// ref resolves all-or-nothing before anything is written; echoing what was
// typed back to the human is the CLI's job, not this one's. A rerun with the
// identical list is a no-change success, same as Link's own no-change
// contract for depends_on.
func SetOrder(tree *store.Tree, deps Deps, in SetOrderInput) (SetOrderResult, error) {
	if !(in.Noun == store.NounEpic || in.Noun == store.NounFeature || in.Noun == store.NounTask || store.IsWorkNoun(in.Noun)) {
		ranked := append([]string{"epics", "features", "tasks"}, shelfDirs()...)
		return SetOrderResult{}, fmt.Errorf("mutate: order ranks %s; a %s keeps id order", englishList("and", ranked...), in.Noun)
	}
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return SetOrderResult{}, err
	}

	if len(in.Nodes) == 0 {
		if !in.Clear {
			return SetOrderResult{}, fmt.Errorf("mutate: order needs at least one node, or --clear")
		}
		return setOrderClear(tree, deps, owner, in)
	}
	// Clear and a parent flag each name their own request, with no siblings
	// of their own to record: Clear erases a parent's list outright, and a
	// parent flag only ever scopes a Clear. Neither combines with a
	// non-empty Nodes list, so both guards sit here, ahead of resolution,
	// rather than resolving a list while a conflicting flag rides along
	// unexamined (whole-branch review, finding 1).
	if in.Clear {
		return SetOrderResult{}, fmt.Errorf("mutate: order takes a node list or --clear, never both; %s", legalOrderForms(in.Noun))
	}
	if in.ParentEpic != "" || in.ParentFeature != "" {
		return SetOrderResult{}, fmt.Errorf("mutate: --epic and --feature scope --clear alone and take no node list beside them; %s", legalOrderForms(in.Noun))
	}

	nodes, err := resolveOrderNodes(tree, in.Noun, in.Nodes)
	if err != nil {
		return SetOrderResult{}, err
	}
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.NodeID()
	}

	if store.IsWorkNoun(in.Noun) {
		return applyShelfOrder(tree, deps, owner, in.Noun, ids)
	}
	switch in.Noun {
	case store.NounEpic:
		return applyEpicOrder(tree, ids)
	case store.NounFeature:
		epic, err := sharedFeatureParent(tree, nodes)
		if err != nil {
			return SetOrderResult{}, err
		}
		return applyFeatureOrder(tree, deps, owner, epic, ids)
	default: // store.NounTask: the only noun left after the guard above
		feat, err := sharedTaskParent(tree, nodes)
		if err != nil {
			return SetOrderResult{}, err
		}
		return applyTaskOrder(tree, deps, owner, feat, ids)
	}
}

// setOrderClear resolves --clear's named parent (none needed at epic grain,
// the root has no parent to name) and replaces its list with nil.
func setOrderClear(tree *store.Tree, deps Deps, owner store.Contributor, in SetOrderInput) (SetOrderResult, error) {
	if store.IsWorkNoun(in.Noun) {
		// A shelf, like the root, has no parent to name: a parent flag beside
		// it has nothing to scope and refuses the same as one beside a Nodes
		// list above, rather than being silently ignored the way epic grain
		// below never examines it.
		if in.ParentEpic != "" || in.ParentFeature != "" {
			return SetOrderResult{}, fmt.Errorf("mutate: --epic and --feature scope --clear alone and take no node list beside them; %s", legalOrderForms(in.Noun))
		}
		return applyShelfOrder(tree, deps, owner, in.Noun, nil)
	}
	switch in.Noun {
	case store.NounEpic:
		return applyEpicOrder(tree, nil)

	case store.NounFeature:
		if in.ParentEpic == "" {
			return SetOrderResult{}, fmt.Errorf("mutate: order %s --clear needs --%s to name whose list to clear", store.NounFeature, store.NounEpic)
		}
		n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounEpic, Raw: in.ParentEpic})
		if err != nil {
			return SetOrderResult{}, err
		}
		return applyFeatureOrder(tree, deps, owner, n.(*store.Epic), nil)

	default: // store.NounTask
		if in.ParentFeature == "" {
			return SetOrderResult{}, fmt.Errorf("mutate: order %s --clear needs --%s to name whose list to clear", store.NounTask, store.NounFeature)
		}
		n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounFeature, Raw: in.ParentFeature})
		if err != nil {
			return SetOrderResult{}, err
		}
		feat := n.(*store.Feature)
		if feat.Key == nil {
			return SetOrderResult{}, fmt.Errorf("mutate: feature %s has no key in its feature.yml; restore the key line before ordering", feat.ID)
		}
		return applyTaskOrder(tree, deps, owner, feat, nil)
	}
}

// legalOrderForms names order's two mutually exclusive shapes for noun, the
// clause every "not both" refusal above points at: record a sequence, or
// erase the parent's recorded list.
func legalOrderForms(noun string) string {
	return fmt.Sprintf("`order %s <node> <node> ...` records, `order %s --clear` erases", noun, noun)
}

// resolveOrderNodes resolves every ref against the tree, all-or-nothing, and
// refuses a node named twice. It returns the resolved nodes in the order they
// are about to be recorded in, which is the sequence the caller stated.
func resolveOrderNodes(tree *store.Tree, noun string, raws []string) ([]store.Node, error) {
	r := resolve.NewResolver(tree)
	nodes := make([]store.Node, 0, len(raws))
	seen := make(map[string]bool, len(raws))
	for _, raw := range raws {
		n, err := r.Resolve(resolve.Ref{Noun: noun, Raw: raw})
		if err != nil {
			return nil, err
		}
		if seen[n.NodeID()] {
			return nil, fmt.Errorf("mutate: %s is listed twice in the order", n.NodeID())
		}
		seen[n.NodeID()] = true
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// sharedFeatureParent checks that every resolved feature sits under one epic
// and returns it. A feature's EpicKey names its parent by key, not id (the
// directory layout link, not part of feature.yml), so the epic node is looked
// up the same way epicByKey resolves it everywhere else in this package.
func sharedFeatureParent(tree *store.Tree, nodes []store.Node) (*store.Epic, error) {
	first := nodes[0].(*store.Feature)
	firstEpic := epicByKey(tree, first.EpicKey)
	if firstEpic == nil {
		return nil, fmt.Errorf("mutate: feature %s names no addressable parent epic, so its order cannot be recorded", first.ID)
	}
	for _, n := range nodes[1:] {
		f := n.(*store.Feature)
		if f.EpicKey == first.EpicKey {
			continue
		}
		otherID := f.EpicKey
		if other := epicByKey(tree, f.EpicKey); other != nil {
			otherID = other.ID
		}
		return nil, fmt.Errorf("mutate: order %ss must share one parent; %s is under %s and %s under %s",
			store.NounFeature, first.ID, firstEpic.ID, f.ID, otherID)
	}
	return firstEpic, nil
}

// sharedTaskParent checks that every resolved task sits under one feature and
// returns it. A task's FeatureID is already the parent's id (store.Tree keys
// Features by id), so unlike the feature grain above no extra lookup is
// needed to name the mismatch.
func sharedTaskParent(tree *store.Tree, nodes []store.Node) (*store.Feature, error) {
	first := nodes[0].(*store.Task)
	for _, n := range nodes[1:] {
		tk := n.(*store.Task)
		if tk.FeatureID == first.FeatureID {
			continue
		}
		return nil, fmt.Errorf("mutate: order %ss must share one parent; %s is under %s and %s under %s",
			store.NounTask, first.ID, first.FeatureID, tk.ID, tk.FeatureID)
	}
	feat, ok := tree.Features[first.FeatureID]
	if !ok || feat.Key == nil {
		return nil, fmt.Errorf("mutate: task %s names no addressable parent feature, so its order cannot be recorded", first.ID)
	}
	return feat, nil
}

// applyEpicOrder is the epic grain: a read-modify-write of the project root's
// .tiller/order.yml, which may not exist yet. It records no event: v1 has no
// project-grain event stream, and order.yml under git is the record.
func applyEpicOrder(tree *store.Tree, ids []string) (SetOrderResult, error) {
	relYml, before, data, after, changed, err := readModifyEpicOrder(tree, ids)
	if err != nil {
		return SetOrderResult{}, err
	}
	res := SetOrderResult{Noun: store.NounEpic, Order: after, Changed: changed}
	if !changed {
		return res, nil
	}
	if err := commitMutation(tree, "order epic", []store.PlannedWrite{
		store.PlanWrite(filepath.Join(tree.Root, relYml), before, data),
	}, nil); err != nil {
		return SetOrderResult{}, err
	}

	return res, nil
}

// applyFeatureOrder is the feature grain: a read-modify-write of the parent
// epic's epic.yml order field, plus the order.features event on the epic's
// stream.
func applyFeatureOrder(tree *store.Tree, deps Deps, owner store.Contributor, epic *store.Epic, ids []string) (SetOrderResult, error) {
	relYml, before, data, after, changed, err := readModifyFeatureOrder(tree, epic, ids)
	if err != nil {
		return SetOrderResult{}, err
	}
	res := SetOrderResult{Noun: store.NounFeature, Parent: epic.ID, Order: after, Changed: changed}
	if !changed {
		return res, nil
	}
	ev := events.NewEvent(deps.clock(), eventOrderFeatures, owner.ID, orderCommand(store.NounFeature, ids), epic.ID)
	// EV ids are appended to the event ledger, not keyed into the tree's id
	// maps, so there is nothing to retry against (artifact-schema.md, The eval
	// model: the AT- and EV- classes are separate non-node identifiers).
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"order": after}
	ev.Files = []events.FileChange{{Path: relYml}}
	if err := commitMutation(tree, "order feature", []store.PlannedWrite{
		store.PlanWrite(filepath.Join(tree.Root, relYml), before, data),
	}, []events.Event{ev}); err != nil {
		return SetOrderResult{}, err
	}

	return res, nil
}

// applyTaskOrder is the task grain: a read-modify-write of the parent
// feature's feature.yml order field, plus the order.tasks event on the
// feature's stream.
func applyTaskOrder(tree *store.Tree, deps Deps, owner store.Contributor, feat *store.Feature, ids []string) (SetOrderResult, error) {
	relYml, before, data, after, changed, err := readModifyTaskOrder(tree, feat, ids)
	if err != nil {
		return SetOrderResult{}, err
	}
	res := SetOrderResult{Noun: store.NounTask, Parent: feat.ID, Order: after, Changed: changed}
	if !changed {
		return res, nil
	}
	ev := events.NewEvent(deps.clock(), eventOrderTasks, owner.ID, orderCommand(store.NounTask, ids), feat.ID)
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"order": after}
	ev.Files = []events.FileChange{{Path: relYml}}
	if err := commitMutation(tree, "order task", []store.PlannedWrite{
		store.PlanWrite(filepath.Join(tree.Root, relYml), before, data),
	}, []events.Event{ev}); err != nil {
		return SetOrderResult{}, err
	}

	return res, nil
}

// readEpicOrderSnapshot reads .tiller/order.yml fresh. Absence is the valid
// zero ProjectOrder state and yields an absent snapshot suitable for either a
// guarded create or a no-op read guard.
func readEpicOrderSnapshot(tree *store.Tree) (relYml string, doc store.ProjectOrder, before store.FileSnapshot, err error) {
	relYml = store.OrderRelPath
	before, err = readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, relYml), &doc)
	if errors.Is(err, fs.ErrNotExist) {
		before, err = store.AbsentSnapshot(), nil
	}
	return
}

// readModifyEpicOrder snapshots the root order and replaces Epics with ids.
// SetOrder uses this whole-list operation for both replace and clear.
func readModifyEpicOrder(tree *store.Tree, ids []string) (relYml string, before store.FileSnapshot, data []byte, after []string, changed bool, err error) {
	relYml, doc, before, err := readEpicOrderSnapshot(tree)
	if err != nil {
		return relYml, before, nil, nil, false, err
	}
	changed = !slices.Equal(doc.Epics, ids)
	doc.Epics = ids
	data, err = yaml.Marshal(doc)
	return relYml, before, data, doc.Epics, changed, err
}

// planAppendProjectOrder snapshots the root order and appends only ids not
// already recorded. Existing entries are never reordered, removed, or filled
// from the live tree. A no-op still guards the snapshot observed by approval.
func planAppendProjectOrder(tree *store.Tree, ids []string) (store.PlannedWrite, error) {
	relYml, doc, before, err := readEpicOrderSnapshot(tree)
	if err != nil {
		return store.PlannedWrite{}, err
	}

	seen := make(map[string]struct{}, len(doc.Epics)+len(ids))
	for _, id := range doc.Epics {
		seen[id] = struct{}{}
	}
	after := append([]string(nil), doc.Epics...)
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		after = append(after, id)
	}

	path := filepath.Join(tree.Root, relYml)
	if len(after) == len(doc.Epics) {
		return store.PlanReadGuard(path, before), nil
	}
	doc.Epics = after
	data, err := yaml.Marshal(doc)
	if err != nil {
		return store.PlannedWrite{}, err
	}
	return store.PlanWrite(path, before, data), nil
}

// readModifyFeatureOrder reads the parent epic's epic.yml fresh and replaces
// its order field with ids.
func readModifyFeatureOrder(tree *store.Tree, epic *store.Epic, ids []string) (relYml string, before store.FileSnapshot, data []byte, after []string, changed bool, err error) {
	relYml = filepath.Join(".tiller", "epics", deref(epic.Key), "epic.yml")
	var doc store.Epic
	before, err = readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, relYml), &doc)
	if err != nil {
		return
	}
	changed = !slices.Equal(doc.Order, ids)
	doc.Order = ids
	data, err = yaml.Marshal(doc)
	after = doc.Order
	return
}

// readModifyTaskOrder reads the parent feature's feature.yml fresh and
// replaces its order field with ids.
func readModifyTaskOrder(tree *store.Tree, feat *store.Feature, ids []string) (relYml string, before store.FileSnapshot, data []byte, after []string, changed bool, err error) {
	relYml = filepath.Join(featureRelDir(feat), "feature.yml")
	var doc store.Feature
	before, err = readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, relYml), &doc)
	if err != nil {
		return
	}
	// EpicKey comes from the directory layout, not from feature.yml, so a doc
	// read straight off disk carries none (the same restore setNodeDependsOn
	// and batch.editFor both make on their own fresh reads).
	doc.EpicKey = feat.EpicKey
	changed = !slices.Equal(doc.Order, ids)
	doc.Order = ids
	data, err = yaml.Marshal(doc)
	after = doc.Order
	return
}

// orderCommand is the literal command an order event records: the resolved
// ids for a set, or the bare --clear form when ids is empty, so a reader of a
// node's history sees what was actually recorded rather than the raw refs the
// caller may have typed by key.
func orderCommand(noun string, ids []string) string {
	if len(ids) == 0 {
		return "order " + noun + " --clear"
	}
	return "order " + noun + " " + strings.Join(ids, " ")
}
