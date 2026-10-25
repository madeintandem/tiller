package views

// This file carries the reporting half of the active-basis rule
// (cli-surface.md, "Specifying a feature": `status`, `serve`, and `drift`
// report active_basis while `start`, `check`, and `submit` refuse it). The
// refusal composes its own sentence in internal/cli; what lives here is the
// part both surfaces must agree on, the node whose pin moved and the exact
// command that clears it, so the terminal and the console can never hand a
// reader two different remedies for one drift.
//
// The comparison itself is I/O (git hashes the working tree), so it stays with
// the callers: mutate.BasisDrift runs in the CLI's status command and in the
// serve node handler, and only its already-computed answer reaches this pure
// projection.

import (
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// ActiveBasisView is one node's drift report: the pinned files whose current
// working-tree content no longer matches the accepted basis, which node's pin
// moved, and the move that clears it.
//
// The node named here is not always the node the page is about. A task's guard
// compares its parent feature's spec and contract before its own task.md
// (mutate.guardTask), so a task page can carry a report whose Noun and ID are
// the FEATURE's, which is the grain the remedy addresses. A renderer that drops
// that distinction hands a reader a feature's command with nothing saying why.
//
// Missing names the pinned files that are gone rather than changed. It matters
// to the reader because `--refresh-basis` re-records from the working tree and
// cannot hash a file that is not there: the file has to be restored first, so
// Command alone would walk the reader into a failure.
type ActiveBasisView struct {
	Drifted []string `json:"drifted"`
	Missing []string `json:"missing,omitempty"`
	Noun    string   `json:"noun"`
	ID      string   `json:"id"`
	Ref     string   `json:"ref"`
	Command string   `json:"command"`
}

// ActiveBasisOf projects a computed drift onto the shared report. The refresh
// command comes from RefreshBasisCommand, the one composer every surface
// shares (the CLI's remedy clause, this view, and the drift panel's
// candidate rows), so no two surfaces can hand a reader different moves.
func ActiveBasisOf(t *store.Tree, ab *mutate.ActiveBasisError) ActiveBasisView {
	return ActiveBasisView{
		Drifted: ab.Drifted,
		Missing: ab.Missing,
		Noun:    ab.Noun,
		ID:      ab.NodeID,
		Ref:     gen.RefByID(t, ab.NodeID),
		Command: RefreshBasisCommand(ab.Noun, basisAddr(t, ab)),
	}
}

// RefreshBasisCommand composes the one move that clears an active_basis
// drift, for the node addressed as the caller resolved it (key where one
// exists, id otherwise). Exported as the single composer behind
// ActiveBasisView.Command and the drift panel's candidate remedy: one
// composer, so a terminal reader and a console reader cannot be told to run
// different things (the precedent activeBasisRemedy's own doc records).
func RefreshBasisCommand(noun, addr string) string {
	return "tiller approve " + noun + " " + addr + " --refresh-basis"
}

// ReviewedAssumptionCommand composes the one move that clears an
// answered_question drift for one question: --reviewed-assumption reviews
// one question at a time by deliberate design (mutate.ReviewAssumption), so
// a caller holding several questions composes several commands. Shared by
// the CLI's submit refusal remedy and the drift panel's candidate rows, the
// same one-composer rule RefreshBasisCommand states.
func ReviewedAssumptionCommand(addr, questionID string) string {
	return "tiller approve feature " + addr + " --reviewed-assumption " + questionID
}

// basisAddr is the address the refresh command names: the key of the node whose
// pin has to be refreshed, falling back to its id. The lookup goes through the
// drift's own noun and id rather than through whatever the caller addressed,
// since a task's guard reports its parent feature's pin.
func basisAddr(t *store.Tree, ab *mutate.ActiveBasisError) string {
	var n store.Node
	switch ab.Noun {
	case store.NounFeature:
		if f, ok := t.Features[ab.NodeID]; ok {
			n = f
		}
	case store.NounTask:
		if tk, ok := t.Tasks[ab.NodeID]; ok {
			n = tk
		}
	}
	if n != nil {
		if k := Key(n); k != "" {
			return k
		}
	}
	return ab.NodeID
}
