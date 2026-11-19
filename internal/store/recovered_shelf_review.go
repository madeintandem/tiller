package store

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// RecoveredShelfReview binds a bug, chore, or tweak to the immutable evidence
// of a review that merged before Tiller recorded its submission. A shelf node
// has no riders and no acceptance contract, so the record is the evidence and
// its provenance alone. Branch is the node's recorded working branch, absent
// when the fix merged before the node was ever started.
type RecoveredShelfReview struct {
	Branch            string      `yaml:"branch,omitempty"`
	Head              RevisionRef `yaml:"head"`
	MergeCommit       RevisionRef `yaml:"merge_commit"`
	IntegrationBranch string      `yaml:"integration_branch"`
	Basis             string      `yaml:"basis"`
	RecoveredBy       string      `yaml:"recovered_by"`
	RecoveredAt       time.Time   `yaml:"recovered_at"`
	Reason            string      `yaml:"reason"`
}

const (
	recoveredShelfFields   = "branch head merge_commit integration_branch basis recovered_by recovered_at reason"
	recoveredShelfRequired = "head merge_commit integration_branch basis recovered_by recovered_at reason"
)

// UnmarshalYAML is as strict as the aggregate record's: a typo or a partial
// record refuses instead of leaving the node to ordinary observation.
func (r *RecoveredShelfReview) UnmarshalYAML(node *yaml.Node) error {
	fields, err := recoveryMapping(node, recoveredShelfFields, recoveredShelfRequired)
	if err != nil {
		return fmt.Errorf("recovered_review: %w", err)
	}
	for _, name := range []string{"head", "merge_commit"} {
		if _, err := recoveryMapping(fields[name], "kind value path observed_at", "kind value"); err != nil {
			return fmt.Errorf("recovered_review: %s: %w", name, err)
		}
	}
	type plain RecoveredShelfReview
	var decoded plain
	if err := node.Decode(&decoded); err != nil {
		return fmt.Errorf("recovered_review: %w", err)
	}
	*r = RecoveredShelfReview(decoded)
	return r.Validate()
}

// Validate checks durable shape independent of Git availability.
func (r *RecoveredShelfReview) Validate() error {
	bad := func(format string, args ...any) error { return fmt.Errorf("recovered_review: "+format, args...) }
	if r == nil {
		return bad("record must be a mapping")
	}
	return validateRecoveryProvenance(bad, r.Branch, r.IntegrationBranch, r.RecoveredBy, r.Reason, r.Basis, r.RecoveredAt, r.Head, r.MergeCommit)
}

// Review is the record in the aggregate shape, with no riders and no policy
// snapshots, for the evidence check and the readers both record kinds share.
func (r *RecoveredShelfReview) Review() *RecoveredReview {
	if r == nil {
		return nil
	}
	return &RecoveredReview{
		Branch: r.Branch, Head: r.Head, MergeCommit: r.MergeCommit,
		IntegrationBranch: r.IntegrationBranch, Basis: r.Basis,
		RecoveredBy: r.RecoveredBy, RecoveredAt: r.RecoveredAt, Reason: r.Reason,
	}
}

func (b *Bug) UnmarshalYAML(node *yaml.Node) error {
	if err := validateRecoveryPresence(node); err != nil {
		return err
	}
	type plain Bug
	return node.Decode((*plain)(b))
}

func (c *Chore) UnmarshalYAML(node *yaml.Node) error {
	if err := validateRecoveryPresence(node); err != nil {
		return err
	}
	type plain Chore
	return node.Decode((*plain)(c))
}

func (t *Tweak) UnmarshalYAML(node *yaml.Node) error {
	if err := validateRecoveryPresence(node); err != nil {
		return err
	}
	type plain Tweak
	return node.Decode((*plain)(t))
}
