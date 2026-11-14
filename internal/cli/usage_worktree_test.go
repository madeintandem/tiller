package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// These tests pin which directories a capture covers across worktrees. A
// project is found in each worktree at its own place in the repository, so
// one that lives at apps/a never takes in the checkout around it or the
// project beside it.

// usageProject lays a Tiller project with usage capture on, a declared
// integration branch, and one event dated before the sessions the tests lay.
func usageProject(t *testing.T, dir string) {
	t.Helper()
	manifest := fmt.Sprintf(gatedGitManifest, "true") + "usage:\n  capture: true\ngit:\n  integration_branch: \"main\"\n"
	event := "id: EV-usage0\ntype: init\nat: 2026-01-01T00:00:00Z\ncontributor: jc\ncommand: init\n"
	for path, body := range map[string]string{
		filepath.Join(".tiller", "tiller.yml"):                                  manifest,
		filepath.Join(".tiller", "events", "2026", "01", "01", "EV-usage0.yml"): event,
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte(body), 0o644))
	}
}

// realTempDir is a temp dir by its resolved path, the one git reports for a
// worktree and a harness records as a session's working directory.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

// usageHome points the harness folders at an empty home.
func usageHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	return home
}

// layClaudeSessionIn writes one settled Claude Code session, named id, that
// ran in cwd: 62 tokens, later than the project's first event.
func layClaudeSessionIn(t *testing.T, home, cwd, id string) {
	t.Helper()
	line := `{"type":"assistant","timestamp":"2026-01-02T10:00:00.000Z","sessionId":"` + id + `","cwd":"` + cwd +
		`","gitBranch":"main","message":{"id":"m-` + id + `","model":"claude-opus-5-5","role":"assistant","content":[{"type":"text","text":"ok"}],` +
		`"usage":{"input_tokens":2,"cache_creation_input_tokens":10,"cache_read_input_tokens":20,"output_tokens":30}}}` + "\n"
	p := filepath.Join(home, ".claude", "projects", strings.NewReplacer("/", "-", ".", "-").Replace(cwd), id+".jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(line), 0o644))
	old := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(p, old, old))
}

// capturedSessions is the sessions with a usage file under project, sorted.
func capturedSessions(t *testing.T, project string) []string {
	t.Helper()
	ids := []string{}
	for _, p := range usageFiles(t, project) {
		ids = append(ids, strings.TrimSuffix(filepath.Base(p), ".yml"))
	}
	sort.Strings(ids)
	return ids
}

// monorepo lays one repository holding two Tiller projects, apps/a and
// apps/b, each with capture on, and a session that ran in each project, in a
// subdirectory of each, and at the repository root.
func monorepo(t *testing.T) (repo, home string) {
	t.Helper()
	repo = realTempDir(t)
	testgit.Init(t, repo)
	for _, app := range []string{"a", "b"} {
		usageProject(t, filepath.Join(repo, "apps", app))
		require.NoError(t, os.MkdirAll(filepath.Join(repo, "apps", app, "src"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(repo, "apps", app, "src", "main.go"), []byte("package main\n"), 0o644))
	}
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-m", "init")

	home = usageHome(t)
	layClaudeSessionIn(t, home, repo, "root")
	for _, app := range []string{"a", "b"} {
		layClaudeSessionIn(t, home, filepath.Join(repo, "apps", app), app)
		layClaudeSessionIn(t, home, filepath.Join(repo, "apps", app, "src"), app+"-src")
	}
	return repo, home
}

// A project nested in a repository captures the sessions that ran in it and
// beneath it. The primary checkout's root is not one of its paths, so the
// sibling project's sessions, and one run at the repository root, are left
// out, with no linked worktree at all.
func TestUsageNestedProjectLeavesTheSiblingProjectOut(t *testing.T) {
	repo, _ := monorepo(t)
	a := filepath.Join(repo, "apps", "a")
	t.Chdir(a)

	out, _, code := execCLI(t, "usage", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, []string{"a", "a-src"}, capturedSessions(t, a))
	first := jsonData(t, out)["harnesses"].([]any)[0].(map[string]any)
	require.EqualValues(t, 2, first["sessions"])
	require.EqualValues(t, 2, first["written"])
	require.EqualValues(t, 124, first["tokens"])
	require.Empty(t, usageFiles(t, filepath.Join(repo, "apps", "b")))
}

// With a linked worktree, the project is found where it sits in that
// worktree: apps/a under the worktree's root, never the worktree's root, so
// the sibling's sessions there stay out as well.
func TestUsageNestedProjectFollowsItsPrefixIntoWorktrees(t *testing.T) {
	repo, home := monorepo(t)
	wt := filepath.Join(realTempDir(t), "wt")
	gitRun(t, repo, "worktree", "add", "-b", "side", wt)
	layClaudeSessionIn(t, home, wt, "wt-root")
	for _, app := range []string{"a", "b"} {
		layClaudeSessionIn(t, home, filepath.Join(wt, "apps", app), "wt-"+app)
		layClaudeSessionIn(t, home, filepath.Join(wt, "apps", app, "src"), "wt-"+app+"-src")
	}
	a := filepath.Join(repo, "apps", "a")
	t.Chdir(a)

	out, _, code := execCLI(t, "usage", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, []string{"a", "a-src", "wt-a", "wt-a-src"}, capturedSessions(t, a))
	first := jsonData(t, out)["harnesses"].([]any)[0].(map[string]any)
	require.EqualValues(t, 4, first["sessions"])
	require.EqualValues(t, 248, first["tokens"])

	// The sibling sees the mirror image.
	b := filepath.Join(repo, "apps", "b")
	t.Chdir(b)
	out, _, code = execCLI(t, "usage", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, []string{"b", "b-src", "wt-b", "wt-b-src"}, capturedSessions(t, b))
}

// A project at the repository root keeps covering every worktree whole.
func TestUsageRootProjectCoversItsWorktrees(t *testing.T) {
	repo := realTempDir(t)
	testgit.Init(t, repo)
	usageProject(t, repo)
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-m", "init")
	wt := filepath.Join(realTempDir(t), "wt")
	gitRun(t, repo, "worktree", "add", "-b", "side", wt)
	home := usageHome(t)
	layClaudeSessionIn(t, home, repo, "root")
	layClaudeSessionIn(t, home, filepath.Join(repo, "src"), "root-src")
	layClaudeSessionIn(t, home, wt, "wt")
	layClaudeSessionIn(t, home, filepath.Join(wt, "src"), "wt-src")
	layClaudeSessionIn(t, home, filepath.Dir(wt), "outside")
	t.Chdir(repo)

	out, _, code := execCLI(t, "usage", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, []string{"root", "root-src", "wt", "wt-src"}, capturedSessions(t, repo))
}
