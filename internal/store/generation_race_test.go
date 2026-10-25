package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/stretchr/testify/require"
)

// Issue #151: readableGeneration reads the generation file without the
// mutation lock, so a concurrent writer can replace it between the reader's
// lstat and open. That is a stale read, not an unexplained environment failure.

// withGenerationSwap lays a generation file and makes the next leaf open of it
// first run swap, which stands in for a concurrent writer.
func withGenerationSwap(t *testing.T, swap func(cache string)) string {
	t.Helper()
	root := t.TempDir()
	cache := filepath.Join(root, ".tiller", cacheDirName)
	require.NoError(t, os.MkdirAll(cache, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cache, generationFileName), []byte("one"), 0o644))
	swapped := false
	previous := repositoryReaderWalker
	repositoryReaderWalker = rootpath.Walker{BeforeOpenFile: func(name string) {
		if swapped || name != generationFileName {
			return
		}
		swapped = true
		swap(cache)
	}}
	t.Cleanup(func() { repositoryReaderWalker = previous })
	return root
}

func TestGenerationReplacedByAConcurrentWriterIsAStaleRead(t *testing.T) {
	root := withGenerationSwap(t, func(cache string) {
		next := filepath.Join(cache, "generation.next")
		require.NoError(t, os.WriteFile(next, []byte("two"), 0o644))
		require.NoError(t, os.Rename(next, filepath.Join(cache, generationFileName)))
	})

	_, err := readableGeneration(root)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationStale, env.Reason, "error: %v", err)
}

func TestGenerationReplacedByASymlinkKeepsTheUnsafeRefusal(t *testing.T) {
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "generation"), []byte("forged"), 0o644))
	root := withGenerationSwap(t, func(cache string) {
		path := filepath.Join(cache, generationFileName)
		require.NoError(t, os.Remove(path))
		require.NoError(t, os.Symlink(filepath.Join(outside, "generation"), path))
	})

	_, err := readableGeneration(root)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Empty(t, env.Reason, "a symlink is not a concurrent Tiller write: %v", err)
}
