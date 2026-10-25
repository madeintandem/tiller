package serve_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func observationGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	return string(output)
}

func observationFile(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

const observationTaskPath = ".tiller/epics/work/features/build/tasks/one/task.yml"

func observationFixture(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()
	root := t.TempDir()
	observationGit(t, root, "init", "-q", "-b", "main")
	observationGit(t, root, "config", "user.name", "Smoke")
	observationGit(t, root, "config", "user.email", "smoke@example.test")
	observationFile(t, root, ".tiller/tiller.yml", serveManifest+"git:\n  integration_branch: main\n")
	observationFile(t, root, ".tiller/epics/work/epic.yml", "id: E-aaaaaa\nkey: work\ntitle: Work\nstatus: ready\n")
	observationFile(t, root, ".tiller/epics/work/features/build/feature.yml", "id: F-bbbbbb\nkey: work/build\ntitle: Build\nstatus: ready\n")
	observationFile(t, root, observationTaskPath, "id: T-cccccc\nkey: work/build/one\ntitle: One\nstatus: ready\n")
	observationFile(t, root, ".tiller/epics/work/features/build/tasks/one/task.md", "# One\n\nImplement the first task.\n")
	observationGit(t, root, "add", ".")
	observationGit(t, root, "commit", "-qm", "approved plan")
	worktree := filepath.Join(t.TempDir(), "execution")
	observationGit(t, root, "worktree", "add", "-q", "-b", "tiller/work/build/one", worktree)
	srv := httptest.NewServer(serve.Handler(root, nil, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)
	return srv, root, worktree
}

func TestConsoleObservesLifecycleWithoutChangingIntegration(t *testing.T) {
	srv, root, worktree := observationFixture(t)
	original, err := os.ReadFile(filepath.Join(root, observationTaskPath))
	require.NoError(t, err)
	for _, status := range []string{"in_progress", "check", "in_review", "in_progress"} {
		t.Run(status, func(t *testing.T) {
			observationFile(t, worktree, observationTaskPath, fmt.Sprintf("id: T-cccccc\nkey: work/build/one\ntitle: One\nstatus: %s\nstarted: true\nowner: jc\nbranch: tiller/work/build/one\n", status))
			var page struct {
				Node      views.NodeView `json:"node"`
				Execution struct {
					Source            string `json:"source"`
					IntegrationStatus string `json:"integration_status"`
				} `json:"execution"`
				Workflow any `json:"workflow"`
			}
			require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/task/T-cccccc", &page))
			require.Equal(t, status, page.Node.Status)
			require.Equal(t, "jc", page.Node.Owner)
			require.Equal(t, "working_tree", page.Execution.Source)
			require.Equal(t, "ready", page.Execution.IntegrationStatus)
			require.Nil(t, page.Workflow)
			var list struct {
				Nodes []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"nodes"`
			}
			require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/ls?noun=tasks&status="+status, &list))
			require.Equal(t, []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}{{"T-cccccc", status}}, list.Nodes)
			var board struct {
				Unclaimed []struct {
					ID string `json:"id"`
				} `json:"unclaimed"`
			}
			require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/board", &board))
			for _, task := range board.Unclaimed {
				require.NotEqual(t, "T-cccccc", task.ID)
			}
		})
	}
	unchanged, err := os.ReadFile(filepath.Join(root, observationTaskPath))
	require.NoError(t, err)
	require.Equal(t, original, unchanged)
	require.Empty(t, observationGit(t, root, "status", "--porcelain"))
}

func TestConsoleEvidenceUsesSelectedExecutionSourceAndRejectsStaleLinks(t *testing.T) {
	srv, root, worktree := observationFixture(t)
	const evidenceRef = "artifacts/tiller/gates/build.log"
	observationFile(t, root, evidenceRef, "integration log must not be served")
	observationFile(t, worktree, evidenceRef, "execution build passed")
	observationFile(t, worktree, observationTaskPath, "id: T-cccccc\nkey: work/build/one\ntitle: One\nstatus: check\nstarted: true\nowner: jc\nbranch: tiller/work/build/one\ngates:\n  build:\n    required: true\n    status: pass\n    evidence:\n      - {kind: command, ref: artifacts/tiller/gates/build.log}\n  tests:\n    required: true\n    status: pass\n    evidence:\n      - {kind: command, ref: artifacts/tiller/gates/missing.log}\n")
	var page struct {
		Node views.NodeView `json:"node"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/task/T-cccccc", &page))
	link := page.Node.Gates["build"].Evidence[0].URL
	require.Contains(t, link, "task=T-cccccc")
	require.NotEmpty(t, page.Node.Gates["tests"].Evidence[0].Unavailable)
	response, err := srv.Client().Get(srv.URL + link)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "execution build passed", string(body))
	unrecorded, err := url.Parse(link)
	require.NoError(t, err)
	query := unrecorded.Query()
	query.Set("ref", "unrecorded.txt")
	unrecorded.RawQuery = query.Encode()
	observationFile(t, worktree, "unrecorded.txt", "not a recorded gate artifact")
	require.Equal(t, http.StatusBadRequest, getJSON(t, srv, unrecorded.String(), nil))
	observationFile(t, worktree, evidenceRef, "rewritten after detail was observed")
	require.Equal(t, http.StatusConflict, getJSON(t, srv, link, nil))
	observationFile(t, worktree, observationTaskPath, "id: T-cccccc\nkey: work/build/one\nstatus: in_progress\nstarted: true\nowner: jc\nbranch: tiller/work/build/one\n")
	require.Equal(t, http.StatusConflict, getJSON(t, srv, link, nil))
}

// TestConsoleExecutionEvidenceRefusesOversizedFiles pins the served-file cap
// on execution-source evidence for both source kinds. An oversized file is
// refused with 413 before its digest is compared, so a request carrying any
// well-formed digest cannot force a whole-file read; the node page reports
// the file unavailable rather than hashing it to mint a link.
func TestConsoleExecutionEvidenceRefusesOversizedFiles(t *testing.T) {
	const smallRef = "artifacts/tiller/gates/small.log"
	const hugeRef = "artifacts/tiller/gates/huge.log"
	task := "id: T-cccccc\nkey: work/build/one\ntitle: One\nstatus: check\nstarted: true\nowner: jc\nbranch: tiller/work/build/one\ngates:\n  build:\n    required: true\n    status: pass\n    evidence:\n      - {kind: command, ref: " + smallRef + "}\n      - {kind: command, ref: " + hugeRef + "}\n"
	for _, source := range []string{"working_tree", "local_branch"} {
		t.Run(source, func(t *testing.T) {
			srv, root, worktree := observationFixture(t)
			observationFile(t, worktree, smallRef, "small log")
			observationFile(t, worktree, hugeRef, strings.Repeat("a", serve.MaxServedFileBytes+1))
			observationFile(t, worktree, observationTaskPath, task)
			if source == "local_branch" {
				observationGit(t, worktree, "add", "-f", ".")
				observationGit(t, worktree, "commit", "-qm", "record evidence")
				observationGit(t, root, "worktree", "remove", worktree)
			}
			var page struct {
				Node      views.NodeView `json:"node"`
				Execution struct {
					Source string `json:"source"`
				} `json:"execution"`
			}
			require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/task/T-cccccc", &page))
			require.Equal(t, source, page.Execution.Source)
			evidence := page.Node.Gates["build"].Evidence
			require.Empty(t, evidence[1].URL)
			require.Contains(t, evidence[1].Unavailable, "too large")
			link, err := url.Parse(evidence[0].URL)
			require.NoError(t, err)
			query := link.Query()
			query.Set("ref", hugeRef)
			query.Set("digest", strings.Repeat("0", 64))
			link.RawQuery = query.Encode()
			require.Equal(t, http.StatusRequestEntityTooLarge, getJSON(t, srv, link.String(), nil))
		})
	}
}

func TestConsoleTaskHistoryReadsExecutionEventsAndDisclosesFailure(t *testing.T) {
	srv, _, worktree := observationFixture(t)
	observationFile(t, worktree, observationTaskPath, "id: T-cccccc\nkey: work/build/one\nstatus: in_progress\nstarted: true\nowner: jc\nbranch: tiller/work/build/one\n")
	const eventPath = ".tiller/events/2026/10/06/EV-dddddd.yml"
	observationFile(t, worktree, eventPath, "id: EV-dddddd\ntype: start.task\nat: 2026-10-06T12:00:00Z\ncontributor: jc\ncommand: tiller start task T-cccccc\ntargets: [T-cccccc]\n")
	var page struct {
		Node         views.NodeView       `json:"node"`
		History      []views.HistoryEntry `json:"history"`
		HistoryError string               `json:"execution_history_error"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/task/T-cccccc", &page))
	require.Equal(t, []views.HistoryEntry{{At: "2026-10-06T12:00:00Z", Type: "start.task", Contributor: "jc", Command: "tiller start task T-cccccc"}}, page.History)
	observationFile(t, worktree, eventPath, "broken: [")
	page.History = nil
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/task/T-cccccc", &page))
	require.Equal(t, "in_progress", page.Node.Status)
	require.Contains(t, page.HistoryError, "EV-dddddd")
	require.Empty(t, page.History)
}

func TestObservedParentActivityAgreesWithoutAdvancingParentLifecycle(t *testing.T) {
	srv, _, worktree := observationFixture(t)
	observationFile(t, worktree, observationTaskPath, "id: T-cccccc\nkey: work/build/one\nstatus: in_progress\nstarted: true\nowner: jc\nbranch: tiller/work/build/one\n")
	var page struct {
		Node views.NodeView `json:"node"`
	}
	var list struct {
		Nodes []views.ListItem `json:"nodes"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/epic/E-aaaaaa", &page))
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/ls?noun=epics", &list))
	require.Equal(t, "active", page.Node.Activity)
	require.Equal(t, "ready", page.Node.Status)
	require.Equal(t, page.Node.Activity, list.Nodes[0].Activity)
	require.Equal(t, "ready", list.Nodes[0].Status)
}
