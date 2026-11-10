package serve

// The console's only writable state is convenience memory in the gitignored
// .tiller/.cache: a single last-visit timestamp, the horizon the recap answers
// "what changed while you were away" against (ui-surface.md, "The recap"). It
// is not tree state, which is why a read-only console may touch it; the
// prose/state discipline is untouched. The whole read-only server performs
// exactly one kind of disk write, and it happens here, only when the reader
// presses Mark caught up (a marked recap request), never at boot and never on
// a read.

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// consoleCache is the on-disk shape of .tiller/.cache/console.json.
type consoleCache struct {
	LastVisit time.Time `json:"last_visit"`
}

// visitState is the recap's visit horizon, the console's convenience memory in
// .tiller/.cache. The cache file is the single source of truth: every read
// goes to it, so a restart, a second console over the same root, and the next
// request in this process all see the same instant with no in-process copy to
// drift. Nothing is read or written until a request needs it, so booting the
// server touches no disk. now is injected so the clamping rule can be asserted
// against a controlled clock; production wires time.Now().UTC.
//
// There is no session gap. Nothing advances the horizon on its own any more:
// only a marked request does (mark), at once, so there is no burst of
// requests to hold still.
type visitState struct {
	mu  sync.Mutex
	now func() time.Time
}

// newVisitState builds the state with the production clock.
func newVisitState() *visitState {
	return &visitState{now: func() time.Time { return time.Now().UTC() }}
}

// mark moves the visit horizon for one marked visit-window recap request
// against root and returns the horizon as it was BEFORE the move, which is the
// horizon that request's own digest is computed against. The new horizon is
// through, the instant the reader's on-screen digest was computed, clamped into
// [previous horizon, now]: the horizon never moves backwards, and a client
// cannot claim a future instant. A zero through (absent or unparseable in the
// request) means now. The new horizon is persisted atomically; a write that
// fails is returned, so the caller can refuse to answer as a success.
func (v *visitState) mark(root string, through time.Time) (time.Time, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	prev := readLastVisit(root)
	now := v.now()
	target := through
	if target.IsZero() || target.After(now) {
		target = now
	}
	if target.Before(prev) {
		target = prev
	}
	if err := writeLastVisit(root, target); err != nil {
		return prev, err
	}
	return prev, nil
}

// peek answers the current horizon, the one an unmarked visit-window recap
// and Home's recap signal read against, without writing the cache. It is the
// read behind Home's recap signal: glancing at Home must never spend a recap
// you have not opened (ui-surface.md, "Recap"), and it answers the same
// instant the recap view reads, so the signal and the view always agree.
func (v *visitState) peek(root string) time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	return readLastVisit(root)
}

// readLastVisit reads the recorded last visit from the cache, returning the
// zero time when the file is absent or its JSON is corrupt: an unreadable
// memory recaps nothing, and the next mark repairs it.
func readLastVisit(root string) time.Time {
	data, err := store.ReadRepositoryFile(root, filepath.Join(".tiller", ".cache", "console.json"))
	if err != nil {
		return time.Time{}
	}
	var c consoleCache
	if err := json.Unmarshal(data, &c); err != nil {
		return time.Time{}
	}
	return c.LastVisit
}

// writeLastVisit records at as the last visit, atomically so a reader (this
// process on its next request, or a second console over the same root) never
// sees a half-written file. A failure is returned: a marked request that could
// not persist must not answer as a success.
func writeLastVisit(root string, at time.Time) error {
	data, err := json.Marshal(consoleCache{LastVisit: at})
	if err != nil {
		return err
	}
	return store.WriteRepositoryFileAtomic(root, filepath.Join(".tiller", ".cache", "console.json"), data)
}

// consoleCachePath is the console's memory file under the gitignored cache.
func consoleCachePath(root string) string {
	return filepath.Join(root, ".tiller", ".cache", "console.json")
}
