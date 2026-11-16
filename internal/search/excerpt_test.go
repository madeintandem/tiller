package search_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/madeintandem/tiller/internal/search"
	"github.com/stretchr/testify/require"
)

// These tests pin the excerpt human output prints for one hit. Prose is
// written one paragraph per line, so a matching line can run to hundreds of
// characters. The excerpt holds the match, is at most 120 runes of the line,
// and carries an ellipsis at each end where text was cut. The JSON snippet
// keeps its own 200-rune window and says when it is not the whole line.

// longHit writes one line into a design doc and returns its hit for query.
func longHit(t *testing.T, line, query string) search.Hit {
	t.Helper()
	root, tree := fixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "design", "03-long.md"), []byte(line+"\n"), 0o644))
	res := run(t, root, tree, query)
	require.Len(t, res.Documents, 1)
	require.Len(t, res.Documents[0].Hits, 1)
	return res.Documents[0].Hits[0]
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func TestExcerptOfAShortLineIsTheLine(t *testing.T) {
	line := "A needle in a line of ordinary length. " + strings.Repeat("x", 81)
	require.Equal(t, 120, runeLen(line))
	hit := longHit(t, "  "+line+"  ", "needle")
	require.Equal(t, line, hit.Excerpt())
	require.Equal(t, line, hit.Snippet)
	require.False(t, hit.Truncated)
}

func TestExcerptCutsAroundAMatchDeepInTheLine(t *testing.T) {
	line := strings.Repeat("a", 300) + " needle-deep " + strings.Repeat("b", 300)
	hit := longHit(t, line, "needle-deep")
	got := hit.Excerpt()
	require.Contains(t, got, "needle-deep")
	require.True(t, strings.HasPrefix(got, "…a"), got)
	require.True(t, strings.HasSuffix(got, "b…"), got)
	require.Equal(t, 122, runeLen(got), "120 runes of the line and a mark at each cut")
}

func TestExcerptMarksOnlyTheEndThatWasCut(t *testing.T) {
	early := longHit(t, "needle-early "+strings.Repeat("x", 300), "needle-early").Excerpt()
	require.True(t, strings.HasPrefix(early, "needle-early x"), early)
	require.True(t, strings.HasSuffix(early, "x…"), early)
	require.Equal(t, 121, runeLen(early))

	late := longHit(t, strings.Repeat("x", 300)+" needle-late", "needle-late").Excerpt()
	require.True(t, strings.HasPrefix(late, "…x"), late)
	require.True(t, strings.HasSuffix(late, " needle-late"), late)
	require.Equal(t, 121, runeLen(late))
}

// TestExcerptCutsOnRuneBoundaries: multi-byte text is cut between runes, and
// a line whose lowercase form has a different byte length (the Kelvin sign
// lowercases from three bytes to one) still shows the match.
func TestExcerptCutsOnRuneBoundaries(t *testing.T) {
	for _, pad := range []string{"é", "日", "K"} {
		line := strings.Repeat(pad, 300) + " Needle-Wide " + strings.Repeat(pad, 300)
		got := longHit(t, line, "needle-wide").Excerpt()
		require.True(t, utf8.ValidString(got), "pad %q", pad)
		require.Contains(t, got, "Needle-Wide", "pad %q", pad)
		require.Equal(t, 122, runeLen(got), "pad %q", pad)
		require.True(t, strings.HasPrefix(got, "…"+pad), "pad %q", pad)
		require.True(t, strings.HasSuffix(got, pad+"…"), "pad %q", pad)
	}
}

// TestExcerptKeepsAWholeMatchThatFits: a long query leaves less room before
// it, never a match cut short.
func TestExcerptKeepsAWholeMatchThatFits(t *testing.T) {
	query := strings.Repeat("q", 110)
	got := longHit(t, strings.Repeat("a", 300)+query+strings.Repeat("b", 300), query).Excerpt()
	require.Contains(t, got, query)
	require.Equal(t, 122, runeLen(got))
}

// TestSnippetSaysWhenItIsNotTheWholeLine: the JSON snippet is unchanged, and
// a hit whose snippet was cut carries truncated: true; a whole line omits it.
func TestSnippetSaysWhenItIsNotTheWholeLine(t *testing.T) {
	whole := longHit(t, strings.Repeat("x", 180)+" needle", "needle")
	require.False(t, whole.Truncated)
	require.Equal(t, 187, runeLen(whole.Snippet))
	got, err := json.Marshal(whole)
	require.NoError(t, err)
	require.NotContains(t, string(got), "truncated")
	require.NotContains(t, string(got), "xcerpt")

	cut := longHit(t, "needle "+strings.Repeat("x", 300), "needle")
	require.True(t, cut.Truncated)
	require.Equal(t, 200, runeLen(cut.Snippet))
	got, err = json.Marshal(cut)
	require.NoError(t, err)
	require.JSONEq(t, `{"path":".tiller/design/03-long.md","line":1,"snippet":"`+cut.Snippet+`","truncated":true}`, string(got))
}

// TestAQuestionFieldIsExcerptedToo: a statement lives in the ledger, not in
// a file line, and is bounded the same way.
func TestAQuestionFieldIsExcerptedToo(t *testing.T) {
	root, tree := fixture(t)
	for _, q := range tree.Questions {
		q.Statement = strings.Repeat("a", 300) + " needle-field " + strings.Repeat("b", 300)
	}
	res := run(t, root, tree, "needle-field")
	require.Len(t, res.Nodes, 1)
	hit := res.Nodes[0].Hits[0]
	require.Equal(t, "statement", hit.Field)
	require.True(t, hit.Truncated)
	require.Equal(t, 122, runeLen(hit.Excerpt()))
	require.Contains(t, hit.Excerpt(), "needle-field")
}
