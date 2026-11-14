package cli

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// TestHumanSyncReopenedSaysWhenANodeCrossedAgain covers the one run where a
// node moves in both directions: an epic whose late feature finished before
// any sync saw it reopens and crosses again at once. The reopened line must
// not leave a reader thinking the epic is still at ready.
func TestHumanSyncReopenedSaysWhenANodeCrossedAgain(t *testing.T) {
	res := mutate.SyncResult{
		Reopened: []mutate.Reopening{
			{Noun: "feature", ID: "F-aaaaaa", Status: "active"},
			{Noun: "epic", ID: "E-aaaaaa", Status: "ready"},
		},
		Crossings: []mutate.Crossing{
			{Noun: "task", ID: "T-aaaaaa", Status: "done"},
			{Noun: "epic", ID: "E-aaaaaa", Status: "done"},
		},
	}
	out := humanSyncReopened(&store.Tree{}, res)
	require.Contains(t, out, "  - feature F-aaaaaa -> active\n")
	require.Contains(t, out, "  - epic E-aaaaaa -> ready, then done again in this run\n")
}
