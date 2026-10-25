package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// layTaskInFlightOnWorktree claims and starts the fixture task, commits that
// state on the task branch, and checks the branch out in a linked worktree
// while the integration checkout still records the task ready and unclaimed:
// the field shape behind issue #149.
func layTaskInFlightOnWorktree(t *testing.T) (taskID, branch string) {
	t.Helper()
	layFeatureReadyToStart(t)
	dir, err := os.Getwd()
	require.NoError(t, err)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "approved plan")
	taskID = soleTaskID(t)
	claimTaskAsOwner(t, cliTaskKey, "jc")
	_, stderr, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	branch = checkoutTaskBranchCLI(t, taskID)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "claim and start")
	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "worktree", "add", t.TempDir(), branch)
	require.Equal(t, "ready", taskStatusCLI(t, taskID), "integration still records the task ready")
	return taskID, branch
}

// readoutNextLine is the first ranked move a readout lists.
func readoutNextLine(t *testing.T, readout string) string {
	t.Helper()
	_, next := splitAtNext(t, readout)
	line, _, _ := strings.Cut(next, "\n")
	return line
}

func TestReadoutNextDropsClaimForObservedTask(t *testing.T) {
	_, branch := layTaskInFlightOnWorktree(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	readout, stderr, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code, stderr)
	next := readoutNextLine(t, readout)
	require.NotContains(t, next, "claim it with")
	require.Contains(t, next, "1. Agent: continue "+cliTaskKey+" (")
	require.True(t, strings.HasSuffix(next, " Continue it on "+branch+"."), "a move in flight elsewhere continues there, with no command: %s", next)
	require.NotContains(t, next, "`tiller", "nothing to run from this integration view")
	require.Equal(t, 1, strings.Count(next, branch), "the line names its branch once: %s", next)
	require.Contains(t, next, "Your task, in flight on its branch.", "the reason asserts no progress on an unstarted task")

	guide, stderr, code := execCLI(t, "guide")
	require.Equal(t, 0, code, stderr)
	require.NotContains(t, guide, "claim it with")
	require.Contains(t, guide, "Your task is in flight on "+branch+"; continue it there.")
	require.Equal(t, 1, strings.Count(guide, "in flight on "+branch), "guide says it once: %s", guide)
}

// Inside the reader's own task checkout the branch is the served tree, not
// work in flight elsewhere: the move keeps its ordinary execute pointer.
func TestReadoutInsideOwnTaskCheckoutIsNotMuted(t *testing.T) {
	layFeatureReadyToStart(t)
	dir, err := os.Getwd()
	require.NoError(t, err)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "approved plan")
	taskID := soleTaskID(t)
	claimTaskAsOwner(t, cliTaskKey, "jc")
	_, stderr, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	checkoutTaskBranchCLI(t, taskID)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "claim and start")
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	readout, stderr, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code, stderr)
	next := readoutNextLine(t, readout)
	require.NotContains(t, next, "Continue it on")
	require.Equal(t, "1. Agent: continue "+cliTaskKey+" ("+taskID+"). Your task is in progress. `tiller guide execute`", next)

	guide, stderr, code := execCLI(t, "guide")
	require.Equal(t, 0, code, stderr)
	require.NotContains(t, guide, "in flight on")
	require.Contains(t, guide, "You hold a task in in_progress")
}

// A second contributor caught up behind someone else's observed task: the
// pool signpost leaves that task out instead of misdiagnosing it as held, and
// with nothing else left the readout and guide say so plainly (R4).
func TestCaughtUpBehindOthersObservedTaskSaysNothingRemains(t *testing.T) {
	_, branch := layTaskInFlightOnWorktree(t)
	t.Setenv("TILLER_CONTRIBUTOR", "robin")

	readout, stderr, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code, stderr)
	_, next := splitAtNext(t, readout)
	require.Equal(t, views.CaughtUpText+"\n\n", next, readout)

	guide, stderr, code := execCLI(t, "guide")
	require.Equal(t, 0, code, stderr)
	require.NotContains(t, guide, "unclaimed task", guide)
	require.NotContains(t, guide, "awaits a valid accepted basis", guide)
	require.True(t, strings.HasPrefix(guide, views.CaughtUpText+"\n\nSuggested skill: orient\n"), guide)
	require.NotContains(t, guide, branch, "another contributor's branch is no move of this reader's")
}
