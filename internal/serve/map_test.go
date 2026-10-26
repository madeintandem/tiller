package serve_test

// These tests pin GET /api/map:
// views.MapOf served fresh over the real console Handler, mux and all, the same
// posture build_test.go's own header states for /api/build.
// TestMapEndpointServesNodesAndEdges rides buildServerWithOpenWork, the fixture
// that actually attaches a bug to a feature (form-engine/schema-engine) and a
// chore to a task (form-engine/schema-engine/parse-the-schema). buildServer
// alone (nodeFixture with no work items) carries neither, so it cannot pin the
// attached_to edges this test exists to prove.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type mapPayload struct {
	Nodes []struct {
		Noun   string `json:"noun"`
		ID     string `json:"id"`
		Parent string `json:"parent"`
		Holds  int    `json:"holds"`
	} `json:"nodes"`
	Edges []struct {
		From string `json:"from"`
		To   string `json:"to"`
		Kind string `json:"kind"`
	} `json:"edges"`
}

// TestMapEndpointServesNodesAndEdges pins /api/map over the open-work
// fixture: every noun the map draws is present, features name their epic,
// and the fixture's attached bug and chore each produce an attached_to
// edge into the tree.
func TestMapEndpointServesNodesAndEdges(t *testing.T) {
	srv := buildServerWithOpenWork(t)
	resp, err := srv.Client().Get(srv.URL + "/api/map")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got mapPayload
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

	nouns := map[string]int{}
	var featureParent string
	for _, n := range got.Nodes {
		nouns[n.Noun]++
		if n.Noun == "feature" && featureParent == "" {
			featureParent = n.Parent
		}
	}
	require.Greater(t, nouns["epic"], 0)
	require.Greater(t, nouns["feature"], 0)
	require.Greater(t, nouns["task"], 0)
	require.Equal(t, 1, nouns["bug"])
	require.Equal(t, 1, nouns["chore"])
	require.NotEmpty(t, featureParent, "a feature names its epic as its container")

	kinds := map[string]int{}
	for _, e := range got.Edges {
		kinds[e.Kind]++
	}
	require.Equal(t, 2, kinds["attached_to"], "the fixture's bug and chore are each attached")
}

// TestMapEndpointRefusesWrites pins the pass-1 read-only posture on this
// route, the same guard api() enforces for every other endpoint.
func TestMapEndpointRefusesWrites(t *testing.T) {
	srv := buildServer(t)
	resp, err := srv.Client().Post(srv.URL+"/api/map", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}
