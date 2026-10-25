package mutate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvidenceCacheReusesOnlyPasses(t *testing.T) {
	t.Parallel()
	cache := &evidenceCache{limit: 2, entries: map[evidenceKey]bool{}}
	calls := 0
	pass := func() error { calls++; return nil }
	fail := func() error { calls++; return errors.New("result_not_on_integration") }
	key := evidenceKey{root: "/repo", into: "main", tip: "aaaa"}

	require.NoError(t, cache.check(key, pass))
	require.NoError(t, cache.check(key, pass))
	require.Equal(t, 1, calls, "a pass for the same record and tip is reused")

	moved := key
	moved.tip = "bbbb"
	require.Error(t, cache.check(moved, fail))
	require.Error(t, cache.check(moved, fail))
	require.Equal(t, 3, calls, "a refusal is rechecked every time")
	require.NoError(t, cache.check(moved, pass))
	require.Equal(t, 4, calls)

	third := key
	third.tip = "cccc"
	require.NoError(t, cache.check(third, pass))
	require.Len(t, cache.entries, 1, "reaching the limit clears before storing")

	rebased := third
	rebased.basis = "content"
	before := calls
	require.NoError(t, cache.check(rebased, pass))
	require.Equal(t, before+1, calls, "a key differing only in the recorded basis is revalidated")
}
