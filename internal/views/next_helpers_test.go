package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// topMove is the first move the shortlist ranks for reader, failing the test
// when nothing is legal.
func topMove(t *testing.T, tree *store.Tree, reader string, in workflow.Input) views.NextCandidate {
	t.Helper()
	short := nextFor(tree, reader, in).Shortlist()
	require.NotEmpty(t, short, "no legal move")
	return short[0]
}

// candidateKinds lists each candidate's kind, in order.
func candidateKinds(cands []views.NextCandidate) []string {
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.Kind)
	}
	return out
}

// frontierIDs lists the frontier's draft features, best first.
func frontierIDs(tree *store.Tree) []string {
	out := []string{}
	for _, f := range views.FrontierFeatures(tree, gen.DependencyOrder(tree)) {
		out = append(out, f.ID)
	}
	return out
}
