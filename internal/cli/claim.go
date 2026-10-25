package cli

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

type ClaimView struct {
	Noun     string `json:"noun"`
	ID       string `json:"id"`
	Key      string `json:"key,omitempty"`
	Owner    string `json:"owner"`
	Changed  bool   `json:"changed"`
	Takeover bool   `json:"takeover,omitempty"`
	From     string `json:"from,omitempty"`
}

type ReleaseView struct {
	Noun    string `json:"noun"`
	ID      string `json:"id"`
	Key     string `json:"key,omitempty"`
	From    string `json:"from,omitempty"`
	Changed bool   `json:"changed"`
}

type ReassignView struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

func newClaimCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "claim", Short: "Reserve a task, bug, chore, or tweak before it starts.",
		Long: "Claim individual execution work. Taking another developer's unstarted reservation requires --from naming its holder and an explicit human handoff instruction. Started and terminal ownership cannot change; started unfinished work needs an explicitly authorized transfer.",
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	for _, noun := range []string{store.NounTask, store.NounBug, store.NounChore, store.NounTweak} {
		cmd.AddCommand(newClaimWorkCmd(noun))
	}
	return cmd
}

func newClaimWorkCmd(noun string) *cobra.Command {
	var from string
	cmd := &cobra.Command{Use: noun + " <node>", Aliases: []string{noun + "s"}, Short: "Reserve an unstarted " + noun + ".",
		Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return runClaimWork(cmd, noun, args[0], from) }}
	cmd.Flags().StringVar(&from, "from", "", "current holder; use only for an explicit human-directed pre-start handoff")
	addContributorFlag(cmd)
	return cmd
}

func runClaimWork(cmd *cobra.Command, noun, ref, from string) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "claim", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "claim", err)
	}
	var res mutate.ClaimResult
	if noun == store.NounTask {
		res, err = mutate.ClaimTask(tree, d, ref, from)
	} else {
		res, err = mutate.ClaimWork(tree, d, noun, ref, from)
	}
	if err != nil {
		return reportTransitionError(cmd, "claim", tree, nil, err)
	}
	view := ClaimView{Noun: res.Noun, ID: res.ID, Key: res.Key, Owner: res.Owner, Changed: res.Changed, Takeover: res.Takeover, From: res.From}
	return emitSuccess(cmd, "claim", view, humanClaim(gen.RefParts(res.Key, res.ID), res))
}

func humanClaim(ref string, res mutate.ClaimResult) string {
	if !res.Changed {
		return fmt.Sprintf("%s is already claimed by %s; no change.", ref, res.Owner)
	}
	if res.Takeover {
		return fmt.Sprintf("Claimed %s for %s, taken over from %s.", ref, res.Owner, res.From)
	}
	return fmt.Sprintf("Claimed %s for %s.", ref, res.Owner)
}

func newReleaseCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "release", Short: "Return an unstarted execution claim to the pool.",
		Long: "Release your own task, bug, chore, or tweak reservation before start. Started and terminal ownership is immutable.",
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	for _, noun := range []string{store.NounTask, store.NounBug, store.NounChore, store.NounTweak} {
		cmd.AddCommand(newReleaseWorkCmd(noun))
	}
	return cmd
}

func newReleaseWorkCmd(noun string) *cobra.Command {
	cmd := &cobra.Command{Use: noun + " <node>", Aliases: []string{noun + "s"}, Short: "Release an unstarted " + noun + ".", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return runReleaseWork(cmd, noun, args[0]) }}
	addContributorFlag(cmd)
	return cmd
}

func runReleaseWork(cmd *cobra.Command, noun, ref string) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "release", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "release", err)
	}
	var res mutate.ReleaseResult
	if noun == store.NounTask {
		res, err = mutate.ReleaseTask(tree, d, ref)
	} else {
		res, err = mutate.ReleaseWork(tree, d, noun, ref)
	}
	if err != nil {
		return reportTransitionError(cmd, "release", tree, nil, err)
	}
	view := ReleaseView{Noun: res.Noun, ID: res.ID, Key: res.Key, From: res.From, Changed: res.Changed}
	return emitSuccess(cmd, "release", view, humanRelease(gen.RefParts(res.Key, res.ID), res))
}

func humanRelease(ref string, res mutate.ReleaseResult) string {
	if !res.Changed {
		return fmt.Sprintf("%s is already unclaimed; no change.", ref)
	}
	return fmt.Sprintf("Released %s; back in the pool.", ref)
}

func newReassignCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "reassign", Short: "Hand an unstarted execution claim to a registered contributor.",
		Long: "Reassign only before start. An agent runs this human decision only on an explicit instruction naming the work, recipient, and reason. For started unfinished work use transfer; terminal history never changes.",
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	for _, noun := range []string{store.NounTask, store.NounBug, store.NounChore, store.NounTweak} {
		cmd.AddCommand(newReassignWorkCmd(noun))
	}
	return cmd
}

func newReassignWorkCmd(noun string) *cobra.Command {
	var to, reason string
	cmd := &cobra.Command{Use: noun + " <node> --to <owner> --reason <text>", Aliases: []string{noun + "s"}, Short: "Reassign an unstarted " + noun + ".", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return runReassignWork(cmd, noun, args[0], to, reason) }}
	cmd.Flags().StringVar(&to, "to", "", "registered recipient (required)")
	cmd.Flags().StringVar(&reason, "reason", "", "human-directed reason (required)")
	addContributorFlag(cmd)
	return cmd
}

func runReassignWork(cmd *cobra.Command, noun, ref, to, reason string) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "reassign", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "reassign", err)
	}
	var res mutate.ReassignResult
	if noun == store.NounTask {
		res, err = mutate.ReassignTask(tree, d, ref, to, reason)
	} else {
		res, err = mutate.ReassignWork(tree, d, noun, ref, to, reason)
	}
	if err != nil {
		return reportTransitionError(cmd, "reassign", tree, nil, err)
	}
	view := ReassignView{Noun: res.Noun, ID: res.ID, Key: res.Key, From: res.From, To: res.To, Reason: res.Reason}
	return emitSuccess(cmd, "reassign", view, humanReassign(gen.RefParts(res.Key, res.ID), res))
}

func humanReassign(ref string, res mutate.ReassignResult) string {
	from := res.From
	if from == "" {
		from = "the pool"
	}
	return fmt.Sprintf("Reassigned %s from %s to %s: %s.", ref, from, res.To, res.Reason)
}
