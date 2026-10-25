package views

import (
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
)

// RecentWindow is how far back a deliberate act still places the reader's
// work (ranked-next design, decision 12). Callers read events this far back.
const RecentWindow = 14 * 24 * time.Hour

// maxLocalizedTargets caps the targets an event may name and still say where
// the reader was working. A wider event is a sweep, not a place.
const maxLocalizedTargets = 3

// countedVerbs are the event verbs (the segment before the first dot) that
// record a deliberate act by the reader. sync, migrate, contributor, join and
// upgrade are bookkeeping and never count.
var countedVerbs = map[string]bool{
	"claim": true, "start": true, "submit": true, "check": true, "spec": true,
	"approve": true, "answer": true, "attest": true, "add": true, "link": true,
	"reassign": true, "release": true,
}

// Locality is where the reader has recently been working, read from the event
// log. The zero value means unknown, which ranks nothing as nearby. An epic's
// recency is the newest At among the touches carrying its EpicKey.
type Locality struct {
	CurrentFeature string  // feature id, "" when none
	CurrentEpic    string  // epic key, "" when none
	Touched        []Touch // most recent first, CurrentFeature included
}

// Touch is one feature or epic the reader acted on. An epic-only touch leaves
// FeatureID empty.
type Touch struct {
	FeatureID string
	EpicKey   string
	At        time.Time
}

// LocateOf derives the reader's Locality from events inside the recent
// window. It is pure: now is a parameter and the events arrive already read.
func LocateOf(t *store.Tree, evs []events.Event, reader string, now time.Time) Locality {
	if reader == "" || t == nil {
		return Locality{}
	}
	cutoff := now.Add(-RecentWindow)
	recent := make([]events.Event, 0, len(evs))
	for _, ev := range evs {
		if countedEvent(ev, reader) && !ev.At.Before(cutoff) {
			recent = append(recent, ev)
		}
	}
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].At.After(recent[j].At) })

	var loc Locality
	seenFeature := map[string]bool{}
	seenEpic := map[string]bool{}
	var newestEpicOnly string
	for _, ev := range recent {
		// An add says where work was laid down, not where the reader is
		// working: it touches the added node's parent for recency and never
		// sets the current feature or epic, so a decompose burst cannot.
		adding := strings.HasPrefix(ev.Type, "add.")
		for _, raw := range ev.Targets {
			touch, ok := touchOf(t, raw, ev.At)
			if adding {
				touch, ok = addTouchOf(t, ev, raw)
			}
			if !ok {
				continue
			}
			if touch.FeatureID != "" {
				if !adding && loc.CurrentFeature == "" {
					loc.CurrentFeature, loc.CurrentEpic = touch.FeatureID, touch.EpicKey
				}
				if seenFeature[touch.FeatureID] {
					continue
				}
				seenFeature[touch.FeatureID] = true
			} else {
				if !adding && newestEpicOnly == "" {
					newestEpicOnly = touch.EpicKey
				}
				if seenEpic[touch.EpicKey] {
					continue
				}
				seenEpic[touch.EpicKey] = true
			}
			loc.Touched = append(loc.Touched, touch)
		}
	}
	if loc.CurrentFeature == "" {
		loc.CurrentEpic = newestEpicOnly
	}
	return loc
}

// addTouchOf is where an add event places the reader: the added node's
// parent. An added task touches its feature (the tree's, else the event's
// recorded parent for a task since removed), an added feature its epic, and
// an added question the node it blocks. An added epic has no parent and
// touches nothing.
func addTouchOf(t *store.Tree, ev events.Event, raw string) (Touch, bool) {
	if q := t.Questions[raw]; q != nil {
		return touchOf(t, raw, ev.At)
	}
	switch n := resolveWorkNode(t, raw).(type) {
	case *store.Task:
		return featureTouch(t, n.FeatureID, ev.At)
	case *store.Feature:
		return Touch{EpicKey: n.EpicKey, At: ev.At}, n.EpicKey != ""
	case nil:
		if feature, ok := ev.After["feature"].(string); ok && ev.Type == "add.task" {
			return featureTouch(t, feature, ev.At)
		}
	}
	return Touch{}, false
}

// featureTouch is a touch on the feature with id, false when it is gone.
func featureTouch(t *store.Tree, id string, at time.Time) (Touch, bool) {
	f := t.Features[id]
	if f == nil {
		return Touch{}, false
	}
	return Touch{FeatureID: f.ID, EpicKey: f.EpicKey, At: at}, true
}

// countedEvent reports whether ev is a deliberate act of the reader that can
// place their work: a counted verb, the reader's own, naming few enough
// targets to mean somewhere. Amendment events are deliberate acts but name an
// amendment id, which maps to no feature, so they simply never resolve.
func countedEvent(ev events.Event, reader string) bool {
	if reader == "" || ev.Contributor != reader || len(ev.Targets) > maxLocalizedTargets {
		return false
	}
	switch ev.Type {
	case "amendment_opened", "amendment_staged":
		return true
	}
	verb, _, dotted := strings.Cut(ev.Type, ".")
	return dotted && countedVerbs[verb]
}

// touchOf maps one event target to the feature or epic it sits in. Questions
// map through the first node they block that still resolves; anything else
// that is not a task, feature or epic (designs, amendments, removed nodes)
// maps to nothing.
func touchOf(t *store.Tree, raw string, at time.Time) (Touch, bool) {
	if q := t.Questions[raw]; q != nil {
		for _, blocked := range q.Blocks {
			if node := resolveWorkNode(t, blocked); node != nil {
				return touchOfNode(t, node, at)
			}
		}
		return Touch{}, false
	}
	node := resolveWorkNode(t, raw)
	if node == nil {
		return Touch{}, false
	}
	return touchOfNode(t, node, at)
}

func touchOfNode(t *store.Tree, node store.Node, at time.Time) (Touch, bool) {
	switch n := node.(type) {
	case *store.Task:
		f := t.Features[n.FeatureID]
		if f == nil {
			return Touch{}, false
		}
		return Touch{FeatureID: f.ID, EpicKey: f.EpicKey, At: at}, true
	case *store.Feature:
		return Touch{FeatureID: n.ID, EpicKey: n.EpicKey, At: at}, true
	case *store.Epic:
		if n.Key == nil {
			return Touch{}, false
		}
		return Touch{EpicKey: *n.Key, At: at}, true
	}
	return Touch{}, false
}
