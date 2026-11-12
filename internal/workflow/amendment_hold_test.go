package workflow

import (
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// TestCloseoutReadinessIgnoresHeldDrafts: a held draft is not there for the
// coarse review or the task-grain closeout either. A held task does not keep
// its feature from submitting or closing out, and a held feature is not one
// of its epic's live features.
func TestCloseoutReadinessIgnoresHeldDrafts(t *testing.T) {
	epicKey, featKey, otherKey := "pay", "pay/checkout", "pay/gamma"
	epic := &store.Epic{ID: "E-aaaaaa", Key: &epicKey, Status: "ready"}
	f := &store.Feature{ID: "F-aaaaaa", Key: &featKey, Status: "active", EpicKey: epicKey}
	other := &store.Feature{ID: "F-bbbbbb", Key: &otherKey, Status: "draft", EpicKey: epicKey}
	done := &store.Task{ID: "T-aaaaaa", Status: "done", FeatureID: f.ID}
	held := &store.Task{ID: "T-bbbbbb", Status: "proposed", FeatureID: f.ID}
	tree := &store.Tree{
		Manifest:     &store.Manifest{},
		Epics:        map[string]*store.Epic{epic.ID: epic},
		Features:     map[string]*store.Feature{f.ID: f, other.ID: other},
		Tasks:        map[string]*store.Task{done.ID: done, held.ID: held},
		FeatureTasks: map[string][]*store.Task{f.ID: {done, held}},
		Amendments:   map[string]*store.Amendment{"A-aaaaaa": {ID: "A-aaaaaa", Status: store.AmendmentStatusDraft}},
	}
	require.False(t, FeatureTasksReadyToSubmit(tree, f), "unheld, the proposed task holds the submit")
	require.False(t, featureTasksDone(tree, f))
	require.Len(t, EpicLiveFeatures(tree, gen.DependencyOrder(tree), epic), 2)

	tree.AmendmentHolds = map[string]string{held.ID: "A-aaaaaa", other.ID: "A-aaaaaa"}
	require.True(t, FeatureTasksReadyToSubmit(tree, f))
	require.True(t, featureTasksDone(tree, f))
	live := EpicLiveFeatures(tree, gen.DependencyOrder(tree), epic)
	require.Len(t, live, 1)
	require.Equal(t, f.ID, live[0].ID)
}
