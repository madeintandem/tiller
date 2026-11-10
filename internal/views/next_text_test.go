package views_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// Sentence capitalizes a plain leading word but never a leading node key,
// and ends with exactly one period, unless it already ends a question or an
// exclamation.
func TestSentenceLeavesALeadingKeyAlone(t *testing.T) {
	for in, want := range map[string]string{
		"no work is planned yet":            "No work is planned yet.",
		"in review, waiting on your merge.": "In review, waiting on your merge.",
		"checkout/alpha/t1 (T-x) is ready":  "checkout/alpha/t1 (T-x) is ready.",
		"rides-alone review":                "rides-alone review.",
		"2 more remain..":                   "2 more remain.",
		"holds it; Which schema?":           "Holds it; Which schema?",
		"really!":                           "Really!",
		"   ":                               "",
	} {
		require.Equal(t, want, views.Sentence(in), "input %q", in)
	}
}

// A move with a skill and no command carries, as guide, the command its text
// line prints; the console copies it. A move with a command, or one that
// continues on another branch, carries none.
func TestNextGuideIsTheCommandTheTextLinePrints(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa0"].Status = "ready"
	draftFeatureUnder(t, tree, "F-aaaaa0", "epic-0/first", "epic-0")

	next := views.NextOf(tree, views.NextInput{Now: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)})
	var specify, approve *views.NextCandidate
	for i, c := range next.Candidates {
		switch {
		case c.Kind == "specify":
			specify = &next.Candidates[i]
		case c.Command != "":
			approve = &next.Candidates[i]
		}
	}
	require.NotNil(t, specify, "the fixture yields a specify move")
	require.NotNil(t, approve, "the fixture yields a move with a command")

	require.Equal(t, "tiller guide specify", specify.Guide)
	require.Contains(t, views.NextLine(*specify), " `tiller guide specify`")
	raw, err := json.Marshal(specify)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"guide":"tiller guide specify"`)

	require.Empty(t, approve.Guide)
	raw, err = json.Marshal(approve)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"guide"`)

	branch := views.NextCandidate{Actor: "agent", Kind: "continue", Action: "continue", Skill: "execute", Branch: "tiller/x"}
	require.Empty(t, views.NextGuide(branch))
	require.Equal(t, "tiller guide execute", views.NextGuide(views.NextCandidate{Skill: "execute"}))
	require.Empty(t, views.NextGuide(views.NextCandidate{Skill: "execute", Command: "tiller start task x"}))
}
