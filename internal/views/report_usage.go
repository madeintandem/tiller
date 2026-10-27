package views

import (
	"math"
	"sort"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/usage"
)

// UsageReportInput is what UsageReportOf reads besides the tree.
type UsageReportInput struct {
	Files    []usage.File
	Events   []events.Event
	Now      time.Time
	Location *time.Location // week boundaries and harness dates; nil means time.Local
}

// UsageReport is the Report page's Agent use payload. Like Report it carries
// counts only: no contributor, owner, node id, or key. The one name it holds is
// a finished feature's title, for the per-feature chart's hover.
type UsageReport struct {
	Weeks           []string          `json:"weeks"`           // identical to Report.Weeks for the same events and clock
	WeekBoundaries  []string          `json:"week_boundaries"` // identical to Report.WeekBoundaries
	Harnesses       []UsageHarness    `json:"harnesses"`
	FullCoverage    string            `json:"full_coverage"` // YYYY-MM-DD, the latest harness since; "" with no usage
	Coverage        []string          `json:"coverage"`      // per week: "none", "partial", or "full"
	KPIs            UsageKPIs         `json:"kpis"`
	Weekly          []UsageWeek       `json:"weekly"`       // len == len(Weeks)
	Stages          []UsageStage      `json:"stages"`       // all nine, in display order
	Models          []string          `json:"models"`       // top four by total, then "Other" when more remain
	ModelStages     []UsageModelStage `json:"model_stages"` // all nine stages, totals aligned to Models
	Features        []UsageFeature    `json:"features"`
	FeaturesLeftOut int               `json:"features_left_out"`
	ItemBands       []UsageBand       `json:"item_bands"`       // the eight bands, smallest first
	ItemsLeftOut    int               `json:"items_left_out"`   // done items with usage that started before full coverage
	ItemsUnmeasured int               `json:"items_unmeasured"` // done items with no usage of their own
}

// UsageHarness is one harness measured, with the local date of its earliest
// captured hour.
type UsageHarness struct {
	Harness string `json:"harness"`
	Label   string `json:"label"`
	Since   string `json:"since"` // YYYY-MM-DD
}

// UsageKPIs is the headline strip. Totals count every captured row; the
// per-item median and count cover only work items under the coverage rule.
type UsageKPIs struct {
	Total         int64   `json:"total"`
	CacheRead     int64   `json:"cache_read"`
	Input         int64   `json:"input"`
	CacheWrite    int64   `json:"cache_write"`
	Output        int64   `json:"output"`
	MedianPerItem *int64  `json:"median_per_item"` // nil with no work item
	Items         int     `json:"items"`
	TiedShare     float64 `json:"tied_share"` // fraction 0..1 of tokens on rows tied to a node
}

// UsageWeek is one week's four token counts.
type UsageWeek struct {
	Input      int64 `json:"input"`
	CacheWrite int64 `json:"cache_write"`
	CacheRead  int64 `json:"cache_read"`
	Output     int64 `json:"output"`
}

// UsageStage is one stage's total and its cache reads.
type UsageStage struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Total     int64  `json:"total"`
	CacheRead int64  `json:"cache_read"`
}

// UsageModelStage is one stage's tokens split by model, aligned to Models.
type UsageModelStage struct {
	Stage  string  `json:"stage"`
	Totals []int64 `json:"totals"`
}

// UsageFeature is one finished feature's tokens, its own plus its tasks', with
// the rolling median that ends at it.
type UsageFeature struct {
	Title  string `json:"title"`
	Done   string `json:"done"` // RFC3339
	Total  int64  `json:"total"`
	Median int64  `json:"median"` // rolling, last 5 including this one
}

// UsageBand is one size band of the per-work-item histogram.
type UsageBand struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// otherModels is the label for every model past the top four.
const otherModels = "Other"

// topModels is how many models are named before the rest fold into Other.
const topModels = 4

// usageBands are the histogram's bands: a total falls in the first band whose
// upper bound it is under, and the last band has none.
var usageBands = []struct {
	label string
	under int64
}{
	{"<100K", 100_000},
	{"100K–300K", 300_000},
	{"300K–1M", 1_000_000},
	{"1M–3M", 3_000_000},
	{"3M–10M", 10_000_000},
	{"10M–30M", 30_000_000},
	{"30M–100M", 100_000_000},
	{">100M", math.MaxInt64},
}

// harnessLabels are the page names of the known harnesses; any other id is
// shown as written.
var harnessLabels = map[string]string{"claude-code": "Claude Code", "codex": "Codex", "pi": "Pi"}

// UsageReportOf projects the committed usage files onto the Project report's
// week axis. It is pure: the clock and timezone arrive in the input, and it
// never modifies the files it is handed. t must be a loaded tree.
func UsageReportOf(t *store.Tree, in UsageReportInput) UsageReport {
	loc := in.Location
	if loc == nil {
		loc = time.Local
	}
	weeks := reportWeeks(in.Events, in.Now, loc)
	r := newReplay(t, in.Events, weeks, in.Now)
	sums := sumUsage(in.Files, weeks)

	rep := UsageReport{
		Weekly:    sums.weekly,
		Harnesses: []UsageHarness{},
		Coverage:  make([]string, len(weeks)),
	}
	rep.Weeks, rep.WeekBoundaries = weekLabels(weeks)

	fullAt, coverage := harnessCoverage(sums.harnesses, loc, &rep)
	for i, w := range weeks {
		rep.Coverage[i] = coverage(w)
	}

	rep.KPIs = UsageKPIs{
		Total:      sums.all.Total(),
		CacheRead:  sums.all.CacheRead,
		Input:      sums.all.Input,
		CacheWrite: sums.all.CacheWrite,
		Output:     sums.all.Output,
	}
	if total := sums.all.Total(); total > 0 {
		rep.KPIs.TiedShare = float64(sums.tied) / float64(total)
	}
	for i, s := range usage.Stages {
		rep.Stages = append(rep.Stages, UsageStage{
			Key: string(s), Label: s.Label(),
			Total: sums.stages[i].Total(), CacheRead: sums.stages[i].CacheRead,
		})
	}
	rep.Models, rep.ModelStages = modelStages(sums.models)

	items, left, unmeasured := workItemTotals(t, r, sums.nodes, fullAt)
	rep.ItemsLeftOut, rep.ItemsUnmeasured = left, unmeasured
	rep.KPIs.Items = len(items)
	rep.KPIs.MedianPerItem = medianTokens(items)
	rep.ItemBands = bandCounts(items)
	rep.Features, rep.FeaturesLeftOut = featureTokens(t, r, sums.nodes, fullAt)
	return rep
}

// usageSums is one pass over every captured row.
type usageSums struct {
	all       usage.Tokens
	tied      int64                     // tokens on rows tied to a node
	weekly    []UsageWeek               // per axis week
	stages    []usage.Tokens            // aligned to usage.Stages
	models    []map[string]int64        // per stage, tokens by model
	nodes     map[string]int64          // tokens by node id; present means the node has rows
	harnesses map[string]*harnessExtent // by harness id
}

type harnessExtent struct {
	earliest time.Time
	total    int64
}

func sumUsage(files []usage.File, weeks []time.Time) usageSums {
	stageIndex := make(map[usage.Stage]int, len(usage.Stages))
	for i, s := range usage.Stages {
		stageIndex[s] = i
	}
	other := stageIndex[usage.Other]
	s := usageSums{
		weekly:    make([]UsageWeek, len(weeks)),
		stages:    make([]usage.Tokens, len(usage.Stages)),
		models:    make([]map[string]int64, len(usage.Stages)),
		nodes:     map[string]int64{},
		harnesses: map[string]*harnessExtent{},
	}
	for i := range s.models {
		s.models[i] = map[string]int64{}
	}
	for _, f := range files {
		for _, row := range f.Rows {
			total := row.Total()
			s.all.Add(row.Tokens)
			if row.Node != "" {
				s.tied += total
				s.nodes[row.Node] += total
			}
			// A row before the axis counts at its start, as an early event
			// does; weekIndex already clamps a row after it to the last week.
			w := &s.weekly[max(0, weekIndex(weeks, row.Hour))]
			w.Input += row.Input
			w.CacheWrite += row.CacheWrite
			w.CacheRead += row.CacheRead
			w.Output += row.Output
			stage, known := stageIndex[row.Stage]
			if !known {
				stage = other
			}
			s.stages[stage].Add(row.Tokens)
			s.models[stage][row.Model] += total
			h := s.harnesses[f.Harness]
			if h == nil {
				h = &harnessExtent{earliest: row.Hour}
				s.harnesses[f.Harness] = h
			}
			if row.Hour.Before(h.earliest) {
				h.earliest = row.Hour
			}
			h.total += total
		}
	}
	return s
}

// harnessCoverage fills the harness list and the full-coverage date, and
// returns the instant full coverage begins (local midnight of that date) with
// the rule that classifies a week by its Monday: none when it ends at or
// before the earliest harness date, partial when any of it precedes full
// coverage, full otherwise. With no usage every week is none.
func harnessCoverage(harnesses map[string]*harnessExtent, loc *time.Location, rep *UsageReport) (time.Time, func(time.Time) string) {
	if len(harnesses) == 0 {
		return time.Time{}, func(time.Time) string { return "none" }
	}
	ids := make([]string, 0, len(harnesses))
	for id := range harnesses {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := harnesses[ids[i]], harnesses[ids[j]]
		if a.total != b.total {
			return a.total > b.total
		}
		return ids[i] < ids[j]
	})
	var earliestAt, fullAt time.Time
	for i, id := range ids {
		local := harnesses[id].earliest.In(loc)
		since := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		if i == 0 || since.Before(earliestAt) {
			earliestAt = since
		}
		if i == 0 || since.After(fullAt) {
			fullAt = since
		}
		label, ok := harnessLabels[id]
		if !ok {
			label = id
		}
		rep.Harnesses = append(rep.Harnesses, UsageHarness{Harness: id, Label: label, Since: since.Format("2006-01-02")})
	}
	rep.FullCoverage = fullAt.Format("2006-01-02")
	return fullAt, func(week time.Time) string {
		switch {
		case !week.AddDate(0, 0, 7).After(earliestAt):
			return "none"
		case week.Before(fullAt):
			return "partial"
		default:
			return "full"
		}
	}
}

// modelStages names the top four models by total tokens, ties by model id,
// folds the rest into Other, and splits each stage's tokens across them.
func modelStages(perStage []map[string]int64) ([]string, []UsageModelStage) {
	totals := map[string]int64{}
	for _, byModel := range perStage {
		for model, n := range byModel {
			totals[model] += n
		}
	}
	ranked := make([]string, 0, len(totals))
	for model := range totals {
		ranked = append(ranked, model)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if totals[ranked[i]] != totals[ranked[j]] {
			return totals[ranked[i]] > totals[ranked[j]]
		}
		return ranked[i] < ranked[j]
	})
	models := ranked
	if len(ranked) > topModels {
		models = append(append([]string{}, ranked[:topModels]...), otherModels)
	}
	column := make(map[string]int, len(ranked))
	for i, model := range ranked {
		column[model] = min(i, topModels)
	}
	out := make([]UsageModelStage, len(usage.Stages))
	for i, s := range usage.Stages {
		out[i] = UsageModelStage{Stage: string(s), Totals: make([]int64, len(models))}
		for model, n := range perStage[i] {
			out[i].Totals[column[model]] += n
		}
	}
	return models, out
}

// medianTokens is the median of token totals, rounded to a whole token, or nil
// with none.
func medianTokens(totals []int64) *int64 {
	values := make([]float64, len(totals))
	for i, n := range totals {
		values[i] = float64(n)
	}
	m := median(values)
	if m == nil {
		return nil
	}
	rounded := int64(math.Round(*m))
	return &rounded
}

// bandCounts places each work item's total in its size band.
func bandCounts(totals []int64) []UsageBand {
	out := make([]UsageBand, len(usageBands))
	for i, b := range usageBands {
		out[i].Label = b.label
	}
	for _, n := range totals {
		for i, b := range usageBands {
			if n < b.under || i == len(usageBands)-1 {
				out[i].Count++
				break
			}
		}
	}
	return out
}
