package views

// NextOwner answers "whose move is it" for one node, the small additive
// read-model projection task A2 of the console chart-table pass adds. It
// holds no rules of its own: it strictly folds the lifecycle engine's two
// existing answers, the legal-next verb set (lifecycle.NextMoves, exactly
// what the status oracle already reports as legal_next) and who may run each
// verb (lifecycle.ActorFor, the verbActors table #19 added), into the one bit
// a chart or table needs.

import (
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// NextOwner reports whose move a node is: "human" when every legal next verb
// from its current state is a human-only gate (approve, return,
// cancel, archive, answer, and the like), "agent" when at least one legal
// next verb is the agent's own assertion or a shared either verb (spec,
// start, check, submit, resume, sync), or "" at a terminal state with no
// legal next verb at all.
//
// "cancel" (and, from a ready state, "return") is a legal, human-only verb
// from nearly every non-terminal state in the table, so a reading of "any
// legal verb is human-only implies human" would answer "human" almost
// everywhere and "agent" nowhere. That is not what this reports: a node whose
// ONLY way forward is a human gate answers "human" (a specced feature
// awaiting approval, a proposed task awaiting approval, an open question
// awaiting its answer), while a node the agent can also act on right now
// answers "agent" even though a human can still cancel or return it from the
// same state (a ready task the agent can start, a task in review the agent
// can resume or whose merge it can sync). The human's cancel/return is an
// always-available override, not the node's forward progress.
//
// A question is not a transitions-table noun: NextMoves special-cases its
// open -> [answer, cancel] life outside the generic table, and that flows through
// here unchanged. answer and cancel are human-only, so an open question reports "human";
// an answered or cancelled question is terminal and reports "".
func NextOwner(n store.Node) string {
	moves := lifecycle.NextMoves(n)
	if len(moves) == 0 {
		return ""
	}
	for _, verb := range moves {
		if lifecycle.ActorFor(verb) != lifecycle.ActorHuman {
			return "agent"
		}
	}
	return "human"
}

// WhoseMoveCounts is the project-wide tally of whose move it is, the JSON
// face of ProjectSummary.WhoseMove: how many non-terminal nodes currently
// wait on a human-only gate and how many the agent can act on right now. A
// terminal node (NextOwner reports "") contributes to neither count.
type WhoseMoveCounts struct {
	Human int `json:"human"`
	Agent int `json:"agent"`
}

// WhoseMoveOf tallies NextOwner across every node the loaded tree carries.
// Live carried shelf nodes have left the queue and contribute no move; all
// other shelf nodes fold through the same lifecycle derivation as tree grains.
func WhoseMoveOf(t *store.Tree) WhoseMoveCounts {
	var c WhoseMoveCounts
	tally := func(n store.Node) {
		switch NextOwner(n) {
		case "human":
			c.Human++
		case "agent":
			c.Agent++
		}
	}
	for _, e := range t.Epics {
		tally(e)
	}
	for _, f := range t.Features {
		tally(f)
	}
	for _, tk := range t.Tasks {
		tally(tk)
	}
	for _, noun := range store.ShelfNouns() {
		for _, work := range t.WorkNodesOf(noun) {
			if gen.WorkActivity(t, work) == gen.ActivityCarried {
				continue
			}
			tally(work)
		}
	}
	for _, q := range t.Questions {
		tally(q)
	}
	return c
}
