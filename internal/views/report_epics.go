package views

import (
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// epicProgress is the per-epic feature snapshot (D10). It lists every approved
// epic (ready, done, or archived) in the order the Build board uses, with its
// non-cancelled features bucketed by current status. Draft epics interleave in
// the same order as Planned rows carrying only their planned feature count, so
// a client sees the future work without it counting as scope. It reads the
// tree only, not the replay.
// The result is never nil.
func epicProgress(t *store.Tree) []ReportEpic {
	out := []ReportEpic{}
	for _, id := range gen.DependencyOrder(t).Epics() {
		epic := t.Epics[id]
		if epic == nil {
			continue
		}
		switch epic.Status {
		case "draft":
			out = append(out, ReportEpic{Title: epic.Title, Planned: true, PlannedFeatures: plannedFeatures(t, epic)})
		case "ready", "done", "archived":
			if epic.Key != nil {
				out = append(out, approvedProgress(t, epic))
			}
		}
	}
	return out
}

func observedOr(observed, t *store.Tree) *store.Tree {
	if observed != nil {
		return observed
	}
	return t
}

// approvedProgress buckets an approved epic's features by their board column,
// the column Build draws: the start that activates a feature is recorded on
// its task's branch, so a ready feature with a task in flight is in progress,
// and a feature waiting on review is still in progress until it merges.
func approvedProgress(t *store.Tree, epic *store.Epic) ReportEpic {
	re := ReportEpic{Title: epic.Title}
	for _, f := range t.Features {
		if f.EpicKey != *epic.Key {
			continue
		}
		switch gen.FeatureBoardStatus(t, f) {
		case "done":
			re.Done++
		case "active", gen.ReviewState:
			re.InProgress++
		case "draft", "specced", "ready":
			re.NotStarted++
		}
	}
	return re
}

// plannedFeatures counts a draft epic's non-cancelled features. A keyless
// epic has none.
func plannedFeatures(t *store.Tree, epic *store.Epic) int {
	if epic.Key == nil {
		return 0
	}
	n := 0
	for _, f := range t.Features {
		if f.EpicKey == *epic.Key && f.Status != "cancelled" {
			n++
		}
	}
	return n
}
