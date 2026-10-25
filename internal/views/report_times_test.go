package views_test

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// day is an instant n days (fractions allowed) after the first week's Monday.
func day(n float64) time.Time {
	return burnUpWeek0.Add(time.Duration(n * 24 * float64(time.Hour)))
}

// timesTree has one feature per id in the given status, each with one task.
func timesTree(status string, fids ...string) *store.Tree {
	tr := reportTree("p", "main")
	tr.FeatureTasks = map[string][]*store.Task{}
	for _, fid := range fids {
		tr.Features[fid] = &store.Feature{ID: fid, Title: "Feature " + fid, Status: status}
		tid := "T-" + fid[2:]
		task := &store.Task{ID: tid, FeatureID: fid, Status: "done"}
		tr.Tasks[tid] = task
		tr.FeatureTasks[fid] = []*store.Task{task}
	}
	return tr
}

// shipped is the event run that makes a feature done: approval at ready, a
// task start at start, and completion at done.
func shipped(fid string, ready, start, done float64) []events.Event {
	return []events.Event{
		statusEv("approve.feature", day(ready), "ready", fid),
		statusEv("start.task", day(start), "in_progress", "T-"+fid[2:]),
		statusEv("sync.feature", day(done), "done", fid),
	}
}

func timesReport(tr *store.Tree, evs []events.Event) views.Report {
	return views.ReportOf(tr, views.ReportInput{Events: evs, Now: burnUpNow, Location: time.UTC})
}

func TestReportFeatureTimes(t *testing.T) {
	tr := timesTree("done", "F-aaaaaa")
	tr.FeatureTasks["F-aaaaaa"] = append(tr.FeatureTasks["F-aaaaaa"], &store.Task{ID: "T-bbbbbb", FeatureID: "F-aaaaaa", Status: "done"})
	tr.Tasks["T-bbbbbb"] = tr.FeatureTasks["F-aaaaaa"][1]
	evs := []events.Event{
		statusEv("approve.feature", day(0), "ready", "F-aaaaaa"),
		statusEv("start.task", day(5), "in_progress", "T-bbbbbb"),
		statusEv("start.task", day(3), "in_progress", "T-aaaaaa"),
		statusEv("sync.feature", day(9.5), "done", "F-aaaaaa"),
	}
	got := timesReport(tr, evs).FeatureTimes
	require.Len(t, got, 1)
	require.Equal(t, 9.5, got[0].LeadDays)
	require.Equal(t, 6.5, got[0].CycleDays)
	require.Equal(t, "Feature F-aaaaaa", got[0].Title)
	require.Equal(t, day(9.5).UTC().Format(time.RFC3339), got[0].CompletedAt)
}

func TestReportFeatureTimesStartFromFeatureEvent(t *testing.T) {
	tr := timesTree("done", "F-aaaaaa")
	evs := []events.Event{
		statusEv("approve.feature", day(0), "ready", "F-aaaaaa"),
		statusEv("start.feature", day(2), "active", "F-aaaaaa"),
		statusEv("start.task", day(4), "in_progress", "T-aaaaaa"),
		statusEv("sync.feature", day(6), "done", "F-aaaaaa"),
	}
	require.Equal(t, 4.0, timesReport(tr, evs).FeatureTimes[0].CycleDays)
}

func TestReportFeatureTimesUseFirstApprovalAndLastDone(t *testing.T) {
	tr := timesTree("done", "F-aaaaaa")
	evs := []events.Event{
		statusEv("approve.feature", day(0), "ready", "F-aaaaaa"),
		statusEv("return.feature", day(1), "specced", "F-aaaaaa"),
		statusEv("approve.feature", day(2), "ready", "F-aaaaaa"),
		statusEv("start.task", day(3), "in_progress", "T-aaaaaa"),
		statusEv("sync.feature", day(4), "done", "F-aaaaaa"),
		statusEv("start.feature", day(5), "active", "F-aaaaaa"),
		statusEv("sync.feature", day(8), "done", "F-aaaaaa"),
	}
	got := timesReport(tr, evs).FeatureTimes[0]
	require.Equal(t, 8.0, got.LeadDays)
	require.Equal(t, 5.0, got.CycleDays)
}

func TestReportFeatureTimesSkipUnevidenced(t *testing.T) {
	tr := timesTree("done", "F-aaaaaa", "F-bbbbbb", "F-cccccc")
	evs := []events.Event{
		// F-aaaaaa: approved and done, but never started.
		statusEv("approve.feature", day(0), "ready", "F-aaaaaa"),
		statusEv("sync.feature", day(3), "done", "F-aaaaaa"),
		// F-bbbbbb: no events at all, so it is reconciled to done.
	}
	evs = append(evs, shipped("F-cccccc", 0, 1, 2)...)
	got := timesReport(tr, evs).FeatureTimes
	require.Len(t, got, 1)
	require.Equal(t, "Feature F-cccccc", got[0].Title)
}

func TestReportFeatureTimesOnlyDoneFeatures(t *testing.T) {
	tr := timesTree("active", "F-aaaaaa")
	require.Empty(t, timesReport(tr, shipped("F-aaaaaa", 0, 1, 2)).FeatureTimes)
}

func TestReportFeatureTimesOrderedByCompletion(t *testing.T) {
	tr := timesTree("done", "F-aaaaaa", "F-bbbbbb")
	evs := append(shipped("F-aaaaaa", 0, 1, 9), shipped("F-bbbbbb", 0, 1, 4)...)
	got := timesReport(tr, evs).FeatureTimes
	require.Equal(t, "Feature F-bbbbbb", got[0].Title)
	require.Equal(t, "Feature F-aaaaaa", got[1].Title)
}

func TestReportRollingMedians(t *testing.T) {
	fids := []string{"F-aaaaaa", "F-bbbbbb", "F-cccccc", "F-dddddd", "F-eeeeee", "F-ffffff"}
	tr := timesTree("done", fids...)
	var evs []events.Event
	for i, fid := range fids {
		// Cycle i+1 days, lead i+2 days, finishing one day apart.
		done := float64(10 + i)
		evs = append(evs, shipped(fid, done-float64(i+2), done-float64(i+1), done)...)
	}
	got := timesReport(tr, evs).FeatureTimes
	require.Len(t, got, 6)
	require.Equal(t, 6.0, got[5].CycleDays)
	require.Equal(t, 4.0, got[5].CycleMedianDays)
	require.Equal(t, 5.0, got[5].LeadMedianDays)
	require.Equal(t, 1.5, got[1].CycleMedianDays)
}

func TestReportMedianKPIs(t *testing.T) {
	fids := []string{"F-aaaaaa", "F-bbbbbb", "F-cccccc"}
	tr := timesTree("done", fids...)
	var evs []events.Event
	for i, c := range []float64{2, 4, 10} {
		evs = append(evs, shipped(fids[i], 0, 0.5, 0.5+c)...)
	}
	k := timesReport(tr, evs).KPIs
	require.Equal(t, 3, k.CompletedFeatures)
	require.NotNil(t, k.MedianCycleDays)
	require.Equal(t, 4.0, *k.MedianCycleDays)
	require.Equal(t, 4.5, *k.MedianLeadDays)

	empty := timesReport(timesTree("done"), nil).KPIs
	require.Zero(t, empty.CompletedFeatures)
	require.Nil(t, empty.MedianCycleDays)
	require.Nil(t, empty.MedianLeadDays)
}

func TestReportFeatureTimesBreakCompletionTiesByID(t *testing.T) {
	tr := timesTree("done", "F-bbbbbb", "F-aaaaaa")
	evs := append(shipped("F-bbbbbb", 0, 1, 6), shipped("F-aaaaaa", 0, 2, 6)...)
	for range 20 {
		got := timesReport(tr, evs).FeatureTimes
		require.Len(t, got, 2)
		require.Equal(t, "Feature F-aaaaaa", got[0].Title)
		require.Equal(t, "Feature F-bbbbbb", got[1].Title)
	}
}

func TestReportFeatureTimesIgnoreRestatedStatus(t *testing.T) {
	// A correction months after completion restates the done status; it is
	// not a second completion.
	tr := timesTree("done", "F-aaaaaa")
	evs := append(shipped("F-aaaaaa", 0, 1, 2), statusEv("correct-eval.feature", day(2).AddDate(0, 2, 0), "done", "F-aaaaaa"))
	rep := views.ReportOf(tr, views.ReportInput{Events: evs, Now: day(2).AddDate(0, 3, 0), Location: time.UTC})
	require.Len(t, rep.FeatureTimes, 1)
	got := rep.FeatureTimes[0]
	require.Equal(t, day(2).UTC().Format(time.RFC3339), got.CompletedAt)
	require.Equal(t, 2.0, got.LeadDays)
	require.Equal(t, 1.0, got.CycleDays)
}

func TestReportFeatureTimesCountReopenedCompletion(t *testing.T) {
	// The feature reopened with no event, then synced done again: the second
	// sync's before status shows a real transition.
	tr := timesTree("done", "F-aaaaaa")
	again := statusEv("sync.feature", day(8), "done", "F-aaaaaa")
	again.Before = map[string]any{"status": "active"}
	got := timesReport(tr, append(shipped("F-aaaaaa", 0, 1, 2), again)).FeatureTimes[0]
	require.Equal(t, day(8).UTC().Format(time.RFC3339), got.CompletedAt)
	require.Equal(t, 8.0, got.LeadDays)
}
