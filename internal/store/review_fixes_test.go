package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// acquireMutationLock releases the lock through the cache directory handle after
// it returns, so that handle must be the transaction's retained one.
func TestMutationCacheParentIsRetainedByItsTransaction(t *testing.T) {
	for _, existing := range []bool{false, true} {
		root := t.TempDir()
		cacheDir := filepath.Join(".tiller", cacheDirName)
		require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
		if existing {
			require.NoError(t, os.Mkdir(filepath.Join(root, cacheDir), 0o755))
		}
		rooted, err := openRootedTransaction(root, rootOperationHooks{})
		require.NoError(t, err)
		require.NoError(t, rooted.mkdirAll(cacheDir, 0o755))
		_, transient, err := rooted.openParent(cacheDir)
		require.NoError(t, err)
		require.False(t, transient, "existing=%v", existing)
		require.NoError(t, rooted.close())
	}
}

func TestSyncExistingDirectorySyncsOnlyARealDirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "epics"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte("x: 1\n"), 0o644))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, ".tiller", "linked")))
	rooted, err := openRootedTransaction(root, rootOperationHooks{})
	require.NoError(t, err)
	defer rooted.close()
	var synced []string
	sync := func(path string) error { synced = append(synced, path); return nil }
	at := func(relative string) error {
		return syncExistingDirectory(rooted, relative, filepath.Join(root, relative), sync)
	}

	require.NoError(t, at(filepath.Join(".tiller", "epics")))
	require.NoError(t, at(filepath.Join(".tiller", "missing")))
	require.NoError(t, at("."))
	require.Error(t, at(filepath.Join(".tiller", "tiller.yml")))
	require.Error(t, at(filepath.Join(".tiller", "linked")))
	require.Equal(t, []string{filepath.Join(root, ".tiller", "epics"), filepath.Join(root, ".")}, synced)
}

func TestRepositoryRelativePathAnchorsAtTheNamedRoot(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct{ path, relative string }{
		{filepath.Join(root, ".tiller", "tiller.yml"), filepath.Join(".tiller", "tiller.yml")},
		{filepath.Join(root, ".tiller", "x", ".tiller", "y.yml"), filepath.Join(".tiller", "x", ".tiller", "y.yml")},
		{filepath.Join(root, "artifacts", "tiller", "gates", "out.txt"), filepath.Join("artifacts", "tiller", "gates", "out.txt")},
		{root, "."},
	} {
		relative, err := RepositoryRelativePath(root, tc.path)
		require.NoError(t, err, tc.path)
		require.Equal(t, tc.relative, relative, tc.path)
	}
	for _, outside := range []string{filepath.Dir(root), filepath.Join(root, "..", "sibling", ".tiller", "tiller.yml")} {
		_, err := RepositoryRelativePath(root, outside)
		require.ErrorContains(t, err, "outside repository root", outside)
	}
}
