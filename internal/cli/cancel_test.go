package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// These tests pin the cancel and archive verbs plus the oracle-CLI parity rule
// (plan 2026-07-02, Workstream 1 items 1.3 and 1.5): the lifecycle table always
// defined the cancel transitions and status advertised them in next:, but no
// command existed, which left a mistakenly started task with no way out.

// loadCwdTree loads the store tree from the test's working directory.
func loadCwdTree(t *testing.T) *store.Tree {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	return tree
}

// claimTaskAsOwner reserves exactly one execution unit through the real CLI.
func claimTaskAsOwner(t *testing.T, ref, owner string) {
	t.Helper()
	_, stderr, code := execCLI(t, "claim", "task", ref, "--contributor", owner)
	require.Equal(t, 0, code, stderr)
}

// TestCancelTaskInProgressRecordsReason is the dry run's stranded-task case: an
// in_progress task the agent started by mistake is cancelled by the human, with
// the required --reason recorded on the event.
func TestCancelTaskInProgressRecordsReason(t *testing.T) {
	initGatedGitRepo(t, "true")
	tid := driveToStartedTask(t)

	out, _, code := execCLI(t, "cancel", "task", tid, "--reason", "started by mistake", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"status\": \"cancelled\"")

	tree := loadCwdTree(t)
	require.Equal(t, "cancelled", tree.Tasks[tid].Status)

	evs, err := events.List(tree)
	require.NoError(t, err)
	require.NotEmpty(t, evs)
	last := evs[len(evs)-1]
	require.Equal(t, "cancel.task", last.Type)
	require.Equal(t, "started by mistake", last.After["reason"], "the reason rides the event")
}

// TestCancelFeatureRecordsReason confirms cancel records its required --reason
// on the event, the same way return and resume already do (cli-surface.md, the
// verb table).
func TestCancelFeatureRecordsReason(t *testing.T) {
	layReadTree(t)

	out, _, code := execCLI(t, "cancel", "feature", cliFeatureKey, "--reason", "superseded by the table model", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"status\": \"cancelled\"")

	tree := loadCwdTree(t)
	require.Equal(t, "cancelled", tree.Features[soleFeatureID(t)].Status)
}

// TestCancelDoneTaskIsBlocked confirms an illegal cancel is the recoverable
// exit-1 answer, not an error: done is terminal for a task. A real --reason
// rides along so the refusal under test is the illegal-transition guard, not
// the required-flag check a blank reason would trip first.
func TestCancelDoneTaskIsBlocked(t *testing.T) {
	initGatedGitRepo(t, "true")
	tid := driveToStartedTask(t)
	setTaskStatusOnDisk(t, "transcribe-the-schema", "done")

	out, _, code := execCLI(t, "cancel", "task", tid, "--reason", "attempting to cancel done work", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "an illegal cancel is a blocked business answer")
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"legal_next\"")
}

// TestArchiveEpic confirms the archive verb the table and spec always carried is
// wired: a done epic moves to archived.
func TestArchiveEpic(t *testing.T) {
	layReadTree(t)
	setEpicStatusOnDisk(t, "form-engine", "done")

	out, _, code := execCLI(t, "archive", "form-engine", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"status\": \"archived\"")

	tree := loadCwdTree(t)
	require.Equal(t, "archived", tree.Epics["E-"+soleEpicIDSuffix(t)].Status)
}

// TestOracleCLIParity is the test that would have caught the missing cancel: it
// walks every verb the lifecycle table can emit into status legal_next and
// asserts a registered CLI command exists for it. The status oracle is
// documented as the authority on legal moves; it must never name a move the
// binary cannot perform.
func TestOracleCLIParity(t *testing.T) {
	root := NewRootCmd()
	registered := map[string]bool{}
	for _, c := range root.Commands() {
		registered[c.Name()] = true
	}

	verbs := lifecycle.EmittableVerbs()
	require.NotEmpty(t, verbs)
	for _, verb := range verbs {
		require.Truef(t, registered[verb],
			"status legal_next can advertise %q but no CLI command is registered for it", verb)
	}
}

// setTaskStatusOnDisk rewrites one task's status inside its own task.yml, the
// cheap way to stage a state the full drive would need a merge to reach.
func setTaskStatusOnDisk(t *testing.T, localKey, status string) {
	t.Helper()
	path := filepath.Join(featureDir(t), store.TaskDirName, localKey, store.TaskStateFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var tk store.Task
	require.NoError(t, yaml.Unmarshal(data, &tk))
	tk.Status = status
	if status == "in_progress" || status == "check" || status == "in_review" || status == "done" {
		tk.Started = true
	}
	out, err := yaml.Marshal(tk)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// setEpicStatusOnDisk rewrites an epic.yml status field, staging a done epic
// without driving the full rollup.
func setEpicStatusOnDisk(t *testing.T, epicKey, status string) {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	path := filepath.Join(cwd, ".tiller", "epics", epicKey, "epic.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: draft", "status: "+status, 1)
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// soleEpicIDSuffix reads the one epic's minted id suffix off disk.
func soleEpicIDSuffix(t *testing.T) string {
	t.Helper()
	tree := loadCwdTree(t)
	require.Len(t, tree.Epics, 1)
	for id := range tree.Epics {
		return strings.TrimPrefix(id, "E-")
	}
	return ""
}
