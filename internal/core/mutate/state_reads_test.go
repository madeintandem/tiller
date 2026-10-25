package mutate

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalkMutationStateMatchesWalkDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, file := range []string{"x/a.yml", "x/b.yml", "x/sub/c.yml", "y/d.yml"} {
		path := filepath.Join(root, ".tiller", file)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("k: v\n"), 0o644))
	}
	start := filepath.Join(root, ".tiller")
	for name, stop := range map[string]struct {
		base string
		err  error
	}{
		"SkipDir on a file skips its remaining siblings": {"a.yml", fs.SkipDir},
		"SkipDir on a directory skips that directory":    {"sub", fs.SkipDir},
		"SkipAll stops the walk":                         {"b.yml", fs.SkipAll},
	} {
		t.Run(name, func(t *testing.T) {
			collect := func(visited *[]string) fs.WalkDirFunc {
				return func(path string, _ fs.DirEntry, err error) error {
					require.NoError(t, err)
					*visited = append(*visited, path)
					if filepath.Base(path) == stop.base {
						return stop.err
					}
					return nil
				}
			}
			var want, got []string
			require.NoError(t, filepath.WalkDir(start, collect(&want)))
			require.NoError(t, walkMutationState(root, start, collect(&got)))
			require.Equal(t, want, got)
		})
	}
	t.Run("SkipDir on a file walked as the root", func(t *testing.T) {
		file := filepath.Join(start, "y", "d.yml")
		require.NoError(t, walkMutationState(root, file, func(string, fs.DirEntry, error) error { return fs.SkipDir }))
	})
}

func TestWalkMutationStateSkipDirOnAnUnreadableDirectoryContinues(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	locked := filepath.Join(root, ".tiller", "a")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "b"), 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	var visited []string
	err := walkMutationState(root, filepath.Join(root, ".tiller"), func(path string, _ fs.DirEntry, err error) error {
		visited = append(visited, filepath.Base(path))
		if err != nil {
			return fs.SkipDir
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{".tiller", "a", "a", "b"}, visited)
}
