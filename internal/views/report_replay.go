package views

import (
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
)

// statusChange is one point on a node's status timeline.
type statusChange struct {
	At     time.Time
	Status string
	Type   string // the event type, e.g. "start.task"; empty for a reconciled change
}

// replay holds every current-tree node's status timeline, oldest first,
// already reconciled (D18): a node with no recorded change starts in its
// current status at the start of the week holding the earliest event (the
// axis start when there are no events), and a node whose last recorded change
// disagrees with the tree gets a closing change to the current status at now,
// so a replayed final state always matches the tree. A reconciled change
// carries an empty Type, which is how the metric builders tell it from a real
// event.
type replay struct {
	timelines map[string][]statusChange
}

// newReplay walks the event stream once, oldest first, and records each
// status an event set on a node that still exists in the tree (statusKey says
// which). An event with a zero timestamp is malformed and ignored. Every other
// change is clamped onto the axis: one stamped after now is recorded at now,
// so a clock running slightly ahead of the reader still counts the change, and
// one stamped before the first week is recorded at its start, so every
// timeline stays in order. An event that only restates a node's status is
// not a change (restated says which). events.List already returns the stream
// sorted, so it is copied and sorted only when it is not.
func newReplay(t *store.Tree, evs []events.Event, weeks []time.Time, now time.Time) *replay {
	current := currentStatuses(t)
	r := &replay{timelines: make(map[string][]statusChange, len(current))}
	if !isSorted(evs) {
		evs = append([]events.Event(nil), evs...)
		events.Sort(evs)
	}
	unexplainedAt, seen := weeks[0], false
	for _, ev := range evs {
		if ev.At.IsZero() {
			continue
		}
		at := ev.At
		if at.After(now) {
			at = now
		}
		if at.Before(weeks[0]) {
			at = weeks[0]
		}
		if !seen {
			unexplainedAt, seen = weeks[weekIndex(weeks, at)], true
		}
		verb := ev.Type
		noun := verb[strings.LastIndex(verb, ".")+1:]
		prefix, err := id.PrefixFor(noun)
		if err != nil {
			continue
		}
		for _, target := range ev.Targets {
			if _, tracked := current[target]; !tracked || idPrefix(target) != prefix {
				continue
			}
			key, ok := statusKey(ev, target)
			if !ok {
				continue
			}
			status, ok := statusIn(ev.After, key, target)
			if !ok {
				continue
			}
			line := r.timelines[target]
			if len(line) > 0 && restated(ev, key, target, status, line[len(line)-1].Status) {
				continue
			}
			r.timelines[target] = append(line, statusChange{At: at, Status: status, Type: ev.Type})
		}
	}
	for nodeID, cur := range current {
		line := r.timelines[nodeID]
		switch {
		case len(line) == 0:
			r.timelines[nodeID] = []statusChange{{At: unexplainedAt, Status: cur}}
		case line[len(line)-1].Status != cur:
			r.timelines[nodeID] = append(line, statusChange{At: now, Status: cur})
		}
	}
	return r
}

// isSorted reports whether evs is already in events.Sort order: by timestamp,
// then by id.
func isSorted(evs []events.Event) bool {
	for i := 1; i < len(evs); i++ {
		if events.Less(evs[i], evs[i-1]) {
			return false
		}
	}
	return true
}

// restated reports whether an event only restates a target's status: it sets
// the status the target's previous recorded change already set, and its before,
// when it records one, names that status too. A feature eval correction, a
// revise, and an add.question re-emit all write the unchanged current status
// into after. A sync that finds a rollup reopened with no event (before
// active, after done) is a genuine transition and is recorded.
func restated(ev events.Event, key, target, status, previous string) bool {
	if status != previous {
		return false
	}
	before, ok := statusIn(ev.Before, key, target)
	return !ok || before == status
}

// statusKey is the key under which an event records the status it set on one
// of its targets, already known to carry the event type's noun prefix:
// multi-target events list amendment ids beside the node, and those targets
// are not the noun the event changed. An event whose after names its subject
// by id sets "status" on that id alone: transfer.<noun> lists the original and
// its replacement, and sets "replacement_status" on after.replacement_id. Any
// other event sets "status" on every target.
func statusKey(ev events.Event, target string) (string, bool) {
	subject, named := ev.After["id"].(string)
	if !named {
		return "status", true
	}
	switch replacement, _ := ev.After["replacement_id"].(string); target {
	case subject:
		return "status", true
	case replacement:
		return "replacement_status", true
	default:
		return "", false
	}
}

// statusIn reads a status from an event's before or after map. A legacy
// add.question re-emit can carry an empty status, which means open, as it
// does in the tree.
func statusIn(m map[string]any, key, target string) (string, bool) {
	status, ok := m[key].(string)
	if ok && status == "" && idPrefix(target) == "Q" {
		status = "open"
	}
	return status, ok
}

// currentStatuses maps every tracked node id to its status in the tree.
func currentStatuses(t *store.Tree) map[string]string {
	out := map[string]string{}
	for k, e := range t.Epics {
		out[k] = e.Status
	}
	for k, f := range t.Features {
		out[k] = f.Status
	}
	for k, task := range t.Tasks {
		out[k] = task.Status
	}
	for k, b := range t.Bugs {
		out[k] = b.Status
	}
	for k, c := range t.Chores {
		out[k] = c.Status
	}
	for k, tw := range t.Tweaks {
		out[k] = tw.Status
	}
	for k, q := range t.Questions {
		out[k] = q.Status
		if q.Status == "" {
			out[k] = "open"
		}
	}
	return out
}

// idPrefix is the part of an id before its first hyphen.
func idPrefix(nodeID string) string {
	prefix, _, _ := strings.Cut(nodeID, "-")
	return prefix
}

// statusAt is the node's status at an instant: the last change at or before
// it, or "" before the node exists.
func (r *replay) statusAt(nodeID string, at time.Time) string {
	line := r.timelines[nodeID]
	i := sort.Search(len(line), func(i int) bool { return line[i].At.After(at) })
	if i == 0 {
		return ""
	}
	return line[i-1].Status
}

// A changeFilter picks which changes a timeline scan considers.
type changeFilter func(statusChange) bool

// anyChange accepts every change, a reconciled one included.
func anyChange(statusChange) bool { return true }

// realChange accepts only a change an event recorded. A reconciled change has
// no type and is not evidence of when anything happened.
func realChange(c statusChange) bool { return c.Type != "" }

// changeFrom accepts only a change recorded by one event type.
func changeFrom(typ string) changeFilter {
	return func(c statusChange) bool { return c.Type == typ }
}

// firstAt is when the node first entered status through a change keep accepts.
func (r *replay) firstAt(nodeID, status string, keep changeFilter) (time.Time, bool) {
	for _, c := range r.timelines[nodeID] {
		if c.Status == status && keep(c) {
			return c.At, true
		}
	}
	return time.Time{}, false
}

// lastAt is when the node most recently entered status through a change keep
// accepts.
func (r *replay) lastAt(nodeID, status string, keep changeFilter) (time.Time, bool) {
	line := r.timelines[nodeID]
	for i := len(line) - 1; i >= 0; i-- {
		if line[i].Status == status && keep(line[i]) {
			return line[i].At, true
		}
	}
	return time.Time{}, false
}

// changes is the node's full timeline, oldest first.
func (r *replay) changes(nodeID string) []statusChange {
	return r.timelines[nodeID]
}
