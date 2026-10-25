package vcs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// nestedProjectRepo lays a repository whose Tiller project lives at project/,
// with a repository-root sibling that shares every project path's name. A
// committed-tree read from the project must never answer with the sibling.
func nestedProjectRepo(t *testing.T) (repo, project string) {
	t.Helper()
	repo = t.TempDir()
	snapshotGit(t, repo, "init", "-q", "-b", "main")
	for path, data := range map[string]string{
		"src/a.swift":           "root sibling\n",
		"chrome.swift":          "root chrome\n",
		"project/src/a.swift":   "project child\n",
		"project/chrome.swift":  "project chrome\n",
		"project/.tiller/x.yml": "x\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(repo, path)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(repo, path), []byte(data), 0o644))
	}
	snapshotGit(t, repo, "add", ".")
	snapshotGit(t, repo, "commit", "-qm", "fixture")
	return repo, filepath.Join(repo, "project")
}

func revParse(t *testing.T, dir, spec string) string {
	t.Helper()
	out := snapshotGit(t, dir, "rev-parse", spec)
	return out[:len(out)-1]
}

// Committed-tree reads take project-relative paths, the coordinates recorded
// surfaces and prose pins are stored in, so a nested project reads its own
// child rather than the repository-root sibling of the same name.
func TestCommittedReadsResolveProjectRelativePaths(t *testing.T) {
	repo, project := nestedProjectRepo(t)
	readers := map[string]vcs.Revisions{"git": vcs.New(project), "snapshot": vcs.NewHeadSnapshot(vcs.New(project))}
	for name, rev := range readers {
		tree, err := rev.TreeHash("src")
		require.NoError(t, err, name)
		require.Equal(t, revParse(t, repo, "HEAD:project/src"), tree, "%s TreeHash(src)", name)

		blob, err := rev.BlobHash("chrome.swift")
		require.NoError(t, err, name)
		require.Equal(t, revParse(t, repo, "HEAD:project/chrome.swift"), blob, "%s BlobHash(chrome.swift)", name)

		whole, err := rev.TreeHash(".")
		require.NoError(t, err, name)
		require.Equal(t, revParse(t, repo, "HEAD:project"), whole, "%s TreeHash(.) is the project tree", name)
	}

	git := vcs.New(project)
	tree, err := git.TreeHashAt("HEAD", "src")
	require.NoError(t, err)
	require.Equal(t, revParse(t, repo, "HEAD:project/src"), tree)
	blob, err := git.BlobHashAt("HEAD", "chrome.swift")
	require.NoError(t, err)
	require.Equal(t, revParse(t, repo, "HEAD:project/chrome.swift"), blob)
	inventory, err := git.TreeInventoryAt("HEAD", "src")
	require.NoError(t, err)
	require.Equal(t, []string{"src/a.swift"}, inventory)
}
