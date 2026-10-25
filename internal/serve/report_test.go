package serve_test

// These tests pin GET /api/report: the views.ReportOf payload served through
// the full serve.Handler, fresh per request, read-only. homeFixture lays no
// .git directory, so every test here runs outside a git repo; the off-branch
// notice logic itself is pinned at the views level (TestReportOffBranchNotice).

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// TestReportHandlerServesTheReport approves the fixture's epic so its feature
// counts as scope, then reads the payload.
func TestReportHandlerServesTheReport(t *testing.T) {
	root, deps := homeFixture(t)
	tree, err := store.Load(root)
	require.NoError(t, err)
	epic, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounEpic, Raw: "form-engine"})
	require.NoError(t, err)
	_, err = mutate.Transition(tree, deps, epic, mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)
	srv := homeRecapServer(t, root)

	var body map[string]any
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/report", &body))
	require.Equal(t, "Home Test", body["project"])
	require.GreaterOrEqual(t, len(body["weeks"].([]any)), 4)
	burnUp := body["burn_up"].([]any)
	require.Equal(t, float64(1), burnUp[len(burnUp)-1].(map[string]any)["scope"])
	for _, key := range []string{"kpis", "throughput", "bugs", "feature_times", "epics"} {
		require.Contains(t, body, key)
	}
}

// TestReportHandlerWithoutGit pins that an unreadable branch is not a failed
// read: the fixture has no .git, so Branch() errors and the notice is absent.
func TestReportHandlerWithoutGit(t *testing.T) {
	root, _ := homeFixture(t)
	srv := homeRecapServer(t, root)

	var body map[string]any
	require.Equal(t, 200, homeGetJSON(t, srv, "/api/report", &body))
	require.NotContains(t, body, "off_branch")
}

// TestReportHandlerRefusesWrites pins the read-only posture on the route.
func TestReportHandlerRefusesWrites(t *testing.T) {
	root, _ := homeFixture(t)
	srv := homeRecapServer(t, root)

	resp, err := srv.Client().Post(srv.URL+"/api/report", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
	require.Equal(t, "GET, HEAD", resp.Header.Get("Allow"))
}
