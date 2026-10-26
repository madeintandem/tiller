package serve_test

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// These tests pin GET /api/ls (ui-surface.md, Explore's filter mode): the
// CLI's ls projection made fetchable, one row per node with the human reference
// carried explicitly (the embed's whole point, since the CLI keeps Ref out of
// its own JSON). They ride the shared nodeFixture (node_test.go), whose fixed
// ids make the payload pinnable, and cover the singular and plural nouns, one
// round-trip per filter axis, and the misuse convention: a bad noun is a 400,
// while an unresolved parent is a 200 naming the stale axis in unresolved.

func lsServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(serve.ListHandler(nodeFixture(t)))
	t.Cleanup(srv.Close)
	return srv
}

// listPayloadShape decodes enough of the envelope to assert counts and the
// per-row ref the console renders.
type listPayloadShape struct {
	Noun  string `json:"noun"`
	Count int    `json:"count"`
	Nodes []struct {
		ID  string `json:"id"`
		Ref string `json:"ref"`
	} `json:"nodes"`
}

// TestListEpicsPayload pins the exact body for a single-row noun: the row
// carries id and key structurally and the composed ref once, the field the
// wrapper exists to add.
func TestListEpicsPayload(t *testing.T) {
	t.Setenv("TILLER_CONTRIBUTOR", "")
	srv := lsServer(t)

	resp, err := srv.Client().Get(srv.URL + "/api/ls?noun=epics")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.JSONEq(t, `{
	  "noun": "epic",
	  "count": 1,
	  "nodes": [
	    {
	      "id": "E-000001",
	      "key": "form-engine",
	      "title": "Form Engine",
	      "status": "draft",
	      "activity": "idle",
	      "contributors": [],
	      "ref": "form-engine (E-000001)"
	    }
	  ],
	  "facets": [
	    {"axis":"status","options":[{"value":"draft","label":"draft","count":1}]},
	    {"axis":"contributor","options":[]}
	  ],
	  "axes": {
	    "epic":["status","contributor"],
	    "feature":["status","contributor","epic"],
	    "task":["status","owner","epic","feature"],
	    "question":["status","owner"],
	    "amendment":["status"],
	    "bug":["status","owner"],
	    "chore":["status","owner"],
	    "tweak":["status","owner"]
	  },
	  "unresolved": {}
	}`, string(body))
}

// TestListSingularAndPlural pins that both noun spellings resolve to the same
// class: the console can say "feature" or "features" and get the same answer.
func TestListSingularAndPlural(t *testing.T) {
	srv := lsServer(t)

	for _, noun := range []string{"feature", "features"} {
		var got listPayloadShape
		require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun="+noun, &got), "noun=%s", noun)
		require.Equal(t, "feature", got.Noun, "the payload always names the class singular")
		require.Equal(t, 2, got.Count, "noun=%s lists both features", noun)
	}
}

// TestListShelfNouns pins the existing Explore contract: bugs and chores use
// the same singular-and-plural list endpoint as the work tree nouns.
func TestListShelfNouns(t *testing.T) {
	srv := lsServer(t)

	for _, tc := range []struct {
		query string
		noun  string
	}{
		{"bug", "bug"},
		{"bugs", "bug"},
		{"chore", "chore"},
		{"chores", "chore"},
		{"tweak", "tweak"},
		{"tweaks", "tweak"},
	} {
		var got listPayloadShape
		require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun="+tc.query, &got), "noun=%s", tc.query)
		require.Equal(t, tc.noun, got.Noun)
		require.Empty(t, got.Nodes)
	}
}

// TestListFilterAxes pins one round-trip per narrowing axis, each a distinct
// slice of the fixture so a dropped filter would change the count.
func TestListFilterAxes(t *testing.T) {
	root := nodeFixture(t)
	claimTaskInFixture(t, root, "T-000001", "jc")
	srv := httptest.NewServer(serve.ListHandler(root))
	t.Cleanup(srv.Close)

	cases := []struct {
		name  string
		query string
		count int
	}{
		{"status", "/api/ls?noun=features&status=draft", 2},
		{"status-miss", "/api/ls?noun=features&status=active", 0},
		{"owner", "/api/ls?noun=tasks&owner=jc", 1},
		{"ancestor-owner", "/api/ls?noun=features&owner=jc", 0},
		{"unstarted-contributor", "/api/ls?noun=features&contributor=jc", 0},
		{"unfinished", "/api/ls?noun=tasks&status=unfinished&owner=jc", 1},
		{"feature", "/api/ls?noun=tasks&feature=form-engine/schema-engine", 1},
		{"epic", "/api/ls?noun=features&epic=form-engine", 2},
	}
	for _, c := range cases {
		var got listPayloadShape
		require.Equal(t, 200, getJSON(t, srv, c.query, &got), c.name)
		require.Equal(t, c.count, got.Count, "%s: %s", c.name, c.query)
		require.Len(t, got.Nodes, c.count, c.name)
	}
}

func TestListContributorTracksStartedHistorySeparatelyFromOwner(t *testing.T) {
	root := nextFixture(t)
	srv := httptest.NewServer(serve.ListHandler(root))
	t.Cleanup(srv.Close)
	var got listPayloadShape
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=features&contributor=jc", &got))
	require.Empty(t, got.Nodes, "reserved ready tasks do not establish contributors")
	setTaskPR(t, root, "schema-engine", "transcribe-the-schema", 11)
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=features&contributor=jc", &got))
	require.Equal(t, 1, got.Count)
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=features&owner=jc", &got))
	require.Empty(t, got.Nodes)
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=tasks&owner=jc&status=unfinished", &got))
	require.Equal(t, 1, got.Count, "in-review assignments remain unfinished")
}

// TestListUnknownNounIs400 pins the misuse convention: an unaddressable noun is
// a client error, not a 404.
func TestListUnknownNounIs400(t *testing.T) {
	srv := lsServer(t)

	var got struct {
		Error string `json:"error"`
	}
	require.Equal(t, 400, getJSON(t, srv, "/api/ls?noun=widget", &got))
	require.Equal(t, 400, getJSON(t, srv, "/api/ls", &got), "a missing noun is a 400 too")
}

// Unresolved parents return no rows even when another axis can resolve.
func TestListUnresolvedParentFilterIsReported(t *testing.T) {
	t.Setenv("TILLER_CONTRIBUTOR", "")
	srv := lsServer(t)
	for _, tc := range []struct {
		path       string
		unresolved map[string]string
	}{
		{"/api/ls?noun=tasks&feature=no-such-feature", map[string]string{"feature": "no-such-feature"}},
		{"/api/ls?noun=features&epic=no-such-epic", map[string]string{"epic": "no-such-epic"}},
		{"/api/ls?noun=tasks&epic=missing&feature=form-engine/schema-engine", map[string]string{"epic": "missing"}},
		{"/api/ls?noun=tasks&epic=form-engine&feature=missing", map[string]string{"feature": "missing"}},
		{"/api/ls?noun=tasks&epic=missing&feature=other", map[string]string{"epic": "missing", "feature": "other"}},
		{"/api/ls?noun=tasks&feature=E-000001", map[string]string{"feature": "E-000001"}},
		{"/api/ls?noun=epics&feature=missing", map[string]string{"feature": "missing"}},
	} {
		var got struct {
			Count      int               `json:"count"`
			Nodes      []json.RawMessage `json:"nodes"`
			Unresolved map[string]string `json:"unresolved"`
		}
		require.Equal(t, 200, getJSON(t, srv, tc.path, &got))
		require.Zero(t, got.Count)
		require.NotNil(t, got.Nodes)
		require.Empty(t, got.Nodes)
		require.Equal(t, tc.unresolved, got.Unresolved)
	}
}

// TestListEmptyResultIsAnArray pins the zero case: a filter that matches
// nothing answers count 0 with nodes as an empty array, never null, so a
// caller parses one shape.
func TestListEmptyResultIsAnArray(t *testing.T) {
	srv := lsServer(t)

	resp, err := srv.Client().Get(srv.URL + "/api/ls?noun=features&owner=nobody")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var envelope struct {
		Noun       string            `json:"noun"`
		Count      int               `json:"count"`
		Unresolved map[string]string `json:"unresolved"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.Equal(t, "feature", envelope.Noun)
	require.Zero(t, envelope.Count)
	require.NotNil(t, envelope.Unresolved)
	require.Empty(t, envelope.Unresolved)

	// The array is present, not null: decode confirms the shape.
	var got listPayloadShape
	require.NoError(t, json.Unmarshal(body, &got))
	require.NotNil(t, got.Nodes)
	require.Empty(t, got.Nodes)
}

// TestListRefusesWrites pins the pass-1 read-only posture on this route.
func TestListRefusesWrites(t *testing.T) {
	srv := lsServer(t)

	resp, err := srv.Client().Post(srv.URL+"/api/ls?noun=features", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
	require.Equal(t, "GET, HEAD", resp.Header.Get("Allow"))
}

func TestListFacetContractViewerAndOldParentURLs(t *testing.T) {
	t.Setenv("TILLER_CONTRIBUTOR", "")
	root := nodeFixture(t)
	claimTaskInFixture(t, root, "T-000001", "jc")
	srv := httptest.NewServer(serve.ListHandler(root))
	t.Cleanup(srv.Close)
	var got struct {
		Count      int                 `json:"count"`
		Facets     []views.Facet       `json:"facets"`
		Axes       map[string][]string `json:"axes"`
		Unresolved map[string]string   `json:"unresolved"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=tasks&owner=jc&epic=form-engine&feature=form-engine/schema-engine", &got))
	require.Equal(t, 1, got.Count)
	require.Equal(t, views.FilterAxesByNoun(), got.Axes)
	require.NotNil(t, got.Unresolved)
	require.Empty(t, got.Unresolved)
	require.Len(t, got.Facets, 4)
	require.Equal(t, []string{"status", "owner", "epic", "feature"}, []string{got.Facets[0].Axis, got.Facets[1].Axis, got.Facets[2].Axis, got.Facets[3].Axis})
	require.Equal(t, []views.FacetOption{{Value: "jc", Label: "JC Grubbs", Count: 1, Selected: true}}, got.Facets[1].Options)
	require.True(t, got.Facets[2].Options[0].Selected)
	require.Equal(t, "E-000001", got.Facets[2].Options[0].Value)
	require.True(t, got.Facets[3].Options[0].Selected)
	require.Equal(t, "F-000001", got.Facets[3].Options[0].Value)
	require.Equal(t, "E-000001", got.Facets[3].Options[0].Parent)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=tasks", &got))
	require.True(t, got.Facets[1].Options[0].You)
}

func TestListUnresolvedParentCannotMatchRawParentIdentity(t *testing.T) {
	t.Setenv("TILLER_CONTRIBUTOR", "")
	root := nodeFixture(t)
	path := filepath.Join(root, ".tiller", "epics", "form-engine", "epic.yml")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var epic store.Epic
	require.NoError(t, yaml.Unmarshal(body, &epic))
	epic.Key = nil
	body, err = yaml.Marshal(epic)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, body, 0o644))
	tree, err := store.Load(root)
	require.NoError(t, err)
	f := views.ListFilters{Epic: "form-engine"}
	require.Equal(t, map[string]string{"epic": "form-engine"}, views.ResolveParentFiltersLeniently(tree, &f))
	require.True(t, views.Matches(tree, tree.Features["F-000001"], f), "the raw folder parent can match despite failing resolution")
	srv := httptest.NewServer(serve.ListHandler(root))
	t.Cleanup(srv.Close)
	var got struct {
		Count      int               `json:"count"`
		Nodes      []json.RawMessage `json:"nodes"`
		Unresolved map[string]string `json:"unresolved"`
		Facets     []views.Facet     `json:"facets"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=features&epic=form-engine", &got))
	require.Zero(t, got.Count)
	require.NotNil(t, got.Nodes)
	require.Empty(t, got.Nodes)
	require.Equal(t, map[string]string{"epic": "form-engine"}, got.Unresolved)
	require.Equal(t, []views.Facet{
		{Axis: "status", Options: []views.FacetOption{}},
		{Axis: "contributor", Options: []views.FacetOption{}},
		{Axis: "epic", Options: []views.FacetOption{{Value: "form-engine", Label: "form-engine", Selected: true}}},
	}, got.Facets, "an unresolved raw identity never contributes counts")
	got.Unresolved = nil
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=features", &got))
	require.Equal(t, 2, got.Count)
	require.Empty(t, got.Unresolved)
	require.Equal(t, []views.FacetOption{}, got.Facets[2].Options, "unresolvable parent links are not offered as picks")
}

// TestListFeatureColumnFollowsWorkInFlight pins the console-only column on a
// feature row, the field Home's funnel counts so it agrees with Build: a
// ready feature reads ready until a task under it is in flight, and in_review
// once its only task is in review, while its stored status stays ready.
func TestListFeatureColumnFollowsWorkInFlight(t *testing.T) {
	root := nextFixture(t)
	srv := httptest.NewServer(serve.ListHandler(root))
	t.Cleanup(srv.Close)
	type row struct {
		Key    string `json:"key"`
		Status string `json:"status"`
		Column string `json:"column"`
	}
	var got struct {
		Nodes []row `json:"nodes"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=features", &got))
	require.Contains(t, got.Nodes, row{Key: "form-engine/schema-engine", Status: "ready", Column: "ready"})
	setTaskPR(t, root, "schema-engine", "transcribe-the-schema", 11)
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=features", &got))
	require.Contains(t, got.Nodes, row{Key: "form-engine/schema-engine", Status: "ready", Column: "in_review"})
}
