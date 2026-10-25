package readiness

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestCheckCancelledDependenciesDoNotBlockReadiness(t *testing.T) {
	for _, scenario := range []string{"cancelled features in retained epics", "cancelled epic with live descendants", "cancelled feature target by key", "cancelled task", "cancelled feature with live tasks"} {
		t.Run(scenario, func(t *testing.T) {
			m := loadRepo(t, completeManifest, true)
			a := cycleFeature("F-alpha", "alpha/work", "alpha", "F-beta")
			b := cycleFeature("F-beta", "beta/work", "beta", "F-alpha")
			tree := &store.Tree{
				Epics: map[string]*store.Epic{
					"E-alpha": cycleEpic("E-alpha", "alpha"),
					"E-beta":  cycleEpic("E-beta", "beta"),
				},
				Features: map[string]*store.Feature{a.ID: a, b.ID: b},
			}
			switch scenario {
			case "cancelled features in retained epics":
				a.Status, b.Status = "cancelled", "cancelled"
				tree.Features["F-new"] = cycleFeature("F-new", "alpha/replacement", "alpha")
			case "cancelled epic with live descendants":
				tree.Epics["E-beta"].Status = "cancelled"
				tree.Epics["E-alpha"].DependsOn = []string{"beta"}
				tree.Epics["E-beta"].DependsOn = []string{"E-alpha"}
			case "cancelled feature target by key":
				b.Status = "cancelled"
				a.DependsOn = []string{*b.Key}
			case "cancelled task", "cancelled feature with live tasks":
				a.DependsOn, b.DependsOn = nil, nil
				firstKey, secondKey := "alpha/work/first", "alpha/work/second"
				first := &store.Task{ID: "T-first", Key: &firstKey, FeatureID: a.ID, Status: "ready", DependsOn: []string{"T-second"}}
				second := &store.Task{ID: "T-second", Key: &secondKey, FeatureID: a.ID, Status: "ready", DependsOn: []string{"T-first"}}
				tree.Tasks = map[string]*store.Task{first.ID: first, second.ID: second}
				tree.FeatureTasks = map[string][]*store.Task{a.ID: {first, second}}
				if scenario == "cancelled task" {
					second.Status = "cancelled"
				} else {
					a.Status = "cancelled"
				}
			}
			before, err := json.Marshal(tree)
			require.NoError(t, err)
			historical := gen.DependencyOrder(tree).Cycles()
			require.NotEmpty(t, historical)

			report := Check(m, tree)

			require.True(t, report.Ready, "%+v", report.Gaps)
			require.Empty(t, report.Gaps)
			after, err := json.Marshal(tree)
			require.NoError(t, err)
			require.Equal(t, before, after, "readiness must not rewrite dependencies or task slices")
			require.Equal(t, historical, gen.DependencyOrder(tree).Cycles(), "historical ordering remains inspectable")
		})
	}
}

func TestCheckCancellationDoesNotHideRemainingLiveCycle(t *testing.T) {
	m := loadRepo(t, completeManifest, true)
	tree := &store.Tree{
		Epics: map[string]*store.Epic{"E-alpha": cycleEpic("E-alpha", "alpha")},
		Features: map[string]*store.Feature{
			"F-first":  cycleFeature("F-first", "alpha/first", "alpha", "F-second", "F-old"),
			"F-second": cycleFeature("F-second", "alpha/second", "alpha", "F-first"),
			"F-old":    cycleFeature("F-old", "alpha/old", "alpha", "F-first"),
		},
	}
	tree.Features["F-old"].Status = "cancelled"

	report := Check(m, tree)

	require.False(t, report.Ready)
	require.Len(t, report.Gaps, 1)
	require.Equal(t, ReasonDependencyCycle, report.Gaps[0].Reason)
	require.Contains(t, report.Gaps[0].Message, "alpha/first")
	require.Contains(t, report.Gaps[0].Message, "alpha/second")
	require.NotContains(t, report.Gaps[0].Message, "alpha/old")
}

func TestCheckCancelledFeatureEdgesDoNotImplyEpicCycle(t *testing.T) {
	m := loadRepo(t, completeManifest, true)
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-alpha": cycleEpic("E-alpha", "alpha"),
			"E-beta":  cycleEpic("E-beta", "beta"),
		},
		Features: map[string]*store.Feature{
			"F-base":       cycleFeature("F-base", "beta/base", "beta"),
			"F-package":    cycleFeature("F-package", "alpha/package", "alpha", "F-base"),
			"F-regenerate": cycleFeature("F-regenerate", "beta/regenerate", "beta", "F-package"),
			"F-current":    cycleFeature("F-current", "alpha/current", "alpha"),
		},
	}
	for _, id := range []string{"F-base", "F-package", "F-regenerate"} {
		tree.Features[id].Status = "cancelled"
	}
	historical := gen.DependencyOrder(tree).Cycles()
	require.Len(t, historical, 1)
	require.Equal(t, store.NounEpic, historical[0].Noun)

	report := Check(m, tree)

	require.True(t, report.Ready, "%+v", report.Gaps)
	require.Empty(t, report.Gaps)
}
