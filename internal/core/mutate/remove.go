package mutate

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

const (
	reasonRemoveReasonRequired       = "remove_reason_required"
	reasonRemoveStateLocked          = "remove_state_locked"
	reasonProposedAmendmentReference = "proposed_amendment_reference"
	reasonRemoveLineageReference     = "remove_lineage_reference"
	reasonNonRegularRemoveEntry      = "non_regular_remove_entry"
)

type RemoveInput struct {
	Noun   string
	Node   string
	Reason string
}

type RemoveResult struct {
	Noun         string
	ID           string
	Key          string
	Reason       string
	RemovedIDs   []string
	ChangedFiles []string
	// Detached is the shelf node ids whose attached_to, resolved_by, or
	// duplicate_of provenance field was cleared because it named a removed
	// node.
	Detached []string
}

// RemoveBlockedError is a business refusal: the named proposal cannot be
// erased while its lifecycle, lineage, staged amendments, or on-disk shape
// makes that unsafe.
type RemoveBlockedError struct {
	Noun   string
	ID     string
	Key    string
	Status string
	Code   string
	Detail string
}

func (e *RemoveBlockedError) Error() string {
	ref := e.ID
	if e.Key != "" {
		ref = e.Key + " (" + e.ID + ")"
	}
	if e.Detail == "" {
		return fmt.Sprintf("remove refuses %s %s", e.Noun, ref)
	}
	return fmt.Sprintf("remove refuses %s %s: %s", e.Noun, ref, e.Detail)
}
func (e *RemoveBlockedError) Reason() string { return e.Code }

type removedNode struct {
	noun          string
	id            string
	key           string
	title         string
	dir           string
	statePath     string
	stateSnapshot store.FileSnapshot
}

// RemoveDraft permanently removes only unapproved proposal nodes: a proposed
// task, a task-free draft feature, or a draft epic whose draft features are all
// task-free. References, deletion, and the surviving audit event commit as one
// journaled mutation.
func RemoveDraft(tree *store.Tree, deps Deps, in RemoveInput) (RemoveResult, error) {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return RemoveResult{}, &usageError{reason: reasonRemoveReasonRequired, msg: "remove requires a non-blank --reason explaining why the proposal is being removed"}
	}
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return RemoveResult{}, err
	}
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Node})
	if err != nil {
		return RemoveResult{}, err
	}

	removed, err := collectRemovedNodes(tree, node)
	if err != nil {
		return RemoveResult{}, err
	}
	removedIDs := make([]string, 0, len(removed))
	removedSet := make(map[string]struct{}, len(removed))
	for _, item := range removed {
		removedIDs = append(removedIDs, item.id)
		removedSet[item.id] = struct{}{}
	}
	if err := validateRemovedNodeStates(tree, removed); err != nil {
		return RemoveResult{}, err
	}
	stateSnapshots := make(map[string]store.FileSnapshot, len(removed))
	for _, item := range removed {
		stateSnapshots[item.statePath] = item.stateSnapshot
	}

	epicsDir := filepath.Join(tree.Root, ".tiller", "epics")
	lineage, err := freshRenameLineage(tree, epicsDir)
	if err != nil {
		return RemoveResult{}, err
	}
	writes := []store.PlannedWrite{store.PlanLineageInventoryGuard(epicsDir, lineage)}
	if store.IsWorkNoun(removed[0].noun) {
		shelfDir := filepath.Join(tree.Root, ".tiller", store.ShelfDir(removed[0].noun))
		shelfInventory, err := store.ReadDirectorySnapshot(tree.Root, shelfDir)
		if err != nil {
			return RemoveResult{}, err
		}
		writes = append(writes, store.PlanDirectoryGuard(shelfDir, shelfInventory))
	}
	changed := map[string]struct{}{}

	stateWrites, err := planRemovalStateReferences(tree, removedSet, changed)
	if err != nil {
		return RemoveResult{}, err
	}
	writes = append(writes, stateWrites...)
	questionWrite, err := planRemovalQuestions(tree, removedSet, changed)
	if err != nil {
		return RemoveResult{}, err
	}
	writes = append(writes, questionWrite)
	orderWrite, err := planRemovalProjectOrder(tree, removedSet, changed)
	if err != nil {
		return RemoveResult{}, err
	}
	writes = append(writes, orderWrite)
	shelfWrites, detached, err := planRemovalShelfReferences(tree, removedSet, changed)
	if err != nil {
		return RemoveResult{}, err
	}
	writes = append(writes, shelfWrites...)
	for _, noun := range store.ShelfNouns() {
		shelfOrderWrite, err := planRemovalShelfOrder(tree, noun, removedSet, changed)
		if err != nil {
			return RemoveResult{}, err
		}
		writes = append(writes, shelfOrderWrite)
	}
	amendmentGuards, err := planRemovalAmendments(tree, removedSet)
	if err != nil {
		return RemoveResult{}, err
	}
	writes = append(writes, amendmentGuards...)

	for _, item := range removed {
		if item.dir == "" {
			continue
		}
		deleteWrites, deleted, err := planRemovalTree(tree.Root, item.dir, stateSnapshots)
		if err != nil {
			return RemoveResult{}, err
		}
		writes = append(writes, deleteWrites...)
		for _, path := range deleted {
			changed[path] = struct{}{}
		}
	}
	changedFiles := sortedSet(changed)

	metadata := make([]map[string]string, 0, len(removed))
	for _, item := range removed {
		metadata = append(metadata, map[string]string{"noun": item.noun, "id": item.id, "key": item.key, "title": item.title})
	}
	eventType := "remove." + removed[0].noun
	switch removed[0].noun {
	case store.NounTask:
		eventType = "remove.proposed.task"
	case store.NounFeature:
		eventType = "remove.draft.feature"
	case store.NounEpic:
		eventType = "remove.draft.epic"
	default:
		if store.IsWorkNoun(removed[0].noun) {
			eventType = "remove.proposed." + removed[0].noun
		}
	}
	ev := events.NewEvent(deps.clock(), eventType, owner.ID,
		fmt.Sprintf("remove %s %s --reason %q", in.Noun, node.NodeID(), reason), removedIDs...)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"removed": metadata}
	ev.After = map[string]any{"reason": reason, "removed_ids": removedIDs, "changed_files": changedFiles}
	for _, path := range changedFiles {
		ev.Files = append(ev.Files, events.FileChange{Path: filepath.FromSlash(path)})
	}

	if err := commitMutation(tree, "remove "+in.Noun, writes, []events.Event{ev}); err != nil {
		return RemoveResult{}, err
	}
	result := RemoveResult{
		Noun: in.Noun, ID: node.NodeID(), Key: deref(node.NodeKey()), Reason: reason,
		RemovedIDs: removedIDs, ChangedFiles: changedFiles, Detached: detached,
	}

	return result, nil
}

func collectRemovedNodes(tree *store.Tree, node store.Node) ([]removedNode, error) {
	locked := func(noun, id, key, status, detail string) ([]removedNode, error) {
		return nil, &RemoveBlockedError{Noun: noun, ID: id, Key: key, Status: status, Code: reasonRemoveStateLocked, Detail: detail}
	}
	switch n := node.(type) {
	case *store.Task:
		if n.Key == nil {
			return nil, fmt.Errorf("mutate: task %s carries no key, so its folder cannot be named", n.ID)
		}
		key := *n.Key
		if n.Status != statusProposed {
			return locked(store.NounTask, n.ID, key, n.Status, "tasks must be proposed; return a ready, unstarted task before removing it")
		}
		parent := tree.Features[n.FeatureID]
		if parent == nil || parent.Key == nil {
			return nil, fmt.Errorf("mutate: task %s has no addressable parent feature", n.ID)
		}
		return []removedNode{{noun: store.NounTask, id: n.ID, key: key, title: n.Title, dir: filepath.Join(tree.Root, store.TaskRelDir(parent.EpicKey, localKey(*parent.Key), localKey(key)))}}, nil
	case *store.Feature:
		if n.Key == nil {
			return nil, fmt.Errorf("mutate: feature %s carries no key, so its folder cannot be named", n.ID)
		}
		key := *n.Key
		if n.Status != statusDraft {
			return locked(store.NounFeature, n.ID, key, n.Status, "features must be draft")
		}
		if len(tree.FeatureTasks[n.ID]) != 0 {
			return locked(store.NounFeature, n.ID, key, n.Status, "features must be task-free")
		}
		return []removedNode{{noun: store.NounFeature, id: n.ID, key: key, title: n.Title, dir: filepath.Join(tree.Root, featureRelDir(n))}}, nil
	case *store.Epic:
		if n.Key == nil {
			return nil, fmt.Errorf("mutate: epic %s carries no key, so its folder cannot be named", n.ID)
		}
		key := *n.Key
		if n.Status != statusDraft {
			return locked(store.NounEpic, n.ID, key, n.Status, "epics must be draft")
		}
		children := make([]*store.Feature, 0)
		for _, feature := range tree.Features {
			if feature.EpicKey != key {
				continue
			}
			if feature.Key == nil {
				return nil, fmt.Errorf("mutate: feature %s carries no key, so its folder cannot be named", feature.ID)
			}
			children = append(children, feature)
		}
		sort.Slice(children, func(i, j int) bool { return *children[i].Key < *children[j].Key })
		out := []removedNode{{noun: store.NounEpic, id: n.ID, key: key, title: n.Title, dir: filepath.Join(tree.Root, epicRelDir(n))}}
		for _, feature := range children {
			featureKey := *feature.Key
			if feature.Status != statusDraft {
				return locked(store.NounEpic, n.ID, key, n.Status, fmt.Sprintf("child feature %s is %s, not draft", feature.ID, feature.Status))
			}
			if len(tree.FeatureTasks[feature.ID]) != 0 {
				return locked(store.NounEpic, n.ID, key, n.Status, fmt.Sprintf("child feature %s has tasks", feature.ID))
			}
			out = append(out, removedNode{noun: store.NounFeature, id: feature.ID, key: featureKey, title: feature.Title})
		}
		return out, nil
	default:
		w, ok := store.AsWorkNode(node)
		if !ok {
			supported := append([]string{store.NounEpic, store.NounFeature, store.NounTask}, store.ShelfNouns()...)
			return nil, &usageError{reason: "remove_noun", msg: "remove supports " + englishList("or", supported...)}
		}
		b := w.Body()
		if b.Key == nil {
			return nil, fmt.Errorf("mutate: %s %s carries no key, so its folder cannot be named", w.Noun(), b.ID)
		}
		if b.Status != statusProposed {
			return locked(w.Noun(), b.ID, *b.Key, b.Status, store.ShelfDir(w.Noun())+" must be proposed; return a ready, unstarted "+w.Noun()+" before removing it")
		}
		return []removedNode{{noun: w.Noun(), id: b.ID, key: *b.Key, title: b.Title, dir: filepath.Join(tree.Root, workItemRelDir(w))}}, nil
	}
}

func validateRemovedNodeStates(tree *store.Tree, removed []removedNode) error {
	for i := range removed {
		item := &removed[i]
		switch item.noun {
		case store.NounEpic:
			doc := tree.Epics[item.id]
			if doc == nil {
				return removalStale(item.id)
			}
			path := filepath.Join(tree.Root, epicRelDir(doc), "epic.yml")
			var disk store.Epic
			snapshot, err := readRemovalYAML(tree.Root, path, &disk)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(disk, *doc) {
				return removalStale(path)
			}
			item.statePath, item.stateSnapshot = path, snapshot
		case store.NounFeature:
			doc := tree.Features[item.id]
			if doc == nil {
				return removalStale(item.id)
			}
			path := filepath.Join(tree.Root, featureRelDir(doc), "feature.yml")
			var disk store.Feature
			snapshot, err := readRemovalYAML(tree.Root, path, &disk)
			if err != nil {
				return err
			}
			expected := *doc
			expected.EpicKey = ""
			expected.HasEval = false
			if !reflect.DeepEqual(disk, expected) {
				return removalStale(path)
			}
			if len(disk.Supersedes) != 0 || disk.SupersededBy != nil {
				return removalLineageBlocked(store.NounFeature, disk.ID, deref(disk.Key))
			}
			item.statePath, item.stateSnapshot = path, snapshot
		case store.NounTask:
			doc := tree.Tasks[item.id]
			if doc == nil {
				return removalStale(item.id)
			}
			parent := tree.Features[doc.FeatureID]
			if parent == nil || parent.Key == nil {
				return removalStale(item.id)
			}
			path := filepath.Join(tree.Root, store.TaskRelDir(parent.EpicKey, localKey(deref(parent.Key)), localKey(deref(doc.Key))), store.TaskStateFile)
			var disk store.Task
			snapshot, err := readRemovalYAML(tree.Root, path, &disk)
			if err != nil {
				return err
			}
			expected := *doc
			expected.FeatureID = ""
			if !reflect.DeepEqual(disk, expected) {
				return removalStale(path)
			}
			if len(disk.Supersedes) != 0 || disk.SupersededBy != nil {
				return removalLineageBlocked(store.NounTask, disk.ID, deref(disk.Key))
			}
			item.statePath, item.stateSnapshot = path, snapshot
		default:
			doc, ok := tree.WorkNode(item.id)
			if !ok {
				return removalStale(item.id)
			}
			path := filepath.Join(tree.Root, workItemRelDir(doc), store.ShelfStateFile(item.noun))
			disk, err := store.NewWorkNode(item.noun)
			if err != nil {
				return err
			}
			snapshot, err := readRemovalYAML(tree.Root, path, disk)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(*disk.Body(), *doc.Body()) {
				return removalStale(path)
			}
			item.statePath, item.stateSnapshot = path, snapshot
		}
	}
	return nil
}

func planRemovalStateReferences(tree *store.Tree, removed map[string]struct{}, changed map[string]struct{}) ([]store.PlannedWrite, error) {
	var writes []store.PlannedWrite
	epicIDs := sortedMapKeys(tree.Epics)
	for _, id := range epicIDs {
		doc := tree.Epics[id]
		if _, gone := removed[id]; gone {
			continue
		}
		path := filepath.Join(tree.Root, epicRelDir(doc), "epic.yml")
		var disk store.Epic
		snapshot, err := readRemovalYAML(tree.Root, path, &disk)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(disk, *doc) {
			return nil, removalStale(path)
		}
		disk.DependsOn, _ = withoutRemoved(disk.DependsOn, removed)
		disk.Order, _ = withoutRemoved(disk.Order, removed)
		write, err := removalStateWrite(tree.Root, path, snapshot, *doc, disk, changed)
		if err != nil {
			return nil, err
		}
		writes = append(writes, write)
	}
	featureIDs := sortedMapKeys(tree.Features)
	for _, id := range featureIDs {
		doc := tree.Features[id]
		if _, gone := removed[id]; gone {
			continue
		}
		path := filepath.Join(tree.Root, featureRelDir(doc), "feature.yml")
		var disk store.Feature
		snapshot, err := readRemovalYAML(tree.Root, path, &disk)
		if err != nil {
			return nil, err
		}
		expected := *doc
		expected.EpicKey = ""
		expected.HasEval = false
		if !reflect.DeepEqual(disk, expected) {
			return nil, removalStale(path)
		}
		if hasRemovedLineage(disk.Supersedes, disk.SupersededBy, removed) {
			return nil, removalLineageBlocked(store.NounFeature, disk.ID, deref(disk.Key))
		}
		disk.DependsOn, _ = withoutRemoved(disk.DependsOn, removed)
		disk.Order, _ = withoutRemoved(disk.Order, removed)
		write, err := removalStateWrite(tree.Root, path, snapshot, expected, disk, changed)
		if err != nil {
			return nil, err
		}
		writes = append(writes, write)
	}
	taskIDs := sortedMapKeys(tree.Tasks)
	for _, id := range taskIDs {
		doc := tree.Tasks[id]
		if _, gone := removed[id]; gone {
			continue
		}
		parent := tree.Features[doc.FeatureID]
		if parent == nil || parent.Key == nil {
			return nil, fmt.Errorf("mutate: task %s has no addressable parent feature", id)
		}
		path := filepath.Join(tree.Root, store.TaskRelDir(parent.EpicKey, localKey(deref(parent.Key)), localKey(deref(doc.Key))), store.TaskStateFile)
		var disk store.Task
		snapshot, err := readRemovalYAML(tree.Root, path, &disk)
		if err != nil {
			return nil, err
		}
		expected := *doc
		expected.FeatureID = ""
		if !reflect.DeepEqual(disk, expected) {
			return nil, removalStale(path)
		}
		if hasRemovedLineage(disk.Supersedes, disk.SupersededBy, removed) {
			return nil, removalLineageBlocked(store.NounTask, disk.ID, deref(disk.Key))
		}
		disk.DependsOn, _ = withoutRemoved(disk.DependsOn, removed)
		write, err := removalStateWrite(tree.Root, path, snapshot, expected, disk, changed)
		if err != nil {
			return nil, err
		}
		writes = append(writes, write)
	}
	return writes, nil
}

func readRemovalYAML(root, path string, out any) (store.FileSnapshot, error) {
	data, snapshot, err := store.ReadRegularFileSnapshot(root, path)
	if err != nil {
		return store.FileSnapshot{}, err
	}
	if !snapshot.Exists() {
		return store.FileSnapshot{}, removalStale(path)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return store.FileSnapshot{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return snapshot, nil
}

func removalStateWrite(root, path string, snapshot store.FileSnapshot, before, after any, changed map[string]struct{}) (store.PlannedWrite, error) {
	if reflect.DeepEqual(before, after) {
		return store.PlanReadGuard(path, snapshot), nil
	}
	data, err := yaml.Marshal(after)
	if err != nil {
		return store.PlannedWrite{}, err
	}
	changed[relativeSlash(root, path)] = struct{}{}
	return store.PlanWrite(path, snapshot, data), nil
}

func planRemovalQuestions(tree *store.Tree, removed map[string]struct{}, changed map[string]struct{}) (store.PlannedWrite, error) {
	path := filepath.Join(tree.Root, ".tiller", "decisions", "open-questions.yml")
	data, snapshot, err := store.ReadGuardedFileSnapshot(tree.Root, path)
	if err != nil {
		return store.PlannedWrite{}, err
	}
	if !snapshot.Exists() {
		if len(tree.Questions) != 0 {
			return store.PlannedWrite{}, removalStale(path)
		}
		return store.PlanReadGuard(path, snapshot), nil
	}
	var disk store.QuestionsFile
	if err := yaml.Unmarshal(data, &disk); err != nil {
		return store.PlannedWrite{}, err
	}
	if !questionsMatchTree(disk.Questions, tree.Questions) {
		return store.PlannedWrite{}, removalStale(path)
	}
	changedAny := false
	for _, question := range disk.Questions {
		var one bool
		question.Blocks, one = withoutRemoved(question.Blocks, removed)
		changedAny = changedAny || one
	}
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

func planRemovalProjectOrder(tree *store.Tree, removed map[string]struct{}, changed map[string]struct{}) (store.PlannedWrite, error) {
	path := filepath.Join(tree.Root, store.OrderRelPath)
	data, snapshot, err := store.ReadGuardedFileSnapshot(tree.Root, path)
	if err != nil {
		return store.PlannedWrite{}, err
	}
	if !snapshot.Exists() {
		if len(tree.ProjectOrder.Epics) != 0 {
			return store.PlannedWrite{}, removalStale(path)
		}
		return store.PlanReadGuard(path, snapshot), nil
	}
	var disk store.ProjectOrder
	if err := yaml.Unmarshal(data, &disk); err != nil {
		return store.PlannedWrite{}, err
	}
	if !reflect.DeepEqual(disk, tree.ProjectOrder) {
		return store.PlannedWrite{}, removalStale(path)
	}
	var changedAny bool
	disk.Epics, changedAny = withoutRemoved(disk.Epics, removed)
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

func planRemovalAmendments(tree *store.Tree, removed map[string]struct{}) ([]store.PlannedWrite, error) {
	dir := filepath.Join(tree.Root, ".tiller", "amendments")
	inventory, err := store.ReadDirectorySnapshot(tree.Root, dir)
	if err != nil {
		return nil, err
	}
	if !inventory.Exists {
		if len(tree.Amendments) != 0 {
			return nil, removalStale(dir)
		}
		return []store.PlannedWrite{store.PlanDirectoryGuard(dir, inventory)}, nil
	}
	entries, err := readMutationDirectory(tree.Root, dir)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	writes := []store.PlannedWrite{store.PlanDirectoryGuard(dir, inventory)}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		var disk store.Amendment
		snapshot, err := readRemovalYAML(tree.Root, path, &disk)
		if err != nil {
			return nil, err
		}
		expected, ok := tree.Amendments[disk.ID]
		if !ok || !reflect.DeepEqual(disk, *expected) {
			return nil, removalStale(path)
		}
		seen[disk.ID] = struct{}{}
		if disk.Status == store.AmendmentStatusProposed {
			for _, op := range disk.Operations {
				refs := append([]string{op.Target, op.By, op.SupersededBy}, op.Blocks...)
				for _, ref := range refs {
					if removalReferenceMatches(tree, ref, removed) {
						return nil, &RemoveBlockedError{Noun: store.NounAmendment, ID: disk.ID, Key: deref(disk.Key), Status: disk.Status, Code: reasonProposedAmendmentReference, Detail: fmt.Sprintf("proposed operation references removed node %s", ref)}
					}
				}
			}
		}
		writes = append(writes, store.PlanReadGuard(path, snapshot))
	}
	if len(seen) != len(tree.Amendments) {
		return nil, removalStale(dir)
	}
	return writes, nil
}

func planRemovalTree(root, sourceDir string, validatedState map[string]store.FileSnapshot) ([]store.PlannedWrite, []string, error) {
	info, err := mutationPathInfo(root, sourceDir)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, nonRegularRemoveEntry(sourceDir)
	}
	type filePlan struct {
		path     string
		snapshot store.FileSnapshot
	}
	type dirPlan struct {
		path     string
		snapshot store.DirectorySnapshot
	}
	var files []filePlan
	var dirs []dirPlan
	var walk func(string) error
	walk = func(dir string) error {
		snapshot, err := store.ReadDirectorySnapshot(root, dir)
		if err != nil {
			return err
		}
		dirs = append(dirs, dirPlan{path: dir, snapshot: snapshot})
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
				return nonRegularRemoveEntry(path)
			}
			if info.IsDir() {
				if err := walk(path); err != nil {
					return err
				}
				continue
			}
			if !info.Mode().IsRegular() {
				return nonRegularRemoveEntry(path)
			}
			snapshot, validated := validatedState[path]
			if !validated {
				_, snapshot, err = store.ReadRegularFileSnapshot(root, path)
				if err != nil {
					return err
				}
			}
			files = append(files, filePlan{path: path, snapshot: snapshot})
		}
		return nil
	}
	if err := walk(sourceDir); err != nil {
		return nil, nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	sort.Slice(dirs, func(i, j int) bool {
		left, right := strings.Count(dirs[i].path, string(filepath.Separator)), strings.Count(dirs[j].path, string(filepath.Separator))
		if left == right {
			return dirs[i].path > dirs[j].path
		}
		return left > right
	})
	writes := make([]store.PlannedWrite, 0, len(files)+len(dirs))
	deleted := make([]string, 0, len(files))
	for _, file := range files {
		writes = append(writes, store.PlanDeleteFile(file.path, file.snapshot))
		deleted = append(deleted, relativeSlash(root, file.path))
	}
	for _, dir := range dirs {
		writes = append(writes, store.PlanRemoveEmptyDir(dir.path, dir.snapshot))
	}
	return writes, deleted, nil
}

func nonRegularRemoveEntry(path string) error {
	return &RemoveBlockedError{Code: reasonNonRegularRemoveEntry, Detail: fmt.Sprintf("removal permits only regular files and directories; %s is not regular", path)}
}
func removalStale(path string) error {
	return &store.EnvError{Reason: store.ReasonMutationStale, Msg: fmt.Sprintf("mutate: %s changed after the tree was loaded; reload and retry", path)}
}
func removalLineageBlocked(noun, id, key string) error {
	return &RemoveBlockedError{Noun: noun, ID: id, Key: key, Code: reasonRemoveLineageReference, Detail: "a surviving supersession edge references a node being removed"}
}

func withoutRemoved(values []string, removed map[string]struct{}) ([]string, bool) {
	out := make([]string, 0, len(values))
	changed := false
	for _, value := range values {
		if _, gone := removed[value]; gone {
			changed = true
			continue
		}
		out = append(out, value)
	}
	if !changed {
		return values, false
	}
	return out, true
}
func hasRemovedLineage(supersedes []string, supersededBy *string, removed map[string]struct{}) bool {
	for _, id := range supersedes {
		if _, gone := removed[id]; gone {
			return true
		}
	}
	if supersededBy != nil {
		_, gone := removed[*supersededBy]
		return gone
	}
	return false
}
func questionsMatchTree(questions []*store.Question, expected map[string]*store.Question) bool {
	if len(questions) != len(expected) {
		return false
	}
	for _, question := range questions {
		treeQuestion, ok := expected[question.ID]
		if !ok || !reflect.DeepEqual(*question, *treeQuestion) {
			return false
		}
	}
	return true
}

func removalReferenceMatches(tree *store.Tree, raw string, removed map[string]struct{}) bool {
	if _, matched := removed[raw]; matched {
		return true
	}
	for id := range removed {
		var node store.Node
		if tree.Epics[id] != nil {
			node = tree.Epics[id]
		}
		if tree.Features[id] != nil {
			node = tree.Features[id]
		}
		if tree.Tasks[id] != nil {
			node = tree.Tasks[id]
		}
		if node != nil && node.NodeKey() != nil && *node.NodeKey() == raw {
			return true
		}
	}
	noun := store.NounEpic
	switch strings.Count(raw, "/") {
	case 1:
		noun = store.NounFeature
	case 2:
		noun = store.NounTask
	}
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: noun, Raw: raw})
	if err != nil {
		return false
	}
	_, matched := removed[node.NodeID()]
	return matched
}
func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func sortedSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
func relativeSlash(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
