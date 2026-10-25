package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// WorktreeBlobHash answers the blob id `git add` would store. These pin that
// it answers exactly what git would for content git stores verbatim, so every
// basis pin recorded by an earlier binary keeps matching, that it applies the
// path's checkin conversion without writing an object, and that the error
// shape for a missing path is unchanged. WorktreeBlobMatches is the same
// comparison with an in-process raw fast path (issue #87: one `git
// hash-object` subprocess per compared file cost thousands of spawns).

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return strings.TrimSpace(string(out))
}

// Under a text rule an LF blob checked out with CRLF line endings is the same
// content: the working-tree hash is the committed blob, a real edit is not,
// and neither read writes an object. The rule names the project's path from the
// repository root, so it only applies if --path resolves from the nested
// project directory, as the hashed path does.
func TestWorktreeBlobHashAppliesCheckinConversion(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q", "-b", "main")
	// Background maintenance after commit could repack and move the object
	// counts this test compares.
	gitIn(t, repo, "config", "gc.auto", "0")
	gitIn(t, repo, "config", "maintenance.auto", "false")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "project"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("project/*.md text\n"), 0o644))
	spec := filepath.Join(repo, "project", "feature.md")
	require.NoError(t, os.WriteFile(spec, []byte("# Spec\n\nProse.\n"), 0o644))
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-qm", "spec")
	committed := gitIn(t, repo, "rev-parse", "HEAD:project/feature.md")
	require.NoError(t, os.WriteFile(spec, []byte("# Spec\r\n\r\nProse.\r\n"), 0o644))

	before := gitIn(t, repo, "count-objects", "-v")
	g := New(filepath.Join(repo, "project"))
	got, err := g.WorktreeBlobHash("feature.md")
	require.NoError(t, err)
	require.Equal(t, committed, got, "an unedited CRLF checkout hashes to its committed blob")
	matches, err := g.WorktreeBlobMatches("feature.md", committed)
	require.NoError(t, err)
	require.True(t, matches)

	require.NoError(t, os.WriteFile(spec, []byte("# Spec\r\n\r\nEdited.\r\n"), 0o644))
	got, err = g.WorktreeBlobHash("feature.md")
	require.NoError(t, err)
	require.NotEqual(t, committed, got, "a real edit hashes differently")
	matches, err = g.WorktreeBlobMatches("feature.md", committed)
	require.NoError(t, err)
	require.False(t, matches)
	require.Equal(t, before, gitIn(t, repo, "count-objects", "-v"), "hashing writes no object")
}

// A pin recorded as the raw blob id by an earlier binary still matches its
// unedited file, whatever conversion the path now carries.
func TestWorktreeBlobMatchesARawPin(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("* text=auto\n"), 0o644))
	body := []byte("a\r\nb\r\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.md"), body, 0o644))
	matches, err := New(dir).WorktreeBlobMatches("x.md", blobID(body))
	require.NoError(t, err)
	require.True(t, matches)
	_, err = New(dir).WorktreeBlobMatches("gone.md", blobID(body))
	var nv *NotVersionedError
	require.ErrorAs(t, err, &nv)
}

func TestWorktreeBlobHashMatchesGitForAKnownBlob(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0o644))
	g := &Git{Dir: dir}
	got, err := g.WorktreeBlobHash("hello.txt")
	require.NoError(t, err)
	// `printf 'hello\n' | git hash-object --stdin`
	require.Equal(t, "ce013625030ba8dba906f756967f9e9ca394464a", got)
}

func TestWorktreeBlobHashAgreesWithGitHashObjectOnRealContent(t *testing.T) {
	dir := t.TempDir()
	body := "---\nnode: F-abc123\n---\n\n# Spec\n\nSome prose with unicode: naïve café.\n"
	path := filepath.Join(dir, "feature.md")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	out, err := exec.Command("git", "hash-object", "--", path).Output()
	require.NoError(t, err)
	g := &Git{Dir: dir}
	got, err := g.WorktreeBlobHash("feature.md")
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(string(out)), got)
}

func TestWorktreeBlobHashOfAMissingOrDirectoryPathIsNotVersioned(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	g := &Git{Dir: dir}
	_, err := g.WorktreeBlobHash("nope.md")
	var nv *NotVersionedError
	require.ErrorAs(t, err, &nv)
	require.Equal(t, "nope.md", nv.Path)
	_, err = g.WorktreeBlobHash("sub")
	require.ErrorAs(t, err, &nv)
}

func TestWorktreeBlobHashRefusesUnsafeFilesWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	pipe := filepath.Join(dir, "pipe.md")
	require.NoError(t, syscall.Mkfifo(pipe, 0o600))
	done := make(chan error, 1)
	go func() {
		_, err := (&Git{Dir: dir}).WorktreeBlobHash("pipe.md")
		done <- err
	}()
	select {
	case err := <-done:
		var unavailable *NotVersionedError
		require.ErrorAs(t, err, &unavailable)
	case <-time.After(time.Second):
		writer, err := os.OpenFile(pipe, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			_ = writer.Close()
			<-done
		}
		t.Fatal("basis hashing blocked while opening a FIFO")
	}

	outside := filepath.Join(t.TempDir(), "spec.md")
	require.NoError(t, os.WriteFile(outside, []byte("outside the checkout"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "linked.md")))
	_, err := (&Git{Dir: dir}).WorktreeBlobHash("linked.md")
	var unavailable *NotVersionedError
	require.ErrorAs(t, err, &unavailable)
}
