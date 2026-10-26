package mutate

// This file is the mutate layer's LIVE reads for `tiller show amendment` and
// the console's node page: EvaluateAmendmentPreconditions, Task 4's export,
// reused from approve's own comparison
// (amendment_approve.go's evalPrecondition) rather than a second copy of it;
// EvaluateAmendmentSource, Task 5's sibling for the one amendment-level fact
// rather than the per-op ones, reused from approve's own
// checkSource/evalSource the identical way; EvaluateAmendmentOpSources, the
// per-op sibling of THAT one, reused from approve's own checkOpSources
// the same way again; and EvaluateAmendmentShape, the per-op sibling of the
// other two hand-edit defect classes #57 names (a duplicate staged key, a
// staged-epic replacement), reused from resolveAmendment's own extracted
// predicates (duplicateStagedKey, stagedEpicKey; amendment_approve.go)
// rather than a second copy of either. Four callers now compare a staged
// precondition, an intake-minted amendment's own Source, one of its
// operations' own citations, or the doc's own shape, against the current
// tree (or, for EvaluateAmendmentOpSources and EvaluateAmendmentShape,
// against nothing but the doc itself) the identical way approve does:
// approve refuses on the first thing that does not hold, and show reports
// everything, valid findings included, so a human reviews the batch before
// running approve rather than after it refuses.

import (
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// Precondition name constants, exported for a caller outside this package
// (the CLI's show amendment) that needs to recognize a specific precondition
// by name without re-declaring the wire literal as an untethered magic
// string. Each mirrors amendment.go's own unexported constant exactly: that
// file keeps its private vocabulary for this package's staging code, and
// these are a second, public-facing name for the same five stable strings
// (artifact-schema.md, Amendments).
const (
	PreconditionTargetStatus   = preconditionTargetStatus
	PreconditionTargetRevision = preconditionTargetRevision
	PreconditionKeyAbsent      = preconditionKeyAbsent
	PreconditionSpecRevision   = preconditionSpecRevision
	PreconditionEvalRevision   = preconditionEvalRevision
)

// The three LIVE validity states EvaluateAmendmentPreconditions reports.
// Valid and Stale are the same verdict checkPreconditions itself reaches
// (holds, or does not); Unknown is not a verdict at all, it is the honest
// answer for a precondition whose operation carries no live target yet (an
// add-epic/add-feature/add-task's own staged key, or an add-question's own
// statement, in either case a node that exists only once approve
// materializes it), so calling it "stale" would claim something changed when
// nothing has been comparable from the start.
const (
	PreconditionValid   = "valid"
	PreconditionStale   = "stale"
	PreconditionUnknown = "unknown"
)

// PreconditionValidity is one staged operation's precondition as it reads
// against the live tree right now. Target names the operation's subject the
// same way StalePrecondition's own field does: its resolved target where it
// has one, its staged key where it does not (preconditionSubject).
type PreconditionValidity struct {
	Index        int
	Op           string
	Target       string
	Precondition string
	Expected     string
	Actual       string
	Status       string // PreconditionValid | PreconditionStale | PreconditionUnknown
	Note         string
}

// PreconditionNames returns the five precondition names in the fixed order
// approve evaluates them (preconditionNames), so a caller rendering every
// staged precondition (show amendment) lists them in the same order a stale
// report would rather than inventing a second ordering decision. It returns a
// fresh copy each call: the backing array is this package's own internal
// order and must not become mutable through the returned slice.
func PreconditionNames() []string {
	out := make([]string, len(preconditionNames))
	copy(out, preconditionNames)
	return out
}

// EvaluateAmendmentPreconditions is show amendment's live read: every staged
// precondition on every operation, compared against the CURRENT tree through
// the identical comparison ApproveAmendment's checkPreconditions runs in
// phase 1 (evalPrecondition), reused rather than reimplemented so the two
// surfaces can never disagree about what counts as stale.
//
// Unlike checkPreconditions it never refuses on a stale finding and it
// reports every precondition, valid ones included: a human reviewing a batch
// needs to see what already holds, not only what would block. Resolution
// failure is different: an operation whose target has vanished, names an
// epic, or is otherwise malformed is not a validity question at all (the
// same distinction resolveAmendment's own doc comment draws), so that error
// is returned rather than folded into the per-precondition report.
//
// It performs no writes; the batch it builds is the same throwaway
// bookkeeping struct phase 1 already used, in-memory only, since nothing here
// ever reaches applyAmendment.
func EvaluateAmendmentPreconditions(tree *store.Tree, deps Deps, amendment *store.Amendment) ([]PreconditionValidity, error) {
	doc, _, _, err := loadAmendmentDoc(tree, amendment)
	if err != nil {
		return nil, err
	}
	rev := deps.rev(tree.Root)

	plan, err := resolveAmendment(tree, doc)
	if err != nil {
		return nil, err
	}

	b := newBatch()
	var out []PreconditionValidity
	for _, r := range plan {
		if r.op.Op == store.AmendmentOpMoveFeature {
			pv := PreconditionValidity{Index: r.index, Op: r.op.Op, Target: r.op.Target, Precondition: "move", Expected: "reviewed move snapshot", Actual: "reviewed move snapshot", Status: PreconditionValid}
			fresh, err := store.Load(tree.Root)
			if err != nil {
				return nil, err
			}
			if err := evaluateFeatureMove(fresh, doc, r.index); err != nil {
				pv.Status = PreconditionStale
				pv.Actual = "changed"
				pv.Note = err.Error()
			} else if err := checkMoveConflicts(fresh, doc); err != nil {
				pv.Status = PreconditionStale
				pv.Note = err.Error()
			}
			out = append(out, pv)
		}
		for _, name := range preconditionNames {
			expected, ok := r.op.Preconditions[name]
			if !ok {
				continue
			}
			sp, err := evalPrecondition(tree, b, rev, r, name, expected)
			if err != nil {
				return nil, err
			}
			pv := PreconditionValidity{
				Index: r.index, Op: r.op.Op, Target: preconditionSubject(r),
				Precondition: name, Expected: expected,
			}
			switch {
			case sp == nil:
				pv.Status = PreconditionValid
				pv.Actual = expected
			case r.target == nil:
				// Only an add-epic/add-feature/add-task/add-question operation
				// ever reaches evalPrecondition with no target (every revise,
				// supersede, or cancel target resolved to a live node back in
				// resolveAmendment, or this loop would never have reached it).
				// Its subject is the staged key (or, for add-question, which
				// has none, the statement — preconditionSubject's own
				// fallback): the note here always reads "the operation names
				// no target...", which is not that something changed, it is
				// that nothing to compare against has been materialized yet.
				pv.Status = PreconditionUnknown
				pv.Note = sp.Note
			default:
				pv.Status = PreconditionStale
				pv.Actual = sp.Actual
				pv.Note = sp.Note
			}
			out = append(out, pv)
		}
	}
	return out, nil
}

// AmendmentSourceValidity is the amendment-level twin of PreconditionValidity:
// an intake-minted amendment's own Source, as it reads
// against the live tree right now, in the identical valid/stale vocabulary
// (PreconditionValid/PreconditionStale). It carries no PreconditionUnknown
// case: unlike a staged op's own precondition, which may target a key that
// only materializes when approve applies the batch, Source always names a
// real path on disk from the moment intake lands it, so there is never
// "nothing yet to compare against" the way an add op's own unmaterialized
// target has. Actual is the live hash when one could be read; Note carries
// the one case Actual alone would mislead (a vanished artifact has no live
// hash to report), the same role StalePrecondition's own Note plays for a
// missing basis file.
type AmendmentSourceValidity struct {
	Status string // PreconditionValid | PreconditionStale
	Actual string
	Note   string
}

// EvaluateAmendmentSource is show amendment's and the serve payload's live
// read of an intake-minted amendment's own Source, EvaluateAmendmentPreconditions'
// sibling for the one amendment-level fact rather than the per-op ones: it
// re-hashes the artifact Source.Path names through the identical evalSource
// comparison ApproveAmendment's own checkSource refuses on
// (amendment_approve.go), mapping the SAME verdict (evalSource's own
// sourceDrift, nil-means-valid) onto this package's Status/Actual/Note shape
// rather than re-deriving vanished-vs-changed from a raw hash pair a second
// time — the one-computation constraint checkSource's own doc comment now
// states explicitly. So the two surfaces can never disagree about whether a
// citation still traces to what intake fingerprinted.
//
// It returns (nil, nil) for a hand-opened amendment (doc.Source == nil): not
// an empty PreconditionUnknown-carrying value, but the honest absence of
// anything to evaluate, so a caller (the CLI, the node page) renders no
// source block at all rather than a live read for a fact that does not
// exist. An error is a tool failure (the amendment itself failed to load, or
// git could not answer), never a verdict, the same distinction
// EvaluateAmendmentPreconditions' own doc comment draws.
//
// It performs no writes, the same no-writes promise EvaluateAmendmentPreconditions
// keeps: re-hashing a working-tree file is a read.
func EvaluateAmendmentSource(tree *store.Tree, deps Deps, amendment *store.Amendment) (*AmendmentSourceValidity, error) {
	doc, _, _, err := loadAmendmentDoc(tree, amendment)
	if err != nil {
		return nil, err
	}
	if doc.Source == nil {
		return nil, nil
	}
	rev := deps.rev(tree.Root)
	sd, err := evalSource(tree, rev, doc.Source)
	if err != nil {
		return nil, err
	}
	if sd == nil {
		return &AmendmentSourceValidity{Status: PreconditionValid, Actual: doc.Source.Revision}, nil
	}
	return &AmendmentSourceValidity{Status: PreconditionStale, Actual: sd.Actual, Note: sd.Note}, nil
}

// OpSourceValidity is one staged operation whose citation is missing:
// checkOpSources' own per-operation defect (#54, amendment_approve.go),
// reported here rather than refused on. Index and Op are the same
// caller-facing pair PreconditionValidity keys its own findings by; unlike
// that struct there is no separate Status/Actual/Note to carry, because the
// finding IS the fact (the operation's Source is blank) and there is nothing
// live to compare it against the way a precondition or the amendment's own
// Source has.
type OpSourceValidity struct {
	Index int
	Op    string
}

// EvaluateAmendmentOpSources is show amendment's live read of checkOpSources'
// own per-operation provenance guard (amendment_approve.go): an intake-minted
// amendment requires every operation to carry a Source, and a hand edit can
// drop one without ever passing through StageOperation's own gate
// (checkOpSources' own doc comment explains the tamper path this closes).
// checkOpSources refuses on the FIRST blanked operation it finds, the right
// posture for a gate; this evaluator instead reports EVERY one, the same
// evaluator-beside-checker relationship EvaluateAmendmentPreconditions keeps
// with checkPreconditions: the two share the identical per-item predicate
// (strings.TrimSpace(op.Source) == "") but not checkOpSources' own
// first-offender control flow, so a human reviewing a batch sees the whole
// defect at once rather than fixing one citation at a time across repeated
// approve refusals.
//
// It returns nil for a hand-opened amendment (amendment.Source == nil), the
// same pass-through checkOpSources itself keeps: a hand amendment never
// required a per-op citation to begin with, so there is nothing to flag.
//
// Unlike EvaluateAmendmentPreconditions this takes no tree and calls
// resolveAmendment for nothing: it is pure over the doc, reading only
// amendment.Source and amendment.Operations, the fields loading an amendment
// already populates, the identical no-resolution posture EvaluateAmendmentSource
// keeps for the one amendment-level fact. That purity is what lets it survive
// a resolveAmendment failure a vanished or malformed target would cause: show
// amendment renders BOTH the resolution failure and every blanked citation
// for the same batch, because this read never depends on the other one
// succeeding.
//
// It carries no error return, unlike its two tree-reading siblings above:
// there is no failing read here to report (no doc load, no git, no tree
// walk), only a loop over a slice already in memory, so every path through
// this function returns nil for that reason and always did (a review finding
// carried into #57's own batch). EvaluateAmendmentShape, below, is pure the
// identical way and keeps the same error-free signature from the start.
func EvaluateAmendmentOpSources(amendment *store.Amendment) []OpSourceValidity {
	if amendment.Source == nil {
		return nil
	}
	var out []OpSourceValidity
	for i, op := range amendment.Operations {
		if strings.TrimSpace(op.Source) == "" {
			out = append(out, OpSourceValidity{Index: i, Op: op.Op})
		}
	}
	return out
}

// OpShapeFinding is one staged operation whose own SHAPE, independent of the
// live tree, is a hand-edit defect resolveAmendment would refuse on: a
// DUPLICATE STAGED KEY (two add-epic/add-feature/add-task operations staging
// the same key, resolveAmendment's own duplicateStagedKey predicate,
// amendment_approve.go) or a STAGED-EPIC REPLACEMENT (a supersede or a
// cancel naming a staged add-epic key as its By or SupersededBy, at any
// position in the doc, which is never a legal replacement grain —
// resolveStageReplacement's own deliberate exclusion, amendment.go;
// resolveAmendment's own stagedEpicKey predicate). Kind carries the SAME
// reason token resolveAmendment's own refusal names
// (ReasonDuplicateStagedKey, ReasonStagedEpicReplacement,
// amendment_approve.go), so the CLI and approve speak one language rather
// than inventing a second vocabulary for the identical fact.
type OpShapeFinding struct {
	Index int
	Op    string
	Kind  string // ReasonDuplicateStagedKey | ReasonStagedEpicReplacement
}

// EvaluateAmendmentShape is show amendment's live read of resolveAmendment's
// own two shape-only refusals, the per-op siblings
// EvaluateAmendmentOpSources' own class gets here: DUPLICATE STAGED KEY and
// STAGED-EPIC REPLACEMENT. Both predicates it consults (duplicateStagedKey,
// stagedEpicKey; amendment_approve.go) are doc-only, exactly like
// checkOpSources' own (strings.TrimSpace(op.Source) == ""): neither needs
// the live tree or a successful resolveAmendment run, only the doc's own
// operations, so this evaluator takes no tree and calls resolveAmendment for
// nothing, the identical posture EvaluateAmendmentOpSources already keeps
// and for the identical reason — a batch whose target has vanished still
// gets every operation's shape checked, because this read never depends on
// the other one succeeding.
//
// The two predicates diverge on ordering, and deliberately: duplicateStagedKey
// only looks BEFORE its own index (which occurrence is the defect depends on
// position — the first is legitimate, only a repeat is not), while
// stagedEpicKey looks at the WHOLE doc (an epic is never a legal replacement
// in any position, so nothing about ordering changes whether it is a
// defect). Both predicates' own doc comments (amendment_approve.go) explain
// this further.
//
// resolveAmendment refuses on the FIRST defect either predicate finds, the
// right posture for a gate; this evaluator instead reports EVERY one across
// BOTH kinds, in staged order, the same evaluator-beside-checker
// relationship EvaluateAmendmentPreconditions keeps with checkPreconditions.
//
// One combined evaluator rather than two EvaluateAmendmentOpSources-style
// siblings, because the two kinds are mutually exclusive by op kind
// (duplicateStagedKey only ever applies to an add-epic/add-feature/add-task
// operation; stagedEpicKey only ever applies to a supersede/cancel
// operation), so a caller's own per-op verdict is naturally ONE slot keyed
// by index either way — the CLI's own ShapeError field, singular
// (amendment_cmds.go). One Kind-tagged finding list is a plainer match for
// that than two lists a caller would still have to merge by index itself.
func EvaluateAmendmentShape(amendment *store.Amendment) []OpShapeFinding {
	var out []OpShapeFinding
	for i, op := range amendment.Operations {
		switch op.Op {
		case store.AmendmentOpAddEpic, store.AmendmentOpAddFeature, store.AmendmentOpAddTask:
			if duplicateStagedKey(amendment, i, op.Key) {
				out = append(out, OpShapeFinding{Index: i, Op: op.Op, Kind: ReasonDuplicateStagedKey})
			}
		case store.AmendmentOpSupersede, store.AmendmentOpCancel:
			// At most one of By/SupersededBy is ever non-blank on a given
			// op: store.ValidateAmendment's own per-op check
			// (validateAmendmentOp) refuses By set on anything but a
			// supersede and SupersededBy set on anything but a cancel, and
			// Op is a single value, so an operation can never be both at
			// once. resolveAmendment's own copy of this same raw-selection
			// (amendment_approve.go) leans on the identical guarantee; it
			// is repeated here rather than shared because the two run over
			// different inputs (a resolvedOp there, a raw store.AmendmentOp
			// here) for a two-line expression not worth threading a shared
			// helper through.
			raw := op.By
			if op.SupersededBy != "" {
				raw = op.SupersededBy
			}
			if raw != "" && stagedEpicKey(amendment, raw) {
				out = append(out, OpShapeFinding{Index: i, Op: op.Op, Kind: ReasonStagedEpicReplacement})
			}
		}
	}
	return out
}
