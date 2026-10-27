package usage

import (
	"fmt"
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

func TestCodexDiscoverReadsEachFirstLineOnce(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "09", "07")
	a, b := filepath.Join(dir, "rollout-a.jsonl"), filepath.Join(dir, "rollout-b.jsonl")
	mustWrite(t, a, codexRollout("/p/app"))
	mustWrite(t, b, codexRollout("/p/other"))
	memo := newCwdMemo(nil)
	if _, err := NewCodexReader().Discover(Roots{Home: home, Project: []string{"/p/app"}, cwd: memo}); err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{a: "/p/app", b: "/p/other"}; !reflect.DeepEqual(memo.seen, want) {
		t.Fatalf("memo = %v, want %v", memo.seen, want)
	}

	// Known files are not opened again; a new one is read once and added.
	unreadable(t, a, b)
	c := filepath.Join(dir, "rollout-c.jsonl")
	mustWrite(t, c, codexRollout("/p/app"))
	next := newCwdMemo(memo.seen)
	refs, err := NewCodexReader().Discover(Roots{Home: home, Project: []string{"/p/app"}, cwd: next})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range refs {
		got = append(got, r.Session)
	}
	if want := []string{"rollout-a", "rollout-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sessions = %v, want %v", got, want)
	}
	if want := map[string]string{a: "/p/app", b: "/p/other", c: "/p/app"}; !reflect.DeepEqual(next.seen, want) {
		t.Fatalf("memo = %v, want %v", next.seen, want)
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
	if s.Harness != "codex" || s.ID != "rollout" || len(s.Calls) != 3 {
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
		{Kind: MarkVerb, Stage: Implementation, Refs: []string{"T-abc123"}, Verb: "start"},
		{Kind: MarkVerb, Stage: Checks, Refs: []string{"T-abc123"}, Verb: "check"},
	}
	if !reflect.DeepEqual(s.Calls[0].Markers, want) {
		t.Errorf("markers = %+v, want %+v", s.Calls[0].Markers, want)
	}
	if len(s.Calls[1].Markers) != 0 {
		t.Errorf("second call markers = %+v", s.Calls[1].Markers)
	}
}

// TestCodexJSWrappedExecCreditsTheNode pins the real custom_tool_call shape:
// the exec input is JS calling tools.exec_command with the command in a JS
// string, escaped newline and all. Both commands mark, and Credit puts the
// call on the node they name.
func TestCodexJSWrappedExecCreditsTheNode(t *testing.T) {
	path := filepath.Join("testdata", "codex", "rollout.jsonl")
	s, err := NewCodexReader().Parse([]LogRef{{Harness: "codex", Path: path, Session: "rollout"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Calls) != 3 {
		t.Fatalf("calls = %+v", s.Calls)
	}
	want := []Marker{
		{Kind: MarkVerb, Stage: Implementation, Refs: []string{"T-def456"}, Verb: "start"},
		{Kind: MarkVerb, Stage: Checks, Refs: []string{"T-def456"}, Verb: "check"},
	}
	if !reflect.DeepEqual(s.Calls[2].Markers, want) {
		t.Errorf("markers = %+v, want %+v", s.Calls[2].Markers, want)
	}
	resolve := func(ref string) string {
		if ref == "T-abc123" || ref == "T-def456" {
			return ref
		}
		return ""
	}
	rows := Credit(s, resolve)
	if r, ok := find(rows, "T-def456", Checks); !ok || r.Calls != 1 || r.Total() != 210 {
		t.Errorf("rows = %+v", rows)
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
	want := []Marker{{Kind: MarkVerb, Stage: Implementation, Refs: []string{"T-abc123"}, Verb: "start"}}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0].Markers, want) {
		t.Fatalf("calls = %+v", calls)
	}
}

// TestCodexRepeatedTokenCountIsSkipped pins the double-count guard: Codex
// sometimes writes a token_count again with an unchanged total_token_usage,
// repeating the last call's usage. The repeat is not a call; markers seen
// before it wait for the next real one. A token_count with no running total
// is always a call.
func TestCodexRepeatedTokenCountIsSkipped(t *testing.T) {
	tc := func(ts, total string, last int) string {
		info := fmt.Sprintf(`"last_token_usage":{"input_tokens":%d,"output_tokens":1,"total_tokens":%d}`, last, last+1)
		if total != "" {
			info = `"total_token_usage":` + total + "," + info
		}
		return `{"timestamp":"2026-09-07T19:41:` + ts + `.000Z","type":"event_msg","payload":{"type":"token_count","info":{` + info + `}}}` + "\n"
	}
	marker := `{"timestamp":"2026-09-07T19:41:20.000Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"tiller start task T-abc123\"}"}}` + "\n"
	path := filepath.Join(t.TempDir(), "r.jsonl")
	mustWrite(t, path,
		tc("10", `{"input_tokens":10,"output_tokens":1,"total_tokens":11}`, 10)+
			marker+
			tc("21", `{"input_tokens":10,"output_tokens":1,"total_tokens":11}`, 10)+
			tc("30", `{"input_tokens":30,"output_tokens":2,"total_tokens":32}`, 20)+
			tc("40", "", 5)+
			tc("41", "", 5))
	calls, err := parseCodexLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 4 {
		t.Fatalf("calls = %+v", calls)
	}
	var sum int64
	for _, c := range calls {
		sum += c.Tokens.Input + c.Tokens.Output
	}
	if sum != 11+21+6+6 {
		t.Errorf("tokens = %d", sum)
	}
	if len(calls[0].Markers) != 0 || len(calls[1].Markers) != 1 || calls[1].Markers[0].Verb != "start" {
		t.Errorf("markers = %+v / %+v", calls[0].Markers, calls[1].Markers)
	}
}
