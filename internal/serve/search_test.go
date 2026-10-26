package serve_test

import (
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/search"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin GET /api/search (ui-surface.md, "Explore"): the console's
// search answers the same grouped payload `tiller search --json` carries in
// Data, refuses an empty query or unknown kind with a 400, and
// holds the pass-1 read-only posture. The fixture keeps node ids fixed while
// minting every immutable event onto a distinct stable path.

const searchTestManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Search Serve Test
  key: searchserve
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

type searchClock struct{ t time.Time }

func (c searchClock) Now() time.Time { return c.t }

// searchFixtureAt lays an owned repo with one epic, one feature, and one bug
// (fixed ids) plus a design doc, and returns its root and a server over
// SearchHandler alone, the way the handler is meant to be tested: directly,
// route wiring being the integrator's.
func searchFixtureAt(t *testing.T) (string, *httptest.Server) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(searchTestManifest), 0o644))

	eventID := 0
	deps := mutate.Deps{
		Contributor: "jc",
		Clock:       searchClock{t: time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)},
		Mint: func(prefix string) string {
			if prefix == "EV" {
				eventID++
				return fmt.Sprintf("%s-%06d", prefix, eventID)
			}
			return prefix + "-aaaaaa"
		},
	}
	tree, err := store.Load(root)
	require.NoError(t, err)
	_, err = mutate.AddEpic(tree, deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	tree, err = store.Load(root)
	require.NoError(t, err)
	_, err = mutate.AddFeature(tree, deps, mutate.FeatureInput{
		Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create",
	})
	require.NoError(t, err)
	bugDir := filepath.Join(root, store.ShelfRelDir(store.NounBug, "blank-on-reload"))
	require.NoError(t, os.MkdirAll(bugDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(bugDir, store.ShelfStateFile(store.NounBug)),
		[]byte("id: B-aaaaaa\nkey: bugs/blank-on-reload\ntitle: Blank on reload\nstatus: proposed\nblocked: false\nproposed_by: jc\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(bugDir, store.ShelfProseFile(store.NounBug)),
		[]byte("## Reproduction\n\nThe page must reload after saving.\n"),
		0o644,
	))

	designDir := filepath.Join(root, ".tiller", "design")
	require.NoError(t, os.MkdirAll(designDir, 0o755))
	doc := "# Upload notes\n\nThe uploader parses certification PDFs.\n"
	require.NoError(t, os.WriteFile(filepath.Join(designDir, "01-notes.md"), []byte(doc), 0o644))

	srv := httptest.NewServer(serve.SearchHandler(root))
	t.Cleanup(srv.Close)
	return root, srv
}

// searchFixtureFlooded is the fixture plus `docs` design docs that each
// match, enough to blow past any page the cap tests ask for.
func searchFixtureFlooded(t *testing.T, docs int) *httptest.Server {
	t.Helper()
	root, srv := searchFixtureAt(t)
	for i := 1; i <= docs; i++ {
		path := filepath.Join(root, ".tiller", "design", fmt.Sprintf("flood-%03d.md", i))
		require.NoError(t, os.WriteFile(path, []byte("flood-me\n"), 0o644))
	}
	return srv
}

// searchFixture is searchFixtureAt for the tests that only need the server.
func searchFixture(t *testing.T) *httptest.Server {
	t.Helper()
	_, srv := searchFixtureAt(t)
	return srv
}

// TestSearchEndpointAnswersTheSearch pins the 200 payload byte for byte: the
// same grouped result `tiller search --json` carries in Data.
func TestSearchEndpointAnswersTheSearch(t *testing.T) {
	srv := searchFixture(t)

	resp, err := srv.Client().Get(srv.URL + "/api/search?q=" + url.QueryEscape("Certification form loading"))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	want := `{
  "query": "Certification form loading",
  "count": 1,
  "truncated": false,
  "kinds": [
    {
      "kind": "feature",
      "count": 1
    }
  ],
  "nodes": [
    {
      "noun": "feature",
      "id": "F-aaaaaa",
      "key": "form-engine/certification-form-loading",
      "title": "Certification form loading",
      "status": "draft",
      "ref": "form-engine/certification-form-loading (F-aaaaaa)",
      "match": "title",
      "count": 1,
      "hits": [
        {
          "path": ".tiller/epics/form-engine/features/certification-form-loading/feature.md",
          "line": 1,
          "snippet": "# Certification form loading"
        }
      ]
    }
  ],
  "documents": []
}
`
	require.Equal(t, want, string(body))
}

// TestSearchEndpointFindsShelfNodesByIDAndProse pins the shelf answers over
// HTTP: the bug's id names it, and its title ranks it.
func TestSearchEndpointFindsShelfNodesByIDAndProse(t *testing.T) {
	srv := searchFixture(t)
	for _, tc := range []struct{ query, match string }{
		{"B-aaaaaa", "id"},
		{"reload", "title"},
	} {
		var got search.Result
		require.Equal(t, 200, getJSON(t, srv, "/api/search?q="+tc.query, &got))
		require.Len(t, got.Nodes, 1, tc.query)
		require.Equal(t, "B-aaaaaa", got.Nodes[0].ID)
		require.Equal(t, "bug", got.Nodes[0].Noun)
		require.Equal(t, tc.match, got.Nodes[0].Match)
	}
}

// TestSearchEndpointEmptyResultIsAnAnswer pins the empty answer: 200 with
// count 0 and empty arrays, never null.
func TestSearchEndpointEmptyResultIsAnAnswer(t *testing.T) {
	srv := searchFixture(t)

	resp, err := srv.Client().Get(srv.URL + "/api/search?q=zz-nothing-here")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"query":"zz-nothing-here","count":0,"truncated":false,"kinds":[],"nodes":[],"documents":[]}`, string(body))
}

// TestSearchEndpointRefusesMisuse pins the 400s: a missing or all-whitespace
// q, and a kind search does not know, are the HTTP twins of the CLI's misuse
// exit.
func TestSearchEndpointRefusesMisuse(t *testing.T) {
	srv := searchFixture(t)

	for _, target := range []string{"/api/search", "/api/search?q=", "/api/search?q=%20%20", "/api/search?q=form&kind=widget"} {
		resp, err := srv.Client().Get(srv.URL + target)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, 400, resp.StatusCode, "GET %s", target)
		require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	}
}

// TestSearchEndpointKindNarrowsTheResults pins ?kind=: the results keep one
// kind while kinds still tallies them all.
func TestSearchEndpointKindNarrowsTheResults(t *testing.T) {
	srv := searchFixture(t)

	var got search.Result
	require.Equal(t, 200, getJSON(t, srv, "/api/search?q=form&kind=feature", &got))
	require.Equal(t, "feature", got.Kind)
	require.NotEmpty(t, got.Nodes)
	for _, n := range got.Nodes {
		require.Equal(t, "feature", n.Noun)
	}
	require.Greater(t, len(got.Kinds), 1, "kinds still counts the other kinds")
}

// TestSearchEndpointLimitBoundsThePage pins ?limit=: the page holds at most
// that many results and count stays the true total. An unparsable or absent
// limit falls back to MaxResults, the same clamp idiom the event feed uses.
func TestSearchEndpointLimitBoundsThePage(t *testing.T) {
	srv := searchFixtureFlooded(t, search.MaxResults+50)

	for _, tc := range []struct {
		query   string
		results int
	}{
		{"?q=flood-me&limit=3", 3},
		{"?q=flood-me&limit=99999", search.MaxResults},
		{"?q=flood-me&limit=nonsense", search.MaxResults},
		{"?q=flood-me", search.MaxResults},
	} {
		var got search.Result
		require.Equal(t, 200, getJSON(t, srv, "/api/search"+tc.query, &got), tc.query)
		require.Len(t, got.Documents, tc.results, tc.query)
		require.Equal(t, search.MaxResults+50, got.Count, "count stays the true total whatever the page: %s", tc.query)
		require.True(t, got.Truncated, tc.query)
	}
}

// TestSearchEndpointRefusesUndestinationableCorpus pins the HTTP projection of
// the core corpus refusal: Explore must answer 500 rather than return a hit
// whose path looks local but dereferences through a symlink outside the repo.
func TestSearchEndpointRefusesUndestinationableCorpus(t *testing.T) {
	root, srv := searchFixtureAt(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(outside, []byte("# outside-only-token\n"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".tiller", "design", "02-outside.md")))

	resp, err := srv.Client().Get(srv.URL + "/api/search?q=outside-only-token")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 500, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
}

// TestSearchEndpointIsReadOnly pins the pass-1 posture on this handler:
// non-GET methods refuse with 405 and the Allow header.
func TestSearchEndpointIsReadOnly(t *testing.T) {
	srv := searchFixture(t)

	resp, err := srv.Client().Post(srv.URL+"/api/search?q=x", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
	require.Equal(t, "GET, HEAD", resp.Header.Get("Allow"))
}
