package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// The recovery tests run the printed recovery commands the way a person would:
// copied out of the JSON steps array and handed to a POSIX shell. Every
// repository they act on is a disposable fixture under t.TempDir.

// refusedRecovery runs the refused --accept-fast-forward for the fixture task
// and returns the recovery object from the JSON envelope.
func refusedRecovery(t *testing.T) RecoveryView {
	t.Helper()
	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", soleTaskID(t), "--json")
	require.Equal(t, 1, code, out)
	var envelope struct {
		Data SyncAttestRefusedData `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope), out)
	require.Len(t, envelope.Data.Nodes, 1, out)
	require.NotNil(t, envelope.Data.Nodes[0].Recovery, out)
	return *envelope.Data.Nodes[0].Recovery
}

// stepCommand strips a recovery step's label and returns the command a person
// would copy.
func stepCommand(t *testing.T, step string) string {
	t.Helper()
	for _, label := range []string{"preserve the local tip: ", "realign the branch: ", "rerun: "} {
		if command, ok := strings.CutPrefix(step, label); ok {
			return command
		}
	}
	t.Fatalf("recovery step %q carries no known label", step)
	return ""
}

// runCopied hands one copied command to sh from cwd.
func runCopied(cwd, command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = cwd
	cmd.Env = testgit.Env()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestSyncRecoveryRealignNeverDiscardsUncommittedWorkCLI executes the worktree
// arm's first two steps against a worktree in each state a person could leave
// it in. The rescue branch from step 1 preserves commits only, so step 2 has to
// leave staged and unstaged edits alone: carry them across the realignment
// when it does not touch their file, and stop without changing anything when
// it would.
func TestSyncRecoveryRealignNeverDiscardsUncommittedWorkCLI(t *testing.T) {
	cases := []struct {
		name string
		// file is the tracked file edited in the worktree before the recovery
		// runs; empty leaves the worktree clean. state.txt differs between the
		// local tip and the upstream, README.md does not.
		file    string
		stage   bool
		realign bool
	}{
		{name: "clean worktree", realign: true},
		{name: "unstaged edit outside the realignment", file: "README.md", realign: true},
		{name: "staged edit outside the realignment", file: "README.md", stage: true, realign: true},
		{name: "unstaged edit the realignment would overwrite", file: "state.txt"},
		{name: "staged edit the realignment would overwrite", file: "state.txt", stage: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := initGatedGitRepo(t, "true")
			branch := layOrphanedStateCommitCLI(t, dir)
			worktree := t.TempDir()
			gitRun(t, dir, "worktree", "add", worktree, branch)
			localTip := gitOut(t, dir, "rev-parse", branch)
			upstreamTip := gitOut(t, dir, "rev-parse", "origin/"+branch)

			const edit = "uncommitted work\n"
			if tc.file != "" {
				require.NoError(t, os.WriteFile(filepath.Join(worktree, tc.file), []byte(edit), 0o644))
				if tc.stage {
					gitRun(t, worktree, "add", tc.file)
				}
			}

			recovery := refusedRecovery(t)
			out, err := runCopied(dir, stepCommand(t, recovery.Steps[0]))
			require.NoError(t, err, out)
			require.Equal(t, localTip, gitOut(t, dir, "rev-parse", "recovery/"+cliTaskKey+"-in-review"),
				"step 1 preserves the local tip's commits")
			out, err = runCopied(dir, stepCommand(t, recovery.Steps[1]))

			if tc.file != "" {
				got, rerr := os.ReadFile(filepath.Join(worktree, tc.file))
				require.NoError(t, rerr, "the edited file is still there after step 2: %s", out)
				require.Equal(t, edit, string(got), "step 2 discarded uncommitted work: %s", out)
			}
			if tc.realign {
				require.NoError(t, err, out)
				require.Equal(t, upstreamTip, gitOut(t, worktree, "rev-parse", "HEAD"))
				return
			}
			require.Error(t, err, "step 2 stops when the realignment would overwrite an uncommitted edit: %s", out)
			require.Equal(t, localTip, gitOut(t, worktree, "rev-parse", "HEAD"), "a refused step moves nothing")
		})
	}
}

// TestSyncRecoveryCommandsStayLiteralForAwkwardWorktreePathsCLI copies the
// worktree arm's commands for worktrees whose paths a shell would reinterpret,
// and runs them from a scratch directory. The command has to act on that
// worktree and nothing else, and no part of the path may run as a command.
func TestSyncRecoveryCommandsStayLiteralForAwkwardWorktreePathsCLI(t *testing.T) {
	for _, name := range []string{
		"two words",
		"it's here",
		"a $(touch pwned) b",
		"a; touch pwned; b",
		"a && touch pwned",
		"back`touch pwned`tick",
	} {
		t.Run(name, func(t *testing.T) {
			dir := initGatedGitRepo(t, "true")
			branch := layOrphanedStateCommitCLI(t, dir)
			worktree := filepath.Join(t.TempDir(), name)
			gitRun(t, dir, "worktree", "add", worktree, branch)
			upstreamTip := gitOut(t, dir, "rev-parse", "origin/"+branch)

			scratch := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(scratch, "unrelated.txt"), []byte("keep\n"), 0o644))

			recovery := refusedRecovery(t)
			require.Len(t, recovery.Steps, 3)
			// Step 1 names no path, so it runs in the repository. Step 2 names
			// the worktree itself and has to work from anywhere.
			out, err := runCopied(dir, stepCommand(t, recovery.Steps[0]))
			require.NoError(t, err, out)
			realign := stepCommand(t, recovery.Steps[1])
			out, err = runCopied(scratch, realign)
			require.NoError(t, err, "%s\n%s", realign, out)

			require.Equal(t, upstreamTip, gitOut(t, worktree, "rev-parse", "HEAD"),
				"the realignment acted on the worktree the refusal named")
			entries, err := os.ReadDir(scratch)
			require.NoError(t, err)
			require.Len(t, entries, 1, "nothing in the path ran as a command")
			require.Equal(t, "unrelated.txt", entries[0].Name())
		})
	}
}

// TestRecoveryStepsKeepEveryArgumentOneLiteralWord shadows git and tiller with
// shell functions that print their arguments, then runs each rendered step.
// Whatever a branch, upstream, key, or worktree path contains, the shell must
// hand it over as one unchanged argument.
func TestRecoveryStepsKeepEveryArgumentOneLiteralWord(t *testing.T) {
	const (
		branch   = "tiller/$(touch pwned)/it's;x"
		key      = "epic/feature/task"
		upstream = "origin/tiller/$(touch pwned)/it's;x"
		localTip = "abc1234"
	)
	shadow := `git() { printf '%s\n' git "$@"; }; tiller() { printf '%s\n' tiller "$@"; }; `
	run := func(t *testing.T, step string) []string {
		t.Helper()
		scratch := t.TempDir()
		out, err := runCopied(scratch, shadow+stepCommand(t, step))
		require.NoError(t, err, out)
		entries, rerr := os.ReadDir(scratch)
		require.NoError(t, rerr)
		require.Empty(t, entries, "no argument ran as a command")
		return strings.Split(strings.TrimRight(out, "\n"), "\n")
	}

	t.Run("no worktree", func(t *testing.T) {
		steps := recoverySteps(mutate.AttestNotEligible{Key: key, Branch: branch,
			Recovery: &mutate.Recovery{Upstream: upstream, LocalTip: localTip}})
		require.Equal(t, []string{"git", "branch", "recovery/" + key + "-in-review", localTip}, run(t, steps[0]))
		require.Equal(t, []string{"git", "branch", "-f", branch, upstream}, run(t, steps[1]))
		require.Equal(t, []string{"tiller", "sync", "--accept-fast-forward", key}, run(t, steps[2]))
	})

	t.Run("worktree", func(t *testing.T) {
		const worktree = "/tmp/it's a $(touch pwned) dir; touch pwned &"
		steps := recoverySteps(mutate.AttestNotEligible{Key: key, Branch: branch,
			Recovery: &mutate.Recovery{Upstream: upstream, LocalTip: localTip, Worktree: worktree}})
		got := run(t, steps[1])
		require.Equal(t, []string{"git", "-C", worktree, "reset", "--keep", upstream}, got)
	})
}
