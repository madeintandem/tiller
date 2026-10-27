package serve

// The Report page's Agent use tab (docs/plans/2026-10-26-agent-use-report-design.md,
// "Serving it"): a thin shell over views.UsageReportOf. It reads the committed
// .tiller/usage/ files and the event stream through their process caches,
// walks no Git history, and never writes.

import (
	"net/http"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/usage"
	"github.com/madeintandem/tiller/internal/views"
)

// UsageReportHandler serves GET /api/report/usage on the same week axis and
// integration tree as ReportHandler. A broken event stream or a malformed
// usage file is a 500 naming it: the tab is those files.
func UsageReportHandler(root string) http.HandlerFunc {
	return api(root, func(_ *store.Tree, r *http.Request) (any, error) {
		t := executionSnapshot(r).Base
		evs, err := events.List(t)
		if err != nil {
			return nil, err
		}
		files, err := usage.LoadFiles(root)
		if err != nil {
			return nil, err
		}
		return views.UsageReportOf(t, views.UsageReportInput{
			Files:    files,
			Events:   evs,
			Now:      time.Now(),
			Location: time.Local,
		}), nil
	})
}
