package gen

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func executionCycleTree() *store.Tree {
	ptr := func(s string) *string { return &s }
	a := &store.Epic{ID: "E-a", Key: ptr("a"), Status: "ready"}
	b := &store.Epic{ID: "E-b", Key: ptr("b"), Status: "ready"}
	first := &store.Feature{ID: "F-first", Key: ptr("a/first"), EpicKey: "a", Status: "ready"}
	middle := &store.Feature{ID: "F-middle", Key: ptr("b/middle"), EpicKey: "b", Status: "ready", DependsOn: []string{first.ID}}
	last := &store.Feature{ID: "F-last", Key: ptr("a/last"), EpicKey: "a", Status: "ready", DependsOn: []string{middle.ID}}
	return &store.Tree{Epics: map[string]*store.Epic{a.ID: a, b.ID: b}, Features: map[string]*store.Feature{first.ID: first, middle.ID: middle, last.ID: last}}
}

func TestClassifyCyclesPreservesGroupingAndRecordedHistory(t *testing.T) {
	tree := executionCycleTree()
	before, err := json.Marshal(tree)
	require.NoError(t, err)
	order := DependencyOrder(tree)
	classification := ClassifyCycles(tree)
	require.Empty(t, classification.Blocking)
	require.Equal(t, []ProjectedCycle{{Members: []string{"E-a", "E-b"}, Edges: []DependencyEdge{{"F-last", "F-middle"}, {"F-middle", "F-first"}}}}, classification.Advisory)
	for range 20 {
		require.Equal(t, classification, ClassifyCycles(tree), "map iteration never changes diagnostics")
		require.Equal(t, order.Epics(), DependencyOrder(tree).Epics())
		require.Equal(t, order.Cycles(), DependencyOrder(tree).Cycles())
	}
	after, err := json.Marshal(tree)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestClassifyCyclesActualObligations(t *testing.T) {
	for _, scenario := range []string{"feature", "epic", "mixed", "inherited", "task", "historical done", "historical archived"} {
		t.Run(scenario, func(t *testing.T) {
			tree := executionCycleTree()
			switch scenario {
			case "feature", "historical done":
				tree.Features["F-first"].DependsOn = []string{"F-last"}
				if scenario == "historical done" {
					tree.Features["F-first"].Status = "done"
				}
			case "epic", "historical archived":
				tree.Epics["E-a"].DependsOn = []string{"E-b"}
				tree.Epics["E-b"].DependsOn = []string{"E-a"}
				if scenario == "historical archived" {
					tree.Epics["E-a"].Status = "archived"
				}
			case "mixed":
				tree.Features["F-middle"].DependsOn = []string{"E-a"}
			case "inherited":
				tree.Epics["E-a"].DependsOn = []string{"F-first"}
			case "task":
				first := &store.Task{ID: "T-a", FeatureID: "F-first", Status: "ready", DependsOn: []string{"T-b"}}
				last := &store.Task{ID: "T-b", FeatureID: "F-first", Status: "ready", DependsOn: []string{"T-a"}}
				tree.Tasks = map[string]*store.Task{first.ID: first, last.ID: last}
				tree.FeatureTasks = map[string][]*store.Task{"F-first": {first, last}}
			}
			got := ClassifyCycles(tree)
			require.Len(t, got.Blocking, 1)
			require.NotEmpty(t, got.Blocking[0].Edges, "every blocker names recorded prerequisites")
			if scenario != "task" && scenario != "inherited" {
				require.Empty(t, got.Advisory, "a genuine cross-epic loop is not relabeled as grouping-only")
			}
		})
	}
}

func TestClassifyCyclesCancellationRecomputesMixedObligations(t *testing.T) {
	for _, scenario := range []string{"cancelled rollup child", "cancelled prerequisite", "cancelled epic descendants", "unrelated cancelled child"} {
		t.Run(scenario, func(t *testing.T) {
			tree := executionCycleTree()
			tree.Features["F-middle"].DependsOn = []string{"E-a"}
			switch scenario {
			case "cancelled rollup child":
				tree.Features["F-last"].Status = "cancelled"
			case "cancelled prerequisite":
				tree.Features["F-middle"].Status = "cancelled"
			case "cancelled epic descendants":
				tree.Epics["E-a"].Status = "cancelled"
			case "unrelated cancelled child":
				tree.Features["F-first"].Status = "cancelled"
			}
			before, err := json.Marshal(tree)
			require.NoError(t, err)
			result := ClassifyCycles(tree)
			if scenario == "unrelated cancelled child" {
				require.Len(t, result.Blocking, 1, "the remaining mixed loop still blocks")
				require.NotContains(t, result.Blocking[0].Members, "F-first")
			} else {
				require.Empty(t, result.Blocking)
				require.Empty(t, result.Advisory)
			}
			after, err := json.Marshal(tree)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
