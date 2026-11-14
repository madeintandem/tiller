package testgit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

func TestInitWritesTheSameConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	testgit.Init(t, dir)

	want := map[string]string{
		"user.email":       "test@example.com",
		"user.name":        "Tiller Test",
		"commit.gpgsign":   "false",
		"gc.auto":          "0",
		"maintenance.auto": "false",
	}
	for key, value := range want {
		require.Equal(t, value, testgit.Run(t, dir, "config", "--get", key), key)
	}
	require.Equal(t, "refs/heads/main", testgit.Run(t, dir, "symbolic-ref", "HEAD"))
}

// A suite started by a Git hook or `git rebase --exec` inherits that
// checkout's routing. A fixture must still be built where it was asked for,
// and must leave the checkout the routing names alone.
//
// Serial: t.Setenv routes Git through the process environment.
func TestRunIgnoresInheritedRouting(t *testing.T) {
	developer := t.TempDir()
	testgit.Init(t, developer)
	testgit.Run(t, developer, "commit", "-q", "--allow-empty", "-m", "the developer's work")
	head := testgit.Run(t, developer, "rev-parse", "HEAD")

	t.Setenv("GIT_DIR", filepath.Join(developer, ".git"))
	t.Setenv("GIT_WORK_TREE", developer)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(developer, ".git", "index"))

	fixture := t.TempDir()
	testgit.Init(t, fixture)
	require.NoError(t, os.WriteFile(filepath.Join(fixture, "a.txt"), []byte("a\n"), 0o644))
	testgit.Run(t, fixture, "add", "-A")
	testgit.Run(t, fixture, "commit", "-q", "-m", "fixture")

	require.Equal(t, "fixture", testgit.Run(t, fixture, "log", "-1", "--format=%s"))
	require.Equal(t, head, testgit.Run(t, developer, "rev-parse", "HEAD"))
	require.Empty(t, testgit.Run(t, developer, "status", "--porcelain"))
}
