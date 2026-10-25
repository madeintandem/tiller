package mutate

import (
	"fmt"
	"sort"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

type reviewGrainChange struct {
	Noun    string
	Epic    *store.Epic
	Feature *store.Feature
	Desired string
}

// Review-grain change blockers have stable reasons so every command path can
// render the same business refusal without parsing Error().
const (
	ReasonReviewGrainWorkInFlight = "review_grain_work_in_flight"
	ReasonReviewGrainBranchLocked = "review_grain_branch_locked"
)

// ReviewGrainChangeBlockedError identifies the target change and the first
// affected live node that makes retargeting unsafe.
type ReviewGrainChangeBlockedError struct {
	Reason        string
	TargetNoun    string
	TargetID      string
	TargetKey     string
	Desired       string
	BlockerNoun   string
	BlockerID     string
	BlockerKey    string
	BlockerStatus string
	BeforeGrain   string
	AfterGrain    string
}

func (e *ReviewGrainChangeBlockedError) Error() string {
	target := e.TargetNoun
	if ident := reviewGrainChangeIdent(e.TargetID, e.TargetKey); ident != "" {
		target += " " + ident
	}
	return fmt.Sprintf("mutate: changing review grain on %s from %s to %s is blocked by %s %s (%s)",
		target, e.BeforeGrain, e.AfterGrain, e.BlockerNoun,
		reviewGrainChangeIdent(e.BlockerID, e.BlockerKey), e.BlockerStatus)
}

func reviewGrainChangeIdent(id, key string) string {
	if key != "" {
		return key
	}
	return id
}

func resolvedReviewGrainAfterChange(tree *store.Tree, epic *store.Epic, feature *store.Feature, change reviewGrainChange) string {
	manifest := *tree.Manifest
	var epicCopy *store.Epic
	if epic != nil {
		copy := *epic
		epicCopy = &copy
	}
	var featureCopy *store.Feature
	if feature != nil {
		copy := *feature
		featureCopy = &copy
	}
	switch change.Noun {
	case "project":
		manifest.Execution.ReviewGrain = change.Desired
	case store.NounEpic:
		if epicCopy != nil && change.Epic != nil && epicCopy.ID == change.Epic.ID {
			epicCopy.ReviewGrain = change.Desired
		}
	case store.NounFeature:
		if featureCopy != nil && change.Feature != nil && featureCopy.ID == change.Feature.ID {
			featureCopy.ReviewGrain = change.Desired
		}
	}
	return store.ResolvedReviewGrain(&manifest, epicCopy, featureCopy)
}

func evaluateReviewGrainChange(tree *store.Tree, change reviewGrainChange) error {
	epicIDs := make([]string, 0, len(tree.Epics))
	for id := range tree.Epics {
		epicIDs = append(epicIDs, id)
	}
	sort.Strings(epicIDs)
	epicsByKey := make(map[string]*store.Epic, len(epicIDs))

	for _, id := range epicIDs {
		epic := reviewGrainCurrentEpic(tree.Epics[id], change)
		epicsByKey[deref(epic.Key)] = epic
		before := store.ResolvedReviewGrain(tree.Manifest, epic, nil)
		after := resolvedReviewGrainAfterChange(tree, epic, nil, change)
		if before == after || (epic.PR == nil && epic.ReviewRef == nil) {
			continue
		}
		return reviewGrainBlocked(change, store.NounEpic, epic.ID, deref(epic.Key), epic.Status, before, after)
	}

	featureIDs := make([]string, 0, len(tree.Features))
	for id := range tree.Features {
		featureIDs = append(featureIDs, id)
	}
	sort.Strings(featureIDs)

	for _, id := range featureIDs {
		feature := reviewGrainCurrentFeature(tree.Features[id], change)
		epic := epicsByKey[feature.EpicKey]
		before := store.ResolvedReviewGrain(tree.Manifest, epic, feature)
		after := resolvedReviewGrainAfterChange(tree, epic, feature, change)
		if before == after {
			continue
		}
		if feature.PR != nil || feature.ReviewRef != nil {
			return reviewGrainBlocked(change, store.NounFeature, feature.ID, deref(feature.Key), feature.Status, before, after)
		}
		for _, task := range gen.OrderTasks(tree, tree.FeatureTasks[feature.ID], feature.Order) {
			if reviewGrainTaskInFlight(task.Status) {
				return reviewGrainBlocked(change, store.NounTask, task.ID, deref(task.Key), task.Status, before, after)
			}
		}
		if feature.Branch != "" {
			return reviewGrainBranchBlocked(change, feature, before, after)
		}
	}

	return nil
}

func guardReviewGrainChange(tree *store.Tree, change reviewGrainChange) ([]store.PlannedWrite, error) {
	fresh, freshChange, guards, err := freshReviewGrainState(tree, change)
	if err != nil {
		return nil, err
	}
	if err := evaluateReviewGrainChange(fresh, freshChange); err != nil {
		return nil, err
	}
	return guards, nil
}

func reviewGrainCurrentEpic(epic *store.Epic, change reviewGrainChange) *store.Epic {
	if epic != nil && change.Epic != nil && epic.ID == change.Epic.ID {
		return change.Epic
	}
	return epic
}

func reviewGrainCurrentFeature(feature *store.Feature, change reviewGrainChange) *store.Feature {
	if feature != nil && change.Feature != nil && feature.ID == change.Feature.ID {
		return change.Feature
	}
	return feature
}

func reviewGrainTaskInFlight(status string) bool {
	switch status {
	case "in_progress", "check", "in_review":
		return true
	default:
		return false
	}
}

func reviewGrainBlocked(change reviewGrainChange, blockerNoun, blockerID, blockerKey, blockerStatus, before, after string) error {
	targetNoun, targetID, targetKey := reviewGrainChangeTarget(change)
	return &ReviewGrainChangeBlockedError{
		Reason: ReasonReviewGrainWorkInFlight, TargetNoun: targetNoun, TargetID: targetID, TargetKey: targetKey,
		Desired: change.Desired, BlockerNoun: blockerNoun, BlockerID: blockerID, BlockerKey: blockerKey,
		BlockerStatus: blockerStatus, BeforeGrain: before, AfterGrain: after,
	}
}

func reviewGrainBranchBlocked(change reviewGrainChange, feature *store.Feature, before, after string) error {
	targetNoun, targetID, targetKey := reviewGrainChangeTarget(change)
	return &ReviewGrainChangeBlockedError{
		Reason: ReasonReviewGrainBranchLocked, TargetNoun: targetNoun, TargetID: targetID, TargetKey: targetKey,
		Desired: change.Desired, BlockerNoun: store.NounFeature, BlockerID: feature.ID, BlockerKey: deref(feature.Key),
		BlockerStatus: feature.Status, BeforeGrain: before, AfterGrain: after,
	}
}

func reviewGrainChangeTarget(change reviewGrainChange) (noun, id, key string) {
	switch change.Noun {
	case store.NounEpic:
		if change.Epic != nil {
			return change.Noun, change.Epic.ID, deref(change.Epic.Key)
		}
	case store.NounFeature:
		if change.Feature != nil {
			return change.Noun, change.Feature.ID, deref(change.Feature.Key)
		}
	}
	return change.Noun, "", ""
}
