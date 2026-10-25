package readinput

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// ForNext carries the reader and the clock reading, and a failed event read
// inside the window degrades to no recent events instead of failing the read.
func TestForNextDegradesOnAFailedEventRead(t *testing.T) {
	dir := t.TempDir()
	partition := filepath.Join(dir, ".tiller", "events", "2026", "10", "20")
	require.NoError(t, os.MkdirAll(partition, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(partition, "EV-aaaaaa.yml"), []byte("id: [unterminated\n"), 0o644))
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	in := ForNext(&store.Tree{Root: dir}, "jc", now)
	require.Equal(t, "jc", in.Reader)
	require.Equal(t, now, in.Now)
	require.Nil(t, in.Recent)
}
