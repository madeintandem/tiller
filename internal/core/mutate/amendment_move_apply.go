package mutate

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

func moveStale(index int, op store.AmendmentOp, err error) *StalePrecondition {
	return &StalePrecondition{Index: index, Op: op.Op, Target: op.Target, Precondition: "move", Expected: "reviewed move snapshot", Actual: "changed", Note: err.Error()}
}

func evaluateFeatureMove(tree *store.Tree, doc *store.Amendment, index int) error {
	op := doc.Operations[index]
	current, err := captureFeatureMove(tree, doc.Operations[:index], op)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, op.Move) {
		return moveRefusal("source subtree, accepted bases, claim, parent policy or reference inventory changed; restage the move")
	}
	return nil
}

func checkFeatureMoves(tree *store.Tree, b *batch, doc *store.Amendment) error {
	hasMove := false
	for _, op := range doc.Operations {
		if op.Op == store.AmendmentOpMoveFeature {
			hasMove = true
			break
		}
	}
	if !hasMove {
		return nil
	}
	guards, err := captureMoveReadGuards(tree.Root)
	if err != nil {
		return err
	}
	b.moveGuards = append(b.moveGuards, guards...)
	fresh, err := store.Load(tree.Root)
	if err != nil {
		return err
	}
	if err := checkMoveConflicts(fresh, doc); err != nil {
		return err
	}
	var stale []StalePrecondition
	candidate := fresh
	for index, op := range doc.Operations {
		if op.Op != store.AmendmentOpMoveFeature {
			continue
		}
		if err := evaluateFeatureMove(fresh, doc, index); err != nil {
			stale = append(stale, *moveStale(index, op, err))
			continue
		}
		dest, _, err := moveDestination(candidate, doc.Operations[:index], op.Key)
		if err != nil {
			return err
		}
		candidate = moveCandidate(candidate, candidate.Features[op.Target], dest, op.Key)
	}
	if len(stale) > 0 {
		return &AmendmentPreconditionsStaleError{AmendmentID: doc.ID, Stale: stale}
	}
	if cycles := gen.ClassifyCycles(candidate).Blocking; len(cycles) > 0 {
		return moveRefusal(fmt.Sprintf("resulting dependency graph contains execution cycles: %v", cycles))
	}
	return nil
}

func (b *batch) moveEpicEdit(tree *store.Tree, e *store.Epic) (*nodeEdit, error) {
	rel := filepath.Join(epicRelDir(e), "epic.yml")
	if edit := b.edits[rel]; edit != nil {
		return edit, nil
	}
	var doc store.Epic
	before, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, rel), &doc)
	if err != nil {
		return nil, err
	}
	if doc.ID != e.ID || deref(doc.Key) != deref(e.Key) {
		return nil, moveRefusal("parent identity changed")
	}
	return b.register(&nodeEdit{noun: store.NounEpic, rel: rel, doc: &doc, before: before}), nil
}

func (b *batch) moveFeatures(tree *store.Tree, deps Deps, owner store.Contributor, doc *store.Amendment, plan []resolvedOp, applied []AppliedOperation) error {
	if err := b.remapMoveLocators(tree, doc); err != nil {
		return err
	}
	refs := map[string]string{}
	for _, op := range doc.Operations {
		if op.Op == store.AmendmentOpMoveFeature {
			for key, id := range moveIdentityLocators(tree, tree.Features[op.Target]) {
				refs[key] = id
			}
		}
	}
	for _, r := range plan {
		if r.op.Op != store.AmendmentOpMoveFeature {
			continue
		}
		op := r.op
		f := tree.Features[op.Target]
		source, err := resolveEpicForRename(tree, f.EpicKey)
		if err != nil {
			return err
		}
		destination, staged, err := moveDestination(tree, doc.Operations[:r.index], op.Key)
		if err != nil {
			return err
		}
		var destinationEdit *nodeEdit
		if staged {
			destinationEdit = b.staged[deref(destination.Key)]
			if destinationEdit == nil {
				return moveRefusal("staged destination was not materialized")
			}
			destination = destinationEdit.doc.(*store.Epic)
		}
		if source.ID != destination.ID {
			sourceEdit, err := b.moveEpicEdit(tree, source)
			if err != nil {
				return err
			}
			if destinationEdit == nil {
				destinationEdit, err = b.moveEpicEdit(tree, destination)
				if err != nil {
					return err
				}
			}
			src := sourceEdit.doc.(*store.Epic)
			dst := destinationEdit.doc.(*store.Epic)
			order := make([]string, 0, len(src.Order))
			for _, id := range src.Order {
				if id != f.ID {
					order = append(order, id)
				}
			}
			src.Order = order
			dst.Order = append(dst.Order, f.ID)
			b.dirty[sourceEdit.rel] = true
			b.dirty[destinationEdit.rel] = true
		}
		sourceDir := filepath.Join(tree.Root, featureRelDir(f))
		destinationDir := filepath.Join(tree.Root, ".tiller", "epics", deref(destination.Key), "features", localKey(op.Key))
		if err := requireAbsentRenameDestination(tree.Root, destinationDir, store.NounFeature, op.Key); err != nil {
			return err
		}
		hashes, err := moveFileHashes(tree.Root, sourceDir)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(hashes, op.Move.Files) {
			return moveRefusal("subtree changed while planning approval")
		}
		writes, moved, err := planRenameTreeMove(tree.Root, sourceDir, destinationDir, func(relative string, data []byte) ([]byte, error) {
			if relative == "feature.yml" {
				var feature store.Feature
				if err := yaml.Unmarshal(data, &feature); err != nil {
					return nil, err
				}
				if feature.ID != f.ID {
					return nil, moveRefusal("source identity changed")
				}
				remapMoveNode(&feature, refs, true)
				feature.Key = &op.Key
				if op.Move.MaterializeReviewGrain {
					feature.ReviewGrain = op.Move.ReviewGrain
				}
				return yaml.Marshal(feature)
			}
			if filepath.Base(relative) == store.TaskStateFile && strings.HasPrefix(relative, store.TaskDirName+string(filepath.Separator)) {
				var task store.Task
				if err := yaml.Unmarshal(data, &task); err != nil {
					return nil, err
				}
				remapMoveNode(&task, refs, true)
				key := op.Key + "/" + localKey(deref(task.Key))
				task.Key = &key
				return yaml.Marshal(task)
			}
			return data, nil
		})
		if err != nil {
			return err
		}
		b.prose = append(b.prose, writes...)
		b.movedDirs = append(b.movedDirs, sourceDir)
		if err := b.remapMoveQuestions(tree, op.Move.SourceKey, op.Key); err != nil {
			return err
		}
		ev := amendmentOpEvent(deps, owner, doc, "move.feature", []string{f.ID, source.ID, destination.ID}, moved...)
		ev.Before = map[string]any{"key": op.Move.SourceKey, "epic": source.ID}
		ev.After["key"] = op.Key
		ev.After["epic"] = destination.ID
		ev.After["reason"] = op.Reason
		ev.After["operation_index"] = r.index
		beforeTasks, afterTasks := map[string]string{}, map[string]string{}
		for _, task := range tree.FeatureTasks[f.ID] {
			beforeTasks[task.ID] = deref(task.Key)
			afterTasks[task.ID] = op.Key + "/" + localKey(deref(task.Key))
		}
		ev.Before["task_keys"] = beforeTasks
		ev.After["task_keys"] = afterTasks
		b.events = append(b.events, ev)
		applied[r.index] = AppliedOperation{Index: r.index, Op: op.Op, Noun: store.NounFeature, NodeID: f.ID, Key: op.Key, Status: f.Status, Move: op.Move}
	}
	return nil
}

func (b *batch) movedPath(path string) bool {
	for _, dir := range b.movedDirs {
		if path == dir || strings.HasPrefix(path, dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func appendUniqueMoveGuards(writes, guards []store.PlannedWrite) ([]store.PlannedWrite, error) {
	seen := make(map[string]int, len(writes))
	for i, write := range writes {
		seen[write.Path] = i
	}
	for _, guard := range guards {
		if index, exists := seen[guard.Path]; exists {
			constrained, err := writes[index].ConstrainByReadGuard(guard)
			if err != nil {
				return nil, err
			}
			writes[index] = constrained
		} else {
			seen[guard.Path] = len(writes)
			writes = append(writes, guard)
		}
	}
	return writes, nil
}

func (b *batch) remapMoveQuestions(tree *store.Tree, oldKey, newKey string) error {
	needed := false
	for _, q := range tree.Questions {
		for _, block := range q.Blocks {
			if block == oldKey || strings.HasPrefix(block, oldKey+"/") {
				needed = true
			}
		}
	}
	if !needed {
		return nil
	}
	questions, err := b.loadQuestions(tree)
	if err != nil {
		return err
	}
	for _, q := range questions.Questions {
		if remapMoveBlocks(q.Blocks, oldKey, newKey) {
			b.questionsDirty = true
		}
	}
	return nil
}
