// Package readinput gathers local observations for pure read projections.
package readinput

import (
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/execution"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
)

// Workflow observes contracts, accepted bases, ordinary aggregate inclusion,
// and immutable recovery evidence once per owning aggregate.
func Workflow(t *store.Tree) workflow.Input { return mutate.ObserveWorkflow(t) }

// WorkflowObserved is Workflow plus the task execution observed on local
// branches and worktrees (issue #149), for route guidance. Observation is a
// pure read that degrades: on any discovery or stability error Observed
// stays nil and routing proceeds on integration alone.
func WorkflowObserved(t *store.Tree) workflow.Input {
	in := Workflow(t)
	snapshot, err := execution.Load(t)
	if err != nil {
		return in
	}
	if _, err := snapshot.Settle(); err != nil {
		return in
	}
	in.Observed = ObservationsFrom(snapshot)
	return in
}

// ObservationsFrom projects a settled execution snapshot onto the routing
// input: each observed task's branch and the owner that branch records. A
// task whose source is the served checkout itself is left out: from its own
// task checkout the route is ordinary execution, not work in flight elsewhere,
// on the CLI and on a console served from that checkout alike.
func ObservationsFrom(s *execution.Snapshot) map[string]workflow.Observation {
	if s == nil {
		return nil
	}
	var out map[string]workflow.Observation
	for id, record := range s.Observations {
		if s.Served(id) {
			continue
		}
		if out == nil {
			out = map[string]workflow.Observation{}
		}
		out[id] = workflow.Observation{Branch: record.Branch, Owner: record.Owner}
	}
	return out
}

// ForNext gathers what ranking next moves reads beyond the tree: the observed
// workflow input and the events inside the recency window. Like the
// observation, a failed event read degrades, here to no recent events.
func ForNext(t *store.Tree, reader string, now time.Time) views.NextInput {
	return views.NextInput{Reader: reader, Workflow: WorkflowObserved(t), Recent: RecentEvents(t, now), Now: now}
}

// RecentEvents reads the events inside the recency window ending at now. A
// failed read degrades to none, which ranks without recency.
func RecentEvents(t *store.Tree, now time.Time) []events.Event {
	recent, err := events.ListSince(t, now.Add(-views.RecentWindow))
	if err != nil {
		return nil
	}
	return recent
}

// WorkflowForNode scopes observations to the addressed node's resolved review.
func WorkflowForNode(t *store.Tree, node store.Node) workflow.Input {
	return mutate.ObserveWorkflowForNode(t, node)
}

// WorkflowForNodes scopes observations to the addressed nodes' resolved reviews.
func WorkflowForNodes(t *store.Tree, nodes []store.Node) workflow.Input {
	return mutate.ObserveWorkflowForNodes(t, nodes)
}
