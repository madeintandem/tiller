package mutate

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// RecoverReviewInput names the authorized aggregate and exact historical Git
// commits. PR is caller-supplied association, never host-observed evidence.
type RecoverReviewInput struct {
	Noun, Key                 string
	PR                        *int
	Head, MergeCommit, Reason string
}

// RecoverReviewResult contains the durable binding without a status transition.
// An unchanged result preserves its original audit fields and does not assert
// that this call re-proved its integration evidence.
type RecoverReviewResult struct {
	Noun, ID, Key   string
	PR              *int
	ReviewRef       *store.ReviewRef
	RecoveredReview *store.RecoveredReview
	Changed         bool
}

// RecoverReview records a human-authorized historical binding. No lifecycle,
// gate, accepted basis, task, or Git ref is changed by this planner.
func RecoverReview(tree *store.Tree, deps Deps, in RecoverReviewInput) (result RecoverReviewResult, err error) {
	if in.Noun != store.NounFeature && in.Noun != store.NounEpic {
		return result, &usageError{reason: "unsupported_noun", msg: "merged review recovery applies only to feature and epic"}
	}
	if strings.TrimSpace(in.Reason) == "" || strings.TrimSpace(in.Head) == "" || strings.TrimSpace(in.MergeCommit) == "" {
		return result, &usageError{reason: "recovery_args_required", msg: "recovery requires a historical --head, exact --merge-commit, and nonblank --reason"}
	}
	if !vcs.IsHexObjectID(in.Head) || !vcs.IsHexObjectID(in.MergeCommit) {
		return result, &usageError{reason: "recovery_oid_required", msg: "recovery head and integration result must be hexadecimal commit IDs, not branch names or revision expressions"}
	}
	if in.PR != nil && *in.PR <= 0 {
		return result, &usageError{reason: "pr_required", msg: "recovery --pr must be a positive caller-supplied PR number"}
	}
	// Annotate every domain refusal with the resolved owner, including evidence
	// errors shared with sync. Environment and usage errors retain their classes.
	defer func() {
		var refusal *RecoverReviewError
		if errors.As(err, &refusal) {
			refusal.Noun = in.Noun
			if refusal.NodeID == "" {
				refusal.NodeID = result.ID
			}
		}
	}()
	manifestPath := filepath.Join(tree.Root, ".tiller/tiller.yml")
	manifestData, manifestSnapshot, err := store.ReadRegularFileSnapshot(tree.Root, manifestPath)
	if err != nil {
		return result, err
	}
	manifest, err := store.ParseManifest(tree.Root, manifestData)
	if err != nil {
		return result, err
	}
	treeCopy := *tree
	treeCopy.Manifest = manifest
	tree = &treeCopy
	actor, err := store.ResolveContributor(manifest, deps.Contributor)
	if err != nil {
		return result, err
	}
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Key})
	if err != nil {
		return result, err
	}
	var change reviewGrainChange
	var doc any
	var branch, status, path, key string
	var pr *int
	var review *store.ReviewRef
	var recovered *store.RecoveredReview
	var snapshot store.FileSnapshot
	switch n := node.(type) {
	case *store.Feature:
		path = filepath.Join(featureRelDir(n), "feature.yml")
		var f store.Feature
		snapshot, err = readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, path), &f)
		if err != nil {
			return result, err
		}
		f.EpicKey = n.EpicKey
		doc = &f
		change = reviewGrainChange{Noun: in.Noun, Feature: &f}
		result.ID = f.ID
		key = deref(f.Key)
		branch = f.Branch
		status = f.Status
		pr = f.PR
		review = f.ReviewRef
		recovered = f.RecoveredReview
	case *store.Epic:
		path = filepath.Join(epicRelDir(n), "epic.yml")
		var e store.Epic
		snapshot, err = readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, path), &e)
		if err != nil {
			return result, err
		}
		doc = &e
		change = reviewGrainChange{Noun: in.Noun, Epic: &e}
		result.ID = e.ID
		key = deref(e.Key)
		branch = e.Branch
		status = e.Status
		pr = e.PR
		review = e.ReviewRef
		recovered = e.RecoveredReview
	}
	result.Noun = in.Noun
	result.Key = key
	rev := deps.rev(tree.Root)
	history, ok := rev.(vcs.HistoricalRevisions)
	if !ok {
		return result, recoveryRefusal("history_unavailable", "", "use Git with immutable historical-object reads")
	}
	head, err := history.ResolveCommit(in.Head)
	if err != nil {
		return result, recoveryReadError("object_unavailable", in.Head, "fetch or restore the historical reviewed commit using ordinary Git", err)
	}
	merge, err := history.ResolveCommit(in.MergeCommit)
	if err != nil {
		return result, recoveryReadError("object_unavailable", in.MergeCommit, "fetch or restore the named integration result using ordinary Git", err)
	}
	into := strings.TrimSpace(manifest.Git.IntegrationBranch)
	if into == "" {
		return result, recoveryRefusal("integration_not_configured", "", "configure git.integration_branch before authorized recovery")
	}
	checkout, err := rev.HeadBranch()
	if err != nil {
		return result, err
	}
	if checkout != strings.TrimPrefix(into, "refs/heads/") {
		return result, recoveryRefusal("integration_checkout_required", checkout, "check out configured integration with ordinary Git before recovery")
	}
	if branch == "" {
		return result, recoveryRefusal("aggregate_branch_missing", "", "restore the recorded aggregate branch identity for this review round")
	}
	if !validWorkingBranch(branch) {
		return result, recoveryRefusal("aggregate_branch_invalid", branch, "restore the recorded local aggregate branch identity, not a revision expression or non-branch ref")
	}
	// A retry is deliberately identity-only. It remains a no-write success after
	// sync (even with uncommitted sync state), preserving its original audit data.
	if recovered != nil {
		if samePR(pr, in.PR) && recovered.Branch == branch && recoveryOID(recovered.Head) == head && recoveryOID(recovered.MergeCommit) == merge && recovered.IntegrationBranch == into {
			result.PR = pr
			result.ReviewRef = review
			result.RecoveredReview = recovered
			return result, nil
		}
		return result, recoveryRefusal("recovery_identity_changed", merge, "retain the existing immutable review round; a different witness or PR cannot replace it")
	}
	remote, ok := rev.(interface{ HasRemote() (bool, error) })
	if !ok {
		return result, recoveryRefusal("remote_identity_unknown", "", "use Git to inspect whether the repository has a remote")
	}
	hasRemote, err := remote.HasRemote()
	if err != nil {
		return result, err
	}
	if hasRemote && in.PR == nil {
		return result, &usageError{reason: "pr_required", msg: "recovery with a configured remote requires a positive caller-supplied --pr"}
	}
	newRef := (*store.ReviewRef)(nil)
	if in.PR == nil {
		newRef = &store.ReviewRef{Branch: branch, Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + head}}
	}
	existingReviewHead := ""
	if review != nil {
		existingReviewHead, err = history.ResolveCommit(recoveryOID(review.Head))
		if err != nil {
			return result, recoveryReadError("object_unavailable", review.Head.Value, "restore the existing no-remote review identity", err)
		}
	}
	if pr != nil && !samePR(pr, in.PR) || review != nil && (newRef == nil || review.Branch != branch || existingReviewHead != head) {
		return result, recoveryRefusal("review_identity_conflict", branch, "retain the existing PR or no-remote review identity; do not absorb a different review round")
	}
	fresh, change, guards, err := freshReviewGrainState(tree, change)
	if err != nil {
		return result, err
	}
	tree = fresh
	var features []*store.Feature
	if in.Noun == store.NounFeature {
		f := change.Feature
		epic := epicByKey(tree, f.EpicKey)
		grain := store.ResolvedReviewGrain(manifest, epic, f)
		if grain != store.ReviewGrainFeature {
			return result, recoveryGrainRefusal(grain)
		}
		if f.Status != "active" || epic == nil || epicTerminal(epic.Status) {
			return result, recoveryRefusal("aggregate_not_live", branch, "recover only an active feature under a live epic")
		}
		features = []*store.Feature{f}
	} else {
		e := change.Epic
		grain := store.ResolvedReviewGrain(manifest, e, nil)
		if grain != store.ReviewGrainEpic {
			return result, recoveryGrainRefusal(grain)
		}
		if status == statusDraft || epicTerminal(status) {
			return result, recoveryRefusal("aggregate_not_live", branch, "recover only a started live epic")
		}
		for _, f := range sortedFeatures(tree) {
			if f.EpicKey != key || f.Status == statusCancelled {
				continue
			}
			if store.ResolvedReviewGrain(manifest, e, f) != store.ReviewGrainEpic {
				if f.Status != statusDone {
					return result, recoveryRefusal("independent_review_open", f.ID, "finish the independent feature/task-grain review before epic recovery")
				}
				continue
			}
			if f.PR != nil || f.ReviewRef != nil || f.RecoveredReview != nil {
				return result, recoveryRefusal("independent_review_open", f.ID, "preserve the feature's independent review round before epic recovery")
			}
			features = append(features, f)
		}
	}
	var tasks []*store.Task
	for _, f := range features {
		for _, t := range tree.FeatureTasks[f.ID] {
			if t.Status == statusCancelled {
				continue
			}
			if t.Status != "in_review" && t.Status != statusDone {
				ref := recoveryRefusal("rider_not_submitted", t.Branch, "every live rider must already be in_review or done")
				ref.TaskID = t.ID
				return result, ref
			}
			if t.Status == "in_review" {
				if t.PR != nil || t.StartedGrain == store.ReviewGrainTask {
					ref := recoveryRefusal("task_review_open", t.Branch, "finish the task-owned PR without absorbing its review history")
					ref.TaskID = t.ID
					return result, ref
				}
				tasks = append(tasks, t)
			}
		}
	}
	if len(tasks) == 0 {
		return result, recoveryRefusal("no_eligible_riders", branch, "restore at least one eligible in_review rider for this aggregate round")
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	record := &store.RecoveredReview{Branch: branch, Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + head}, MergeCommit: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + merge}, IntegrationBranch: into, RecoveredBy: actor.ID, Reason: strings.TrimSpace(in.Reason)}
	historical, err := readRecoveryHistory(history, head, tree.Root)
	if err != nil {
		return result, err
	}
	if err := historical.matchAggregate(tree, in.Noun, result.ID, path, branch, in.PR, newRef, history); err != nil {
		return result, err
	}
	refPins := map[string]string{}
	integrationRef := branchRef(strings.TrimPrefix(into, "refs/heads/"))
	integrationTip, err := rev.MergeBase(integrationRef, integrationRef)
	if err != nil {
		return result, err
	}
	if integrationTip == "" {
		return result, recoveryRefusal("integration_ref_missing", into, "restore the configured integration branch using ordinary Git")
	}
	refPins[integrationRef] = integrationTip
	for _, task := range tasks {
		anchor, err := historical.bindTask(tree, rev, history, head, task, refPins)
		if err != nil {
			return result, err
		}
		record.Tasks = append(record.Tasks, anchor)
	}
	record.PolicySnapshots, err = historical.policySnapshots(history, head, features, tasks)
	if err != nil {
		return result, err
	}
	obs, err := validateRecoveredEvidence(rev, record, into, integrationTip)
	if err != nil {
		return result, err
	}
	record.Basis = obs.Basis
	if obs.NoMergeWitness {
		record.Basis = "contained"
	}
	// Stamp only after every historical validation; clocks may expose concurrent
	// changes in deterministic tests, so no source/ref read after this is trusted.
	record.RecoveredAt = deps.clock().Now().UTC()
	if err := record.Validate(); err != nil {
		return result, recoveryRefusal("recovery_record_invalid", path, err.Error())
	}
	guards = append(guards, store.PlanReadGuard(manifestPath, manifestSnapshot))
	relevant := []string{".tiller/tiller.yml", path}
	for _, f := range features {
		dir := featureRelDir(f)
		relevant = append(relevant, filepath.Join(dir, "feature.yml"), filepath.Join(dir, "feature.md"), filepath.Join(dir, "feature.eval.md"))
		parent := epicByKey(tree, f.EpicKey)
		if parent != nil {
			relevant = append(relevant, filepath.Join(epicRelDir(parent), "epic.yml"), filepath.Join(epicRelDir(parent), "epic.md"))
		}
		for _, task := range tree.FeatureTasks[f.ID] {
			if task.Status != statusCancelled {
				dir := taskDir(tree, task)
				relevant = append(relevant, filepath.Join(dir, "task.yml"), filepath.Join(dir, "task.md"))
			}
		}
	}
	// Git status supplies index/worktree cleanliness with the repository's
	// filters, EOL rules, and object format. Raw snapshots independently retain
	// the bytes used by this source transaction; unrelated files are not gated.
	scopedStatus, ok := rev.(interface {
		DirtyUnder(string) ([]string, error)
	})
	if !ok {
		return result, recoveryRefusal("cleanliness_unavailable", "", "use Git's scoped status to inspect relevant staged and working-tree changes")
	}
	for _, rel := range sortedRecoveryStrings(relevant) {
		_, pin, err := store.ReadRegularFileSnapshot(tree.Root, filepath.Join(tree.Root, rel))
		if err != nil {
			return result, err
		}
		if !pin.Exists() {
			return result, recoveryRefusal("relevant_input_missing", rel, "restore the relevant work/prose input before recovery")
		}
		if _, err := rev.BlobHash(rel); err != nil {
			return result, recoveryReadError("relevant_input_uncommitted", rel, "commit the relevant manifest/work/prose inputs on integration before recovery", err)
		}
		dirty, err := scopedStatus.DirtyUnder(":(literal)" + filepath.ToSlash(rel))
		if err != nil {
			return result, err
		}
		if len(dirty) > 0 {
			return result, recoveryRefusal("relevant_input_uncommitted", rel, "commit the relevant staged and working-tree manifest/work/prose inputs on integration before recovery")
		}
		if rel != path && rel != ".tiller/tiller.yml" {
			guards = append(guards, store.PlanReadGuard(filepath.Join(tree.Root, rel), pin))
		}
	}
	eventDeps := deps
	eventDeps.Clock = designClock{at: record.RecoveredAt}
	ev := submitEvent(eventDeps, actor, "recover_review."+in.Noun, in.Noun, result.ID, key, path, pr, review, in.PR, newRef)
	ev.Command = "submit " + in.Noun + " " + key + " --recover-merged"
	ev.After["reason"] = record.Reason
	ev.After["recovered_review"] = record
	switch n := doc.(type) {
	case *store.Feature:
		n.PR = in.PR
		n.ReviewRef = newRef
		n.RecoveredReview = record
	case *store.Epic:
		n.PR = in.PR
		n.ReviewRef = newRef
		n.RecoveredReview = record
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return result, err
	}
	// Source guards are checked under the journal lock. Git's independent ref
	// namespace is re-read immediately before entering that guarded transaction.
	for ref, tip := range refPins {
		now, err := rev.MergeBase(ref, ref)
		if err != nil {
			return result, err
		}
		if now != tip {
			return result, recoveryRefusal("recovery_stale", ref, "reload after the integration or retained task ref changed")
		}
	}
	nowBranch, err := rev.HeadBranch()
	if err != nil {
		return result, err
	}
	if nowBranch != checkout {
		return result, recoveryRefusal("recovery_stale", nowBranch, "restore the integration checkout and retry")
	}
	ownerWrite := store.PlanWrite(filepath.Clean(filepath.Join(tree.Root, path)), snapshot, data)
	for i := range guards {
		guards[i].Path = filepath.Clean(guards[i].Path)
	}
	writes, err := appendUniqueMoveGuards([]store.PlannedWrite{ownerWrite}, guards)
	if err != nil {
		return result, err
	}
	if err := commitMutation(tree, "recover_review."+in.Noun, writes, []events.Event{ev}); err != nil {
		return result, err
	}
	result.PR = in.PR
	result.ReviewRef = newRef
	result.RecoveredReview = record
	result.Changed = true
	return result, nil
}

func taskDir(tree *store.Tree, t *store.Task) string {
	f := tree.Features[t.FeatureID]
	return filepath.Join(featureRelDir(f), store.TaskDirName, filepath.Base(deref(t.Key)))
}
