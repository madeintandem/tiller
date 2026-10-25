package gen

import "github.com/madeintandem/tiller/internal/store"

// currentDependencyTree excludes cancelled work and descendants while keeping
// the original records and all done/archived history. Resolution must still use
// the original snapshot, so filtering cannot disambiguate a legacy key.
func currentDependencyTree(t *store.Tree) *store.Tree {
	if !hasCancelledDependencyWork(t) {
		return t
	}

	current := *t
	current.Epics = make(map[string]*store.Epic, len(t.Epics))
	cancelledEpics := map[string]bool{}
	for id, epic := range t.Epics {
		if epic.Status == "cancelled" {
			if epic.Key != nil {
				cancelledEpics[*epic.Key] = true
			}
			continue
		}
		current.Epics[id] = epic
	}
	current.Features = make(map[string]*store.Feature, len(t.Features))
	current.FeatureTasks = make(map[string][]*store.Task, len(t.FeatureTasks))
	for id, feature := range t.Features {
		if feature.Status == "cancelled" || cancelledEpics[feature.EpicKey] {
			continue
		}
		current.Features[id] = feature
		// Own the slice: filtering must not overwrite the loaded task index.
		for _, task := range t.FeatureTasks[id] {
			if task.Status != "cancelled" {
				current.FeatureTasks[id] = append(current.FeatureTasks[id], task)
			}
		}
	}
	return &current
}

func hasCancelledDependencyWork(t *store.Tree) bool {
	for _, epic := range t.Epics {
		if epic.Status == "cancelled" {
			return true
		}
	}
	for _, feature := range t.Features {
		if feature.Status == "cancelled" {
			return true
		}
	}
	for _, tasks := range t.FeatureTasks {
		for _, task := range tasks {
			if task.Status == "cancelled" {
				return true
			}
		}
	}
	return false
}
