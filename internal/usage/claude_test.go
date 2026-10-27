package usage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", "claude", name)
}

func TestClaudeDiscoverMatchesRootAndWorktreePrefix(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, ".claude", "projects")
	line := `{"type":"assistant"}` + "\n"
	mustWrite(t, filepath.Join(proj, "-p-app", "s1.jsonl"), line)
	mustWrite(t, filepath.Join(proj, "-p-app", "s1", "subagents", "agent-a.jsonl"), line)
	mustWrite(t, filepath.Join(proj, "-p-app--worktrees-x", "s2.jsonl"), line)
	mustWrite(t, filepath.Join(proj, "-p-other", "s3.jsonl"), line)
	mustWrite(t, filepath.Join(proj, "-p-app-two", "s4.jsonl"), line)
	mustWrite(t, filepath.Join(proj, "-p-app--claude-worktrees-gone", "s5.jsonl"), line)
	mustWrite(t, filepath.Join(proj, "-p-app-wt", "s6.jsonl"), line)

	refs, err := NewClaudeReader().Discover(Roots{Home: home, Project: []string{"/p/app", "/p/app-wt"}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]LogRef{}
	for _, r := range refs {
		if r.Harness != "claude-code" {
			t.Errorf("harness = %q", r.Harness)
		}
		got[r.Path] = r
	}
	if len(got) != 5 {
		t.Fatalf("refs = %+v, want 5", refs)
	}
	for _, folder := range []string{"-p-app-two", "-p-other"} {
		for path := range got {
			if strings.Contains(path, string(filepath.Separator)+folder+string(filepath.Separator)) {
				t.Errorf("%s discovered", folder)
			}
		}
	}
	for _, folder := range []string{"-p-app--claude-worktrees-gone", "-p-app-wt"} {
		if _, ok := got[filepath.Join(proj, folder, map[string]string{"-p-app--claude-worktrees-gone": "s5", "-p-app-wt": "s6"}[folder]+".jsonl")]; !ok {
			t.Errorf("%s not discovered", folder)
		}
	}
	top := got[filepath.Join(proj, "-p-app", "s1.jsonl")]
	if top.Session != "s1" || top.Parent != "" {
		t.Errorf("top = %+v", top)
	}
	sub := got[filepath.Join(proj, "-p-app", "s1", "subagents", "agent-a.jsonl")]
	if sub.Parent != "s1" || sub.Session != "agent-a" {
		t.Errorf("sub = %+v", sub)
	}
	wt := got[filepath.Join(proj, "-p-app--worktrees-x", "s2.jsonl")]
	if wt.Session != "s2" || wt.Parent != "" {
		t.Errorf("worktree = %+v", wt)
	}
}

func TestClaudeHonorsConfigDir(t *testing.T) {
	home := t.TempDir()
	cfg := t.TempDir()
	line := `{"type":"assistant"}` + "\n"
	mustWrite(t, filepath.Join(home, ".claude", "projects", "-p-app", "old.jsonl"), line)
	mustWrite(t, filepath.Join(cfg, "projects", "-p-app", "new.jsonl"), line)
	env := func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return cfg
		}
		return ""
	}
	refs, err := NewClaudeReader().Discover(Roots{Home: home, Env: env, Project: []string{"/p/app"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Session != "new" {
		t.Fatalf("refs = %+v", refs)
	}
}

func TestClaudeDiscoverNoProjectsDir(t *testing.T) {
	refs, err := NewClaudeReader().Discover(Roots{Home: t.TempDir(), Project: []string{"/p/app"}})
	if err != nil || len(refs) != 0 {
		t.Fatalf("refs = %v, err = %v", refs, err)
	}
}

func parseOne(t *testing.T, path string) Session {
	t.Helper()
	s, err := NewClaudeReader().Parse([]LogRef{{Harness: "claude-code", Path: path, Session: "s1"}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestClaudeParseDedupesByMessageID(t *testing.T) {
	s := parseOne(t, fixture(t, "split.jsonl"))
	if s.Harness != "claude-code" || s.ID != "s1" {
		t.Errorf("session = %s/%s", s.Harness, s.ID)
	}
	if len(s.Calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(s.Calls))
	}
	c := s.Calls[0]
	if c.Tokens.Input != 2 || c.Tokens.Output != 140 {
		t.Errorf("usage counted more than once: %+v", c.Tokens)
	}
	if len(c.Markers) != 2 || c.Markers[0].Verb != "start" || !reflect.DeepEqual(c.Markers[0].Refs, []string{"T-abc123"}) ||
		c.Markers[1].Kind != MarkSkill || c.Markers[1].Stage != Implementation {
		t.Errorf("markers = %+v", c.Markers)
	}
	if c.Branch != "main" || s.Calls[1].Branch != "feature" {
		t.Errorf("branches = %q, %q", c.Branch, s.Calls[1].Branch)
	}
}

func TestClaudeParseMapsTokens(t *testing.T) {
	s := parseOne(t, fixture(t, "split.jsonl"))
	c := s.Calls[0]
	want := Tokens{Input: 2, CacheWrite: 29140, CacheRead: 30444, Output: 140, Reasoning: 41}
	if c.Tokens != want {
		t.Errorf("tokens = %+v, want %+v", c.Tokens, want)
	}
	if c.Model != "claude-opus-5-5" {
		t.Errorf("model = %q", c.Model)
	}
	if c.At.UTC().Format("2006-01-02T15:04:05Z") != "2026-10-20T14:03:55Z" {
		t.Errorf("at = %v", c.At)
	}
}

func TestClaudeParseSkipsSynthetic(t *testing.T) {
	s := parseOne(t, fixture(t, "synthetic.jsonl"))
	if len(s.Calls) != 1 || s.Calls[0].Model != "claude-opus-5-5" {
		t.Fatalf("calls = %+v", s.Calls)
	}
}

func TestClaudeParseSkipsOversizedLine(t *testing.T) {
	mk := func(id, ts string) string {
		return `{"type":"assistant","timestamp":"` + ts + `","message":{"id":"` + id + `","model":"m","content":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n"
	}
	big := `{"type":"assistant","timestamp":"2026-10-20T14:00:30Z","message":{"id":"big","model":"m","usage":{"input_tokens":9,"output_tokens":9},"content":[{"type":"tool_result","text":"` +
		strings.Repeat("x", 9<<20) + `"}]}}` + "\n"
	path := filepath.Join(t.TempDir(), "s1.jsonl")
	mustWrite(t, path, mk("a", "2026-10-20T14:00:00Z")+big+mk("b", "2026-10-20T14:01:00Z"))
	s := parseOne(t, path)
	if len(s.Calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(s.Calls))
	}
}

func TestClaudeParseSkipsMalformedAndOrdersByTime(t *testing.T) {
	mk := func(id, ts string) string {
		return `{"type":"assistant","timestamp":"` + ts + `","message":{"id":"` + id + `","model":"m","content":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n"
	}
	path := filepath.Join(t.TempDir(), "s1.jsonl")
	mustWrite(t, path, mk("late", "2026-10-20T15:00:00Z")+`{"type":"assistant", broken`+"\n"+mk("early", "2026-10-20T14:00:00Z"))
	s := parseOne(t, path)
	if len(s.Calls) != 2 || !s.Calls[0].At.Before(s.Calls[1].At) {
		t.Fatalf("calls = %+v", s.Calls)
	}
}

func TestClaudeParseSubagents(t *testing.T) {
	dir := t.TempDir()
	call := func(id string) string {
		return `{"type":"assistant","timestamp":"2026-10-20T14:00:00Z","message":{"id":"` + id + `","model":"m","content":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n"
	}
	top := filepath.Join(dir, "s1.jsonl")
	sub := filepath.Join(dir, "s1", "subagents", "agent-a.jsonl")
	mustWrite(t, top, call("t"))
	mustWrite(t, sub, call("u"))
	s, err := NewClaudeReader().Parse([]LogRef{
		{Harness: "claude-code", Path: sub, Session: "agent-a", Parent: "s1"},
		{Harness: "claude-code", Path: top, Session: "s1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "s1" || len(s.Calls) != 1 || len(s.Subagents) != 1 || s.Subagents[0].ID != "agent-a" || len(s.Subagents[0].Calls) != 1 {
		t.Fatalf("session = %+v", s)
	}
}

// TestClaudeParseCountsAResponseOnceAcrossTheSession pins the cross-file
// guard: real Claude Code subagent logs can repeat a response another log of
// the same session already holds (same message and request id). It counts
// once, in the first log that holds it (the top-level log first); the same
// message id under another request id is a different response.
func TestClaudeParseCountsAResponseOnceAcrossTheSession(t *testing.T) {
	dir := t.TempDir()
	call := func(id, req, ts string) string {
		r := ""
		if req != "" {
			r = `"requestId":"` + req + `",`
		}
		return `{"type":"assistant",` + r + `"timestamp":"` + ts + `","message":{"id":"` + id + `","model":"m","content":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n"
	}
	top := filepath.Join(dir, "s1.jsonl")
	subA := filepath.Join(dir, "s1", "subagents", "agent-a.jsonl")
	subB := filepath.Join(dir, "s1", "subagents", "agent-b.jsonl")
	mustWrite(t, top, call("shared", "r1", "2026-10-20T14:00:00Z"))
	mustWrite(t, subA, call("shared", "r1", "2026-10-20T14:00:00Z")+call("fork", "r2", "2026-10-20T14:01:00Z")+call("a-own", "", "2026-10-20T14:02:00Z"))
	mustWrite(t, subB, call("fork", "r2", "2026-10-20T14:01:00Z")+call("fork", "r3", "2026-10-20T14:03:00Z")+call("b-own", "", "2026-10-20T14:04:00Z"))
	s, err := NewClaudeReader().Parse([]LogRef{
		{Harness: "claude-code", Path: top, Session: "s1"},
		{Harness: "claude-code", Path: subA, Session: "agent-a", Parent: "s1"},
		{Harness: "claude-code", Path: subB, Session: "agent-b", Parent: "s1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Calls) != 1 || len(s.Subagents) != 2 {
		t.Fatalf("session = %+v", s)
	}
	if n := len(s.Subagents[0].Calls); n != 2 {
		t.Errorf("agent-a calls = %d, want 2 (fork, a-own)", n)
	}
	if n := len(s.Subagents[1].Calls); n != 2 {
		t.Errorf("agent-b calls = %d, want 2 (fork under r3, b-own)", n)
	}
}
