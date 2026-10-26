package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin the question ledger's write half: tiller add
// question mints a Q- id into decisions/open-questions.yml with resolved
// blocks, and tiller answer question moves open to answered, recording the
// answer and its revision. The read surfaces (ls, status, show) and the
// blocks enforcement already exist; these ops are what let a real blocker
// reach them.

// writeQuestionRepo lays an owned repo with one epic and one feature, the
// targets a question's blocks list resolves against.
func writeQuestionRepo(t *testing.T) string {
	t.Helper()
	root := writeOwnedRepo(t)
	tree := load(t, root)
	_, err := mutate.AddEpic(tree, testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	tree = load(t, root)
	mintF := func(prefix string) string { return prefix + "-bbbbbb" }
	deps := testDeps()
	deps.Mint = mintF
	_, err = mutate.AddFeature(tree, deps, mutate.FeatureInput{Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	return root
}

// TestAddQuestionMintsIntoTheLedger is the core create: a Q- id minted into
// decisions/open-questions.yml, open, owned, with blocks resolved to ids no
// matter how the caller addressed the targets.
func TestAddQuestionMintsIntoTheLedger(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	tree := load(t, root)

	res, err := mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{
		Title:     "Design system location",
		Statement: "Where does the front-end design system live?",
		Blocks:    []string{"form-engine/certification-form-loading", "E-aaaaaa"},
		Due:       store.QuestionDue{Date: "2026-07-12"},
		Source:    "briefing",
	})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.Equal(t, "Q-aaaaaa", res.ID)
	require.Equal(t, "open", res.Status)
	require.Equal(t, []string{"F-bbbbbb", "E-aaaaaa"}, res.Blocks, "keys resolve to stored ids")

	// The ledger is on disk and loads back through the store.
	tree = load(t, root)
	q := tree.Questions["Q-aaaaaa"]
	require.NotNil(t, q)
	require.Equal(t, "Where does the front-end design system live?", q.Statement)
	require.Equal(t, "jc", q.Owner)
	require.Equal(t, []string{"F-bbbbbb", "E-aaaaaa"}, q.Blocks)
	require.Equal(t, "2026-07-12", q.Due.Date)
	require.Equal(t, "briefing", q.Source)
	require.Equal(t, "open", q.Status)

	// One event recorded.
	evs, err := events.List(tree)
	require.NoError(t, err)
	require.NotEmpty(t, evs)
}

// TestAddQuestionIsIdempotentByStatement mirrors the task ledger's posture: a
// retry with the same statement is a no-op success returning the existing
// entry, never a duplicate.
func TestAddQuestionIsIdempotentByStatement(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	tree := load(t, root)
	first, err := mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{Title: "Gate commands", Statement: "What are the gate commands?"})
	require.NoError(t, err)
	require.True(t, first.Created)

	tree = load(t, root)
	again, err := mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{Title: "Gate commands", Statement: "What are the gate commands?"})
	require.NoError(t, err)
	require.False(t, again.Created)
	require.Equal(t, first.ID, again.ID)

	tree = load(t, root)
	require.Len(t, tree.Questions, 1, "no duplicate ledger entry")
}

// TestAddQuestionRefusesADanglingBlock is the all-or-nothing address rule: one
// unresolvable blocks target fails the add with nothing written.
func TestAddQuestionRefusesADanglingBlock(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	tree := load(t, root)

	_, err := mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{
		Title:     "Ghost blocker",
		Statement: "Blocked on nothing real?",
		Blocks:    []string{"F-zzzzzz"},
	})
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(root, ".tiller", "decisions", "open-questions.yml"))
	require.True(t, os.IsNotExist(statErr), "nothing written on a refused add")
}

// TestAnswerQuestionRecordsAnswerAndRevision pins the answer transition: open
// to answered, the answer text stored, and the answer revision recorded from
// the working tree (the clean-tree stub records the HEAD commit).
func TestAnswerQuestionRecordsAnswerAndRevision(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	tree := load(t, root)
	added, err := mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{Title: "Canonical schema version", Statement: "Which schema version is canonical?"})
	require.NoError(t, err)

	tree = load(t, root)
	res, err := mutate.AnswerQuestion(tree, testDeps(), stubRev(root), mutate.AnswerInput{
		Node:   added.ID,
		Answer: "The 2025 revision; 2022 is read-only reference.",
	})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "answered", res.Status)

	tree = load(t, root)
	q := tree.Questions[added.ID]
	require.Equal(t, "answered", q.Status)
	require.NotNil(t, q.Answer)
	require.Equal(t, "The 2025 revision; 2022 is read-only reference.", *q.Answer)
	require.Equal(t, store.KindGitCommit, q.AnswerRevision.Kind, "a clean tree pins the HEAD commit")
	require.NotEmpty(t, q.AnswerRevision.Value)
}

// TestReAddUnionsNewBlocks pins the second-reader case: re-adding the same
// statement with new blocks is not a plain retry, so the new targets union
// into the entry as a real mutation instead of being silently dropped, which
// would leave the named node unheld by the enforcement.
func TestReAddUnionsNewBlocks(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	tree := load(t, root)
	first, err := mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{
		Title:     "Authentication provider",
		Statement: "Which auth provider?",
		Blocks:    []string{"E-aaaaaa"},
	})
	require.NoError(t, err)

	tree = load(t, root)
	again, err := mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{
		Title:     "Authentication provider",
		Statement: "Which auth provider?",
		Blocks:    []string{"form-engine/certification-form-loading", "E-aaaaaa"},
	})
	require.NoError(t, err)
	require.False(t, again.Created)
	require.True(t, again.Changed, "a re-add naming new blocks is a real mutation")
	require.Equal(t, first.ID, again.ID)
	require.ElementsMatch(t, []string{"E-aaaaaa", "F-bbbbbb"}, again.Blocks)

	tree = load(t, root)
	require.ElementsMatch(t, []string{"E-aaaaaa", "F-bbbbbb"}, tree.Questions[first.ID].Blocks, "the union persisted")

	// Once answered, a re-add stops unioning: a blocker on a closed question
	// enforces nothing, so the no-op reports the recorded state instead.
	_, err = mutate.AnswerQuestion(tree, testDeps(), stubRev(root), mutate.AnswerInput{Node: first.ID, Answer: "Devise."})
	require.NoError(t, err)
	tree = load(t, root)
	closed, err := mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{
		Title:     "Authentication provider",
		Statement: "Which auth provider?",
		Blocks:    []string{"form-engine/certification-form-loading"},
	})
	require.NoError(t, err)
	require.False(t, closed.Changed)
	require.Equal(t, "Devise.", closed.Answer, "the recorded answer rides the no-op result")
	tree = load(t, root)
	require.ElementsMatch(t, []string{"E-aaaaaa", "F-bbbbbb"}, tree.Questions[first.ID].Blocks, "no union into a closed question")
}

// TestAddQuestionRefusesNonBlockableTargets pins the work-tree rule: a
// question blocks an epic, a feature, or a task; a shelf or ledger id refuses
// plainly rather than recording a blocker nothing enforces.
func TestAddQuestionRefusesNonBlockableTargets(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	tree := load(t, root)

	for _, target := range []string{"B-aaaaaa", "CH-aaaaaa", "TW-aaaaaa", "Q-aaaaaa", "A-aaaaaa", "ADR-aaaaaa", "EV-aaaaaa", "AT-aaaaaa", "tweaks/nudge"} {
		_, err := mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{
			Title:     "Non-blockable target",
			Statement: "Held by the wrong class?",
			Blocks:    []string{target},
		})
		require.ErrorContains(t, err, "not blockable", "target %s", target)
	}
}

// TestAddQuestionBlocksATaskByItsKey pins the third blockable class at the
// address every keyed noun is addressed by. A task's canonical key is the
// three-segment <epic-key>/<feature-key>/<task-key> (artifact-schema.md, Keys
// and paths), and cli-surface.md's "A node is addressed by id or by key" admits
// it wherever the id is admitted, which is what `link task --depends-on`
// already does with the same string.
func TestAddQuestionBlocksATaskByItsKey(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key:   "form-engine/certification-form-loading/render-the-form",
		Title: "Render the form",
	})
	require.NoError(t, err)

	res, err := mutate.AddQuestion(load(t, root), testDeps(), mutate.QuestionInput{
		Title:     "Form renderer",
		Statement: "Which renderer does the form use?",
		Blocks:    []string{"form-engine/certification-form-loading/render-the-form"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"T-aaaaaa"}, res.Blocks, "the task key resolves to the task's id")
}

// TestAnswerQuestionHandlesHandAuthoredStatuses pins the honest refusals: an
// entry with no status key reads as open everywhere else and stays
// answerable, while a closed non-answered status refuses by naming the actual
// state rather than claiming an answer exists.
func TestAnswerQuestionHandlesHandAuthoredStatuses(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	ledger := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(ledger), 0o755))
	body := "questions:\n" +
		"  - id: Q-nostat\n    statement: No status key at all?\n" +
		"  - id: Q-cancel\n    statement: Cancelled by hand?\n    status: cancelled\n"
	require.NoError(t, os.WriteFile(ledger, []byte(body), 0o644))

	tree := load(t, root)
	res, err := mutate.AnswerQuestion(tree, testDeps(), stubRev(root), mutate.AnswerInput{Node: "Q-nostat", Answer: "Yes."})
	require.NoError(t, err, "an empty status reads as open, matching the blocks guard")
	require.Equal(t, "answered", res.Status)

	tree = load(t, root)
	_, err = mutate.AnswerQuestion(tree, testDeps(), stubRev(root), mutate.AnswerInput{Node: "Q-cancel", Answer: "Yes."})
	var closed *mutate.QuestionClosedError
	require.ErrorAs(t, err, &closed)
	require.Equal(t, "cancelled", closed.Status, "the refusal names the actual state")
}

// TestAnswerRevisionOnADirtyTree pins the provenance rule's other arm: a dirty
// working tree records the sha256 fingerprint, never a commit that does not
// contain the answer.
func TestAnswerRevisionOnADirtyTree(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	tree := load(t, root)
	added, err := mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{Title: "Dirty tree", Statement: "Dirty tree?"})
	require.NoError(t, err)

	tree = load(t, root)
	_, err = mutate.AnswerQuestion(tree, testDeps(), dirtyStubRevisions{}, mutate.AnswerInput{Node: added.ID, Answer: "Yes."})
	require.NoError(t, err)

	tree = load(t, root)
	require.Equal(t, store.KindSHA256, tree.Questions[added.ID].AnswerRevision.Kind)
}

// dirtyStubRevisions is stubRevisions with a dirty working tree.
type dirtyStubRevisions struct{ stubRevisions }

func (dirtyStubRevisions) IsDirty() (bool, error) { return true, nil }

// TestAnswerQuestionIsIdempotentAndGuarded pins re-answer semantics: the same
// answer again is a no-change success, a different answer is refused (the
// recorded answer does not silently change).
func TestAnswerQuestionIsIdempotentAndGuarded(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	tree := load(t, root)
	added, err := mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{Title: "Canonical schema version", Statement: "Which schema version is canonical?"})
	require.NoError(t, err)

	tree = load(t, root)
	_, err = mutate.AnswerQuestion(tree, testDeps(), stubRev(root), mutate.AnswerInput{Node: added.ID, Answer: "The 2025 revision."})
	require.NoError(t, err)

	tree = load(t, root)
	same, err := mutate.AnswerQuestion(tree, testDeps(), stubRev(root), mutate.AnswerInput{Node: added.ID, Answer: "The 2025 revision."})
	require.NoError(t, err)
	require.False(t, same.Changed, "the same answer again is a no-change success")

	tree = load(t, root)
	_, err = mutate.AnswerQuestion(tree, testDeps(), stubRev(root), mutate.AnswerInput{Node: added.ID, Answer: "Actually the 2022 one."})
	var already *mutate.AlreadyAnsweredError
	require.ErrorAs(t, err, &already, "a different answer is a refusal, not a silent overwrite")
	require.Equal(t, added.ID, already.QuestionID)
}

func TestAddQuestionPersistsTitleAndEvent(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)

	res, err := mutate.AddQuestion(load(t, root), testDeps(), mutate.QuestionInput{
		Title:     "Design system location",
		Statement: "Where does the front-end design system live?",
	})
	require.NoError(t, err)
	require.Equal(t, "Design system location", res.Title)

	tree := load(t, root)
	require.Equal(t, "Design system location", tree.Questions[res.ID].Title)
	evs, err := events.List(tree)
	require.NoError(t, err)
	added := eventsOfType(evs, "add.question")
	require.Len(t, added, 1)
	require.Equal(t, "Design system location", added[0].After["title"])
}

func TestAddQuestionRefusesMissingOrMultilineTitleWithoutWrites(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		title string
	}{
		{name: "missing", title: ""},
		{name: "line feed", title: "Schema\nversion"},
		{name: "carriage return", title: "Schema\rversion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeQuestionRepo(t)
			tree := load(t, root)
			before, err := events.List(tree)
			require.NoError(t, err)

			_, err = mutate.AddQuestion(tree, testDeps(), mutate.QuestionInput{
				Title:     tt.title,
				Statement: "Which schema version is canonical?",
			})
			require.Error(t, err)
			var reasoned interface{ Reason() string }
			require.ErrorAs(t, err, &reasoned)
			if tt.title == "" {
				require.Equal(t, "title_required", reasoned.Reason())
			}

			_, statErr := os.Stat(filepath.Join(root, ".tiller", "decisions", "open-questions.yml"))
			require.True(t, os.IsNotExist(statErr), "title validation happens before the ledger write")
			after, listErr := events.List(load(t, root))
			require.NoError(t, listErr)
			require.Len(t, after, len(before), "title validation happens before the event write")
		})
	}
}

func TestAddQuestionBackfillsMissingTitleOnClosedAnsweredDuplicate(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	ledger := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(ledger), 0o755))
	require.NoError(t, os.WriteFile(ledger, []byte(
		"questions:\n"+
			"  - id: Q-legacy\n"+
			"    statement: Which schema version is canonical?\n"+
			"    owner: jc\n"+
			"    status: answered\n"+
			"    answer: The 2025 revision.\n"), 0o644))

	res, err := mutate.AddQuestion(load(t, root), testDeps(), mutate.QuestionInput{
		Title:     "Canonical schema version",
		Statement: "Which schema version is canonical?",
	})
	require.NoError(t, err)
	require.False(t, res.Created)
	require.True(t, res.Changed, "filling an absent title is a real mutation")
	require.Equal(t, "Canonical schema version", res.Title)
	require.Equal(t, "answered", res.Status, "title backfill does not reopen a closed question")
	require.Equal(t, "The 2025 revision.", res.Answer, "title backfill does not change the answer")

	tree := load(t, root)
	q := tree.Questions["Q-legacy"]
	require.Equal(t, "Canonical schema version", q.Title)
	require.Equal(t, "answered", q.Status)
	require.NotNil(t, q.Answer)
	require.Equal(t, "The 2025 revision.", *q.Answer)
	evs, err := events.List(tree)
	require.NoError(t, err)
	added := eventsOfType(evs, "add.question")
	require.Len(t, added, 1)
	require.Equal(t, "Canonical schema version", added[0].After["title"])
}

func TestAddQuestionDoesNotOverwriteExistingTitle(t *testing.T) {
	t.Parallel()
	root := writeQuestionRepo(t)
	first, err := mutate.AddQuestion(load(t, root), testDeps(), mutate.QuestionInput{
		Title:     "Canonical schema version",
		Statement: "Which schema version is canonical?",
	})
	require.NoError(t, err)

	again, err := mutate.AddQuestion(load(t, root), testDeps(), mutate.QuestionInput{
		Title:     "Replacement title",
		Statement: "Which schema version is canonical?",
	})
	require.NoError(t, err)
	require.False(t, again.Changed)
	require.Equal(t, "Canonical schema version", again.Title)

	tree := load(t, root)
	require.Equal(t, "Canonical schema version", tree.Questions[first.ID].Title)
	evs, err := events.List(tree)
	require.NoError(t, err)
	require.Len(t, eventsOfType(evs, "add.question"), 1, "a rejected title replacement records no event")
}

// layOpenQuestionBlockingATask lays the question repo and one open question
// blocking the feature, returning the root and the question id.
func layOpenQuestionBlockingATask(t *testing.T) (string, string) {
	t.Helper()
	root := writeQuestionRepo(t)
	res, err := mutate.AddQuestion(load(t, root), testDeps(), mutate.QuestionInput{
		Title: "Which store", Statement: "Which store holds the forms?",
		Blocks: []string{"form-engine/certification-form-loading"},
	})
	require.NoError(t, err)
	return root, res.ID
}

func ledgerBytes(t *testing.T, root string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".tiller", "decisions", "open-questions.yml"))
	require.NoError(t, err)
	return data
}

func TestCancelQuestionOpen(t *testing.T) {
	t.Parallel()
	root, qid := layOpenQuestionBlockingATask(t)

	res, err := mutate.CancelQuestion(load(t, root), testDeps(), mutate.CancelInput{Node: qid, Reason: "The decision moved to the vendor."})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "cancelled", res.Status)
	require.Equal(t, "The decision moved to the vendor.", res.CancelReason)

	tree := load(t, root)
	q := tree.Questions[qid]
	require.Equal(t, "cancelled", q.Status)
	require.Equal(t, "The decision moved to the vendor.", q.CancelReason)
	require.Nil(t, q.Answer)
	require.Equal(t, store.RevisionRef{}, q.AnswerRevision)
	require.Equal(t, []string{"F-bbbbbb"}, q.Blocks, "a cancel keeps the blocks list on record")

	evs, err := events.List(tree)
	require.NoError(t, err)
	var cancels []events.Event
	for _, ev := range evs {
		if ev.Type == "cancel.question" {
			cancels = append(cancels, ev)
		}
	}
	require.Len(t, cancels, 1)
	require.Equal(t, "cancelled", cancels[0].After["status"])
	require.Equal(t, "The decision moved to the vendor.", cancels[0].After["cancel_reason"])
}

func TestCancelQuestionAlreadyCancelled(t *testing.T) {
	t.Parallel()
	root, qid := layOpenQuestionBlockingATask(t)
	_, err := mutate.CancelQuestion(load(t, root), testDeps(), mutate.CancelInput{Node: qid, Reason: "first reason"})
	require.NoError(t, err)
	before := ledgerBytes(t, root)

	res, err := mutate.CancelQuestion(load(t, root), testDeps(), mutate.CancelInput{Node: qid, Reason: "a different reason"})
	require.NoError(t, err)
	require.False(t, res.Changed)
	require.Equal(t, "cancelled", res.Status)
	require.Equal(t, "first reason", res.CancelReason)
	require.Equal(t, before, ledgerBytes(t, root))

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	n := 0
	for _, ev := range evs {
		if ev.Type == "cancel.question" {
			n++
		}
	}
	require.Equal(t, 1, n, "a no-change call records no event")
}

func TestCancelQuestionAnsweredRefuses(t *testing.T) {
	t.Parallel()
	root, qid := layOpenQuestionBlockingATask(t)
	_, err := mutate.AnswerQuestion(load(t, root), testDeps(), stubRev(root), mutate.AnswerInput{Node: qid, Answer: "Postgres."})
	require.NoError(t, err)
	before := ledgerBytes(t, root)

	_, err = mutate.CancelQuestion(load(t, root), testDeps(), mutate.CancelInput{Node: qid, Reason: "moot"})
	var closed *mutate.QuestionClosedError
	require.ErrorAs(t, err, &closed)
	require.Equal(t, "cancel", closed.Verb)
	require.Equal(t, "answered", closed.Status)
	require.Contains(t, closed.Error(), "only an open question can be cancelled")
	require.Equal(t, before, ledgerBytes(t, root))
}

func TestCancelQuestionBlankReasonRefuses(t *testing.T) {
	t.Parallel()
	root, qid := layOpenQuestionBlockingATask(t)
	before := ledgerBytes(t, root)

	_, err := mutate.CancelQuestion(load(t, root), testDeps(), mutate.CancelInput{Node: qid, Reason: "   "})
	require.Error(t, err)
	require.Contains(t, err.Error(), "reason")
	require.Equal(t, before, ledgerBytes(t, root))
}

func TestCancelQuestionNamesAssumers(t *testing.T) {
	t.Parallel()
	root, qid := layOpenQuestionBlockingATask(t)
	tree := load(t, root)
	basis := store.AssumptionBasis{qid: {Reason: "proceeding"}}
	tree.Features["F-zzzzzz"] = &store.Feature{ID: "F-zzzzzz", AssumptionBasis: basis}
	tree.Features["F-bbbbbb"].AssumptionBasis = basis

	res, err := mutate.CancelQuestion(tree, testDeps(), mutate.CancelInput{Node: qid, Reason: "moot"})
	require.NoError(t, err)
	require.Equal(t, []string{"F-bbbbbb", "F-zzzzzz"}, res.AssumedBy)
}

func TestAnswerCancelledQuestionRefuses(t *testing.T) {
	t.Parallel()
	root, qid := layOpenQuestionBlockingATask(t)
	_, err := mutate.CancelQuestion(load(t, root), testDeps(), mutate.CancelInput{Node: qid, Reason: "moot"})
	require.NoError(t, err)

	_, err = mutate.AnswerQuestion(load(t, root), testDeps(), stubRev(root), mutate.AnswerInput{Node: qid, Answer: "Postgres."})
	var closed *mutate.QuestionClosedError
	require.ErrorAs(t, err, &closed)
	require.Equal(t, "cancelled", closed.Status)
	require.Contains(t, closed.Error(), "only an open question can be answered")
}

func TestAddQuestionReasksCancelled(t *testing.T) {
	t.Parallel()
	root, qid := layOpenQuestionBlockingATask(t)
	_, err := mutate.CancelQuestion(load(t, root), testDeps(), mutate.CancelInput{Node: qid, Reason: "moot"})
	require.NoError(t, err)

	deps := testDeps()
	deps.Mint = func(prefix string) string { return prefix + "-cccccc" }
	res, err := mutate.AddQuestion(load(t, root), deps, mutate.QuestionInput{
		Title: "Which store", Statement: "Which store holds the forms?",
	})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.NotEqual(t, qid, res.ID)
	require.Equal(t, qid, res.Reasks)
	require.Equal(t, "cancelled", load(t, root).Questions[qid].Status)
}

func TestAddQuestionAnsweredStillDedupes(t *testing.T) {
	t.Parallel()
	root, qid := layOpenQuestionBlockingATask(t)
	_, err := mutate.AnswerQuestion(load(t, root), testDeps(), stubRev(root), mutate.AnswerInput{Node: qid, Answer: "Postgres."})
	require.NoError(t, err)

	res, err := mutate.AddQuestion(load(t, root), testDeps(), mutate.QuestionInput{
		Title: "Which store", Statement: "Which store holds the forms?",
	})
	require.NoError(t, err)
	require.False(t, res.Created)
	require.Equal(t, qid, res.ID)
	require.Equal(t, "Postgres.", res.Answer)
	require.Empty(t, res.Reasks)
}

// answerOnDisk answers the question through a separate tree load, so a tree
// loaded earlier is stale against the ledger.
func answerOnDisk(t *testing.T, root, qid string) {
	t.Helper()
	_, err := mutate.AnswerQuestion(load(t, root), testDeps(), stubRev(root), mutate.AnswerInput{Node: qid, Answer: "Postgres."})
	require.NoError(t, err)
}

func TestCancelQuestionStaleTreeRefusesAnswered(t *testing.T) {
	t.Parallel()
	root, qid := layOpenQuestionBlockingATask(t)
	stale := load(t, root)
	answerOnDisk(t, root, qid)
	before := ledgerBytes(t, root)

	_, err := mutate.CancelQuestion(stale, testDeps(), mutate.CancelInput{Node: qid, Reason: "moot"})
	var closed *mutate.QuestionClosedError
	require.ErrorAs(t, err, &closed)
	require.Equal(t, "cancel", closed.Verb)
	require.Equal(t, "answered", closed.Status)
	require.Equal(t, before, ledgerBytes(t, root))
}

func TestAnswerQuestionStaleTreeRefusesCancelled(t *testing.T) {
	t.Parallel()
	root, qid := layOpenQuestionBlockingATask(t)
	stale := load(t, root)
	_, err := mutate.CancelQuestion(load(t, root), testDeps(), mutate.CancelInput{Node: qid, Reason: "moot"})
	require.NoError(t, err)
	before := ledgerBytes(t, root)

	_, err = mutate.AnswerQuestion(stale, testDeps(), stubRev(root), mutate.AnswerInput{Node: qid, Answer: "Postgres."})
	var closed *mutate.QuestionClosedError
	require.ErrorAs(t, err, &closed)
	require.Equal(t, "cancelled", closed.Status)
	require.Equal(t, before, ledgerBytes(t, root))
}
