package serve

// These tests pin the console's visit horizon, the one piece of writable state
// the read-only server keeps (visit.go). They drive visitState directly with an
// injected clock so the clamping rule asserts without real time,
// and one integration case runs two Handlers over the same root to pin the
// cross-process handoff the recap relies on.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

const visitManifest = `schema_version: 14
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

// TestVisitMarkFirstVisitReadsZero pins the first mark: with no memory on disk
// the horizon the marked request's own digest reads against is zero (a
// first-ever visit recaps nothing), and the mark creates the cache recording
// now when the request names no through instant.
func TestVisitMarkFirstVisitReadsZero(t *testing.T) {
	root := visitRepo(t)
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	v := &visitState{now: fixedNow(now)}

	prev, err := v.mark(root, time.Time{})
	require.NoError(t, err)
	require.True(t, prev.IsZero(), "a first-ever visit recaps nothing")
	require.True(t, readCacheVisit(t, root).Equal(now), "the mark records now")
}

// TestVisitMarkMovesTheHorizonAtOnce replaces the old burst-hold rule: a mark
// moves the horizon immediately, peek reads the new instant on the very next
// call, and a second mark minutes later moves it again.
func TestVisitMarkMovesTheHorizonAtOnce(t *testing.T) {
	root := visitRepo(t)
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)

	clock := seeded.Add(time.Hour)
	v := &visitState{now: func() time.Time { return clock }}

	prev, err := v.mark(root, time.Time{})
	require.NoError(t, err)
	require.True(t, prev.Equal(seeded), "the marked request's own digest reads the horizon before the move")
	require.True(t, v.peek(root).Equal(clock), "the very next read sees the new horizon, no session gap")

	clock = clock.Add(5 * time.Minute)
	prev, err = v.mark(root, time.Time{})
	require.NoError(t, err)
	require.True(t, prev.Equal(clock.Add(-5*time.Minute)))
	require.True(t, v.peek(root).Equal(clock), "a second mark minutes later moves it again")
}

// TestVisitMarkClampsThrough pins the through rule: the new horizon is the
// instant the on-screen digest was computed, clamped into [horizon, now]; a
// missing instant means now.
func TestVisitMarkClampsThrough(t *testing.T) {
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	now := seeded.Add(time.Hour)
	cases := map[string]struct {
		through time.Time
		want    time.Time
	}{
		"inside the range":   {seeded.Add(20 * time.Minute), seeded.Add(20 * time.Minute)},
		"before the horizon": {seeded.Add(-time.Hour), seeded},
		"in the future":      {now.Add(time.Hour), now},
		"absent":             {time.Time{}, now},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root := visitRepo(t)
			writeLastVisit(root, seeded)
			v := &visitState{now: fixedNow(now)}
			_, err := v.mark(root, tc.through)
			require.NoError(t, err)
			require.True(t, v.peek(root).Equal(tc.want), "got %s", v.peek(root))
		})
	}
}

// TestVisitMarkCorruptCacheDegradesToZero pins the corrupt-memory rule: a
// cache that is not valid JSON is read as no memory (horizon zero) and
// rewritten by the mark so the next read is clean.
func TestVisitMarkCorruptCacheDegradesToZero(t *testing.T) {
	root := visitRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(consoleCachePath(root)), 0o755))
	require.NoError(t, os.WriteFile(consoleCachePath(root), []byte("{not json"), 0o644))

	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	v := &visitState{now: fixedNow(now)}

	prev, err := v.mark(root, time.Time{})
	require.NoError(t, err)
	require.True(t, prev.IsZero(), "a corrupt memory recaps nothing")
	require.True(t, readCacheVisit(t, root).Equal(now), "the mark repairs the cache")
}

// TestVisitMarkReportsAFailedWrite pins that a mark whose cache write fails is
// an error, not a swallowed success: the console's failure sentence depends on
// it.
func TestVisitMarkReportsAFailedWrite(t *testing.T) {
	root := visitRepo(t)
	// A directory where the cache file must go makes the atomic write fail.
	require.NoError(t, os.MkdirAll(consoleCachePath(root), 0o755))
	v := &visitState{now: fixedNow(time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC))}
	_, err := v.mark(root, time.Time{})
	require.Error(t, err)
}

// TestVisitPeekReadsDiskAndOpensNothing: peek answers the recorded last visit
// straight from the cache.
func TestVisitPeekReadsDisk(t *testing.T) {
	root := visitRepo(t)
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)

	v := &visitState{now: fixedNow(seeded.Add(time.Hour))}
	require.True(t, v.peek(root).Equal(seeded), "peek answers the recorded memory")
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
// console's mark as its horizon, so events appended in between surface in the
// second console's recap.
func TestSecondConsolePicksUpTheHorizon(t *testing.T) {
	root := visitRepo(t)
	assets := fstest.MapFS{"index.html": {Data: []byte("x")}}

	t1 := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	first := newMux(root, assets, &visitState{now: fixedNow(t1)}, "test", DefaultAuthority())
	// The first console's one marked request reads a zero horizon (no memory
	// yet) and persists t1.
	rec := recapThrough(t, first, "")
	require.Equal(t, "", rec.Since, "the first-ever visit has no horizon")
	require.Equal(t, 0, rec.Total)

	// Work lands after the first console marked.
	appendVisitEvent(t, root, "EV-000001", "add.epic", t1.Add(time.Hour), []string{"E-000001"})

	t2 := t1.Add(2 * time.Hour)
	second := newMux(root, assets, &visitState{now: fixedNow(t2)}, "test", DefaultAuthority())
	rec = recapUnmarked(t, second)
	require.Equal(t, "2026-07-01T09:00:00Z", rec.Since, "a restart reads the persisted horizon")
	require.Equal(t, 1, rec.Total, "the event appended after the mark surfaces")
}

// TestRecapMarkTakesEffectAtOnceThroughTheMux is the handler-level pin of the
// press: a marked request moves the horizon to its through instant at once,
// the next unmarked visit-window recap and the peek read it with no session
// gap, a second marked request moves it again, and the marked response itself
// is computed against the horizon BEFORE the move.
func TestRecapMarkTakesEffectAtOnceThroughTheMux(t *testing.T) {
	root := visitRepo(t)
	assets := fstest.MapFS{"index.html": {Data: []byte("x")}}
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)

	clock := seeded.Add(time.Hour)
	h := newMux(root, assets, &visitState{now: func() time.Time { return clock }}, "test", DefaultAuthority())

	through := seeded.Add(30 * time.Minute).Format(time.RFC3339)
	marked := recapThrough(t, h, through)
	require.Equal(t, "2026-07-01T09:00:00Z", marked.Since, "the marked response reads the horizon before the move")
	require.Equal(t, clock.Format(time.RFC3339), marked.Through, "the digest carries the instant it was computed")

	require.Equal(t, through, recapUnmarked(t, h).Since, "an unmarked recap reads the new horizon at once")
	require.Equal(t, through, peekSince(t, h), "and so does the peek")

	clock = clock.Add(5 * time.Minute)
	recapThrough(t, h, "")
	require.Equal(t, clock.Format(time.RFC3339), recapUnmarked(t, h).Since, "a second press minutes later moves it again")
}

// TestRecapMarkClampsThroughAtTheHandler pins the clamp and the garbage rule
// through the real query parameter.
func TestRecapMarkClampsThroughAtTheHandler(t *testing.T) {
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	now := seeded.Add(time.Hour)
	for name, tc := range map[string]struct{ through, want string }{
		"earlier than the horizon": {seeded.Add(-time.Hour).Format(time.RFC3339), seeded.Format(time.RFC3339)},
		"in the future":            {now.Add(24 * time.Hour).Format(time.RFC3339), now.Format(time.RFC3339)},
		"garbage":                  {"yesterday-ish", now.Format(time.RFC3339)},
		"missing":                  {"", now.Format(time.RFC3339)},
	} {
		t.Run(name, func(t *testing.T) {
			root := visitRepo(t)
			writeLastVisit(root, seeded)
			h := newMux(root, nil, &visitState{now: fixedNow(now)}, "test", DefaultAuthority())
			recapThrough(t, h, tc.through)
			require.Equal(t, tc.want, recapUnmarked(t, h).Since)
		})
	}
}

// TestRecapUnmarkedNeverMovesOrWrites: reading, with or without a through
// parameter, leaves the cache byte-identical.
func TestRecapUnmarkedNeverMovesOrWrites(t *testing.T) {
	root := visitRepo(t)
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)
	before, err := os.ReadFile(consoleCachePath(root))
	require.NoError(t, err)

	h := newMux(root, nil, &visitState{now: fixedNow(seeded.Add(time.Hour))}, "test", DefaultAuthority())
	getVisit(t, h, "/api/recap?window=visit&through="+url.QueryEscape(seeded.Add(time.Minute).Format(time.RFC3339)), false)
	recapUnmarked(t, h)
	peekSince(t, h)

	after, err := os.ReadFile(consoleCachePath(root))
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// TestRecapMarkedRequestWhoseWriteFailsIsAnError pins D: a press that could
// not persist answers an error status, never a success.
func TestRecapMarkedRequestWhoseWriteFailsIsAnError(t *testing.T) {
	root := visitRepo(t)
	require.NoError(t, os.MkdirAll(consoleCachePath(root), 0o755)) // a directory blocks the file
	srv := httptest.NewServer(newMux(root, nil, &visitState{now: fixedNow(time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC))}, "test", DefaultAuthority()))
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/recap?window=visit", nil)
	require.NoError(t, err)
	req.Header.Set(visitHeader, "1")
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// visitRecap decodes the horizon, instant, and total the visit tests assert.
type visitRecap struct {
	Since   string `json:"since"`
	Through string `json:"through"`
	Total   int    `json:"total"`
}

func getVisit(t *testing.T, h http.Handler, path string, marked bool) []byte {
	t.Helper()
	srv := httptest.NewServer(h)
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	require.NoError(t, err)
	if marked {
		req.Header.Set(visitHeader, "1")
	}
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var raw json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
	return raw
}

// recapThrough serves one press (GET /api/recap carrying the console's visit
// marker, and the through instant when given) against a handler.
func recapThrough(t *testing.T, h http.Handler, through string) visitRecap {
	t.Helper()
	path := "/api/recap?window=visit"
	if through != "" {
		path += "&through=" + url.QueryEscape(through)
	}
	var rec visitRecap
	require.NoError(t, json.Unmarshal(getVisit(t, h, path, true), &rec))
	return rec
}

// recapUnmarked serves one plain read of the visit window.
func recapUnmarked(t *testing.T, h http.Handler) visitRecap {
	t.Helper()
	var rec visitRecap
	require.NoError(t, json.Unmarshal(getVisit(t, h, "/api/recap?window=visit", false), &rec))
	return rec
}

func peekSince(t *testing.T, h http.Handler) string {
	t.Helper()
	var rec visitRecap
	require.NoError(t, json.Unmarshal(getVisit(t, h, "/api/recap/peek", false), &rec))
	return rec.Since
}

// TestRecapMarkKeepsSubsecondPrecision pins the round trip of a press whose
// digest instant falls mid-second: the digest names its instant at full
// precision, the press echoes it, and the persisted horizon is exactly that
// instant, so an event the reader saw (landing earlier in the same second) is
// not new again on the next visit.
func TestRecapMarkKeepsSubsecondPrecision(t *testing.T) {
	root := visitRepo(t)
	assets := fstest.MapFS{"index.html": {Data: []byte("x")}}
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)

	asOf := time.Date(2026, 7, 1, 12, 0, 0, 800_000_000, time.UTC)
	appendVisitEvent(t, root, "EV-000001", "add.epic", asOf.Add(-500*time.Millisecond), []string{"E-000001"})
	h := newMux(root, assets, &visitState{now: fixedNow(asOf)}, "test", DefaultAuthority())

	seen := recapUnmarked(t, h)
	require.Equal(t, 1, seen.Total, "the digest holds the event")
	press := recapThrough(t, h, seen.Through)
	require.Equal(t, 1, press.Total)

	require.True(t, readCacheVisit(t, root).Equal(asOf), "the horizon is the instant the digest was taken")
	require.Equal(t, 0, recapUnmarked(t, h).Total, "an event the reader saw is not new again")
}

// TestVisitHorizonWholeSecondStillParses pins that a horizon persisted before
// sub-second precision reads back unchanged.
func TestVisitHorizonWholeSecondStillParses(t *testing.T) {
	root := visitRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(consoleCachePath(root)), 0o755))
	require.NoError(t, os.WriteFile(consoleCachePath(root), []byte(`{"last_visit":"2026-07-01T09:00:00Z"}`), 0o644))
	v := &visitState{now: fixedNow(time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC))}
	require.True(t, v.peek(root).Equal(time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)))
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
