package selfupdate

// This one white-box test reaches SwapBinary's cross-device fallback
// directly, in package selfupdate rather than selfupdate_test, because the
// seam it exercises (renameFile) is unexported. Forcing a genuine EXDEV
// needs two real filesystems, which a portable unit test cannot assume;
// stubbing renameFile's first call is the substitute, delegating to the
// real os.Rename afterward so the fallback's own staging, rename, mode,
// and cleanup all still happen for real.

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSwapBinaryFallsBackOnACrossDeviceRename(t *testing.T) {
	original := renameFile
	calls := 0
	renameFile = func(oldpath, newpath string) error {
		calls++
		if calls == 1 {
			return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
		}
		return os.Rename(oldpath, newpath)
	}
	t.Cleanup(func() { renameFile = original })

	dir := t.TempDir()
	target := filepath.Join(dir, "tiller")
	require.NoError(t, os.WriteFile(target, []byte("old-bytes"), 0755))

	newPath := filepath.Join(dir, "tiller.download")
	require.NoError(t, os.WriteFile(newPath, []byte("new-bytes"), 0644))

	err := SwapBinary(newPath, target)
	require.NoError(t, err)
	require.Equal(t, 2, calls, "expected the first rename to fail cross-device and the fallback's rename to succeed")

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "new-bytes", string(got))

	info, err := os.Stat(target)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0755), info.Mode().Perm())

	_, err = os.Stat(newPath)
	require.True(t, os.IsNotExist(err), "expected %s to be gone after the swap", newPath)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "expected only the target to remain in %s, got %v", dir, entries)
}
