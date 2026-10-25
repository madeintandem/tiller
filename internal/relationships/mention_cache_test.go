package relationships

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
)

// Each helper call models a new request with a lazily constructed parser.
type testMentionCache struct{ *mentionCache }

func (c testMentionCache) mentions(source []byte) []mention {
	var markdown goldmark.Markdown
	return c.mentionCache.mentions(source, &markdown)
}

func newTestMentionCache(entries, bytes int) testMentionCache {
	return testMentionCache{newMentionCache(entries, bytes)}
}

func TestMentionCacheWarmHitsLeaveParserUninitialized(t *testing.T) {
	cache := newMentionCache(4, 4096)
	var markdown goldmark.Markdown
	cache.mentions([]byte("F-aaaaaa"), &markdown)
	require.NotNil(t, markdown)
	initialized := markdown
	cache.mentions([]byte("F-bbbbbb"), &markdown)
	require.Same(t, initialized, markdown, "misses in one request reuse its parser")
	markdown = nil
	require.Equal(t, []mention{{id: "F-aaaaaa", line: 1}}, cache.mentions([]byte("F-aaaaaa"), &markdown))
	require.Nil(t, markdown, "warm hits neither construct a parser nor parse Markdown")
	cache.mentions(nil, &markdown)
	require.NotNil(t, markdown)
	markdown = nil
	require.Empty(t, cache.mentions(nil, &markdown))
	require.Nil(t, markdown, "empty parsing results are hits too")
}

func TestMentionCacheReusesFactsWithoutRetainingSource(t *testing.T) {
	cache := newTestMentionCache(4, 4096)
	source := []byte("F-aaaaaa\n\n`T-bbbbbb`\n")
	first := cache.mentions(source)
	require.Equal(t, []mention{{id: "F-aaaaaa", line: 1}, {id: "T-bbbbbb", line: 3}}, first)
	second := cache.mentions(append([]byte(nil), source...))
	require.Same(t, &first[0], &second[0], "a hit reuses the already parsed facts")
	source[2] = 'c'
	require.Equal(t, "F-aaaaaa", first[0].id, "cached facts do not alias filesystem bytes")
	require.Equal(t, "F-caaaaa", cache.mentions(source)[0].id)
	require.Len(t, cache.entries, 2)
}

func TestMentionCacheBoundsEvictionAndOversize(t *testing.T) {
	cache := newTestMentionCache(2, 4096)
	one, two, three := []byte("F-aaaaaa"), []byte("F-bbbbbb"), []byte("F-cccccc")
	cache.mentions(one)
	cache.mentions(two)
	cache.mentions(one)
	cache.mentions(three)
	require.Len(t, cache.entries, 2)
	require.Contains(t, cache.entries, sha256.Sum256(one))
	require.NotContains(t, cache.entries, sha256.Sum256(two))
	require.LessOrEqual(t, cache.bytes, cache.maxBytes)
	require.Equal(t, []mention{{id: "F-bbbbbb", line: 1}}, cache.mentions(two), "evicted input still parses correctly")

	for _, source := range [][]byte{
		[]byte(strings.Repeat("F-aaaaaa\n", 200)), // Small input, oversized parsed facts.
		[]byte("F-aaaaaa\n\n" + strings.Repeat("x", 4097)),
	} {
		before := cache.bytes
		facts := cache.mentions(source)
		require.NotEmpty(t, facts)
		require.NotContains(t, cache.entries, sha256.Sum256(source))
		require.Equal(t, before, cache.bytes, "oversize bypass does not evict useful facts")
	}
}

func TestMentionCacheEnforcesByteBudgetBeforeEntryBudget(t *testing.T) {
	cache := newTestMentionCache(8, 4096)
	for i := range 20 {
		source := []byte(fmt.Sprintf("F-%06d\n", i) + strings.Repeat("T-bbbbbb\n", 20))
		require.NotEmpty(t, cache.mentions(source))
		require.LessOrEqual(t, cache.bytes, cache.maxBytes)
		require.LessOrEqual(t, len(cache.entries), cache.maxEntries)
	}
	require.Less(t, len(cache.entries), cache.maxEntries)
}

func TestMentionCacheConcurrentReaders(t *testing.T) {
	cache := newTestMentionCache(8, 4096)
	var readers sync.WaitGroup
	for worker := range 16 {
		readers.Go(func() {
			for i := range 40 {
				id := fmt.Sprintf("F-%06d", (worker+i)%12)
				facts := cache.mentions([]byte(id + "\n`T-aaaaaa`"))
				assert.Equal(t, []mention{{id: id, line: 1}, {id: "T-aaaaaa", line: 2}}, facts)
			}
		})
	}
	readers.Wait()
	require.LessOrEqual(t, len(cache.entries), cache.maxEntries)
	require.LessOrEqual(t, cache.bytes, cache.maxBytes)
}

func TestCachedMentionsProjectCurrentTreePathAndOwnership(t *testing.T) {
	content := []byte("F-aaaaaa\n\nF-aaaaaa\n")
	firstTree := &store.Tree{Features: map[string]*store.Feature{
		"F-bbbbbb": {ID: "F-bbbbbb", Key: new("work/first")},
	}}
	first := BuildStructural(firstTree)
	firstPath := ".tiller/epics/work/features/first/feature.md"
	AddDocuments(firstTree, first, []Document{{Path: firstPath, Content: content}})
	first.Finalize()
	require.True(t, first.Vertices["F-aaaaaa"].Unresolved)
	require.Equal(t, "F-bbbbbb", first.Vertices[DocumentID(firstPath)].DocumentOwner)
	require.Len(t, first.Diagnostics, 2)

	secondTree := &store.Tree{Features: map[string]*store.Feature{
		"F-aaaaaa": {ID: "F-aaaaaa", Key: new("work/current")},
	}}
	second := BuildStructural(secondTree)
	secondPath := ".tiller/epics/work/features/current/feature.md"
	AddDocuments(secondTree, second, []Document{{Path: secondPath, Content: append([]byte(nil), content...)}})
	second.Finalize()
	require.False(t, second.Vertices["F-aaaaaa"].Unresolved)
	require.Equal(t, "F-aaaaaa", second.Vertices[DocumentID(secondPath)].DocumentOwner)
	require.Empty(t, second.Diagnostics)
	require.NotContains(t, second.Vertices, DocumentID(firstPath))
	edges := second.Adjacent(DocumentID(secondPath), "mentions")
	require.Len(t, edges, 1)
	require.Equal(t, []Source{{File: secondPath, Line: 1}, {File: secondPath, Line: 3}}, edges[0].Sources)
	edges[0].Sources[0].Line = 99
	third := BuildStructural(secondTree)
	AddDocuments(secondTree, third, []Document{{Path: secondPath, Content: content}})
	third.Finalize()
	require.Equal(t, 1, third.Adjacent(DocumentID(secondPath), "mentions")[0].Sources[0].Line, "index mutation cannot affect cached facts")
}
