package mutate_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// TestAttestRefusesTheEvalsGate confirms attest --gate evals is refused as misuse
// rather than reaching setGateAttestationStatus, which would hand-write the evals
// gate's status: a value gates.GateStatusFromCriteria derives from the criteria
// map on every check and eval. A direct write here would be clobbered by the very
// next check, and in the meantime would look like a bulk re-attest escape hatch
// around the per-criterion ceremony this task's stale value exists to enforce.
func TestAttestRefusesTheEvalsGate(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "F-aaaaaa")

	_, err := mutate.Attest(tree, testDeps(), feat, mutate.AttestInput{Gate: "evals", Status: store.GateStatusPass})

	require.Error(t, err)
	var ue interface{ Reason() string }
	require.ErrorAs(t, err, &ue)
	require.Equal(t, "derived_gate", ue.Reason())
	require.Contains(t, err.Error(), "--criterion",
		"the refusal must name the move that actually works")
}
