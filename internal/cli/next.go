package cli

import (
	"time"

	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file implements `tiller next`: the ranked list of the legal moves for
// this checkout, best first. It is read-only.

func newNextCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "next",
		Short: "List your likely next moves, ranked.",
		Long: "next lists the legal moves for this checkout, best first: who makes each,\n" +
			"what it does, to which node, why it ranks here, and the command or skill that\n" +
			"does it. It prints the top five; --all lists every legal move. It is read-only.\n\n" +
			"The list is ranked for the acting contributor (--contributor, then\n" +
			"TILLER_CONTRIBUTOR, then .tiller/identity.yml). With none, it is plan order,\n" +
			"not personalized.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runNext(cmd, all) },
	}
	cmd.Flags().BoolVar(&all, "all", false, "list every legal move, not just the top five")
	// The flag is registered directly, not through addContributorFlag: that
	// helper also marks the command as a mutation, which would add an "acting
	// as" line and an actor field to this read.
	cmd.Flags().String("contributor", "", "rank for this contributor id (falls back to TILLER_CONTRIBUTOR, then .tiller/identity.yml)")
	return cmd
}

func runNext(cmd *cobra.Command, all bool) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "next", err)
	}
	next := views.NextOf(tree, readinput.ForNext(tree, resolveActingContributor(cmd, tree.Root), time.Now()))
	payload := next
	if !all {
		payload.Candidates = next.Shortlist()
	}
	return emitSuccess(cmd, "next", payload, renderNextText(next, all))
}
