package views

import (
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

func featureHasAssignment(t *store.Tree, feature *store.Feature, owner string) bool {
	if owner == "" {
		return false
	}
	for _, task := range t.FeatureTasks[feature.ID] {
		if task.Owner == owner && gen.UnfinishedAssignment(task.Status) {
			return true
		}
	}
	return false
}
