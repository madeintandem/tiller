package corpus_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/corpus"
	"github.com/stretchr/testify/require"
)

func TestWalkIncludesGeneratedMarkdownInSortedOrder(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller/design"), 0755))
	for name, text := range map[string]string{"board.md": "generated", "orientation.md": "guidance", "design/a.md": "authored", "design/b.MD": "excluded"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", name), []byte(text), 0644))
	}
	var paths []string
	require.NoError(t, corpus.Walk(context.Background(), root, func(d corpus.Document) error {
		paths = append(paths, d.Path)
		return nil
	}))
	require.Equal(t, []string{".tiller/design/a.md", ".tiller/orientation.md"}, paths)
}

func TestWalkSkipsDotEntries(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{".tiller/.cache", ".tiller/design/.private"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0755))
	}
	for name, text := range map[string]string{".cache/secret.md": "secret", "design/.private/x.md": "secret", "design/.draft.md": "secret", "design/a.md": "authored"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", name), []byte(text), 0644))
	}
	var paths []string
	require.NoError(t, corpus.Walk(context.Background(), root, func(d corpus.Document) error {
		paths = append(paths, d.Path)
		return nil
	}))
	require.Equal(t, []string{".tiller/design/a.md"}, paths)
}

func TestWalkRefusesUnsafeCorpus(t *testing.T) {
	for _, kind := range []string{"root-symlink", "file-symlink", "directory-symlink", "markdown-directory", "binary", "invalid-utf8"} {
		t.Run(kind, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0644))
			if kind == "root-symlink" {
				require.NoError(t, os.Symlink(outside, filepath.Join(root, ".tiller")))
			} else {
				dir := filepath.Join(root, ".tiller")
				require.NoError(t, os.Mkdir(dir, 0755))
				switch kind {
				case "file-symlink":
					require.NoError(t, os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "x.md")))
				case "directory-symlink":
					require.NoError(t, os.Symlink(outside, filepath.Join(dir, "nested")))
				case "markdown-directory":
					require.NoError(t, os.Mkdir(filepath.Join(dir, "x.md"), 0755))
				case "binary":
					require.NoError(t, os.WriteFile(filepath.Join(dir, "x.md"), []byte("bad\x00text"), 0644))
				case "invalid-utf8":
					require.NoError(t, os.WriteFile(filepath.Join(dir, "x.md"), []byte{0xff}, 0644))
				}
			}
			require.Error(t, corpus.Walk(context.Background(), root, func(d corpus.Document) error { return nil }))
		})
	}
}

func TestWalkHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, corpus.Walk(ctx, t.TempDir(), func(d corpus.Document) error { t.Fatal("visited canceled corpus"); return nil }), context.Canceled)
}

func TestReadCitedMarkdownCannotEscapeRepository(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "docs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs/source.md"), []byte("source"), 0644))
	document, err := corpus.Read(context.Background(), root, "docs/source.md")
	require.NoError(t, err)
	require.Equal(t, "source", string(document.Content))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "alias")))
	_, err = corpus.Read(context.Background(), root, "alias/secret.md")
	require.Error(t, err)
	_, err = corpus.Read(context.Background(), root, "../secret.md")
	require.Error(t, err)
}
