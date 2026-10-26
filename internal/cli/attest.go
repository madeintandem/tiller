package cli

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file implements `tiller attest <noun> <node>` (cli-surface.md, "Gates,
// evals, and attestation"): the human judgment Tiller cannot produce. With --gate,
// it sets an attestation-backed optional gate on a feature, task, bug, chore, or tweak.
// With --criterion, it signs off a manual or required eval criterion on a feature
// or task. The sign-off is recorded in the node's attestations list and satisfies
// the target. This is a thin shell over internal/core/mutate.Attest.
//
// attest is human-only (the lifecycle actor map classes it ActorHuman), and it is
// the only path that satisfies a required eval criterion: the agent's recorded
// rubric leaves the criterion needs_attestation, and a passing attest here is what
// flips it to pass. The outcome is a SUCCESS (the recorded attestation) or a tool
// failure (a bad address, an unknown target, a missing owner); attest carries no
// blocked business answer of its own.

// AttestView is the data block of an attest: the node, its unchanged lifecycle
// status, the target signed off, the attested status, the minted attestation id,
// and whether the target is now satisfied.
type AttestView struct {
	Noun          string   `json:"noun"`
	ID            string   `json:"id"`
	Key           string   `json:"key,omitempty"`
	Status        string   `json:"status"`
	TargetKind    string   `json:"target_kind"`
	TargetID      string   `json:"target_id"`
	AttestStatus  string   `json:"attest_status"`
	AttestationID string   `json:"attestation_id"`
	Satisfied     bool     `json:"satisfied"`
	Changed       bool     `json:"changed"`
	Warnings      []string `json:"warnings,omitempty"`
}

func newAttestCmd() *cobra.Command {
	var gate, criterion, status, reason string
	cmd := &cobra.Command{
		Use:   "attest <noun> <node>",
		Short: "Record a human attestation for a gate or a criterion.",
		Long: "attest records the human judgments Tiller cannot produce itself. With\n" +
			"--gate it sets an attestation-backed optional review gate on a feature,\n" +
			"task, bug, chore, or tweak. With --criterion it signs off a manual or required\n" +
			"eval criterion on a feature or task that `tiller eval` reported pending.\n" +
			"A passing attestation satisfies the target. A required eval criterion is\n" +
			"never satisfied by the agent's rubric alone, only here.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttest(cmd, args[0], args[1], mutate.AttestInput{
				Gate: gate, Criterion: criterion, Status: status, Reason: reason,
			})
		},
	}
	cmd.Flags().StringVar(&gate, "gate", "", "an attestation-backed optional gate to sign off")
	cmd.Flags().StringVar(&criterion, "criterion", "", "a manual or eval criterion to sign off")
	cmd.Flags().StringVar(&status, "status", "", "the attested status: pass or fail (required)")
	cmd.Flags().StringVar(&reason, "reason", "", "an optional note recorded on the event")
	_ = cmd.MarkFlagRequired("status")
	addContributorFlag(cmd)
	return cmd
}

// runAttest resolves the node, records the attestation through the core, and renders
// the success envelope.
func runAttest(cmd *cobra.Command, noun, node string, in mutate.AttestInput) error {
	tree, n, err := resolveForMutation(cmd, "attest", noun, node, []string{store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak})
	if err != nil {
		return err
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "attest", err)
	}
	res, err := mutate.Attest(tree, d, n, in)
	if err != nil {
		if data, human, ok := branchBlockedView(tree, err); ok {
			return emitReport(cmd, "attest", false, data, human, 1)
		}
		return emitFailure(cmd, "attest", err)
	}
	// attest is one of the four human gates (gateMomentFor, readout_gate.go),
	// so its confirmation carries the readout after it. act.Attested carries
	// exactly what an earlier review found missing: n, res.TargetKind, and
	// res.TargetID were already in scope here, but emitReportWithReadout's
	// old signature had nowhere to put them, so attestContent
	// (internal/views/readout.go) fell back to a tree-wide tally instead of
	// the per-node "what remains unmet" the design table asks for.
	act := &views.GateAct{Attested: &views.GateActAttest{
		NodeID: res.ID, TargetKind: res.TargetKind, TargetID: res.TargetID,
	}}
	return emitReportWithReadout(cmd, "attest", true, attestView(res), humanAttest(res, gen.Ref(tree, n)), 0, gateMomentFor("attest"), tree.Root, act)
}

// attestView projects the core result onto the JSON data block.
func attestView(res mutate.AttestResult) AttestView {
	return AttestView{
		Noun: res.Noun, ID: res.ID, Key: res.Key, Status: res.Status,
		TargetKind: res.TargetKind, TargetID: res.TargetID, AttestStatus: res.AttestStatus,
		AttestationID: res.AttestationID, Satisfied: res.Satisfied, Changed: res.Changed,
		Warnings: res.Warnings,
	}
}

// humanAttest is the plain-text confirmation for a terminal reader. The node
// is named by the caller-computed reference ("<key-path> (<id>)").
func humanAttest(res mutate.AttestResult, ref string) string {
	s := fmt.Sprintf("Attested %s %s on %s %s: %s (%s).",
		res.TargetKind, res.TargetID, res.Noun, ref, res.AttestStatus, res.AttestationID)
	for _, w := range res.Warnings {
		s += "\nWarning: " + w
	}
	return s
}
