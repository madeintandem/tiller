package serve_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestCancelledEpicsStayDiscoverableOutsideBrowseSurfaces(t *testing.T) {
	root := nodeFixture(t)
	writeFixtureFile(t, root, ".tiller/epics/current/epic.yml", []byte("id: E-000002\nkey: current\ntitle: Current work\nstatus: ready\n"))
	writeFixtureFile(t, root, ".tiller/epics/current/epic.md", []byte("# Current work\n"))
	srv := buildServerAt(t, root)
	for _, status := range []string{"draft", "ready", "done", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			writeFixtureFile(t, root, ".tiller/epics/form-engine/epic.yml", []byte("id: E-000001\nkey: form-engine\ntitle: Form Engine\nstatus: "+status+"\n"))
			wantEpics := []string{"E-000002", "E-000001"}
			wantMap := []string{"E-000002", "E-000001", "F-000001", "T-000001", "F-000002"}
			if status == "cancelled" {
				wantEpics = []string{"E-000002"}
				wantMap = []string{"E-000002"}
			}
			for _, surface := range []struct {
				path, field string
				want        []string
			}{
				{"/api/tree", "nodes", wantEpics},
				{"/api/build", "epics", wantEpics},
				{"/api/map", "nodes", wantMap},
			} {
				t.Run(surface.path, func(t *testing.T) {
					var payload map[string]json.RawMessage
					require.Equal(t, 200, getJSON(t, srv, surface.path, &payload))
					var nodes []struct{ ID string }
					require.NoError(t, json.Unmarshal(payload[surface.field], &nodes))
					ids := make([]string, 0, len(nodes))
					for _, node := range nodes {
						ids = append(ids, node.ID)
					}
					require.Equal(t, surface.want, ids)
					if status == "cancelled" && surface.path == "/api/map" {
						require.JSONEq(t, "[]", string(payload["edges"]), "no layout edges may reference the hidden subtree")
					}
				})
			}
		})
	}

	var filtered struct{ Nodes []struct{ ID, Status string } }
	require.Equal(t, 200, getJSON(t, srv, "/api/ls?noun=epics&status=cancelled", &filtered))
	require.Len(t, filtered.Nodes, 1)
	require.Equal(t, "E-000001", filtered.Nodes[0].ID)
	require.Equal(t, "cancelled", filtered.Nodes[0].Status)

	var searched struct {
		Nodes []struct{ ID string }
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/search?q=Form%20Engine", &searched))
	found := false
	for _, node := range searched.Nodes {
		found = found || node.ID == "E-000001"
	}
	require.True(t, found, "cancelled epic remains a search destination")

	for _, addr := range []string{"epic/E-000001", "feature/F-000001", "task/T-000001"} {
		var detail struct{ Node struct{ ID string } }
		require.Equal(t, 200, getJSON(t, srv, "/api/node/"+addr, &detail))
		_, id, _ := strings.Cut(addr, "/")
		require.Equal(t, id, detail.Node.ID)
	}
	tree, err := store.Load(root)
	require.NoError(t, err)
	require.Equal(t, "E-000001", gen.Tree(tree).Nodes[1].ID, "CLI tree retains cancelled history")
}
