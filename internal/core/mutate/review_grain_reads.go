package mutate

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// freshReviewGrainLineage reads the feature/task directory inventory and every
// state file in it. The returned guards pin both the set and the bytes under
// the mutation lock. override is the target feature a caller already read with
// the snapshot it will write; that document replaces the second read and gets
// no duplicate read guard.
func freshReviewGrainLineage(tree *store.Tree, override *store.Feature) (*store.Tree, []store.PlannedWrite, error) {
	epicsDir := filepath.Join(tree.Root, ".tiller", "epics")
	inventory, err := store.ReadLineageInventory(tree.Root, epicsDir)
	if err != nil {
		return nil, nil, err
	}
	guards := []store.PlannedWrite{store.PlanLineageInventoryGuard(epicsDir, inventory)}

	fresh := *tree
	fresh.Features = make(map[string]*store.Feature)
	fresh.Tasks = make(map[string]*store.Task)
	fresh.FeatureTasks = make(map[string][]*store.Task)
	featureByDir := make(map[string]*store.Feature)

	for _, file := range inventory.Files {
		if file.Noun != store.NounFeature {
			continue
		}
		parts := splitReviewGrainInventoryPath(file.Path)
		if len(parts) != 4 || parts[1] != "features" || parts[3] != "feature.yml" {
			return nil, nil, fmt.Errorf("mutate: unexpected feature inventory path %q", file.Path)
		}
		rel := filepath.Join(".tiller", "epics", file.Path)
		path := filepath.Join(tree.Root, rel)
		var doc store.Feature
		snapshot, err := readYAMLSnapshot(tree.Root, path, &doc)
		if err != nil {
			return nil, nil, err
		}
		doc.EpicKey = parts[0]
		if override != nil && doc.ID == override.ID {
			copy := *override
			copy.EpicKey = parts[0]
			doc = copy
		} else {
			guards = append(guards, store.PlanReadGuard(path, snapshot))
		}
		fresh.Features[doc.ID] = &doc
		fresh.FeatureTasks[doc.ID] = nil
		featureByDir[filepath.Join(parts[0], parts[2])] = &doc
	}

	for _, file := range inventory.Files {
		if file.Noun != store.NounTask {
			continue
		}
		parts := splitReviewGrainInventoryPath(file.Path)
		if len(parts) != 6 || parts[1] != "features" || parts[3] != store.TaskDirName || parts[5] != store.TaskStateFile {
			return nil, nil, fmt.Errorf("mutate: unexpected task inventory path %q", file.Path)
		}
		feature := featureByDir[filepath.Join(parts[0], parts[2])]
		if feature == nil {
			return nil, nil, fmt.Errorf("mutate: task inventory path %q has no feature state", file.Path)
		}
		path := filepath.Join(epicsDir, file.Path)
		var doc store.Task
		snapshot, err := readYAMLSnapshot(tree.Root, path, &doc)
		if err != nil {
			return nil, nil, err
		}
		doc.FeatureID = feature.ID
		fresh.Tasks[doc.ID] = &doc
		fresh.FeatureTasks[feature.ID] = append(fresh.FeatureTasks[feature.ID], &doc)
		guards = append(guards, store.PlanReadGuard(path, snapshot))
	}
	return &fresh, guards, nil
}

func freshReviewGrainState(tree *store.Tree, change reviewGrainChange) (*store.Tree, reviewGrainChange, []store.PlannedWrite, error) {
	var featureOverride *store.Feature
	if change.Noun == store.NounFeature {
		featureOverride = change.Feature
	}
	fresh, guards, err := freshReviewGrainLineage(tree, featureOverride)
	if err != nil {
		return nil, reviewGrainChange{}, nil, err
	}
	epicsDir := filepath.Join(tree.Root, ".tiller", "epics")
	entries, err := readMutationDirectory(tree.Root, epicsDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, reviewGrainChange{}, nil, err
	}
	fresh.Epics = make(map[string]*store.Epic)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(epicsDir, entry.Name(), "epic.yml")
		var doc store.Epic
		snapshot, err := readYAMLSnapshot(tree.Root, path, &doc)
		if err != nil {
			return nil, reviewGrainChange{}, nil, err
		}
		if change.Noun == store.NounEpic && change.Epic != nil && doc.ID == change.Epic.ID {
			doc = *change.Epic
		} else {
			guards = append(guards, store.PlanReadGuard(path, snapshot))
		}
		fresh.Epics[doc.ID] = &doc
	}
	if change.Epic != nil {
		change.Epic = fresh.Epics[change.Epic.ID]
	}
	if change.Feature != nil {
		change.Feature = fresh.Features[change.Feature.ID]
	}
	return fresh, change, guards, nil
}

func splitReviewGrainInventoryPath(path string) []string {
	return strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
}
