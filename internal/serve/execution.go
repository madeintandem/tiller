package serve

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/execution"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
)

type executionContextKey struct{}

func withExecutionSnapshot(r *http.Request, snapshot *execution.Snapshot) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), executionContextKey{}, snapshot))
}

func executionSnapshot(r *http.Request) *execution.Snapshot {
	return r.Context().Value(executionContextKey{}).(*execution.Snapshot)
}

// titledObservation is an observation plus its task's title. The title is
// added here, in the serve layer, so internal/execution stays free of the tree.
type titledObservation struct {
	execution.Record
	Title string `json:"title,omitempty"`
}

type titledExecutionReportBody struct {
	Observations []titledObservation    `json:"observations"`
	Diagnostics  []execution.Diagnostic `json:"diagnostics"`
}

func titledExecutionReport(t *store.Tree, report execution.Report) titledExecutionReportBody {
	out := titledExecutionReportBody{
		Observations: make([]titledObservation, len(report.Observations)),
		Diagnostics:  report.Diagnostics,
	}
	for i, record := range report.Observations {
		out.Observations[i] = titledObservation{Record: record}
		if task := t.Tasks[record.ID]; task != nil {
			out.Observations[i].Title = task.Title
		}
	}
	return out
}

type observedNodePage struct {
	views.NodePage
	Execution             *execution.Record `json:"execution,omitempty"`
	ExecutionHistoryError string            `json:"execution_history_error,omitempty"`
	// HistoryPending marks an integration-only answer for an observed node:
	// its branch history was not read, and the plain node path merges it.
	HistoryPending bool `json:"history_pending,omitempty"`
}

func observedEvents(ctx context.Context, snapshot *execution.Snapshot, id string, base []events.Event) ([]events.Event, error) {
	if _, ok := snapshot.Observations[id]; !ok {
		return base, nil
	}
	source, err := snapshot.Events(ctx, id)
	if err != nil {
		return base, err
	}
	byID := make(map[string]events.Event, len(base)+len(source))
	for _, event := range base {
		byID[event.ID] = event
	}
	for _, event := range source {
		if prior, exists := byID[event.ID]; exists {
			// Integration owns already-published history; immutable identities
			// cannot be replaced by a branch's competing copy.
			if !reflect.DeepEqual(prior, event) {
				return base, fmt.Errorf("execution history conflicts with integration event %s", event.ID)
			}
			continue
		}
		byID[event.ID] = event
	}
	merged := make([]events.Event, 0, len(byID))
	for _, event := range byID {
		merged = append(merged, event)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].At.Equal(merged[j].At) {
			return merged[i].ID < merged[j].ID
		}
		return merged[i].At.Before(merged[j].At)
	})
	return merged, nil
}

func executionEvidence(snapshot *execution.Snapshot, record execution.Record, node *views.NodeView) {
	qualify := func(evidence []views.EvidenceView) {
		for i := range evidence {
			e := &evidence[i]
			if e.Ref == "" {
				continue
			}
			name, err := evidenceRelativePath(e.Ref)
			if err != nil {
				e.Unavailable = err.Error()
				continue
			}
			data, err := snapshot.ReadFile(record.ID, record.Revision, name, maxServedFileBytes)
			if errors.Is(err, execution.ErrTooLarge) {
				e.Unavailable = fmt.Sprintf("Execution-source evidence is too large for the console (over %d MiB)", maxServedFileBytes>>20)
				continue
			}
			if err != nil {
				e.Unavailable = "Execution-source evidence unavailable: " + err.Error()
				continue
			}
			query := url.Values{"ref": {e.Ref}, "task": {record.ID}, "revision": {record.Revision}, "digest": {fmt.Sprintf("%x", sha256.Sum256(data))}}
			e.URL = "/api/evidence?" + query.Encode()
		}
	}
	for name, gate := range node.Gates {
		qualify(gate.Evidence)
		for id, criterion := range gate.Criteria {
			qualify(criterion.Evidence)
			gate.Criteria[id] = criterion
		}
		node.Gates[name] = gate
	}
}

func nodeEvidenceRef(node views.NodeView, ref string) bool {
	for _, gate := range node.Gates {
		for _, evidence := range gate.Evidence {
			if evidence.Ref == ref {
				return true
			}
		}
		for _, criterion := range gate.Criteria {
			for _, evidence := range criterion.Evidence {
				if evidence.Ref == ref {
					return true
				}
			}
		}
	}
	return false
}
