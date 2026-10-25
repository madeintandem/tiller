package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// assumeQuestionOnDisk records qid in the sole feature's assumption_basis, the
// entry `approve feature --assume-question` leaves behind.
func assumeQuestionOnDisk(t *testing.T, qid string) {
	t.Helper()
	path := filepath.Join(featureDir(t), "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var feature store.Feature
	require.NoError(t, yaml.Unmarshal(data, &feature))
	feature.AssumptionBasis = store.AssumptionBasis{qid: {Reason: "use the documented fallback"}}
	out, err := yaml.Marshal(feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

func addCancelFixtureQuestion(t *testing.T, blocks string) string {
	t.Helper()
	out, _, code := execCLI(t, "add", "question", "Which schema version is canonical?",
		"--title", "Canonical schema version", "--blocks", blocks, "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	return extractID(t, out, "Q-")
}

func TestCancelQuestionCLI(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	qid := addCancelFixtureQuestion(t, ids[0])
	assumeQuestionOnDisk(t, qid)
	fid := soleFeatureID(t)

	human, _, code := execCLI(t, "cancel", "question", qid, "--reason", "moot", "--contributor", "jc")
	require.Equal(t, 0, code, human)
	confirm := fmt.Sprintf("question %s (%q): %q is now cancelled.", qid, "Canonical schema version", "Which schema version is canonical?")
	assumed := fmt.Sprintf("form-engine/nrca-form-loading (%s) assumed %s; its plan stands as built.", fid, qid)
	require.Contains(t, human, confirm+"\n"+assumed)
	require.Contains(t, human, "Question cancelled")
	require.Less(t, strings.Index(human, assumed), strings.Index(human, "Question cancelled"),
		"the readout follows the confirmation")
	require.Contains(t, human, "Nodes this cancel cleared of every open question: 1 of 1")
	require.Contains(t, human, "Open questions remaining: 0")
	require.Equal(t, "cancelled", loadTestTree(t).Questions[qid].Status)

	out, _, code := execCLI(t, "cancel", "question", qid, "--reason", "moot", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	require.Equal(t, "cancelled", data["status"])
	require.Equal(t, "moot", data["cancel_reason"])
	require.Equal(t, []any{fid}, data["assumed_by"])
	require.Contains(t, data, "readout")
}

func TestCancelQuestionCLIOmitsEmptyAssumedBy(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	qid := addCancelFixtureQuestion(t, ids[0])

	out, _, code := execCLI(t, "cancel", "question", qid, "--reason", "moot", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	require.NotContains(t, data, "assumed_by")
	require.Equal(t, "moot", data["cancel_reason"])
}

func TestCancelQuestionCLIAgain(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	qid := addCancelFixtureQuestion(t, ids[0])
	mustRun(t, "cancel", "question", qid, "--reason", "moot", "--contributor", "jc")

	human, _, code := execCLI(t, "cancel", "question", qid, "--reason", "moot", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, human, fmt.Sprintf("question %s (%q): %q is already cancelled (no change).",
		qid, "Canonical schema version", "Which schema version is canonical?"))
	require.Contains(t, human, "Question cancelled")
}

func TestCancelQuestionCLIAnswered(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	gitifyCwd(t)
	qid := addCancelFixtureQuestion(t, ids[0])
	mustRun(t, "answer", "question", qid, "--answer", "The 2025 revision.", "--contributor", "jc")

	stdout, stderr, code := execCLI(t, "cancel", "question", qid, "--reason", "moot", "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, stdout+stderr, fmt.Sprintf("Blocked: question %s is answered; only an open question can be cancelled.", qid))

	out, _, code := execCLI(t, "cancel", "question", qid, "--reason", "moot", "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	data := jsonData(t, out)
	require.Equal(t, "question_closed", data["reason"])
	require.Equal(t, "answered", data["current_state"])
	require.Equal(t, "answered", loadTestTree(t).Questions[qid].Status)
}

func TestCancelQuestionRejectsResolution(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	qid := addCancelFixtureQuestion(t, ids[0])

	out, _, code := execCLI(t, "cancel", "question", qid, "--reason", "moot", "--resolution", "wontfix", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "bad_args"`)
	require.Equal(t, "open", loadTestTree(t).Questions[qid].Status)
}

func TestAddQuestionReaskLine(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	old := addCancelFixtureQuestion(t, ids[0])
	mustRun(t, "cancel", "question", old, "--reason", "asked too early", "--contributor", "jc")

	human, _, code := execCLI(t, "add", "question", "Which schema version is canonical?",
		"--title", "Canonical schema version", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, human, "Created question Q-")
	require.Contains(t, human, fmt.Sprintf(`: "Which schema version is canonical?"; re-asks cancelled %s.`, old))
	require.NotContains(t, human, fmt.Sprintf("Created question %s ", old))
}

// TestCancelReleasesPlanGate is Review Focus 5: a specced feature held by an
// open question refuses approve; cancelling the question releases the plan
// gate, so approve succeeds and the queue offers approve_feature again.
func TestCancelReleasesPlanGate(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	_, _, code := execCLI(t, "spec", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	writeBlockingQuestion(t, "Q-aaaaaa", soleTaskID(t))

	_, _, code = execCLI(t, "approve", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	require.Equal(t, 1, code, "the open question holds the plan")
	require.Empty(t, queueKinds(views.QueueOf(loadTestTree(t)), "approve_feature"))

	mustRun(t, "cancel", "question", "Q-aaaaaa", "--reason", "no longer needs a decision", "--contributor", "jc")
	require.Len(t, queueKinds(views.QueueOf(loadTestTree(t)), "approve_feature"), 1,
		"a cancelled question releases the plan and the row returns")

	_, stderr, code := execCLI(t, "approve", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "ready", loadTestTree(t).Features[soleFeatureID(t)].Status)
}

func queueKinds(q views.Queue, kind string) []views.QueueEntry {
	var out []views.QueueEntry
	for _, e := range q.Entries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}
