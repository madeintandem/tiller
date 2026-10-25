package views

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func replayTree(features map[string]string) *store.Tree {
	t := &store.Tree{
		Epics:     map[string]*store.Epic{},
		Features:  map[string]*store.Feature{},
		Tasks:     map[string]*store.Task{},
		Bugs:      map[string]*store.Bug{},
		Chores:    map[string]*store.Chore{},
		Tweaks:    map[string]*store.Tweak{},
		Questions: map[string]*store.Question{},
	}
	for id, status := range features {
		t.Features[id] = &store.Feature{ID: id, Status: status}
	}
	return t
}

// replayWeeks is a three-week axis starting at replayStart; replayNow falls
// in the last week.
var (
	replayStart = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	replayNow   = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	replayWeeks = []time.Time{replayStart, replayStart.AddDate(0, 0, 7), replayStart.AddDate(0, 0, 14)}
)

func TestReportReplayIgnoresNonMatchingTargets(t *testing.T) {
	tree := replayTree(map[string]string{"F-aaaaaa": "cancelled"})
	evs := []events.Event{{
		ID: "EV-aaaaaa", Type: "cancel.feature", At: replayStart.Add(time.Hour),
		Targets: []string{"F-aaaaaa", "A-aaaaaa"},
		After:   map[string]any{"status": "cancelled"},
	}}
	r := newReplay(tree, evs, replayWeeks, replayNow)
	require.Equal(t, []statusChange{{At: replayStart.Add(time.Hour), Status: "cancelled", Type: "cancel.feature"}}, r.changes("F-aaaaaa"))
	require.Empty(t, r.changes("A-aaaaaa"))
}

func TestReportReplayStatusAt(t *testing.T) {
	t1, t2 := replayStart.Add(time.Hour), replayStart.Add(48*time.Hour)
	tree := replayTree(map[string]string{"F-aaaaaa": "ready"})
	evs := []events.Event{
		{ID: "EV-000002", Type: "approve.feature", At: t2, Targets: []string{"F-aaaaaa"}, After: map[string]any{"status": "ready"}},
		{ID: "EV-000001", Type: "add.feature", At: t1, Targets: []string{"F-aaaaaa", "A-aaaaaa"}, After: map[string]any{"status": "draft"}},
	}
	r := newReplay(tree, evs, replayWeeks, replayNow)
	require.Equal(t, "", r.statusAt("F-aaaaaa", t1.Add(-time.Nanosecond)))
	require.Equal(t, "draft", r.statusAt("F-aaaaaa", t1))
	require.Equal(t, "draft", r.statusAt("F-aaaaaa", t2.Add(-time.Nanosecond)))
	require.Equal(t, "ready", r.statusAt("F-aaaaaa", t2))
	first, ok := r.firstAt("F-aaaaaa", "ready", anyChange)
	require.True(t, ok)
	require.Equal(t, t2, first)
	last, ok := r.lastAt("F-aaaaaa", "draft", anyChange)
	require.True(t, ok)
	require.Equal(t, t1, last)
	_, ok = r.firstAt("F-aaaaaa", "done", anyChange)
	require.False(t, ok)
}

func TestReportReplayScanFilters(t *testing.T) {
	t1, t2, t3 := replayStart.Add(time.Hour), replayStart.Add(48*time.Hour), replayStart.Add(96*time.Hour)
	tree := replayTree(map[string]string{"F-aaaaaa": "active", "F-bbbbbb": "done"})
	evs := []events.Event{
		{ID: "EV-000001", Type: "start.feature", At: t1, Targets: []string{"F-aaaaaa"}, After: map[string]any{"status": "active"}},
		{ID: "EV-000002", Type: "sync.feature", At: t2, Targets: []string{"F-aaaaaa"}, After: map[string]any{"status": "done"}},
		{ID: "EV-000003", Type: "start.feature", At: t3, Targets: []string{"F-aaaaaa"}, After: map[string]any{"status": "active"}},
	}
	r := newReplay(tree, evs, replayWeeks, replayNow)

	// F-aaaaaa's latest active is a real event; F-bbbbbb's done is reconciled.
	at, ok := r.lastAt("F-aaaaaa", "active", realChange)
	require.True(t, ok)
	require.Equal(t, t3, at)
	at, ok = r.firstAt("F-aaaaaa", "active", changeFrom("start.feature"))
	require.True(t, ok)
	require.Equal(t, t1, at)
	_, ok = r.firstAt("F-aaaaaa", "done", changeFrom("start.feature"))
	require.False(t, ok)
	_, ok = r.firstAt("F-bbbbbb", "done", realChange)
	require.False(t, ok)
	at, ok = r.firstAt("F-bbbbbb", "done", anyChange)
	require.True(t, ok)
	require.Equal(t, replayStart, at)
}

func TestReportReplayLeavesUnsortedInputInPlace(t *testing.T) {
	tree := replayTree(map[string]string{"F-aaaaaa": "ready"})
	t1, t2 := replayStart.Add(time.Hour), replayStart.Add(48*time.Hour)
	evs := []events.Event{
		{ID: "EV-000002", Type: "approve.feature", At: t2, Targets: []string{"F-aaaaaa"}, After: map[string]any{"status": "ready"}},
		{ID: "EV-000001", Type: "add.feature", At: t1, Targets: []string{"F-aaaaaa"}, After: map[string]any{"status": "draft"}},
	}
	r := newReplay(tree, evs, replayWeeks, replayNow)
	require.Equal(t, "draft", r.statusAt("F-aaaaaa", t1))
	require.Equal(t, []string{"EV-000002", "EV-000001"}, []string{evs[0].ID, evs[1].ID})
}

func TestReportReplayNodeWithoutEventsStartsInCurrentStatus(t *testing.T) {
	r := newReplay(replayTree(map[string]string{"F-aaaaaa": "done"}), nil, replayWeeks, replayNow)
	require.Equal(t, "done", r.statusAt("F-aaaaaa", replayStart))
}

func TestReportReplayReconcilesDivergentEnd(t *testing.T) {
	tree := replayTree(map[string]string{"F-aaaaaa": "active"})
	evs := []events.Event{{
		ID: "EV-aaaaaa", Type: "sync.feature", At: replayStart.Add(time.Hour),
		Targets: []string{"F-aaaaaa"}, After: map[string]any{"status": "done"},
	}}
	r := newReplay(tree, evs, replayWeeks, replayNow)
	require.Equal(t, "done", r.statusAt("F-aaaaaa", replayNow.Add(-time.Nanosecond)))
	require.Equal(t, "active", r.statusAt("F-aaaaaa", replayNow))
	last := r.changes("F-aaaaaa")[1]
	require.Empty(t, last.Type)
}

func TestReportWeekIndex(t *testing.T) {
	loc := time.FixedZone("CST", -6*3600)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, loc)
	at := time.Date(2026, 10, 11, 23, 30, 0, 0, loc)
	weeks := weekStarts(at, now, loc)
	require.Equal(t, 1, weekIndex(weeks, at))
	require.Equal(t, -1, weekIndex(weeks, weeks[0].Add(-time.Second)))
	require.Equal(t, 3, weekIndex(weeks, now.AddDate(0, 0, 30)))
}

func TestReportReplayTransferSplitsStatusByTarget(t *testing.T) {
	tree := replayTree(nil)
	tree.Bugs["B-aaaaaa"] = &store.Bug{WorkItem: store.WorkItem{ID: "B-aaaaaa", Status: "cancelled"}}
	tree.Bugs["B-bbbbbb"] = &store.Bug{WorkItem: store.WorkItem{ID: "B-bbbbbb", Status: "ready"}}
	t1, t2 := replayStart.Add(time.Hour), replayStart.Add(48*time.Hour)
	evs := []events.Event{
		{ID: "EV-000001", Type: "add.bug", At: t1, Targets: []string{"B-aaaaaa"}, After: map[string]any{"status": "proposed"}},
		{
			ID: "EV-000002", Type: "transfer.bug", At: t2, Targets: []string{"B-aaaaaa", "B-bbbbbb"},
			After: map[string]any{"id": "B-aaaaaa", "status": "cancelled", "replacement_id": "B-bbbbbb", "replacement_status": "ready"},
		},
	}
	r := newReplay(tree, evs, replayWeeks, replayNow)
	require.Equal(t, []statusChange{
		{At: t1, Status: "proposed", Type: "add.bug"},
		{At: t2, Status: "cancelled", Type: "transfer.bug"},
	}, r.changes("B-aaaaaa"))
	require.Equal(t, []statusChange{{At: t2, Status: "ready", Type: "transfer.bug"}}, r.changes("B-bbbbbb"))
}

func TestReportReplayClampsEventsAfterNow(t *testing.T) {
	tree := replayTree(map[string]string{"F-aaaaaa": "done", "F-bbbbbb": "active"})
	late := replayNow.Add(time.Minute)
	evs := []events.Event{
		{ID: "EV-000001", Type: "sync.feature", At: late, Targets: []string{"F-aaaaaa"}, After: map[string]any{"status": "done"}},
		{ID: "EV-000002", Type: "sync.feature", At: late, Targets: []string{"F-bbbbbb"}, After: map[string]any{"status": "done"}},
	}
	r := newReplay(tree, evs, replayWeeks, replayNow)
	require.Equal(t, "done", r.statusAt("F-aaaaaa", replayNow))
	require.Equal(t, "active", r.statusAt("F-bbbbbb", replayNow))
	for _, nodeID := range []string{"F-aaaaaa", "F-bbbbbb"} {
		line := r.changes(nodeID)
		for i, c := range line {
			require.False(t, c.At.After(replayNow), "%s change %d is after now", nodeID, i)
			if i > 0 {
				require.False(t, c.At.Before(line[i-1].At), "%s timeline unsorted at %d", nodeID, i)
			}
		}
	}
}

func TestReportReplayLegacyQuestionWithoutStatusIsOpen(t *testing.T) {
	tree := replayTree(nil)
	tree.Questions["Q-aaaaaa"] = &store.Question{ID: "Q-aaaaaa"}
	t1 := replayStart.Add(time.Hour)
	evs := []events.Event{{ID: "EV-000001", Type: "add.question", At: t1, Targets: []string{"Q-aaaaaa"}, After: map[string]any{"status": ""}}}
	r := newReplay(tree, evs, replayWeeks, replayNow)
	require.Equal(t, "open", r.statusAt("Q-aaaaaa", t1))
	require.Equal(t, []statusChange{{At: t1, Status: "open", Type: "add.question"}}, r.changes("Q-aaaaaa"))
}

func TestReportReplaySkipsRestatedStatus(t *testing.T) {
	tree := replayTree(nil)
	tree.Questions["Q-aaaaaa"] = &store.Question{ID: "Q-aaaaaa", Status: "open"}
	t1, t2 := replayStart.Add(time.Hour), replayStart.Add(48*time.Hour)
	evs := []events.Event{
		{ID: "EV-000001", Type: "add.question", At: t1, Targets: []string{"Q-aaaaaa"}, After: map[string]any{"status": "open"}},
		{ID: "EV-000002", Type: "add.question", At: t2, Targets: []string{"Q-aaaaaa"}, After: map[string]any{"status": "open"}},
	}
	r := newReplay(tree, evs, replayWeeks, replayNow)
	require.Equal(t, []statusChange{{At: t1, Status: "open", Type: "add.question"}}, r.changes("Q-aaaaaa"))
}

func TestReportReplayRecordsRestatementWithDifferentBefore(t *testing.T) {
	tree := replayTree(map[string]string{"F-aaaaaa": "done"})
	t1, t2 := replayStart.Add(time.Hour), replayStart.Add(48*time.Hour)
	evs := []events.Event{
		{ID: "EV-000001", Type: "sync.feature", At: t1, Targets: []string{"F-aaaaaa"}, After: map[string]any{"status": "done"}},
		{
			ID: "EV-000002", Type: "sync.feature", At: t2, Targets: []string{"F-aaaaaa"},
			Before: map[string]any{"status": "active"}, After: map[string]any{"status": "done"},
		},
	}
	r := newReplay(tree, evs, replayWeeks, replayNow)
	require.Equal(t, []statusChange{
		{At: t1, Status: "done", Type: "sync.feature"},
		{At: t2, Status: "done", Type: "sync.feature"},
	}, r.changes("F-aaaaaa"))
}

func TestReportReplayIgnoresZeroTimestamp(t *testing.T) {
	tree := replayTree(map[string]string{"F-aaaaaa": "active"})
	evs := []events.Event{{ID: "EV-000001", Type: "sync.feature", Targets: []string{"F-aaaaaa"}, After: map[string]any{"status": "done"}}}
	r := newReplay(tree, evs, replayWeeks, replayNow)
	require.Equal(t, []statusChange{{At: replayStart, Status: "active"}}, r.changes("F-aaaaaa"))
}
