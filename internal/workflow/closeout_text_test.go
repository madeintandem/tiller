package workflow

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func humanItem(id string) Work {
	return Work{Kind: "criterion", ID: id, Status: "needs_attestation", Actor: "human"}
}

func agentItem(kind, id string) Work {
	return Work{Kind: kind, ID: id, Status: "missing", Actor: "agent"}
}

func TestOpenAcceptanceSentence(t *testing.T) {
	rows := []struct {
		name  string
		items []Work
		want  string
	}{
		{"one human", []Work{humanItem("C2")}, "Acceptance is still open: C2 needs your attestation."},
		{"two human", []Work{humanItem("C2"), humanItem("C5")}, "Acceptance is still open: C2 and C5 need your attestation."},
		{"three human", []Work{humanItem("C2"), humanItem("C5"), humanItem("C7")}, "Acceptance is still open: C2, C5, and C7 need your attestation."},
		{"one agent", []Work{agentItem("rubric", "C3")}, "Acceptance is still open: C3 needs the agent's rubric."},
		{"two agent", []Work{agentItem("rubric", "C3"), agentItem("gate", "lint_gate")}, "Acceptance is still open: C3 needs the agent's rubric and lint gate needs the agent's gate."},
		{"three agent", []Work{agentItem("rubric", "C3"), agentItem("criterion", "C4"), agentItem("configuration", "unit_tests")}, "Acceptance is still open: C3 needs the agent's rubric, C4 needs the agent's criterion, and unit tests needs the agent's configuration."},
		{"mix lists human first", []Work{agentItem("rubric", "C3"), humanItem("C2"), humanItem("C5")}, "Acceptance is still open: C2 and C5 need your attestation, and C3 needs the agent's rubric."},
		{"node prefix", []Work{{Kind: "criterion", ID: "C2", Status: "needs_attestation", Actor: "human", NodeID: "F-aaaaaa"}}, "Acceptance is still open: F-aaaaaa C2 needs your attestation."},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			got := openAcceptanceSentence(r.items)
			require.Equal(t, r.want, got)
			require.NotContains(t, got, "_")
		})
	}
}

func TestViolationSentences(t *testing.T) {
	require.Equal(t, "", policyCountSentence("violation", "historical policy", 0))
	require.Equal(t, "1 historical policy violation remains on record.", policyCountSentence("violation", "historical policy", 1))
	require.Equal(t, "2 historical policy violations remain on record.", policyCountSentence("violation", "historical policy", 2))
	require.Equal(t, "1 source-pinned historical policy warning remains on record.", policyCountSentence("warning", "source-pinned historical policy", 1))
	require.Equal(t, "2 independent crossing-time policy warnings remain on record.", policyCountSentence("warning", "independent crossing-time policy", 2))
}

func TestDoneReasonReadsAsSentence(t *testing.T) {
	m, f, in := humanAttestFixture()
	m.Gates.Required = map[string]store.GateSpec{"evals": {Kind: "command", On: store.StringList{"feature"}}}
	f.GatesBlock = store.GatesBlock{Gates: map[string]store.Gate{"evals": {Required: true, Status: store.GateStatusPass}}}
	in.Contracts[f.ID].Criteria = []store.EvalCriterion{
		{ID: "C2", Statement: "a", Required: true, Binding: store.EvalBinding{Type: store.BindingManual}},
		{ID: "C5", Statement: "b", Required: true, Binding: store.EvalBinding{Type: store.BindingManual}},
	}
	f.Status = "done"
	f.PolicyViolations = []store.PolicyViolation{{Gate: "evals", Status: "pending"}}
	a := Classify(m, f, f, store.ReviewGrainFeature, true, in)
	require.Equal(t, "done", a.Phase)
	require.Equal(t, "Done is recorded. Acceptance is still open: C2 and C5 need your attestation. 1 historical policy violation remains on record.", a.Reason)
	require.Len(t, a.Outstanding, 2)
	require.Equal(t, "missing", a.Outstanding[0].Status)

	f.PolicyViolations = append(f.PolicyViolations, store.PolicyViolation{Gate: "evals", Status: "pending"})
	a = Classify(m, f, f, store.ReviewGrainFeature, true, in)
	require.Equal(t, "Done is recorded. Acceptance is still open: C2 and C5 need your attestation. 2 historical policy violations remain on record.", a.Reason)
}
