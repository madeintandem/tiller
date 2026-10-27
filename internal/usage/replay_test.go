package usage

import (
	"testing"
	"time"
)

func at(h, m int) time.Time { return time.Date(2026, 10, 20, h, m, 0, 0, time.UTC) }

func verb(stage Stage, refs ...string) Marker {
	return Marker{Kind: MarkVerb, Stage: stage, Refs: refs}
}

func call(t time.Time, ms ...Marker) Call {
	return Call{At: t, Model: "m", Tokens: Tokens{Input: 1, Output: 2}, Markers: ms}
}

func idResolver(ref string) string {
	if len(ref) > 1 && ref[1] == '-' {
		return ref
	}
	return ""
}

func find(rows []Row, node string, stage Stage) (Row, bool) {
	for _, r := range rows {
		if r.Node == node && r.Stage == stage {
			return r, true
		}
	}
	return Row{}, false
}

func TestCreditFollowsMarkers(t *testing.T) {
	s := Session{Calls: []Call{
		call(at(10, 0), verb(Implementation, "T-a")),
		call(at(10, 1)),
		call(at(10, 2), verb(Publishing, "T-a")),
	}}
	rows := Credit(s, idResolver)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	impl, ok := find(rows, "T-a", Implementation)
	if !ok || impl.Calls != 2 || impl.Input != 2 || impl.Output != 4 {
		t.Errorf("implementation = %+v", impl)
	}
	if pub, ok := find(rows, "T-a", Publishing); !ok || pub.Calls != 1 {
		t.Errorf("publishing = %+v", pub)
	}
}

func TestCreditMarkerCallBelongsToNewState(t *testing.T) {
	s := Session{Calls: []Call{
		call(at(10, 0), verb(Implementation, "T-a")),
		call(at(10, 1), verb(Implementation, "T-b")),
	}}
	rows := Credit(s, idResolver)
	if r, ok := find(rows, "T-b", Implementation); !ok || r.Calls != 1 {
		t.Errorf("rows = %+v", rows)
	}
}

func TestCreditBareMutatingVerbClearsNode(t *testing.T) {
	s := Session{Calls: []Call{
		call(at(10, 0), verb(Implementation, "T-a")),
		call(at(10, 1), verb(Closeout)),
		call(at(10, 2)),
	}}
	rows := Credit(s, idResolver)
	if r, ok := find(rows, "", Closeout); !ok || r.Calls != 2 {
		t.Errorf("rows = %+v", rows)
	}
}

func TestCreditReadAndSkillMarkers(t *testing.T) {
	s := Session{Calls: []Call{
		call(at(10, 0), Marker{Kind: MarkRead, Refs: []string{"T-a"}}),
		call(at(10, 1), Marker{Kind: MarkSkill, Stage: CodeReview}),
		call(at(10, 2), Marker{Kind: MarkRead, Refs: []string{"T-b"}}),
	}}
	rows := Credit(s, idResolver)
	if r, ok := find(rows, "T-a", Other); !ok || r.Calls != 1 {
		t.Errorf("rows = %+v", rows)
	}
	if r, ok := find(rows, "T-a", CodeReview); !ok || r.Calls != 2 {
		t.Errorf("rows = %+v", rows)
	}
}

func TestCreditBranchFallback(t *testing.T) {
	resolve := func(ref string) string {
		if ref == "e/f/t" {
			return "T-key001"
		}
		return ""
	}
	s := Session{Calls: []Call{{At: at(10, 0), Model: "m", Branch: "tiller/e/f/t"}}}
	rows := Credit(s, resolve)
	if len(rows) != 1 || rows[0].Node != "T-key001" || rows[0].Stage != Other {
		t.Errorf("rows = %+v", rows)
	}
}

func TestCreditSubagentInheritsParentState(t *testing.T) {
	s := Session{
		Calls: []Call{
			call(at(10, 0), verb(Implementation, "T-a")),
			call(at(10, 30), verb(Publishing, "T-z")),
		},
		Subagents: []Session{{Calls: []Call{call(at(10, 5)), call(at(10, 6))}}},
	}
	rows := Credit(s, idResolver)
	if r, ok := find(rows, "T-a", Implementation); !ok || r.Calls != 3 {
		t.Errorf("rows = %+v", rows)
	}
}

func TestCreditUnresolvedIDKeptAsWritten(t *testing.T) {
	s := Session{Calls: []Call{
		call(at(10, 0), verb(Implementation, "T-zzzzzz")),
		call(at(10, 1), verb(Planning, "not-a-node")),
	}}
	rows := Credit(s, func(string) string { return "" })
	if _, ok := find(rows, "T-zzzzzz", Implementation); !ok {
		t.Errorf("rows = %+v", rows)
	}
	if _, ok := find(rows, "", Planning); !ok {
		t.Errorf("rows = %+v", rows)
	}
}

// TestCreditTakesFirstResolvingCandidate pins the ref choice among a
// marker's candidates: the first that resolves, else the first shaped like a
// node id, else none, so a flag value never wins over the node it precedes.
func TestCreditTakesFirstResolvingCandidate(t *testing.T) {
	resolve := func(ref string) string {
		if ref == "e/f/t" {
			return "T-key001"
		}
		return ""
	}
	cases := []struct {
		name string
		refs []string
		want string
	}{
		{"flag value before key", []string{"4", "e/f/t"}, "T-key001"},
		{"flag value before unresolved id", []string{"jc", "T-zzzzzz"}, "T-zzzzzz"},
		{"resolving key beats earlier id", []string{"T-zzzzzz", "e/f/t"}, "T-key001"},
		{"nothing usable clears", []string{"jc"}, ""},
	}
	for _, c := range cases {
		s := Session{Calls: []Call{
			call(at(10, 0), verb(Implementation, "T-a")),
			call(at(10, 1), verb(Checks, c.refs...)),
		}}
		rows := Credit(s, resolve)
		if r, ok := find(rows, c.want, Checks); !ok || r.Calls != 1 {
			t.Errorf("%s: rows = %+v", c.name, rows)
		}
	}
}

func TestCreditBucketsByUTCHour(t *testing.T) {
	s := Session{Calls: []Call{call(at(14, 5)), call(at(14, 55)), call(at(15, 1))}}
	rows := Credit(s, idResolver)
	if len(rows) != 2 || rows[0].Calls != 2 || rows[1].Calls != 1 || !rows[1].Hour.Equal(at(15, 0)) {
		t.Errorf("rows = %+v", rows)
	}
}
