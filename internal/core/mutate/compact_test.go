package mutate_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCompactFoldsOnlyDaysOlderThanTheDelay(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	looseEvent(t, root, "EV-bbbbbb", "start.task", "2026-11-03T11:00:00Z")
	looseEvent(t, root, "EV-cccccc", "add.task", "2026-11-04T10:00:00Z")
	looseEvent(t, root, "EV-dddddd", "add.task", "2026-11-05T10:00:00Z")
	looseEvent(t, root, "EV-eeeeee", "add.task", "2026-11-11T09:00:00Z")
	before := listEvents(t, root)

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Len(t, res.Days, 2)
	require.Equal(t, "2026-11-03", res.Days[0].Day)
	require.Equal(t, 2, res.Days[0].Events)
	require.Equal(t, ".tiller/events/2026/11/03", res.Days[0].Directory)
	require.Equal(t, "2026-11-04", res.Days[1].Day)
	require.Equal(t, 3, res.Events)
	require.Zero(t, res.Skipped)
	require.Equal(t, []string{
		".tiller/events/2026/11/03", res.Days[0].Segment,
		".tiller/events/2026/11/04", res.Days[1].Segment,
	}, res.Stage)
	require.NoDirExists(t, filepath.Join(root, ".tiller/events/2026/11/03"))
	require.NoDirExists(t, filepath.Join(root, ".tiller/events/2026/11/04"))
	require.FileExists(t, filepath.Join(root, res.Days[0].Segment))
	require.FileExists(t, filepath.Join(root, res.Days[1].Segment))
	require.FileExists(t, filepath.Join(root, ".tiller/events/2026/11/05/EV-dddddd.yml"))
	require.FileExists(t, filepath.Join(root, ".tiller/events/2026/11/11/EV-eeeeee.yml"))
	requireSameEvents(t, before, listEvents(t, root))
}

func TestCompactHonorsTheManifestDelay(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	appendManifest(t, root, "events:\n  compact_after_days: 2\n")
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	looseEvent(t, root, "EV-cccccc", "add.task", "2026-11-04T10:00:00Z")
	looseEvent(t, root, "EV-dddddd", "add.task", "2026-11-05T10:00:00Z")
	looseEvent(t, root, "EV-eeeeee", "add.task", "2026-11-11T09:00:00Z")

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Equal(t, []string{"2026-11-03", "2026-11-04", "2026-11-05"}, compactedDays(res))
	require.NoDirExists(t, filepath.Join(root, ".tiller/events/2026/11/05"))
	require.FileExists(t, filepath.Join(root, ".tiller/events/2026/11/11/EV-eeeeee.yml"))
}

func TestCompactAllIncludesToday(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	looseEvent(t, root, "EV-eeeeee", "add.task", "2026-11-11T09:00:00Z")
	before := listEvents(t, root)

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z"), All: true})

	require.NoError(t, err)
	require.Equal(t, []string{"2026-11-03", "2026-11-11"}, compactedDays(res))
	require.Empty(t, dayDirectories(t, root))
	requireSameEvents(t, before, listEvents(t, root))
}

func TestCompactUsesTheUTCDay(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-04T23:59:00Z")

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-10T23:59:00Z")})
	require.NoError(t, err)
	require.Empty(t, res.Days)
	require.FileExists(t, filepath.Join(root, ".tiller/events/2026/11/04/EV-aaaaaa.yml"))

	res, err = mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T00:01:00Z")})
	require.NoError(t, err)
	require.Equal(t, []string{"2026-11-04"}, compactedDays(res))
	require.NoDirExists(t, filepath.Join(root, ".tiller/events/2026/11/04"))
}

func TestCompactReadsAnOffsetNowAsItsUTCDay(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-04T23:59:00Z")

	// 2026-11-11 locally, but still 2026-11-10 in UTC.
	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T01:00:00+05:00")})
	require.NoError(t, err)
	require.Empty(t, res.Days)
	require.FileExists(t, filepath.Join(root, ".tiller/events/2026/11/04/EV-aaaaaa.yml"))

	// 2026-11-10 locally, but already 2026-11-11 in UTC.
	res, err = mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-10T20:01:00-05:00")})
	require.NoError(t, err)
	require.Equal(t, []string{"2026-11-04"}, compactedDays(res))
	require.NoDirExists(t, filepath.Join(root, ".tiller/events/2026/11/04"))
}

// TestCompactRefusesWhenSomethingUnreadSitsAtTheSegmentPath: the day's
// segment read loads every segment of the day, so anything already at the
// new segment's path is something no reader counts. Compaction must not
// delete loose files on its strength.
func TestCompactRefusesWhenSomethingUnreadSitsAtTheSegmentPath(t *testing.T) {
	t.Parallel()
	now := at(t, "2026-11-11T12:00:00Z")
	day := func(t *testing.T, root string) {
		looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
		looseEvent(t, root, "EV-bbbbbb", "start.task", "2026-11-03T11:00:00Z")
	}
	// segmentFor compacts an identical day in a separate checkout and returns
	// the segment's path and bytes.
	segmentFor := func(t *testing.T) (string, []byte) {
		other := writeOwnedRepo(t)
		day(t, other)
		res, err := mutate.Compact(other, mutate.CompactOptions{Now: now})
		require.NoError(t, err)
		require.Len(t, res.Days, 1)
		return res.Days[0].Segment, readFile(t, other, res.Days[0].Segment)
	}
	requireRefused := func(t *testing.T, root, segment string) {
		before := eventTree(t, root)
		res, err := mutate.Compact(root, mutate.CompactOptions{Now: now})
		require.Error(t, err)
		require.Contains(t, err.Error(), filepath.FromSlash(segment)+" exists but was not read as a segment of the day")
		require.Empty(t, res.Days)
		require.FileExists(t, filepath.Join(root, ".tiller/events/2026/11/03/EV-aaaaaa.yml"))
		require.FileExists(t, filepath.Join(root, ".tiller/events/2026/11/03/EV-bbbbbb.yml"))
		require.Equal(t, before, eventTree(t, root))
	}

	t.Run("a directory", func(t *testing.T) {
		t.Parallel()
		segment, _ := segmentFor(t)
		root := writeOwnedRepo(t)
		day(t, root)
		require.NoError(t, os.MkdirAll(filepath.Join(root, segment), 0o755))
		requireRefused(t, root, segment)
	})

	t.Run("the same bytes under a case-folded name", func(t *testing.T) {
		t.Parallel()
		segment, data := segmentFor(t)
		root := writeOwnedRepo(t)
		day(t, root)
		dir, name := filepath.Split(filepath.Join(root, segment))
		folded := filepath.Join(dir, strings.ToUpper(strings.TrimSuffix(name, ".jsonl"))+".jsonl")
		require.NoError(t, os.WriteFile(folded, data, 0o644))
		if _, err := os.Stat(filepath.Join(root, segment)); err != nil {
			t.Skip("filesystem is case-sensitive, so the folded name is a different path")
		}
		requireRefused(t, root, segment)
	})
}

func TestCompactLeavesAnEventItCannotCarryLoose(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	odd := looseEvent(t, root, "EV-bbbbbb", "add.task", "2026-11-03T11:00:00Z")
	raw, err := os.ReadFile(odd)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(odd, append(raw, []byte("surprise: 1\n")...), 0o644))
	before := listEvents(t, root)

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Equal(t, 1, res.Skipped)
	require.Len(t, res.Days, 1)
	require.Equal(t, 1, res.Days[0].Events)
	require.FileExists(t, odd)
	require.NoFileExists(t, filepath.Join(root, ".tiller/events/2026/11/03/EV-aaaaaa.yml"))
	require.Equal(t, []string{"EV-aaaaaa"}, segmentIDs(t, root, res.Days[0].Segment))
	require.Equal(t, []string{".tiller/events/2026/11/03/EV-aaaaaa.yml", res.Days[0].Segment}, res.Stage,
		"a directory compaction leaves in place is staged file by file")
	requireSameEvents(t, before, listEvents(t, root))
}

// TestCompactLeavesAnEventWithAnInvalidIDLoose pins that only an event whose
// id is a valid EV- token is carried: an id that is not one, such as one with
// shell metacharacters in it, stays loose, is counted as skipped, and is never
// named in the stage list.
func TestCompactLeavesAnEventWithAnInvalidIDLoose(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	crafted := looseEvent(t, root, "EV-a b;touch PWNED", "add.task", "2026-11-03T11:00:00Z")

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Equal(t, 1, res.Skipped)
	require.Len(t, res.Days, 1)
	require.Equal(t, 1, res.Days[0].Events)
	require.FileExists(t, crafted)
	require.Equal(t, []string{"EV-aaaaaa"}, segmentIDs(t, root, res.Days[0].Segment))
	for _, p := range res.Stage {
		require.NotContains(t, p, "PWNED")
	}
}

// TestCompactLeavesAnEventFiledUnderTheWrongDayLoose pins the dated-path check:
// a loose file whose event belongs to another day is not carried into this
// day's segment. It stays where it is and counts as skipped.
func TestCompactLeavesAnEventFiledUnderTheWrongDayLoose(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	moved := looseEvent(t, root, "EV-bbbbbb", "add.task", "2026-11-04T10:00:00Z")
	misfiled := filepath.Join(root, ".tiller/events/2026/11/03/EV-bbbbbb.yml")
	require.NoError(t, os.Rename(moved, misfiled))
	require.NoError(t, os.Remove(filepath.Dir(moved)))

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Equal(t, 1, res.Skipped)
	require.Len(t, res.Days, 1)
	require.Equal(t, 1, res.Days[0].Events)
	require.FileExists(t, misfiled)
	require.Equal(t, []string{"EV-aaaaaa"}, segmentIDs(t, root, res.Days[0].Segment))
}

// TestCompactKeepsADayDirectoryThatHoldsAnotherFile pins that only a directory
// left empty is removed: a non-.yml entry is not an event, so compaction
// leaves it and the directory that holds it.
func TestCompactKeepsADayDirectoryThatHoldsAnotherFile(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	loose := looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	notes := filepath.Join(filepath.Dir(loose), "notes.txt")
	require.NoError(t, os.WriteFile(notes, []byte("keep me\n"), 0o644))

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Zero(t, res.Skipped)
	require.Equal(t, []string{"2026-11-03"}, compactedDays(res))
	require.NoFileExists(t, loose)
	require.FileExists(t, notes)
	require.Equal(t, []string{"EV-aaaaaa"}, segmentIDs(t, root, res.Days[0].Segment))
}

func TestCompactRemovesALooseCopyAlreadyInASegment(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	first, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})
	require.NoError(t, err)
	segment := first.Days[0].Segment
	segmentBytes := readFile(t, root, segment)
	loose := looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Len(t, res.Days, 1)
	require.Equal(t, 1, res.Days[0].Events)
	require.Empty(t, res.Days[0].Segment)
	require.Equal(t, []string{".tiller/events/2026/11/03"}, res.Stage)
	require.NoFileExists(t, loose)
	require.Equal(t, segmentBytes, readFile(t, root, segment))
	require.Len(t, listEvents(t, root), 1)
}

func TestCompactAddsAStragglerSegmentWithoutTouchingTheFirst(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	first, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})
	require.NoError(t, err)
	firstSegment := first.Days[0].Segment
	firstBytes := readFile(t, root, firstSegment)
	looseEvent(t, root, "EV-bbbbbb", "start.task", "2026-11-03T15:00:00Z")

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-12T12:00:00Z")})

	require.NoError(t, err)
	require.Len(t, res.Days, 1)
	second := res.Days[0].Segment
	require.NotEmpty(t, second)
	require.NotEqual(t, firstSegment, second)
	require.Equal(t, firstBytes, readFile(t, root, firstSegment))
	require.Equal(t, []string{"EV-bbbbbb"}, segmentIDs(t, root, second))
	got := listEvents(t, root)
	require.Len(t, got, 2)
	require.Equal(t, "EV-aaaaaa", got[0].ID)
	require.Equal(t, "EV-bbbbbb", got[1].ID)
}

func TestCompactRefusesADayWhoseCopiesDisagree(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	first, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})
	require.NoError(t, err)
	segment := first.Days[0].Segment
	looseEvent(t, root, "EV-aaaaaa", "start.task", "2026-11-03T10:00:00Z")
	before := eventTree(t, root)

	_, err = mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.Error(t, err)
	require.Contains(t, err.Error(), filepath.FromSlash(segment))
	require.Contains(t, err.Error(), filepath.FromSlash(".tiller/events/2026/11/03/EV-aaaaaa.yml"))
	require.Equal(t, before, eventTree(t, root))
}

// TestCompactLeavesARefusedDayAndCompactsTheNextOne pins that one day that
// refuses does not hold every newer day back: the refused day is left exactly
// as it was, the newer day compacts, and the returned error names the refused
// day.
func TestCompactLeavesARefusedDayAndCompactsTheNextOne(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	first, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})
	require.NoError(t, err)
	segment := first.Days[0].Segment
	looseEvent(t, root, "EV-aaaaaa", "start.task", "2026-11-03T10:00:00Z")
	looseEvent(t, root, "EV-bbbbbb", "add.task", "2026-11-04T10:00:00Z")
	refused := eventTree(t, root)

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.Error(t, err)
	require.Contains(t, err.Error(), "2026-11-03")
	require.Contains(t, err.Error(), filepath.FromSlash(segment))
	require.Contains(t, err.Error(), filepath.FromSlash(".tiller/events/2026/11/03/EV-aaaaaa.yml"))
	require.Equal(t, []string{"2026-11-04"}, compactedDays(res))
	require.Equal(t, 1, res.Events)
	require.Equal(t, []string{"EV-bbbbbb"}, segmentIDs(t, root, res.Days[0].Segment))
	require.NoDirExists(t, filepath.Join(root, ".tiller/events/2026/11/04"))
	after := eventTree(t, root)
	for path, raw := range refused {
		if strings.HasPrefix(filepath.ToSlash(path), "2026/11/03") || path == filepath.Base(segment) {
			require.Equal(t, raw, after[path], path)
		}
	}
	require.FileExists(t, filepath.Join(root, ".tiller/events/2026/11/03/EV-aaaaaa.yml"))
}

// TestCompactJoinsTheErrorOfEveryRefusedDay pins that each refused day's error
// reaches the caller, not only the first.
func TestCompactJoinsTheErrorOfEveryRefusedDay(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	looseEvent(t, root, "EV-bbbbbb", "add.task", "2026-11-04T10:00:00Z")
	_, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})
	require.NoError(t, err)
	looseEvent(t, root, "EV-aaaaaa", "start.task", "2026-11-03T10:00:00Z")
	looseEvent(t, root, "EV-bbbbbb", "start.task", "2026-11-04T10:00:00Z")

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.Error(t, err)
	require.Contains(t, err.Error(), "2026-11-03")
	require.Contains(t, err.Error(), "2026-11-04")
	require.Empty(t, res.Days)
}

func TestCompactIsANoOpWhenNothingIsEligible(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	_, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})
	require.NoError(t, err)
	looseEvent(t, root, "EV-eeeeee", "add.task", "2026-11-11T09:00:00Z")
	generation := readFile(t, root, ".tiller/.cache/generation")
	before := eventTree(t, root)

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Empty(t, res.Days)
	require.Empty(t, res.Stage)
	require.Equal(t, generation, readFile(t, root, ".tiller/.cache/generation"))
	require.Equal(t, before, eventTree(t, root))
}

func TestCompactDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	looseEvent(t, root, "EV-bbbbbb", "add.task", "2026-11-03T11:00:00Z")
	before := eventTree(t, root)

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z"), DryRun: true})

	require.NoError(t, err)
	require.Equal(t, []string{"2026-11-03"}, compactedDays(res))
	require.Equal(t, 2, res.Events)
	require.NotEmpty(t, res.Days[0].Segment)
	require.Equal(t, []string{".tiller/events/2026/11/03", res.Days[0].Segment}, res.Stage)
	require.Equal(t, before, eventTree(t, root))

	real, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})
	require.NoError(t, err)
	require.Equal(t, res, real)
}

func TestCompactRecordsNoEvent(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	looseEvent(t, root, "EV-bbbbbb", "add.task", "2026-11-04T10:00:00Z")
	before := listEvents(t, root)

	_, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Len(t, listEvents(t, root), len(before))
}

func TestTwoCheckoutsCompactingTheSameDayWriteTheSameSegment(t *testing.T) {
	t.Parallel()
	left, right := writeOwnedRepo(t), writeOwnedRepo(t)
	for _, root := range []string{left, right} {
		looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
		looseEvent(t, root, "EV-bbbbbb", "start.task", "2026-11-03T11:00:00Z")
	}

	leftRes, err := mutate.Compact(left, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})
	require.NoError(t, err)
	rightRes, err := mutate.Compact(right, mutate.CompactOptions{Now: at(t, "2026-11-20T12:00:00Z")})
	require.NoError(t, err)

	require.Len(t, leftRes.Days, 1)
	require.Len(t, rightRes.Days, 1)
	require.Equal(t, leftRes.Days[0].Segment, rightRes.Days[0].Segment)
	require.Equal(t, readFile(t, left, leftRes.Days[0].Segment), readFile(t, right, rightRes.Days[0].Segment))
}

// TestCompactAfterPullingAnotherCheckoutsSegmentOnlyRemovesLooseCopies pins
// the recovery for two checkouts that compacted the same day: once the
// committed segment is pulled in, the next compaction deletes the loose
// copies it holds and writes nothing new.
func TestCompactAfterPullingAnotherCheckoutsSegmentOnlyRemovesLooseCopies(t *testing.T) {
	t.Parallel()
	pushed, pulling := writeOwnedRepo(t), writeOwnedRepo(t)
	for _, root := range []string{pushed, pulling} {
		looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
		looseEvent(t, root, "EV-bbbbbb", "start.task", "2026-11-03T11:00:00Z")
	}
	first, err := mutate.Compact(pushed, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})
	require.NoError(t, err)
	segment := first.Days[0].Segment
	segmentBytes := readFile(t, pushed, segment)
	require.NoError(t, os.WriteFile(filepath.Join(pulling, segment), segmentBytes, 0o644))
	before := listEvents(t, pulling)

	res, err := mutate.Compact(pulling, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Len(t, res.Days, 1)
	require.Equal(t, 2, res.Days[0].Events)
	require.Empty(t, res.Days[0].Segment, "no new segment is written")
	require.Equal(t, segmentBytes, readFile(t, pulling, segment))
	segments, err := filepath.Glob(filepath.Join(pulling, ".tiller", "events", "2026", "11", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, segments, 1)
	require.NoDirExists(t, filepath.Join(pulling, ".tiller/events/2026/11/03"))
	requireSameEvents(t, before, listEvents(t, pulling))
}

func TestCompactCarriesALegacyOwnerKeyAsContributor(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	path := looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	legacy := strings.Replace(string(raw), "contributor: jc", "owner: jc", 1)
	require.NotEqual(t, string(raw), legacy)
	require.NoError(t, os.WriteFile(path, []byte(legacy), 0o644))
	before := listEvents(t, root)

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Len(t, res.Days, 1)
	segment := string(readFile(t, root, res.Days[0].Segment))
	require.Contains(t, segment, `"contributor":"jc"`)
	require.NotContains(t, segment, "owner")
	requireSameEvents(t, before, listEvents(t, root))
}

// looseEvent writes one event as a loose YAML file at its RelPath and returns
// the file's absolute path.
func looseEvent(t *testing.T, root, id, typ, when string) string {
	t.Helper()
	e := events.Event{
		ID:          id,
		Type:        typ,
		At:          at(t, when),
		Contributor: "jc",
		Command:     "tiller " + typ,
		Targets:     []string{"T-aaaaaa"},
		Before:      map[string]any{"status": "ready"},
		After:       map[string]any{"status": "active", "attempt": 2},
		Files: []events.FileChange{{
			Path:  ".tiller/epics/e/feature.yml",
			After: store.RevisionRef{Kind: "git_blob", Value: "0123456789abcdef0123456789abcdef01234567"},
		}},
	}
	data, err := yaml.Marshal(e)
	require.NoError(t, err)
	path := filepath.Join(root, events.RelPath(e))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func at(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed
}

func listEvents(t *testing.T, root string) []events.Event {
	t.Helper()
	tree, err := store.Load(root)
	require.NoError(t, err)
	evs, err := events.List(tree)
	require.NoError(t, err)
	return evs
}

func requireSameEvents(t *testing.T, want, got []events.Event) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		require.True(t, events.Equal(want[i], got[i]), "event %d differs: want %+v, got %+v", i, want[i], got[i])
	}
}

func appendManifest(t *testing.T, root, text string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "tiller.yml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(raw, []byte(text)...), 0o644))
}

func compactedDays(res mutate.CompactResult) []string {
	var days []string
	for _, d := range res.Days {
		days = append(days, d.Day)
	}
	return days
}

func readFile(t *testing.T, root, relative string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, relative))
	require.NoError(t, err)
	return raw
}

func segmentIDs(t *testing.T, root, relative string) []string {
	t.Helper()
	evs, err := events.DecodeSegment(relative, readFile(t, root, relative))
	require.NoError(t, err)
	var ids []string
	for _, e := range evs {
		ids = append(ids, e.ID)
	}
	return ids
}

// eventTree maps every path under .tiller/events to its bytes, with
// directories mapped to nil, so a test can assert that nothing changed.
func eventTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	base := filepath.Join(root, ".tiller", "events")
	require.NoError(t, filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			out[rel] = nil
			return nil
		}
		raw, err := os.ReadFile(path)
		out[rel] = raw
		return err
	}))
	return out
}

// dayDirectories lists the YYYY/MM/DD directories under .tiller/events.
func dayDirectories(t *testing.T, root string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, ".tiller", "events", "*", "*", "*"))
	require.NoError(t, err)
	var dirs []string
	for _, m := range matches {
		info, err := os.Stat(m)
		require.NoError(t, err)
		if info.IsDir() {
			dirs = append(dirs, m)
		}
	}
	return dirs
}
