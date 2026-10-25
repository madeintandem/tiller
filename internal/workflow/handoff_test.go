package workflow

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// humanAttestFixture is a feature whose only outstanding work is one required
// attestation gate, so Classify lands on the human acceptance handoff.
func humanAttestFixture() (*store.Manifest, *store.Feature, Input) {
	m := &store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
		"signoff": {Kind: store.GateKindAttestation, On: store.StringList{store.NounFeature}},
	}}}
	f := &store.Feature{ID: "F-a", Branch: "tiller/reviews/features/a"}
	in := Input{Contracts: map[string]*store.EvalFile{f.ID: {Criteria: []store.EvalCriterion{
		{ID: "c1", Statement: "works", Binding: store.EvalBinding{Type: store.BindingManual}},
	}}}}
	return m, f, in
}

func TestClassifyCoarseHandoffNamesAggregateBranch(t *testing.T) {
	m, f, in := humanAttestFixture()
	a := Classify(m, f, f, store.ReviewGrainFeature, true, in)
	require.Equal(t, "attest", a.Action)
	require.Contains(t, a.Reason, "on aggregate branch tiller/reviews/features/a")
	require.Contains(t, a.Reason, "commit there, then submit")
}

func TestClassifyEpicHandoffNamesTheEpicBranch(t *testing.T) {
	m, f, in := humanAttestFixture()
	epic := &store.Epic{ID: "E-a", Branch: "tiller/reviews/epics/a"}
	a := Classify(m, f, epic, store.ReviewGrainEpic, true, in)
	require.Equal(t, "attest", a.Action)
	require.Contains(t, a.Reason, "on aggregate branch tiller/reviews/epics/a")
}

func TestClassifyTaskGrainHandoffOmitsBranch(t *testing.T) {
	m, f, in := humanAttestFixture()
	a := Classify(m, f, f, store.ReviewGrainTask, true, in)
	require.Equal(t, "attest", a.Action)
	require.NotContains(t, a.Reason, "aggregate branch")
}
