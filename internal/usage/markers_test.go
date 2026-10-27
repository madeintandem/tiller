package usage

import (
	"reflect"
	"testing"
)

func mv(stage Stage, verb string, refs ...string) Marker {
	return Marker{Kind: MarkVerb, Stage: stage, Refs: refs, Verb: verb}
}

func TestShellMarkersVerbs(t *testing.T) {
	cases := []struct {
		cmd  string
		want []Marker
	}{
		{"tiller start task T-abc123", []Marker{mv(Implementation, "start", "T-abc123")}},
		{"cd x && tiller check T-abc123 --json", []Marker{mv(Checks, "check", "T-abc123")}},
		{"tiller submit task T-abc123 --pr 4", []Marker{mv(Publishing, "submit", "T-abc123", "4")}},
		{"tiller sync", []Marker{mv(Closeout, "sync")}},
		{"tiller sync --json", []Marker{mv(Closeout, "sync")}},
		{"tiller show T-abc123", []Marker{{Kind: MarkRead, Refs: []string{"T-abc123"}, Verb: "show"}}},
		{"tiller guide specify", []Marker{{Kind: MarkSkill, Stage: SpecWriting, Verb: "guide"}}},
		{"tiller guide", []Marker{{Kind: MarkRead, Verb: "guide"}}},
		{"tiller attest F-abc123", []Marker{mv(AcceptanceEvals, "attest", "F-abc123")}},
		{"tiller approve e/f/t", []Marker{mv(Other, "approve", "e/f/t")}},
		{"tiller rename --json E-abc123 x", []Marker{mv(Planning, "rename", "E-abc123", "x")}},
		{"(tiller spec F-abc123)", []Marker{mv(SpecWriting, "spec", "F-abc123")}},
		{`bash -c "tiller cancel T-abc123"`, []Marker{mv(Closeout, "cancel", "T-abc123")}},
		{"tiller sync; tiller start T-abc123", []Marker{
			mv(Closeout, "sync"),
			mv(Implementation, "start", "T-abc123"),
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

// TestShellMarkersRealShapes pins the command shapes found in real harness
// logs: commands wrapped in JS or JSON text, escaped newlines inside a quoted
// string, flags with values before the ref, and root flags before the verb.
func TestShellMarkersRealShapes(t *testing.T) {
	cases := []struct {
		cmd  string
		want []Marker
	}{
		{`exec_command({cmd: "tiller start task T-abc123"})`,
			[]Marker{mv(Implementation, "start", "T-abc123")}},
		{"exec_command({cmd: `tiller start T-abc123`})",
			[]Marker{mv(Implementation, "start", "T-abc123")}},
		{`await tools.exec_command({"cmd":"tiller check T-abc123 --json"})`,
			[]Marker{mv(Checks, "check", "T-abc123")}},
		{`exec_command({cmd: "tiller start T-abc123\ntiller check T-abc123"})`,
			[]Marker{mv(Implementation, "start", "T-abc123"), mv(Checks, "check", "T-abc123")}},
		{`exec_command({cmd: "tiller start T-abc123\tnow"})`,
			[]Marker{mv(Implementation, "start", "T-abc123", "now")}},
		{`exec_command({cmd: "tiller sync\"})`, []Marker{mv(Closeout, "sync")}},
		{`exec_command({cmd: 'tiller show \'T-abc123\''})`,
			[]Marker{{Kind: MarkRead, Refs: []string{"T-abc123"}, Verb: "show"}}},
		{`tiller submit --pr 4 T-abc123`, []Marker{mv(Publishing, "submit", "4", "T-abc123")}},
		{`tiller start --contributor jc T-abc123`, []Marker{mv(Implementation, "start", "jc", "T-abc123")}},
		{`tiller sync --contributor jc`, []Marker{mv(Closeout, "sync", "jc")}},
		{`tiller --json start T-abc123`, []Marker{mv(Implementation, "start", "T-abc123")}},
		{`tiller --json=true start task T-abc123`, []Marker{mv(Implementation, "start", "T-abc123")}},
		{`tiller --json`, nil},
		{`tiller start T-abc123,`, []Marker{mv(Implementation, "start", "T-abc123")}},
		{`tiller start 'T-abc123'`, []Marker{mv(Implementation, "start", "T-abc123")}},
		{`tiller start T-abc123\x`, []Marker{mv(Implementation, "start", "T-abc123")}},
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
	if !reflect.DeepEqual(m.Refs, []string{"T-abc123"}) {
		t.Errorf("refs = %q", m.Refs)
	}
}
