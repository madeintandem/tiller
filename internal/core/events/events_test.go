package events

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// fixedClock is a deterministic Clock for tests: every Now is the same instant.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// eventsDir is where the dated stream lives under a tree root.
func eventsDir(root string) string {
	return filepath.Join(root, ".tiller", "events")
}
func commitEvent(t *testing.T, tree *store.Tree, e Event) {
	t.Helper()
	path, data, err := File(tree, e)
	require.NoError(t, err)
	require.NoError(t, tree.Commit("test event", []store.PlannedWrite{
		store.PlanWrite(path, store.AbsentSnapshot(), data),
	}))
}

func TestAppendWritesOneDatedFileWithMintedID(t *testing.T) {
	dir := t.TempDir()
	tree := &store.Tree{Root: dir}
	clk := fixedClock{t: at("2026-06-27T14:30:00Z")}

	e := NewEvent(clk, "add.feature", "sam", "add feature form-engine/nrca-form-loading", "F-79dh3q")
	commitEvent(t, tree, e)

	// The file lands at events/YYYY/MM/DD/ derived from the injected clock.
	dayDir := filepath.Join(eventsDir(dir), "2026", "06", "27")
	entries, err := os.ReadDir(dayDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one event file for one Append")

	name := entries[0].Name()
	require.True(t, regexp.MustCompile(`^EV-[a-z0-9]{6}\.yml$`).MatchString(name), "filename is a minted EV- id, got %q", name)
	mintedID := name[:len(name)-len(".yml")]
	require.True(t, id.Valid(mintedID), "minted id is valid: %q", mintedID)

	// The stored event round-trips and carries the minted id and injected time.
	evs, err := List(tree)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	require.Equal(t, mintedID, evs[0].ID)
	require.Equal(t, "add.feature", evs[0].Type)
	require.Equal(t, "sam", evs[0].Contributor)
	require.Equal(t, []string{"F-79dh3q"}, evs[0].Targets)
	require.True(t, clk.Now().Equal(evs[0].At), "stored At matches the injected clock")
}

func TestAppendSerializesBeforeAfterMapsAndFileRevisionRefs(t *testing.T) {
	dir := t.TempDir()
	tree := &store.Tree{Root: dir}
	clk := fixedClock{t: at("2026-06-27T09:00:00Z")}

	e := NewEvent(clk, "spec.feature", "jc", "spec feature form-engine/nrca-form-loading", "F-79dh3q")
	e.ID = "EV-aaaaaa" // fixed id so the assertion is deterministic
	e.Before = map[string]any{"status": "draft"}
	e.After = map[string]any{"status": "specced"}
	e.Files = append(e.Files, FileChange{
		Path:   "epics/form-engine/features/nrca-form-loading/feature.md",
		Before: store.RevisionRef{Kind: store.KindGitBlob, Value: "git:31fd9c2", Path: "epics/form-engine/features/nrca-form-loading/feature.md", ObservedAt: at("2026-06-27T09:00:00Z")},
		After:  store.RevisionRef{Kind: store.KindGitBlob, Value: "git:b71a322", Path: "epics/form-engine/features/nrca-form-loading/feature.md", ObservedAt: at("2026-06-27T09:00:00Z")},
	})
	commitEvent(t, tree, e)

	evs, err := List(tree)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	got := evs[0]

	require.Equal(t, map[string]any{"status": "draft"}, got.Before)
	require.Equal(t, map[string]any{"status": "specced"}, got.After)
	require.Len(t, got.Files, 1)
	require.Equal(t, "epics/form-engine/features/nrca-form-loading/feature.md", got.Files[0].Path)
	require.Equal(t, store.KindGitBlob, got.Files[0].Before.Kind)
	require.Equal(t, "git:31fd9c2", got.Files[0].Before.Value)
	require.Equal(t, "git:b71a322", got.Files[0].After.Value)
	require.Equal(t, "epics/form-engine/features/nrca-form-loading/feature.md", got.Files[0].After.Path)
}

func TestSecondAppendCreatesNewFileNeverEditsSharedLog(t *testing.T) {
	dir := t.TempDir()
	tree := &store.Tree{Root: dir}
	clk := fixedClock{t: at("2026-06-27T12:00:00Z")}

	first := NewEvent(clk, "start.feature", "sam", "start feature", "F-79dh3q")
	first.ID = "EV-111111"
	commitEvent(t, tree, first)

	firstPath := filepath.Join(eventsDir(dir), "2026", "06", "27", "EV-111111.yml")
	before, err := os.ReadFile(firstPath)
	require.NoError(t, err)

	second := NewEvent(clk, "submit.task", "sam", "submit task", "T-k3p9w6")
	second.ID = "EV-222222"
	commitEvent(t, tree, second)

	// The first file is untouched: append is by file addition, not log edit.
	after, err := os.ReadFile(firstPath)
	require.NoError(t, err)
	require.Equal(t, before, after, "first event file must not be rewritten by a second Append")

	// Both files exist side by side.
	require.FileExists(t, filepath.Join(eventsDir(dir), "2026", "06", "27", "EV-222222.yml"))
	evs, err := List(tree)
	require.NoError(t, err)
	require.Len(t, evs, 2)
}

func TestListSortsByTimestampThenID(t *testing.T) {
	dir := t.TempDir()
	tree := &store.Tree{Root: dir}

	// Append out of order. Two events share a timestamp so the id tiebreak shows.
	mk := func(idv, ts string) Event {
		e := NewEvent(fixedClock{t: at(ts)}, "x", "jc", "cmd")
		e.ID = idv
		return e
	}
	commitEvent(t, tree, mk("EV-bbbbbb", "2026-06-27T10:00:00Z"))
	commitEvent(t, tree, mk("EV-zzzzzz", "2026-06-25T10:00:00Z"))
	commitEvent(t, tree, mk("EV-aaaaaa", "2026-06-27T10:00:00Z"))

	evs, err := List(tree)
	require.NoError(t, err)
	require.Len(t, evs, 3)

	gotIDs := []string{evs[0].ID, evs[1].ID, evs[2].ID}
	// Earliest day first; within the shared 06-27 timestamp, id ascends.
	require.Equal(t, []string{"EV-zzzzzz", "EV-aaaaaa", "EV-bbbbbb"}, gotIDs)
}

func TestAppendRefusesUnstampedEvent(t *testing.T) {
	dir := t.TempDir()
	tree := &store.Tree{Root: dir}

	// An event with a zero timestamp has no dated path; File refuses rather
	// than reaching for an uncontrolled wall clock.
	_, _, err := File(tree, Event{ID: "EV-aaaaaa", Type: "x"})
	require.Error(t, err)
}

func TestSystemClockReturnsUTC(t *testing.T) {
	now := SystemClock{}.Now()
	require.Equal(t, time.UTC, now.Location())
}

// TestLoadReadsOneEventFile is the single-file read `tiller readout <seam>
// --since <sha>` makes for each event git names as added since the sha: the
// same parse List performs over the whole stream, applied to one path.
func TestLoadReadsOneEventFile(t *testing.T) {
	dir := t.TempDir()
	tree := &store.Tree{Root: dir}
	e := NewEvent(fixedClock{t: at("2026-06-27T10:00:00Z")}, "add.feature", "jc", "add feature form-engine/loading", "F-abcdef")
	e.ID = "EV-aaaaaa"
	e.After = map[string]any{"status": "draft", "key": "form-engine/loading"}
	commitEvent(t, tree, e)

	got, err := Load(dir, filepath.Join(".tiller", "events", "2026", "06", "27", "EV-aaaaaa.yml"))
	require.NoError(t, err)
	require.Equal(t, "EV-aaaaaa", got.ID)
	require.Equal(t, "add.feature", got.Type)
	require.Equal(t, []string{"F-abcdef"}, got.Targets)
	require.Equal(t, "form-engine/loading", got.After["key"])
}

func TestLoadNamesAnUnparseableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "EV-broken.yml")
	require.NoError(t, os.WriteFile(path, []byte("id: [unclosed\n"), 0o644))
	_, err := Load(dir, "EV-broken.yml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "EV-broken.yml")
}

func TestEventReadsRefuseEscapingPaths(t *testing.T) {
	for _, component := range []string{"leaf", "dated-directory", "events-directory"} {
		t.Run(component, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			relative := filepath.Join(".tiller", "events", "2026", "06", "27", "EV-private.yml")
			external := filepath.Join(outside, "EV-private.yml")
			require.NoError(t, os.WriteFile(external, []byte("id: EV-private\ntype: confidential\n"), 0o644))
			switch component {
			case "leaf":
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, relative)), 0o755))
				require.NoError(t, os.Symlink(external, filepath.Join(root, relative)))
			case "dated-directory":
				parent := filepath.Join(root, ".tiller", "events", "2026", "06")
				require.NoError(t, os.MkdirAll(parent, 0o755))
				require.NoError(t, os.Symlink(outside, filepath.Join(parent, "27")))
			case "events-directory":
				require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
				require.NoError(t, os.Symlink(outside, filepath.Join(root, ".tiller", "events")))
				relative = filepath.Join(".tiller", "events", "EV-private.yml")
			}

			got, err := Load(root, relative)
			require.Error(t, err)
			require.Empty(t, got.ID)
			stream, err := List(&store.Tree{Root: root})
			require.Error(t, err)
			require.Empty(t, stream, "a refused stream must not publish partial event data")
		})
	}
}

func TestLoadRefusesParentTraversal(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "checkout")
	require.NoError(t, os.Mkdir(root, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "EV-private.yml"), []byte("id: EV-private\n"), 0o644))
	event, err := Load(root, "../EV-private.yml")
	require.Error(t, err)
	require.Empty(t, event.ID)
}

func TestEventAttributionPreservesHistoryAndWritesContributor(t *testing.T) {
	root := t.TempDir()
	relative := ".tiller/events/2026/10/06/EV-aaaaaa.yml"
	path := filepath.Join(root, relative)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	legacy := []byte("id: EV-aaaaaa\ntype: owner.add\nat: 2026-10-06T12:00:00Z\nowner: jc\ncommand: owner add jc\n")
	require.NoError(t, os.WriteFile(path, legacy, 0o644))

	historical, err := Load(root, relative)
	require.NoError(t, err)
	require.Equal(t, "jc", historical.Contributor)
	require.Equal(t, "owner.add", historical.Type)
	unchanged, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, legacy, unchanged)

	tree := &store.Tree{Root: root}
	fresh := NewEvent(fixedClock{at("2026-10-06T13:00:00Z")}, "contributor.add", "dana", "add contributor dana")
	fresh.ID = "EV-bbbbbb"
	newPath, raw, err := File(tree, fresh)
	require.NoError(t, err)
	require.Contains(t, string(raw), "\ncontributor: dana\n")
	require.NotContains(t, string(raw), "\nowner:")
	require.NoError(t, os.WriteFile(newPath, raw, 0o644))
	newRelative, err := filepath.Rel(root, newPath)
	require.NoError(t, err)
	loaded, err := Load(root, newRelative)
	require.NoError(t, err)
	require.Equal(t, "dana", loaded.Contributor)
}

func TestEventRefusesAmbiguousActorAttribution(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "event.yml")
	raw := []byte("id: EV-aaaaaa\ntype: add.task\nat: 2026-10-06T12:00:00Z\nowner: jc\ncontributor: dana\ncommand: add task\n")
	require.NoError(t, os.WriteFile(path, raw, 0o644))
	_, err := Load(root, "event.yml")
	require.ErrorContains(t, err, "both owner and contributor")
	unchanged, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, raw, unchanged)
}

// writeEventAt commits an event of the given type at the given instant.
func writeEventAt(t *testing.T, tree *store.Tree, evID, typ, when string) {
	t.Helper()
	e := NewEvent(fixedClock{t: at(when)}, typ, "jc", "test", "F-aaaaaa")
	e.ID = evID
	commitEvent(t, tree, e)
}

func eventIDs(evs []Event) []string {
	ids := make([]string, 0, len(evs))
	for _, e := range evs {
		ids = append(ids, e.ID)
	}
	return ids
}

func TestListSinceSkipsOlderPartitions(t *testing.T) {
	dir := t.TempDir()
	tree := &store.Tree{Root: dir}
	writeEventAt(t, tree, "EV-aaaaaa", "claim.task", "2026-10-01T10:00:00Z")
	writeEventAt(t, tree, "EV-bbbbbb", "start.task", "2026-10-10T10:00:00Z")
	writeEventAt(t, tree, "EV-cccccc", "submit.task", "2026-10-20T10:00:00Z")

	evs, err := ListSince(tree, at("2026-10-07T00:00:00Z"))
	require.NoError(t, err)
	require.Equal(t, []string{"EV-bbbbbb", "EV-cccccc"}, eventIDs(evs))

	old := filepath.Join(eventsDir(dir), "2026", "10", "01", "EV-aaaaaa.yml")
	require.NoError(t, os.WriteFile(old, []byte("id: [unterminated\n"), 0o644))

	evs, err = ListSince(tree, at("2026-10-07T00:00:00Z"))
	require.NoError(t, err, "an older partition is never read")
	require.Equal(t, []string{"EV-bbbbbb", "EV-cccccc"}, eventIDs(evs))
	_, err = List(tree)
	require.Error(t, err, "List still reads every partition")
}

func TestListSincePrunesOlderMonthsAndYears(t *testing.T) {
	dir := t.TempDir()
	tree := &store.Tree{Root: dir}
	writeEventAt(t, tree, "EV-aaaaaa", "claim.task", "2025-12-31T10:00:00Z")
	writeEventAt(t, tree, "EV-bbbbbb", "claim.task", "2026-09-30T10:00:00Z")
	writeEventAt(t, tree, "EV-cccccc", "claim.task", "2026-10-01T10:00:00Z")
	writeEventAt(t, tree, "EV-dddddd", "claim.task", "2026-11-02T10:00:00Z")
	for _, bad := range []string{"2025/12/31/EV-aaaaaa.yml", "2026/09/30/EV-bbbbbb.yml"} {
		require.NoError(t, os.WriteFile(filepath.Join(eventsDir(dir), filepath.FromSlash(bad)), []byte("id: [x\n"), 0o644))
	}

	evs, err := ListSince(tree, at("2026-10-01T00:00:00Z"))
	require.NoError(t, err)
	require.Equal(t, []string{"EV-cccccc", "EV-dddddd"}, eventIDs(evs))
}

func TestListSinceUsesTheUTCPartitionForTheBoundaryDay(t *testing.T) {
	dir := t.TempDir()
	tree := &store.Tree{Root: dir}
	writeEventAt(t, tree, "EV-aaaaaa", "claim.task", "2026-10-21T06:30:00Z")

	evs, err := ListSince(tree, at("2026-10-20T23:00:00-08:00"))
	require.NoError(t, err)
	require.Empty(t, evs, "the boundary is 07:00Z, after the event")

	evs, err = ListSince(tree, at("2026-10-20T22:00:00-08:00"))
	require.NoError(t, err)
	require.Len(t, evs, 1, "the boundary is 06:00Z, before the event")
}

func TestListSinceMissingEventsDirIsEmpty(t *testing.T) {
	evs, err := ListSince(&store.Tree{Root: t.TempDir()}, at("2026-10-07T00:00:00Z"))
	require.NoError(t, err)
	require.Nil(t, evs)
}

func TestListSinceKeepsTheCacheHonestForList(t *testing.T) {
	c := useCache(t)
	root := t.TempDir()
	tree := &store.Tree{Root: root}
	writeEventAt(t, tree, "EV-aaaaaa", "claim.task", "2026-10-01T10:00:00Z")
	writeEventAt(t, tree, "EV-bbbbbb", "start.task", "2026-10-20T10:00:00Z")

	_, err := List(tree)
	require.NoError(t, err)
	require.Equal(t, 2, c.misses())

	evs, err := ListSince(tree, at("2026-10-07T00:00:00Z"))
	require.NoError(t, err)
	require.Len(t, evs, 1)
	require.Equal(t, 2, c.misses(), "a windowed read reuses the cache")
	require.Len(t, c.known(root), 2, "a windowed read evicts nothing")

	evs, err = List(tree)
	require.NoError(t, err)
	require.Len(t, evs, 2)
	require.Equal(t, 2, c.misses(), "List after a windowed read reparses nothing")
}

func TestListSinceCachesWhatItParsed(t *testing.T) {
	c := useCache(t)
	root := t.TempDir()
	tree := &store.Tree{Root: root}
	writeEventAt(t, tree, "EV-aaaaaa", "claim.task", "2026-10-01T10:00:00Z")
	writeEventAt(t, tree, "EV-bbbbbb", "start.task", "2026-10-20T10:00:00Z")
	since := at("2026-10-07T00:00:00Z")

	_, err := ListSince(tree, since)
	require.NoError(t, err)
	_, err = ListSince(tree, since)
	require.NoError(t, err)
	require.Equal(t, 1, c.misses(), "the window is parsed once")
	require.Len(t, c.known(root), 1, "only the window was cached")
}

// BenchmarkList reads a stream shaped like a busy project: 2,000 events
// across 40 dated directories.
func BenchmarkList(b *testing.B) {
	root := b.TempDir()
	for day := 0; day < 40; day++ {
		dir := filepath.Join(root, ".tiller", "events", "2026", "09", fmt.Sprintf("%02d", day%28+1))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		for i := 0; i < 50; i++ {
			id := fmt.Sprintf("EV-%02d%04d", day, i)
			data := fmt.Sprintf("id: %s\ntype: claim.task\nat: 2026-09-%02dT10:00:00Z\ncontributor: jc\ncommand: claim\ntargets: [T-aaaaaa]\n", id, day%28+1)
			if err := os.WriteFile(filepath.Join(dir, id+".yml"), []byte(data), 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	tree := &store.Tree{Root: root}
	b.ResetTimer()
	for b.Loop() {
		if _, err := List(tree); err != nil {
			b.Fatal(err)
		}
	}
}
