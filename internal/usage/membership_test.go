package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// A harness names a session folder by turning every separator and dot of the
// working directory into a dash, so /p/app.foo, /p/app-foo and /p/app/foo
// share one folder. These tests pin that the folder only nominates sessions:
// each top-level log's own recorded working directory decides, and a subagent
// log belongs to the session it sits under.

// sessionIDs is the discovered sessions as "id" for a top-level log and
// "parent/id" for a subagent log, sorted.
func sessionIDs(refs []LogRef) []string {
	ids := []string{}
	for _, r := range refs {
		id := r.Session
		if r.Parent != "" {
			id = r.Parent + "/" + id
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func TestClaudeDiscoverJudgesEachSessionInASharedFolder(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".claude", "projects", "-p-app-foo")
	for id, cwd := range map[string]string{"dot": "/p/app.foo", "dash": "/p/app-foo", "sub": "/p/app/foo"} {
		mustWrite(t, filepath.Join(folder, id+".jsonl"), cwdLog(cwd))
		mustWrite(t, filepath.Join(folder, id, "subagents", "agent-a.jsonl"), cwdLog(cwd))
	}
	// No working directory recorded: nothing says whose session this is.
	mustWrite(t, filepath.Join(folder, "nocwd.jsonl"), `{"type":"assistant"}`+"\n")
	mustWrite(t, filepath.Join(folder, "nocwd", "subagents", "agent-a.jsonl"), `{"type":"assistant"}`+"\n")

	for project, want := range map[string][]string{
		"/p/app.foo": {"dot", "dot/agent-a"},
		"/p/app-foo": {"dash", "dash/agent-a"},
		"/p/app/foo": {"sub", "sub/agent-a"},
		"/p/app":     {"sub", "sub/agent-a"}, // a session started beneath the checkout
	} {
		refs, bad := NewClaudeReader().Discover(Roots{Home: home, Project: []string{project}})
		if bad != 0 {
			t.Errorf("%s: unreadable = %d", project, bad)
		}
		if got := sessionIDs(refs); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: sessions = %v, want %v", project, got, want)
		}
	}
}

// The root's encoding plus "--" is how a worktree under one of the root's dot
// directories encodes, and also how a sibling whose name continues with a
// dash or dot does. A removed worktree's sessions are still found, by their
// recorded working directory; the sibling's are not.
func TestClaudeDiscoverJudgesEachSessionInARemovedWorktreeFolder(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".claude", "projects", "-p-app--worktrees-gone")
	mustWrite(t, filepath.Join(folder, "ours.jsonl"), cwdLog("/p/app/.worktrees/gone"))
	mustWrite(t, filepath.Join(folder, "theirs.jsonl"), cwdLog("/p/app-.worktrees/gone"))
	mustWrite(t, filepath.Join(folder, "nocwd.jsonl"), `{"type":"assistant"}`+"\n")

	refs, bad := NewClaudeReader().Discover(Roots{Home: home, Project: []string{"/p/app"}})
	if bad != 0 {
		t.Fatalf("unreadable = %d", bad)
	}
	if got := sessionIDs(refs); !reflect.DeepEqual(got, []string{"ours"}) {
		t.Fatalf("sessions = %v, want [ours]", got)
	}
}

func piHeader(cwd string) string {
	return `{"type":"session","version":3,"id":"x","timestamp":"2026-07-30T20:22:45.196Z","cwd":"` + cwd + `"}` + "\n"
}

func TestPiDiscoverJudgesEachSessionInASharedFolder(t *testing.T) {
	home := t.TempDir()
	dev := filepath.Join(home, "dev")
	cwds := map[string]string{
		"dot": filepath.Join(dev, "app.foo"), "dash": filepath.Join(dev, "app-foo"), "sub": filepath.Join(dev, "app", "foo"),
	}
	// OhMyPi names the folder for the home-relative path, Pi for the full one.
	for _, folder := range []string{
		filepath.Join(home, ".omp", "agent", "sessions", "-dev-app-foo"),
		filepath.Join(home, ".pi", "agent", "sessions", "--"+piInner(cwds["dot"])+"--"),
	} {
		tag := "omp-"
		if strings.Contains(folder, ".pi") {
			tag = "pi-"
		}
		for id, cwd := range cwds {
			mustWrite(t, filepath.Join(folder, tag+id+".jsonl"), piHeader(cwd))
			mustWrite(t, filepath.Join(folder, tag+id, "0-Explore.jsonl"), piHeader(cwd))
		}
		mustWrite(t, filepath.Join(folder, tag+"nocwd.jsonl"), `{"type":"message"}`+"\n")
		mustWrite(t, filepath.Join(folder, tag+"nocwd", "0-Explore.jsonl"), `{"type":"message"}`+"\n")
	}

	for project, id := range map[string]string{
		cwds["dot"]: "dot", cwds["dash"]: "dash", cwds["sub"]: "sub", filepath.Join(dev, "app"): "sub",
	} {
		want := []string{"omp-" + id, "omp-" + id + "/0-Explore", "pi-" + id, "pi-" + id + "/0-Explore"}
		refs, bad := NewPiReader().Discover(Roots{Home: home, Project: []string{project}})
		if bad != 0 {
			t.Errorf("%s: unreadable = %d", project, bad)
		}
		if got := sessionIDs(refs); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: sessions = %v, want %v", project, got, want)
		}
	}
}

// piCall is one Pi assistant line with usage.
func piCall(at time.Time) string {
	return fmt.Sprintf(`{"type":"message","timestamp":%q,"message":{"role":"assistant","content":[],"model":"gpt-5.6-sol","usage":{"input":7,"output":3}}}`+"\n",
		at.Format(time.RFC3339Nano))
}

// TestCaptureWritesOnlyThisProjectsSessionsFromASharedFolder is the capture
// a project sees: with /…/app.foo's capture on, the sessions another project
// ran in /…/app-foo and /…/app/foo, and their subagents, produce no usage
// file and no tokens, for Claude Code and for Pi. Every session is settled
// and later than the project's first event, so only membership decides.
func TestCaptureWritesOnlyThisProjectsSessionsFromASharedFolder(t *testing.T) {
	base := t.TempDir()
	e := captureEnv{home: t.TempDir(), root: filepath.Join(base, "app.foo")}
	if err := os.MkdirAll(e.root, 0o755); err != nil {
		t.Fatal(err)
	}
	cwds := map[string]string{"ours": e.root, "dash": filepath.Join(base, "app-foo"), "sub": filepath.Join(base, "app", "foo")}
	at := captureNow.Add(-3 * time.Hour)

	claude := filepath.Join(e.home, ".claude", "projects", claudeFolder(e.root))
	pi := filepath.Join(e.home, ".pi", "agent", "sessions", "--"+piInner(e.root)+"--")
	for id, cwd := range cwds {
		for path, body := range map[string]string{
			filepath.Join(claude, id+".jsonl"):                         claudeCwd(cwd) + claudeCall(at, id+"-top", ""),
			filepath.Join(claude, id, "subagents", "agent-a.jsonl"):    claudeCall(at, id+"-agent", ""),
			filepath.Join(pi, "pi-"+id+".jsonl"):                       piHeader(cwd) + piCall(at),
			filepath.Join(pi, "pi-"+id, "0-Explore.jsonl"):             piHeader(cwd) + piCall(at),
			filepath.Join(claude, "nocwd-"+id+".jsonl"):                claudeCall(at, id+"-nocwd", ""),
			filepath.Join(pi, "pi-nocwd-"+id+".jsonl"):                 piCall(at),
			filepath.Join(pi, "pi-nocwd-"+id, "0-Explore.jsonl"):       piCall(at),
			filepath.Join(claude, "nocwd-"+id, "subagents", "a.jsonl"): claudeCall(at, id+"-nocwd-agent", ""),
		} {
			mustWrite(t, path, body)
			touch(t, path, at)
		}
	}

	o := e.options(NewClaudeReader(), NewPiReader())
	o.Since = func() (time.Time, error) { return captureNow.Add(-24 * time.Hour), nil }
	res, _ := mustCapture(t, o)

	var written []string
	for _, w := range res.Writes {
		rel, err := filepath.Rel(filepath.Join(e.root, ".tiller", "usage"), w.Path)
		if err != nil {
			t.Fatal(err)
		}
		written = append(written, filepath.ToSlash(rel))
	}
	sort.Strings(written)
	if want := []string{"claude-code/ours.yml", "pi/pi-ours.yml"}; !reflect.DeepEqual(written, want) {
		t.Errorf("usage files = %v, want %v", written, want)
	}
	// One Claude call is 135 tokens and one Pi call 10; a session is its
	// top-level call plus its subagent's.
	for harness, tokens := range map[string]int64{"claude-code": 270, "pi": 20} {
		h := harnessResult(t, res, harness)
		if h.Sessions != 1 || h.Written != 1 || h.Tokens != tokens || h.Unreadable != 0 {
			t.Errorf("%s = %+v, want 1 session, 1 written, %d tokens", harness, h, tokens)
		}
	}
}
