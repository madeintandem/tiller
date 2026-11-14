package views

import (
	"sort"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
)

// eventMoveFeature is the event an approved amendment's move-feature writes.
// It names the feature first among its targets and carries the stable ids of
// the epic it left and the epic it joined as before.epic and after.epic.
const eventMoveFeature = "move.feature"

// parentChange is one point on a feature's parent timeline: from At on, the
// feature sits under the epic with this id. The first point of a timeline has
// a zero At, since it holds from before the axis.
type parentChange struct {
	At   time.Time
	Epic string
}

// recordMove notes a reviewed move of a tracked feature. Only move.feature
// crosses an approved epic: a direct key rename reparents a draft feature
// between two draft epics, and an epic never returns to draft, so neither
// parent was scope in any week at or before such a rename and the timeline
// needs no point for it.
func (r *replay) recordMove(ev events.Event, at time.Time, tracked map[string]string) {
	if ev.Type != eventMoveFeature {
		return
	}
	from, hasFrom := ev.Before["epic"].(string)
	to, hasTo := ev.After["epic"].(string)
	if !hasFrom || !hasTo {
		return
	}
	for _, target := range ev.Targets {
		if _, ok := tracked[target]; !ok || idPrefix(target) != "F" {
			continue
		}
		line := r.parents[target]
		if len(line) == 0 {
			line = []parentChange{{Epic: from}}
		}
		r.parents[target] = append(line, parentChange{At: at, Epic: to})
	}
}

// reconcileParents closes every feature's parent timeline against the tree,
// the way the status timelines are closed: a feature with no recorded move
// sat under its current epic throughout, and one whose last recorded move
// ends somewhere other than its current epic is moved there at now, so the
// last week always reads the tree's parentage. The current epic is the only
// place a key is resolved; every earlier point is an id a move recorded.
func (r *replay) reconcileParents(t *store.Tree, now time.Time) {
	epicOf := make(map[string]string, len(t.Epics))
	for id, e := range t.Epics {
		if e.Key != nil {
			epicOf[*e.Key] = id
		}
	}
	for fid, f := range t.Features {
		current, ok := epicOf[f.EpicKey]
		if !ok {
			continue
		}
		line := r.parents[fid]
		switch {
		case len(line) == 0:
			r.parents[fid] = []parentChange{{Epic: current}}
		case line[len(line)-1].Epic != current:
			r.parents[fid] = append(line, parentChange{At: now, Epic: current})
		}
	}
}

// parentAt is the id of the epic the feature sat under at an instant: the
// last parent change at or before it, or "" when the feature has no known
// parent.
func (r *replay) parentAt(featureID string, at time.Time) string {
	line := r.parents[featureID]
	i := sort.Search(len(line), func(i int) bool { return line[i].At.After(at) })
	if i == 0 {
		return ""
	}
	return line[i-1].Epic
}
