package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// A claim suggestion names the task it suggests claiming, the way every other
// execute suggestion names its move, on bare guide and on check readiness.
func TestClaimSuggestionNamesTheTask(t *testing.T) {
	layFeatureReadyToStart(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	want := "Next move: claim and start " + cliTaskKey + " (T-"

	out, stderr, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, "execute", data["skill"])
	require.Contains(t, data["reason"], want)

	// Readiness wants a design input before it closes on guide's suggestion.
	require.NoError(t, os.MkdirAll(filepath.Join(".tiller", "design"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(".tiller", "design", "roadmap.md"), []byte("# Roadmap\n"), 0o644))
	readiness, stderr, code := execCLI(t, "check", "readiness")
	require.Equal(t, 0, code, readiness+stderr)
	require.Contains(t, readiness, "Next (tiller guide execute):")
	require.Contains(t, readiness, want)
}

// Bare guide prints the caught-up hint once, under the list; its suggestion
// does not repeat it. Check readiness has no list above its suggestion, so it
// carries the hint, worded as a sentence.
func TestCaughtUpHintPrintsOnce(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)

	human, stderr, code := execCLI(t, "guide")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, 1, strings.Count(human, "o work is planned yet"), human)
	require.Contains(t, human, "Suggested skill: orient\nNo move is legal right now.\n")

	empty := &store.Tree{Manifest: &store.Manifest{}}
	skill, reason := suggestSkill(empty, "jc")
	require.Equal(t, "orient", skill)
	hint := views.NextOf(empty, views.NextInput{Reader: "jc"}).Hint
	require.NotEmpty(t, hint)
	require.Equal(t, views.Sentence(hint), reason)
}
