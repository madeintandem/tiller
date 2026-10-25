package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests pin the human node-reference convention (issue #10): human output
// names a node by its key path with the id in parentheses, and a task's key
// path is its own three-segment key. The degenerate "task T-x (T-x)" the old
// key-else-id fallbacks produced must never render. Minted ids are random, so
// assertions bracket them.

// firstTaskKey is the canonical key of the task prepActiveFeatureWithTasks adds
// first, the node these reference tests walk.
const firstTaskKey = "form-engine/nrca-form-loading/schema-transcription"

// TestTaskRefsCarryTheirOwnKeyPath walks one task through add, approve, status,
// show, and ls, asserting every human surface names it by its own canonical key
// path plus its id, with the add confirmation and status header echoing the
// title.
func TestTaskRefsCarryTheirOwnKeyPath(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)

	// The add confirmation names the task by its own path and echoes the
	// recorded title.
	out, _, code := execCLI(t, "add", "task", "form-engine/nrca-form-loading/wire-the-cli-shell",
		"--title", "Wire the CLI shell", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Created task form-engine/nrca-form-loading/wire-the-cli-shell (T-")
	require.Contains(t, out, `) in proposed: "Wire the CLI shell"`)

	// A transition confirmation names the task the same way.
	out, _, code = execCLI(t, "approve", "task", ids[0], "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "task "+firstTaskKey+" (T-")
	require.Contains(t, out, ") is now ready.")

	// The status oracle's header carries the task's own path and its title, and
	// the degenerate "T-x (T-x)" shape cannot appear.
	out, _, code = execCLI(t, "status", "task", ids[0])
	require.Equal(t, 0, code)
	require.Contains(t, out, fmt.Sprintf("task %s (%s): %q", firstTaskKey, ids[0], "Transcribe the schema"))
	require.NotContains(t, out, fmt.Sprintf("%s (%s)", ids[0], ids[0]))

	// show renders the same header reference.
	out, _, code = execCLI(t, "show", "task", ids[0])
	require.Equal(t, 0, code)
	require.Contains(t, out, fmt.Sprintf("task %s (%s)", firstTaskKey, ids[0]))
	require.NotContains(t, out, fmt.Sprintf("%s (%s)", ids[0], ids[0]))

	// ls rows carry the reference in the label column.
	out, _, code = execCLI(t, "ls", "tasks")
	require.Equal(t, 0, code)
	require.Contains(t, out, firstTaskKey+" (T-")
}

// TestBlockedLinesNameTheNodeByRef pins the illegal-transition report, which
// before issue #10 named no node at all: it now carries the full reference of
// the node it refuses.
func TestBlockedLinesNameTheNodeByRef(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)

	// The tasks are proposed; start is illegal from there.
	stdout, stderr, code := execCLI(t, "start", "task", ids[1], "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, stdout+stderr, fmt.Sprintf("Blocked: cannot start task form-engine/nrca-form-loading/form-render (%s) from %q.", ids[1], "proposed"))
}

// TestStatusTaskJSONCarriesTitleNotRef pins the status oracle's title field for
// tasks and the leak guard beside it: the human-only Ref never reaches the JSON
// payload, which keeps carrying id and key structurally rather than composed
// labels.
func TestStatusTaskJSONCarriesTitleNotRef(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)

	out, _, code := execCLI(t, "status", "task", ids[0], "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"title": "Transcribe the schema"`)
	require.Contains(t, out, fmt.Sprintf(`"id": %q`, ids[0]))
	require.NotContains(t, out, `"ref"`)
	require.NotContains(t, out, fmt.Sprintf("%s (%s)", firstTaskKey, ids[0]),
		"composed human labels never enter the machine payload")
}

// TestQuestionRefsCarryTitleAndFullStatement pins both sides of the question
// contract: scans use the persisted or bounded legacy title exactly once,
// while status/show keep the full statement on the subject-specific body.
func TestQuestionRefsCarryTitleAndFullStatement(t *testing.T) {
	t.Run("persisted title", func(t *testing.T) {
		prepActiveFeatureWithTasks(t)
		statement := strings.Repeat("This is the full titled question statement. ", 6)
		added, _, code := execCLI(t, "add", "question", statement,
			"--title", "Canonical schema", "--contributor", "jc", "--json")
		require.Equal(t, 0, code)
		qid := extractID(t, added, "Q-")

		status, _, code := execCLI(t, "status", "question", qid)
		require.Equal(t, 0, code)
		require.Contains(t, status, `question `+qid+`: "Canonical schema"`)
		require.Contains(t, status, "statement: "+statement)

		show, _, code := execCLI(t, "show", "question", qid)
		require.Equal(t, 0, code)
		require.Contains(t, show, `question `+qid+`: "Canonical schema"`)
		require.Contains(t, show, "statement: "+statement)

		list, _, code := execCLI(t, "ls", "questions")
		require.Equal(t, 0, code)
		require.Equal(t, 1, strings.Count(list, "Canonical schema"))
		require.NotContains(t, list, statement)

		jsonList, _, code := execCLI(t, "ls", "questions", "--json")
		require.Equal(t, 0, code)
		require.Contains(t, jsonList, `"title": "Canonical schema"`)
		require.Contains(t, jsonList, `"statement": `)
		require.Contains(t, jsonList, statement)
	})

	t.Run("legacy bounded fallback", func(t *testing.T) {
		prepActiveFeatureWithTasks(t)
		statement := strings.Repeat("界", 81)
		fallback := strings.Repeat("界", 79) + "…"
		writeLegacyQuestion(t, "Q-k3x9w1", statement)

		status, _, code := execCLI(t, "status", "question", "Q-k3x9w1")
		require.Equal(t, 0, code)
		require.Contains(t, status, `question Q-k3x9w1: "`+fallback+`"`)
		require.Contains(t, status, "statement: "+statement)

		show, _, code := execCLI(t, "show", "question", "Q-k3x9w1")
		require.Equal(t, 0, code)
		require.Contains(t, show, `question Q-k3x9w1: "`+fallback+`"`)
		require.Contains(t, show, "statement: "+statement)

		list, _, code := execCLI(t, "ls", "questions")
		require.Equal(t, 0, code)
		require.Equal(t, 1, strings.Count(list, fallback))
		require.NotContains(t, list, statement)

		jsonList, _, code := execCLI(t, "ls", "questions", "--json")
		require.Equal(t, 0, code)
		require.Contains(t, jsonList, `"title": "`+fallback+`"`)
		require.Contains(t, jsonList, statement)
	})
}

func writeLegacyQuestion(t *testing.T, id, statement string) {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	dir := filepath.Join(cwd, ".tiller", "decisions")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	body := "questions:\n  - id: " + id + "\n    statement: " + statement + "\n    status: open\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "open-questions.yml"), []byte(body), 0o644))
}

// TestCheckLineNamesTaskByRef pins the check confirmation's head, which used to
// render a bare task id: the node is named by its own key path.
func TestCheckLineNamesTaskByRef(t *testing.T) {
	initGatedGitRepo(t, "true")
	tid := driveToStartedTask(t)

	out, _, code := execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, fmt.Sprintf("Checked task form-engine/nrca-form-loading/transcribe-the-schema (%s): all required gates green", tid))
}

// TestFoundExistingAddEchoesTitle pins the idempotent re-add arm: the same
// reference and title echo as the created arm, under the Found existing verb.
func TestFoundExistingAddEchoesTitle(t *testing.T) {
	prepActiveFeatureWithTasks(t)

	out, _, code := execCLI(t, "add", "task", firstTaskKey,
		"--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Found existing task "+firstTaskKey+" (T-")
	require.Contains(t, out, `) in proposed: "Transcribe the schema"`)
}
