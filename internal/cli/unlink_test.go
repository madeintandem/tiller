package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestUnlinkHumanOutputNamesRemovedReferencesAndRemainingCount(t *testing.T) {
	layUnlinkCLI(t)

	out, stderr, code := execCLI(t, "unlink", "feature", "form-engine/rendering-modes",
		"--depends-on", "form-engine/schema-engine", "--depends-on", "payments", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "Removed form-engine/schema-engine (F-")
	require.Contains(t, out, "from feature form-engine/rendering-modes (F-")
	require.Contains(t, out, "); 1 dependency remains.")

	out, stderr, code = execCLI(t, "unlink", "feature", "form-engine/rendering-modes",
		"--depends-on", "form-engine/schema-engine", "--depends-on", "payments", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "No requested dependencies were present")
	require.Contains(t, out, "1 dependency remains (no change).")
}

func TestUnlinkJSONKeepsDependencyIDsStructural(t *testing.T) {
	layUnlinkCLI(t)

	out, stderr, code := execCLI(t, "unlink", "feature", "form-engine/rendering-modes",
		"--depends-on", "form-engine/schema-engine", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, "feature", data["noun"])
	require.Equal(t, "form-engine/rendering-modes", data["key"])
	require.Equal(t, true, data["changed"])
	require.Regexp(t, `^F-[a-z0-9]{6}$`, data["id"])
	removed, ok := data["removed"].([]any)
	require.True(t, ok)
	require.Len(t, removed, 1)
	require.Regexp(t, `^F-[a-z0-9]{6}$`, removed[0])
	remaining, ok := data["depends_on"].([]any)
	require.True(t, ok)
	require.Len(t, remaining, 1)
	require.Regexp(t, `^E-[a-z0-9]{6}$`, remaining[0])
}

func TestUnlinkRequiresAValidTarget(t *testing.T) {
	layUnlinkCLI(t)

	out, _, code := execCLI(t, "unlink", "feature", "form-engine/rendering-modes", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "unlink_target"`)

	out, _, code = execCLI(t, "unlink", "feature", "form-engine/rendering-modes",
		"--depends-on", "F-zzzzzz", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "not_found"`)
}

func TestUnlinkAbsentEdgeRemainsANoOpAfterWorkStarts(t *testing.T) {
	layUnlinkCLI(t)
	setCLIFeatureDependencyState(t, "active", "tiller/form-engine/rendering-modes")

	out, stderr, code := execCLI(t, "unlink", "feature", "form-engine/rendering-modes",
		"--depends-on", "payments", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, false, jsonData(t, out)["changed"])
}

func layUnlinkCLI(t *testing.T) {
	t.Helper()
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/rendering-modes", "--title", "Rendering modes", "--contributor", "jc")
	mustRun(t, "link", "feature", "form-engine/rendering-modes",
		"--depends-on", "form-engine/schema-engine", "--depends-on", "form-engine", "--contributor", "jc")
}

func setCLIFeatureDependencyState(t *testing.T, status, branch string) {
	t.Helper()
	root, err := repoRoot()
	require.NoError(t, err)
	path := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "rendering-modes", "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Status = status
	doc.Branch = branch
	data, err = yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}
