package execution

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func indexGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

// indexRepo builds a one-commit repository and an uncaptured source for it.
func indexRepo(t *testing.T) (*source, string) {
	t.Helper()
	root := t.TempDir()
	indexGit(t, root, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("one\n"), 0o644))
	indexGit(t, root, "add", ".")
	indexGit(t, root, "commit", "-qm", "one")
	common, head, branch, index, err := worktreeHead(root)
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(index), "index path %q is absolute", index)
	require.Equal(t, "refs/heads/main", branch)
	return &source{repo: &repository{common: common}, branch: "main", oid: head, worktreeRoot: root, indexPath: index}, root
}

// backdate ages a file past the racy window; Chtimes follows symlinks.
func backdate(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))
}

// capturedSource captures an index old enough to be trusted by identity.
func capturedSource(t *testing.T) (*source, string) {
	t.Helper()
	src, root := indexRepo(t)
	backdate(t, src.indexPath)
	require.NoError(t, src.captureIndex(src.indexPath))
	require.NotNil(t, src.indexInfo)
	return src, root
}

func countListings(t *testing.T) *int {
	t.Helper()
	listings := 0
	previous := listIndex
	listIndex = func(root string, keep bool) (map[string]bool, [32]byte, error) {
		listings++
		return previous(root, keep)
	}
	t.Cleanup(func() { listIndex = previous })
	return &listings
}

func TestUnchangedIndexFileSkipsTheListing(t *testing.T) {
	src, _ := capturedSource(t)
	listings := countListings(t)
	require.NoError(t, src.checkWorktreeIdentity())
	require.Equal(t, 0, *listings)
}

func TestARewrittenIndexWithTheSameEntriesStillPasses(t *testing.T) {
	src, root := capturedSource(t)
	listings := countListings(t)
	indexGit(t, root, "update-index", "--really-refresh")
	indexGit(t, root, "read-tree", "HEAD")
	require.NoError(t, src.checkWorktreeIdentity())
	require.Equal(t, 1, *listings, "a changed index file falls back to the content comparison")
}

func TestAStagedChangeStillFailsTheCheck(t *testing.T) {
	src, root := capturedSource(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("two\n"), 0o644))
	indexGit(t, root, "add", "README.md")
	require.ErrorContains(t, src.checkWorktreeIdentity(), "Git index changed")
}

func TestASymlinkedIndexStillDetectsAStagedChange(t *testing.T) {
	src, root := indexRepo(t)
	target := filepath.Join(root, ".git", "index-target")
	require.NoError(t, os.Rename(src.indexPath, target))
	require.NoError(t, os.Symlink(target, src.indexPath))
	backdate(t, target)
	// Age the link itself too (-h), so a check that stats the link rather than
	// its target would see an old, untouched file.
	out, err := exec.Command("touch", "-h", "-t", "200001010000", src.indexPath).CombinedOutput()
	require.NoError(t, err, "%s", out)
	require.NoError(t, src.captureIndex(src.indexPath))
	require.NotNil(t, src.indexInfo)
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("two\n"), 0o644))
	indexGit(t, root, "add", "README.md")
	link, err := os.Lstat(src.indexPath)
	require.NoError(t, err)
	require.NotZero(t, link.Mode()&os.ModeSymlink, "git left the link in place and rewrote its target")
	require.ErrorContains(t, src.checkWorktreeIdentity(), "Git index changed")
}

func TestARecentlyWrittenIndexAlwaysRelists(t *testing.T) {
	src, _ := indexRepo(t)
	require.NoError(t, src.captureIndex(src.indexPath))
	require.Nil(t, src.indexInfo, "an index inside the racy window keeps no identity")
	listings := countListings(t)
	require.NoError(t, src.checkWorktreeIdentity())
	require.Equal(t, 1, *listings)
}

func TestCaptureWorktreeRecordsTheIndexIdentity(t *testing.T) {
	src, _ := indexRepo(t)
	backdate(t, src.indexPath)
	src.indexPath = ""
	_, _, _, index, err := worktreeHead(src.worktreeRoot)
	require.NoError(t, err)
	require.NoError(t, src.captureIndex(index))
	require.Equal(t, index, src.indexPath)
	require.NotNil(t, src.indexInfo)
	require.NotZero(t, src.indexDigest)
	listings := countListings(t)
	require.NoError(t, src.checkWorktreeIdentity())
	require.Equal(t, 0, *listings)
}
