package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

type transferRefView struct {
	mutate.TransferRef
	ReviewRef *ReviewRefView `json:"review_ref,omitempty"`
}

func transferRefViewOf(ref mutate.TransferRef) transferRefView {
	return transferRefView{ref, reviewRefView(ref.ReviewRef)}
}

func newTransferCmd() *cobra.Command {
	var to, reason string
	cmd := &cobra.Command{
		Use:   "transfer <task|bug|chore|tweak> <ref> --to <registered-owner> --reason <text>",
		Short: "Replace started unfinished work with an unstarted assignment to another owner.",
		Long: "Transfer is a human decision. An agent may run it only on an explicit instruction naming the work and recipient.\n" +
			"The original is cancelled with its owner, branch, review, evidence, and attribution retained.\n" +
			"A same-noun replacement receives unchanged approved scope, a new identity, and a ready claim.\n" +
			"Use reassign before start; merged or terminal work cannot transfer.\n" +
			"Tiller changes source records atomically, never Git refs or host PRs. The result lists required external follow-through.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			noun, err := validNoun(cmd, "transfer", args[0], []string{store.NounTask, store.NounBug, store.NounChore, store.NounTweak})
			if err != nil {
				return err
			}
			tree, err := loadTree(cmd)
			if err != nil {
				return emitFailure(cmd, "transfer", err)
			}
			d, err := deps(cmd, tree.Root)
			if err != nil {
				return emitFailure(cmd, "transfer", err)
			}
			res, err := mutate.Transfer(tree, d, mutate.TransferInput{Noun: noun, Node: args[1], To: to, Reason: reason})
			if err != nil {
				var blocked *mutate.TransferBlockedError
				if errors.As(err, &blocked) {
					payload := map[string]any{"reason": blocked.Code, "detail": blocked.Detail}
					if blocked.Successor != "" {
						payload["successor"] = blocked.Successor
					}
					return emitReport(cmd, "transfer", false, payload, blocked.Detail, 1)
				}
				return emitFailure(cmd, "transfer", err)
			}
			view := struct {
				mutate.TransferResult
				Original    transferRefView `json:"original"`
				Replacement transferRefView `json:"replacement"`
			}{res, transferRefViewOf(res.Original), transferRefViewOf(res.Replacement)}
			var human strings.Builder
			fmt.Fprintf(&human, "Transferred %s %s (%s) to %s (%s).\nOriginal cancelled; replacement ready and not started. Reason: %s\n", noun, gen.RefParts(res.Original.Key, res.Original.ID), res.Original.Owner, gen.RefParts(res.Replacement.Key, res.Replacement.ID), res.Replacement.Owner, res.Reason)
			if res.Original.Branch != "" {
				fmt.Fprintf(&human, "Historical branch retained: %s\n", res.Original.Branch)
			}
			if res.Original.PR != nil {
				fmt.Fprintf(&human, "Historical PR retained: #%d\n", *res.Original.PR)
			}
			if res.Original.ReviewRef != nil {
				fmt.Fprintf(&human, "Historical review retained: %s at %s\n", res.Original.ReviewRef.Branch, res.Original.ReviewRef.Head.Value)
			}
			for _, change := range res.Relationships {
				fmt.Fprintf(&human, "%s %s %s: [%s] -> [%s]\n", change.Noun, change.ID, change.Relation, strings.Join(change.Before, ", "), strings.Join(change.After, ", "))
			}
			if len(res.Invalidated) > 0 {
				fmt.Fprintf(&human, "Gate and acceptance results invalidated: %s\n", strings.Join(res.Invalidated, ", "))
			}
			for _, step := range res.FollowThrough {
				fmt.Fprintf(&human, "\n%s\n", step)
			}
			return emitSuccess(cmd, "transfer", view, strings.TrimSpace(human.String()))
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "registered recipient named by the human")
	cmd.Flags().StringVar(&reason, "reason", "", "human's reason for the transfer (required)")
	addContributorFlag(cmd)
	return cmd
}
