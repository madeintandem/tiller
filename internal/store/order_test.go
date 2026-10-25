package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// seedMinimalTree lays a minimal valid manifest under a temp repo root, enough
// for Load to succeed with no epics, features, or tasks yet (mirrors
// writeManifestFixture in identity_test.go, this package's nearest Load-test
// seeding helper).
func seedMinimalTree(t *testing.T) string {
	t.Helper()
	return writeManifestFixture(t, rosterManifest)
}

// writeFile writes content to path, creating parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// TestLoadReadsProjectOrder: .tiller/order.yml is the machine-owned root
// soft order; Load carries it on the tree.
func TestLoadReadsProjectOrder(t *testing.T) {
	root := seedMinimalTree(t) // reuse or mirror the package's existing seeding helper
	writeFile(t, filepath.Join(root, ".tiller", "order.yml"), "epics:\n  - E-form01\n  - E-audit1\n")
	tree, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, []string{"E-form01", "E-audit1"}, tree.ProjectOrder.Epics)
}

// TestLoadWithoutOrderFileIsZero: absent order.yml is not an error and reads
// as no recorded order.
func TestLoadWithoutOrderFileIsZero(t *testing.T) {
	root := seedMinimalTree(t)
	tree, err := Load(root)
	require.NoError(t, err)
	require.Empty(t, tree.ProjectOrder.Epics)
}

// TestEpicAndFeatureOrderRoundTrip marshals an Epic and a Feature that each
// carry Order, then unmarshals the result back into a fresh value of the same
// type: it proves the order field survives a direct yaml round trip beside
// depends_on, at both grains.
func TestEpicAndFeatureOrderRoundTrip(t *testing.T) {
	e := Epic{ID: "E-x", Order: []string{"F-b", "F-a"}}
	data, err := yaml.Marshal(e)
	require.NoError(t, err)
	require.Contains(t, string(data), "order:")
	var backEpic Epic
	require.NoError(t, yaml.Unmarshal(data, &backEpic))
	require.Equal(t, e.Order, backEpic.Order)

	f := Feature{ID: "F-x", Order: []string{"T-b", "T-a"}}
	data, err = yaml.Marshal(f)
	require.NoError(t, err)
	require.Contains(t, string(data), "order:")
	var backFeature Feature
	require.NoError(t, yaml.Unmarshal(data, &backFeature))
	require.Equal(t, f.Order, backFeature.Order)
}

// TestLoadProjectOrderMalformedYAMLIsRefused pins that a malformed order.yml
// fails LoadProjectOrder with an error naming the file, the same refusal
// shape every other malformed state file gets, rather than a silently
// swallowed zero value.
func TestLoadProjectOrderMalformedYAMLIsRefused(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, OrderRelPath), "epics: [this is not a mapping\n")

	_, err := LoadProjectOrder(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), OrderRelPath)
}
