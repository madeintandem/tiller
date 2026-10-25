package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/readiness"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func TestReadoutDistinguishesInterleavingFromCompletionDeadlock(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	for _, epic := range tree.Epics {
		epic.Status = "ready"
	}
	early := feat(t, "F-early", "epic-0/early", "epic-0")
	early.Status = "done"
	middle := feat(t, "F-middle", "epic-1/middle", "epic-1")
	middle.DependsOn = []string{early.ID}
	last := feat(t, "F-last", "epic-0/last", "epic-0")
	last.DependsOn = []string{middle.ID}
	tree.Features[early.ID], tree.Features[middle.ID], tree.Features[last.ID] = early, middle, last
	for _, blocking := range []bool{false, true} {
		if blocking {
			middle.DependsOn = []string{"E-aaaaa0"}
		}
		diagnostics := readiness.DependencyDiagnostics(tree)
		for _, moment := range []views.Moment{views.MomentOrient, views.MomentDecompose} {
			readout := views.ReadoutOfInput(tree, moment, views.ReadoutInput{})
			checks := checkLabels(readout)
			if blocking {
				require.Contains(t, checks["Dependency order"], diagnostics.Gaps[0].Message)
				require.Contains(t, checks["Dependency order"], "Blocking:")
				require.NotContains(t, checks["Dependency order"], "Advisory")
			} else {
				require.Contains(t, checks["Dependency order"], diagnostics.Advisories[0].Message)
				require.Contains(t, checks["Dependency order"], "nonblocking")
				require.NotContains(t, checks["Dependency order"], "Blocking:")
			}
		}
	}
}
