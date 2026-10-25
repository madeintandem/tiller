package gates

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func preserve(id string, b store.EvalBinding, required bool) store.EvalCriterion {
	return store.EvalCriterion{ID: id, Polarity: store.PolarityPreserve, Binding: b, Required: required}
}

func testsManifest(run string, on store.StringList) *store.Manifest {
	return &store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
		"tests": {Kind: store.GateKindCommand, Run: run, On: on},
	}}}
}

func TestCoveredPreserveCriteria(t *testing.T) {
	t.Parallel()
	const run = "go test ./..."
	feature := store.StringList{"feature", "task"}

	cases := []struct {
		name string
		crit store.EvalCriterion
		m    *store.Manifest
		want []PreserveCoverage
	}{
		{"command equal to tests run", preserve("C5", store.EvalBinding{Type: store.BindingCommand, Run: run}, true),
			testsManifest(run, feature), []PreserveCoverage{{CriterionID: "C5", ExactCommand: true}}},
		{"command equal after whitespace", preserve("C5", store.EvalBinding{Type: store.BindingCommand, Run: "  " + run + "\n"}, true),
			testsManifest(run, feature), []PreserveCoverage{{CriterionID: "C5", ExactCommand: true}}},
		{"test binding whose run equals tests run", preserve("C5", store.EvalBinding{Type: store.BindingTest, Run: run}, true),
			testsManifest(run, feature), []PreserveCoverage{{CriterionID: "C5", ExactCommand: true}}},
		{"test binding ref only", preserve("C5", store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"TestX"}}, true),
			testsManifest(run, feature), []PreserveCoverage{{CriterionID: "C5"}}},
		{"test binding two refs no run", preserve("C5", store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"TestX", "TestY"}}, true),
			testsManifest(run, feature), []PreserveCoverage{{CriterionID: "C5"}}},
		{"test binding targeted run", preserve("C5", store.EvalBinding{Type: store.BindingTest, Run: "go test ./internal/x -run TestX"}, true),
			testsManifest(run, feature), nil},
		{"optional preserve ref only", preserve("C5", store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"TestX"}}, false),
			testsManifest(run, feature), []PreserveCoverage{{CriterionID: "C5"}}},
		{"introduce polarity ref only", store.EvalCriterion{ID: "C5", Polarity: "introduce", Required: true,
			Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"TestX"}}},
			testsManifest(run, feature), nil},
		{"manual preserve", preserve("C5", store.EvalBinding{Type: store.BindingManual}, true),
			testsManifest(run, feature), nil},
		{"tests gate on task only", preserve("C5", store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"TestX"}}, true),
			testsManifest(run, store.StringList{"task"}), nil},
		{"tests gate on feature scalar", preserve("C5", store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"TestX"}}, true),
			testsManifest(run, store.StringList{"feature"}), []PreserveCoverage{{CriterionID: "C5"}}},
		{"tests gate with a blank run", preserve("C5", store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"TestX"}}, true),
			testsManifest("  \n", feature), nil},
		{"tests gate that is an attestation", preserve("C5", store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"TestX"}}, true),
			&store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
				"tests": {Kind: store.GateKindAttestation, On: feature},
			}}}, nil},
		{"no tests gate", preserve("C5", store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"TestX"}}, true),
			&store.Manifest{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ef := &store.EvalFile{Criteria: []store.EvalCriterion{tc.crit}}
			require.Equal(t, tc.want, CoveredPreserveCriteria(ef, tc.m))
		})
	}

	t.Run("nil contract or manifest", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, CoveredPreserveCriteria(nil, testsManifest(run, feature)))
		require.Empty(t, CoveredPreserveCriteria(&store.EvalFile{}, nil))
	})

	t.Run("contract order", func(t *testing.T) {
		t.Parallel()
		ef := &store.EvalFile{Criteria: []store.EvalCriterion{
			preserve("C2", store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"TestX"}}, true),
			preserve("C1", store.EvalBinding{Type: store.BindingCommand, Run: run}, true),
		}}
		require.Equal(t, []PreserveCoverage{{CriterionID: "C2"}, {CriterionID: "C1", ExactCommand: true}},
			CoveredPreserveCriteria(ef, testsManifest(run, feature)))
	})
}

func TestPreserveCoverageWarning(t *testing.T) {
	t.Parallel()
	require.Equal(t,
		"criterion C5 (preserve) runs the tests gate's own command, which already runs on every check: give it a targeted check, drop it, or say in Notes from this run why it stays",
		PreserveCoverage{CriterionID: "C5", ExactCommand: true}.Warning())
	require.Equal(t,
		"criterion C5 (preserve) is a test the tests gate already runs on every check: drop it, or say in Notes from this run why it stays",
		PreserveCoverage{CriterionID: "C5"}.Warning())
}
