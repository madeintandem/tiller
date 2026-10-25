package store

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// UnmarshalYAML preserves legacy crossing warnings while refusing malformed
// historical provenance. A bad source must never become an ordinary warning.
func (p *PolicyViolation) UnmarshalYAML(node *yaml.Node) error {
	type plain PolicyViolation
	var decoded plain
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	var fields map[string]yaml.Node
	if err := node.Decode(&fields); err != nil {
		return err
	}
	source, hasSource := fields["source_revision"]
	_, hasCriteria := fields["unresolved_criteria"]
	_, hasEvidence := fields["evidence_refs"]
	if hasSource || hasCriteria || hasEvidence {
		if _, err := recoveryMapping(node, "gate observed status evidence_ref source_revision unresolved_criteria evidence_refs", "gate status source_revision"); err != nil {
			return fmt.Errorf("policy_violations: %w", err)
		}
		if _, err := recoveryMapping(&source, "kind value path observed_at", "kind value path"); err != nil {
			return fmt.Errorf("policy_violations source_revision: %w", err)
		}
		if !recoveryPath(decoded.SourceRevision.Path) {
			return fmt.Errorf("policy_violations: source_revision must pin a project-relative historical source")
		}
		if err := canonicalRecoveryRevision(decoded.SourceRevision, KindGitBlob, decoded.SourceRevision.Path); err != nil {
			return fmt.Errorf("policy_violations source_revision: %w", err)
		}
		observation := HistoricalPolicyObservation{Gate: decoded.Gate, Status: decoded.Status, UnresolvedCriteria: decoded.UnresolvedCriteria, EvidenceRefs: decoded.EvidenceRefs}
		if err := validateHistoricalObservation(observation); err != nil {
			return fmt.Errorf("policy_violations: %w", err)
		}
	}
	*p = PolicyViolation(decoded)
	return nil
}
