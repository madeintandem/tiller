package serve

// This file serves GET /api/next, the console's ranked list of legal next
// moves (ranked-next design, "Surfaces / Console"). The payload is the
// views.Next that `tiller next --json` prints, with Candidates cut to the
// shortlist, so the console and the CLI cannot disagree about what comes
// first. It replaces /api/route, which is gone.

import (
	"net/http"
	"time"

	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
)

// NextHandler answers GET /api/next, ranked for the served checkout's
// contributor through actingContributor (home.go): TILLER_CONTRIBUTOR, else
// the checkout's identity file, else empty, which ranks unpersonalized.
func NextHandler(root string) http.HandlerFunc {
	return api(root, func(_ *store.Tree, r *http.Request) (any, error) {
		// Ranking reads the served checkout. Observations never change
		// eligibility: the reader's own in-flight task is a move on its
		// branch and anyone else's is no move here, as on the CLI (#149).
		snapshot := executionSnapshot(r)
		t := snapshot.Base
		in := readinput.Workflow(t)
		in.Observed = readinput.ObservationsFrom(snapshot)
		now := time.Now()
		next := views.NextOf(t, views.NextInput{
			Reader: actingContributor(root), Workflow: in,
			Recent: readinput.RecentEvents(t, now), Now: now,
		})
		next.Candidates = next.Shortlist()
		return next, nil
	})
}
