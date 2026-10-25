package readinput

import (
	"container/list"
	"crypto/sha256"
	"strings"
	"sync"

	"github.com/madeintandem/tiller/internal/store"
)

const (
	contractCacheEntries = 2048
	contractCacheBytes   = 8 << 20
)

var corpusContracts = newContractCache(contractCacheEntries, contractCacheBytes)

type contractCacheEntry struct {
	digest   [sha256.Size]byte
	contract *store.EvalFile
	failure  string
	bytes    int
}

// Only freshly read, unsupplied contract content reaches this cache. Parsed
// models and parse-error strings are immutable; every returned model belongs
// to its request. Ownership checks and validation stay in AddContracts.
type contractCache struct {
	mu         sync.Mutex
	entries    map[[sha256.Size]byte]*list.Element
	recent     list.List
	bytes      int
	maxEntries int
	maxBytes   int
}

func newContractCache(maxEntries, maxBytes int) *contractCache {
	// Reserve map capacity even after eviction: Go maps do not shrink.
	maxEntries = max(0, min(maxEntries, maxBytes/256))
	return &contractCache{entries: make(map[[sha256.Size]byte]*list.Element), bytes: maxEntries * 128, maxEntries: maxEntries, maxBytes: maxBytes}
}

func parseContract(source []byte) (*store.EvalFile, string) {
	contract, err := store.ParseEvalFile(source)
	if err != nil {
		return nil, err.Error()
	}
	return contract, ""
}

func (c *contractCache) parse(source []byte) (*store.EvalFile, string) {
	if len(source) > c.maxBytes || c.maxEntries <= 0 {
		return parseContract(source)
	}
	digest := sha256.Sum256(source)
	c.mu.Lock()
	if element := c.entries[digest]; element != nil {
		c.recent.MoveToFront(element)
		entry := element.Value.(contractCacheEntry)
		c.mu.Unlock()
		return cloneContract(entry.contract), entry.failure
	}
	c.mu.Unlock()

	// Parsing and request-owned copies never hold the cache lock. Concurrent
	// misses may parse twice, but only one immutable result is retained.
	contract, failure := parseContract(source)
	size := contractBytes(contract, failure)
	if size > c.maxBytes-c.maxEntries*128 {
		return contract, failure
	}
	if contract != nil {
		for i := range contract.Criteria {
			if baseline := contract.Criteria[i].Baseline; baseline != nil {
				// Compact revisions trim whitespace while parsing; detach their
				// value from the potentially much larger original YAML scalar.
				baseline.CapturedAt.Value = strings.Clone(baseline.CapturedAt.Value)
			}
		}
	}
	c.mu.Lock()
	if element := c.entries[digest]; element != nil {
		c.recent.MoveToFront(element)
		entry := element.Value.(contractCacheEntry)
		c.mu.Unlock()
		return cloneContract(entry.contract), entry.failure
	}
	for len(c.entries) >= c.maxEntries || c.bytes+size > c.maxBytes {
		oldest := c.recent.Back()
		entry := oldest.Value.(contractCacheEntry)
		delete(c.entries, entry.digest)
		c.recent.Remove(oldest)
		c.bytes -= entry.bytes
	}
	c.entries[digest] = c.recent.PushFront(contractCacheEntry{digest: digest, contract: contract, failure: failure, bytes: size})
	c.bytes += size
	c.mu.Unlock()
	return cloneContract(contract), failure
}

func cloneContract(contract *store.EvalFile) *store.EvalFile {
	if contract == nil {
		return nil
	}
	owned := *contract
	if contract.Criteria != nil {
		owned.Criteria = make([]store.EvalCriterion, len(contract.Criteria))
		copy(owned.Criteria, contract.Criteria)
		for i := range owned.Criteria {
			criterion := &owned.Criteria[i]
			if criterion.Binding.Ref != nil {
				criterion.Binding.Ref = append(store.BindingRefs{}, criterion.Binding.Ref...)
			}
			if criterion.Baseline != nil {
				baseline := *criterion.Baseline
				criterion.Baseline = &baseline
			}
		}
	}
	return &owned
}

// Charge capacities, not lengths, for mutable Go storage, and string payloads
// plus conservative allocation overhead. Entry overhead includes the model,
// digest, map slot, and LRU element. No document bytes or YAML nodes are kept.
func contractBytes(contract *store.EvalFile, failure string) int {
	size := 256 + len(failure) + 16
	if contract == nil {
		return size
	}
	size += len(contract.Node) + 16 + cap(contract.Criteria)*192
	for _, criterion := range contract.Criteria {
		size += len(criterion.ID) + len(criterion.Statement) + len(criterion.Polarity) + 48
		size += len(criterion.Binding.Type) + len(criterion.Binding.Run) + 32 + cap(criterion.Binding.Ref)*16
		for _, ref := range criterion.Binding.Ref {
			size += len(ref) + 16
		}
		if baseline := criterion.Baseline; baseline != nil {
			size += 256 + len(baseline.Surface) + len(baseline.Artifact) + len(baseline.CapturedAt.Kind) + len(baseline.CapturedAt.Value) + len(baseline.CapturedAt.Path) + 80
		}
	}
	return size
}
