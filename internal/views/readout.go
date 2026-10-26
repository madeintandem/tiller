package views

// The readout is the one consistent block Tiller hands the developer at a
// seam boundary or a human gate (docs/spec/cli-surface.md):
// what happened, what the agent checked, what needs the human, what comes
// next. It exists so the developer's experience does not depend on which AI
// harness drove the work: every harness renders the same Readout, derived
// here, never assembled ad hoc in a prompt. Task 1 defined the shape and the
// one section (NeedsYou) a whole tree read can answer on its own. Task 3 (this
// file's bulk) adds the fact selection for all four seams the pass-1 CLI
// drives (internal/skills: orient, decompose, specify, execute), each keyed to
// the per-moment table in docs/spec/cli-surface.md.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/readiness"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
)

// Moment names the seam or gate a readout speaks for. The renderer keys its
// fact selection off this, because what a decompose needs to prove is not what
// an execute needs to prove (see the per-moment table in
// docs/spec/cli-surface.md).
type Moment string

const (
	MomentOrient         Moment = "orient"
	MomentDeriveBaseline Moment = "derive-baseline"
	MomentDecompose      Moment = "decompose"
	MomentSpecify        Moment = "specify"
	MomentExecute        Moment = "execute"
	MomentAmend          Moment = "amend"
	MomentApprove        Moment = "approve"
	MomentAnswer         Moment = "answer"
	MomentCancelQuestion Moment = "cancel-question"
	MomentAttest         Moment = "attest"
	MomentSync           Moment = "sync"
)

// ReadoutEntity is one named thing the moment produced: an epic, a feature, a
// task. Ref carries the shared "<key-path> (<id>)" convention; Note is the
// short trailing clause ("7 features", "depends on platform-foundation").
type ReadoutEntity struct {
	Ref  string `json:"ref"`
	Note string `json:"note,omitempty"`

	// Unpushed carries caller-supplied publication facts for this entity's
	// recorded branch. executeContent in readout_execute.go pairs those facts
	// with the task's feature branch or the shelf item's own branch. The CLI
	// renders the same warning as submit confirmation and status.
	Unpushed *UnpushedReviewView `json:"unpushed,omitempty"`
}

// ReadoutCheck is one verification line: a check and its result, never a bare
// tally. "Dependencies named in prose but not recorded: 0" is the shape.
type ReadoutCheck struct {
	Label  string `json:"label"`
	Result string `json:"result"`
}

// ReadoutAction is a pending or next move. NeedsYou actions carry count text
// and a read-only queue command; Next actions may additionally carry Owner as
// "human" or "agent", matching NextOwner's vocabulary.
type ReadoutAction struct {
	Text    string `json:"text"`
	Owner   string `json:"owner,omitempty"`
	Command string `json:"command,omitempty"`
}

// Readout is the whole block, before rendering. It is pure derivation: every
// field comes from the loaded tree or from an existing projection.
//
// Task 1 also carried an EntityCap string field here, meant to say what the
// entity list's cap was. No content function (one per seam, covering every
// seam the pass-1 CLI drives) ever set it, because the cap is a rendering
// fact, not a derived one: entityCap below is a package constant every
// content function already closes over directly, and the list plus Withheld
// together already say everything a reader needs ("3 named, 9 more"). A
// second field carrying the same "3" as a string would be redundant with the
// constant and read as a derived fact when it never varied. Deleted rather
// than populated (Task 4 brief's instruction to decide one way or the other).
type Readout struct {
	Moment   Moment          `json:"moment"`
	Headline string          `json:"headline"`
	Artifact string          `json:"artifact,omitempty"`
	Entities []ReadoutEntity `json:"entities,omitempty"`
	Withheld int             `json:"withheld,omitempty"`
	Queue    string          `json:"queue"`
	Checks   []ReadoutCheck  `json:"checks"`
	NeedsYou []ReadoutAction `json:"needs_you"`
	// Next is the ranked list of the reader's legal moves, uncut: the block
	// renders its shortlist and counts the rest (ranked-next design,
	// decision 6).
	Next     Next             `json:"next"`
	Baseline *BaselineReadout `json:"baseline,omitempty"`
	// GrainNotes names each in-flight task that started at task grain under a
	// feature now resolving to a coarse grain (issue #146): it takes its own
	// PR, which must merge before the coarse submit.
	GrainNotes []string `json:"grain_notes,omitempty"`
}

// entityCap bounds the named list. The console pass established what happens
// when a surface renders every row; the readout must not reintroduce it in the
// terminal, so the list names a few and says how many it withheld
// (task-3-brief.md, Step 3).
const entityCap = 3

// ReadoutInput carries the facts ReadoutOf cannot derive from the tree alone,
// because views performs no I/O (this task's global constraint). The caller
// reads git and the eval files and passes the answers in.
type ReadoutInput struct {
	Commit string
	Dirty  bool
	Ready  bool

	// DesignDocPresent reports regular prose files observed by the caller.
	// A nil map means existence was not checked, never that all docs exist.
	DesignDocPresent map[string]bool

	// QueueInput carries the live I/O-derived suppression facts used by the
	// action queue. Feeding the same input to QueueOfInput keeps the readout
	// from advertising an entry the console API or `tiller queue` suppresses.
	QueueInput QueueInput

	// Contributor is the acting contributor on this checkout, resolved the way every
	// mutation resolves it (--contributor, then TILLER_CONTRIBUTOR, then the gitignored
	// .tiller/identity.yml). It is the reader the Next list is ranked for;
	// empty ranks unpersonalized. It arrives as input because reading the
	// identity file is I/O.
	Contributor string

	// Recent is the reader's recent events and Now the clock reading they are
	// measured against, both for ranking Next (views reads no clock).
	Recent []events.Event
	Now    time.Time

	// Contracts holds each feature's parsed feature.eval.md, keyed by feature
	// id, for specify's "contracts valid" and "criteria bound by type"
	// checks. Reading and parsing an eval file is I/O (store.LoadEvalFile);
	// ValidateContract and the per-criterion binding tally that
	// specifyContent runs are pure over the already-parsed struct, so the
	// caller loads it and views only ever inspects it.
	//
	// A nil map means the caller never ran this check this pass, and
	// specifyContent renders "not evaluated" rather than a false "0 of N": an
	// unwired caller must not have its silence read back as a real answer
	// (task-3-report.md finding 1, the checks-not-tallies bar reintroduced one
	// layer down). A non-nil map, even an empty one, is the caller's real
	// answer and renders its real count, including a real zero.
	Contracts map[string]*store.EvalFile
	Workflow  workflow.Input

	// EpicSpecs holds, per epic id, whether decompose's per-epic spec check
	// passed: epic.md rewritten past the `tiller add epic` stub
	// (mutate.renderEpicMD). That read is I/O, so the caller supplies the
	// answer; the same nil-means-not-run, non-nil-means-real discipline as
	// Contracts applies.
	EpicSpecs map[string]EpicSpecStatus

	// AmendmentPreconditions holds, per proposed amendment id, its staged
	// preconditions' LIVE validity against the current tree
	// (mutate.EvaluateAmendmentPreconditions, the identical comparison
	// approve's own checkPreconditions runs). Comparing spec_revision and
	// eval_revision hashes working-tree files through git, which is I/O this
	// package performs none of, so the caller reads it and amendContent's
	// Preconditions check only ever inspects the answer. The same
	// nil-means-not-run, non-nil-means-real discipline as Contracts and
	// EpicSpecs applies: nil renders "not evaluated", a real (even empty) map
	// renders its real tally.
	AmendmentPreconditions map[string][]mutate.PreconditionValidity

	// UnpushedReviews holds the review-state warning's caller-computed facts
	// (issue #42), one entry per in-review task whose recorded feature branch
	// carries a commit its PR does not yet show: the I/O (a live git compare
	// against the branch's remote-tracking counterpart, vcs.Git.UnpushedTip)
	// this package performs none of. Node is the task id executeContent
	// matches entries against; Commit and Upstream are what the caller's git
	// read found. Nil, the zero value, when the caller never ran the check
	// (every seam and gate but execute) or found nothing unpushed, which
	// executeContent's entity loop reads as "nothing to warn about" the same
	// way a non-matching id would, so there is no separate not-evaluated
	// state to render here the way Contracts and EpicSpecs carry: an entity
	// with no warning looks identical whether the check ran clean or never
	// ran, and either way there is nothing for a reader to act on.
	UnpushedReviews []UnpushedReview

	// Act carries what a human gate's own mutation just did: which node(s)
	// it touched, whether each was already at the target, and the
	// type-specific detail (attest's target, sync's crossings, answer's
	// blocked set) that only the command itself knows and a post-mutation
	// tree read cannot recover, since the mutation result is gone once the
	// command returns and nothing records "which one of several possible
	// facts this call was about" on disk.
	//
	// Round-1 review found the gate content functions read the tree
	// exactly the way the seams do (a post-mutation snapshot, no
	// memory of the call that just ran), so Verification answered "what is
	// still true of the tree" instead of "what this act did or unblocked,"
	// and for approve specifically that meant a queue tally sitting
	// directly above Your Gate's own identical list. Act is the fix: nil
	// for every seam (ReadoutInput is never asked to carry it there) and
	// for a gate whose readout failed to derive before Act was wired up (a
	// moment none of the gates now reach); populated by
	// readout_gate.go at each gate call site, from data already in
	// scope there (the resolved node, the mutation result).
	Act *GateAct
}

// EpicSpecStatus is one epic's answer to decompose's per-epic spec check,
// computed by the caller (reading epic.md's text) and handed to
// decomposeContent as data. It carried a second field, whether epic.eval.md
// parsed as a well-formed contract, until epic.eval.md was removed: an epic has
// no acceptance contract of its own, so there is no bar at this grain to report
// on (artifact-schema.md, Epics). It stays a struct rather than collapsing to a
// bare bool so the next per-epic answer decompose wants is an added field
// rather than a signature change at every call site.
type EpicSpecStatus struct {
	ProseWritten bool // epic.md is no longer the tiller add epic stub
}

// UnpushedReview is a caller-computed review-state warning. Node identifies
// the execution entity; executeContent gets its branch from the tree rather
// than asking the caller to duplicate a stored fact.
type UnpushedReview struct {
	Node     string
	Commit   string
	Upstream string
}

// GateAct is what a human gate's mutation just did, in the shape each of the
// gate content functions needs. Only the field(s) that moment's own
// call site populates are non-nil/non-empty; a content function reads only
// its own and ignores the rest, so one shared struct threads through
// emitReportWithReadout instead of a bespoke type per gate.
type GateAct struct {
	// DesignApproval records path identities, not work-node transitions.
	DesignApproval *DesignApprovalAct

	// Nodes names what a lifecycle transition (approve) acted on,
	// one entry per node in argument order: a single node the common case,
	// several on a batch approve. Changed is false
	// for an idempotent no-op (already at the target), the exact flag
	// mutate.TransitionResult already carries; a fresh tree read alone
	// cannot tell "just moved here" from "was already here."
	Nodes []GateActNode

	// BasisRefresh is set when the approve that ran was `--refresh-basis`, the
	// arm that re-records a node's accepted_against and returns before the
	// legality check (mutate's transitionFeature and transitionTask). The CLI
	// sets it from the flag it was invoked with (lifecycle_cmds.go's
	// transitionAct); nothing on disk after the write says which arm produced
	// it, since the arm's whole point is that the node looks the same
	// afterwards.
	//
	// approveContent below reads it, because the plain-approve account is
	// wrong for this arm in both of its parts: the headline said "Approval
	// recorded" and the check said "Moved to ready", for a command whose
	// contract is that nothing moved (task-9-brief.md, inherited item 8).
	BasisRefresh bool

	// AssumptionReviewed is set when the approve that ran was
	// `--reviewed-assumption <Q-id>`, the third act under the approve verb
	// beside a plain transition and --refresh-basis. Like BasisRefresh, it
	// changes no lifecycle state (mutate.ReviewAssumption's own doc
	// comment), so the plain-transition account below ("Moved to ready") is
	// exactly as wrong for it as it was for BasisRefresh
	// (task-9-brief.md, inherited item 8; fix round 1 on task 5 found the
	// same gap here): approveContent takes its own early-return branch for
	// it instead of falling into the Nodes tally.
	AssumptionReviewed *GateActAssumptionReviewed

	// AmendmentApplied is `approve amendment`'s own target:
	// mutate.ApproveResult's own Applied field, carried in by the CLI's call
	// site (cli/amendment_cmds.go's runApproveAmendment). An amendment's
	// approve is not a single lifecycle move the way every other approve is
	// (GateActNode's own shape assumes one status move per entry): it is a
	// batch of up to five different operation kinds landing under one lock,
	// so approveContent takes its own early-return branch for it too, the
	// same reason AssumptionReviewed and BasisRefresh each get their own.
	AmendmentApplied *GateActAmendmentApplied

	// Attested is attest's own target: which gate or criterion the call
	// signed off, and whether that left it satisfied. A fresh tree read
	// can see every gate or criterion still unmet on a node, but not which
	// one THIS call touched, so attestContent needs this to exclude the
	// one just handled from "what remains unmet."
	Attested *GateActAttest

	// Answered is answer question's own target: the node ids its Blocks
	// named, so answerContent can check, on the fresh tree, whether each is
	// now clear of every OTHER open question (the design table's "what it
	// unblocked"). A fresh tree read cannot know which question this call
	// closed.
	Answered *GateActAnswer

	// Cancelled is cancel question's own target: the node ids its Blocks
	// named, so cancelContent can report, as answerContent does, which of
	// them the cancel left clear of every open question.
	Cancelled *GateActCancel

	// Crossed and Awaiting are sync's own detail, carried straight from
	// mutate.SyncResult (Crossings, Awaiting): the basis each crossing
	// rested on, and what stayed in review and why (a missing branch, a
	// fast-forward strand). mutate.Sync already computes both by walking
	// git; a tree read has no way to recover either without walking git a
	// second time, and syncContent must not do that (views performs no
	// I/O).
	Crossed  []GateActCrossing
	Awaiting []GateActAwaiting
}

// GateActNode is one node a lifecycle-verb gate (approve) acted on.
type GateActNode struct {
	Noun    string
	ID      string
	Changed bool
}

// GateActAmendmentApplied is `approve amendment`'s own detail: every
// operation the batch actually applied, in staged order. It carries no
// per-operation identity beyond the op kind: the command's own confirmation
// (humanApproveAmendment, cli/amendment_cmds.go) already names every applied
// operation by ref, and approveContent's job is a tally beside that, never a
// second copy of the same line.
type GateActAmendmentApplied struct {
	Applied []GateActAmendmentOp
}

// GateActAmendmentOp is one operation an amendment approve applied: its op
// kind, the one field amendmentOpBreakdown tallies by.
type GateActAmendmentOp struct {
	Op string
}

// GateActAttest is attest's own target: mutate.AttestResult's TargetKind
// ("gate" or "criterion") and TargetID, plus the node it was recorded on.
// mutate.AttestResult also carries Noun and whether the sign-off left the
// target Satisfied; attestContent (below) only ever reads NodeID,
// TargetKind, and TargetID to exclude this call's own target from "what
// remains unmet," so Noun and Satisfied are gone rather than kept as an
// unread translation (whole-branch review, finding 3).
type GateActAttest struct {
	NodeID     string
	TargetKind string
	TargetID   string
}

// GateActAnswer is answer question's own target: the node ids it named on
// Blocks. mutate.QuestionResult also carries the question's own id, the same
// id the call site (question.go) already has in hand; answerContent (below)
// never reads it back off this struct, so it is gone (whole-branch review,
// finding 3).
type GateActAnswer struct {
	Blocks []string
}

// GateActCancel is cancel question's own target: the node ids it named on
// Blocks. A cancel releases holds exactly as an answer does.
type GateActCancel struct {
	Blocks []string
}

// GateActAssumptionReviewed is `approve --reviewed-assumption`'s own target:
// the feature the entry lives on and the question whose answer was stamped
// in, plus whether the stamp was a real move (mutate.ReviewAssumptionResult's
// own Changed) or a no-op retry of one already matching.
type GateActAssumptionReviewed struct {
	NodeID     string
	QuestionID string
	Changed    bool
}

// GateActCrossing is one crossing sync recorded this run: the Basis
// (ancestry, content, rollup, human_attested) syncContent's byBasis tally
// reads. mutate.Crossing also carries the node's own Noun and ID, but
// syncContent (below) only ever tallies Basis, so the identity fields are
// gone rather than translated and never read (whole-branch review, finding
// 3).
type GateActCrossing struct {
	Basis                      string
	NodeID                     string
	HistoricalPolicyViolations []PolicyViolationView
}

// GateActAwaiting is one node sync's own walk found still sitting in
// review: why nothing crossed (BranchMissing, NoMergeWitness), the same
// distinction humanSyncAwaiting (sync.go) already renders in the
// confirmation. mutate.Awaiting also carries the node's Noun, ID, and the
// branch it observed; syncContent (below) only ever tallies the two
// booleans, so the rest is gone rather than translated and never read
// (whole-branch review, finding 3).
type GateActAwaiting struct {
	BranchMissing  bool
	NoMergeWitness bool
	ReviewMissing  bool
}

// ReadoutOf derives the readout for a moment with no I/O-derived facts, the
// zero-input convenience wrapper Task 1's test still exercises. A real caller
// (Task 5's `tiller readout` and gate commands) uses ReadoutOfInput so the
// commit, dirty, and readiness facts it read from git and `check readiness`
// land in the block.
func ReadoutOf(t *store.Tree, m Moment) Readout {
	return ReadoutOfInput(t, m, ReadoutInput{})
}

// ReadoutOfInput derives the readout for a moment, given the I/O-derived
// facts the caller already gathered. Work attention comes from QueueOf;
// baseline review comes from its ledger, without inventing a queue kind.
// The headline's artifact is the commit only when the tree is both
// named and clean; an uncommitted tree leaves the human nothing to review
// (docs/spec/cli-surface.md), so Artifact
// stays empty rather than naming a commit the working tree has since moved
// past.
func ReadoutOfInput(t *store.Tree, m Moment, in ReadoutInput) Readout {
	r := Readout{Moment: m, Headline: headlineFor(m), Artifact: artifactOf(in.Commit, in.Dirty), Queue: ReadyShelfQueueLine(t), Checks: []ReadoutCheck{}, NeedsYou: []ReadoutAction{}}
	order := gen.DependencyOrder(t)
	if in.Workflow.Contracts == nil {
		in.Workflow.Contracts = in.Contracts
	}
	switch m {
	case MomentOrient:
		orientContent(t, in, order, &r)
	case MomentDecompose:
		decomposeContent(t, in, &r)
	case MomentSpecify:
		specifyContent(t, in, &r)
	case MomentExecute:
		executeContent(t, in, order, &r)
	case MomentAmend:
		amendContent(t, in, &r)
	case MomentApprove:
		approveContent(t, in, &r)
	case MomentAnswer:
		answerContent(t, in, &r)
	case MomentCancelQuestion:
		cancelContent(t, in, &r)
	case MomentAttest:
		attestContent(t, in, &r)
	case MomentSync:
		syncContent(t, in, &r)
	}
	r.Baseline = baselineReadoutFor(t, m, in.DesignDocPresent, in.Act != nil && in.Act.DesignApproval != nil)
	in.QueueInput.Workflow = in.Workflow
	r.NeedsYou = append(r.NeedsYou, needsYouFromQueue(QueueOfInput(t, in.QueueInput))...)
	r.Next = NextOf(t, NextInput{Reader: in.Contributor, Workflow: in.Workflow, Queue: in.QueueInput, Recent: in.Recent, Now: in.Now})
	r.GrainNotes = grainNotes(t)
	return r
}

// grainNotes is guidance only, never a refusal: for each feature resolving to
// a coarse grain, every non-done task that started at task grain rides its own
// PR (or, remoteless, its review ref), which must merge before the coarse
// submit (issue #146). Tasks that never
// recorded a started grain resolve through their parents and get no note.
func grainNotes(t *store.Tree) []string {
	ids := make([]string, 0, len(t.Features))
	for id := range t.Features {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var notes []string
	for _, id := range ids {
		f := t.Features[id]
		epic := recapEpicByKey(t, f.EpicKey)
		var reviewed store.Node
		switch store.ResolvedReviewGrain(t.Manifest, epic, f) {
		case store.ReviewGrainFeature:
			reviewed = f
		case store.ReviewGrainEpic:
			if epic != nil {
				reviewed = epic
			}
		}
		if reviewed == nil {
			continue
		}
		for _, task := range gen.OrderTasks(t, t.FeatureTasks[f.ID], f.Order) {
			if task.StartedGrain != store.ReviewGrainTask || task.Status == "done" || task.Status == "cancelled" {
				continue
			}
			review := "PR"
			if task.PR == nil {
				review = "PR or review ref"
			}
			notes = append(notes, fmt.Sprintf("%s rides task grain; its %s must merge before submit %s %s.", gen.Ref(t, task), review, reviewed.Noun(), gen.Ref(t, reviewed)))
		}
	}
	return notes
}

// artifactOf is the headline's committed-at rule, stated once for the full
// block and the correction block (correction.go): the commit is named only
// when the tree is both named and clean, because an uncommitted tree leaves
// the human nothing to review at that sha.
func artifactOf(commit string, dirty bool) string {
	if commit != "" && !dirty {
		return commit
	}
	return ""
}

// attentionCategory is one count bucket in the Needs attention list. Kind and
// Noun together split attest_criterion at the grain where the human reviews
// it; every other queue kind uses an empty noun to count the complete kind.
type attentionCategory struct {
	kind    string
	noun    string
	command string
}

// attentionCategories fixes the count-list order to QueueOfInput's group
// order. attest_criterion becomes adjacent feature and task buckets without
// disturbing the queue entries themselves.
var attentionCategories = [...]attentionCategory{
	{kind: "approve_amendment", command: "tiller queue --kind approve_amendment"},
	{kind: "approve_epic", command: "tiller queue --kind approve_epic"},
	{kind: "approve_feature", command: "tiller queue --kind approve_feature"},
	{kind: "approve_tasks", command: "tiller queue --kind approve_tasks"},
	{kind: "approve_bug", command: "tiller queue --kind approve_bug"},
	{kind: "approve_chore", command: "tiller queue --kind approve_chore"},
	{kind: "approve_tweak", command: "tiller queue --kind approve_tweak"},
	{kind: "answer_question", command: "tiller queue --kind answer_question"},
	{kind: "carrier_cancelled", command: "tiller queue --kind carrier_cancelled"},
	{kind: "attest_gate", command: "tiller queue --kind attest_gate"},
	{kind: "attest_criterion", noun: store.NounFeature, command: "tiller queue --kind attest_criterion --noun feature"},
	{kind: "attest_criterion", noun: store.NounTask, command: "tiller queue --kind attest_criterion --noun task"},
	{kind: "feature_check", command: "tiller queue --kind feature_check"},
	{kind: "workflow_closeout", command: "tiller queue --kind workflow_closeout"},
	{kind: "merge_epic", command: "tiller queue --kind merge_epic"},
	{kind: "merge_feature", command: "tiller queue --kind merge_feature"},
}

// needsYouFromQueue reduces the already-derived queue to exact counts. It does
// not retain entry identities or mutation commands: every action drills into
// the corresponding current queue slice through a read-only command.
func needsYouFromQueue(q Queue) []ReadoutAction {
	var counts [len(attentionCategories)]int
	for _, entry := range q.Entries {
		for i, category := range attentionCategories {
			if entry.Kind == category.kind && (category.noun == "" || entry.Noun == category.noun) {
				counts[i]++
				break
			}
		}
	}

	out := make([]ReadoutAction, 0, len(attentionCategories))
	for i, category := range attentionCategories {
		if counts[i] == 0 {
			continue
		}
		out = append(out, ReadoutAction{
			Text:    attentionSummary(category, counts[i]),
			Command: category.command,
		})
	}
	return out
}

// attentionSummary owns the category-specific count sentences. The noun and
// verb both inflect at exactly one; the wording otherwise stays independent
// of queue-entry content so no node, question, gate, or criterion identity can
// leak into the readout.
func attentionSummary(category attentionCategory, count int) string {
	noun := func(singular, plural string) string {
		if count == 1 {
			return singular
		}
		return plural
	}
	await := "await"
	if count == 1 {
		await = "awaits"
	}

	switch category.kind {
	case "approve_amendment":
		return fmt.Sprintf("%d %s %s your approval", count, noun("proposed amendment", "proposed amendments"), await)
	case "approve_epic":
		return fmt.Sprintf("%d %s %s your approval", count, noun("epic", "epics"), await)
	case "approve_feature":
		return fmt.Sprintf("%d %s %s your approval", count, noun("feature plan", "feature plans"), await)
	case "approve_tasks":
		have := "have"
		if count == 1 {
			have = "has"
		}
		return fmt.Sprintf("%d %s %s added tasks awaiting your approval", count, noun("feature", "features"), have)
	case "approve_bug":
		return fmt.Sprintf("%d %s %s triage", count, noun("bug", "bugs"), await)
	case "approve_chore":
		return fmt.Sprintf("%d %s %s triage", count, noun("chore", "chores"), await)
	case "approve_tweak":
		return fmt.Sprintf("%d %s %s triage", count, noun("tweak", "tweaks"), await)
	case "answer_question":
		return fmt.Sprintf("%d %s %s your answer", count, noun("question", "questions"), await)
	case "carrier_cancelled":
		have := "have"
		if count == 1 {
			have = "has"
		}
		return fmt.Sprintf("%d %s %s a cancelled carrier", count, noun("shelf item", "shelf items"), have)
	case "attest_gate":
		return fmt.Sprintf("%d %s %s your attestation", count, noun("review gate", "review gates"), await)
	case "attest_criterion":
		if category.noun == store.NounFeature {
			return fmt.Sprintf("%d %s %s your attestation", count, noun("feature criterion", "feature criteria"), await)
		}
		return fmt.Sprintf("%d %s %s your attestation", count, noun("task criterion", "task criteria"), await)
	case "feature_check":
		return fmt.Sprintf("%d %s %s a check run", count, noun("feature", "features"), await)
	case "workflow_closeout":
		return fmt.Sprintf("%d %s %s shared closeout", count, noun("review unit", "review units"), await)
	case "merge_epic":
		return fmt.Sprintf("%d %s %s your review and merge", count, noun("epic review", "epic reviews"), await)
	case "merge_feature":
		return fmt.Sprintf("%d %s %s your review and merge", count, noun("feature review", "feature reviews"), await)
	default:
		return ""
	}
}

// plural selects the word form for a count. Pool guidance (its noun and its
// verb) and amendment entity notes share it.
func plural(n int, singular, many string) string {
	if n == 1 {
		return singular
	}
	return many
}

// headlineFor names what a moment's readout leads with. Every moment the
// pass-1 CLI drives (internal/skills.Names) and every human gate
// (approve, answer question, cancel question, attest, sync) has prose here; a moment
// outside both sets falls back to its own name rather than an empty string.
func headlineFor(m Moment) string {
	switch m {
	case MomentOrient:
		return "Project surveyed"
	case MomentDeriveBaseline:
		return "Design baseline surveyed"
	case MomentDecompose:
		return "Decomposition laid down"
	case MomentSpecify:
		return "Specs written"
	case MomentExecute:
		return "Execution status"
	case MomentAmend:
		return "Amendments surveyed"
	case MomentApprove:
		return "Approval recorded"
	case MomentAnswer:
		return "Question answered"
	case MomentCancelQuestion:
		return "Question cancelled"
	case MomentAttest:
		return "Attestation recorded"
	case MomentSync:
		return "Crossings recorded"
	default:
		return string(m)
	}
}

// approveContent, answerContent, attestContent, and
// syncContent fill in the human-gate moments' checks.
// The design doc's per-moment table (docs/spec/cli-surface.md) gives each a
// "produced" and a "verification" column, but Task 3 only ever wrote content
// functions for the seams; the switch above fell through to the zero-value
// Readout for every gate until this task, which is the empty-block gap
// task-6-brief.md's own author could not see (gate content was never assigned
// to a task) and this task's own brief flags directly: these moments are what a
// developer sees on every approve, answer, attest, and sync they run, more
// often than any seam.
//
// None of them populates Entities. The design's settled decision is that
// "existing confirmation lines are unchanged and the readout follows them,"
// and for a gate specifically those lines already are the per-node account
// of what transitioned, exactly the per-moment table's "produced" column
// (task-6-brief.md's own global constraint: "on a batch approve those lines
// are the per-node record of what transitioned and what was already in the
// target state; a summary cannot replace them"). Repeating that in Entities
// would be the block narrating what the line directly above it already
// said.
//
// Round 1 shipped every one of those Verification sections as a queue
// tally (a count of what QueueOf still lists as pending), which read as
// noise: `approve epic alpha` printed "Epics awaiting approval: 1" directly
// above a Your Gate section naming that same epic, the identical fact
// twice. The root cause was architectural, not a wording problem: a tally
// answers "what is still true of the tree," which is exactly what Your Gate
// already answers below it, never "what this act did or unblocked," the
// question the design table's verification column actually asks. Every
// content function below reads in.Act instead (the mutation's own result,
// carried in from the gate call site that ran it, ReadoutInput's own doc
// comment), and where a fact is genuinely unrecoverable from Act plus a
// fresh tree read (a true pre-mutation diff no gate command captures), it
// says so in its own comment rather than substituting a tally that only
// looks like an answer.
func approveContent(t *store.Tree, in ReadoutInput, r *Readout) {
	act := in.Act
	if act == nil {
		return
	}
	if approval := act.DesignApproval; approval != nil {
		r.Headline = "Design confirmation recorded"
		r.Checks = append(r.Checks,
			ReadoutCheck{Label: "Design docs confirmed", Result: strconv.Itoa(len(approval.Confirmed))},
			ReadoutCheck{Label: "Confirmations unchanged", Result: strconv.Itoa(len(approval.Unchanged))})
		return
	}
	// --reviewed-assumption is a third act under the same verb: it stamps
	// one assumption_basis entry and moves no lifecycle state, so it gets
	// its own headline and checks rather than the Nodes tally below, which
	// would print "Moved to ready" for a node that did not move (the same
	// failure BasisRefresh's own branch exists to avoid). The remaining
	// count is what actually answers "is my submit unblocked now": it reads
	// mutate.UnreviewedAssumptions fresh off t, the identical predicate
	// Task 6's submit guard calls, rather than trust the stamp alone, since
	// a feature can carry more than one open assumption and this call only
	// ever reviews one in.
	if ar := act.AssumptionReviewed; ar != nil {
		r.Headline = "Assumption reviewed in"
		label := "Stamp recorded"
		if !ar.Changed {
			label = "Stamp already matched (no change)"
		}
		r.Checks = append(r.Checks, ReadoutCheck{Label: label, Result: ar.QuestionID})
		if feat := t.Features[ar.NodeID]; feat != nil {
			remaining := len(mutate.UnreviewedAssumptions(t, feat))
			r.Checks = append(r.Checks,
				ReadoutCheck{Label: "Unreviewed assumptions remaining on this node", Result: strconv.Itoa(remaining)})
		}
		return
	}
	// --refresh-basis is a different act under the same verb: it re-records
	// accepted_against and returns before the legality check, so the node it
	// names is wherever it already was. The headline and the checks below are
	// both about a move, so this arm writes its own (task-9-brief.md,
	// inherited item 8). Nothing was approved, so there is no "what did this
	// unblock" to answer either: legalNext on an unmoved node lists the same
	// moves it listed a second ago.
	//
	// It sits AHEAD of the zero-node return, and emits its count even at zero,
	// because `approve task --feature <key> --refresh-basis` on a feature with
	// nothing proposed reaches here with an empty Nodes: falling through would
	// leave the headline reading "Approval recorded" for a command that
	// approved nothing (task-9-findings-round-1.md, Important 2). The count is
	// what keeps the headline from over-claiming on that path: it names what
	// the act reached, including zero.
	if act.BasisRefresh {
		r.Headline = "Accepted basis re-recorded"
		r.Checks = append(r.Checks,
			ReadoutCheck{Label: "Nodes re-pinned", Result: strconv.Itoa(len(act.Nodes))})
		return
	}
	// `approve amendment` is a fourth act under the same verb: a whole batch
	// of up to seven different operation kinds applied under one lock, never
	// a single status move, so the plain Nodes tally below (which assumes
	// exactly that) does not fit it. The headline stays the plain "Approval
	// recorded" (unlike BasisRefresh and AssumptionReviewed, it is not
	// wrong for this act, only generic), and the checks add the one thing
	// those two headlines already give their own verb: what the batch
	// actually did, tallied by op kind. humanApproveAmendment
	// (cli/amendment_cmds.go) already names every applied operation by ref
	// directly above this block, so this stays a tally rather than a second
	// copy of that line.
	if aa := act.AmendmentApplied; aa != nil {
		byOp := map[string]int{}
		for _, op := range aa.Applied {
			byOp[op.Op]++
		}
		r.Checks = append(r.Checks,
			ReadoutCheck{Label: "Operations applied", Result: strconv.Itoa(len(aa.Applied))},
			ReadoutCheck{Label: "By kind", Result: amendmentOpBreakdown(byOp)})
		return
	}
	if len(act.Nodes) == 0 {
		// runApproveTasksByFeature's zero-proposed-tasks branch is the one
		// real case: nothing transitioned, so there is nothing to verify.
		// renderReadout already omits the Verification header when Checks
		// is empty, which is the honest rendering of "nothing happened."
		return
	}
	moved, already := 0, 0
	unblocked := map[string]bool{}
	for _, n := range act.Nodes {
		if n.Changed {
			moved++
			// lookupNode reads the FRESH reloaded tree (renderReadoutFor's
			// own store.Load), not the stale in-memory node the CLI
			// resolved before calling mutate.Transition: transitionEpic/
			// transitionFeature/transitionTask each re-read the node from
			// disk and write that copy back rather than mutating the
			// tree's own pointer (mutate/transition.go), so only a fresh
			// reload sees the post-approval status lifecycle.NextMoves
			// needs to answer "what did this specific approval unblock."
			if fresh := lookupNode(t, n.Noun, n.ID); fresh != nil {
				for _, v := range legalNext(fresh) {
					unblocked[v] = true
				}
			}
			continue
		}
		already++
	}
	r.Checks = append(r.Checks,
		ReadoutCheck{Label: "Moved to ready", Result: fmt.Sprintf("%d of %d", moved, len(act.Nodes))})
	if already > 0 {
		r.Checks = append(r.Checks,
			ReadoutCheck{Label: "Already ready (no change)", Result: strconv.Itoa(already)})
	}
	r.Checks = append(r.Checks,
		ReadoutCheck{Label: "Now legal on the node(s) just approved", Result: verbListOrNone(unblocked)})
}

// answerContent answers the table's "what it unblocked, how many remain
// open": act.Answered.Blocks is the exact set of node ids THIS question
// held (mutate.QuestionResult's own Blocks field, carried in by
// readout_gate.go), so checking each against every OTHER still-open
// question on the fresh tree is a real answer to "what it unblocked," not a
// tally of what is left (that remains a fair thing to report too, and the
// design table asks for both).
func answerContent(t *store.Tree, in ReadoutInput, r *Readout) {
	act := in.Act
	if act == nil || act.Answered == nil {
		return
	}
	questionReleaseChecks(t, "answer", act.Answered.Blocks, r)
}

// cancelContent is answerContent's twin for cancel question: a cancel
// releases every hold the question had, so the same two checks apply.
func cancelContent(t *store.Tree, in ReadoutInput, r *Readout) {
	act := in.Act
	if act == nil || act.Cancelled == nil {
		return
	}
	questionReleaseChecks(t, "cancel", act.Cancelled.Blocks, r)
}

// questionReleaseChecks reports, for a question an act just closed, how many
// of the nodes it blocked are now clear of every open question (only when it
// blocked any) and how many questions remain open.
func questionReleaseChecks(t *store.Tree, act string, blocks []string, r *Readout) {
	cleared := 0
	for _, id := range blocks {
		if !blockedByAnOpenQuestion(t, id) {
			cleared++
		}
	}
	remaining := 0
	for _, q := range t.Questions {
		if gen.QuestionOpen(q) {
			remaining++
		}
	}
	if len(blocks) > 0 {
		r.Checks = append(r.Checks,
			ReadoutCheck{Label: "Nodes this " + act + " cleared of every open question",
				Result: fmt.Sprintf("%d of %d", cleared, len(blocks))})
	}
	r.Checks = append(r.Checks,
		ReadoutCheck{Label: "Open questions remaining", Result: strconv.Itoa(remaining)})
}

// attestContent answers "what remains unmet on the node," at the node
// grain the design table actually asks for (round 1 fell back to tree
// grain because ReadoutInput carried no node identity at all; the
// reviewer's finding was that the constraint was emitReportWithReadout's
// signature, a plumbing choice, not a real limit, so this task added
// act.Attested to carry it). queueAttestGates and queueAttestCriteria are
// the same predicates QueueOf already runs for NeedsYou (never a second
// computation), filtered down to this one node and this one target
// excluded, since a fresh read normally would not still list what this
// call just satisfied anyway; the exclusion documents the intent rather
// than changing the count in the common case.
func attestContent(t *store.Tree, in ReadoutInput, r *Readout) {
	act := in.Act
	if act == nil || act.Attested == nil {
		return
	}
	at := act.Attested
	order := gen.DependencyOrder(t)
	gates := 0
	for _, e := range queueAttestGates(t, order) {
		if e.ID == at.NodeID && !(at.TargetKind == "gate" && e.Gate == at.TargetID) {
			gates++
		}
	}
	criteria := 0
	for _, e := range queueAttestCriteria(t, order) {
		if e.ID == at.NodeID && !(at.TargetKind == "criterion" && e.Prose == at.TargetID) {
			criteria++
		}
	}
	r.Checks = append(r.Checks,
		ReadoutCheck{Label: "Gates still awaiting attestation on this node", Result: strconv.Itoa(gates)},
		ReadoutCheck{Label: "Criteria still awaiting attestation on this node", Result: strconv.Itoa(criteria)},
	)
}

// syncContent answers "the basis each crossing rested on, what stayed in
// review and why" straight from act.Crossed and act.Awaiting, both carried
// verbatim from mutate.SyncResult's own Crossings and Awaiting
// (readout_gate.go): sync already computes the basis (ancestry, content,
// rollup, human_attested) and the stayed-in-review reason (a missing
// branch, a fast-forward strand it cannot tell from a branch that never
// diverged) by walking git, and a tree read has no way to recover either
// without walking git a second time, which views performs no I/O to do.
// Round 1's in_review census is gone: it could not have named a reason, and
// mutate.Sync already has one for exactly the nodes it looked at.
func syncContent(t *store.Tree, in ReadoutInput, r *Readout) {
	act := in.Act
	if act == nil {
		return
	}
	// headlineFor's "Crossings recorded" is the moment's name, not an
	// observation, and a sync that observed nothing is the ordinary case: it
	// runs on every serve refresh and on demand. Printed unchanged it sat
	// directly under a confirmation line saying no crossings were recorded
	// (task-9-findings-round-1.md, small item 3). The headline says which way
	// the run came out; the checks below still carry the counts.
	if len(act.Crossed) == 0 {
		r.Headline = "No crossings to record"
	}
	if len(act.Crossed) > 0 {
		byBasis := map[string]int{}
		for _, c := range act.Crossed {
			byBasis[c.Basis]++
			for _, violation := range c.HistoricalPolicyViolations {
				result := violation.Status + "; historical source " + violation.SourceRevision
				if len(violation.UnresolvedCriteria) > 0 {
					result += "; unresolved criteria: " + strings.Join(violation.UnresolvedCriteria, ", ")
				}
				r.Checks = append(r.Checks, ReadoutCheck{Label: "Historical policy " + c.NodeID + " " + violation.Gate, Result: result})
			}
		}
		r.Checks = append(r.Checks,
			ReadoutCheck{Label: "Crossings recorded", Result: strconv.Itoa(len(act.Crossed))},
			ReadoutCheck{Label: "Basis", Result: basisBreakdown(byBasis)})
	}
	if len(act.Awaiting) > 0 {
		stranded := 0
		for _, a := range act.Awaiting {
			if a.NoMergeWitness || a.BranchMissing || a.ReviewMissing {
				stranded++
			}
		}
		r.Checks = append(r.Checks,
			ReadoutCheck{Label: "Still in review", Result: strconv.Itoa(len(act.Awaiting))},
			ReadoutCheck{Label: "Of which stranded (missing review, missing branch, or no merge witness)",
				Result: strconv.Itoa(stranded)})
	}
}

// lookupNode resolves noun/id back to a node in t, the tree ReadoutOfInput
// was handed. approveContent is the one caller: it needs a node whose
// NodeStatus() reads AFTER the approval, and the tree ReadoutOfInput
// receives is exactly that when the caller reloaded fresh from disk
// (renderReadoutFor's own store.Load), which every gate call site does
// (readout_gate.go).
func lookupNode(t *store.Tree, noun, id string) store.Node {
	switch noun {
	case store.NounEpic:
		if n, ok := t.Epics[id]; ok {
			return n
		}
	case store.NounFeature:
		if n, ok := t.Features[id]; ok {
			return n
		}
	case store.NounTask:
		if n, ok := t.Tasks[id]; ok {
			return n
		}
	default:
		if w, ok := t.WorkNode(id); ok && w.Noun() == noun {
			return w
		}
	}
	return nil
}

// resolveWorkNode resolves the three grains questions may block from either a
// canonical ID or a legacy recorded key. New question writes store IDs, but an
// answer to a hand-authored older ledger preserves its recorded Blocks values.
func resolveWorkNode(t *store.Tree, raw string) store.Node {
	if t == nil {
		return nil
	}
	if node := t.Epics[raw]; node != nil {
		return node
	}
	if node := t.Features[raw]; node != nil {
		return node
	}
	if node := t.Tasks[raw]; node != nil {
		return node
	}
	noun := ""
	switch strings.Count(raw, "/") {
	case 0:
		noun = store.NounEpic
	case 1:
		noun = store.NounFeature
	case 2:
		noun = store.NounTask
	default:
		return nil
	}
	node, err := resolve.NewResolver(t).Resolve(resolve.Ref{Noun: noun, Raw: raw})
	if err != nil {
		return nil
	}
	switch node.(type) {
	case *store.Epic, *store.Feature, *store.Task:
		return node
	default:
		return nil
	}
}

// verbListOrNone renders a set of legal-next verbs, sorted for determinism
// (a map's range order is unspecified), or "none" when an approval left
// nothing newly legal (a batch of every-node-already-ready no-ops, say).
func verbListOrNone(verbs map[string]bool) string {
	if len(verbs) == 0 {
		return "none"
	}
	out := make([]string, 0, len(verbs))
	for v := range verbs {
		out = append(out, v)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// blockedByAnOpenQuestion reports whether the resolved work node still has an
// operative question blocker. Ancestor inheritance and feature assumptions
// therefore match the command guards instead of a weaker direct-ID scan.
func blockedByAnOpenQuestion(t *store.Tree, nodeID string) bool {
	node := resolveWorkNode(t, nodeID)
	return node != nil && len(questionBlockersForNode(t, node)) > 0
}

// decomposeContent fills in decompose's readout (task-3-brief.md, Step 3):
// every draft epic named with its feature count, plus the readiness gate and
// DependencyShape's edge count as the completeness bar made mechanical
// (internal/views/depcheck.go).
func decomposeContent(t *store.Tree, in ReadoutInput, r *Readout) {
	order := gen.DependencyOrder(t)
	for _, id := range order.Epics() {
		e := t.Epics[id]
		if e == nil || e.Key == nil {
			continue
		}
		if len(r.Entities) < entityCap {
			r.Entities = append(r.Entities, ReadoutEntity{
				Ref:  gen.Ref(t, e),
				Note: fmt.Sprintf("%d features", featureCount(t, e.ID)),
			})
			continue
		}
		r.Withheld++
	}
	ready := "not checked"
	if in.Ready {
		ready = "passed"
	}

	// Draft epics and draft features are counted by Status, not by tree size:
	// the brief's own Step 3 snippet counted len(t.Epics) under a "Draft
	// epics" label, which reads wrong once an approved epic is in the same
	// tree (a decompose readout run mid-project, not only at the first lay-
	// down). Fixed here alongside the design doc's missing draft-features
	// line (task-3-report.md finding 3) since both are the same "the label
	// promises draft, the number does not" defect.
	draftEpics, draftFeatures := 0, 0
	for _, e := range t.Epics {
		if e.Status == "draft" {
			draftEpics++
		}
	}
	for _, f := range t.Features {
		if f.Status == "draft" {
			draftFeatures++
		}
	}

	// Open questions needs no I/O-derived input: the ledger is loaded into
	// the tree already (store.Tree.Questions, from .tiller/decisions/
	// open-questions.yml), and gen.QuestionOpen is the same open/closed
	// predicate queueAnswerQuestions (views/queue.go) filters by, so this
	// count and the queue's own "answer_question" group can never disagree.
	openQuestions := 0
	for _, q := range t.Questions {
		if gen.QuestionOpen(q) {
			openQuestions++
		}
	}

	shape := DependencyShape(t)
	r.Checks = append(r.Checks,
		ReadoutCheck{Label: "Readiness", Result: ready},
		ReadoutCheck{Label: "Draft epics", Result: strconv.Itoa(draftEpics)},
		ReadoutCheck{Label: "Draft features", Result: strconv.Itoa(draftFeatures)},
	)
	r.Checks = append(r.Checks, epicSpecChecks(t, in)...)
	r.Checks = append(r.Checks,
		ReadoutCheck{Label: "Dependency edges recorded",
			Result: fmt.Sprintf("%d, of which %d cross-epic", shape.Edges, shape.CrossEpic)},
		// A neutral graph-shape diagnostic, not a dependency-density quality bar.
		ReadoutCheck{Label: "Epics with no cross-epic edge",
			Result: fmt.Sprintf("%d of %d", shape.EpicsWithNoCrossEpic, shape.Epics)},
		ReadoutCheck{Label: "Open questions", Result: strconv.Itoa(openQuestions)},
	)
	// The decompose moment is where the edges were just laid, so degradation
	// belongs beside the edge counts it undermines.
	if c := dependencyOrderCheck(t, order); c != nil {
		r.Checks = append(r.Checks, *c)
	}
}

// epicSpecChecks renders decompose's "epic specs written" line (design doc's
// per-moment table; task-3-report.md finding 3) from the caller-supplied
// per-epic answers, since reading epic.md's text is I/O views performs none
// of. A nil EpicSpecs means the caller never ran this check this pass and
// the line reads "not evaluated" rather than a false zero; a non-nil map,
// even an empty one, renders its real count, including a real zero for an
// epic id it holds no entry for (the same nil-versus-empty discipline
// Contracts follows, task-3-report.md finding 1).
//
// The companion "Acceptance bars valid" line is gone with epic.eval.md: an
// epic has no acceptance contract to validate, and the bars that matter are
// the features' own, which specifyContent already counts.
func epicSpecChecks(t *store.Tree, in ReadoutInput) []ReadoutCheck {
	if in.EpicSpecs == nil {
		return []ReadoutCheck{{Label: "Epic specs written", Result: "not evaluated"}}
	}
	proseWritten := 0
	for _, e := range t.Epics {
		if in.EpicSpecs[e.ID].ProseWritten {
			proseWritten++
		}
	}
	return []ReadoutCheck{
		{Label: "Epic specs written", Result: fmt.Sprintf("%d of %d", proseWritten, len(t.Epics))},
	}
}

// featureCount counts epicID's features. Feature.EpicKey holds the owning
// epic's KEY, not its id (store/node.go's own comment on the field, tree.go's
// loader), so the match runs through the epic's Key rather than its ID.
func featureCount(t *store.Tree, epicID string) int {
	e := t.Epics[epicID]
	if e == nil || e.Key == nil {
		return 0
	}
	n := 0
	for _, f := range t.Features {
		if f.EpicKey == *e.Key {
			n++
		}
	}
	return n
}

// orientContent fills in orient's readout: the project census by status. The
// ranked Next list, not a check, says what to do (plan ruling 7).
func orientContent(t *store.Tree, in ReadoutInput, order *gen.Order, r *Readout) {
	ready := "not checked"
	if in.Ready {
		ready = "passed"
	}

	r.Checks = append(r.Checks,
		ReadoutCheck{Label: "Readiness", Result: ready},
		ReadoutCheck{Label: "Epics by state", Result: stateCounts(epicStatuses(t), epicStatusOrder)},
		ReadoutCheck{Label: "Features by state", Result: stateCounts(featureStatuses(t), featureStatusOrder)},
		ReadoutCheck{Label: "Tasks by state", Result: stateCounts(taskStatuses(t), taskStatusOrder)},
	)
	if c := dependencyOrderCheck(t, order); c != nil {
		r.Checks = append(r.Checks, *c)
	}
}

// dependencyOrderCheck keeps execution blockers distinct from grouping-only
// advisories and independently explains overridden rendering preferences.
func dependencyOrderCheck(t *store.Tree, order *gen.Order) *ReadoutCheck {
	diagnostics := readiness.DependencyDiagnostics(t)
	refX, refY, overridden := overriddenSoftOrder(t, order)
	if len(diagnostics.Gaps) == 0 && len(diagnostics.Advisories) == 0 && !overridden {
		return nil
	}

	var parts []string
	for _, gap := range diagnostics.Gaps {
		parts = append(parts, "Blocking: "+gap.Message)
	}
	for _, advisory := range diagnostics.Advisories {
		parts = append(parts, "Advisory (nonblocking): "+advisory.Message)
	}
	result := strings.Join(parts, "; ")

	if overridden {
		clause := fmt.Sprintf("soft order for %s and %s is overridden by recorded dependencies",
			gen.RefByID(t, refX), gen.RefByID(t, refY))
		if result == "" {
			result = clause
		} else {
			result += "; " + clause
		}
	}

	return &ReadoutCheck{Label: "Dependency order", Result: result}
}

// overriddenSoftOrder finds the first scope, the root epic list, then each
// epic's own feature list, then each feature's own task list, where two
// soft-listed siblings render in the opposite order from their recorded
// ordinals: the dependency layers (never violated, the top of the
// precedence stack) placed one ahead of the other despite the soft list
// asking the other way. Rendered inversion is the definition of overridden,
// because soft order is the top tiebreak below the layers
// (docs/spec/cli-surface.md), so anything that outranks it in the final render
// can only be a recorded edge. Detector, not a gate: soft order never gates, so
// this names the fact and touches nothing else in the tree. It consumes the
// order gen.DependencyOrder already computed rather than walking the graph
// again; refX is the soft-listed-earlier of the pair, refY the one dependencies
// rendered ahead of it. found is false once no scope has one.
func overriddenSoftOrder(t *store.Tree, order *gen.Order) (refX, refY string, found bool) {
	if x, y, ok := firstInvertedPair(t.ProjectOrder.Epics, order.Epics()); ok {
		return x, y, true
	}
	for _, eid := range order.Epics() {
		e := t.Epics[eid]
		if e == nil || e.Key == nil {
			continue
		}
		if x, y, ok := firstInvertedPair(e.Order, order.Features(*e.Key)); ok {
			return x, y, true
		}
	}
	for _, f := range workflow.OrderedFeatures(t, order) {
		ranked := order.Tasks(f.ID)
		ids := make([]string, len(ranked))
		for i, tk := range ranked {
			ids[i] = tk.ID
		}
		if x, y, ok := firstInvertedPair(f.Order, ids); ok {
			return x, y, true
		}
	}
	return "", "", false
}

// firstInvertedPair is the pairwise scan one scope's detection runs. It
// restricts rendered to the ids soft also lists, keeping rendered's own
// order (call that restricted sequence listed), then scans every pair of
// positions (i, j) with i < j over listed and returns the first one whose
// soft ordinals disagree with that render order (ord[listed[i]] >
// ord[listed[j]]).
//
// The rule for "first": lowest i wins, and for that i, lowest j wins,
// exactly the order two nested loops (i outer, j inner, both ascending from
// 0) visit pairs in. It is not the pair with the widest ordinal gap, the
// pair closest to the end of listed, or any other measure, and it is
// deterministic because listed has one fixed order and the nested scan
// always walks it the same way, so a given (soft, rendered) always names
// the same pair. Pinning i first, ahead of j, means an earlier position in
// listed can claim the first inversion against a later position before a
// later position gets the chance to pair with something nearer to it: for
// soft = [P, Q, R, S] and rendered = [Q, S, P] (R renders nowhere in this
// scope), listed is [Q, S, P] and i=0 (Q) already inverts against j=2 (P)
// before the scan ever reaches i=1 (S) against j=2 (P), even though that
// second pair is also inverted, and by a wider ordinal gap; the named pair
// is (P, Q), not (P, S).
//
// x is the soft-listed-earlier id of the winning pair (the smaller
// ordinal), y the one that rendered ahead of it anyway. A soft list of
// fewer than two ids has no pair to invert.
func firstInvertedPair(soft, rendered []string) (x, y string, found bool) {
	if len(soft) < 2 {
		return "", "", false
	}
	ord := make(map[string]int, len(soft))
	for i, id := range soft {
		ord[id] = i
	}
	var listed []string
	for _, id := range rendered {
		if _, ok := ord[id]; ok {
			listed = append(listed, id)
		}
	}
	for i := 0; i < len(listed); i++ {
		for j := i + 1; j < len(listed); j++ {
			if ord[listed[i]] > ord[listed[j]] {
				return listed[j], listed[i], true
			}
		}
	}
	return "", "", false
}

// ObservedTask is a task execution observations show in flight on Branch,
// owned there by Owner (issue #149).
type ObservedTask struct {
	Task   *store.Task
	Branch string
	Owner  string
}

// inReviewHumanWait reports whether an in_review task waits on a human
// merge. A recorded task PR is historical authority and wins before current
// grain resolution, protecting pre-hardening trees whose settings were
// changed by hand. Without one, task grain (including a task that started at
// task grain under a now-coarse parent) waits on the task as before; coarse
// grain waits only after the currently reviewed feature or epic has recorded
// its own surface.
func inReviewHumanWait(t *store.Tree, task *store.Task) bool {
	if task.PR != nil {
		return true
	}
	feature := t.Features[task.FeatureID]
	if feature == nil {
		return true
	}
	epic := recapEpicByKey(t, feature.EpicKey)
	// A task started at task grain keeps its own review (issue #146).
	grain, _ := store.TaskReviewGrain(t.Manifest, epic, feature, task)
	switch grain {
	case store.ReviewGrainFeature:
		return feature.RecoveredReview == nil && workflow.FeatureTasksReadyToSubmit(t, feature) &&
			workflow.HasRecordedReview(feature.PR, feature.ReviewRef)
	case store.ReviewGrainEpic:
		return epic != nil && epic.RecoveredReview == nil && workflow.HasRecordedReview(epic.PR, epic.ReviewRef) &&
			workflow.EpicAdvice(t, gen.DependencyOrder(t), epic, workflow.Input{}) != nil
	default:
		return true
	}
}

// ownTaskGuidance is what the owner still owes their task before its next
// step, "" when nothing is owed.
func ownTaskGuidance(t *store.Tree, feature *store.Feature, task *store.Task) string {
	switch {
	case task.Status == "check":
		return "finish any outstanding checks, then publish and submit it"
	case task.Status == "ready" && len(task.DependsOn) > 0 && store.ResolvedReviewGrain(t.Manifest, recapEpicByKey(t, feature.EpicKey), feature) != store.ReviewGrainTask:
		return "assemble submitted prerequisites on the aggregate review branch before starting this task"
	}
	return ""
}

// ownedFeatureProblem names what holds a live feature the owner has
// unfinished work in: its observed eligibility failure, an open question on
// it, or the first observed eligibility failure on one of the owner's tasks.
// It is "" when the feature is not the owner's or nothing holds it.
func ownedFeatureProblem(t *store.Tree, feature *store.Feature, owner string, in workflow.Input) string {
	if (feature.Status != "ready" && feature.Status != "active") || !featureHasAssignment(t, feature, owner) {
		return ""
	}
	problem := in.Eligibility[feature.ID]
	if blockers := gen.FeatureQuestionBlockers(t, feature); len(blockers) > 0 {
		problem = "open question " + blockers[0].Question
	}
	if problem == "" {
		for _, task := range t.FeatureTasks[feature.ID] {
			if task.Owner == owner && gen.UnfinishedAssignment(task.Status) && in.Eligibility[task.ID] != "" {
				problem = in.Eligibility[task.ID]
				break
			}
		}
	}
	return problem
}

// blockedFeatureAdvice is the human resolve_blocker advice for a held
// feature, keeping any missing-review guidance its closeout would give.
func blockedFeatureAdvice(t *store.Tree, feature *store.Feature, problem string, in workflow.Input) *workflow.Advice {
	advice := workflow.AdviceOf(t, feature, in)
	if advice == nil {
		advice = &workflow.Advice{Phase: "blocked", Actor: "human", Action: "resolve_blocker", Reason: problem, ReviewID: feature.ID, FeatureID: feature.ID, ContractStatus: "unread"}
		if owner := workflow.ReviewOwnerOf(t, feature); owner != nil {
			if _, missing := workflow.ReviewStateOf(t, feature); missing {
				advice.ReviewMissing = true
				advice.ReviewGuidance = workflow.MissingReviewGuidance(owner)
				advice.Reason += "; " + advice.ReviewGuidance
			}
		}
		return advice
	}
	advice.Phase, advice.Actor, advice.Action, advice.Command = "blocked", "human", "resolve_blocker", ""
	advice.Reason = problem
	if advice.ReviewGuidance != "" {
		advice.Reason += "; " + advice.ReviewGuidance
	}
	return advice
}

// routeTaskParent applies the status and owner checks that precede the
// question guard, and resolves the task's feature.
func routeTaskParent(t *store.Tree, task *store.Task, owner string) (*store.Feature, bool) {
	if task.Status != "ready" && task.Status != "in_progress" && task.Status != "check" {
		return nil, false
	}
	if owner == "" || task.Owner != owner {
		return nil, false
	}
	feature := t.Features[task.FeatureID]
	return feature, feature != nil
}

// frontierFeatures lists every draft feature the frontier accepts, best
// first by frontierRank: each is a specify move (skill-set.md, "draft
// features whose depends_on are satisfied").
func frontierFeatures(t *store.Tree, order *gen.Order) []*store.Feature {
	type rankedFeature struct {
		feature *store.Feature
		key     frontierRank
	}
	pref := newFrontierPreference(t)
	var accepted []rankedFeature
	for pos, feat := range flowFeatures(t, order) {
		if feat.Status != "draft" {
			continue
		}
		// A retired ancestor epic suppresses the frontier here for the same
		// reason it suppresses the queue's approve rows (queue.go's
		// queueApproveFeatures and queueApproveTaskBatches, issue #37):
		// `tiller spec feature` refuses draft -> specced under a cancelled or
		// archived epic (transition.go's spec case, epicRetired, issue #36),
		// so routing an agent at this feature would be sending it at a command
		// that fails when copied. The walk keeps going rather than stopping,
		// the same skip-not-stop shape the terminal-feature filter above it
		// already uses.
		if gen.RetiredAncestor(t, feat) != nil {
			continue
		}
		// The strict dependency gate (JC's call, 2026-08-05): a draft is
		// whose-turn-is-now only once every recorded dependency is settled
		// and built, so a feature waiting on in-flight work is skipped, not
		// suggested: the clause skill-set.md promised ("draft features
		// whose depends_on are satisfied") and this walk never implemented
		// until the Acme incident exposed the gap. `tiller spec feature`
		// stays legal on any draft the human names; the gate shapes only the
		// suggestion.
		//
		// A draft epic is a decomposition the human has not approved yet, so
		// its features are not a backlog to pull from: the frontier offers
		// only features under an approved epic (issue #93), and the epic's
		// approval is its own move instead. Same skip-not-stop shape as the
		// filters above, and the same suggestion-only reach: `tiller spec
		// feature` stays legal on any draft the human names. A feature whose
		// EpicKey names no epic gates on its own edges alone, as
		// workflow.UnmetDependency already does.
		owning := pref.epicByKey[feat.EpicKey]
		if owning != nil && owning.Status == "draft" {
			continue
		}
		if workflow.UnmetDependency(t, feat, owning) != "" {
			continue
		}
		accepted = append(accepted, rankedFeature{feat, pref.rank(feat, pos)})
	}
	sort.SliceStable(accepted, func(i, j int) bool { return accepted[i].key.less(accepted[j].key) })
	out := make([]*store.Feature, len(accepted))
	for i, r := range accepted {
		out[i] = r.feature
	}
	return out
}

// frontierRank orders the frontier's candidates by recorded intent before
// flow position: the owning epic's soft ordinal (listed before unlisted),
// then the feature's own soft ordinal within that epic, then the flow
// position the walk found the candidate at. Eligibility stays the dependency
// gate's alone; this rank only chooses among candidates already eligible, so
// a tree with no recorded soft order keeps the pure flow answer, every
// candidate unlisted and flow position deciding. Flow position stays the
// last tier rather than a co-equal one because layer depth is static
// structure: once a candidate's dependencies are settled its depth is
// history, not a live constraint, and preferring it would punish exactly the
// epics whose foundations are complete (the Acme finding, issue #46).
type frontierRank struct {
	epicOrd int
	featOrd int
	flowPos int
}

func (a frontierRank) less(b frontierRank) bool {
	if a.epicOrd != b.epicOrd {
		return a.epicOrd < b.epicOrd
	}
	if a.featOrd != b.featOrd {
		return a.featOrd < b.featOrd
	}
	return a.flowPos < b.flowPos
}

// frontierPreference resolves a candidate's soft ordinals from the tree's
// recorded order lists: the root epic list for the epic tier, the owning
// epic's own list for the feature tier. An unlisted id ranks after every
// listed one at its tier.
type frontierPreference struct {
	preference gen.Preference
	epicByKey  map[string]*store.Epic
}

// unlistedOrd is the rank an id carries at a tier whose recorded list does
// not name it. It is an absolute sentinel rather than one derived from any
// list's length, because the two tiers have unrelated cardinalities: an
// epic's own Order list can easily run longer than the count the epic tier
// would suggest, and a derived sentinel small enough to collide would rank a
// deep-listed feature at or below an unlisted one, inverting the
// listed-before-unlisted contract the soft order rests on.
const unlistedOrd = math.MaxInt

func newFrontierPreference(t *store.Tree) frontierPreference {
	p := frontierPreference{
		preference: gen.NewPreference(t),
		epicByKey:  make(map[string]*store.Epic, len(t.Epics)),
	}
	for _, e := range t.Epics {
		if k := e.Key; k != nil && *k != "" {
			p.epicByKey[*k] = e
		}
	}
	return p
}

func (p frontierPreference) rank(f *store.Feature, flowPos int) frontierRank {
	key := frontierRank{epicOrd: unlistedOrd, featOrd: unlistedOrd, flowPos: flowPos}
	e := p.epicByKey[f.EpicKey]
	if e == nil {
		return key
	}
	key.epicOrd = p.preference.Rank(e.ID)
	key.featOrd = p.preference.Rank(f.ID)
	return key
}

// flowFeatures resolves the frontier's order (gen.Order.FeatureFlow, one
// global layered order over the recorded feature-grain edges) to the tree's
// own feature values. The flow covers every feature the tree holds, orphans
// included, so unlike workflow.OrderedFeatures there is no appendix to reconcile; a
// flow id the tree no longer answers to (an order built from another tree, a
// fixture mismatch) is simply skipped.
//
// The frontier walks THIS order rather than the epic-grouped render order
// because the two can still disagree exactly where it matters: an implied
// epic cycle degrades epic rank to its own features' flow order rather than
// key order (gen.CycleGroup), but the epic-grouped render still clusters
// each epic's own features together at that one rank (the Acme finding: 8
// of 11 epics in one group manufactured from legal feature edges); the
// frontier needs the fully global flow across every epic, not clustered by
// epic, while the feature-grain graph itself stays acyclic and honest
// throughout.
func flowFeatures(t *store.Tree, order *gen.Order) []*store.Feature {
	ids := order.FeatureFlow()
	out := make([]*store.Feature, 0, len(ids))
	for _, id := range ids {
		if f := t.Features[id]; f != nil {
			out = append(out, f)
		}
	}
	return out
}

// specifyContent fills in specify's readout: every specced feature awaiting
// approval, named with its criteria count, plus the two checks
// task-3-brief.md's Step 5 names as specify's distinctive ones. "Each
// self-satisfiable" (the design doc's third verification item, the
// contract-grain rule specify/SKILL.md spells out) is a judgment about
// whether a criterion can be proven by the feature that carries it, not a
// structural fact the tree records, so it is not rendered as its own
// mechanical check here; ValidateContract's well-formedness pass is the part
// of that rule a pure reader can actually stand behind.
func specifyContent(t *store.Tree, in ReadoutInput, r *Readout) {
	order := gen.DependencyOrder(t)
	var specced []*store.Feature
	for _, f := range workflow.OrderedFeatures(t, order) {
		if f.Status == "specced" {
			specced = append(specced, f)
		}
	}

	for _, f := range specced {
		note := "contract not read"
		if ef := in.Contracts[f.ID]; ef != nil {
			note = fmt.Sprintf("%d criteria", len(ef.Criteria))
		}
		if len(r.Entities) < entityCap {
			r.Entities = append(r.Entities, ReadoutEntity{Ref: gen.Ref(t, f), Note: note})
			continue
		}
		r.Withheld++
	}

	// A nil Contracts means no caller has ever read a single feature.eval.md
	// this pass (Task 5's CLI wiring not landed yet, or simply not run this
	// time): "0 of N" would assert a real answer about contract validity that
	// was never computed, the checks-not-tallies bar reintroduced one layer
	// down (task-3-report.md finding 1). Both lines read "not evaluated"
	// instead. A non-nil, empty map is different: the caller genuinely read
	// zero contracts (there may be zero specced features to read), and that
	// real zero renders below rather than being folded into this branch.
	if in.Contracts == nil {
		r.Checks = append(r.Checks,
			ReadoutCheck{Label: "Contracts valid", Result: "not evaluated"},
			ReadoutCheck{Label: "Criteria bound by type", Result: "not evaluated"},
		)
		return
	}

	valid := 0
	byType := map[string]int{}
	for _, f := range specced {
		ef := in.Contracts[f.ID]
		if ef == nil || ef.ValidateContract() != nil {
			continue
		}
		valid++
		for _, c := range ef.Criteria {
			byType[c.Binding.Type]++
		}
	}

	r.Checks = append(r.Checks,
		ReadoutCheck{Label: "Contracts valid", Result: fmt.Sprintf("%d of %d", valid, len(specced))},
		ReadoutCheck{Label: "Criteria bound by type", Result: bindingBreakdown(byType)},
	)
}

// proposedAmendments returns every proposed amendment, id-ascending. Id order
// is the only deterministic order available: amendments carry no dependency
// edges among themselves the way epics and features do (an amendment is a
// flat staging document, not part of the one-tree hierarchy, store/amendment.go's
// own doc comment), so there is no gen.Order ranking to borrow.
func proposedAmendments(t *store.Tree) []*store.Amendment {
	var out []*store.Amendment
	for _, a := range t.Amendments {
		if a.Status == store.AmendmentStatusProposed {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// amendContent fills in amend's readout (task-5-brief.md): every proposed
// amendment named with its staged operation count, plus the two checks that
// distinguish this moment from every other one: how many operations are
// staged tree-wide (a plain tree read, no I/O needed), and how each staged
// precondition reads against the live tree right now (a git-backed
// comparison only the caller can run, fed through in.AmendmentPreconditions
// the same way orient's Readiness line is fed through in.Ready). NeedsYou
// picks up the queue's own approve_amendment rows (ReadoutOfInput's own
// append of needsYouFromQueue, after every content function runs), and Next
// comes from NextOf automatically, so neither is this function's job. With no
// proposed amendments Entities and Withheld both stay zero, which renders no
// Produced section at all, the same honest empty every other moment's
// content function leaves when it finds nothing to name.
func amendContent(t *store.Tree, in ReadoutInput, r *Readout) {
	amendments := proposedAmendments(t)

	totalOps := 0
	for _, a := range amendments {
		totalOps += len(a.Operations)
		if len(r.Entities) < entityCap {
			r.Entities = append(r.Entities, ReadoutEntity{
				Ref: gen.Ref(t, a),
				Note: fmt.Sprintf("%d %s staged", len(a.Operations),
					plural(len(a.Operations), "operation", "operations")),
			})
			continue
		}
		r.Withheld++
	}

	r.Checks = append(r.Checks,
		ReadoutCheck{Label: "Operations staged", Result: strconv.Itoa(totalOps)},
		amendmentPreconditionsCheck(amendments, in),
	)
}

// amendmentPreconditionsCheck renders amend's live-validity line. nil means
// the caller never ran mutate.EvaluateAmendmentPreconditions this pass (the
// nil-versus-empty discipline Contracts and EpicSpecs already follow):
// "not evaluated" rather than a false "0 valid, 0 stale" that would claim a
// real answer nobody computed. A precondition read as PreconditionUnknown
// (an add-epic, add-feature, or add-task operation's own staged key, not yet
// materialized) counts toward neither valid nor stale: neither verdict is
// what an unmaterialized target is answering, the same distinction
// EvaluateAmendmentPreconditions' own doc comment draws.
func amendmentPreconditionsCheck(amendments []*store.Amendment, in ReadoutInput) ReadoutCheck {
	if in.AmendmentPreconditions == nil {
		return ReadoutCheck{Label: "Preconditions", Result: "not evaluated"}
	}
	valid, stale := 0, 0
	for _, a := range amendments {
		for _, pv := range in.AmendmentPreconditions[a.ID] {
			switch pv.Status {
			case mutate.PreconditionValid:
				valid++
			case mutate.PreconditionStale:
				stale++
			}
		}
	}
	return ReadoutCheck{Label: "Preconditions", Result: fmt.Sprintf("%d valid, %d stale", valid, stale)}
}

// gateHasEvidence reports whether a gate carries recorded evidence, either at
// gate grain (Evidence, the shape a command or test gate uses) or at
// criterion grain (Criteria[*].Evidence, the shape the evals gate uses
// instead; store/gates.go's Gate doc comment). Checking only one would miss
// the other kind's honest pass.
func gateHasEvidence(g store.Gate) bool {
	if len(g.Evidence) > 0 {
		return true
	}
	for _, c := range g.Criteria {
		if len(c.Evidence) > 0 {
			return true
		}
	}
	return false
}

// gatesGreenWithEvidence reports whether every required gate on a task is
// pass AND backed by recorded evidence, execute's "gates green with
// evidence" distinctive check (task-3-brief.md, Step 5): a required gate
// marked pass with nothing behind it is the hollow pass this task exists to
// catch, the same mechanical-over-narrated thesis the whole readout rests on
// (docs/spec/cli-surface.md). A task
// with no required gates at all, whether it carries no gates or only
// optional ones, returns false rather than passing vacuously
// (task-3-report.md finding 4): a required build-and-tests gate is the
// normal configuration per execute/SKILL.md, so a task with none recorded is
// an unconfigured project, not a task with nothing left to prove.
func gatesGreenWithEvidence(gates map[string]store.Gate) bool {
	required := 0
	for _, g := range gates {
		if !g.Required {
			continue
		}
		required++
		if g.Status != store.GateStatusPass || !gateHasEvidence(g) {
			return false
		}
	}
	return required > 0
}

// orderedTasks returns every task in feature order then sibling order: each
// feature's own tasks ranked by gen.Order (the sibling sequence
// depends_on records, artifact-schema.md, Tasks: Ordering) within the feature
// ordering workflow.OrderedFeatures already established, so a tree-wide task scan
// (orient's task-in-hand check, execute's in-review batch, ranked next)
// reads deterministically without a bespoke sort at each call
// site.
func orderedTasks(t *store.Tree, order *gen.Order) []*store.Task {
	var out []*store.Task
	for _, f := range workflow.OrderedFeatures(t, order) {
		out = append(out, order.Tasks(f.ID)...)
	}
	return out
}

// epicStatusOrder, featureStatusOrder, and taskStatusOrder fix the column
// order orient's census reads in: each noun's own state list from
// lifecycle/table.go's transitions table, earliest state first, so the count
// line reads the same regardless of map iteration order.
var (
	epicStatusOrder    = []string{"draft", "ready", "done", "cancelled", "archived"}
	featureStatusOrder = []string{"draft", "specced", "ready", "active", "done", "cancelled"}
	taskStatusOrder    = []string{"proposed", "ready", "in_progress", "check", "in_review", "done", "cancelled"}
)

func epicStatuses(t *store.Tree) map[string]int {
	m := map[string]int{}
	for _, e := range t.Epics {
		m[e.Status]++
	}
	return m
}

func featureStatuses(t *store.Tree) map[string]int {
	m := map[string]int{}
	for _, f := range t.Features {
		m[f.Status]++
	}
	return m
}

func taskStatuses(t *store.Tree) map[string]int {
	m := map[string]int{}
	for _, tk := range t.Tasks {
		m[tk.Status]++
	}
	return m
}

// stateCounts renders a status census in order's sequence, omitting any
// status with zero members so a small tree's line stays short; a tree order
// never sees a member of at all (an empty project) reads "none" rather than
// a blank line the eventual markdown renderer would have to special-case.
func stateCounts(counts map[string]int, order []string) string {
	var parts []string
	for _, s := range order {
		if n := counts[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

// bindingBreakdown renders specify's criteria-by-type line in the eval
// model's own binding order (store/eval.go's Binding type consts), the same
// "counted, not guessed" answer DependencyShape gives decompose: a contract
// that is all `manual` reads differently from one that is mostly `test`, and
// this is the line that shows which.
func bindingBreakdown(counts map[string]int) string {
	order := []string{store.BindingTest, store.BindingCommand, store.BindingManual, store.BindingEval}
	var parts []string
	for _, k := range order {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	if len(parts) == 0 {
		return "none recorded"
	}
	return strings.Join(parts, ", ")
}

// basisBreakdown renders sync's own crossing-basis tally (syncContent) in
// mutate.Sync's own order: ancestry, content, rollup, human_attested
// (mutate/sync.go's basisAncestry/basisContent/basisRollup/
// basisHumanAttested consts, unexported there, so this file names the same
// four literal strings rather than importing across the package boundary).
// bindingBreakdown just above renders a DIFFERENT four-value vocabulary
// (eval's test/command/manual/eval binding types) that shares no values
// with sync's basis; the two must stay separate functions, since reusing
// bindingBreakdown here would silently render "none recorded" for every
// real crossing, an easy mistake to make and to miss (caught in review
// before it shipped, not after).
func basisBreakdown(counts map[string]int) string {
	order := []string{"ancestry", "content", "rollup", "human_attested"}
	var parts []string
	for _, k := range order {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	if len(parts) == 0 {
		return "none recorded"
	}
	return strings.Join(parts, ", ")
}

// amendmentOpBreakdown reports applied operations in the store's fixed kind order.
func amendmentOpBreakdown(counts map[string]int) string {
	order := []string{
		store.AmendmentOpRevise, store.AmendmentOpSupersede, store.AmendmentOpCancel,
		store.AmendmentOpAddEpic, store.AmendmentOpAddFeature, store.AmendmentOpAddTask,
		store.AmendmentOpAddQuestion, store.AmendmentOpMoveFeature, store.AmendmentOpCorrectEval,
	}
	var parts []string
	for _, k := range order {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}
