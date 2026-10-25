package mutate

import (
	"path/filepath"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// This file holds acceptance expiry: when a feature's eval contract changes
// and the new contract is accepted (approve feature --refresh-basis, or an
// amendment's revise of the feature), the acceptance verdicts its unfinished
// child tasks recorded against the old contract no longer describe the
// contract in force. A task's evals gate carries the human's criterion
// attestations and recorded rubric verdicts, and check deliberately preserves
// a settled pass, so without expiry a changed C1 statement would keep a task
// green on judgment given about different words.
//
// Expiry stales the child's evals gate and every criterion in it, the same
// rule resume and a reopened rollup apply (lifecycle.StaleGate). The
// attestation list and the evidence behind each verdict stay as history; only
// the verdicts expire. Done and cancelled tasks keep their record untouched,
// and a spec-only edit expires nothing, since refreshFeatureBasis reports a
// change only when the eval leg moved.

// expiredChild is one child task whose acceptance a contract change expired:
// its id and the repo-relative state file the expiry rewrites, which the
// refreshing event names.
type expiredChild struct {
	id  string
	rel string
}

// expireTaskAcceptance stales an unfinished task's evals verdict and reports
// whether the document changed. A terminal task, a task with no evals gate,
// and a gate already fully stale are left alone.
func expireTaskAcceptance(tk *store.Task) bool {
	if tk.Status == statusDone || tk.Status == statusCancelled {
		return false
	}
	g, ok := tk.Gates["evals"]
	if !ok {
		return false
	}
	changed := g.Status != store.GateStatusStale
	for _, c := range g.Criteria {
		if c.Status != store.CriterionStatusStale {
			changed = true
		}
	}
	if changed {
		lifecycle.StaleGate(tk, "evals")
	}
	return changed
}

// planChildAcceptanceExpiry reads every child task of feat fresh from disk and
// plans the direct refresh's half of the expiry: a write for each child whose
// acceptance expired, and a read guard for each child left alone, so a child
// changed after this read refuses the whole refresh rather than being
// overwritten or judged on stale status.
func planChildAcceptanceExpiry(tree *store.Tree, feat *store.Feature) ([]store.PlannedWrite, []expiredChild, error) {
	var writes []store.PlannedWrite
	var expired []expiredChild
	for _, tk := range tree.FeatureTasks[feat.ID] {
		doc, rel, snapshot, err := loadTaskFile(tree, feat, tk)
		if err != nil {
			return nil, nil, err
		}
		path := filepath.Join(tree.Root, rel)
		if !expireTaskAcceptance(doc) {
			writes = append(writes, store.PlanReadGuard(path, snapshot))
			continue
		}
		data, err := yaml.Marshal(doc)
		if err != nil {
			return nil, nil, err
		}
		writes = append(writes, store.PlanWrite(path, snapshot, data))
		expired = append(expired, expiredChild{id: doc.ID, rel: rel})
	}
	return writes, expired, nil
}

// expireChildAcceptance is the amendment batch's half of the expiry. Each
// child is taken through the batch's own staged document, so a cancel earlier
// in the batch is already terminal here and a child's own revise is kept
// rather than overwritten. A child left alone stays registered as a read
// guard on the batch's transaction.
func (b *batch) expireChildAcceptance(tree *store.Tree, featureID string) ([]expiredChild, error) {
	var expired []expiredChild
	for _, tk := range tree.FeatureTasks[featureID] {
		e, err := b.editFor(tree, tk)
		if err != nil {
			return nil, err
		}
		doc, ok := e.doc.(*store.Task)
		if !ok || !expireTaskAcceptance(doc) {
			continue
		}
		b.dirty[e.rel] = true
		expired = append(expired, expiredChild{id: doc.ID, rel: e.rel})
	}
	return expired, nil
}

// recordExpiry names the expired children on the event that changed the
// contract basis: their ids as targets, so each task's history shows why its
// verdict went stale, and their state files as changed files.
func recordExpiry(ev *events.Event, expired []expiredChild) {
	if len(expired) == 0 {
		return
	}
	ids := make([]string, 0, len(expired))
	for _, c := range expired {
		ids = append(ids, c.id)
		ev.Targets = append(ev.Targets, c.id)
		ev.Files = append(ev.Files, events.FileChange{Path: c.rel})
	}
	if ev.After == nil {
		ev.After = map[string]any{}
	}
	ev.After["acceptance_expired"] = ids
}
