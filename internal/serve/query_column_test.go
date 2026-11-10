package serve_test

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/stretchr/testify/require"
)

// The console's Map merges a relationship query's vertices over its own, so
// an epic or feature vertex here carries the same board column the map and
// search carry; without it the inspector would fall back to stored status (a
// ready feature whose map card reads in review). `tiller query --json` keeps
// the stored-status-only shape.
func TestQueryEndpointVerticesCarryTheBoardColumn(t *testing.T) {
	root := nextFixture(t)
	setTaskPR(t, root, "schema-engine", "transcribe-the-schema", 11)
	srv := httptest.NewServer(serve.QueryHandler(root))
	t.Cleanup(srv.Close)
	type vertex struct {
		Key    string `json:"key"`
		Kind   string `json:"kind"`
		Status string `json:"status"`
		Column string `json:"column"`
	}
	var got struct {
		Vertices []vertex `json:"vertices"`
	}
	query := "/api/query?from=" + url.QueryEscape("form-engine") + "&follow=contains"
	require.Equal(t, 200, getJSON(t, srv, query, &got))
	require.Contains(t, got.Vertices, vertex{Key: "form-engine/schema-engine", Kind: "feature", Status: "ready", Column: "in_review"})
	for _, v := range got.Vertices {
		if v.Kind == "epic" || v.Kind == "feature" {
			require.NotEmpty(t, v.Column, v.Key)
		}
	}
}
