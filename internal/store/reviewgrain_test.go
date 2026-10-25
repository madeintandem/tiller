package store

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolvedReviewGrainMostSpecificWins(t *testing.T) {
	m := &Manifest{}
	e := &Epic{}
	f := &Feature{}
	require.Equal(t, "task", ResolvedReviewGrain(m, e, f), "everything absent defaults to task")
	m.Execution.ReviewGrain = "feature"
	require.Equal(t, "feature", ResolvedReviewGrain(m, e, f), "project default applies")
	e.ReviewGrain = "epic"
	require.Equal(t, "epic", ResolvedReviewGrain(m, e, f), "epic beats project")
	f.ReviewGrain = "task"
	require.Equal(t, "task", ResolvedReviewGrain(m, e, f), "feature beats epic, in either direction")
	require.Equal(t, "epic", ResolvedReviewGrain(m, e, nil), "nil feature resolves at epic")
	require.Equal(t, "feature", ResolvedReviewGrain(m, nil, nil), "nil epic resolves at project")
}

// TestResolvedReviewGrainWithSourceNamesTheWinningHome pins the one resolver
// both ResolvedReviewGrain and the console's node header payload
// (views.reviewGrainOf) consume (Fix round 1, Important 1): the grain half
// must always agree with ResolvedReviewGrain's own answer, and source must
// name whichever of the three homes (docs/plans/2026-08-26-review-grain-
// design.md, "The setting") actually produced it, or "default" when none of
// the three is set.
func TestResolvedReviewGrainWithSourceNamesTheWinningHome(t *testing.T) {
	cases := []struct {
		name       string
		m          *Manifest
		e          *Epic
		f          *Feature
		wantGrain  string
		wantSource string
	}{
		{
			name: "everything absent defaults to task", m: &Manifest{}, e: &Epic{}, f: &Feature{},
			wantGrain: "task", wantSource: "default",
		},
		{
			name: "project default applies",
			m:    &Manifest{Execution: ExecutionBlock{ReviewGrain: "feature"}}, e: &Epic{}, f: &Feature{},
			wantGrain: "feature", wantSource: "project",
		},
		{
			name: "epic beats project",
			m:    &Manifest{Execution: ExecutionBlock{ReviewGrain: "feature"}}, e: &Epic{ReviewGrain: "epic"}, f: &Feature{},
			wantGrain: "epic", wantSource: "epic",
		},
		{
			name: "feature beats epic, in either direction",
			m:    &Manifest{Execution: ExecutionBlock{ReviewGrain: "feature"}}, e: &Epic{ReviewGrain: "epic"}, f: &Feature{ReviewGrain: "task"},
			wantGrain: "task", wantSource: "feature",
		},
		{
			name: "nil feature resolves at epic",
			m:    &Manifest{Execution: ExecutionBlock{ReviewGrain: "feature"}}, e: &Epic{ReviewGrain: "epic"}, f: nil,
			wantGrain: "epic", wantSource: "epic",
		},
		{
			name: "nil epic resolves at project",
			m:    &Manifest{Execution: ExecutionBlock{ReviewGrain: "feature"}}, e: nil, f: nil,
			wantGrain: "feature", wantSource: "project",
		},
		{
			name: "everything nil resolves to the hardcoded default", m: nil, e: nil, f: nil,
			wantGrain: "task", wantSource: "default",
		},
		// The named ambiguous case (Fix round 1, Important 2): an explicit
		// feature-level "task" override under an epic-grain epic resolves to
		// grain "task" - the SAME value the hardcoded default would also
		// produce - but the source is still "feature", the explicit override
		// that actually won, never "default". Source names WHERE the value
		// came from, not whether it happens to differ from the fallback.
		{
			name: "an explicit feature override back to task under an epic-grain epic sources as feature, not default",
			m:    &Manifest{}, e: &Epic{ReviewGrain: "epic"}, f: &Feature{ReviewGrain: "task"},
			wantGrain: "task", wantSource: "feature",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grain, source := ResolvedReviewGrainWithSource(tc.m, tc.e, tc.f)
			require.Equal(t, tc.wantGrain, grain)
			require.Equal(t, tc.wantSource, source)
			// ResolvedReviewGrain delegates to this function, so the two can
			// never silently disagree about the grain half.
			require.Equal(t, tc.wantGrain, ResolvedReviewGrain(tc.m, tc.e, tc.f))
		})
	}
}

func TestValidReviewGrainPerNoun(t *testing.T) {
	for _, g := range []string{"task", "feature", "epic"} {
		require.True(t, ValidReviewGrain(NounEpic, g))
		require.True(t, ValidReviewGrain("project", g))
	}
	require.True(t, ValidReviewGrain(NounFeature, "task"))
	require.True(t, ValidReviewGrain(NounFeature, "feature"))
	require.True(t, ValidReviewGrain(NounFeature, "epic"), "reviewed moves retain inherited epic grain explicitly")
	require.False(t, ValidReviewGrain(NounEpic, "tasks"))
	require.False(t, ValidReviewGrain(NounEpic, ""))
}

func TestRenderExecutionReviewGrainRewritesOrAppendsSurgically(t *testing.T) {
	// An existing execution block: only its review_grain line moves.
	in := []byte("schema_version: 12\n\nexecution:\n  review_grain: task   # keep my comment\n\ncontributors: []\n")
	out, err := RenderExecutionReviewGrain(in, "feature")
	require.NoError(t, err)
	require.Equal(t, "schema_version: 12\n\nexecution:\n  review_grain: feature\n\ncontributors: []\n", string(out))
	// No execution block: one is appended at the end, untouched everything else.
	in2 := []byte("schema_version: 12\ncontributors: []\n")
	out2, err := RenderExecutionReviewGrain(in2, "epic")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(string(out2), "\nexecution:\n  review_grain: epic\n"))
	require.True(t, strings.HasPrefix(string(out2), "schema_version: 12\ncontributors: []\n"))
	// Round-trips: the parsed manifest carries the value.
	m, err := ParseManifest("/repo", out2)
	require.NoError(t, err)
	require.Equal(t, "epic", m.Execution.ReviewGrain)
}

func TestRenderExecutionReviewGrainRefusesFlowMapping(t *testing.T) {
	in := []byte("schema_version: 9\nexecution: {review_grain: task}\ncontributors: []\n")
	out, err := RenderExecutionReviewGrain(in, "feature")
	require.Error(t, err)
	require.Nil(t, out)
	require.Contains(t, err.Error(), "block mapping")
}

func TestRenderExecutionReviewGrainRefusesScalar(t *testing.T) {
	in := []byte("schema_version: 9\nexecution: feature\ncontributors: []\n")
	out, err := RenderExecutionReviewGrain(in, "task")
	require.Error(t, err)
	require.Nil(t, out)
	require.Contains(t, err.Error(), "block mapping")
}

func TestRenderExecutionReviewGrainFindsKeyAfterBlankAndCommentLines(t *testing.T) {
	in := []byte("schema_version: 9\nexecution:\n\n  # human note\n  review_grain: task\ncontributors: []\n")
	out, err := RenderExecutionReviewGrain(in, "feature")
	require.NoError(t, err)
	require.Equal(t,
		"schema_version: 9\nexecution:\n\n  # human note\n  review_grain: feature\ncontributors: []\n",
		string(out))
	require.Equal(t, 1, strings.Count(string(out), "review_grain:"))
}

func TestRenderExecutionReviewGrainDoesNotRewriteNestedKey(t *testing.T) {
	in := []byte("schema_version: 9\nexecution:\n  policy:\n    review_grain: task\ncontributors: []\n")
	out, err := RenderExecutionReviewGrain(in, "feature")
	require.NoError(t, err)
	require.Equal(t,
		"schema_version: 9\nexecution:\n  review_grain: feature\n  policy:\n    review_grain: task\ncontributors: []\n",
		string(out))
}

// TestTaskReviewGrainPrefersStartedGrain pins issue #146's remedy: a task
// keeps the grain it started under, whatever its feature now resolves to.
func TestTaskReviewGrainPrefersStartedGrain(t *testing.T) {
	m := &Manifest{}
	f := &Feature{ReviewGrain: "feature"}
	grain, source := TaskReviewGrain(m, nil, f, &Task{StartedGrain: "task"})
	require.Equal(t, "task", grain)
	require.Equal(t, "started", source)
}

func TestTaskReviewGrainFallsBackWithoutStartedGrain(t *testing.T) {
	m := &Manifest{}
	e := &Epic{ReviewGrain: "epic"}
	f := &Feature{}
	grain, source := TaskReviewGrain(m, e, f, &Task{})
	require.Equal(t, "epic", grain)
	require.Equal(t, "epic", source)
	grain, source = TaskReviewGrain(m, e, f, nil)
	require.Equal(t, "epic", grain, "a nil task resolves through its parents")
	require.Equal(t, "epic", source)
}
