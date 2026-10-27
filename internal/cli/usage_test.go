package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// usageRepo lays the gated git repo with usage capture on, a declared
// integration branch, and a clean environment for the harness folders.
func usageRepo(t *testing.T) (dir, home string) {
	t.Helper()
	dir = initGatedGitRepo(t, "true")
	p := filepath.Join(dir, ".tiller", "tiller.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, append(data, []byte("usage:\n  capture: true\n")...), 0o644))
	declareIntegrationBranchCLI(t, dir)
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	return dir, home
}

func claudeProjectFolder(t *testing.T, home string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(home, ".claude", "projects", strings.NewReplacer("/", "-", ".", "-").Replace(cwd))
}

// layClaudeSession writes one settled Claude Code session whose working
// directory is the current checkout.
func layClaudeSession(t *testing.T, home string) {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	line := `{"type":"assistant","timestamp":"2026-01-02T10:00:00.000Z","sessionId":"s1","cwd":"` + cwd +
		`","gitBranch":"main","message":{"id":"m1","model":"claude-opus-5-5","role":"assistant","content":[{"type":"text","text":"ok"}],` +
		`"usage":{"input_tokens":2,"cache_creation_input_tokens":10,"cache_read_input_tokens":20,"output_tokens":30}}}` + "\n"
	p := filepath.Join(claudeProjectFolder(t, home), "s1.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(line), 0o644))
	old := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(p, old, old)) // a settled log, quiet for months
}

func usageFiles(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, ".tiller", "usage", "claude-code", "*.yml"))
	require.NoError(t, err)
	return m
}

func fileCount(t *testing.T, p string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(p, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func TestUsageRefusesWhenCaptureOff(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	out, _, code := execCLI(t, "usage", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, `"ok": false`)
	require.Contains(t, out, `"reason": "usage_capture_off"`)
	require.NoDirExists(t, filepath.Join(dir, ".tiller", "usage"))

	_, errOut, code := execCLI(t, "usage")
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "Agent usage capture is off. Set usage.capture: true in .tiller/tiller.yml to turn it on.")
}

func TestUsageRefusesOffIntegrationBranch(t *testing.T) {
	dir, home := usageRepo(t)
	layClaudeSession(t, home)
	gitRun(t, dir, "checkout", "-b", "task-branch")

	out, _, code := execCLI(t, "usage", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, `"reason": "not_integration_branch"`)
	require.Contains(t, out, "task-branch")
	require.Contains(t, out, "main")
	require.NoDirExists(t, filepath.Join(dir, ".tiller", "usage"))
	require.NoFileExists(t, filepath.Join(dir, ".tiller", ".cache", "usage.yml"), "no cursor when nothing was committed")
}

func TestUsageWritesFilesAndNoEvent(t *testing.T) {
	dir, home := usageRepo(t)
	layClaudeSession(t, home)
	before := fileCount(t, filepath.Join(dir, ".tiller", "events"))

	out, _, code := execCLI(t, "usage", "--json")
	require.Equal(t, 0, code, out)
	require.Len(t, usageFiles(t, dir), 1)
	require.Equal(t, before, fileCount(t, filepath.Join(dir, ".tiller", "events")))
	require.FileExists(t, filepath.Join(dir, ".tiller", ".cache", "usage.yml"))

	data := jsonData(t, out)
	require.Equal(t, false, data["partial"])
	hs := data["harnesses"].([]any)
	require.Len(t, hs, 3)
	first := hs[0].(map[string]any)
	require.Equal(t, "claude-code", first["harness"])
	require.Equal(t, true, first["found"])
	require.EqualValues(t, 1, first["sessions"])
	require.EqualValues(t, 1, first["written"])
	require.EqualValues(t, 62, first["tokens"])
}

func TestUsageHumanOutput(t *testing.T) {
	_, home := usageRepo(t)
	layClaudeSession(t, home)

	human, _, code := execCLI(t, "usage")
	require.Equal(t, 0, code)
	require.Contains(t, human, "Claude Code: 1 session found, 1 captured, 62 tokens captured")
	require.Contains(t, human, "Codex: no session logs found")

	again, _, code := execCLI(t, "usage")
	require.Equal(t, 0, code)
	require.Contains(t, again, "Claude Code: 1 session found, nothing new to capture")
}

// advanceGeneration makes the tree captureUsage holds stale by committing a
// real write through a second loaded copy.
func advanceGeneration(t *testing.T) func(*store.Tree) {
	t.Helper()
	return func(tree *store.Tree) {
		other, err := store.Load(tree.Root)
		require.NoError(t, err)
		target := filepath.Join(tree.Root, ".tiller", "usage", "other", "x.yml")
		_, snap, err := store.ReadGuardedFileSnapshot(tree.Root, target)
		require.NoError(t, err)
		require.NoError(t, other.Commit("test write", []store.PlannedWrite{store.PlanWrite(target, snap, []byte("x: 1\n"))}))
	}
}

func TestUsageStaleTreeIsARefusalToRunAgain(t *testing.T) {
	dir, home := usageRepo(t)
	layClaudeSession(t, home)
	usageBeforeCommit = advanceGeneration(t)
	t.Cleanup(func() { usageBeforeCommit = nil })

	out, _, code := execCLI(t, "usage", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, `"reason": "mutation_stale"`)
	require.Empty(t, usageFiles(t, dir))
	require.NoFileExists(t, filepath.Join(dir, ".tiller", ".cache", "usage.yml"), "no cursor for an uncommitted capture")

	_, errOut, code := execCLI(t, "usage")
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "Run tiller usage again.")
}

func TestSyncStaleCaptureIsAWarning(t *testing.T) {
	dir, home := usageRepo(t)
	layClaudeSession(t, home)
	usageBeforeCommit = advanceGeneration(t)
	t.Cleanup(func() { usageBeforeCommit = nil })

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, `"ok": true`)
	require.NotEmpty(t, jsonData(t, out)["usage_warning"])
	require.NoFileExists(t, filepath.Join(dir, ".tiller", ".cache", "usage.yml"))
}

// backdateEvents moves every recorded event to before the fixture session.
func backdateEvents(t *testing.T, dir string) {
	t.Helper()
	at := regexp.MustCompile(`(?m)^at: .*$`)
	require.NoError(t, filepath.WalkDir(filepath.Join(dir, ".tiller", "events"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(p, at.ReplaceAll(data, []byte("at: 2025-12-01T00:00:00Z")), 0o644)
	}))
}

// A sync that records a crossing advances the generation; capture must reload.
func TestSyncWithCrossingStillCaptures(t *testing.T) {
	dir, home := usageRepo(t)
	driveFeatureToReadyCLI(t)
	tid := submitTaskOnFeatureCLI(t, cliFeatureKey)
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)
	backdateEvents(t, dir) // the fixture session must not predate the project's first event
	layClaudeSession(t, home)

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, `"status": "done"`)
	require.NotContains(t, jsonData(t, out), "usage_warning")
	require.Len(t, usageFiles(t, dir), 1)
}

func TestUsageNeedsNoIdentity(t *testing.T) {
	dir, home := usageRepo(t)
	layClaudeSession(t, home)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	require.NoFileExists(t, filepath.Join(dir, ".tiller", "identity.yml"))

	_, _, code := execCLI(t, "usage")
	require.Equal(t, 0, code)
	require.Len(t, usageFiles(t, dir), 1)
}

func TestSyncCapturesWhenEnabled(t *testing.T) {
	dir, home := usageRepo(t)
	layClaudeSession(t, home)

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Len(t, usageFiles(t, dir), 1)
	require.NotContains(t, jsonData(t, out), "usage_warning")
}

func TestSyncCaptureFailureIsAWarning(t *testing.T) {
	dir, home := usageRepo(t)
	// An unreadable session folder makes capture fail.
	proj := claudeProjectFolder(t, home)
	require.NoError(t, os.MkdirAll(proj, 0o755))
	require.NoError(t, os.Chmod(proj, 0o000))
	t.Cleanup(func() { _ = os.Chmod(proj, 0o755) })

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, `"ok": true`)
	require.NotEmpty(t, jsonData(t, out)["usage_warning"])
	require.NoDirExists(t, filepath.Join(dir, ".tiller", "usage"))
	require.NoFileExists(t, filepath.Join(dir, ".tiller", ".cache", "usage.yml"))
}

func TestSyncSkipsCaptureWhenOff(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	home := t.TempDir()
	t.Setenv("HOME", home)
	layClaudeSession(t, home)

	_, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NoDirExists(t, filepath.Join(dir, ".tiller", "usage"))
}

func TestSyncNotesCaptureSkippedOffIntegrationBranch(t *testing.T) {
	dir, home := usageRepo(t)
	layClaudeSession(t, home)
	gitRun(t, dir, "checkout", "-b", "scratch")

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--into", "main")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "Agent usage not captured: not on main.")
	require.NoDirExists(t, filepath.Join(dir, ".tiller", "usage"))
}

func TestFormatTokens(t *testing.T) {
	for in, want := range map[int64]string{
		0: "0", 950: "950", 1_200: "1.2K", 412_000: "412K", 999_600: "1.0M",
		8_900_000: "8.9M", 186_000_000: "186M", 1_200_000_000: "1.2B",
	} {
		require.Equal(t, want, formatTokens(in), "%d", in)
	}
}
