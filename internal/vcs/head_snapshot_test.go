package vcs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

func snapshotGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return string(out)
}

type answer struct {
	hash string
	err  string
}

func ask(read func(string) (string, error), path string) answer {
	hash, err := read(path)
	if err != nil {
		return answer{err: err.Error()}
	}
	return answer{hash: hash}
}

// HeadSnapshot must answer exactly what Git answers, including every refusal,
// from the top of the repository and from a nested Tiller project directory.
func TestHeadSnapshotMatchesGitForEveryPathShape(t *testing.T) {
	root := t.TempDir()
	snapshotGit(t, root, "init", "-q", "-b", "main")
	for path, data := range map[string]string{"app/models/user.rb": "u\n", "app/jobs/job.rb": "j\n", "README.md": "r\n", "project/.tiller/tiller.yml": "x\n"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(data), 0o644))
	}
	snapshotGit(t, root, "add", ".")
	snapshotGit(t, root, "update-index", "--add", "--cacheinfo", "160000,1111111111111111111111111111111111111111,vendor/lib")
	snapshotGit(t, root, "commit", "-qm", "fixture")

	paths := []string{"", ".", "app", "app/models", "app/models/user.rb", "README.md", "missing", "app/missing.rb", "app/", "./app", "vendor/lib", "README.md/inner"}
	for _, dir := range []string{root, filepath.Join(root, "project")} {
		git, snapshot := vcs.New(dir), vcs.NewHeadSnapshot(vcs.New(dir))
		for _, path := range paths {
			require.Equal(t, ask(git.TreeHash, path), ask(snapshot.TreeHash, path), "TreeHash(%q) from %s", path, dir)
			require.Equal(t, ask(git.BlobHash, path), ask(snapshot.BlobHash, path), "BlobHash(%q) from %s", path, dir)
		}
	}
}

func TestHeadSnapshotMatchesGitOnAnUnbornRepository(t *testing.T) {
	root := t.TempDir()
	snapshotGit(t, root, "init", "-q", "-b", "main")
	git, snapshot := vcs.New(root), vcs.NewHeadSnapshot(vcs.New(root))
	for _, path := range []string{"", "app", "README.md"} {
		require.Equal(t, ask(git.TreeHash, path), ask(snapshot.TreeHash, path), "TreeHash(%q)", path)
		require.Equal(t, ask(git.BlobHash, path), ask(snapshot.BlobHash, path), "BlobHash(%q)", path)
	}
}

func TestHeadSnapshotPinsTheHeadItFirstRead(t *testing.T) {
	root := t.TempDir()
	snapshotGit(t, root, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("one\n"), 0o644))
	snapshotGit(t, root, "add", ".")
	snapshotGit(t, root, "commit", "-qm", "one")
	snapshot := vcs.NewHeadSnapshot(vcs.New(root))
	before, err := snapshot.BlobHash("README.md")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("two\n"), 0o644))
	snapshotGit(t, root, "commit", "-qam", "two")
	after, err := snapshot.BlobHash("README.md")
	require.NoError(t, err)
	require.Equal(t, before, after, "one sweep reads one consistent HEAD")
}

var _ vcs.Revisions = (*vcs.HeadSnapshot)(nil)
