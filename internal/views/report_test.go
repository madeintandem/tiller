package views_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func reportTree(project, integration string) *store.Tree {
	m := &store.Manifest{}
	m.Project.Name = project
	m.Git.IntegrationBranch = integration
	return &store.Tree{
		Manifest:  m,
		Epics:     map[string]*store.Epic{},
		Features:  map[string]*store.Feature{},
		Tasks:     map[string]*store.Task{},
		Bugs:      map[string]*store.Bug{},
		Chores:    map[string]*store.Chore{},
		Tweaks:    map[string]*store.Tweak{},
		Questions: map[string]*store.Question{},
	}
}

func TestReportWeeksUseLocalMondays(t *testing.T) {
	loc := time.FixedZone("CST", -6*3600)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, loc)
	at := time.Date(2026, 10, 11, 23, 30, 0, 0, loc) // Sunday local, Monday UTC
	tr := flowTree(nil, nil, "T-aaaaaa")
	rep := views.ReportOf(tr, views.ReportInput{
		Events:   []events.Event{statusEv("sync.task", at, "done", "T-aaaaaa")},
		Now:      now,
		Location: loc,
	})
	require.Equal(t, []string{"2026-09-28", "2026-10-05", "2026-10-12", "2026-10-19"}, rep.Weeks)
	require.Equal(t, []string{
		"2026-09-28T00:00:00-06:00",
		"2026-10-05T00:00:00-06:00",
		"2026-10-12T00:00:00-06:00",
		"2026-10-19T00:00:00-06:00",
		"2026-10-26T00:00:00-06:00",
	}, rep.WeekBoundaries)
	// Its UTC date is Monday, but it belongs to the server's Sunday band.
	require.Equal(t, 1, rep.Throughput[1].FeatureWork)
	require.Equal(t, 0, rep.Throughput[2].FeatureWork)
}

func TestReportWeekBoundariesAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	for _, tc := range []struct {
		name       string
		now        time.Time
		start, end string
		hours      time.Duration
	}{
		{
			name:  "spring forward",
			now:   time.Date(2026, 3, 8, 18, 0, 0, 0, loc),
			start: "2026-03-02T00:00:00-05:00",
			end:   "2026-03-09T00:00:00-04:00",
			hours: 167,
		},
		{
			name:  "fall back",
			now:   time.Date(2026, 11, 1, 18, 0, 0, 0, loc),
			start: "2026-10-26T00:00:00-04:00",
			end:   "2026-11-02T00:00:00-05:00",
			hours: 169,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rep := views.ReportOf(reportTree("p", "main"), views.ReportInput{Now: tc.now, Location: loc})
			require.Len(t, rep.WeekBoundaries, len(rep.Weeks)+1)
			last := len(rep.Weeks) - 1
			require.Equal(t, tc.start[:10], rep.Weeks[last])
			require.Equal(t, tc.start, rep.WeekBoundaries[last])
			require.Equal(t, tc.end, rep.WeekBoundaries[last+1])
			start, err := time.Parse(time.RFC3339, rep.WeekBoundaries[last])
			require.NoError(t, err)
			end, err := time.Parse(time.RFC3339, rep.WeekBoundaries[last+1])
			require.NoError(t, err)
			previous, err := time.Parse(time.RFC3339, rep.WeekBoundaries[last-1])
			require.NoError(t, err)
			require.Equal(t, tc.hours*time.Hour, end.Sub(start))
			require.Equal(t, 168*time.Hour, start.Sub(previous))
			for i, label := range rep.Weeks {
				require.Equal(t, label, rep.WeekBoundaries[i][:10])
			}
		})
	}
}

func TestReportEmptyStream(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	rep := views.ReportOf(reportTree("p", "main"), views.ReportInput{Now: now, Location: time.UTC})
	require.Len(t, rep.Weeks, 4)
	require.Equal(t, "2026-10-19", rep.Weeks[3])
	require.Equal(t, make([]views.ReportBurnUpWeek, 4), rep.BurnUp)
	require.Equal(t, make([]views.ReportThroughputWeek, 4), rep.Throughput)
	require.Equal(t, make([]views.ReportBugWeek, 4), rep.Bugs)
	require.Equal(t, make([]int, 4), rep.KPIs.OpenQuestionsByWeek)
	require.Nil(t, rep.KPIs.MedianCycleDays)
	require.Nil(t, rep.KPIs.MedianLeadDays)
	require.NotNil(t, rep.Epics)
	require.Empty(t, rep.Epics)
	require.NotNil(t, rep.FeatureTimes)
	require.Empty(t, rep.FeatureTimes)

	raw, err := json.Marshal(rep)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"epics":[]`)
	require.Contains(t, string(raw), `"feature_times":[]`)
}

func TestReportOffBranchNotice(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		branch, integration string
		want                *views.ReportBranchNotice
	}{
		{"main", "main", nil},
		{"feature/x", "main", &views.ReportBranchNotice{Branch: "feature/x", Integration: "main"}},
		{"HEAD", "main", &views.ReportBranchNotice{Branch: "HEAD", Integration: "main"}},
		{"", "main", nil},
		{"feature/x", "", nil},
	}
	for _, c := range cases {
		rep := views.ReportOf(reportTree("p", c.integration), views.ReportInput{Now: now, Location: time.UTC, Branch: c.branch})
		require.Equal(t, c.want, rep.OffBranch, "branch %q integration %q", c.branch, c.integration)
	}
}

// TestReportAgentUseFollowsCapture pins that the Agent use tab is offered only
// while usage.capture is on: turning capture off hides the tab even when
// usage files remain from before.
func TestReportAgentUseFollowsCapture(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tr := reportTree("p", "main")
	require.False(t, views.ReportOf(tr, views.ReportInput{Now: now, Location: time.UTC}).AgentUse)
	tr.Manifest.Usage.Capture = true
	require.True(t, views.ReportOf(tr, views.ReportInput{Now: now, Location: time.UTC}).AgentUse)
}

func TestReportHeader(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.FixedZone("CST", -6*3600))
	rep := views.ReportOf(reportTree("Acme", "main"), views.ReportInput{Now: now})
	require.Equal(t, "Acme", rep.Project)
	require.Equal(t, now.UTC().Format(time.RFC3339), rep.AsOf)
}

func TestReportIgnoresZeroTimestamp(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tr := reportTree("p", "main")
	tr.Tasks["T-aaaaaa"] = &store.Task{ID: "T-aaaaaa", Status: "in_progress"}
	evs := []events.Event{
		{ID: "EV-aaaaaa", Type: "sync.task", Targets: []string{"T-aaaaaa"}, After: map[string]any{"status": "done"}},
		{ID: "EV-aaaaab", Type: "start.task", At: now.Add(-time.Hour), Targets: []string{"T-aaaaaa"}, After: map[string]any{"status": "in_progress"}},
	}
	rep := views.ReportOf(tr, views.ReportInput{Events: evs, Now: now, Location: time.UTC})
	require.Len(t, rep.Weeks, 4)
	require.Equal(t, make([]views.ReportThroughputWeek, 4), rep.Throughput)
}

func TestReportAxisSpansAtMostTenYears(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tr := reportTree("p", "main")
	tr.Tasks["T-aaaaaa"] = &store.Task{ID: "T-aaaaaa", Status: "done"}
	evs := []events.Event{{
		ID: "EV-aaaaaa", Type: "sync.task", At: time.Date(1970, 1, 5, 0, 0, 0, 0, time.UTC),
		Targets: []string{"T-aaaaaa"}, After: map[string]any{"status": "done"},
	}}
	rep := views.ReportOf(tr, views.ReportInput{Events: evs, Now: now, Location: time.UTC})
	require.Len(t, rep.Weeks, 520)
	require.Equal(t, "2026-10-19", rep.Weeks[519])
	require.Equal(t, 1, rep.Throughput[0].FeatureWork)
}
