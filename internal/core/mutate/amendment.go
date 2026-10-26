package mutate

// This file holds the mutate layer for amendments (docs/spec/cli-surface.md),
// the CLI's `tiller add amendment`, `tiller amend`, and `tiller withdraw
// amendment`.
//
// It builds on Task 1's store shapes rather than reinventing them:
// store.Amendment / store.AmendmentOp are the on-disk record, and
// store.ValidateAmendment is the one place a staged op's shape is judged
// structurally; StageOperation runs it as a final check after appending, so a
// shape defect this file has no bespoke reason for still refuses before it
// reaches disk.
//
// An amendment is not folder-backed, so unlike AddEpic/AddFeature/AddTask
// there is no idempotent re-add: AddAmendment always opens a new proposal,
// refusing outright on a key collision among other PROPOSED amendments. An
// approved or withdrawn amendment's key is free again, because it is no
// longer an open proposal to collide with.
//
// StageOperation and StageAmendsDesign only ever touch a proposed amendment's
// own yml; they never read or write the live epic/feature/task tree, which is
// what lets a staged reshape be reviewed and edited freely before approve (a
// later task) applies it atomically.

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// Event kinds for the amendment verbs. They are named literally rather than in
// the dotted style the rest of the event stream uses (add.epic, link.feature),
// because these three tokens are pinned exactly.
const (
	eventAmendmentOpened    = "amendment_opened"
	eventAmendmentStaged    = "amendment_staged"
	eventAmendmentWithdrawn = "amendment_withdrawn"
)

// amendmentKeyRe is a mutate-local copy of internal/cli's keyRe (init.go):
// lowercase segments joined by single hyphens. It cannot be imported from cli,
// since cli already imports mutate. Unlike AddEpic/AddFeature/AddTask, whose
// keys are ALSO validated at the CLI layer before the core sees them, neither
// `add amendment` nor `tiller intake` keeps an independent CLI-side copy of
// this pattern for the amendment key: both route through
// PrecheckAddAmendment (this file) instead, so this var and cli's keyRe are
// the only two copies of the grammar, not three.
var amendmentKeyRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// The precondition names a staged operation's map may carry (artifact-schema.md,
// Amendments). Each is a guard approve validates against the
// live tree before applying anything; what each one compares is set out where
// it is evaluated (amendment_approve.go's evalPrecondition).
const (
	preconditionTargetStatus   = "target_status"
	preconditionTargetRevision = "target_revision"
	preconditionKeyAbsent      = "key_absent"
	preconditionSpecRevision   = "spec_revision"
	preconditionEvalRevision   = "eval_revision"
)

// preconditionNames is that fixed set as a list, in the order approve evaluates
// them. The order is load-bearing: it is what makes a stale-precondition report
// deterministic rather than a map's shuffle, so a human comparing two runs sees
// the same lines in the same places.
var preconditionNames = []string{
	preconditionTargetStatus,
	preconditionTargetRevision,
	preconditionKeyAbsent,
	preconditionSpecRevision,
	preconditionEvalRevision,
}

// knownPreconditionNames is preconditionNames as a set, for the staging-time
// shape check. It is derived rather than written twice, so a name can only ever
// be added in one place.
var knownPreconditionNames = func() map[string]bool {
	out := make(map[string]bool, len(preconditionNames))
	for _, name := range preconditionNames {
		out[name] = true
	}
	return out
}()

// AmendmentInput is the request for AddAmendment: the new amendment's key, an
// optional title, the ADR it implements, the design doc(s) whose change
// triggered it, and the design doc(s) its approval keeps current
// (artifact-schema.md, Amendments; Provenance). Source is nil for every hand
// `tiller add amendment`; `tiller intake` (a later task) is the one caller
// that sets it, riding along unconditionally the same inert way
// Implements/TriggeredBy/AmendsDesign already do. Folding Source in
// here, rather than a second mutation call after AddAmendment returns, is what
// keeps the amendment_opened event singular: the brief's own "keep one event,
// not two" is only satisfiable if the whole amendment (Source included) lands
// in AddAmendment's one write, since every other mutate function in this file
// that touches an already-open amendment's yml (StageOperation,
// StageAmendsDesign, WithdrawAmendment) commits and emits its own event, and a
// second such call here would double both.
type AmendmentInput struct {
	Key          string
	Title        string
	Implements   string
	TriggeredBy  []string
	AmendsDesign []string
	Source       *store.AmendmentSource
}

// AmendmentResult describes a freshly opened amendment. Path is the amendment's
// yml, the meaningful location for a flat file pair that carries no folder of
// its own.
type AmendmentResult struct {
	ID           string
	Key          string
	Title        string
	Status       string
	Implements   string
	TriggeredBy  []string
	AmendsDesign []string
	BaseRevision string
	Path         string
	Source       *store.AmendmentSource
}

// AddAmendment opens a new amendment: a proposed staging document anchored to
// the current HEAD, with an empty operations list a caller stages onto through
// StageOperation. Re-running with the same key is not idempotent the way
// AddEpic/AddFeature/AddTask are, because a second open amendment under one
// key is not the same proposal as the first; it refuses instead, unless the
// key's earlier holder is no longer proposed.
func AddAmendment(tree *store.Tree, deps Deps, in AmendmentInput) (AmendmentResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return AmendmentResult{}, err
	}

	// Every precondition that does not itself require a write lives in
	// PrecheckAddAmendment, shared with any caller that has a side effect of
	// its own to sequence before this function's mint (tiller intake's
	// artifact copy, which must land before the amendment exists so its
	// fingerprint can be computed into Source). Calling the shared function
	// here rather than re-inlining the same three checks is what keeps this
	// copy and intake's pre-copy copy provably identical.
	head, err := PrecheckAddAmendment(tree, deps, in.Key)
	if err != nil {
		return AmendmentResult{}, err
	}

	aid, err := deps.mintIn(tree, "A")
	if err != nil {
		return AmendmentResult{}, err
	}

	key := in.Key
	amendsDesign := dedupPreserveOrder(in.AmendsDesign)
	doc := store.Amendment{
		ID:           aid,
		Key:          &key,
		Title:        in.Title,
		Status:       store.AmendmentStatusProposed,
		Implements:   in.Implements,
		TriggeredBy:  in.TriggeredBy,
		AmendsDesign: amendsDesign,
		BaseRevision: head.Value,
		Source:       in.Source,
	}
	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return AmendmentResult{}, err
	}

	relYml := amendmentRelYML(&doc)
	relMd := amendmentRelMD(&doc)

	ev := events.NewEvent(deps.clock(), eventAmendmentOpened, owner.ID, "add amendment "+in.Key, aid)
	// EV ids are appended to the event ledger, not keyed into the tree's id
	// maps, so there is nothing to retry against (artifact-schema.md, The eval
	// model: the AT- and EV- classes are separate non-node identifiers).
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"status": store.AmendmentStatusProposed, "key": in.Key}
	// The artifact path rides the SAME event intake mints, present only
	// when this call is intake's own (in.Source set); a hand `add amendment`
	// carries no source key at all, so this event's After is unchanged from
	// before Source existed.
	if in.Source != nil {
		ev.After["source"] = in.Source.Path
	}
	ev.Files = []events.FileChange{
		{Path: relMd},
		{Path: relYml},
	}

	writes := []store.PlannedWrite{
		store.PlanWrite(filepath.Join(tree.Root, relMd), store.AbsentSnapshot(), []byte(renderAmendmentMD(heading(in.Title, in.Key)))),
		store.PlanWrite(filepath.Join(tree.Root, relYml), store.AbsentSnapshot(), ydata),
	}
	if err := commitMutation(tree, "add amendment", writes, []events.Event{ev}); err != nil {
		return AmendmentResult{}, err
	}

	return AmendmentResult{
		ID:           aid,
		Key:          in.Key,
		Title:        in.Title,
		Status:       store.AmendmentStatusProposed,
		Implements:   in.Implements,
		TriggeredBy:  in.TriggeredBy,
		AmendsDesign: amendsDesign,
		BaseRevision: head.Value,
		Path:         relYml,
		Source:       in.Source,
	}, nil
}

// PrecheckAddAmendment validates every AddAmendment precondition that does
// not itself require a write: the key's grammar, whether a PROPOSED
// amendment already holds it, and whether HEAD resolves (the commit anchor
// BaseRevision pins, the same read-only git window approve pins
// accepted_against.Commit through, transition.go's recordBasis). AddAmendment
// calls it internally, right after resolving the owner, so the two never
// drift apart.
//
// It is also exported so a caller with an irreversible side effect of its
// own to sequence BEFORE AddAmendment's mint can validate first: `tiller
// intake` must land its artifact copy before minting the amendment (the
// fingerprint Source carries can only be computed once the copy exists), but
// a key collision AddAmendment would otherwise discover only after that copy
// lands orphans the artifact for good — the copy sits on disk with no
// amendment referencing it, and every retry (even under a different --key)
// hits intake's own basename-collision refusal for a document that never
// actually landed. Running this check first closes that window: a caller
// with side effects to sequence before a mint runs every precondition this
// function can answer without writing anything, THEN performs its side
// effect, THEN calls AddAmendment, which re-runs these same checks a moment
// later (redundant against a single caller, but AddAmendment must remain
// self-contained for every OTHER caller that never pre-checks) — the same
// belt-and-suspenders precedent StageOperation's own re-validation against
// store.ValidateAmendment already sets in this file. The gap this cannot
// close is the ordinary generation-check race any two mutations can hit
// (a concurrent process opening the same key between this call and the
// eventual write), which mutate's own mutation lock already catches at
// commit time, the same as everywhere else in this codebase.
func PrecheckAddAmendment(tree *store.Tree, deps Deps, key string) (store.RevisionRef, error) {
	if !amendmentKeyRe.MatchString(key) {
		return store.RevisionRef{}, &usageError{
			reason: "amendment_key_invalid",
			msg:    fmt.Sprintf("amendment key %q must match [a-z0-9]+(-[a-z0-9]+)*", key),
		}
	}
	if openAmendmentKeyExists(tree, key) {
		return store.RevisionRef{}, &usageError{
			reason: "amendment_exists",
			msg:    fmt.Sprintf("a proposed amendment already carries the key %q; withdraw or approve it before reusing the key", key),
		}
	}
	head, err := deps.rev(tree.Root).CurrentCommit()
	if err != nil {
		return store.RevisionRef{}, err
	}
	return head, nil
}

// openAmendmentKeyExists reports whether a PROPOSED amendment already carries
// key. An approved or withdrawn amendment is terminal and no longer an open
// proposal, so its key is free to reuse.
func openAmendmentKeyExists(tree *store.Tree, key string) bool {
	for _, a := range tree.Amendments {
		if a.Status == store.AmendmentStatusProposed && a.Key != nil && *a.Key == key {
			return true
		}
	}
	return false
}

// dedupPreserveOrder drops repeats from list, keeping the first occurrence of
// each value and every value's relative order, the same shape unionStrings
// produces when it grows an existing list.
func dedupPreserveOrder(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(list))
	out := make([]string, 0, len(list))
	for _, v := range list {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// renderAmendmentMD is the prose stub for a freshly opened amendment: an H1
// title, then three H2 sections (Why, What changes, What to watch) each
// carrying one placeholder sentence, in the style of the task.md stub
// `tiller add task` seeds (add.go's renderTaskMD), reshaped so its own three
// sections start the human's narrative rather than sitting bare.
func renderAmendmentMD(title string) string {
	return fmt.Sprintf("# %s\n\n"+
		"Amendment opened by `tiller add amendment`. Write the narrative for this\n"+
		"reshape here before staging its operations.\n\n"+
		"## Why\n\nPlaceholder: explain what prompted this reshape.\n\n"+
		"## What changes\n\nPlaceholder: summarize the operations this amendment stages.\n\n"+
		"## What to watch\n\nPlaceholder: note risks or follow-up review this reshape calls for.\n",
		title)
}

// amendmentFileStem is the shared basename the .yml and the .md both write to,
// <A-id>-<key> (internal/store/amendment.go).
func amendmentFileStem(a *store.Amendment) string {
	return a.ID + "-" + deref(a.Key)
}

func amendmentRelYML(a *store.Amendment) string {
	return filepath.Join(".tiller", "amendments", amendmentFileStem(a)+".yml")
}

func amendmentRelMD(a *store.Amendment) string {
	return filepath.Join(".tiller", "amendments", amendmentFileStem(a)+".md")
}

// loadAmendmentDoc reads an amendment's yml fresh from disk, keyed off the
// already-resolved node's id and key, and confirms the file actually holds the
// addressed amendment. Every stage/withdraw write reads this fresh copy rather
// than trusting the in-memory tree, the same discipline loadTaskFile (add.go)
// uses one grain down.
func loadAmendmentDoc(tree *store.Tree, amendment *store.Amendment) (doc *store.Amendment, relYml string, snapshot store.FileSnapshot, err error) {
	relYml = amendmentRelYML(amendment)
	doc = &store.Amendment{}
	snapshot, err = readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, relYml), doc)
	if err != nil {
		return nil, "", store.FileSnapshot{}, err
	}
	if doc.ID != amendment.ID {
		return nil, "", store.FileSnapshot{}, fmt.Errorf("mutate: %s holds amendment %s, not the addressed %s", relYml, doc.ID, amendment.ID)
	}
	return doc, relYml, snapshot, nil
}

// amendmentNotOpenErr refuses an action that only a proposed amendment
// accepts, naming the amendment's actual status.
func amendmentNotOpenErr(doc *store.Amendment) error {
	return &usageError{
		reason: "amendment_not_open",
		msg:    fmt.Sprintf("amendment %s is %s, not %s", doc.ID, doc.Status, store.AmendmentStatusProposed),
	}
}

// sourcelessOpErr refuses an operation staged with no citation onto an
// intake-minted amendment (docs/spec/cli-surface.md). The message is pinned
// verbatim.
func sourcelessOpErr() error {
	return &usageError{
		reason: "source_required",
		msg:    "intake amendments trace every operation: pass --source citing the feedback document",
	}
}

// StageInput is one operation proposed onto an already-open amendment. Which
// fields apply depends on Op, the same shape store.AmendmentOp carries: Target
// names a revise/supersede/cancel's existing node; By names a supersede's
// replacement; Key/Title stage an add-epic/add-feature/add-task, ChangeType
// riding along for add-feature only; Reason and SupersededBy record a
// cancel's justification and optional replacement; Title and Statement are
// add-question's independent one-line scanning title and full question; Blocks
// names the nodes that question holds, each resolved against the live tree or
// a same-amendment staged add's key the same way By is
// (resolveStageBlockTarget, mirroring resolveStageReplacement); Source cites
// the feedback this operation traces to, legal on every op kind; Retire names
// the criteria a correct-eval deliberately removes and is legal on that op
// alone; Preconditions
// is the guard approve validates before applying anything. StageOperation
// requires a non-blank Source only when the amendment being staged onto is
// itself intake-minted (its own Source is set); a hand amendment leaves it
// optional (docs/spec/cli-surface.md).
type StageInput struct {
	Op            string
	Target        string
	By            string
	Key           string
	Title         string
	Statement     string `yaml:"statement,omitempty"`
	ChangeType    string
	Reason        string
	SupersededBy  string
	Blocks        []string
	Retire        []string
	Source        string
	Preconditions map[string]string
}

// StageResult is the outcome of one StageOperation call: the amendment it
// landed on, the op as staged (targets and replacements resolved to ids where
// the tree could resolve them), and its position in the amendment's operations
// list.
type StageResult struct {
	AmendmentID string
	Op          store.AmendmentOp
	Index       int
}

// StageOperation appends one staged operation to an already-open amendment and
// rewrites its yml atomically. It validates against the live tree (an unknown
// or epic target, an unknown replacement) and against the op's own shape (a
// misplaced by/superseded_by, a malformed precondition), but asserts nothing
// about the live tree beyond id existence: whether a precondition still holds
// is approve's question, not staging's.
func StageOperation(tree *store.Tree, deps Deps, amendment *store.Amendment, in StageInput) (StageResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return StageResult{}, err
	}

	doc, relYml, snapshot, err := loadAmendmentDoc(tree, amendment)
	if err != nil {
		return StageResult{}, err
	}
	if doc.Status != store.AmendmentStatusProposed {
		return StageResult{}, amendmentNotOpenErr(doc)
	}
	// An intake-minted amendment (doc.Source set) traces every operation to the
	// feedback that motivated it; a hand amendment (`tiller add amendment`)
	// carries no Source and leaves --source optional, exactly as before this
	// field existed. This conditions on the amendment's own state, which is why
	// it lives here rather than in store.ValidateAmendment (that package has no
	// notion of "this amendment" beyond the one op it is currently judging).
	if doc.Source != nil && strings.TrimSpace(in.Source) == "" {
		return StageResult{}, sourcelessOpErr()
	}

	op, err := buildStagedOp(tree, deps, doc, in)
	if err != nil {
		return StageResult{}, err
	}
	doc.Operations = append(doc.Operations, op)
	if err := checkMoveConflicts(tree, doc); err != nil {
		return StageResult{}, err
	}

	// Belt and suspenders: the shape just built must still satisfy Task 1's own
	// validator before it reaches disk. Every defect this function has a
	// bespoke reason for is already caught above; this is the backstop for
	// anything else ValidateAmendment judges (starting with an op kind outside
	// the seven this package knows how to build).
	if err := store.ValidateAmendment(doc); err != nil {
		return StageResult{}, err
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return StageResult{}, err
	}
	index := len(doc.Operations) - 1

	ev := events.NewEvent(deps.clock(), eventAmendmentStaged, owner.ID, fmt.Sprintf("amend %s --%s", doc.ID, op.Op), doc.ID)
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"op": op.Op, "index": index}
	if op.Key != "" {
		ev.After["key"] = op.Key
		if parentKey, _, ok := strings.Cut(op.Key, "/"); ok {
			noun := store.NounEpic
			if op.Op == store.AmendmentOpAddTask {
				parentKey = op.Key[:strings.LastIndex(op.Key, "/")]
				noun = store.NounFeature
			}
			if parent, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: noun, Raw: parentKey}); err == nil {
				ev.After["parent"] = parent.NodeID()
			}
		}
	}
	ev.Files = []events.FileChange{{Path: relYml}}

	if err := commitNodeWrites(tree, "amend."+op.Op, []nodeWrite{
		{rel: relYml, data: data, before: snapshot, event: ev},
	}); err != nil {
		return StageResult{}, err
	}

	return StageResult{AmendmentID: doc.ID, Op: op, Index: index}, nil
}

// buildStagedOp validates in against the shape its Op requires and returns the
// store.AmendmentOp to append, with Target/By/SupersededBy resolved to ids
// where the tree could resolve them.
func buildStagedOp(tree *store.Tree, deps Deps, doc *store.Amendment, in StageInput) (store.AmendmentOp, error) {
	if err := validatePreconditions(in.Preconditions); err != nil {
		return store.AmendmentOp{}, err
	}

	// Source rides on every op kind unconditionally, unlike By/SupersededBy
	// below (which are legal on some kinds and refused on others): the intake
	// design makes a source citation legal, and on an intake-minted amendment
	// required, for all seven staged verbs alike (store/amendment.go's
	// AmendmentOp doc comment).
	op := store.AmendmentOp{Op: in.Op, Source: in.Source, Preconditions: in.Preconditions}
	if in.Statement != "" && in.Op != store.AmendmentOpAddQuestion {
		return store.AmendmentOp{}, statementMisplacedErr()
	}

	// Blocks is legal on exactly one kind, unlike By/SupersededBy (each legal
	// on one of several others below), so it gets one check here rather than
	// a repeated one inside every other arm: a caller naming --blocks on
	// anything but add-question is refused loudly, the same misplaced-field
	// treatment By and SupersededBy already get, and the same enforcement
	// store.validateAmendmentOp's own backstop carries (store/amendment.go).
	if len(in.Blocks) > 0 && in.Op != store.AmendmentOpAddQuestion {
		return store.AmendmentOp{}, blocksMisplacedErr()
	}

	// Retire is legal on correct-eval alone, checked once here for the same
	// reason Blocks is above.
	if len(in.Retire) > 0 && in.Op != store.AmendmentOpCorrectEval {
		return store.AmendmentOp{}, retireMisplacedErr()
	}

	switch in.Op {
	case store.AmendmentOpMoveFeature:
		return stageFeatureMove(tree, doc, in)
	case store.AmendmentOpCorrectEval:
		return stageCorrectEval(tree, deps, in)
	case store.AmendmentOpRevise, store.AmendmentOpSupersede, store.AmendmentOpCancel:
		target, err := resolveStageTarget(tree, in.Target)
		if err != nil {
			return store.AmendmentOp{}, err
		}
		op.Target = target

		if in.Op == store.AmendmentOpSupersede {
			if strings.TrimSpace(in.By) == "" {
				return store.AmendmentOp{}, supersedeByRequiredErr()
			}
			by, err := resolveStageReplacement(tree, doc.Operations, in.By)
			if err != nil {
				return store.AmendmentOp{}, err
			}
			op.By = by
		} else if in.By != "" {
			return store.AmendmentOp{}, supersedeByMisplacedErr()
		}

		if in.Op == store.AmendmentOpCancel {
			op.Reason = in.Reason
			if in.SupersededBy != "" {
				supersededBy, err := resolveStageReplacement(tree, doc.Operations, in.SupersededBy)
				if err != nil {
					return store.AmendmentOp{}, err
				}
				op.SupersededBy = supersededBy
			}
		} else if in.SupersededBy != "" {
			return store.AmendmentOp{}, supersededByMisplacedErr()
		}

	case store.AmendmentOpAddEpic:
		// An epic is a root: unlike add-feature and add-task, there is no
		// parent to resolve and no key to split, so this arm is the shortest
		// of the three add kinds.
		if in.By != "" {
			return store.AmendmentOp{}, supersedeByMisplacedErr()
		}
		if in.SupersededBy != "" {
			return store.AmendmentOp{}, supersededByMisplacedErr()
		}
		op.Key = in.Key
		op.Title = in.Title

	case store.AmendmentOpAddFeature:
		if in.By != "" {
			return store.AmendmentOp{}, supersedeByMisplacedErr()
		}
		if in.SupersededBy != "" {
			return store.AmendmentOp{}, supersededByMisplacedErr()
		}
		epicKey, _, err := splitFeatureKey(in.Key)
		if err != nil {
			return store.AmendmentOp{}, err
		}
		if err := resolveStageParentEpic(tree, doc.Operations, epicKey); err != nil {
			return store.AmendmentOp{}, err
		}
		op.Key = in.Key
		op.Title = in.Title
		op.ChangeType = in.ChangeType

	case store.AmendmentOpAddTask:
		if in.By != "" {
			return store.AmendmentOp{}, supersedeByMisplacedErr()
		}
		if in.SupersededBy != "" {
			return store.AmendmentOp{}, supersededByMisplacedErr()
		}
		epicKey, featureKey, _, err := splitTaskKey(in.Key)
		if err != nil {
			return store.AmendmentOp{}, err
		}
		// Unlike add-feature above, add-task always requires a LIVE feature: a
		// new feature's task explosion is specify's act, run after the plan
		// gate, so a same-batch staged add-feature never rescues an add-task's
		// parent (ruled 2026-08-12).
		if _, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounFeature, Raw: epicKey + "/" + featureKey}); err != nil {
			return store.AmendmentOp{}, err
		}
		op.Key = in.Key
		op.Title = in.Title

	case store.AmendmentOpAddQuestion:
		// A question is not a node: no key, no parent to resolve, and no by
		// or superseded_by (those only ever record a lineage edge, and a
		// question has none). Its required one-line Title and full Statement
		// remain independent all the way to foldQuestion.
		if in.By != "" {
			return store.AmendmentOp{}, supersedeByMisplacedErr()
		}
		if in.SupersededBy != "" {
			return store.AmendmentOp{}, supersededByMisplacedErr()
		}
		if err := validateQuestionTitle(in.Title); err != nil {
			return store.AmendmentOp{}, err
		}
		if strings.TrimSpace(in.Statement) == "" {
			return store.AmendmentOp{}, statementRequiredErr()
		}
		op.Title = in.Title
		op.Statement = in.Statement
		for _, raw := range in.Blocks {
			resolved, err := resolveStageBlockTarget(tree, doc.Operations, raw)
			if err != nil {
				return store.AmendmentOp{}, err
			}
			op.Blocks = append(op.Blocks, resolved)
		}

	default:
		// An op kind outside the seven known verbs carries no bespoke refusal
		// here; the caller runs store.ValidateAmendment right after this
		// returns, which is where that judgment belongs (see StageOperation).
	}

	// Self-supersession is judged last, after every shape check above, so an
	// operation that is BOTH malformed and self-referential still answers about
	// its shape first: --superseded-by on a supersede is misplaced whatever it
	// names, and saying so is more useful than arguing about the id.
	if op.Target != "" && (op.By == op.Target || op.SupersededBy == op.Target) {
		return store.AmendmentOp{}, selfSupersessionErr(op.Target)
	}

	return op, nil
}

// validatePreconditions refuses a precondition map naming anything outside
// knownPreconditionNames, or naming a known field with a blank value. Which
// names apply to which op is approve's business; this only judges whether the
// map itself is well-formed.
func validatePreconditions(pre map[string]string) error {
	for name, value := range pre {
		if !knownPreconditionNames[name] {
			return &usageError{
				reason: "precondition_invalid",
				msg: fmt.Sprintf("unrecognized precondition %q (want target_status, target_revision, key_absent, spec_revision, or eval_revision)",
					name),
			}
		}
		if strings.TrimSpace(value) == "" {
			return &usageError{
				reason: "precondition_invalid",
				msg:    fmt.Sprintf("precondition %q needs a non-empty value", name),
			}
		}
	}
	return nil
}

// classifyStageTarget resolves raw against the live tree by id or by key,
// reporting the node if it is a feature or a task, and separately whether raw
// named an epic instead. An amendment reshapes existing structure and an epic
// reshapes only through its features (cli-surface.md, Large changes:
// amendments), so a target or a replacement naming one is refused with its own
// reason rather than folded into "unknown".
func classifyStageTarget(tree *store.Tree, raw string) (node store.Node, isEpic bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	r := resolve.NewResolver(tree)
	if id.Valid(raw) {
		n, err := r.Resolve(resolve.Ref{Raw: raw})
		if err != nil {
			return nil, false
		}
		return classifyResolvedTarget(n)
	}

	noun := store.NounEpic
	switch strings.Count(raw, "/") {
	case 1:
		noun = store.NounFeature
	case 2:
		noun = store.NounTask
	}
	n, err := r.Resolve(resolve.Ref{Noun: noun, Raw: raw})
	if err != nil {
		return nil, false
	}
	return classifyResolvedTarget(n)
}

func classifyResolvedTarget(n store.Node) (store.Node, bool) {
	switch n.Noun() {
	case store.NounFeature, store.NounTask:
		return n, false
	case store.NounEpic:
		return nil, true
	default:
		return nil, false
	}
}

// resolveStageTarget resolves a revise/supersede/cancel's target to its id.
func resolveStageTarget(tree *store.Tree, raw string) (string, error) {
	n, isEpic := classifyStageTarget(tree, raw)
	if isEpic {
		return "", epicTargetErr()
	}
	if n == nil {
		return "", unknownTargetErr(raw)
	}
	return n.NodeID(), nil
}

// resolveStageReplacement resolves a supersede's --by or a cancel's
// --superseded-by. Either name may address an existing feature or task (id or
// key, resolved to its id), or the key of an add-feature/add-task operation
// already staged earlier on this same amendment, whose id does not exist yet
// and is resolved only when approve materializes it. Deliberately not add-epic:
// see the loop below.
func resolveStageReplacement(tree *store.Tree, staged []store.AmendmentOp, raw string) (string, error) {
	n, isEpic := classifyStageTarget(tree, raw)
	if isEpic {
		return "", epicTargetErr()
	}
	if n != nil {
		return n.NodeID(), nil
	}
	// Deliberately not store.AmendmentOpAddEpic here: a supersede or a cancel
	// only ever targets a feature or a task, so a staged epic's key is never a
	// legal replacement and this loop must not recognize it. A --by or
	// --superseded-by naming one refuses unknown_target right here, at stage
	// time, before it ever reaches the amendment's own yml. The only way such
	// a shape reaches approve at all is a hand-edited amendment (the same
	// class of defect resolveAmendment's own re-validation guards against
	// elsewhere); resolveAmendment's staged-key bookkeeping DOES include
	// add-epic (for its own duplicate-key check), but its replacement
	// resolution enforces this exact same exclusion a second time
	// (stagedEpicReplacementErr, amendment_approve.go), rather than letting
	// that bookkeeping resolve the key and leaving it to checkSupersessionGraph's
	// unrelated noun check to catch by accident.
	for _, op := range staged {
		if (op.Op == store.AmendmentOpAddFeature || op.Op == store.AmendmentOpAddTask) && op.Key == raw {
			return raw, nil
		}
	}
	return "", unknownTargetErr(raw)
}

// resolveStageBlockTarget resolves one --blocks entry staged on an
// add-question op: an existing epic, feature, or task (id or key, resolved to
// its id, the same noun-aware dispatch resolveBlockTarget already gives the
// live `tiller add question` path in question.go), or the key of an
// add-epic/add-feature/add-task operation already staged earlier on this same
// amendment, whose id does not exist yet and is resolved only when approve
// materializes it — the same same-batch allowance resolveStageReplacement
// gives a supersede's by, above, extended to epics here since a question can
// block one and a supersede or cancel never targets one.
func resolveStageBlockTarget(tree *store.Tree, staged []store.AmendmentOp, raw string) (string, error) {
	n, err := resolveBlockTarget(resolve.NewResolver(tree), raw)
	if err == nil {
		return n.NodeID(), nil
	}
	var notFound *resolve.ErrNotFound
	if !errors.As(err, &notFound) {
		// A definite refusal (raw named a non-blockable class like B- or
		// Q-, question.go's own resolveBlockTarget): no staged key could
		// rescue an address like that either, so propagate the refusal
		// resolveBlockTarget already built rather than searching further.
		return "", err
	}
	for _, op := range staged {
		switch op.Op {
		case store.AmendmentOpAddEpic, store.AmendmentOpAddFeature, store.AmendmentOpAddTask:
			if op.Key == raw {
				return raw, nil
			}
		}
	}
	return "", blocksUnknownTargetErr(raw)
}

func blocksUnknownTargetErr(raw string) error {
	return &usageError{
		reason: "unknown_target",
		msg:    fmt.Sprintf("%q names no epic, feature, or task this amendment can block", raw),
	}
}

// resolveStageParentEpic confirms an add-feature op's epic segment names a
// real parent: an epic already live in the tree, or the key of an add-epic
// operation already staged EARLIER on this same amendment, whose id does not
// exist yet and is resolved only when approve materializes it — the same
// same-batch allowance resolveStageReplacement gives a supersede's by and
// resolveStageBlockTarget gives a question's blocks, extended here to the
// one remaining grain.
//
// It mirrors resolveStageBlockTarget's own dispatch: resolve against the live
// tree first, and only on *resolve.ErrNotFound fall through to the staged
// scan; any other resolver error (an ambiguous key, say) propagates
// unrescued, exactly as there.
func resolveStageParentEpic(tree *store.Tree, staged []store.AmendmentOp, epicKey string) error {
	_, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounEpic, Raw: epicKey})
	if err == nil {
		return nil
	}
	var notFound *resolve.ErrNotFound
	if !errors.As(err, &notFound) {
		return err
	}
	for _, op := range staged {
		if op.Op == store.AmendmentOpAddEpic && op.Key == epicKey {
			return nil
		}
	}
	return parentUnknownErr(epicKey, err)
}

// parentUnknownErr refuses an add-feature op whose epic segment resolved
// nowhere: not live in the tree, and not among an add-epic staged earlier on
// this same amendment. cause is the live resolver's *resolve.ErrNotFound,
// carried through usageError's Unwrap so a caller unwinding for that
// concrete type still finds it, even though the message now names both
// misses instead of only the live-tree one.
func parentUnknownErr(epicKey string, cause error) error {
	return &usageError{
		reason: "not_found",
		msg:    fmt.Sprintf("%q names no epic in the tree, and no add-epic staged earlier on this amendment", epicKey),
		cause:  cause,
	}
}

func epicTargetErr() error {
	return &usageError{reason: "epic_target", msg: "an epic reshapes through its features"}
}

func unknownTargetErr(raw string) error {
	return &usageError{
		reason: "unknown_target",
		msg:    fmt.Sprintf("%q names no feature or task this amendment can target", raw),
	}
}

func supersedeByRequiredErr() error {
	return &usageError{reason: "supersede_by_required", msg: "a supersede operation needs --by naming its replacement"}
}

func supersedeByMisplacedErr() error {
	return &usageError{reason: "supersede_by_misplaced", msg: "--by only names a supersede operation's replacement"}
}

func supersededByMisplacedErr() error {
	return &usageError{reason: "superseded_by_misplaced", msg: "--superseded-by only names a cancel operation's replacement"}
}

func statementRequiredErr() error {
	return &usageError{reason: "statement_required", msg: "--add-question needs a statement"}
}

func statementMisplacedErr() error {
	return &usageError{reason: "statement_misplaced", msg: "statement only names an add-question operation's full question"}
}

func blocksMisplacedErr() error {
	return &usageError{reason: "blocks_misplaced", msg: "--blocks only names an add-question operation's held nodes"}
}

// selfSupersessionErr refuses an operation naming its own target as its
// replacement. Supersession is a lineage between two nodes, and a node cannot
// replace itself: applying one would write supersedes and superseded_by pointing
// at the same id, which reads as a recorded human decision to every consumer
// (drift exempts a superseded feature from the stale_spec sweep on that field's
// presence alone) while recording nothing anyone decided.
func selfSupersessionErr(nodeID string) error {
	return &usageError{
		reason: "self_supersession",
		msg:    fmt.Sprintf("%s cannot supersede itself; a supersession names the node that replaces it", nodeID),
	}
}

// AmendsDesignResult is the outcome of a StageAmendsDesign call: the amendment
// it landed on, its amends_design after the union, and whether anything
// actually grew.
type AmendsDesignResult struct {
	AmendmentID  string
	AmendsDesign []string
	Changed      bool
}

// StageAmendsDesign unions paths into an open amendment's amends_design,
// recorded verbatim: a design doc a staged reshape names may not exist yet
// (it may be created by the reshape itself), so nothing here checks the
// filesystem. Dedup and order follow Link's depends_on union (depends.go): a
// no-op re-add reports Changed=false and writes nothing.
func StageAmendsDesign(tree *store.Tree, deps Deps, amendment *store.Amendment, paths []string) (AmendsDesignResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return AmendsDesignResult{}, err
	}
	if len(paths) == 0 {
		return AmendsDesignResult{}, fmt.Errorf("mutate: stage amends-design needs at least one path")
	}

	doc, relYml, snapshot, err := loadAmendmentDoc(tree, amendment)
	if err != nil {
		return AmendsDesignResult{}, err
	}
	if doc.Status != store.AmendmentStatusProposed {
		return AmendsDesignResult{}, amendmentNotOpenErr(doc)
	}

	changed := unionStrings(&doc.AmendsDesign, paths)
	res := AmendsDesignResult{AmendmentID: doc.ID, AmendsDesign: doc.AmendsDesign, Changed: changed}
	if !changed {
		return res, nil
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return AmendsDesignResult{}, err
	}

	ev := events.NewEvent(deps.clock(), eventAmendmentStaged, owner.ID, "amend "+doc.ID+" --amends-design", doc.ID)
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"amends_design": doc.AmendsDesign}
	ev.Files = []events.FileChange{{Path: relYml}}

	if err := commitNodeWrites(tree, "amend.amends_design", []nodeWrite{
		{rel: relYml, data: data, before: snapshot, event: ev},
	}); err != nil {
		return AmendsDesignResult{}, err
	}

	return res, nil
}

// unionStrings appends every value in add not already in *list, preserving
// first-seen order, the same shape setNodeDependsOn's union closure produces
// for depends_on (depends.go). It reports whether anything grew.
func unionStrings(list *[]string, add []string) bool {
	have := make(map[string]bool, len(*list))
	for _, v := range *list {
		have[v] = true
	}
	grew := false
	for _, v := range add {
		if !have[v] {
			*list = append(*list, v)
			have[v] = true
			grew = true
		}
	}
	return grew
}

// WithdrawResult is the outcome of a WithdrawAmendment call.
type WithdrawResult struct {
	ID     string
	Key    string
	Status string
	Reason string
}

// WithdrawAmendment drops a proposed amendment without applying it: status
// moves to withdrawn, terminal like approved, so its key is free for reuse and
// a later StageOperation or StageAmendsDesign on it refuses amendment_not_open.
// The reason discipline mirrors cancel's (transition.go's reasonRequired):
// withdrawing is the amendment's own removal verb, and an unexplained one
// leaves no record of why the proposal was dropped.
func WithdrawAmendment(tree *store.Tree, deps Deps, amendment *store.Amendment, reason string) (WithdrawResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return WithdrawResult{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return WithdrawResult{}, &usageError{reason: "reason_required", msg: "withdrawing an amendment needs a non-empty reason"}
	}

	doc, relYml, snapshot, err := loadAmendmentDoc(tree, amendment)
	if err != nil {
		return WithdrawResult{}, err
	}
	if doc.Status != store.AmendmentStatusProposed {
		return WithdrawResult{}, amendmentNotOpenErr(doc)
	}

	doc.Status = store.AmendmentStatusWithdrawn
	data, err := yaml.Marshal(doc)
	if err != nil {
		return WithdrawResult{}, err
	}

	ev := events.NewEvent(deps.clock(), eventAmendmentWithdrawn, owner.ID, "withdraw amendment "+doc.ID, doc.ID)
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"status": store.AmendmentStatusWithdrawn, "reason": reason}
	ev.Files = []events.FileChange{{Path: relYml}}

	if err := commitNodeWrites(tree, "withdraw.amendment", []nodeWrite{
		{rel: relYml, data: data, before: snapshot, event: ev},
	}); err != nil {
		return WithdrawResult{}, err
	}

	return WithdrawResult{ID: doc.ID, Key: deref(doc.Key), Status: doc.Status, Reason: reason}, nil
}
