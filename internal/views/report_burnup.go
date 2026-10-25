package views

import (
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// burnUp measures approved feature scope and completed features at the end of
// each week (D6). A week ends strictly before the next Monday, since an event
// at exactly that instant belongs to the next week; the last week ends at now.
// A feature is in scope when it exists, is not cancelled, and its epic is
// approved (ready, done, or archived). It is done when in scope and done.
func burnUp(t *store.Tree, r *replay, weeks []time.Time, now time.Time) []ReportBurnUpWeek {
	epicOf := make(map[string]string, len(t.Epics))
	for id, e := range t.Epics {
		if e.Key != nil {
			epicOf[*e.Key] = id
		}
	}
	out := make([]ReportBurnUpWeek, len(weeks))
	for i := range weeks {
		end := weekEnd(weeks, i, now)
		for fid, f := range t.Features {
			status := r.statusAt(fid, end)
			if status == "" || status == "cancelled" {
				continue
			}
			epicID, ok := epicOf[f.EpicKey]
			if !ok {
				continue
			}
			switch r.statusAt(epicID, end) {
			case "ready", "done", "archived":
			default:
				continue
			}
			out[i].Scope++
			if status == "done" {
				out[i].Done++
			}
		}
	}
	return out
}
