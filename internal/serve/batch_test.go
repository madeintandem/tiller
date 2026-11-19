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

// batchServeFixture is nodeShelfFixture with its bug and chore put under one
// batch, written the way add batch leaves the files.
func batchServeFixture(t *testing.T) string {
	t.Helper()
	root := nodeShelfFixture(t)
	batch := "id: BT-aaaaaa\nkey: batches/core\ntitle: Core fixes\nstatus: ready\nblocked: false\nstarted: false\n"
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "batches", "core"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "batches", "core", "batch.yml"), []byte(batch), 0o644))
	for _, rel := range []string{"bugs/blank-on-reload/bug.yml", "chores/bump-go/chore.yml"} {
		path := filepath.Join(root, ".tiller", rel)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, append(data, []byte("batch: BT-aaaaaa\n")...), 0o644))
	}
	return root
}

// Every batch link the console builds is /node/batch/<id>, so the node route
// has to answer for the noun rather than 404.
func TestNodeHandlerServesABatchWithItsMembers(t *testing.T) {
	root := batchServeFixture(t)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	for _, addr := range []string{"BT-aaaaaa", "batches/core"} {
		var got struct {
			Node struct {
				Noun    string `json:"noun"`
				Title   string `json:"title"`
				Members []struct {
					ID string `json:"id"`
				} `json:"members"`
			} `json:"node"`
		}
		require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/batch/"+addr, &got))
		require.Equal(t, "batch", got.Node.Noun)
		require.Equal(t, "Core fixes", got.Node.Title)
		require.Len(t, got.Node.Members, 2)
	}

	var member struct {
		Node struct {
			Activity string `json:"activity"`
			Batch    struct {
				ID string `json:"id"`
			} `json:"batch"`
		} `json:"node"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/bug/B-aaaaaa", &member))
	require.Equal(t, "batched", member.Node.Activity)
	require.Equal(t, "BT-aaaaaa", member.Node.Batch.ID)
}

func TestListHandlerListsBatches(t *testing.T) {
	srv := httptest.NewServer(serve.ListHandler(batchServeFixture(t)))
	t.Cleanup(srv.Close)
	for _, noun := range []string{"batch", "batches"} {
		var got struct {
			Noun  string `json:"noun"`
			Count int    `json:"count"`
			Nodes []struct {
				ID      string `json:"id"`
				Members int    `json:"members"`
			} `json:"nodes"`
		}
		require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/ls?noun="+noun, &got))
		require.Equal(t, "batch", got.Noun)
		require.Equal(t, 1, got.Count)
		require.Equal(t, 2, got.Nodes[0].Members)
	}
}
