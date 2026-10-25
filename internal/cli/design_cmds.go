package cli

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Design docs have path identities, not work-node IDs or lifecycle states.
type designDocView struct {
	views.BaselineDocView
	Noun    string `json:"noun"`
	Changed bool   `json:"changed"`
}

type designApprovalView struct {
	Noun    string          `json:"noun"`
	Docs    []designDocView `json:"docs"`
	Changed bool            `json:"changed"`
}

func designDocViewOf(result mutate.DesignResult) designDocView {
	return designDocView{
		BaselineDocView: views.BaselineDocOf(result.Doc), Noun: store.NounDesign, Changed: result.Changed,
	}
}

// These flags belong only to design. Refuse them before an existing noun's
// handler can mutate one of its supported axes and silently drop the rest.
func rejectDesignOnlyFlags(cmd *cobra.Command, command string) error {
	for _, name := range []string{"describes", "from"} {
		if cmd.Flags().Changed(name) {
			return emitFailure(cmd, command, &usageError{
				reason: "bad_args", msg: fmt.Sprintf("--%s applies only to %s design", name, command),
			})
		}
	}
	return nil
}

// Test presence rather than value: an explicitly empty string or false bool is
// still an instruction that a design mutation must not silently ignore.
func validateDesignFlags(cmd *cobra.Command, command string) error {
	var rejected []string
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if !cmd.Flags().Changed(flag.Name) {
			return
		}
		switch flag.Name {
		case "contributor", "json":
			return
		case "describes", "from":
			if command != "approve" {
				return
			}
		}
		rejected = append(rejected, "--"+flag.Name)
	})
	if len(rejected) != 0 {
		return emitFailure(cmd, command, &usageError{
			reason: "bad_args",
			msg:    fmt.Sprintf("%s design does not accept %s", command, strings.Join(rejected, ", ")),
		})
	}
	return nil
}

func runDesignLink(cmd *cobra.Command, command string, input mutate.DesignLinkInput) error {
	if err := validateDesignFlags(cmd, command); err != nil {
		return err
	}
	if len(input.Describes) == 0 && len(input.From) == 0 {
		return emitFailure(cmd, command, &usageError{
			reason: command + "_target",
			msg:    fmt.Sprintf("%s design needs --describes or --from", command),
		})
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, command, err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, command, err)
	}
	var result mutate.DesignResult
	if command == "unlink" {
		result, err = mutate.UnlinkDesign(tree, d, input)
	} else {
		result, err = mutate.LinkDesign(tree, d, input)
	}
	if err != nil {
		return emitFailure(cmd, command, err)
	}
	human := fmt.Sprintf("Design .tiller/design/%s links unchanged (no change).", result.Doc.Path)
	if result.Changed {
		human = fmt.Sprintf("Updated design links for .tiller/design/%s.", result.Doc.Path)
		if command == "unlink" && len(result.Doc.Describes) == 0 {
			human = fmt.Sprintf("Removed .tiller/design/%s from the baseline ledger; design prose is unchanged.", result.Doc.Path)
		}
	}
	return emitSuccess(cmd, command, designDocViewOf(result), human)
}

func runApproveDesign(cmd *cobra.Command, docs []string) error {
	if err := validateDesignFlags(cmd, "approve"); err != nil {
		return err
	}
	if len(docs) == 0 {
		return emitFailure(cmd, "approve", &usageError{
			reason: "bad_args", msg: "approve design needs at least one named design doc",
		})
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "approve", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "approve", err)
	}
	// One atomic mutation for every named doc, never the work-node batch path
	// that permits one successful transition beside another node's refusal.
	result, err := mutate.ApproveDesign(tree, d, docs)
	if err != nil {
		return emitFailure(cmd, "approve", err)
	}
	view := designApprovalView{
		Noun: store.NounDesign, Docs: make([]designDocView, 0, len(result.Docs)),
	}
	act := &views.DesignApprovalAct{}
	lines := make([]string, 0, len(result.Docs))
	for _, doc := range result.Docs {
		view.Docs = append(view.Docs, designDocViewOf(doc))
		view.Changed = view.Changed || doc.Changed
		if doc.Changed {
			act.Confirmed = append(act.Confirmed, doc.Doc.Path)
			lines = append(lines, fmt.Sprintf("Confirmed design .tiller/design/%s.", doc.Doc.Path))
		} else {
			act.Unchanged = append(act.Unchanged, doc.Doc.Path)
			lines = append(lines, fmt.Sprintf("Design .tiller/design/%s confirmation unchanged (no change).", doc.Doc.Path))
		}
	}
	return emitReportWithReadout(cmd, "approve", true, view,
		strings.Join(lines, "\n"), 0,
		views.MomentApprove, tree.Root, &views.GateAct{DesignApproval: act})
}
