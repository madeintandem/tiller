package cli

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// A re-approve that finishes an interrupted approval reports the operations
// the earlier run landed as already applied, apart from the ones this run
// applied, in the confirmation and in the --json payload.
func TestApproveAmendmentOutputSeparatesAlreadyAppliedOperations(t *testing.T) {
	res := mutate.ApproveResult{ID: "A-000001", Status: store.AmendmentStatusApproved, Applied: []mutate.AppliedOperation{
		{Index: 0, Op: store.AmendmentOpCancel, Noun: store.NounTask, NodeID: "T-000003", Key: "e/f/t", Status: "cancelled",
			Outcome: mutate.AppliedAlready},
		{Index: 1, Op: store.AmendmentOpRevise, Noun: store.NounFeature, NodeID: "F-000002", Key: "e/f", Status: "ready"},
	}}

	out := humanApproveAmendment(&store.Tree{}, "iphone-only (A-000001)", res, "robin")
	require.Contains(t, out, "1 operation(s) applied, 1 already applied by an earlier interrupted approval.")
	require.Contains(t, out, "operation 0 (cancel) e/f/t (T-000003): cancelled (already applied)")
	require.Contains(t, out, "operation 1 (revise) e/f (F-000002): ready")
	require.NotContains(t, out, "ready (already applied)")

	view := amendmentApproveView(res)
	require.Equal(t, "already_applied", view.Applied[0].Outcome)
	require.Empty(t, view.Applied[1].Outcome)
}
