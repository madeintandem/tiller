package usage

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Call is one model call in a session.
type Call struct {
	At      time.Time
	Model   string
	Tokens  Tokens
	Markers []Marker
	Branch  string
}

// Session is one harness session, with the sessions of its subagents.
type Session struct {
	Harness, ID string
	Calls       []Call
	Subagents   []Session
}

// Resolver maps a node id or key to a node id, or "" when the ref names
// nothing in the tree.
type Resolver func(ref string) string

var idShaped = regexp.MustCompile(`^(E|F|T|B|CH|TW)-[a-z0-9]{6}$`)

type state struct {
	node  string
	stage Stage
}

type bucket struct {
	hour  time.Time
	node  string
	stage Stage
	model string
}

// Credit replays the session and its subagents in time order and returns the
// model calls aggregated per UTC hour, node, stage and model, sorted by that
// key. Of a marker's candidate refs, the first the resolver resolves wins;
// failing that, the first shaped like a node id is kept as written, so an id
// from a since-changed ledger is not dropped; failing that, the marker names
// no node.
func Credit(s Session, resolve Resolver) []Row {
	res := func(refs ...string) string {
		for _, ref := range refs {
			if ref == "" {
				continue
			}
			if id := resolve(ref); id != "" {
				return id
			}
		}
		for _, ref := range refs {
			if idShaped.MatchString(ref) {
				return ref
			}
		}
		return ""
	}
	acc := map[bucket]*Row{}
	replay(s, state{}, res, acc)

	rows := make([]Row, 0, len(acc))
	for _, r := range acc {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if !a.Hour.Equal(b.Hour) {
			return a.Hour.Before(b.Hour)
		}
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		if a.Stage != b.Stage {
			return a.Stage < b.Stage
		}
		return a.Model < b.Model
	})
	return rows
}

// replay walks s's calls from the starting state, crediting each into acc,
// and replays each subagent from the state s held at the subagent's first call.
func replay(s Session, start state, resolve refResolver, acc map[bucket]*Row) {
	calls := append([]Call(nil), s.Calls...)
	sort.SliceStable(calls, func(i, j int) bool { return calls[i].At.Before(calls[j].At) })

	st := start
	for _, c := range calls {
		st = apply(st, c.Markers, resolve)
		node := st.node
		if node == "" && strings.HasPrefix(c.Branch, "tiller/") {
			node = resolve(strings.TrimPrefix(c.Branch, "tiller/"))
		}
		stage := st.stage
		if stage == "" {
			stage = Other
		}
		k := bucket{c.At.UTC().Truncate(time.Hour), node, stage, c.Model}
		r := acc[k]
		if r == nil {
			r = &Row{Hour: k.hour, Node: node, Stage: stage, Model: c.Model}
			acc[k] = r
		}
		r.Calls++
		r.Tokens.Add(c.Tokens)
	}

	for _, sub := range s.Subagents {
		first, ok := firstCall(sub)
		if !ok {
			replay(sub, start, resolve, acc)
			continue
		}
		parent := start
		for _, c := range calls {
			if !c.At.Before(first) {
				break
			}
			parent = apply(parent, c.Markers, resolve)
		}
		replay(sub, parent, resolve, acc)
	}
}

// refResolver picks the node a marker's candidate refs name, or "".
type refResolver func(refs ...string) string

func apply(st state, markers []Marker, resolve refResolver) state {
	for _, m := range markers {
		switch m.Kind {
		case MarkVerb:
			st.stage = m.Stage
			st.node = resolve(m.Refs...)
		case MarkSkill:
			st.stage = m.Stage
			if len(m.Refs) > 0 {
				st.node = resolve(m.Refs...)
			}
		case MarkRead:
			if st.node == "" && len(m.Refs) > 0 {
				st.node = resolve(m.Refs...)
			}
			if st.stage == "" {
				st.stage = Other
			}
		}
	}
	return st
}

func firstCall(s Session) (time.Time, bool) {
	var first time.Time
	found := false
	for _, c := range s.Calls {
		if !found || c.At.Before(first) {
			first, found = c.At, true
		}
	}
	return first, found
}
