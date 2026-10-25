package gen

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestClassifyCyclesEpicReviewCompletionObligations(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		configure func(*store.Tree)
		blocking  bool
	}{
		{"epic aggregate", func(tree *store.Tree) { tree.Epics["E-a"].ReviewGrain = store.ReviewGrainEpic }, true},
		{"project epic aggregate", func(tree *store.Tree) {
			tree.Manifest = &store.Manifest{}
			tree.Manifest.Execution.ReviewGrain = store.ReviewGrainEpic
		}, true},
		{"early task override", func(tree *store.Tree) {
			tree.Epics["E-a"].ReviewGrain = store.ReviewGrainEpic
			tree.Features["F-first"].ReviewGrain = store.ReviewGrainTask
		}, false},
		{"early feature override", func(tree *store.Tree) {
			tree.Epics["E-a"].ReviewGrain = store.ReviewGrainEpic
			tree.Features["F-first"].ReviewGrain = store.ReviewGrainFeature
		}, false},
		{"late independent review still required", func(tree *store.Tree) {
			tree.Epics["E-a"].ReviewGrain = store.ReviewGrainEpic
			tree.Features["F-last"].ReviewGrain = store.ReviewGrainFeature
		}, true},
		{"already completed early feature", func(tree *store.Tree) {
			tree.Epics["E-a"].ReviewGrain = store.ReviewGrainEpic
			tree.Features["F-first"].Status = "done"
		}, false},
		{"cancelled aggregate sibling", func(tree *store.Tree) {
			tree.Epics["E-a"].ReviewGrain = store.ReviewGrainEpic
			tree.Features["F-last"].Status = "cancelled"
		}, false},
		{"independent riders with sibling task assembly", func(tree *store.Tree) {
			tree.Epics["E-a"].ReviewGrain = store.ReviewGrainEpic
			tree.Features["F-middle"].DependsOn = nil
			tree.Features["F-last"].DependsOn = nil
			first := tree.FeatureTasks["F-first"][0]
			first.Status = "in_review"
			second := &store.Task{ID: "T-next", FeatureID: "F-first", Status: "ready", DependsOn: []string{first.ID}}
			tree.Tasks[second.ID] = second
			tree.FeatureTasks["F-first"] = append(tree.FeatureTasks["F-first"], second)
		}, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			tree := executionCycleTree()
			tree.Tasks = map[string]*store.Task{}
			tree.FeatureTasks = map[string][]*store.Task{}
			for id, feature := range tree.Features {
				task := &store.Task{ID: "T-" + id, FeatureID: id, Status: "ready"}
				tree.Tasks[task.ID] = task
				tree.FeatureTasks[id] = []*store.Task{task}
				feature.Status = "ready"
			}
			scenario.configure(tree)
			if tree.Features["F-first"].Status == "done" {
				tree.FeatureTasks["F-first"][0].Status = "done"
			}
			got := ClassifyCycles(tree)
			if scenario.blocking {
				require.Len(t, got.Blocking, 1)
				require.Empty(t, got.Advisory)
				require.Contains(t, got.Blocking[0].Edges, DependencyEdge{Source: "F-middle", Target: "F-first"})
				require.Contains(t, got.Blocking[0].Edges, DependencyEdge{Source: "F-last", Target: "F-middle"})
				require.Contains(t, got.Blocking[0].ReviewEpics, "E-a")
				for _, id := range got.Blocking[0].Members {
					require.True(t, tree.Epics[id] != nil || tree.Features[id] != nil || tree.Tasks[id] != nil, "diagnostics expose work identities, never internal obligation vertices")
				}
			} else {
				require.Empty(t, got.Blocking)
			}
		})
	}
}
