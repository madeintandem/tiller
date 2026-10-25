package serve

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/stretchr/testify/require"
)

// TestReadRegularFromRootRejectsSymlinkComponents models the state after an
// approved canonical evidence name is swapped at either its leaf or parent.
// The component-bound reader must refuse before any bytes from the new target
// can be returned.
func TestReadRegularFromRootRejectsSymlinkComponents(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".git")
	require.NoError(t, os.MkdirAll(hidden, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hidden, "config"), []byte("secret\n"), 0o644))
	visible := filepath.Join(root, "artifacts", "tiller", "gates")
	require.NoError(t, os.MkdirAll(visible, 0o755))
	require.NoError(t, os.Symlink(filepath.Join(hidden, "config"), filepath.Join(visible, "leaf.txt")))
	require.NoError(t, os.Symlink(hidden, filepath.Join(root, "linked-gates")))

	anchored, err := os.OpenRoot(root)
	require.NoError(t, err)
	defer anchored.Close()

	for _, name := range []string{
		"artifacts/tiller/gates/leaf.txt",
		"linked-gates/config",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := readRegularFromRoot(anchored, name)
			require.Nil(t, data)
			require.True(t, errors.Is(err, errUnsafeServedPath), "error: %v", err)
		})
	}
}

// TestEvidenceReadRejectsParentSwap proves an approved evidence name cannot be
// redirected into a hidden directory between component operations. The hidden
// replacement has the same leaf name, so the rejection depends on binding the
// opened gates directory to both parent observations before descending further.
func TestEvidenceReadRejectsParentSwap(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".git")
	gates := filepath.Join(root, "artifacts", "tiller", "gates")
	require.NoError(t, os.MkdirAll(hidden, 0o755))
	require.NoError(t, os.MkdirAll(gates, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hidden, "config"), []byte("secret\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(gates, "config"), []byte("approved\n"), 0o644))
	name := filepath.FromSlash("artifacts/tiller/gates/config")

	anchored, err := os.OpenRoot(root)
	require.NoError(t, err)
	defer anchored.Close()

	swapped := false
	data, err := readRegularFromRootWithWalker(anchored, name, rootpath.Walker{
		AfterOpenRoot: func(opened string) {
			if swapped || opened != filepath.FromSlash("artifacts/tiller/gates") {
				return
			}
			swapped = true
			require.NoError(t, os.Rename(gates, filepath.Join(root, "approved-gates")))
			require.NoError(t, os.Symlink(hidden, gates))
		},
	})
	require.True(t, swapped)
	require.Nil(t, data)
	require.True(t, errors.Is(err, errUnsafeServedPath), "error: %v", err)
}
