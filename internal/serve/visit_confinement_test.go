package serve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVisitCacheDoesNotReadOrWriteOutsideState(t *testing.T) {
	for _, coordinate := range []string{".tiller", ".tiller/.cache", ".tiller/.cache/console.json"} {
		t.Run(coordinate, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			externalVisit := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
			externalCache := filepath.Join(outside, "console.json")
			data, err := json.Marshal(consoleCache{LastVisit: externalVisit})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(externalCache, data, 0o640))
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", ".cache"), 0o755))
			inside := filepath.Join(root, filepath.FromSlash(coordinate))
			target := outside
			if coordinate == ".tiller/.cache/console.json" {
				target = externalCache
			} else if coordinate == ".tiller" {
				require.NoError(t, os.Mkdir(filepath.Join(outside, ".cache"), 0o755))
				externalCache = filepath.Join(outside, ".cache", "console.json")
				require.NoError(t, os.WriteFile(externalCache, data, 0o640))
			}
			require.NoError(t, os.RemoveAll(inside))
			require.NoError(t, os.Symlink(target, inside))
			v := newVisitState()
			v.now = func() time.Time { return externalVisit.Add(time.Hour) }
			require.True(t, v.peek(root).IsZero(), "the external visit must not become the recap horizon")
			require.True(t, v.mark(root).IsZero())
			after, err := os.ReadFile(externalCache)
			require.NoError(t, err)
			require.Equal(t, data, after, "outside console memory must remain byte-identical")
			info, err := os.Stat(externalCache)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0o640), info.Mode().Perm())
		})
	}
}
