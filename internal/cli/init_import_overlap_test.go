package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Bug hunt defect 12 (design import copies its destination recursively): a
// --design source that contains the checkout's .tiller/design/ would read its
// own output. Each spelling of that overlap refuses as misuse before any
// scaffold write, leaving the whole checkout byte-identical.
func TestInitDesignRefusesSourceContainingDestination(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source func(t *testing.T, root string) string
	}{
		{"checkout itself", func(t *testing.T, root string) string { return "." }},
		{"ancestor of the checkout", func(t *testing.T, root string) string { return filepath.Dir(root) }},
		{"checkout reached through a symlinked ancestor", func(t *testing.T, root string) string {
			alias := filepath.Join(t.TempDir(), "alias")
			require.NoError(t, os.Symlink(filepath.Dir(root), alias))
			return filepath.Join(alias, filepath.Base(root))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "project")
			writeFixture(t, root, "docs/design.md", "# Design\n")
			writeFixture(t, filepath.Dir(root), "notes.md", "# Notes\n")
			source := tc.source(t, root)
			t.Chdir(root)
			beforeFiles, beforeDirs := snapshotWholeTree(t, root)

			out, stderr, code := execCLI(t, "init", "--name", "P", "--key", "p", "--kind", "greenfield", "--design", source, "--json")
			require.Equal(t, 2, code, out+stderr)
			require.Contains(t, out, `"reason": "invalid_design"`)

			afterFiles, afterDirs := snapshotWholeTree(t, root)
			require.Equal(t, beforeFiles, afterFiles)
			require.ElementsMatch(t, beforeDirs, afterDirs, "no scaffold directory or ignore file may be written before the refusal")
		})
	}
}

// A rerun over an existing project with an overlapping source refuses the same
// way and leaves every existing project byte unchanged.
func TestInitDesignRerunWithOverlappingSourceLeavesProjectUnchanged(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	_, stderr, code := execCLI(t, "init", "--name", "P", "--key", "p", "--kind", "greenfield")
	require.Equal(t, 0, code, stderr)
	writeFixture(t, root, ".tiller/design/existing.md", "# Existing\n")
	beforeFiles, beforeDirs := snapshotWholeTree(t, root)

	for _, source := range []string{".", ".tiller/design"} {
		out, stderr, code := execCLI(t, "init", "--name", "P", "--key", "p", "--kind", "greenfield", "--design", source, "--json")
		require.Equal(t, 2, code, out+stderr)
		require.Contains(t, out, `"reason": "invalid_design"`)
	}
	afterFiles, afterDirs := snapshotWholeTree(t, root)
	require.Equal(t, beforeFiles, afterFiles)
	require.ElementsMatch(t, beforeDirs, afterDirs)
}

// The shared copy operation init and intake both use refuses on its own to
// copy a directory tree that contains, or sits inside, its destination, so no
// caller can make it recurse into its own output. A single file cannot
// recurse, so a file source inside the destination copies normally.
func TestImportDocsCannotEnterItsOwnDestination(t *testing.T) {
	t.Run("source contains destination", func(t *testing.T) {
		root := t.TempDir()
		writeFixture(t, root, "a.md", "a\n")
		before := snapshotTree(t, root)
		_, err := importDocs(root, root, filepath.Join(".tiller", "design"))
		require.Error(t, err)
		require.Equal(t, before, snapshotTree(t, root))
		require.NoDirExists(t, filepath.Join(root, ".tiller"))
	})
	t.Run("directory source inside destination", func(t *testing.T) {
		root := t.TempDir()
		writeFixture(t, root, ".tiller/feedback/old/review.md", "review\n")
		before := snapshotTree(t, root)
		_, err := importDocs(root, filepath.Join(root, ".tiller", "feedback", "old"), filepath.Join(".tiller", "feedback"))
		require.Error(t, err)
		require.Equal(t, before, snapshotTree(t, root))
	})
	t.Run("file source inside destination", func(t *testing.T) {
		root := t.TempDir()
		writeFixture(t, root, ".tiller/feedback/old/review.md", "review\n")
		n, err := importDocs(root, filepath.Join(root, ".tiller", "feedback", "old", "review.md"), filepath.Join(".tiller", "feedback"))
		require.NoError(t, err)
		require.Equal(t, 1, n)
		data, err := os.ReadFile(filepath.Join(root, ".tiller", "feedback", "review.md"))
		require.NoError(t, err)
		require.Equal(t, "review\n", string(data))
	})
}
