package mutate_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

func TestMutationsLeaveRetiredBoardUntouched(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"absent", "stale", "invalid", "nul", "unreadable", "directory", "outside-symlink", "broken-symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			root, deps := layRenameTree(t)
			path := filepath.Join(root, ".tiller", "board.md")
			require.NoFileExists(t, path)
			switch kind {
			case "stale", "invalid", "nul", "unreadable":
				contents := map[string][]byte{"stale": []byte("legacy T-aaaaaa\n"), "invalid": {0xff}, "nul": {0}, "unreadable": []byte("private\n")}[kind]
				require.NoError(t, os.WriteFile(path, contents, 0o644))
				if kind == "unreadable" {
					require.NoError(t, os.Chmod(path, 0))
					t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
				}
			case "directory":
				require.NoError(t, os.Mkdir(path, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(path, "nested.md"), []byte("legacy\n"), 0o644))
			case "outside-symlink", "broken-symlink":
				target := filepath.Join(t.TempDir(), "outside.md")
				if kind == "outside-symlink" {
					require.NoError(t, os.WriteFile(target, []byte("outside\n"), 0o644))
				}
				require.NoError(t, os.Symlink(target, path))
			case "fifo":
				require.NoError(t, syscall.Mkfifo(path, 0o600))
			}
			before, beforeErr := os.Lstat(path)
			result, err := mutate.RenameTitle(load(t, root), deps, mutate.RenameTitleInput{Noun: store.NounEpic, Node: "E-8wqkyr", Title: "Retitled"})
			require.NoError(t, err)
			require.True(t, result.Changed)
			tree := load(t, root)
			require.Equal(t, "Retitled", tree.Epics["E-8wqkyr"].Title)
			require.True(t, hasEvent(t, tree, "rename.title.epic"))
			after, err := os.Lstat(path)
			if os.IsNotExist(beforeErr) {
				require.True(t, os.IsNotExist(err), "mutation must not create the retired board")
			} else {
				require.NoError(t, err)
				require.True(t, os.SameFile(before, after))
				require.Equal(t, before.Mode(), after.Mode())
				require.Equal(t, before.ModTime(), after.ModTime())
			}
		})
	}
}

func TestSyncWithoutCrossingsDoesNotCreateBoard(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	path := filepath.Join(root, ".tiller", "board.md")
	require.NoFileExists(t, path)
	result, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{Into: "main"})
	require.NoError(t, err)
	require.Empty(t, result.Crossings)
	require.NoFileExists(t, path)
}
