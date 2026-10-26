package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file implements the five amendment verbs (docs/spec/cli-surface.md):
// `tiller add amendment`, `tiller amend`, `tiller show amendment`, `tiller
// approve amendment` (wired into lifecycle_cmds.go's shared `approve` command),
// and `tiller withdraw amendment`. It is a thin shell over
// internal/core/mutate's AddAmendment, StageOperation, StageAmendsDesign,
// ApproveAmendment, and WithdrawAmendment, the same division of labor add.go
// and lifecycle_cmds.go keep for the epic/feature/task verbs: this file
// validates flag shape, resolves the address, hands a clean request to the
// core, and renders the outcome.
//
// add amendment and amend (staging) are agent-runnable, resolving the acting
// owner the same way every other mutation does. approve amendment and
// withdraw amendment are the two human gates (verbActors "human",
// lifecycle/table.go), which here, as everywhere else in this codebase, means
// guidance rather than a code-level refusal: cli-surface.md's "The actor
// model" is explicit that Tiller does not refuse the agent, because on one
// machine the agent and the human share an identity and any such refusal
// would be theater. There is no --actor flag or agent/human distinction
// anywhere in store.Contributor (manifest.go: "A developer's agent acts as that
// developer"), so nothing here adds one; a caller resolves the same acting
// contributor through --contributor/TILLER_CONTRIBUTOR/.tiller/identity.yml every other
// mutation uses.

// AmendmentData is the shared --json data block every amendment-returning
// command carries: identity, the human-owned
// lists, and the staged operations, each target structured as id and key
// separately, never a composed label. add amendment and amend leave every
// operation's preconditions structural (name and the staged expected value
// only); show amendment additionally fills each precondition's LIVE
// Status/Actual/Note, computed once by mutate.EvaluateAmendmentPreconditions
// rather than a second comparison here, and Source's own LIVE
// Status/Actual/Note the same way, computed once by
// mutate.EvaluateAmendmentSource.
type AmendmentData struct {
	ID           string                   `json:"id"`
	Key          string                   `json:"key,omitempty"`
	Title        string                   `json:"title,omitempty"`
	Status       string                   `json:"status"`
	Implements   string                   `json:"implements,omitempty"`
	TriggeredBy  []string                 `json:"triggered_by,omitempty"`
	AmendsDesign []string                 `json:"amends_design,omitempty"`
	BaseRevision string                   `json:"base_revision,omitempty"`
	Source       *AmendmentSourceView     `json:"source,omitempty"`
	Operations   []AmendmentOperationView `json:"operations"`
	Ref          string                   `json:"-"`
}

// AmendmentSourceView is the fingerprinted feedback artifact an intake-minted
// amendment was exploded from (store.AmendmentSource).
// A decoupled JSON view rather than the store shape reused directly, the same
// separation AmendmentRefView keeps: store.AmendmentSource carries only yaml
// tags, and this package's JSON surface is snake_case throughout (base_revision,
// triggered_by, ...), which a bare field-name fallback would break. Present
// only on an amendment `tiller intake` minted; nil for one opened by hand.
// Status/Actual/Note are the LIVE read against the current tree, the same
// flat convention AmendmentPreconditionView already carries alongside its own
// Name/Expected rather than a nested sub-object; add and amend leave them
// empty exactly as they leave every precondition's own three fields empty,
// and only show amendment computes them (mutate.EvaluateAmendmentSource).
type AmendmentSourceView struct {
	Path     string `json:"path,omitempty"`
	Revision string `json:"revision,omitempty"`
	Status   string `json:"status,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Note     string `json:"note,omitempty"`
}

// amendmentSourceView projects a.Source, or nil when the amendment carries
// none (every hand-opened amendment, and every amendment minted before this
// field existed). live is nil for add and amend (structural only, the same
// contract amendmentOperationView's own validity parameter keeps); show
// amendment passes the live read mutate.EvaluateAmendmentSource computed.
func amendmentSourceView(s *store.AmendmentSource, live *mutate.AmendmentSourceValidity) *AmendmentSourceView {
	if s == nil {
		return nil
	}
	v := &AmendmentSourceView{Path: s.Path, Revision: s.Revision}
	if live != nil {
		v.Status, v.Actual, v.Note = live.Status, live.Actual, live.Note
	}
	return v
}

// AmendmentOperationView is one staged operation: id and key separate for
// every ref it names, never a composed label, so a caller never re-parses a
// human string apart. Note explains the intrinsic approval rule when a
// supersede stages no optional target_status stale guard. Source cites the
// feedback this operation traces to, legal on every op kind
// (store.AmendmentOp's own doc comment); show amendment renders it as the
// headline's own trailing citation (renderOperationHeadline). SourceMissing
// is show amendment's own pre-flight of a hand-blanked citation (#57,
// mutate.EvaluateAmendmentOpSources): true only when this operation's own
// Source is blank on an intake-minted amendment, the same defect approve's
// own checkOpSources refuses on, surfaced here before approve ever runs; add
// and amend leave it false exactly as they leave every precondition's own
// live fields empty, and only show amendment computes it. ShapeError is
// SourceMissing's own sibling for the other two hand-edit defect classes
// #57 names (mutate.EvaluateAmendmentShape): the reason token
// (mutate.ReasonDuplicateStagedKey or mutate.ReasonStagedEpicReplacement)
// when resolveAmendment would refuse on this operation's own shape, blank
// otherwise. A token rather than a message, mirroring how every OTHER
// refusal on this surface names itself (the "reason" field on a blocked
// answer, ReasonAmendmentPreconditionsStale) rather than baking English
// prose into the JSON payload; the human render (renderAmendmentHuman) is
// what turns the token into a sentence, the same split SourceMissing's own
// fixed phrase already keeps between this struct (a fact) and the renderer
// (its wording). A plain string composes with SourceMissing without
// redundancy: the two are orthogonal facts an operation can carry at once
// (a cancel or a supersede can be both hand-blanked AND a staged-epic
// replacement,
// TestShowAmendmentRendersEveryHandEditDefectAlongsideTheResolutionFailure
// pins exactly that on one operation), so a shared boolean would
// either need a second field naming which shape defect fired anyway, or
// merge two unrelated facts into one flag; SourceMissing stays a bool
// because it has only ever had one shape to report. Blocks is legal
// only on add-question, resolved through amendmentRefView the same way
// Target/By/SupersededBy already are: a live feature or task's id and key,
// or a staged key naming an add op on this same amendment marked Staged, so
// `show amendment --json` can finally display what a staged question blocks.
type AmendmentOperationView struct {
	Index         int                 `json:"index"`
	Op            string              `json:"op"`
	Target        *AmendmentRefView   `json:"target,omitempty"`
	By            *AmendmentRefView   `json:"by,omitempty"`
	Key           string              `json:"key,omitempty"`
	Title         string              `json:"title,omitempty"`
	Statement     string              `json:"statement,omitempty"`
	ChangeType    string              `json:"change_type,omitempty"`
	Reason        string              `json:"reason,omitempty"`
	SupersededBy  *AmendmentRefView   `json:"superseded_by,omitempty"`
	Blocks        []*AmendmentRefView `json:"blocks,omitempty"`
	Retire        []string            `json:"retire,omitempty"`
	Source        string              `json:"source,omitempty"`
	SourceMissing bool                `json:"source_missing,omitempty"`
	// SourceUnresolved marks a source written as a document citation that
	// provenance cannot resolve (#143); it is kept as opaque text.
	SourceUnresolved bool                            `json:"source_unresolved,omitempty"`
	ShapeError       string                          `json:"shape_error,omitempty"`
	Preconditions    []AmendmentPreconditionView     `json:"preconditions,omitempty"`
	Move             *store.FeatureMovePreconditions `json:"move,omitempty"`
	Note             string                          `json:"note,omitempty"`
}

// AmendmentRefView names a target, by, or superseded_by: an id and a key when
// it resolves to a live feature or task, or just the staged key with Staged
// true when it names an add op already staged on this same amendment, whose
// node does not exist until approve materializes it
// (mutate.resolveStageReplacement's own rule). Ref is the reference-
// convention string for the plain-text renderer only.
type AmendmentRefView struct {
	ID     string `json:"id,omitempty"`
	Key    string `json:"key,omitempty"`
	Staged bool   `json:"staged,omitempty"`
	Ref    string `json:"-"`
}

// AmendmentPreconditionView is one staged precondition: the name and the
// value staged for it always, and Status/Actual/Note only when a caller
// (show amendment) computed live validity; add and amend leave them empty.
type AmendmentPreconditionView struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Status   string `json:"status,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Note     string `json:"note,omitempty"`
}

// preKey addresses one staged precondition by its operation index and name,
// the join key amendmentOperationView uses to fold a live-validity read into
// the structural view show amendment renders.
type preKey struct {
	Index int
	Name  string
}

// loadAmendmentTree loads the tree and resolves addr to one amendment,
// refusing any other resolved noun (resolve.ErrNounMismatch, the same
// checksum every other address resolution runs). It emits the failure
// envelope on any error and returns the wrapped exitError, so a caller just
// propagates a non-nil error, mirroring resolveNodeWithTree's own contract
// (show.go) for the four generic nouns.
func loadAmendmentTree(cmd *cobra.Command, command, addr string) (*store.Tree, *store.Amendment, error) {
	tree, err := loadTree(cmd)
	if err != nil {
		return nil, nil, emitFailure(cmd, command, err)
	}
	n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounAmendment, Raw: addr})
	if err != nil {
		return nil, nil, emitFailure(cmd, command, err)
	}
	a, ok := n.(*store.Amendment)
	if !ok {
		// Unreachable through the resolver above, which already checksums the
		// noun against the resolved node's own Noun() and would have refused
		// as ErrNounMismatch first; kept as a defensive type assertion rather
		// than a silent panic if that ever stops being true.
		return nil, nil, emitFailure(cmd, command, fmt.Errorf("cli: resolved node %s is not an amendment", n.NodeID()))
	}
	return tree, a, nil
}

// amendmentDataView projects an amendment onto the shared JSON/human shape.
// validity is nil for add and amend (structural only); show amendment passes
// the live read keyed by preKey. sourceValidity is the identical structural-
// only-unless-show contract for the one amendment-level fact rather than the
// per-op ones (mutate.EvaluateAmendmentSource's own result, or nil).
// opSourceMissing is nil for add and amend the same way, marking the flagged
// indices; show amendment passes the set mutate.EvaluateAmendmentOpSources
// computed (#57). opShape is the identical nil-unless-show contract for the
// other two hand-edit defect classes, keyed by index to the reason token
// mutate.EvaluateAmendmentShape found there (#57).
func amendmentDataView(tree *store.Tree, a *store.Amendment, validity map[preKey]mutate.PreconditionValidity, sourceValidity *mutate.AmendmentSourceValidity, opSourceMissing map[int]bool, opShape map[int]string) AmendmentData {
	ops := make([]AmendmentOperationView, len(a.Operations))
	for i, op := range a.Operations {
		ops[i] = amendmentOperationView(tree, i, op, validity, opSourceMissing, opShape)
	}
	return AmendmentData{
		ID: a.ID, Key: views.Key(a), Title: a.Title, Status: a.Status,
		Implements: a.Implements, TriggeredBy: a.TriggeredBy, AmendsDesign: a.AmendsDesign,
		BaseRevision: a.BaseRevision, Source: amendmentSourceView(a.Source, sourceValidity),
		Operations: ops, Ref: gen.Ref(tree, a),
	}
}

// amendmentResultView projects a freshly opened amendment (mutate.AddAmendment's
// own result) onto the same shared shape, with an empty (never nil)
// operations list: a new amendment always opens with none staged.
func amendmentResultView(r mutate.AmendmentResult) AmendmentData {
	return AmendmentData{
		ID: r.ID, Key: r.Key, Title: r.Title, Status: r.Status,
		Implements: r.Implements, TriggeredBy: r.TriggeredBy, AmendsDesign: r.AmendsDesign,
		BaseRevision: r.BaseRevision, Source: amendmentSourceView(r.Source, nil),
		Operations: []AmendmentOperationView{},
		Ref:        gen.RefParts(r.Key, r.ID),
	}
}

// amendmentOperationView projects one staged operation, resolving its target
// and replacement refs against the live tree and folding in live precondition
// validity, op-source validity, and op-shape validity when the caller
// computed them.
func amendmentOperationView(tree *store.Tree, index int, op store.AmendmentOp, validity map[preKey]mutate.PreconditionValidity, opSourceMissing map[int]bool, opShape map[int]string) AmendmentOperationView {
	v := AmendmentOperationView{
		Index: index, Op: op.Op, Key: op.Key, Title: op.Title, Statement: op.Statement,
		ChangeType: op.ChangeType, Reason: op.Reason, Source: op.Source,
		SourceMissing:    opSourceMissing[index],
		SourceUnresolved: relationships.UnresolvedCitation(op.Source),
		ShapeError:       opShape[index],
		Move:             op.Move,
		Retire:           op.Retire,
	}
	if op.Target != "" {
		v.Target = amendmentRefView(tree, op.Target)
	}
	if op.By != "" {
		v.By = amendmentRefView(tree, op.By)
	}
	if op.SupersededBy != "" {
		v.SupersededBy = amendmentRefView(tree, op.SupersededBy)
	}
	if len(op.Blocks) > 0 {
		v.Blocks = make([]*AmendmentRefView, len(op.Blocks))
		for i, raw := range op.Blocks {
			v.Blocks[i] = amendmentRefView(tree, raw)
		}
	}
	// Staging may propose a supersede before its target is done. Approval
	// enforces done intrinsically; target_status is an optional stale guard
	// that proves the reviewed state has not moved.
	if op.Op == store.AmendmentOpSupersede {
		if _, ok := op.Preconditions[mutate.PreconditionTargetStatus]; !ok {
			v.Note = "approve requires the supersede target to be done even without a target_status precondition; stage target_status=done when the reviewed batch should also prove the state has not moved."
		}
	}
	if op.Move != nil {
		pv := AmendmentPreconditionView{Name: "move", Expected: "reviewed move snapshot"}
		if got, ok := validity[preKey{index, "move"}]; ok {
			pv.Status, pv.Actual, pv.Note = got.Status, got.Actual, got.Note
		}
		v.Preconditions = append(v.Preconditions, pv)
	}
	for _, name := range mutate.PreconditionNames() {
		expected, ok := op.Preconditions[name]
		if !ok {
			continue
		}
		pv := AmendmentPreconditionView{Name: name, Expected: expected}
		if validity != nil {
			if got, ok := validity[preKey{index, name}]; ok {
				pv.Status = got.Status
				pv.Actual = got.Actual
				pv.Note = got.Note
			}
		}
		v.Preconditions = append(v.Preconditions, pv)
	}
	return v
}

// amendmentRefView resolves a target/by/superseded_by/blocks raw value to
// its structured ref: a live epic, feature, or task's id and key when raw is
// a well-formed id the tree can find, or the staged key itself, marked
// Staged, when raw names an add op already staged on this same amendment
// (the only other shape resolveStageReplacement/resolveStageBlockTarget ever
// accept). A well-formed id the tree cannot find (a vanished target)
// degrades to the bare id, the same honest fallback gen.RefByID gives.
// Target, By, and SupersededBy never name an epic in practice (epics
// reshape through their features, per cli-surface.md), but Blocks can
// (resolveStageBlockTarget's own doc comment: "extended to epics here since
// a question can block one"), so the lookup below checks all three tree
// maps rather than the two Target/By/SupersededBy alone would ever need.
func amendmentRefView(tree *store.Tree, raw string) *AmendmentRefView {
	if id.Valid(raw) {
		if n := lookupEpicFeatureOrTask(tree, raw); n != nil {
			return &AmendmentRefView{ID: n.NodeID(), Key: views.Key(n), Ref: gen.Ref(tree, n)}
		}
		return &AmendmentRefView{ID: raw, Ref: raw}
	}
	return &AmendmentRefView{Key: raw, Staged: true, Ref: raw + " (staged, materializes on apply)"}
}

// lookupEpicFeatureOrTask finds a live epic, feature, or task by id
// (amendmentRefView's own doc comment explains why all three tree maps are
// in play here even though only Blocks ever legitimately names an epic).
func lookupEpicFeatureOrTask(tree *store.Tree, rawID string) store.Node {
	if e, ok := tree.Epics[rawID]; ok {
		return e
	}
	if f, ok := tree.Features[rawID]; ok {
		return f
	}
	if t, ok := tree.Tasks[rawID]; ok {
		return t
	}
	return nil
}

// renderAmendmentHuman is the plain-text render for `show amendment`, the
// full batch beside which a human reviews the git diff:
// title, status, the identity lists, then every staged operation with its
// target refs in the reference convention and its preconditions' LIVE
// validity. validityErr is non-empty only when the live read itself could
// not run (the batch does not resolve against the current tree), in which
// case every precondition renders structural-only and the reason is named
// once at the end rather than repeated per line.
func renderAmendmentHuman(v AmendmentData, validityErr string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "amendment %s\n", v.Ref)
	if v.Title != "" {
		fmt.Fprintf(&b, "  title:  %s\n", v.Title)
	}
	fmt.Fprintf(&b, "  status: %s\n", v.Status)
	if v.BaseRevision != "" {
		fmt.Fprintf(&b, "  base_revision: %s\n", v.BaseRevision)
	}
	if v.Source != nil {
		fmt.Fprintf(&b, "  source: %s (%s)\n", v.Source.Path, v.Source.Revision)
		switch {
		case v.Source.Status == "":
			// No live read available (validityErr below explains why), the
			// same silent-column convention a precondition's own blank Status
			// keeps.
		case v.Source.Note != "":
			fmt.Fprintf(&b, "    live: [%s], %s\n", v.Source.Status, v.Source.Note)
		case v.Source.Actual != "":
			fmt.Fprintf(&b, "    live: [%s], actual %s\n", v.Source.Status, v.Source.Actual)
		default:
			fmt.Fprintf(&b, "    live: [%s]\n", v.Source.Status)
		}
	}
	if v.Implements != "" {
		fmt.Fprintf(&b, "  implements: %s\n", v.Implements)
	}
	if len(v.TriggeredBy) > 0 {
		fmt.Fprintf(&b, "  triggered_by: %s\n", strings.Join(v.TriggeredBy, ", "))
	}
	if len(v.AmendsDesign) > 0 {
		fmt.Fprintf(&b, "  amends_design: %s\n", strings.Join(v.AmendsDesign, ", "))
	}
	if len(v.Operations) == 0 {
		fmt.Fprint(&b, "  operations: (none staged)")
		return strings.TrimRight(b.String(), "\n")
	}
	fmt.Fprint(&b, "  operations:")
	for _, op := range v.Operations {
		fmt.Fprintf(&b, "\n    %d. %s", op.Index, renderOperationHeadline(op))
		if op.Note != "" {
			fmt.Fprintf(&b, "\n       note: %s", op.Note)
		}
		if op.Move != nil {
			fmt.Fprintf(&b, "\n       reviewed source: %s", op.Move.SourceKey)
			fmt.Fprintf(&b, "\n       review grain: %s (preserved", op.Move.ReviewGrain)
			if op.Move.MaterializeReviewGrain {
				fmt.Fprint(&b, " with explicit override")
			}
			fmt.Fprint(&b, ")")
			switch op.Move.OrderEffect {
			case "append":
				fmt.Fprint(&b, "\n       order: remove from source; append to destination in staged move order")
			case "preserve-position":
				fmt.Fprint(&b, "\n       order: preserve sibling position")
			}
			if op.Move.DestinationStaged {
				fmt.Fprint(&b, "\n       destination epic: staged earlier in this amendment")
			}
			for _, warning := range op.Move.ReferenceWarnings {
				fmt.Fprintf(&b, "\n       reference review: %s", warning)
			}
		}
		if op.SourceMissing {
			fmt.Fprint(&b, "\n       source: missing (intake amendments cite every operation)")
		}
		if op.SourceUnresolved {
			fmt.Fprintf(&b, "\n       source: %s", unresolvedCitationNote)
		}
		if op.ShapeError != "" {
			fmt.Fprintf(&b, "\n       shape: %s", shapeErrorNote(op.ShapeError))
		}
		for _, pc := range op.Preconditions {
			fmt.Fprintf(&b, "\n       precondition %s: expected %q", pc.Name, pc.Expected)
			switch {
			case pc.Status == "":
				// No live read available (validityErr below explains why).
			case pc.Note != "":
				fmt.Fprintf(&b, " [%s], %s", pc.Status, pc.Note)
			case pc.Actual != "":
				fmt.Fprintf(&b, " [%s], actual %q", pc.Status, pc.Actual)
			default:
				fmt.Fprintf(&b, " [%s]", pc.Status)
			}
		}
	}
	if validityErr != "" {
		fmt.Fprintf(&b, "\n  live validity unavailable: %s", validityErr)
	}
	return b.String()
}

// renderOperationHeadline is one staged operation's one-line summary, in the
// reference convention (key path with id in parentheses; bare ids only
// inside commands, which this line is not). add-epic reads in add-task's own
// key-and-title voice, since like add-task (and unlike add-feature) it
// carries no change_type. add-question leads with its short scanning title
// and includes the full statement as quoted detail. Source is legal on every
// op kind, and an intake-minted amendment requires every op to carry one, so
// it trails every headline alike as the quiet " · citation" suffix this CLI's
// provenance idiom already uses for a secondary fact beside a line's headline
// (readout_render.go's Ref/Note rows).
func renderOperationHeadline(op AmendmentOperationView) string {
	var line string
	switch op.Op {
	case store.AmendmentOpMoveFeature:
		line = fmt.Sprintf("move-feature %s to %s: %s", refOrDash(op.Target), op.Key, op.Reason)
	case store.AmendmentOpRevise:
		line = fmt.Sprintf("revise %s", refOrDash(op.Target))
	case store.AmendmentOpCorrectEval:
		line = fmt.Sprintf("correct-eval %s: %s", refOrDash(op.Target), op.Reason)
		if len(op.Retire) > 0 {
			line += ", retiring " + strings.Join(op.Retire, ", ")
		}
	case store.AmendmentOpSupersede:
		line = fmt.Sprintf("supersede %s by %s", refOrDash(op.Target), refOrDash(op.By))
	case store.AmendmentOpCancel:
		line = fmt.Sprintf("cancel %s: %s", refOrDash(op.Target), op.Reason)
		if op.SupersededBy != nil {
			line += fmt.Sprintf(", superseded by %s", op.SupersededBy.Ref)
		}
	case store.AmendmentOpAddFeature:
		line = fmt.Sprintf("add-feature %s (change_type: %s)", op.Key, op.ChangeType)
	case store.AmendmentOpAddTask:
		line = fmt.Sprintf("add-task %s: %q", op.Key, op.Title)
	case store.AmendmentOpAddEpic:
		line = fmt.Sprintf("add-epic %s: %q", op.Key, op.Title)
	case store.AmendmentOpAddQuestion:
		line = fmt.Sprintf("add-question %s: %q", op.Title, op.Statement)
	default:
		line = op.Op
	}
	// TrimSpace, not a bare != "": a whitespace-only Source is exactly the
	// blank citation SourceMissing already flags (mutate.EvaluateAmendmentOpSources
	// runs the identical trim), so rendering it here would print a citation
	// that reads as present (" · " followed by invisible whitespace) right
	// beside the "source: missing" note calling it absent — a fold-in from
	// #57's own review (the whitespace double-render).
	if strings.TrimSpace(op.Source) != "" {
		line += fmt.Sprintf(" · %s", op.Source)
	}
	return line
}

func refOrDash(r *AmendmentRefView) string {
	if r == nil {
		return "-"
	}
	return r.Ref
}

// shapeErrorNote turns one of resolveAmendment's own shape-refusal tokens
// (mutate.ReasonDuplicateStagedKey, mutate.ReasonStagedEpicReplacement) into
// the sentence renderAmendmentHuman prints beside a flagged operation, the
// same split SourceMissing's own fixed phrase keeps between the JSON token
// (AmendmentOperationView.ShapeError) and its human wording: the fact lives
// on the struct, the English lives here. The default arm is unreachable
// through this package's own two callers (both come from
// mutate.EvaluateAmendmentShape, which never mints a third token) and exists
// only so an unrecognized token still renders something rather than an
// empty line.
func shapeErrorNote(kind string) string {
	switch kind {
	case mutate.ReasonDuplicateStagedKey:
		return "duplicate staged key (another operation already stages this key; one amendment materializes each key once)"
	case mutate.ReasonStagedEpicReplacement:
		return "staged epic replacement (names a staged add-epic key; a supersede or a cancel only ever targets a feature or a task)"
	default:
		return kind
	}
}

// --- add amendment ---

func newAddAmendmentCmd() *cobra.Command {
	var title, implements string
	var triggeredBy []string
	cmd := &cobra.Command{
		Use:   "amendment <key>",
		Short: "Open a new amendment: a proposed staging document for a reshape.",
		Long: "add amendment opens a proposed staging document anchored to the current\n" +
			"HEAD, with an empty operations list. Stage operations onto it with\n" +
			"`tiller amend`, review the batch with `tiller show amendment`, and cross\n" +
			"the one human gate with `tiller approve amendment` (or drop it with\n" +
			"`tiller withdraw amendment`). Re-adding a key already held by another\n" +
			"proposed amendment is refused rather than idempotent: a second open\n" +
			"amendment under one key is not the same proposal as the first.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAddAmendment(cmd, args[0], title, implements, triggeredBy)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "human-readable amendment title")
	cmd.Flags().StringVar(&implements, "implements", "", "the ADR this amendment carries out, if any")
	cmd.Flags().StringSliceVar(&triggeredBy, "triggered-by", nil, "design doc(s) whose change prompted this reshape (repeatable)")
	addContributorFlag(cmd)
	return cmd
}

func runAddAmendment(cmd *cobra.Command, key, title, implements string, triggeredBy []string) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	res, err := mutate.AddAmendment(tree, d, mutate.AmendmentInput{
		Key: key, Title: title, Implements: implements, TriggeredBy: triggeredBy,
	})
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	view := amendmentResultView(res)
	human := fmt.Sprintf("Opened amendment %s in %s.", view.Ref, res.Status)
	if res.Title != "" {
		human = fmt.Sprintf("Opened amendment %s in %s: %q.", view.Ref, res.Status, res.Title)
	}
	return emitSuccess(cmd, "add", view, human)
}

// --- amend (staging) ---

// amendFlags carries every flag `tiller amend` accepts, gathered into one
// struct so buildStageInput and runAmend read them by name rather than
// threading a dozen positional parameters through.
type amendFlags struct {
	addEpic                string
	addFeature, changeType string
	addTask, title         string
	addQuestion            string
	blocks                 []string
	supersede, by          string
	cancel, reason         string
	supersededBy           string
	revise                 string
	correctEval            string
	retire                 []string
	moveFeature, to        string
	source                 string
	preconditions          []string
	amendsDesign           []string
}

// amendOpFlags lists the operation-selector flags in the fixed order
// the exactly-one-operation refusal names them in, so two runs report the
// same text (cmd.Flags().Changed has no ordering guarantee of its own).
var amendOpFlags = []string{"add-epic", "add-feature", "add-task", "add-question", "supersede", "cancel", "revise", "correct-eval", "move-feature"}

func newAmendCmd() *cobra.Command {
	var f amendFlags
	cmd := &cobra.Command{
		Use:   "amend <A-id|key> [operation flags]",
		Short: "Stage one operation, or an amends-design addition, onto a proposed amendment.",
		Long: "amend appends one staged operation to a proposed amendment's operations\n" +
			"list, unions design doc paths into its amends_design, or both in one\n" +
			"call. Exactly one operation kind is legal per invocation:\n" +
			"  --add-epic <epic-key> [--title <t>] (a root: no parent to resolve)\n" +
			"  --add-feature <epic-key>/<feature-key> --change-type <ct> [--title <t>]\n" +
			"  --add-task <epic-key>/<feature-key>/<task-key> --title <t>\n" +
			"  --add-question <statement> --title <short-title> [--blocks <id-or-key-or-staged-key>]... (no key at all)\n" +
			"  --supersede <id-or-key> --by <id-or-staged-key> (done target at approve)\n" +
			"  --cancel <id-or-key> --reason <text> [--superseded-by <id-or-staged-key>] (not-yet-done replacement)\n" +
			"  --revise <id-or-key> (non-terminal target at approve)\n" +
			"  --correct-eval <feature-id-or-key> --reason <text> [--retire <criterion-id>]... (a done feature's eval bindings)\n" +
			"  --move-feature <feature-id-or-key> --to <epic-key>/<feature-key> --reason <text>\n" +
			"--precondition name=value is repeatable and attaches to the operation\n" +
			"staged this call; approve validates every staged precondition against\n" +
			"the live tree before applying anything. --source cites the feedback\n" +
			"this operation traces to (free text, by convention path#Lstart-Lend)\n" +
			"and is required on every op staged onto an intake-minted amendment.\n" +
			"--amends-design is repeatable and may ride alone or beside an\n" +
			"operation. Nothing here executes: only approve does.\n" +
			"Staging a supersede before its target is done remains legal, but approval\n" +
			"always requires done even without a target_status precondition. Stage\n" +
			"--precondition target_status=done as an optional stale guard when the\n" +
			"reviewed batch should also prove the state has not moved. Replace\n" +
			"not-yet-done work with --cancel and --superseded-by instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAmend(cmd, args[0], f)
		},
	}
	cmd.Flags().StringVar(&f.addEpic, "add-epic", "", "stage a new epic at <epic-key> (a root: no parent to resolve)")
	cmd.Flags().StringVar(&f.addFeature, "add-feature", "", "stage a new feature at <epic-key>/<feature-key>")
	cmd.Flags().StringVar(&f.changeType, "change-type", "create", "the staged feature's change_type: create | modify | replace | migrate")
	cmd.Flags().StringVar(&f.addTask, "add-task", "", "stage a new task at <epic-key>/<feature-key>/<task-key>")
	cmd.Flags().StringVar(&f.title, "title", "", "the staged epic's, feature's, task's, or question's title (required with --add-task and --add-question)")
	cmd.Flags().StringVar(&f.addQuestion, "add-question", "", "stage a new open question with this full statement (no key at all)")
	cmd.Flags().StringSliceVar(&f.blocks, "blocks", nil, "nodes the staged question holds (id, key, or an add op's staged key; repeatable)")
	cmd.Flags().StringVar(&f.supersede, "supersede", "",
		"an existing feature or task (id or key) retired after it is done; staging may occur earlier, but approve intrinsically requires done; use --cancel with --superseded-by for non-done replacement")
	cmd.Flags().StringVar(&f.by, "by", "", "the supersede's replacement (id, key, or an add op's staged key)")
	cmd.Flags().StringVar(&f.cancel, "cancel", "", "an existing feature or task (id or key) this amendment cancels")
	cmd.Flags().StringVar(&f.reason, "reason", "", "why the target is cancelled, moved, or its eval corrected (required with --cancel, --move-feature, and --correct-eval)")
	cmd.Flags().StringVar(&f.supersededBy, "superseded-by", "", "the cancel's replacement, if any (id, key, or an add op's staged key)")
	cmd.Flags().StringVar(&f.revise, "revise", "", "an existing non-terminal feature or task (id or key) revised in place")
	cmd.Flags().StringVar(&f.correctEval, "correct-eval", "", "a done feature (id or key) whose eval bindings are corrected in place, criteria unchanged")
	cmd.Flags().StringSliceVar(&f.retire, "retire", nil, "a criterion id the --correct-eval deliberately removes (repeatable)")
	cmd.Flags().StringVar(&f.moveFeature, "move-feature", "", "an unstarted feature (id or key) to relocate with identity and lifecycle preserved")
	cmd.Flags().StringVar(&f.to, "to", "", "destination <epic-key>/<feature-key> (required only with --move-feature)")
	cmd.Flags().StringVar(&f.source, "source", "", "the feedback this operation traces to (free text, e.g. path#Lstart-Lend); required on an intake-minted amendment")
	cmd.Flags().StringArrayVar(&f.preconditions, "precondition", nil, "name=value guard approve validates before applying (repeatable)")
	cmd.Flags().StringSliceVar(&f.amendsDesign, "amends-design", nil, "design doc(s) this amendment's approval keeps current (repeatable)")
	addContributorFlag(cmd)
	return cmd
}

// selectedAmendOps reports which operation-selector flags were
// actually passed, in amendOpFlags' fixed order.
func selectedAmendOps(cmd *cobra.Command) []string {
	var out []string
	for _, name := range amendOpFlags {
		if cmd.Flags().Changed(name) {
			out = append(out, name)
		}
	}
	return out
}

func runAmend(cmd *cobra.Command, addr string, f amendFlags) error {
	selected := selectedAmendOps(cmd)
	if len(selected) > 1 {
		return emitFailure(cmd, "amend", &usageError{
			reason: "multiple_operations",
			msg:    fmt.Sprintf("amend stages exactly one operation kind per invocation; got --%s", strings.Join(selected, ", --")),
		})
	}
	if cmd.Flags().Changed("to") && (len(selected) != 1 || selected[0] != "move-feature") {
		return emitFailure(cmd, "amend", &usageError{
			reason: "to_misplaced",
			msg:    "--to is only valid with --move-feature",
		})
	}
	if cmd.Flags().Changed("retire") && (len(selected) != 1 || selected[0] != "correct-eval") {
		return emitFailure(cmd, "amend", &usageError{
			reason: "retire_misplaced",
			msg:    "--retire is only valid with --correct-eval",
		})
	}
	pre, err := parsePreconditions(f.preconditions)
	if err != nil {
		return emitFailure(cmd, "amend", err)
	}
	// A precondition names WHICH operation it guards, so it needs one staged
	// this same call; this is checked ahead of nothing_to_stage below so a
	// caller who typed --precondition alone (a plausible mistake: they meant
	// to also pass an op flag) gets told about the flag it needs rather than
	// the more generic "nothing to stage" (true, but less specific).
	if len(selected) == 0 && len(pre) > 0 {
		return emitFailure(cmd, "amend", &usageError{
			reason: "precondition_without_op",
			msg:    "--precondition needs an operation flag to attach to",
		})
	}
	if len(selected) == 0 && len(f.amendsDesign) == 0 {
		return emitFailure(cmd, "amend", &usageError{
			reason: "nothing_to_stage",
			msg:    "amend needs one operation flag (--add-epic, --add-feature, --add-task, --add-question, --supersede, --cancel, --revise, --correct-eval, --move-feature) or --amends-design",
		})
	}

	tree, a, err := loadAmendmentTree(cmd, "amend", addr)
	if err != nil {
		return err
	}
	// One owner resolution for the whole call: amend can stage an operation
	// and an amends-design union in the same invocation, and both attribute
	// to the same acting contributor, so a corrupt identity file refuses once here
	// rather than independently at each arm below.
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "amend", err)
	}

	var stagedOp *mutate.StageResult
	if len(selected) == 1 {
		in, err := buildStageInput(selected[0], f, pre)
		if err != nil {
			return emitFailure(cmd, "amend", err)
		}
		res, err := mutate.StageOperation(tree, d, a, in)
		if err != nil {
			return emitFailure(cmd, "amend", err)
		}
		stagedOp = &res
	}

	var design *mutate.AmendsDesignResult
	if len(f.amendsDesign) > 0 {
		res, err := mutate.StageAmendsDesign(tree, d, a, f.amendsDesign)
		if err != nil {
			return emitFailure(cmd, "amend", err)
		}
		design = &res
	}

	// StageOperation and StageAmendsDesign each write through a copy loaded
	// fresh off disk (loadAmendmentDoc), never the caller's in-memory
	// amendment, so the full, current operations list this renders has to
	// come from a fresh read too.
	freshTree, fresh, err := loadAmendmentTree(cmd, "amend", addr)
	if err != nil {
		return err
	}
	view := amendmentDataView(freshTree, fresh, nil, nil, nil, nil)
	return emitSuccess(cmd, "amend", view, humanAmend(view.Ref, stagedOp, design))
}

// buildStageInput turns the one selected operation flag into the StageInput
// mutate.StageOperation validates. It runs the same up-front checks the
// sibling add verbs run for their own key and change-type flags (add.go's
// validateFeatureKey, validateTaskKey, validChangeTypes, and runAddEpic's own
// inline keyRe check for --add-epic, which carries no <epic-key>/<feature-key>
// segment to split), so a malformed address or an unknown change type is
// refused before the tree is even touched, and it adds two flag-pairing
// checks mutate itself leaves lax:
// --add-task needs --title (store.ValidateAmendment does not require one,
// but a titleless task is not something this CLI hands through, mirroring
// `tiller add task`'s own required --title), and --cancel needs --reason up
// front rather than deferred to approve's resolveAmendment (mirroring the
// standalone `cancel` verb's own required --reason, lifecycle_cmds.go's
// newCancelCmd).
// By, SupersededBy, Blocks, and Source ride on every arm below, not only the
// arms that use them: mutate.buildStagedOp (amendment.go) is what refuses a
// misplaced --by, --superseded-by, or --blocks (supersede_by_misplaced,
// superseded_by_misplaced, blocks_misplaced), and StageOperation is what
// requires --source on an intake-minted amendment (source_required); each
// can only refuse a field this function actually forwards. Dropping any of
// the four on an arm that does not use it would silently swallow a caller's
// misplaced or missing flag instead of letting mutate's own checks catch it.
func buildStageInput(opFlag string, f amendFlags, pre map[string]string) (mutate.StageInput, error) {
	switch opFlag {
	case "move-feature":
		if strings.TrimSpace(f.to) == "" {
			return mutate.StageInput{}, &usageError{reason: "to_required", msg: "--move-feature needs --to <epic-key>/<feature-key>"}
		}
		if err := validateFeatureKey(f.to); err != nil {
			return mutate.StageInput{}, err
		}
		if strings.TrimSpace(f.reason) == "" {
			return mutate.StageInput{}, &usageError{reason: "reason_required", msg: "--move-feature needs --reason"}
		}
		return mutate.StageInput{
			Op: store.AmendmentOpMoveFeature, Target: f.moveFeature, Key: f.to, Reason: f.reason,
			By: f.by, SupersededBy: f.supersededBy, Blocks: f.blocks, Source: f.source, Preconditions: pre,
		}, nil

	case "add-epic":
		if !keyRe.MatchString(f.addEpic) {
			return mutate.StageInput{}, &usageError{
				reason: "invalid_key",
				msg:    fmt.Sprintf("epic key %q must match [a-z0-9]+(-[a-z0-9]+)*", f.addEpic),
			}
		}
		return mutate.StageInput{
			Op: store.AmendmentOpAddEpic, Key: f.addEpic, Title: f.title,
			By: f.by, SupersededBy: f.supersededBy, Blocks: f.blocks, Source: f.source, Preconditions: pre,
		}, nil

	case "add-feature":
		if err := validateFeatureKey(f.addFeature); err != nil {
			return mutate.StageInput{}, err
		}
		if !validChangeTypes[f.changeType] {
			return mutate.StageInput{}, &usageError{
				reason: "invalid_change_type",
				msg:    fmt.Sprintf("change-type %q must be one of create, modify, replace, migrate", f.changeType),
			}
		}
		return mutate.StageInput{
			Op: store.AmendmentOpAddFeature, Key: f.addFeature, Title: f.title,
			ChangeType: f.changeType, By: f.by, SupersededBy: f.supersededBy, Blocks: f.blocks, Source: f.source, Preconditions: pre,
		}, nil

	case "add-task":
		if err := validateTaskKey(f.addTask); err != nil {
			return mutate.StageInput{}, err
		}
		if strings.TrimSpace(f.title) == "" {
			return mutate.StageInput{}, &usageError{reason: "title_required", msg: "--add-task needs --title"}
		}
		return mutate.StageInput{
			Op: store.AmendmentOpAddTask, Key: f.addTask, Title: f.title,
			By: f.by, SupersededBy: f.supersededBy, Blocks: f.blocks, Source: f.source, Preconditions: pre,
		}, nil

	case "add-question":
		return mutate.StageInput{
			Op: store.AmendmentOpAddQuestion, Title: f.title, Statement: f.addQuestion, Blocks: f.blocks,
			By: f.by, SupersededBy: f.supersededBy, Source: f.source, Preconditions: pre,
		}, nil

	case "supersede":
		return mutate.StageInput{
			Op: store.AmendmentOpSupersede, Target: f.supersede, By: f.by,
			SupersededBy: f.supersededBy, Blocks: f.blocks, Source: f.source, Preconditions: pre,
		}, nil

	case "cancel":
		if strings.TrimSpace(f.reason) == "" {
			return mutate.StageInput{}, &usageError{reason: "reason_required", msg: "--cancel needs --reason"}
		}
		return mutate.StageInput{
			Op: store.AmendmentOpCancel, Target: f.cancel, Reason: f.reason,
			By: f.by, SupersededBy: f.supersededBy, Blocks: f.blocks, Source: f.source, Preconditions: pre,
		}, nil

	case "correct-eval":
		if strings.TrimSpace(f.reason) == "" {
			return mutate.StageInput{}, &usageError{reason: "reason_required", msg: "--correct-eval needs --reason"}
		}
		return mutate.StageInput{
			Op: store.AmendmentOpCorrectEval, Target: f.correctEval, Reason: f.reason, Retire: f.retire,
			By: f.by, SupersededBy: f.supersededBy, Blocks: f.blocks, Source: f.source, Preconditions: pre,
		}, nil

	case "revise":
		return mutate.StageInput{
			Op: store.AmendmentOpRevise, Target: f.revise,
			By: f.by, SupersededBy: f.supersededBy, Blocks: f.blocks, Source: f.source, Preconditions: pre,
		}, nil
	}
	// Unreachable: opFlag always comes from selectedAmendOps, which only ever
	// names one of the cases above.
	return mutate.StageInput{}, fmt.Errorf("cli: unreachable amend op flag %q", opFlag)
}

// parsePreconditions turns each repeated --precondition name=value token into
// the map StageOperation validates, refusing a token with no "=" (or a blank
// name) as misuse before the tree is touched: a malformed flag is a caller
// error mutate can never see, since it only ever receives the parsed map.
// Whether a NAME is one of the five recognized preconditions, and whether a
// VALUE is non-empty, is StageOperation's own check (validatePreconditions);
// this only judges whether the flag itself parses.
func parsePreconditions(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(raw))
	for _, tok := range raw {
		name, value, ok := strings.Cut(tok, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, &usageError{
				reason: "precondition_malformed",
				msg:    fmt.Sprintf("--precondition %q must be name=value", tok),
			}
		}
		out[name] = value
	}
	return out, nil
}

// humanAmend is the plain-text confirmation for `tiller amend`: what was
// staged, and what amends_design now reads, in whichever combination this
// call performed.
// unresolvedCitationNote names the citation forms provenance resolves, for a
// staged source written as a citation that does not match them (#143).
const unresolvedCitationNote = "not a citation Tiller can resolve (path.md#Lstart-Lend, more ranges after commas)"

func humanAmend(ref string, staged *mutate.StageResult, design *mutate.AmendsDesignResult) string {
	var lines []string
	if staged != nil {
		lines = append(lines, fmt.Sprintf("Staged operation %d (%s) on amendment %s.", staged.Index, staged.Op.Op, ref))
		if relationships.UnresolvedCitation(staged.Op.Source) {
			lines = append(lines, fmt.Sprintf("Warning: --source %q is %s. Nothing edits a staged operation, so it stays opaque text unless the amendment is withdrawn and restaged.", staged.Op.Source, unresolvedCitationNote))
		}
		if staged.Op.Move != nil {
			lines = append(lines, fmt.Sprintf("Proposed move %s to %s: %s. Nothing moves until approval.",
				gen.RefParts(staged.Op.Move.SourceKey, staged.Op.Target), staged.Op.Key, staged.Op.Reason))
		}
	}
	if design != nil {
		if design.Changed {
			lines = append(lines, fmt.Sprintf("amends_design on %s now: %s.", ref, strings.Join(design.AmendsDesign, ", ")))
		} else {
			lines = append(lines, fmt.Sprintf("amends_design on %s unchanged (already recorded).", ref))
		}
	}
	return strings.Join(lines, "\n")
}

// --- show amendment ---

// joinLiveReadErrors folds runShowAmendment's independent live-read errors
// (mutate.EvaluateAmendmentPreconditions and mutate.EvaluateAmendmentSource,
// the only two of the four evaluators runShowAmendment calls that can ever
// fail at all: EvaluateAmendmentOpSources and EvaluateAmendmentShape are
// pure over the doc already in memory and carry no error return to begin
// with, amendment_validity.go's own doc comments) into the one
// validity_error string, deduped by message: the two open with the SAME
// loadAmendmentDoc read, so a doc-load failure (a corrupt or hand-mangled
// yml) surfaces identically from both, and joining unconditionally would
// report the one failure twice ("X; X") rather than naming the single thing
// that actually went wrong. A nil error is skipped; order is preserved
// (preconditions, then Source) so a caller reading the joined string sees
// the same order the evaluators ran in.
//
// internal/serve/node.go's joinAmendmentValidityErrors is this function's
// console-side mirror, for the node page's own fillAmendmentValidity (#57):
// a faithful local copy rather than a shared export, since it is five lines
// with no state and threading an import between cli and serve for it would
// cost more than the duplication does. Change one, check the other.
func joinLiveReadErrors(errs ...error) string {
	seen := make(map[string]bool, len(errs))
	var out []string
	for _, err := range errs {
		if err == nil {
			continue
		}
		msg := err.Error()
		if seen[msg] {
			continue
		}
		seen[msg] = true
		out = append(out, msg)
	}
	return strings.Join(out, "; ")
}

// runShowAmendment is show.go's amendment arm: the full render, title through
// every staged operation's LIVE precondition validity, computed once by
// mutate.EvaluateAmendmentPreconditions rather than a second comparison here
// (single-sourcing the logic is the brief's own instruction; views purity is
// not at stake in this package, but the evaluation itself must never diverge
// from approve's own), plus an intake-minted amendment's own Source, computed
// the identical way through mutate.EvaluateAmendmentSource, plus
// every operation's own hand-blanked-citation pre-flight, computed the
// identical way through mutate.EvaluateAmendmentOpSources (#57), plus every
// operation's own shape pre-flight (a duplicate staged key, a staged-epic
// replacement), computed the identical way through mutate.EvaluateAmendmentShape
// (#57's own widened class).
//
// A batch that cannot even resolve (a vanished target, an epic named as one)
// does not fail the whole command: show is a read, and a stuck amendment is
// exactly what a human runs show to diagnose. The static fields and every
// operation's structural shape still render; only the live Status/Actual/Note
// columns go blank, with the reason(s) named once at the end. The four live
// reads are independent: a batch that cannot resolve still gets its Source
// checked (Source needs no operation resolution at all) and every op's own
// citation and shape checked (EvaluateAmendmentOpSources and
// EvaluateAmendmentShape need no resolution either), a vanished or changed
// Source still leaves every precondition's own live read intact, and a
// blanked op citation or a shape defect still leaves every other read
// intact — the combined-defect tests below pin all four firing on one
// batch at once.
func runShowAmendment(cmd *cobra.Command, addr string) error {
	tree, a, err := loadAmendmentTree(cmd, "show", addr)
	if err != nil {
		return err
	}

	validityList, verr := mutate.EvaluateAmendmentPreconditions(tree, mutate.Deps{}, a)
	validity := make(map[preKey]mutate.PreconditionValidity, len(validityList))
	if verr == nil {
		for _, pv := range validityList {
			validity[preKey{pv.Index, pv.Precondition}] = pv
		}
	}

	sourceValidity, serr := mutate.EvaluateAmendmentSource(tree, mutate.Deps{}, a)
	if serr != nil {
		sourceValidity = nil
	}

	opSourceList := mutate.EvaluateAmendmentOpSources(a)
	opSourceMissing := make(map[int]bool, len(opSourceList))
	for _, osv := range opSourceList {
		opSourceMissing[osv.Index] = true
	}

	shapeList := mutate.EvaluateAmendmentShape(a)
	opShape := make(map[int]string, len(shapeList))
	for _, sf := range shapeList {
		opShape[sf.Index] = sf.Kind
	}

	validityErr := joinLiveReadErrors(verr, serr)

	view := amendmentDataView(tree, a, validity, sourceValidity, opSourceMissing, opShape)
	human := renderAmendmentHuman(view, validityErr)
	if jsonOut(cmd) {
		out := struct {
			AmendmentData
			ValidityError string `json:"validity_error,omitempty"`
		}{AmendmentData: view, ValidityError: validityErr}
		return emitSuccess(cmd, "show", out, human)
	}
	return emitSuccess(cmd, "show", view, human)
}

// --- withdraw amendment ---

func newWithdrawCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "withdraw amendment <addr>",
		Short: "Drop a proposed amendment without applying it (-> withdrawn).",
		Long: "withdraw is the human dropping a proposed amendment before anything in\n" +
			"it is ever applied: proposed -> withdrawn. A --reason is required and\n" +
			"recorded on the event. withdraw is the amendment's own removal verb;\n" +
			"nothing it staged ever executes, and its key is free to reuse once\n" +
			"withdrawn.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != store.NounAmendment {
				return emitFailure(cmd, "withdraw", &usageError{
					reason: "unknown_noun",
					msg:    fmt.Sprintf("withdraw only takes the amendment noun, not %q", args[0]),
				})
			}
			return runWithdrawAmendment(cmd, args[1], reason)
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why the proposal is being dropped (required)")
	_ = cmd.MarkFlagRequired("reason")
	addContributorFlag(cmd)
	return cmd
}

// AmendmentWithdrawView is the --json payload of `withdraw amendment`.
type AmendmentWithdrawView struct {
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func runWithdrawAmendment(cmd *cobra.Command, addr, reason string) error {
	tree, a, err := loadAmendmentTree(cmd, "withdraw", addr)
	if err != nil {
		return err
	}
	ref := gen.Ref(tree, a)
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "withdraw", err)
	}
	res, err := mutate.WithdrawAmendment(tree, d, a, reason)
	if err != nil {
		return emitFailure(cmd, "withdraw", err)
	}
	view := AmendmentWithdrawView{ID: res.ID, Key: res.Key, Status: res.Status, Reason: res.Reason}
	human := fmt.Sprintf("Withdrew amendment %s: %s.", ref, res.Reason)
	return emitSuccess(cmd, "withdraw", view, human)
}

// --- approve amendment ---

// runApproveAmendmentDispatch handles `approve amendment <addr>` inside the
// shared `approve <noun> <node>...` command (lifecycle_cmds.go's
// newApproveCmd). An amendment's approve is not a lifecycle transition on the
// amendment itself: mutate.ApproveAmendment validates and applies a whole
// batch under one lock, so it never reaches runTransition, and none of
// approve's other acts under that verb (--refresh-basis, --feature,
// --reviewed-assumption) apply to an amendment.
func runApproveAmendmentDispatch(cmd *cobra.Command, args []string, refreshBasis bool, feature, reviewedAssumption string, reviewedAssumptionSet bool) error {
	if refreshBasis || feature != "" || reviewedAssumptionSet {
		return emitFailure(cmd, "approve", &usageError{
			reason: "bad_args",
			msg:    "approve amendment takes no --refresh-basis, --feature, or --reviewed-assumption",
		})
	}
	if len(args) != 2 {
		return emitFailure(cmd, "approve", &usageError{
			reason: "bad_args",
			msg:    "approve amendment takes exactly one address",
		})
	}
	return runApproveAmendment(cmd, args[1])
}

// AmendmentApproveView is the SUCCESS --json payload of `approve amendment`:
// the amendment now terminal, and every operation it applied.
type AmendmentApproveView struct {
	ID      string                 `json:"id"`
	Key     string                 `json:"key,omitempty"`
	Status  string                 `json:"status"`
	Applied []AmendmentAppliedView `json:"applied"`
}

// AmendmentAppliedView is one applied operation: id and key separate, never a
// composed label, and Replacement the resolved node id a supersede or a
// cancel's superseded_by landed on. Outcome is add-question's own
// (mutate.AppliedOperation's own doc comment), and so is Note apart from an
// applied correct-eval's commit hint: on add-question they distinguish a
// genuine no-op from real work, so a caller reading this payload alone — not
// just the human confirmation line — can tell the two apart.
type AmendmentAppliedView struct {
	Index       int                             `json:"index"`
	Op          string                          `json:"op"`
	Noun        string                          `json:"noun"`
	ID          string                          `json:"id"`
	Key         string                          `json:"key,omitempty"`
	Status      string                          `json:"status"`
	Replacement string                          `json:"replacement,omitempty"`
	Outcome     string                          `json:"outcome,omitempty"`
	Note        string                          `json:"note,omitempty"`
	Move        *store.FeatureMovePreconditions `json:"move,omitempty"`
}

func amendmentApproveView(res mutate.ApproveResult) AmendmentApproveView {
	out := AmendmentApproveView{ID: res.ID, Key: res.Key, Status: res.Status, Applied: []AmendmentAppliedView{}}
	for _, op := range res.Applied {
		out.Applied = append(out.Applied, AmendmentAppliedView{
			Index: op.Index, Op: op.Op, Noun: op.Noun, ID: op.NodeID, Key: op.Key,
			Status: op.Status, Replacement: op.Replacement, Outcome: op.Outcome, Note: op.Note,
			Move: op.Move,
		})
	}
	return out
}

// AmendmentStaleData is the BLOCKED (exit 1) --json data block for approve's
// stale-precondition refusal: every stale operation, never only the first,
// the same whole-batch report mutate.AmendmentPreconditionsStaleError
// carries.
type AmendmentStaleData struct {
	ID     string                 `json:"id"`
	Reason string                 `json:"reason"`
	Stale  []AmendmentStaleOpView `json:"stale"`
}

// AmendmentStaleOpView is one stale precondition: Target names the
// operation's subject (its resolved target, or its staged key for an add
// op), and Note carries the explanation where Actual alone would mislead
// (mutate.StalePrecondition's own doc comment).
type AmendmentStaleOpView struct {
	Index        int    `json:"index"`
	Op           string `json:"op"`
	Target       string `json:"target"`
	Precondition string `json:"precondition"`
	Expected     string `json:"expected"`
	Actual       string `json:"actual,omitempty"`
	Note         string `json:"note,omitempty"`
}

func amendmentStaleData(e *mutate.AmendmentPreconditionsStaleError) AmendmentStaleData {
	out := AmendmentStaleData{ID: e.AmendmentID, Reason: mutate.ReasonAmendmentPreconditionsStale, Stale: []AmendmentStaleOpView{}}
	for _, sp := range e.Stale {
		out.Stale = append(out.Stale, AmendmentStaleOpView{
			Index: sp.Index, Op: sp.Op, Target: sp.Target, Precondition: sp.Precondition,
			Expected: sp.Expected, Actual: sp.Actual, Note: sp.Note,
		})
	}
	return out
}

// humanAmendmentStale renders the blocked stale-precondition answer, one
// stale operation per line. Note is rendered in place of Actual when present
// (StalePrecondition's own doc comment): Actual alone would mislead for a
// comparison that could not even run (an add op's staged key, a target with
// no recorded anchor), where Note carries the reason instead.
func humanAmendmentStale(tree *store.Tree, ref string, e *mutate.AmendmentPreconditionsStaleError) string {
	var b strings.Builder
	fmt.Fprintf(&b, "amendment %s has %d stale precondition(s):", ref, len(e.Stale))
	for _, sp := range e.Stale {
		subject := gen.RefByID(tree, sp.Target)
		fmt.Fprintf(&b, "\n  operation %d (%s) %s: %s expected %q", sp.Index, sp.Op, subject, sp.Precondition, sp.Expected)
		if sp.Note != "" {
			fmt.Fprintf(&b, ", %s", sp.Note)
		} else {
			fmt.Fprintf(&b, ", actual %q", sp.Actual)
		}
	}
	return b.String()
}

// attributionClause names the attribution grains a batch minted. Callers
// invoke it only when at least one of epics or questions is positive.
func attributionClause(epics, questions int, owner string) string {
	switch {
	case epics > 0 && questions > 0:
		return fmt.Sprintf("%d epic(s) and %d question(s) minted and owned by %s", epics, questions, owner)
	case epics > 0:
		return fmt.Sprintf("%d epic(s) minted and owned by %s", epics, owner)
	default:
		return fmt.Sprintf("%d question(s) minted and owned by %s", questions, owner)
	}
}

// poolMinted reports whether op minted a POOL-grain node: a feature, and a
// feature alone. Under birth-unclaimed (design doc, "Approver owns what
// approve mints" is revised at feature and task grain) a minted feature is
// never stamped with the approver's id, so a batch that adds one is
// reporting a pool arrival, never an ownership stamp: the design's own
// closing sentence says so precisely ("it names the attribution grains and
// reports FEATURES as entering the pool").
//
// add-task is deliberately excluded, the fix for a review finding (OL3
// round 1): the design's own prose also called a minted task "ownerless",
// but that is not reliably true. planAddTask (add.go), the same planner
// materialize calls, stamps a fresh task with its PARENT FEATURE's current
// owner, not always empty: under a feature the acting contributor has already
// claimed (claim first, amend --add-task, approve, all reachable through
// the shipped CLI), the minted task lands owned by that claimant, not in
// any pool. Deciding "into the pool" from Op alone for add-task would print
// that false claim in exactly this ordinary sequence. The minimum honest
// fix, chosen over resolving each task's real landing owner from the
// pre-approve tree (op.Key against the parent feature's live claim state):
// a minted task gets NO ownership clause at all, honest under every
// feature-claim state without teaching the headline to walk the tree.
// TestApproveAmendmentAddTaskUnderAClaimedFeatureCarriesNoOwnershipClause
// pins the case that had no honest pin before this fix.
func poolMinted(op mutate.AppliedOperation) bool {
	return op.Op == store.AmendmentOpAddFeature
}

// poolClause names how many features a batch minted into the pool.
func poolClause(features int) string {
	return fmt.Sprintf("%d feature(s) into the pool", features)
}

// humanApproveAmendment names every applied operation by ref, the batch
// confirmation a human reviews before the gate readout follows it.
// tree is the pre-approve tree (still holding every node
// that existed before this call), used to resolve a replacement that
// pre-dates this batch; a replacement this SAME batch just materialized
// resolves against res.Applied instead, since a brand new node's key never
// reached that stale copy.
//
// owner is the acting contributor approve mints under, but the headline credits it
// only for the attribution grains it actually stamps. Every applied add-epic
// is fresh because an existing key refuses rather than folds. An add-question
// counts only when its outcome is AppliedCreated: a fold unions Blocks into
// an existing question without restamping Owner, and a no-change operation
// likewise minted nothing. Under birth-unclaimed a minted feature is never
// the approver's (poolMinted above), so it is reported separately as entering
// the pool. A batch minting both attribution and pool grains carries both
// clauses. A minted task carries neither, since its real landing owner depends
// on live tree state Op alone cannot decide honestly (poolMinted's own doc
// comment); a batch minting nothing at all keeps the plain headline.
func humanApproveAmendment(tree *store.Tree, ref string, res mutate.ApproveResult, owner string) string {
	byID := make(map[string]mutate.AppliedOperation, len(res.Applied))
	mintedEpics := 0
	mintedQuestions := 0
	pooledFeatures := 0
	for _, op := range res.Applied {
		byID[op.NodeID] = op
		switch op.Op {
		case store.AmendmentOpAddEpic:
			mintedEpics++
		case store.AmendmentOpAddQuestion:
			if op.Outcome == mutate.AppliedCreated {
				mintedQuestions++
			}
		}
		if poolMinted(op) {
			pooledFeatures++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Approved amendment %s: %d operation(s) applied", ref, len(res.Applied))
	switch {
	case (mintedEpics > 0 || mintedQuestions > 0) && pooledFeatures > 0:
		fmt.Fprintf(&b, "; %s; %s.", attributionClause(mintedEpics, mintedQuestions, owner), poolClause(pooledFeatures))
	case mintedEpics > 0 || mintedQuestions > 0:
		fmt.Fprintf(&b, "; %s.", attributionClause(mintedEpics, mintedQuestions, owner))
	case pooledFeatures > 0:
		fmt.Fprintf(&b, "; %s.", poolClause(pooledFeatures))
	default:
		b.WriteString(".")
	}
	for _, op := range res.Applied {
		line := fmt.Sprintf("\n  operation %d (%s) %s: %s", op.Index, op.Op, gen.RefParts(op.Key, op.NodeID), op.Status)
		if op.Replacement != "" {
			replRef := gen.RefByID(tree, op.Replacement)
			if r, ok := byID[op.Replacement]; ok {
				replRef = gen.RefParts(r.Key, r.NodeID)
			}
			line += fmt.Sprintf(", replaced by %s", replRef)
		}
		// Outcome is add-question's own (mutate.AppliedOperation's doc
		// comment), blank for the other six kinds: a genuine no-op still
		// shows up here (foldQuestion folds a duplicate rather than
		// refusing), so it says so rather than reading like every other
		// line's plain confirmation of real work done.
		if op.Outcome == mutate.AppliedNoChange {
			line += " (no change)"
		}
		if op.Note != "" {
			line += fmt.Sprintf("\n    %s", op.Note)
		}
		b.WriteString(line)
	}
	return b.String()
}

// runApproveAmendment is the human gate: it runs ApproveAmendment, renders
// the blocked stale-preconditions answer (exit 1) naming every stale
// operation, and on success names every applied operation by ref before
// printing the gate readout through the same renderReadoutFor path every
// other human gate uses (emitReportWithReadout). act carries what the batch
// applied (amendmentApprovedAct below), the same GateAct plumbing every other
// gate call site fills from its own mutation result, so approveContent
// (internal/views/readout.go) can give this gate real content instead of the
// empty Verification section a nil act leaves.
func runApproveAmendment(cmd *cobra.Command, addr string) error {
	tree, a, err := loadAmendmentTree(cmd, "approve", addr)
	if err != nil {
		return err
	}
	ref := gen.Ref(tree, a)

	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "approve", err)
	}
	res, err := mutate.ApproveAmendment(tree, d, a)
	if err != nil {
		var stale *mutate.AmendmentPreconditionsStaleError
		if errors.As(err, &stale) {
			return emitReport(cmd, "approve", false, amendmentStaleData(stale), humanAmendmentStale(tree, ref, stale), 1)
		}
		return emitFailure(cmd, "approve", err)
	}

	view := amendmentApproveView(res)
	// d.Contributor is already the resolved acting contributor (deps, above): the same
	// value humanApproveAmendment needs to name who minted nodes own,
	// threaded through rather than resolved a second time. The readout
	// routing and the JSON envelope's own "actor" field resolve separately,
	// in the shared envelope layer (resolveActor, envelope.go), which every
	// mutating command's report goes through and this one does not bypass.
	human := humanApproveAmendment(tree, ref, res, d.Contributor)
	return emitReportWithReadout(cmd, "approve", true, view, human, 0, gateMomentFor("approve"), tree.Root, amendmentApprovedAct(res))
}

// amendmentApprovedAct projects mutate.ApproveResult's own Applied onto the
// readout's GateAct, carrying only the op kind each operation applied: what
// approveContent's AmendmentApplied branch tallies by. Every other field on
// mutate.AppliedOperation (NodeID, Key, Status, Replacement) is already named
// in full by humanApproveAmendment's own per-operation confirmation line,
// printed directly above the block this act feeds, so nothing here repeats
// it (the same drop-what-nothing-downstream-reads discipline
// syncAwaitingAct, sync.go, already follows).
func amendmentApprovedAct(res mutate.ApproveResult) *views.GateAct {
	ops := make([]views.GateActAmendmentOp, len(res.Applied))
	for i, op := range res.Applied {
		ops[i] = views.GateActAmendmentOp{Op: op.Op}
	}
	return &views.GateAct{AmendmentApplied: &views.GateActAmendmentApplied{Applied: ops}}
}
