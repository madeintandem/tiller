package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/stretchr/testify/require"
)

func TestNodeInventoriesRejectSymlinkHomes(t *testing.T) {
	for _, parent := range []string{
		".tiller/epics",
		".tiller/epics/form-engine/features",
		".tiller/epics/form-engine/features/certification-form-loading/tasks",
		".tiller/bugs", ".tiller/chores", ".tiller/tweaks",
	} {
		t.Run(parent, func(t *testing.T) {
			root := copyTree(t, fixtureTreeRoot)
			dir := filepath.Join(root, filepath.FromSlash(parent))
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(dir, "hidden")))
			_, err := Load(root)
			require.ErrorIs(t, err, rootpath.ErrUnsafe)
			require.Contains(t, err.Error(), "hidden")
			if parent == ".tiller/epics" || parent == ".tiller/epics/form-engine/features" || parent == ".tiller/epics/form-engine/features/certification-form-loading/tasks" {
				_, err = ReadLineageInventory(root, filepath.Join(root, ".tiller", "epics"))
				require.ErrorIs(t, err, rootpath.ErrUnsafe)
			}
		})
	}
}

func TestNodeInventoriesStillSkipOrdinaryFiles(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	before, err := Load(root)
	require.NoError(t, err)
	for _, shelf := range []string{"bugs", "chores", "tweaks"} {
		dir := filepath.Join(root, ".tiller", shelf)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "order.yml"), []byte("order: []\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("not a node\n"), 0o644))
	}
	tree, err := Load(root)
	require.NoError(t, err)
	require.Len(t, tree.Tasks, 1)
	require.Equal(t, before.Bugs, tree.Bugs)
	require.Equal(t, before.Chores, tree.Chores)
	require.Equal(t, before.Tweaks, tree.Tweaks)
}
