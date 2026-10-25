package views_test

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// flowTree holds one node of each work kind plus the given bug and question
// statuses, all current in the tree.
func flowTree(bugs, questions map[string]string, done ...string) *store.Tree {
	tr := reportTree("p", "main")
	for _, id := range done {
		switch idPrefix(id) {
		case "T":
			tr.Tasks[id] = &store.Task{ID: id, Status: "done"}
		case "CH":
			tr.Chores[id] = &store.Chore{WorkItem: store.WorkItem{ID: id, Status: "done"}}
		case "TW":
			tr.Tweaks[id] = &store.Tweak{WorkItem: store.WorkItem{ID: id, Status: "done"}}
		case "B":
			tr.Bugs[id] = &store.Bug{WorkItem: store.WorkItem{ID: id, Status: "done"}}
		}
	}
	for id, st := range bugs {
		tr.Bugs[id] = &store.Bug{WorkItem: store.WorkItem{ID: id, Status: st}}
	}
	for id, st := range questions {
		tr.Questions[id] = &store.Question{ID: id, Status: st}
	}
	return tr
}

func idPrefix(id string) string {
	for i := range id {
		if id[i] == '-' {
			return id[:i]
		}
	}
	return id
}

func flowReport(tr *store.Tree, evs []events.Event) views.Report {
	return views.ReportOf(tr, views.ReportInput{Events: evs, Now: burnUpNow, Location: time.UTC})
}

func TestReportThroughputStacksByKind(t *testing.T) {
	tr := flowTree(nil, nil, "T-aaaaaa", "T-bbbbbb", "B-aaaaaa", "CH-aaaaaa", "TW-aaaaaa")
	evs := []events.Event{
		statusEv("sync.task", inWeek(2), "done", "T-aaaaaa"),
		statusEv("sync.task", inWeek(2), "done", "T-bbbbbb"),
		statusEv("sync.bug", inWeek(2), "done", "B-aaaaaa"),
		statusEv("sync.chore", inWeek(2), "done", "CH-aaaaaa"),
		statusEv("sync.tweak", inWeek(2), "done", "TW-aaaaaa"),
	}
	got := flowReport(tr, evs).Throughput
	require.Equal(t, views.ReportThroughputWeek{FeatureWork: 2, Fixes: 1, Upkeep: 2}, got[2])
	require.Equal(t, views.ReportThroughputWeek{}, got[0])
	require.Equal(t, views.ReportThroughputWeek{}, got[1])
	require.Equal(t, views.ReportThroughputWeek{}, got[3])
}

func TestReportThroughputCountsEachItemOnce(t *testing.T) {
	tr := flowTree(nil, nil, "T-aaaaaa")
	evs := []events.Event{
		statusEv("sync.task", inWeek(1), "done", "T-aaaaaa"),
		statusEv("start.task", inWeek(2), "active", "T-aaaaaa"),
		statusEv("sync.task", inWeek(3), "done", "T-aaaaaa"),
	}
	got := flowReport(tr, evs).Throughput
	require.Equal(t, 1, got[1].FeatureWork)
	require.Equal(t, 0, got[3].FeatureWork)
}

func TestReportCarriedBugCountsWithCarrier(t *testing.T) {
	tr := flowTree(nil, nil, "B-aaaaaa")
	carried := statusEv("sync.bug", inWeek(2), "done", "B-aaaaaa")
	carried.After["basis"] = "carried"
	evs := []events.Event{statusEv("sync.feature", inWeek(2), "done", "F-aaaaaa"), carried}
	require.Equal(t, 1, flowReport(tr, evs).Throughput[2].Fixes)
}

func TestReportShippedLast7Days(t *testing.T) {
	tr := flowTree(nil, nil, "T-aaaaaa", "T-bbbbbb")
	evs := []events.Event{
		statusEv("sync.task", burnUpNow.Add(-2*24*time.Hour), "done", "T-aaaaaa"),
		statusEv("sync.task", burnUpNow.Add(-9*24*time.Hour), "done", "T-bbbbbb"),
	}
	require.Equal(t, 1, flowReport(tr, evs).KPIs.ShippedLast7Days)
}

func TestReportBugsReportedFixedOpen(t *testing.T) {
	tr := flowTree(map[string]string{"B-bbbbbb": "cancelled"}, nil, "B-aaaaaa")
	evs := []events.Event{
		statusEv("add.bug", inWeek(0), "ready", "B-aaaaaa"),
		statusEv("add.bug", inWeek(1), "ready", "B-bbbbbb"),
		statusEv("cancel.bug", inWeek(2), "cancelled", "B-bbbbbb"),
		statusEv("sync.bug", inWeek(3), "done", "B-aaaaaa"),
	}
	rep := flowReport(tr, evs)
	var reported, fixed, open []int
	for _, w := range rep.Bugs {
		reported = append(reported, w.Reported)
		fixed = append(fixed, w.Fixed)
		open = append(open, w.Open)
	}
	require.Equal(t, []int{1, 1, 0, 0}, reported)
	require.Equal(t, []int{0, 0, 0, 1}, fixed)
	require.Equal(t, []int{1, 2, 1, 0}, open)
	require.Equal(t, 0, rep.KPIs.OpenBugs)
}

func TestReportOpenQuestionsByWeek(t *testing.T) {
	tr := flowTree(nil, map[string]string{"Q-aaaaaa": "answered", "Q-bbbbbb": "open"})
	evs := []events.Event{
		statusEv("add.question", inWeek(0), "open", "Q-aaaaaa"),
		statusEv("add.question", inWeek(1), "open", "Q-bbbbbb"),
		statusEv("answer.question", inWeek(2), "answered", "Q-aaaaaa"),
	}
	rep := flowReport(tr, evs)
	require.Equal(t, []int{1, 2, 1, 1}, rep.KPIs.OpenQuestionsByWeek)
	require.Equal(t, 1, rep.KPIs.OpenQuestions)
	require.Equal(t, 1, rep.KPIs.AnsweredQuestions)
}

func TestReportBugTransferIsOneReport(t *testing.T) {
	tr := flowTree(map[string]string{"B-aaaaaa": "cancelled", "B-bbbbbb": "ready"}, nil)
	transfer := statusEv("transfer.bug", inWeek(1), "cancelled", "B-aaaaaa", "B-bbbbbb")
	transfer.After["id"] = "B-aaaaaa"
	transfer.After["replacement_id"] = "B-bbbbbb"
	transfer.After["replacement_status"] = "ready"
	evs := []events.Event{statusEv("add.bug", inWeek(0), "proposed", "B-aaaaaa"), transfer}
	rep := flowReport(tr, evs)
	var reported, fixed, open []int
	for _, w := range rep.Bugs {
		reported = append(reported, w.Reported)
		fixed = append(fixed, w.Fixed)
		open = append(open, w.Open)
	}
	require.Equal(t, []int{1, 0, 0, 0}, reported)
	require.Equal(t, []int{0, 0, 0, 0}, fixed)
	require.Equal(t, []int{1, 1, 1, 1}, open)
	require.Equal(t, 1, rep.KPIs.OpenBugs)
}

func TestReportLegacyQuestionWithoutStatusCountsOpen(t *testing.T) {
	tr := flowTree(nil, map[string]string{"Q-aaaaaa": ""})
	evs := []events.Event{statusEv("add.question", inWeek(0), "", "Q-aaaaaa")}
	require.Equal(t, []int{1, 1, 1, 1}, flowReport(tr, evs).KPIs.OpenQuestionsByWeek)
}

func TestReportUnexplainedNodeStartsInFirstEventWeek(t *testing.T) {
	// A young project: one event this week on a four-week padded axis, and a
	// migrated done task with no events of its own.
	tr := flowTree(nil, nil, "T-aaaaaa")
	evs := []events.Event{statusEv("add.question", inWeek(3), "open", "Q-aaaaaa")}
	got := flowReport(tr, evs).Throughput
	require.Equal(t, 0, got[0].FeatureWork)
	require.Equal(t, 1, got[3].FeatureWork)
}
