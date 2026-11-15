package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

const lateTaskLocalKey = "one-more-thing"

// driveSoleFeatureToDoneCLI walks the fixture's one feature and one task
// through the real verbs to a recorded done, feature gates checked before the
// merge, and commits the result. It is the clean arc
// TestEndToEndTaskAndFeatureReachDoneCLI asserts step by step, laid here only
// as the starting point for work that arrives after done.
func driveSoleFeatureToDoneCLI(t *testing.T, dir string) (featureID string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		_, stderr, code := execCLI(t, append(args, "--contributor", "jc")...)
		require.Equal(t, 0, code, stderr)
	}
	run("add", "epic", "form-engine", "--title", "Form Engine")
	run("add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create")
	run("approve", "epic", "form-engine")
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	run("spec", "feature", cliFeatureKey)
	run("approve", "feature", cliFeatureKey)
	tid := soleTaskID(t)
	claimTaskAsOwner(t, tid, "jc")
	run("start", "task", tid)
	branch := checkoutTaskBranchCLI(t, tid)
	run("check", "task", tid)
	run("submit", "task", tid, "--pr", "41")
	run("check", "feature", cliFeatureKey)
	mergeTaskWorkCLI(t, dir, branch, "work.txt")
	run("sync")

	featureID = soleFeatureID(t)
	require.Equal(t, "done", loadCwdTree(t).Features[featureID].Status)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "record done")
	return featureID
}

// mergeTaskWorkCLI commits the recorded state and one work file on the
// checked-out task branch, then merges it into main with a merge commit.
func mergeTaskWorkCLI(t *testing.T, dir, branch, file string) {
	t.Helper()
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "record task state")
	require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte("work\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "task work")
	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "merge", "--no-ff", "-m", "merge "+branch, branch)
}

// TestEndToEndReopenedFeatureIsNamedForItsCheck is the late-work arc at the
// CLI. A task is added under a done feature, approved, finished, and merged
// with no sync in between, so the next sync crosses the task and reopens the
// feature with every gate stale. That feature is owed a feature check exactly
// as one whose gates never ran is, and the sync that reopens it must say so:
// before, checks_due stayed empty because the gates were stale, not missing,
// and a caller reading it alone saw nothing to do until the next sync crossed
// the feature with policy violations. The queue entry the console's Home desk
// and Build board read names the same check.
func TestEndToEndReopenedFeatureIsNamedForItsCheck(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	featureID := driveSoleFeatureToDoneCLI(t, dir)

	addSpeccedTask(t, lateTaskLocalKey, "One more thing")
	lateKey := cliFeatureKey + "/" + lateTaskLocalKey
	lateID := soleTaskIDByKey(t, lateKey)
	for _, args := range [][]string{
		{"approve", "task", lateID},
		{"claim", "task", lateID},
		{"start", "task", lateID},
	} {
		_, stderr, code := execCLI(t, append(args, "--contributor", "jc")...)
		require.Equal(t, 0, code, stderr)
	}
	branch := checkoutTaskBranchCLI(t, lateID)
	_, stderr, code := execCLI(t, "check", "task", lateID, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = execCLI(t, "submit", "task", lateID, "--pr", "42", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "done", loadCwdTree(t).Features[featureID].Status,
		"nothing before sync reopens the done feature")
	mergeTaskWorkCLI(t, dir, branch, "late.txt")

	// The sync that reopens the feature names its check. The human block is
	// rendered from this same list (humanSyncChecksDue).
	syncJSON, stderr, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	var env struct {
		Data struct {
			Reopened  []ReopenedView `json:"reopened"`
			ChecksDue []CheckDueView `json:"checks_due"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(syncJSON), &env), "sync --json must parse")
	require.Len(t, env.Data.Reopened, 2, "the feature and the epic above it reopen")
	require.Equal(t, []CheckDueView{{Noun: store.NounFeature, ID: featureID, Key: cliFeatureKey}}, env.Data.ChecksDue)

	reopened := loadCwdTree(t)
	require.Equal(t, "active", reopened.Features[featureID].Status)
	require.Equal(t, store.GateStatusStale, reopened.Features[featureID].Gates["build"].Status)

	// The queue the Home desk and the Build board read carries the same check.
	queue, _ := runQueueCommandJSON(t, "--kind", "feature_check")
	require.Len(t, queue.Entries, 1)
	require.Equal(t, featureID, queue.Entries[0].ID)
	require.Equal(t, "tiller check feature "+featureID, queue.Entries[0].Command)

	// The check reruns the stale gates, so nothing is owed any more, and the
	// next sync records the crossing without naming the feature again.
	_, stderr, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, store.GateStatusPass, loadCwdTree(t).Features[featureID].Gates["build"].Status)

	finalSync, stderr, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.NotContains(t, finalSync, "Awaiting feature check")
	require.Equal(t, "done", loadCwdTree(t).Features[featureID].Status)
}
