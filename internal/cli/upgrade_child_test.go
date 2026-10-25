package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/selfupdate"
	"github.com/stretchr/testify/require"
)

func TestMaintenanceChildLostOutputDoesNotClaimRollback(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "tiller")
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'tiller version 1.0.0'; exit 0; fi\nprintf changed > side-effect\necho malformed\nexit 3\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	selected, err := selfupdate.ReadExecutable(context.Background(), path)
	require.NoError(t, err)
	result, err := runMaintenanceChild(context.Background(), selected, root, "", false)
	require.Error(t, err)
	require.Equal(t, "changed", readFile(t, root, "side-effect"))
	require.Equal(t, "unknown", result.Project.Migration.Status)
	require.Equal(t, "unknown", result.Project.Artifacts.Status)
	require.False(t, result.Complete)
}

func TestMaintenanceChildCancellationBoundsDescendantPipes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "tiller")
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'tiller version 1.0.0'; exit 0; fi\nsleep 30 &\nwait\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	selected, err := selfupdate.ReadExecutable(context.Background(), path)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	result, err := runMaintenanceChild(ctx, selected, root, "", false)
	require.Error(t, err)
	require.Less(t, time.Since(start), 3*time.Second)
	require.Equal(t, "maintenance_child_timeout", classify(err).Reason)
	require.Equal(t, "unknown", result.Project.Status)
}
