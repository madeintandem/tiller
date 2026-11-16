package search_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/search"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin the wider corpus `tiller search` reads: the files the
// baseline ledger records under derived_from, at their working-tree content,
// each one a document result. The same confinement holds for them as for
// .tiller/: nothing is read through a symlink or outside the project.

// sourceFixture adds a baseline ledger to the ordinary fixture. Its one as-is
// page names every path in sources as a derived_from source. The files
// themselves are the caller's to lay.
func sourceFixture(t *testing.T, sources ...string) string {
	t.Helper()
	root, _ := fixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "design", "as-is"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "design", "as-is", "rules.md"),
		[]byte("# Rules\n\nThe rules live in docs/spec/rules.md.\n"), 0o644))
	ledger := "docs:\n    - path: as-is/rules.md\n      describes:\n        - internal\n      derived_from:\n"
	for _, source := range sources {
		ledger += fmt.Sprintf("        - kind: git_blob\n          value: git:e4e6449ef45d9b6e092ee27d0aae3e66acefc31a\n"+
			"          path: %s\n          observed_at: 2026-11-14T23:09:09Z\n", source)
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, store.BaselineRelPath), []byte(ledger), 0o644))
	return root
}

func writeSource(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// searchSources loads the tree and searches with the ledger's sources in the
// corpus, the way the CLI does.
func searchSources(t *testing.T, root, query string) (search.Result, error) {
	t.Helper()
	tree, err := store.Load(root)
	require.NoError(t, err)
	return search.Search(root, tree, query, search.Options{Sources: true})
}

// TestALedgerSourceIsADocumentResult is the defect: a keyword that appears
// only in a document the baseline points at returned nothing.
func TestALedgerSourceIsADocumentResult(t *testing.T) {
	root := sourceFixture(t, "docs/spec/rules.md")
	writeSource(t, root, "docs/spec/rules.md", "# Rules\n\nEvery rubric names its grader.\nA rubric is scored once.\n")

	res, err := searchSources(t, root, "rubric")
	require.NoError(t, err)
	require.Empty(t, res.Nodes)
	require.Equal(t, []search.DocumentResult{{
		Path:  "docs/spec/rules.md",
		Count: 2,
		Hits: []search.Hit{
			{Path: "docs/spec/rules.md", Line: 3, Snippet: "Every rubric names its grader."},
			{Path: "docs/spec/rules.md", Line: 4, Snippet: "A rubric is scored once."},
		},
	}}, res.Documents)
	require.Equal(t, []search.KindCount{{Kind: search.KindDocument, Count: 1}}, res.Kinds)
}

// TestSourcesAreReadOnlyWhenAsked: the console's search passes no Sources
// option and keeps the .tiller/ corpus it has always had.
func TestSourcesAreReadOnlyWhenAsked(t *testing.T) {
	root := sourceFixture(t, "docs/spec/rules.md")
	writeSource(t, root, "docs/spec/rules.md", "Every rubric names its grader.\n")
	tree, err := store.Load(root)
	require.NoError(t, err)

	res, err := search.Search(root, tree, "rubric", search.Options{})
	require.NoError(t, err)
	require.Zero(t, res.Count)
}

// TestOnlyRecordedSourcesAreRead: a neighbor of a recorded source is not in
// the corpus. The ledger's sources are the bounded set.
func TestOnlyRecordedSourcesAreRead(t *testing.T) {
	root := sourceFixture(t, "docs/spec/rules.md")
	writeSource(t, root, "docs/spec/rules.md", "Nothing to see.\n")
	writeSource(t, root, "docs/spec/other.md", "A rubric nobody recorded.\n")

	res, err := searchSources(t, root, "rubric")
	require.NoError(t, err)
	require.Zero(t, res.Count)
}

// TestAMissingSourceIsSkipped: the ledger keeps a source's path after the
// file is deleted or moved, and a path whose parent is now a file is just as
// gone. Neither fails the search or hides the sources that remain.
func TestAMissingSourceIsSkipped(t *testing.T) {
	root := sourceFixture(t, "docs/gone.md", "docs/spec/rules.md", "docs/spec/rules.md/part.md")
	writeSource(t, root, "docs/spec/rules.md", "Every rubric names its grader.\n")

	res, err := searchSources(t, root, "rubric")
	require.NoError(t, err)
	require.Len(t, res.Documents, 1)
	require.Equal(t, "docs/spec/rules.md", res.Documents[0].Path)
}

// TestADirectorySourceIsSkipped: the ledger records file blobs. A directory
// standing where a source was is not a document, and search does not walk it.
func TestADirectorySourceIsSkipped(t *testing.T) {
	root := sourceFixture(t, "docs/spec")
	writeSource(t, root, "docs/spec/rules.md", "Every rubric names its grader.\n")

	res, err := searchSources(t, root, "rubric")
	require.NoError(t, err)
	require.Zero(t, res.Count)
}

// TestANonTextSourceIsSkipped: a source may be any file (a PDF, a
// spreadsheet). One that is not UTF-8 text has no lines to match and is left
// out rather than refusing every search in the project.
func TestANonTextSourceIsSkipped(t *testing.T) {
	root := sourceFixture(t, "docs/legacy.bin", "docs/nul.txt", "docs/spec/rules.md")
	writeSource(t, root, "docs/legacy.bin", "rubric \xff\xfe\n")
	writeSource(t, root, "docs/nul.txt", "rubric\x00\n")
	writeSource(t, root, "docs/spec/rules.md", "Every rubric names its grader.\n")

	res, err := searchSources(t, root, "rubric")
	require.NoError(t, err)
	require.Len(t, res.Documents, 1)
	require.Equal(t, "docs/spec/rules.md", res.Documents[0].Path)
}

// TestASourceIsReadOnce: two as-is pages naming one source make one result,
// and a source under .tiller/ is not read a second time.
func TestASourceIsReadOnce(t *testing.T) {
	root := sourceFixture(t, "docs/spec/rules.md", ".tiller/design/01-form-shapes.md", ".tiller/tiller.yml")
	writeSource(t, root, "docs/spec/rules.md", "The certification rubric.\n")
	ledger, err := os.ReadFile(filepath.Join(root, store.BaselineRelPath))
	require.NoError(t, err)
	second := strings.Replace(strings.TrimPrefix(string(ledger), "docs:\n"), "as-is/rules.md", "as-is/more.md", 1)
	require.NoError(t, os.WriteFile(filepath.Join(root, store.BaselineRelPath), append(ledger, second...), 0o644))

	res, err := searchSources(t, root, "certification")
	require.NoError(t, err)
	paths := []string{}
	for _, d := range res.Documents {
		paths = append(paths, d.Path)
	}
	require.Equal(t, []string{".tiller/design/01-form-shapes.md", "docs/spec/rules.md"}, paths)
	require.Equal(t, 1, res.Documents[0].Count)
}

// TestAHiddenSourceIsSkipped: a dot-prefixed path outside .tiller/ is hidden
// local state under the corpus policy, never quoted.
func TestAHiddenSourceIsSkipped(t *testing.T) {
	root := sourceFixture(t, ".notes/private.md")
	writeSource(t, root, ".notes/private.md", "A private rubric.\n")

	res, err := searchSources(t, root, "rubric")
	require.NoError(t, err)
	require.Zero(t, res.Count)
}

// TestASymlinkedSourceRefusesTheSearch: a source that is now a symlink, or
// sits behind a symlinked directory, is refused with the corpus refusal, and
// the file it points at outside the project is never read.
func TestASymlinkedSourceRefusesTheSearch(t *testing.T) {
	for _, tc := range []struct {
		name string
		lay  func(t *testing.T, root, outside string)
	}{
		{"the file is a symlink", func(t *testing.T, root, outside string) {
			require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "spec"), 0o755))
			require.NoError(t, os.Symlink(filepath.Join(outside, "rules.md"), filepath.Join(root, "docs", "spec", "rules.md")))
		}},
		{"a parent directory is a symlink", func(t *testing.T, root, outside string) {
			require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
			require.NoError(t, os.Symlink(outside, filepath.Join(root, "docs", "spec")))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := sourceFixture(t, "docs/spec/rules.md")
			outside := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(outside, "rules.md"), []byte("outside-only-token\n"), 0o644))
			tc.lay(t, root, outside)

			got, err := searchSources(t, root, "outside-only-token")
			require.Equal(t, search.Result{}, got)
			var envErr *store.EnvError
			require.ErrorAs(t, err, &envErr)
			require.Equal(t, 3, envErr.ExitCode())
			require.Equal(t, search.ReasonCorpusUnreadable, envErr.Reason)
		})
	}
}
