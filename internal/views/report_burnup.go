package views

import (
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// burnUp measures approved feature scope and completed features at the end of
// each week (D6). A week ends strictly before the next Monday, since an event
// at exactly that instant belongs to the next week; the last week ends at now.
// A feature is in scope when it exists, is not cancelled, and the epic it sat
// under at that week's end was approved then (ready, done, or archived). The
// parent is replayed with the status, so a later move between epics never
// rewrites an earlier week. It is done when in scope and done.
func burnUp(t *store.Tree, r *replay, weeks []time.Time, now time.Time) []ReportBurnUpWeek {
	out := make([]ReportBurnUpWeek, len(weeks))
	for i := range weeks {
		end := weekEnd(weeks, i, now)
		for fid := range t.Features {
			status := r.statusAt(fid, end)
			if status == "" || status == "cancelled" {
				continue
			}
			switch r.statusAt(r.parentAt(fid, end), end) {
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
