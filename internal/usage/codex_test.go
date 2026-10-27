package usage

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func codexRollout(cwd string) string {
	return `{"timestamp":"2026-09-07T19:41:17.000Z","type":"session_meta","payload":{"id":"x","cwd":"` + cwd + `"}}` + "\n"
}

func TestCodexMatchesByFirstLineCwd(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "09", "07")
	mustWrite(t, filepath.Join(dir, "rollout-a.jsonl"), codexRollout("/p/app"))
	mustWrite(t, filepath.Join(dir, "rollout-b.jsonl"), codexRollout("/p/other"))
	mustWrite(t, filepath.Join(dir, "rollout-c.jsonl"), codexRollout("/p/app/sub"))
	mustWrite(t, filepath.Join(dir, "rollout-d.jsonl"), codexRollout("/p/app-two"))
	mustWrite(t, filepath.Join(dir, "rollout-e.jsonl"), "not json\n")

	refs, err := NewCodexReader().Discover(Roots{Home: home, Project: []string{"/p/app"}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range refs {
		if r.Harness != "codex" || r.Parent != "" {
			t.Errorf("ref = %+v", r)
		}
		got = append(got, r.Session)
	}
	if want := []string{"rollout-a", "rollout-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sessions = %v, want %v", got, want)
	}
}

func TestCodexHonorsCodexHome(t *testing.T) {
	home, ch := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(ch, "sessions", "2026", "09", "07", "rollout-z.jsonl"), codexRollout("/p/app"))
	mustWrite(t, filepath.Join(home, ".codex", "sessions", "2026", "09", "07", "rollout-h.jsonl"), codexRollout("/p/app"))
	env := func(k string) string {
		if k == "CODEX_HOME" {
			return ch
		}
		return ""
	}
	refs, err := NewCodexReader().Discover(Roots{Home: home, Env: env, Project: []string{"/p/app"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Session != "rollout-z" {
		t.Fatalf("refs = %+v", refs)
	}
}

func TestCodexSessionCwdMissingFile(t *testing.T) {
	if _, err := codexSessionCwd(filepath.Join(t.TempDir(), "nope.jsonl")); err == nil {
		t.Fatal("want error")
	}
}

func TestCodexInputExcludesCached(t *testing.T) {
	path := filepath.Join("testdata", "codex", "rollout.jsonl")
	s, err := NewCodexReader().Parse([]LogRef{{Harness: "codex", Path: path, Session: "rollout"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Harness != "codex" || s.ID != "rollout" || len(s.Calls) != 2 {
		t.Fatalf("session = %+v", s)
	}
	c := s.Calls[0]
	if c.Tokens != (Tokens{Input: 16631, CacheWrite: 3, CacheRead: 6016, Output: 184, Reasoning: 122}) {
		t.Errorf("tokens = %+v", c.Tokens)
	}
	if c.Model != "gpt-6.1-sol" {
		t.Errorf("model = %q", c.Model)
	}
	if want := time.Date(2026, 9, 7, 19, 41, 22, 0, time.UTC); !c.At.Equal(want) {
		t.Errorf("at = %v", c.At)
	}
}

func TestCodexMarkersAttachToNextTokenCount(t *testing.T) {
	path := filepath.Join("testdata", "codex", "rollout.jsonl")
	s, err := NewCodexReader().Parse([]LogRef{{Harness: "codex", Path: path, Session: "rollout"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Marker{
		{Kind: MarkVerb, Stage: Implementation, Ref: "T-abc123", Verb: "start"},
		{Kind: MarkVerb, Stage: Checks, Ref: "T-abc123", Verb: "check"},
	}
	if !reflect.DeepEqual(s.Calls[0].Markers, want) {
		t.Errorf("markers = %+v, want %+v", s.Calls[0].Markers, want)
	}
	if len(s.Calls[1].Markers) != 0 {
		t.Errorf("second call markers = %+v", s.Calls[1].Markers)
	}
}

func TestCodexFirstLineNotSessionMetaOrTooLongIsNotDiscovered(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "09", "07")
	mustWrite(t, filepath.Join(dir, "rollout-a.jsonl"), `{"type":"turn_context","payload":{"cwd":"/p/app"}}`+"\n")
	big := `{"type":"session_meta","payload":{"cwd":"/p/app","instructions":"` + strings.Repeat("x", maxLineBytes+10) + `"}}` + "\n"
	mustWrite(t, filepath.Join(dir, "rollout-b.jsonl"), big)
	mustWrite(t, filepath.Join(dir, "rollout-c.jsonl"), codexRollout("/p/app"))
	refs, err := NewCodexReader().Discover(Roots{Home: home, Project: []string{"/p/app"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Session != "rollout-c" {
		t.Fatalf("refs = %+v", refs)
	}
}

func TestCodexCmdArgumentsStillYieldMarkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.jsonl")
	mustWrite(t, path, `{"timestamp":"2026-09-07T19:41:19.000Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"tiller start task T-abc123\"}"}}`+"\n"+
		`{"timestamp":"2026-09-07T19:41:22.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10,"output_tokens":1}}}}`+"\n")
	calls, err := parseCodexLog(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []Marker{{Kind: MarkVerb, Stage: Implementation, Ref: "T-abc123", Verb: "start"}}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0].Markers, want) {
		t.Fatalf("calls = %+v", calls)
	}
}
