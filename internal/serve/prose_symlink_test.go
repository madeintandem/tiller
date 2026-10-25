package serve_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// TestProseRegisteredDocIdentityNeverFollowsSymlinks pins issue #113: the
// ledger entry is chosen by the requested address, and a registered document
// that has become a symlink is refused the way readiness refuses it, so the
// console never shows one document's address with another's prose and
// confirmation.
func TestProseRegisteredDocIdentityNeverFollowsSymlinks(t *testing.T) {
	const ledger = `docs:
  - path: a.md
    describes: [app]
    confirmed:
      by: reviewer
      commit: {kind: git_commit, value: "git:1234567", observed_at: "2026-09-02T10:00:00Z"}
      surfaces:
        - {path: app, kind: git_tree, value: "git:abcdef0", inventory: [app/main.go], observed_at: "2026-09-02T10:00:00Z"}
  - path: b.md
    describes: [app]
    confirmed:
      by: reviewer
      commit: {kind: git_commit, value: "git:1234567", observed_at: "2026-09-02T10:00:00Z"}
      surfaces:
        - {path: app, kind: git_tree, value: "git:abcdef0", inventory: [app/main.go], observed_at: "2026-09-02T10:00:00Z"}
`
	const unsafeMessage = "registered design doc .tiller/design/a.md is missing or unreadable; restore its prose or remove its links with `tiller unlink design`"
	type page struct {
		Path     string                 `json:"path"`
		Content  string                 `json:"content"`
		Baseline *views.BaselineDocView `json:"baseline"`
	}
	setup := func(t *testing.T) (string, *httptest.Server) {
		root, srv := proseFixture(t)
		enableDriftChecks(t, root, "stale_design")
		writeProse(t, root, ".tiller/design/baseline.yml", ledger)
		writeProse(t, root, ".tiller/design/a.md", "# Document A\n")
		writeProse(t, root, ".tiller/design/b.md", "# Document B\n")
		writeProse(t, root, ".tiller/design/notes.md", "# Unregistered notes\n")
		return root, srv
	}
	relink := func(t *testing.T, root, name, target string) {
		full := filepath.Join(root, ".tiller", "design", name)
		require.NoError(t, os.Remove(full))
		require.NoError(t, os.Symlink(target, full))
	}
	// The whole response body as raw fields regardless of status, so an error
	// answer can prove it carries no prose or baseline beside its message.
	rawBody := func(t *testing.T, srv *httptest.Server, path string) (int, map[string]json.RawMessage) {
		t.Helper()
		resp, err := srv.Client().Get(srv.URL + path)
		require.NoError(t, err)
		defer resp.Body.Close()
		var top map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&top))
		return resp.StatusCode, top
	}

	t.Run("registered doc symlinked to another registered doc", func(t *testing.T) {
		root, srv := setup(t)
		relink(t, root, "a.md", "b.md")
		for _, ref := range []string{".tiller/design/a.md", "design/a.md"} {
			status, body := rawBody(t, srv, proseRequest(ref, "1"))
			require.Equal(t, http.StatusInternalServerError, status, ref)
			require.Contains(t, string(body["error"]), unsafeMessage, ref)
			require.NotContains(t, body, "baseline")
			require.NotContains(t, body, "content")
		}
		var b page
		require.Equal(t, http.StatusOK, getJSON(t, srv, proseRequest("design/b.md", "1"), &b))
		require.Equal(t, "design/b.md", b.Path)
		require.Equal(t, "# Document B\n", b.Content)
		require.NotNil(t, b.Baseline)
		require.Equal(t, "b.md", b.Baseline.Path)
		require.NotNil(t, b.Baseline.Confirmed)
		require.Equal(t, "reviewer", b.Baseline.Confirmed.By)
	})

	t.Run("registered doc symlinked to an unregistered doc", func(t *testing.T) {
		root, srv := setup(t)
		relink(t, root, "a.md", "notes.md")
		status, body := rawBody(t, srv, proseRequest(".tiller/design/a.md", "1"))
		require.Equal(t, http.StatusInternalServerError, status)
		require.Contains(t, string(body["error"]), unsafeMessage)
		require.NotContains(t, body, "baseline")
	})

	t.Run("unregistered doc symlinked to a registered doc is refused", func(t *testing.T) {
		root, srv := setup(t)
		require.NoError(t, os.Symlink("b.md", filepath.Join(root, ".tiller", "design", "c.md")))
		status, body := rawBody(t, srv, proseRequest(".tiller/design/c.md", "1"))
		require.Equal(t, http.StatusBadRequest, status)
		require.NotContains(t, body, "content")
		require.NotContains(t, body, "baseline")
	})

	t.Run("healthy registered doc resolves under both spellings", func(t *testing.T) {
		_, srv := setup(t)
		for _, ref := range []string{".tiller/design/a.md", "design/a.md"} {
			var got page
			require.Equal(t, http.StatusOK, getJSON(t, srv, proseRequest(ref, "1"), &got), ref)
			require.Equal(t, ref, got.Path)
			require.Equal(t, "# Document A\n", got.Content)
			require.NotNil(t, got.Baseline, ref)
			require.Equal(t, "a.md", got.Baseline.Path)
			require.NotNil(t, got.Baseline.Confirmed)
		}
	})
}
