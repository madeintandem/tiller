package serve_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin the watching console's P1 surface (plan 2026-07-05): the
// read API renders the same views the CLI's --json renders, fresh from the
// working tree per request, and the embedded app is served with an SPA
// fallback so client-side routes deep-link.

const serveManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Serve Test
  key: servetest
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

// serveFixture lays an owned repo with one epic and one feature and returns a
// handler over it plus the repo root.
func serveFixture(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))

	tree, err := store.Load(root)
	require.NoError(t, err)
	deps := mutate.Deps{Contributor: "jc"}
	_, err = mutate.AddEpic(tree, deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	tree, err = store.Load(root)
	require.NoError(t, err)
	_, err = mutate.AddFeature(tree, deps, mutate.FeatureInput{Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "create"})
	require.NoError(t, err)

	assets := fstest.MapFS{
		"index.html":        {Data: []byte("<!doctype html><title>Tiller</title>")},
		"assets/app.js":     {Data: []byte("// app")},
		"assets/app.css":    {Data: []byte("body{}")},
		"fonts/inter.woff2": {Data: []byte("font")},
	}
	srv := httptest.NewServer(serve.Handler(root, assets, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)
	return srv, root
}

func getJSON(t *testing.T, srv *httptest.Server, path string, out any) int {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + path)
	require.NoError(t, err)
	defer resp.Body.Close()
	if out != nil && resp.StatusCode == 200 {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
	}
	return resp.StatusCode
}

// TestStatusEndpointRendersTheSummary pins /api/status: the same
// views.Summary payload the CLI's status --json carries.
func TestStatusEndpointRendersTheSummary(t *testing.T) {
	srv, _ := serveFixture(t)

	var got struct {
		Project struct {
			Name string `json:"name"`
			Key  string `json:"key"`
			Kind string `json:"kind"`
		} `json:"project"`
		Counts struct {
			Epics    int `json:"epics"`
			Features int `json:"features"`
		} `json:"counts"`
		Contributors []struct {
			ID string `json:"id"`
		} `json:"contributors"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/status", &got))
	require.Equal(t, "servetest", got.Project.Key)
	require.Equal(t, "greenfield", got.Project.Kind)
	require.Equal(t, 1, got.Counts.Epics)
	require.Equal(t, 1, got.Counts.Features)
	require.Len(t, got.Contributors, 1)
}

func TestVersionRemainsReadableWhenProjectCannotLoad(t *testing.T) {
	srv := httptest.NewServer(serve.Handler(t.TempDir(), nil, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)

	require.Equal(t, http.StatusInternalServerError, getJSON(t, srv, "/api/status", nil))
	var got struct {
		Version string `json:"version"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/version", &got))
	require.Equal(t, "test", got.Version)
}

// TestBoardEndpointRendersTheStreams pins /api/board: live per-contributor streams
// with references precomputed.
func TestBoardEndpointRendersTheStreams(t *testing.T) {
	srv, _ := serveFixture(t)

	var got struct {
		Contributors []struct {
			Contributor struct {
				ID string `json:"id"`
			} `json:"contributor"`
			Active        []map[string]any `json:"active"`
			Blocked       []map[string]any `json:"blocked"`
			GatesInReview []map[string]any `json:"gates_in_review"`
			OpenQuestions []map[string]any `json:"open_questions"`
		} `json:"contributors"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/board", &got))
	require.Len(t, got.Contributors, 1)
	require.Equal(t, "jc", got.Contributors[0].Contributor.ID)
	// Draft nodes are pre-work, so the fixture's active lens is empty but
	// present: every list renders as an array, never null.
	require.NotNil(t, got.Contributors[0].Active)
	require.NotNil(t, got.Contributors[0].OpenQuestions)
}

func TestRosterPayloadsUseContributorFieldsWithoutOwnerAliases(t *testing.T) {
	srv, _ := serveFixture(t)
	for _, endpoint := range []string{"/api/status", "/api/board"} {
		t.Run(endpoint, func(t *testing.T) {
			var payload map[string]any
			require.Equal(t, http.StatusOK, getJSON(t, srv, endpoint, &payload))
			require.NotContains(t, payload, "owners")
			roster, ok := payload["contributors"].([]any)
			require.True(t, ok)
			require.Len(t, roster, 1)
			person := roster[0].(map[string]any)
			if endpoint == "/api/board" {
				require.NotContains(t, person, "owner")
				person = person["contributor"].(map[string]any)
			}
			require.Equal(t, "jc", person["id"])
		})
	}
}

// TestBoardReadsFreshPerRequest pins liveness's foundation: the handler loads
// the working tree per request, so a mutation between requests appears without
// a restart.
func TestBoardReadsFreshPerRequest(t *testing.T) {
	srv, root := serveFixture(t)

	var before struct {
		Counts struct {
			Epics int `json:"epics"`
		} `json:"counts"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/status", &before))
	require.Equal(t, 1, before.Counts.Epics)

	tree, err := store.Load(root)
	require.NoError(t, err)
	_, err = mutate.AddEpic(tree, mutate.Deps{Contributor: "jc"}, mutate.EpicInput{Key: "payments", Title: "Payments"})
	require.NoError(t, err)

	var after struct {
		Counts struct {
			Epics int `json:"epics"`
		} `json:"counts"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/status", &after))
	require.Equal(t, 2, after.Counts.Epics, "the picture is live local files, not a cache")
}

// TestUnknownAPIRouteIs404JSON pins the API error shape: unknown /api paths
// answer JSON, not the SPA fallback.
func TestUnknownAPIRouteIs404JSON(t *testing.T) {
	srv, _ := serveFixture(t)

	resp, err := srv.Client().Get(srv.URL + "/api/nonsense")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 404, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
}

// TestStaticAndSPAFallback pins the app serving: real asset paths serve their
// bytes, and any non-API route serves index.html so client-side routes
// deep-link.
func TestStaticAndSPAFallback(t *testing.T) {
	srv, _ := serveFixture(t)

	resp, err := srv.Client().Get(srv.URL + "/assets/app.js")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	for _, route := range []string{"/", "/explore", "/node/feature/F-abcdef", "/file/.tiller/design/01-overview.md"} {
		resp, err := srv.Client().Get(srv.URL + route)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, 200, resp.StatusCode, "SPA fallback serves the app at %s", route)
		require.Contains(t, resp.Header.Get("Content-Type"), "text/html")
	}
}

// TestCacheHeadersByPathClass pins the stale-app bug's fix: with no
// Cache-Control at all, a browser's own caching heuristic once kept serving a
// prior session's index.html and hashed bundle straight through a binary
// upgrade. index.html revalidates every time (no-cache, not no-store, so a
// conditional GET can still 304); Vite's content-hashed assets/* are
// immutable for a year, since a changed file is a new URL; fonts/* is stable
// but unhashed, so it gets a short cache instead of "forever".
func TestCacheHeadersByPathClass(t *testing.T) {
	srv, _ := serveFixture(t)

	cases := []struct {
		path string
		want string
	}{
		{"/", "no-cache"},
		{"/assets/app.js", "public, max-age=31536000, immutable"},
		{"/fonts/inter.woff2", "public, max-age=86400"},
	}
	for _, c := range cases {
		resp, err := srv.Client().Get(srv.URL + c.path)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, 200, resp.StatusCode, "GET %s", c.path)
		require.Equal(t, c.want, resp.Header.Get("Cache-Control"), "GET %s", c.path)
	}
}

// TestAPICacheHeaderIsNoStore pins the read API's own posture: every /api/*
// answer is a fresh read of the live working tree, never something safe for a
// browser to reuse, so it opts out of caching outright.
func TestAPICacheHeaderIsNoStore(t *testing.T) {
	srv, _ := serveFixture(t)

	resp, err := srv.Client().Get(srv.URL + "/api/status")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
}

// TestEveryRouteAnswersThroughHandler is the mux-level smoke: every wired route
// answers 200 through Handler() with a shape-distinguishing key, so a typo'd
// route string (a handler wired to the wrong path, or dropped) fails a test.
// The node and search routes are addressed against the fixture's real graph.
func TestEveryRouteAnswersThroughHandler(t *testing.T) {
	srv, root := serveFixture(t)

	cases := []struct {
		path string
		key  string
	}{
		{"/api/status", "project"},
		{"/api/board", "contributors"},
		{"/api/tree", "nodes"},
		{"/api/ls?noun=features", "noun"},
		{"/api/node/feature/form-engine/schema-engine", "node"},
		{"/api/events", "events"},
		{"/api/queue", "entries"},
		{"/api/recap", "groups"},
		// Go's ServeMux exact-matches /api/recap, so without its own
		// HandleFunc line the peek falls to the /api/ catch-all's 404.
		{"/api/recap/peek", "total"},
		{"/api/search?q=Form", "nodes"},
		{"/api/prose?" + url.Values{
			"path": {".tiller/epics/form-engine/features/schema-engine/feature.md"},
			"line": {"1"},
		}.Encode(), "line_text"},
		// serveFixture's manifest carries no drift: block, so every marker
		// reads disabled and the sweep finds nothing to compare; "compared"
		// is the one field DriftReportView never omits (json:"compared" with
		// no omitempty), so it is present on a clean, all-disabled run too.
		{"/api/drift", "compared"},
	}
	for _, c := range cases {
		var got map[string]json.RawMessage
		require.Equal(t, 200, getJSON(t, srv, c.path, &got), "GET %s", c.path)
		require.Contains(t, got, c.key, "GET %s carries %q", c.path, c.key)
	}

	// The evidence route is the read API's one non-JSON answer, so it cannot
	// ride the key table above: pin its registration by round-tripping a raw
	// text body instead. A dropped or typo'd HandleFunc line answers the
	// catch-all's JSON 404 here and fails the assertion. The endpoint serves
	// only recorded refs, so the feature's yml records this one as its build
	// gate's evidence, the holder real gate runs write.
	const evidenceRef = "artifacts/tiller/gates/build-sha256-d938b60e00c3.txt"
	featureYML := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "schema-engine", "feature.yml")
	recorded, err := os.ReadFile(featureYML)
	require.NoError(t, err)
	recorded = append(recorded, []byte(gateEvidenceYAML(evidenceRef))...)
	require.NoError(t, os.WriteFile(featureYML, recorded, 0o644))
	full := filepath.Join(root, filepath.FromSlash(evidenceRef))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte("ok\n"), 0o644))

	resp, err := srv.Client().Get(srv.URL + "/api/evidence?ref=" + evidenceRef)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "GET /api/evidence")
	require.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "ok\n", string(body))
}

// TestTreeEndpointPinsThePayload pins /api/tree byte for byte over the
// deterministic node fixture: the whole work tree, epics and features in
// DependencyOrder (issue #27) and tasks in id order, each with its file
// footprint and prose drill path, the same model `tiller tree --json` renders.
// schema-engine leads form-loading here because form-loading's fixture setup
// records --depends-on form-engine/schema-engine (node_test.go's nodeFixture):
// before Task A1 this golden pinned the two in plain key order, which put the
// dependent (form-loading) ahead of the dependency it names, coincidentally
// matching alphabetical order alone.
func TestTreeEndpointPinsThePayload(t *testing.T) {
	srv := httptest.NewServer(serve.Handler(nodeFixture(t), nil, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Get(srv.URL + "/api/tree")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.JSONEq(t, `{
	  "project": "servetest",
	  "nodes": [
	    {
	      "noun": "epic", "id": "E-000001", "key": "form-engine", "title": "Form Engine",
	      "status": "draft", "contributors": [], "activity": "idle",
	      "path": ".tiller/epics/form-engine/epic.md",
	      "files": [".tiller/epics/form-engine/epic.md", ".tiller/epics/form-engine/epic.yml"],
	      "children": [
	        {
	          "noun": "feature", "id": "F-000001", "key": "form-engine/schema-engine", "title": "Schema engine",
	          "status": "draft", "contributors": [],
	          "path": ".tiller/epics/form-engine/features/schema-engine/feature.md",
	          "files": [
	            ".tiller/epics/form-engine/features/schema-engine/feature.md",
	            ".tiller/epics/form-engine/features/schema-engine/feature.yml"
	          ],
	          "children": [
	            {
	              "noun": "task", "id": "T-000001",
	              "key": "form-engine/schema-engine/parse-the-schema", "title": "Parse the schema",
	              "status": "proposed",
	              "path": ".tiller/epics/form-engine/features/schema-engine/tasks/parse-the-schema/task.md",
	              "files": [
	                ".tiller/epics/form-engine/features/schema-engine/tasks/parse-the-schema/task.md",
	                ".tiller/epics/form-engine/features/schema-engine/tasks/parse-the-schema/task.yml"
	              ],
	              "children": []
	            }
	          ]
	        },
	        {
	          "noun": "feature", "id": "F-000002", "key": "form-engine/form-loading", "title": "Form loading",
	          "status": "draft", "contributors": [],
	          "path": ".tiller/epics/form-engine/features/form-loading/feature.md",
	          "files": [
	            ".tiller/epics/form-engine/features/form-loading/feature.md",
	            ".tiller/epics/form-engine/features/form-loading/feature.yml"
	          ],
	          "children": []
	        }
	      ]
	    }
	  ]
	}`, string(body))
}

// TestWritesAreRefused pins the P1 posture: the console is read-only, so
// non-GET methods refuse across the API.
func TestWritesAreRefused(t *testing.T) {
	srv, _ := serveFixture(t)

	resp, err := srv.Client().Post(srv.URL+"/api/status", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode, "pass 1 has no write endpoints")
}
