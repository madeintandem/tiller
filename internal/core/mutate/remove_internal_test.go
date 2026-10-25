package mutate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestPlanRemovalTreeKeepsValidatedStateSnapshotWhenBytesChangeBeforePlanning(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, ".tiller", "epics", "proposal", "features", "obsolete", "tasks", "remove-me")
	path := filepath.Join(dir, store.TaskStateFile)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(path, []byte("id: T-rem001\nstatus: proposed\n"), 0o644))
	_, validated, err := store.ReadRegularFileSnapshot(root, path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("id: T-rem001\nstatus: ready\n"), 0o644))

	writes, _, err := planRemovalTree(root, dir, map[string]store.FileSnapshot{path: validated})
	require.NoError(t, err)
	require.NotEmpty(t, writes)
	require.Equal(t, path, writes[0].Path)
	require.Equal(t, validated, writes[0].Before,
		"the deletion must retain the eligibility snapshot instead of accepting newer state bytes")
}
