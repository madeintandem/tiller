package cli

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Issue #152 item 1: the plan gate refuses while an open question holds the
// plan, and the refusal tells the human to answer it, never to assume it.

// TestApproveFeatureHeldHumanLine pins the approve variant of the
// open_question_blocker line: it names `tiller answer question`, folds the
// answers into the plan, and never offers --assume-question (decision 8).
func TestApproveFeatureHeldHumanLine(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	_, _, code := execCLI(t, "spec", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	writeBlockingQuestion(t, "Q-aaaaaa", soleTaskID(t))

	stdout, stderr, code := execCLI(t, "approve", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")

	require.Equal(t, 1, code, "an open question holding the plan is a blocked business answer (exit 1)")
	human := stdout + stderr
	require.Contains(t, human, fmt.Sprintf(
		`Blocked: feature form-engine/nrca-form-loading (%s) is held by 1 open question(s): Q-aaaaaa ("Canonical schema version"); answer them with tiller answer question <Q-id> --answer "...", or cancel one that no longer needs a decision with tiller cancel question <Q-id> --reason "...", fold the answers into the plan, and retry.`,
		soleFeatureID(t)))
	require.NotContains(t, human, "--assume-question")
	require.Equal(t, "specced", loadTestTree(t).Features[soleFeatureID(t)].Status, "a refused plan gate must not move the feature")

	out, _, code := execCLI(t, "approve", "feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	data := jsonData(t, out)
	require.Equal(t, "open_question_blocker", data["reason"])
	require.Equal(t, "specced", data["current_state"])
}

// TestApproveTasksByFeaturePerTask: the batch keeps its convention. The held
// task is a blocked entry, the unheld one approves, and the batch exits 1.
func TestApproveTasksByFeaturePerTask(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	// Approve one task singly so exactly two proposed tasks remain.
	_, _, code := execCLI(t, "approve", "task", ids[2], "--contributor", "jc")
	require.Equal(t, 0, code)
	writeBlockingQuestion(t, "Q-aaaaaa", ids[0])

	out, _, code := execCLI(t, "approve", "task", "--feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")

	require.Equal(t, 1, code, "a batch with a blocked entry exits 1")
	env := parseBatch(t, out)
	require.False(t, env.OK)
	require.Equal(t, 2, env.Data.Summary.Total)
	require.Equal(t, 1, env.Data.Summary.Blocked)
	require.Equal(t, 1, env.Data.Summary.Changed)
	require.Len(t, env.Data.Results, 2)
	// Sibling order is not this test's subject, so entries are read by id.
	byID := map[string]map[string]any{}
	for _, entry := range env.Data.Results {
		byID[entry["id"].(string)] = entry
	}
	require.Equal(t, false, byID[ids[0]]["ok"])
	require.Equal(t, "open_question_blocker", byID[ids[0]]["reason"])
	require.Equal(t, true, byID[ids[1]]["ok"])

	tree := loadTestTree(t)
	require.Equal(t, "proposed", tree.Tasks[ids[0]].Status, "the held task stays proposed")
	require.Equal(t, "ready", tree.Tasks[ids[1]].Status, "the unheld task approves")
}
