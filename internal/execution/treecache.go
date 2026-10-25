package execution

import "sync"

// A commit's tree never changes under its object id, so every observation of
// the same tip, in this request or a later one, shares one parsed inventory.
// The console watches a handful of task tips at a time; the limit only stops a
// long session from growing without bound, and clearing on overflow is enough.
// Count alone does not bound memory, since one inventory holds an entry per
// file in the repository, so the cache is also capped by the total number of
// tree entries it holds. An inventory larger than the whole budget is returned
// without being cached.
const (
	treeCacheLimit       = 64
	treeCacheEntryBudget = 250_000
)

type treeCache struct {
	mu      sync.Mutex
	limit   int
	budget  int
	held    int
	entries map[string]map[string]treeEntry
	read    func(root, oid string) (map[string]treeEntry, error)
}

var commitTrees = &treeCache{limit: treeCacheLimit, budget: treeCacheEntryBudget, entries: map[string]map[string]treeEntry{}, read: readTree}

// inventory returns oid's tree inventory, reading it from root at most once
// per repository. The returned map is shared and must not be modified.
func (c *treeCache) inventory(root, common, oid string) (map[string]treeEntry, error) {
	key := common + "\x00" + oid
	c.mu.Lock()
	entries, ok := c.entries[key]
	c.mu.Unlock()
	if ok {
		return entries, nil
	}
	entries, err := c.read(root, oid)
	if err != nil {
		return nil, err
	}
	if len(entries) > c.budget {
		return entries, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cached, ok := c.entries[key]; ok {
		// A concurrent reader stored this commit first; keep that copy.
		return cached, nil
	}
	if len(c.entries) >= c.limit || c.held+len(entries) > c.budget {
		clear(c.entries)
		c.held = 0
	}
	c.entries[key] = entries
	c.held += len(entries)
	return entries, nil
}
