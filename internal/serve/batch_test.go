package serve_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// A search hit on a batch member carries its batched phase, so the console's
// result row does not read it as ready work.
func TestSearchEndpointMarksABatchMember(t *testing.T) {
	srv := httptest.NewServer(serve.SearchHandler(batchServeFixture(t)))
	t.Cleanup(srv.Close)
	type hits struct {
		Nodes []struct {
			Noun     string `json:"noun"`
			ID       string `json:"id"`
			Activity string `json:"activity"`
		} `json:"nodes"`
	}
	var member hits
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/search?q=B-aaaaaa", &member))
	require.NotEmpty(t, member.Nodes)
	require.Equal(t, "B-aaaaaa", member.Nodes[0].ID)
	require.Equal(t, "batched", member.Nodes[0].Activity)

	var batch hits
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/search?q=BT-aaaaaa&kind=batch", &batch))
	require.NotEmpty(t, batch.Nodes)
	require.Equal(t, "batch", batch.Nodes[0].Noun)
	require.Empty(t, batch.Nodes[0].Activity)
}

// A member's page shows its batch's branch and PR, so on a GitHub origin the
// batch reference carries the same two links the batch's own page does.
func TestNodePageLinksAMembersBatchBranchAndPR(t *testing.T) {
	root := batchServeFixture(t)
	path := filepath.Join(root, ".tiller", "batches", "core", "batch.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	started := strings.Replace(string(data), "status: ready\nblocked: false\nstarted: false\n", "status: in_review\nblocked: false\nstarted: true\n", 1)
	require.NotEqual(t, string(data), started)
	require.NoError(t, os.WriteFile(path, []byte(started+"owner: jc\nbranch: tiller/batches/core\npr: 14\n"), 0o644))
	gitRun(t, root, "init")
	setOrigin(t, root, "git@github.com:madeintandem/tiller.git")
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var member struct {
		Node struct {
			Batch struct {
				Branch    string `json:"branch"`
				BranchURL string `json:"branch_url"`
				PR        int    `json:"pr"`
				PRURL     string `json:"pr_url"`
			} `json:"batch"`
		} `json:"node"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/bug/B-aaaaaa", &member))
	require.Equal(t, "tiller/batches/core", member.Node.Batch.Branch)
	require.Equal(t, "https://github.com/madeintandem/tiller/tree/tiller/batches/core", member.Node.Batch.BranchURL)
	require.Equal(t, 14, member.Node.Batch.PR)
	require.Equal(t, "https://github.com/madeintandem/tiller/pull/14", member.Node.Batch.PRURL)
}
