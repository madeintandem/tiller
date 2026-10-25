package serve

// This file serves the raw event feed, GET /api/events?limit=N: the stream the
// recap and every node history derive from, newest first, for the console's
// "what just happened" reads. The payload is a serve-facing shape (the CLI has
// no events read yet); it carries each event's identity, actor, command,
// targets, and the touched file paths, never the before/after state blobs,
// which stay a node-page and recap concern.

import (
	"net/http"
	"strconv"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
)

// Feed limits: how many events come back when the caller does not say (the
// console's first page) and the most it may ask for.
const (
	eventsDefaultLimit = 100
	eventsMaxLimit     = 1000
)

// eventFeed is the /api/events payload. Count is the number of events in this
// response (the same convention as ls's count over its nodes), and Events is
// always an array, empty rather than null.
type eventFeed struct {
	Count  int              `json:"count"`
	Events []eventFeedEntry `json:"events"`
}

// eventFeedEntry is one event in the feed: the minted id, the RFC3339 UTC
// timestamp, the type discriminator, the acting contributor, the literal command,
// the node ids it targeted, and the paths of the files it touched. Targets and
// Files are always arrays, empty rather than null.
type eventFeedEntry struct {
	ID          string   `json:"id"`
	At          string   `json:"at"`
	Type        string   `json:"type"`
	Contributor string   `json:"contributor"`
	Command     string   `json:"command"`
	Targets     []string `json:"targets"`
	Files       []string `json:"files"`
}

// EventsHandler serves the feed over one repo root, reading the stream fresh
// per request like every read endpoint. Unlike the node page, a broken event
// file here is a 500: the feed has nothing to serve but the stream itself.
func EventsHandler(root string) http.HandlerFunc {
	return apiIntegration(root, func(t *store.Tree, r *http.Request) (any, error) {
		evs, err := events.List(t)
		if err != nil {
			return nil, err
		}

		limit := eventsLimit(r.URL.Query().Get("limit"))
		feed := eventFeed{Events: []eventFeedEntry{}}
		// events.List returns the canonical ascending order; the feed walks it
		// backwards so the newest event is first.
		for i := len(evs) - 1; i >= 0 && len(feed.Events) < limit; i-- {
			feed.Events = append(feed.Events, feedEntry(evs[i]))
		}
		feed.Count = len(feed.Events)
		return feed, nil
	})
}

// eventsLimit parses the limit parameter: absent or unparsable falls back to
// the default, and anything above the cap is clamped to it.
func eventsLimit(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return eventsDefaultLimit
	}
	if n > eventsMaxLimit {
		return eventsMaxLimit
	}
	return n
}

// feedEntry projects one stored event onto the feed row, files reduced to
// their paths.
func feedEntry(e events.Event) eventFeedEntry {
	files := make([]string, 0, len(e.Files))
	for _, f := range e.Files {
		files = append(files, f.Path)
	}
	return eventFeedEntry{
		ID:          e.ID,
		At:          e.At.UTC().Format(time.RFC3339),
		Type:        e.Type,
		Contributor: e.Contributor,
		Command:     e.Command,
		Targets:     append([]string{}, e.Targets...),
		Files:       files,
	}
}
