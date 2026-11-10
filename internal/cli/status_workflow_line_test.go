package cli

import (
	"testing"

	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// The workflow line's reason is a sentence ending in a period, so its
// command follows after a space, never glued on with "; ".
func TestStatusWorkflowLineJoinsItsCommandAfterTheReasonSentence(t *testing.T) {
	v := NodeStatusView{Noun: "feature", Ref: "checkout/alpha (F-aaaaa0)", Status: "active",
		Workflow: &workflow.Advice{Phase: "review", Actor: "human", Reason: "Formal submission is recorded. Merge it, then read back state.", Command: "tiller sync"}}
	out := renderStatusNodeHuman(v, "", nil)
	require.Contains(t, out, "\n  workflow: review (human): Formal submission is recorded. Merge it, then read back state. `tiller sync`")
	require.NotContains(t, out, ".; ")
}
