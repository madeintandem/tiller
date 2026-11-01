package mutate_test

import (
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func setBugStatus(t *testing.T, root, key, status string) {
	t.Helper()
	p := filepath.Join(root, ".tiller", "bugs", key, "bug.yml")
	var b store.Bug
	readYAMLTest(t, p, &b)
	b.Status = status
	b.Started = true
	b.Owner = "jc"
	writeYAMLTest(t, p, b)
}

func TestGradeSetsSeverityAndRecordsEvent(t *testing.T) {
	t.Parallel()
	root := layBugGraded(t, "blank-on-reload", "minor")
	res, err := mutate.Grade(load(t, root), testDeps(), mutate.GradeInput{Noun: store.NounBug, Node: "blank-on-reload", Severity: "important"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "B-aaaaaa", res.ID)
	require.Equal(t, "bugs/blank-on-reload", res.Key)
	require.Equal(t, "minor", res.OldSeverity)
	require.Equal(t, "important", res.Severity)
	require.Equal(t, "important", load(t, root).Bugs["B-aaaaaa"].Severity)

	ev := lastEvent(t, root, "grade.bug")
	require.Equal(t, "grade bug B-aaaaaa important", ev.Command)
	require.Equal(t, map[string]any{"severity": "minor"}, ev.Before)
	require.Equal(t, map[string]any{"severity": "important"}, ev.After)
}

func TestGradeGradesAnUnratedBug(t *testing.T) {
	t.Parallel()
	root := layBugGraded(t, "blank-on-reload", "")
	res, err := mutate.Grade(load(t, root), testDeps(), mutate.GradeInput{Noun: store.NounBug, Node: "B-aaaaaa", Severity: "blocker"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "", res.OldSeverity)
	require.Equal(t, "blocker", load(t, root).Bugs["B-aaaaaa"].Severity)
}

func TestGradeSameLevelIsNoop(t *testing.T) {
	t.Parallel()
	root := layBugGraded(t, "blank-on-reload", "medium")
	before, err := events.List(load(t, root))
	require.NoError(t, err)
	res, err := mutate.Grade(load(t, root), testDeps(), mutate.GradeInput{Noun: store.NounBug, Node: "blank-on-reload", Severity: "medium"})
	require.NoError(t, err)
	require.False(t, res.Changed)
	after, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, after, len(before))
}

func TestGradeRefusesTerminalBug(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"done", "cancelled"} {
		root := layBugGraded(t, "blank-on-reload", "medium")
		setBugStatus(t, root, "blank-on-reload", status)
		_, err := mutate.Grade(load(t, root), testDeps(), mutate.GradeInput{Noun: store.NounBug, Node: "blank-on-reload", Severity: "minor"})
		require.Error(t, err)
		require.Equal(t, "terminal_grade", reasonOf(t, err))
		require.Contains(t, err.Error(), "is "+status)
		require.Equal(t, "medium", load(t, root).Bugs["B-aaaaaa"].Severity)
	}
}

func TestGradeRefusesInvalidLevel(t *testing.T) {
	t.Parallel()
	root := layBug(t, "blank-on-reload")
	for _, level := range []string{"", "urgent", "Blocker"} {
		_, err := mutate.Grade(load(t, root), testDeps(), mutate.GradeInput{Noun: store.NounBug, Node: "blank-on-reload", Severity: level})
		require.Error(t, err, level)
		require.Equal(t, "invalid_severity", reasonOf(t, err))
	}
}

func TestGradeRefusesNonBugNoun(t *testing.T) {
	t.Parallel()
	root := layBug(t, "blank-on-reload")
	_, err := mutate.Grade(load(t, root), testDeps(), mutate.GradeInput{Noun: store.NounChore, Node: "x", Severity: "minor"})
	require.Error(t, err)
	require.Equal(t, "severity_not_bug", reasonOf(t, err))
}

func TestGradeAllowsInReview(t *testing.T) {
	t.Parallel()
	root := layBug(t, "blank-on-reload")
	setBugStatus(t, root, "blank-on-reload", "in_review")
	res, err := mutate.Grade(load(t, root), testDeps(), mutate.GradeInput{Noun: store.NounBug, Node: "blank-on-reload", Severity: "blocker"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "blocker", load(t, root).Bugs["B-aaaaaa"].Severity)
}
