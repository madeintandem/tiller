package vcs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// caseInsensitiveDir reports whether dir's filesystem resolves case variants
// of one name to the same entry, the APFS/NTFS default this regression needs.
func caseInsensitiveDir(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	require.NoError(t, os.WriteFile(probe, []byte("probe\n"), 0o644))
	defer os.Remove(probe)
	lower, err := os.Stat(filepath.Join(dir, "caseprobe"))
	if err != nil {
		require.ErrorIs(t, err, os.ErrNotExist)
		return false
	}
	upper, err := os.Stat(probe)
	require.NoError(t, err)
	return os.SameFile(lower, upper)
}

func TestWorktreeSurfaceKeepsTrackedFileAfterUnstagedCaseRename(t *testing.T) {
	dir, _ := newRepo(t)
	if !caseInsensitiveDir(t, dir) {
		t.Skip("needs a case-insensitive filesystem")
	}
	require.NoError(t, os.Rename(filepath.Join(dir, "app/main.go"), filepath.Join(dir, "app/MAIN.go")))
	// Git's index still spells the file the old way; only the directory
	// listing shows the rename.
	require.Equal(t, "app/main.go", run(t, dir, "ls-files", "--", "app"))

	renamed, err := WorktreeSurface(dir, "app", "app", clockAt)
	require.NoError(t, err)
	require.Equal(t, store.KindGitTree, renamed.Kind)
	require.Equal(t, []string{"app/MAIN.go"}, renamed.Inventory)
	empty := "git:4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	require.NotEqual(t, empty, renamed.Value)

	require.NoError(t, writeRepoFile(dir, "app/MAIN.go", "package edited\n"))
	edited, err := WorktreeSurface(dir, "app", "app", clockAt)
	require.NoError(t, err)
	require.Equal(t, []string{"app/MAIN.go"}, edited.Inventory)
	require.NotEqual(t, renamed.Value, edited.Value)
}

func TestWorktreeSurfaceKeepsTrackedDirectoryAfterUnstagedCaseRename(t *testing.T) {
	dir, _ := newRepo(t)
	if !caseInsensitiveDir(t, dir) {
		t.Skip("needs a case-insensitive filesystem")
	}
	require.NoError(t, writeRepoFile(dir, "code/app/main.go", "package main\n"))
	run(t, dir, "add", "code")
	run(t, dir, "commit", "-m", "code")
	require.NoError(t, os.Rename(filepath.Join(dir, "code/app"), filepath.Join(dir, "code/App")))
	require.Equal(t, "code/app/main.go", run(t, dir, "ls-files", "--", "code"))

	renamed, err := WorktreeSurface(dir, "code", "code", clockAt)
	require.NoError(t, err)
	require.Equal(t, []string{"code/App/main.go"}, renamed.Inventory)

	require.NoError(t, writeRepoFile(dir, "code/App/main.go", "package edited\n"))
	edited, err := WorktreeSurface(dir, "code", "code", clockAt)
	require.NoError(t, err)
	require.NotEqual(t, renamed.Value, edited.Value)
}

// On a case-sensitive filesystem a.go and A.go are two files; an untracked
// case variant must neither borrow the tracked file's index entry nor vanish.
func TestWorktreeSurfaceKeepsCaseVariantsDistinct(t *testing.T) {
	dir, _ := newRepo(t)
	if caseInsensitiveDir(t, dir) {
		t.Skip("needs a case-sensitive filesystem")
	}
	require.NoError(t, writeRepoFile(dir, ".gitignore", "MAIN.go\n"))
	require.NoError(t, writeRepoFile(dir, "app/MAIN.go", "ignored variant\n"))
	got, err := WorktreeSurface(dir, "app", "app", clockAt)
	require.NoError(t, err)
	// The ignored variant is a distinct file: it must not ride main.go's
	// index entry into the inventory.
	require.Equal(t, []string{"app/main.go"}, got.Inventory)
	require.NoError(t, os.Remove(filepath.Join(dir, ".gitignore")))
	both, err := WorktreeSurface(dir, "app", "app", clockAt)
	require.NoError(t, err)
	require.Equal(t, []string{"app/MAIN.go", "app/main.go"}, both.Inventory)
}
