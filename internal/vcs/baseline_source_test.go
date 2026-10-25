package vcs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBaselineSourcePinsObservedCommitAcrossHEADMovement(t *testing.T) {
	root, git := newRepo(t)
	for _, p := range []string{"docs/source.md", "docs/a[b],c.md"} {
		require.NoError(t, writeRepoFile(root, p, "original source\n"))
	}
	run(t, root, "add", "docs")
	run(t, root, "commit", "-m", "record source")
	observed, err := git.CurrentCommit()
	require.NoError(t, err)
	require.NoError(t, writeRepoFile(root, "docs/source.md", "later source\n"))
	run(t, root, "add", "docs/source.md")
	run(t, root, "commit", "-m", "advance HEAD")
	require.NoError(t, os.RemoveAll(filepath.Join(root, "docs")))
	for _, p := range []string{"docs/source.md", "docs/a[b],c.md"} {
		ref, err := BaselineSource(root, observed, p, clockAt)
		require.NoError(t, err)
		require.Equal(t, "original source", run(t, root, "show", ref.Value[4:]))
		require.Equal(t, p, ref.Path)
	}
}

func TestBaselineSourceNestedProjectPinsLiteralRegularBlobs(t *testing.T) {
	root, git := newRepo(t)
	project := filepath.Join(root, "apps", "project")
	sources := []string{"docs/source.md", "docs/a[b],c.md", "docs/a\tb.md", ":source.md"}
	for _, source := range sources {
		require.NoError(t, writeRepoFile(root, source, "repository-root decoy\n"))
		require.NoError(t, writeRepoFile(project, source, "original "+source+"\n"))
	}
	require.NoError(t, os.Chmod(filepath.Join(project, ":source.md"), 0o755))
	require.NoError(t, os.Symlink("docs/source.md", filepath.Join(project, "alias.md")))
	run(t, root, "add", ".")
	run(t, root, "commit", "-m", "record nested project sources")
	projectGit := New(project)
	observed, err := projectGit.CurrentCommit()
	require.NoError(t, err)
	for _, source := range sources {
		require.NoError(t, writeRepoFile(project, source, "later source\n"))
	}
	require.NoError(t, os.Remove(filepath.Join(project, "alias.md")))
	require.NoError(t, writeRepoFile(project, "alias.md", "now regular\n"))
	run(t, root, "add", ".")
	run(t, root, "commit", "-m", "advance project sources")
	for _, source := range sources {
		require.NoError(t, os.Remove(filepath.Join(project, source)))
		ref, err := BaselineSource(project, observed, source, clockAt)
		require.NoError(t, err)
		require.Equal(t, store.KindGitBlob, ref.Kind)
		require.Equal(t, source, ref.Path)
		require.Equal(t, clockAt, ref.ObservedAt)
		content, err := git.runRaw("show", ref.Value[4:])
		require.NoError(t, err)
		require.Equal(t, "original "+source+"\n", content)
	}
	for _, source := range []string{"docs", "alias.md"} {
		_, err := BaselineSource(project, observed, source, clockAt)
		require.Error(t, err, source)
	}
}
