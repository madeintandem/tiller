package workflow

import (
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// reasonFixture is a feature with one required command gate keyed lint_gate and
// one required attestation gate keyed sign_off, plus a valid manual contract.
// Keys with an underscore are names a reader looks up, so they must survive
// verbatim; status tokens with an underscore must never reach the sentence.
func reasonFixture() (*store.Manifest, *store.Feature, Input) {
	m := &store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
		"lint_gate": {Kind: store.GateKindCommand, Run: "make lint", On: store.StringList{store.NounFeature}},
		"sign_off":  {Kind: store.GateKindAttestation, On: store.StringList{store.NounFeature}},
	}}}
	f := &store.Feature{ID: "F-a", Branch: "tiller/reviews/features/a"}
	in := Input{Contracts: map[string]*store.EvalFile{f.ID: {Criteria: []store.EvalCriterion{
		{ID: "c1", Statement: "works", Binding: store.EvalBinding{Type: store.BindingManual}},
	}}}}
	return m, f, in
}

func passLint(f *store.Feature) {
	f.GatesBlock = store.GatesBlock{Gates: map[string]store.Gate{
		"lint_gate": {Required: true, Status: store.GateStatusPass},
	}}
}

func requireSentences(t *testing.T, reason string) {
	t.Helper()
	require.NotEmpty(t, reason)
	require.True(t, strings.HasSuffix(reason, "."), "ends in a period: %q", reason)
	require.Equal(t, strings.ToUpper(reason[:1]), reason[:1], "starts capitalized: %q", reason)
	for _, token := range []string{"needs_attestation", "(missing", "(pending", ", agent)", ", human)"} {
		require.NotContains(t, reason, token)
	}
}

func TestReasonAgentOwedRequiredWork(t *testing.T) {
	m, f, in := reasonFixture()
	delete(m.Gates.Required, "sign_off")
	f.PR = intPtr(7)
	a := Classify(m, f, f, store.ReviewGrainFeature, true, in)
	require.Equal(t, "checks", a.Phase)
	require.Equal(t, "Required work remains on feature F-a: lint_gate needs the agent's gate.", a.Reason)
	requireSentences(t, a.Reason)
}

func TestReasonAgentOwedConfiguration(t *testing.T) {
	m, f, in := reasonFixture()
	delete(m.Gates.Required, "sign_off")
	f.PR = intPtr(7)
	m.Gates.Required["lint_gate"] = store.GateSpec{Kind: store.GateKindCommand, On: store.StringList{store.NounFeature}}
	a := Classify(m, f, f, store.ReviewGrainFeature, true, in)
	require.Equal(t, "Required work remains on feature F-a: lint_gate needs the agent's configuration. Configure the required gate command in the manifest before running check.", a.Reason)
	requireSentences(t, a.Reason)
}

func TestReasonHumanOwedTaskGrain(t *testing.T) {
	m, f, in := reasonFixture()
	delete(m.Gates.Required, "lint_gate")
	a := Classify(m, f, f, store.ReviewGrainTask, true, in)
	require.Equal(t, "acceptance", a.Phase)
	require.Equal(t, "Human acceptance remains on feature F-a: sign_off needs your attestation. Complete applicable checks and publish integration-branch state (or verify the local tip offline) before handoff. Do not open a new PR for state-only closeout. Human decision: `tiller attest feature F-a --gate sign_off --status <pass|fail>`.", a.Reason)
	requireSentences(t, a.Reason)
}

func TestReasonHumanOwedCoarseGrain(t *testing.T) {
	m, f, in := reasonFixture()
	delete(m.Gates.Required, "lint_gate")
	f.PR = intPtr(7)
	a := Classify(m, f, f, store.ReviewGrainFeature, true, in)
	require.Equal(t, "Human acceptance remains on feature F-a: sign_off needs your attestation. Execute must prepare and verify the draft or offline review surface before handoff. Host publication is not observed here. Human decision: `tiller attest feature F-a --gate sign_off --status <pass|fail>`. Record each decision on aggregate branch tiller/reviews/features/a (in its worktree if checked out there), commit there, then submit.", a.Reason)
	requireSentences(t, a.Reason)
}

func TestReasonContractStatus(t *testing.T) {
	m, f, in := reasonFixture()
	f.PR = intPtr(7)
	in.Contracts = map[string]*store.EvalFile{}
	in.ContractErrors = map[string]string{f.ID: "bad"}
	a := Classify(m, f, f, store.ReviewGrainFeature, true, in)
	require.Equal(t, "The acceptance contract is unavailable. Read or repair the feature contract before classifying acceptance.", a.Reason)
	requireSentences(t, a.Reason)
}

func TestReasonReviewPhase(t *testing.T) {
	m, f, in := reasonFixture()
	m.Gates.Required = map[string]store.GateSpec{}
	in.Contracts[f.ID].Criteria[0].Required = false
	f.PR = intPtr(7)
	a := Classify(m, f, f, store.ReviewGrainFeature, true, in)
	require.Equal(t, "review", a.Phase)
	require.Equal(t, "Formal submission is recorded. Human review and merge are awaited.", a.Reason)
	requireSentences(t, a.Reason)
}

func TestReasonDoneWithRecovery(t *testing.T) {
	m, f, in := reasonFixture()
	passLint(f)
	delete(m.Gates.Required, "sign_off")
	in.Contracts[f.ID].Criteria[0].Required = false
	f.Status = "done"
	f.PolicyViolations = []store.PolicyViolation{{Gate: "lint_gate", Status: "pending", SourceRevision: store.RevisionRef{Value: "src1"}}}
	f.RecoveredReview = recoveredFixture()
	a := Classify(m, f, f, store.ReviewGrainFeature, true, in)
	require.Equal(t, "done", a.Phase)
	want := "Done is recorded. Recovered historical review of branch tiller/reviews/features/a: head abc123, integration result def456, basis observed. Recorded by jc: merged before tiller. Historical policy for F-a lint_gate: needs attestation (source src1; unresolved criteria C8)."
	require.Equal(t, want, a.Reason)
	requireSentences(t, a.Reason)
}

func TestReasonRecoveredResolveLine(t *testing.T) {
	m, f, in := reasonFixture()
	f.PR = intPtr(12)
	f.RecoveredReview = recoveredFixture()
	a := Classify(m, f, f, store.ReviewGrainFeature, true, in)
	applyRecoveredCloseout(&store.Tree{}, &a)
	require.Contains(t, a.Reason, "Historical riders have crossed. Resolve independent current acceptance on integration before final rollup: sign_off needs your attestation, and lint_gate needs the agent's gate.")
	requireSentences(t, a.Reason)
	require.Contains(t, a.Reason, "lint_gate")
	require.Contains(t, a.Reason, "sign_off")
}

func TestRecoveredNotes(t *testing.T) {
	m, f, in := reasonFixture()
	delete(m.Gates.Required, "lint_gate")
	delete(m.Gates.Required, "sign_off")
	in.Contracts[f.ID].Criteria[0].Required = false
	f.PR = intPtr(12)
	f.RecoveredReview = recoveredFixture()
	a := Classify(m, f, f, store.ReviewGrainFeature, true, in)
	requireSentences(t, a.Reason)
	require.Contains(t, a.Reason, "Run ordinary sync on integration main, then publish and read back state.")
}

func intPtr(n int) *int { return &n }

func recoveredFixture() *store.RecoveredReview {
	return &store.RecoveredReview{
		Branch: "tiller/reviews/features/a", Head: store.RevisionRef{Value: "abc123"},
		MergeCommit: store.RevisionRef{Value: "def456"}, IntegrationBranch: "main",
		Basis: "observed", RecoveredBy: "jc", Reason: "merged before tiller",
		PolicySnapshots: []store.HistoricalPolicySnapshot{{NodeID: "F-a", Source: store.RevisionRef{Value: "src1"},
			Observations: []store.HistoricalPolicyObservation{{Gate: "lint_gate", Status: "needs_attestation", UnresolvedCriteria: []string{"C8"}}}}},
	}
}

func TestSentenceHelper(t *testing.T) {
	require.Equal(t, "Parent epic is not approved.", sentence("parent epic is not approved"))
	require.Equal(t, "Already done.", sentence("Already done."))
	require.Equal(t, "Tip `x`.", sentence("tip `x`"))
	require.Equal(t, "", sentence("  "))
}
