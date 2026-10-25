package mutate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// TransferInput records a human decision, delegable only by an explicit
// instruction naming this execution unit and its recipient. Identity is a
// workflow guard, not authentication; the actor need not be the original owner.
type TransferInput struct {
	Noun   string
	Node   string
	To     string
	Reason string
}

// TransferRef retains the immutable identity and external review history.
type TransferRef struct {
	ID        string           `json:"id"`
	Key       string           `json:"key"`
	Owner     string           `json:"owner"`
	Branch    string           `json:"branch,omitempty"`
	PR        *int             `json:"pr,omitempty"`
	ReviewRef *store.ReviewRef `json:"review_ref,omitempty"`
}

// TransferRelationship names a live edge changed without rewriting history.
type TransferRelationship struct {
	Noun     string   `json:"noun"`
	ID       string   `json:"id"`
	Relation string   `json:"relation"`
	Before   []string `json:"before"`
	After    []string `json:"after"`
}

type TransferResult struct {
	Noun          string                 `json:"noun"`
	Original      TransferRef            `json:"original"`
	Replacement   TransferRef            `json:"replacement"`
	Reason        string                 `json:"reason"`
	Relationships []TransferRelationship `json:"relationships"`
	Invalidated   []string               `json:"invalidated"`
	StartCommand  string                 `json:"start_command"`
	FollowThrough []string               `json:"follow_through"`
	Files         []string               `json:"files"`
}

// TransferBlockedError names a safe refusal and, on repetition, the successor.
type TransferBlockedError struct {
	Code      string
	Detail    string
	Successor string
}

func (e *TransferBlockedError) Error() string  { return e.Detail }
func (e *TransferBlockedError) Reason() string { return e.Code }
func (e *TransferBlockedError) ExitCode() int  { return 1 }

func transferBlocked(code, format string, args ...any) error {
	return &TransferBlockedError{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// Transfer cancels and replaces started, unfinished work in one guarded source
// transaction. It never changes a Git ref, creates a branch, or calls a host API.
func Transfer(tree *store.Tree, deps Deps, in TransferInput) (TransferResult, error) {
	actor, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return TransferResult{}, err
	}
	if in.Noun != store.NounTask && !store.IsWorkNoun(in.Noun) {
		supported := append([]string{store.NounTask}, store.ShelfNouns()...)
		return TransferResult{}, &usageError{reason: "unsupported_noun", msg: "transfer applies only to " + englishList("or", supported...)}
	}
	in.To, in.Reason = strings.TrimSpace(in.To), strings.TrimSpace(in.Reason)
	if in.To == "" || in.Reason == "" {
		return TransferResult{}, &usageError{reason: "transfer_arguments_required", msg: "transfer requires --to <registered-owner> and a nonblank --reason"}
	}
	recipient, err := store.ResolveContributor(tree.Manifest, in.To)
	if err != nil {
		return TransferResult{}, err
	}
	p, err := loadTransferSnapshot(tree)
	if err != nil {
		return TransferResult{}, err
	}
	n, err := resolve.NewResolver(p.tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Node})
	if err != nil {
		return TransferResult{}, err
	}
	res := TransferResult{Noun: in.Noun, Reason: in.Reason, Relationships: []TransferRelationship{}, Invalidated: []string{}}
	var started bool
	var successor *string
	var basis *store.BasisRefs
	var relDir string
	var parent *store.Feature
	switch original := n.(type) {
	case *store.Task:
		parent = p.tree.Features[original.FeatureID]
		res.Original = TransferRef{ID: original.ID, Key: deref(original.Key), Owner: original.Owner, Branch: original.Branch, PR: original.PR, ReviewRef: original.ReviewRef}
		started, successor, basis = original.Started, original.SupersededBy, original.AcceptedAgainst
		relDir = store.TaskRelDir(parent.EpicKey, localKey(deref(parent.Key)), localKey(deref(original.Key)))
	case store.WorkNode:
		w := original.Body()
		res.Original = TransferRef{ID: w.ID, Key: deref(w.Key), Owner: w.Owner, Branch: w.Branch, PR: w.PR, ReviewRef: w.ReviewRef}
		started, successor, basis = w.Started, w.SupersededBy, w.AcceptedAgainst
		relDir = workItemRelDir(original)
	}
	if successor != nil && *successor != "" {
		return TransferResult{}, &TransferBlockedError{Code: "transfer_already_replaced", Successor: *successor, Detail: fmt.Sprintf("%s %s already has successor %s; continue that work instead", in.Noun, res.Original.Key, *successor)}
	}
	if transferTerminal(n.NodeStatus()) {
		return TransferResult{}, transferBlocked("transfer_terminal", "%s %s is %s; terminal ownership and history cannot be transferred", in.Noun, res.Original.Key, n.NodeStatus())
	}
	if !started {
		return TransferResult{}, transferBlocked("transfer_not_started", "%s %s has not started; use tiller reassign %s %s --to %s --reason %q instead", in.Noun, res.Original.Key, in.Noun, res.Original.Key, recipient.ID, in.Reason)
	}
	if res.Original.Owner == recipient.ID {
		return TransferResult{}, transferBlocked("transfer_same_owner", "%s %s already belongs to %s; transfer needs a different recipient", in.Noun, res.Original.Key, recipient.ID)
	}
	if res.Original.Owner == "" {
		return TransferResult{}, transferBlocked("transfer_owner_missing", "%s %s has no recorded original owner; review its ownership migration before transferring", in.Noun, res.Original.Key)
	}
	if basis == nil || basis.Spec.Value == "" {
		return TransferResult{}, transferBlocked("transfer_basis_missing", "%s %s lacks an accepted contract; review and approve its basis before transfer", in.Noun, res.Original.Key)
	}
	rev := deps.rev(tree.Root)
	proseRel := filepath.Join(relDir, in.Noun+".md")
	prose, err := p.read(proseRel)
	if err != nil {
		return TransferResult{}, err
	}
	if err := checkBasis(tree.Root, rev, n.NodeID(), in.Noun, n.NodeStatus(), []pinnedFile{{basis.Spec, proseRel}}); err != nil {
		return TransferResult{}, err
	}
	if parent != nil {
		epic := epicByKey(p.tree, parent.EpicKey)
		if (parent.Status != "active" && parent.Status != "ready") || epic == nil || transferTerminal(epic.Status) {
			return TransferResult{}, transferBlocked("transfer_parent_retired", "task %s needs a live approved parent contract; review the feature and epic before transfer", res.Original.Key)
		}
		if parent.AcceptedAgainst.Spec.Value == "" || parent.AcceptedAgainst.Eval.Value == "" {
			return TransferResult{}, transferBlocked("transfer_basis_missing", "feature %s lacks an accepted spec/eval contract; review and approve its basis first", deref(parent.Key))
		}
		for _, name := range []string{"feature.md", "feature.eval.md"} {
			if _, err := p.read(filepath.Join(featureRelDir(parent), name)); err != nil {
				return TransferResult{}, err
			}
		}
		if err := checkFeatureBasis(tree.Root, featureRelDir(parent), parent, rev); err != nil {
			return TransferResult{}, err
		}
	}
	if err := guardTransferMerged(p.tree, rev, n, parent); err != nil {
		return TransferResult{}, err
	}
	prefix, err := id.PrefixFor(in.Noun)
	if err != nil {
		return TransferResult{}, err
	}
	nid, err := deps.mintIn(p.tree, prefix)
	if err != nil {
		return TransferResult{}, err
	}
	if !id.Valid(nid) || !strings.HasPrefix(nid, prefix+"-") {
		return TransferResult{}, &usageError{reason: "invalid_id", msg: "transfer minter returned an invalid replacement id"}
	}
	key, newDir, err := p.successorKey(n, relDir)
	if err != nil {
		return TransferResult{}, err
	}
	newProseRel := filepath.Join(newDir, in.Noun+".md")
	accepted := *basis
	accepted.Spec.Path, accepted.Spec.ObservedAt = newProseRel, deps.clock().Now().UTC()
	accepted.Eval = store.RevisionRef{}
	accepted.Commit, err = rev.CurrentCommit()
	if err != nil {
		return TransferResult{}, err
	}
	res.Replacement = TransferRef{ID: nid, Key: key, Owner: recipient.ID}
	res.StartCommand = fmt.Sprintf("tiller start %s %s --contributor %s", in.Noun, key, recipient.ID)
	var replacement store.Node
	switch original := n.(type) {
	case *store.Task:
		replacement = &store.Task{ID: nid, Key: &key, Title: original.Title, Status: "ready", Blocked: original.Blocked, Owner: recipient.ID, ProposedBy: original.ProposedBy, DependsOn: append([]string(nil), original.DependsOn...), Supersedes: []string{original.ID}, AcceptedAgainst: &accepted, FeatureID: original.FeatureID}
		original.Status, original.SupersededBy = statusCancelled, &nid
		before := append([]string(nil), parent.Order...)
		ordered := gen.OrderTasks(p.tree, p.tree.FeatureTasks[parent.ID], parent.Order)
		parent.Order = make([]string, 0, len(ordered))
		for _, task := range ordered {
			parent.Order = append(parent.Order, task.ID)
		}
		parent.Order = transferReplaceEdge(parent.Order, original.ID, nid)
		if !equalStringLists(before, parent.Order) {
			res.Relationships = append(res.Relationships, TransferRelationship{Noun: store.NounFeature, ID: parent.ID, Relation: "order", Before: before, After: parent.Order})
		}
		lifecycle.StaleGates(parent)
		res.Invalidated = append(res.Invalidated, parent.ID)
		if err := p.put(filepath.Join(featureRelDir(parent), "feature.yml"), parent); err != nil {
			return TransferResult{}, err
		}
	case store.WorkNode:
		w := original.Body()
		fresh, err := store.NewWorkNode(in.Noun)
		if err != nil {
			return TransferResult{}, err
		}
		*fresh.Body() = store.WorkItem{ID: nid, Key: &key, Title: w.Title, Status: "ready", Blocked: w.Blocked, Owner: recipient.ID, ProposedBy: w.ProposedBy, AttachedTo: w.AttachedTo, ResolvedBy: w.ResolvedBy, Source: w.Source, DependsOn: append([]string(nil), w.DependsOn...), Supersedes: []string{w.ID}, AcceptedAgainst: &accepted}
		replacement = fresh
		w.Status, w.SupersededBy, w.Resolution = statusCancelled, &nid, store.ResolutionTransferred
		if w.ResolvedBy != "" && (!carrierCanCarry(carrierState(p.tree, w.ResolvedBy)) || carrierState(p.tree, w.ResolvedBy) == statusDone) {
			return TransferResult{}, transferBlocked("transfer_carrier_unsafe", "%s %s has a retired, completed, or missing carrier %s; a human must review tiller resolve and sync before transfer", in.Noun, res.Original.Key, w.ResolvedBy)
		}
		if err := p.shelfOrder(in.Noun, w.ID, nid, &res); err != nil {
			return TransferResult{}, err
		}
	}
	if err := p.relationships(n, replacement, &res); err != nil {
		return TransferResult{}, err
	}
	switch next := replacement.(type) {
	case *store.Task:
		p.tree.Tasks[next.ID] = next
		p.tree.FeatureTasks[next.FeatureID] = append(p.tree.FeatureTasks[next.FeatureID], next)
	case *store.Bug:
		p.tree.Bugs[next.ID] = next
	case *store.Chore:
		p.tree.Chores[next.ID] = next
	case *store.Tweak:
		p.tree.Tweaks[next.ID] = next
	}
	if err := p.tree.ValidateOwnership(); err != nil {
		return TransferResult{}, err
	}
	if err := p.put(filepath.Join(relDir, in.Noun+".yml"), n); err != nil {
		return TransferResult{}, err
	}
	if err := p.put(filepath.Join(newDir, in.Noun+".yml"), replacement); err != nil {
		return TransferResult{}, err
	}
	p.create(newProseRel, prose)
	res.FollowThrough = transferContinuation(p.tree, parent, res)
	res.Files = append([]string(nil), p.changed...)
	ev := events.NewEvent(deps.clock(), "transfer."+in.Noun, actor.ID, fmt.Sprintf("transfer %s %s --to %s --reason %q", in.Noun, res.Original.Key, recipient.ID, in.Reason), res.Original.ID, nid)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"id": res.Original.ID, "owner": res.Original.Owner, "status": p.originalStatus[res.Original.ID]}
	ev.After = map[string]any{"id": res.Original.ID, "status": statusCancelled, "superseded_by": nid, "replacement_id": nid, "replacement_owner": recipient.ID, "replacement_status": "ready", "reason": in.Reason, "relationships": res.Relationships, "invalidated": res.Invalidated}
	for _, rel := range res.Files {
		ev.Files = append(ev.Files, events.FileChange{Path: rel})
	}
	eventPlans, err := eventWrites(tree, []events.Event{ev})
	if err != nil {
		return TransferResult{}, err
	}
	// Source guards are checked under the lock; observe local merge evidence
	// again immediately before writes. This is not a lock on Git or the host.
	if err := tree.CommitChecked("transfer "+in.Noun, append(p.writes(), eventPlans...), func() error {
		return guardTransferMerged(p.tree, rev, n, parent)
	}); err != nil {
		return TransferResult{}, err
	}

	return res, nil
}

func transferTerminal(status string) bool {
	return status == statusDone || status == statusCancelled || status == statusArchived
}

func guardTransferMerged(tree *store.Tree, rev vcs.Revisions, n store.Node, parent *store.Feature) error {
	type review struct {
		branch   string
		recorded bool
		ref      *store.ReviewRef
		recovery *store.RecoveredReview
		id       string
	}
	var reviews []review
	switch node := n.(type) {
	case *store.Task:
		if aggregate, noun, _ := aggregateForTask(tree, parent); noun != "" {
			if noun == store.NounFeature {
				reviews = append(reviews, review{id: parent.ID, branch: aggregate, recorded: parent.PR != nil || parent.ReviewRef != nil, ref: parent.ReviewRef, recovery: parent.RecoveredReview})
			} else {
				epic := epicByKey(tree, parent.EpicKey)
				reviews = append(reviews, review{id: epic.ID, branch: aggregate, recorded: epic.PR != nil || epic.ReviewRef != nil, ref: epic.ReviewRef, recovery: epic.RecoveredReview})
			}
		}
		reviews = append(reviews, review{branch: node.Branch, recorded: node.PR != nil || node.ReviewRef != nil, ref: node.ReviewRef})
	case store.WorkNode:
		w := node.Body()
		reviews = append(reviews, review{branch: w.Branch, recorded: w.PR != nil || w.ReviewRef != nil, ref: w.ReviewRef})
	}
	into, source, err := resolveInto(tree.Manifest, SyncInput{}, rev.HeadBranch)
	if err != nil {
		return err
	}
	if err := guardIntoNotFeatureBranch(tree, into, source, rev.HeadBranch); err != nil {
		return err
	}
	cache := &patchCache{}
	for _, review := range reviews {
		if review.recovery != nil {
			if err := review.recovery.Validate(); err != nil {
				return err
			}
			if canonicalBranch(tree.Manifest.Git.IntegrationBranch, "") != canonicalBranch(review.recovery.IntegrationBranch, "") || canonicalBranch(into, "") != canonicalBranch(review.recovery.IntegrationBranch, "") {
				return transferBlocked("transfer_review_unobservable", "%s %s recovery belongs to integration %q; restore that configured integration and sync before transferring", n.Noun(), deref(n.NodeKey()), review.recovery.IntegrationBranch)
			}
			// Valid evidence always observes the round merged or contained, and
			// every started task beneath a live recovery rode it: recovery refuses
			// unsubmitted riders, and start refuses aggregate_round_closed until
			// sync reopens the parent.
			integrationTip, err := rev.RefCommit(branchRef(review.recovery.IntegrationBranch))
			if err != nil {
				return err
			}
			if _, err := validateRecoveredEvidence(rev, review.recovery, review.recovery.IntegrationBranch, integrationTip); err != nil {
				return recoveredEvidenceError(review.id, err)
			}
			return transferBlocked("transfer_review_merged", "%s %s recovered review is merged or contained in integration; sync first, then create forward work instead of transferring merged history", n.Noun(), deref(n.NodeKey()))
		}
		observed, err := observeMerge(rev, review.branch, into, cache)
		if err != nil {
			return err
		}
		merged := observed.Merged || (review.recorded && observed.NoMergeWitness)
		witness := observed.BranchExists
		// The recorded review head survives a deleted or subsequently advanced
		// branch; integrated review history cannot be hidden by moving that ref.
		if review.ref != nil && review.ref.Head.Value != "" {
			head := strings.TrimPrefix(review.ref.Head.Value, "git:")
			submitted, err := observeMergeRef(rev, head, head, into, cache)
			if err != nil {
				return err
			}
			merged = merged || submitted.Merged || submitted.NoMergeWitness
			witness = witness || submitted.BranchExists
		}
		if merged {
			return transferBlocked("transfer_review_merged", "%s %s review is merged or contained in integration; sync first (review fast-forward evidence if needed), then create forward work instead of transferring merged history", n.Noun(), deref(n.NodeKey()))
		}
		if review.recorded && !witness {
			return transferBlocked("transfer_review_unobservable", "%s %s has a recorded review but its branch and usable submitted head are unavailable locally. Restore or fetch the review branch and run tiller sync before deciding whether transfer is safe; merged history requires forward work", n.Noun(), deref(n.NodeKey()))
		}
	}
	return nil
}

func transferContinuation(tree *store.Tree, parent *store.Feature, res TransferResult) []string {
	steps := []string{res.StartCommand, "Run the start command only when the preserved manual pauses, questions, prerequisites, and carrier allow it. Carried shelf work still waits for its carrier; transfer is not permission to clear that relationship.", "Commit and share the Tiller source transaction through ordinary Git; reconcile it into every continuing checkout before working on the replacement."}
	if parent != nil {
		branch, noun, target := aggregateForTask(tree, parent)
		if noun != "" {
			if res.Original.PR != nil {
				steps = append(steps, fmt.Sprintf("Close the superseded task PR #%d without merging; its replacement rides the existing aggregate review rather than opening another task PR.", *res.Original.PR))
			}
			var pr *int
			var reviewRef *store.ReviewRef
			if noun == store.NounFeature {
				pr, reviewRef = parent.PR, parent.ReviewRef
			} else {
				epic := epicByKey(tree, parent.EpicKey)
				pr, reviewRef = epic.PR, epic.ReviewRef
			}
			submit := fmt.Sprintf("tiller submit %s %s", noun, target)
			if pr != nil {
				submit += fmt.Sprintf(" --pr %d", *pr)
				steps = append(steps, fmt.Sprintf("Keep existing unmerged aggregate PR #%d on %s; ordinary-merge the replacement branch and its submission-state commits into that aggregate.", *pr, branch))
			} else if reviewRef != nil {
				steps = append(steps, fmt.Sprintf("Keep the existing unmerged local aggregate review on %s; ordinary-merge the replacement branch and its submission-state commits into that aggregate.", branch))
			} else {
				steps = append(steps, fmt.Sprintf("No %s aggregate review is recorded yet. Assemble the replacement into the applicable aggregate after start; open a host PR only after fresh checks and acceptance, adding --pr <number> to the submit command when using a host.", noun))
			}
			steps = append(steps, fmt.Sprintf("Rerun tiller check feature %s; obtain fresh human acceptance for affected criteria, then run %s on the aggregate branch.", deref(parent.Key), submit))
		} else {
			steps = append(steps, transferCloseReview(res), fmt.Sprintf("After the live tasks finish, rerun tiller check feature %s and obtain fresh human acceptance for affected criteria.", deref(parent.Key)))
		}
	} else {
		steps = append(steps, transferCloseReview(res))
	}
	return append(steps, "Reuse implementation commits only through ordinary Git with their original attribution. The replacement has no branch, review, checks, attestations, evidence results, or completion basis; run fresh checks and review.", "Tiller changed records only: no Git refs or host PRs were changed or checked through a host API. Verify the old review is unmerged before follow-through; if it has merged, stop, sync, and use forward work. Cancellation neither removes aggregate commits nor revokes host approvals. Report failed external follow-through as incomplete; it does not roll back this committed transaction.")
}

func transferCloseReview(res TransferResult) string {
	if res.Original.PR != nil {
		return fmt.Sprintf("Close old PR #%d without merging; open a new PR for %s %s after its fresh checks, then record it with tiller submit %s %s --pr <new-number>.", *res.Original.PR, res.Noun, res.Replacement.Key, res.Noun, res.Replacement.Key)
	}
	return fmt.Sprintf("Retain old branch/review history without merging the old review; after fresh checks open a new review for %s %s and record it with tiller submit %s %s (add --pr <new-number> when using a host PR).", res.Noun, res.Replacement.Key, res.Noun, res.Replacement.Key)
}
