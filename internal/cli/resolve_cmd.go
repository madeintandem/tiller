package cli

import (
	"errors"
	"fmt"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

// resolveView is the SUCCESS payload of resolve.
type resolveView struct {
	Noun       string `json:"noun"`
	ID         string `json:"id"`
	Key        string `json:"key,omitempty"`
	ResolvedBy string `json:"resolved_by,omitempty"`
	Before     string `json:"before,omitempty"`
	Changed    bool   `json:"changed"`
}

// newResolveCmd builds `resolve <bug|chore|tweak> <node> --by <target> | --clear`,
// the carrier pointer's one writer (docs/spec/cli-surface.md).
// Approve class: human, or agent on an explicit instruction naming the node and
// the carrier.
func newResolveCmd() *cobra.Command {
	var by string
	var clear bool
	cmd := &cobra.Command{
		Use:   "resolve <bug|chore|tweak> <node> (--by <epic|feature|task> | --clear)",
		Short: "Point a bug, chore, or tweak at the tree work whose merge fixes it.",
		Long: "resolve records the carrier: the epic, feature, or task whose merge\n" +
			"fixes this bug, chore, or tweak. The node leaves the queue (its derived phase\n" +
			"reads carried) and sync crosses it to done, basis carried, in the run\n" +
			"that records the carrier's done. --clear returns it to the queue. It\n" +
			"moves no status. Who may run: human, or the agent on an explicit\n" +
			"instruction naming the node and the carrier; an agent that notices its\n" +
			"work fixes a bug says so and does not resolve it unbidden.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sing, err := validNoun(cmd, "resolve", args[0], store.ShelfNouns())
			if err != nil {
				return err
			}
			tree, err := loadTree(cmd)
			if err != nil {
				return emitFailure(cmd, "resolve", err)
			}
			d, err := deps(cmd, tree.Root)
			if err != nil {
				return emitFailure(cmd, "resolve", err)
			}
			res, err := mutate.Resolve(tree, d, mutate.ResolveInput{Noun: sing, Node: args[1], By: by, Clear: clear})
			if err != nil {
				var terminal *mutate.WorkTerminalError
				if errors.As(err, &terminal) {
					return emitReport(cmd, "resolve", false, map[string]any{
						"reason": terminal.Reason, "noun": terminal.Noun, "id": terminal.ID, "key": terminal.Key, "status": terminal.Status,
					}, fmt.Sprintf("%s %s is %s; nothing is left to resolve.", terminal.Noun, gen.RefParts(terminal.Key, terminal.ID), terminal.Status), 1)
				}
				return emitFailure(cmd, "resolve", err)
			}
			view := resolveView{Noun: res.Noun, ID: res.ID, Key: res.Key, ResolvedBy: res.ResolvedBy, Before: res.Before, Changed: res.Changed}
			ref := gen.RefParts(res.Key, res.ID)
			var human string
			switch {
			case !res.Changed && res.ResolvedBy == "":
				human = fmt.Sprintf("%s %s has no carrier (no change).", res.Noun, ref)
			case !res.Changed:
				human = fmt.Sprintf("%s %s is already carried by %s (no change).", res.Noun, ref, gen.RefByID(tree, res.ResolvedBy))
			case res.ResolvedBy == "":
				human = fmt.Sprintf("%s %s returned to the queue; it was carried by %s.", res.Noun, ref, gen.RefByID(tree, res.Before))
			default:
				human = fmt.Sprintf("%s %s is now carried by %s; sync crosses it when that merge lands.", res.Noun, ref, gen.RefByID(tree, res.ResolvedBy))
			}
			return emitSuccess(cmd, "resolve", view, human)
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "the epic, feature, or task whose merge fixes this (id or key)")
	cmd.Flags().BoolVar(&clear, "clear", false, "clear the carrier and return the node to the queue")
	addContributorFlag(cmd)
	return cmd
}
