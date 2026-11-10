package views_test

// These tests pin the fields a Home decision card renders beyond the queue's
// one line of prose: the node's title, the criterion an attestation is about,
// and the size of the plan an approval signs off. They live apart from
// queue_test.go, which already carries the fixture they reuse.

import (
	"fmt"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// TestQueueAttestCriterionCarriesStatement: the node's title, the criterion id
// and the criterion's statement from the feature's eval file. The statement
// rides in through the workflow input's contracts, the same observation the
// readout uses.
func TestQueueAttestCriterionCarriesStatement(t *testing.T) {
	tree := queueFixture(t)
	featureID := featureIDByKeyQ(t, tree, "form-engine/audit-export")
	in := views.QueueInput{Workflow: workflow.Input{Contracts: map[string]*store.EvalFile{
		featureID: {Node: featureID, Criteria: []store.EvalCriterion{
			{ID: "C1", Statement: "Exports include every audit row"},
			{ID: "C2", Statement: "Export finishes under two seconds"},
		}},
	}}}
	var got []views.QueueEntry
	for _, e := range entriesOfKind(views.QueueOfInput(tree, in), "attest_criterion") {
		if e.ID == featureID {
			got = append(got, e)
		}
	}
	require.Len(t, got, 1, "C2 is already passed in the fixture; C1 is the one waiting")
	require.Equal(t, "Audit export", got[0].Title)
	require.Equal(t, "C1", got[0].Criterion)
	require.Equal(t, "Exports include every audit row", got[0].Statement)
	require.Equal(t, "C1", got[0].Prose, "prose is unchanged so existing consumers keep working")

	// Without a contract the statement is absent, never invented.
	for _, e := range entriesOfKind(views.QueueOf(tree), "attest_criterion") {
		require.Empty(t, e.Statement)
		require.NotEmpty(t, e.Criterion)
	}
}

// TestQueueApproveFeatureCarriesPlanCounts: the plan the human is asked to
// approve, as task, criterion and dependency counts.
func TestQueueApproveFeatureCarriesPlanCounts(t *testing.T) {
	epicKey, bigKey, bareKey := "widgets", "widgets/big", "widgets/bare"
	big := &store.Feature{ID: "F-big", Key: &bigKey, Title: "Big", Status: "specced", EpicKey: epicKey}
	for i := 0; i < 10; i++ {
		big.DependsOn = append(big.DependsOn, fmt.Sprintf("F-dep%d", i))
	}
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Epics:    map[string]*store.Epic{"E-widgets": {ID: "E-widgets", Key: &epicKey, Title: "Widgets", Status: "ready"}},
		Features: map[string]*store.Feature{
			"F-big":  big,
			"F-bare": {ID: "F-bare", Key: &bareKey, Title: "Bare", Status: "specced", EpicKey: epicKey},
		},
		FeatureTasks: map[string][]*store.Task{},
	}
	for i := 0; i < 7; i++ {
		tree.FeatureTasks["F-big"] = append(tree.FeatureTasks["F-big"], &store.Task{ID: fmt.Sprintf("T-%d", i), FeatureID: "F-big"})
	}
	// Cancelled work is not part of the plan being approved.
	tree.FeatureTasks["F-big"] = append(tree.FeatureTasks["F-big"], &store.Task{ID: "T-gone", FeatureID: "F-big", Status: "cancelled"})
	contract := &store.EvalFile{Node: "F-big"}
	for i := 0; i < 15; i++ {
		contract.Criteria = append(contract.Criteria, store.EvalCriterion{ID: fmt.Sprintf("C%d", i)})
	}
	in := views.QueueInput{Workflow: workflow.Input{Contracts: map[string]*store.EvalFile{"F-big": contract}}}

	byID := map[string]views.QueueEntry{}
	for _, e := range entriesOfKind(views.QueueOfInput(tree, in), "approve_feature") {
		byID[e.ID] = e
	}
	require.NotNil(t, byID["F-big"].Plan)
	require.Equal(t, views.QueuePlan{Tasks: 7, Criteria: 15, Dependencies: 10}, *byID["F-big"].Plan)
	require.Equal(t, "Big", byID["F-big"].Title)
	require.NotNil(t, byID["F-bare"].Plan)
	require.Equal(t, views.QueuePlan{}, *byID["F-bare"].Plan, "no contract and no tasks: zeros, not absent")
}
