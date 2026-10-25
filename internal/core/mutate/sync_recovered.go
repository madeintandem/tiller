package mutate

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// recoveredSync caches exact-result validation and frozen submitted state once
// per aggregate, before any crossing is written. Ordinary reviews never use it.
type recoveredSync struct {
	id          string
	guards      []store.PlannedWrite
	record      *store.RecoveredReview
	obs         mergeObs
	tasks       map[string]*store.Task
	refsChecked bool
}

func aggregateRecovery(tree *store.Tree, id string) *store.RecoveredReview {
	if feature := tree.Features[id]; feature != nil {
		return feature.RecoveredReview
	}
	if epic := tree.Epics[id]; epic != nil {
		return epic.RecoveredReview
	}
	return nil
}

func aggregateRoundDone(tree *store.Tree, id string) bool {
	if feature := tree.Features[id]; feature != nil {
		return feature.Status == statusDone
	}
	if epic := tree.Epics[id]; epic != nil {
		return epic.Status == statusDone || epic.Status == statusArchived || epic.Status == statusCancelled
	}
	return false
}

func recoveredSyncRefusal(id, task, reason, prerequisite string) *RecoverReviewError {
	noun := store.NounFeature
	if strings.HasPrefix(id, "E-") {
		noun = store.NounEpic
	}
	return &RecoverReviewError{Reason: reason, Noun: noun, NodeID: id, TaskID: task, Prerequisite: prerequisite}
}

func recoveredEvidenceError(id string, err error) error {
	var refusal *RecoverReviewError
	if errors.As(err, &refusal) {
		refusal.NodeID = id
		refusal.Noun = store.NounFeature
		if strings.HasPrefix(id, "E-") {
			refusal.Noun = store.NounEpic
		}
	}
	return err
}

func prepareRecoveredSync(tree *store.Tree, rev vcs.Revisions, into, intoCommit string) (map[string]*recoveredSync, error) {
	hasRecovery := false
	for _, feature := range tree.Features {
		hasRecovery = hasRecovery || feature.RecoveredReview != nil
	}
	for _, epic := range tree.Epics {
		hasRecovery = hasRecovery || epic.RecoveredReview != nil
	}
	if !hasRecovery {
		return nil, nil
	}
	for id, feature := range tree.Features {
		if feature.RecoveredReview == nil || aggregateRoundDone(tree, id) || feature.Status == statusCancelled {
			continue
		}
		selected, _, _ := reviewedAggregate(tree, feature)
		if selected != id {
			return nil, recoveredSyncRefusal(id, "", "recovery_review_grain_changed", "Restore the recovered feature's review grain; historical evidence cannot fall back to task or epic review.")
		}
	}
	for id, epic := range tree.Epics {
		if epic.RecoveredReview == nil || aggregateRoundDone(tree, id) {
			continue
		}
		for _, anchor := range epic.RecoveredReview.Tasks {
			current := tree.Tasks[anchor.ID]
			if current == nil || tree.Features[current.FeatureID] == nil {
				return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_task_changed", "Restore the frozen epic rider and its historical owning feature before syncing.")
			}
			selected, _, _ := reviewedAggregate(tree, tree.Features[current.FeatureID])
			if selected != id {
				return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_review_grain_changed", "Restore the frozen rider's epic review grain; an independent review cannot borrow this recovery or replace it with live-branch observation.")
			}
		}
	}
	out := map[string]*recoveredSync{}
	for _, feature := range sortedFeatures(tree) {
		id, branch, recorded := reviewedAggregate(tree, feature)
		record := aggregateRecovery(tree, id)
		if record == nil || aggregateRoundDone(tree, id) || feature.Status == statusDone || feature.Status == statusCancelled {
			continue
		}
		if out[id] != nil {
			continue
		}
		if err := record.Validate(); err != nil {
			return nil, err
		}
		if !recorded || branch != record.Branch {
			return nil, recoveredSyncRefusal(id, "", "recovery_identity_changed", "Restore the recorded aggregate review identity before syncing recovered work.")
		}
		if canonicalBranch(into, "") != canonicalBranch(record.IntegrationBranch, "") || canonicalBranch(tree.Manifest.Git.IntegrationBranch, "") != canonicalBranch(record.IntegrationBranch, "") {
			return nil, recoveredSyncRefusal(id, "", "recovery_integration_changed", fmt.Sprintf("Run sync against recorded configured integration %q; do not reinterpret this recovery on another branch.", record.IntegrationBranch))
		}
		obs, err := validateRecoveredEvidence(rev, record, record.IntegrationBranch, intoCommit)
		if err != nil {
			return nil, recoveredEvidenceError(id, err)
		}
		history, ok := rev.(vcs.HistoricalRevisions)
		if !ok {
			return nil, recoveredSyncRefusal(id, "", "historical_objects_unavailable", "Use a Git revision reader that can read the recovered historical submitted-state blobs.")
		}
		documents := make(map[string][]byte, len(record.PolicySnapshots))
		for _, snapshot := range record.PolicySnapshots {
			data, blob, err := history.ReadFileAt(strings.TrimPrefix(record.Head.Value, "git:"), snapshot.Path)
			if err != nil {
				return nil, recoveredEvidenceError(id, recoveryReadError("object_unavailable", snapshot.Path, "fetch or restore the recovered historical policy/task blob using ordinary Git", err))
			}
			if !sameRecoveryRevision(blob, snapshot.Source) {
				return nil, recoveredSyncRefusal(id, snapshot.NodeID, "recovery_source_changed", "Restore the exact historical policy blob pinned by this recovered review.")
			}
			documents[snapshot.Path] = data
		}
		guards, err := recoveredSourceGuards(tree, id, record)
		if err != nil {
			return nil, err
		}
		state := &recoveredSync{id: id, guards: guards, record: record, obs: obs, tasks: make(map[string]*store.Task, len(record.Tasks))}
		head := strings.TrimPrefix(record.Head.Value, "git:")
		parents := map[string]bool{}
		for _, anchor := range record.Tasks {
			current := tree.Tasks[anchor.ID]
			if current == nil {
				return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_task_changed", "Restore the frozen riding task; a missing or replaced task cannot use this historical review.")
			}
			parent := tree.Features[current.FeatureID]
			if parent == nil || parent.Key == nil || current.Key == nil {
				return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_task_changed", "Restore the historical task ownership and canonical path before syncing.")
			}
			ownerID, _, _ := reviewedAggregate(tree, parent)
			expected := filepath.ToSlash(filepath.Join(featureRelDir(parent), "tasks", localKey(deref(current.Key)), "task.yml"))
			if ownerID != id || expected != anchor.Path {
				return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_task_changed", "Restore the frozen task's owning feature, review grain, and historical path before syncing.")
			}
			if !parents[parent.ID] {
				if parent.ID != id {
					path := filepath.Join(tree.Root, featureRelDir(parent), "feature.yml")
					var doc store.Feature
					snapshot, err := readYAMLSnapshot(tree.Root, path, &doc)
					if err != nil {
						return nil, err
					}
					if doc.ID != parent.ID || deref(doc.Key) != deref(parent.Key) || doc.Status != parent.Status || doc.Branch != parent.Branch || doc.ReviewGrain != parent.ReviewGrain || !samePR(doc.PR, parent.PR) || !sameReviewRef(doc.ReviewRef, parent.ReviewRef) || !reflect.DeepEqual(doc.RecoveredReview, parent.RecoveredReview) {
						return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_source_changed", "Reload the participating feature's current ownership, review grain, and review round before syncing.")
					}
					state.guards = append(state.guards, store.PlanReadGuard(path, snapshot))
				}
				data := documents[filepath.ToSlash(filepath.Join(featureRelDir(parent), "feature.yml"))]
				if len(data) == 0 {
					return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_source_changed", "Restore the frozen task's historical parent policy snapshot before syncing.")
				}
				var historical store.Feature
				if err := yaml.Unmarshal(data, &historical); err != nil {
					return nil, err
				}
				if historical.ID != parent.ID {
					return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_task_changed", "Restore the frozen task's historical parent identity before syncing.")
				}
				parents[parent.ID] = true
			}
			data := documents[anchor.Path]
			if len(data) == 0 {
				return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_source_changed", "Restore the exact submitted-state policy snapshot pinned by this recovered review.")
			}
			var historical store.Task
			if err := yaml.Unmarshal(data, &historical); err != nil {
				return nil, err
			}
			historical.FeatureID = parent.ID
			if !recoveredTaskIdentityMatches(current, &historical) || (historical.Status != "in_review" && historical.Status != statusDone) || (current.Status != "in_review" && current.Status != statusDone) || (current.Status == "in_review" && historical.Status != "in_review") {
				return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_task_changed", "Restore the frozen task submission, owner, and accepted task scope; new work needs its own review round.")
			}
			if current.Status == "in_review" {
				if current.PR != nil || historical.PR != nil {
					return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_task_review_changed", "Preserve the task-owned PR as an independent review; it cannot ride the frozen aggregate recovery.")
				}
				if current.ReviewRef == nil {
					return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_task_changed", "Restore the immutable submitted task head before syncing.")
				}
				included, err := rev.IsMergedInto(strings.TrimPrefix(current.ReviewRef.Head.Value, "git:"), head)
				if err != nil {
					return nil, err
				}
				exists, err := rev.RefExists(branchRef(current.Branch))
				if err != nil {
					return nil, err
				}
				if included && exists {
					included, err = rev.IsMergedInto(branchRef(current.Branch), head)
					if err != nil {
						return nil, err
					}
				}
				if !included {
					return nil, recoveredSyncRefusal(id, anchor.ID, "recovery_task_not_included", "The retained task tip or submitted head advanced outside the historical review; reconcile that work through an independent review before syncing.")
				}
			}
			state.tasks[anchor.ID] = &historical
		}
		// A frozen witness cannot complete a new rider added after recovery.
		for _, parent := range sortedFeatures(tree) {
			ownerID, _, _ := reviewedAggregate(tree, parent)
			if ownerID != id {
				continue
			}
			for _, task := range tree.FeatureTasks[parent.ID] {
				if task.Status != statusCancelled && task.Status != statusDone && state.tasks[task.ID] == nil {
					return nil, recoveredSyncRefusal(id, task.ID, "recovery_rider_set_changed", "Sync and reopen the consumed aggregate before starting a new review round; the historical witness covers only its frozen riders.")
				}
			}
		}
		out[id] = state
	}
	return out, nil
}

func historicalPolicyForNode(record *store.RecoveredReview, nodeID string) []store.HistoricalPolicySnapshot {
	if record == nil {
		return nil
	}
	for i, snapshot := range record.PolicySnapshots {
		if snapshot.NodeID == nodeID {
			end := i + 1
			for end < len(record.PolicySnapshots) && record.PolicySnapshots[end].NodeID == nodeID {
				end++
			}
			return record.PolicySnapshots[i:end]
		}
	}
	return nil
}

// The owning document supplies node identity. Source/node/gate dedup therefore
// compares only source and gate here; current crossing-time entries have no
// historical source and remain independent, even when their gate name matches.
func recordHistoricalPolicyViolations(gb *store.GatesBlock, snapshots []store.HistoricalPolicySnapshot, observed store.RevisionRef) {
	for _, snapshot := range snapshots {
		for _, observation := range snapshot.Observations {
			found := false
			for _, existing := range gb.PolicyViolations {
				if existing.Gate == observation.Gate && sameRecoveryRevision(existing.SourceRevision, snapshot.Source) {
					found = true
					break
				}
			}
			if found {
				continue
			}
			gb.PolicyViolations = append(gb.PolicyViolations, store.PolicyViolation{
				Gate: observation.Gate, Status: observation.Status, Observed: observed,
				SourceRevision: snapshot.Source, UnresolvedCriteria: append([]string(nil), observation.UnresolvedCriteria...), EvidenceRefs: append([]string(nil), observation.EvidenceRefs...),
			})
		}
	}
}

func recoveredFeatureParticipated(tree *store.Tree, state *recoveredSync, feature *store.Feature) bool {
	if state == nil || (!state.obs.Merged && !state.obs.NoMergeWitness) {
		return false
	}
	for _, task := range tree.FeatureTasks[feature.ID] {
		historical := state.tasks[task.ID]
		if historical != nil && historical.Status == "in_review" && task.Status == statusDone && (task.DoneBasis == basisAncestry || task.DoneBasis == basisContent || task.DoneBasis == basisHumanAttested) {
			return true
		}
	}
	return false
}

func historicalCrossingViolations(violations []store.PolicyViolation) []store.PolicyViolation {
	var historical []store.PolicyViolation
	for _, violation := range violations {
		if violation.SourceRevision.Value != "" {
			historical = append(historical, violation)
		}
	}
	return historical
}

func recoveredSourceGuards(tree *store.Tree, id string, record *store.RecoveredReview) ([]store.PlannedWrite, error) {
	manifestPath := filepath.Join(tree.Root, ".tiller", "tiller.yml")
	var manifest store.Manifest
	manifestSnapshot, err := readYAMLSnapshot(tree.Root, manifestPath, &manifest)
	if err != nil {
		return nil, err
	}
	if manifest.Git.IntegrationBranch != tree.Manifest.Git.IntegrationBranch || manifest.Execution.ReviewGrain != tree.Manifest.Execution.ReviewGrain {
		return nil, recoveredSyncRefusal(id, "", "recovery_source_changed", "Reload the current integration and review-grain configuration before syncing recovered work.")
	}
	var source *store.RecoveredReview
	var path string
	var snapshot store.FileSnapshot
	if feature := tree.Features[id]; feature != nil {
		path = filepath.Join(tree.Root, featureRelDir(feature), "feature.yml")
		var doc store.Feature
		snapshot, err = readYAMLSnapshot(tree.Root, path, &doc)
		if err != nil {
			return nil, err
		}
		if doc.ID != id || deref(doc.Key) != deref(feature.Key) || doc.Status != feature.Status || doc.Branch != feature.Branch || doc.ReviewGrain != feature.ReviewGrain || !samePR(doc.PR, feature.PR) || !sameReviewRef(doc.ReviewRef, feature.ReviewRef) {
			return nil, recoveredSyncRefusal(id, "", "recovery_source_changed", "Reload the aggregate's current review round before syncing.")
		}
		source = doc.RecoveredReview
	} else {
		epic := tree.Epics[id]
		path = filepath.Join(tree.Root, epicRelDir(epic), "epic.yml")
		var doc store.Epic
		snapshot, err = readYAMLSnapshot(tree.Root, path, &doc)
		if err != nil {
			return nil, err
		}
		if doc.ID != id || deref(doc.Key) != deref(epic.Key) || doc.Status != epic.Status || doc.Branch != epic.Branch || doc.ReviewGrain != epic.ReviewGrain || !samePR(doc.PR, epic.PR) || !sameReviewRef(doc.ReviewRef, epic.ReviewRef) {
			return nil, recoveredSyncRefusal(id, "", "recovery_source_changed", "Reload the aggregate's current review round before syncing.")
		}
		source = doc.RecoveredReview
	}
	if !reflect.DeepEqual(source, record) {
		return nil, recoveredSyncRefusal(id, "", "recovery_source_changed", "Reload the immutable recovery record before syncing.")
	}
	return []store.PlannedWrite{store.PlanReadGuard(manifestPath, manifestSnapshot), store.PlanReadGuard(path, snapshot)}, nil
}

// Exact immutable evidence is cached above. The guarded write only rechecks
// mutable containment: Git refs are not protected by Tiller's source lock.
func commitRecoveredCrossing(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, in TransitionInput,
	noun, id, rel string, data []byte, snapshot store.FileSnapshot, before, after string, state *recoveredSync, task *store.Task) error {
	if state == nil {
		return commitTransition(tree, deps, owner, in, noun, id, rel, data, snapshot, before, after, "")
	}
	target := filepath.Join(tree.Root, rel)
	writes := make([]store.PlannedWrite, 0, len(state.guards)+2)
	targetGuard := -1
	for index, guard := range state.guards {
		if guard.Path != target {
			writes = append(writes, guard)
		} else if guard.Before != snapshot {
			return recoveredSyncRefusal(state.id, "", "recovery_source_changed", "Reload the aggregate's current review round before syncing.")
		} else {
			targetGuard = index
		}
	}
	writes = append(writes, store.PlanWrite(target, snapshot, data))
	event := transitionEvent(deps, owner, in, noun, id, rel, before, after, "")
	eventPlans, err := eventWrites(tree, []events.Event{event})
	if err != nil {
		return err
	}
	err = tree.CommitChecked("sync."+noun, append(writes, eventPlans...), func() error {
		reachable, err := rev.FirstParentContains(branchRef(state.record.IntegrationBranch), recoveryOID(state.record.MergeCommit))
		if err != nil {
			return err
		}
		if !reachable {
			return recoveredSyncRefusal(state.id, "", "result_not_on_integration", "Restore the named result on configured integration's first-parent history before syncing.")
		}
		if !state.refsChecked {
			for taskID := range state.tasks {
				current := tree.Tasks[taskID]
				if current != nil && current.Status == "in_review" {
					if err := guardRecoveredTaskTip(rev, state, current); err != nil {
						return err
					}
				}
			}
			state.refsChecked = true
		} else if task != nil {
			if err := guardRecoveredTaskTip(rev, state, task); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if targetGuard >= 0 {
		current, nextSnapshot, err := store.ReadRegularFileSnapshot(tree.Root, target)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, data) {
			return recoveredSyncRefusal(state.id, "", "recovery_source_changed", "Reload after the participating feature changed immediately after this sync write.")
		}
		state.guards[targetGuard] = store.PlanReadGuard(target, nextSnapshot)
	}
	return nil
}

// Feature policy describes the review's participating feature, not just the
// eventual rollup. An unchecked current gate may delay rollup without erasing
// the historical policy at the merge that already crossed its tasks.
func persistRecoveredFeaturePolicy(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, feature *store.Feature, observed store.RevisionRef, state *recoveredSync) error {
	rel := filepath.Join(featureRelDir(feature), "feature.yml")
	var doc store.Feature
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, rel), &doc)
	if err != nil {
		return err
	}
	count := len(doc.PolicyViolations)
	recordHistoricalPolicyViolations(&doc.GatesBlock, historicalPolicyForNode(state.record, feature.ID), observed)
	if len(doc.PolicyViolations) == count {
		return nil
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	if err := commitRecoveredCrossing(tree, deps, owner, rev, TransitionInput{Verb: "sync"},
		store.NounFeature, doc.ID, rel, data, snapshot, doc.Status, doc.Status, state, nil); err != nil {
		return err
	}
	feature.PolicyViolations = doc.PolicyViolations
	return nil
}

func guardRecoveredTaskTip(rev vcs.Revisions, state *recoveredSync, task *store.Task) error {
	exists, err := rev.RefExists(branchRef(task.Branch))
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	included, err := rev.IsMergedInto(branchRef(task.Branch), recoveryOID(state.record.Head))
	if err != nil {
		return err
	}
	if !included {
		refusal := recoveredSyncRefusal(state.id, task.ID, "recovery_task_not_included", "Reconcile the retained task tip that advanced outside the historical review before syncing.")
		refusal.Ref = branchRef(task.Branch)
		return refusal
	}
	return nil
}
