package readiness

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func interleavingTree() *store.Tree {
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-office": cycleEpic("E-office", "office"),
			"E-domain": cycleEpic("E-domain", "domain"),
		},
		Features: map[string]*store.Feature{
			"F-early":  cycleFeature("F-early", "office/early", "office"),
			"F-domain": cycleFeature("F-domain", "domain/work", "domain", "F-early"),
			"F-later":  cycleFeature("F-later", "office/later", "office", "F-domain"),
		},
	}
}

func TestExecutionCyclesInterleavingIsAdvisory(t *testing.T) {
	for _, status := range []string{"draft", "done"} {
		t.Run(status, func(t *testing.T) {
			tree := interleavingTree()
			tree.Features["F-early"].Status = status
			report := Check(loadRepo(t, completeManifest, true), tree)
			require.True(t, report.Ready)
			require.Empty(t, report.Gaps)
			data, err := json.Marshal(report)
			require.NoError(t, err)
			var wire struct {
				Advisories []Gap `json:"advisories"`
			}
			require.NoError(t, json.Unmarshal(data, &wire))
			require.Len(t, wire.Advisories, 1)
			require.Equal(t, "projected_epic_order_cycle", wire.Advisories[0].Reason)
			for _, key := range []string{"office/early", "domain/work", "office/later"} {
				require.Contains(t, wire.Advisories[0].Message, key)
			}
		})
	}
}

func TestExecutionCyclesMixedCompletionDeadlockBlocks(t *testing.T) {
	tree := interleavingTree()
	// Requiring the entire office epic, instead of its early feature, makes
	// domain/work wait for office/later, which already waits for domain/work.
	tree.Features["F-domain"].DependsOn = []string{"E-office"}
	report := Check(loadRepo(t, completeManifest, true), tree)
	require.False(t, report.Ready)
	require.Len(t, report.Gaps, 1)
	require.Equal(t, ReasonDependencyCycle, report.Gaps[0].Reason)
	for _, key := range []string{"office", "domain/work", "office/later"} {
		require.Contains(t, report.Gaps[0].Message, key)
	}
}

func TestExecutionCyclesInheritedEpicPrerequisiteBlocks(t *testing.T) {
	tree := interleavingTree()
	// There is no projected epic loop here: the parent requires its own
	// child to finish before that child can execute.
	tree.Features["F-domain"].DependsOn = nil
	tree.Features["F-later"].DependsOn = nil
	tree.Epics["E-office"].DependsOn = []string{"F-early"}
	report := Check(loadRepo(t, completeManifest, true), tree)
	require.False(t, report.Ready)
	require.Len(t, report.Gaps, 1)
	require.Contains(t, report.Gaps[0].Message, "office/early")
	require.Contains(t, report.Gaps[0].Message, "office")
}
