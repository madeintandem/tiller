package drift_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// TestActiveBasisAppliesIsTheSweepsStatusRule pins the one status rule every
// reporter of basis drift shares: ready and active features, and approved
// tasks that are not finished. Done, cancelled, and unapproved nodes, and
// nouns with no accepted basis, are never compared.
func TestActiveBasisAppliesIsTheSweepsStatusRule(t *testing.T) {
	for status, want := range map[string]bool{"draft": false, "specced": false, "ready": true, "active": true, "done": false, "cancelled": false} {
		require.Equalf(t, want, drift.ActiveBasisApplies(&store.Feature{Status: status}), "feature %s", status)
	}
	for status, want := range map[string]bool{"proposed": false, "ready": true, "in_progress": true, "check": true, "in_review": true, "done": false, "cancelled": false} {
		require.Equalf(t, want, drift.ActiveBasisApplies(&store.Task{Status: status}), "task %s", status)
	}
	require.False(t, drift.ActiveBasisApplies(&store.Epic{Status: "active"}))
	require.False(t, drift.ActiveBasisApplies(&store.Question{}))
}
