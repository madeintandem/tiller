package store

import (
	"fmt"
	"strings"
)

// This file holds the amendment: the reviewed, recorded staging document for a
// mid-flight reshape of the plan, the proposal unit for a reshape the way the
// draft tree is the proposal unit for an initial decomposition
// (artifact-schema.md, Amendments). Its on-disk state is a flat file pair under
// .tiller/amendments/: <A-id>-<key>.yml holds this shape, and the companion
// <A-id>-<key>.md holds the human-owned narrative, the same prose-and-state
// split every other node keeps. Unlike the epic/feature/task tree, an amendment
// is not folder-backed: a staged reshape is one reviewed document, not a
// subtree with children of its own, so it carries no parent linkage and no
// gates block.

// Amendment status values (artifact-schema.md, Amendments: States).
const (
	AmendmentStatusProposed  = "proposed"
	AmendmentStatusApproved  = "approved"
	AmendmentStatusWithdrawn = "withdrawn"
)

// Amendment operation kinds: the staged verbs an amendment's operations list
// may name. revise marks a feature or task revised in place, its
// preconditions pinning the spec or eval revision the change was reasoned
// against; supersede and cancel retire an existing node, either replacing it or
// closing it with a reason; correct-eval corrects the eval bindings of a done
// feature in a reviewed way, recording its reason and naming in Retire any
// criteria it deliberately removes; add-epic, add-feature, and add-task stage a new
// draft node the same shape `tiller add` would, held for approve to
// materialize atomically with the rest of the batch (artifact-schema.md,
// Amendments). add-epic is the one add op with no parent to resolve: an epic
// is a root, so it carries only a key and a title, no epic or feature segment
// the way add-feature and add-task's canonical keys do. add-question stages a
// new open question with a required one-line scanning title and independent
// full statement; unlike the other three add kinds it carries no key at all,
// since a question's identity is its statement rather than a folder
// (artifact-schema.md, "the one node class that carries an id and no key"),
// and its Blocks names the nodes it holds.
const (
	AmendmentOpRevise      = "revise"
	AmendmentOpSupersede   = "supersede"
	AmendmentOpCancel      = "cancel"
	AmendmentOpAddEpic     = "add-epic"
	AmendmentOpAddFeature  = "add-feature"
	AmendmentOpAddTask     = "add-task"
	AmendmentOpAddQuestion = "add-question"
	AmendmentOpMoveFeature = "move-feature"
	AmendmentOpCorrectEval = "correct-eval"
)

// Amendment is a reviewed, recorded change to the plan (artifact-schema.md,
// Amendments). Its on-disk state is <A-id>-<key>.yml.
type Amendment struct {
	ID    string  `yaml:"id"`
	Key   *string `yaml:"key,omitempty"`
	Title string  `yaml:"title,omitempty"`

	// Status is one of the three AmendmentStatus* values. proposed is the
	// staging state: while it holds, the operations below are proposals and
	// nothing is executed. approved and withdrawn are terminal (artifact-schema.md,
	// Amendments: States).
	Status string `yaml:"status,omitempty"`

	// Implements names the ADR this amendment carries out. It is optional: this
	// build carries no ADR node surface yet, so a human may leave it empty
	// (artifact-schema.md, Amendments).
	Implements string `yaml:"implements,omitempty"`

	// TriggeredBy and AmendsDesign both name design docs but answer different
	// questions. TriggeredBy is provenance: the doc whose change prompted this
	// reshape. AmendsDesign is the doc(s) this amendment's approval keeps
	// current, the durable-intent edits a human folds in alongside the staged
	// tree changes (artifact-schema.md, Amendments; Provenance).
	TriggeredBy  []string `yaml:"triggered_by,omitempty"`
	AmendsDesign []string `yaml:"amends_design,omitempty"`

	// BaseRevision is the repository revision the staged operations were
	// reasoned against, recorded at open from the current HEAD. It is a compact
	// "git:<sha>" token rather than a RevisionRef: unlike accepted_against and
	// verified_against, it pins one repository-wide anchor rather than a
	// specific file's content, so it needs no kind or path to disambiguate
	// (artifact-schema.md, Amendments).
	BaseRevision string `yaml:"base_revision,omitempty"`

	// Source names the feedback document this amendment was exploded from, when
	// it was minted by `tiller intake` rather than staged by hand
	// (docs/spec/artifact-schema.md). It is nil for every amendment
	// `tiller add amendment` opens; nothing in this build sets it yet
	// (intake, a later task, is the one writer). Its presence is what
	// StageOperation's per-op citation refusal keys on: every operation on an
	// intake-minted amendment must itself carry a Source.
	Source *AmendmentSource `yaml:"source,omitempty"`

	Operations []AmendmentOp `yaml:"operations,omitempty"`
}

func (a *Amendment) NodeID() string     { return a.ID }
func (a *Amendment) NodeKey() *string   { return a.Key }
func (a *Amendment) Noun() string       { return NounAmendment }
func (a *Amendment) NodeStatus() string { return a.Status }

// AmendmentSource is the fingerprinted feedback artifact an intake-minted
// amendment was exploded from (docs/spec/artifact-schema.md). Path is the
// artifact's repo-relative location, landed under .tiller/feedback/ by intake;
// Revision is its content fingerprint at intake time, a "git:<blob-sha>" token
// in the same compact idiom BaseRevision already uses (a bare string rather
// than a RevisionRef: like BaseRevision, this pins one artifact snapshot, not a
// field this package needs to disambiguate by kind or path).
type AmendmentSource struct {
	Path     string `yaml:"path,omitempty"`
	Revision string `yaml:"revision,omitempty"`
}

// AmendmentOp is one staged operation in an amendment's operations list. Which
// fields apply depends on Op: Target names the existing node a revise,
// supersede, or cancel acts on; By names a supersede's replacement; Key and
// Title stage a new add-epic, add-feature, or add-task node (add-epic's Key is
// a bare single-segment key, with no parent to resolve); ChangeType carries an
// add-feature's brownfield change type; Reason and SupersededBy record a
// cancel's justification and, when the cancelled work is replaced rather than
// simply dropped, its replacement. On add-question, Title is the required
// one-line scanning title and Statement is the required full question; the op
// has no Key. Blocks names the ids or keys it holds: an existing epic, feature,
// or task, or the key of an add-epic/add-feature/add-task staged earlier on
// this same amendment, whose id does not exist yet and is resolved to the
// materialized one when approve applies the batch (mutate/amendment.go's
// resolveStageBlockTarget). Statement and Blocks are legal only on
// add-question; validateAmendmentOp refuses either on any other op kind, the
// same misplaced-field check By and SupersededBy already get.
// Retire lists the criterion ids a correct-eval deliberately removes from the
// target feature's eval contract; it is legal only on correct-eval, which also
// requires a Target and a Reason.
// Preconditions is the guard approve validates against the live tree before
// applying anything (artifact-schema.md, Amendments). Source cites the feedback
// this operation traces to, free text by convention "path#Lstart-Lend" (also
// "path#Lstart-end"); relationship reads accept both without rewriting the source.
// It is legal on every op kind (docs/spec/artifact-schema.md), so
// validateAmendmentOp carries no per-kind rule for it. An intake-minted
// amendment (Amendment.Source set) requires every one of its ops to carry one;
// StageOperation enforces that, not this struct or ValidateAmendment, because
// the rule conditions on the amendment's own state (mutate/amendment.go's
// StageOperation).
type AmendmentOp struct {
	Op            string                    `yaml:"op"`
	Target        string                    `yaml:"target,omitempty"`
	By            string                    `yaml:"by,omitempty"`
	Key           string                    `yaml:"key,omitempty"`
	Title         string                    `yaml:"title,omitempty"`
	Statement     string                    `yaml:"statement,omitempty"`
	ChangeType    string                    `yaml:"change_type,omitempty"`
	Reason        string                    `yaml:"reason,omitempty"`
	SupersededBy  string                    `yaml:"superseded_by,omitempty"`
	Blocks        []string                  `yaml:"blocks,omitempty"`
	Source        string                    `yaml:"source,omitempty"`
	Preconditions map[string]string         `yaml:"preconditions,omitempty"`
	Move          *FeatureMovePreconditions `yaml:"move,omitempty" json:"move,omitempty"`
	Retire        []string                  `yaml:"retire,omitempty"`
}

// FeatureMovePreconditions pins the reviewed placement, policy and complete
// subtree bytes. Historical acceptance locators remain in those original bytes.
type FeatureMovePreconditions struct {
	SourceKey              string            `yaml:"source_key" json:"source_key"`
	SourceEpicID           string            `yaml:"source_epic_id" json:"source_epic_id"`
	DestinationEpicID      string            `yaml:"destination_epic_id,omitempty" json:"destination_epic_id,omitempty"`
	DestinationEpicKey     string            `yaml:"destination_epic_key" json:"destination_epic_key"`
	DestinationStaged      bool              `yaml:"destination_staged,omitempty" json:"destination_staged,omitempty"`
	ReviewGrain            string            `yaml:"review_grain" json:"review_grain"`
	MaterializeReviewGrain bool              `yaml:"materialize_review_grain,omitempty" json:"materialize_review_grain,omitempty"`
	OrderEffect            string            `yaml:"order_effect" json:"order_effect"`
	Files                  map[string]string `yaml:"files" json:"files"`
	Policies               map[string]string `yaml:"policies" json:"policies"`
	ReferenceWarnings      []string          `yaml:"reference_warnings,omitempty" json:"reference_warnings,omitempty"`
}

// AmendmentShapeError reports that a parsed amendment is not a well-formed
// staged reshape: an unrecognized status, or an operation naming an unknown op,
// carrying a field illegal for its op, or missing a field its op requires. It
// is a misuse (exit 2) the agent fixes by rewriting the amendment to the
// documented shape, the same class as EvalContractError. OpIndex is the
// zero-based position of the offending operation in Operations, or -1 for a
// file-level fault (the amendment's own status).
type AmendmentShapeError struct {
	OpIndex int
	Msg     string
}

func (e *AmendmentShapeError) Error() string  { return e.Msg }
func (e *AmendmentShapeError) ExitCode() int  { return 2 }
func (e *AmendmentShapeError) Reason() string { return "amendment_shape_invalid" }

// ValidateAmendment checks a parsed amendment is a well-formed staged reshape
// (artifact-schema.md, Amendments): Status must be one of the three recognized
// values, and every operation's Op must be one of the recognized kinds,
// carry only the fields legal for that kind, and carry every field that kind
// requires. It returns an AmendmentShapeError naming the first offending
// operation by index; a malformed amendment is never silently accepted.
// Precondition values and cross-references against the live tree are not
// checked here, since that needs a tree this package has no access to; that is
// approve's job.
func ValidateAmendment(a *Amendment) error {
	if a == nil {
		return &AmendmentShapeError{OpIndex: -1, Msg: "amendment is nil"}
	}

	switch a.Status {
	case AmendmentStatusProposed, AmendmentStatusApproved, AmendmentStatusWithdrawn:
	default:
		return &AmendmentShapeError{
			OpIndex: -1,
			Msg: fmt.Sprintf("amendment %s has an unrecognized status %q (want %s, %s, or %s)",
				a.ID, a.Status, AmendmentStatusProposed, AmendmentStatusApproved, AmendmentStatusWithdrawn),
		}
	}

	for i, op := range a.Operations {
		if err := validateAmendmentOp(i, op); err != nil {
			return err
		}
	}
	return nil
}

// validateAmendmentOp checks one staged operation against the shape its Op
// requires, returning a named AmendmentShapeError on the first defect.
func validateAmendmentOp(i int, op AmendmentOp) error {
	switch op.Op {
	case AmendmentOpRevise, AmendmentOpSupersede, AmendmentOpCancel,
		AmendmentOpAddEpic, AmendmentOpAddFeature, AmendmentOpAddTask, AmendmentOpAddQuestion, AmendmentOpMoveFeature,
		AmendmentOpCorrectEval:
	default:
		return amendmentOpErr(i, fmt.Sprintf(
			"operation %d has an unrecognized op %q (want %s, %s, %s, %s, %s, %s, %s, %s, or %s)",
			i, op.Op, AmendmentOpRevise, AmendmentOpSupersede, AmendmentOpCancel,
			AmendmentOpAddEpic, AmendmentOpAddFeature, AmendmentOpAddTask, AmendmentOpAddQuestion, AmendmentOpMoveFeature,
			AmendmentOpCorrectEval))
	}

	if op.SupersededBy != "" && op.Op != AmendmentOpCancel {
		return amendmentOpErr(i, fmt.Sprintf(
			"operation %d (%s) sets superseded_by, which only a %s operation may carry",
			i, op.Op, AmendmentOpCancel))
	}
	if op.By != "" && op.Op != AmendmentOpSupersede {
		return amendmentOpErr(i, fmt.Sprintf(
			"operation %d (%s) sets by, which only a %s operation may carry",
			i, op.Op, AmendmentOpSupersede))
	}
	if op.Statement != "" && op.Op != AmendmentOpAddQuestion {
		return amendmentOpErr(i, fmt.Sprintf(
			"operation %d (%s) sets statement, which only a %s operation may carry",
			i, op.Op, AmendmentOpAddQuestion))
	}
	if len(op.Blocks) > 0 && op.Op != AmendmentOpAddQuestion {
		return amendmentOpErr(i, fmt.Sprintf(
			"operation %d (%s) sets blocks, which only a %s operation may carry",
			i, op.Op, AmendmentOpAddQuestion))
	}
	if len(op.Retire) > 0 && op.Op != AmendmentOpCorrectEval {
		return amendmentOpErr(i, fmt.Sprintf(
			"operation %d (%s) sets retire, which only a %s operation may carry",
			i, op.Op, AmendmentOpCorrectEval))
	}
	if op.Move != nil && op.Op != AmendmentOpMoveFeature {
		return amendmentOpErr(i, "move preconditions are only legal on move-feature")
	}
	if op.Op == AmendmentOpMoveFeature {
		if strings.TrimSpace(op.Target) == "" || strings.TrimSpace(op.Key) == "" || strings.TrimSpace(op.Reason) == "" || op.Move == nil {
			return amendmentOpErr(i, "move-feature requires target, key, reason and captured move preconditions")
		}
		if op.Move.SourceKey == "" || op.Move.SourceEpicID == "" || op.Move.DestinationEpicKey == "" || len(op.Move.Files) == 0 || len(op.Move.Policies) == 0 {
			return amendmentOpErr(i, "move-feature has incomplete move preconditions")
		}
	}

	switch op.Op {
	case AmendmentOpAddEpic, AmendmentOpAddFeature, AmendmentOpAddTask:
		if strings.TrimSpace(op.Key) == "" {
			return amendmentOpErr(i, fmt.Sprintf("operation %d (%s) has no key", i, op.Op))
		}
	case AmendmentOpAddQuestion:
		if strings.TrimSpace(op.Title) == "" {
			return amendmentOpErr(i, fmt.Sprintf("operation %d (%s) has no title", i, op.Op))
		}
		if strings.ContainsAny(op.Title, "\r\n") {
			return amendmentOpErr(i, fmt.Sprintf("operation %d (%s) title must be one line", i, op.Op))
		}
		if strings.TrimSpace(op.Statement) == "" {
			return amendmentOpErr(i, fmt.Sprintf("operation %d (%s) has no statement", i, op.Op))
		}
	case AmendmentOpRevise, AmendmentOpSupersede, AmendmentOpCancel:
		if strings.TrimSpace(op.Target) == "" {
			return amendmentOpErr(i, fmt.Sprintf("operation %d (%s) has no target", i, op.Op))
		}
	case AmendmentOpCorrectEval:
		return validateCorrectEval(i, op)
	}
	return nil
}

// validateCorrectEval checks a correct-eval's required target and reason, and
// that its retire ids are non-blank and each named once. Ids compare trimmed,
// the form staging stores and approval matches criteria by, so a hand-edited
// " C7" is the same criterion as "C7".
func validateCorrectEval(i int, op AmendmentOp) error {
	if strings.TrimSpace(op.Target) == "" {
		return amendmentOpErr(i, fmt.Sprintf("operation %d (%s) has no target", i, op.Op))
	}
	if strings.TrimSpace(op.Reason) == "" {
		return amendmentOpErr(i, fmt.Sprintf("operation %d (%s) has no reason", i, op.Op))
	}
	seen := make(map[string]bool, len(op.Retire))
	for _, raw := range op.Retire {
		id := strings.TrimSpace(raw)
		if id == "" {
			return amendmentOpErr(i, fmt.Sprintf("operation %d (%s) retires a blank criterion id", i, op.Op))
		}
		if seen[id] {
			return amendmentOpErr(i, fmt.Sprintf("operation %d (%s) retires %s twice", i, op.Op, id))
		}
		seen[id] = true
	}
	return nil
}

// amendmentOpErr builds a named AmendmentShapeError for operation i.
func amendmentOpErr(i int, msg string) *AmendmentShapeError {
	return &AmendmentShapeError{OpIndex: i, Msg: msg}
}
