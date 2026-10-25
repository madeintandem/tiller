package views_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// A deadline row says when the question is due, then the question itself, so
// the line names what it asks and not only its date.
func TestRankDeadlinesOutrankLocality(t *testing.T) {
	tree := readyEpics(t, 2)
	f, near := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	far, _ := readyFeatureWithTask(t, tree, "F-aaaaa1", "epic-1/work", "epic-1")
	holdClaimableNode(tree, "Q-near00", near.ID)
	for _, date := range []string{"2026-10-30", "2026-10-23", "2026-10-22", "2026-10-21", "2026-10-19"} {
		datedQuestion(tree, "Q-"+date, far.ID, store.QuestionDue{Date: date})
	}

	next := nextFor(tree, "jc", workflow.Input{}, locateEvent("a", "spec.feature", "jc", 1, f.ID))
	want := []struct{ id, reason string }{
		{"Q-2026-10-19", "overdue (was due 2026-10-19); Which schema?"},
		{"Q-2026-10-21", "due today; Which schema?"},
		{"Q-2026-10-22", "due tomorrow; Which schema?"},
		{"Q-2026-10-23", "due 2026-10-23; Which schema?"},
		{"Q-near00", fmt.Sprintf("holds %s in your current feature; Which schema?", gen.Ref(tree, near))},
	}
	for i, w := range want {
		row := next.Candidates[i]
		require.Equal(t, w.id, row.Node.ID, "row %d", i)
		require.Equal(t, 1, row.Tier, w.id)
		require.Equal(t, w.reason, row.Reason, w.id)
	}
	late, _ := rowFor(t, next, "Q-2026-10-30")
	require.Equal(t, 5, late.Tier, "a deadline more than two days out gets no boost")

	unread := nextFor(tree, "", workflow.Input{})
	require.Equal(t, "Q-2026-10-19", unread.Candidates[0].Node.ID, "deadlines rank without identity (plan ruling 5)")
	require.Equal(t, 1, unread.Candidates[0].Tier)
}

func TestRankIgnoresHintAndMalformedDue(t *testing.T) {
	tree := readyEpics(t, 2)
	f, near := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	far, _ := readyFeatureWithTask(t, tree, "F-aaaaa1", "epic-1/work", "epic-1")
	datedQuestion(tree, "Q-hint00", far.ID, store.QuestionDue{Hint: "before launch"})
	datedQuestion(tree, "Q-junk00", far.ID, store.QuestionDue{Date: "next week"})
	datedQuestion(tree, "Q-near00", near.ID, store.QuestionDue{Hint: "before launch"})

	next := nextFor(tree, "jc", workflow.Input{}, locateEvent("a", "spec.feature", "jc", 1, f.ID))
	for _, id := range []string{"Q-hint00", "Q-junk00"} {
		row, _ := rowFor(t, next, id)
		require.Equal(t, 5, row.Tier, id)
		require.NotContains(t, row.Reason, "due", id)
	}
	nearRow, _ := rowFor(t, next, "Q-near00")
	require.Equal(t, 1, nearRow.Tier)
	require.Contains(t, nearRow.Reason, "your current feature")
}

func TestRankLateDatedHoldRanksAfterDeadlines(t *testing.T) {
	tree := readyEpics(t, 2)
	f, near := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	far, _ := readyFeatureWithTask(t, tree, "F-aaaaa1", "epic-1/work", "epic-1")
	datedQuestion(tree, "Q-near00", near.ID, store.QuestionDue{Date: "2026-10-30"})
	datedQuestion(tree, "Q-soon00", far.ID, store.QuestionDue{Date: "2026-10-22"})

	next := nextFor(tree, "jc", workflow.Input{}, locateEvent("a", "spec.feature", "jc", 1, f.ID))
	require.Equal(t, "Q-soon00", next.Candidates[0].Node.ID)
	require.Equal(t, "Q-near00", next.Candidates[1].Node.ID)
	require.Equal(t, 1, next.Candidates[1].Tier, "a late deadline still holds the current feature")
	require.Equal(t, fmt.Sprintf("holds %s in your current feature; Which schema?", gen.Ref(tree, near)), next.Candidates[1].Reason)
}

// A due date is a zoneless calendar date read in the reader's own day: just
// before midnight in Chicago, a question due that local date is due today,
// though UTC has already moved on to the next day.
func TestRankDeadlineTodayIsTheReadersLocalDate(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)
	tree := readyEpics(t, 1)
	_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	datedQuestion(tree, "Q-today0", task.ID, store.QuestionDue{Date: "2026-10-21"})
	datedQuestion(tree, "Q-tomor0", task.ID, store.QuestionDue{Date: "2026-10-22"})

	now := time.Date(2026, 10, 21, 23, 30, 0, 0, chicago)
	require.Equal(t, 22, now.UTC().Day(), "the fixture straddles UTC midnight")
	next := views.NextOf(tree, views.NextInput{Reader: "jc", Now: now})
	today, _ := rowFor(t, next, "Q-today0")
	require.Equal(t, "due today; Which schema?", today.Reason)
	tomorrow, _ := rowFor(t, next, "Q-tomor0")
	require.Equal(t, "due tomorrow; Which schema?", tomorrow.Reason)
}
