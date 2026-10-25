package serve_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

const proseTestManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Prose Serve Test
  key: proseserve
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

func proseRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	design := filepath.Join(root, ".tiller", "design")
	require.NoError(t, os.MkdirAll(design, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(proseTestManifest), 0o644))
	return root
}

func proseFixture(t *testing.T) (string, *httptest.Server) {
	t.Helper()
	root := proseRoot(t)
	srv := httptest.NewServer(serve.ProseHandler(root))
	t.Cleanup(srv.Close)
	return root, srv
}

func proseRequest(path, line string) string {
	q := url.Values{}
	if path != "" {
		q.Set("path", path)
	}
	if line != "" {
		q.Set("line", line)
	}
	return "/api/prose?" + q.Encode()
}

func proseURL(srv *httptest.Server, path, line string) string {
	return srv.URL + proseRequest(path, line)
}

// proseBody fetches one prose request and returns its status and raw body, so
// a refusal test can prove the body never carried the refused file's bytes.
func proseBody(t *testing.T, srv *httptest.Server, request string) (int, string) {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + request)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func writeProse(t *testing.T, root, ref, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(ref))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// TestProseEndpointAnswersExactLine pins the endpoint's complete success
// envelope. Content is the whole file, while line_text is the requested line
// byte-for-byte, including indentation, tabs, and trailing spaces.
func TestProseEndpointAnswersExactLine(t *testing.T) {
	root, srv := proseFixture(t)
	const ref = ".tiller/design/01-overview.md"
	const content = "# Overview\n\n  indented\tvalue  \n"
	writeProse(t, root, ref, content)

	resp, err := srv.Client().Get(proseURL(srv, ref, "3"))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	want := `{
  "path": ".tiller/design/01-overview.md",
  "content": "# Overview\n\n  indented\tvalue  \n",
  "line": 3,
  "line_text": "  indented\tvalue  "
}
`
	require.Equal(t, want, string(body))
}

// TestProseEndpointCountsTheTrailingEmptyLine pins strings.Split semantics:
// a trailing newline creates one final, addressable empty line rather than
// shortening the document at its last non-empty line.
func TestProseEndpointCountsTheTrailingEmptyLine(t *testing.T) {
	root, srv := proseFixture(t)
	const ref = ".tiller/design/01-trailing.md"
	writeProse(t, root, ref, "only line\n")

	resp, err := srv.Client().Get(proseURL(srv, ref, "2"))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	want := `{
  "path": ".tiller/design/01-trailing.md",
  "content": "only line\n",
  "line": 2,
  "line_text": ""
}
`
	require.Equal(t, want, string(body))
}

// TestProseEndpointReadsNodeSourcePaths keeps the file reader and node source
// citation contract on one wire. design/ is the human-facing alias for
// .tiller/design/, while other repo-relative Markdown names its file directly
// and serves because a node's source cites it.
func TestProseEndpointReadsNodeSourcePaths(t *testing.T) {
	root, srv := proseFixture(t)
	writeLedgerChore(t, root, "source: docs/spec/source.md#L1\n")
	cases := []struct {
		name   string
		source string
		stored string
	}{
		{"design alias", "design/01-source.md", ".tiller/design/01-source.md"},
		{"repo Markdown", "docs/spec/source.md", "docs/spec/source.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeProse(t, root, tc.stored, "# Source\n")

			resp, err := srv.Client().Get(proseURL(srv, tc.source, "1"))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 200, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Contains(t, string(body), `"path": "`+tc.source+`"`)
			require.Contains(t, string(body), `"line_text": "# Source"`)
		})
	}
}

// TestProseEndpointRejectsMalformedPath keeps caller mistakes distinct from a
// repo-relative Markdown path that was valid but disappeared.
func TestProseEndpointRejectsMalformedPath(t *testing.T) {
	root, srv := proseFixture(t)
	cases := []struct {
		name string
		path string
	}{
		{"missing", ""},
		{"absolute", filepath.Join(root, ".tiller", "design", "01-overview.md")},
		{"traversal outside corpus", ".tiller/design/../../secret.md"},
		{"traversal within corpus", ".tiller/design/../design/overview.md"},
		{"non-Markdown", ".tiller/design/overview.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.Client().Get(proseURL(srv, tc.path, "1"))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 400, resp.StatusCode)
			require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
		})
	}
}

// TestProseEndpointRejectsMalformedLine pins line as a required positive,
// base-10, machine-sized integer. Parse overflow remains caller misuse rather
// than wrapping into an addressable line.
func TestProseEndpointRejectsMalformedLine(t *testing.T) {
	root, srv := proseFixture(t)
	const ref = ".tiller/design/01-overview.md"
	writeProse(t, root, ref, "first\nsecond\n")

	for _, line := range []string{"", "not-a-number", "0", "-1", "18446744073709551616"} {
		t.Run("line="+line, func(t *testing.T) {
			resp, err := srv.Client().Get(proseURL(srv, ref, line))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 400, resp.StatusCode)
			require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
		})
	}
}

// TestProseEndpointReturns404ForVanishedOrShortenedFile reserves not-found for
// a valid destination that no longer exists and for a requested line the
// current file no longer has.
func TestProseEndpointReturns404ForVanishedOrShortenedFile(t *testing.T) {
	root, srv := proseFixture(t)
	const vanished = ".tiller/design/01-vanished.md"
	writeProse(t, root, vanished, "gone soon\n")
	require.NoError(t, os.Remove(filepath.Join(root, filepath.FromSlash(vanished))))

	const shortened = ".tiller/design/02-shortened.md"
	writeProse(t, root, shortened, "first\nsecond\n")

	for _, tc := range []struct {
		name string
		path string
		line string
	}{
		{"vanished path", vanished, "1"},
		{"out-of-range line", shortened, "4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.Client().Get(proseURL(srv, tc.path, tc.line))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 404, resp.StatusCode)
			require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
		})
	}
}

// TestProseEndpointRejectsEscapingSymlinks makes canonical confinement apply
// at both possible escape points: the Markdown leaf itself and a parent
// directory. These are broken corpus state, not malformed query text.
func TestProseEndpointRejectsEscapingSymlinks(t *testing.T) {
	root, srv := proseFixture(t)
	repoOutside := filepath.Join(root, "docs")
	require.NoError(t, os.MkdirAll(repoOutside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoOutside, "outside.md"), []byte("# repo outside corpus\n"), 0o644))
	external := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(external, "outside.md"), []byte("# outside repo\n"), 0o644))

	leaf := filepath.Join(root, ".tiller", "design", "02-leaf.md")
	require.NoError(t, os.Symlink(filepath.Join(repoOutside, "outside.md"), leaf))
	parent := filepath.Join(root, ".tiller", "linked-design")
	require.NoError(t, os.Symlink(external, parent))

	for _, ref := range []string{
		".tiller/design/02-leaf.md",
		".tiller/linked-design/outside.md",
	} {
		t.Run(ref, func(t *testing.T) {
			resp, err := srv.Client().Get(proseURL(srv, ref, "1"))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
		})
	}
}

// TestProseEndpointRefusesInternalSymlinks pins one confinement policy with
// the store: a served prose address is read only as the regular file it names,
// never through a symlink, even one whose target stays inside the repo. A
// cited document aliasing .env and a corpus document aliasing an uncited
// in-repo note would otherwise serve those files under a served address.
// Like evidence, the refusal is caller misuse (400), not a server failure.
func TestProseEndpointRefusesInternalSymlinks(t *testing.T) {
	root, srv := proseFixture(t)
	writeLedgerChore(t, root, "source: docs/cited.md#L1\n")
	writeProse(t, root, ".env", "SECRET=super-secret\n")
	writeProse(t, root, "notes/private.md", "# super-secret\n")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.Symlink("../.env", filepath.Join(root, "docs", "cited.md")))
	require.NoError(t, os.Symlink("../../notes/private.md", filepath.Join(root, ".tiller", "design", "alias.md")))
	require.NoError(t, os.Symlink("../notes", filepath.Join(root, ".tiller", "linked")))

	for _, ref := range []string{"docs/cited.md", ".tiller/design/alias.md", "design/alias.md", ".tiller/linked/private.md"} {
		t.Run(ref, func(t *testing.T) {
			status, body := proseBody(t, srv, proseRequest(ref, "1"))
			require.Equal(t, http.StatusBadRequest, status, "a symlinked address is caller misuse, as for evidence")
			require.NotContains(t, body, "super-secret")
			require.NotContains(t, body, root, "the refusal never exposes an absolute path")
		})
	}
}

// TestProseEndpointRefusesDotDirectoriesInCorpus keeps gitignored machine
// state such as .tiller/.cache/ out of the served corpus: a dot-prefixed
// segment below .tiller/ is outside scope, the same 404 whether or not a file
// exists there.
func TestProseEndpointRefusesDotDirectoriesInCorpus(t *testing.T) {
	root, srv := proseFixture(t)
	refused := []string{".tiller/.cache/secret.md", ".tiller/.private/x.md", ".tiller/design/.hidden.md", "design/.hidden.md"}
	for _, ref := range refused[:3] {
		writeProse(t, root, ref, "# super-secret\n")
	}
	for _, ref := range append(refused, ".tiller/.cache/absent.md") {
		t.Run(ref, func(t *testing.T) {
			status, body := proseBody(t, srv, proseRequest(ref, "1"))
			require.Equal(t, http.StatusNotFound, status)
			require.NotContains(t, body, "super-secret")
		})
	}
}

// TestProseEndpointRefusesUndisplayableFiles pins regular UTF-8 text as the
// only readable object class. A directory, invalid UTF-8, and NUL-bearing
// Markdown are corpus failures and never successful prose payloads.
func TestProseEndpointRefusesUndisplayableFiles(t *testing.T) {
	root, srv := proseFixture(t)
	directory := filepath.Join(root, ".tiller", "design", "02-directory.md")
	require.NoError(t, os.Mkdir(directory, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "design", "03-invalid.md"), []byte{'#', ' ', 0xff, '\n'}, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "design", "04-nul.md"), []byte("# prose\x00payload\n"), 0o644))

	for _, ref := range []string{
		".tiller/design/02-directory.md",
		".tiller/design/03-invalid.md",
		".tiller/design/04-nul.md",
	} {
		t.Run(ref, func(t *testing.T) {
			resp, err := srv.Client().Get(proseURL(srv, ref, "1"))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 500, resp.StatusCode)
			require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
		})
	}
}

// TestProseEndpointRefusesFIFOWithoutReading is deliberately driven directly
// rather than through an HTTP server. A correct handler stats the path and
// answers immediately; an implementation that calls ReadFile first would
// block opening the FIFO. The timeout path opens a writer only to release that
// incorrect read before failing, so this test can never hang the suite.
func TestProseEndpointRefusesFIFOWithoutReading(t *testing.T) {
	root := proseRoot(t)
	const ref = ".tiller/design/05-pipe.md"
	full := filepath.Join(root, filepath.FromSlash(ref))
	require.NoError(t, syscall.Mkfifo(full, 0o600))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, proseRequest(ref, "1"), nil)
	done := make(chan struct{})
	go func() {
		serve.ProseHandler(root).ServeHTTP(recorder, request)
		close(done)
	}()

	select {
	case <-done:
		require.Equal(t, 500, recorder.Code)
	case <-time.After(time.Second):
		writer, err := os.OpenFile(full, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			_, _ = writer.Write([]byte("not prose\n"))
			_ = writer.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
		t.Fatal("prose handler blocked reading a FIFO instead of refusing it after stat")
	}
}

// TestProseEndpointAllowsGETAndHEADOnly pins HEAD as the body's readless twin
// and refuses every mutating method through the shared API method guard.
func TestProseEndpointAllowsGETAndHEADOnly(t *testing.T) {
	root, srv := proseFixture(t)
	const ref = ".tiller/design/01-overview.md"
	writeProse(t, root, ref, "# Overview\n")

	req, err := http.NewRequest(http.MethodHead, proseURL(srv, ref, "1"), nil)
	require.NoError(t, err)
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Empty(t, body)

	resp, err = srv.Client().Post(proseURL(srv, ref, "1"), "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
	require.Equal(t, "GET, HEAD", resp.Header.Get("Allow"))
}

func TestProseBaselineAliasRemainsUnconfirmedAndReadOnly(t *testing.T) {
	root, srv := proseFixture(t)
	enableDriftChecks(t, root, "stale_design")
	const ledger = "docs:\n  - path: as-is/API.MD\n    describes: [app]\n"
	writeProse(t, root, ".tiller/design/as-is/API.MD", "# API\n")
	writeProse(t, root, ".tiller/design/baseline.yml", ledger)
	writeProse(t, root, "docs/as-is/api.md", "# Not a registered design doc\n")
	writeLedgerChore(t, root, "source: docs/as-is/api.md\n")
	for _, ref := range []string{".tiller/design/as-is/API.MD", "design/as-is/API.MD"} {
		var got struct {
			Content  string                 `json:"content"`
			Baseline *views.BaselineDocView `json:"baseline"`
			Drift    *views.NodeDriftView   `json:"drift_detail"`
		}
		require.Equal(t, http.StatusOK, getJSON(t, srv, proseRequest(ref, "1"), &got))
		require.Equal(t, "# API\n", got.Content)
		require.NotNil(t, got.Baseline)
		require.Nil(t, got.Baseline.Confirmed)
		require.NotNil(t, got.Drift)
		require.Empty(t, got.Drift.Candidates)
		require.Len(t, got.Drift.Uncovered, 1)
		require.Equal(t, "unconfirmed", got.Drift.Uncovered[0].Reason)
		require.Equal(t, "as-is/API.MD", got.Drift.Uncovered[0].Key)
		require.Empty(t, got.Drift.Uncovered[0].ID)
	}
	status, unrelated := rawPage(t, srv, proseRequest("docs/as-is/api.md", "1"))
	require.Equal(t, http.StatusOK, status)
	require.NotContains(t, unrelated, "baseline")
	require.NotContains(t, unrelated, "drift_detail")
	after, err := os.ReadFile(filepath.Join(root, ".tiller/design/baseline.yml"))
	require.NoError(t, err)
	require.Equal(t, ledger, string(after))
}

func TestProseBaselineComparisonErrorPreservesConfirmationAndProse(t *testing.T) {
	root, srv := proseFixture(t)
	enableDriftChecks(t, root, "unknown_design_check")
	const ledger = `docs:
  - path: api.md
    describes: [app/main.go]
    derived_from:
      - {kind: git_blob, value: "git:abcdef0", path: docs/retired.md, observed_at: "2026-09-01T10:00:00Z"}
    confirmed:
      by: retired-owner
      commit: {kind: git_commit, value: "git:1234567", observed_at: "2026-09-02T10:00:00Z"}
      surfaces:
        - {path: app/main.go, kind: git_blob, value: "git:abcdef0", inventory: [app/main.go], observed_at: "2026-09-02T10:00:00Z"}
`
	writeProse(t, root, ".tiller/design/api.md", "# Still readable\n")
	writeProse(t, root, ".tiller/design/baseline.yml", ledger)
	var got struct {
		Content  string                 `json:"content"`
		Baseline *views.BaselineDocView `json:"baseline"`
		Drift    *views.NodeDriftView   `json:"drift_detail"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv, proseRequest("design/api.md", "1"), &got))
	require.Equal(t, "# Still readable\n", got.Content)
	require.NotNil(t, got.Baseline)
	require.NotNil(t, got.Baseline.Confirmed)
	require.Equal(t, "retired-owner", got.Baseline.Confirmed.By)
	require.Equal(t, "git:1234567", got.Baseline.Confirmed.Commit.Value)
	require.Equal(t, "2026-09-02T10:00:00Z", got.Baseline.Confirmed.Commit.ObservedAt)
	require.Len(t, got.Baseline.DerivedFrom, 1)
	require.Equal(t, "docs/retired.md", got.Baseline.DerivedFrom[0].Path)
	require.NotNil(t, got.Drift)
	require.Empty(t, got.Drift.Candidates)
	require.Len(t, got.Drift.Uncovered, 1)
	require.Equal(t, "comparison_error", got.Drift.Uncovered[0].Reason)
	require.Contains(t, got.Drift.Uncovered[0].Detail, "unknown_design_check")
	after, err := os.ReadFile(filepath.Join(root, ".tiller/design/baseline.yml"))
	require.NoError(t, err)
	require.Equal(t, ledger, string(after))
}

func TestProseBaselineShowsWorkingTreeDriftWithoutWritingLedger(t *testing.T) {
	root, srv := proseFixture(t)
	writeProse(t, root, ".tiller/tiller.yml", proseTestManifest+"inputs:\n  code:\n    - {root: app, mode: surface}\n")
	enableDriftChecks(t, root, "shifted_design")
	writeProse(t, root, ".tiller/design/api.md", "# API\n")
	writeProse(t, root, "app/main.go", "package app\n")
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.name", "Test")
	gitRun(t, root, "config", "user.email", "test@example.com")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-m", "code before confirmation")
	deps := mutate.Deps{Contributor: "jc", Clock: fixedClock{}, Mint: sequentialMint()}
	tree, err := store.Load(root)
	require.NoError(t, err)
	_, err = mutate.LinkDesign(tree, deps, mutate.DesignLinkInput{Doc: "api.md", Describes: []string{"app/main.go"}})
	require.NoError(t, err)
	tree, err = store.Load(root)
	require.NoError(t, err)
	_, err = mutate.ApproveDesign(tree, deps, []string{"api.md"})
	require.NoError(t, err)
	before, err := os.ReadFile(filepath.Join(root, store.BaselineRelPath))
	require.NoError(t, err)
	writeProse(t, root, "app/main.go", "package app\n// changed but not committed\n")
	var got struct {
		Baseline *views.BaselineDocView `json:"baseline"`
		Drift    *views.NodeDriftView   `json:"drift_detail"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv, proseRequest(".tiller/design/api.md", "1"), &got))
	require.NotNil(t, got.Baseline)
	require.NotNil(t, got.Baseline.Confirmed)
	require.NotNil(t, got.Drift)
	require.Len(t, got.Drift.Candidates, 1)
	candidate := got.Drift.Candidates[0]
	require.Equal(t, "shifted_design", candidate.Marker)
	require.Equal(t, "design", candidate.Noun)
	require.Equal(t, "api.md", candidate.Key)
	require.Empty(t, candidate.ID)
	require.Contains(t, candidate.Files, "app/main.go")
	require.NotNil(t, candidate.Since)
	require.Equal(t, got.Baseline.Confirmed.Commit.Value, candidate.Since.Value)
	require.Contains(t, candidate.Remedy, "tiller approve design")
	after, err := os.ReadFile(filepath.Join(root, store.BaselineRelPath))
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestProseRetiredCoordinateIsNotFound(t *testing.T) {
	for _, kind := range []string{"absent", "stale", "invalid-utf8", "nul", "unreadable", "directory", "outside-symlink", "broken-symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			root, srv := proseFixture(t)
			retired := filepath.Join(root, ".tiller/board.md")
			switch kind {
			case "stale":
				require.NoError(t, os.WriteFile(retired, []byte("stale F-zzzzzz"), 0644))
			case "invalid-utf8":
				require.NoError(t, os.WriteFile(retired, []byte{0xff}, 0644))
			case "nul":
				require.NoError(t, os.WriteFile(retired, []byte("bad\x00text"), 0644))
			case "unreadable":
				require.NoError(t, os.WriteFile(retired, []byte("unreadable"), 0000))
			case "directory":
				require.NoError(t, os.Mkdir(retired, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(retired, "nested.md"), []byte("nested"), 0644))
			case "outside-symlink", "broken-symlink":
				target := filepath.Join(t.TempDir(), "outside.md")
				if kind == "outside-symlink" {
					require.NoError(t, os.WriteFile(target, []byte("outside"), 0644))
				}
				require.NoError(t, os.Symlink(target, retired))
			case "fifo":
				require.NoError(t, syscall.Mkfifo(retired, 0600))
			}
			srv.Client().Timeout = 2 * time.Second
			for _, ref := range []string{".tiller/board.md", ".tiller/board.md/nested.md"} {
				resp, err := srv.Client().Get(proseURL(srv, ref, "1"))
				require.NoError(t, err)
				require.Equal(t, http.StatusNotFound, resp.StatusCode)
				require.NoError(t, resp.Body.Close())
			}
			writeProse(t, root, ".tiller/design/board.md", "authored")
			resp, err := srv.Client().Get(proseURL(srv, ".tiller/design/board.md", "1"))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

// TestProseEndpointServesOnlyCorpusAndCitedDocuments pins the network-safe
// scope: the reader serves the .tiller/ Markdown corpus and any other
// Markdown a ledger record cites, nothing else. Every refused file exists, is
// well-formed Markdown, and sits beside one that serves, so removing the scope
// check turns each refusal into a 200 carrying its secret.
func TestProseEndpointServesOnlyCorpusAndCitedDocuments(t *testing.T) {
	root, srv := proseFixture(t)
	writeLedgerChore(t, root, "source: docs/cited.md#L1\n")
	writeProse(t, root, "docs/cited.md", "# Cited\n")
	writeProse(t, root, ".tiller/design/01-corpus.md", "# Corpus\n")
	refused := []string{"docs/uncited.md", "README.md", ".claude/notes.md", ".github/secret.md"}
	for _, ref := range refused {
		writeProse(t, root, ref, "# super-secret\n")
	}

	for _, ref := range refused {
		t.Run(ref, func(t *testing.T) {
			status, body := proseBody(t, srv, proseRequest(ref, "1"))
			require.Equal(t, http.StatusNotFound, status)
			require.NotContains(t, body, "super-secret")
		})
	}
	for _, ref := range []string{"docs/cited.md", ".tiller/design/01-corpus.md", "design/01-corpus.md"} {
		t.Run(ref, func(t *testing.T) {
			status, _ := proseBody(t, srv, proseRequest(ref, "1"))
			require.Equal(t, http.StatusOK, status)
		})
	}
}

// TestProseEndpointRefusesCitedHiddenDocuments pins hidden-segment exclusion
// ahead of citation admission (bug hunt defect 07, a cited hidden Markdown
// file is served): a ledger citation can widen the reader to an outside
// document, never to a dot-prefixed path, and never into .tiller/'s hidden
// machine state. The ordinary cited document beside them still serves.
func TestProseEndpointRefusesCitedHiddenDocuments(t *testing.T) {
	for _, ref := range []string{".notes/secret.md", "docs/.hidden/plan.md", ".tiller/.cache/state.md"} {
		t.Run(ref, func(t *testing.T) {
			root, srv := proseFixture(t)
			writeLedgerChore(t, root, "source: "+ref+"#L1\n")
			writeProse(t, root, ref, "# super-secret\n")
			status, body := proseBody(t, srv, proseRequest(ref, "1"))
			require.Equal(t, http.StatusNotFound, status)
			require.NotContains(t, body, "super-secret")
		})
	}
	root, srv := proseFixture(t)
	writeLedgerChore(t, root, "source: docs/cited.md#L1\n")
	writeProse(t, root, "docs/cited.md", "# Cited\n")
	status, _ := proseBody(t, srv, proseRequest("docs/cited.md", "1"))
	require.Equal(t, http.StatusOK, status)
}

// TestProseEndpointRefusesOversizedFile pins the shared read cap at the prose
// reader: a corpus document over the served-file limit answers 413 rather
// than being read whole into memory.
func TestProseEndpointRefusesOversizedFile(t *testing.T) {
	root, srv := proseFixture(t)
	const ref = ".tiller/design/huge.md"
	writeProse(t, root, ref, strings.Repeat("a", serve.MaxServedFileBytes+1))

	status, _ := proseBody(t, srv, proseRequest(ref, "1"))
	require.Equal(t, http.StatusRequestEntityTooLarge, status)
}
