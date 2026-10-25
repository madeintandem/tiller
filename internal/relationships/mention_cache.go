package relationships

import (
	"container/list"
	"crypto/sha256"
	"sync"

	"github.com/yuin/goldmark"
)

const (
	mentionCacheEntries = 2048
	mentionCacheBytes   = 8 << 20
)

var documentMentions = newMentionCache(mentionCacheEntries, mentionCacheBytes)

type mentionCacheEntry struct {
	digest [sha256.Size]byte
	facts  []mention
	bytes  int
}

// The cache owns immutable ID/line facts, never source bytes, ASTs, paths, or
// index data. AddDocuments projects these facts into a fresh index each time.
type mentionCache struct {
	mu         sync.Mutex
	entries    map[[sha256.Size]byte]*list.Element
	recent     list.List
	bytes      int
	maxEntries int
	maxBytes   int
}

func newMentionCache(maxEntries, maxBytes int) *mentionCache {
	// Reserve map capacity even after eviction: Go maps do not shrink.
	maxEntries = max(0, min(maxEntries, maxBytes/256))
	return &mentionCache{entries: make(map[[sha256.Size]byte]*list.Element), bytes: maxEntries * 128, maxEntries: maxEntries, maxBytes: maxBytes}
}

func (c *mentionCache) mentions(source []byte, markdown *goldmark.Markdown) []mention {
	// Oversized documents still parse normally, without hash or retention work.
	if len(source) > c.maxBytes || c.maxEntries <= 0 {
		return parseMentions(source, markdown)
	}
	digest := sha256.Sum256(source)
	c.mu.Lock()
	if element := c.entries[digest]; element != nil {
		c.recent.MoveToFront(element)
		facts := element.Value.(mentionCacheEntry).facts
		c.mu.Unlock()
		return facts
	}
	c.mu.Unlock()

	// Independent misses may duplicate parsing; parsing never holds the lock.
	facts := parseMentions(source, markdown)
	// Conservative retained-data accounting: entry/list overhead, full slice
	// capacity (a mention is 24 bytes on 64-bit), and owned 8-byte IDs.
	size := 256 + cap(facts)*32 + len(facts)*16
	if size > c.maxBytes-c.maxEntries*128 {
		return facts
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if element := c.entries[digest]; element != nil {
		c.recent.MoveToFront(element)
		return element.Value.(mentionCacheEntry).facts
	}
	for len(c.entries) >= c.maxEntries || c.bytes+size > c.maxBytes {
		oldest := c.recent.Back()
		entry := oldest.Value.(mentionCacheEntry)
		delete(c.entries, entry.digest)
		c.recent.Remove(oldest)
		c.bytes -= entry.bytes
	}
	c.entries[digest] = c.recent.PushFront(mentionCacheEntry{digest: digest, facts: facts, bytes: size})
	c.bytes += size
	return facts
}
