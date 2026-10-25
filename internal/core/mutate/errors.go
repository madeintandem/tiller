package mutate

import "fmt"

// IntoIsFeatureBranchError reports that sync refused because the integration ref
// it resolved is a live task's or aggregate review's recorded branch. A branch is always its own
// ancestor, so that comparison can never be evidence of a merge, and recording a
// crossing from it would invent a done the human's merge never earned. Source
// names where the value came from (--into, git.integration_branch, HEAD) so the
// human knows what to change.
//
// The refusal is unconditional on that source. A project that literally declares
// git.integration_branch as a feature branch, or passes one to --into, is refused
// too: there is no legitimate reading of "this branch was merged into itself".
//
// Like the other blocked answers in this package it is exit 1, rendered through
// the CLI's emitReport as an ok:false data block, and it deliberately does not
// implement ExitCode, because the report sets that.
type IntoIsFeatureBranchError struct {
	Into   string
	Source string
	Noun   string
	NodeID string
	Key    string
	Branch string
}

func (e *IntoIsFeatureBranchError) Error() string {
	return fmt.Sprintf("sync refuses to observe merges into %s: it is %s %s's own branch", e.Into, e.Noun, e.NodeID)
}

// IntoUnresolvedError reports that sync could not tell which branch merges land
// on. It refuses rather than fall back to HEAD, because a HEAD detached at a
// feature branch tip reproduces the self-merge defect with no branch name left to
// catch it by. Exit 1, a data block.
//
// Two different paths raise it and they are not the same sentence, which is why
// Into exists. Empty means nothing was typed and nothing was declared: HEAD is
// detached, there is no --into, and the manifest carries no
// git.integration_branch. Non-empty means the human explicitly passed --into HEAD
// (or its @ alias) on a detached checkout, where a literal HEAD names no branch
// to compare a feature's against; the project may well declare an integration
// branch there, and saying it does not would be false.
type IntoUnresolvedError struct {
	Into string
}

func (e *IntoUnresolvedError) Error() string {
	if e.Into != "" {
		return fmt.Sprintf("sync cannot tell which branch merges land on: --into %s names no branch on a detached HEAD", e.Into)
	}
	return "sync cannot tell which branch merges land on: HEAD is detached and no integration branch is set"
}

// IntegrationMissingError reports that the integration branch sync selected
// resolves to no commit, so there is no revision to observe merges into or to
// pin completion records to. It refuses rather than read the checkout's HEAD
// instead. Exit 1, a data block.
type IntegrationMissingError struct {
	Into   string
	Source string
}

func (e *IntegrationMissingError) Error() string {
	return fmt.Sprintf("sync cannot observe merges into %s (from %s): it resolves to no commit", e.Into, e.Source)
}

// AttestIntoNotDeclaredError reports that sync refused --accept-fast-forward
// because the ref it would measure the assertion against is not the project's
// declared git.integration_branch. Declared is that field's value, and is empty
// when the project declares none at all, which is the other half of the same
// refusal: with nothing declared there is no trunk for the assertion to be about.
//
// The flag's safety property is "the integration branch already contains this
// work", and half of that is a claim about into. Containment in a scratch branch
// cut off the feature, in a release cut taken off it, or in the feature's own
// branch under a spelling the name-based self-merge guard cannot reduce, are all
// true observations about the wrong ref, and each recorded done for work no
// integration branch carried. Refusing here is what makes the flag's own
// documented promise true. Plain observation is untouched and still resolves into
// from --into, then the manifest, then the branch HEAD is on.
//
// Like the other blocked answers in this package it is exit 1, rendered through
// the CLI's emitReport as an ok:false data block, and it deliberately does not
// implement ExitCode, because the report sets that.
type AttestIntoNotDeclaredError struct {
	Into     string
	Source   string
	Declared string
}

func (e *AttestIntoNotDeclaredError) Error() string {
	if e.Declared == "" {
		return "--accept-fast-forward needs a declared git.integration_branch to measure the assertion against"
	}
	return fmt.Sprintf("--accept-fast-forward asserts about the declared integration branch %s, and this run resolved %s",
		e.Declared, e.Into)
}

// The reasons a node the human named on --accept-fast-forward is not in the
// candidate set. They are exported because the CLI renders one phrase per reason
// and carries the token in the --json refusal, and both must agree with the core
// rather than restate it.
//
// branch_missing and not_contained are the safety property speaking: the flag
// crosses only work the integration branch already contains on a branch that
// still resolves, so a deleted branch and an unmerged branch are refused, not
// asserted over. nothing_to_cross means the named node does not own a pending
// review at its resolved grain, or its aggregate includes no reviewed task.
const (
	AttestReasonBranchMissing  = "branch_missing"
	AttestReasonNotContained   = "not_contained"
	AttestReasonNothingToCross = "nothing_to_cross"
)

// AttestNotEligible is one node the human named on --accept-fast-forward that the
// assertion cannot be applied to, with the reason it is out of the candidate set.
//
// Recovery is non-nil only on a not_contained node, and only when it is honest:
// the branch's pushed remote-tracking tip resolves and IS itself contained in
// into, so the refusal has something real to prescribe (see Recovery's own
// doc). Every other reason, and a not_contained node with nothing merged on
// either tip, leaves it nil; the CLI renders no remediation block for a nil
// Recovery, so a refusal with nothing honest to prescribe reads exactly as it
// did before this field existed.
type AttestNotEligible struct {
	Noun     string
	NodeID   string
	Key      string
	Branch   string
	Reason   string
	Recovery *Recovery
}

// Recovery is the honest remediation for a not_contained refusal: the branch's
// pushed remote-tracking tip is contained in into while the local tip is not,
// which is the shape a state commit left unpushed before its PR merged leaves
// behind (issue #43's field case, and the review-state warning issue #42 names
// from the other side). RemoteTip and LocalTip are short shas, the form every
// other prescription in this file names a commit by; Upstream is the ref they
// were measured against, branch's configured @{upstream} or origin/<branch>
// when none is configured (Task 2's UnpushedTip resolves it, reused rather
// than duplicated here). Worktree is the path holding branch checked out, ""
// when nothing does, and it is what tells the CLI's two remediation arms
// apart: `git branch -f` refuses outright when the branch is checked out
// anywhere, so a worktree holding it changes which command is honest to name.
//
// It is computed once, in the core, alongside the classification that already
// walked this branch's containment (attestedObservations): the CLI renders
// these facts and never re-derives a git answer of its own.
type Recovery struct {
	RemoteTip string
	LocalTip  string
	Upstream  string
	Worktree  string
}

// AttestNotEligibleError reports that sync refused --accept-fast-forward because
// at least one named node is not in the contained-but-unwitnessed set. The flag
// widens the basis Tiller will accept, never the set of branches it will cross,
// so a node whose work the integration branch does not carry, or whose branch no
// longer resolves, is refused rather than crossed on the human's word.
//
// The refusal is all-or-nothing across the named set and runs before any
// observation is acted on and before any write, the same posture as
// IntoIsFeatureBranchError: a refused sync changes nothing, including for the
// named nodes that were eligible. Partial application would leave the human
// guessing which half of their assertion landed.
//
// Like the other blocked answers in this package it is exit 1, rendered through
// the CLI's emitReport as an ok:false data block, and it deliberately does not
// implement ExitCode, because the report sets that.
type AttestNotEligibleError struct {
	Into  string
	Nodes []AttestNotEligible
}

// The sentence names the set rather than one of its three conditions. Saying
// "%s does not contain them" was false for nothing_to_cross, which is only
// reachable once containment has already been established, so a caller reading
// Error() alone was told the opposite of what the per-node reason says.
func (e *AttestNotEligibleError) Error() string {
	return fmt.Sprintf("--accept-fast-forward refused: %d named node(s) are outside the set it may cross against %s",
		len(e.Nodes), e.Into)
}

// usageError is a misuse (exit 2) the core raises when a request is malformed in
// a way only the core can judge, such as a feature key that is not the canonical
// two-segment shape. It carries a stable reason discriminator and maps to exit 2
// through the CLI's error classifier, which keys off ExitCode and Reason.
//
// cause is nil unless this refusal widens an existing typed error into a
// fuller message rather than replacing it outright. When set, it is that
// original error, carried through Unwrap so a caller unwinding for the
// concrete type a narrower refusal would have returned still finds it —
// the rule that keeps Unwrap honest about what this usageError stands in
// for. Two constructors use it this way today, both widening a bare
// *resolve.ErrNotFound into a message naming a second possibility, a
// same-batch staged epic: parentUnknownErr (amendment.go) at stage time and
// materializeParentUnknownErr (amendment_approve.go) at approve time.
type usageError struct {
	reason string
	msg    string
	cause  error
}

func (e *usageError) Error() string  { return e.msg }
func (e *usageError) ExitCode() int  { return 2 }
func (e *usageError) Reason() string { return e.reason }
func (e *usageError) Unwrap() error  { return e.cause }
