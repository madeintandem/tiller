package serve

// This file is the console's Explore search endpoint (ui-surface.md,
// "Explore"): GET /api/search?q=<query>, the same keyword scan `tiller
// search` runs and the same payload shape its --json Data carries, so the two
// surfaces answer identically. It rides the shared api() wrapper like every
// other read endpoint: the query parameter is the only thing it reads off the
// request, plus the optional kind filter and limit, and search.Search does the
// rest over the freshly loaded tree.

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/madeintandem/tiller/internal/search"
	"github.com/madeintandem/tiller/internal/store"
)

// SearchHandler answers GET /api/search?q=...&kind=K&limit=N with the
// search's grouped payload. An empty or missing q and an unknown kind are a
// 400, the HTTP twins of the CLI's misuse exit; a corpus the scan cannot read
// is a 500, the environment's answer rather than the caller's.
func SearchHandler(root string) http.HandlerFunc {
	return api(root, func(t *store.Tree, r *http.Request) (any, error) {
		params := r.URL.Query()
		res, err := search.Search(root, t, params.Get("q"), search.Options{
			Kind:  params.Get("kind"),
			Limit: searchLimit(params.Get("limit")),
		})
		if err != nil {
			var empty *search.EmptyQueryError
			if errors.As(err, &empty) {
				return nil, &httpError{http.StatusBadRequest, empty.Error()}
			}
			var unknown *search.UnknownKindError
			if errors.As(err, &unknown) {
				return nil, &httpError{http.StatusBadRequest, unknown.Error()}
			}
			return nil, err
		}
		return res, nil
	})
}

// searchLimit parses the limit parameter the same way the event feed's
// eventsLimit does: absent or unparsable falls back to the default, and
// anything above the ceiling is clamped to it.
func searchLimit(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return search.MaxResults
	}
	return min(n, search.MaxResults)
}
