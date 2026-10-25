package cli

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/stretchr/testify/require"
)

func TestQueryCLIAndHTTPReturnTheSameReadWithoutIdentity(t *testing.T) {
	laySearchTree(t)
	root, err := os.Getwd()
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(root, ".tiller", "identity.yml"))
	t.Setenv("TILLER_CONTRIBUTOR", "")
	before := fingerprintRepo(t, root)
	out, stderr, code := execCLI(t, "query", "--from", cliFeatureKey, "--from", "form-engine", "--follow", "contains,document_of,has_document", "--depth", "2", "--where", "kind=document", "--json")
	require.Equal(t, 0, code, stderr+out)
	var envelope struct {
		OK   bool                      `json:"ok"`
		Data relationships.QueryResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	require.True(t, envelope.OK)
	require.NotEmpty(t, envelope.Data.Matched, "owned prose must be reached through the same shared index")

	params := url.Values{"from": {cliFeatureKey, "form-engine"}, "follow": {"contains,document_of,has_document"}, "depth": {"2"}, "where": {"kind=document"}}
	recorder := httptest.NewRecorder()
	serve.QueryHandler(root).ServeHTTP(recorder, httptest.NewRequest("GET", "/api/query?"+params.Encode(), nil))
	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	var response relationships.QueryResult
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, envelope.Data, response)
	require.Equal(t, before, fingerprintRepo(t, root), "query reads cannot create identity, events, or other project files")
}

func TestQueryMalformedCLIAlwaysUsesErrorEnvelope(t *testing.T) {
	for _, args := range [][]string{
		{"--from", "E-aaaaaa", "--follow", "contains", "extra", "--json"},
		{"--from", "E-aaaaaa", "--follow", "contains", "--bogus", "--json"},
		{"--json", "--from"},
		{"--from", "E-aaaaaa", "--follow", "contains", "--depth", "-1", "--json"},
		{"--from", "E-aaaaaa", "--follow", "contains", "--where", "title=x", "--json"},
	} {
		out, _, code := execCLI(t, append([]string{"query"}, args...)...)
		require.Equal(t, 2, code, out)
		var envelope struct {
			OK      bool       `json:"ok"`
			Command string     `json:"command"`
			Error   *ErrorBody `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &envelope), "all syntax failures, including Cobra flag errors, must be machine-readable")
		require.False(t, envelope.OK)
		require.Equal(t, "query", envelope.Command)
		require.NotNil(t, envelope.Error)
		require.Equal(t, 2, envelope.Error.Code)
	}
}

func TestQueryMissingStartAndUnreadableCorpusHaveDifferentExits(t *testing.T) {
	laySearchTree(t)
	out, _, code := execCLI(t, "query", "--from", "F-ffffff", "--follow", "contains", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "not_found"`)

	root, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "absent.md"), filepath.Join(root, ".tiller", "design", "broken.md")))
	out, _, code = execCLI(t, "query", "--from", cliFeatureKey, "--follow", "mentioned_by", "--json")
	require.Equal(t, 3, code, out)
}

func TestQueryHumanIncompleteAnswerCannotLookComplete(t *testing.T) {
	result := relationships.QueryResult{
		Matched: []string{}, Complete: false,
		Diagnostics: []relationships.Diagnostic{{Code: "eval_unavailable", Facet: "eval", Message: "contract unavailable"}},
	}
	human := renderQueryHuman(result)
	require.Contains(t, human, "incomplete")
	require.Contains(t, human, "eval")
	require.Contains(t, human, "contract unavailable")
}
