package serve_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/stretchr/testify/require"
)

// These tests pin /api/events (plan 2026-07-05, P2): the raw feed newest
// first, the default and explicit limits, and the entry shape with targets and
// file paths always present. The fixture is nodeFixture (node_test.go), whose
// five mutations record events EV-000001..EV-000005 at one fixed instant, so
// the feed's order is the deterministic id tiebreak reversed.

type eventFeedPayload struct {
	Count  int `json:"count"`
	Events []struct {
		ID          string   `json:"id"`
		At          string   `json:"at"`
		Type        string   `json:"type"`
		Contributor string   `json:"contributor"`
		Command     string   `json:"command"`
		Targets     []string `json:"targets"`
		Files       []string `json:"files"`
	} `json:"events"`
}

func eventsServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(serve.EventsHandler(nodeFixture(t)))
	t.Cleanup(srv.Close)
	return srv
}

// TestEventsFeedNewestFirst pins the default read: every event, newest first,
// count matching the returned length, each entry carrying the stable
// timestamp, the actor, the command, its targets, and the touched paths.
func TestEventsFeedNewestFirst(t *testing.T) {
	srv := eventsServer(t)

	var got eventFeedPayload
	require.Equal(t, 200, getJSON(t, srv, "/api/events", &got))

	require.Equal(t, 5, got.Count)
	require.Len(t, got.Events, 5)

	wantTypes := []string{
		"add.question", // EV-000005
		"add.task",     // EV-000004
		"add.feature",  // EV-000003
		"add.feature",  // EV-000002
		"add.epic",     // EV-000001
	}
	for i, e := range got.Events {
		require.Equal(t, wantTypes[i], e.Type, "events[%d]", i)
		require.Equal(t, "2026-07-01T10:00:00Z", e.At)
		require.Equal(t, "jc", e.Contributor)
		require.NotNil(t, e.Targets, "targets is always an array")
		require.NotNil(t, e.Files, "files is always an array")
	}
	require.Equal(t, "EV-000005", got.Events[0].ID)
	require.Equal(t, "EV-000001", got.Events[4].ID)

	// The task add pins the entry shape: its own target, its own touched file.
	added := got.Events[1]
	require.Equal(t, "add.task", added.Type)
	require.Equal(t, []string{"T-000001"}, added.Targets)
	require.Equal(t, []string{
		".tiller/epics/form-engine/features/schema-engine/tasks/parse-the-schema/task.md",
		".tiller/epics/form-engine/features/schema-engine/tasks/parse-the-schema/task.yml",
	}, added.Files)
}

func TestEventFeedUsesContributorAttributionWithoutOwnerAlias(t *testing.T) {
	srv := eventsServer(t)
	var payload struct {
		Events []map[string]any `json:"events"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/events", &payload))
	require.NotEmpty(t, payload.Events)
	for _, event := range payload.Events {
		require.Equal(t, "jc", event["contributor"])
		require.NotContains(t, event, "owner")
	}
}

// TestEventsFeedLimit pins ?limit=N: the newest N, count matching what came
// back.
func TestEventsFeedLimit(t *testing.T) {
	srv := eventsServer(t)

	var got eventFeedPayload
	require.Equal(t, 200, getJSON(t, srv, "/api/events?limit=2", &got))

	require.Equal(t, 2, got.Count)
	require.Len(t, got.Events, 2)
	require.Equal(t, "EV-000005", got.Events[0].ID)
	require.Equal(t, "EV-000004", got.Events[1].ID)
}

// TestEventsFeedLimitFallbacks pins the parameter's edges: garbage and
// non-positive values fall back to the default, and an over-cap ask still
// answers (clamped, invisible at this fixture's size).
func TestEventsFeedLimitFallbacks(t *testing.T) {
	srv := eventsServer(t)

	for _, q := range []string{"?limit=abc", "?limit=0", "?limit=-3", "?limit=999999"} {
		var got eventFeedPayload
		require.Equal(t, 200, getJSON(t, srv, "/api/events"+q, &got), "GET /api/events%s", q)
		require.Equal(t, 5, got.Count, "GET /api/events%s", q)
	}
}

// TestEventsFeedEmptyStream pins the zero case: a repo with no events answers
// an empty array, not null.
func TestEventsFeedEmptyStream(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))
	srv := httptest.NewServer(serve.EventsHandler(root))
	t.Cleanup(srv.Close)

	var got eventFeedPayload
	require.Equal(t, 200, getJSON(t, srv, "/api/events", &got))
	require.Equal(t, 0, got.Count)
	require.NotNil(t, got.Events)
	require.Empty(t, got.Events)
}

// TestEventsFeedBrokenStreamIs500 pins the deliberate contrast the handler's
// comment promises against /api/node: where a broken event file empties a
// node's timeline, the feed itself is the stream, so a stream it cannot read is
// a 500, not a silent empty page.
func TestEventsFeedBrokenStreamIs500(t *testing.T) {
	root := nodeFixture(t)
	broken := filepath.Join(root, ".tiller", "events", "2026", "07", "01", "EV-broken.yml")
	require.NoError(t, os.WriteFile(broken, []byte(":\tnot yaml"), 0o644))

	srv := httptest.NewServer(serve.EventsHandler(root))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Get(srv.URL + "/api/events")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 500, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
}

// TestEventsFeedRefusesWrites pins the read-only posture.
func TestEventsFeedRefusesWrites(t *testing.T) {
	srv := eventsServer(t)

	resp, err := srv.Client().Post(srv.URL+"/api/events", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
}
