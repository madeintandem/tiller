package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file holds the two commands a shelf batch has to itself, `tiller add
// batch` and `tiller batch`, and the renderers the read and refusal paths
// share. Every other verb a batch takes (claim, start, check, submit, resume,
// cancel, show, status, ls, sync) is the shelf's own command with batch as its
// noun. batch.go, despite its name, is the multi-node arm of approve.

// BatchMemberRef is one member in a batch command's envelope.
type BatchMemberRef struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Status string `json:"status"`
}

// BatchView is the --json payload of `add batch` and `batch`.
type BatchView struct {
	Noun        string           `json:"noun"`
	ID          string           `json:"id"`
	Key         string           `json:"key"`
	Title       string           `json:"title,omitempty"`
	Status      string           `json:"status"`
	Members     []BatchMemberRef `json:"members"`
	Added       []BatchMemberRef `json:"added,omitempty"`
	Removed     []BatchMemberRef `json:"removed,omitempty"`
	Created     bool             `json:"created,omitempty"`
	Changed     bool             `json:"changed,omitempty"`
	GatesStaled bool             `json:"gates_staled,omitempty"`
	Cancelled   bool             `json:"cancelled,omitempty"`
	Warnings    []string         `json:"warnings,omitempty"`
}

func batchMemberRefs(members []mutate.BatchMember) []BatchMemberRef {
	out := make([]BatchMemberRef, len(members))
	for i, m := range members {
		out[i] = BatchMemberRef{Noun: m.Noun, ID: m.ID, Key: m.Key, Status: m.Status}
	}
	return out
}

func batchView(res mutate.BatchResult) BatchView {
	v := BatchView{
		Noun: store.NounBatch, ID: res.ID, Key: res.Key, Title: res.Title, Status: res.Status,
		Members: batchMemberRefs(res.Members), Created: res.Created, Changed: res.Changed,
		GatesStaled: res.GatesStaled, Cancelled: res.Cancelled, Warnings: res.Warnings,
	}
	if len(res.Added) > 0 {
		v.Added = batchMemberRefs(res.Added)
	}
	if len(res.Removed) > 0 {
		v.Removed = batchMemberRefs(res.Removed)
	}
	return v
}

// batchDelegation is the who-may-run sentence both batch commands carry.
const batchDelegation = "Who may run: human, or the agent on an explicit instruction that names the\n" +
	"members and asks for one review; an agent does not batch work unbidden."

// newAddBatchCmd builds `add batch <key> --title <text> <member>...`.
func newAddBatchCmd() *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:     "batch <key> --title <text> <member>...",
		Aliases: []string{store.ShelfDir(store.NounBatch)},
		Short:   "Put two or more bugs, chores, or tweaks under one review (.tiller/batches/<key>/).",
		Long: "add batch creates .tiller/batches/<key>/batch.yml: one branch, one pull\n" +
			"request, and one merge for several bugs, chores, and tweaks, which may be\n" +
			"mixed. Name each member by id (B-, CH-, TW-) or by shelf key (bugs/<key>).\n" +
			"The batch is created ready and needs no approval of its own. Its members\n" +
			"may still be proposed, but start batch refuses until each is approved.\n" +
			"A member keeps its stored status and reads batched; the batch alone is\n" +
			"claimed, started, checked, and submitted, and sync crosses every member\n" +
			"to done with it. A node that is done or cancelled, has started its own\n" +
			"work, rides a carrier, holds a recovered review, is in another live\n" +
			"batch, or is claimed by someone else cannot join.\n" + batchDelegation,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tree, err := loadTree(cmd)
			if err != nil {
				return emitFailure(cmd, "add", err)
			}
			d, err := deps(cmd, tree.Root)
			if err != nil {
				return emitFailure(cmd, "add", err)
			}
			res, err := mutate.AddBatch(tree, d, mutate.BatchInput{Key: args[0], Title: title, Members: args[1:]})
			if err != nil {
				return reportBatchError(cmd, "add", tree, err)
			}
			return emitSuccess(cmd, "add", batchView(res), humanAddBatch(res))
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "display title (required)")
	_ = cmd.MarkFlagRequired("title")
	addContributorFlag(cmd)
	return cmd
}

func humanAddBatch(res mutate.BatchResult) string {
	ref := gen.RefParts(res.Key, res.ID)
	if !res.Created {
		return fmt.Sprintf("Found existing batch %s in %s; unchanged. Change its members with tiller batch %s --add or --remove.",
			ref, res.Status, localBatchKey(res.Key))
	}
	lines := []string{fmt.Sprintf("Created batch %s in %s: %q, reviewing %d members:", ref, res.Status, res.Title, len(res.Members))}
	lines = append(lines, batchMemberLines(res.Members)...)
	if unapproved := proposedMembers(res.Members); len(unapproved) > 0 {
		lines = append(lines, fmt.Sprintf("start batch waits on approval of %s.", strings.Join(unapproved, ", ")))
	}
	lines = append(lines, fmt.Sprintf("Next: tiller claim batch %s, then tiller start batch %s.", localBatchKey(res.Key), localBatchKey(res.Key)))
	return strings.Join(lines, "\n")
}

func batchMemberLines(members []mutate.BatchMember) []string {
	lines := make([]string, len(members))
	for i, m := range members {
		lines[i] = fmt.Sprintf("  - %s %s (%s)", m.Noun, gen.RefParts(m.Key, m.ID), m.Status)
	}
	return lines
}

func proposedMembers(members []mutate.BatchMember) []string {
	var out []string
	for _, m := range members {
		if m.Status == "proposed" {
			out = append(out, m.ID)
		}
	}
	return out
}

// localBatchKey is the key a batch command takes: the segment after batches/.
func localBatchKey(canonical string) string {
	return strings.TrimPrefix(canonical, store.ShelfDir(store.NounBatch)+"/")
}

// newBatchCmd builds `batch <key> --add <member>... --remove <member>...`.
func newBatchCmd() *cobra.Command {
	var add, remove []string
	cmd := &cobra.Command{
		Use:   "batch <batch> (--add <member> | --remove <member>)...",
		Short: "Add members to a batch, or remove them.",
		Long: "batch changes which bugs, chores, and tweaks a batch reviews. It is\n" +
			"allowed while the batch is ready, in_progress, or check, and refuses in\n" +
			"in_review until tiller resume batch. A change after check stales the\n" +
			"batch's gates, so check runs again before submit. A member added to a\n" +
			"started batch must already be approved. A removed member keeps its\n" +
			"status and returns to its own queue; commits already made for it on the\n" +
			"batch branch are yours to revert. A batch that loses its last member is\n" +
			"cancelled.\n" + batchDelegation,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tree, err := loadTree(cmd)
			if err != nil {
				return emitFailure(cmd, "batch", err)
			}
			d, err := deps(cmd, tree.Root)
			if err != nil {
				return emitFailure(cmd, "batch", err)
			}
			res, err := mutate.EditBatch(tree, d, mutate.BatchEditInput{Batch: args[0], Add: add, Remove: remove})
			if err != nil {
				return reportBatchError(cmd, "batch", tree, err)
			}
			return emitSuccess(cmd, "batch", batchView(res), humanEditBatch(res))
		},
	}
	cmd.Flags().StringSliceVar(&add, "add", nil, "a bug, chore, or tweak to add (id or shelf key, repeatable)")
	cmd.Flags().StringSliceVar(&remove, "remove", nil, "a member to remove (id or shelf key, repeatable)")
	addContributorFlag(cmd)
	return cmd
}

func humanEditBatch(res mutate.BatchResult) string {
	ref := gen.RefParts(res.Key, res.ID)
	if !res.Changed {
		return fmt.Sprintf("batch %s already has those members (no change).", ref)
	}
	var lines []string
	if len(res.Added) > 0 {
		lines = append(lines, fmt.Sprintf("Added to batch %s:", ref))
		lines = append(lines, batchMemberLines(res.Added)...)
	}
	if len(res.Removed) > 0 {
		lines = append(lines, fmt.Sprintf("Removed from batch %s:", ref))
		lines = append(lines, batchMemberLines(res.Removed)...)
	}
	switch {
	case res.Cancelled:
		lines = append(lines, fmt.Sprintf("batch %s has no members left and is cancelled.", ref))
	case res.GatesStaled:
		lines = append(lines, fmt.Sprintf("The batch was checked over different members, so its gates are stale: run tiller check batch %s again before submit.", localBatchKey(res.Key)))
	}
	for _, warning := range res.Warnings {
		lines = append(lines, "Warning: "+warning+".")
	}
	if !res.Cancelled {
		lines = append(lines, fmt.Sprintf("batch %s is %s with %d members.", ref, res.Status, len(res.Members)))
	}
	return strings.Join(lines, "\n")
}

// BatchRefusalData is the data block of a blocked batch answer.
type BatchRefusalData struct {
	Reason       string `json:"reason"`
	Noun         string `json:"noun"`
	ID           string `json:"id"`
	Key          string `json:"key,omitempty"`
	CurrentState string `json:"current_state,omitempty"`
	Batch        string `json:"batch,omitempty"`
	BatchKey     string `json:"batch_key,omitempty"`
	Member       string `json:"member,omitempty"`
	Command      string `json:"command,omitempty"`
	// Paths are the state files to commit, on batch_state_uncommitted.
	Paths []string `json:"paths,omitempty"`
}

// batchBlockedView maps a batch refusal onto its data block and human line,
// the same blocked-answer shape the other lifecycle refusals take.
func batchBlockedView(_ *store.Tree, err error) (any, string, bool) {
	var refusal *mutate.BatchError
	if !errors.As(err, &refusal) {
		return nil, "", false
	}
	return BatchRefusalData{
		Reason: refusal.Reason, Noun: refusal.Noun, ID: refusal.NodeID, Key: refusal.Key, CurrentState: refusal.CurrentState,
		Batch: refusal.Batch, BatchKey: refusal.BatchKey, Member: refusal.Member, Command: refusal.Seam, Paths: refusal.Paths,
	}, "Blocked: " + refusal.Error() + ".", true
}

// reportBatchError renders a batch refusal as a blocked answer (exit 1) and
// anything else as a tool failure.
func reportBatchError(cmd *cobra.Command, command string, tree *store.Tree, err error) error {
	if data, human, matched := batchBlockedView(tree, err); matched {
		return emitReport(cmd, command, false, data, human, 1)
	}
	return emitFailure(cmd, command, err)
}

// renderBatchHuman writes the two batch blocks a node page can carry: on a
// member, the batch reviewing it with its status, branch, and review; on a
// batch, every member with its stored status.
func renderBatchHuman(b *strings.Builder, batch *views.BatchRefView, members []views.BatchMemberView) {
	if batch != nil {
		fmt.Fprintf(b, "\n  batch: %s is %s", batch.Ref, batch.Status)
		if batch.Branch != "" {
			fmt.Fprintf(b, ", branch %s", batch.Branch)
		}
		switch {
		case batch.PR != nil:
			fmt.Fprintf(b, ", PR #%d", *batch.PR)
		case batch.ReviewRef != nil && batch.ReviewRef.Head != "":
			fmt.Fprintf(b, ", review ref %s", batch.ReviewRef.Head)
		}
	}
	if members != nil {
		fmt.Fprintf(b, "\n  members: %d", len(members))
		for _, m := range members {
			fmt.Fprintf(b, "\n    %s %s: %s", m.Noun, m.Ref, m.Status)
			if m.Severity != "" {
				fmt.Fprintf(b, " (%s)", m.Severity)
			}
		}
	}
}
