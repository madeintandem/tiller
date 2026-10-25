package serve

// The Report page's data (docs/spec/ui-surface.md, Report): the
// client-facing metrics payload, a thin shell over views.ReportOf. It loads the
// working tree fresh per request like every read endpoint and never writes.

import (
	"net/http"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
)

// ReportHandler serves GET /api/report. Weeks follow the serving process's
// local timezone (D5). A checkout whose branch cannot be read (no git repo, no
// git binary) reports an empty branch, which suppresses the off-branch notice
// rather than failing the read.
//
// Epic progress reads the observed tree, since in-flight task states live on
// branches; everything else reads the integration tree the event log
// describes, so the replay is unchanged.
func ReportHandler(root string) http.HandlerFunc {
	return api(root, func(observed *store.Tree, r *http.Request) (any, error) {
		t := executionSnapshot(r).Base
		evs, err := events.List(t)
		if err != nil {
			// Like the recap, a broken stream is a 500: the report is the stream.
			return nil, err
		}
		branch, err := vcs.New(root).Branch()
		if err != nil {
			branch = ""
		}
		return views.ReportOf(t, views.ReportInput{
			Events:   evs,
			Now:      time.Now(),
			Location: time.Local,
			Branch:   branch,
			Observed: observed,
		}), nil
	})
}
