package views

import (
	"sort"
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// medianWindow is how many completed features each rolling median covers.
const medianWindow = 5

// featureTimes reports each done feature's cycle and lead time with the
// rolling medians that end at it, ordered by completion and then by feature
// id (D8, D9). Every instant comes from a real event: a reconciled change has
// an empty Type and is not evidence. A done feature missing a completion, an
// approval, or a start is skipped rather than given an invented duration.
func featureTimes(t *store.Tree, r *replay) []ReportFeatureTime {
	type timed struct {
		id    string
		title string
		done  time.Time
		cycle float64
		lead  float64
	}
	var all []timed
	for fid, f := range t.Features {
		if f.Status != "done" {
			continue
		}
		done, ok := r.lastAt(fid, "done", realChange)
		if !ok {
			continue
		}
		approved, ok := r.firstAt(fid, "ready", realChange)
		if !ok {
			continue
		}
		started, ok := featureStart(t, r, fid)
		if !ok {
			continue
		}
		all = append(all, timed{
			id:    fid,
			title: f.Title,
			done:  done,
			cycle: done.Sub(started).Hours() / 24,
			lead:  done.Sub(approved).Hours() / 24,
		})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].done.Equal(all[j].done) {
			return all[i].id < all[j].id
		}
		return all[i].done.Before(all[j].done)
	})

	out := make([]ReportFeatureTime, len(all))
	for i, f := range all {
		from := max(0, i+1-medianWindow)
		var cycles, leads []float64
		for _, w := range all[from : i+1] {
			cycles = append(cycles, w.cycle)
			leads = append(leads, w.lead)
		}
		out[i] = ReportFeatureTime{
			Title:           f.title,
			CompletedAt:     formatTime(f.done),
			CycleDays:       f.cycle,
			LeadDays:        f.lead,
			CycleMedianDays: *median(cycles),
			LeadMedianDays:  *median(leads),
		}
	}
	return out
}

// featureStart is the earliest real start of a feature: a task moving to
// in_progress through start.task, or the feature itself moving to active
// through start.feature.
func featureStart(t *store.Tree, r *replay, fid string) (time.Time, bool) {
	var best time.Time
	found := false
	consider := func(at time.Time, ok bool) {
		if ok && (!found || at.Before(best)) {
			best, found = at, true
		}
	}
	for _, task := range t.FeatureTasks[fid] {
		consider(r.firstAt(task.ID, "in_progress", changeFrom("start.task")))
	}
	consider(r.firstAt(fid, "active", changeFrom("start.feature")))
	return best, found
}

// median is the middle value, or the mean of the two middle values of an even
// count. It returns nil for no values.
func median(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	s := append([]float64(nil), values...)
	sort.Float64s(s)
	m := s[len(s)/2]
	if len(s)%2 == 0 {
		m = (s[len(s)/2-1] + s[len(s)/2]) / 2
	}
	return &m
}
