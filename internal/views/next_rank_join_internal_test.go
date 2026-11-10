package views

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A human candidate's own reason may be a workflow sentence. The locality
// reason placed before it closes into a sentence of its own rather than
// gluing the sentence on with "; "; a plain clause still joins as a clause.
func TestWithBaseClosesTheLocalityReasonBeforeASentenceReason(t *testing.T) {
	sentence := NextCandidate{Actor: actorHuman, Reason: "Formal submission is recorded. Human review and merge are awaited."}
	require.Equal(t, "In your current epic. Formal submission is recorded. Human review and merge are awaited.", withBase("in your current epic", sentence))

	clause := NextCandidate{Actor: actorHuman, Reason: "its feature plan awaits your approval"}
	require.Equal(t, "in your current epic; its feature plan awaits your approval", withBase("in your current epic", clause))
}
