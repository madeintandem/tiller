package mutate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

const (
	eventRenameTitleEpic    = "rename.title.epic"
	eventRenameTitleFeature = "rename.title.feature"
	eventRenameTitleTask    = "rename.title.task"
	eventRenameKeyEpic      = "rename.key.epic"
	eventRenameKeyFeature   = "rename.key.feature"
	eventRenameKeyTask      = "rename.key.task"
)

// RenameTitleInput names an epic, feature, or task and its new display title.
type RenameTitleInput struct {
	Noun  string
	Node  string
	Title string
}

// RenameTitleResult reports the stable node identity and the title change.
type RenameTitleResult struct {
	Noun     string
	ID       string
	Key      string
	OldTitle string
	Title    string
	Changed  bool
}

func validateFreshNodeIdentity(noun, id, rel, actualID, actualKey, expectedKey, expectedParent string) error {
	actualParent := ""
	if i := strings.LastIndex(actualKey, "/"); i >= 0 {
		actualParent = actualKey[:i]
	}
	if actualID == id && actualKey == expectedKey && actualParent == expectedParent {
		return nil
	}
	return &store.EnvError{
		Reason: store.ReasonMutationStale,
		Msg: fmt.Sprintf(
			"mutate: %s changed identity after %s %s was loaded (id %q key %q under %q, expected id %q key %q under %q); reload and retry",
			rel, noun, id, actualID, actualKey, actualParent, id, expectedKey, expectedParent,
		),
	}
}

// RenameTitle changes only the machine-owned title in one node state file. It
// is legal at every lifecycle state because a title is display metadata, not
// accepted implementation state. The companion Markdown file is never read or
// written.
func RenameTitle(tree *store.Tree, deps Deps, in RenameTitleInput) (RenameTitleResult, error) {
	if strings.TrimSpace(in.Title) == "" {
		return RenameTitleResult{}, &usageError{
			reason: "invalid_title",
			msg:    "mutate: display title must not be blank",
		}
	}
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return RenameTitleResult{}, err
	}
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Node})
	if err != nil {
		return RenameTitleResult{}, err
	}

	switch n := node.(type) {
	case *store.Epic:
		if n.Key == nil {
			return RenameTitleResult{}, fmt.Errorf("mutate: epic %s carries no key, so its folder cannot be named", n.ID)
		}
		rel := filepath.Join(epicRelDir(n), "epic.yml")
		var doc store.Epic
		snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, rel), &doc)
		if err != nil {
			return RenameTitleResult{}, err
		}
		if err := validateFreshNodeIdentity(store.NounEpic, n.ID, rel, doc.ID, deref(doc.Key), deref(n.Key), ""); err != nil {
			return RenameTitleResult{}, err
		}
		oldTitle := doc.Title
		var data []byte
		if oldTitle != in.Title {
			doc.Title = in.Title
			data, err = yaml.Marshal(doc)
			if err != nil {
				return RenameTitleResult{}, err
			}
		}
		return commitTitleRename(tree, deps, owner, store.NounEpic, doc.ID, deref(doc.Key), oldTitle, in.Title, rel, snapshot, data, eventRenameTitleEpic)
	case *store.Feature:
		if n.Key == nil {
			return RenameTitleResult{}, fmt.Errorf("mutate: feature %s carries no key, so its folder cannot be named", n.ID)
		}
		rel := filepath.Join(featureRelDir(n), "feature.yml")
		var doc store.Feature
		snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, rel), &doc)
		if err != nil {
			return RenameTitleResult{}, err
		}
		if err := validateFreshNodeIdentity(store.NounFeature, n.ID, rel, doc.ID, deref(doc.Key), deref(n.Key), n.EpicKey); err != nil {
			return RenameTitleResult{}, err
		}
		oldTitle := doc.Title
		var data []byte
		if oldTitle != in.Title {
			doc.Title = in.Title
			data, err = yaml.Marshal(doc)
			if err != nil {
				return RenameTitleResult{}, err
			}
		}
		return commitTitleRename(tree, deps, owner, store.NounFeature, doc.ID, deref(doc.Key), oldTitle, in.Title, rel, snapshot, data, eventRenameTitleFeature)
	case *store.Task:
		feature, ok := tree.Features[n.FeatureID]
		if !ok || feature.Key == nil {
			return RenameTitleResult{}, fmt.Errorf("mutate: task %s names no addressable parent feature, so its folder cannot be found", n.ID)
		}
		doc, rel, snapshot, err := loadTaskFile(tree, feature, n)
		if err != nil {
			return RenameTitleResult{}, err
		}
		if err := validateFreshNodeIdentity(store.NounTask, n.ID, rel, doc.ID, deref(doc.Key), deref(n.Key), deref(feature.Key)); err != nil {
			return RenameTitleResult{}, err
		}
		oldTitle := doc.Title
		var data []byte
		if oldTitle != in.Title {
			doc.Title = in.Title
			data, err = yaml.Marshal(doc)
			if err != nil {
				return RenameTitleResult{}, err
			}
		}
		return commitTitleRename(tree, deps, owner, store.NounTask, doc.ID, deref(doc.Key), oldTitle, in.Title, rel, snapshot, data, eventRenameTitleTask)
	default:
		w, ok := store.AsWorkNode(node)
		if !ok {
			return RenameTitleResult{}, fmt.Errorf("mutate: rename title resolved %s to an unsupported node type", in.Noun)
		}
		doc, rel, snapshot, err := loadWorkItemFile(tree, w)
		if err != nil {
			return RenameTitleResult{}, err
		}
		b := doc.Body()
		oldTitle := b.Title
		var data []byte
		if oldTitle != in.Title {
			b.Title = in.Title
			data, err = yaml.Marshal(doc)
			if err != nil {
				return RenameTitleResult{}, err
			}
		}
		return commitTitleRename(tree, deps, owner, w.Noun(), b.ID, deref(b.Key), oldTitle, in.Title, rel, snapshot, data, "rename.title."+w.Noun())
	}
}

// commitTitleRename records and commits one already-marshaled concrete node.
// An equal title returns before event construction or any write.
func commitTitleRename(tree *store.Tree, deps Deps, owner store.Contributor, noun, nodeID, key, oldTitle, title, rel string, snapshot store.FileSnapshot, data []byte, eventType string) (RenameTitleResult, error) {
	result := RenameTitleResult{
		Noun: noun, ID: nodeID, Key: key, OldTitle: oldTitle, Title: title,
	}
	if oldTitle == title {
		return result, nil
	}

	ev := events.NewEvent(deps.clock(), eventType, owner.ID,
		fmt.Sprintf("rename %s %s --title %q", result.Noun, nodeID, title), nodeID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"title": oldTitle}
	ev.After = map[string]any{"title": title}
	ev.Files = []events.FileChange{{Path: rel}}

	if err := commitMutation(tree, "rename title "+result.Noun, []store.PlannedWrite{
		store.PlanWrite(filepath.Join(tree.Root, rel), snapshot, data),
	}, []events.Event{ev}); err != nil {
		return RenameTitleResult{}, err
	}
	result.Changed = true

	return result, nil
}

var renameKeySegmentRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// RenameKeyInput names an epic, feature, or task and its new canonical key.
type RenameKeyInput struct {
	Noun string
	Node string
	Key  string
}

// RenameKeyResult reports one identity-preserving proposal move.
type RenameKeyResult struct {
	Noun    string
	ID      string
	OldKey  string
	Key     string
	Moved   []string
	Changed bool
}

// RenameKey moves a proposal-backed node without changing its stable id. Epic
// moves rewrite child feature key prefixes, feature moves may change draft
// epic parent, and task moves stay within one feature.
func RenameKey(tree *store.Tree, deps Deps, in RenameKeyInput) (RenameKeyResult, error) {
	if err := validateRenameKey(in.Noun, in.Key); err != nil {
		return RenameKeyResult{}, err
	}
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return RenameKeyResult{}, err
	}
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Node})
	if err != nil {
		return RenameKeyResult{}, err
	}
	oldKey := deref(node.NodeKey())
	if oldKey == "" {
		return RenameKeyResult{}, fmt.Errorf("mutate: %s %s carries no key, so its folder cannot be named", in.Noun, node.NodeID())
	}
	// Every other noun's Key already arrives canonical. A shelf noun's Key
	// is one local segment with the shelf prefix implied (validateRenameKey
	// above refused anything else), so newKey is the canonical form every
	// comparison, result, and event below actually means; in.Key itself
	// stays the literal argument the caller typed, for the recorded command
	// and the destination folder name.
	newKey := in.Key
	if store.IsWorkNoun(in.Noun) {
		newKey = store.ShelfCanonicalKey(in.Noun, in.Key)
	}
	result := RenameKeyResult{Noun: in.Noun, ID: node.NodeID(), OldKey: oldKey, Key: newKey}
	if oldKey == newKey {
		return result, nil
	}
	if renameKeyExists(tree, in.Noun, newKey, node.NodeID()) {
		return RenameKeyResult{}, renameDestinationExists(in.Noun, newKey)
	}

	var writes []store.PlannedWrite
	if store.IsWorkNoun(in.Noun) {
		shelfDir := filepath.Join(tree.Root, ".tiller", store.ShelfDir(in.Noun))
		shelfSnapshot, err := store.ReadDirectorySnapshot(tree.Root, shelfDir)
		if err != nil {
			return RenameKeyResult{}, err
		}
		writes = []store.PlannedWrite{store.PlanDirectoryGuard(shelfDir, shelfSnapshot)}
	} else {
		epicsDir := filepath.Join(tree.Root, ".tiller", "epics")
		lineage, err := freshRenameLineage(tree, epicsDir)
		if err != nil {
			return RenameKeyResult{}, err
		}
		writes = []store.PlannedWrite{store.PlanLineageInventoryGuard(epicsDir, lineage)}
	}
	var sourceDir, destinationDir, eventType string
	var rewrite func(string, []byte) ([]byte, error)
	var eventBefore, eventAfter map[string]any
	var extraEventFiles, changedEventTargets []string

	switch n := node.(type) {
	case *store.Epic:
		if n.Status != statusDraft {
			return RenameKeyResult{}, keyRenameLocked(store.NounEpic, oldKey, n.Status, "epic key changes require draft")
		}
		children := make(map[string]*store.Feature)
		for _, feature := range tree.Features {
			if feature.EpicKey != oldKey {
				continue
			}
			if feature.Status != statusDraft {
				return RenameKeyResult{}, keyRenameLocked(store.NounEpic, oldKey, n.Status,
					fmt.Sprintf("child feature %s is %s", feature.ID, feature.Status))
			}
			if len(tree.FeatureTasks[feature.ID]) != 0 {
				return RenameKeyResult{}, keyRenameLocked(store.NounEpic, oldKey, n.Status,
					fmt.Sprintf("child feature %s has tasks", feature.ID))
			}
			children[filepath.Join("features", localKey(deref(feature.Key)), "feature.yml")] = feature
			changedEventTargets = append(changedEventTargets, feature.ID)
		}
		sourceDir = filepath.Join(tree.Root, epicRelDir(n))
		destinationDir = filepath.Join(tree.Root, ".tiller", "epics", in.Key)
		rewrite = epicKeyStateRewriter(n, children, in.Key)
		eventType = eventRenameKeyEpic
		eventBefore = map[string]any{"key": oldKey}
		eventAfter = map[string]any{"key": in.Key}

	case *store.Feature:
		newEpicKey, _, _ := splitFeatureKey(in.Key)
		if n.Status != statusDraft {
			return RenameKeyResult{}, keyRenameLocked(store.NounFeature, oldKey, n.Status, "feature key changes require draft")
		}
		if len(tree.FeatureTasks[n.ID]) != 0 {
			return RenameKeyResult{}, keyRenameLocked(store.NounFeature, oldKey, n.Status, "feature has tasks")
		}
		sourceEpic, err := resolveEpicForRename(tree, n.EpicKey)
		if err != nil {
			return RenameKeyResult{}, err
		}
		if newEpicKey != n.EpicKey {
			destinationEpic, err := resolveEpicForRename(tree, newEpicKey)
			if err != nil {
				return RenameKeyResult{}, err
			}
			if sourceEpic.Status != statusDraft {
				return RenameKeyResult{}, keyRenameLocked(store.NounFeature, oldKey, n.Status,
					fmt.Sprintf("source epic %s is %s", n.EpicKey, sourceEpic.Status))
			}
			if destinationEpic.Status != statusDraft {
				return RenameKeyResult{}, keyRenameLocked(store.NounFeature, oldKey, n.Status,
					fmt.Sprintf("destination epic %s is %s", newEpicKey, destinationEpic.Status))
			}
			orderWrite, orderChanged, err := planFeatureSourceOrder(tree, sourceEpic, n.ID)
			if err != nil {
				return RenameKeyResult{}, err
			}
			writes = append(writes, orderWrite)
			if orderChanged {
				extraEventFiles = append(extraEventFiles, filepath.Join(epicRelDir(sourceEpic), "epic.yml"))
				changedEventTargets = append(changedEventTargets, sourceEpic.ID)
			}
			destinationGuard, err := planFreshEpicGuard(tree, destinationEpic)
			if err != nil {
				return RenameKeyResult{}, err
			}
			writes = append(writes, destinationGuard)
		}
		sourceDir = filepath.Join(tree.Root, featureRelDir(n))
		destinationDir = filepath.Join(tree.Root, ".tiller", "epics", newEpicKey, "features", localKey(in.Key))
		rewrite = featureKeyStateRewriter(n, in.Key)
		eventType = eventRenameKeyFeature
		eventBefore = map[string]any{"key": oldKey, "epic": n.EpicKey}
		eventAfter = map[string]any{"key": in.Key, "epic": newEpicKey}

	case *store.Task:
		newEpicKey, newFeatureKey, _, _ := splitTaskKey(in.Key)
		if n.Status != "proposed" {
			return RenameKeyResult{}, keyRenameLocked(store.NounTask, oldKey, n.Status, "task key changes require proposed")
		}
		parent, ok := tree.Features[n.FeatureID]
		if !ok || parent.Key == nil {
			return RenameKeyResult{}, fmt.Errorf("mutate: task %s has no addressable parent feature", n.ID)
		}
		newParent := newEpicKey + "/" + newFeatureKey
		if newParent != deref(parent.Key) {
			return RenameKeyResult{}, &usageError{
				reason: "task_reparent_not_supported",
				msg: fmt.Sprintf("mutate: task key %q must stay under feature %s; task reparenting is not supported",
					in.Key, deref(parent.Key)),
			}
		}
		sourceDir = filepath.Join(tree.Root, store.TaskRelDir(parent.EpicKey, localKey(deref(parent.Key)), localKey(oldKey)))
		destinationDir = filepath.Join(tree.Root, store.TaskRelDir(parent.EpicKey, localKey(deref(parent.Key)), localKey(in.Key)))
		rewrite = taskKeyStateRewriter(n, parent, in.Key)
		eventType = eventRenameKeyTask
		eventBefore = map[string]any{"key": oldKey}
		eventAfter = map[string]any{"key": in.Key}

	case *store.Bug, *store.Chore, *store.Tweak:
		w, ok := store.AsWorkNode(node)
		if !ok {
			return RenameKeyResult{}, fmt.Errorf("mutate: rename key resolved %s to an unsupported node type", in.Noun)
		}
		if w.Body().Status != statusProposed {
			return RenameKeyResult{}, keyRenameLocked(w.Noun(), oldKey, w.Body().Status, fmt.Sprintf("%s key changes require proposed", w.Noun()))
		}
		sourceDir = filepath.Join(tree.Root, workItemRelDir(w))
		destinationDir = filepath.Join(tree.Root, store.ShelfRelDir(w.Noun(), in.Key))
		rewrite = workKeyStateRewriter(w, newKey)
		eventType = "rename.key." + w.Noun()
		eventBefore = map[string]any{"key": oldKey}
		eventAfter = map[string]any{"key": newKey}

	default:
		return RenameKeyResult{}, fmt.Errorf("mutate: rename key resolved %s to an unsupported node type", in.Noun)
	}

	if err := requireAbsentRenameDestination(tree.Root, destinationDir, in.Noun, newKey); err != nil {
		return RenameKeyResult{}, err
	}
	moveWrites, moved, err := planRenameTreeMove(tree.Root, sourceDir, destinationDir, rewrite)
	if err != nil {
		return RenameKeyResult{}, err
	}
	writes = append(writes, moveWrites...)

	ev := events.NewEvent(deps.clock(), eventType, owner.ID,
		fmt.Sprintf("rename %s %s --key %q", in.Noun, node.NodeID(), in.Key),
		renameEventTargets(node.NodeID(), changedEventTargets...)...)
	ev.ID = deps.mintRaw("EV")
	ev.Before = eventBefore
	ev.After = eventAfter
	for _, path := range moved {
		ev.Files = append(ev.Files, events.FileChange{Path: path})
	}
	for _, path := range extraEventFiles {
		ev.Files = append(ev.Files, events.FileChange{Path: path})
	}

	if err := commitMutation(tree, "rename key "+in.Noun, writes, []events.Event{ev}); err != nil {
		return RenameKeyResult{}, err
	}
	result.Moved = moved
	result.Changed = true

	return result, nil
}

func validateRenameKey(noun, key string) error {
	var parts []string
	switch noun {
	case store.NounEpic:
		parts = strings.Split(key, "/")
		if len(parts) != 1 {
			return invalidRenameKey(noun, key, "<epic-key>")
		}
		// The same refusal `add epic` makes (add.go), for the same collisions:
		// an epic keyed bugs would give its features the keys bugs/<f>, which
		// collide with the shelf's own canonical keys in the by-key index, and
		// the branches tiller/bugs/<f>, which collide with a bug's own branch.
		// Enforcing it only at add would leave rename as the way around it.
		if store.IsReservedEpicKey(key) {
			return &usageError{reason: "reserved_key", msg: fmt.Sprintf("%q is a shelf folder name (.tiller/%s/) and cannot be an epic key", key, key)}
		}
	case store.NounFeature:
		var err error
		_, _, err = splitFeatureKey(key)
		if err != nil {
			return err
		}
		parts = strings.Split(key, "/")
	case store.NounTask:
		var err error
		_, _, _, err = splitTaskKey(key)
		if err != nil {
			return err
		}
		parts = strings.Split(key, "/")
	case store.NounBug, store.NounChore, store.NounTweak:
		parts = strings.Split(key, "/")
		if len(parts) != 1 {
			return invalidRenameKey(noun, key, "one local key segment, the shelf prefix is implied")
		}
	default:
		return &usageError{reason: "invalid_noun", msg: fmt.Sprintf("mutate: rename does not support noun %q", noun)}
	}
	for _, part := range parts {
		if !renameKeySegmentRE.MatchString(part) {
			return invalidRenameKey(noun, key, "lowercase key segments matching [a-z0-9]+(-[a-z0-9]+)*")
		}
	}
	return nil
}

func invalidRenameKey(noun, key, shape string) error {
	return &usageError{
		reason: "invalid_key",
		msg:    fmt.Sprintf("%s key %q must be %s", noun, key, shape),
	}
}

func renameKeyExists(tree *store.Tree, noun, key, excludeID string) bool {
	var nodes []store.Node
	switch noun {
	case store.NounEpic:
		for _, node := range tree.Epics {
			nodes = append(nodes, node)
		}
	case store.NounFeature:
		for _, node := range tree.Features {
			nodes = append(nodes, node)
		}
	case store.NounTask:
		for _, node := range tree.Tasks {
			nodes = append(nodes, node)
		}
	default:
		if store.IsWorkNoun(noun) {
			for _, node := range tree.WorkNodesOf(noun) {
				nodes = append(nodes, node)
			}
		}
	}
	for _, node := range nodes {
		if node.NodeID() != excludeID && deref(node.NodeKey()) == key {
			return true
		}
	}
	return false
}

func renameDestinationExists(noun, key string) error {
	return &usageError{
		reason: "rename_destination_exists",
		msg:    fmt.Sprintf("mutate: cannot rename %s to %q because its destination path already exists", noun, key),
	}
}

func keyRenameLocked(noun, key, status, detail string) error {
	return &usageError{
		reason: "key_rename_locked",
		msg:    fmt.Sprintf("mutate: cannot rename %s %s while it is %s: %s", noun, key, status, detail),
	}
}

func resolveEpicForRename(tree *store.Tree, key string) (*store.Epic, error) {
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounEpic, Raw: key})
	if err != nil {
		return nil, err
	}
	epic, ok := node.(*store.Epic)
	if !ok || epic.Key == nil {
		return nil, fmt.Errorf("mutate: %q does not resolve to an addressable epic", key)
	}
	return epic, nil
}

func freshRenameLineage(tree *store.Tree, epicsDir string) (store.LineageInventory, error) {
	inventory, err := store.ReadLineageInventory(tree.Root, epicsDir)
	if err != nil {
		return store.LineageInventory{}, err
	}
	expected := make(map[string]string, len(tree.Epics)+len(tree.Features)+len(tree.Tasks))
	for id, epic := range tree.Epics {
		path := ""
		if epic.Key != nil {
			path = filepath.Join(*epic.Key, "epic.yml")
		}
		expected[store.NounEpic+":"+id] = path
	}
	for id, feature := range tree.Features {
		path := ""
		if feature.Key != nil {
			path = filepath.Join(feature.EpicKey, "features", localKey(*feature.Key), "feature.yml")
		}
		expected[store.NounFeature+":"+id] = path
	}
	for id, task := range tree.Tasks {
		path := ""
		parent := tree.Features[task.FeatureID]
		if task.Key != nil && parent != nil && parent.Key != nil {
			path = filepath.Join(parent.EpicKey, "features", localKey(*parent.Key), store.TaskDirName,
				localKey(*task.Key), store.TaskStateFile)
		}
		expected[store.NounTask+":"+id] = path
	}
	actual := make(map[string]string, len(inventory.Files))
	for _, file := range inventory.Files {
		data, snapshot, err := store.ReadRegularFileSnapshot(tree.Root, filepath.Join(epicsDir, file.Path))
		if err != nil || !snapshot.Exists() {
			return store.LineageInventory{}, renameLineageStale()
		}
		var identity struct {
			ID string `yaml:"id"`
		}
		if err := yaml.Unmarshal(data, &identity); err != nil {
			return store.LineageInventory{}, err
		}
		key := file.Noun + ":" + identity.ID
		if _, duplicate := actual[key]; duplicate {
			return store.LineageInventory{}, renameLineageStale()
		}
		actual[key] = filepath.Clean(file.Path)
	}
	if !renamePathsMatch(expected, actual) {
		return store.LineageInventory{}, renameLineageStale()
	}

	return inventory, nil
}

func renamePathsMatch(expected, actual map[string]string) bool {
	if len(expected) != len(actual) {
		return false
	}
	for key, expectedPath := range expected {
		actualPath, ok := actual[key]
		if !ok || expectedPath != "" && filepath.Clean(expectedPath) != actualPath {
			return false
		}
	}
	return true
}

func renameLineageStale() error {
	return &store.EnvError{
		Reason: store.ReasonMutationStale,
		Msg:    "mutate: project lineage paths changed after the tree was loaded; reload and retry",
	}
}

func planFeatureSourceOrder(tree *store.Tree, epic *store.Epic, featureID string) (store.PlannedWrite, bool, error) {
	rel := filepath.Join(epicRelDir(epic), "epic.yml")
	var doc store.Epic
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, rel), &doc)
	if err != nil {
		return store.PlannedWrite{}, false, err
	}
	if err := validateFreshKeyState(store.NounEpic, epic.ID, rel, doc.ID, deref(doc.Key), deref(epic.Key), "", doc.Status, epic.Status); err != nil {
		return store.PlannedWrite{}, false, err
	}
	next := make([]string, 0, len(doc.Order))
	for _, id := range doc.Order {
		if id != featureID {
			next = append(next, id)
		}
	}
	if len(next) == len(doc.Order) {
		return store.PlanReadGuard(filepath.Join(tree.Root, rel), snapshot), false, nil
	}
	doc.Order = next
	data, err := yaml.Marshal(doc)
	if err != nil {
		return store.PlannedWrite{}, false, err
	}
	return store.PlanWrite(filepath.Join(tree.Root, rel), snapshot, data), true, nil
}

func planFreshEpicGuard(tree *store.Tree, epic *store.Epic) (store.PlannedWrite, error) {
	rel := filepath.Join(epicRelDir(epic), "epic.yml")
	var doc store.Epic
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, rel), &doc)
	if err != nil {
		return store.PlannedWrite{}, err
	}
	if err := validateFreshKeyState(store.NounEpic, epic.ID, rel, doc.ID, deref(doc.Key), deref(epic.Key), "", doc.Status, epic.Status); err != nil {
		return store.PlannedWrite{}, err
	}
	return store.PlanReadGuard(filepath.Join(tree.Root, rel), snapshot), nil
}

func requireAbsentRenameDestination(root, path, noun, key string) error {
	exists, err := store.PathExistsNoSymlinks(root, path)
	if err != nil {
		return err
	}
	if exists {
		return renameDestinationExists(noun, key)
	}
	return nil
}

type renameMoveFile struct {
	relative string
	data     []byte
	snapshot store.FileSnapshot
	mode     os.FileMode
}

type renameMoveDirectory struct {
	path     string
	snapshot store.DirectorySnapshot
}

func renameEventTargets(primary string, changed ...string) []string {
	sorted := append([]string(nil), changed...)
	sort.Strings(sorted)
	targets := []string{primary}
	seen := map[string]struct{}{primary: {}}
	for _, id := range sorted {
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		targets = append(targets, id)
	}
	return targets
}

func planRenameTreeMove(root, sourceDir, destinationDir string, rewrite func(string, []byte) ([]byte, error)) ([]store.PlannedWrite, []string, error) {
	info, err := mutationPathInfo(root, sourceDir)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, nonRegularMoveEntry(sourceDir)
	}
	var files []renameMoveFile
	var directories []renameMoveDirectory
	var walk func(string) error
	walk = func(dir string) error {
		snapshot, err := store.ReadDirectorySnapshot(root, dir)
		if err != nil {
			return err
		}
		directories = append(directories, renameMoveDirectory{path: dir, snapshot: snapshot})
		entries, err := readMutationDirectory(root, dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nonRegularMoveEntry(path)
			}
			if info.IsDir() {
				if err := walk(path); err != nil {
					return err
				}
				continue
			}
			if !info.Mode().IsRegular() {
				return nonRegularMoveEntry(path)
			}
			data, snapshot, err := store.ReadRegularFileSnapshot(root, path)
			if err != nil {
				return err
			}
			after, err := mutationPathInfo(root, path)
			if err != nil || !after.Mode().IsRegular() || after.Mode()&os.ModeSymlink != 0 ||
				after.Mode().Perm() != info.Mode().Perm() {
				return nonRegularMoveEntry(path)
			}
			relative, err := filepath.Rel(sourceDir, path)
			if err != nil {
				return err
			}
			if rewrite != nil {
				data, err = rewrite(relative, data)
				if err != nil {
					return err
				}
			}
			files = append(files, renameMoveFile{relative: relative, data: data, snapshot: snapshot, mode: info.Mode().Perm()})
		}
		return nil
	}
	if err := walk(sourceDir); err != nil {
		return nil, nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].relative < files[j].relative })
	sort.Slice(directories, func(i, j int) bool {
		leftDepth := strings.Count(directories[i].path, string(filepath.Separator))
		rightDepth := strings.Count(directories[j].path, string(filepath.Separator))
		if leftDepth == rightDepth {
			return directories[i].path > directories[j].path
		}
		return leftDepth > rightDepth
	})

	writes := make([]store.PlannedWrite, 0, len(files)*2+len(directories))
	moved := make([]string, 0, len(files))
	for _, file := range files {
		destination := filepath.Join(destinationDir, file.relative)
		writes = append(writes, store.PlanWriteWithMode(destination, store.AbsentSnapshot(), file.data, file.mode))
		rel, err := filepath.Rel(root, destination)
		if err != nil {
			return nil, nil, err
		}
		moved = append(moved, rel)
	}
	for i := len(directories) - 1; i >= 0; i-- {
		dir := directories[i]
		relative, err := filepath.Rel(sourceDir, dir.path)
		if err != nil {
			return nil, nil, err
		}
		writes = append(writes, store.PlanSetCreatedDirectoryMode(
			filepath.Join(destinationDir, relative), dir.snapshot.Mode))
	}
	for _, file := range files {
		writes = append(writes, store.PlanDeleteFileWithMode(
			filepath.Join(sourceDir, file.relative), file.snapshot, file.mode))
	}
	for _, dir := range directories {
		writes = append(writes, store.PlanRemoveEmptyDir(dir.path, dir.snapshot))
	}
	return writes, moved, nil
}

func nonRegularMoveEntry(path string) error {
	return &usageError{
		reason: "non_regular_move_entry",
		msg:    fmt.Sprintf("mutate: identity-preserving moves require regular files and directories; %s is not regular", path),
	}
}

func epicKeyStateRewriter(epic *store.Epic, children map[string]*store.Feature, newEpicKey string) func(string, []byte) ([]byte, error) {
	return func(relative string, data []byte) ([]byte, error) {
		if relative == "epic.yml" {
			var doc store.Epic
			if err := yaml.Unmarshal(data, &doc); err != nil {
				return nil, err
			}
			if err := validateFreshKeyState(store.NounEpic, epic.ID, relative, doc.ID, deref(doc.Key), deref(epic.Key), "", doc.Status, epic.Status); err != nil {
				return nil, err
			}
			doc.Key = &newEpicKey
			return yaml.Marshal(doc)
		}
		child, ok := children[relative]
		if !ok {
			return data, nil
		}
		var doc store.Feature
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		if err := validateFreshKeyState(store.NounFeature, child.ID, relative, doc.ID, deref(doc.Key), deref(child.Key), deref(epic.Key), doc.Status, child.Status); err != nil {
			return nil, err
		}
		key := newEpicKey + "/" + localKey(deref(child.Key))
		doc.Key = &key
		return yaml.Marshal(doc)
	}
}

func featureKeyStateRewriter(feature *store.Feature, newKey string) func(string, []byte) ([]byte, error) {
	return func(relative string, data []byte) ([]byte, error) {
		if relative != "feature.yml" {
			return data, nil
		}
		var doc store.Feature
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		if err := validateFreshKeyState(store.NounFeature, feature.ID, relative, doc.ID, deref(doc.Key), deref(feature.Key), feature.EpicKey, doc.Status, feature.Status); err != nil {
			return nil, err
		}
		doc.Key = &newKey
		return yaml.Marshal(doc)
	}
}

func taskKeyStateRewriter(task *store.Task, parent *store.Feature, newKey string) func(string, []byte) ([]byte, error) {
	return func(relative string, data []byte) ([]byte, error) {
		if relative != store.TaskStateFile {
			return data, nil
		}
		var doc store.Task
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		if err := validateFreshKeyState(store.NounTask, task.ID, relative, doc.ID, deref(doc.Key), deref(task.Key), deref(parent.Key), doc.Status, task.Status); err != nil {
			return nil, err
		}
		doc.Key = &newKey
		return yaml.Marshal(doc)
	}
}

// workKeyStateRewriter rewrites the canonical key inside a shelf node's
// state file during a folder move, after re-validating that the file still
// holds the node the caller addressed, proposed.
func workKeyStateRewriter(w store.WorkNode, newKey string) func(string, []byte) ([]byte, error) {
	stateFile := store.ShelfStateFile(w.Noun())
	return func(rel string, data []byte) ([]byte, error) {
		if filepath.Base(rel) != stateFile {
			return data, nil
		}
		doc, err := store.NewWorkNode(w.Noun())
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, doc); err != nil {
			return nil, err
		}
		b := doc.Body()
		if err := validateFreshKeyState(w.Noun(), w.NodeID(), rel, b.ID, deref(b.Key), deref(w.NodeKey()), store.ShelfDir(w.Noun()), b.Status, statusProposed); err != nil {
			return nil, err
		}
		b.Key = &newKey
		return yaml.Marshal(doc)
	}
}

func validateFreshKeyState(noun, id, rel, actualID, actualKey, expectedKey, expectedParent, actualStatus, expectedStatus string) error {
	if err := validateFreshNodeIdentity(noun, id, rel, actualID, actualKey, expectedKey, expectedParent); err != nil {
		return err
	}
	if actualStatus == expectedStatus {
		return nil
	}
	return &store.EnvError{
		Reason: store.ReasonMutationStale,
		Msg: fmt.Sprintf(
			"mutate: %s changed after %s %s was loaded (id %q key %q status %q); reload and retry",
			rel, noun, id, actualID, actualKey, actualStatus,
		),
	}
}
