package usage

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// The console rereads .tiller/usage/ on every Agent use request. A usage file
// changes only when capture rewrites it, so a process that opts in keeps each
// parsed file keyed by its path and reuses it while the directory listing
// still reports the same file, size, modification time, and mode, the same
// rule as the event cache (internal/core/events/cache.go). Every LoadFiles
// still lists every harness directory, so a new, removed, or rewritten file
// is seen on the very next read. One-shot CLI commands never enable it.

// processCache is nil unless EnableProcessCache ran.
var processCache atomic.Pointer[cache]

var enableOnce sync.Once

// EnableProcessCache turns the parsed usage-file cache on for this process.
// tiller serve calls it once at startup.
func EnableProcessCache() {
	enableOnce.Do(func() { processCache.Store(newCache()) })
}

type cachedFile struct {
	info fs.FileInfo
	file File
}

// matches reports whether a fresh Lstat still names the file this entry was
// parsed from.
func (c cachedFile) matches(info fs.FileInfo) bool {
	return os.SameFile(c.info, info) &&
		c.info.Size() == info.Size() &&
		c.info.ModTime().Equal(info.ModTime()) &&
		c.info.Mode() == info.Mode()
}

type cache struct {
	mu     sync.Mutex
	roots  map[string]map[string]cachedFile
	parsed int
}

func newCache() *cache {
	return &cache{roots: map[string]map[string]cachedFile{}}
}

func cacheKey(root string) string {
	if absolute, err := filepath.Abs(root); err == nil {
		return absolute
	}
	return root
}

// known returns the root's cached files by repository-relative path. The map
// is never modified after replace stores it, so callers read it without a lock.
func (c *cache) known(root string) map[string]cachedFile {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.roots[cacheKey(root)]
}

// replace stores what one successful listing saw, dropping every path it did
// not.
func (c *cache) replace(root string, listed map[string]cachedFile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.roots[cacheKey(root)] = listed
}

func (c *cache) countParsed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.parsed++
}

// misses is how many usage files this cache has parsed, for tests.
func (c *cache) misses() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.parsed
}
