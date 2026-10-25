package corpus

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/stretchr/testify/require"
)

// TestCorpusFSRejectsIntermediateSwap proves an fs.FS open cannot be redirected
// after one directory segment has been checked but before traversal continues.
// The replacement has the same remaining shape and a readable Markdown leaf,
// so only the parent identity binding can distinguish it from the approved tree.
func TestCorpusFSRejectsIntermediateSwap(t *testing.T) {
	corpus := t.TempDir()
	approved := filepath.Join(corpus, "design", "nested")
	replacement := filepath.Join(corpus, "replacement", "nested")
	require.NoError(t, os.MkdirAll(approved, 0o755))
	require.NoError(t, os.MkdirAll(replacement, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(approved, "doc.md"), []byte("approved\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(replacement, "doc.md"), []byte("redirected\n"), 0o644))

	anchored, err := os.OpenRoot(corpus)
	require.NoError(t, err)
	defer anchored.Close()

	swapped := false
	fsys := corpusFS{reader: rootpath.NewReader(anchored, rootpath.Walker{AfterOpenRoot: func(name string) {
		if swapped || name != "design" {
			return
		}
		swapped = true
		require.NoError(t, os.Rename(
			filepath.Join(corpus, "design"),
			filepath.Join(corpus, "approved-design"),
		))
		require.NoError(t, os.Symlink("replacement", filepath.Join(corpus, "design")))
	}})}

	file, err := fsys.Open("design/nested/doc.md")
	if file != nil {
		require.NoError(t, file.Close())
	}
	require.True(t, swapped)
	require.True(t, errors.Is(err, rootpath.ErrUnsafe), "error: %v", err)
}
