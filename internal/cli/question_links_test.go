package cli

import (
	"fmt"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"

	"github.com/stretchr/testify/require"
)

func TestLinkQuestionBlocksCLI(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	qid := addCancelFixtureQuestion(t, ids[0])
	first := fmt.Sprintf("form-engine/certification-form-loading/schema-transcription (%s)", ids[0])
	second := refOfTask(t, ids[1])

	human, _, code := execCLI(t, "link", "question", qid, "--blocks", ids[1], "--contributor", "jc")
	require.Equal(t, 0, code, human)
	require.Contains(t, human, fmt.Sprintf("question %s now blocks %s, %s.", qid, first, second))

	human, _, code = execCLI(t, "link", "question", qid, "--blocks", ids[1], "--contributor", "jc")
	require.Equal(t, 0, code, human)
	require.Contains(t, human, fmt.Sprintf("question %s already blocks %s, %s (no change).", qid, first, second))

	out, _, code := execCLI(t, "link", "question", qid, "--blocks", ids[1], "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	require.Equal(t, "question", data["noun"])
	require.Equal(t, qid, data["id"])
	require.Equal(t, []any{ids[0], ids[1]}, data["blocks"])
	require.Equal(t, false, data["changed"])
	require.Equal(t, []string{ids[0], ids[1]}, loadTestTree(t).Questions[qid].Blocks)
}

func TestUnlinkQuestionBlocksCLI(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	qid := addCancelFixtureQuestion(t, ids[0])
	mustRun(t, "link", "question", qid, "--blocks", ids[1], "--contributor", "jc")
	first := fmt.Sprintf("form-engine/certification-form-loading/schema-transcription (%s)", ids[0])
	second := refOfTask(t, ids[1])

	human, _, code := execCLI(t, "unlink", "question", qid, "--blocks", ids[0], "--contributor", "jc")
	require.Equal(t, 0, code, human)
	require.Contains(t, human, fmt.Sprintf("Removed %s from question %s; it blocks %s.", first, qid, second))

	human, _, code = execCLI(t, "unlink", "question", qid, "--blocks", ids[0], "--contributor", "jc")
	require.Equal(t, 0, code, human)
	require.Contains(t, human, fmt.Sprintf("question %s did not block %s (no change).", qid, first))

	out, _, code := execCLI(t, "unlink", "question", qid, "--blocks", ids[1], "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	require.Equal(t, "question", data["noun"])
	require.Equal(t, qid, data["id"])
	require.Equal(t, []any{}, data["blocks"])
	require.Equal(t, true, data["changed"])

	mustRun(t, "link", "question", qid, "--blocks", ids[1], "--contributor", "jc")
	human, _, code = execCLI(t, "unlink", "question", qid, "--blocks", ids[1], "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, human, fmt.Sprintf("Removed %s from question %s; it blocks nothing.", second, qid))
}

func TestLinkQuestionClosedRefuses(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	qid := addCancelFixtureQuestion(t, ids[0])
	mustRun(t, "cancel", "question", qid, "--reason", "moot", "--contributor", "jc")

	stdout, stderr, code := execCLI(t, "unlink", "question", qid, "--blocks", ids[0], "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, stdout+stderr, fmt.Sprintf("Blocked: question %s is cancelled; only an open question can be unlinked.", qid))

	out, _, code := execCLI(t, "link", "question", qid, "--blocks", ids[1], "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	data := jsonData(t, out)
	require.Equal(t, "question_closed", data["reason"])
	require.Equal(t, "cancelled", data["current_state"])
}

func TestLinkQuestionRefusesOtherAxes(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	qid := addCancelFixtureQuestion(t, ids[0])

	for _, argv := range [][]string{
		{"link", "question", qid, "--depends-on", ids[1]},
		{"unlink", "question", qid, "--depends-on", ids[1]},
		{"link", "question", qid, "--blocks", ids[1], "--surface", "src"},
		{"link", "feature", "form-engine/certification-form-loading", "--blocks", ids[1]},
		{"unlink", "task", ids[0], "--blocks", ids[1]},
	} {
		out, _, code := execCLI(t, append(argv, "--contributor", "jc", "--json")...)
		require.Equalf(t, 2, code, "%v: %s", argv, out)
		require.Containsf(t, out, `"reason": "bad_args"`, "%v", argv)
	}
	out, _, code := execCLI(t, "link", "question", qid, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "blocks_required"`)
	require.Equal(t, []string{ids[0]}, loadTestTree(t).Questions[qid].Blocks)
}

func refOfTask(t *testing.T, id string) string {
	t.Helper()
	return gen.RefByID(loadTestTree(t), id)
}
