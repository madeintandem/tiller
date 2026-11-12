package vcs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// commitReadsRepos runs a check against a project at the repository root and
// against one in a subdirectory, since both resolve paths from Dir.
func commitReadsRepos(t *testing.T, check func(t *testing.T, dir, prefix string, g *Git)) {
	t.Helper()
	t.Run("root", func(t *testing.T) {
		dir, g := newRepo(t)
		check(t, dir, "", g)
	})
	t.Run("subdirectory", func(t *testing.T) {
		dir, g := newNestedRepo(t)
		check(t, dir, "proj/", g)
	})
}

func TestPathsAtListsACommitsFiles(t *testing.T) {
	commitReadsRepos(t, func(t *testing.T, dir, prefix string, g *Git) {
		require.NoError(t, writeRepoFile(dir, prefix+"data/a/one.txt", "1\n"))
		require.NoError(t, writeRepoFile(dir, prefix+"data/two.txt", "2\n"))
		run(t, dir, "add", "-A")
		run(t, dir, "commit", "-m", "data")
		at := run(t, dir, "rev-parse", "HEAD")
		require.NoError(t, writeRepoFile(dir, prefix+"data/later.txt", "3\n"))
		run(t, dir, "add", "-A")
		run(t, dir, "commit", "-m", "later")

		paths, err := g.PathsAt(at, "data")
		require.NoError(t, err)
		require.Equal(t, []string{"data/a/one.txt", "data/two.txt"}, paths)

		none, err := g.PathsAt(at, "missing")
		require.NoError(t, err)
		require.Empty(t, none)
	})
}

func TestFileAtReadsACommitsBytes(t *testing.T) {
	commitReadsRepos(t, func(t *testing.T, dir, prefix string, g *Git) {
		require.NoError(t, writeRepoFile(dir, prefix+"data/f.txt", "first\n"))
		run(t, dir, "add", "-A")
		run(t, dir, "commit", "-m", "first")
		at := run(t, dir, "rev-parse", "HEAD")
		require.NoError(t, writeRepoFile(dir, prefix+"data/f.txt", "second\n"))
		run(t, dir, "add", "-A")
		run(t, dir, "commit", "-m", "second")

		got, err := g.FileAt(at, "data/f.txt")
		require.NoError(t, err)
		require.Equal(t, "first\n", string(got))

		_, err = g.FileAt(at, "data/absent.txt")
		require.Error(t, err)
	})
}
