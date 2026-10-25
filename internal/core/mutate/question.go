package mutate

// This file is the question ledger's write half (issue #19): AddQuestion mints
// a Q- entry into .tiller/decisions/open-questions.yml, and AnswerQuestion
// moves it open -> answered, recording the answer and its revision
// (artifact-schema.md, "Decisions and open questions"; cli-surface.md, the add
// and answer verbs). The read surfaces (ls, status, show) and the
// blocks enforcement (start/check/submit refusing a question-blocked node)
// already exist; these ops are what let a real blocker reach them. A question
// is a ledger item like a task: an id, no key, idempotency by its prose.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

const (
	eventAddQuestion    = "add.question"
	eventAnswerQuestion = "answer.question"
	eventCancelQuestion = "cancel.question"

	statusOpen     = "open"
	statusAnswered = "answered"
)

func validateQuestionTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return &usageError{reason: "title_required", msg: "a question needs a --title"}
	}
	if strings.ContainsAny(title, "\r\n") {
		return &usageError{reason: "title_one_line", msg: "a question title must be one line"}
	}
	return nil
}

// QuestionInput is a request to record an open question.
type QuestionInput struct {
	Title     string
	Statement string
	// Blocks are the nodes this question structurally holds, addressed by id
	// or key; they are resolved and stored as ids.
	Blocks []string
	Due    store.QuestionDue
	Source string
}

// AnswerInput is a request to record the answer to an open question.
type AnswerInput struct {
	Node   string // the question, by Q- id
	Answer string
}

// QuestionResult is the outcome of a question mutation: the ledger entry's
// identity and state, with Blocks carrying the resolved ids.
type QuestionResult struct {
	Noun      string
	ID        string
	Title     string
	Statement string
	Status    string
	Blocks    []string
	Answer    string
	Path      string
	Created   bool
	Changed   bool
	// CancelReason is the recorded reason of a cancelled question.
	CancelReason string
	// AssumedBy lists, sorted, the features whose assumption_basis names a
	// question just cancelled; the entries stay on record and stop counting.
	AssumedBy []string
	// Reasks is the cancelled question id an add question re-asked, else empty.
	Reasks string
}

// CancelInput is a request to cancel an open question that needs no decision.
type CancelInput struct {
	Node   string // the question, by Q- id
	Reason string
}

// AlreadyAnsweredError refuses a re-answer that would silently change a
// recorded answer. The same answer again is an idempotent no-change success;
// a different one is this refusal, a blocked business answer for the CLI.
type AlreadyAnsweredError struct {
	QuestionID string
	Answer     string
}

func (e *AlreadyAnsweredError) Error() string {
	return fmt.Sprintf("question %s is already answered; the recorded answer does not silently change", e.QuestionID)
}

// QuestionClosedError refuses an answer to a question in a closed non-answered
// state (a hand-recorded resolved or cancelled entry): the refusal names the
// actual status rather than claiming an answer exists.
type QuestionClosedError struct {
	QuestionID string
	Status     string
	// Verb is the refused move: "answer", "cancel", "link", or "unlink".
	// Empty reads as "answer".
	Verb string
}

func (e *QuestionClosedError) Error() string {
	past := map[string]string{"cancel": "cancelled", "link": "linked", "unlink": "unlinked"}[e.Verb]
	if past == "" {
		past = "answered"
	}
	return fmt.Sprintf("question %s is %s; only an open question can be %s", e.QuestionID, e.Status, past)
}

// questionsRelPath is the ledger's repo-relative path.
const questionsRelPath = ".tiller/decisions/open-questions.yml"

// plannedQuestion is AddQuestion's whole judgment, up to but not including the
// write: the ledger's own mutated content (File), the outcome to report
// (result), the snapshot the write must be planned over, and the event to
// record, non-nil only when something actually changed. It exists so the
// amendment's approve can fold a staged add-question into its own batch the
// same way plannedAdd lets it fold a staged add-epic/add-feature/add-task in
// (add.go's plannedAdd doc comment). Unlike those three, though, a question's
// write target is the ONE shared ledger file every add-question op in a batch
// contends for, so approve does not call planAddQuestion per op (that would
// re-read the same unmodified-on-disk file for every op after the first,
// silently dropping whatever an earlier op in the same batch already folded
// in); it threads foldQuestion across every staged op against one
// batch-held QuestionsFile instead (amendment_approve.go's materialize and
// its own batch.loadQuestions, which reads the ledger once for the whole
// batch). This type stays the shape AddQuestion's own single-question path
// needs.
type plannedQuestion struct {
	result   QuestionResult
	file     store.QuestionsFile
	snapshot store.FileSnapshot
	event    *events.Event
}

// planAddQuestion is AddQuestion's whole judgment, up to but not including the
// write: it validates the explicit title, resolves block addresses, reads the
// ledger fresh off disk, and folds in through foldQuestion. A duplicate can
// union blocks into an open entry or fill an absent legacy title even when the
// question is closed; otherwise it reports the recorded state unchanged. A new
// statement takes the fresh-mint arm. AddQuestion is plan-then-commit over this
// result, like the add verbs' plannedAdd for epic, feature, and task (add.go).
func planAddQuestion(tree *store.Tree, deps Deps, in QuestionInput) (plannedQuestion, error) {
	if err := validateQuestionTitle(in.Title); err != nil {
		return plannedQuestion{}, err
	}
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return plannedQuestion{}, err
	}
	if strings.TrimSpace(in.Statement) == "" {
		return plannedQuestion{}, fmt.Errorf("mutate: a question needs a statement")
	}
	blocks, err := resolveBlockTargets(tree, in.Blocks)
	if err != nil {
		return plannedQuestion{}, err
	}

	path := filepath.Join(tree.Root, filepath.FromSlash(questionsRelPath))
	raw, snapshot, err := store.ReadRegularFileSnapshot(tree.Root, path)
	if err != nil {
		return plannedQuestion{}, err
	}
	var qf store.QuestionsFile
	if snapshot.Exists() {
		if err := yaml.Unmarshal(raw, &qf); err != nil {
			return plannedQuestion{}, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	res, ev, err := foldQuestion(tree, deps, owner, &qf, in.Title, in.Statement, blocks, in.Due, in.Source)
	if err != nil {
		return plannedQuestion{}, err
	}
	return plannedQuestion{result: res, file: qf, snapshot: snapshot, event: ev}, nil
}

// foldQuestion is planAddQuestion's core judgment, factored out so it can run
// against an ALREADY-loaded, already-mutated QuestionsFile rather than a fresh
// disk read: the statement-idempotency arm and the fresh-mint arm, exactly as
// AddQuestion's own body ran them inline before this extraction. blocks
// arrives ALREADY RESOLVED to ids, never raw addresses: planAddQuestion
// resolves in.Blocks against the live tree once, up front, and the
// amendment's approve resolves each staged Blocks entry its own way
// (amendment_approve.go's materialize) before calling in, because a
// same-batch entry resolves to a node this package's own tree argument
// cannot see yet (it was materialized this same batch, not loaded with the
// tree), so re-resolving it here through the live tree would refuse a
// perfectly good id as not found.
//
// planAddQuestion calls this once, against a copy read straight off disk. The
// amendment's approve calls it once PER staged add-question op, threading the
// SAME in-memory QuestionsFile across every call: every add-question op in
// one batch contends for the one shared ledger file, so folding them in
// memory, one after another, against the one QuestionsFile the batch is
// building, is what keeps a second op from clobbering what the first one
// just added.
//
// It returns the event to record only when something actually changed: a fresh
// question, an open entry's blocks growing, or an absent legacy title being
// filled. A duplicate that makes none of those changes returns a nil event and
// mints nothing (deps.mintIn is only reached on the fresh-mint arm).
func foldQuestion(tree *store.Tree, deps Deps, owner store.Contributor, qf *store.QuestionsFile, title, statement string, blocks []string, due store.QuestionDue, source string) (QuestionResult, *events.Event, error) {
	if strings.TrimSpace(statement) == "" {
		return QuestionResult{}, nil, fmt.Errorf("mutate: a question needs a statement")
	}
	if title != "" {
		if err := validateQuestionTitle(title); err != nil {
			return QuestionResult{}, nil, err
		}
	}

	// Idempotency is by the full statement, never the scanning title. A
	// duplicate can fill an absent legacy title, but an existing title is
	// immutable through add-question retries.
	// A cancelled entry is skipped: asking its statement again is a fresh
	// question, and the fresh-mint arm reports which cancelled one it re-asked.
	var reasks string
	for _, existing := range qf.Questions {
		if existing.Statement != statement {
			continue
		}
		if existing.Status == statusCancelled {
			reasks = existing.ID
			continue
		}
		titleBackfilled := existing.Title == "" && title != ""
		if titleBackfilled {
			existing.Title = title
		}
		res := QuestionResult{
			Noun:      store.NounQuestion,
			ID:        existing.ID,
			Title:     gen.QuestionTitle(existing),
			Statement: existing.Statement,
			Status:    existing.Status,
			Blocks:    existing.Blocks,
			Path:      questionsRelPath,
			Created:   false,
		}
		if existing.Answer != nil {
			res.Answer = *existing.Answer
		}

		var added []string
		if existing.Status == statusOpen || existing.Status == "" {
			added = unionInto(&existing.Blocks, blocks)
		}
		if !titleBackfilled && len(added) == 0 {
			return res, nil, nil
		}

		ev := events.NewEvent(deps.clock(), eventAddQuestion, owner.ID, "add question", existing.ID)
		ev.ID = deps.mintRaw("EV")
		ev.After = map[string]any{
			"title":  gen.QuestionTitle(existing),
			"status": existing.Status,
			"blocks": existing.Blocks,
		}
		ev.Files = []events.FileChange{{Path: questionsRelPath}}
		res.Blocks = existing.Blocks
		res.Changed = true
		return res, &ev, nil
	}

	qid, err := deps.mintIn(tree, "Q")
	if err != nil {
		return QuestionResult{}, nil, err
	}
	// mintIn only sees the tree this call loaded, not the ids a batch's
	// earlier folds already minted onto qf in memory. Guard the shared ledger
	// against a broken or deterministic minter returning the same id twice.
	for _, existing := range qf.Questions {
		if existing.ID == qid {
			return QuestionResult{}, nil, &usageError{
				reason: "mint_collision",
				msg:    fmt.Sprintf("%s would be minted twice onto one ledger; the minter is broken or exhausted", qid),
			}
		}
	}
	q := &store.Question{
		ID:        qid,
		Title:     title,
		Statement: statement,
		Owner:     owner.ID,
		Due:       due,
		Blocks:    blocks,
		Status:    statusOpen,
		Source:    source,
	}
	qf.Questions = append(qf.Questions, q)

	ev := events.NewEvent(deps.clock(), eventAddQuestion, owner.ID, "add question", qid)
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"title": gen.QuestionTitle(q), "status": statusOpen, "blocks": blocks}
	ev.Files = []events.FileChange{{Path: questionsRelPath}}

	return QuestionResult{
		Noun:      store.NounQuestion,
		ID:        qid,
		Title:     gen.QuestionTitle(q),
		Statement: statement,
		Status:    statusOpen,
		Blocks:    blocks,
		Path:      questionsRelPath,
		Created:   true,
		Reasks:    reasks,
	}, &ev, nil
}

// AddQuestion appends an open question to the decisions ledger, minting its
// Q- id and recording an event. Every blocks address must resolve before
// anything is written (the all-or-nothing rule); idempotency is by statement,
// the ledger's stand-in for the key a folder node is deduplicated by.
func AddQuestion(tree *store.Tree, deps Deps, in QuestionInput) (QuestionResult, error) {
	pq, err := planAddQuestion(tree, deps, in)
	if err != nil {
		return QuestionResult{}, err
	}
	if pq.event == nil {
		return pq.result, nil
	}
	data, err := yaml.Marshal(pq.file)
	if err != nil {
		return QuestionResult{}, err
	}
	path := filepath.Join(tree.Root, filepath.FromSlash(questionsRelPath))
	if err := commitMutation(tree, "add question", []store.PlannedWrite{
		store.PlanWrite(path, pq.snapshot, data),
	}, []events.Event{*pq.event}); err != nil {
		return QuestionResult{}, err
	}

	return pq.result, nil
}

// unionInto appends the additions missing from the target list, returning the
// ones it added.
func unionInto(target *[]string, additions []string) []string {
	have := make(map[string]bool, len(*target))
	for _, id := range *target {
		have[id] = true
	}
	var added []string
	for _, id := range additions {
		if !have[id] {
			*target = append(*target, id)
			have[id] = true
			added = append(added, id)
		}
	}
	return added
}

// AnswerQuestion records the answer to an open question: open -> answered,
// with the answer text and the revision it was recorded at (the HEAD commit
// on a clean tree, a working-tree fingerprint on a dirty one, the same
// provenance rule the gates use). Clearing the computed blockers falls out of
// the change itself: the blocks guard only counts open questions.
func AnswerQuestion(tree *store.Tree, deps Deps, rev vcs.Revisions, in AnswerInput) (QuestionResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return QuestionResult{}, err
	}
	if strings.TrimSpace(in.Answer) == "" {
		return QuestionResult{}, fmt.Errorf("mutate: an answer needs text")
	}

	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounQuestion, Raw: in.Node})
	if err != nil {
		return QuestionResult{}, err
	}
	q := node.(*store.Question)

	// An empty status reads as open in the blocks guard, so a hand-authored
	// entry with no status key stays answerable.
	if q.Status != statusOpen && q.Status != "" {
		if q.Status == statusAnswered {
			// The same answer again is an idempotent no-change success;
			// a different one refuses rather than silently changing the
			// recorded answer.
			if q.Answer != nil && *q.Answer == in.Answer {
				return QuestionResult{
					Noun:      store.NounQuestion,
					ID:        q.ID,
					Title:     gen.QuestionTitle(q),
					Statement: q.Statement,
					Status:    q.Status,
					Blocks:    q.Blocks,
					Answer:    *q.Answer,
					Path:      questionsRelPath,
					Changed:   false,
				}, nil
			}
			return QuestionResult{}, &AlreadyAnsweredError{QuestionID: q.ID, Answer: in.Answer}
		}
		return QuestionResult{}, &QuestionClosedError{QuestionID: q.ID, Status: q.Status}
	}

	revision, err := gateAgainst(rev)
	if err != nil {
		return QuestionResult{}, err
	}

	// Re-read the ledger from disk and mutate the matching entry, the same
	// read-modify-write the task ledger uses.
	path := filepath.Join(tree.Root, filepath.FromSlash(questionsRelPath))
	var qf store.QuestionsFile
	snapshot, err := readYAMLSnapshot(tree.Root, path, &qf)
	if err != nil {
		return QuestionResult{}, err
	}
	var target *store.Question
	for _, entry := range qf.Questions {
		if entry.ID == q.ID {
			target = entry
			break
		}
	}
	if target == nil {
		return QuestionResult{}, fmt.Errorf("mutate: question %s resolved but is missing from %s", q.ID, questionsRelPath)
	}
	// The status checked above came from the tree loaded before this read;
	// another process may have closed the entry in between.
	if target.Status != statusOpen && target.Status != "" {
		return QuestionResult{}, &QuestionClosedError{QuestionID: q.ID, Status: target.Status}
	}
	answer := in.Answer
	target.Status = statusAnswered
	target.Answer = &answer
	target.AnswerRevision = revision

	data, err := yaml.Marshal(qf)
	if err != nil {
		return QuestionResult{}, err
	}

	ev := events.NewEvent(deps.clock(), eventAnswerQuestion, owner.ID, "answer question "+q.ID, q.ID)
	// EV: same non-retry rationale as AddQuestion above; not tree-keyed.
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"status": statusAnswered}
	ev.Files = []events.FileChange{{Path: questionsRelPath}}

	if err := commitMutation(tree, "answer question", []store.PlannedWrite{
		store.PlanWrite(path, snapshot, data),
	}, []events.Event{ev}); err != nil {
		return QuestionResult{}, err
	}

	return QuestionResult{
		Noun:      store.NounQuestion,
		ID:        q.ID,
		Title:     gen.QuestionTitle(target),
		Statement: q.Statement,
		Status:    statusAnswered,
		Blocks:    q.Blocks,
		Answer:    in.Answer,
		Path:      questionsRelPath,
		Changed:   true,
	}, nil
}

// CancelQuestion cancels an open question that needs no decision: open ->
// cancelled with the human reason recorded. It writes no answer and no
// revision, keeps the blocks list on record, and runs no gate (a cancel is
// not a plan decision). A second cancel is a no-change success that keeps the
// first reason; an answered or otherwise closed question refuses.
func CancelQuestion(tree *store.Tree, deps Deps, in CancelInput) (QuestionResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return QuestionResult{}, err
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return QuestionResult{}, &usageError{reason: "reason_required", msg: "cancelling a question needs a --reason"}
	}

	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounQuestion, Raw: in.Node})
	if err != nil {
		return QuestionResult{}, err
	}
	q := node.(*store.Question)

	if q.Status == statusCancelled {
		return QuestionResult{
			Noun:         store.NounQuestion,
			ID:           q.ID,
			Title:        gen.QuestionTitle(q),
			Statement:    q.Statement,
			Status:       q.Status,
			Blocks:       q.Blocks,
			Path:         questionsRelPath,
			CancelReason: q.CancelReason,
			AssumedBy:    assumersOf(tree, q.ID),
		}, nil
	}
	if q.Status != statusOpen && q.Status != "" {
		return QuestionResult{}, &QuestionClosedError{QuestionID: q.ID, Status: q.Status, Verb: "cancel"}
	}

	path := filepath.Join(tree.Root, filepath.FromSlash(questionsRelPath))
	var qf store.QuestionsFile
	snapshot, err := readYAMLSnapshot(tree.Root, path, &qf)
	if err != nil {
		return QuestionResult{}, err
	}
	var target *store.Question
	for _, entry := range qf.Questions {
		if entry.ID == q.ID {
			target = entry
			break
		}
	}
	if target == nil {
		return QuestionResult{}, fmt.Errorf("mutate: question %s resolved but is missing from %s", q.ID, questionsRelPath)
	}
	// Re-check against the entry just read: the tree above may be stale.
	if target.Status != statusOpen && target.Status != "" {
		return QuestionResult{}, &QuestionClosedError{QuestionID: q.ID, Status: target.Status, Verb: "cancel"}
	}
	target.Status = statusCancelled
	target.CancelReason = reason

	data, err := yaml.Marshal(qf)
	if err != nil {
		return QuestionResult{}, err
	}

	ev := events.NewEvent(deps.clock(), eventCancelQuestion, owner.ID, "cancel question "+q.ID, q.ID)
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"status": statusCancelled, "cancel_reason": reason}
	ev.Files = []events.FileChange{{Path: questionsRelPath}}

	if err := commitMutation(tree, "cancel question", []store.PlannedWrite{
		store.PlanWrite(path, snapshot, data),
	}, []events.Event{ev}); err != nil {
		return QuestionResult{}, err
	}

	return QuestionResult{
		Noun:         store.NounQuestion,
		ID:           q.ID,
		Title:        gen.QuestionTitle(target),
		Statement:    q.Statement,
		Status:       statusCancelled,
		Blocks:       q.Blocks,
		Path:         questionsRelPath,
		Changed:      true,
		CancelReason: reason,
		AssumedBy:    assumersOf(tree, q.ID),
	}, nil
}

// assumersOf lists, sorted, the features whose assumption_basis names qid.
func assumersOf(tree *store.Tree, qid string) []string {
	var ids []string
	for id, f := range tree.Features {
		if _, ok := f.AssumptionBasis[qid]; ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// resolveBlockTargets resolves a blocks list to stored node ids. A question
// blocks work-tree nodes only, an epic, a feature, or a task: those are
// the classes the start/check/submit guard walks, so admitting anything else
// would record a blocker nothing enforces. An entry is an id or a key (a
// feature's is the canonical epic-qualified form), and every entry must
// resolve, so one typo cannot record a dangling blocker.
func resolveBlockTargets(tree *store.Tree, raws []string) ([]string, error) {
	if len(raws) == 0 {
		return nil, nil
	}
	r := resolve.NewResolver(tree)
	ids := make([]string, 0, len(raws))
	for _, raw := range raws {
		n, err := resolveBlockTarget(r, raw)
		if err != nil {
			return nil, err
		}
		ids = append(ids, n.NodeID())
	}
	return ids, nil
}

// resolveBlockTarget resolves one blocks address without a noun in hand: an id
// names its class by prefix, and a key names its class by segment count, since
// the canonical keys of the three blockable nouns are one, two, and three
// segments (artifact-schema.md, Keys and paths: an epic's key is the bare
// segment, a feature's is <epic-key>/<feature-key>, a task's is
// <epic-key>/<feature-key>/<task-key>). An id of a non-blockable class, or a
// shelf key, refuses plainly rather than as a confusing noun mismatch.
//
// Nothing here inspects the tree before choosing, so an address with more
// segments than any key has reaches the task lookup and refuses as a task that
// was not found, the longest form it could have been aiming at.
func resolveBlockTarget(r *resolve.Resolver, raw string) (store.Node, error) {
	nounByPrefix := map[string]string{
		"E-": store.NounEpic,
		"F-": store.NounFeature,
		"T-": store.NounTask,
	}
	for prefix, noun := range nounByPrefix {
		if strings.HasPrefix(raw, prefix) {
			return r.Resolve(resolve.Ref{Noun: noun, Raw: raw})
		}
	}
	if hasIDPrefix(raw, "Q-", "A-", "ADR-", "EV-", "AT-") || isShelfRef(raw) {
		return nil, fmt.Errorf("mutate: blocks target %q is not blockable; a question blocks an epic, a feature, or a task", raw)
	}
	switch strings.Count(raw, "/") {
	case 0:
		return r.Resolve(resolve.Ref{Noun: store.NounEpic, Raw: raw})
	case 1:
		return r.Resolve(resolve.Ref{Noun: store.NounFeature, Raw: raw})
	default:
		return r.Resolve(resolve.Ref{Noun: store.NounTask, Raw: raw})
	}
}
