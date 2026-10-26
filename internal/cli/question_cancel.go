package cli

// cancel question is the human closing an open question that no longer needs
// a decision (docs/spec/cli-surface.md, "Proposal correction commands", the
// Questions paragraphs). It shares the cancel verb with the work tree, and like
// answer it is a human gate whose readout follows the confirmation.

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

func runCancelQuestion(cmd *cobra.Command, node, reason string) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "cancel", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "cancel", err)
	}
	res, err := mutate.CancelQuestion(tree, d, mutate.CancelInput{Node: node, Reason: reason})
	if err != nil {
		return reportQuestionError(cmd, "cancel", err)
	}
	outcome := "is now cancelled."
	if !res.Changed {
		outcome = "is already cancelled (no change)."
	}
	lines := []string{fmt.Sprintf("question %s (%q): %q %s", res.ID, res.Title, res.Statement, outcome)}
	for _, id := range res.AssumedBy {
		lines = append(lines, fmt.Sprintf("%s assumed %s; its plan stands as built.", gen.RefByID(tree, id), res.ID))
	}
	// Both outcomes carry the readout, the no-change one included: it is
	// still a completed cancel with ok true, as a repeated answer is.
	act := &views.GateAct{Cancelled: &views.GateActCancel{Blocks: res.Blocks}}
	return emitReportWithReadout(cmd, "cancel", true, questionPayload(res),
		strings.Join(lines, "\n"), 0, views.MomentCancelQuestion, tree.Root, act)
}
