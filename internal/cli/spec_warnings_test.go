package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// layCoveredPreserveFeature lays a speccable feature whose project requires a
// tests gate on features and whose contract carries one required preserve
// criterion. binding is the criterion's binding line, so a caller picks the
// ref-only test binding the tests gate's whole-suite run already exercises, or
// a targeted run for the quiet case.
func layCoveredPreserveFeature(t *testing.T, binding string) {
	t.Helper()
	laySpeccableFeature(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	manifest := filepath.Join(cwd, ".tiller", "tiller.yml")
	raw, err := os.ReadFile(manifest)
	require.NoError(t, err)
	gates := "gates:\n  required:\n    tests: { kind: command, run: \"go test ./...\", on: [feature, task] }\n"
	require.NoError(t, os.WriteFile(manifest, append(raw, []byte(gates)...), 0o644))
	contract := "---\nnode: F-aaaaaa\ncriteria:\n" +
		"  - id: C1\n    statement: Existing behavior survives the change\n    polarity: preserve\n" +
		"    binding: " + binding + "\n" +
		"    baseline:\n      surface: app\n" +
		"      captured_at: { kind: git_tree, value: \"git:aaaaaaa\", path: app, observed_at: \"2026-08-06T00:00:00Z\" }\n" +
		"    required: true\n---\n\n# Acceptance\n"
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.eval.md"), []byte(contract), 0o644))
}

const coveredPreserveCLIWarning = "criterion C1 (preserve) is a test the tests gate already runs on every check: drop it, or say in Notes from this run why it stays"

// TestSpecFeatureHumanWarning pins that the preserve advisory reaches the human
// line as a Warning: line, the way check renders its own, without changing the
// exit code, and that the no-change re-spec prints it too.
func TestSpecFeatureHumanWarning(t *testing.T) {
	layCoveredPreserveFeature(t, "{ type: test, ref: TestExisting }")

	out, stderr, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code, "the warning never blocks: out=%s stderr=%s", out, stderr)
	require.Contains(t, out, "is now specced.")
	require.Contains(t, out, "\nWarning: "+coveredPreserveCLIWarning)

	out, stderr, code = execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code, "out=%s stderr=%s", out, stderr)
	require.Contains(t, out, "(no change).")
	require.Contains(t, out, "\nWarning: "+coveredPreserveCLIWarning)
}

// TestSpecFeatureJSONWarnings pins the same advisory on the --json payload, and
// that a contract with no covered preserve criterion omits the key.
func TestSpecFeatureJSONWarnings(t *testing.T) {
	t.Run("covered preserve criterion", func(t *testing.T) {
		layCoveredPreserveFeature(t, "{ type: test, ref: TestExisting }")
		out, stderr, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc", "--json")
		require.Equal(t, 0, code, "out=%s stderr=%s", out, stderr)
		require.Contains(t, out, `"warnings": [`)
		require.Contains(t, out, coveredPreserveCLIWarning)
	})
	t.Run("targeted preserve criterion", func(t *testing.T) {
		layCoveredPreserveFeature(t, `{ type: test, run: "go test ./internal/forms -run TestExisting" }`)
		out, stderr, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc", "--json")
		require.Equal(t, 0, code, "out=%s stderr=%s", out, stderr)
		require.NotContains(t, out, `"warnings"`)
	})
}
