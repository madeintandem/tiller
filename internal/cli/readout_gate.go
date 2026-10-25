package cli

import (
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file is the human gates' own readout wiring: a gate prints its existing
// confirmation and then the moment's block, through emitReportWithReadout
// (envelope.go), so a developer running the command directly gets the same
// brief `tiller readout <seam>` prints at a seam, with nothing for an agent to
// remember to run. lifecycle_cmds.go, question.go, attest.go, and sync.go all
// call into this file rather than each carrying its own copy of the
// command-to-moment mapping.

// gateMomentFor maps a command onto the human-gate moment its readout renders
// under, and "" for every command that is not one of them. The four gates are
// approve, answer question, attest, and sync (docs/spec/cli-surface.md:172).
// cancel question also emits a gate readout, but it shares the cancel command
// with non-gate node cancels, so runCancelQuestion (question_cancel.go) passes
// views.MomentCancelQuestion directly rather than routing through here.
func gateMomentFor(command string) views.Moment {
	switch command {
	case "approve":
		return views.MomentApprove
	case "answer":
		return views.MomentAnswer
	case "attest":
		return views.MomentAttest
	case "sync":
		return views.MomentSync
	default:
		return ""
	}
}

// emitTransitionSuccess is runTransition's shared success tail: emitSuccess
// unchanged when m is "" (every non-gate verb, byte-identical to before this
// task), or the moment's block appended through emitReportWithReadout when m
// names a gate. Routing both cases through one function keeps the gate-or-not
// decision in exactly one place per call site. act is nil whenever m is "" (a
// non-gate verb has nothing to build one for); every gate call site builds it
// from the mutate.TransitionResult it already has in hand (singleNodeAct
// below, or applyTransitionBatch's own accumulation across a batch).
func emitTransitionSuccess(cmd *cobra.Command, command, root string, m views.Moment, act *views.GateAct, data any, human string) error {
	if m == "" {
		return emitSuccess(cmd, command, data, human)
	}
	return emitReportWithReadout(cmd, command, true, data, human, 0, m, root, act)
}

// singleNodeAct wraps one lifecycle transition's own result as the
// readout's Act payload: the exact noun/id/Changed a single-node approve
// already computed (mutate.TransitionResult), so approveContent
// (internal/views/readout.go) can answer "what this act did or
// unblocked" without a second tree scan.
func singleNodeAct(noun, id string, changed bool) *views.GateAct {
	return &views.GateAct{Nodes: []views.GateActNode{{Noun: noun, ID: id, Changed: changed}}}
}
