package views

import (
	"sort"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// The two events that date a feature's amendment hold. Staging an add-feature
// writes the held draft's add.feature with the amendment in after.amendment;
// approving the amendment writes amendment_approved with the amendment as its
// target, and that approval is the only thing that releases the hold.
const (
	eventAddFeature        = "add.feature"
	eventAmendmentApproved = "amendment_approved"
)

// holdChange is one point on a feature's hold timeline: from At on, an open
// amendment holds the feature, or no longer does. A hold the stream never
// dated has a zero At, since it covers every week.
type holdChange struct {
	At   time.Time
	Held bool
}

// stagedAdd is the staging that created a held feature.
type stagedAdd struct {
	At        time.Time
	Amendment string
}

// holdLedger collects, during the replay's one pass over the stream, which
// amendment staged each tracked feature and when each amendment was approved.
// A withdrawal is deliberately not collected: withdrawing removes the drafts
// an amendment held, and it never turns one into approved scope.
type holdLedger struct {
	staged   map[string]stagedAdd
	approved map[string]time.Time
}

func newHoldLedger() *holdLedger {
	return &holdLedger{staged: map[string]stagedAdd{}, approved: map[string]time.Time{}}
}

// record notes a staged add of a tracked feature or an amendment's approval.
// A node is added once, so the first add stands.
func (l *holdLedger) record(ev events.Event, at time.Time, tracked map[string]string) {
	switch ev.Type {
	case eventAmendmentApproved:
		for _, target := range ev.Targets {
			if _, seen := l.approved[target]; !seen && idPrefix(target) == "A" {
				l.approved[target] = at
			}
		}
	case eventAddFeature:
		amendment, _ := ev.After["amendment"].(string)
		if amendment == "" {
			return
		}
		for _, target := range ev.Targets {
			if _, ok := tracked[target]; !ok || idPrefix(target) != "F" {
				continue
			}
			if _, seen := l.staged[target]; !seen {
				l.staged[target] = stagedAdd{At: at, Amendment: amendment}
			}
		}
	}
}

// timelines turns the ledger into each feature's hold timeline, closed
// against the tree the way the status timelines are. A staged feature is held
// from its staging until its amendment's approval; an add an approval wrote
// in its own batch, as amendments materialized nodes before staging created
// held drafts, is released at once, whichever of the two events sorts first.
// A hold still open in the stream that the tree no longer carries ends at
// now, and a hold the tree carries that the stream never dated covers every
// week, so the last week always reads the tree's holds.
func (l *holdLedger) timelines(t *store.Tree, now time.Time) map[string][]holdChange {
	out := make(map[string][]holdChange, len(l.staged))
	for fid, f := range t.Features {
		var line []holdChange
		if add, ok := l.staged[fid]; ok {
			line = append(line, holdChange{At: add.At, Held: true})
			if released, ok := l.approved[add.Amendment]; ok {
				if released.Before(add.At) {
					released = add.At
				}
				line = append(line, holdChange{At: released, Held: false})
			}
		}
		held := gen.AmendmentHold(t, f) != nil
		switch {
		case len(line) == 0 && held:
			line = []holdChange{{Held: true}}
		case len(line) > 0 && line[len(line)-1].Held != held:
			line = append(line, holdChange{At: now, Held: held})
		}
		if len(line) > 0 {
			out[fid] = line
		}
	}
	return out
}

// heldAt reports whether an open amendment held the feature at an instant:
// the last hold change at or before it, and not held before any.
func (r *replay) heldAt(featureID string, at time.Time) bool {
	line := r.holds[featureID]
	i := sort.Search(len(line), func(i int) bool { return line[i].At.After(at) })
	return i > 0 && line[i-1].Held
}
