package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

const movingTaskPath = ".tiller/epics/work/features/build/tasks/one/task.yml"

func movingGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Smoke", "GIT_AUTHOR_EMAIL=smoke@example.test", "GIT_COMMITTER_NAME=Smoke", "GIT_COMMITTER_EMAIL=smoke@example.test")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
}

func movingFile(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// An agent committing in its task worktree while a console read runs must not
// fail the read: the moved observation is dropped and the projection reruns
// over integration's value for that task.
func TestAPISettlesAMovedExecutionSourceInsteadOfFailing(t *testing.T) {
	root := t.TempDir()
	movingGit(t, root, "init", "-q", "-b", "main")
	movingFile(t, root, ".tiller/tiller.yml", "schema_version: 12\nschema_epoch: initial\nproject: {name: Moving, key: moving, kind: greenfield}\ncontributors:\n  - {id: jc, name: JC}\ngit: {integration_branch: main}\n")
	movingFile(t, root, ".tiller/epics/work/epic.yml", "id: E-aaaaaa\nkey: work\nstatus: ready\n")
	movingFile(t, root, ".tiller/epics/work/features/build/feature.yml", "id: F-bbbbbb\nkey: work/build\nstatus: ready\n")
	movingFile(t, root, movingTaskPath, "id: T-cccccc\nkey: work/build/one\nstatus: ready\n")
	movingGit(t, root, "add", ".")
	movingGit(t, root, "commit", "-qm", "approved plan")
	worktree := filepath.Join(t.TempDir(), "execution")
	movingGit(t, root, "worktree", "add", "-q", "-b", "tiller/work/build/one", worktree)
	movingFile(t, worktree, movingTaskPath, "id: T-cccccc\nkey: work/build/one\nstatus: in_progress\nstarted: true\nowner: jc\nbranch: tiller/work/build/one\n")

	var seen []string
	committed := false
	handler := api(root, func(tree *store.Tree, _ *http.Request) (any, error) {
		seen = append(seen, tree.Tasks["T-cccccc"].Status)
		if !committed {
			committed = true
			movingGit(t, worktree, "add", ".")
			movingGit(t, worktree, "commit", "-qm", "agent commits during the read")
		}
		return map[string]string{"status": tree.Tasks["T-cccccc"].Status}, nil
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/test", nil))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, []string{"in_progress", "ready"}, seen)
	var body map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "ready", body["status"])

	// The next read observes the committed branch again.
	seen = nil
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/test", nil))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, []string{"in_progress"}, seen)
}
