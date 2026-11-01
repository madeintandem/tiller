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
