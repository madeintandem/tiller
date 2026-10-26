package serve_test

// This file pins GET /api/drift: the exact
// views.DriftReportOf projection `tiller drift` itself renders, served fresh
// per request over the real read API, so the CLI and the console cannot
// disagree about what a sweep found. TestEveryRouteAnswersThroughHandler
// (serve_test.go) already smoke-tests that the route is wired at all; the
// two tests here cover the handler's two outcomes: an induced candidate
// riding through, and the one error drift.Run can return.

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// driftServeManifestTemplate carries a drift.checks block a caller fills in,
// mirroring internal/cli/drift_test.go's own driftManifestTemplate: an
// absent drift: block reads as every check disabled (drift.go's own
// enabledSet doc comment), so a test that wants a specific sweep scope has
// to say so explicitly.
const driftServeManifestTemplate = `schema_version: 12
schema_epoch: initial
project:
  name: Drift Serve Test
  key: driftserve
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
drift:
  checks: [%s]
`

// driftServeAssets is the minimal embedded-app stand-in serve.Handler needs;
// these tests never touch the SPA half of the handler.
var driftServeAssets = fstest.MapFS{
	"index.html": {Data: []byte("<!doctype html><title>Tiller</title>")},
}

// driftServeRoot lays a plain, non-git working tree whose manifest enables
// exactly checks, and returns the loaded tree. missing_surface needs no git
// at all: drift.Run's own marker switch calls sweepMissingSurface(t, &rep)
// with no rev argument, unlike active_basis or answered_question, which is
// what keeps this fixture a plain temp dir rather than the heavier real-git
// setup those two markers need (internal/cli/drift_test.go's own
// initDriftGitRepo).
func driftServeRoot(t *testing.T, checks string) *store.Tree {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	manifest := fmt.Sprintf(driftServeManifestTemplate, checks)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(manifest), 0o644))

	tree, err := store.Load(root)
	require.NoError(t, err)
	return tree
}

// TestDriftEndpointReturnsInducedCandidate is the happy-path handler test
// the Step 1 asks for: a project with one induced candidate
// returns it. The induced candidate is missing_surface's own vanish rule (a
// feature's tiller link --surface path that no longer exists in the working
// tree), the lightest-weight candidate to construct since it needs no git
// history and no spec/approve lifecycle, only a recorded surface path.
func TestDriftEndpointReturnsInducedCandidate(t *testing.T) {
	tree := driftServeRoot(t, "missing_surface")
	deps := mutate.Deps{Contributor: "jc"}

	_, err := mutate.AddEpic(tree, deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	tree, err = store.Load(tree.Root)
	require.NoError(t, err)
	_, err = mutate.AddFeature(tree, deps, mutate.FeatureInput{
		Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "modify",
	})
	require.NoError(t, err)
	tree, err = store.Load(tree.Root)
	require.NoError(t, err)
	require.Len(t, tree.Features, 1)
	var feat *store.Feature
	for _, f := range tree.Features {
		feat = f
	}
	require.NoError(t, os.MkdirAll(filepath.Join(tree.Root, "app/models/vanished"), 0o755))
	_, err = mutate.SetSurface(tree, deps, feat, []string{"app/models/vanished"})
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(tree.Root, "app/models/vanished")))

	srv := httptest.NewServer(serve.Handler(tree.Root, driftServeAssets, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)

	var got struct {
		Candidates map[string][]struct {
			Marker string   `json:"marker"`
			Noun   string   `json:"noun"`
			ID     string   `json:"id"`
			Key    string   `json:"key"`
			Detail string   `json:"detail"`
			Files  []string `json:"files"`
			Since  *struct {
				Kind string `json:"kind"`
			} `json:"since"`
		} `json:"candidates"`
		Compared int      `json:"compared"`
		Disabled []string `json:"disabled"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/drift", &got))

	rows, ok := got.Candidates["missing_surface"]
	require.True(t, ok, "the induced candidate rides under its own marker key")
	require.Len(t, rows, 1)
	require.Equal(t, "feature", rows[0].Noun)
	require.Equal(t, feat.NodeID(), rows[0].ID)
	require.Equal(t, "form-engine/schema-engine", rows[0].Key)
	require.Equal(t, []string{"app/models/vanished"}, rows[0].Files)
	// missing_surface never sets Since (drift.go's own reportMissingSurface
	// doc comment: "there is nothing honest to date a candidate by here").
	require.Nil(t, rows[0].Since)
	require.Equal(t, 1, got.Compared)
	require.Contains(t, got.Disabled, "active_basis", "every other marker rides the footer as disabled")
}

// TestDriftEndpointRejectsUnknownCheck pins resolution 1's misuse mapping:
// drift.Run's one error, an unknown drift.checks entry, is returned to
// api() unwrapped (serve.go's own comment at the route), so it rides the
// wrapper's existing default path rather than a bespoke arm: the same 500
// JSON error shape every other uncaught projection error already gets
// (serve.go's api() doc comment, "Any other error is a 500").
func TestDriftEndpointRejectsUnknownCheck(t *testing.T) {
	tree := driftServeRoot(t, "not_a_real_marker")

	srv := httptest.NewServer(serve.Handler(tree.Root, driftServeAssets, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Get(srv.URL + "/api/drift")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 500, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var got struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Contains(t, got.Error, "unknown check")
	require.Contains(t, got.Error, "not_a_real_marker")
}
