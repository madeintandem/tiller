package cli

// This file wires `tiller order <epic|feature|task> <node> [<node> ...]`
// (docs/plans/2026-08-08-soft-order-design.md), the soft order's one command.
// It is a thin shell over internal/core/mutate.SetOrder, built the same way
// link.go shells mutate.Link: flags in, a
// SetOrderInput out, and the standard success/error envelope on the result.
// Every rule this command's own refusals could duplicate (all-or-nothing
// resolution, the duplicate/split-parent/empty/clear-parent guards, replace
// semantics) lives in mutate.SetOrder instead, so this file states none of
// them a second time.

import (
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

// orderView is the SUCCESS payload of order: the grain, the parent node id
// that now carries the list (absent at epic grain, the root), the recorded
// sequence after the write, and whether anything changed.
type orderView struct {
	Noun    string   `json:"noun"`
	Parent  string   `json:"parent,omitempty"` // parent node id; absent at epic grain
	Order   []string `json:"order"`
	Changed bool     `json:"changed"`
}

func newOrderCmd() *cobra.Command {
	var clear bool
	var epicParent, featureParent string
	cmd := &cobra.Command{
		Use:   "order <epic|feature|task|bug|chore|tweak> <node> [<node> ...] [--clear] [--epic <e>] [--feature <f>]",
		Short: "Record the soft order: the intended sequence among siblings.",
		Long: "order records the soft order: intent, not constraint. It arranges\n" +
			"every rendered sibling list and steers tiller's recommendations\n" +
			"wherever depends_on leaves the choice open. It never gates: working in\n" +
			"another order is never refused. A rerun replaces the recorded list\n" +
			"outright; a partial list is legal, and the siblings it names lead\n" +
			"their unlisted peers wherever dependencies allow. --clear erases one\n" +
			"parent's list: `tiller order epic --clear` erases the root list, and\n" +
			"--epic or --feature names the parent whose list to clear at the other\n" +
			"two grains. A bug, chore, or tweak order is the shelf's own list, with no\n" +
			"parent to name: `order bug --clear` erases it. Who may run: both; an\n" +
			"agent records an order on the developer's explicit instruction, per\n" +
			"the delegation rules.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOrder(cmd, args[0], args[1:], clear, epicParent, featureParent)
		},
	}
	cmd.Flags().BoolVar(&clear, "clear", false, "erase the recorded list at one parent (root, or the parent named by --epic/--feature)")
	cmd.Flags().StringVar(&epicParent, "epic", "", "the epic whose feature order --clear erases")
	cmd.Flags().StringVar(&featureParent, "feature", "", "the feature whose task order --clear erases")
	addContributorFlag(cmd)
	return cmd
}

func runOrder(cmd *cobra.Command, noun string, nodes []string, clear bool, epicParent, featureParent string) error {
	sing, err := validNoun(cmd, "order", noun, []string{store.NounEpic, store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak})
	if err != nil {
		return err
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "order", err)
	}

	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "order", err)
	}
	res, err := mutate.SetOrder(tree, d, mutate.SetOrderInput{
		Noun:          sing,
		Nodes:         nodes,
		Clear:         clear,
		ParentEpic:    epicParent,
		ParentFeature: featureParent,
	})
	if err != nil {
		return emitFailure(cmd, "order", err)
	}

	view := orderView{Noun: res.Noun, Parent: res.Parent, Order: nonNilStrings(res.Order), Changed: res.Changed}
	return emitSuccess(cmd, "order", view, humanOrderLine(tree, res))
}

// humanOrderLine renders order's confirmation: the scope (the noun, plus the
// owning parent's ref where one exists), then the recorded sequence by ref
// per the reference convention, or "cleared"/"empty" when the list is now
// nil. Changed and no-change both report, the same shape link.go's own
// confirmation takes.
func humanOrderLine(tree *store.Tree, res mutate.SetOrderResult) string {
	scope := res.Noun + " order"
	if res.Parent != "" {
		scope += " under " + gen.RefByID(tree, res.Parent)
	}
	if len(res.Order) == 0 {
		if res.Changed {
			return scope + " cleared."
		}
		return scope + " already empty (no change)."
	}
	list := strings.Join(refsByID(tree, res.Order), ", ")
	if res.Changed {
		return scope + " now: " + list + "."
	}
	return scope + " already: " + list + " (no change)."
}
