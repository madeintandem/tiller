package serve_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/stretchr/testify/require"
)

func TestNodeProseRejectsOversizedContent(t *testing.T) {
	root := nodeFixture(t)
	prose := ".tiller/epics/form-engine/features/schema-engine/feature.md"
	require.NoError(t, os.WriteFile(filepath.Join(root, prose), []byte(strings.Repeat("x", (8<<20)+1)), 0o644))
	srv := httptest.NewServer(serve.NodeHandler(root))
	defer srv.Close()
	response, err := srv.Client().Get(srv.URL + "/api/node/feature/form-engine/schema-engine")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
}

func TestNodeRelationshipsDiagnoseOversizedCorpusWithoutReturningIt(t *testing.T) {
	root := nodeFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller/design"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/design/large.md"), []byte(strings.Repeat("oversized-marker ", 600000)), 0o644))
	srv := httptest.NewServer(serve.NodeHandler(root))
	defer srv.Close()
	response, err := srv.Client().Get(srv.URL + "/api/node/feature/form-engine/schema-engine")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "corpus_unreadable")
	require.Contains(t, string(encoded), "size limit")
	require.NotContains(t, string(encoded), "oversized-marker")
}

func TestNodeWorkflowDiagnosesOversizedContractBeforeParsing(t *testing.T) {
	root := nodeFixture(t)
	path := filepath.Join(root, ".tiller/epics/form-engine/features/schema-engine/feature.eval.md")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("oversized-contract-marker ", 400000)), 0o644))
	srv := httptest.NewServer(serve.NodeHandler(root))
	defer srv.Close()
	response, err := srv.Client().Get(srv.URL + "/api/node/feature/form-engine/schema-engine")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "size limit")
	require.NotContains(t, string(encoded), "oversized-contract-marker")
}
