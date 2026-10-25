package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// writeFeatureEval lays a feature.eval.md beside the fixture feature so
// correct-eval has a contract to pin. The content only has to exist: staging
// hashes the file and never parses it.
func writeFeatureEval(t *testing.T) {
	t.Helper()
	dir := filepath.Join(".tiller", "epics", "form-engine", "features", "nrca-form-loading")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"), []byte("# contract\n"), 0o644))
}

func TestAmendStagesCorrectEvalAndShowRendersItsHeadline(t *testing.T) {
	layAmendmentTree(t)
	writeFeatureEval(t)
	aid := openAmendmentCLI(t, "eval-bindings", "Correct eval bindings")

	out, stderr, code := execCLI(t, "amend", aid, "--correct-eval", "form-engine/nrca-form-loading",
		"--reason", "the binding file moved", "--retire", "C7", "--retire", "C8", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)

	ops := jsonData(t, out)["operations"].([]any)
	require.Len(t, ops, 1)
	op := ops[0].(map[string]any)
	require.Equal(t, "correct-eval", op["op"])
	require.Equal(t, "the binding file moved", op["reason"])
	require.Equal(t, []any{"C7", "C8"}, op["retire"])
	pre := op["preconditions"].([]any)
	require.Len(t, pre, 1, "staging pins eval_revision")
	require.Equal(t, "eval_revision", pre[0].(map[string]any)["name"])

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code)
	require.Contains(t, human, "correct-eval form-engine/nrca-form-loading")
	require.Contains(t, human, ": the binding file moved, retiring C7, C8")
}

// The approve confirmation carries correct-eval's commit hint under its
// operation line, since a second correction refuses until the first one's
// feature.eval.md is committed.
func TestApproveAmendmentHumanOutputCarriesTheCorrectEvalCommitHint(t *testing.T) {
	note := "re-pinned verified_against.eval to git:1b44f52; commit .tiller/epics/e/features/f/feature.eval.md with this change, since a later correction reads it back from git"
	res := mutate.ApproveResult{ID: "A-000001", Status: store.AmendmentStatusApproved, Applied: []mutate.AppliedOperation{{
		Index: 0, Op: store.AmendmentOpCorrectEval, Noun: store.NounFeature, NodeID: "F-000002", Key: "e/f", Status: "done", Note: note,
	}}}

	out := humanApproveAmendment(&store.Tree{}, "eval-bindings (A-000001)", res, "jc")
	require.Contains(t, out, "operation 0 (correct-eval) e/f (F-000002): done\n    "+note)
	require.Equal(t, note, amendmentApproveView(res).Applied[0].Note, "the --json payload carries it too")
}

func TestAmendCorrectEvalWithoutRetireHasNoRetiringClause(t *testing.T) {
	layAmendmentTree(t)
	writeFeatureEval(t)
	aid := openAmendmentCLI(t, "eval-bindings", "Correct eval bindings")

	_, _, code := execCLI(t, "amend", aid, "--correct-eval", "form-engine/nrca-form-loading",
		"--reason", "the binding file moved", "--contributor", "jc")
	require.Equal(t, 0, code)

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code)
	require.Contains(t, human, "the binding file moved")
	require.NotContains(t, human, "retiring")
}

func TestAmendCorrectEvalRefusals(t *testing.T) {
	layAmendmentTree(t)
	writeFeatureEval(t)
	aid := openAmendmentCLI(t, "eval-bindings", "Correct eval bindings")
	feature := "form-engine/nrca-form-loading"

	for _, tc := range []struct {
		name   string
		flags  []string
		reason string
	}{
		{"missing reason", []string{"--correct-eval", feature}, "reason_required"},
		{"blank reason", []string{"--correct-eval", feature, "--reason", " "}, "reason_required"},
		{"retire with revise", []string{"--revise", feature, "--retire", "C7"}, "retire_misplaced"},
		{"retire with cancel", []string{"--cancel", feature, "--reason", "r", "--retire", "C7"}, "retire_misplaced"},
		{"retire with no op", []string{"--amends-design", "design/a.md", "--retire", "C7"}, "retire_misplaced"},
		{"retire with an empty value", []string{"--revise", feature, "--retire="}, "retire_misplaced"},
		{"two operations", []string{"--correct-eval", feature, "--revise", feature, "--reason", "r"}, "multiple_operations"},
		{"a task target", []string{"--correct-eval", "form-engine/nrca-form-loading/schema-review", "--reason", "r"}, "correct_eval_not_feature"},
		{"duplicate retire ids", []string{"--correct-eval", feature, "--reason", "r", "--retire", "C7", "--retire", "C7"}, "amendment_shape_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"amend", aid, "--json", "--contributor", "jc"}, tc.flags...)
			out, stderr, code := execCLI(t, args...)
			require.Equal(t, 2, code, out+stderr)
			require.Contains(t, out, `"`+tc.reason+`"`)
			require.Empty(t, loadCwdTree(t).Amendments[aid].Operations, "a refused call stages nothing")
		})
	}
}
