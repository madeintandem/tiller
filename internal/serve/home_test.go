package serve_test

// These tests pin the Home view's two endpoints (plan 2026-07-05): /api/queue
// renders views.QueueOf fresh per request, and /api/recap renders
// views.RecapOf with the visit horizon read from the console's cache memory and
// the 24h/7d windows overriding it. The queue handler is exercised directly;
// the recap goes through the full serve.Handler so the visit memory and the
// reader resolution are wired the way production wires them. The fixture events
// are stamped by a fixed clock so payloads pin.

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

const homeManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Home Test
  key: hometest
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

// homeFixedNow is the instant every fixture mutation is stamped with.
var homeFixedNow = time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)

type homeClock struct{ t time.Time }

func (c homeClock) Now() time.Time { return c.t }

// homeMint mints sequential per-prefix ids so the fixture's refs and commands
// are pinnable in payload assertions.
func homeMint() func(string) string {
	counts := map[string]int{}
	return func(prefix string) string {
		i := counts[prefix]
		counts[prefix]++
		return prefix + "-" + strings.Repeat(string(rune('a'+i)), 6)
	}
}

// homeFixture lays a repo whose queue has one entry (a specced feature) and
// whose event stream holds the three mutation events that produced it, all
// stamped at homeFixedNow.
func homeFixture(t *testing.T) (string, mutate.Deps) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(homeManifest), 0o644))
	deps := mutate.Deps{Contributor: "jc", Clock: homeClock{t: homeFixedNow}, Mint: homeMint()}

	load := func() *store.Tree {
		tree, err := store.Load(root)
		require.NoError(t, err)
		return tree
	}
	_, err := mutate.AddEpic(load(), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(), deps, mutate.FeatureInput{Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "create"})
	require.NoError(t, err)

	dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "schema-engine")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Schema engine\n\nReal spec prose for the home fixture.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: It works\n    polarity: introduce\n    binding: { type: test, ref: spec/works_spec.rb }\n---\n\n# Acceptance\n"), 0o644))

	// The explosion is part of the stage-2 output `spec feature` validates
	// (cli-surface.md, "Specifying a feature"), so the fixture writes one task
	// with a real implementation spec before asserting the stage.
	_, err = mutate.AddTask(load(), deps, mutate.TaskInput{
		Key: "form-engine/schema-engine/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, store.TaskDirName, "transcribe-the-schema", store.TaskProseFile),
		[]byte("# Transcribe the schema\n\n## Approach\n\nTranscribe it.\n"), 0o644))

	tree := load()
	feat, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounFeature, Raw: "form-engine/schema-engine"})
	require.NoError(t, err)
	_, err = mutate.Transition(tree, deps, feat, mutate.TransitionInput{Verb: "spec"})
	require.NoError(t, err)

	return root, deps
}

// homeAppendEvent writes one synthetic event file into the fixture's stream.
func homeAppendEvent(t *testing.T, root, id, typ, command string, at time.Time, targets []string, after map[string]any) {
	t.Helper()
	tree, err := store.Load(root)
	require.NoError(t, err)
	eventPath, data, err := events.File(tree, events.Event{ID: id, Type: typ, At: at, Contributor: "jc", Command: command, Targets: targets, After: after})
	require.NoError(t, err)
	require.NoError(t, tree.Commit("test event", []store.PlannedWrite{
		store.PlanWrite(eventPath, store.AbsentSnapshot(), data),
	}))
}

// homeGetJSON GETs a path from the handler under test and decodes the body.
func homeGetJSON(t *testing.T, srv *httptest.Server, path string, out any) int {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + path)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	if out != nil && resp.StatusCode == 200 {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
	}
	return resp.StatusCode
}

// TestQueueHandlerPinsThePayload pins GET /api/queue: the views.QueueOf shape
// with the draft epic's approval first (the approve_epic group leads the queue)
// and the specced feature's copy-command entry after it.
func TestQueueHandlerPinsThePayload(t *testing.T) {
	root, _ := homeFixture(t)
	srv := httptest.NewServer(serve.QueueHandler(root))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Get(srv.URL + "/api/queue")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	var raw json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))

	want := `{
	  "entries": [
	    {"kind":"approve_epic","noun":"epic","id":"E-aaaaaa","key":"form-engine",
	     "ref":"form-engine (E-aaaaaa)","prose":"Form Engine",
	     "command":"tiller approve epic form-engine"},
	    {"kind":"approve_feature","noun":"feature","id":"F-aaaaaa","key":"form-engine/schema-engine",
	     "ref":"form-engine/schema-engine (F-aaaaaa)","prose":"Schema engine",
	     "command":"tiller approve feature form-engine/schema-engine"}
	  ]
	}`
	require.JSONEq(t, want, string(raw))
}

// TestQueueHandlerReadsFreshPerRequest pins the liveness foundation: a
// mutation between requests appears without a restart.
func TestQueueHandlerReadsFreshPerRequest(t *testing.T) {
	root, deps := homeFixture(t)
	srv := httptest.NewServer(serve.QueueHandler(root))
	t.Cleanup(srv.Close)

	var before struct {
		Entries []map[string]any `json:"entries"`
	}
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/queue", &before))
	require.Len(t, before.Entries, 2, "the draft epic and the specced feature")

	tree, err := store.Load(root)
	require.NoError(t, err)
	_, err = mutate.AddQuestion(tree, deps, mutate.QuestionInput{Title: "Rounding policy", Statement: "Which rounding applies?"})
	require.NoError(t, err)

	var after struct {
		Entries []map[string]any `json:"entries"`
	}
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/queue", &after))
	require.Len(t, after.Entries, 3, "the new open question queues without a restart")
	require.Equal(t, "answer_question", after.Entries[2]["kind"])
}

// TestQueueHandlerRefusesWrites pins the pass-1 posture on the queue route.
func TestQueueHandlerRefusesWrites(t *testing.T) {
	root, _ := homeFixture(t)
	srv := httptest.NewServer(serve.QueueHandler(root))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Post(srv.URL+"/api/queue", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
	require.Equal(t, "GET, HEAD", resp.Header.Get("Allow"))
}

// TestRecapPeekCountsWithoutConsumingTheVisit pins the read behind Home's
// recap signal: it answers the same counts the visit-window recap would, and
// five of them leave the visit memory byte-identical. Glancing at Home must
// never spend a recap the human has not opened, which is the whole reason the
// route exists rather than Home refetching /api/recap.
func TestRecapPeekCountsWithoutConsumingTheVisit(t *testing.T) {
	root, _ := homeFixture(t)
	seedVisit(t, root, homeFixedNow.Add(-time.Hour))
	before, err := os.ReadFile(filepath.Join(root, ".tiller", ".cache", "console.json"))
	require.NoError(t, err)

	srv := homeRecapServer(t, root)

	for range 5 {
		var got struct {
			Since  string `json:"since"`
			Reader string `json:"reader"`
			Total  int    `json:"total"`
			Asks   int    `json:"asks"`
		}
		require.Equal(t, 200, homeGetJSON(t, srv, "/api/recap/peek", &got))
		require.Equal(t, "2026-07-01T09:00:00Z", got.Since, "the peek reports the horizon, unmoved")
		require.Equal(t, "", got.Reader, "the checkout has no identity, so no reader")
		require.Equal(t, 4, got.Total, "add.epic, add.feature, add.task, spec.feature")
		require.Equal(t, 1, got.Asks, "the specced feature is the one gate arrival")
	}

	after, err := os.ReadFile(filepath.Join(root, ".tiller", ".cache", "console.json"))
	require.NoError(t, err)
	require.Equal(t, before, after, "a peek never advances the visit")
}

// TestRecapPeekAgreesWithTheRecap is the anti-drift pin: the signal's total and
// the digest's total come from one derivation (views.RecapOf) over one horizon,
// so Home can never claim a number the page then contradicts. The peek runs
// first precisely because it must not be the thing that moved the horizon the
// recap then reads.
func TestRecapPeekAgreesWithTheRecap(t *testing.T) {
	root, _ := homeFixture(t)
	seedVisit(t, root, homeFixedNow.Add(-time.Hour))
	srv := homeRecapServer(t, root)

	var peek struct {
		Since string `json:"since"`
		Total int    `json:"total"`
		Asks  int    `json:"asks"`
	}
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/recap/peek", &peek))

	var full struct {
		Since  string `json:"since"`
		Total  int    `json:"total"`
		Groups []struct {
			Entries []struct {
				Ask bool `json:"ask"`
			} `json:"entries"`
		} `json:"groups"`
	}
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/recap", &full))

	asks := 0
	for _, g := range full.Groups {
		for _, e := range g.Entries {
			if e.Ask {
				asks++
			}
		}
	}
	require.Equal(t, full.Since, peek.Since, "one horizon behind both surfaces")
	require.Equal(t, full.Total, peek.Total, "one count behind both surfaces")
	require.Equal(t, asks, peek.Asks)
}

// TestRecapPeekRefusesWrites pins the pass-1 posture on the peek route.
func TestRecapPeekRefusesWrites(t *testing.T) {
	root, _ := homeFixture(t)
	srv := homeRecapServer(t, root)

	resp, err := srv.Client().Post(srv.URL+"/api/recap/peek", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
	require.Equal(t, "GET, HEAD", resp.Header.Get("Allow"))
}

// TestRecapPeekBrokenStreamKeepsTheVisit mirrors
// TestRecapHandlerBrokenStreamKeepsTheVisit's ordering on the peek: a stream
// that cannot be read is a 500 and the visit memory is untouched. The peek
// never writes at all, so this pins the shape of the failure rather than a
// consumption, and it pins that the peek reads the stream before the horizon.
func TestRecapPeekBrokenStreamKeepsTheVisit(t *testing.T) {
	root, _ := homeFixture(t)
	seeded := homeFixedNow.Add(-time.Hour)
	seedVisit(t, root, seeded)
	dir := filepath.Join(root, ".tiller", "events", "2026", "07", "01")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "EV-broken.yml"), []byte(":\tnot yaml"), 0o644))

	srv := homeRecapServer(t, root)
	resp, err := srv.Client().Get(srv.URL + "/api/recap/peek")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 500, resp.StatusCode)

	data, err := os.ReadFile(filepath.Join(root, ".tiller", ".cache", "console.json"))
	require.NoError(t, err)
	var cache struct {
		LastVisit time.Time `json:"last_visit"`
	}
	require.NoError(t, json.Unmarshal(data, &cache))
	require.True(t, cache.LastVisit.Equal(seeded), "a failed peek leaves the visit exactly where it was")
}

// homeRecapServer wraps the fixture root in the full serve.Handler, the way
// production wires the recap: the visit horizon comes from the console cache on
// disk (see seedVisit) and the reader from the checkout's identity, not an
// injected func. Assets are a stub index.html the /api routes never reach.
func homeRecapServer(t *testing.T, root string) *httptest.Server {
	t.Helper()
	assets := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Tiller</title>")}}
	srv := httptest.NewServer(serve.Handler(root, assets, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)
	return srv
}

// seedVisit plants a recorded last visit in the console cache, the horizon the
// first visit-window recap reads. It is how a test stages "the human last
// looked at this instant" without an injected clock.
func seedVisit(t *testing.T, root string, at time.Time) {
	t.Helper()
	dir := filepath.Join(root, ".tiller", ".cache")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := json.Marshal(struct {
		LastVisit time.Time `json:"last_visit"`
	}{LastVisit: at})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "console.json"), data, 0o644))
}

// TestRecapHandlerVisitWindow pins the default window: the horizon comes
// from the seeded visit memory, the fixture's three mutation events plus a
// synthesized claim event all land in one epic group, and the spec ask is
// flagged. claim.feature renders like any other event (task 1's review,
// carried into task 4): it is the newest of the five, so it leads the
// newest-first order.
func TestRecapHandlerVisitWindow(t *testing.T) {
	root, _ := homeFixture(t)
	homeAppendEvent(t, root, "EV-claim1", "claim.feature", "claim feature F-aaaaaa",
		homeFixedNow.Add(30*time.Minute), []string{"F-aaaaaa"}, nil)
	seedVisit(t, root, homeFixedNow.Add(-time.Hour))

	srv := homeRecapServer(t, root)

	var got struct {
		Since  string `json:"since"`
		Window string `json:"window"`
		Reader string `json:"reader"`
		Total  int    `json:"total"`
		Groups []struct {
			Epic *struct {
				Key string `json:"key"`
				ID  string `json:"id"`
				Ref string `json:"ref"`
			} `json:"epic"`
			Entries []struct {
				Type string `json:"type"`
				Ask  bool   `json:"ask"`
			} `json:"entries"`
		} `json:"groups"`
	}
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/recap", &got))
	require.Equal(t, "visit", got.Window)
	require.Equal(t, "", got.Reader, "the checkout has no identity, so no reader")
	require.Equal(t, "2026-07-01T09:00:00Z", got.Since)
	require.Equal(t, 5, got.Total, "add.epic, add.feature, add.task, spec.feature, claim.feature")
	require.Len(t, got.Groups, 1)
	require.NotNil(t, got.Groups[0].Epic)
	require.Equal(t, "form-engine (E-aaaaaa)", got.Groups[0].Epic.Ref)
	types := make([]string, 0, 5)
	for _, e := range got.Groups[0].Entries {
		types = append(types, e.Type)
	}
	require.Equal(t, []string{"claim.feature", "spec.feature", "add.task", "add.feature", "add.epic"}, types,
		"entries run newest first, claim.feature 30 minutes ahead of the rest")
	require.False(t, got.Groups[0].Entries[0].Ask, "a claim is routine, not a gate arrival")
	require.True(t, got.Groups[0].Entries[1].Ask, "the specced feature is an ask for a reader we cannot identify")
}

// Ancestor gate arrivals are shared asks even for an unrelated identified reader.
func TestRecapHandlerKeepsAncestorAsksShared(t *testing.T) {
	root, _ := homeFixture(t)
	seedVisit(t, root, homeFixedNow.Add(-time.Hour))
	t.Setenv("TILLER_CONTRIBUTOR", "sam")

	srv := homeRecapServer(t, root)

	var got struct {
		Reader string `json:"reader"`
		Groups []struct {
			Entries []struct {
				Type string `json:"type"`
				Ask  bool   `json:"ask"`
			} `json:"entries"`
		} `json:"groups"`
	}
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/recap", &got))
	require.Equal(t, "sam", got.Reader, "the reader echoes the resolved owner")
	require.Len(t, got.Groups, 1)
	require.Equal(t, "spec.feature", got.Groups[0].Entries[0].Type)
	require.True(t, got.Groups[0].Entries[0].Ask,
		"feature plan approval is a shared team ask, not an implementation claim")
}

// TestRecapHandlerWindowOverrides pins 24h and 7d: the horizon becomes now
// minus the window regardless of the visit memory, and the window echoes. The
// repo is bare (manifest only) with events stamped relative to the real now,
// so the expected counts cannot drift with the wall clock. The seeded visit
// memory would show everything; the overrides must ignore it.
func TestRecapHandlerWindowOverrides(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(homeManifest), 0o644))
	now := time.Now().UTC()
	homeAppendEvent(t, root, "EV-fresh1", "start.feature", "start feature form-engine/schema-engine",
		now.Add(-time.Hour), []string{"F-aaaaaa"}, map[string]any{"status": "active"})
	homeAppendEvent(t, root, "EV-mid001", "submit.task", "submit task T-aaaaaa",
		now.Add(-3*24*time.Hour), []string{"T-aaaaaa"}, map[string]any{"status": "in_review"})
	homeAppendEvent(t, root, "EV-old001", "resume.task", "resume task T-aaaaaa",
		now.Add(-8*24*time.Hour), []string{"T-aaaaaa"}, map[string]any{"status": "in_progress"})
	seedVisit(t, root, homeFixedNow.Add(-time.Hour))

	srv := homeRecapServer(t, root)

	var day struct {
		Window string `json:"window"`
		Total  int    `json:"total"`
	}
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/recap?window=24h", &day))
	require.Equal(t, "24h", day.Window)
	require.Equal(t, 1, day.Total, "only the hour-old event is inside 24h")

	var week struct {
		Window string `json:"window"`
		Total  int    `json:"total"`
	}
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/recap?window=7d", &week))
	require.Equal(t, "7d", week.Window)
	require.Equal(t, 2, week.Total, "the three-day-old event joins inside 7d; the eight-day-old stays out")
}

// TestRecapHandlerZeroSinceIsEmpty pins the first-visit rule end to end: with
// no visit memory on disk, the horizon is zero and the recap is empty, never
// everything.
func TestRecapHandlerZeroSinceIsEmpty(t *testing.T) {
	root, _ := homeFixture(t)
	srv := homeRecapServer(t, root)

	resp, err := srv.Client().Get(srv.URL + "/api/recap")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	var raw json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
	require.JSONEq(t, `{"since":"","window":"visit","reader":"","total":0,"groups":[]}`, string(raw))
}

// TestRecapHandlerRefusesUnknownWindow pins the window validation: an unknown
// window is a 400 with the JSON error shape, not a silent default.
func TestRecapHandlerRefusesUnknownWindow(t *testing.T) {
	root, _ := homeFixture(t)
	srv := homeRecapServer(t, root)

	var got struct {
		Error string `json:"error"`
	}
	require.Equal(t, 400, homeGetJSON(t, srv, "/api/recap?window=1y", &got))
}

// TestRecapHandlerRefusesWrites pins the pass-1 posture on the recap route.
func TestRecapHandlerRefusesWrites(t *testing.T) {
	root, _ := homeFixture(t)
	srv := homeRecapServer(t, root)

	resp, err := srv.Client().Post(srv.URL+"/api/recap", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
	require.Equal(t, "GET, HEAD", resp.Header.Get("Allow"))
}

// TestRecapHandlerBrokenStreamKeepsTheVisit pins the marking order: the digest
// is the stream, so a stream the handler cannot read answers 500 and leaves
// the visit memory untouched. A failed recap must not consume the window it
// never answered.
func TestRecapHandlerBrokenStreamKeepsTheVisit(t *testing.T) {
	root, _ := homeFixture(t)
	seeded := homeFixedNow.Add(-time.Hour)
	seedVisit(t, root, seeded)
	dir := filepath.Join(root, ".tiller", "events", "2026", "07", "01")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "EV-broken.yml"), []byte(":\tnot yaml"), 0o644))

	srv := homeRecapServer(t, root)
	resp, err := srv.Client().Get(srv.URL + "/api/recap")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 500, resp.StatusCode)

	data, err := os.ReadFile(filepath.Join(root, ".tiller", ".cache", "console.json"))
	require.NoError(t, err)
	var cache struct {
		LastVisit time.Time `json:"last_visit"`
	}
	require.NoError(t, json.Unmarshal(data, &cache))
	require.True(t, cache.LastVisit.Equal(seeded), "a 500 recap must not advance the visit")
}
