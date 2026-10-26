package views

import (
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/readiness"
	"github.com/madeintandem/tiller/internal/workflow"
)

// Candidate actors: who performs a ranked next move.
const (
	actorHuman = "human"
	actorAgent = "agent"
)

// NextNode is the node a next move acts on.
type NextNode struct {
	Noun  string `json:"noun"`
	ID    string `json:"id"`
	Key   string `json:"key,omitempty"`
	Ref   string `json:"ref"`
	Title string `json:"title,omitempty"`
}

// NextCandidate is one legal next move for the reader's checkout: who makes
// it, what it does, to which node, and the command or skill that does it.
type NextCandidate struct {
	Actor    string           `json:"actor"`
	Kind     string           `json:"kind"`
	Action   string           `json:"action"`
	Node     *NextNode        `json:"node,omitempty"`
	Command  string           `json:"command,omitempty"`
	Skill    string           `json:"skill,omitempty"`
	Reason   string           `json:"reason"`
	Tier     int              `json:"tier"`
	Branch   string           `json:"branch,omitempty"`
	Workflow *workflow.Advice `json:"workflow,omitempty"`

	// arm is the old route walk's arm order and pos the arm's own order:
	// together the plan-order tie-break inside a tier (plan ruling 3).
	arm, pos int
	// due is an answerable question's due date, as recorded.
	due string
}

// NextInput is what ranking reads beyond the tree: the reader, the I/O
// observations routing already consumes, and the reader's recent events with
// the clock reading they are measured against.
type NextInput struct {
	Reader   string
	Workflow workflow.Input
	Queue    QueueInput
	Recent   []events.Event
	Now      time.Time
}

// Next is the ranked answer to "what are my likely next moves": every legal
// move for the reader's checkout, best first, one row per node (ranked-next
// design, decisions 4 and 10). Surfaces render Shortlist; --all renders
// Candidates.
type Next struct {
	// Reader is who the list is ranked for, "" when unpersonalized.
	Reader string `json:"reader"`
	// Candidates is the full ranked list, never nil.
	Candidates []NextCandidate `json:"candidates"`
	// CaughtUp marks a checkout with no legal move; Hint then explains any
	// held work (plan ruling 6).
	CaughtUp bool `json:"caught_up"`
	// Unplanned is the caught-up flavor of a tree with no epics; Hint then
	// names how work enters.
	Unplanned bool   `json:"unplanned,omitempty"`
	Hint      string `json:"hint,omitempty"`
	// DependencyDiagnostics is set only when the tree has dependency gaps or
	// advisories.
	DependencyDiagnostics *readiness.Report `json:"dependency_diagnostics,omitempty"`

	// keep is the node of the reader's top Continue move, recorded before
	// dedupe so Shortlist keeps that node even when a better-ranked row for
	// it replaced the Continue row.
	keep string
}
