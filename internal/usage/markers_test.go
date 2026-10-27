package usage

import (
	"reflect"
	"testing"
)

func TestShellMarkersVerbs(t *testing.T) {
	cases := []struct {
		cmd  string
		want []Marker
	}{
		{"tiller start task T-abc123", []Marker{{MarkVerb, Implementation, "T-abc123", "start"}}},
		{"cd x && tiller check T-abc123 --json", []Marker{{MarkVerb, Checks, "T-abc123", "check"}}},
		{"tiller submit task T-abc123 --pr 4", []Marker{{MarkVerb, Publishing, "T-abc123", "submit"}}},
		{"tiller sync", []Marker{{MarkVerb, Closeout, "", "sync"}}},
		{"tiller sync --json", []Marker{{MarkVerb, Closeout, "", "sync"}}},
		{"tiller show T-abc123", []Marker{{MarkRead, "", "T-abc123", "show"}}},
		{"tiller guide specify", []Marker{{MarkSkill, SpecWriting, "", "guide"}}},
		{"tiller guide", []Marker{{MarkRead, "", "", "guide"}}},
		{"tiller attest F-abc123", []Marker{{MarkVerb, AcceptanceEvals, "F-abc123", "attest"}}},
		{"tiller approve e/f/t", []Marker{{MarkVerb, Other, "e/f/t", "approve"}}},
		{"tiller rename --json E-abc123 x", []Marker{{MarkVerb, Planning, "E-abc123", "rename"}}},
		{"(tiller spec F-abc123)", []Marker{{MarkVerb, SpecWriting, "F-abc123", "spec"}}},
		{`bash -c "tiller cancel T-abc123"`, []Marker{{MarkVerb, Closeout, "T-abc123", "cancel"}}},
		{"tiller sync; tiller start T-abc123", []Marker{
			{MarkVerb, Closeout, "", "sync"},
			{MarkVerb, Implementation, "T-abc123", "start"},
		}},
		{"cat .tiller/tiller.yml", nil},
		{"go run ./cmd/tiller status", nil},
		{"tiller frobnicate T-abc123", nil},
	}
	for _, c := range cases {
		got := ShellMarkers(c.cmd)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %+v, want %+v", c.cmd, got, c.want)
		}
	}
}

func TestSkillMarker(t *testing.T) {
	cases := []struct {
		skill string
		stage Stage
		ok    bool
	}{
		{"tiller-code-review", CodeReview, true},
		{"code-review", CodeReview, true},
		{"tiller:tiller-execute", Implementation, true},
		{"specify", SpecWriting, true},
		{"tiller-decompose", Planning, true},
		{"tiller-orient", Other, true},
		{"brainstorming", "", false},
		{"superpowers:brainstorming", "", false},
	}
	for _, c := range cases {
		m, ok := SkillMarker(c.skill, "")
		if ok != c.ok || m.Stage != c.stage || (ok && m.Kind != MarkSkill) {
			t.Errorf("%q: got %+v ok=%v", c.skill, m, ok)
		}
	}
	m, _ := SkillMarker("tiller-execute", "T-abc123")
	if m.Ref != "T-abc123" {
		t.Errorf("ref = %q", m.Ref)
	}
}
