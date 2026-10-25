package serve

// The Home view's data (ui-surface.md, "Home"): the action queue and the
// recap. Both are thin shells over the pure derivations in internal/views,
// loading the working tree fresh per request like every read endpoint, so the
// picture is live local files. The recap is the one endpoint that also touches
// the console's visit memory, and only for the default (visit) window.

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
)

// QueueHandler serves GET /api/queue using the shared workflow read boundary,
// including live eligibility and accepted-basis checks.
func QueueHandler(root string) http.HandlerFunc {
	return apiIntegration(root, func(t *store.Tree, _ *http.Request) (any, error) {
		in := views.QueueInput{Workflow: readinput.Workflow(t)}
		return views.QueueOfInput(t, in), nil
	})
}

// recapHandler serves GET /api/recap?window=visit|24h|7d: the mechanically
// derived digest of the event stream since a horizon. The visit window (the
// default) advances and reads the console's visit memory, so the horizon is the
// last time this reader looked; 24h and 7d ignore that memory and use now minus
// the window, the Monday-morning case. An unknown window is a client error, not
// a silent default. The reader the asks are scoped to is resolved per request
// from the checkout's identity.
//
// Only a request carrying visitHeader advances the horizon. Any page or peer
// that reaches the port can send a plain GET, but a cross-site page cannot
// attach a custom header without a CORS preflight, and the console answers
// none. An unmarked request reads the recap at the peeked horizon and writes
// nothing, the way Home's count read does.
func recapHandler(root string, visits *visitState) http.HandlerFunc {
	return recapHandlerWithEvents(root, visits, events.List)
}

// visitHeader marks the console's own History visit, the one request allowed
// to advance the recap horizon. The UI sends it as "1" (api.ts visitMarker).
const visitHeader = "X-Tiller-Visit"

func recapHandlerWithEvents(
	root string,
	visits *visitState,
	list func(*store.Tree) ([]events.Event, error),
) http.HandlerFunc {
	return apiIntegration(root, func(t *store.Tree, r *http.Request) (any, error) {
		window := r.URL.Query().Get("window")
		if window == "" {
			window = "visit"
		}
		switch window {
		case "visit", "24h", "7d":
		default:
			return nil, &httpError{http.StatusBadRequest, "unknown window " + window + "; use visit, 24h, or 7d"}
		}

		evs, err := list(t)
		if err != nil {
			// Unlike the node page, a broken stream is a 500 here: the digest is
			// the stream, so there is nothing else to serve.
			return nil, err
		}

		var horizon time.Time
		switch window {
		case "visit":
			horizon = visits.peek(root)
		case "24h":
			horizon = time.Now().UTC().Add(-24 * time.Hour)
		case "7d":
			horizon = time.Now().UTC().Add(-7 * 24 * time.Hour)
		}
		recap := views.RecapOf(t, evs, horizon, window, actingContributor(root))
		if window == "visit" && r.Header.Get(visitHeader) == "1" {
			return afterStability(recap, func() { visits.mark(root) }), nil
		}
		return recap, nil
	})
}

// recapPeek is the counts-only projection of the digest, the ~90 bytes behind
// Home's recap signal: the horizon the recap view would read against, who the
// asks are scoped to, and how many events and asks sit behind it.
type recapPeek struct {
	Since  string `json:"since"`
	Reader string `json:"reader"`
	Total  int    `json:"total"`
	Asks   int    `json:"asks"`
}

// recapPeekHandler serves GET /api/recap/peek: the same digest the recap view
// renders, projected down to its counts and, crucially, WITHOUT advancing the
// visit horizon. Home carries a quiet line naming how many events have piled up
// behind the horizon, and reading that line must never consume a recap the
// human has not opened (ui-surface.md, "Recap"), so the horizon comes from
// visits.peek rather than visits.mark.
//
// It runs the counts through the same views.RecapOf the view calls rather than
// counting events itself, so the claim-churn filter and the ask rule live in
// one place and the two surfaces can never disagree about what an event is.
// It is a route rather than a query parameter on /api/recap because a
// parameter that silently changes a GET's write behavior is a worse API than a
// name that states the one thing that matters: looking does not consume.
func recapPeekHandler(root string, visits *visitState) http.HandlerFunc {
	return apiIntegration(root, func(t *store.Tree, _ *http.Request) (any, error) {
		evs, err := events.List(t)
		if err != nil {
			// Same order the recap handler keeps: nothing is read from the
			// visit state until the stream proves readable.
			return nil, err
		}
		r := views.RecapOf(t, evs, visits.peek(root), "visit", actingContributor(root))
		asks := 0
		for _, g := range r.Groups {
			for _, e := range g.Entries {
				if e.Ask {
					asks++
				}
			}
		}
		// Asks are never capped by the per-group cap, so counting the served
		// entries counts all of them; Total is already the view's own true
		// pre-cap number.
		return recapPeek{Since: r.Since, Reader: r.Reader, Total: r.Total, Asks: asks}, nil
	})
}

// actingContributor resolves the checkout's acting contributor the same way the CLI does
// (internal/cli/add.go's resolveActingContributor, minus the --contributor flag no read
// endpoint carries): the TILLER_CONTRIBUTOR override wins, else the checkout's
// local identity file, else empty. It is every contributor-aware read endpoint's one
// resolution path (the recap's reader scoping here, the route's execute-arm
// scoping in route.go), so a checkout's acting contributor reads the same everywhere
// this process serves it. A malformed identity file is treated as no identity
// rather than failing the read: identity is a courtesy the caller degrades
// gracefully without, not a precondition.
func actingContributor(root string) string {
	if owner := strings.TrimSpace(os.Getenv("TILLER_CONTRIBUTOR")); owner != "" {
		return owner
	}
	if owner, err := store.ReadLocalIdentity(root); err == nil {
		return owner
	}
	return ""
}
