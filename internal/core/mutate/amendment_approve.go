package mutate

// This file holds the amendment's approve: the single human gate that validates
// every staged operation against the live tree and then applies the whole batch
// (cli-surface.md, "Large changes: amendments"; artifact-schema.md, Amendments).
// Staging (amendment.go) writes proposals and executes nothing, so this is the
// only code path that performs a reshape, and the only writer of the supersedes
// link at either grain.
//
// It runs in two phases, and the split is the whole promise:
//
//	Phase 1 resolves every operation against the tree and evaluates every
//	precondition, writing NOTHING. It collects every stale precondition rather
//	than the first, so a human fixes the batch in one pass, and answers the
//	amendment_preconditions_stale blocked answer (exit 1) naming each one.
//
//	Phase 2 applies the batch in the order the brief fixes: the add operations
//	materialize first, so a supersedes link naming a staged key has a node to
//	point at; then supersessions, then cancels, then the basis re-pins (revise,
//	then correct-eval's completion-record re-pin); and the amendment's own
//	status flips last.
//
// ATOMICITY, honestly. Phase 2 plans every file and every event and hands them
// to one store.Tree.Commit, which takes the repository mutation lock, journals a
// preimage of every destination, writes each file atomically, and rolls the
// whole set back if any single write fails. So the batch is one transaction
// against concurrent Tiller mutations and against its own write errors, not a
// per-file sequence a reader can catch halfway.
//
// What it is NOT is a two-phase commit against the machine dying mid-rename.
// After a kill the lock file is still there, because the release runs in a defer
// nothing gets to run and nothing else reaps it, so the next mutation refuses
// mutation_locked and says another mutation is in progress; only once a human
// removes the lock does the retained journal surface, refusing
// mutation_recovery_needed. Both refusals stop work rather than proceeding over
// an unknown state, and neither one repairs anything by itself.
//
// The amendment's status write is placed LAST in the transaction, after the node
// files and after the event files (applyAmendment does that explicitly rather
// than leaving the events to commitMutation's append). So the one half-state a
// kill can leave is operations landed under an amendment still proposed, and the
// recovery contract for it is: re-approve. The gate runs again, and every
// operation that already landed refuses loudly rather than double-applying. An
// add whose node now exists refuses key_exists (it does not fold into the
// idempotent no-op `tiller add` gives) — EXCEPT add-question, whose own
// idempotency rule (question.go's foldQuestion, the same rule `tiller add
// question` itself follows) is a legal fold rather than a refusal: a
// re-approve landing on a statement it, or a prior interrupted run, already
// recorded reports AppliedOperation.Outcome as AppliedNoChange (or
// AppliedUpdated, when blocks this run staged were genuinely new) rather
// than either a refusal or a silent claim of work that never happened; a
// cancel of an already-cancelled node refuses already_cancelled (the
// lifecycle engine would read a second cancel as an idempotent no-op, which
// is right for a human typing the verb twice and wrong for a batch that must
// never report work it did not do); a supersession and a basis re-pin are
// genuinely idempotent and simply land the same values again. Preconditions
// the first pass consumed (a target_status that has since moved) refuse in
// phase 1 before any of that.

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// Event kinds the apply records. amendment_approved is the gate's own event,
// named literally like the other three amendment events (amendment.go);
// superseded is the target-side record of a supersession, the one operation
// whose event is about a node the write does not touch. A cancel and a revise
// record cancel.<noun> and revise.<noun>, the dotted style the rest of the
// event stream uses, and a materialized node keeps the add.<noun> event
// `tiller add` itself would have written, so a reader sees one shape per fact.
const (
	eventAmendmentApproved = "amendment_approved"
	eventSuperseded        = "superseded"
	eventRevisePrefix      = "revise."
	eventCancelPrefix      = "cancel."
	eventCorrectEvalPrefix = "correct-eval."
)

// ReasonAmendmentPreconditionsStale is the reason token the CLI renders on the
// stale-precondition blocked answer. It lives here rather than as a literal in
// the CLI so the two cannot drift apart.
const ReasonAmendmentPreconditionsStale = "amendment_preconditions_stale"

// ReasonDuplicateStagedKey and ReasonStagedEpicReplacement are
// resolveAmendment's own two shape-only refusal tokens (below), exported the
// same way ReasonAmendmentPreconditionsStale is: EvaluateAmendmentShape
// (amendment_validity.go, #57) reuses them verbatim as its own Kind values,
// so a hand-blanked batch's live pre-flight at show and its refusal at
// approve name the identical fact with the identical word rather than two
// vocabularies for one thing.
const (
	ReasonDuplicateStagedKey    = "duplicate_staged_key"
	ReasonStagedEpicReplacement = "staged_epic_replacement"
)

// ApproveResult is the outcome of the gate: the amendment, now terminal, and
// what each staged operation did. Applied is in STAGED order (the order the
// human reviewed), not the order phase 2 executed, so a reader can lay it
// beside the amendment's own operations list one for one.
type ApproveResult struct {
	ID      string
	Key     string
	Status  string
	Applied []AppliedOperation
}

// AppliedOperation is one staged operation and the node it acted on. For an add
// it names the node that was materialized; for the other three, the target.
// Status is the node's status after the operation, so a cancel reports
// cancelled and a supersede reports the target's untouched done. Replacement is
// set on a supersede and on a cancel that carried a superseded_by, and carries
// the resolved node id even where the amendment staged a key.
//
// Outcome exists for add-question alone, and so does Note apart from
// correct-eval's commit hint (below); both are blank for the other kinds. Every other add refuses key_exists before materialize ever
// populates this struct for it (an add whose node already exists never
// reaches here), and supersede/cancel/revise always genuinely mutate
// something, so "was this really applied" never comes up for them. add-question
// is different: its own idempotency rule (question.go's foldQuestion) folds a
// same-statement duplicate rather than refusing, and a fold onto a closed
// question, or an open one already holding every staged block, changes
// NOTHING at all. Reporting that as if it landed would be exactly the lie
// this file's own recovery contract refuses everywhere else (this file's
// header comment); Outcome is how a genuine no-op still shows up here
// honestly, and Note explains a staged Blocks or Source that never reached
// the ledger. That happens on two different outcomes, not one: on a genuine
// no-op (AppliedNoChange) neither Blocks nor Source lands; on a real union
// (AppliedUpdated) Blocks DOES land, but a staged Source still never does,
// because foldQuestion's own union arm (question.go) never writes it either
// way — a question's recorded provenance is fixed at the statement that
// first minted it, and a later fold's own citation is not a second origin.
// Note names that drop on both outcomes so it is never silent. Note has one
// other writer: an applied correct-eval carries its commit hint there
// (commitHint, amendment_correct_eval.go), since its new pin names a blob git
// does not hold until the corrected feature.eval.md is committed.
type AppliedOperation struct {
	Index       int
	Op          string
	Noun        string
	NodeID      string
	Key         string
	Status      string
	Replacement string
	Outcome     string
	Note        string
	Move        *store.FeatureMovePreconditions
}

// The three outcomes AppliedOperation.Outcome ever carries, add-question's
// own three-way report of what its materialize actually did (mirroring
// QuestionResult.Created/Changed, question.go): AppliedCreated for a fresh
// Q- id, AppliedUpdated for a same-statement fold that unioned new blocks
// into an existing open entry, AppliedNoChange for a fold that landed
// nothing at all. Every other op kind leaves Outcome blank (the struct's own
// doc comment).
const (
	AppliedCreated  = "created"
	AppliedUpdated  = "updated"
	AppliedNoChange = "no_change"
)

// StalePrecondition is one staged operation's precondition that no longer holds
// against the live tree. Target names the operation's target where it has one
// and its staged key where it does not (the add operations), so every entry
// says what it is about. Note carries the cases where Actual alone would be
// misleading: a target that records no anchor to compare, a pinned file that is
// not in the working tree, a precondition naming something its operation has
// none of.
type StalePrecondition struct {
	Index        int
	Op           string
	Target       string
	Precondition string
	Expected     string
	Actual       string
	Note         string
}

// AmendmentPreconditionsStaleError reports that approve refused because at
// least one staged precondition no longer holds. It carries EVERY stale
// precondition, never only the first: the whole point of staging a batch is
// reviewing it as one, so a human repairing it must be able to see all of it.
//
// Nothing was written when this is returned, which is the guarantee the
// operations themselves lean on: a refused approval leaves the amendment
// proposed and the tree exactly as it was.
//
// Like the other blocked answers in this package it is exit 1, rendered through
// the CLI's emitReport as an ok:false data block, and it deliberately does not
// implement ExitCode, because the report sets that.
type AmendmentPreconditionsStaleError struct {
	AmendmentID string
	Stale       []StalePrecondition
}

func (e *AmendmentPreconditionsStaleError) Error() string {
	names := make([]string, 0, len(e.Stale))
	for _, s := range e.Stale {
		names = append(names, fmt.Sprintf("operation %d (%s) %s", s.Index, s.Op, s.Precondition))
	}
	return fmt.Sprintf("amendment %s has %d stale precondition(s): %s",
		e.AmendmentID, len(e.Stale), strings.Join(names, ", "))
}

// ApproveAmendment is the human gate. It refuses an amendment that is not
// proposed (a misuse: approved and withdrawn are terminal), validates the
// staged batch against the live tree without writing, and then applies it.
//
// The actor rule lives at the CLI layer, which is what enforces that a human
// ran this; mutate enforces state alone. That division is deliberate and is
// what lets the amendment's apply perform a cancel: routing the cancel through
// here rather than through the human-only cancel verb keeps that verb an
// execution path, and the amendment's approve IS the human act (cli-surface.md,
// "Large changes: amendments").
//
// Order of judgment: state, then shape, then resolution and the operation-level
// requirements a precondition cannot express (a cancel with no reason, a target
// that has vanished, a replacement naming nothing), then the amendment's own
// source honesty (checkSource, an intake-minted amendment only) and, right
// beside it, every operation's own citation (checkOpSources, the same
// amendment-only gate, and the approve-side re-assertion of the rule
// StageOperation already enforces at stage time), then every precondition, the
// prospective lineage graph, intrinsic target-state rules, a correct-eval's
// judgment of its corrected contract against the verified one
// (judgeEvalCorrections, amendment_correct_eval.go), and last the file
// requirement a precondition CAN express but need not have been staged for (a
// revise whose basis files are not in the working tree, checkReviseTargets).
//
// Shape and resolution come first for the reason Transition puts its own reason
// check first: they cost no comparison work, and answering "your batch is
// malformed" is more useful than answering "precondition 3 moved" to a caller
// whose batch could not have run either way. checkSource runs right before
// preconditions rather than after, so a caller whose whole batch traces to a
// feedback document that has since moved learns that first, ahead of working
// through a stale-precondition report against a foundation that already
// changed under it (docs/plans/2026-08-11-intake-design.md: "The fingerprint
// also makes post-approval edits to the document visible as the same class of
// fact drift already reports elsewhere"). Every structural graph and
// target-state gate follows preconditions so a staged target_status guard keeps
// its richer exit-1 stale answer when the target moved. The revise check comes
// last for the mirror-image reason: a human who staged an eval_revision guard
// over that very file has already asked for the richer exit-1 answer naming
// expected and actual, and the check is only the backstop for a batch that
// staged no such guard.
func ApproveAmendment(tree *store.Tree, deps Deps, amendment *store.Amendment) (ApproveResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return ApproveResult{}, err
	}

	doc, relYml, snapshot, err := loadAmendmentDoc(tree, amendment)
	if err != nil {
		return ApproveResult{}, err
	}
	if doc.Status != store.AmendmentStatusProposed {
		return ApproveResult{}, amendmentNotOpenErr(doc)
	}
	// The same backstop StageOperation runs: an amendment reached by a hand edit
	// rather than by staging must still be a well-formed batch before anything
	// is applied from it.
	if err := store.ValidateAmendment(doc); err != nil {
		return ApproveResult{}, err
	}

	rev := deps.rev(tree.Root)

	plan, err := resolveAmendment(tree, doc)
	if err != nil {
		return ApproveResult{}, err
	}
	b := newBatch()
	// checkSource sits beside checkPreconditions, and runs first: see
	// ApproveAmendment's own "Order of judgment" paragraph above for why.
	if err := checkSource(tree, rev, doc); err != nil {
		return ApproveResult{}, err
	}
	// checkOpSources runs right after: checkSource judges the amendment's OWN
	// citation, this judges each operation's, and a batch whose foundation
	// document has drifted should hear about that before a narrower per-op
	// gap (#54).
	if err := checkOpSources(doc); err != nil {
		return ApproveResult{}, err
	}
	// One batch spans both phases. Phase 1 needs the CURRENT status and anchor of
	// every target, not the ones the tree carried when it was loaded, and the
	// batch is what loads a node's own yml fresh (and once). Carrying it forward
	// into phase 2 is not an optimization: it means the bytes a precondition was
	// compared against and the bytes the write is planned over are the same read,
	// so a target that moves between the two phases cannot slip through.
	if err := checkPreconditions(tree, b, rev, doc, plan); err != nil {
		return ApproveResult{}, err
	}
	if err := checkFeatureMoves(tree, b, doc); err != nil {
		return ApproveResult{}, err
	}
	if err := checkSupersessionGraph(tree, b, plan); err != nil {
		return ApproveResult{}, err
	}
	if err := checkAmendmentTargetStates(tree, b, plan); err != nil {
		return ApproveResult{}, err
	}
	if err := b.judgeEvalCorrections(tree, rev, doc, plan); err != nil {
		return ApproveResult{}, err
	}
	if err := checkReviseTargets(tree, plan); err != nil {
		return ApproveResult{}, err
	}
	return applyAmendment(tree, deps, owner, rev, b, doc, relYml, snapshot, plan)
}

// resolvedOp is one staged operation with everything the live tree can settle
// about it: the node a revise, supersede, or cancel targets, and the
// replacement a supersede's by or a cancel's superseded_by names, which is
// either a live node or the KEY of an add operation in this same batch, whose
// id does not exist until phase 2 materializes it.
type resolvedOp struct {
	index       int
	op          store.AmendmentOp
	target      store.Node
	replace     store.Node
	replaceKey  string
	replaceNoun string
}

// resolveAmendment resolves every staged operation against the live tree and
// checks the requirements no precondition covers. It writes nothing: this is
// still phase 1, and a batch that cannot be resolved is refused as a misuse
// (exit 2) rather than reported as staleness, because a vanished target or an
// unresolvable replacement is not a comparison that came out false.
func resolveAmendment(tree *store.Tree, doc *store.Amendment) ([]resolvedOp, error) {
	// Deliberately not store.AmendmentOpAddQuestion here, and not an
	// oversight: this map is keyed by op.Key, which every add-question op
	// leaves blank (a question has no key at all). Recognizing it would map
	// every staged add-question to the same key "" and refuse the SECOND one
	// as duplicate_staged_key, even though two questions with different
	// statements in one batch are perfectly legal and unrelated. A question's
	// own notion of "duplicate" is its statement, and foldQuestion's
	// idempotency rule (question.go) already handles that correctly without
	// this map's help (task-3-brief.md: "reason it through, don't assume").
	//
	// This map DOES include store.AmendmentOpAddEpic, unlike
	// resolveStageReplacement's own staged-key loop (amendment.go): the
	// duplicate-key check just below needs every add op's key, epics
	// included. That is bookkeeping only, not a rescue: the replacement
	// resolution further down (raw's switch, below) still refuses a staged
	// epic key the same way staging does, naming it explicitly
	// (stagedEpicReplacementErr) rather than falling through to this map's
	// generic staged[raw] != "" arm.
	staged := map[string]string{}
	for i, op := range doc.Operations {
		var noun string
		switch op.Op {
		case store.AmendmentOpAddEpic:
			noun = store.NounEpic
		case store.AmendmentOpAddFeature:
			noun = store.NounFeature
		case store.AmendmentOpAddTask:
			noun = store.NounTask
		default:
			continue
		}
		// duplicateStagedKey (below) is the extracted predicate
		// EvaluateAmendmentShape (amendment_validity.go, #57) shares: this call
		// site tests it and refuses immediately (phase 1's first-offender
		// posture), the evaluator collects every index it holds for instead.
		// It is provably equivalent to the old inline "_, exists :=
		// staged[op.Key]" check it replaces: staged, at this point in THIS
		// loop, holds exactly the keys set by operations 0..i-1 (the loop
		// returns the moment a duplicate is found, so it never advances past
		// one), which is precisely what duplicateStagedKey's own backward scan
		// over doc.Operations[0:i] recomputes.
		if duplicateStagedKey(doc, i, op.Key) {
			return nil, &usageError{
				reason: ReasonDuplicateStagedKey,
				msg: fmt.Sprintf("operation %d stages the key %q a second time; one amendment materializes each key once",
					i, op.Key),
			}
		}
		staged[op.Key] = noun
	}

	out := make([]resolvedOp, 0, len(doc.Operations))
	for i, op := range doc.Operations {
		r := resolvedOp{index: i, op: op}

		// The same check staging runs, run again here rather than trusted from
		// there. store.ValidateAmendment judges an operation's fields and not its
		// precondition map, so an amendment reached by a hand edit could carry a
		// name nothing evaluates, and a precondition nobody compares is worse
		// than no precondition: it reads on the review surface as a guard that
		// held.
		if err := validatePreconditions(op.Preconditions); err != nil {
			return nil, err
		}

		switch op.Op {
		case store.AmendmentOpAddEpic, store.AmendmentOpAddFeature, store.AmendmentOpAddTask:
			// Nothing to resolve: the key IS the node, and it does not exist yet.
		case store.AmendmentOpAddQuestion:
			// Nothing to resolve either, for a different reason: a question
			// carries no key or target at all (its identity is its
			// statement), so there is no live node to classify against.
			// Without this arm the op falls to default below and
			// classifyStageTarget(tree, "") refuses unknown_target on
			// every staged add-question, the same bug add-epic's own arm
			// fixed one line up (task-2's review finding).
		default:
			n, isEpic := classifyStageTarget(tree, op.Target)
			if isEpic {
				return nil, epicTargetErr()
			}
			if n == nil {
				return nil, unknownTargetErr(op.Target)
			}
			r.target = n
		}

		if op.Op == store.AmendmentOpCancel && strings.TrimSpace(op.Reason) == "" {
			return nil, &usageError{
				reason: "missing_reason",
				msg: fmt.Sprintf("operation %d (cancel) carries no reason; a cancelled node with no reason is a dead branch nobody can explain",
					i),
			}
		}
		if op.Op == store.AmendmentOpSupersede && strings.TrimSpace(op.By) == "" {
			return nil, supersedeByRequiredErr()
		}

		// ValidateAmendment has already refused by on anything but a supersede and
		// superseded_by on anything but a cancel, so at most one of the two is set.
		raw := op.By
		if op.SupersededBy != "" {
			raw = op.SupersededBy
		}
		if raw != "" {
			n, isEpic := classifyStageTarget(tree, raw)
			switch {
			case isEpic:
				return nil, epicTargetErr()
			case n != nil:
				r.replace = n
				r.replaceNoun = n.Noun()
			case stagedEpicKey(doc, raw):
				// stagedEpicKey (below) is the extracted predicate
				// EvaluateAmendmentShape (amendment_validity.go, #57) shares:
				// this call site tests it in place of the old inline
				// "staged[raw] == store.NounEpic" it replaces, with zero
				// behavioral delta (stagedEpicKey's own doc comment proves
				// the equivalence). Mirrors resolveStageReplacement's own
				// deliberate exclusion (amendment.go: "Deliberately not
				// store.AmendmentOpAddEpic here"): a supersede or a cancel
				// only ever targets a feature or a task, so a staged epic's
				// key is never a legal replacement in ANY position, and the
				// arm below would otherwise resolve it anyway and accept a
				// shape staging itself refuses at stage time (#48).
				return nil, stagedEpicReplacementErr(i, raw)
			case staged[raw] != "":
				r.replaceKey = raw
				r.replaceNoun = staged[raw]
			default:
				return nil, unknownTargetErr(raw)
			}
			// The same guard staging applies, applied again for the same reason
			// the precondition names are re-validated above: a hand-edited
			// amendment could name a node as its own replacement, and applying it
			// would write a node superseded by itself, which drift then exempts
			// forever on a decision nobody made.
			if r.replace != nil && r.target != nil && r.replace.NodeID() == r.target.NodeID() {
				return nil, selfSupersessionErr(r.target.NodeID())
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// stagedEpicReplacementErr refuses a supersede's --by or a cancel's
// --superseded-by that names a same-batch staged add-epic key. It is the
// approve-side named refusal for the rule resolveStageReplacement
// (amendment.go) already enforces at stage time: a supersede or a cancel only
// ever targets a feature or a task, so a staged epic can never be the
// replacement, and letting the staged map's own add-epic bookkeeping rescue
// one here would accept a shape staging itself refuses (#48). It is op-indexed
// like reviseTargetIncompleteErr, below, because phase 1 resolves a whole
// batch at once and the offending operation is not necessarily the one that
// staged the epic.
func stagedEpicReplacementErr(index int, key string) error {
	return &usageError{
		reason: ReasonStagedEpicReplacement,
		msg: fmt.Sprintf("operation %d names %q, a staged add-epic key, as its replacement; a supersede or a cancel only ever targets a feature or a task, and an epic reshapes through its features",
			index, key),
	}
}

// duplicateStagedKey reports whether an EARLIER add-epic, add-feature, or
// add-task operation in doc.Operations (any index below index) already
// stages key. It is resolveAmendment's own duplicate-key predicate (above),
// extracted so EvaluateAmendmentShape (amendment_validity.go, #57) shares
// ONE definition of the fact rather than a second copy of the loop:
// resolveAmendment tests it and refuses on the first index it holds for,
// phase 1's first-offender posture; the evaluator collects every one
// instead, the same evaluator-beside-checker split EvaluateAmendmentOpSources
// already keeps with checkOpSources.
//
// Deliberately excludes add-question, mirroring the exclusion resolveAmendment
// itself has always kept for the identical reason (this function's own call
// site, above): op.Key is always blank on an add-question, so recognizing it
// here would map every staged add-question to the same key "" and flag the
// second one, even though two questions with different statements in one
// batch are unrelated (a question's own notion of "duplicate" is its
// statement, and foldQuestion's idempotency rule, question.go, already
// handles that correctly without this predicate's help).
func duplicateStagedKey(doc *store.Amendment, index int, key string) bool {
	for i, op := range doc.Operations {
		if i >= index {
			return false
		}
		switch op.Op {
		case store.AmendmentOpAddEpic, store.AmendmentOpAddFeature, store.AmendmentOpAddTask:
			if op.Key == key {
				return true
			}
		}
	}
	return false
}

// stagedEpicKey reports whether ANY add-epic operation in doc.Operations
// stages key, at any index. It is resolveAmendment's own replacement-check
// predicate (above), extracted so EvaluateAmendmentShape
// (amendment_validity.go, #57) shares this exact definition rather than
// inventing its own.
//
// Order-independent, unlike duplicateStagedKey just above — and this is the
// one place these two predicates genuinely differ, not an oversight: a
// duplicate's OFFENDER is whichever occurrence comes second, so "before
// index" is what the fact IS; a staged epic key, by contrast, is NEVER a
// legal supersede/cancel replacement (resolveStageReplacement's own
// deliberate exclusion, amendment.go) in any position, so nothing about
// "which occurrence" changes whether it is a defect, only whether it exists
// in the doc at all.
//
// This is provably the SAME fact the inline check it replaces
// (staged[raw] == store.NounEpic) always computed, not a widened one: staged
// is built, in the loop above, over doc.Operations in full BEFORE this
// predicate's own call site (the resolution loop) ever runs, so
// "staged[raw] == store.NounEpic" already meant "some add-epic anywhere in
// the doc stages raw" — only an add-epic operation ever writes NounEpic into
// that map, and the very same loop above refuses duplicate_staged_key before
// resolution ever begins whenever any key (epic or not) is staged twice, so
// no key the resolution loop ever sees was written more than once. Scanning
// doc.Operations directly here, instead of consulting resolveAmendment's own
// staged bookkeeping map, is what lets EvaluateAmendmentShape (which builds
// no such map) share the identical definition.
func stagedEpicKey(doc *store.Amendment, key string) bool {
	for _, op := range doc.Operations {
		if op.Op == store.AmendmentOpAddEpic && op.Key == key {
			return true
		}
	}
	return false
}

func checkSupersessionGraph(tree *store.Tree, b *batch, plan []resolvedOp) error {
	if !hasSupersessionEdge(plan) {
		return nil
	}

	epicsDir := filepath.Join(tree.Root, ".tiller", "epics")
	inventory, err := store.ReadLineageInventory(tree.Root, epicsDir)
	if err != nil {
		return err
	}
	b.guards = append(b.guards, store.PlanLineageInventoryGuard(epicsDir, inventory))

	loaded, err := loadedLineageNodesByPath(tree)
	if err != nil {
		return err
	}
	next := make(map[string]string)
	claimants := make(map[string][]string)
	edges := make(map[string]map[string]struct{})
	for _, file := range inventory.Files {
		if file.Noun == store.NounEpic {
			continue
		}
		rel := filepath.Join(".tiller", "epics", file.Path)
		var current *nodeEdit
		if node := loaded[rel]; node != nil {
			current, err = b.editFor(tree, node)
		} else {
			current, err = b.lineageEditAt(tree, rel, file.Noun)
		}
		if err != nil {
			return err
		}
		seedSupersessionGraph(next, claimants, edges, current.doc)
	}

	stagedEdges := make(map[string]string)
	var overlaid []resolvedOp
	for _, r := range plan {
		if r.target == nil ||
			(r.op.Op != store.AmendmentOpSupersede && r.op.Op != store.AmendmentOpCancel) ||
			(r.replace == nil && r.replaceKey == "") {
			continue
		}

		replacementID := r.replaceKey
		if r.replace != nil {
			replacementID = r.replace.NodeID()
		}
		targetID := r.target.NodeID()
		if r.target.Noun() != r.replaceNoun {
			return &usageError{
				reason: "supersession_noun_mismatch",
				msg: fmt.Sprintf("operation %d cannot record supersession from %s %s to %s %s: target and replacement must have the same noun",
					r.index, r.target.Noun(), targetID, r.replaceNoun, replacementID),
			}
		}
		if firstReplacement, exists := stagedEdges[targetID]; exists {
			return &usageError{
				reason: "supersession_target_reused",
				msg: fmt.Sprintf("operation %d cannot replace target %s with %s: this amendment already replaces it with %s",
					r.index, targetID, replacementID, firstReplacement),
			}
		}
		stagedEdges[targetID] = replacementID

		if existing := next[targetID]; existing != "" && existing != replacementID {
			return supersessionConflictErr(r, existing, replacementID)
		}
		var conflictingClaimant string
		for _, claimant := range claimants[targetID] {
			if claimant != replacementID &&
				(conflictingClaimant == "" || claimant < conflictingClaimant) {
				conflictingClaimant = claimant
			}
		}
		if conflictingClaimant != "" {
			return supersessionConflictErr(r, conflictingClaimant, replacementID)
		}

		if r.replace != nil {
			addSupersessionEdge(edges, targetID, replacementID)
			overlaid = append(overlaid, r)
		}
	}

	for _, r := range overlaid {
		if revisited, cycle := supersessionCycleFrom(r.target.NodeID(), edges); cycle {
			return &usageError{
				reason: "supersession_cycle",
				msg: fmt.Sprintf("operation %d cannot replace target %s with %s: prospective lineage revisits %s and creates a cycle",
					r.index, r.target.NodeID(), r.replace.NodeID(), revisited),
			}
		}
	}
	return nil
}

func hasSupersessionEdge(plan []resolvedOp) bool {
	for _, r := range plan {
		if r.target == nil || (r.replace == nil && r.replaceKey == "") {
			continue
		}
		if r.op.Op == store.AmendmentOpSupersede || r.op.Op == store.AmendmentOpCancel {
			return true
		}
	}
	return false
}

func loadedLineageNodesByPath(tree *store.Tree) (map[string]store.Node, error) {
	nodes := make(map[string]store.Node, len(tree.Features)+len(tree.Tasks))
	for _, feature := range tree.Features {
		if feature.Key == nil {
			return nil, fmt.Errorf("mutate: feature %s has no canonical key", feature.ID)
		}
		rel := filepath.Join(featureRelDir(feature), "feature.yml")
		nodes[rel] = feature
	}
	for _, task := range tree.Tasks {
		feature := tree.Features[task.FeatureID]
		if feature == nil || feature.Key == nil || task.Key == nil {
			return nil, fmt.Errorf("mutate: task %s has no resolvable state path", task.ID)
		}
		rel := filepath.Join(
			store.TaskRelDir(feature.EpicKey, deref(feature.Key), deref(task.Key)),
			store.TaskStateFile,
		)
		nodes[rel] = task
	}
	return nodes, nil
}

func seedSupersessionGraph(next map[string]string, claimants map[string][]string,
	edges map[string]map[string]struct{}, n store.Node) {

	supersedes, supersededBy := supersessionLinks(n)
	if supersededBy != "" {
		next[n.NodeID()] = supersededBy
		addSupersessionEdge(edges, n.NodeID(), supersededBy)
	}
	for _, targetID := range supersedes {
		claimants[targetID] = append(claimants[targetID], n.NodeID())
		addSupersessionEdge(edges, targetID, n.NodeID())
	}
}

func addSupersessionEdge(edges map[string]map[string]struct{}, targetID, replacementID string) {
	if edges[targetID] == nil {
		edges[targetID] = make(map[string]struct{})
	}
	edges[targetID][replacementID] = struct{}{}
}

func supersessionCycleFrom(start string, edges map[string]map[string]struct{}) (string, bool) {
	visiting := make(map[string]struct{})
	complete := make(map[string]struct{})
	var walk func(string) (string, bool)
	walk = func(current string) (string, bool) {
		if _, exists := visiting[current]; exists {
			return current, true
		}
		if _, exists := complete[current]; exists {
			return "", false
		}
		visiting[current] = struct{}{}
		replacements := make([]string, 0, len(edges[current]))
		for replacement := range edges[current] {
			replacements = append(replacements, replacement)
		}
		sort.Strings(replacements)
		for _, replacement := range replacements {
			if revisited, cycle := walk(replacement); cycle {
				return revisited, true
			}
		}
		delete(visiting, current)
		complete[current] = struct{}{}
		return "", false
	}
	return walk(start)
}

func supersessionLinks(n store.Node) (supersedes []string, supersededBy string) {
	switch v := n.(type) {
	case *store.Feature:
		if v.SupersededBy != nil {
			supersededBy = *v.SupersededBy
		}
		return v.Supersedes, supersededBy
	case *store.Task:
		if v.SupersededBy != nil {
			supersededBy = *v.SupersededBy
		}
		return v.Supersedes, supersededBy
	default:
		return nil, ""
	}
}

func supersessionConflictErr(r resolvedOp, existingReplacement, proposedReplacement string) error {
	return &usageError{
		reason: "supersession_conflict",
		msg: fmt.Sprintf("operation %d cannot replace target %s with %s: existing lineage names replacement %s, not %s",
			r.index, r.target.NodeID(), proposedReplacement, existingReplacement, proposedReplacement),
	}
}

// checkSource is phase 1's provenance guard, beside checkPreconditions: an
// intake-minted amendment's own feedback document (doc.Source) must still
// read, on the live working tree, as what intake fingerprinted when it
// staged this amendment. A hand-opened amendment carries no Source at all
// (doc.Source is nil) and this is a pure pass-through, the same nil-means-
// "not intake-minted" convention every other Source-conditioned check in
// this package already follows (StageOperation's own sourcelessOpErr guard,
// and checkOpSources just below, this function's per-operation
// counterpart).
//
// Re-hashing here rather than writing is what keeps this inside phase 1's
// no-writes promise, the identical posture checkPreconditions keeps one
// function down.
//
// A changed document and a vanished one refuse distinctly (sourceChangedErr
// vs sourceVanishedErr): a rewritten artifact still names something to
// compare hashes against and the refusal carries both, but a deleted one has
// no live hash to report at all, and folding it into the "recorded X, live
// Y" shape would print a blank live value implying a comparison that never
// actually ran.
func checkSource(tree *store.Tree, rev vcs.Revisions, doc *store.Amendment) error {
	if doc.Source == nil {
		return nil
	}
	sd, err := evalSource(tree, rev, doc.Source)
	if err != nil {
		return err
	}
	if sd == nil {
		return nil
	}
	if sd.Vanished {
		return sourceVanishedErr(doc.Source.Path)
	}
	return sourceChangedErr(doc.Source.Revision, sd.Actual)
}

// checkOpSources is checkSource's per-operation counterpart (#54): when an
// amendment is intake-minted (doc.Source set), StageOperation already refuses
// a blank --source at stage time (sourcelessOpErr, amendment.go), but that
// gate only runs on the ordinary staging path. A hand edit reaches
// ApproveAmendment without ever passing through StageOperation (writeAmendmentOps
// in this package's own tests stands in for a human editing the amendment's
// yml directly, the same carve-out ValidateAmendment's own re-check at the top
// of ApproveAmendment exists for), so an operation whose citation was dropped
// by the edit would otherwise materialize uncited and the provenance chain
// intake exists to guarantee would have a tamper path. A hand-opened amendment
// carries no Source at all (doc.Source is nil) and this is a pure pass-through,
// the same convention checkSource follows one function up.
//
// It refuses the FIRST sourceless operation by index rather than collecting
// every one: unlike checkPreconditions, which must report a whole batch's
// staleness in one pass because a human is reviewing preconditions they
// themselves staged, a dropped citation is not something a batch review turns
// up more than one of in the ordinary case, and stagedEpicReplacementErr and
// reviseTargetIncompleteErr already take this same first-offender posture for
// the same reason.
func checkOpSources(doc *store.Amendment) error {
	if doc.Source == nil {
		return nil
	}
	for i, op := range doc.Operations {
		if strings.TrimSpace(op.Source) == "" {
			return opSourceRequiredErr(i, op.Op)
		}
	}
	return nil
}

// opSourceRequiredErr refuses one operation, named by index, that carries no
// source on an intake-minted amendment: checkOpSources' own named refusal, in
// the file's op-indexed idiom (stagedEpicReplacementErr, reviseTargetIncompleteErr,
// above). The message teaches the same thing sourcelessOpErr teaches at stage
// time (amendment.go) without repeating its wording verbatim, since this
// refusal is reached only through a hand edit staging itself never permits and
// names which operation the edit left uncited.
func opSourceRequiredErr(index int, op string) error {
	return &usageError{
		reason: "op_source_required",
		msg: fmt.Sprintf("operation %d (%s) carries no source; intake amendments trace every operation to the feedback document, and a hand edit cannot leave one uncited",
			index, op),
	}
}

// sourceDrift is evalSource's verdict: nil when the artifact's live hash
// still matches what intake recorded (holds, the same nil-means-valid
// contract StalePrecondition's own sibling evalPrecondition returns), non-nil
// when it does not. It is the VERDICT, not the raw hash pair: both
// checkSource and EvaluateAmendmentSource read this one struct rather than
// each re-deriving vanished-vs-changed from fresh.Value themselves, which is
// what makes "the two surfaces can never disagree" true structurally rather
// than by two independently-written comparisons that happen to agree today.
type sourceDrift struct {
	// Vanished is true when the artifact is simply gone from the working
	// tree, false when it is still there with different content. The two
	// refuse (and render) distinctly: a vanished document has no live hash
	// to report, so Actual stays empty and Note explains why, rather than a
	// blank Actual standing in for a comparison that never ran.
	Vanished bool
	Actual   string
	Note     string
}

// evalSource is checkSource's and EvaluateAmendmentSource's shared
// comparison (amendment_validity.go), the Source-side twin of
// evalPrecondition a few functions down: it re-hashes source's own artifact
// against the live working tree through CompareWorktreeBlob, in the same
// checkin-hash space WorktreeBlobRef gave intake when it pinned
// Source.Revision, so approve's refusal and show's live read can never
// quietly disagree about whether a citation still traces to what intake
// fingerprinted (evalPrecondition's own doc comment
// states the identical promise for the five staged preconditions — this is
// the same one-computation shape: a single verdict returned, not a hash pair
// each caller judges over again).
func evalSource(tree *store.Tree, rev vcs.Revisions, source *store.AmendmentSource) (*sourceDrift, error) {
	vanished, changed, err := CompareWorktreeBlob(rev, tree.Root, source.Path, source.Revision)
	if err != nil {
		return nil, err
	}
	if vanished {
		return &sourceDrift{
			Vanished: true,
			Note:     "the feedback document no longer exists in the working tree",
		}, nil
	}
	if !changed {
		return nil, nil
	}
	fresh, err := WorktreeBlobRef(rev, tree.Root, source.Path)
	if err != nil {
		return nil, err
	}
	return &sourceDrift{Actual: fresh.Value}, nil
}

// sourceChangedErr refuses approve when an intake-minted amendment's own
// feedback document has been edited since intake staged it. The message is
// pinned by task-5-brief.md, "the drift-idiom message", both hashes named so
// a human can tell at a glance whether the edit was theirs.
func sourceChangedErr(recorded, live string) error {
	return &usageError{
		reason: "source_changed",
		msg: fmt.Sprintf(
			"the feedback document changed since intake staged this amendment (recorded %s, live %s); re-read it and re-stage, or withdraw",
			recorded, live),
	}
}

// sourceVanishedErr refuses approve when an intake-minted amendment's own
// feedback document is no longer in the working tree at all: not a content
// edit but the artifact itself gone, so there is no live hash to compare
// (checkSource's own doc comment states why this is a distinct message
// rather than sourceChangedErr with a blank live value).
func sourceVanishedErr(path string) error {
	return &usageError{
		reason: "source_vanished",
		msg: fmt.Sprintf(
			"the feedback document %s no longer exists; restore it, re-read it and re-stage, or withdraw",
			path),
	}
}

// checkPreconditions is phase 1's comparison pass. It walks the operations in
// staged order and each operation's preconditions in preconditionNames order,
// so the report a human reads is deterministic rather than a map's shuffle, and
// it evaluates all of them before answering.
//
// Loading a node's yml to read its current status is not a write, so this stays
// inside phase 1's no-writes promise; the batch it loads into is the same one
// phase 2 writes through (ApproveAmendment's own comment).
func checkPreconditions(tree *store.Tree, b *batch, rev vcs.Revisions, doc *store.Amendment, plan []resolvedOp) error {
	var stale []StalePrecondition
	for _, r := range plan {
		for _, name := range preconditionNames {
			expected, ok := r.op.Preconditions[name]
			if !ok {
				continue
			}
			sp, err := evalPrecondition(tree, b, rev, r, name, expected)
			if err != nil {
				return err
			}
			if sp != nil {
				stale = append(stale, *sp)
			}
		}
	}
	if len(stale) == 0 {
		return nil
	}
	return &AmendmentPreconditionsStaleError{AmendmentID: doc.ID, Stale: stale}
}

// checkAmendmentTargetStates enforces operation rules against the same fresh
// batch copy that checkPreconditions established as the approval authority and
// the terminal state earlier phase-2 passes would create.
func checkAmendmentTargetStates(tree *store.Tree, b *batch, plan []resolvedOp) error {
	var cancelledTargets map[string]struct{}
	for _, r := range plan {
		if r.op.Op != store.AmendmentOpCancel || r.target == nil {
			continue
		}
		if cancelledTargets == nil {
			cancelledTargets = make(map[string]struct{})
		}
		cancelledTargets[r.target.NodeID()] = struct{}{}
	}

	for _, r := range plan {
		if r.target == nil {
			continue
		}
		current, err := b.editFor(tree, r.target)
		if err != nil {
			return err
		}
		status := current.doc.NodeStatus()
		switch r.op.Op {
		case store.AmendmentOpSupersede:
			if status != statusDone {
				return &usageError{
					reason: "supersede_target_not_done",
					msg: fmt.Sprintf("operation %d cannot supersede %s %s: it is %s; supersede requires done work, use cancel --superseded-by for work that has not finished",
						r.index, current.noun, current.doc.NodeID(), status),
				}
			}
		case store.AmendmentOpRevise:
			if status != statusDone && status != statusCancelled {
				if _, cancelled := cancelledTargets[current.doc.NodeID()]; cancelled {
					status = statusCancelled
				}
			}
			if status == statusDone || status == statusCancelled {
				return &usageError{
					reason: "revise_target_terminal",
					msg: fmt.Sprintf("operation %d cannot revise %s %s: it is %s; terminal work is append-only and must be replaced with new forward work",
						r.index, current.noun, current.doc.NodeID(), status),
				}
			}
		case store.AmendmentOpCorrectEval:
			if err := checkCorrectEvalTarget(r, current); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkReviseTargets is phase 1's last gate, and it exists because a revise's
// re-pin hashes files: refreshFeatureBasis reads the target's feature.md and
// feature.eval.md, recordTaskBasis its task.md, and a file that is not in the
// working tree cannot be hashed. Left to phase 2 that answered
// `vcs: git could not resolve path .../feature.eval.md`, a raw environment error
// for a plain fact about the tree, naming neither the operation that caused it
// nor a way out (the carried defect from task 7's own drive).
//
// It is confined to the revise operation because that is the only one that
// hashes anything: an add materializes its own files, and a supersede or a
// cancel writes state and never reads prose.
//
// It runs AFTER checkPreconditions, so a batch that staged spec_revision or
// eval_revision over the same file keeps answering through that guard, whose
// exit-1 report carries expected, actual, and the "not in the working tree" note
// per operation. This is the backstop for the batch that staged neither, which is
// exactly the shape the defect was found on (a bare `--revise` with no
// preconditions at all). Nothing has been written when it refuses: phase 2 has
// not started, and no earlier phase-1 step writes.
func checkReviseTargets(tree *store.Tree, plan []resolvedOp) error {
	for _, r := range plan {
		if r.op.Op != store.AmendmentOpRevise {
			continue
		}
		if missing := missingBasisFiles(tree, r.target); len(missing) > 0 {
			return reviseTargetIncompleteErr(r.index, r.target, missing)
		}
	}
	return nil
}

// evalPrecondition compares one precondition against the live tree, answering
// nil when it holds and the stale record when it does not. An error is a tool
// failure (git could not answer, or a name nothing here compares), never a
// verdict: a comparison that cannot run must not be reported as a comparison
// that came out false.
//
// target_status and target_revision read the target's own yml FRESH off disk
// rather than the tree's loaded copy, because what the gate is asking is whether
// the batch still holds against the CURRENT tree, and a tree loaded a moment
// earlier is not that. The other three read what is current by construction: a
// content hash is taken from the working tree, and key_absent is about a folder
// the loader would have had to see.
func evalPrecondition(tree *store.Tree, b *batch, rev vcs.Revisions, r resolvedOp, name, expected string) (*StalePrecondition, error) {
	sp := StalePrecondition{
		Index:        r.index,
		Op:           r.op.Op,
		Target:       preconditionSubject(r),
		Precondition: name,
		Expected:     expected,
	}

	switch name {
	case preconditionTargetStatus:
		if r.target == nil {
			sp.Note = "the operation names no target to read a status from"
			return &sp, nil
		}
		current, err := b.editFor(tree, r.target)
		if err != nil {
			return nil, err
		}
		sp.Actual = current.doc.NodeStatus()

	case preconditionTargetRevision:
		if r.target == nil {
			sp.Note = "the operation names no target to read a revision from"
			return &sp, nil
		}
		current, err := b.editFor(tree, r.target)
		if err != nil {
			return nil, err
		}
		value, note := targetAnchor(current.doc)
		if note != "" {
			sp.Note = note
			return &sp, nil
		}
		sp.Actual = value

	case preconditionKeyAbsent:
		holder := keyHolder(tree, expected)
		if holder == nil {
			return nil, nil
		}
		sp.Actual = holder.NodeID()
		sp.Note = fmt.Sprintf("%s %s holds the key", holder.Noun(), holder.NodeID())
		return &sp, nil

	case preconditionSpecRevision, preconditionEvalRevision:
		if r.target == nil {
			sp.Note = "the operation names no target to read a spec or contract from"
			return &sp, nil
		}
		relPath, ok := basisFileFor(tree, r.target, name)
		if !ok {
			sp.Note = fmt.Sprintf("%s %s has no %s file to compare",
				r.target.Noun(), r.target.NodeID(), strings.TrimSuffix(name, "_revision"))
			return &sp, nil
		}
		// The same working-tree content hash the basis system records and
		// compares through, so a precondition and an accepted_against pin can
		// never disagree about what the current content is (transition.go's
		// WorktreeBlobRef over gitBlobRef).
		ref, err := WorktreeBlobRef(rev, tree.Root, relPath)
		if err != nil {
			return nil, err
		}
		if ref.Value == "" {
			sp.Note = relPath + " is not in the working tree"
			return &sp, nil
		}
		sp.Actual = ref.Value

	default:
		// Unreachable through staging or through resolveAmendment, both of which
		// refuse a name outside preconditionNames. It is here so that ADDING a
		// sixth name to that list without a comparison to go with it fails loudly
		// instead of reporting every use of it as a blank mismatch, which would
		// read as an honest stale answer.
		return nil, fmt.Errorf("mutate: precondition %q has no comparison in evalPrecondition", name)
	}

	if sp.Actual == expected {
		return nil, nil
	}
	return &sp, nil
}

// preconditionSubject is what a stale report names as the operation's subject:
// its target where it has one, its staged key where it does not, and for
// add-question, which has neither, its full Statement. Without this last
// fallback a precondition staged on an add-question read as "about" a blank
// string, which fails the promise every other arm keeps: that this field always
// says what the entry is about.
func preconditionSubject(r resolvedOp) string {
	if r.target != nil {
		return r.target.NodeID()
	}
	if r.op.Target != "" {
		return r.op.Target
	}
	if r.op.Key != "" {
		return r.op.Key
	}
	return r.op.Statement
}

// targetAnchor is the commit revision target_revision compares against: the
// completion record's anchor for a done target, and the accepted basis's anchor
// for anything earlier (artifact-schema.md, Accepted basis; the schema's own
// amendment example pins target_revision beside target_status: done). A target
// carrying no such anchor answers the reason it has none, so the refusal says
// "nothing to compare" rather than reporting a blank mismatch.
//
// A done TASK is the one case with no anchor by construction: the completion
// record lives at feature grain and a task's done is the merge itself
// (artifact-schema.md, Tasks), so a target_revision staged against one can
// never hold and says so.
func targetAnchor(n store.Node) (value, note string) {
	switch t := n.(type) {
	case *store.Feature:
		if t.Status == statusDone {
			if t.VerifiedAgainst.Commit.Value == "" {
				return "", fmt.Sprintf("done feature %s records no verified_against.commit anchor", t.ID)
			}
			return t.VerifiedAgainst.Commit.Value, ""
		}
		if t.AcceptedAgainst.Commit.Value == "" {
			return "", fmt.Sprintf("feature %s records no accepted_against.commit anchor", t.ID)
		}
		return t.AcceptedAgainst.Commit.Value, ""
	case *store.Task:
		if t.Status == statusDone {
			return "", fmt.Sprintf("task %s records no verified_against: a task's done is the merge", t.ID)
		}
		if t.AcceptedAgainst == nil || t.AcceptedAgainst.Commit.Value == "" {
			return "", fmt.Sprintf("task %s records no accepted_against.commit anchor", t.ID)
		}
		return t.AcceptedAgainst.Commit.Value, ""
	}
	return "", fmt.Sprintf("%s %s carries no revision anchor", n.Noun(), n.NodeID())
}

// keyHolder is the live epic, feature, or task carrying key, or nil when the
// key is free. Any status counts, cancelled included: the key names a FOLDER,
// and a cancelled node still occupies its own, so a key it holds is not
// absent and materializing over it would collide on the write rather than
// honor the precondition. Epics joined this lookup alongside features and
// tasks once add-epic gave key_absent something epic-shaped to guard
// (task-2-brief.md): an epic's key is a live node's key exactly as a
// feature's or a task's is, and the three namespaces never collide (an
// epic's key is always one segment, a feature's two, a task's three), so
// widening this check can only ever add a match, never take one away.
func keyHolder(tree *store.Tree, key string) store.Node {
	for _, e := range tree.Epics {
		if deref(e.Key) == key {
			return e
		}
	}
	for _, f := range tree.Features {
		if deref(f.Key) == key {
			return f
		}
	}
	for _, tk := range tree.Tasks {
		if deref(tk.Key) == key {
			return tk
		}
	}
	return nil
}

// basisFileFor is the working-tree file a spec_revision or eval_revision
// precondition is about: the feature's own feature.md and feature.eval.md, or a
// task's task.md for its spec leg. A task has no eval file at all (the
// acceptance contract enters the tree at feature grain), so eval_revision on a
// task target reports false rather than comparing against nothing.
func basisFileFor(tree *store.Tree, n store.Node, name string) (string, bool) {
	switch t := n.(type) {
	case *store.Feature:
		if t.Key == nil {
			return "", false
		}
		dir := featureRelDir(t)
		if name == preconditionEvalRevision {
			return filepath.Join(dir, "feature.eval.md"), true
		}
		return filepath.Join(dir, "feature.md"), true
	case *store.Task:
		if name == preconditionEvalRevision {
			return "", false
		}
		feat := tree.Features[t.FeatureID]
		if feat == nil || feat.Key == nil || t.Key == nil {
			return "", false
		}
		dir := store.TaskRelDir(feat.EpicKey, localKey(*feat.Key), localKey(*t.Key))
		return filepath.Join(dir, store.TaskProseFile), true
	}
	return "", false
}

// missingBasisFiles names the working-tree files a revise's re-pin has to hash
// that are not there, in spec-then-eval order so two runs report the same list.
// It reads the SAME set basisFileFor names for the two content preconditions,
// which is exactly the set recordBasis and recordTaskBasis hash, so the guard
// and the write can never disagree about which files matter.
//
// The presence test is os-level rather than a hash attempt read backwards,
// which is how the basis system itself tells "nothing there" from "could not
// look" (WorktreeBlobRef and checkBasis's own missing arm both stat the path
// rather than parse git's error text). An empty list means every file the
// re-pin needs is present; it does not mean the content is any good, which is
// the eval contract's own validator's business (refreshFeatureBasis loads it).
func missingBasisFiles(tree *store.Tree, n store.Node) []string {
	var missing []string
	for _, name := range []string{preconditionSpecRevision, preconditionEvalRevision} {
		rel, ok := basisFileFor(tree, n, name)
		if !ok {
			continue
		}
		if !fileExists(tree.Root, filepath.Join(tree.Root, rel)) {
			missing = append(missing, rel)
		}
	}
	return missing
}

// reviseTargetIncompleteErr refuses a revise whose target is missing a file the
// re-pin reads. It names the operation, the target, every missing path, and both
// ways out, because which one is right is the human's call: a draft feature that
// never got its contract wants the contract written, while a spec deleted since
// the batch was staged wants the file restored or the operation dropped.
//
// It is a misuse (exit 2) rather than the blocked answer (exit 1) for the same
// reason every other refusal in this phase is: exit 1 on `approve amendment` is
// the stale-precondition answer with its own structured list of what moved, and
// this is not a comparison that came out false. It also matches what the sibling
// re-pin already answers, since `approve <noun> <node> --refresh-basis` over a
// deleted pin exits 2 as well (TestRefreshBasisOnADeletedPinFailsAndTheRefusalSaidSo).
func reviseTargetIncompleteErr(index int, target store.Node, missing []string) error {
	which := "which is"
	if len(missing) > 1 {
		which = "which are"
	}
	return &usageError{
		reason: "revise_target_incomplete",
		msg: fmt.Sprintf("operation %d revises %s %s, whose basis re-pin reads %s, %s not in the working tree; write or restore what is missing, or drop the operation",
			index, target.Noun(), target.NodeID(), strings.Join(missing, " and "), which),
	}
}

// nodeEdit is one live node the batch writes: loaded fresh from disk exactly
// once, then mutated in place by every operation that touches it. The
// once-only rule is not an optimization. Two operations can land on one node (a
// cancel and the supersedes link its replacement takes), and store.Tree refuses
// a batch that plans the same destination twice, so the document has to be
// shared and marshalled once at the end.
type nodeEdit struct {
	noun   string
	rel    string
	doc    store.Node
	before store.FileSnapshot
}

// dir is the node's repo-relative folder, the base the basis records build
// their paths from.
func (e *nodeEdit) dir() string { return filepath.Dir(e.rel) }

// batch is phase 2's accumulated output before anything is committed: the node
// documents phase 1 guards or phase 2 edits (in first-touch order), the subset
// phase 2 dirtied, the prose stubs materialized nodes land with, the events,
// the staged add keys mapped to the nodes they became, and successfully
// materialized epic ids in staged order.
//
// questions is the shared ledger's own accumulator, held separately from
// edits/order/dirty because a QuestionsFile is not a store.Node (question.go's
// own Question is, individually, but the ledger every add-question op
// contends for is ONE file holding all of them, unlike the one-file-per-node
// shape the rest of this batch tracks). It is nil until the first staged
// add-question op touches it (loadQuestions), non-nil and questionsDirty
// false when a batch stages one but every fold turned out to be a genuine
// no-op, and non-nil and dirty when at least one landed or unioned.
type batch struct {
	order  []string
	edits  map[string]*nodeEdit
	dirty  map[string]bool
	guards []store.PlannedWrite
	prose  []store.PlannedWrite
	events []events.Event
	staged map[string]*nodeEdit

	addedEpicIDs []string
	moveGuards   []store.PlannedWrite
	movedDirs    []string

	questions       *store.QuestionsFile
	questionsBefore store.FileSnapshot
	questionsDirty  bool

	evalCorrections map[int]evalCorrection // phase 1's correct-eval verdicts, by operation index
}

// loadQuestions reads the shared decisions ledger fresh off disk exactly once
// per batch and caches it, the once-only rule nodeEdit documents follow
// (editFor's own doc comment) and for the identical reason: every
// add-question op in the batch contends for the same file, so it has to be
// read once and folded in memory, not re-read (and so re-based on stale
// content) by every op after the first.
func (b *batch) loadQuestions(tree *store.Tree) (*store.QuestionsFile, error) {
	if b.questions != nil {
		return b.questions, nil
	}
	path := filepath.Join(tree.Root, filepath.FromSlash(questionsRelPath))
	raw, snapshot, err := store.ReadRegularFileSnapshot(tree.Root, path)
	if err != nil {
		return nil, err
	}
	var qf store.QuestionsFile
	if snapshot.Exists() {
		if err := yaml.Unmarshal(raw, &qf); err != nil {
			return nil, fmt.Errorf("mutate: parse %s: %w", path, err)
		}
	}
	b.questions = &qf
	b.questionsBefore = snapshot
	return b.questions, nil
}

func newBatch() *batch {
	return &batch{
		edits:  map[string]*nodeEdit{},
		dirty:  map[string]bool{},
		staged: map[string]*nodeEdit{},
	}
}

func (b *batch) register(e *nodeEdit) *nodeEdit {
	if existing, ok := b.edits[e.rel]; ok {
		return existing
	}
	b.edits[e.rel] = e
	b.order = append(b.order, e.rel)
	return e
}

// holdsID reports whether the batch already materialized a node under this id.
// mintIn only sees the LOADED tree, so two add operations in one batch could
// otherwise be handed the same token by a broken or deterministic minter and
// write two nodes sharing an id, a tree only the next load would refuse.
func (b *batch) holdsID(id string) bool {
	for _, e := range b.edits {
		if e.doc.NodeID() == id {
			return true
		}
	}
	return false
}

// stagedNoun reports whether this batch has already materialized a node
// under key as noun: materialize's own counterpart to resolveStageParentEpic
// (amendment.go), which asks the identical question of the amendment's op
// list at STAGE time before any node exists. Here the answer comes from
// b.staged instead, and needs no earlier-only filter of its own to add,
// because b.staged only ever holds entries the materialize loop's earlier
// iterations already wrote (the loop's own doc comment): a key this call
// cannot yet see is one materialize has not reached yet either, so nothing
// here widens the same-batch allowance past what loop order already grants.
// It is a pure map read; nothing here resolves an epic or writes to tree.
func (b *batch) stagedNoun(noun, key string) bool {
	e, ok := b.staged[key]
	return ok && e.noun == noun
}

// editFor loads the node's own yml fresh from disk (or hands back the copy this
// batch already holds) and refuses when the file it reached is not the
// addressed node, the same check loadTaskFile makes for the same reason: a
// rewrite that lands on a sibling cancels a node nobody named.
//
// The two arms are asymmetric in one way worth naming. The feature arm restores
// EpicKey onto the fresh document, because that field comes from the directory
// layout rather than from feature.yml (yaml:"-") and featureRelDir would build
// the wrong path without it. The task arm restores no FeatureID, and does not
// need to today: every task path here is built from the PARENT feature passed
// separately (loadTaskFile's own signature), so nothing reads the field off the
// fresh doc. A future caller that does read it must set it the way the feature
// arm sets EpicKey.
func (b *batch) editFor(tree *store.Tree, n store.Node) (*nodeEdit, error) {
	switch t := n.(type) {
	case *store.Feature:
		if t.Key == nil {
			return nil, fmt.Errorf("mutate: feature %s has no canonical key", t.ID)
		}
		rel := filepath.Join(featureRelDir(t), "feature.yml")
		if e, ok := b.edits[rel]; ok {
			return e, nil
		}
		var doc store.Feature
		snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, rel), &doc)
		if err != nil {
			return nil, err
		}
		if doc.ID != t.ID {
			return nil, fmt.Errorf("mutate: %s holds feature %s, not the addressed %s", rel, doc.ID, t.ID)
		}
		// EpicKey comes from the directory layout, not from feature.yml, so a doc
		// read straight off disk carries none and featureRelDir would build the
		// wrong path from it (transitionFeature restores it the same way).
		doc.EpicKey = t.EpicKey
		return b.register(&nodeEdit{noun: store.NounFeature, rel: rel, doc: &doc, before: snapshot}), nil

	case *store.Task:
		feat := tree.Features[t.FeatureID]
		if feat == nil || feat.Key == nil {
			return nil, fmt.Errorf("mutate: task %s has no resolvable parent feature", t.ID)
		}
		doc, rel, snapshot, err := loadTaskFile(tree, feat, t)
		if err != nil {
			return nil, err
		}
		return b.register(&nodeEdit{noun: store.NounTask, rel: rel, doc: doc, before: snapshot}), nil
	}
	return nil, &usageError{
		reason: "unsupported_noun",
		msg:    fmt.Sprintf("an amendment does not operate on a %s", n.Noun()),
	}
}

func (b *batch) lineageEditAt(tree *store.Tree, rel, noun string) (*nodeEdit, error) {
	if e, ok := b.edits[rel]; ok {
		return e, nil
	}
	var doc store.Node
	switch noun {
	case store.NounFeature:
		doc = &store.Feature{}
	case store.NounTask:
		doc = &store.Task{}
	default:
		return nil, fmt.Errorf("mutate: %s cannot contribute amendment lineage", noun)
	}
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, rel), doc)
	if err != nil {
		return nil, err
	}
	return b.register(&nodeEdit{noun: noun, rel: rel, doc: doc, before: snapshot}), nil
}

func (b *batch) writeFor(tree *store.Tree, n store.Node) (*nodeEdit, error) {
	e, err := b.editFor(tree, n)
	if err != nil {
		return nil, err
	}
	b.dirty[e.rel] = true
	return e, nil
}

// replacementFor is the node a supersede's by or a cancel's superseded_by
// names: a live node, or the one a staged add materialized earlier in this same
// batch. The staged lookup cannot miss, because resolveAmendment already
// refused a key naming no add operation and the add pass runs first.
func (b *batch) replacementFor(tree *store.Tree, r resolvedOp) (*nodeEdit, error) {
	if r.replaceKey != "" {
		e, ok := b.staged[r.replaceKey]
		if !ok {
			return nil, fmt.Errorf("mutate: staged key %q named no materialized node", r.replaceKey)
		}
		return e, nil
	}
	if r.replace == nil {
		return nil, fmt.Errorf("mutate: operation %d names no replacement", r.index)
	}
	return b.writeFor(tree, r.replace)
}

// applyAmendment is phase 2. Every pass mutates in-memory documents and appends
// events; nothing reaches disk until the single commit at the end, which is
// what makes the whole batch one transaction (this file's own doc comment).
//
// The batch arrives already holding whatever phase 1 read, so graph validation,
// preconditions, and phase 2 all share one document read. Phase 2 marks the
// documents it mutates dirty; every other graph input becomes a file read guard,
// and the graph directory inventory becomes an inventory read guard. Commit
// snapshot-checks both under the mutation lock without writing either kind.
func applyAmendment(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, b *batch,
	doc *store.Amendment, relYml string, snapshot store.FileSnapshot, plan []resolvedOp) (ApproveResult, error) {

	applied := make([]AppliedOperation, len(plan))

	if err := b.materialize(tree, deps, owner, doc, plan, applied); err != nil {
		return ApproveResult{}, err
	}
	if err := b.supersede(tree, deps, owner, doc, plan, applied); err != nil {
		return ApproveResult{}, err
	}
	if err := b.cancel(tree, deps, owner, doc, plan, applied); err != nil {
		return ApproveResult{}, err
	}
	if err := b.revise(tree, deps, owner, rev, doc, plan, applied); err != nil {
		return ApproveResult{}, err
	}
	if err := b.correctEval(tree, deps, owner, doc, plan, applied); err != nil {
		return ApproveResult{}, err
	}
	if err := b.moveFeatures(tree, deps, owner, doc, plan, applied); err != nil {
		return ApproveResult{}, err
	}

	writes := make([]store.PlannedWrite, 0, len(b.guards)+len(b.prose)+len(b.order)+3)
	writes = append(writes, b.guards...)
	writes = append(writes, b.prose...)
	for _, rel := range b.order {
		e := b.edits[rel]
		path := filepath.Join(tree.Root, rel)
		if b.movedPath(path) {
			continue
		}
		if !b.dirty[rel] {
			writes = append(writes, store.PlanReadGuard(path, e.before))
			continue
		}
		data, err := yaml.Marshal(e.doc)
		if err != nil {
			return ApproveResult{}, err
		}
		writes = append(writes, store.PlanWrite(path, e.before, data))
	}
	// The shared decisions ledger rides the same discipline as every node
	// file above: read-guarded when a staged add-question touched it but
	// folded to a genuine no-op, written once with every fold's own
	// mutation when at least one landed or unioned. It is not in b.order
	// because a QuestionsFile is not a store.Node (the batch's own doc
	// comment), so it gets its one write planned here instead.
	if b.questions != nil {
		path := filepath.Join(tree.Root, filepath.FromSlash(questionsRelPath))
		if !b.questionsDirty {
			writes = append(writes, store.PlanReadGuard(path, b.questionsBefore))
		} else {
			qdata, err := yaml.Marshal(b.questions)
			if err != nil {
				return ApproveResult{}, err
			}
			writes = append(writes, store.PlanWrite(path, b.questionsBefore, qdata))
		}
	}

	// Root order is an append-only consequence of newly materialized epics.
	// Planning it after node state keeps those creates ahead of their ordinal,
	// while placing it before events and the status-last recovery marker keeps
	// the entire approval in one rollback boundary.
	if len(b.addedEpicIDs) > 0 {
		orderWrite, err := planAppendProjectOrder(tree, b.addedEpicIDs)
		if err != nil {
			return ApproveResult{}, err
		}
		writes = append(writes, orderWrite)
	}

	before := doc.Status
	doc.Status = store.AmendmentStatusApproved
	adata, err := yaml.Marshal(doc)
	if err != nil {
		return ApproveResult{}, err
	}

	ev := amendmentOpEvent(deps, owner, doc, eventAmendmentApproved, nil, relYml)
	ev.Before = map[string]any{"status": before}
	ev.After["status"] = store.AmendmentStatusApproved
	ev.After["operations"] = len(plan)
	b.events = append(b.events, ev)

	// The event files are placed here rather than left to commitMutation's own
	// append, so the amendment's status write is last in the TRANSACTION and not
	// merely last among the state writes. That ordering is the whole recovery
	// contract: the only half-state a killed process can leave is operations
	// landed under an amendment still proposed, which re-approving answers
	// loudly, and never an approved amendment whose operations or history are
	// missing.
	appended, err := eventWrites(tree, b.events)
	if err != nil {
		return ApproveResult{}, err
	}
	writes = append(writes, appended...)
	writes = append(writes, store.PlanWrite(filepath.Join(tree.Root, relYml), snapshot, adata))
	writes, err = appendUniqueMoveGuards(writes, b.moveGuards)
	if err != nil {
		return ApproveResult{}, err
	}

	if err := tree.Commit("approve.amendment", writes); err != nil {
		return ApproveResult{}, err
	}

	return ApproveResult{
		ID:      doc.ID,
		Key:     deref(doc.Key),
		Status:  doc.Status,
		Applied: applied,
	}, nil
}

// materialize is phase 2 step 1: the add operations become real nodes, in the
// order they were staged, before anything can need to point at them.
//
// Ownership follows add.go's own rule with no separate handling here
// (docs/plans/2026-08-17-owner-lifecycle-design.md, "What this supersedes"):
// an add-epic arm mints an epic owned by the approver (planAddEpic stamps
// whatever owner it is called with, and that is owner here, the acting
// approver, not each op's original proposer); an add-feature arm mints a
// feature born unclaimed; an add-task arm mints a task carrying its parent
// feature's current claimant, empty under a same-batch-materialized feature
// since that one just landed unclaimed too. All three arms call planAddEpic,
// planAddFeature, and planAddTask directly below, so this is inherited
// automatically rather than reimplemented.
//
// Source rides through on the add-epic and add-feature arms (EpicInput and
// FeatureInput both carry a Source field; `tiller add epic --source` and
// `tiller add feature --source` are live, ordinary CLI flags, add.go), so a
// staged op's citation lands on the materialized node's own provenance, the
// same node-source record intake's design doc promises for every node an
// explosion creates. add-task carries none: TaskInput has no Source field at
// all, and task.yml's own schema (artifact-schema.md, Tasks) names no
// `source` key, so a staged add-task's citation stays where every other op's
// stays regardless of kind, on the amendment's own operations list
// (`show amendment --json`), never on the task node itself.
func (b *batch) materialize(tree *store.Tree, deps Deps, owner store.Contributor,
	doc *store.Amendment, plan []resolvedOp, applied []AppliedOperation) error {

	for _, r := range plan {
		var (
			pa  plannedAdd
			err error
		)
		switch r.op.Op {
		case store.AmendmentOpAddEpic:
			pa, err = planAddEpic(tree, deps, owner, EpicInput{
				Key: r.op.Key, Title: r.op.Title, Source: r.op.Source})
		case store.AmendmentOpAddFeature:
			pa, err = planAddFeature(tree, deps, owner, FeatureInput{
				Key: r.op.Key, Title: r.op.Title, ChangeType: r.op.ChangeType, Source: r.op.Source},
				b.stagedNoun)
			if err != nil {
				// b.stagedNoun (above) already gave planAddFeature's epic check
				// every same-batch epic this loop has materialized so far, so the
				// only way it can still answer *resolve.ErrNotFound here is a
				// hand-edited amendment: a shape resolveStageParentEpic
				// (amendment.go) already refuses before it ever reaches the
				// amendment's own yml, so no StageOperation call could have
				// produced it. It is also the ONLY way planAddFeature can answer
				// *resolve.ErrNotFound at all from this call site: the
				// FeatureInput built here carries no DependsOn (an AmendmentOp has
				// no such field), so resolveDependsOn never runs with a target to
				// resolve, and the epic check is the sole resolver call left that
				// could raise it. That bare, op-less error is upgraded to a named,
				// op-indexed refusal, reviseTargetIncompleteErr's idiom applied to
				// this defect (materializeParentUnknownErr, below); every other
				// error planAddFeature can return here propagates unchanged.
				var notFound *resolve.ErrNotFound
				if errors.As(err, &notFound) {
					epicKey, _, splitErr := splitFeatureKey(r.op.Key)
					if splitErr == nil {
						return materializeParentUnknownErr(r.index, epicKey, err)
					}
				}
			}
		case store.AmendmentOpAddTask:
			pa, err = planAddTask(tree, deps, owner, TaskInput{Key: r.op.Key, Title: r.op.Title})
		default:
			continue
		}
		if err != nil {
			return err
		}
		// The add verbs answer an existing node with the idempotent no-op success
		// the mutation contract promises. An amendment must not: the batch would
		// report an add it never performed, and on a re-approve after a partial
		// apply that is exactly the lie the recovery contract has to refuse.
		if !pa.result.Created {
			return &usageError{
				reason: "key_exists",
				msg: fmt.Sprintf("operation %d (%s) stages the key %q, which %s %s already holds; an amendment never re-adds a node that exists",
					r.index, r.op.Op, r.op.Key, pa.result.Noun, pa.result.ID),
			}
		}
		if b.holdsID(pa.result.ID) {
			return fmt.Errorf("mutate: amendment %s would mint %s twice in one batch", doc.ID, pa.result.ID)
		}

		e := b.register(&nodeEdit{
			noun:   pa.result.Noun,
			rel:    pa.ymlRel,
			doc:    pa.doc,
			before: store.AbsentSnapshot(),
		})
		b.dirty[e.rel] = true
		b.staged[r.op.Key] = e
		b.prose = append(b.prose, pa.prose)
		if r.op.Op == store.AmendmentOpAddEpic {
			b.addedEpicIDs = append(b.addedEpicIDs, pa.result.ID)
		}

		// The materialized node keeps the add.<noun> event `tiller add` writes,
		// with the amendment added as provenance and the command corrected to the
		// one that actually ran (refinement 8 of the driver doc: a materialized
		// node's event carries the amendment id).
		ev := pa.event
		ev.Command = amendApproveCommand(doc)
		ev.Targets = append(ev.Targets, doc.ID)
		ev.After["amendment"] = doc.ID
		ev.After["operation_index"] = r.index
		if r.op.Op == store.AmendmentOpAddFeature {
			epicKey, _, _ := splitFeatureKey(r.op.Key)
			if parent, ok := b.staged[epicKey]; ok {
				ev.After["epic"] = parent.doc.NodeID()
			} else {
				parent, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounEpic, Raw: epicKey})
				if err != nil {
					return err
				}
				ev.After["epic"] = parent.NodeID()
			}
		}
		b.events = append(b.events, ev)

		applied[r.index] = AppliedOperation{
			Index:  r.index,
			Op:     r.op.Op,
			Noun:   pa.result.Noun,
			NodeID: pa.result.ID,
			Key:    pa.result.Key,
			Status: pa.result.Status,
		}
	}

	// Second pass: every staged add-question op, in staged order, run only
	// AFTER every add-epic/add-feature/add-task above has already
	// materialized. This ordering is the acceptance's heart
	// (task-3-brief.md): it is what lets a question block a node the same
	// amendment adds, since a Blocks entry naming a staged key resolves
	// through b.staged, which the loop above just populated — the same
	// same-batch allowance resolveStageReplacement gives a supersede's by
	// (amendment.go), reaching materialize time here instead of stage time
	// because that is when the id it names actually starts to exist.
	//
	// Unlike the loop above, this one never refuses on a "duplicate": a
	// question has no key, so nothing here can collide the way two add-epics
	// sharing a key would, and two questions sharing a STATEMENT is not a
	// batch lying about what it did, it is foldQuestion's own documented
	// idempotency rule (question.go) landing through one more caller,
	// exactly the same for a same-batch duplicate as for one that already
	// exists live on the ledger.
	for _, r := range plan {
		if r.op.Op != store.AmendmentOpAddQuestion {
			continue
		}
		// Each Blocks entry resolves one of two ways, matching how it was
		// staged (resolveStageBlockTarget, amendment.go): a staged key
		// resolves through b.staged, which the node loop above just
		// populated, to the id THIS batch just minted — a lookup foldQuestion
		// itself cannot make, since it never sees anything this batch
		// materialized, only the tree loaded at the top of approve. Anything
		// else is already a real id from live-tree resolution at stage time,
		// re-validated against the current tree the same way this file
		// re-checks everything else it resolved earlier (resolveAmendment's
		// own doc comment) rather than trusted from there.
		resolvedBlocks := make([]string, 0, len(r.op.Blocks))
		for _, raw := range r.op.Blocks {
			if e, ok := b.staged[raw]; ok {
				resolvedBlocks = append(resolvedBlocks, e.doc.NodeID())
				continue
			}
			// Reached only through a hand-edited amendment: staging already
			// resolved raw once (resolveStageBlockTarget, amendment.go), and
			// nothing between then and now can make a live id or key stop
			// resolving. A bare error here — resolve.ErrNotFound, or
			// resolveBlockTarget's own untyped "not blockable" refusal for a
			// B-/Q-/etc address — would otherwise bubble out of phase 2
			// unnamed; blocksUnknownTargetErr is the exact refusal staging
			// itself would have given raw, so a hand-edited amendment gets
			// the same named, phase-appropriate, exit-2 answer a caller
			// staging it fresh would.
			if _, err := resolveBlockTargets(tree, []string{raw}); err != nil {
				return blocksUnknownTargetErr(raw)
			}
			resolvedBlocks = append(resolvedBlocks, raw)
		}

		qf, err := b.loadQuestions(tree)
		if err != nil {
			return err
		}
		res, ev, err := foldQuestion(tree, deps, owner, qf, r.op.Title, r.op.Statement, resolvedBlocks, store.QuestionDue{}, r.op.Source)
		if err != nil {
			return err
		}

		// Outcome distinguishes what actually happened, because unlike the
		// node loop above (which refuses key_exists rather than ever
		// reporting a no-op add), add-question folds a duplicate rather than
		// refusing (this function's own comment above the loop) — so
		// "applied" here does not always mean "changed something", and this
		// file's own recovery contract (the header comment) forbids
		// reporting work that was not done.
		outcome := AppliedNoChange
		switch {
		case res.Created:
			outcome = AppliedCreated
		case res.Changed:
			outcome = AppliedUpdated
		}

		// A staged Source never reaches the ledger through this fold, on
		// EITHER outcome below: foldQuestion's existing-entry arm (question.go)
		// only ever unions Blocks, and deliberately never writes Source,
		// whether the union found something new (AppliedUpdated) or not
		// (AppliedNoChange). This is the reasoned call, not an oversight: a
		// question's Source is the citation that first raised it, the same
		// create-once field a node's own key is, and AddQuestion's own
		// idempotent arm (the one foldQuestion IS, for both `tiller add
		// question` and this materialize) already treats it that way for a
		// live duplicate — a later batch's own citation for a fold that only
		// grows what the question blocks is not a second origin, so it does
		// not overwrite the first. What changed here is only that the drop is
		// now named on BOTH outcomes rather than the no-op one alone: staged
		// Blocks reaching the ledger (AppliedUpdated) used to leave a staged
		// Source's own silent drop unremarked, exactly the kind of quiet loss
		// this file's own recovery contract (the header comment) refuses
		// everywhere else.
		var note string
		switch outcome {
		case AppliedNoChange:
			var dropped []string
			if len(r.op.Blocks) > 0 {
				dropped = append(dropped, "blocks")
			}
			if r.op.Source != "" {
				dropped = append(dropped, "source")
			}
			if len(dropped) > 0 {
				reason := "it already holds every staged block"
				if res.Status != statusOpen && res.Status != "" {
					reason = fmt.Sprintf("it is %s", res.Status)
				}
				note = fmt.Sprintf("%s already existed (%s); staged %s did not land",
					res.ID, reason, strings.Join(dropped, " and "))
			}
		case AppliedUpdated:
			if r.op.Source != "" {
				note = fmt.Sprintf(
					"%s already existed; staged blocks landed but source did not (a question's recorded source is set once, at creation)",
					res.ID)
			}
		}

		if ev != nil {
			// Same provenance tagging the node materialize loop above gives
			// every add.<noun> event: the command corrected to the gate that
			// actually ran, and the amendment recorded as the targets/After
			// provenance a reader's history walk follows.
			e := *ev
			e.Command = amendApproveCommand(doc)
			e.Targets = append(e.Targets, doc.ID)
			e.After["amendment"] = doc.ID
			e.After["operation_index"] = r.index
			e.After["outcome"] = outcome
			b.events = append(b.events, e)
			b.questionsDirty = true
		}

		applied[r.index] = AppliedOperation{
			Index:   r.index,
			Op:      r.op.Op,
			Noun:    store.NounQuestion,
			NodeID:  res.ID,
			Status:  res.Status,
			Outcome: outcome,
			Note:    note,
		}
	}
	return nil
}

// materializeParentUnknownErr refuses an add-feature op, at materialize time,
// whose epic segment names no parent: not live in the tree, and not staged as
// an add-epic earlier in this same batch (b.stagedNoun, above). Staging
// refuses this shape before it ever reaches the amendment's own yml
// (resolveStageParentEpic's own parentUnknownErr, amendment.go), so the only
// amendment that can still carry it here is a hand-edited one, the same class
// of otherwise-unreachable defect reviseTargetIncompleteErr guards against
// elsewhere in this file; the op-indexed shape below is that same idiom
// applied to this defect. cause is planAddFeature's own returned err, not the
// *resolve.ErrNotFound errors.As unwrapped from it at the call site: the two
// are the same value today (planAddFeature returns the resolver's error
// bare), but passing the outer err rather than the narrowed local preserves
// any wrapping a future planAddFeature might add, and either way it carries
// through usageError's Unwrap (errors.go) so a caller unwinding for the
// concrete *resolve.ErrNotFound type still finds it, the same double-duty
// parentUnknownErr's own cause field already serves at staging.
func materializeParentUnknownErr(index int, epicKey string, cause error) error {
	return &usageError{
		reason: "not_found",
		msg: fmt.Sprintf("operation %d stages a feature under %q, which names no epic in the tree, and no add-epic staged earlier on this amendment",
			index, epicKey),
		cause: cause,
	}
}

// supersede is phase 2 step 2: both directions of the supersession edge, and
// nothing else. The target's STATUS does not move (supersede targets done work,
// and done is a merge nobody un-merges), but its file is written, because the
// superseded_by half of the record lives on it and a decision made after done
// is not a rewrite of done work (recordSupersession's own comment).
func (b *batch) supersede(tree *store.Tree, deps Deps, owner store.Contributor,
	doc *store.Amendment, plan []resolvedOp, applied []AppliedOperation) error {

	for _, r := range plan {
		if r.op.Op != store.AmendmentOpSupersede {
			continue
		}
		target, err := b.writeFor(tree, r.target)
		if err != nil {
			return err
		}
		repl, err := b.replacementFor(tree, r)
		if err != nil {
			return err
		}
		if err := recordSupersession(target.doc, repl.doc); err != nil {
			return err
		}

		ev := amendmentOpEvent(deps, owner, doc, eventSuperseded,
			[]string{target.doc.NodeID(), repl.doc.NodeID()}, target.rel, repl.rel)
		ev.After["superseded_by"] = repl.doc.NodeID()
		ev.After["operation_index"] = r.index
		b.events = append(b.events, ev)

		applied[r.index] = AppliedOperation{
			Index:       r.index,
			Op:          r.op.Op,
			Noun:        target.noun,
			NodeID:      target.doc.NodeID(),
			Key:         deref(target.doc.NodeKey()),
			Status:      target.doc.NodeStatus(),
			Replacement: repl.doc.NodeID(),
		}
	}
	return nil
}

// cancel is phase 2 step 3: the cancel transition, under exactly the legality
// the human-only cancel verb enforces. Only the actor differs, and that
// difference is the CLI's to enforce, so nothing here relaxes a state rule.
//
// An already-cancelled target refuses ahead of the legality check on purpose.
// The lifecycle engine reads a second cancel as an idempotent no-op (its own
// safe-retry rule), which is right for a human running the verb twice and wrong
// for a batch: a re-approve after a partial apply would silently report a
// cancel this run never performed.
func (b *batch) cancel(tree *store.Tree, deps Deps, owner store.Contributor,
	doc *store.Amendment, plan []resolvedOp, applied []AppliedOperation) error {

	for _, r := range plan {
		if r.op.Op != store.AmendmentOpCancel {
			continue
		}
		e, err := b.writeFor(tree, r.target)
		if err != nil {
			return err
		}
		before := e.doc.NodeStatus()
		if before == statusCancelled {
			return &usageError{
				reason: "already_cancelled",
				msg: fmt.Sprintf("operation %d cancels %s %s, which is already cancelled; nothing here re-cancels a node so a re-approved batch cannot report work it did not do",
					r.index, e.noun, e.doc.NodeID()),
			}
		}
		if legal, moves := lifecycle.Legal(e.doc, "cancel"); !legal {
			return &lifecycle.IllegalTransitionError{
				Noun: e.noun, From: before, Verb: "cancel", Legal: moves,
			}
		}
		if err := lifecycle.Apply(e.doc, "cancel", lifecycle.Payload{Reason: r.op.Reason}); err != nil {
			return err
		}

		files := []string{e.rel}
		replacement := ""
		if r.op.SupersededBy != "" {
			repl, err := b.replacementFor(tree, r)
			if err != nil {
				return err
			}
			// The same two-directional link a supersede records, on work that
			// never reached done: the field case's task at check, replaced before
			// completion (driver doc refinement 3).
			if err := recordSupersession(e.doc, repl.doc); err != nil {
				return err
			}
			replacement = repl.doc.NodeID()
			if repl.rel != e.rel {
				files = append(files, repl.rel)
			}
		}

		ev := amendmentOpEvent(deps, owner, doc, eventCancelPrefix+e.noun, []string{e.doc.NodeID()}, files...)
		ev.After["operation_index"] = r.index
		ev.Before = map[string]any{"status": before}
		ev.After["status"] = e.doc.NodeStatus()
		ev.After["reason"] = r.op.Reason
		if replacement != "" {
			ev.After["superseded_by"] = replacement
		}
		b.events = append(b.events, ev)

		applied[r.index] = AppliedOperation{
			Index:       r.index,
			Op:          r.op.Op,
			Noun:        e.noun,
			NodeID:      e.doc.NodeID(),
			Key:         deref(e.doc.NodeKey()),
			Status:      e.doc.NodeStatus(),
			Replacement: replacement,
		}
	}
	return nil
}

// revise is phase 2 step 4: the target's accepted_against is re-pinned from the
// current working tree, through the very code path approve --refresh-basis
// runs (refreshFeatureBasis at feature grain, recordTaskBasis at task grain),
// including the eval-gate staling that path performs and, on a changed
// contract, the expiry of the unfinished child tasks' acceptance, planned as
// part of this same batch through the batch's staged child documents.
//
// The operation asserts that the human reviewed the CURRENT spec and contract,
// which the preconditions pinned in phase 1. Re-pinning here is what keeps the
// batch from landing with an active_basis refusal standing on work the human
// just approved (driver doc refinement 4). It moves no lifecycle state, at
// either grain, exactly as refresh-basis moves none.
func (b *batch) revise(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions,
	doc *store.Amendment, plan []resolvedOp, applied []AppliedOperation) error {

	for _, r := range plan {
		if r.op.Op != store.AmendmentOpRevise {
			continue
		}
		e, err := b.writeFor(tree, r.target)
		if err != nil {
			return err
		}

		var before, after store.RevisionRef
		var expired []expiredChild
		switch n := e.doc.(type) {
		case *store.Feature:
			before = n.AcceptedAgainst.Spec
			evalChanged, err := refreshFeatureBasis(tree, e.dir(), n, deps, rev)
			if err != nil {
				return err
			}
			if evalChanged {
				if expired, err = b.expireChildAcceptance(tree, n.ID); err != nil {
					return err
				}
			}
			after = n.AcceptedAgainst.Spec
		case *store.Task:
			if n.AcceptedAgainst != nil {
				before = n.AcceptedAgainst.Spec
			}
			if err := recordTaskBasis(e.dir(), n, deps, rev); err != nil {
				return err
			}
			after = n.AcceptedAgainst.Spec
		default:
			return &usageError{
				reason: "unsupported_noun",
				msg:    fmt.Sprintf("a revise operation does not apply to a %s", e.noun),
			}
		}

		ev := amendmentOpEvent(deps, owner, doc, eventRevisePrefix+e.noun, []string{e.doc.NodeID()}, e.rel)
		ev.After["operation_index"] = r.index
		ev.Before = map[string]any{"spec": before.Value}
		ev.After["spec"] = after.Value
		ev.After["status"] = e.doc.NodeStatus()
		recordExpiry(&ev, expired)
		b.events = append(b.events, ev)

		applied[r.index] = AppliedOperation{
			Index:  r.index,
			Op:     r.op.Op,
			Noun:   e.noun,
			NodeID: e.doc.NodeID(),
			Key:    deref(e.doc.NodeKey()),
			Status: e.doc.NodeStatus(),
		}
	}
	return nil
}

// recordSupersession writes BOTH directions of one supersession edge: the
// target onto the replacement's supersedes list, and the replacement onto the
// target's own superseded_by. One decision, two readers. The forward direction
// answers "what did this replace" from the replacement; the backward one
// answers "what replaced this" from the retired node, which is the question the
// stale_spec sweep asks (drift's staleSpecFeatureExempt reads superseded_by
// alone) and the one a human reading the retired node has no other way to
// answer.
//
// Writing onto a done target is legitimate and is not a rewrite of done work:
// sync already writes verified_against onto a node at its done crossing, and
// this records a decision made after it, leaving the status and the completion
// record untouched.
//
// The forward leg is a union, so recording the same lineage twice is one entry
// rather than two; the backward leg is a single pointer that is simply set to
// the same value again. Together they are what makes a re-approved batch safe on
// this operation.
func recordSupersession(target, replacement store.Node) error {
	if err := setSupersededBy(target, replacement.NodeID()); err != nil {
		return err
	}
	return recordSupersedes(replacement, target.NodeID())
}

func recordSupersedes(n store.Node, targetID string) error {
	switch t := n.(type) {
	case *store.Feature:
		unionStrings(&t.Supersedes, []string{targetID})
	case *store.Task:
		unionStrings(&t.Supersedes, []string{targetID})
	default:
		return fmt.Errorf("mutate: a %s cannot record a supersedes link", n.Noun())
	}
	return nil
}

func setSupersededBy(n store.Node, replacementID string) error {
	var current *string
	switch v := n.(type) {
	case *store.Feature:
		current = v.SupersededBy
		if current == nil {
			v.SupersededBy = &replacementID
		}
	case *store.Task:
		current = v.SupersededBy
		if current == nil {
			v.SupersededBy = &replacementID
		}
	default:
		return fmt.Errorf("mutate: a %s cannot record superseded_by", n.Noun())
	}
	if current != nil && *current != replacementID {
		return fmt.Errorf("mutate: %s %s is already superseded by %s, not %s",
			n.Noun(), n.NodeID(), *current, replacementID)
	}
	return nil
}

// amendmentOpEvent builds the event one applied operation records. Every event
// the apply writes carries the amendment on Targets and on After["amendment"],
// which is the provenance trail a node's history renders: the command that ran
// was the amendment's approve, whatever verb the event's type names.
func amendmentOpEvent(deps Deps, owner store.Contributor, doc *store.Amendment,
	typ string, targets []string, files ...string) events.Event {

	ev := events.NewEvent(deps.clock(), typ, owner.ID, amendApproveCommand(doc),
		append(append([]string{}, targets...), doc.ID)...)
	// EV ids are appended to the event ledger, not keyed into the tree's id
	// maps, so there is nothing to retry against (artifact-schema.md, The eval
	// model: the AT- and EV- classes are separate non-node identifiers).
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"amendment": doc.ID}
	for _, f := range files {
		ev.Files = append(ev.Files, events.FileChange{Path: f})
	}
	return ev
}

// amendApproveCommand is the literal command every event this apply writes
// records, so a reader of a cancelled node's history sees the gate that
// cancelled it rather than a cancel verb nobody typed.
func amendApproveCommand(doc *store.Amendment) string {
	return "approve amendment " + doc.ID
}
