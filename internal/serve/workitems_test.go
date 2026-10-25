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

const workItemsManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Work Items Test
  key: workitemstest
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

func TestWorkItemsEndpointServesOrderedShelfBoard(t *testing.T) {
	root := workItemsRoot(t)
	srv := httptest.NewServer(serve.Handler(root, nil, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)

	var got struct {
		Open []struct {
			ID, Noun, Status, Ref string
		} `json:"open"`
		Closed []struct {
			ID, Noun, Status string
			ClosedAt         string `json:"closed_at"`
		} `json:"closed"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/work-items", &got))
	require.Equal(t, "B-aaaaaa", got.Open[0].ID)
	require.Equal(t, "bug", got.Open[0].Noun)
	require.Equal(t, "CH-bbbbbb", got.Closed[0].ID)
	require.Equal(t, "cancelled", got.Closed[0].Status)
	require.NotEmpty(t, got.Closed[0].ClosedAt)
}

func TestWorkItemsEndpointRefusesWrites(t *testing.T) {
	srv := httptest.NewServer(serve.Handler(workItemsRoot(t), nil, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Post(srv.URL+"/api/work-items", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestWorkItemsEndpointReportsBrokenEventStream(t *testing.T) {
	root := workItemsRoot(t)
	writeFixtureFile(t, root, ".tiller/events/2026/08/29/EV-broken.yml", []byte(":\tbad yaml"))
	srv := httptest.NewServer(serve.Handler(root, nil, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Get(srv.URL + "/api/work-items")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func workItemsRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFixtureFile(t, root, ".tiller/tiller.yml", []byte(workItemsManifest))
	writeFixtureFile(t, root, ".tiller/bugs/ready-bug/bug.md", []byte("# Ready bug\n"))
	writeFixtureFile(t, root, ".tiller/bugs/ready-bug/bug.yml", []byte(`id: B-aaaaaa
key: bugs/ready-bug
title: Ready bug
status: ready
blocked: false
owner: jc
proposed_by: jc
`))
	writeFixtureFile(t, root, ".tiller/bugs/order.yml", []byte("order: [B-aaaaaa]\n"))
	writeFixtureFile(t, root, ".tiller/chores/cancelled-chore/chore.md", []byte("# Cancelled chore\n"))
	writeFixtureFile(t, root, ".tiller/chores/cancelled-chore/chore.yml", []byte(`id: CH-bbbbbb
key: chores/cancelled-chore
title: Cancelled chore
status: cancelled
blocked: false
proposed_by: jc
resolution: wontfix
`))
	writeFixtureFile(t, root, ".tiller/chores/order.yml", []byte("order: [CH-bbbbbb]\n"))
	writeFixtureFile(t, root, ".tiller/events/2026/08/29/EV-cancel.yml", []byte(`id: EV-cancel
type: cancel.chore
at: 2026-08-29T12:00:00Z
owner: jc
command: tiller cancel chore CH-bbbbbb --resolution wontfix
targets: [CH-bbbbbb]
`))
	return root
}

func writeFixtureFile(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}
