package search_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/search"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin what search refuses: a query that constrains nothing, and a
// corpus it cannot safely read. Either way the answer is a typed error and
// never a partial page.

func TestEmptyQueryIsTypedMisuse(t *testing.T) {
	root, tree := fixture(t)
	for _, q := range []string{"", "   ", "\t\n"} {
		res, err := search.Search(root, tree, q, search.Options{})
		require.Equal(t, search.Result{}, res)
		var typed *search.EmptyQueryError
		require.ErrorAs(t, err, &typed, "query %q", q)
		require.Equal(t, 2, typed.ExitCode())
		require.Equal(t, "empty_query", typed.Reason())
	}
}

// TestUnreadableCorpusIsEnvironment: a file the walk cannot read (a dangling
// .md symlink stands in portably for any I/O failure) aborts the whole search
// as an exit-3 environment failure. The query was fine; the repo is not.
func TestUnreadableCorpusIsEnvironment(t *testing.T) {
	root, tree := fixture(t)
	require.NoError(t, os.Symlink(
		filepath.Join(root, "no-such-target.md"),
		filepath.Join(root, ".tiller", "design", "02-dangling.md"),
	))
	_, err := search.Search(root, tree, "form", search.Options{})
	var envErr *store.EnvError
	require.ErrorAs(t, err, &envErr)
	require.Equal(t, 3, envErr.ExitCode())
	require.Equal(t, search.ReasonCorpusUnreadable, envErr.Reason)
}

// TestSearchRefusesEscapingMarkdownSymlink: a Markdown-shaped entry inside
// .tiller must not turn search into a reader for an arbitrary file reached
// through a symlink.
func TestSearchRefusesEscapingMarkdownSymlink(t *testing.T) {
	root, tree := fixture(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(outside, []byte("# outside-only-token\n"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".tiller", "design", "02-outside.md")))

	got, err := search.Search(root, tree, "outside-only-token", search.Options{})
	require.Equal(t, search.Result{}, got)
	var envErr *store.EnvError
	require.ErrorAs(t, err, &envErr)
	require.Equal(t, search.ReasonCorpusUnreadable, envErr.Reason)
}

// TestSearchRefusesBinaryMarkdown: invalid UTF-8 is not coerced and a
// NUL-bearing file is not treated as ordinary prose.
func TestSearchRefusesBinaryMarkdown(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"invalid UTF-8", []byte{'#', ' ', 0xff, '\n'}},
		{"NUL byte", []byte("# binary\x00markdown\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, tree := fixture(t)
			require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "design", "02-binary.md"), tc.data, 0o644))
			got, err := search.Search(root, tree, "binary", search.Options{})
			require.Equal(t, search.Result{}, got)
			var envErr *store.EnvError
			require.ErrorAs(t, err, &envErr)
			require.Equal(t, search.ReasonCorpusUnreadable, envErr.Reason)
		})
	}
}

// TestSearchRefusesFIFOWithoutReading covers the non-regular corpus path. The
// timeout fallback opens a writer only to release a regressed blocking read
// before failing the test.
func TestSearchRefusesFIFOWithoutReading(t *testing.T) {
	root, tree := fixture(t)
	pipe := filepath.Join(root, ".tiller", "design", "02-pipe.md")
	require.NoError(t, syscall.Mkfifo(pipe, 0o600))

	var got search.Result
	var err error
	done := make(chan struct{})
	go func() {
		got, err = search.Search(root, tree, "anything", search.Options{})
		close(done)
	}()
	select {
	case <-done:
		require.Equal(t, search.Result{}, got)
		var envErr *store.EnvError
		require.ErrorAs(t, err, &envErr)
		require.Equal(t, search.ReasonCorpusUnreadable, envErr.Reason)
	case <-time.After(time.Second):
		writer, openErr := os.OpenFile(pipe, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if openErr == nil {
			_ = writer.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
		t.Fatal("search blocked opening a FIFO instead of refusing it as non-regular corpus")
	}
}

// Orientation is generated guidance, while the retired root board is outside
// the corpus altogether. Neither is searchable authored prose.
func TestSearchSkipsGeneratedFiles(t *testing.T) {
	root, tree := fixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "board.md"), []byte("needlephrase on the board\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "orientation.md"), []byte("needlephrase in guidance\n"), 0o644))
	require.Equal(t, 0, run(t, root, tree, "needlephrase").Count)
}
