package mutate

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

func moveRefusal(detail string) error {
	return &usageError{reason: "feature_move_refused", msg: "cannot move feature: " + detail}
}

func stageFeatureMove(tree *store.Tree, doc *store.Amendment, in StageInput) (store.AmendmentOp, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return store.AmendmentOp{}, moveRefusal("a reason is required")
	}
	if in.By != "" || in.SupersededBy != "" || in.Title != "" || in.ChangeType != "" {
		return store.AmendmentOp{}, moveRefusal("move-feature only accepts target, destination, reason, source and preconditions")
	}
	if err := validateRenameKey(store.NounFeature, in.Key); err != nil {
		return store.AmendmentOp{}, err
	}
	fresh, err := store.Load(tree.Root)
	if err != nil {
		return store.AmendmentOp{}, err
	}
	node, err := resolve.NewResolver(fresh).Resolve(resolve.Ref{Noun: store.NounFeature, Raw: in.Target})
	if err != nil {
		return store.AmendmentOp{}, err
	}
	op := store.AmendmentOp{Op: store.AmendmentOpMoveFeature, Target: node.NodeID(), Key: in.Key, Reason: in.Reason, Source: in.Source, Preconditions: in.Preconditions}
	snapshot, err := captureFeatureMove(fresh, doc.Operations, op)
	if err != nil {
		return store.AmendmentOp{}, err
	}
	op.Move = snapshot
	return op, nil
}

func moveDestination(tree *store.Tree, earlier []store.AmendmentOp, key string) (*store.Epic, bool, error) {
	epicKey, _, err := splitFeatureKey(key)
	if err != nil {
		return nil, false, err
	}
	for _, e := range tree.Epics {
		if deref(e.Key) == epicKey {
			return e, false, nil
		}
	}
	for _, op := range earlier {
		if op.Op == store.AmendmentOpAddEpic && op.Key == epicKey {
			return &store.Epic{Key: &epicKey, Status: statusDraft}, true, nil
		}
	}
	return nil, false, moveRefusal("destination epic " + epicKey + " must exist or be staged earlier")
}

func captureFeatureMove(tree *store.Tree, earlier []store.AmendmentOp, op store.AmendmentOp) (*store.FeatureMovePreconditions, error) {
	if err := validateRenameKey(store.NounFeature, op.Key); err != nil {
		return nil, err
	}
	feature := tree.Features[op.Target]
	if feature == nil {
		return nil, moveRefusal("target must be an existing feature ID")
	}
	if deref(feature.Key) == op.Key {
		return nil, moveRefusal("destination is already the feature's canonical address")
	}
	source, err := resolveEpicForRename(tree, feature.EpicKey)
	if err != nil {
		return nil, err
	}
	dest, staged, err := moveDestination(tree, earlier, op.Key)
	if err != nil {
		return nil, err
	}
	for _, e := range []*store.Epic{source, dest} {
		if e.Status != statusDraft && e.Status != "ready" {
			return nil, moveRefusal(fmt.Sprintf("parent %s is %s; parents must be draft or ready", deref(e.Key), e.Status))
		}
		if e.PR != nil || e.ReviewRef != nil {
			return nil, moveRefusal("parent " + deref(e.Key) + " has an open aggregate review")
		}
	}
	if feature.Status != statusDraft && feature.Status != "specced" && feature.Status != "ready" {
		return nil, moveRefusal("feature " + feature.ID + " is " + feature.Status)
	}
	if feature.Branch != "" || feature.PR != nil || feature.ReviewRef != nil || feature.DoneBasis != "" || !reflect.DeepEqual(feature.VerifiedAgainst, store.VerifiedAgainst{}) || moveGateHistory(feature.GatesBlock) {
		return nil, moveRefusal("feature " + feature.ID + " has execution or review history")
	}
	ids := map[string]bool{feature.ID: true}
	for _, task := range tree.FeatureTasks[feature.ID] {
		ids[task.ID] = true
		if (task.Status != statusProposed && task.Status != "ready") || task.Started || task.Branch != "" || task.PR != nil || task.ReviewRef != nil || task.DoneBasis != "" || len(task.Evidence) > 0 || moveGateHistory(task.GatesBlock) {
			return nil, moveRefusal("task " + task.ID + " has started or carries execution/review history")
		}
	}
	history, err := events.List(tree)
	if err != nil {
		return nil, err
	}
	for _, ev := range history {
		for _, id := range ev.Targets {
			if !ids[id] {
				continue
			}
			if moveExecutionEvent(ev) {
				return nil, moveRefusal(id + " has recorded execution history in " + ev.ID)
			}
		}
	}
	destinationDir := filepath.Join(tree.Root, ".tiller", "epics", deref(dest.Key), "features", localKey(op.Key))
	if err := requireAbsentRenameDestination(tree.Root, destinationDir, store.NounFeature, op.Key); err != nil {
		return nil, err
	}
	before := moveConstraints(tree, feature, source)
	candidate := moveCandidate(tree, feature, dest, op.Key)
	after := moveConstraints(candidate, candidate.Features[feature.ID], dest)
	if !reflect.DeepEqual(before, after) {
		return nil, moveRefusal(fmt.Sprintf("effective prerequisites or question holds would change: before %v; after %v; correct those constraints before restaging", before, after))
	}
	if cycles := gen.ClassifyCycles(candidate).Blocking; len(cycles) > 0 {
		return nil, moveRefusal(fmt.Sprintf("resulting dependency graph contains execution cycles: %v", cycles))
	}
	files, err := moveFileHashes(tree.Root, filepath.Join(tree.Root, featureRelDir(feature)))
	if err != nil {
		return nil, err
	}
	grain := store.ResolvedReviewGrain(tree.Manifest, source, feature)
	policies := map[string]string{"source_epic": moveHashValue(source), "destination_epic": moveHashValue(dest), "execution": moveHashValue(tree.Manifest.Execution), "constraints": moveHashValue(before)}
	result := &store.FeatureMovePreconditions{SourceKey: deref(feature.Key), SourceEpicID: source.ID, DestinationEpicID: dest.ID, DestinationEpicKey: deref(dest.Key), DestinationStaged: staged, ReviewGrain: grain, MaterializeReviewGrain: grain != store.ResolvedReviewGrain(tree.Manifest, dest, feature), OrderEffect: "append", Files: files, Policies: policies}
	if source.ID == dest.ID && !staged {
		result.OrderEffect = "preserve-position"
	}
	result.ReferenceWarnings, err = moveReferenceWarnings(tree, feature, op.Key)
	return result, err
}

func moveGateHistory(g store.GatesBlock) bool {
	if len(g.Attestations) > 0 || len(g.PolicyViolations) > 0 {
		return true
	}
	for _, gate := range g.Gates {
		if !gate.RunAt.IsZero() || gate.RunBy != "" || gate.Against.Value != "" || len(gate.Evidence) > 0 || len(gate.Criteria) > 0 || (gate.Status != "" && gate.Status != store.GateStatusPending) {
			return true
		}
	}
	return false
}

func moveExecutionEvent(ev events.Event) bool {
	verb, _, _ := strings.Cut(ev.Type, ".")
	switch verb {
	case "start", "check", "submit", "resume", "attest", "eval", "done":
		return true
	}
	for _, values := range []map[string]any{ev.Before, ev.After} {
		if status, ok := values["status"].(string); ok {
			switch status {
			case "active", "in_progress", "check", "in_review", "done", "cancelled", "archived":
				return true
			}
		}
	}
	return false
}

func moveHashValue(value any) string {
	data, _ := yaml.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func moveFileHashes(root, dir string) (map[string]string, error) {
	hashes := map[string]string{}
	err := walkMutationState(root, dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!entry.IsDir() && !info.Mode().IsRegular()) {
			return nonRegularMoveEntry(path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			hashes[filepath.ToSlash(rel)+"/"] = fmt.Sprintf("mode:%o", info.Mode().Perm())
			return nil
		}
		data, _, err := store.ReadRegularFileSnapshot(root, path)
		if err != nil {
			return err
		}
		hashes[filepath.ToSlash(rel)] = fmt.Sprintf("%o:%x", info.Mode().Perm(), sha256.Sum256(data))
		return nil
	})
	return hashes, err
}

func moveCandidate(tree *store.Tree, feature *store.Feature, dest *store.Epic, key string) *store.Tree {
	out := *tree
	refs := moveIdentityLocators(tree, feature)
	out.Epics = make(map[string]*store.Epic, len(tree.Epics)+1)
	for id, e := range tree.Epics {
		out.Epics[id] = e
		if deps, changed := moveDependencyRefs(e.DependsOn, refs); changed {
			copy := *e
			copy.DependsOn = deps
			out.Epics[id] = &copy
		}
	}
	if dest.ID == "" {
		copy := *dest
		copy.ID = "staged-epic:" + deref(dest.Key)
		dest = &copy
	}
	out.Epics[dest.ID] = dest
	out.Features = make(map[string]*store.Feature, len(tree.Features))
	for id, f := range tree.Features {
		out.Features[id] = f
		if deps, changed := moveDependencyRefs(f.DependsOn, refs); changed {
			copy := *f
			copy.DependsOn = deps
			out.Features[id] = &copy
		}
	}
	f := *out.Features[feature.ID]
	f.Key = &key
	f.EpicKey = deref(dest.Key)
	out.Features[f.ID] = &f
	out.Tasks = make(map[string]*store.Task, len(tree.Tasks))
	for id, t := range tree.Tasks {
		out.Tasks[id] = t
		if deps, changed := moveDependencyRefs(t.DependsOn, refs); changed {
			copy := *t
			copy.DependsOn = deps
			out.Tasks[id] = &copy
		}
	}
	out.Questions = make(map[string]*store.Question, len(tree.Questions))
	for id, q := range tree.Questions {
		copy := *q
		copy.Blocks = append([]string(nil), q.Blocks...)
		remapMoveBlocks(copy.Blocks, deref(feature.Key), key)
		out.Questions[id] = &copy
	}
	out.FeatureTasks = make(map[string][]*store.Task, len(tree.FeatureTasks))
	for id, tasks := range tree.FeatureTasks {
		out.FeatureTasks[id] = tasks
	}
	out.FeatureTasks[f.ID] = nil
	for _, task := range tree.FeatureTasks[f.ID] {
		copy := *out.Tasks[task.ID]
		k := key + "/" + localKey(deref(task.Key))
		copy.Key = &k
		out.Tasks[copy.ID] = &copy
		out.FeatureTasks[f.ID] = append(out.FeatureTasks[f.ID], &copy)
	}
	return &out
}

func moveConstraints(tree *store.Tree, f *store.Feature, e *store.Epic) []string {
	set := map[string]bool{}
	canonical := func(raw string) string {
		for _, noun := range []string{store.NounEpic, store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak} {
			node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: noun, Raw: raw})
			if err == nil {
				return node.NodeID()
			}
		}
		return raw
	}
	for _, id := range append(append([]string{}, e.DependsOn...), f.DependsOn...) {
		set["dependency:"+canonical(id)] = true
	}
	for _, q := range gen.FeatureQuestionBlockers(tree, f) {
		set["feature-question:"+q.Question] = true
	}
	for _, task := range tree.FeatureTasks[f.ID] {
		for _, id := range task.DependsOn {
			set[task.ID+":dependency:"+canonical(id)] = true
		}
		for _, q := range gen.TaskQuestionBlockers(tree, f, task) {
			set[task.ID+":question:"+q.Question] = true
		}
	}
	if e.Blocked {
		set["parent-blocked"] = true
	}
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func checkMoveConflicts(tree *store.Tree, doc *store.Amendment) error {
	for i, move := range doc.Operations {
		if move.Op != store.AmendmentOpMoveFeature {
			continue
		}
		feature := tree.Features[move.Target]
		if feature == nil {
			return moveRefusal("unknown source " + move.Target)
		}
		for j, op := range doc.Operations {
			if i == j {
				continue
			}
			if err := moveOperationConflict(tree, feature, move, op); err != nil {
				return err
			}
		}
		for _, pending := range tree.Amendments {
			if pending.ID == doc.ID || pending.Status != store.AmendmentStatusProposed {
				continue
			}
			for _, op := range pending.Operations {
				if err := moveOperationConflict(tree, feature, move, op); err != nil {
					return moveRefusal("conflicting pending amendment " + pending.ID + ": " + err.Error())
				}
			}
		}
	}
	return nil
}

func moveOperationConflict(tree *store.Tree, feature *store.Feature, move, op store.AmendmentOp) error {
	subtree := func(raw string) bool {
		if raw == feature.ID || raw == deref(feature.Key) || strings.HasPrefix(raw, deref(feature.Key)+"/") {
			return true
		}
		t := tree.Tasks[raw]
		return t != nil && t.FeatureID == feature.ID
	}
	if op.Op == store.AmendmentOpMoveFeature && (op.Target == move.Target || op.Key == move.Key) {
		return moveRefusal("duplicate or overlapping move")
	}
	if subtree(op.Target) || subtree(op.By) || subtree(op.SupersededBy) || subtree(op.Key) || op.Key == move.Key || strings.HasPrefix(op.Key, move.Key+"/") {
		return moveRefusal("operation " + op.Op + " conflicts with moved subtree " + feature.ID)
	}
	destinationEpic, _, _ := strings.Cut(move.Key, "/")
	for _, block := range op.Blocks {
		parent := tree.Epics[block]
		if subtree(block) || block == feature.EpicKey || block == destinationEpic || parent != nil && (deref(parent.Key) == feature.EpicKey || deref(parent.Key) == destinationEpic) {
			return moveRefusal("staged question changes moved subtree holds")
		}
	}
	return nil
}

func remapMoveBlocks(blocks []string, oldKey, newKey string) bool {
	changed := false
	for i, block := range blocks {
		if block == oldKey || strings.HasPrefix(block, oldKey+"/") {
			blocks[i] = newKey + strings.TrimPrefix(block, oldKey)
			changed = true
		}
	}
	return changed
}

func moveDependencyRefs(values []string, refs map[string]string) ([]string, bool) {
	var out []string
	for i, value := range values {
		if id := refs[value]; id != "" {
			if out == nil {
				out = append([]string(nil), values...)
			}
			out[i] = id
		}
	}
	if out == nil {
		return values, false
	}
	return out, true
}
