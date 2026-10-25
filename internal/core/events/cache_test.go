package events

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
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

func writeEvent(t *testing.T, root, day, id, typ string) string {
	t.Helper()
	path := filepath.Join(root, ".tiller", "events", "2026", "09", day, id+".yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("id: "+id+"\ntype: "+typ+"\nat: 2026-09-"+day+"T10:00:00Z\ncontributor: jc\ncommand: test\n"), 0o644))
	return path
}

func eventTypes(evs []Event) map[string]string {
	out := map[string]string{}
	for _, e := range evs {
		out[e.ID] = e.Type
	}
	return out
}

func TestCachedListRereadsNothingUnchanged(t *testing.T) {
	c := useCache(t)
	root := t.TempDir()
	writeEvent(t, root, "01", "EV-aaaaaa", "claim.task")
	writeEvent(t, root, "02", "EV-bbbbbb", "start.task")
	tree := &store.Tree{Root: root}

	first, err := List(tree)
	require.NoError(t, err)
	require.Equal(t, 2, c.misses())
	second, err := List(tree)
	require.NoError(t, err)
	require.Equal(t, 2, c.misses(), "an unchanged stream is not reparsed")
	require.Equal(t, first, second)
}

func TestCachedListPicksUpEventsAddedToAnOldDay(t *testing.T) {
	c := useCache(t)
	root := t.TempDir()
	writeEvent(t, root, "01", "EV-aaaaaa", "claim.task")
	tree := &store.Tree{Root: root}
	_, err := List(tree)
	require.NoError(t, err)

	// A merged branch carries an event dated into an already-read directory.
	writeEvent(t, root, "01", "EV-cccccc", "release.task")
	evs, err := List(tree)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"EV-aaaaaa": "claim.task", "EV-cccccc": "release.task"}, eventTypes(evs))
	require.Equal(t, 2, c.misses(), "only the new file is parsed")
}

// A checkout replaces files by writing a new file and renaming it into place.
// Even with the same size and modification time, the new file is a different
// file, so the cached event must not be reused.
func TestCachedListRereadsAFileReplacedUnderTheSamePath(t *testing.T) {
	useCache(t)
	root := t.TempDir()
	path := writeEvent(t, root, "01", "EV-aaaaaa", "claim.task")
	tree := &store.Tree{Root: root}
	_, err := List(tree)
	require.NoError(t, err)
	info, err := os.Lstat(path)
	require.NoError(t, err)

	replacement := path + ".tmp"
	require.NoError(t, os.WriteFile(replacement, []byte("id: EV-aaaaaa\ntype: start.task\nat: 2026-09-01T10:00:00Z\ncontributor: jc\ncommand: test\n"), 0o644))
	require.NoError(t, os.Chtimes(replacement, info.ModTime(), info.ModTime()))
	require.NoError(t, os.Rename(replacement, path))

	evs, err := List(tree)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"EV-aaaaaa": "start.task"}, eventTypes(evs))
}

func TestCachedListRereadsAFileEditedInPlace(t *testing.T) {
	useCache(t)
	root := t.TempDir()
	path := writeEvent(t, root, "01", "EV-aaaaaa", "claim.task")
	tree := &store.Tree{Root: root}
	_, err := List(tree)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, []byte("id: EV-aaaaaa\ntype: reassign.task\nat: 2026-09-01T10:00:00Z\ncontributor: jc\ncommand: test\n"), 0o644))
	later := time.Now().Add(time.Minute)
	require.NoError(t, os.Chtimes(path, later, later))

	evs, err := List(tree)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"EV-aaaaaa": "reassign.task"}, eventTypes(evs))
}

func TestCachedListForgetsRemovedEvents(t *testing.T) {
	c := useCache(t)
	root := t.TempDir()
	writeEvent(t, root, "01", "EV-aaaaaa", "claim.task")
	gone := writeEvent(t, root, "02", "EV-bbbbbb", "start.task")
	tree := &store.Tree{Root: root}
	_, err := List(tree)
	require.NoError(t, err)

	require.NoError(t, os.Remove(gone))
	evs, err := List(tree)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"EV-aaaaaa": "claim.task"}, eventTypes(evs))
	require.Len(t, c.known(root), 1, "a removed event leaves the cache")
}

func TestCachedListForgetsEventsWhenTheDirectoryVanishes(t *testing.T) {
	c := useCache(t)
	root := t.TempDir()
	writeEvent(t, root, "01", "EV-aaaaaa", "claim.task")
	writeEvent(t, root, "02", "EV-bbbbbb", "start.task")
	tree := &store.Tree{Root: root}
	_, err := List(tree)
	require.NoError(t, err)
	require.Len(t, c.known(root), 2)

	require.NoError(t, os.RemoveAll(filepath.Join(root, ".tiller", "events")))
	evs, err := List(tree)
	require.NoError(t, err)
	require.Empty(t, evs)
	require.Empty(t, c.known(root), "a vanished stream leaves nothing cached")
}

func TestCachedListStillRefusesASymlinkAndPublishesNothing(t *testing.T) {
	c := useCache(t)
	root, outside := t.TempDir(), t.TempDir()
	path := writeEvent(t, root, "01", "EV-aaaaaa", "claim.task")
	tree := &store.Tree{Root: root}
	_, err := List(tree)
	require.NoError(t, err)

	external := filepath.Join(outside, "EV-aaaaaa.yml")
	require.NoError(t, os.WriteFile(external, []byte("id: EV-aaaaaa\ntype: confidential\n"), 0o644))
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Symlink(external, path))

	evs, err := List(tree)
	require.Error(t, err)
	require.Empty(t, evs)
	require.Len(t, c.known(root), 1, "a refused walk leaves the cache as it was")
}

func TestListWithoutTheCacheParsesEveryTime(t *testing.T) {
	processCache.Store(nil)
	root := t.TempDir()
	writeEvent(t, root, "01", "EV-aaaaaa", "claim.task")
	evs, err := List(&store.Tree{Root: root})
	require.NoError(t, err)
	require.Len(t, evs, 1)
}
