package execution_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/execution"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// segmentEvent is a check event for the first task at the given second of
// 2026-10-06 UTC.
func segmentEvent(id string, second int) events.Event {
	return events.Event{
		ID:          id,
		Type:        "check.task",
		At:          time.Date(2026, 10, 6, 12, 0, second, 0, time.UTC),
		Contributor: "sam",
		Command:     "check task",
		Targets:     []string{firstID},
	}
}

// putSegment compacts evs, all on 2026-10-06, into one segment under root and
// returns its path.
func putSegment(t *testing.T, root string, evs ...events.Event) string {
	t.Helper()
	data, err := events.EncodeSegment(evs)
	require.NoError(t, err)
	path := events.SegmentRelPath(evs[0].At, data)
	put(t, root, path, string(data))
	return path
}

// putLoose writes ev at its dated loose path under root.
func putLoose(t *testing.T, root string, ev events.Event) {
	t.Helper()
	data, err := yaml.Marshal(ev)
	require.NoError(t, err)
	put(t, root, events.RelPath(ev), string(data))
}

// branchWith starts the first task's branch, runs prepare on its worktree, and
// serves the branch as a worktree source, or as a committed branch with its
// worktree removed.
func branchWith(t *testing.T, committed bool, prepare func(wt string)) *execution.Snapshot {
	t.Helper()
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "in_review")
	prepare(wt)
	if committed {
		git(t, wt, "add", ".")
		git(t, wt, "commit", "-m", "submitted execution")
		git(t, root, "worktree", "remove", wt)
	}
	s := load(t, base(t, root))
	require.Contains(t, s.Observations, firstID)
	return s
}

func TestCommittedSourceReadsATasksEventsFromASegment(t *testing.T) {
	s := branchWith(t, true, func(wt string) {
		putSegment(t, wt, segmentEvent("EV-seg002", 2), segmentEvent("EV-seg001", 1), segmentEvent("EV-seg003", 3))
	})
	evs, err := s.Events(context.Background(), firstID)
	require.NoError(t, err)
	require.Equal(t, []string{"EV-seg001", "EV-seg002", "EV-seg003"}, eventIDs(evs))
}

func TestWorktreeSourceReadsATasksEventsFromASegment(t *testing.T) {
	s := branchWith(t, false, func(wt string) {
		putSegment(t, wt, segmentEvent("EV-seg002", 2), segmentEvent("EV-seg001", 1), segmentEvent("EV-seg003", 3))
	})
	evs, err := s.Events(context.Background(), firstID)
	require.NoError(t, err)
	require.Equal(t, []string{"EV-seg001", "EV-seg002", "EV-seg003"}, eventIDs(evs))
}

func TestSourceCountsAnEventOnceWhenLooseAndCompacted(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "worktree", true: "committed"}[committed], func(t *testing.T) {
			s := branchWith(t, committed, func(wt string) {
				ev := segmentEvent("EV-seg001", 1)
				putSegment(t, wt, ev)
				putLoose(t, wt, ev)
			})
			evs, err := s.Events(context.Background(), firstID)
			require.NoError(t, err)
			require.Len(t, evs, 1)
		})
	}
}

func TestSourceRefusesTwoDifferentRecordsForOneEventID(t *testing.T) {
	s := branchWith(t, false, func(wt string) {
		putSegment(t, wt, segmentEvent("EV-seg001", 1))
		changed := segmentEvent("EV-seg001", 1)
		changed.Type = "start.task"
		putLoose(t, wt, changed)
	})
	_, err := s.Events(context.Background(), firstID)
	require.ErrorContains(t, err, "EV-seg001")
	require.ErrorContains(t, err, ".jsonl")
	require.ErrorContains(t, err, "EV-seg001.yml")
}

func TestBranchHoldingIntegrationsOwnershipLooseStandsWhenIntegrationCompactedIt(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "worktree", true: "committed"}[committed], func(t *testing.T) {
			root := fixture(t)
			put(t, root, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\nowner: sam\n")
			putSegment(t, root, claimEvent())
			git(t, root, "add", ".")
			git(t, root, "commit", "-m", "sam claims alfa, compacted")
			wt := worktree(t, root, firstKey)
			// The branch still carries its own loose copy of integration's claim,
			// so its unevented release is the newer ownership record.
			ownershipEvent(t, wt, "EV-claim1", "claim.task", "2026-10-06T10:00:00Z", "sam")
			put(t, wt, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\nstarted: false\n")
			if committed {
				git(t, wt, "add", ".")
				git(t, wt, "commit", "-m", "released on the branch")
				git(t, root, "worktree", "remove", wt)
			}
			s := load(t, base(t, root))
			require.Contains(t, s.Observations, firstID)
			require.Empty(t, s.Tree.Tasks[firstID].Owner)
			require.Equal(t, "sam", s.Base.Tasks[firstID].Owner)
		})
	}
}

func TestBranchHoldingIntegrationsOwnershipInASegmentStands(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "worktree", true: "committed"}[committed], func(t *testing.T) {
			root := fixture(t)
			put(t, root, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\nowner: sam\n")
			ownershipEvent(t, root, "EV-claim1", "claim.task", "2026-10-06T10:00:00Z", "sam")
			git(t, root, "add", ".")
			git(t, root, "commit", "-m", "sam claims alfa")
			wt := worktree(t, root, firstKey)
			// The branch compacted integration's claim into a segment, so the
			// claim is still its own, and its unevented release is newer.
			require.NoError(t, os.Remove(filepath.Join(wt, events.RelPath(claimEvent()))))
			putSegment(t, wt, claimEvent())
			put(t, wt, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\nstarted: false\n")
			if committed {
				git(t, wt, "add", ".")
				git(t, wt, "commit", "-m", "released on the branch")
				git(t, root, "worktree", "remove", wt)
			}
			s := load(t, base(t, root))
			require.Contains(t, s.Observations, firstID)
			require.Empty(t, s.Tree.Tasks[firstID].Owner)
			require.Equal(t, "sam", s.Base.Tasks[firstID].Owner)
		})
	}
}

// claimEvent is the claim ownershipEvent writes loose as EV-claim1.
func claimEvent() events.Event {
	return events.Event{
		ID:          "EV-claim1",
		Type:        "claim.task",
		At:          time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC),
		Contributor: "sam",
		Targets:     []string{firstID},
	}
}

// committedSegmentSource commits a branch whose events all live in segments:
// one segment per day over the given number of days, each holding perDay events.
func committedSegmentSource(t *testing.T, days, perDay int) *execution.Snapshot {
	t.Helper()
	return branchWith(t, true, func(wt string) {
		for day := range days {
			var evs []events.Event
			for i := range perDay {
				ev := segmentEvent(fmt.Sprintf("EV-d%02d%03d", day, i), i)
				ev.At = ev.At.AddDate(0, 0, day)
				evs = append(evs, ev)
			}
			data, err := events.EncodeSegment(evs)
			require.NoError(t, err)
			put(t, wt, events.SegmentRelPath(evs[0].At, data), string(data))
		}
	})
}

// TestSourceReadsALooseEventInTheShapeItsCompactedCopyHas pins one decode
// boundary: a timestamp inside a loose event's after map reads as the same
// RFC 3339 string a segment carries, so an event's Go types do not change
// when it is compacted.
func TestSourceReadsALooseEventInTheShapeItsCompactedCopyHas(t *testing.T) {
	ev := segmentEvent("EV-seg001", 1)
	ev.After = map[string]any{"due": time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC), "ratio": 2.0}
	read := func(committed, compacted bool) events.Event {
		s := branchWith(t, committed, func(wt string) {
			if compacted {
				putSegment(t, wt, ev)
			} else {
				putLoose(t, wt, ev)
			}
		})
		evs, err := s.Events(context.Background(), firstID)
		require.NoError(t, err)
		require.Len(t, evs, 1)
		return evs[0]
	}
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "worktree", true: "committed"}[committed], func(t *testing.T) {
			loose, compacted := read(committed, false), read(committed, true)
			require.Equal(t, "2026-10-07T09:00:00Z", loose.After["due"])
			require.Equal(t, compacted.After, loose.After)
		})
	}
}
