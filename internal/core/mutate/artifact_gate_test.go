package mutate_test

// Tests for the spec gate's baseline-artifact shape rule (the c+a batch, JC
// 2026-08-06, from the Acme false-drift finding): an agent recorded a
// semicolon-joined list of four paths in one baseline artifact string, the
// gate accepted it silently, and orphan_eval later statted the whole string
// as one path and reported healthy files as vanished. The write side refuses
// the shape at `tiller spec feature`; ValidateContract itself stays tolerant
// so already-approved contracts keep rendering everywhere.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/stretchr/testify/require"
)

func writeEvalWithArtifact(t *testing.T, root, artifact string) {
	t.Helper()
	evalPath := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.eval.md")
	eval := `---
node: F-aaaaaa
criteria:
  - id: C1
    statement: existing behavior survives the change
    polarity: preserve
    binding: { type: command, run: "true" }
    baseline:
      surface: "app"
      captured_at: { kind: git_tree, value: "git:aaaaaaa", path: app, observed_at: "2026-08-06T00:00:00Z" }
      artifact: "` + artifact + `"
    required: true
---

# Acceptance
`
	require.NoError(t, os.WriteFile(evalPath, []byte(eval), 0o644))
}

// TestSpecRefusesAMultiPathBaselineArtifact pins the write-side rule: a
// baseline artifact carrying a separator or whitespace is a list, not a
// path, and the gate refuses it as misuse naming the criterion, before the
// shape can land and read back later as false drift.
func TestSpecRefusesAMultiPathBaselineArtifact(t *testing.T) {
	t.Parallel()
	for name, artifact := range map[string]string{
		"semicolon list": "spec/a_spec.rb; spec/b_spec.rb",
		"space list":     "spec/a_spec.rb spec/b_spec.rb",
	} {
		t.Run(name, func(t *testing.T) {
			root := laySpeccableFeature(t, "create")
			writeEvalWithArtifact(t, root, artifact)
			tree := load(t, root)

			_, err := mutate.Transition(tree, testDeps(), tree.Features["F-aaaaaa"],
				mutate.TransitionInput{Verb: "spec"})

			var ue interface{ Reason() string }
			require.ErrorAs(t, err, &ue)
			require.Equal(t, "eval_artifact_invalid", ue.Reason())
			require.Contains(t, err.Error(), "C1", "the refusal names the criterion")
			require.Contains(t, err.Error(), "one repo-relative file")
			require.Equal(t, "draft", load(t, root).Features["F-aaaaaa"].Status,
				"a refused spec must not move the feature")
		})
	}
}

// TestSpecAllowsASinglePathBaselineArtifact is the other half: one honest
// path passes the gate untouched.
func TestSpecAllowsASinglePathBaselineArtifact(t *testing.T) {
	t.Parallel()
	root := laySpeccableFeature(t, "create")
	writeEvalWithArtifact(t, root, "spec/fixtures/baseline_2026-08-06.json")
	tree := load(t, root)

	_, err := mutate.Transition(tree, testDeps(), tree.Features["F-aaaaaa"],
		mutate.TransitionInput{Verb: "spec"})
	require.NoError(t, err)
	require.Equal(t, "specced", load(t, root).Features["F-aaaaaa"].Status)
}
