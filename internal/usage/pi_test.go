package usage

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPiDiscoverBothEncodings(t *testing.T) {
	home := t.TempDir()
	omp := filepath.Join(home, ".omp", "agent", "sessions")
	pi := filepath.Join(home, ".pi", "agent", "sessions")
	app := filepath.Join(home, "dev", "app")
	line := `{"type":"message"}` + "\n"
	mustWrite(t, filepath.Join(omp, "-dev-app", "a.jsonl"), line)
	mustWrite(t, filepath.Join(pi, "--"+piInner(app)+"--", "b.jsonl"), line)
	mustWrite(t, filepath.Join(omp, "-dev-application", "c.jsonl"), line)
	mustWrite(t, filepath.Join(omp, "-dev-app-two", "d.jsonl"), line)
	mustWrite(t, filepath.Join(omp, "-dev-app--worktrees-gone", "e.jsonl"), line)
	mustWrite(t, filepath.Join(pi, "--"+piInner(app)+"--worktrees-gone--", "f.jsonl"), line)
	mustWrite(t, filepath.Join(pi, "--"+piInner(filepath.Join(home, "dev", "app-two"))+"--", "g.jsonl"), line)

	refs, err := NewPiReader().Discover(Roots{Home: home, Project: []string{app}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range refs {
		if r.Harness != "pi" {
			t.Errorf("harness = %q", r.Harness)
		}
		got[r.Session] = true
	}
	for _, want := range []string{"a", "b", "e", "f"} {
		if !got[want] {
			t.Errorf("session %s not discovered: %+v", want, refs)
		}
	}
	for _, not := range []string{"c", "d", "g"} {
		if got[not] {
			t.Errorf("session %s discovered", not)
		}
	}
}

func TestPiDiscoverMissingRoots(t *testing.T) {
	refs, err := NewPiReader().Discover(Roots{Home: t.TempDir(), Project: []string{"/x/app"}})
	if err != nil || len(refs) != 0 {
		t.Fatalf("refs=%v err=%v", refs, err)
	}
}

func TestPiParseMapsTokensAndMarkers(t *testing.T) {
	path := filepath.Join("testdata", "pi", "session.jsonl")
	s, err := NewPiReader().Parse([]LogRef{{Harness: "pi", Path: path, Session: "session"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Harness != "pi" || s.ID != "session" {
		t.Fatalf("session = %+v", s)
	}
	if len(s.Calls) != 1 {
		t.Fatalf("calls = %+v, want 1 (no usage means no call)", s.Calls)
	}
	c := s.Calls[0]
	if c.Model != "gpt-5.6-sol" {
		t.Errorf("model = %q", c.Model)
	}
	if want := time.Date(2026, 7, 30, 20, 22, 56, 125000000, time.UTC); !c.At.Equal(want) {
		t.Errorf("at = %v", c.At)
	}
	if c.Tokens != (Tokens{Input: 20737, CacheWrite: 7, CacheRead: 11, Output: 194, Reasoning: 165}) {
		t.Errorf("tokens = %+v", c.Tokens)
	}
	want := []Marker{
		{Kind: MarkVerb, Stage: Implementation, Ref: "T-abc123", Verb: "start"},
		{Kind: MarkSkill, Stage: Implementation, Verb: "execute"},
	}
	if !reflect.DeepEqual(c.Markers, want) {
		t.Errorf("markers = %+v, want %+v", c.Markers, want)
	}
}

func TestPiSubagentFolderIsParentStem(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, ".omp", "agent", "sessions", "-dev-app")
	line := `{"type":"message"}` + "\n"
	mustWrite(t, filepath.Join(folder, "2026_parent.jsonl"), line)
	mustWrite(t, filepath.Join(folder, "2026_parent", "0-Explore.jsonl"), line)
	mustWrite(t, filepath.Join(folder, "2026_parent", "0-Explore.md"), "x")
	mustWrite(t, filepath.Join(folder, "2026_parent", "meta.json"), "{}")
	mustWrite(t, filepath.Join(folder, "2026_parent", "run.log"), "x")

	refs, err := NewPiReader().Discover(Roots{Home: home, Project: []string{"/dev/app"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs = %+v, want 2", refs)
	}
	var sub LogRef
	for _, r := range refs {
		if r.Parent != "" {
			sub = r
		}
	}
	if sub.Parent != "2026_parent" || sub.Session != "0-Explore" {
		t.Errorf("sub = %+v", sub)
	}
}
