package mutate

import (
	"errors"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// prepareTaskAggregate makes branch recording and the first task start one
// transaction. A completed aggregate round gets a new ref identity; old refs,
// terminal task identities, and audit events are never rewritten.
//
// adopt is start --adopt-aggregate: instead of minting the
// derived aggregate name it records an existing local ref at exactly that
// name, after the same reservation checks. minted reports that this start
// named an aggregate nothing recorded before, which taskStartBase needs so a
// dependency refusal does not point at a branch that was never written.
func prepareTaskAggregate(tree *store.Tree, rev vcs.Revisions, feature *store.Feature, task *store.Task, adopt bool) (*store.Tree, *store.Feature, []store.PlannedWrite, []string, bool, error) {
	fresh, _, guards, err := freshReviewGrainState(tree, reviewGrainChange{})
	if err != nil {
		return nil, nil, nil, nil, false, err
	}
	feature = fresh.Features[feature.ID]
	branch, noun, id := aggregateForTask(fresh, feature)
	if adopt && noun == "" {
		return nil, nil, nil, nil, false, &BranchSafetyError{Reason: "aggregate_adopt_grain", Noun: store.NounFeature, NodeID: feature.ID}
	}
	minted := false
	epic := epicByKey(fresh, feature.EpicKey)
	if noun == store.NounFeature && feature.Status == statusDone || noun == store.NounEpic && epic.Status == statusDone {
		return nil, nil, nil, nil, false, &BranchSafetyError{Reason: "aggregate_round_closed", Noun: noun, NodeID: id, Branch: branch}
	}
	if aggregateRecovery(fresh, id) != nil {
		return nil, nil, nil, nil, false, &BranchSafetyError{Reason: "aggregate_round_closed", Noun: noun, NodeID: id, Branch: branch}
	}
	if noun != "" {
		var pr *int
		var reviewRef *store.ReviewRef
		key := "reviews/features/" + deref(feature.Key)
		if noun == store.NounEpic {
			pr, reviewRef = epic.PR, epic.ReviewRef
			key = "reviews/epics/" + deref(epic.Key)
		} else {
			pr, reviewRef = feature.PR, feature.ReviewRef
		}
		if adopt && (branch != "" || pr != nil || reviewRef != nil) {
			return nil, nil, nil, nil, false, &BranchSafetyError{Reason: "aggregate_adopt_recorded", Noun: noun, NodeID: id, Branch: branch}
		}
		completed := false
		inFlight := false
		for _, sibling := range fresh.Tasks {
			parent := fresh.Features[sibling.FeatureID]
			if parent == nil {
				continue
			}
			_, _, aggregateID := aggregateForTask(fresh, parent)
			if aggregateID != id {
				continue
			}
			completed = completed || sibling.Status == statusDone
			inFlight = inFlight || reviewGrainTaskInFlight(sibling.Status)
		}
		rotate := branch != "" && completed && !inFlight && pr == nil && reviewRef == nil
		if branch == "" || rotate {
			branch = branchFor(fresh.Manifest, key)
			if rotate {
				branch += "-" + task.ID
			}
			if adopt {
				if err := guardAdoptedBranch(fresh, rev, noun, id, branch); err != nil {
					return nil, nil, nil, nil, false, err
				}
			} else if err := guardNewBranch(fresh, rev, noun, id, branch); err != nil {
				// A ref at exactly the derived name is a hand-built aggregate
				// nothing recorded, not a naming conflict: name the adopt path.
				var safety *BranchSafetyError
				if !rotate && errors.As(err, &safety) && safety.Reason == "branch_ref_conflict" && safety.Conflict == branch {
					safety.Reason = "aggregate_unrecorded"
				}
				return nil, nil, nil, nil, false, err
			}
			minted = !adopt && !rotate
		}
	}
	var writes []store.PlannedWrite
	var files []string
	snapshots := make(map[string]store.FileSnapshot, len(guards))
	for _, guard := range guards {
		snapshots[guard.Path] = guard.Before
	}
	stamp := func(rel string, doc any) error {
		path := filepath.Join(tree.Root, rel)
		snapshot := snapshots[path]
		data, err := yaml.Marshal(doc)
		if err != nil {
			return err
		}
		writes = append(writes, store.PlanWrite(path, snapshot, data))
		files = append(files, rel)
		return nil
	}
	activated := feature.Status == "ready"
	if activated {
		feature.Status = "active"
	}
	if noun == store.NounEpic && epic.Branch != branch {
		epic.Branch = branch
		if err := stamp(filepath.Join(epicRelDir(epic), "epic.yml"), epic); err != nil {
			return nil, nil, nil, nil, false, err
		}
	}
	featureIDs := []string{feature.ID}
	if noun != "" {
		featureIDs = sortedMapKeys(fresh.Features)
	}
	for _, featureID := range featureIDs {
		riding := fresh.Features[featureID]
		_, _, aggregateID := aggregateForTask(fresh, riding)
		if (noun == "" || aggregateID != id || riding.Branch == branch || riding.Status == statusCancelled) && !(riding.ID == feature.ID && activated) {
			continue
		}
		if riding.ID != feature.ID && riding.Status == statusDone {
			continue
		}
		if noun != "" {
			riding.Branch = branch
		}
		if err := stamp(filepath.Join(featureRelDir(riding), "feature.yml"), riding); err != nil {
			return nil, nil, nil, nil, false, err
		}
	}
	// Retain the fresh lineage snapshots except for files replaced by this
	// transaction (including the task document owned by transitionTask).
	replaced := map[string]bool{filepath.Join(tree.Root, featureRelDir(feature), "tasks", filepath.Base(deref(task.Key)), "task.yml"): true}
	for _, write := range writes {
		replaced[write.Path] = true
	}
	for _, guard := range guards {
		if !replaced[guard.Path] {
			writes = append(writes, guard)
		}
	}
	return fresh, feature, writes, files, minted, nil
}
