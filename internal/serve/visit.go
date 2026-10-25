package serve

// The console's only writable state is convenience memory in the gitignored
// .tiller/.cache: a single last-visit timestamp, the horizon the recap answers
// "what changed while you were away" against (ui-surface.md, "The recap"). It
// is not tree state, which is why a read-only console may touch it; the
// prose/state discipline is untouched. The whole read-only server performs
// exactly one kind of disk write, and it happens here, lazily, the first time a
// visit-window recap is served, never at boot.

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// visitSessionGap bounds one visit. A visit is a burst of requests, not a
// single click: reloading the console or clicking between views inside the gap
// is the same visit, so the horizon holds still and the recap does not blank
// out from under a human who is still reading it. Stepping away for longer ends
// the visit; the next request opens a new one whose horizon is where the last
// one left off.
const visitSessionGap = 30 * time.Minute

// consoleCache is the on-disk shape of .tiller/.cache/console.json.
type consoleCache struct {
	LastVisit time.Time `json:"last_visit"`
}

// visitState is the recap's visit horizon, the console's convenience memory in
// .tiller/.cache. It is lazy on purpose: nothing is read or written until the
// first visit-window recap request, so booting the server touches no disk and
// does not consume the window. now is injected so the burst semantics can be
// asserted against a controlled clock; production wires time.Now().UTC.
type visitState struct {
	mu      sync.Mutex
	horizon time.Time
	lastReq time.Time
	loaded  bool
	now     func() time.Time
}

// newVisitState builds the state with the production clock.
func newVisitState() *visitState {
	return &visitState{now: func() time.Time { return time.Now().UTC() }}
}

// mark advances the visit horizon for one visit-window recap request against
// root and returns the horizon that request should recap from. The first call
// of a process reads the last recorded visit from the cache (the zero time when
// it is absent or corrupt, so a first-ever visit recaps nothing rather than
// everything). Later calls hold the horizon still while requests keep arriving
// inside visitSessionGap (one visit is a burst) and advance it to the previous
// burst's last request once a gap has passed (a new visit begins where the old
// one ended). Every call records now as this session's last request and
// persists it as the new last-visit, so the next process, a second console over
// the same root, picks up where this one left off.
func (v *visitState) mark(root string) time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := v.now()
	switch {
	case !v.loaded:
		v.horizon = readLastVisit(root)
		v.loaded = true
	case now.Sub(v.lastReq) > visitSessionGap:
		v.horizon = v.lastReq
	}
	v.lastReq = now
	writeLastVisit(root, now)
	return v.horizon
}

// peek answers the horizon the NEXT visit-window recap would use, without
// advancing it and without writing the cache. It is the read behind Home's
// recap signal: glancing at Home must never spend a recap you have not opened
// (ui-surface.md, "Recap"). It mirrors mark's horizon computation arm for arm
// and deliberately touches none of loaded, horizon, or lastReq, so a peek can
// neither open a visit nor freeze one, and Home and the recap view always
// answer against the same instant.
func (v *visitState) peek(root string) time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()

	switch {
	case !v.loaded:
		return readLastVisit(root)
	case v.now().Sub(v.lastReq) > visitSessionGap:
		return v.lastReq
	default:
		return v.horizon
	}
}

// readLastVisit reads the recorded last visit from the cache, returning the
// zero time when the file is absent or its JSON is corrupt: an unreadable
// memory recaps nothing, and the next write repairs it.
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

// writeLastVisit records now as the last visit, atomically so a reader (this
// process on its next boot, or a second console over the same root) never sees
// a half-written file. A write failure is swallowed: the cache is convenience
// memory, and a console that cannot persist its horizon still serves.
func writeLastVisit(root string, now time.Time) {
	if data, err := json.Marshal(consoleCache{LastVisit: now}); err == nil {
		_ = store.WriteRepositoryFileAtomic(root, filepath.Join(".tiller", ".cache", "console.json"), data)
	}
}

// consoleCachePath is the console's memory file under the gitignored cache.
func consoleCachePath(root string) string {
	return filepath.Join(root, ".tiller", ".cache", "console.json")
}
