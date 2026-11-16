package gen

import "github.com/madeintandem/tiller/internal/store"

// RidesSameEpicAggregate reports whether dependent and prerequisite are
// features of one epic that both take their review on that epic's aggregate.
// A feature that overrides to task or feature grain takes its own review and
// never rides it.
func RidesSameEpicAggregate(t *store.Tree, dependent, prerequisite *store.Feature) bool {
	if dependent == nil || prerequisite == nil || dependent.EpicKey != prerequisite.EpicKey {
		return false
	}
	for _, epic := range t.Epics {
		if keyStr(epic.Key) == dependent.EpicKey {
			return store.ResolvedReviewGrain(t.Manifest, epic, dependent) == store.ReviewGrainEpic &&
				store.ResolvedReviewGrain(t.Manifest, epic, prerequisite) == store.ReviewGrainEpic
		}
	}
	return false
}

// FeatureTasksSubmitted reports whether every live task under f is in review
// or done, ready for the coarse review. A held task is not live work yet, so
// it neither counts nor blocks (AmendmentHold).
func FeatureTasksSubmitted(t *store.Tree, f *store.Feature) bool {
	live := 0
	for _, tk := range t.FeatureTasks[f.ID] {
		if tk.Status == "cancelled" || AmendmentHold(t, tk) != nil {
			continue
		}
		live++
		// A task PR, or a task started at task grain (R6), is a
		// task-owned review the aggregate never absorbs; it holds the submit.
		if tk.Status == ReviewState && (tk.PR != nil || tk.StartedGrain == store.ReviewGrainTask) {
			return false
		}
		if tk.Status != ReviewState && tk.Status != "done" {
			return false
		}
	}
	return live > 0
}

// FeaturePrerequisiteSubmitted reports whether dependent's recorded
// prerequisite on a feature of the same epic is met before that feature is
// done: both ride the epic's aggregate and every live task of the
// prerequisite is submitted. Start verifies Git inclusion in the aggregate;
// this is the pure hint routing reads.
func FeaturePrerequisiteSubmitted(t *store.Tree, dependent, prerequisite *store.Feature) bool {
	return RidesSameEpicAggregate(t, dependent, prerequisite) && FeatureTasksSubmitted(t, prerequisite)
}
