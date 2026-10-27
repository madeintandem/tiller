package views

import (
	"sort"
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// workItemTotals returns the token total of each work item under the coverage
// rule, and how many done items with usage of their own it left out. A work
// item is a done task, bug, chore, or tweak with rows of its own whose first
// real start is on or after fullAt; a carried item that never started on its
// own, or one started before every harness was measured, is left out rather
// than drawn artificially cheap. In-progress items would pull the figures down
// with partial totals, and items with no rows are unknown, so neither counts.
func workItemTotals(t *store.Tree, r *replay, nodes map[string]int64, fullAt time.Time) (items []int64, leftOut int) {
	consider := func(id, status, noun string) {
		total, has := nodes[id]
		if status != "done" || !has {
			return
		}
		started, ok := r.firstAt(id, "in_progress", changeFrom("start."+noun))
		if !ok || started.Before(fullAt) {
			leftOut++
			return
		}
		items = append(items, total)
	}
	for id, task := range t.Tasks {
		consider(id, task.Status, store.NounTask)
	}
	for id, b := range t.Bugs {
		consider(id, b.Status, store.NounBug)
	}
	for id, c := range t.Chores {
		consider(id, c.Status, store.NounChore)
	}
	for id, tw := range t.Tweaks {
		consider(id, tw.Status, store.NounTweak)
	}
	return items, leftOut
}

// featureTokens returns each finished feature's tokens, its own rows plus its
// tasks', with the rolling median over the last five, ordered by completion
// and then by feature id. A feature is dated as the lead and cycle chart dates
// it: its latest real move to done. It is plotted only when it has usage and
// its first start is on or after fullAt; every other done feature with usage
// is counted as left out.
func featureTokens(t *store.Tree, r *replay, nodes map[string]int64, fullAt time.Time) ([]UsageFeature, int) {
	type finished struct {
		id    string
		title string
		done  time.Time
		total int64
	}
	var all []finished
	leftOut := 0
	for fid, f := range t.Features {
		if f.Status != "done" {
			continue
		}
		total, has := nodes[fid]
		for _, task := range t.FeatureTasks[fid] {
			n, ok := nodes[task.ID]
			total += n
			has = has || ok
		}
		if !has {
			continue
		}
		done, ok := r.lastAt(fid, "done", realChange)
		started, startOK := featureStart(t, r, fid)
		if !ok || !startOK || started.Before(fullAt) {
			leftOut++
			continue
		}
		all = append(all, finished{id: fid, title: f.Title, done: done, total: total})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].done.Equal(all[j].done) {
			return all[i].id < all[j].id
		}
		return all[i].done.Before(all[j].done)
	})
	out := make([]UsageFeature, len(all))
	for i, f := range all {
		window := make([]int64, 0, medianWindow)
		for _, w := range all[max(0, i+1-medianWindow) : i+1] {
			window = append(window, w.total)
		}
		out[i] = UsageFeature{Title: f.title, Done: formatTime(f.done), Total: f.total, Median: *medianTokens(window)}
	}
	return out, leftOut
}
