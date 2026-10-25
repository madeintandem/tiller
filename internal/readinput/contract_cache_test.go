package readinput

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cachedContractFixture = "---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: preserves behavior\n    polarity: preserve\n    required: true\n    binding:\n      type: test\n      ref: [src/one_test.go, src/two_test.go]\n    baseline:\n      surface: src\n      captured_at: git:abcdef0\n      artifact: testdata/before.json\n---\n"

func TestContractCacheReturnsRequestOwnedModels(t *testing.T) {
	cache := newContractCache(4, 8192)
	source := []byte(cachedContractFixture)
	first, failure := cache.parse(source)
	require.Empty(t, failure)
	expected, err := store.ParseEvalFile(source)
	require.NoError(t, err)
	require.Equal(t, expected, first)
	entry := cache.entries[sha256.Sum256(source)].Value.(contractCacheEntry)
	require.NotSame(t, entry.contract, first)
	require.NotSame(t, &entry.contract.Criteria[0], &first.Criteria[0])
	require.NotSame(t, entry.contract.Criteria[0].Baseline, first.Criteria[0].Baseline)
	first.Node = "F-bbbbbb"
	first.Criteria[0].Statement = "mutated"
	first.Criteria[0].Binding.Ref[0] = "src/changed.go"
	first.Criteria[0].Baseline.CapturedAt.Value = "git:changed"
	first.Criteria[0].Baseline.Artifact = "changed.json"
	first.Criteria = append(first.Criteria, store.EvalCriterion{ID: "C2"})
	second, failure := cache.parse(append([]byte(nil), source...))
	require.Empty(t, failure)
	require.Equal(t, expected, second)
	require.Same(t, entry.contract, cache.entries[sha256.Sum256(source)].Value.(contractCacheEntry).contract)
	source[11] = 'z'
	require.Equal(t, expected, entry.contract, "parsed models do not retain filesystem byte slices")
}

func TestContractCacheRetainsFailuresAndRepairsByContent(t *testing.T) {
	cache := newContractCache(4, 4096)
	broken := []byte("---\nnode: [\n---\n")
	contract, failure := cache.parse(broken)
	require.Nil(t, contract)
	require.NotEmpty(t, failure)
	require.Equal(t, failure, cache.entries[sha256.Sum256(broken)].Value.(contractCacheEntry).failure)
	again, repeated := cache.parse(append([]byte(nil), broken...))
	require.Nil(t, again)
	require.Equal(t, failure, repeated)
	repaired, failure := cache.parse([]byte("---\nnode: F-aaaaaa\ncriteria: []\n---\n"))
	require.Empty(t, failure)
	require.Equal(t, "F-aaaaaa", repaired.Node)
	require.Len(t, cache.entries, 2)
}

func TestContractCacheBoundsEvictionAndOversize(t *testing.T) {
	cache := newContractCache(2, 4096)
	content := func(id string) []byte { return []byte("---\nnode: " + id + "\ncriteria: []\n---\n") }
	one, two, three := content("F-aaaaaa"), content("F-bbbbbb"), content("F-cccccc")
	cache.parse(one)
	cache.parse(two)
	cache.parse(one)
	cache.parse(three)
	require.Len(t, cache.entries, 2)
	require.Contains(t, cache.entries, sha256.Sum256(one))
	require.NotContains(t, cache.entries, sha256.Sum256(two))
	require.LessOrEqual(t, cache.bytes, cache.maxBytes)
	contract, failure := cache.parse(two)
	require.Empty(t, failure)
	require.Equal(t, "F-bbbbbb", contract.Node)

	// YAML aliases can expand a small document into a much larger Go value.
	expanded := []byte("---\nnode: F-aaaaaa\ncriteria:\n  - &criterion\n    id: C1\n    statement: works\n" + strings.Repeat("  - *criterion\n", 40) + "---\n")
	for _, source := range [][]byte{expanded, append(content("F-aaaaaa"), []byte(strings.Repeat("x", 4097))...)} {
		before := cache.bytes
		contract, failure := cache.parse(source)
		require.Empty(t, failure)
		require.Equal(t, "F-aaaaaa", contract.Node)
		require.NotContains(t, cache.entries, sha256.Sum256(source))
		require.Equal(t, before, cache.bytes, "oversized models do not evict retained parses")
	}
}

func TestContractCacheEnforcesByteBudgetBeforeEntryBudget(t *testing.T) {
	cache := newContractCache(8, 4096)
	for i := range 20 {
		source := []byte(fmt.Sprintf("---\nnode: F-%06d\ncriteria:\n  - id: C1\n    statement: %s\n---\n", i, strings.Repeat("works", 100)))
		contract, failure := cache.parse(source)
		require.Empty(t, failure)
		require.NotNil(t, contract)
		require.LessOrEqual(t, cache.bytes, cache.maxBytes)
		require.LessOrEqual(t, len(cache.entries), cache.maxEntries)
	}
	require.Less(t, len(cache.entries), cache.maxEntries)
}

func TestContractCacheConcurrentReadersOwnTheirCopies(t *testing.T) {
	cache := newContractCache(8, 8192)
	var readers sync.WaitGroup
	for worker := range 16 {
		readers.Go(func() {
			for i := range 40 {
				id := fmt.Sprintf("F-%06d", (worker+i)%12)
				source := []byte(strings.Replace(cachedContractFixture, "F-aaaaaa", id, 1))
				contract, failure := cache.parse(source)
				assert.Empty(t, failure)
				if !assert.NotNil(t, contract) {
					return
				}
				assert.Equal(t, id, contract.Node)
				assert.Equal(t, "src/one_test.go", contract.Criteria[0].Binding.Ref[0])
				assert.Equal(t, "git:abcdef0", contract.Criteria[0].Baseline.CapturedAt.Value)
				contract.Criteria[0].Binding.Ref[0] = "mutated.go"
				contract.Criteria[0].Baseline.CapturedAt.Value = "git:mutated"
			}
		})
	}
	readers.Wait()
	require.LessOrEqual(t, len(cache.entries), cache.maxEntries)
	require.LessOrEqual(t, cache.bytes, cache.maxBytes)
}
