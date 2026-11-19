package mutate

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// basisRecovered is the done basis of a shelf node sync crossed on a recovered
// review. The record on the node names the Git evidence the crossing rested on.
const basisRecovered = "recovered"

// recoverShelfReview is RecoverReview for a bug, chore, or tweak whose fix
// merged before any submission was recorded. It writes the record and one
// event, and moves nothing: sync crosses the node on the record afterwards.
// A shelf node has no riders and no contract, so the evidence is the reviewed
// head and the integration result alone, checked exactly as an aggregate's is.
func recoverShelfReview(tree *store.Tree, deps Deps, actor store.Contributor, in RecoverReviewInput, manifestGuard store.PlannedWrite) (result RecoverReviewResult, err error) {
	result.Noun = in.Noun
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Key})
	if err != nil {
		return result, err
	}
	live, ok := store.AsWorkNode(node)
	if !ok {
		return result, fmt.Errorf("mutate: recovery resolved %s to an unsupported node type", in.Noun)
	}
	doc, path, snapshot, err := loadWorkItemFile(tree, live)
	if err != nil {
		return result, err
	}
	work := doc.Body()
	result.ID, result.Key = work.ID, deref(work.Key)
	manifest := tree.Manifest
	rev := deps.rev(tree.Root)

	// A recorded round answers before the lifecycle does, so a retry after
	// sync crossed the node is still a no-write success.
	if work.RecoveredReview == nil {
		switch work.Status {
		case "proposed":
			return result, recoveryRefusal("node_not_approved", "", "approve the node before recovering its review; a proposal was never authorized work")
		case statusDone, statusCancelled:
			return result, recoveryRefusal("node_terminal", "", "a "+work.Status+" node has no review left to record")
		}
		if batch := liveBatchOf(tree, live); batch != nil {
			return result, recoveryRefusal("batched_by_live_work", batch.ID, "the batch's own merge closes this node; take it out with tiller batch --remove before recovering a review of its own")
		}
		if workNodeCarried(tree, live) {
			return result, recoveryRefusal("carried_by_live_work", work.ResolvedBy, "the carrier's own merge closes this node; clear it with tiller resolve --clear before recovering a review of its own")
		}
	}
	commits, err := resolveRecoveryCommits(rev, manifest, in)
	if err != nil {
		return result, err
	}
	head, merge, into := commits.head, commits.merge, commits.into
	if work.Branch != "" && !validWorkingBranch(work.Branch) {
		return result, recoveryRefusal("work_branch_invalid", work.Branch, "restore the node's recorded local branch identity, not a revision expression or non-branch ref")
	}
	// A retry is identity-only, as it is for an aggregate: it writes nothing
	// and keeps the original audit fields.
	if recorded := work.RecoveredReview; recorded != nil {
		if samePR(work.PR, in.PR) && recoveryOID(recorded.Head) == head && recoveryOID(recorded.MergeCommit) == merge && recorded.IntegrationBranch == into {
			result.PR, result.ReviewRef, result.RecoveredReview = work.PR, work.ReviewRef, recorded.Review()
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
	if work.PR != nil && !samePR(work.PR, in.PR) {
		return result, recoveryRefusal("review_identity_conflict", work.Branch, "retain the PR this node already recorded; do not absorb a different review round")
	}
	refPins := map[string]string{}
	if err := shelfWorkIncluded(rev, commits, work, refPins); err != nil {
		return result, err
	}
	integrationRef := branchRef(strings.TrimPrefix(into, "refs/heads/"))
	integrationTip, err := rev.MergeBase(integrationRef, integrationRef)
	if err != nil {
		return result, err
	}
	if integrationTip == "" {
		return result, recoveryRefusal("integration_ref_missing", into, "restore the configured integration branch using ordinary Git")
	}
	refPins[integrationRef] = integrationTip

	record := &store.RecoveredShelfReview{
		Branch:            work.Branch,
		Head:              store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + head},
		MergeCommit:       store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + merge},
		IntegrationBranch: into, RecoveredBy: actor.ID, Reason: strings.TrimSpace(in.Reason),
	}
	obs, err := validateRecoveredEvidence(rev, record.Review(), into, integrationTip)
	if err != nil {
		return result, err
	}
	record.Basis = obs.Basis
	if obs.NoMergeWitness {
		record.Basis = "contained"
	}
	record.RecoveredAt = deps.clock().Now().UTC()
	if err := record.Validate(); err != nil {
		return result, recoveryRefusal("recovery_record_invalid", path, err.Error())
	}

	prose := filepath.Join(filepath.Dir(path), in.Noun+".md")
	guards, err := committedRecoveryInputs(tree, rev, []string{".tiller/tiller.yml", path, prose}, path, ".tiller/tiller.yml")
	if err != nil {
		return result, err
	}
	guards = append(guards, manifestGuard)

	eventDeps := deps
	eventDeps.Clock = designClock{at: record.RecoveredAt}
	ev := submitEvent(eventDeps, actor, "recover_review."+in.Noun, in.Noun, work.ID, result.Key, path, work.PR, work.ReviewRef, in.PR, work.ReviewRef)
	ev.Command = "submit " + in.Noun + " " + result.Key + " --recover-merged"
	ev.After["reason"] = record.Reason
	ev.After["recovered_review"] = record
	work.PR = in.PR
	work.RecoveredReview = record
	_, result.MissingGates = requiredGatesRecordedPass(manifest, in.Noun, &work.GatesBlock)
	data, err := yaml.Marshal(doc)
	if err != nil {
		return result, err
	}
	if err := recoveryRefsUnchanged(rev, refPins, commits.checkout); err != nil {
		return result, err
	}
	for i := range guards {
		guards[i].Path = filepath.Clean(guards[i].Path)
	}
	writes := append([]store.PlannedWrite{store.PlanWrite(filepath.Clean(filepath.Join(tree.Root, path)), snapshot, data)}, guards...)
	if err := commitMutation(tree, "recover_review."+in.Noun, writes, []events.Event{ev}); err != nil {
		return result, err
	}
	result.PR, result.ReviewRef, result.RecoveredReview, result.Changed = in.PR, work.ReviewRef, record.Review(), true
	return result, nil
}

// shelfWorkIncluded refuses a recovery that would close work the reviewed
// head never carried: the head a submitted node recorded, or the tip its
// recorded branch still holds. A node that never started has neither. Each
// branch tip read is pinned so a ref that moves before the write refuses.
func shelfWorkIncluded(rev vcs.Revisions, commits recoveryCommits, work *store.WorkItem, refPins map[string]string) error {
	if work.ReviewRef != nil {
		submitted, err := commits.history.ResolveCommit(recoveryOID(work.ReviewRef.Head))
		if err != nil {
			return recoveryReadError("object_unavailable", work.ReviewRef.Head.Value, "fetch or restore the node's submitted head using ordinary Git", err)
		}
		included, err := rev.IsMergedInto(submitted, commits.head)
		if err != nil {
			return err
		}
		if !included {
			return recoveryRefusal("submitted_head_not_included", submitted, "name the reviewed head that carries this node's submitted work")
		}
	}
	if work.Branch == "" {
		return nil
	}
	ref := branchRef(work.Branch)
	exists, err := rev.RefExists(ref)
	if err != nil {
		return err
	}
	if !exists {
		refPins[ref] = ""
		return nil
	}
	tip, err := rev.MergeBase(ref, ref)
	if err != nil {
		return err
	}
	refPins[ref] = tip
	included, err := rev.IsMergedInto(tip, commits.head)
	if err != nil {
		return err
	}
	if !included {
		return recoveryRefusal("retained_branch_tip_not_included", ref, "integrate or preserve the later work on this branch separately; do not complete it using the historical review")
	}
	return nil
}

// prepareRecoveredShelfSync validates every pending shelf recovery against the
// one integration commit sync resolved, before any crossing is written, and
// returns each node's observation by id. A record that no longer holds refuses
// the run, as an aggregate's does: recovery never falls back to observing a
// live branch.
func prepareRecoveredShelfSync(tree *store.Tree, rev vcs.Revisions, into, intoCommit string) (map[string]mergeObs, error) {
	var out map[string]mergeObs
	for _, noun := range store.ShelfNouns() {
		for _, live := range tree.WorkNodesOf(noun) {
			work := live.Body()
			record := work.RecoveredReview
			if record == nil || work.Status == statusDone || work.Status == statusCancelled || work.Key == nil || workNodeCarried(tree, live) {
				continue
			}
			refuse := func(reason, prerequisite string) error {
				return &RecoverReviewError{Reason: reason, Noun: noun, NodeID: work.ID, Prerequisite: prerequisite}
			}
			if err := record.Validate(); err != nil {
				return nil, err
			}
			if canonicalBranch(into, "") != canonicalBranch(record.IntegrationBranch, "") || canonicalBranch(tree.Manifest.Git.IntegrationBranch, "") != canonicalBranch(record.IntegrationBranch, "") {
				return nil, refuse("recovery_integration_changed", fmt.Sprintf("Run sync against recorded configured integration %q; do not reinterpret this recovery on another branch.", record.IntegrationBranch))
			}
			obs, err := validateRecoveredEvidence(rev, record.Review(), record.IntegrationBranch, intoCommit)
			if err != nil {
				var refusal *RecoverReviewError
				if errors.As(err, &refusal) {
					refusal.Noun, refusal.NodeID = noun, work.ID
				}
				return nil, err
			}
			if out == nil {
				out = map[string]mergeObs{}
			}
			out[work.ID] = obs
		}
	}
	return out, nil
}

// crossWorkRecovered records the crossing a recovered review earns: done set
// directly, the way a carried crossing sets it, because the node may never
// have started and the lifecycle table carries no row from there. The basis
// says the crossing rests on the recovery record; the event names the evidence
// beside it (ancestry, content, or the human's fast-forward assertion).
// Recovery approved no gate, so every required gate without a recorded pass is
// written as a policy violation here, as any other crossing would write it.
func crossWorkRecovered(tree *store.Tree, deps Deps, owner store.Contributor, live store.WorkNode, observed store.RevisionRef, obs mergeObs) (Crossing, error) {
	doc, ymlRel, snapshot, err := loadWorkItemFile(tree, live)
	if err != nil {
		return Crossing{}, err
	}
	w := doc.Body()
	if !reflect.DeepEqual(w.RecoveredReview, live.Body().RecoveredReview) {
		return Crossing{}, &RecoverReviewError{Reason: "recovery_source_changed", Noun: live.Noun(), NodeID: w.ID, Prerequisite: "Reload the immutable recovery record before syncing."}
	}
	before := w.Status
	w.Status = statusDone
	w.DoneBasis = basisRecovered
	violations := recordPolicyViolations(tree.Manifest, live.Noun(), &w.GatesBlock, observed)
	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return Crossing{}, err
	}
	match := ""
	if obs.Commit != "" {
		match = "git:" + obs.Commit
	}
	ev := transitionEvent(deps, owner, TransitionInput{Verb: "sync", SyncBasis: basisRecovered, SyncMatch: match}, live.Noun(), w.ID, ymlRel, before, w.Status, "")
	ev.After["evidence"] = obs.Basis
	if err := commitMutation(tree, "sync "+live.Noun(), []store.PlannedWrite{
		store.PlanWrite(filepath.Join(tree.Root, ymlRel), snapshot, ydata),
	}, []events.Event{ev}); err != nil {
		return Crossing{}, err
	}
	live.Body().Status = w.Status
	return Crossing{Noun: live.Noun(), ID: w.ID, Key: deref(w.Key), Status: w.Status,
		Observed: observed.Value, Basis: basisRecovered, Evidence: obs.Basis, MatchedCommit: match, PolicyViolations: violations}, nil
}
