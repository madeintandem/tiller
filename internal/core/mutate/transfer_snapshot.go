package mutate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// The transfer read set includes every execution state and both graph
// inventories: a new dependent or carrier must not slip between planning and
// commit. Documents are private copies; a refusal cannot mutate the caller's tree.
type transferPlan struct {
	tree           *store.Tree
	guards         []store.PlannedWrite
	snapshots      map[string]store.FileSnapshot
	updates        map[string][]byte
	changed        []string
	questions      store.QuestionsFile
	originalStatus map[string]string
}

func loadTransferSnapshot(tree *store.Tree) (*transferPlan, error) {
	fresh := *tree
	fresh.Epics = map[string]*store.Epic{}
	fresh.Features = map[string]*store.Feature{}
	fresh.Tasks = map[string]*store.Task{}
	fresh.FeatureTasks = map[string][]*store.Task{}
	fresh.Bugs = map[string]*store.Bug{}
	fresh.Chores = map[string]*store.Chore{}
	fresh.Tweaks = map[string]*store.Tweak{}
	p := &transferPlan{tree: &fresh, snapshots: map[string]store.FileSnapshot{}, updates: map[string][]byte{}, originalStatus: map[string]string{}}
	var manifest store.Manifest
	if err := p.load(filepath.Join(".tiller", "tiller.yml"), &manifest, tree.Manifest); err != nil {
		return nil, err
	}
	fresh.Manifest = &manifest
	epicsDir := filepath.Join(tree.Root, ".tiller", "epics")
	inventory, err := freshRenameLineage(tree, epicsDir)
	if err != nil {
		return nil, err
	}
	p.guards = append(p.guards, store.PlanLineageInventoryGuard(epicsDir, inventory))
	for _, nid := range sortedMapKeys(tree.Epics) {
		node := tree.Epics[nid]
		var doc store.Epic
		if err := p.load(filepath.Join(epicRelDir(node), "epic.yml"), &doc, node); err != nil {
			return nil, err
		}
		fresh.Epics[nid] = &doc
	}
	for _, nid := range sortedMapKeys(tree.Features) {
		node := tree.Features[nid]
		var doc store.Feature
		if err := p.load(filepath.Join(featureRelDir(node), "feature.yml"), &doc, node); err != nil {
			return nil, err
		}
		doc.EpicKey, doc.HasEval = node.EpicKey, node.HasEval
		fresh.Features[nid] = &doc
	}
	for _, nid := range sortedMapKeys(tree.Tasks) {
		node := tree.Tasks[nid]
		parent := fresh.Features[node.FeatureID]
		if parent == nil || parent.Key == nil || node.Key == nil {
			return nil, fmt.Errorf("transfer: task %s has no addressable parent or key", nid)
		}
		var doc store.Task
		rel := filepath.Join(store.TaskRelDir(parent.EpicKey, localKey(deref(parent.Key)), localKey(deref(node.Key))), store.TaskStateFile)
		if err := p.load(rel, &doc, node); err != nil {
			return nil, err
		}
		doc.FeatureID = node.FeatureID
		fresh.Tasks[nid] = &doc
		fresh.FeatureTasks[doc.FeatureID] = append(fresh.FeatureTasks[doc.FeatureID], &doc)
		p.originalStatus[nid] = doc.Status
	}
	for _, noun := range store.ShelfNouns() {
		dir := filepath.Join(tree.Root, ".tiller", store.ShelfDir(noun))
		snapshot, err := store.ReadDirectorySnapshot(tree.Root, dir)
		if err != nil {
			return nil, err
		}
		p.guards = append(p.guards, store.PlanDirectoryGuard(dir, snapshot))
		entries, err := readMutationDirectory(tree.Root, dir)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		var actual, expected []string
		for _, entry := range entries {
			if entry.IsDir() {
				actual = append(actual, entry.Name())
			}
		}
		for _, node := range tree.WorkNodesOf(noun) {
			expected = append(expected, localKey(deref(node.NodeKey())))
			doc, err := store.NewWorkNode(noun)
			if err != nil {
				return nil, err
			}
			if err := p.load(filepath.Join(workItemRelDir(node), store.ShelfStateFile(noun)), doc, node); err != nil {
				return nil, err
			}
			switch w := doc.(type) {
			case *store.Bug:
				fresh.Bugs[w.ID] = w
			case *store.Chore:
				fresh.Chores[w.ID] = w
			case *store.Tweak:
				fresh.Tweaks[w.ID] = w
			}
			p.originalStatus[doc.NodeID()] = doc.NodeStatus()
		}
		sort.Strings(actual)
		sort.Strings(expected)
		if !reflect.DeepEqual(actual, expected) {
			return nil, removalStale(dir)
		}
	}
	qrel := filepath.FromSlash(questionsRelPath)
	qraw, err := p.read(qrel)
	if err != nil {
		return nil, err
	}
	if len(qraw) != 0 {
		if err := yaml.Unmarshal(qraw, &p.questions); err != nil {
			return nil, err
		}
	}
	if len(p.questions.Questions) != len(tree.Questions) {
		return nil, removalStale(qrel)
	}
	for _, q := range p.questions.Questions {
		if !reflect.DeepEqual(q, tree.Questions[q.ID]) {
			return nil, removalStale(qrel)
		}
	}
	return p, nil
}

func (p *transferPlan) read(rel string) ([]byte, error) {
	path := filepath.Join(p.tree.Root, rel)
	data, snapshot, err := store.ReadRegularFileSnapshot(p.tree.Root, path)
	if err != nil {
		return nil, err
	}
	p.snapshots[rel] = snapshot
	return data, nil
}

func (p *transferPlan) load(rel string, into, expected any) error {
	raw, err := p.read(rel)
	if err != nil {
		return err
	}
	if !p.snapshots[rel].Exists() {
		return removalStale(rel)
	}
	if err := yaml.Unmarshal(raw, into); err != nil {
		return err
	}
	// YAML excludes loader-derived fields (FeatureID, EpicKey, contributor
	// projections), unlike reflection over the structs themselves.
	before, err := yaml.Marshal(expected)
	if err != nil {
		return err
	}
	after, err := yaml.Marshal(into)
	if err != nil {
		return err
	}
	if !bytes.Equal(before, after) {
		return removalStale(rel)
	}
	return nil
}

func (p *transferPlan) put(rel string, value any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	p.create(rel, data)
	return nil
}

func (p *transferPlan) create(rel string, data []byte) {
	if _, exists := p.updates[rel]; !exists {
		p.changed = append(p.changed, rel)
	}
	p.updates[rel] = data
}

func (p *transferPlan) writes() []store.PlannedWrite {
	writes := append([]store.PlannedWrite(nil), p.guards...)
	for _, rel := range sortedMapKeys(p.snapshots) {
		if _, changed := p.updates[rel]; !changed {
			writes = append(writes, store.PlanReadGuard(filepath.Join(p.tree.Root, rel), p.snapshots[rel]))
		}
	}
	for _, rel := range p.changed {
		writes = append(writes, store.PlanWrite(filepath.Join(p.tree.Root, rel), p.snapshots[rel], p.updates[rel]))
	}
	return writes
}

func (p *transferPlan) successorKey(n store.Node, oldDir string) (string, string, error) {
	key := deref(n.NodeKey())
	if key == "" || !renameKeySegmentRE.MatchString(localKey(key)) {
		return "", "", fmt.Errorf("transfer: %s %s has an invalid key", n.Noun(), n.NodeID())
	}
	for sequence := 2; ; sequence++ {
		local := fmt.Sprintf("%s-transfer-%d", localKey(key), sequence)
		dir := filepath.Join(filepath.Dir(oldDir), local)
		exists, err := store.PathExistsNoSymlinks(p.tree.Root, filepath.Join(p.tree.Root, dir))
		if err != nil {
			return "", "", err
		}
		if exists {
			continue
		}
		inventory, err := store.ReadDirectorySnapshot(p.tree.Root, filepath.Join(p.tree.Root, dir))
		if err != nil {
			return "", "", err
		}
		p.guards = append(p.guards, store.PlanDirectoryGuard(filepath.Join(p.tree.Root, dir), inventory))
		return filepath.ToSlash(filepath.Join(filepath.Dir(key), local)), dir, nil
	}
}
