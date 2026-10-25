package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func TestReadoutDoesNotSendHistoricalCyclesToReadiness(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	a := feat(t, "F-aaaaa0", "epic-0/old", "epic-0")
	b := feat(t, "F-bbbbb0", "epic-1/old", "epic-1")
	a.DependsOn, b.DependsOn = []string{b.ID}, []string{a.ID}
	a.Status, b.Status = "cancelled", "cancelled"
	tree.Features[a.ID], tree.Features[b.ID] = a, b
	tree.Features["F-newnew"] = feat(t, "F-newnew", "epic-0/current", "epic-0")
	require.NotEmpty(t, gen.DependencyOrder(tree).Cycles())

	for _, moment := range []views.Moment{views.MomentOrient, views.MomentDecompose} {
		readout := views.ReadoutOfInput(tree, moment, views.ReadoutInput{Ready: true})
		for _, check := range readout.Checks {
			require.NotEqual(t, "Dependency order", check.Label, "cancelled-only cycles are not current readiness work")
		}
	}
}
