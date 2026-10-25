package serve_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func TestTweakReadEndpointsServeAuthoredShelfWork(t *testing.T) {
	root := workItemsRoot(t)
	writeFixtureFile(t, root, ".tiller/tweaks/tune/tweak.md", []byte("# Tune rendering\n\n## What\n\nRefine the button spacing.\n\n## Why\n\nImprove readability.\n\n## Done when\n\nSpacing is consistent.\n"))
	writeFixtureFile(t, root, ".tiller/tweaks/tune/tweak.yml", []byte(`id: TW-aaaaaa
key: tweaks/tune
title: Tune rendering
status: proposed
blocked: false
owner: jc
proposed_by: jc
`))
	writeFixtureFile(t, root, ".tiller/tweaks/order.yml", []byte("order: [TW-aaaaaa]\n"))
	srv := httptest.NewServer(serve.Handler(root, nil, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)

	for _, noun := range []string{"tweak", "tweaks"} {
		var list struct {
			Nodes []views.ListItem `json:"nodes"`
		}
		require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/ls?noun="+noun+"&status=unfinished&owner=jc", &list))
		require.Len(t, list.Nodes, 1)
		require.Equal(t, "TW-aaaaaa", list.Nodes[0].ID)
	}
	for _, addr := range []string{"TW-aaaaaa", "tweaks/tune"} {
		var page struct {
			Node  views.NodeView `json:"node"`
			Prose struct {
				Content string `json:"content"`
			} `json:"prose"`
		}
		require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/tweak/"+addr, &page))
		require.Equal(t, "tweak", page.Node.Noun)
		require.Equal(t, "Tune rendering", page.Node.Title)
		require.Contains(t, page.Prose.Content, "Refine the button spacing.")
	}
	var board views.WorkItemsBoard
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/work-items", &board))
	require.Equal(t, "TW-aaaaaa", board.Open[len(board.Open)-1].ID)
	require.Equal(t, "tweak", board.Open[len(board.Open)-1].Noun)
	var mapped views.MapView
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/map", &mapped))
	require.Equal(t, "TW-aaaaaa", mapped.Nodes[len(mapped.Nodes)-1].ID)
}
