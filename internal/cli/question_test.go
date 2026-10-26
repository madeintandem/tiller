package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// gitifyCwd turns the current fixture directory into a committed git repo, the
// environment answer question needs to record the answer revision (the same
// provenance rule the gates use).
func gitifyCwd(t *testing.T) {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	testgit.Init(t, dir)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "fixture")
}

func setFeatureBlockedOnDisk(t *testing.T) {
	t.Helper()
	path := filepath.Join(featureDir(t), "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var feature store.Feature
	require.NoError(t, yaml.Unmarshal(data, &feature))
	feature.Blocked = true
	out, err := yaml.Marshal(feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// These tests pin the question ledger's write half at the CLI surface (issue
// #19): add question mints into the ledger the blocks machinery
// read, answer question closes it, and the arc a real blocker travels
// (recorded, refusing start, assumed past or answered) works end to end.

// TestAddQuestionRecordsAndEchoes pins the create: the confirmation names the
// question by id plus statement (its reference form) and the blocked nodes by
// the shared convention; the JSON envelope carries the resolved ids.
func TestAddQuestionRecordsAndEchoes(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)

	out, _, code := execCLI(t, "add", "question", "Where does the design system live?",
		"--title", "Design system location",
		"--blocks", "form-engine/certification-form-loading", "--blocks", ids[0],
		"--due", "2026-07-12", "--source", "briefing", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, `Created question Q-`)
	require.Contains(t, out, `: "Where does the design system live?"`)
	require.Contains(t, out, "blocks form-engine/certification-form-loading (F-")
	require.Contains(t, out, fmt.Sprintf("form-engine/certification-form-loading/schema-transcription (%s)", ids[0]),
		"a blocked task is named by its own key path")

	// The ledger loads back through every read surface.
	out, _, code = execCLI(t, "ls", "questions")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Design system location")
	require.NotContains(t, out, "Where does the design system live?")

	out, _, code = execCLI(t, "add", "question", "Where does the design system live?", "--title", "Design system location", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Found existing question Q-", "re-adding the same statement is a no-op success")

	// JSON: resolved ids, open status.
	out, _, code = execCLI(t, "add", "question", "Where does the design system live?", "--title", "Design system location", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"status": "open"`)
	require.Contains(t, out, `"created": false`)
}

// TestAddQuestionRefusesDanglingBlocks pins the all-or-nothing address rule at
// the surface: one bad blocks target is misuse with nothing recorded.
func TestAddQuestionRefusesDanglingBlocks(t *testing.T) {
	prepActiveFeatureWithTasks(t)

	out, _, code := execCLI(t, "add", "question", "Blocked on a ghost?", "--title", "Ghost blocker", "--blocks", "F-zzzzzz", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "not_found"`)

	out, _, code = execCLI(t, "ls", "questions")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "ghost", "nothing recorded on a refused add")
}

// TestQuestionBlocksArcEndToEnd is the issue's whole point: a recorded
// question holds its node at start, --assume-question is the explicit
// escape hatch, and answering clears the blocker for good.
func TestQuestionBlocksArcEndToEnd(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	setFeatureBlockedOnDisk(t)
	gitifyCwd(t)
	claimTaskAsOwner(t, ids[0], "jc")
	mustRun(t, "approve", "task", ids[0], "--contributor", "jc")

	out, _, code := execCLI(t, "add", "question", "Which schema version is canonical?",
		"--title", "Canonical schema version",
		"--blocks", ids[0], "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	qid := extractID(t, out, "Q-")

	require.True(t, loadTestTree(t).Features[soleFeatureID(t)].Blocked,
		"adding a question must preserve the feature's independent manual condition")

	// The open question holds the task.
	stdout, stderr, code := execCLI(t, "start", "task", ids[0], "--contributor", "jc")
	require.Equal(t, 1, code, "an open question blocks start")
	require.Contains(t, stdout+stderr, "is held by 1 open question(s)")

	// The oracle names the way forward on the question itself.
	out, _, code = execCLI(t, "status", "question", qid)
	require.Equal(t, 0, code)
	require.Contains(t, out, fmt.Sprintf("question %s: %q", qid, "Canonical schema version"))
	require.Contains(t, out, "statement: Which schema version is canonical?")
	require.Contains(t, out, "next:  answer")

	// Answering clears the computed blocker; start proceeds. The confirmation
	// names the question by its reference, the id plus its statement.
	out, _, code = execCLI(t, "answer", "question", qid, "--answer", "The 2025 revision.", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, fmt.Sprintf("question %s (%q): %q is now answered.", qid, "Canonical schema version", "Which schema version is canonical?"))

	// Answering removes only the computed task condition.
	require.True(t, loadTestTree(t).Features[soleFeatureID(t)].Blocked,
		"answering the task question must preserve the feature's manual condition")

	out, _, code = execCLI(t, "start", "task", ids[0], "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, ") is now in_progress.")

	// A direct task question remains operative until answered.
	mustRun(t, "approve", "task", ids[1], "--contributor", "jc")
	claimTaskAsOwner(t, ids[1], "jc")
	out, _, code = execCLI(t, "add", "question", "Is the queue FIFO or priority?", "--title", "Queue ordering", "--blocks", ids[1], "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	q2 := extractID(t, out, "Q-")

	_, _, code = execCLI(t, "start", "task", ids[1], "--contributor", "jc")
	require.Equal(t, 1, code, "the open question holds the second task")

	mustRun(t, "answer", "question", q2, "--answer", "FIFO.", "--contributor", "jc")
	out, _, code = execCLI(t, "start", "task", ids[1], "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, ") is now in_progress.")
}

// TestAnswerQuestionGuards pins the re-answer semantics at the surface: the
// same text again is a no-change success, a different text is a blocked
// business answer, and a missing --answer is misuse.
func TestAnswerQuestionGuards(t *testing.T) {
	prepActiveFeatureWithTasks(t)
	gitifyCwd(t)
	out, _, code := execCLI(t, "add", "question", "Which schema version is canonical?", "--title", "Canonical schema version", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	qid := extractID(t, out, "Q-")

	mustRun(t, "answer", "question", qid, "--answer", "The 2025 revision.", "--contributor", "jc")

	out, _, code = execCLI(t, "answer", "question", qid, "--answer", "The 2025 revision.", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, `("Canonical schema version"): "Which schema version is canonical?" is already answered (no change).`)

	stdout, stderr, code := execCLI(t, "answer", "question", qid, "--answer", "Actually 2022.", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "a different answer is a blocked business refusal")
	require.Contains(t, stdout+stderr, `"reason": "already_answered"`)

	_, _, code = execCLI(t, "answer", "task", qid, "--answer", "x", "--contributor", "jc")
	require.Equal(t, 2, code, "answer applies to questions only")
}

// TestAddQuestionDueForms pins due date and hint storage on the ledger.
func TestAddQuestionDueForms(t *testing.T) {
	prepActiveFeatureWithTasks(t)
	gitifyCwd(t)

	_, _, code := execCLI(t, "add", "question", "Due by a date?", "--title", "Date deadline", "--due", "2026-08-15", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	ledger := readFile(t, ".tiller", "decisions", "open-questions.yml")
	require.Contains(t, ledger, "date: \"2026-08-15\"", "the due date is stored on the ledger")

	mustRun(t, "add", "question", "Due by a hint?", "--title", "Hint deadline", "--due-hint", "before specify", "--contributor", "jc")
	ledger = readFile(t, ".tiller", "decisions", "open-questions.yml")
	require.Contains(t, ledger, "hint: before specify")
}

// TestAddQuestionRefusesNonBlockableTargetsCLI pins the misuse surface for a
// blocks target outside the work tree.
func TestAddQuestionRefusesNonBlockableTargetsCLI(t *testing.T) {
	prepActiveFeatureWithTasks(t)

	stdout, stderr, code := execCLI(t, "add", "question", "Held by a question?", "--title", "Non-blockable target", "--blocks", "Q-aaaaaa", "--contributor", "jc")
	require.NotEqual(t, 0, code)
	require.Contains(t, stdout+stderr, "a question blocks an epic, a feature, or a task")
}

// TestShowQuestionJSONCarriesSourceAndBlocks pins issue #55 at the CLI
// surface: `show question --json` renders a question's Source citation and
// its Blocks list, both already recorded on the ledger through add question's
// own --source and --blocks flags but previously dropped from the read
// envelope entirely, leaving an agent reading the machine surface unable to
// see why a question exists or what it holds. A blockless, sourceless
// question renders neither key (views.NodeView's omitempty).
func TestShowQuestionJSONCarriesSourceAndBlocks(t *testing.T) {
	prepActiveFeatureWithTasks(t)

	out, _, code := execCLI(t, "add", "question", "Keep or trim the legacy image parsers?",
		"--title", "Legacy image parsers",
		"--blocks", "form-engine/certification-form-loading",
		"--source", "briefing/2026-08-15.md#L3", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	qid := extractID(t, out, "Q-")

	show, _, code := execCLI(t, "show", "question", qid, "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, show)
	require.Equal(t, "briefing/2026-08-15.md#L3", data["source"])
	blocks, ok := data["blocks"].([]any)
	require.True(t, ok, "show question --json carries blocks: %s", show)
	require.Len(t, blocks, 1)
	row, ok := blocks[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "form-engine/certification-form-loading", row["key"])
	require.NotEmpty(t, row["id"])

	// A blockless, sourceless question renders neither key.
	bare, _, code := execCLI(t, "add", "question", "No provenance, holds nothing.", "--title", "No provenance", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	bareID := extractID(t, bare, "Q-")

	bareShow, _, code := execCLI(t, "show", "question", bareID, "--json")
	require.Equal(t, 0, code)
	bareData := jsonData(t, bareShow)
	_, hasSource := bareData["source"]
	require.False(t, hasSource, "a blockless, sourceless question omits the source key: %s", bareShow)
	_, hasBlocks := bareData["blocks"]
	require.False(t, hasBlocks, "a blockless, sourceless question omits the blocks key: %s", bareShow)
}

func TestAddQuestionRequiresTitleBeforeWrites(t *testing.T) {
	prepActiveFeatureWithTasks(t)

	out, _, code := execCLI(t, "add", "question", "Which schema version is canonical?", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "title_required"`)

	_, err := os.Stat(".tiller/decisions/open-questions.yml")
	require.True(t, os.IsNotExist(err), "missing --title refuses before the ledger write")
}

func TestAddQuestionTitleHumanAndJSON(t *testing.T) {
	prepActiveFeatureWithTasks(t)

	human, _, code := execCLI(t, "add", "question",
		"Which specification revision should new implementations use?",
		"--title", "Canonical schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, human, `question Q-`)
	require.Contains(t, human, `("Canonical schema")`)
	require.Contains(t, human, `"Which specification revision should new implementations use?"`)

	out, _, code := execCLI(t, "add", "question",
		"Where should the front-end design system live?",
		"--title", "Design system location", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	require.Equal(t, "Design system location", data["title"])
	require.Equal(t, "Where should the front-end design system live?", data["statement"])
}

func TestAnswerQuestionTitleHumanAndJSON(t *testing.T) {
	prepActiveFeatureWithTasks(t)
	gitifyCwd(t)

	added, _, code := execCLI(t, "add", "question",
		"Which specification revision should new implementations use?",
		"--title", "Canonical schema", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	qid := extractID(t, added, "Q-")

	human, _, code := execCLI(t, "answer", "question", qid,
		"--answer", "Use the 2025 revision.", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, human, fmt.Sprintf(`question %s ("Canonical schema")`, qid))
	require.Contains(t, human, `"Which specification revision should new implementations use?"`)

	out, _, code := execCLI(t, "answer", "question", qid,
		"--answer", "Use the 2025 revision.", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	require.Equal(t, "Canonical schema", data["title"])
	require.Equal(t, "Which specification revision should new implementations use?", data["statement"])
}

// extractID pulls the first id with the given prefix out of a JSON envelope.
func extractID(t *testing.T, out, prefix string) string {
	t.Helper()
	i := strings.Index(out, `"id": "`+prefix)
	require.GreaterOrEqual(t, i, 0, "an id with prefix %s in %q", prefix, out)
	rest := out[i+len(`"id": "`):]
	return rest[:strings.Index(rest, `"`)]
}

// Final review item 4: a null entry in the questions ledger is a malformed
// ledger like any other, so it reaches the envelope with the same exit code and
// reason as a sibling parse failure of the same file, never a reason of its own.
func TestNullQuestionEntryRefusesLikeAnyMalformedLedgerCLI(t *testing.T) {
	refusal := func(t *testing.T, ledger string) (int, string, string) {
		t.Helper()
		initOwnedRepo(t)
		path := filepath.Join(".tiller", "decisions", "open-questions.yml")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(ledger), 0o644))
		out, _, code := execCLI(t, "status", "--json")
		var env struct {
			Error struct {
				Code    int    `json:"code"`
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &env), out)
		require.Equal(t, code, env.Error.Code)
		return code, env.Error.Reason, env.Error.Message
	}

	nullCode, nullReason, nullMsg := refusal(t, "questions:\n  - null\n")
	badCode, badReason, _ := refusal(t, "questions: [unclosed\n")

	require.Equal(t, 2, nullCode)
	require.Equal(t, "error", nullReason)
	require.Equal(t, badCode, nullCode, "the same exit as a malformed ledger")
	require.Equal(t, badReason, nullReason, "the same reason as a malformed ledger")
	require.Contains(t, nullMsg, "question entry is null")
}
