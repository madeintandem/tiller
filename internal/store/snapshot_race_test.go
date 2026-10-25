package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/stretchr/testify/require"
)

// A mutation plans over snapshots read without the mutation lock, so a
// concurrent writer can replace the file between the reader's lstat and open.
// That is a stale plan, not an unexplained environment failure (the same race
// #151 classified for the generation read).

// withSnapshotSwap lays .tiller/decisions/open-questions.yml and makes the next
// leaf open of it first run swap, which stands in for a concurrent writer.
func withSnapshotSwap(t *testing.T, swap func(dir string)) (root, path string) {
	t.Helper()
	root = t.TempDir()
	dir := filepath.Join(root, ".tiller", "decisions")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path = filepath.Join(dir, "open-questions.yml")
	require.NoError(t, os.WriteFile(path, []byte("questions: []\n"), 0o644))
	swapped := false
	previous := snapshotWalker
	snapshotWalker = rootpath.Walker{BeforeOpenFile: func(name string) {
		if swapped || name != ".tiller/decisions/open-questions.yml" {
			return
		}
		swapped = true
		swap(dir)
	}}
	t.Cleanup(func() { snapshotWalker = previous })
	return root, path
}

func TestSnapshotReplacedByAConcurrentWriterIsAStalePlan(t *testing.T) {
	root, path := withSnapshotSwap(t, func(dir string) {
		next := filepath.Join(dir, "open-questions.yml.next")
		require.NoError(t, os.WriteFile(next, []byte("questions: [{id: Q-x}]\n"), 0o644))
		require.NoError(t, os.Rename(next, filepath.Join(dir, "open-questions.yml")))
	})

	_, _, err := ReadRegularFileSnapshot(root, path)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationStale, env.Reason, "error: %v", err)
}

func TestSnapshotReplacedByASymlinkKeepsTheUnsafeRefusal(t *testing.T) {
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "forged.yml"), []byte("questions: []\n"), 0o644))
	root, path := withSnapshotSwap(t, func(dir string) {
		leaf := filepath.Join(dir, "open-questions.yml")
		require.NoError(t, os.Remove(leaf))
		require.NoError(t, os.Symlink(filepath.Join(outside, "forged.yml"), leaf))
	})

	_, _, err := ReadRegularFileSnapshot(root, path)
	require.Error(t, err, "the O_NOFOLLOW open refuses the swapped-in symlink")
	var env *EnvError
	require.False(t, errors.As(err, &env) && env.Reason == ReasonMutationStale,
		"a symlink is not a concurrent Tiller write: %v", err)
}
