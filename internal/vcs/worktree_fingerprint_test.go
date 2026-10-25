package vcs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fingerprint reads g's working-tree fingerprint value, failing the test on error.
func fingerprint(t *testing.T, g *Git) string {
	t.Helper()
	ref, err := g.WorktreeFingerprint()
	require.NoError(t, err)
	return ref.Value
}

// unusualNames are filesystem-valid names Git quotes or that whitespace
// trimming would corrupt.
var unusualNames = []string{
	"café.txt",
	"tab\tname.txt",
	"new\nline.txt",
	" leading.txt",
	"trailing.txt ",
}

// Defect 10: a quoted or whitespace-edged filename keeps its content identity.
// Each name is checked alone, tracked and untracked, so one name's change can
// never hide behind another's.
func TestWorktreeFingerprint_UnusualFilenamesKeepContentIdentity(t *testing.T) {
	for _, name := range unusualNames {
		t.Run("tracked "+name, func(t *testing.T) {
			dir, g := newRepo(t)
			require.NoError(t, writeRepoFile(dir, name, "committed\n"))
			run(t, dir, "add", "-A")
			run(t, dir, "commit", "-m", "unusual name")

			require.NoError(t, writeRepoFile(dir, name, "first edit\n"))
			first := fingerprint(t, g)
			assert.Equal(t, first, fingerprint(t, g), "unchanged content stays stable")
			require.NoError(t, writeRepoFile(dir, name, "second edit\n"))
			assert.NotEqual(t, first, fingerprint(t, g), "a content change to %q changes the fingerprint", name)
		})
		t.Run("untracked "+name, func(t *testing.T) {
			dir, g := newRepo(t)
			require.NoError(t, writeRepoFile(dir, name, "first\n"))
			first := fingerprint(t, g)
			require.NoError(t, writeRepoFile(dir, name, "second\n"))
			assert.NotEqual(t, first, fingerprint(t, g), "a content change to untracked %q changes the fingerprint", name)
		})
	}
}

// Git's default core.quotepath escapes non-ASCII bytes in plain output; the
// fingerprint must be the same value with quoting on or off.
func TestWorktreeFingerprint_QuotePathCannotAlterIdentity(t *testing.T) {
	dir, g := newRepo(t)
	require.NoError(t, writeRepoFile(dir, "café.txt", "committed\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "unicode")
	require.NoError(t, writeRepoFile(dir, "café.txt", "edited\n"))
	require.NoError(t, writeRepoFile(dir, "naïve.txt", "untracked\n"))

	run(t, dir, "config", "core.quotepath", "true")
	quoted := fingerprint(t, g)
	run(t, dir, "config", "core.quotepath", "false")
	assert.Equal(t, quoted, fingerprint(t, g))
}

// A real delete keeps the deletion sentinel: deleted differs from clean and
// from edited, and is stable.
func TestWorktreeFingerprint_DeletionIsDistinctAndStable(t *testing.T) {
	dir, g := newRepo(t)
	require.NoError(t, writeRepoFile(dir, "extra.txt", "dirty\n"))
	base := fingerprint(t, g)
	require.NoError(t, os.Remove(filepath.Join(dir, "README.md")))
	deleted := fingerprint(t, g)
	assert.NotEqual(t, base, deleted)
	assert.Equal(t, deleted, fingerprint(t, g))
	require.NoError(t, writeRepoFile(dir, "README.md", ""))
	assert.NotEqual(t, deleted, fingerprint(t, g), "an emptied file is not a deleted file")
}

// A changed path that exists but cannot be read is an error, never the
// deletion sentinel.
func TestWorktreeFingerprint_UnreadablePathIsNotADeletion(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads mode-000 files")
	}
	dir, g := newRepo(t)
	path := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(path, []byte("changed\n"), 0o644))
	require.NoError(t, os.Chmod(path, 0))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	_, err := g.WorktreeFingerprint()
	require.Error(t, err)
	var env *store.EnvError
	require.True(t, errors.As(err, &env), "error: %v", err)
	assert.Equal(t, "worktree_unreadable", env.Reason)
}

// newNestedRepo builds a repository whose managed project lives at proj/ and
// returns the repository root plus a *Git rooted at the nested project, which
// is how a nested Tiller project observes Git.
func newNestedRepo(t *testing.T) (string, *Git) {
	t.Helper()
	dir, _ := newRepo(t)
	require.NoError(t, writeRepoFile(dir, "proj/a.txt", "committed\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "nested project")
	g := New(filepath.Join(dir, "proj"))
	g.Clock = fixedClock{t: clockAt}
	return dir, g
}

// Defect 11: inside a nested project, two contents of the same tracked file
// produce different fingerprints.
func TestWorktreeFingerprint_NestedProjectTrackedContent(t *testing.T) {
	dir, g := newNestedRepo(t)
	require.NoError(t, writeRepoFile(dir, "proj/a.txt", "first\n"))
	first := fingerprint(t, g)
	require.NoError(t, writeRepoFile(dir, "proj/a.txt", "second\n"))
	assert.NotEqual(t, first, fingerprint(t, g))
}

// The fingerprint keeps whole-repository coverage from a nested project, for
// tracked and untracked paths outside it, and agrees with the root's reading.
func TestWorktreeFingerprint_NestedProjectCoversWholeRepository(t *testing.T) {
	dir, g := newNestedRepo(t)
	root := New(dir)
	root.Clock = fixedClock{t: clockAt}

	require.NoError(t, writeRepoFile(dir, "README.md", "outside edit one\n"))
	first := fingerprint(t, g)
	assert.Equal(t, fingerprint(t, root), first, "one coordinate system regardless of the observing directory")
	require.NoError(t, writeRepoFile(dir, "README.md", "outside edit two\n"))
	second := fingerprint(t, g)
	assert.NotEqual(t, first, second, "a tracked change outside the project is covered")

	require.NoError(t, writeRepoFile(dir, "other/new.txt", "one\n"))
	third := fingerprint(t, g)
	assert.NotEqual(t, second, third, "an untracked file outside the project is covered")
	require.NoError(t, writeRepoFile(dir, "other/new.txt", "two\n"))
	assert.NotEqual(t, third, fingerprint(t, g))

	require.NoError(t, os.Remove(filepath.Join(dir, "proj", "a.txt")))
	deleted := fingerprint(t, g)
	assert.Equal(t, fingerprint(t, root), deleted, "a deletion reads the same from either directory")
}

// A tracked proj/a.txt must never be hashed through proj/proj/a.txt, the
// similarly named child path a project-relative join would reach.
func TestWorktreeFingerprint_NestedSiblingAndChildPathsStayDistinct(t *testing.T) {
	dir, g := newNestedRepo(t)
	require.NoError(t, writeRepoFile(dir, "proj/proj/a.txt", "child\n"))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "child path")

	require.NoError(t, writeRepoFile(dir, "proj/a.txt", "first\n"))
	first := fingerprint(t, g)
	require.NoError(t, writeRepoFile(dir, "proj/a.txt", "second\n"))
	second := fingerprint(t, g)
	assert.NotEqual(t, first, second, "the project file's own content is hashed")

	require.NoError(t, writeRepoFile(dir, "proj/proj/a.txt", "child edit\n"))
	assert.NotEqual(t, second, fingerprint(t, g), "the child path is its own entry")
}

// gitWriteState snapshots what a read must never change: the index bytes,
// every ref, the stash, and the loose and packed object inventory.
func gitWriteState(t *testing.T, dir string) map[string]string {
	t.Helper()
	index, err := os.ReadFile(filepath.Join(dir, ".git", "index"))
	require.NoError(t, err)
	info, err := os.Stat(filepath.Join(dir, ".git", "index"))
	require.NoError(t, err)
	var objects []string
	require.NoError(t, filepath.Walk(filepath.Join(dir, ".git", "objects"), func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			objects = append(objects, p)
		}
		return err
	}))
	return map[string]string{
		"index":       string(index),
		"index-mtime": info.ModTime().String(),
		"refs":        run(t, dir, "for-each-ref"),
		"stash":       run(t, dir, "stash", "list"),
		"objects":     filepath.Join(objects...),
	}
}

func TestWorktreeFingerprint_WritesNothing(t *testing.T) {
	dir, g := newNestedRepo(t)
	require.NoError(t, writeRepoFile(dir, "proj/a.txt", "dirty\n"))
	require.NoError(t, writeRepoFile(dir, "proj/new.txt", "untracked\n"))
	require.NoError(t, os.Remove(filepath.Join(dir, "README.md")))
	before := gitWriteState(t, dir)
	fingerprint(t, g)
	assert.Equal(t, before, gitWriteState(t, dir))
}

// A symlink's identity is its target, as Git stores it, so retargeting a
// tracked link changes the fingerprint rather than reading as a deletion.
func TestWorktreeFingerprint_SymlinkTargetIsItsIdentity(t *testing.T) {
	dir, g := newRepo(t)
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink("README.md", link))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "link")

	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink("app/main.go", link))
	first := fingerprint(t, g)
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink("app", link))
	assert.NotEqual(t, first, fingerprint(t, g))
	require.NoError(t, os.Remove(link))
	assert.NotEqual(t, first, fingerprint(t, g), "a removed link is a deletion")
}
