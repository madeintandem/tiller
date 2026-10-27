package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var captureNow = time.Date(2026, 10, 20, 20, 0, 0, 0, time.UTC)

// claudeCall is one assistant line; a non-empty cmd adds a Bash tool call.
func claudeCall(at time.Time, id, cmd string) string {
	content := `[]`
	if cmd != "" {
		content = fmt.Sprintf(`[{"type":"tool_use","name":"Bash","input":{"command":%q}}]`, cmd)
	}
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"id":%q,"model":"claude-opus-5-5","content":%s,"usage":{"input_tokens":10,"cache_creation_input_tokens":20,"cache_read_input_tokens":100,"output_tokens":5}}}`+"\n",
		at.Format(time.RFC3339Nano), id, content)
}

// claudeSession writes a Claude log for session id under the folder encoded
// for dir, one call at each time; the first call starts work on T-abc123.
func claudeSession(t *testing.T, home, dir, id string, times ...time.Time) string {
	t.Helper()
	var b strings.Builder
	for i, at := range times {
		cmd := ""
		if i == 0 {
			cmd = "tiller start T-abc123"
		}
		b.WriteString(claudeCall(at, fmt.Sprintf("%s-%d", id, i), cmd))
	}
	path := filepath.Join(home, ".claude", "projects", claudeFolder(dir), id+".jsonl")
	mustWrite(t, path, b.String())
	touch(t, path, times[len(times)-1])
	return path
}

// touch sets a log's modification time, as the harness would have when it
// wrote its last line.
func touch(tb testing.TB, path string, at time.Time) {
	tb.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		tb.Fatal(err)
	}
}

// commitWrites lays the planned writes down, standing in for tree.Commit.
func commitWrites(tb testing.TB, res Result) {
	tb.Helper()
	for _, w := range res.Writes {
		if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(w.Path, w.Data, 0o644); err != nil {
			tb.Fatal(err)
		}
	}
}

// unreadable makes the files fail to open (stat still works) until cleanup.
func unreadable(tb testing.TB, paths ...string) {
	tb.Helper()
	for _, p := range paths {
		if err := os.Chmod(p, 0); err != nil {
			tb.Fatal(err)
		}
		tb.Cleanup(func() { os.Chmod(p, 0o644) })
	}
}

type captureEnv struct {
	home, root string
}

func newCaptureEnv(t *testing.T) captureEnv {
	t.Helper()
	return captureEnv{home: t.TempDir(), root: t.TempDir()}
}

func (e captureEnv) options(readers ...Reader) Options {
	if len(readers) == 0 {
		readers = []Reader{NewClaudeReader()}
	}
	return Options{
		Root:    e.root,
		Roots:   Roots{Home: e.home, Project: []string{e.root}},
		Readers: readers,
		Resolve: func(ref string) string {
			if ref == "T-abc123" {
				return ref
			}
			return ""
		},
		Now: captureNow,
	}
}

func mustCapture(t *testing.T, o Options) (Result, Cursor) {
	t.Helper()
	res, cur, err := Capture(o)
	if err != nil {
		t.Fatal(err)
	}
	return res, cur
}

func harnessResult(t *testing.T, res Result, harness string) HarnessResult {
	t.Helper()
	for _, h := range res.Harnesses {
		if h.Harness == harness {
			return h
		}
	}
	t.Fatalf("no result for %s in %+v", harness, res.Harnesses)
	return HarnessResult{}
}

// countingReader counts Parse calls and can slow each one down.
type countingReader struct {
	Reader
	parses int
	delay  time.Duration
}

func (c *countingReader) Parse(logs []LogRef) (Session, error) {
	c.parses++
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	return c.Reader.Parse(logs)
}

func TestCaptureWritesSettledSessions(t *testing.T) {
	e := newCaptureEnv(t)
	claudeSession(t, e.home, e.root, "s1", captureNow.Add(-3*time.Hour), captureNow.Add(-2*time.Hour))

	res, cur := mustCapture(t, e.options())
	if len(res.Writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(res.Writes))
	}
	w := res.Writes[0]
	if want := filepath.Join(e.root, ".tiller", "usage", "claude-code", "s1.yml"); w.Path != want {
		t.Errorf("path = %s, want %s", w.Path, want)
	}
	if w.Before.Exists() {
		t.Error("snapshot of a new file should be absent")
	}
	f, err := Decode(w.Data)
	if err != nil {
		t.Fatal(err)
	}
	if f.Harness != "claude-code" || f.Session != "s1" || f.Reader != ReaderVersion || !f.CapturedAt.Equal(captureNow) {
		t.Errorf("header = %+v", f)
	}
	if !f.FirstCall.Equal(captureNow.Add(-3*time.Hour)) || !f.LastCall.Equal(captureNow.Add(-2*time.Hour)) {
		t.Errorf("first/last = %v %v", f.FirstCall, f.LastCall)
	}
	if f.Calls() != 2 {
		t.Fatalf("calls = %d, want 2", f.Calls())
	}
	for _, r := range f.Rows {
		if r.Node != "T-abc123" || r.Stage != Implementation || r.Model != "claude-opus-5-5" {
			t.Errorf("row = %+v", r)
		}
	}
	h := harnessResult(t, res, "claude-code")
	want := HarnessResult{Harness: "claude-code", Found: true, Sessions: 1, Written: 1, Tokens: 270, Tied: 270}
	if h != want {
		t.Errorf("harness = %+v, want %+v", h, want)
	}
	if res.Partial {
		t.Error("partial without a budget")
	}
	if len(cur.Logs) != 1 {
		t.Errorf("cursor = %+v, want one entry", cur)
	}
}

func TestCaptureSkipsActiveSession(t *testing.T) {
	e := newCaptureEnv(t)
	path := claudeSession(t, e.home, e.root, "s1", captureNow.Add(-time.Hour), captureNow.Add(-10*time.Minute))
	sub := filepath.Join(e.home, ".claude", "projects", claudeFolder(e.root), "s1", "subagents", "agent-a.jsonl")
	mustWrite(t, sub, claudeCall(captureNow.Add(-50*time.Minute), "a-0", ""))
	touch(t, sub, captureNow.Add(-50*time.Minute))
	unreadable(t, path, sub)
	r := &countingReader{Reader: NewClaudeReader()}

	// A group whose newest log was written within SettleAfter is not opened,
	// even when an older subagent log sits beside it.
	res, cur := mustCapture(t, e.options(r))
	if len(res.Writes) != 0 || r.parses != 0 {
		t.Fatalf("writes = %d parses = %d, want 0 and 0 for an active session", len(res.Writes), r.parses)
	}
	if len(cur.Logs) != 0 {
		t.Fatalf("cursor = %+v: an unsettled session must not enter the cursor", cur.Logs)
	}

	// Once it has gone quiet the same, unchanged log is captured.
	if err := cur.Save(e.root); err != nil {
		t.Fatal(err)
	}
	os.Chmod(path, 0o644)
	os.Chmod(sub, 0o644)
	later := e.options(r)
	later.Now = captureNow.Add(time.Hour)
	res, _ = mustCapture(t, later)
	if len(res.Writes) != 1 || r.parses != 1 {
		t.Fatalf("writes = %d parses = %d, want 1 and 1 once settled", len(res.Writes), r.parses)
	}
}

// A log's modification time can predate its last call only if something reset
// it; the parsed last call still decides.
func TestCaptureSettleCheckUsesLastCall(t *testing.T) {
	e := newCaptureEnv(t)
	path := claudeSession(t, e.home, e.root, "s1", captureNow.Add(-time.Hour), captureNow.Add(-10*time.Minute))
	touch(t, path, captureNow.Add(-2*time.Hour))
	r := &countingReader{Reader: NewClaudeReader()}

	res, cur := mustCapture(t, e.options(r))
	if len(res.Writes) != 0 || r.parses != 1 || len(cur.Logs) != 0 {
		t.Fatalf("writes = %d parses = %d cursor = %d, want 0, 1, 0", len(res.Writes), r.parses, len(cur.Logs))
	}
}

func TestCaptureRecapturesDiscardedFile(t *testing.T) {
	e := newCaptureEnv(t)
	claudeSession(t, e.home, e.root, "s1", captureNow.Add(-3*time.Hour))
	claudeSession(t, e.home, e.root, "s2", captureNow.Add(-3*time.Hour))
	res, cur := mustCapture(t, e.options())
	if len(res.Writes) != 2 {
		t.Fatalf("writes = %d, want 2", len(res.Writes))
	}
	commitWrites(t, res)
	if err := cur.Save(e.root); err != nil {
		t.Fatal(err)
	}

	// The uncommitted s1 file is discarded (git checkout, clean, reset).
	if err := os.Remove(res.Writes[0].Path); err != nil {
		t.Fatal(err)
	}
	r := &countingReader{Reader: NewClaudeReader()}
	again, _ := mustCapture(t, e.options(r))
	if len(again.Writes) != 1 || again.Writes[0].Path != res.Writes[0].Path || r.parses != 1 {
		t.Fatalf("writes = %+v parses = %d, want only the discarded file rewritten", again.Writes, r.parses)
	}
}

func codexCall(at time.Time) string {
	return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10,"cached_input_tokens":4,"output_tokens":3}}}}`+"\n", at.Format(time.RFC3339Nano))
}

func TestCaptureKeepsCodexCwdMemo(t *testing.T) {
	e := newCaptureEnv(t)
	dir := filepath.Join(e.home, ".codex", "sessions", "2026", "10", "20")
	ours := filepath.Join(dir, "rollout-ours.jsonl")
	foreign := filepath.Join(dir, "rollout-foreign.jsonl")
	mustWrite(t, ours, codexRollout(e.root)+codexCall(captureNow.Add(-3*time.Hour)))
	mustWrite(t, foreign, codexRollout("/elsewhere")+codexCall(captureNow.Add(-3*time.Hour)))
	touch(t, ours, captureNow.Add(-3*time.Hour))
	touch(t, foreign, captureNow.Add(-3*time.Hour))
	stale := Cursor{Logs: map[string]cursorEntry{}, Cwd: map[string]string{filepath.Join(dir, "rollout-gone.jsonl"): e.root}}
	if err := stale.Save(e.root); err != nil {
		t.Fatal(err)
	}

	res, cur := mustCapture(t, e.options(NewCodexReader()))
	if len(res.Writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(res.Writes))
	}
	want := map[string]string{ours: e.root, foreign: "/elsewhere"}
	if !reflect.DeepEqual(cur.Cwd, want) {
		t.Errorf("cwd memo = %v, want %v (gone path dropped)", cur.Cwd, want)
	}
}

// A session started in a subdirectory of the checkout is captured, and the
// cursor keeps its folder's working directory so the folder is read once.
func TestCaptureFindsSubdirectorySessionAndRemembersItsFolder(t *testing.T) {
	e := newCaptureEnv(t)
	ui := filepath.Join(e.root, "ui")
	at := captureNow.Add(-3 * time.Hour)
	path := filepath.Join(e.home, ".claude", "projects", claudeFolder(ui), "s1.jsonl")
	mustWrite(t, path, `{"type":"user","cwd":"`+ui+`"}`+"\n"+claudeCall(at, "m1", "tiller start T-abc123"))
	touch(t, path, at)

	res, cur := mustCapture(t, e.options())
	if len(res.Writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(res.Writes))
	}
	if got := cur.Cwd[filepath.Dir(path)]; got != ui {
		t.Errorf("cursor cwd for the folder = %q, want %q (all: %v)", got, ui, cur.Cwd)
	}
}

func TestCaptureSkipsSessionsBeforeSince(t *testing.T) {
	e := newCaptureEnv(t)
	claudeSession(t, e.home, e.root, "old", captureNow.Add(-3*time.Hour), captureNow.Add(-2*time.Hour))
	claudeSession(t, e.home, e.root, "new", captureNow.Add(-50*time.Minute), captureNow.Add(-40*time.Minute))

	o := e.options()
	o.Since = func() (time.Time, error) { return captureNow.Add(-time.Hour), nil }
	res, _ := mustCapture(t, o)
	if len(res.Writes) != 1 || !strings.HasSuffix(res.Writes[0].Path, "new.yml") {
		t.Fatalf("writes = %+v, want only the session after Since", res.Writes)
	}
}

func writeUsageFile(t *testing.T, root string, f File) []byte {
	t.Helper()
	data, err := Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, filepath.FromSlash(Path(f.Harness, f.Session))), string(data))
	return data
}

func TestCaptureNeverShrinks(t *testing.T) {
	e := newCaptureEnv(t)
	times := make([]time.Time, 20)
	for i := range times {
		times[i] = captureNow.Add(-3*time.Hour + time.Duration(i)*time.Minute)
	}
	claudeSession(t, e.home, e.root, "s1", times...)
	big := writeUsageFile(t, e.root, File{Harness: "claude-code", Session: "s1", Reader: ReaderVersion,
		Rows: []Row{{Hour: captureNow.Add(-4 * time.Hour).Truncate(time.Hour), Stage: Other, Model: "m", Calls: 50}}})
	gone := writeUsageFile(t, e.root, File{Harness: "claude-code", Session: "gone", Reader: ReaderVersion,
		Rows: []Row{{Hour: captureNow.Add(-9 * time.Hour).Truncate(time.Hour), Stage: Other, Model: "m", Calls: 7}}})

	res, _ := mustCapture(t, e.options())
	if len(res.Writes) != 0 {
		t.Fatalf("writes = %+v, want none", res.Writes)
	}
	for name, want := range map[string][]byte{"s1": big, "gone": gone} {
		got, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(Path("claude-code", name))))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s changed", name)
		}
	}
}

func TestCaptureGrowsAndLeavesIdenticalFilesAlone(t *testing.T) {
	e := newCaptureEnv(t)
	claudeSession(t, e.home, e.root, "s1", captureNow.Add(-3*time.Hour))
	res, _ := mustCapture(t, e.options())
	if len(res.Writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(res.Writes))
	}
	mustWrite(t, res.Writes[0].Path, string(res.Writes[0].Data))

	// A fresh machine (no cursor) recaptures the same session: nothing to write.
	again := e.options()
	again.Now = captureNow.Add(time.Hour)
	res, _ = mustCapture(t, again)
	if len(res.Writes) != 0 {
		t.Fatalf("writes = %d, want 0 for identical content", len(res.Writes))
	}

	// The session grew: the file is rewritten over its snapshot.
	claudeSession(t, e.home, e.root, "s1", captureNow.Add(-3*time.Hour), captureNow.Add(-2*time.Hour))
	res, _ = mustCapture(t, again)
	if len(res.Writes) != 1 || !res.Writes[0].Before.Exists() {
		t.Fatalf("writes = %+v, want one rewrite guarded by the existing file", res.Writes)
	}
	if f, _ := Decode(res.Writes[0].Data); f.Calls() != 2 {
		t.Errorf("calls = %d, want 2", f.Calls())
	}
}

func TestCaptureUnchangedLogIsNotOpened(t *testing.T) {
	e := newCaptureEnv(t)
	claudeSession(t, e.home, e.root, "s1", captureNow.Add(-3*time.Hour))
	r := &countingReader{Reader: NewClaudeReader()}
	first, cur := mustCapture(t, e.options(r))
	if r.parses != 1 {
		t.Fatalf("first capture parses = %d, want 1", r.parses)
	}
	commitWrites(t, first)
	if err := cur.Save(e.root); err != nil {
		t.Fatal(err)
	}

	r.parses = 0
	res, next := mustCapture(t, e.options(r))
	if r.parses != 0 {
		t.Errorf("parses = %d, want 0 for a log matching the cursor", r.parses)
	}
	if len(res.Writes) != 0 {
		t.Errorf("writes = %d, want 0", len(res.Writes))
	}
	if len(next.Logs) != 1 {
		t.Errorf("cursor = %+v, want the skipped log kept", next)
	}
	if h := harnessResult(t, res, "claude-code"); h.Sessions != 1 || h.Written != 0 {
		t.Errorf("harness = %+v", h)
	}
}

func TestCaptureReparsesGroupWhenASubagentChanged(t *testing.T) {
	e := newCaptureEnv(t)
	claudeSession(t, e.home, e.root, "s1", captureNow.Add(-3*time.Hour))
	sub := filepath.Join(e.home, ".claude", "projects", claudeFolder(e.root), "s1", "subagents", "agent-a.jsonl")
	mustWrite(t, sub, claudeCall(captureNow.Add(-170*time.Minute), "a-0", ""))
	touch(t, sub, captureNow.Add(-170*time.Minute))
	r := &countingReader{Reader: NewClaudeReader()}
	_, cur := mustCapture(t, e.options(r))
	if err := cur.Save(e.root); err != nil {
		t.Fatal(err)
	}

	mustWrite(t, sub, claudeCall(captureNow.Add(-170*time.Minute), "a-0", "")+claudeCall(captureNow.Add(-160*time.Minute), "a-1", ""))
	touch(t, sub, captureNow.Add(-160*time.Minute))
	r.parses = 0
	res, _ := mustCapture(t, e.options(r))
	if r.parses != 1 {
		t.Fatalf("parses = %d, want the group reparsed", r.parses)
	}
	if len(res.Writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(res.Writes))
	}
	if f, _ := Decode(res.Writes[0].Data); f.Calls() != 3 {
		t.Errorf("calls = %d, want 3 (one top-level, two subagent)", f.Calls())
	}
}

func TestCaptureFindsRemovedInRootWorktree(t *testing.T) {
	e := newCaptureEnv(t)
	claudeSession(t, e.home, filepath.Join(e.root, ".worktrees", "gone"), "wt", captureNow.Add(-3*time.Hour))

	res, _ := mustCapture(t, e.options())
	if len(res.Writes) != 1 || !strings.HasSuffix(res.Writes[0].Path, filepath.Join("claude-code", "wt.yml")) {
		t.Fatalf("writes = %+v, want the removed worktree's session", res.Writes)
	}
}

func TestCaptureBudgetStopsAtSessionBoundary(t *testing.T) {
	e := newCaptureEnv(t)
	for _, id := range []string{"s1", "s2", "s3"} {
		claudeSession(t, e.home, e.root, id, captureNow.Add(-3*time.Hour), captureNow.Add(-2*time.Hour))
	}
	r := &countingReader{Reader: NewClaudeReader(), delay: 5 * time.Millisecond}
	o := e.options(r)
	o.Budget = time.Millisecond

	res, cur := mustCapture(t, o)
	if !res.Partial {
		t.Fatal("want Partial")
	}
	if r.parses != 1 || len(res.Writes) != 1 {
		t.Fatalf("parses = %d writes = %d, want one whole session", r.parses, len(res.Writes))
	}
	if f, _ := Decode(res.Writes[0].Data); f.Calls() != 2 {
		t.Errorf("calls = %d, want the whole session", f.Calls())
	}
	if len(cur.Logs) != 1 {
		t.Errorf("cursor = %+v, want only the captured session", cur)
	}
}

func TestCaptureReportsMissingHarness(t *testing.T) {
	e := newCaptureEnv(t)
	claudeSession(t, e.home, e.root, "s1", captureNow.Add(-3*time.Hour))

	res, _ := mustCapture(t, e.options(NewClaudeReader(), NewCodexReader()))
	if h := harnessResult(t, res, "codex"); h != (HarnessResult{Harness: "codex", Found: false}) {
		t.Errorf("codex = %+v", h)
	}
	if h := harnessResult(t, res, "claude-code"); !h.Found {
		t.Errorf("claude = %+v", h)
	}
}

// noChangeFixture lays n settled logs and saves a cursor that matches them all.
func noChangeFixture(tb testing.TB, n int) (captureEnv, *countingReader) {
	tb.Helper()
	e := captureEnv{home: tb.TempDir(), root: tb.TempDir()}
	dir := filepath.Join(e.home, ".claude", "projects", claudeFolder(e.root))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatal(err)
	}
	var logs []string
	for i := range n {
		path := filepath.Join(dir, fmt.Sprintf("s%03d.jsonl", i))
		if err := os.WriteFile(path, []byte(claudeCall(captureNow.Add(-3*time.Hour), fmt.Sprintf("m%d", i), "")), 0o644); err != nil {
			tb.Fatal(err)
		}
		touch(tb, path, captureNow.Add(-3*time.Hour))
		logs = append(logs, path)
	}
	r := &countingReader{Reader: NewClaudeReader()}
	res, cur, err := Capture(e.options(r))
	if err != nil {
		tb.Fatal(err)
	}
	commitWrites(tb, res)
	if len(cur.Logs) != n {
		tb.Fatalf("cursor holds %d entries, want %d", len(cur.Logs), n)
	}
	if err := cur.Save(e.root); err != nil {
		tb.Fatal(err)
	}
	unreadable(tb, logs...)
	r.parses = 0
	return e, r
}

func TestCaptureNoChangeOpensNothing(t *testing.T) {
	e, r := noChangeFixture(t, 200)
	res, cur := mustCapture(t, e.options(r))
	if r.parses != 0 {
		t.Errorf("parses = %d, want 0", r.parses)
	}
	if len(res.Writes) != 0 || len(cur.Logs) != 200 {
		t.Errorf("writes = %d cursor = %d, want 0 and 200", len(res.Writes), len(cur.Logs))
	}
	if h := harnessResult(t, res, "claude-code"); h.Sessions != 200 {
		t.Errorf("sessions = %d, want 200", h.Sessions)
	}
}

// A capture with nothing new never asks for the project's earliest event, so
// sync's no-change capture does not read the event stream.
func TestCaptureNoChangeAsksNoSince(t *testing.T) {
	e, r := noChangeFixture(t, 3)
	o := e.options(r)
	asked := 0
	o.Since = func() (time.Time, error) { asked++; return time.Time{}, nil }
	mustCapture(t, o)
	if asked != 0 {
		t.Errorf("Since asked %d times, want 0", asked)
	}
}

func TestCaptureAsksSinceOnce(t *testing.T) {
	e := newCaptureEnv(t)
	claudeSession(t, e.home, e.root, "a", captureNow.Add(-3*time.Hour), captureNow.Add(-2*time.Hour))
	claudeSession(t, e.home, e.root, "b", captureNow.Add(-3*time.Hour), captureNow.Add(-2*time.Hour))
	o := e.options()
	asked := 0
	o.Since = func() (time.Time, error) { asked++; return captureNow.Add(-24 * time.Hour), nil }
	res, _ := mustCapture(t, o)
	if asked != 1 || len(res.Writes) != 2 {
		t.Errorf("Since asked %d times with %d writes, want 1 and 2", asked, len(res.Writes))
	}
}

// With no event yet there is no lower bound to judge a session by, so
// capture writes nothing and leaves the sessions out of the cursor: they are
// judged once the project has an event, not baked in as prior history.
func TestCaptureWithNoEventsWritesNothingAndRemembersNothing(t *testing.T) {
	e := newCaptureEnv(t)
	claudeSession(t, e.home, e.root, "a", captureNow.Add(-3*time.Hour), captureNow.Add(-2*time.Hour))
	claudeSession(t, e.home, e.root, "b", captureNow.Add(-50*time.Minute), captureNow.Add(-40*time.Minute))
	r := &countingReader{Reader: NewClaudeReader()}
	o := e.options(r)
	o.Since = func() (time.Time, error) { return time.Time{}, nil }
	res, cur := mustCapture(t, o)
	if len(res.Writes) != 0 || len(cur.Logs) != 0 {
		t.Fatalf("writes = %d cursor = %+v, want none", len(res.Writes), cur.Logs)
	}
	if r.parses != 0 {
		t.Errorf("parses = %d, want 0: no event means nothing to judge a session by", r.parses)
	}
	if err := cur.Save(e.root); err != nil {
		t.Fatal(err)
	}

	o.Since = func() (time.Time, error) { return captureNow.Add(-time.Hour), nil }
	res, _ = mustCapture(t, o)
	if len(res.Writes) != 1 || !strings.HasSuffix(res.Writes[0].Path, "b.yml") {
		t.Fatalf("writes = %+v, want the session after the first event", res.Writes)
	}
}

func BenchmarkCaptureNoChange(b *testing.B) {
	e, r := noChangeFixture(b, 200)
	o := e.options(r)
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := Capture(o); err != nil {
			b.Fatal(err)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	root := t.TempDir()
	empty := func(c Cursor) bool { return len(c.Logs) == 0 && len(c.Cwd) == 0 && c.Logs != nil && c.Cwd != nil }
	if got := LoadCursor(root); !empty(got) {
		t.Fatalf("missing cursor = %+v, want empty", got)
	}
	c := Cursor{
		Logs: map[string]cursorEntry{"/h/a.jsonl": {Size: 42, ModTime: time.Date(2026, 10, 20, 1, 2, 3, 456789, time.UTC), File: true}},
		Cwd:  map[string]string{"/h/rollout-x.jsonl": "/p/app"},
	}
	if err := c.Save(root); err != nil {
		t.Fatal(err)
	}
	got := LoadCursor(root)
	e := got.Logs["/h/a.jsonl"]
	if len(got.Logs) != 1 || e.Size != 42 || !e.ModTime.Equal(c.Logs["/h/a.jsonl"].ModTime) || !e.File {
		t.Errorf("round trip = %+v", got)
	}
	if !reflect.DeepEqual(got.Cwd, c.Cwd) {
		t.Errorf("cwd round trip = %v", got.Cwd)
	}

	mustWrite(t, filepath.Join(root, ".tiller", ".cache", "usage.yml"), "{not yaml")
	if got := LoadCursor(root); !empty(got) {
		t.Errorf("corrupt cursor = %+v, want empty", got)
	}
	mustWrite(t, filepath.Join(root, ".tiller", ".cache", "usage.yml"), "reader: 0\nlogs:\n  /h/a.jsonl: {size: 1}\n")
	if got := LoadCursor(root); !empty(got) {
		t.Errorf("cursor from another reader version = %+v, want empty", got)
	}
}
