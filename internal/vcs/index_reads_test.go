package vcs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTrackedNamesDirectoriesTheIndexHoldsFilesIn(t *testing.T) {
	commitReadsRepos(t, func(t *testing.T, dir, prefix string, g *Git) {
		require.NoError(t, writeRepoFile(dir, prefix+"days/01/a.yml", "a\n"))
		require.NoError(t, writeRepoFile(dir, prefix+"days/02/b.yml", "b\n"))
		run(t, dir, "add", "-A")
		run(t, dir, "commit", "-m", "two days")
		require.NoError(t, writeRepoFile(dir, prefix+"days/03/c.yml", "c\n"))
		require.NoError(t, writeRepoFile(dir, prefix+"days/04/d.yml", "d\n"))
		run(t, dir, "add", prefix+"days/04/d.yml")
		run(t, dir, "rm", "-q", "--cached", prefix+"days/02/b.yml")

		tracked, err := g.Tracked([]string{"days/01", "days/02", "days/03", "days/04", "days/05"})
		require.NoError(t, err)
		require.Equal(t, map[string]bool{"days/01": true, "days/04": true}, tracked)

		none, err := g.Tracked(nil)
		require.NoError(t, err)
		require.Empty(t, none)
	})
}

func TestTrackedNamesFilesTheIndexHolds(t *testing.T) {
	commitReadsRepos(t, func(t *testing.T, dir, prefix string, g *Git) {
		require.NoError(t, writeRepoFile(dir, prefix+"days/01/a.yml", "a\n"))
		require.NoError(t, writeRepoFile(dir, prefix+"days/01/b.yml", "b\n"))
		run(t, dir, "add", prefix+"days/01/a.yml")
		run(t, dir, "commit", "-m", "one file")

		tracked, err := g.Tracked([]string{"days/01/a.yml", "days/01/b.yml", "days/01/c.yml", "days/01/a"})
		require.NoError(t, err)
		require.Equal(t, map[string]bool{"days/01/a.yml": true}, tracked)
	})
}
