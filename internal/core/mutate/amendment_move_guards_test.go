package mutate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestReviewedMoveCannotRebaseValidatedInputDuringPlanning(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"epics/source/features/feature/feature.md", "epics/source/epic.yml", "epics/source/features/feature/.cache/authored.txt"} {
		t.Run(file, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".tiller", file)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte("reviewed bytes"), 0o644))
			guards, err := captureMoveReadGuards(root)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, []byte("external edit after validation"), 0o644))
			_, later, err := store.ReadRegularFileSnapshot(root, path)
			require.NoError(t, err)
			// A directory planner or parent editor that rereads must not silently
			// substitute this newer snapshot for the bytes validation observed.
			_, err = appendUniqueMoveGuards([]store.PlannedWrite{store.PlanWrite(path, later, []byte("planned result"))}, guards)
			require.Error(t, err)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "external edit after validation", string(data))
		})
	}
}

// Agent usage files are token counts, never move inputs: the guards skip
// .tiller/usage/, so a capture landing during a move cannot make it stale.
func TestMoveReadGuardsSkipUsageFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, rel := range []string{"epics/source/epic.yml", "usage/claude-code/s1.yml"} {
		path := filepath.Join(root, ".tiller", rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("x: 1\n"), 0o644))
	}
	guards, err := captureMoveReadGuards(root)
	require.NoError(t, err)
	usage := filepath.Join(root, ".tiller", "usage")
	sawEpic := false
	for _, g := range guards {
		require.False(t, g.Path == usage || strings.HasPrefix(g.Path, usage+string(filepath.Separator)), "guard on %s", g.Path)
		sawEpic = sawEpic || strings.HasSuffix(g.Path, "epic.yml")
	}
	require.True(t, sawEpic)
}

func TestReviewedMoveCannotRebaseDirectoryInventoryOrModes(t *testing.T) {
	t.Parallel()
	for _, change := range []string{"membership", "directory-mode", "file-mode"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, ".tiller", "source")
			file := filepath.Join(dir, "feature.md")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(file, []byte("reviewed"), 0o644))
			guards, err := captureMoveReadGuards(root)
			require.NoError(t, err)
			switch change {
			case "membership":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "new.md"), []byte("unreviewed"), 0o644))
			case "directory-mode":
				require.NoError(t, os.Chmod(dir, 0o700))
			case "file-mode":
				require.NoError(t, os.Chmod(file, 0o600))
			}
			directory, err := store.ReadDirectorySnapshot(root, dir)
			require.NoError(t, err)
			_, snapshot, err := store.ReadRegularFileSnapshot(root, file)
			require.NoError(t, err)
			info, err := os.Stat(file)
			require.NoError(t, err)
			writes := []store.PlannedWrite{store.PlanRemoveEmptyDir(dir, directory), store.PlanDeleteFileWithMode(file, snapshot, info.Mode())}
			_, err = appendUniqueMoveGuards(writes, guards)
			require.Error(t, err)
			require.FileExists(t, file)
			if change == "membership" {
				require.FileExists(t, filepath.Join(dir, "new.md"))
			}
		})
	}
}
