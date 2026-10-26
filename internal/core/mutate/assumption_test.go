package mutate_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin the assumption-review write path:
// ReviewAssumption is the dedicated `approve --reviewed-assumption <Q-id>`
// clearing act, and UnreviewedAssumptions is the one predicate Task 6's
// submit guard and Task 7's drift detector both call.

// layFeatureWithOpenAssumedQuestion lays a one-epic-one-feature tree, walks
// the feature through spec and approve to ready, then starts it with
// --assume-question against one open question (Q-aaaaaa), the exact write
// transition.go's guardFeature performs for `start --assume-question`. The
// question is left open, the shape ReviewAssumption's question_not_answered
// refusal and the predicate's "an open question is never listed" case both
// need. Built from layEpicFeature/writeStage2Output/driveTo
// (transition_test.go) plus AddQuestion (question_test.go's own machinery),
// the existing fixtures.
func layFeatureWithOpenAssumedQuestion(t *testing.T) string {
	t.Helper()
	root := layEpicFeature(t)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	_, err := mutate.AddQuestion(load(t, root), testDeps(), mutate.QuestionInput{
		Title:     "Canonical schema version",
		Statement: "Which schema version is canonical?",
	})
	require.NoError(t, err)

	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{
		Verb: "approve", AssumeQuestion: "Q-aaaaaa", Reason: "proceeding under the documented assumption",
	})
	return root
}

// layFeatureWithAssumedQuestion is layFeatureWithOpenAssumedQuestion plus the
// answer: the feature carries assumption_basis[Q-aaaaaa] and Q-aaaaaa is
// answered, so the entry's stamp is immediately stale, the shape
// ReviewAssumption and UnreviewedAssumptions both exist to close.
func layFeatureWithAssumedQuestion(t *testing.T) string {
	t.Helper()
	root := layFeatureWithOpenAssumedQuestion(t)
	_, err := mutate.AnswerQuestion(load(t, root), testDeps(), stubRev(root), mutate.AnswerInput{
		Node: "Q-aaaaaa", Answer: "The 2025 revision; 2022 is read-only reference.",
	})
	require.NoError(t, err)
	return root
}

// layInProgressTaskUnderAssumedFeature is layFeatureWithAssumedQuestion plus
// driving T-aaaaaa (the one task writeStage2Output lays under the fixture
// feature) from its plan-gate-approved ready state to check, the state
// submit is legal from (lifecycle/table.go: {From: "check", Verb: "submit"}
// is the task's only row naming submit). Two real verbs, start then check,
// carry it there; check here is the plain lifecycle move (mutate.Transition
// with Verb "check"), not a gate run through mutate.Check, matching
// transition_test.go's own layCheckedTask convention. Task 6's submit guard
// is what this fixture exists to exercise, so the feature's stale
// assumption_basis[Q-aaaaaa] rides along untouched.
func layInProgressTaskUnderAssumedFeature(t *testing.T) string {
	t.Helper()
	root := layFeatureWithAssumedQuestion(t)
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "check"})
	return root
}

// TestReviewAssumptionStampsTheAnswerRevision is the happy-path
// sketch verbatim: reviewing in stamps exactly the question's current answer
// revision, and never erases the human's recorded reason for proceeding.
func TestReviewAssumptionStampsTheAnswerRevision(t *testing.T) {
	t.Parallel()
	root := layFeatureWithAssumedQuestion(t)
	tree := load(t, root)

	res, err := mutate.ReviewAssumption(tree, testDeps(), tree.Features["F-aaaaaa"], "Q-aaaaaa")
	require.NoError(t, err)
	require.True(t, res.Changed)

	reloaded := load(t, root)
	q := reloaded.Questions["Q-aaaaaa"]
	entry := reloaded.Features["F-aaaaaa"].AssumptionBasis["Q-aaaaaa"]
	require.Equal(t, q.AnswerRevision, entry.AnswerRevision,
		"the stamp records exactly what was reviewed, so a later re-answer reads as fresh drift")
	require.NotEmpty(t, entry.Reason, "reviewing in never erases why the human proceeded")

	// One event recorded for the real move.
	evs, err := events.List(tree)
	require.NoError(t, err)
	require.NotEmpty(t, evs)
}

// TestReviewAssumptionRefusals covers the three named outcomes other than a
// real stamp: no entry for the question at all, the question still open, and
// the idempotent no-op when the stamp already matches.
func TestReviewAssumptionRefusals(t *testing.T) {
	t.Parallel()
	t.Run("no entry for the named question", func(t *testing.T) {
		root := layFeatureWithAssumedQuestion(t)
		tree := load(t, root)

		_, err := mutate.ReviewAssumption(tree, testDeps(), tree.Features["F-aaaaaa"], "Q-zzzzzz")
		require.Error(t, err)
		var reasoner interface{ Reason() string }
		require.ErrorAs(t, err, &reasoner)
		require.Equal(t, "unknown_assumption", reasoner.Reason())
		var coder store.ExitCoder
		require.ErrorAs(t, err, &coder)
		require.Equal(t, 2, coder.ExitCode())
	})

	t.Run("the question is still open", func(t *testing.T) {
		root := layFeatureWithOpenAssumedQuestion(t)
		tree := load(t, root)

		_, err := mutate.ReviewAssumption(tree, testDeps(), tree.Features["F-aaaaaa"], "Q-aaaaaa")
		require.Error(t, err)
		var reasoner interface{ Reason() string }
		require.ErrorAs(t, err, &reasoner)
		require.Equal(t, "question_not_answered", reasoner.Reason())

		unchanged := load(t, root)
		require.Empty(t, unchanged.Features["F-aaaaaa"].AssumptionBasis["Q-aaaaaa"].AnswerRevision.Value,
			"a refused review stamps nothing")
	})

	t.Run("the question was cancelled", func(t *testing.T) {
		root := layFeatureWithOpenAssumedQuestion(t)
		_, err := mutate.CancelQuestion(load(t, root), testDeps(), mutate.CancelInput{Node: "Q-aaaaaa", Reason: "moot"})
		require.NoError(t, err)
		tree := load(t, root)

		_, err = mutate.ReviewAssumption(tree, testDeps(), tree.Features["F-aaaaaa"], "Q-aaaaaa")
		require.Error(t, err)
		require.Contains(t, err.Error(), "was cancelled")
		require.NotContains(t, err.Error(), "still open")
	})

	t.Run("already stamped with the current revision", func(t *testing.T) {
		root := layFeatureWithAssumedQuestion(t)
		tree := load(t, root)
		_, err := mutate.ReviewAssumption(tree, testDeps(), tree.Features["F-aaaaaa"], "Q-aaaaaa")
		require.NoError(t, err)

		tree = load(t, root)
		before, err := events.List(tree)
		require.NoError(t, err)

		res, err := mutate.ReviewAssumption(tree, testDeps(), tree.Features["F-aaaaaa"], "Q-aaaaaa")
		require.NoError(t, err, "a matching stamp is a clean no-op, not a refusal")
		require.False(t, res.Changed)

		after, err := events.List(load(t, root))
		require.NoError(t, err)
		require.Len(t, after, len(before), "the house idempotency idiom: no event on a no-change retry")
	})
}

// TestReviewAssumptionRefusesAZeroRevisionOnAHandAuthoredAnswer pins a review
// Minor finding: a hand-authored ledger entry can carry `status:
// answered` (questionAnswered reads that alone as answered) with no
// answer_revision at all, predating the field or typed by hand the same way
// question_test.go's own TestAnswerQuestionHandlesHandAuthoredStatuses
// exercises a no-status-key entry. Without this check the zero
// AnswerRevision on both sides reads SameAs-equal and ReviewAssumption
// reports "already reviewed in at the current answer revision (no change)"
// for a question that was never given a revision to stamp, an actively
// false claim.
func TestReviewAssumptionRefusesAZeroRevisionOnAHandAuthoredAnswer(t *testing.T) {
	t.Parallel()
	root := layFeatureWithOpenAssumedQuestion(t)
	ledger := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
	body := "questions:\n" +
		"  - id: Q-aaaaaa\n" +
		"    statement: Which schema version is canonical?\n" +
		"    status: answered\n" +
		"    answer: Hand-authored, no revision recorded.\n"
	require.NoError(t, os.WriteFile(ledger, []byte(body), 0o644))

	tree := load(t, root)
	_, err := mutate.ReviewAssumption(tree, testDeps(), tree.Features["F-aaaaaa"], "Q-aaaaaa")
	require.Error(t, err)
	var reasoner interface{ Reason() string }
	require.ErrorAs(t, err, &reasoner)
	require.Equal(t, "question_not_answered", reasoner.Reason())

	unchanged := load(t, root)
	require.Empty(t, unchanged.Features["F-aaaaaa"].AssumptionBasis["Q-aaaaaa"].AnswerRevision.Value,
		"a refused review stamps nothing")
}

// TestReviewAssumptionUpdatesTheInMemoryTree pins the Minor 4: a
// caller holding one *store.Tree across both calls (Task 6's guard, Task 7's
// detector, potentially in one process without an intervening store.Load)
// must see the reviewed state immediately, not only after a fresh Load.
func TestReviewAssumptionUpdatesTheInMemoryTree(t *testing.T) {
	t.Parallel()
	root := layFeatureWithAssumedQuestion(t)
	tree := load(t, root)

	before := mutate.UnreviewedAssumptions(tree, tree.Features["F-aaaaaa"])
	require.Len(t, before, 1, "unreviewed before the call")

	_, err := mutate.ReviewAssumption(tree, testDeps(), tree.Features["F-aaaaaa"], "Q-aaaaaa")
	require.NoError(t, err)

	// Same tree pointer, no reload.
	after := mutate.UnreviewedAssumptions(tree, tree.Features["F-aaaaaa"])
	require.Empty(t, after, "the in-memory feature must agree with disk immediately, with no fresh Load")
}

// TestUnreviewedAssumptionsPredicate covers the four named
// cases: the whole rule is !entry.AnswerRevision.SameAs(question.
// AnswerRevision), which the zero value and a genuine re-answer both trip,
// and an open question never trips since its AnswerRevision stays the zero
// value too. Two more subtests pin the reason SameAs exists at all (a review
// Critical finding): plain struct equality on a RevisionRef compares
// ObservedAt's *Location pointer, not just the instant, so a ledger round
// trip that changes only how a zero UTC offset is spelled must not read as a
// content change, while an actual content change (a different Value) must.
func TestUnreviewedAssumptionsPredicate(t *testing.T) {
	t.Parallel()
	t.Run("never reviewed against an answered question is unreviewed", func(t *testing.T) {
		root := layFeatureWithAssumedQuestion(t)
		tree := load(t, root)

		got := mutate.UnreviewedAssumptions(tree, tree.Features["F-aaaaaa"])
		require.Len(t, got, 1)
		require.Equal(t, "Q-aaaaaa", got[0].QuestionID)
		require.Equal(t, "F-aaaaaa", got[0].NodeID)
		require.Equal(t, store.NounFeature, got[0].Noun)
		require.NotEmpty(t, got[0].Statement)
	})

	t.Run("an open question is never listed", func(t *testing.T) {
		root := layFeatureWithOpenAssumedQuestion(t)
		tree := load(t, root)

		got := mutate.UnreviewedAssumptions(tree, tree.Features["F-aaaaaa"])
		require.Empty(t, got, "an open question is a blocker, not drift")
	})

	t.Run("a stamped entry matching the current answer is clean", func(t *testing.T) {
		root := layFeatureWithAssumedQuestion(t)
		tree := load(t, root)
		_, err := mutate.ReviewAssumption(tree, testDeps(), tree.Features["F-aaaaaa"], "Q-aaaaaa")
		require.NoError(t, err)

		got := mutate.UnreviewedAssumptions(load(t, root), load(t, root).Features["F-aaaaaa"])
		require.Empty(t, got)
	})

	t.Run("re-answered after review is unreviewed again", func(t *testing.T) {
		root := layFeatureWithAssumedQuestion(t)
		tree := load(t, root)
		_, err := mutate.ReviewAssumption(tree, testDeps(), tree.Features["F-aaaaaa"], "Q-aaaaaa")
		require.NoError(t, err)

		// No writer re-answers a question with a fresh revision yet (that is
		// left for later); this mutates the loaded tree directly to simulate
		// the world moving on again, so what is under test is the predicate's
		// own comparison, not a claimed write path nothing in the binary
		// produces today.
		tree = load(t, root)
		tree.Questions["Q-aaaaaa"].AnswerRevision = store.RevisionRef{Kind: store.KindGitCommit, Value: "git:deadbeef"}

		got := mutate.UnreviewedAssumptions(tree, tree.Features["F-aaaaaa"])
		require.Len(t, got, 1)
		require.Equal(t, "Q-aaaaaa", got[0].QuestionID)
	})

	// The two subtests below reproduce the Critical finding directly:
	// two RevisionRefs naming the identical instant, spelled through two
	// different zone forms, parse to distinct *time.Location values. Real
	// callers reach this through a ledger round trip ("+00:00" parses to a
	// fixed zone, gets re-marshaled as "Z" because Go's own RFC3339
	// formatting renders any zero offset as "Z" regardless of the source
	// zone's identity, and reloading "Z" gives literal time.UTC); these
	// subtests construct it directly with time.Parse rather than drive a
	// disk round trip, so what is under test is the comparison itself.
	t.Run("ObservedAt zone spelling differs but the instant does not is not unreviewed", func(t *testing.T) {
		root := layFeatureWithAssumedQuestion(t)
		tree := load(t, root)

		zForm, err := time.Parse(time.RFC3339, "2026-06-23T00:00:00Z")
		require.NoError(t, err)
		offsetForm, err := time.Parse(time.RFC3339, "2026-06-23T00:00:00+00:00")
		require.NoError(t, err)
		require.True(t, zForm.Equal(offsetForm), "sanity: the two forms name the same instant")
		require.NotEqual(t, zForm, offsetForm,
			"sanity: this is the whole bug, struct-unequal despite being the same instant (distinct *Location)")

		entry := tree.Features["F-aaaaaa"].AssumptionBasis["Q-aaaaaa"]
		entry.AnswerRevision = store.RevisionRef{Kind: store.KindGitCommit, Value: "git:abc1234", ObservedAt: offsetForm}
		tree.Features["F-aaaaaa"].AssumptionBasis["Q-aaaaaa"] = entry
		tree.Questions["Q-aaaaaa"].AnswerRevision = store.RevisionRef{Kind: store.KindGitCommit, Value: "git:abc1234", ObservedAt: zForm}

		got := mutate.UnreviewedAssumptions(tree, tree.Features["F-aaaaaa"])
		require.Empty(t, got, "same instant, different zone spelling: SameAs must read this as reviewed")
	})

	t.Run("Value alone differing is unreviewed", func(t *testing.T) {
		root := layFeatureWithAssumedQuestion(t)
		tree := load(t, root)

		at := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
		entry := tree.Features["F-aaaaaa"].AssumptionBasis["Q-aaaaaa"]
		entry.AnswerRevision = store.RevisionRef{Kind: store.KindGitCommit, Value: "git:abc1234", ObservedAt: at}
		tree.Features["F-aaaaaa"].AssumptionBasis["Q-aaaaaa"] = entry
		tree.Questions["Q-aaaaaa"].AnswerRevision = store.RevisionRef{Kind: store.KindGitCommit, Value: "git:def5678", ObservedAt: at}

		got := mutate.UnreviewedAssumptions(tree, tree.Features["F-aaaaaa"])
		require.Len(t, got, 1, "same ObservedAt, different Value: a real content change must still be unreviewed")
		require.Equal(t, "Q-aaaaaa", got[0].QuestionID)
	})
}

// TestUnreviewedAssumptionsWalksTaskUpToParentFeature pins the fact settled
// at plan time: assumption_basis lives on Feature alone, so calling the
// predicate with a task walks up to its parent feature and reports the
// feature as the carrying node, never the task. Task 6's remedy message
// depends on this: a reader handed a task command for a feature's entry gets
// a refusal loop.
func TestUnreviewedAssumptionsWalksTaskUpToParentFeature(t *testing.T) {
	t.Parallel()
	root := layFeatureWithAssumedQuestion(t)
	tree := load(t, root)
	task := tree.Tasks["T-aaaaaa"]
	require.NotNil(t, task, "writeStage2Output lays exactly one task under the fixture feature")

	got := mutate.UnreviewedAssumptions(tree, task)
	require.Len(t, got, 1)
	require.Equal(t, "F-aaaaaa", got[0].NodeID, "the walk-up reports the carrying feature, not the task")
	require.Equal(t, store.NounFeature, got[0].Noun)
	require.Equal(t, mutate.UnreviewedAssumptions(tree, tree.Features["F-aaaaaa"]), got,
		"a task's own view agrees exactly with its parent feature's")
}

// TestUnreviewedAssumptionsIgnoresNodesWithNoBasis pins the other half of the
// walk-up: an epic and a question carry no assumption_basis at all, so the
// predicate returns nothing for either rather than panicking on a node type
// it was never told how to read.
func TestUnreviewedAssumptionsIgnoresNodesWithNoBasis(t *testing.T) {
	t.Parallel()
	root := layFeatureWithAssumedQuestion(t)
	tree := load(t, root)

	require.Empty(t, mutate.UnreviewedAssumptions(tree, tree.Epics["E-aaaaaa"]))
	require.Empty(t, mutate.UnreviewedAssumptions(tree, tree.Questions["Q-aaaaaa"]))
}

// TestUnreviewedAssumptionsSkipsADanglingQuestionReference pins the drift
// engine's own contract: a hand-edited assumption_basis
// entry naming a question the ledger no longer carries is Uncovered there,
// never silently reported as unreviewed drift by this predicate, and never a
// crash either.
func TestUnreviewedAssumptionsSkipsADanglingQuestionReference(t *testing.T) {
	t.Parallel()
	root := layFeatureWithAssumedQuestion(t)
	tree := load(t, root)
	tree.Features["F-aaaaaa"].AssumptionBasis["Q-ghost"] = store.Assumption{Reason: "hand-edited"}

	got := mutate.UnreviewedAssumptions(tree, tree.Features["F-aaaaaa"])
	var ids []string
	for _, u := range got {
		ids = append(ids, u.QuestionID)
	}
	require.NotContains(t, ids, "Q-ghost")
	require.Contains(t, ids, "Q-aaaaaa", "the real entry is still reported alongside the dangling one")
}

func TestCancelledAssumptionRaisesNoDrift(t *testing.T) {
	t.Parallel()
	root := layFeatureWithOpenAssumedQuestion(t)
	_, err := mutate.CancelQuestion(load(t, root), testDeps(), mutate.CancelInput{Node: "Q-aaaaaa", Reason: "no longer needs a decision"})
	require.NoError(t, err)

	tree := load(t, root)
	require.Empty(t, mutate.UnreviewedAssumptions(tree, tree.Features["F-aaaaaa"]))
}
