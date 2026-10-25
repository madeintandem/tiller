package cli

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func TestSyncJSONRetainsHistoricalPolicySeparatelyFromCurrentCrossing(t *testing.T) {
	result := mutate.SyncResult{Crossings: []mutate.Crossing{{
		Noun: store.NounFeature, ID: "F-review", Status: "done", Basis: "rollup", Observed: "git:current-integration",
		PolicyViolations:           []string{"build"},
		HistoricalPolicyViolations: []store.PolicyViolation{{Gate: "evals", Status: "pending", Observed: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:current-integration"}, SourceRevision: store.RevisionRef{Kind: store.KindGitBlob, Value: "git:historical-source"}, UnresolvedCriteria: []string{"C8"}, EvidenceRefs: []string{"historical-grade.json"}}, {Gate: "tests", Status: "unknown", SourceRevision: store.RevisionRef{Kind: store.KindGitBlob, Value: "git:historical-source"}}},
	}}}
	encoded, err := json.Marshal(syncView(result))
	require.NoError(t, err)
	var decoded struct {
		Crossings []struct {
			PolicyViolations []string                    `json:"policy_violations"`
			Historical       []views.PolicyViolationView `json:"historical_policy_violations"`
		} `json:"crossings"`
	}
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Len(t, decoded.Crossings, 1)
	crossing := decoded.Crossings[0]
	require.Equal(t, []string{"build"}, crossing.PolicyViolations)
	require.Len(t, crossing.Historical, 2)
	require.Equal(t, "pending", crossing.Historical[0].Status)
	require.Equal(t, "git:historical-source", crossing.Historical[0].SourceRevision)
	require.Equal(t, "git:current-integration", crossing.Historical[0].Observed)
	require.Equal(t, []string{"C8"}, crossing.Historical[0].UnresolvedCriteria)
	require.Equal(t, []string{"historical-grade.json"}, crossing.Historical[0].EvidenceRefs)
	require.Equal(t, "unknown", crossing.Historical[1].Status)
}

func TestMissingReviewSyncRemedyDoesNotRequireReadySiblings(t *testing.T) {
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			epicKey, featureKey := "work", "work/review"
			epic := &store.Epic{ID: "E-review", Key: &epicKey, Status: "ready", ReviewGrain: grain}
			feature := &store.Feature{ID: "F-review", Key: &featureKey, EpicKey: epicKey, Status: "active", ReviewGrain: grain}
			submitted := &store.Task{ID: "T-submitted", FeatureID: feature.ID, Status: "in_review"}
			sibling := &store.Task{ID: "T-not-ready", FeatureID: feature.ID, Status: "ready"}
			tree := &store.Tree{Manifest: &store.Manifest{}, Epics: map[string]*store.Epic{epic.ID: epic}, Features: map[string]*store.Feature{feature.ID: feature},
				Tasks: map[string]*store.Task{submitted.ID: submitted, sibling.ID: sibling}, FeatureTasks: map[string][]*store.Task{feature.ID: {submitted, sibling}}}
			require.Nil(t, workflow.AdviceOf(tree, submitted, workflow.Input{}))
			owner := workflow.ReviewOwnerOf(tree, submitted)
			require.NotNil(t, owner)
			if grain == store.ReviewGrainEpic {
				require.Equal(t, epic.ID, owner.NodeID())
			} else {
				require.Equal(t, feature.ID, owner.NodeID())
			}
			text := humanSyncAwaiting(tree, mutate.SyncResult{Into: "main", Awaiting: []mutate.Awaiting{{Noun: store.NounTask, ID: submitted.ID, ReviewMissing: true}}})
			require.Contains(t, text, workflow.MissingReviewGuidance(owner))
			require.Contains(t, text, "--recover-merged")
			require.Contains(t, text, "tiller submit "+owner.Noun()+" "+owner.NodeID())
		})
	}
}
