package vcs_test

import (
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// A project nested in a repository is found at its own place in each
// worktree. The worktree roots, which also hold whatever sits beside the
// project, are what a project at the repository root gets.
func TestProjectWorktreePathsKeepTheProjectPrefix(t *testing.T) {
	repo, project := nestedProjectRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	snapshotGit(t, repo, "worktree", "add", "-q", "-b", "side", wt)

	resolved := func(paths []string) []string {
		t.Helper()
		out := make([]string, len(paths))
		for i, p := range paths {
			r, err := filepath.EvalSymlinks(p)
			require.NoError(t, err)
			out[i] = r
		}
		return out
	}

	nested, err := vcs.New(project).ProjectWorktreePaths()
	require.NoError(t, err)
	require.Equal(t, resolved([]string{project, filepath.Join(wt, "project")}), resolved(nested))

	// A directory beneath the project keeps its whole path too.
	deeper, err := vcs.New(filepath.Join(project, "src")).ProjectWorktreePaths()
	require.NoError(t, err)
	require.Equal(t, resolved([]string{filepath.Join(project, "src"), filepath.Join(wt, "project", "src")}), resolved(deeper))

	root, err := vcs.New(repo).ProjectWorktreePaths()
	require.NoError(t, err)
	require.Equal(t, resolved([]string{repo, wt}), resolved(root))
}
