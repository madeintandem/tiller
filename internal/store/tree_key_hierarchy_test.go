package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests pin the loader's key rule for the tree nouns: a recorded key is
// the whole canonical key the node's folders spell, not just a key that ends in
// the node's own folder. The loader hangs a node on its parent by the folder it
// is in, and every reference, branch, and lookup uses the recorded key, so a
// key that names another parent gives one node two identities.

// hierarchyNode is one state file of a hand-laid tree: its path under
// .tiller/epics and the key it records ("" records no key).
type hierarchyNode struct {
	path string
	id   string
	key  string
}

// layHierarchy writes a manifest and the given state files, and returns the
// root and each file's absolute path by its relative one.
func layHierarchy(t *testing.T, nodes ...hierarchyNode) (string, map[string]string) {
	t.Helper()
	root := writeManifestFixture(t, "schema_version: 16\nschema_epoch: initial\nproject:\n  name: X\n  key: x\n  kind: greenfield\ncontributors: []\n")
	paths := map[string]string{}
	for _, n := range nodes {
		abs := filepath.Join(root, ".tiller", "epics", filepath.FromSlash(n.path))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		body := "id: " + n.id + "\n"
		if n.key != "" {
			body += "key: " + n.key + "\n"
		}
		require.NoError(t, os.WriteFile(abs, []byte(body+"status: draft\nblocked: false\n"), 0o644))
		paths[n.path] = abs
	}
	return root, paths
}

// twoEpics is a valid two-epic tree: alpha and beta each hold a feature named
// shared with one task, the shape a cross-parent move starts from.
func twoEpics() []hierarchyNode {
	return []hierarchyNode{
		{"alpha/epic.yml", "E-alpha1", "alpha"},
		{"alpha/features/shared/feature.yml", "F-alpha1", "alpha/shared"},
		{"alpha/features/shared/tasks/build/task.yml", "T-alpha1", "alpha/shared/build"},
		{"beta/epic.yml", "E-beta01", "beta"},
		{"beta/features/other/feature.yml", "F-beta01", "beta/other"},
		{"beta/features/other/tasks/build/task.yml", "T-beta01", "beta/other/build"},
	}
}

func TestLoadAcceptsKeysThatSpellTheirFolders(t *testing.T) {
	root, _ := layHierarchy(t, twoEpics()...)

	tree, err := Load(root)

	require.NoError(t, err)
	require.Equal(t, "alpha", tree.Features["F-alpha1"].EpicKey)
	require.Equal(t, "alpha/shared", *tree.Features["F-alpha1"].Key)
	require.Equal(t, "F-beta01", tree.Tasks["T-beta01"].FeatureID)
	require.Equal(t, "beta/other/build", *tree.Tasks["T-beta01"].Key)
}

// TestLoadRefusesAKeyThatNamesAnotherPlaceInTheTree covers the keys the
// last-segment check let through: each still ends in the node's own folder,
// and each names a different parent, or no possible parent at all. The refusal
// names the state file, the recorded key, and the key the folders spell, and
// changes nothing on disk.
func TestLoadRefusesAKeyThatNamesAnotherPlaceInTheTree(t *testing.T) {
	tests := []struct {
		name     string
		path     string // the state file to corrupt
		recorded string
		want     string
	}{
		{"a feature moved under another epic keeps its old key",
			"beta/features/other/feature.yml", "alpha/other", "beta/other"},
		{"a feature records its local key alone",
			"alpha/features/shared/feature.yml", "shared", "alpha/shared"},
		{"a task names another feature",
			"alpha/features/shared/tasks/build/task.yml", "alpha/elsewhere/build", "alpha/shared/build"},
		{"a task names another epic",
			"alpha/features/shared/tasks/build/task.yml", "beta/shared/build", "alpha/shared/build"},
		{"a task records a feature-shaped key",
			"alpha/features/shared/tasks/build/task.yml", "shared/build", "alpha/shared/build"},
		{"an epic key carries a slash",
			"alpha/epic.yml", "nested/alpha", "alpha"},
		{"an epic key climbs out of its folder",
			"alpha/epic.yml", "../alpha", "alpha"},
		{"a feature key climbs out of its epic",
			"alpha/features/shared/feature.yml", "alpha/../shared", "alpha/shared"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := twoEpics()
			for i := range nodes {
				if nodes[i].path == tt.path {
					nodes[i].key = tt.recorded
				}
			}
			root, paths := layHierarchy(t, nodes...)
			before, err := os.ReadFile(paths[tt.path])
			require.NoError(t, err)

			tree, err := Load(root)

			require.Error(t, err)
			require.Nil(t, tree, "a refused tree is not returned in part")
			require.Contains(t, err.Error(), paths[tt.path], "the state file")
			require.Contains(t, err.Error(), `"`+tt.recorded+`"`, "the recorded key")
			require.Contains(t, err.Error(), `"`+tt.want+`"`, "the key the folders spell")
			after, err := os.ReadFile(paths[tt.path])
			require.NoError(t, err)
			require.Equal(t, string(before), string(after), "the loader rewrites nothing")
			require.DirExists(t, filepath.Dir(paths[tt.path]), "and moves nothing")
		})
	}
}

// TestLoadPlacesAKeylessNodeByItsFolders keeps the absent-key rule at every
// tree grain: a node that records no key loads, and a keyless parent does not
// loosen the rule for the nodes under it, whose keys are still checked against
// the folders.
func TestLoadPlacesAKeylessNodeByItsFolders(t *testing.T) {
	nodes := twoEpics()
	nodes[0].key, nodes[1].key = "", "" // alpha, and alpha/shared
	root, _ := layHierarchy(t, nodes...)

	tree, err := Load(root)

	require.NoError(t, err)
	require.Nil(t, tree.Epics["E-alpha1"].Key)
	require.Nil(t, tree.Features["F-alpha1"].Key)
	require.Equal(t, "alpha", tree.Features["F-alpha1"].EpicKey)
	require.Equal(t, "alpha/shared/build", *tree.Tasks["T-alpha1"].Key)

	nodes[2].key = "beta/shared/build"
	root, paths := layHierarchy(t, nodes...)
	_, err = Load(root)
	require.ErrorContains(t, err, paths["alpha/features/shared/tasks/build/task.yml"])
	require.ErrorContains(t, err, `"alpha/shared/build"`)
}
