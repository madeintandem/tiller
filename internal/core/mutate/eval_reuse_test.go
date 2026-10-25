package mutate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// These tests pin criterion reuse on `tiller eval` (design
// 2026-10-21-gate-reuse): a command or test criterion that passed over the
// same code identity under the same command is not run again, and
// EvalInput.Rerun forces the run.

// evalRuns counts the runs of the fixture's eval criterion C1. The criterion
// runs the tests command with its test ref appended, so each run writes a
// "run <ref>" line; countingRun's exact-match count (runs) would miss them.
func evalRuns(t *testing.T, dir, name string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name+".count"))
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "run") {
			n++
		}
	}
	return n
}

// evalGatedTask runs one eval of the fixture task against real git.
func evalGatedTask(t *testing.T, root, taskID string, deps mutate.Deps, in mutate.EvalInput) mutate.EvalResult {
	t.Helper()
	tree := load(t, root)
	res, err := mutate.Eval(tree, deps, vcs.New(root), resolveNode(t, tree, store.NounTask, taskID), in)
	require.NoError(t, err)
	return res
}

// criterionOutcome returns the named criterion outcome of an eval result.
func criterionOutcome(t *testing.T, res mutate.EvalResult, id string) mutate.CriterionOutcome {
	t.Helper()
	for _, c := range res.Criteria {
		if c.ID == id {
			return c
		}
	}
	require.Failf(t, "no outcome", "eval reported no %q criterion", id)
	return mutate.CriterionOutcome{}
}

func TestEvalRerunFlag(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, taskID := layCountedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))

	first := evalGatedTask(t, root, taskID, testDeps(), mutate.EvalInput{})
	require.False(t, criterionOutcome(t, first, "C1").Reused)
	require.Equal(t, 1, evalRuns(t, dir, "tests"))
	firstGate := load(t, root).Tasks[taskID].Gates["evals"]
	require.NotEmpty(t, firstGate.Code, "the evals gate records the run's code identity")
	require.NotEmpty(t, firstGate.Criteria["C1"].Code)
	require.NotEmpty(t, firstGate.Criteria["C1"].Config)
	gitCommitAllM(t, root, "record eval")

	second := evalGatedTask(t, root, taskID, laterDeps(), mutate.EvalInput{})
	require.Equal(t, 1, evalRuns(t, dir, "tests"), "unchanged code reuses the passed criterion")
	require.True(t, criterionOutcome(t, second, "C1").Reused)
	reused := load(t, root).Tasks[taskID].Gates["evals"]
	require.Equal(t, laterDeps().Clock.Now().UTC(), reused.Criteria["C1"].ReusedAt)
	require.Equal(t, firstGate.Criteria["C1"].Evidence, reused.Criteria["C1"].Evidence)
	require.Equal(t, firstGate.Code, reused.Code)
	gitCommitAllM(t, root, "record reuse")

	third := evalGatedTask(t, root, taskID, laterDeps(), mutate.EvalInput{Rerun: true})
	require.Equal(t, 2, evalRuns(t, dir, "tests"), "--rerun runs the criterion")
	require.False(t, criterionOutcome(t, third, "C1").Reused)
	require.True(t, load(t, root).Tasks[taskID].Gates["evals"].Criteria["C1"].ReusedAt.IsZero())
}

// TestEvalHonorsEvalsReuseFalse pins that the manifest's evals gate `reuse:
// false` turns criterion reuse off for every criterion.
func TestEvalHonorsEvalsReuseFalse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, taskID := layCountedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))
	editManifest(t, root, `${node.id}", on: [feature, task] }`, `${node.id}", on: [feature, task], reuse: false }`)

	evalGatedTask(t, root, taskID, testDeps(), mutate.EvalInput{})
	gitCommitAllM(t, root, "record eval")
	second := evalGatedTask(t, root, taskID, laterDeps(), mutate.EvalInput{})

	require.Equal(t, 2, evalRuns(t, dir, "tests"))
	require.False(t, criterionOutcome(t, second, "C1").Reused)
}

// TestCheckReusesPassedCriterion pins the check side: the evals branch of
// check reuses a passed criterion on unchanged code, --rerun runs it, and the
// evals gate record carries the run's code identity.
func TestCheckReusesPassedCriterion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, taskID := layCountedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))

	checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	first := load(t, root).Tasks[taskID].Gates["evals"]
	require.NotEmpty(t, first.Code)
	require.Equal(t, 2, evalRuns(t, dir, "tests"), "the tests gate and the criterion each ran once")
	gitCommitAllM(t, root, "record check")

	checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{})
	second := load(t, root).Tasks[taskID].Gates["evals"]
	require.Equal(t, laterDeps().Clock.Now().UTC(), second.Criteria["C1"].ReusedAt)
	require.Equal(t, first.Criteria["C1"].Evidence, second.Criteria["C1"].Evidence)
	require.Equal(t, 2, evalRuns(t, dir, "tests"), "neither the gate nor the criterion ran again")
	gitCommitAllM(t, root, "record reuse")

	checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{Rerun: true})
	require.Equal(t, 4, evalRuns(t, dir, "tests"), "--rerun runs the gate and the criterion")
	require.True(t, load(t, root).Tasks[taskID].Gates["evals"].Criteria["C1"].ReusedAt.IsZero())
}
