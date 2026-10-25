package views

import (
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
)

// minReportWeeks is the fewest weeks a time chart spans, so a young project
// still draws an axis (D18). maxReportWeeks is the most, ten years ending at
// the current week, so one absurdly old timestamp cannot stretch the axis.
const (
	minReportWeeks = 4
	maxReportWeeks = 520
)

// ReportInput is what ReportOf reads besides the tree.
type ReportInput struct {
	Events   []events.Event
	Now      time.Time
	Location *time.Location // week boundaries; nil means time.Local
	Branch   string         // checkout branch from vcs; "" when unknown
	// Observed is the tree with task execution observations applied, read
	// only by epic progress: in-flight task states live on branches, while
	// the replay must stay anchored to the integration tree the event log
	// describes. nil means the integration tree.
	Observed *store.Tree
}

// Report is the client-facing metrics payload behind the Report page. It
// carries counts and durations only: no contributor or owner names, ids, or
// counts (D16).
type Report struct {
	Project        string                 `json:"project"`
	AsOf           string                 `json:"as_of"` // RFC3339, UTC
	OffBranch      *ReportBranchNotice    `json:"off_branch,omitempty"`
	Weeks          []string               `json:"weeks"`           // Monday date labels YYYY-MM-DD; last is the current, partial week
	WeekBoundaries []string               `json:"week_boundaries"` // RFC3339 server-local Mondays, including the final week's end; len == len(Weeks)+1
	KPIs           ReportKPIs             `json:"kpis"`
	BurnUp         []ReportBurnUpWeek     `json:"burn_up"`    // len == len(Weeks)
	Throughput     []ReportThroughputWeek `json:"throughput"` // len == len(Weeks)
	Bugs           []ReportBugWeek        `json:"bugs"`       // len == len(Weeks)
	FeatureTimes   []ReportFeatureTime    `json:"feature_times"`
	Epics          []ReportEpic           `json:"epics"`
}

// ReportBranchNotice says the checkout is not on the integration branch, so
// the numbers include unmerged work.
type ReportBranchNotice struct {
	Branch      string `json:"branch"`
	Integration string `json:"integration"`
}

// ReportKPIs is the headline strip.
type ReportKPIs struct {
	FeaturesDone        int      `json:"features_done"`
	FeaturesTotal       int      `json:"features_total"`
	ShippedLast7Days    int      `json:"shipped_last_7_days"`
	MedianCycleDays     *float64 `json:"median_cycle_days"` // nil with no completed feature
	MedianLeadDays      *float64 `json:"median_lead_days"`
	CompletedFeatures   int      `json:"completed_features"`
	OpenBugs            int      `json:"open_bugs"`
	OpenQuestions       int      `json:"open_questions"`
	AnsweredQuestions   int      `json:"answered_questions"`
	OpenQuestionsByWeek []int    `json:"open_questions_by_week"` // len == len(Weeks)
}

// ReportBurnUpWeek is approved scope and completed work at a week's end.
type ReportBurnUpWeek struct {
	Scope int `json:"scope"`
	Done  int `json:"done"`
}

// ReportThroughputWeek is work finished in a week, split by kind.
type ReportThroughputWeek struct {
	FeatureWork int `json:"feature_work"`
	Fixes       int `json:"fixes"`
	Upkeep      int `json:"upkeep"`
}

// ReportBugWeek is a week's bugs reported, fixed, and still open at its end.
type ReportBugWeek struct {
	Reported int `json:"reported"`
	Fixed    int `json:"fixed"`
	Open     int `json:"open"`
}

// ReportFeatureTime is one completed feature's cycle and lead time with the
// rolling medians that end at it.
type ReportFeatureTime struct {
	Title           string  `json:"title"`
	CompletedAt     string  `json:"completed_at"` // RFC3339
	CycleDays       float64 `json:"cycle_days"`
	LeadDays        float64 `json:"lead_days"`
	CycleMedianDays float64 `json:"cycle_median_days"` // rolling, last 5 including this one
	LeadMedianDays  float64 `json:"lead_median_days"`
}

// ReportEpic is one epic's feature progress. A Planned epic is a draft epic
// the client has not approved yet: it carries its title and PlannedFeatures,
// the count of its non-cancelled features, with the progress counts at zero,
// and it never counts toward scope.
type ReportEpic struct {
	Title           string `json:"title"`
	Done            int    `json:"done"`
	InProgress      int    `json:"in_progress"`
	NotStarted      int    `json:"not_started"`
	Planned         bool   `json:"planned"`
	PlannedFeatures int    `json:"planned_features"`
}

// ReportOf projects the tree and its event log into the Report payload. It is
// pure: the clock, timezone, and checkout branch arrive in the input. t must
// be a loaded tree, so it and its manifest are non-nil. An event with a zero
// timestamp is malformed and neither widens the axis nor changes a status.
func ReportOf(t *store.Tree, in ReportInput) Report {
	loc := in.Location
	if loc == nil {
		loc = time.Local
	}
	first := in.Now
	for _, ev := range in.Events {
		if !ev.At.IsZero() && ev.At.Before(first) {
			first = ev.At
		}
	}
	weeks := weekStarts(first, in.Now, loc)
	last := len(weeks) - 1

	// The replay is the shared foundation the metric builders read.
	r := newReplay(t, in.Events, weeks, in.Now)

	rep := Report{
		Project:        t.Manifest.Project.Name,
		AsOf:           in.Now.UTC().Format(time.RFC3339),
		Weeks:          make([]string, len(weeks)),
		WeekBoundaries: make([]string, len(weeks)+1),
		BurnUp:         burnUp(t, r, weeks, in.Now),
		Throughput:     throughput(t, r, weeks),
		Bugs:           bugSeries(t, r, weeks, in.Now),
		FeatureTimes:   featureTimes(t, r),
		Epics:          epicProgress(observedOr(in.Observed, t)),
	}
	for i, w := range weeks {
		rep.Weeks[i] = w.Format("2006-01-02")
		rep.WeekBoundaries[i] = w.Format(time.RFC3339)
	}
	rep.WeekBoundaries[len(weeks)] = weeks[last].AddDate(0, 0, 7).Format(time.RFC3339)
	var cycles, leads []float64
	for _, ft := range rep.FeatureTimes {
		cycles = append(cycles, ft.CycleDays)
		leads = append(leads, ft.LeadDays)
	}
	rep.KPIs.MedianCycleDays = median(cycles)
	rep.KPIs.MedianLeadDays = median(leads)
	rep.KPIs.CompletedFeatures = len(rep.FeatureTimes)
	rep.KPIs.ShippedLast7Days = shippedSince(t, r, in.Now.Add(-7*24*time.Hour))
	rep.KPIs.OpenQuestionsByWeek = openQuestions(t, r, weeks, in.Now)
	rep.KPIs.OpenBugs = rep.Bugs[last].Open
	rep.KPIs.OpenQuestions = rep.KPIs.OpenQuestionsByWeek[last]
	for qid := range t.Questions {
		if r.statusAt(qid, in.Now) == "answered" {
			rep.KPIs.AnsweredQuestions++
		}
	}
	rep.KPIs.FeaturesDone = rep.BurnUp[last].Done
	rep.KPIs.FeaturesTotal = rep.BurnUp[last].Scope
	branch, integration := in.Branch, t.Manifest.Git.IntegrationBranch
	if branch != "" && integration != "" && branch != integration {
		rep.OffBranch = &ReportBranchNotice{Branch: branch, Integration: integration}
	}
	return rep
}

// weekStarts returns the Mondays, at 00:00 in loc, from the week of first
// through the week of now, oldest first. It pads earlier weeks until there are
// at least minReportWeeks so a young project still has an axis, and keeps at
// most the last maxReportWeeks.
func weekStarts(first, now time.Time, loc *time.Location) []time.Time {
	if first.After(now) {
		first = now
	}
	start := mondayOf(first, loc)
	end := mondayOf(now, loc)
	if earliest := end.AddDate(0, 0, -7*(maxReportWeeks-1)); start.Before(earliest) {
		start = earliest
	}
	var weeks []time.Time
	for w := start; !w.After(end); w = w.AddDate(0, 0, 7) {
		weeks = append(weeks, w)
	}
	for len(weeks) < minReportWeeks {
		weeks = append([]time.Time{weeks[0].AddDate(0, 0, -7)}, weeks...)
	}
	return weeks
}

// mondayOf truncates an instant to 00:00 on its Monday in loc.
func mondayOf(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	back := (int(local.Weekday()) + 6) % 7
	return time.Date(local.Year(), local.Month(), local.Day()-back, 0, 0, 0, 0, loc)
}

// weekEnd is the instant a week's figures are measured at. A non-last week
// ends strictly before the next Monday, since statusAt includes changes at
// the instant itself and an event at exactly Monday belongs to the next week;
// the last week ends at now.
func weekEnd(weeks []time.Time, i int, now time.Time) time.Time {
	if i+1 < len(weeks) {
		return weeks[i+1].Add(-time.Nanosecond)
	}
	return now
}

// weekIndex is the index of the week containing at. It returns -1 before the
// axis and clamps to the last week after it.
func weekIndex(weeks []time.Time, at time.Time) int {
	idx := -1
	for i, w := range weeks {
		if w.After(at) {
			break
		}
		idx = i
	}
	return idx
}
