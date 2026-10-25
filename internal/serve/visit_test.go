package serve

// These tests pin the console's visit horizon, the one piece of writable state
// the read-only server keeps (visit.go). They drive visitState directly with an
// injected clock so the burst and session semantics assert without real time,
// and one integration case runs two Handlers over the same root to pin the
// cross-process handoff the recap relies on.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

const visitManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Visit Test
  key: visittest
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

func visitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(visitManifest), 0o644))
	return root
}

func readCacheVisit(t *testing.T, root string) time.Time {
	t.Helper()
	data, err := os.ReadFile(consoleCachePath(root))
	require.NoError(t, err)
	var c consoleCache
	require.NoError(t, json.Unmarshal(data, &c))
	return c.LastVisit
}

func fixedNow(at time.Time) func() time.Time { return func() time.Time { return at } }

// TestVisitStateBootWritesNothing pins that construction is side-effect-free:
// building the state and the mux touches no disk, so booting the server never
// consumes the window.
func TestVisitStateBootWritesNothing(t *testing.T) {
	root := visitRepo(t)
	_ = newMux(root, fstest.MapFS{"index.html": {Data: []byte("x")}}, newVisitState(), "test", DefaultAuthority())

	_, err := os.Stat(consoleCachePath(root))
	require.True(t, os.IsNotExist(err), "no request, no cache write")
}

// TestVisitStateFirstVisitReadsZero pins the first mark of a process: with no
// memory on disk the horizon is zero (a first-ever visit recaps nothing), and
// the mark creates the cache recording now.
func TestVisitStateFirstVisitReadsZero(t *testing.T) {
	root := visitRepo(t)
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	v := &visitState{now: fixedNow(now)}

	require.True(t, v.mark(root).IsZero(), "a first-ever visit recaps nothing")
	require.True(t, readCacheVisit(t, root).Equal(now), "the mark records now")
}

// TestVisitStateHoldsHorizonInSession pins the burst rule: a reload inside the
// session gap serves the same horizon, so the recap does not blank out from
// under a human who is still reading it. This is the primitive the topbar's
// manual ↻ composition rests on (api.ts's fetchGeneration bump, hooks.ts's
// useFetch subscription): a bump re-issues whatever is mounted, History's
// default window=visit recap included, and that re-request is exactly this
// case. TestRecapVisitWindowHoldsHorizonAcrossRequestsWithinSessionGap below
// pins the identical rule through the real HTTP handler, the layer a ↻ click
// actually drives.
func TestVisitStateHoldsHorizonInSession(t *testing.T) {
	root := visitRepo(t)
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)

	clock := seeded.Add(time.Hour)
	v := &visitState{now: func() time.Time { return clock }}

	require.True(t, v.mark(root).Equal(seeded), "the first mark reads the seeded memory")
	clock = clock.Add(5 * time.Minute)
	require.True(t, v.mark(root).Equal(seeded), "a reload inside the gap holds the horizon still")
}

// TestVisitStateGapAdvancesHorizon pins the new-visit rule: once more than
// visitSessionGap passes, the next request opens a new visit whose horizon is
// the previous burst's last request.
func TestVisitStateGapAdvancesHorizon(t *testing.T) {
	root := visitRepo(t)
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)

	first := seeded.Add(time.Hour)
	clock := first
	v := &visitState{now: func() time.Time { return clock }}

	require.True(t, v.mark(root).Equal(seeded), "the first mark reads the seeded memory")
	clock = first.Add(visitSessionGap + time.Minute)
	require.True(t, v.mark(root).Equal(first), "a gap advances the horizon to the prior burst's last request")
}

// TestVisitStateCorruptCacheDegradesToZero pins the corrupt-memory rule: a
// cache that is not valid JSON is read as no memory (horizon zero) and rewritten
// so the next visit reads a clean value.
func TestVisitStateCorruptCacheDegradesToZero(t *testing.T) {
	root := visitRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(consoleCachePath(root)), 0o755))
	require.NoError(t, os.WriteFile(consoleCachePath(root), []byte("{not json"), 0o644))

	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	v := &visitState{now: fixedNow(now)}

	require.True(t, v.mark(root).IsZero(), "a corrupt memory recaps nothing")
	require.True(t, readCacheVisit(t, root).Equal(now), "the mark repairs the cache")
}

// TestVisitPeekBeforeMarkReadsDiskAndLeavesTheSessionClosed pins peek's cold
// path: with nothing loaded yet it answers the recorded last visit straight
// from disk and opens no session, so a later mark still reads the memory for
// itself rather than inheriting a horizon a glance at Home established.
func TestVisitPeekBeforeMarkReadsDiskAndLeavesTheSessionClosed(t *testing.T) {
	root := visitRepo(t)
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)

	now := seeded.Add(time.Hour)
	v := &visitState{now: fixedNow(now)}

	require.True(t, v.peek(root).Equal(seeded), "peek answers the recorded memory")
	require.False(t, v.loaded, "a peek opens no visit")
	require.True(t, v.mark(root).Equal(seeded), "the first real mark still reads the memory itself")
}

// TestVisitPeekInsideBurstMatchesMark is the consistency pin behind Home's
// recap signal: inside one visit, peek must answer the same horizon mark
// already returned, not the newer last_visit sitting on disk. Otherwise Home
// would say "nothing new" while the recap page one click away still shows the
// whole digest.
func TestVisitPeekInsideBurstMatchesMark(t *testing.T) {
	root := visitRepo(t)
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)

	clock := seeded.Add(time.Hour)
	v := &visitState{now: func() time.Time { return clock }}

	marked := v.mark(root)
	clock = clock.Add(5 * time.Minute)
	require.True(t, v.peek(root).Equal(marked), "peek answers the horizon the open visit is reading against")
	require.False(t, v.peek(root).Equal(readLastVisit(root)), "and not the newer last_visit the mark wrote")
}

// TestVisitPeekAfterGapReportsTheNextHorizon pins peek's warm path once the
// visit has ended: it answers the horizon the NEXT mark would open on, the
// previous burst's last request, so the signal and the page agree about the
// instant even across the session boundary.
func TestVisitPeekAfterGapReportsTheNextHorizon(t *testing.T) {
	root := visitRepo(t)
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)

	first := seeded.Add(time.Hour)
	clock := first
	v := &visitState{now: func() time.Time { return clock }}

	require.True(t, v.mark(root).Equal(seeded), "the first mark reads the seeded memory")
	clock = first.Add(visitSessionGap + time.Minute)
	require.True(t, v.peek(root).Equal(first), "peek reports the horizon the next visit would open on")
	require.True(t, v.mark(root).Equal(first), "and the next mark opens on exactly that")
}

// TestVisitPeekWritesNothing is the whole point of the second read path:
// looking at Home must never spend a recap the human has not opened. Five
// peeks leave the cache byte-identical, and a peek over an absent cache does
// not create one.
func TestVisitPeekWritesNothing(t *testing.T) {
	root := visitRepo(t)
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	v := &visitState{now: fixedNow(now)}

	for range 5 {
		require.True(t, v.peek(root).IsZero(), "no memory, no horizon")
	}
	_, err := os.Stat(consoleCachePath(root))
	require.True(t, os.IsNotExist(err), "a peek never creates the cache")

	seeded := now.Add(-time.Hour)
	writeLastVisit(root, seeded)
	before, err := os.ReadFile(consoleCachePath(root))
	require.NoError(t, err)
	for range 5 {
		require.True(t, v.peek(root).Equal(seeded))
	}
	after, err := os.ReadFile(consoleCachePath(root))
	require.NoError(t, err)
	require.Equal(t, before, after, "a peek never rewrites the cache")
}

// TestVisitPeekCorruptCacheDegradesToZero pins that peek inherits
// readLastVisit's degradation: an unreadable memory recaps nothing, so a fresh
// or corrupt checkout shows "nothing new" on Home rather than the whole stream.
func TestVisitPeekCorruptCacheDegradesToZero(t *testing.T) {
	root := visitRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(consoleCachePath(root)), 0o755))
	require.NoError(t, os.WriteFile(consoleCachePath(root), []byte("{not json"), 0o644))

	v := &visitState{now: fixedNow(time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC))}
	require.True(t, v.peek(root).IsZero(), "a corrupt memory peeks as no memory")
}

// TestSecondConsolePicksUpTheHorizon pins the cross-process handoff through the
// mux: a second Handler over the same root (a fresh process) reads the first
// console's last request as its horizon, so events appended in between surface
// in the second console's recap.
func TestSecondConsolePicksUpTheHorizon(t *testing.T) {
	root := visitRepo(t)
	assets := fstest.MapFS{"index.html": {Data: []byte("x")}}

	t1 := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	first := newMux(root, assets, &visitState{now: fixedNow(t1)}, "test", DefaultAuthority())
	// The first console's one request establishes a zero horizon (no memory yet)
	// and persists t1 as the last visit.
	rec := recapThrough(t, first)
	require.Equal(t, "", rec.Since, "the first-ever visit has no horizon")
	require.Equal(t, 0, rec.Total)

	// Work lands after the first console looked.
	appendVisitEvent(t, root, "EV-000001", "add.epic", t1.Add(time.Hour), []string{"E-000001"})

	t2 := t1.Add(2 * time.Hour)
	second := newMux(root, assets, &visitState{now: fixedNow(t2)}, "test", DefaultAuthority())
	rec = recapThrough(t, second)
	require.Equal(t, "2026-07-01T09:00:00Z", rec.Since, "the second console's horizon is the first's last request")
	require.Equal(t, 1, rec.Total, "the event appended after the first visit surfaces")
}

// TestRecapVisitWindowHoldsHorizonAcrossRequestsWithinSessionGap pins the ↻
// composition at the layer it actually runs at: the topbar's manual refresh
// (api.ts's bumpFetchGeneration) re-issues whatever is mounted in place, so a
// click while History sits on its default window re-requests GET
// /api/recap?window=visit a second time. That re-request is safe by the
// controller's settled visit semantics (ui-surface.md's recap model): inside
// the server's visitSessionGap a re-request is part of the same visit burst
// and does not advance the horizon, exactly like reloading the tab, so the
// human never loses the digest they were reading mid-read. This drives two
// consecutive GETs through the real mux (the shape a browser's ↻ produces)
// rather than calling visitState.mark directly, which
// TestVisitStateHoldsHorizonInSession above already covers at the primitive
// level.
func TestRecapVisitWindowHoldsHorizonAcrossRequestsWithinSessionGap(t *testing.T) {
	root := visitRepo(t)
	assets := fstest.MapFS{"index.html": {Data: []byte("x")}}

	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)

	clock := seeded.Add(time.Hour)
	h := newMux(root, assets, &visitState{now: func() time.Time { return clock }}, "test", DefaultAuthority())

	first := recapThrough(t, h)
	require.Equal(t, "2026-07-01T09:00:00Z", first.Since, "the first request of this process opens on the seeded horizon")

	// A ↻ click a few minutes later, well inside visitSessionGap: the same
	// visit burst as the first request, so the horizon must not move.
	clock = clock.Add(5 * time.Minute)
	second := recapThrough(t, h)
	require.Equal(t, first.Since, second.Since, "a ↻ reissue of the mounted visit-window recap inside the session gap answers the same horizon as the first read, never a newer one")
}

// visitRecap decodes the horizon and total the visit tests assert.
type visitRecap struct {
	Since string `json:"since"`
	Total int    `json:"total"`
}

// recapThrough serves one History visit (GET /api/recap carrying the console's
// visit marker) against a handler and decodes it.
func recapThrough(t *testing.T, h http.Handler) visitRecap {
	t.Helper()
	srv := httptest.NewServer(h)
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/recap", nil)
	require.NoError(t, err)
	req.Header.Set(visitHeader, "1")
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var rec visitRecap
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rec))
	return rec
}

func appendVisitEvent(t *testing.T, root, id, typ string, at time.Time, targets []string) {
	t.Helper()
	tree, err := store.Load(root)
	require.NoError(t, err)
	eventPath, data, err := events.File(tree, events.Event{
		ID: id, Type: typ, At: at, Contributor: "jc", Command: typ, Targets: targets,
	})
	require.NoError(t, err)
	require.NoError(t, tree.Commit("test event", []store.PlannedWrite{
		store.PlanWrite(eventPath, store.AbsentSnapshot(), data),
	}))
}
