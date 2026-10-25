package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDriftDesignAddressesLiteralDocAndPreservesLedger(t *testing.T) {
	root := layDesignCLI(t)
	manifest, err := store.LoadManifest(root)
	require.NoError(t, err)
	manifest.Drift.Checks = []string{"stale_design", "shifted_design"}
	data, err := yaml.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/tiller.yml"), data, 0o644))
	const doc = "as-is/it's a model.md"
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/design", doc), []byte("# Current model\n"), 0o644))
	designCLIData(t, "link", "design", doc, "--describes", "app")
	designCLIData(t, "approve", "design", doc)
	before := snapshotTree(t, filepath.Join(root, ".tiller"))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app/other.go"), []byte("package changed\n"), 0o644))
	out, stderr, code := execCLI(t, "drift", "--json")
	require.Equal(t, 1, code, out+stderr)
	result := jsonData(t, out)
	require.Equal(t, float64(0), result["compared"])
	require.Equal(t, float64(1), result["compared_design"])
	candidates := result["candidates"].(map[string]any)["shifted_design"].([]any)
	require.Len(t, candidates, 1)
	candidate := candidates[0].(map[string]any)
	require.Equal(t, "design", candidate["noun"])
	require.Equal(t, "", candidate["id"])
	require.Equal(t, doc, candidate["key"])
	require.Equal(t, []any{"app"}, candidate["files"])
	require.Equal(t, candidate["since"].(map[string]any)["value"], candidate["until"].(map[string]any)["value"])
	require.Equal(t, before, snapshotTree(t, filepath.Join(root, ".tiller")))
}

// Issue #112: an unstaged case-only rename on a case-insensitive filesystem
// left Git's inventory spelling the file one way and the directory listing
// another, so approval fingerprinted an empty tree and a later edit reported
// clean under shifted_design.
func TestDriftDesignSeesTrackedFileAfterUnstagedCaseRename(t *testing.T) {
	root := layDesignCLI(t)
	if !caseInsensitiveCLIDir(t, root) {
		t.Skip("needs a case-insensitive filesystem")
	}
	manifest, err := store.LoadManifest(root)
	require.NoError(t, err)
	manifest.Drift.Checks = []string{"stale_design", "shifted_design"}
	data, err := yaml.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/tiller.yml"), data, 0o644))
	require.NoError(t, os.Rename(filepath.Join(root, "app/other.go"), filepath.Join(root, "app/OTHER.go")))
	const doc = "as-is/one.md"
	designCLIData(t, "link", "design", doc, "--describes", "app")
	approved := designCLIData(t, "approve", "design", doc)
	surfaces := approved["docs"].([]any)[0].(map[string]any)["confirmed"].(map[string]any)["surfaces"].([]any)
	require.Len(t, surfaces, 1)
	require.Equal(t, []any{"app/OTHER.go", "app/one,two.go"}, surfaces[0].(map[string]any)["inventory"])

	require.NoError(t, os.WriteFile(filepath.Join(root, "app/OTHER.go"), []byte("package changed\n"), 0o644))
	out, stderr, code := execCLI(t, "drift", "--json")
	require.Equal(t, 1, code, out+stderr)
	candidates := jsonData(t, out)["candidates"].(map[string]any)["shifted_design"].([]any)
	require.Len(t, candidates, 1)
	require.Equal(t, doc, candidates[0].(map[string]any)["key"])
}

func caseInsensitiveCLIDir(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	require.NoError(t, os.WriteFile(probe, []byte("probe\n"), 0o644))
	defer os.Remove(probe)
	lower, err := os.Stat(filepath.Join(dir, "caseprobe"))
	if err != nil {
		require.ErrorIs(t, err, os.ErrNotExist)
		return false
	}
	upper, err := os.Stat(probe)
	require.NoError(t, err)
	return os.SameFile(lower, upper)
}
