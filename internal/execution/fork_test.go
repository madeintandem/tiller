package execution_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Two reads that share one observation each settle their own fork: a source
// that moves under one read is dropped from that read only.
func TestForksSettleIndependently(t *testing.T) {
	root := fixture(t)
	work := worktree(t, root, firstKey)
	executing(t, work, firstID, firstKey, "sam", "check")
	shared := load(t, base(t, root))
	require.Contains(t, shared.Observations, firstID)

	one, two := shared.Fork(), shared.Fork()
	git(t, work, "add", ".")
	git(t, work, "commit", "-m", "agent commits during the read")

	dropped, err := one.Settle()
	require.NoError(t, err)
	require.True(t, dropped)
	require.NotContains(t, one.Observations, firstID)
	require.Same(t, one.Base.Tasks[firstID], one.Tree.Tasks[firstID])

	require.Contains(t, two.Observations, firstID, "the other fork is untouched until it settles itself")
	require.NotSame(t, two.Base.Tasks[firstID], two.Tree.Tasks[firstID])
	require.Contains(t, shared.Observations, firstID, "the shared snapshot is never settled")
	require.NotSame(t, shared.Base.Tasks[firstID], shared.Tree.Tasks[firstID], "the shared display tree is untouched")
}
