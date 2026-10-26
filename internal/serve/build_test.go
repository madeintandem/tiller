package serve_test

// These tests pin GET /api/build: the Build view's board
// (views.BuildBoard, internal/views/build.go) served fresh over the real
// console Handler, mux and all, so a dropped or typo'd route registration
// fails here rather than against a handler production never reaches (the
// same posture route_test.go's own header states). The ordering, rollup,
// held, and JSON-shape rules are pinned exhaustively in views/build_test.go
// against hand-built trees; this file's job is narrower: prove the wiring,
// the read-only posture, and that the wire payload nodeFixture's real graph
// produces matches the CLI's own dependency-order pin
// (TestTreeEndpointPinsThePayload's schema-engine-leads-form-loading fact).

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func buildServer(t *testing.T) *httptest.Server {
	t.Helper()
	return buildServerAt(t, nodeFixture(t))
}

func buildServerWithOpenWork(t *testing.T) *httptest.Server {
	t.Helper()
	root := nodeFixture(t)
	counts := map[string]int{}
	deps := mutate.Deps{
		Contributor: "jc",
		Clock:       fixedClock{},
		Mint: func(prefix string) string {
			counts[prefix]++
			return fmt.Sprintf("%s-%06d", prefix, 900000+counts[prefix])
		},
	}
	add := func(in mutate.WorkItemInput) {
		t.Helper()
		tree, err := store.Load(root)
		require.NoError(t, err)
		_, err = mutate.AddWorkItem(tree, deps, in)
		require.NoError(t, err)
	}
	add(mutate.WorkItemInput{
		Noun: store.NounBug, Key: "schema-direct", Title: "Schema direct",
		AttachedTo: "form-engine/schema-engine",
	})
	add(mutate.WorkItemInput{
		Noun: store.NounChore, Key: "parse-child", Title: "Parse child",
		AttachedTo: "form-engine/schema-engine/parse-the-schema",
	})
	return buildServerAt(t, root)
}

func buildServerAt(t *testing.T, root string) *httptest.Server {
	t.Helper()
	assets := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Tiller</title>")}}
	srv := httptest.NewServer(serve.Handler(root, assets, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)
	return srv
}

// buildPayload decodes enough of the envelope to assert identity, order,
// and the task rollup.
type buildPayload struct {
	Epics []struct {
		Noun          string         `json:"noun"`
		ID            string         `json:"id"`
		Key           string         `json:"key"`
		Title         string         `json:"title"`
		Ref           string         `json:"ref"`
		Status        string         `json:"status"`
		FeatureCounts map[string]int `json:"feature_counts"`
		Features      []struct {
			ID            string `json:"id"`
			Key           string `json:"key"`
			Title         string `json:"title"`
			Status        string `json:"status"`
			Ref           string `json:"ref"`
			TaskDone      int    `json:"task_done"`
			TaskTotal     int    `json:"task_total"`
			OpenWorkItems int    `json:"open_work_items"`
			Tasks         []struct {
				Noun   string `json:"noun"`
				ID     string `json:"id"`
				Ref    string `json:"ref"`
				Status string `json:"status"`
				Title  string `json:"title"`
			} `json:"tasks"`
		} `json:"features"`
	} `json:"epics"`
}

// TestBuildHandlerServesTheRealGraphInDependencyOrder pins the wire
// registration and the shape end to end against nodeFixture's real graph
// (node_test.go): one epic, schema-engine leading form-loading (the latter
// depends on the former, the exact Acme-precedent divergence
// TestTreeEndpointPinsThePayload's own comment describes), and
// schema-engine's one proposed task.
func TestBuildHandlerServesTheRealGraphInDependencyOrder(t *testing.T) {
	srv := buildServer(t)

	var got buildPayload
	require.Equal(t, 200, getJSON(t, srv, "/api/build", &got))
	require.Len(t, got.Epics, 1)

	epic := got.Epics[0]
	require.Equal(t, "epic", epic.Noun)
	require.Equal(t, "E-000001", epic.ID)
	require.Equal(t, "form-engine", epic.Key)
	require.Equal(t, "Form Engine", epic.Title)
	require.Equal(t, "form-engine (E-000001)", epic.Ref)
	require.Equal(t, "draft", epic.Status)
	require.Equal(t, map[string]int{"draft": 2}, epic.FeatureCounts, "both features are still draft")

	require.Len(t, epic.Features, 2)
	require.Equal(t, "F-000001", epic.Features[0].ID, "schema-engine leads: form-loading depends on it")
	require.Equal(t, "F-000002", epic.Features[1].ID)

	schema := epic.Features[0]
	require.Equal(t, 0, schema.TaskDone)
	require.Equal(t, 1, schema.TaskTotal)
	require.Len(t, schema.Tasks, 1)
	require.Equal(t, "task", schema.Tasks[0].Noun)
	require.Equal(t, "T-000001", schema.Tasks[0].ID)
	require.Equal(t, "proposed", schema.Tasks[0].Status)
	require.Equal(t, "Parse the schema", schema.Tasks[0].Title)

	loading := epic.Features[1]
	require.Equal(t, 0, loading.TaskTotal)
	require.Empty(t, loading.Tasks, "form-loading carries no tasks yet")
}

func TestBuildHandlerServesOpenWorkCount(t *testing.T) {
	srv := buildServerWithOpenWork(t)

	var got buildPayload
	require.Equal(t, 200, getJSON(t, srv, "/api/build", &got))
	require.Len(t, got.Epics, 1)
	require.Len(t, got.Epics[0].Features, 2)
	require.Equal(t, 2, got.Epics[0].Features[0].OpenWorkItems)
}

// TestBuildHandlerRefusesWrites pins the pass-1 read-only posture on this
// route, the same guard api() enforces for every other endpoint.
func TestBuildHandlerRefusesWrites(t *testing.T) {
	srv := buildServer(t)

	resp, err := srv.Client().Post(srv.URL+"/api/build", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
	require.Equal(t, "GET, HEAD", resp.Header.Get("Allow"))
}
