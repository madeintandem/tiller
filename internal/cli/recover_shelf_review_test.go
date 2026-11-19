package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// The accident shelf recovery exists for, driven through the binary's own
// surface: two approved tweaks fixed on a hand-named branch with no start,
// check, or submit, and that branch merged. One recovery command per node
// records the review, ordinary sync crosses both, and every read says so.
func TestRecoverShelfReviewCLIFromRecordToRecoveredDone(t *testing.T) {
	root := initGatedGitRepo(t, "true")
	gitRun(t, root, "branch", "-M", "main")
	declareIntegrationBranchCLI(t, root)
	for _, key := range []string{"outline", "spacing"} {
		mustRun(t, "add", "tweak", key, "--title", "Tweak "+key, "--contributor", "jc")
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tweaks", key, "tweak.md"), []byte("# Tweak "+key+"\n\n## What\n\nx\n\n## Done when\n\ny\n"), 0o644))
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "capture")
	for _, key := range []string{"outline", "spacing"} {
		mustRun(t, "approve", "tweak", key, "--contributor", "jc")
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "approve")
	gitRun(t, root, "checkout", "-b", "polish/both")
	require.NoError(t, os.WriteFile(filepath.Join(root, "polish.txt"), []byte("both\n"), 0o644))
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "polish both")
	reviewed, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	gitRun(t, root, "checkout", "main")
	gitRun(t, root, "merge", "--no-ff", "-m", "human review merge", "polish/both")
	merged, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	gitRun(t, root, "remote", "add", "origin", "https://example.invalid/review.git")
	head, result := strings.TrimPrefix(reviewed.Value, "git:"), strings.TrimPrefix(merged.Value, "git:")

	// A task has no recovery of its own, and a remote needs the real PR number.
	_, _, code := execCLI(t, "submit", "task", "T-none00", "--recover-merged", "--head", head, "--merge-commit", result, "--reason", "x")
	require.Equal(t, 2, code)
	_, _, code = execCLI(t, "submit", "tweak", "outline", "--recover-merged", "--head", head, "--merge-commit", result, "--reason", "x", "--contributor", "jc")
	require.Equal(t, 2, code, "a configured remote requires --pr")

	for _, key := range []string{"outline", "spacing"} {
		out, stderr, code := execCLI(t, "--json", "submit", "tweak", key, "--recover-merged", "--head", head, "--merge-commit", result,
			"--pr", "13", "--reason", "merged before any submission was recorded", "--contributor", "jc")
		require.Equal(t, 0, code, out+stderr)
		var envelope struct {
			OK   bool              `json:"ok"`
			Data RecoverReviewView `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &envelope))
		require.True(t, envelope.OK)
		require.Equal(t, "tweak", envelope.Data.Noun)
		require.True(t, envelope.Data.Changed)
		require.Equal(t, "ancestry", envelope.Data.RecoveredReview.Basis)
		require.Equal(t, "caller_supplied", envelope.Data.PRAssociation)
		require.Equal(t, []string{"build", "tests"}, envelope.Data.MissingGates)
		require.Equal(t, "tiller sync", envelope.Data.NextAction)
	}
	tree := loadTestTree(t)
	for _, tweak := range tree.Tweaks {
		require.Equal(t, "ready", tweak.Status, "recovery moves no lifecycle")
	}

	// Before sync the oracle says what waits and why, and next offers the sync.
	out, _, code := execCLI(t, "status", "tweak", "outline")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "recovered review recorded: the next sync on integration records done")
	require.Contains(t, out, "Recovered historical review of caller-supplied PR #13: head "+reviewed.Value)
	require.Contains(t, out, "merged before any submission was recorded")
	out, _, code = execCLI(t, "next", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "`tiller sync`")
	require.NotContains(t, out, "tiller start tweak")

	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "record recovery")
	out, stderr, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	require.Contains(t, out, "Sync: recorded 2 done crossing(s):")
	require.Equal(t, 2, strings.Count(out, "-> done (recovered review) (policy_violations: [build tests])"), out)
	require.Contains(t, out, "Basis: 2 recovered")
	for _, tweak := range loadTestTree(t).Tweaks {
		require.Equal(t, "done", tweak.Status)
		require.Equal(t, "recovered", tweak.DoneBasis)
		require.Len(t, tweak.PolicyViolations, 2, "missing gate evidence stays on the node as a warning")
	}
	out, _, code = execCLI(t, "show", "tweak", "outline")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "Recovered historical review of caller-supplied PR #13")
}
