package execution_test

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/execution"
	"github.com/stretchr/testify/require"
)

// stableReport is a report with the per-load observation time zeroed, the one
// field that legitimately differs between two loads of the same project.
func stableReport(snapshot *execution.Snapshot) execution.Report {
	report := snapshot.Report()
	for i := range report.Observations {
		report.Observations[i].ObservedAt = time.Time{}
	}
	return report
}

// Concurrent selection must produce the same observation every time.
func TestConcurrentObservationIsDeterministic(t *testing.T) {
	root := fixture(t)
	first := worktree(t, root, firstKey)
	second := worktree(t, root, secondKey)
	executing(t, first, firstID, firstKey, "sam", "check")
	executing(t, second, secondID, secondKey, "pat", "in_progress")
	tree := base(t, root)
	want := stableReport(load(t, tree))
	for i := 0; i < 10; i++ {
		require.Equal(t, want, stableReport(load(t, tree)))
	}
}

// A commit in one of several observed worktrees during a read drops only that
// worktree's observation.
func TestConcurrentObservationDropsOnlyTheMovedSource(t *testing.T) {
	root := fixture(t)
	first := worktree(t, root, firstKey)
	second := worktree(t, root, secondKey)
	executing(t, first, firstID, firstKey, "sam", "check")
	executing(t, second, secondID, secondKey, "pat", "in_progress")
	snapshot := load(t, base(t, root))
	require.Contains(t, snapshot.Observations, firstID)
	require.Contains(t, snapshot.Observations, secondID)

	git(t, first, "add", ".")
	git(t, first, "commit", "-m", "agent commits during the read")
	dropped, err := snapshot.Settle()
	require.NoError(t, err)
	require.True(t, dropped)
	require.NotContains(t, snapshot.Observations, firstID)
	require.Contains(t, snapshot.Observations, secondID)
}
