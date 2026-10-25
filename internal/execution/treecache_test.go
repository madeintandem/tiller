package execution

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func countingTrees(limit int, fail map[string]bool) (*treeCache, *int) {
	reads := 0
	return &treeCache{limit: limit, budget: 1 << 20, entries: map[string]map[string]treeEntry{}, read: func(_, oid string) (map[string]treeEntry, error) {
		reads++
		if fail[oid] {
			return nil, errors.New("no such commit")
		}
		return map[string]treeEntry{"README.md": {mode: "100644", kind: "blob", oid: oid + "-readme"}}, nil
	}}, &reads
}

func TestTreeInventoryIsReadOncePerCommit(t *testing.T) {
	trees, reads := countingTrees(8, nil)
	first, err := trees.inventory("/repo", "/repo/.git", "aaaa")
	require.NoError(t, err)
	second, err := trees.inventory("/repo/worktree", "/repo/.git", "aaaa")
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, 1, *reads, "a commit's tree never changes, so it is read once")
	_, err = trees.inventory("/repo", "/repo/.git", "bbbb")
	require.NoError(t, err)
	require.Equal(t, 2, *reads)
}

func TestTreeInventoryKeysByRepository(t *testing.T) {
	trees, reads := countingTrees(8, nil)
	_, err := trees.inventory("/one", "/one/.git", "aaaa")
	require.NoError(t, err)
	_, err = trees.inventory("/two", "/two/.git", "aaaa")
	require.NoError(t, err)
	require.Equal(t, 2, *reads)
}

func TestTreeInventoryDoesNotCacheFailures(t *testing.T) {
	trees, reads := countingTrees(8, map[string]bool{"gone": true})
	_, err := trees.inventory("/repo", "/repo/.git", "gone")
	require.Error(t, err)
	_, err = trees.inventory("/repo", "/repo/.git", "gone")
	require.Error(t, err)
	require.Equal(t, 2, *reads)
}

func TestTreeInventoryStaysBounded(t *testing.T) {
	trees, _ := countingTrees(2, nil)
	for _, oid := range []string{"a", "b", "c"} {
		_, err := trees.inventory("/repo", "/repo/.git", oid)
		require.NoError(t, err)
	}
	require.LessOrEqual(t, len(trees.entries), 2)
}

func totalCachedEntries(c *treeCache) int {
	total := 0
	for _, entries := range c.entries {
		total += len(entries)
	}
	return total
}

func TestTreeInventoryStaysWithinItsEntryBudget(t *testing.T) {
	// Each inventory holds one entry, so a budget of 3 admits three at a time.
	trees, _ := countingTrees(64, nil)
	trees.budget = 3
	for _, oid := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		_, err := trees.inventory("/repo", "/repo/.git", oid)
		require.NoError(t, err)
		require.LessOrEqual(t, totalCachedEntries(trees), trees.budget, "after %s", oid)
	}
}

func TestTreeInventoryLargerThanTheBudgetIsNotCached(t *testing.T) {
	trees, reads := countingTrees(64, nil)
	trees.budget = 0
	first, err := trees.inventory("/repo", "/repo/.git", "huge")
	require.NoError(t, err)
	require.Len(t, first, 1, "the inventory is still returned")
	require.Empty(t, trees.entries)
	_, err = trees.inventory("/repo", "/repo/.git", "huge")
	require.NoError(t, err)
	require.Equal(t, 2, *reads, "an uncached inventory is read again")
}
