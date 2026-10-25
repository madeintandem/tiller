package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// writePreserveEval writes a feature contract with one required preserve
// criterion carrying the given binding line, plus the baseline a required
// preserve criterion needs to pass contract validation.
func writePreserveEval(t *testing.T, root, binding string) {
	t.Helper()
	eval := `---
node: F-aaaaaa
criteria:
  - id: C1
    statement: existing behavior survives the change
    polarity: preserve
    binding: ` + binding + `
    baseline:
      surface: "app"
      captured_at: { kind: git_tree, value: "git:aaaaaaa", path: app, observed_at: "2026-08-06T00:00:00Z" }
    required: true
---

# Acceptance
`
	require.NoError(t, os.WriteFile(filepath.Join(root, featureDir, "feature.eval.md"), []byte(eval), 0o644))
}

// specWithTestsGate runs `spec` on the fixture feature under a manifest whose
// required tests gate runs on features and tasks.
func specWithTestsGate(t *testing.T, root string) mutate.TransitionResult {
	t.Helper()
	tree := load(t, root)
	tree.Manifest.Gates.Required = map[string]store.GateSpec{
		"tests": {Kind: store.GateKindCommand, Run: "go test ./...", On: store.StringList{"feature", "task"}},
	}
	res, err := mutate.Transition(tree, testDeps(), tree.Features["F-aaaaaa"], mutate.TransitionInput{Verb: "spec"})
	require.NoError(t, err)
	return res
}

const coveredPreserveWarning = "criterion C1 (preserve) is a test the tests gate already runs on every check: drop it, or say in Notes from this run why it stays"

func TestSpecFeatureWarnsOnCoveredPreserve(t *testing.T) {
	t.Parallel()
	root := laySpeccableFeature(t, "create")
	writePreserveEval(t, root, `{ type: test, ref: TestExisting }`)

	first := specWithTestsGate(t, root)
	require.True(t, first.Changed)
	require.Equal(t, "specced", first.Status)
	require.Equal(t, []string{coveredPreserveWarning}, first.Warnings)

	again := specWithTestsGate(t, root)
	require.False(t, again.Changed, "a second spec changes nothing")
	require.Equal(t, []string{coveredPreserveWarning}, again.Warnings,
		"the no-change re-spec still tells the author what the contract repeats")
}

func TestSpecFeatureQuietOnTargetedPreserve(t *testing.T) {
	t.Parallel()
	root := laySpeccableFeature(t, "create")
	writePreserveEval(t, root, `{ type: test, run: "go test ./internal/forms -run TestExisting" }`)

	res := specWithTestsGate(t, root)
	require.Equal(t, "specced", res.Status)
	require.Empty(t, res.Warnings)
}

func TestApproveFeatureCarriesNoSpecWarnings(t *testing.T) {
	t.Parallel()
	root := laySpeccableFeature(t, "create")
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	writePreserveEval(t, root, `{ type: test, ref: TestExisting }`)
	specWithTestsGate(t, root)

	tree := load(t, root)
	tree.Manifest.Gates.Required = map[string]store.GateSpec{
		"tests": {Kind: store.GateKindCommand, Run: "go test ./...", On: store.StringList{"feature", "task"}},
	}
	res, err := mutate.Transition(tree, testDeps(), tree.Features["F-aaaaaa"], mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)
	require.Empty(t, res.Warnings, "the warning is shown on spec, never on approve")
}
