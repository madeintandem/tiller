package drift

import "github.com/madeintandem/tiller/internal/store"

// activeBasisFeatureStatuses and activeBasisTaskStatuses are the sweep's own
// status filter, verbatim from the schema's active_basis bullet: "Sweeps
// ready and active features and approved non-terminal tasks." The filter
// lives here rather than inside mutate.BasisDrift/checkBasis, because
// accepted_against is never cleared at done or cancelled, and an unfiltered
// sweep would misfile stale_spec's business under this marker instead.
var activeBasisFeatureStatuses = map[string]bool{"ready": true, "active": true}
var activeBasisTaskStatuses = map[string]bool{"ready": true, "in_progress": true, "check": true, "in_review": true}

// ActiveBasisApplies reports whether n is a node the active_basis sweep
// compares at all: a ready or active feature, or an approved non-terminal
// task. It is the one rule every reporter of basis drift shares, so the node
// page never shows a finding (and a refresh-basis remedy the CLI refuses) on
// a node `tiller drift` would not list, such as a done feature whose eval an
// approved amendment corrected.
func ActiveBasisApplies(n store.Node) bool {
	switch v := n.(type) {
	case *store.Feature:
		return activeBasisFeatureStatuses[v.Status]
	case *store.Task:
		return activeBasisTaskStatuses[v.Status]
	default:
		return false
	}
}
