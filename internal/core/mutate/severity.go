package mutate

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/store"
)

// severityUsage is the shared refusal for a severity a write would set: it
// applies to bugs only and must be one of the four grades. An empty value is
// "not given" on any noun and passes. Add, grade, and amend all call it.
func severityUsage(noun, value string) error {
	if value == "" {
		return nil
	}
	if noun != store.NounBug {
		return &usageError{reason: "severity_not_bug", msg: "severity applies to bugs"}
	}
	if !store.ValidSeverity(value) {
		return &usageError{reason: "invalid_severity", msg: fmt.Sprintf("severity %q is not blocker, important, medium, or minor", value)}
	}
	return nil
}

// validateTransitionSeverity refuses a Severity on any transition that is not
// an approve crossing a bug's triage gate: another verb, a refresh-basis
// approve, a noun that carries no severity, or a value that is not a grade.
func validateTransitionSeverity(noun string, in TransitionInput) error {
	if in.Severity == nil {
		return nil
	}
	if in.Verb != "approve" || in.RefreshBasis {
		return &usageError{reason: "bad_args", msg: "--severity applies only to approve bug, and not with --refresh-basis"}
	}
	if noun != store.NounBug {
		return severityUsage(noun, *in.Severity)
	}
	if *in.Severity == "" {
		return &usageError{reason: "invalid_severity", msg: "severity needs one of blocker, important, medium, or minor"}
	}
	return severityUsage(noun, *in.Severity)
}

// SeverityRequiredError refuses approve on a bug no one has graded: triage
// does not cross the gate until a severity is chosen.
type SeverityRequiredError struct {
	NodeID string
	Key    string
}

func (e *SeverityRequiredError) Error() string {
	return fmt.Sprintf("bug %s has no severity; approve it with tiller approve bug %s --severity <blocker|important|medium|minor>", e.NodeID, e.Key)
}

// Reason is the stable discriminator the CLI envelope carries.
func (e *SeverityRequiredError) Reason() string { return "severity_required" }
