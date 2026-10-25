package cli

import (
	"errors"
	"fmt"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/spf13/cobra"
)

// RecoverReviewView is record-only success. EvidenceValidated is intentionally
// false on identical retries: those preserve the original record, not claim a
// new local Git inspection. PRAssociation is never a host observation.
type RecoverReviewView struct {
	Noun              string                    `json:"noun"`
	ID                string                    `json:"id"`
	Key               string                    `json:"key,omitempty"`
	PR                *int                      `json:"pr,omitempty"`
	ReviewRef         *ReviewRefView            `json:"review_ref,omitempty"`
	RecoveredReview   *workflow.RecoveredReview `json:"recovered_review"`
	Changed           bool                      `json:"changed"`
	EvidenceValidated bool                      `json:"evidence_validated"`
	PRAssociation     string                    `json:"pr_association,omitempty"`
	NextAction        string                    `json:"next_action"`
}

type RecoverReviewRefusalView struct {
	Reason       string `json:"reason"`
	Noun         string `json:"noun"`
	NodeID       string `json:"node"`
	TaskID       string `json:"task,omitempty"`
	Ref          string `json:"ref,omitempty"`
	Prerequisite string `json:"prerequisite"`
}

func runRecoverReview(cmd *cobra.Command, noun, key string, pr int, prChanged bool, head, result, reason string) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "submit", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "submit", err)
	}
	in := mutate.RecoverReviewInput{Noun: noun, Key: key, Head: head, MergeCommit: result, Reason: reason}
	if prChanged {
		in.PR = &pr
	}
	recovered, err := mutate.RecoverReview(tree, d, in)
	if err != nil {
		var refusal *mutate.RecoverReviewError
		if errors.As(err, &refusal) {
			if refusal.ExitCode() != 1 {
				return emitFailure(cmd, "submit", err)
			}
			payload := RecoverReviewRefusalView{Reason: refusal.Reason, Noun: refusal.Noun, NodeID: refusal.NodeID, TaskID: refusal.TaskID, Ref: refusal.Ref, Prerequisite: refusal.Prerequisite}
			return emitReport(cmd, "submit", false, payload, refusal.Error(), refusal.ExitCode())
		}
		return emitFailure(cmd, "submit", err)
	}
	view := RecoverReviewView{Noun: recovered.Noun, ID: recovered.ID, Key: recovered.Key, PR: recovered.PR, ReviewRef: reviewRefView(recovered.ReviewRef), RecoveredReview: workflow.RecoveryOf(recovered.PR, recovered.RecoveredReview), Changed: recovered.Changed, EvidenceValidated: recovered.Changed, NextAction: "tiller sync"}
	if recovered.PR != nil {
		view.PRAssociation = "caller_supplied"
	}
	human := fmt.Sprintf("Recorded historical %s review %s (%s); lifecycle, current acceptance, and gates are unchanged. Run tiller sync on %s.", noun, recovered.Key, recovered.RecoveredReview.Basis, recovered.RecoveredReview.IntegrationBranch)
	if !recovered.Changed {
		human = fmt.Sprintf("Historical %s review %s is already recorded; no changes and no new Git evidence validation. Run tiller sync on %s.", noun, recovered.Key, recovered.RecoveredReview.IntegrationBranch)
	}
	if recovered.PR != nil {
		human += " PR association is caller-supplied, not host-observed."
	}
	return emitSuccess(cmd, "submit", view, human)
}
