package serve_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func TestQueryEndpointDistinguishesMisuseMissingAndReadFailures(t *testing.T) {
	root, _ := searchFixtureAt(t)
	handler := serve.QueryHandler(root)
	for _, test := range []struct {
		query  string
		status int
	}{
		{"follow=contains", 400},
		{"from=E-aaaaaa&follow=no_such_direction", 400},
		{"from=E-aaaaaa&follow=contains&depth=-1", 400},
		{"from=E-aaaaaa&follow=contains&depth=1&depth=2", 400},
		{"from=E-aaaaaa&follow=contains&where=title=x", 400},
		{"from=F-ffffff&follow=contains", 404},
		{"from=doc:.tiller/absent.md&follow=document_of", 404},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/query?"+test.query, nil))
		require.Equal(t, test.status, recorder.Code, recorder.Body.String())
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(outside, []byte("E-aaaaaa\n"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".tiller", "design", "unsafe.md")))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/query?from=E-aaaaaa&follow=mentioned_by", nil))
	require.Equal(t, 500, recorder.Code, recorder.Body.String())
}

func TestQueryEndpointFreshReadsAndEmptyAnswer(t *testing.T) {
	root, _ := searchFixtureAt(t)
	handler := serve.QueryHandler(root)
	query := func() relationships.QueryResult {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/query?from=F-aaaaaa&follow=mentioned_by&where=kind=document", nil))
		require.Equal(t, 200, recorder.Code, recorder.Body.String())
		var result relationships.QueryResult
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
		return result
	}
	require.Empty(t, query().Matched)
	file := filepath.Join(root, ".tiller", "design", "01-notes.md")
	require.NoError(t, os.WriteFile(file, []byte("# Notes\n\nF-aaaaaa\n"), 0o644))
	result := query()
	require.Equal(t, []string{"doc:.tiller/design/01-notes.md"}, result.Matched)
	require.Len(t, result.Edges, 1)
	require.Equal(t, 3, result.Edges[0].Sources[0].Line)
	require.NoError(t, os.WriteFile(file, []byte("# Notes\n"), 0o644))
	require.Empty(t, query().Matched, "a second request cannot reuse the old mention index")

	for _, method := range []string{"POST", "PUT", "DELETE"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, "/api/query?from=F-aaaaaa&follow=mentioned_by", nil))
		require.Equal(t, 405, recorder.Code)
		require.Equal(t, "GET, HEAD", recorder.Header().Get("Allow"))
	}
}

func TestNodeAndMapDegradeCorpusFailuresButQueryStillRefuses(t *testing.T) {
	root := nodeFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/a.md"), []byte("F-zzzzzz\n"), 0644))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, ".tiller/z-unsafe")))

	recorder := httptest.NewRecorder()
	serve.NodeHandler(root).ServeHTTP(recorder, httptest.NewRequest("GET", "/api/node/feature/F-000001", nil))
	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	var page views.NodePage
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &page))
	require.Equal(t, "F-000001", page.Node.ID)
	require.False(t, page.Backlinks.Complete)
	require.Equal(t, "corpus_unreadable", page.Backlinks.Diagnostics[0].Code)

	recorder = httptest.NewRecorder()
	serve.MapHandler(root).ServeHTTP(recorder, httptest.NewRequest("GET", "/api/map", nil))
	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	var graph views.MapView
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &graph))
	require.False(t, graph.Relationships.Complete)
	require.Equal(t, "corpus_unreadable", graph.Relationships.Diagnostics[0].Code)
	var dependencies []views.MapEdge
	for _, edge := range graph.Edges {
		if edge.Kind == "depends_on" {
			dependencies = append(dependencies, edge)
		}
	}
	require.Equal(t, []views.MapEdge{{From: "F-000002", To: "F-000001", Kind: "depends_on"}}, dependencies)
	for _, vertex := range graph.Relationships.Vertices {
		require.NotEqual(t, "doc:.tiller/a.md", vertex.ID)
		require.NotEqual(t, "F-zzzzzz", vertex.ID)
	}

	recorder = httptest.NewRecorder()
	serve.QueryHandler(root).ServeHTTP(recorder, httptest.NewRequest("GET", "/api/query?from=F-000001&follow=mentioned_by", nil))
	require.Equal(t, 500, recorder.Code)
}

func TestNodeMissingContractRemainsCompleteAndUnsafeFilesNeverLeak(t *testing.T) {
	root := nodeFixture(t)
	dir := filepath.Join(root, ".tiller/epics/form-engine/features/schema-engine")
	request := func() views.NodePage {
		t.Helper()
		recorder := httptest.NewRecorder()
		serve.NodeHandler(root).ServeHTTP(recorder, httptest.NewRequest("GET", "/api/node/feature/F-000001", nil))
		require.Equal(t, 200, recorder.Code, recorder.Body.String())
		var page views.NodePage
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &page))
		return page
	}
	page := request()
	require.True(t, page.Backlinks.Complete)
	require.Empty(t, page.Backlinks.Diagnostics)
	require.Nil(t, page.Eval)

	outside := filepath.Join(t.TempDir(), "unsafe.md")
	require.NoError(t, os.WriteFile(outside, []byte("---\nnode: F-000001\ncriteria:\n  - id: C1\n    statement: secret-contract-bytes\n    binding: {type: test, ref: private/secret.go}\n---\nsecret-prose-bytes\n"), 0644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "feature.eval.md")))
	require.NoError(t, os.Remove(filepath.Join(dir, "feature.md")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "feature.md")))
	page = request()
	require.False(t, page.Backlinks.Complete)
	require.NotNil(t, page.Prose)
	require.Empty(t, page.Prose.Content)
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret-contract-bytes")
	require.NotContains(t, string(encoded), "secret-prose-bytes")
	require.NotContains(t, string(encoded), "private/secret.go")
}

func TestTolerantRoutesStillRefuseMalformedTree(t *testing.T) {
	root := nodeFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/epics/form-engine/epic.yml"), []byte("[broken"), 0644))
	for _, route := range []struct {
		path    string
		handler func(string) http.HandlerFunc
	}{{"/api/node/feature/F-000001", serve.NodeHandler}, {"/api/map", serve.MapHandler}} {
		recorder := httptest.NewRecorder()
		route.handler(root).ServeHTTP(recorder, httptest.NewRequest("GET", route.path, nil))
		require.Equal(t, 500, recorder.Code)
	}
}

// Backlinks from Markdown files whose names carry '#', '?', or ':' resolve to
// distinct documents, and each returned destination opens its own file when
// the path rides the prose reader's query string, encoded exactly once.
func TestPunctuatedDocumentBacklinksOpenTheirOwnFiles(t *testing.T) {
	root, _ := searchFixtureAt(t)
	contents := map[string]string{
		".tiller/design/notes#1.md": "hash notes mention F-aaaaaa\n",
		".tiller/design/notes?1.md": "query notes mention F-aaaaaa\n",
		".tiller/design/notes:1.md": "colon notes mention F-aaaaaa\n",
	}
	for file, content := range contents {
		require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), []byte(content), 0o644))
	}

	recorder := httptest.NewRecorder()
	serve.QueryHandler(root).ServeHTTP(recorder, httptest.NewRequest("GET", "/api/query?from=F-aaaaaa&follow=mentioned_by&where=kind=document", nil))
	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	var result relationships.QueryResult
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	want := make([]string, 0, len(contents))
	for file := range contents {
		want = append(want, "doc:"+file)
	}
	require.ElementsMatch(t, want, result.Matched)
	for _, edge := range result.Edges {
		require.NotEmpty(t, edge.From)
		require.NotEmpty(t, edge.To)
	}

	prose := httptest.NewServer(serve.ProseHandler(root))
	t.Cleanup(prose.Close)
	opened := 0
	for _, vertex := range result.Vertices {
		if vertex.Kind != "document" {
			continue
		}
		require.NotNil(t, vertex.Destination, vertex.ID)
		status, body := proseBody(t, prose, proseRequest(vertex.Destination.Path, "1"))
		require.Equal(t, http.StatusOK, status, body)
		var file struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &file))
		require.Equal(t, vertex.Destination.Path, file.Path)
		require.Equal(t, contents[vertex.Destination.Path], file.Content)
		opened++
	}
	require.Equal(t, len(contents), opened)
}
