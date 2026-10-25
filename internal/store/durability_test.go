package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The durability switch exists for one reason: a fixture that performs ~100
// atomic writes pays ~100 fsyncs, and the suite took twelve minutes (#87).
// Production binaries always sync. Under `go test` syncing is skipped unless
// TILLER_FSYNC=on, and TILLER_FSYNC=off turns it off in a real binary for the
// harnesses that drive one.

func TestFsyncIsOffUnderGoTestByDefault(t *testing.T) {
	t.Setenv("TILLER_FSYNC", "")
	require.False(t, fsyncEnabled(), "a test binary skips fsync unless told otherwise")
}

func TestFsyncEnvOverridesInBothDirections(t *testing.T) {
	t.Setenv("TILLER_FSYNC", "on")
	require.True(t, fsyncEnabled())
	t.Setenv("TILLER_FSYNC", "off")
	require.False(t, fsyncEnabled())
	t.Setenv("TILLER_FSYNC", "ON")
	require.True(t, fsyncEnabled(), "case-insensitive")
}

func TestSyncDirectorySkipsTheOpenWhenFsyncIsOff(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("TILLER_FSYNC", "off")
	require.NoError(t, syncDirectory(missing), "nothing to sync, nothing to open")
	t.Setenv("TILLER_FSYNC", "on")
	require.Error(t, syncDirectory(missing), "a real sync opens the directory and fails honestly")
}

func TestWriteFileAtomicStillWritesWithFsyncOff(t *testing.T) {
	t.Setenv("TILLER_FSYNC", "off")
	path := filepath.Join(t.TempDir(), "nested", "state.yml")
	require.NoError(t, writeFileAtomic(path, []byte("a: 1\n")))
	require.FileExists(t, path)
}
