package views

import (
	"testing"

	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestNodeSharedDependenciesResolveLegacyKeysInBothDirections(t *testing.T) {
	tree := pageTree()
	source := tree.Features["F-111111"]
	target := tree.Features["F-222222"]
	source.DependsOn = []string{*target.Key}
	index := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, index, nil, "")
	index.Finalize()
	forward := NodePageOf(tree, nil, source, index)
	reverse := NodePageOf(tree, nil, target, index)
	require.Contains(t, relatedIDs(forward.Relationships.DependsOn), target.ID)
	require.Contains(t, relatedIDs(reverse.Relationships.DependedOnBy), source.ID)
	require.Equal(t, []string{*target.Key}, source.DependsOn, "read normalization does not rewrite state")
}

func TestRecordedQuestionEdgesDoNotOverrideEffectiveAssumptionHold(t *testing.T) {
	tree := pageTree()
	feature := tree.Features["F-111111"]
	tree.Questions = map[string]*store.Question{"Q-hold00": {ID: "Q-hold00", Title: "Assumed", Status: "open", Blocks: []string{feature.ID}}}
	feature.AssumptionBasis = store.AssumptionBasis{"Q-hold00": {Reason: "Proceed with human-approved assumption"}}
	index := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, index, nil, "")
	index.Finalize()
	require.Len(t, index.Adjacent(feature.ID, "blocked_by"), 1, "recorded edge survives assumption")
	page := NodePageOf(tree, nil, feature, index)
	require.Empty(t, page.Relationships.BlockedByQuestions, "effective holds remain the lifecycle projection's responsibility")
}
