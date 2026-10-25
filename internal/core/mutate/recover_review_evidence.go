package mutate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// RecoverReviewError is a named recovery refusal, not permission to fall back to
// ordinary live-branch observation. Prerequisite describes the required remedy.
type RecoverReviewError struct {
	Reason       string
	Noun         string
	NodeID       string
	TaskID       string
	Ref          string
	Prerequisite string
	Code         int
}

func (e *RecoverReviewError) Error() string {
	target := e.NodeID
	if e.TaskID != "" {
		target += " (task " + e.TaskID + ")"
	}
	if e.Ref != "" {
		target += " [" + e.Ref + "]"
	}
	return fmt.Sprintf("recover merged review %s: %s; %s", target, e.Reason, e.Prerequisite)
}
func (e *RecoverReviewError) ExitCode() int {
	if e.Code != 0 {
		return e.Code
	}
	return 1
}

func recoveryRefusal(reason, ref, remedy string) *RecoverReviewError {
	code := 1
	if reason == "object_unavailable" || reason == "immutable_oid_required" {
		code = 2
	}
	return &RecoverReviewError{Reason: reason, Ref: ref, Prerequisite: remedy, Code: code}
}

func recoveryReadError(reason, ref, remedy string, err error) error {
	var environment *store.EnvError
	if errors.As(err, &environment) {
		return err
	}
	return recoveryRefusal(reason, ref, remedy+": "+err.Error())
}

func recoveryOID(ref store.RevisionRef) string { return strings.TrimPrefix(ref.Value, "git:") }

// validateRecoveredEvidence proves the explicitly named integration result. It
// deliberately does not use the ordinary bounded squash search or live branch.
// Empty Basis is allowed only for the planner's not-yet-recorded candidate.
// into is the integration identity the record must name; intoCommit is the one
// commit the caller resolved it to, which every topology read observes, so a
// ref moving between reads cannot mix revisions into one validation.
func validateRecoveredEvidence(rev vcs.Revisions, record *store.RecoveredReview, into, intoCommit string) (mergeObs, error) {
	if record == nil {
		return mergeObs{}, recoveryRefusal("recovery_missing", "", "restore the recovered review record")
	}
	fail := func(reason, ref, remedy string) (mergeObs, error) {
		return mergeObs{}, recoveryRefusal(reason, ref, remedy)
	}
	if into != record.IntegrationBranch {
		return fail("integration_mismatch", into, "sync on the recovery record's configured integration branch")
	}
	if strings.HasPrefix(into, "refs/") && !strings.HasPrefix(into, "refs/heads/") {
		return fail("integration_mismatch", into, "recovery must name the configured local integration branch, not a tag or remote-tracking ref")
	}
	if !validWorkingBranch(record.Branch) {
		return fail("aggregate_branch_invalid", record.Branch, "restore the recorded local aggregate branch identity")
	}
	if record.Head.Kind != store.KindGitCommit || record.MergeCommit.Kind != store.KindGitCommit || record.Head.Path != "" || record.MergeCommit.Path != "" || !strings.HasPrefix(record.Head.Value, "git:") || !strings.HasPrefix(record.MergeCommit.Value, "git:") {
		return fail("immutable_oid_required", record.Head.Value, "restore typed full immutable historical commit references")
	}
	history, ok := rev.(vcs.HistoricalRevisions)
	if !ok {
		return fail("history_unavailable", "", "use Git with immutable historical-object reads")
	}
	head, err := history.ResolveCommit(recoveryOID(record.Head))
	if err != nil {
		return mergeObs{}, recoveryReadError("object_unavailable", record.Head.Value, "fetch or restore the historical reviewed commit using ordinary Git", err)
	}
	result, err := history.ResolveCommit(recoveryOID(record.MergeCommit))
	if err != nil {
		return mergeObs{}, recoveryReadError("object_unavailable", record.MergeCommit.Value, "fetch or restore the integration result using ordinary Git", err)
	}
	if head != recoveryOID(record.Head) || result != recoveryOID(record.MergeCommit) {
		return fail("immutable_oid_required", record.Head.Value, "record full immutable commit OIDs")
	}
	trunk := false
	if intoCommit != "" {
		trunk, err = rev.FirstParentContains(intoCommit, result)
		if err != nil {
			return mergeObs{}, err
		}
	}
	if !trunk {
		return fail("result_not_on_integration", result, "fetch or restore the named result on configured integration's first-parent history")
	}
	parents, err := history.CommitParents(result)
	if err != nil {
		return mergeObs{}, err
	}
	obs := mergeObs{Branch: record.Branch, BranchExists: true}
	// A merge must carry the reviewed head through a side parent, not merely
	// happen somewhere later than work already on the first-parent trunk.
	if len(parents) > 1 {
		carriedFirst, err := rev.IsMergedInto(head, parents[0])
		if err != nil {
			return mergeObs{}, err
		}
		if !carriedFirst {
			for _, parent := range parents[1:] {
				carried, err := rev.IsMergedInto(head, parent)
				if err != nil {
					return mergeObs{}, err
				}
				if carried {
					obs.Merged = true
					obs.Basis = basisAncestry
					break
				}
			}
		}
	}
	if !obs.Merged {
		// Only naming the contained head itself makes an unwitnessed assertion
		// honest. An unrelated later commit is never an automatic fallback.
		if result == head {
			obs.NoMergeWitness = true
		} else if len(parents) == 1 {
			base, err := rev.MergeBase(head, result)
			if err != nil {
				return mergeObs{}, err
			}
			if base == "" {
				return fail("branch_point_missing", head, "restore a shared branch point for the reviewed head and result")
			}
			before, err := rev.IsMergedInto(result, base)
			if err != nil {
				return mergeObs{}, err
			}
			want, err := rev.PatchID(base, head)
			if err != nil {
				return mergeObs{}, err
			}
			got, err := rev.PatchID(parents[0], result)
			if err != nil {
				return mergeObs{}, err
			}
			if before || want == "" || want != got {
				return fail("result_patch_mismatch", result, "name the exact squash result carrying the complete nonempty reviewed patch; no assertion fallback is permitted")
			}
			wantText, err := rev.PatchDigest(base, head)
			if err != nil {
				return mergeObs{}, err
			}
			gotText, err := rev.PatchDigest(parents[0], result)
			if err != nil {
				return mergeObs{}, err
			}
			if wantText == "" || wantText != gotText {
				return fail("result_patch_mismatch", result, "restore the exact whitespace-faithful whole-patch result")
			}
			obs.Merged = true
			obs.Basis = basisContent
			obs.Commit = result
		} else {
			return fail("merge_witness_missing", result, "name a result whose non-first-parent side carries the historical reviewed head")
		}
	}
	basis := obs.Basis
	if obs.NoMergeWitness {
		basis = "contained"
	}
	if record.Basis != "" && record.Basis != basis {
		return fail("evidence_basis_mismatch", result, "restore the recorded evidence basis; recovery cannot reinterpret a review round")
	}
	return obs, nil
}

func sameRecoveryRevision(a, b store.RevisionRef) bool {
	return a.Kind == b.Kind && a.Value == b.Value && a.Path == b.Path
}

// recoveredTaskIdentityMatches ignores observation times, never submission or
// approved task scope. Feature acceptance may change independently after merge.
func recoveredTaskIdentityMatches(current, historical *store.Task) bool {
	if current == nil || historical == nil || current.ReviewRef == nil || historical.ReviewRef == nil || current.AcceptedAgainst == nil || historical.AcceptedAgainst == nil {
		return false
	}
	return current.ID == historical.ID && deref(current.Key) == deref(historical.Key) && current.FeatureID == historical.FeatureID && current.Owner == historical.Owner && current.Started == historical.Started && current.Branch == historical.Branch && current.ReviewRef.Branch == historical.ReviewRef.Branch && sameRecoveryRevision(current.ReviewRef.Head, historical.ReviewRef.Head) && sameRecoveryRevision(current.AcceptedAgainst.Spec, historical.AcceptedAgainst.Spec)
}

func recoveredSubmitLocked(noun, id, branch string) *RecoverReviewError {
	return &RecoverReviewError{Reason: "recovery_already_recorded", Noun: noun, NodeID: id, Ref: branch, Prerequisite: "run ordinary tiller sync on configured integration; an identical historical retry must use --recover-merged"}
}

func recoveryGrainRefusal(grain string) *RecoverReviewError {
	return &RecoverReviewError{Reason: "wrong_review_grain", Ref: grain, Code: 2, Prerequisite: "preserve the existing " + grain + " review grain; recover only the feature or epic that actually owns the historical round, never change grain to absorb another review"}
}
