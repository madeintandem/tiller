package events

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// The console is a long-lived reader of an append-only stream, and it used to
// reread and reparse every event file on every request. Event files are
// immutable once written (artifact-schema.md, "Event stream"), so a process
// that opts in keeps each parsed event keyed by its path and reuses it while
// the directory listing still reports the same file. Every List still lists
// every dated directory through the confined reader, so a new, removed,
// replaced, or symlinked file is seen on the very next read. One-shot CLI
// commands never enable it.

// processCache is nil unless EnableProcessCache ran.
var processCache atomic.Pointer[cache]

var enableOnce sync.Once

// EnableProcessCache turns the parsed-event cache on for this process.
// tiller serve calls it once at startup.
func EnableProcessCache() {
	enableOnce.Do(func() { processCache.Store(newCache()) })
}

type cachedEvent struct {
	info  fs.FileInfo
	event Event
}

// matches reports whether a fresh Lstat still names the file this event was
// parsed from: the same file, size, modification time, and mode.
func (c cachedEvent) matches(info fs.FileInfo) bool {
	return os.SameFile(c.info, info) &&
		c.info.Size() == info.Size() &&
		c.info.ModTime().Equal(info.ModTime()) &&
		c.info.Mode() == info.Mode()
}

type cache struct {
	mu     sync.Mutex
	roots  map[string]map[string]cachedEvent
	parsed int
}

func newCache() *cache {
	return &cache{roots: map[string]map[string]cachedEvent{}}
}

func cacheKey(root string) string {
	if absolute, err := filepath.Abs(root); err == nil {
		return absolute
	}
	return root
}

// known returns the root's cached events by repository-relative path. The map
// is never modified after replace stores it, so callers read it without a lock.
func (c *cache) known(root string) map[string]cachedEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.roots[cacheKey(root)]
}

// replace stores what one successful walk saw, dropping every path it did not.
func (c *cache) replace(root string, listed map[string]cachedEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.roots[cacheKey(root)] = listed
}

// add keeps everything already cached for the root and adds what a windowed
// walk newly parsed. A windowed walk did not see the whole stream, so it must
// not drop the entries outside its window. The stored map is replaced, never
// modified, because known hands it to readers without a lock.
func (c *cache) add(root string, parsed map[string]cachedEvent) {
	if len(parsed) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := cacheKey(root)
	merged := make(map[string]cachedEvent, len(c.roots[key])+len(parsed))
	for path, entry := range c.roots[key] {
		merged[path] = entry
	}
	for path, entry := range parsed {
		merged[path] = entry
	}
	c.roots[key] = merged
}

func (c *cache) countParsed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.parsed++
}

// misses is how many event files this cache has parsed, for tests.
func (c *cache) misses() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.parsed
}
