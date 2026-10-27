package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// useCache turns the process cache on for one test and off again after it.
func useCache(t *testing.T) *cache {
	t.Helper()
	c := newCache()
	processCache.Store(c)
	t.Cleanup(func() { processCache.Store(nil) })
	return c
}

func writeUsage(t *testing.T, root, harness, session string, calls int) string {
	t.Helper()
	f := File{
		Harness: harness, Session: session, Reader: ReaderVersion,
		Rows: []Row{{Hour: time.Date(2026, 10, 20, 14, 0, 0, 0, time.UTC), Stage: Other, Model: "m", Calls: calls, Tokens: Tokens{Input: int64(calls)}}},
	}
	data, err := Encode(f)
	require.NoError(t, err)
	p := filepath.Join(root, Path(harness, session))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, data, 0o644))
	return p
}

func callsBySession(files []File) map[string]int {
	out := map[string]int{}
	for _, f := range files {
		out[f.Session] = f.Calls()
	}
	return out
}

func TestCachedLoadFilesRereadsNothingUnchanged(t *testing.T) {
	c := useCache(t)
	root := t.TempDir()
	writeUsage(t, root, "claude-code", "a", 1)
	writeUsage(t, root, "pi", "b", 2)

	first, err := LoadFiles(root)
	require.NoError(t, err)
	require.Equal(t, 2, c.misses())
	second, err := LoadFiles(root)
	require.NoError(t, err)
	require.Equal(t, 2, c.misses(), "unchanged files are not reparsed")
	require.Equal(t, callsBySession(first), callsBySession(second))
}

func TestCachedLoadFilesRereadsAGrownFile(t *testing.T) {
	c := useCache(t)
	root := t.TempDir()
	p := writeUsage(t, root, "claude-code", "a", 1)
	_, err := LoadFiles(root)
	require.NoError(t, err)

	writeUsage(t, root, "claude-code", "a", 12345)
	// A rewrite within the filesystem's timestamp granularity still changes
	// the size here; pin the time forward too so the test does not rely on it.
	later := time.Now().Add(time.Minute)
	require.NoError(t, os.Chtimes(p, later, later))
	got, err := LoadFiles(root)
	require.NoError(t, err)
	require.Equal(t, 2, c.misses())
	require.Equal(t, map[string]int{"a": 12345}, callsBySession(got))
}

func TestCachedLoadFilesSeesAddedAndRemovedFiles(t *testing.T) {
	c := useCache(t)
	root := t.TempDir()
	p := writeUsage(t, root, "claude-code", "a", 1)
	_, err := LoadFiles(root)
	require.NoError(t, err)

	writeUsage(t, root, "codex", "b", 2)
	got, err := LoadFiles(root)
	require.NoError(t, err)
	require.Equal(t, 2, c.misses(), "only the new file is parsed")
	require.Equal(t, map[string]int{"a": 1, "b": 2}, callsBySession(got))

	require.NoError(t, os.Remove(p))
	got, err = LoadFiles(root)
	require.NoError(t, err)
	require.Equal(t, map[string]int{"b": 2}, callsBySession(got))
}

func TestCachedLoadFilesStillFailsOnAMalformedFile(t *testing.T) {
	useCache(t)
	root := t.TempDir()
	writeUsage(t, root, "claude-code", "a", 1)
	_, err := LoadFiles(root)
	require.NoError(t, err)

	bad := filepath.Join(root, Path("pi", "broken"))
	require.NoError(t, os.MkdirAll(filepath.Dir(bad), 0o755))
	require.NoError(t, os.WriteFile(bad, []byte("rows: [unclosed"), 0o644))
	for range 2 {
		_, err = LoadFiles(root)
		require.ErrorContains(t, err, ".tiller/usage/pi/broken.yml")
	}
}

func TestLoadFilesWithoutTheCacheParsesEveryTime(t *testing.T) {
	processCache.Store(nil)
	root := t.TempDir()
	writeUsage(t, root, "claude-code", "a", 1)
	for range 2 {
		got, err := LoadFiles(root)
		require.NoError(t, err)
		require.Equal(t, map[string]int{"a": 1}, callsBySession(got))
	}
}
