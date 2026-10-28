package views_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/usage"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// usageNow is a Wednesday; the current week starts Monday 2026-10-19.
var usageNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func usageFile(harness, session string, rows ...usage.Row) usage.File {
	return usage.File{Harness: harness, Session: session, Reader: usage.ReaderVersion, Rows: rows}
}

func usageRow(hour time.Time, node string, stage usage.Stage, model string, tok usage.Tokens) usage.Row {
	return usage.Row{Hour: hour, Node: node, Stage: stage, Model: model, Calls: 1, Tokens: tok}
}

func onDay(month time.Month, d int) time.Time {
	return time.Date(2026, month, d, 10, 0, 0, 0, time.UTC)
}

func input(n int64) usage.Tokens { return usage.Tokens{Input: n} }

func usageReport(tr *store.Tree, files []usage.File, evs []events.Event) views.UsageReport {
	return views.UsageReportOf(tr, views.UsageReportInput{Files: files, Events: evs, Now: usageNow, Location: time.UTC})
}

// claudeSinceSep30 is a file whose earliest row makes the full-coverage date
// Sep 30 when it is the latest harness to begin.
func claudeSinceSep30() usage.File {
	return usageFile("claude-code", "anchor", usageRow(onDay(time.September, 30), "", usage.Other, "m", input(1)))
}

func TestUsageReportSharesTheProjectAxis(t *testing.T) {
	loc := time.FixedZone("CST", -6*3600)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, loc)
	tr := flowTree(nil, nil, "T-aaaaaa")
	evs := []events.Event{statusEv("sync.task", time.Date(2026, 8, 2, 23, 30, 0, 0, loc), "done", "T-aaaaaa")}
	// A row before the first event does not widen the axis; it counts at the
	// axis start, as an early event does.
	files := []usage.File{usageFile("pi", "s", usageRow(time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC), "", usage.Other, "m", input(7)))}

	for _, tc := range []struct {
		name string
		evs  []events.Event
	}{{"with events", evs}, {"without events", nil}} {
		t.Run(tc.name, func(t *testing.T) {
			project := views.ReportOf(tr, views.ReportInput{Events: tc.evs, Now: now, Location: loc})
			got := views.UsageReportOf(tr, views.UsageReportInput{Files: files, Events: tc.evs, Now: now, Location: loc})
			require.Equal(t, project.Weeks, got.Weeks)
			require.Equal(t, project.WeekBoundaries, got.WeekBoundaries)
			require.Len(t, got.Coverage, len(got.Weeks))
			require.Len(t, got.Weekly, len(got.Weeks))
			require.Equal(t, int64(7), got.Weekly[0].Input)
		})
	}
}

func TestUsageReportCoveragePartialBeforeLatestHarness(t *testing.T) {
	loc := time.FixedZone("CST", -6*3600)
	tr := flowTree(nil, nil, "T-aaaaaa")
	evs := []events.Event{statusEv("sync.task", time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC), "done", "T-aaaaaa")}
	files := []usage.File{
		usageFile("pi", "p1",
			usageRow(time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC), "", usage.Other, "m", input(10)),
			usageRow(time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC), "", usage.Other, "m", input(10))),
		// 03:00 UTC on Oct 1 is the evening of Sep 30 in the serving zone.
		usageFile("claude-code", "c1", usageRow(time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC), "", usage.Other, "m", input(500))),
	}
	got := views.UsageReportOf(tr, views.UsageReportInput{Files: files, Events: evs, Now: usageNow, Location: loc})

	require.Equal(t, []views.UsageHarness{
		{Harness: "claude-code", Label: "Claude Code", Since: "2026-09-30"},
		{Harness: "pi", Label: "Pi", Since: "2026-09-01"},
	}, got.Harnesses)
	require.Equal(t, "2026-09-30", got.FullCoverage)
	require.Equal(t, []string{"2026-08-10", "2026-08-17", "2026-08-24", "2026-08-31", "2026-09-07", "2026-09-14",
		"2026-09-21", "2026-09-28", "2026-10-05", "2026-10-12", "2026-10-19"}, got.Weeks)
	// The week of Sep 28 is partial: Claude was measured only from Sep 30.
	require.Equal(t, []string{"none", "none", "none", "partial", "partial", "partial", "partial",
		"partial", "full", "full", "full"}, got.Coverage)
}

// TestUsageReportCoverageOnMondays pins both boundaries on a Monday: a week
// ending exactly when the first harness begins is none, and the week starting
// on the full-coverage date is full.
func TestUsageReportCoverageOnMondays(t *testing.T) {
	files := []usage.File{
		usageFile("pi", "p", usageRow(onDay(time.October, 5), "", usage.Other, "m", input(1))),
		usageFile("claude-code", "c", usageRow(onDay(time.October, 12), "", usage.Other, "m", input(2))),
	}
	got := usageReport(reportTree("p", "main"), files, nil)

	require.Equal(t, []string{"2026-09-28", "2026-10-05", "2026-10-12", "2026-10-19"}, got.Weeks)
	require.Equal(t, "2026-10-12", got.FullCoverage)
	require.Equal(t, []string{"none", "partial", "full", "full"}, got.Coverage)
}

func TestUsageReportItemsLeaveOutEarlyStarts(t *testing.T) {
	tr := flowTree(nil, nil, "T-aaaaaa", "T-bbbbbb", "T-cccccc", "B-aaaaaa", "CH-aaaaaa")
	tr.Tasks["T-dddddd"] = &store.Task{ID: "T-dddddd", Status: "in_progress"}
	evs := []events.Event{
		statusEv("start.task", onDay(time.September, 25), "in_progress", "T-aaaaaa"), // before full coverage
		statusEv("start.task", onDay(time.October, 5), "in_progress", "T-bbbbbb"),
		statusEv("start.task", onDay(time.October, 5), "in_progress", "T-cccccc"), // done, but no usage of its own
		statusEv("start.task", onDay(time.October, 5), "in_progress", "T-dddddd"), // still in progress
		statusEv("start.bug", onDay(time.September, 30), "in_progress", "B-aaaaaa"),
	}
	files := []usage.File{
		claudeSinceSep30(),
		usageFile("claude-code", "work",
			usageRow(onDay(time.October, 1), "T-aaaaaa", usage.Implementation, "m", input(2_000_000)),
			usageRow(onDay(time.October, 6), "T-bbbbbb", usage.Implementation, "m", input(150_000)),
			usageRow(onDay(time.October, 6), "T-dddddd", usage.Implementation, "m", input(5_000_000)),
			usageRow(onDay(time.October, 7), "B-aaaaaa", usage.Implementation, "m", input(50_000_000)),
			// A chore carried by another merge never started on its own.
			usageRow(onDay(time.October, 7), "CH-aaaaaa", usage.Implementation, "m", input(400_000))),
	}
	got := usageReport(tr, files, evs)

	require.Equal(t, 2, got.KPIs.Items)
	require.Equal(t, 2, got.ItemsLeftOut)
	require.Equal(t, 1, got.ItemsUnmeasured, "T-cccccc is done with no usage")
	require.NotNil(t, got.KPIs.MedianPerItem)
	require.Equal(t, int64(25_075_000), *got.KPIs.MedianPerItem)
	counts := map[string]int{}
	for _, b := range got.ItemBands {
		counts[b.Label] = b.Count
	}
	require.Equal(t, map[string]int{
		"<100K": 0, "100K–300K": 1, "300K–1M": 0, "1M–3M": 0,
		"3M–10M": 0, "10M–30M": 0, "30M–100M": 1, ">100M": 0,
	}, counts)
}

// TestUsageReportCountsUnmeasuredItems pins that a done item with no usage of
// its own is unmeasured, never a zero in a band, the median, or left out.
func TestUsageReportCountsUnmeasuredItems(t *testing.T) {
	tr := flowTree(nil, nil, "T-aaaaaa")
	evs := []events.Event{statusEv("start.task", onDay(time.October, 5), "in_progress", "T-aaaaaa")}
	got := usageReport(tr, []usage.File{claudeSinceSep30()}, evs)

	require.Equal(t, 1, got.ItemsUnmeasured)
	require.Zero(t, got.ItemsLeftOut)
	require.Zero(t, got.KPIs.Items)
	require.Nil(t, got.KPIs.MedianPerItem)
	for _, b := range got.ItemBands {
		require.Zero(t, b.Count, b.Label)
	}
}

func TestUsageReportSumsSessionsWithoutPeople(t *testing.T) {
	tr := flowTree(nil, nil, "T-aaaaaa")
	tr.Tasks["T-aaaaaa"].Owner = "dana-whitfield"
	tr.Tasks["T-aaaaaa"].Title = "Wire the form"
	tr.Manifest.Contributors = []store.Contributor{{ID: "dana-whitfield", Name: "Dana Whitfield"}, {ID: "jc-grubbs", Name: "JC Grubbs"}}
	start := statusEv("start.task", onDay(time.October, 6), "in_progress", "T-aaaaaa")
	start.Contributor = "jc-grubbs"
	done := statusEv("sync.task", onDay(time.October, 8), "done", "T-aaaaaa")
	done.Contributor = "dana-whitfield"
	hour := onDay(time.October, 6)
	files := []usage.File{
		usageFile("claude-code", "engineer-one", usageRow(hour, "T-aaaaaa", usage.Implementation, "m", input(100_000))),
		usageFile("claude-code", "engineer-two", usageRow(hour, "T-aaaaaa", usage.Implementation, "m", input(120_000))),
	}
	got := usageReport(tr, files, []events.Event{start, done})

	require.Equal(t, int64(220_000), got.KPIs.Total)
	require.Equal(t, 1, got.KPIs.Items)
	require.Equal(t, int64(220_000), *got.KPIs.MedianPerItem)
	require.Equal(t, 1.0, got.KPIs.TiedShare)
	require.Equal(t, 1, got.ItemBands[1].Count)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	for _, person := range []string{"dana-whitfield", "Dana Whitfield", "jc-grubbs", "JC Grubbs", "T-aaaaaa", "Wire the form"} {
		require.NotContains(t, string(raw), person)
	}
}

func TestUsageReportTopFourModelsThenOther(t *testing.T) {
	hour := onDay(time.October, 20)
	rows := []usage.Row{
		usageRow(hour, "", usage.Implementation, "m-e", input(200)),
		usageRow(hour, "", usage.Implementation, "m-a", input(600)),
		usageRow(hour, "", usage.Checks, "m-b", input(500)),
		usageRow(hour, "", usage.Implementation, "m-d", input(400)),
		usageRow(hour, "", usage.Implementation, "m-c", input(400)),
		usageRow(hour, "", usage.CodeReview, "m-f", input(100)),
	}
	got := usageReport(reportTree("p", "main"), []usage.File{usageFile("pi", "s", rows...)}, nil)

	require.Equal(t, []string{"m-a", "m-b", "m-c", "m-d", "Other"}, got.Models)
	require.Len(t, got.ModelStages, len(usage.Stages))
	byStage := map[string][]int64{}
	for i, ms := range got.ModelStages {
		require.Equal(t, string(usage.Stages[i]), ms.Stage)
		byStage[ms.Stage] = ms.Totals
	}
	require.Equal(t, []int64{600, 0, 400, 400, 200}, byStage["implementation"])
	require.Equal(t, []int64{0, 500, 0, 0, 0}, byStage["checks"])
	require.Equal(t, []int64{0, 0, 0, 0, 100}, byStage["code_review"])
	require.Equal(t, []int64{0, 0, 0, 0, 0}, byStage["spec_writing"])

	four := usageReport(reportTree("p", "main"), []usage.File{usageFile("pi", "s", rows[1:5]...)}, nil)
	require.Equal(t, []string{"m-a", "m-b", "m-c", "m-d"}, four.Models, "no Other with four models")
}

func TestUsageReportFeatureTotalsIncludeTasks(t *testing.T) {
	tr := reportTree("p", "main")
	tr.FeatureTasks = map[string][]*store.Task{}
	feature := func(fid, title, tid string) {
		tr.Features[fid] = &store.Feature{ID: fid, Title: title, Status: "done"}
		if tid != "" {
			task := &store.Task{ID: tid, Status: "done"}
			tr.Tasks[tid] = task
			tr.FeatureTasks[fid] = []*store.Task{task}
		}
	}
	feature("F-aaaaaa", "Schema engine", "T-aaaaaa")
	feature("F-bbbbbb", "Older work", "T-bbbbbb")
	feature("F-cccccc", "Own rows only", "")
	feature("F-dddddd", "No usage", "T-dddddd")
	evs := []events.Event{
		statusEv("start.task", onDay(time.October, 5), "in_progress", "T-aaaaaa"),
		statusEv("sync.feature", onDay(time.October, 9), "done", "F-aaaaaa"),
		statusEv("start.task", onDay(time.September, 20), "in_progress", "T-bbbbbb"),
		statusEv("sync.feature", onDay(time.October, 3), "done", "F-bbbbbb"),
		statusEv("start.feature", onDay(time.October, 6), "active", "F-cccccc"),
		statusEv("sync.feature", onDay(time.October, 12), "done", "F-cccccc"),
		statusEv("start.task", onDay(time.October, 6), "in_progress", "T-dddddd"),
		statusEv("sync.feature", onDay(time.October, 13), "done", "F-dddddd"),
	}
	files := []usage.File{
		claudeSinceSep30(),
		usageFile("claude-code", "work",
			usageRow(onDay(time.October, 5), "F-aaaaaa", usage.Planning, "m", input(1000)),
			usageRow(onDay(time.October, 6), "T-aaaaaa", usage.Implementation, "m", input(3000)),
			usageRow(onDay(time.September, 30), "T-bbbbbb", usage.Implementation, "m", input(500)),
			usageRow(onDay(time.October, 7), "F-cccccc", usage.Implementation, "m", input(2000))),
	}
	got := usageReport(tr, files, evs)

	require.Equal(t, []views.UsageFeature{
		{Title: "Schema engine", Done: "2026-10-09T10:00:00Z", Total: 4000, Median: 4000},
		{Title: "Own rows only", Done: "2026-10-12T10:00:00Z", Total: 2000, Median: 3000},
	}, got.Features)
	require.Equal(t, 1, got.FeaturesLeftOut)
}

func TestUsageReportStagesWeeklyAndKPIs(t *testing.T) {
	files := []usage.File{
		usageFile("aider", "s",
			usageRow(onDay(time.October, 6), "T-aaaaaa", usage.Implementation, "m", usage.Tokens{Input: 1, CacheWrite: 2, CacheRead: 3, Output: 4, Reasoning: 99}),
			usageRow(onDay(time.October, 13), "", usage.Stage("mystery"), "m", usage.Tokens{Input: 10, CacheWrite: 20, CacheRead: 30, Output: 40})),
	}
	got := usageReport(reportTree("p", "main"), files, nil)

	require.Equal(t, []views.UsageHarness{{Harness: "aider", Label: "aider", Since: "2026-10-06"}}, got.Harnesses)
	require.Equal(t, views.UsageKPIs{Total: 110, CacheRead: 33, Input: 11, CacheWrite: 22, Output: 44, TiedShare: 10.0 / 110}, got.KPIs)
	require.Equal(t, []views.UsageWeek{{}, {Input: 1, CacheWrite: 2, CacheRead: 3, Output: 4}, {Input: 10, CacheWrite: 20, CacheRead: 30, Output: 40}, {}}, got.Weekly)
	require.Len(t, got.Stages, len(usage.Stages))
	for i, s := range got.Stages {
		require.Equal(t, string(usage.Stages[i]), s.Key)
		require.Equal(t, usage.Stages[i].Label(), s.Label)
	}
	require.Equal(t, views.UsageStage{Key: "implementation", Label: "Implementation", Total: 10, CacheRead: 3}, got.Stages[2])
	require.Equal(t, views.UsageStage{Key: "other", Label: "Other", Total: 100, CacheRead: 30}, got.Stages[8], "an unknown stage counts under other")
}

func TestUsageReportEmpty(t *testing.T) {
	got := usageReport(reportTree("p", "main"), nil, nil)

	require.Len(t, got.Weeks, 4)
	require.Equal(t, []string{"none", "none", "none", "none"}, got.Coverage)
	require.Equal(t, make([]views.UsageWeek, 4), got.Weekly)
	require.Equal(t, views.UsageKPIs{}, got.KPIs)
	require.Nil(t, got.KPIs.MedianPerItem)
	require.Equal(t, "", got.FullCoverage)
	require.Len(t, got.Stages, 9)
	require.Len(t, got.ModelStages, 9)
	labels := make([]string, len(got.ItemBands))
	for i, b := range got.ItemBands {
		labels[i] = b.Label
		require.Zero(t, b.Count)
	}
	require.Equal(t, []string{"<100K", "100K–300K", "300K–1M", "1M–3M", "3M–10M", "10M–30M", "30M–100M", ">100M"}, labels)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	text := string(raw)
	require.NotContains(t, text, "NaN")
	require.NotContains(t, text, "Inf")
	for _, want := range []string{`"median_per_item":null`, `"tied_share":0`, `"harnesses":[]`, `"models":[]`,
		`"features":[]`, `"totals":[]`, `"full_coverage":""`, `"items_unmeasured":0`} {
		require.Contains(t, text, want)
	}
}

// TestUsageReportFilesWithoutEvents is a fresh ledger with usage already
// captured: rows land on the four-week axis and the per-item figures stay
// unknown rather than zero.
func TestUsageReportFilesWithoutEvents(t *testing.T) {
	files := []usage.File{usageFile("codex", "s", usageRow(onDay(time.October, 20), "T-aaaaaa", usage.Implementation, "gpt", input(42)))}
	got := usageReport(reportTree("p", "main"), files, nil)

	require.Len(t, got.Weeks, 4)
	// Codex begins on Tuesday Oct 20, so the current week is partly measured.
	require.Equal(t, []string{"none", "none", "none", "partial"}, got.Coverage)
	require.Equal(t, int64(42), got.Weekly[3].Input)
	require.Equal(t, []views.UsageHarness{{Harness: "codex", Label: "Codex", Since: "2026-10-20"}}, got.Harnesses)
	require.Nil(t, got.KPIs.MedianPerItem)
	require.Zero(t, got.KPIs.Items)
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "NaN")
}

// BenchmarkUsageReportOf is 100 session files of 25 rows over 50 done tasks,
// the shape of a client platform build's usage at a few times its row count.
func BenchmarkUsageReportOf(b *testing.B) {
	tr := reportTree("p", "main")
	var evs []events.Event
	for i := range 50 {
		tid := fmt.Sprintf("T-%06d", i)
		tr.Tasks[tid] = &store.Task{ID: tid, Status: "done"}
		evs = append(evs,
			statusEv("start.task", onDay(time.September, 1).Add(time.Duration(i)*time.Hour), "in_progress", tid),
			statusEv("sync.task", onDay(time.October, 1).Add(time.Duration(i)*time.Hour), "done", tid))
	}
	events.Sort(evs)
	files := make([]usage.File, 100)
	for f := range files {
		rows := make([]usage.Row, 25)
		for r := range rows {
			rows[r] = usageRow(onDay(time.September, 1).Add(time.Duration(f*25+r)*time.Hour),
				fmt.Sprintf("T-%06d", (f+r)%50), usage.Stages[r%len(usage.Stages)], fmt.Sprintf("model-%d", r%6),
				usage.Tokens{Input: 100, CacheWrite: 1000, CacheRead: 30000, Output: 500})
		}
		files[f] = usageFile([]string{"claude-code", "codex", "pi"}[f%3], fmt.Sprintf("s%03d", f), rows...)
	}
	in := views.UsageReportInput{Files: files, Events: evs, Now: usageNow, Location: time.UTC}
	b.ResetTimer()
	for b.Loop() {
		rep := views.UsageReportOf(tr, in)
		if !strings.HasPrefix(rep.FullCoverage, "2026-") {
			b.Fatal("no coverage date")
		}
	}
}
