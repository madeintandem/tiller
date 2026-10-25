package rootpath_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, root, path, data string) {
	t.Helper()
	full := filepath.Join(root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(data), 0o644))
}

func readAll(t *testing.T, file *os.File) string {
	t.Helper()
	defer file.Close()
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	return string(data)
}

// The whole point of the Reader: many files under the same directories cost
// one verified descent per directory, not one per file.
func TestReaderDescendsEachDirectoryOnce(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one", "two", "three"} {
		writeFile(t, dir, filepath.Join("events", "2026", "10", name+".yml"), name)
	}
	anchor, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer anchor.Close()
	descents := map[string]int{}
	reader := rootpath.NewReader(anchor, rootpath.Walker{AfterOpenRoot: func(name string) { descents[name]++ }})
	defer reader.Close()

	for _, name := range []string{"one", "two", "three"} {
		file, err := reader.Open(filepath.Join("events", "2026", "10", name+".yml"), false)
		require.NoError(t, err)
		require.Equal(t, name, readAll(t, file))
	}
	require.Equal(t, map[string]int{
		"events":                              1,
		filepath.Join("events", "2026"):       1,
		filepath.Join("events", "2026", "10"): 1,
	}, descents)
}

func TestReaderRefusesASymlinkedDirectory(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	writeFile(t, outside, "secret.yml", "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "events"), 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "events", "2026")))
	anchor, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer anchor.Close()
	reader := rootpath.NewReader(anchor, rootpath.Walker{})
	defer reader.Close()

	_, err = reader.Open(filepath.Join("events", "2026", "secret.yml"), false)
	require.True(t, errors.Is(err, rootpath.ErrUnsafe), "error: %v", err)
	_, err = reader.Dir(filepath.Join("events", "2026"))
	require.True(t, errors.Is(err, rootpath.ErrUnsafe), "error: %v", err)
}

func TestReaderRefusesASymlinkedLeaf(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	writeFile(t, outside, "secret.yml", "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "events"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.yml"), filepath.Join(dir, "events", "EV-x.yml")))
	anchor, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer anchor.Close()
	reader := rootpath.NewReader(anchor, rootpath.Walker{})
	defer reader.Close()

	_, err = reader.Open(filepath.Join("events", "EV-x.yml"), false)
	require.True(t, errors.Is(err, rootpath.ErrUnsafe), "error: %v", err)
}

// A regular leaf replaced by another regular file between lstat and open, the
// shape of an atomic rename by a concurrent writer, is ErrChanged as well as
// ErrUnsafe, so a caller can tell it from symlink substitution (#151).
func TestReaderMarksAReplacedRegularLeafAsChanged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, filepath.Join("cache", "generation"), "one")
	anchor, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer anchor.Close()
	reader := rootpath.NewReader(anchor, rootpath.Walker{BeforeOpenFile: func(name string) {
		next := filepath.Join(dir, "cache", "generation.next")
		require.NoError(t, os.WriteFile(next, []byte("two"), 0o644))
		require.NoError(t, os.Rename(next, filepath.Join(dir, "cache", "generation")))
	}})
	defer reader.Close()

	_, err = reader.Open(filepath.Join("cache", "generation"), false)
	require.True(t, errors.Is(err, rootpath.ErrChanged), "error: %v", err)
	require.True(t, errors.Is(err, rootpath.ErrUnsafe), "error: %v", err)
	require.EqualError(t, err, "unsafe anchored path: generation changed while it was opened")
}

// A swap during the first descent is caught by the walker's identity binding.
func TestReaderRefusesASwapDuringDescent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, filepath.Join("design", "doc.md"), "approved")
	writeFile(t, dir, filepath.Join("replacement", "doc.md"), "redirected")
	anchor, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer anchor.Close()
	swapped := false
	reader := rootpath.NewReader(anchor, rootpath.Walker{AfterOpenRoot: func(name string) {
		if swapped || name != "design" {
			return
		}
		swapped = true
		require.NoError(t, os.Rename(filepath.Join(dir, "design"), filepath.Join(dir, "approved-design")))
		require.NoError(t, os.Symlink("replacement", filepath.Join(dir, "design")))
	}})
	defer reader.Close()

	_, err = reader.Open(filepath.Join("design", "doc.md"), false)
	require.True(t, swapped)
	require.True(t, errors.Is(err, rootpath.ErrUnsafe), "error: %v", err)
}

// A directory the Reader already verified stays bound to that directory. A
// later rename plus symlink cannot redirect reads made through it, which is the
// same guarantee rootedTransaction gives a mutation's retained parents.
func TestReaderRetainedDirectoryCannotBeRedirectedAfterVerification(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	writeFile(t, dir, filepath.Join("events", "one.yml"), "inside one")
	writeFile(t, dir, filepath.Join("events", "two.yml"), "inside two")
	writeFile(t, outside, "two.yml", "outside two")
	anchor, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer anchor.Close()
	reader := rootpath.NewReader(anchor, rootpath.Walker{})
	defer reader.Close()

	file, err := reader.Open(filepath.Join("events", "one.yml"), false)
	require.NoError(t, err)
	require.Equal(t, "inside one", readAll(t, file))
	require.NoError(t, os.Rename(filepath.Join(dir, "events"), filepath.Join(dir, "events-moved")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "events")))

	file, err = reader.Open(filepath.Join("events", "two.yml"), false)
	require.NoError(t, err)
	require.Equal(t, "inside two", readAll(t, file))
}

func TestReaderOpensTheAnchorAndRefusesTraversal(t *testing.T) {
	dir := t.TempDir()
	anchor, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer anchor.Close()
	reader := rootpath.NewReader(anchor, rootpath.Walker{})
	defer reader.Close()

	same, err := reader.Dir(".")
	require.NoError(t, err)
	require.Same(t, anchor, same)
	directory, err := reader.Open(".", true)
	require.NoError(t, err)
	require.NoError(t, directory.Close())
	_, err = reader.Open(filepath.Join("..", "x"), false)
	require.True(t, errors.Is(err, rootpath.ErrUnsafe), "error: %v", err)
}

// A walk over many sibling directories must not hold a descriptor per
// directory: the Reader keeps a bounded number open, still descends the shared
// parent once, and descends each sibling once.
func TestReaderBoundsRetainedDirectoriesAcrossSiblings(t *testing.T) {
	dir := t.TempDir()
	const siblings = 200
	for i := 0; i < siblings; i++ {
		writeFile(t, dir, filepath.Join("epics", fmt.Sprintf("e%03d", i), "epic.yml"), "x")
	}
	anchor, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer anchor.Close()
	descents := map[string]int{}
	reader := rootpath.NewReader(anchor, rootpath.Walker{AfterOpenRoot: func(name string) { descents[name]++ }})
	defer reader.Close()

	for i := 0; i < siblings; i++ {
		file, err := reader.Open(filepath.Join("epics", fmt.Sprintf("e%03d", i), "epic.yml"), false)
		require.NoError(t, err)
		require.NoError(t, file.Close())
		require.LessOrEqual(t, reader.RetainedCount(), rootpath.RetainedDirectoryLimit)
	}
	require.Len(t, descents, siblings+1)
	for name, count := range descents {
		require.Equal(t, 1, count, "descents of %s", name)
	}
}

// An evicted directory is verified again, not silently reused or lost.
func TestReaderReopensAnEvictedDirectory(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i <= rootpath.RetainedDirectoryLimit+1; i++ {
		writeFile(t, dir, filepath.Join("d", fmt.Sprintf("s%03d", i), "f.yml"), fmt.Sprintf("file %d", i))
	}
	anchor, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer anchor.Close()
	descents := map[string]int{}
	reader := rootpath.NewReader(anchor, rootpath.Walker{AfterOpenRoot: func(name string) { descents[name]++ }})
	defer reader.Close()

	first := filepath.Join("d", "s000")
	for i := 0; i <= rootpath.RetainedDirectoryLimit+1; i++ {
		file, err := reader.Open(filepath.Join("d", fmt.Sprintf("s%03d", i), "f.yml"), false)
		require.NoError(t, err)
		require.NoError(t, file.Close())
	}
	require.Equal(t, 1, descents[first])

	file, err := reader.Open(filepath.Join(first, "f.yml"), false)
	require.NoError(t, err)
	require.Equal(t, "file 0", readAll(t, file))
	require.Equal(t, 2, descents[first])
}
