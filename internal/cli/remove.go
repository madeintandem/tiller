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

type removeView struct {
	Noun         string   `json:"noun"`
	ID           string   `json:"id"`
	Key          string   `json:"key"`
	Reason       string   `json:"reason"`
	RemovedIDs   []string `json:"removed_ids"`
	ChangedFiles []string `json:"changed_files"`
	Detached     []string `json:"detached,omitempty"`
}

type removeBlockedView struct {
	Noun         string `json:"noun,omitempty"`
	ID           string `json:"id,omitempty"`
	Key          string `json:"key,omitempty"`
	Status       string `json:"status,omitempty"`
	Reason       string `json:"reason"`
	Detail       string `json:"detail"`
	ReturnRemedy string `json:"return_remedy,omitempty"`
	Remedy       string `json:"remedy"`
}

func newRemoveCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "remove <epic|feature|task|bug|chore|tweak> <node> --reason <why-this-proposal-is-removed>",
		Short: "Retire a contributor or remove an unapproved proposal node.",
		Long: "remove contributor retires a registered person who has no live assignments or open questions.\n" +
			"Other remove commands permanently erase only an unapproved proposal: a proposed task, a task-free draft feature,\n" +
			"a draft epic whose draft feature subtree is task-free, or a proposed bug, chore, or tweak. References and the\n" +
			"surviving audit event change atomically.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error { return runRemove(cmd, args[0], args[1], reason) },
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why this unapproved proposal is being removed")
	addContributorFlag(cmd)
	cmd.AddCommand(newContributorRemoveCmd())
	return cmd
}

func runRemove(cmd *cobra.Command, noun, node, reason string) error {
	noun, err := validNoun(cmd, "remove", noun, []string{store.NounEpic, store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak})
	if err != nil {
		return err
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "remove", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "remove", err)
	}
	res, err := mutate.RemoveDraft(tree, d, mutate.RemoveInput{Noun: noun, Node: node, Reason: reason})
	if err != nil {
		return reportRemoveError(cmd, err)
	}

	view := removeView{Noun: res.Noun, ID: res.ID, Key: res.Key, Reason: res.Reason,
		RemovedIDs: nonNilStrings(res.RemovedIDs), ChangedFiles: nonNilStrings(res.ChangedFiles),
		Detached: res.Detached}
	state := "draft"
	if res.Noun == store.NounTask || store.IsWorkNoun(res.Noun) {
		state = "proposed"
	}
	var human strings.Builder
	fmt.Fprintf(&human, "Removed %s %s %s.\nRemoved nodes:", state, res.Noun, gen.RefParts(res.Key, res.ID))
	for _, id := range res.RemovedIDs {
		nodeNoun, key := removeNodeReference(tree, id)
		fmt.Fprintf(&human, "\n  %s %s", nodeNoun, gen.RefParts(key, id))
	}
	fmt.Fprintf(&human, "\nReason: %s", res.Reason)
	seam := "decompose"
	if res.Noun == store.NounTask {
		seam = "specify"
	}
	fmt.Fprintf(&human, "\nAgent: re-run the proposal readout with `tiller readout %s --since <sha>`, naming the commit the last block you reported, before continuing.", seam)
	if len(res.Detached) > 0 {
		fmt.Fprintf(&human, " Cleared references on %d bug(s)/chore(s): %s.", len(res.Detached), strings.Join(res.Detached, ", "))
	}
	return emitSuccess(cmd, "remove", view, human.String())
}

func removeNodeReference(tree *store.Tree, id string) (string, string) {
	if node := tree.Epics[id]; node != nil {
		return store.NounEpic, derefCLI(node.Key)
	}
	if node := tree.Features[id]; node != nil {
		return store.NounFeature, derefCLI(node.Key)
	}
	if node := tree.Tasks[id]; node != nil {
		return store.NounTask, derefCLI(node.Key)
	}
	if node, ok := tree.WorkNode(id); ok {
		return node.Noun(), derefCLI(node.NodeKey())
	}
	return "node", ""
}

func derefCLI(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func reportRemoveError(cmd *cobra.Command, err error) error {
	var blocked *mutate.RemoveBlockedError
	if !errors.As(err, &blocked) {
		return emitFailure(cmd, "remove", err)
	}
	addr := blocked.ID
	if blocked.Key != "" {
		addr = blocked.Key
	}
	amendmentKey := strings.Trim(strings.ReplaceAll(strings.ToLower(addr), "/", "-"), "-") + "-proposal-correction"
	if amendmentKey == "-proposal-correction" {
		amendmentKey = "proposal-correction"
	}
	remedy := fmt.Sprintf("tiller add amendment %s --title 'Correct the proposal before removal'", amendmentKey)
	view := removeBlockedView{Noun: blocked.Noun, ID: blocked.ID, Key: blocked.Key, Status: blocked.Status,
		Reason: blocked.Code, Detail: blocked.Detail, Remedy: remedy}
	if blocked.Noun == store.NounTask && blocked.Status == "ready" {
		view.ReturnRemedy = fmt.Sprintf("tiller return task %s --reason 'revise the proposed task set'", addr)
	}
	var human strings.Builder
	fmt.Fprintf(&human, "Blocked: %s", blocked.Error())
	if blocked.Code == "proposed_amendment_reference" {
		human.WriteString(". Edit or withdraw the proposed amendment reference before removing the node")
	}
	if view.ReturnRemedy != "" {
		fmt.Fprintf(&human, ". Return the ready, unstarted task first with `%s`", view.ReturnRemedy)
	}
	fmt.Fprintf(&human, ". If the correction belongs in reviewed history, stage it with `%s`.", remedy)
	return emitReport(cmd, "remove", false, view, human.String(), 1)
}
