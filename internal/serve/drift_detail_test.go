package serve_test

// The drift panel's serve half (JC's chip-to-panel ask, 2026-08-06): the
// node page carries candidate-grain detail beside the badges, uncovered rows
// included, from the one drift.ForNode call, and both degrade together.

import (
	"net/http/httptest"
	"testing"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/stretchr/testify/require"
)

// TestNodePageCarriesDriftDetailBesideTheBadges pins the panel payload: the
// same active_basis finding that raises the badge arrives at candidate grain
// with its detail sentence, files, and the composed refresh-basis remedy.
func TestNodePageCarriesDriftDetailBesideTheBadges(t *testing.T) {
	root := approvedTaskRoot(t)
	enableDriftChecks(t, root, "active_basis")

	writeUnder(t, root, ".tiller/epics/form-engine/features/schema-engine/feature.md",
		"# Schema engine\n\n## Summary\n\nEdited after approval.\n")

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got struct {
		Drift  []driftBadgePayload `json:"drift"`
		Detail *struct {
			Candidates []struct {
				Marker string   `json:"marker"`
				Detail string   `json:"detail"`
				Files  []string `json:"files"`
				Remedy string   `json:"remedy"`
			} `json:"candidates"`
		} `json:"drift_detail"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/F-000001", &got))
	require.Len(t, got.Drift, 1, "the badge stands unchanged")
	require.NotNil(t, got.Detail, "and the panel detail rides beside it")
	require.Len(t, got.Detail.Candidates, 1)
	c := got.Detail.Candidates[0]
	require.Equal(t, "active_basis", c.Marker)
	require.NotEmpty(t, c.Detail)
	require.NotEmpty(t, c.Files)
	require.Contains(t, c.Remedy, "--refresh-basis")
	require.Contains(t, c.Remedy, "tiller approve feature")
}

// TestNodePageDriftDetailAbsentWhenClean pins the omitempty contract for the
// panel, the same absent-never-empty convention the badges keep.
func TestNodePageDriftDetailAbsentWhenClean(t *testing.T) {
	srv := nodeServer(t)

	status, top := rawPage(t, srv, "/api/node/feature/F-000001")
	require.Equal(t, 200, status)
	_, ok := top["drift_detail"]
	require.False(t, ok, "no findings, no block")
}
