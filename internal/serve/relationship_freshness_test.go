package serve_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/search"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func TestRelationshipRoutesRevalidateWarmedCorpus(t *testing.T) {
	root := nodeFixture(t)
	file := filepath.Join(root, ".tiller", "mentions.md")
	const coordinate = "doc:.tiller/mentions.md"
	require.NoError(t, os.WriteFile(file, []byte("F-000001\n"), 0o644))
	queryHandler := serve.QueryHandler(root)
	nodeHandler := serve.NodeHandler(root)
	mapHandler := serve.MapHandler(root)
	query := func(id string, status int) relationships.QueryResult {
		t.Helper()
		response := httptest.NewRecorder()
		queryHandler.ServeHTTP(response, httptest.NewRequest("GET", "/api/query?from="+id+"&follow=mentioned_by", nil))
		require.Equal(t, status, response.Code, response.Body.String())
		var result relationships.QueryResult
		if status == 200 {
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		}
		return result
	}
	supplemental := func(complete bool) {
		t.Helper()
		response := httptest.NewRecorder()
		nodeHandler.ServeHTTP(response, httptest.NewRequest("GET", "/api/node/feature/F-000001", nil))
		require.Equal(t, 200, response.Code, response.Body.String())
		var page views.NodePage
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
		require.Equal(t, complete, page.Backlinks.Complete)
		response = httptest.NewRecorder()
		mapHandler.ServeHTTP(response, httptest.NewRequest("GET", "/api/map", nil))
		require.Equal(t, 200, response.Code, response.Body.String())
		var graph views.MapView
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &graph))
		require.Equal(t, complete, graph.Relationships.Complete)
		if !complete {
			for _, vertex := range graph.Relationships.Vertices {
				require.NotEqual(t, coordinate, vertex.ID)
			}
		}
	}
	require.Contains(t, query("F-000001", 200).Matched, coordinate)
	supplemental(true)
	info, err := os.Stat(file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, []byte("F-000002\n"), 0o644))
	require.NoError(t, os.Chtimes(file, info.ModTime(), info.ModTime()))
	updated, err := os.Stat(file)
	require.NoError(t, err)
	require.Equal(t, info.Size(), updated.Size())
	require.Equal(t, info.ModTime(), updated.ModTime())
	require.NotContains(t, query("F-000001", 200).Matched, coordinate)
	require.Contains(t, query("F-000002", 200).Matched, coordinate)
	supplemental(true)
	if os.Geteuid() != 0 {
		require.NoError(t, os.Chmod(file, 0))
		t.Cleanup(func() { _ = os.Chmod(file, 0o644) })
		query("F-000002", 500)
		supplemental(false)
		require.NoError(t, os.Chmod(file, 0o644))
		require.Contains(t, query("F-000002", 200).Matched, coordinate)
	}

	require.NoError(t, os.Remove(file))
	require.NotContains(t, query("F-000002", 200).Matched, coordinate)
	outside := filepath.Join(t.TempDir(), "secret.md")
	require.NoError(t, os.WriteFile(outside, []byte("F-000002\n"), 0o644))
	require.NoError(t, os.Symlink(outside, file))
	query("F-000002", 500)
	supplemental(false)
	require.NoError(t, os.Remove(file))
	require.NoError(t, os.Mkdir(file, 0o755))
	query("F-000002", 500)
	supplemental(false)
	require.NoError(t, os.Remove(file))
	require.NoError(t, os.WriteFile(file, []byte("F-000001\n"), 0o644))
	require.Contains(t, query("F-000001", 200).Matched, coordinate)
	require.NotContains(t, query("F-000002", 200).Matched, coordinate)
	supplemental(true)
}

func TestProductionRoutesReadCurrentCorpusBytesWithUnchangedMetadata(t *testing.T) {
	root := nodeFixture(t)
	file := filepath.Join(root, ".tiller", "mentions.md")
	const coordinate = "doc:.tiller/mentions.md"
	require.NoError(t, os.WriteFile(file, []byte("F-000001 oldtoken\n"), 0o644))
	aged := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(file, aged, aged))
	before, err := os.Stat(file)
	require.NoError(t, err)
	srv := httptest.NewServer(serve.Handler(root, nil, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)

	query := func(id string) relationships.QueryResult {
		t.Helper()
		var result relationships.QueryResult
		require.Equal(t, 200, getJSON(t, srv, "/api/query?from="+id+"&follow=mentioned_by", &result))
		require.True(t, result.Complete)
		return result
	}
	searchDocuments := func(token string) []search.DocumentResult {
		t.Helper()
		var result search.Result
		require.Equal(t, 200, getJSON(t, srv, "/api/search?q="+token, &result))
		return result.Documents
	}
	rewrite := func(content string) {
		t.Helper()
		require.Len(t, content, int(before.Size()))
		f, err := os.OpenFile(file, os.O_WRONLY, 0)
		require.NoError(t, err)
		_, err = f.WriteAt([]byte(content), 0)
		require.NoError(t, err)
		require.NoError(t, f.Close())
		require.NoError(t, os.Chtimes(file, before.ModTime(), before.ModTime()))
		after, err := os.Stat(file)
		require.NoError(t, err)
		require.True(t, os.SameFile(before, after))
		require.Equal(t, before.Size(), after.Size())
		require.Equal(t, before.ModTime(), after.ModTime())
		require.Equal(t, before.Mode(), after.Mode())
	}

	require.Contains(t, query("F-000001").Matched, coordinate)
	require.Len(t, searchDocuments("oldtoken"), 1)
	require.Empty(t, searchDocuments("newtoken"))

	rewrite("F-000002 newtoken\n")
	require.NotContains(t, query("F-000001").Matched, coordinate)
	require.Contains(t, query("F-000002").Matched, coordinate)
	require.Empty(t, searchDocuments("oldtoken"))
	require.Len(t, searchDocuments("newtoken"), 1)

	for _, malformed := range []string{"F-000002 newtok\x00n\n", "F-000002 newtok\xffn\n"} {
		rewrite(malformed)
		require.Equal(t, 500, getJSON(t, srv, "/api/query?from=F-000002&follow=mentioned_by", nil))
		require.Equal(t, 500, getJSON(t, srv, "/api/search?q=newtoken", nil))
	}
	rewrite("F-000001 oldtoken\n")
	require.Contains(t, query("F-000001").Matched, coordinate)
	require.Len(t, searchDocuments("oldtoken"), 1)
}
