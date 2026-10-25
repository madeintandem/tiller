package views

import (
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// shippedAt visits each task, bug, chore, and tweak that reached done, with
// the instant it first did. A carried bug crosses done through its own sync.bug in the
// carrier's sync run, so it needs no special case.
func shippedAt(t *store.Tree, r *replay, visit func(id string, at time.Time)) {
	each := func(id string) {
		if at, ok := r.firstAt(id, "done", anyChange); ok {
			visit(id, at)
		}
	}
	for id := range t.Tasks {
		each(id)
	}
	for id := range t.Bugs {
		each(id)
	}
	for id := range t.Chores {
		each(id)
	}
	for id := range t.Tweaks {
		each(id)
	}
}

// throughput counts finished work per week, once per item in the week of its
// first done: tasks as feature work, bugs as fixes, chores and tweaks as
// upkeep (D7).
func throughput(t *store.Tree, r *replay, weeks []time.Time) []ReportThroughputWeek {
	out := make([]ReportThroughputWeek, len(weeks))
	shippedAt(t, r, func(id string, at time.Time) {
		i := weekIndex(weeks, at)
		if i < 0 {
			return
		}
		switch idPrefix(id) {
		case "T":
			out[i].FeatureWork++
		case "B":
			out[i].Fixes++
		default:
			out[i].Upkeep++
		}
	})
	return out
}

// shippedSince counts items whose first done is after since.
func shippedSince(t *store.Tree, r *replay, since time.Time) int {
	n := 0
	shippedAt(t, r, func(_ string, at time.Time) {
		if at.After(since) {
			n++
		}
	})
	return n
}

// bugSeries reports, per week, bugs first seen, bugs first done, and bugs
// still unresolved at the week's end (D11).
func bugSeries(t *store.Tree, r *replay, weeks []time.Time, now time.Time) []ReportBugWeek {
	out := make([]ReportBugWeek, len(weeks))
	for id := range t.Bugs {
		if at, ok := reportedAt(r, id); ok {
			if i := weekIndex(weeks, at); i >= 0 {
				out[i].Reported++
			}
		}
		if at, ok := r.firstAt(id, "done", anyChange); ok {
			if i := weekIndex(weeks, at); i >= 0 {
				out[i].Fixed++
			}
		}
		for i := range weeks {
			switch r.statusAt(id, weekEnd(weeks, i, now)) {
			case "", "done", "cancelled":
			default:
				out[i].Open++
			}
		}
	}
	return out
}

// reportedAt is when a bug was reported: its first add.bug, or with none its
// first change, which for a bug with no recorded change is its reconciled
// start in the week of the earliest event. A transfer replacement carries its
// original's report, so a bug whose first change is a transfer was not
// reported anew.
func reportedAt(r *replay, bugID string) (time.Time, bool) {
	line := r.changes(bugID)
	for _, c := range line {
		if c.Type == "add.bug" {
			return c.At, true
		}
	}
	if len(line) == 0 || strings.HasPrefix(line[0].Type, "transfer.") {
		return time.Time{}, false
	}
	return line[0].At, true
}

// openQuestions counts questions still open at each week's end (D12).
func openQuestions(t *store.Tree, r *replay, weeks []time.Time, now time.Time) []int {
	out := make([]int, len(weeks))
	for id := range t.Questions {
		for i := range weeks {
			if r.statusAt(id, weekEnd(weeks, i, now)) == "open" {
				out[i]++
			}
		}
	}
	return out
}
