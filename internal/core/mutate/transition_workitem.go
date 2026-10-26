package mutate

import (
	"fmt"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// transitionWorkItem is transitionTask for a shelf node (docs/plans/2026-08-28-
// bugs-and-chores-design.md, Lifecycle, contract, gates). The shape is the
// task's, minus what a task inherits from its parent feature: there is no
// parent-state walk, no feature basis, and no question hold, because a
// shelf node has no ancestors. The claim it needs is its own (Task 6), and
// the branch it records at start is cut from the integration branch, never a
// workbench. cancel carries a resolution.
func transitionWorkItem(tree *store.Tree, deps Deps, owner store.Contributor, treeNode store.WorkNode, in TransitionInput) (TransitionResult, error) {
	noun := treeNode.Noun()
	target, ymlRel, snapshot, err := loadWorkItemFile(tree, treeNode)
	if err != nil {
		return TransitionResult{}, err
	}
	w := target.Body()
	before := w.Status
	rev := deps.rev(tree.Root)

	if in.Verb == "approve" && in.RefreshBasis {
		if w.Status == statusDone || w.Status == statusCancelled {
			return TransitionResult{}, &usageError{reason: "terminal_basis_refresh", msg: fmt.Sprintf("%s %s is %s; a terminal node's basis is history and is not refreshed", noun, w.ID, w.Status)}
		}
		if err := recordWorkBasis(filepath.Dir(ymlRel), target, deps, rev); err != nil {
			return TransitionResult{}, err
		}
		ydata, err := yaml.Marshal(target)
		if err != nil {
			return TransitionResult{}, err
		}
		if err := commitTransition(tree, deps, owner, in, noun, w.ID, ymlRel, ydata, snapshot, before, before, ""); err != nil {
			return TransitionResult{}, err
		}
		return workResult(target, true), nil
	}

	if legal, moves := lifecycle.Legal(target, in.Verb); !legal {
		return TransitionResult{}, &lifecycle.IllegalTransitionError{Noun: noun, From: before, Verb: in.Verb, Legal: moves}
	}

	if in.Verb == "start" || in.Verb == "submit" {
		if err := checkWorkBasis(tree.Root, target, rev); err != nil {
			return TransitionResult{}, err
		}
		if in.AssumeQuestion != "" {
			return TransitionResult{}, &usageError{reason: "assume_unsupported", msg: fmt.Sprintf("a %s is never held by a question, so there is no assumption to record", englishList("or", store.ShelfNouns()...))}
		}
	}
	if workVerbs[in.Verb] {
		if err := guardWorkClaim(target, owner.ID); err != nil {
			return TransitionResult{}, err
		}
	}
	if in.Verb == "submit" {
		if err := guardSubmitGates(tree.Manifest, noun, w.ID, deref(w.Key), w.Status, &w.GatesBlock); err != nil {
			return TransitionResult{}, err
		}
	}
	if in.Verb == "cancel" {
		// The survivor is resolved once, here, and the SAME resolved id is
		// what lands on both the node's own duplicate_of and the event's
		// after map below (transitionEvent reads in.DuplicateOf directly):
		// the ledger and the record must never disagree about what
		// duplicate_of means when the caller named the survivor by key or
		// any other non-bare-id reference (round-1 review finding).
		resolved, err := validateWorkResolution(tree, target, in)
		if err != nil {
			return TransitionResult{}, err
		}
		in.DuplicateOf = resolved
	}
	if in.Verb == "approve" && before == statusProposed {
		if err := recordWorkBasis(filepath.Dir(ymlRel), target, deps, rev); err != nil {
			return TransitionResult{}, err
		}
	}

	stampResumeCode(&in, rev)
	if err := lifecycle.Apply(target, in.Verb, payloadFor(in)); err != nil {
		return TransitionResult{}, err
	}
	after := w.Status
	if after == before {
		return workResult(target, false), nil
	}
	switch in.Verb {
	case "start":
		w.Started = true
		// The branch is the node's own, cut from the integration branch and
		// named by branch_pattern over the canonical key: tiller/bugs/<key>.
		w.Branch = branchFor(tree.Manifest, deref(w.Key))
	case "cancel":
		w.Resolution = in.Resolution
		// in.DuplicateOf already holds the resolved id (the cancel arm
		// above), or "" on a non-duplicate resolution.
		w.DuplicateOf = in.DuplicateOf
	}

	ydata, err := yaml.Marshal(target)
	if err != nil {
		return TransitionResult{}, err
	}
	if err := commitTransition(tree, deps, owner, in, noun, w.ID, ymlRel, ydata, snapshot, before, after, ""); err != nil {
		return TransitionResult{}, err
	}
	return workResult(target, true), nil
}

// workResult projects a shelf node the way taskResult projects a task,
// carrying the branch too since a shelf node records its own.
func workResult(n store.WorkNode, changed bool) TransitionResult {
	w := n.Body()
	return TransitionResult{Noun: n.Noun(), ID: w.ID, Key: deref(w.Key), Status: w.Status, Branch: w.Branch, PR: w.PR, ReviewRef: w.ReviewRef, Changed: changed}
}

// recordWorkBasis pins the contract document (bug.md or chore.md) the way
// recordTaskBasis pins task.md: a git_blob of the working tree plus HEAD.
func recordWorkBasis(dir string, n store.WorkNode, deps Deps, rev vcs.Revisions) error {
	ref, err := gitBlobRef(rev, filepath.Join(dir, store.ShelfProseFile(n.Noun())), deps.clock().Now().UTC())
	if err != nil {
		return err
	}
	head, err := rev.CurrentCommit()
	if err != nil {
		return err
	}
	n.Body().AcceptedAgainst = &store.BasisRefs{Spec: ref, Commit: head}
	return nil
}

// checkWorkBasis is checkTaskBasis for a shelf node: the pinned contract
// must still match the working tree before start, check, or submit.
func checkWorkBasis(root string, n store.WorkNode, rev vcs.Revisions) error {
	w := n.Body()
	if w.AcceptedAgainst == nil || w.Key == nil {
		return nil
	}
	return checkBasis(root, rev, w.ID, n.Noun(), w.Status, []pinnedFile{
		{w.AcceptedAgainst.Spec, filepath.Join(workItemRelDir(n), store.ShelfProseFile(n.Noun()))},
	})
}

// guardWorkClaim is the owner lifecycle's work gate at shelf grain: a shelf
// node carries its own claim (the design's Verbs: "claims are at bug grain,
// always"), so start, submit, and resume refuse unless the acting contributor
// holds it.
func guardWorkClaim(n store.WorkNode, actingOwner string) error {
	w := n.Body()
	return guardExecutionClaim(n.Noun(), w.ID, deref(w.Key), w.Status, w.Started, w.Owner, actingOwner)
}

func guardExecutionClaim(noun, id, key, status string, started bool, holder, actingOwner string) error {
	if holder == "" {
		return &WorkUnclaimedError{
			Noun: noun, NodeID: id, Key: key, CurrentState: status,
			Reason: "work_unclaimed", Seam: fmt.Sprintf("tiller claim %s %s", noun, key),
		}
	}
	if holder == actingOwner {
		return nil
	}
	seam := fmt.Sprintf("tiller claim %s %s --from %s", noun, key, holder)
	reassign := fmt.Sprintf(`tiller reassign %s %s --to <contributor-id> --reason "<why>"`, noun, key)
	if started {
		seam = fmt.Sprintf(`tiller transfer %s %s --to <contributor-id> --reason "<why>"`, noun, key)
		reassign = ""
	}
	return &WorkClaimHeldError{
		Noun: noun, NodeID: id, Key: key, CurrentState: status,
		Reason: ReasonClaimHeld, Holder: holder, Seam: seam, ReassignSeam: reassign,
	}
}

// validateWorkResolution is cancel's own check on a shelf node: a
// resolution is required and must be one of the three; duplicate needs the
// survivor, the others must not name one (the design's Resolution). On a
// duplicate resolution it returns the survivor's own resolved (canonical)
// id, so the caller records and reports the very same value the caller
// named by key, partial reference, or bare id (round-1 review finding: the
// event and the node record must agree on what duplicate_of means). It
// returns "" alongside a nil error for every other resolution, since there
// is no survivor to resolve.
func validateWorkResolution(tree *store.Tree, n store.WorkNode, in TransitionInput) (string, error) {
	switch {
	case in.Resolution == "":
		return "", &usageError{reason: "resolution_required", msg: fmt.Sprintf("cancel %s needs --resolution wontfix|invalid|duplicate", n.Noun())}
	case !store.ValidResolution(in.Resolution) || in.Resolution == store.ResolutionTransferred:
		return "", &usageError{reason: "invalid_resolution", msg: fmt.Sprintf("--resolution %q is not wontfix, invalid, or duplicate", in.Resolution)}
	case in.Resolution == store.ResolutionDuplicate && in.DuplicateOf == "":
		return "", &usageError{reason: "duplicate_of_required", msg: "--resolution duplicate needs --duplicate-of naming the surviving " + n.Noun()}
	case in.Resolution != store.ResolutionDuplicate && in.DuplicateOf != "":
		return "", &usageError{reason: "duplicate_of_misplaced", msg: "--duplicate-of applies to --resolution duplicate only"}
	}
	if in.Resolution != store.ResolutionDuplicate {
		return "", nil
	}
	other, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: n.Noun(), Raw: in.DuplicateOf})
	if err != nil {
		return "", err
	}
	if other.NodeID() == n.NodeID() {
		return "", &usageError{reason: "duplicate_of_self", msg: fmt.Sprintf("%s %s cannot be a duplicate of itself", n.Noun(), n.NodeID())}
	}
	return other.NodeID(), nil
}

// WorkUnclaimedError refuses execution without the unit's own claim.
type WorkUnclaimedError struct {
	Noun         string
	NodeID       string
	Key          string
	CurrentState string
	Reason       string // "work_unclaimed"
	Seam         string
}

func (e *WorkUnclaimedError) Error() string {
	return fmt.Sprintf("mutate: %s %s is unclaimed; claim it first", e.Noun, e.NodeID)
}

// WorkClaimHeldError refuses execution by someone other than the unit's owner.
type WorkClaimHeldError struct {
	Noun         string
	NodeID       string
	Key          string
	CurrentState string
	Reason       string // ReasonClaimHeld
	Holder       string
	Seam         string
	ReassignSeam string
}

func (e *WorkClaimHeldError) Error() string {
	return fmt.Sprintf("mutate: %s %s is claimed by %s, not the acting contributor", e.Noun, e.NodeID, e.Holder)
}
