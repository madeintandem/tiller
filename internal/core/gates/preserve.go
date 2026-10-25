package gates

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// PreserveCoverage names a preserve criterion whose check the required tests
// gate already runs on every feature check, so the criterion adds no signal of
// its own. ExactCommand is the stronger case: the criterion's command is the
// tests gate's own command. Otherwise it is a test binding that only names a
// ref, which the tests gate's whole-suite run already exercises.
type PreserveCoverage struct {
	CriterionID  string
	ExactCommand bool
}

// Warning is the advisory `spec feature` prints for the covered criterion. It
// is guidance for a human, never a refusal: the criterion may stay when the
// contract's Notes say why.
func (p PreserveCoverage) Warning() string {
	if p.ExactCommand {
		return fmt.Sprintf("criterion %s (preserve) runs the tests gate's own command, which already runs on every check: give it a targeted check, drop it, or say in Notes from this run why it stays", p.CriterionID)
	}
	return fmt.Sprintf("criterion %s (preserve) is a test the tests gate already runs on every check: drop it, or say in Notes from this run why it stays", p.CriterionID)
}

// CoveredPreserveCriteria returns, in contract order, the preserve criteria
// that only repeat the required tests gate. It is empty when the contract or
// manifest is absent, when there is no required tests gate, or when that gate
// does not run a command on a feature check: a task-only tests gate leaves a
// feature-level preserve criterion as the only place the suite runs, and an
// attestation or blank-run gate runs nothing for a criterion to repeat.
//
// It compares strings and never reads criterion wording: a criterion counts
// only when its resolved command is exactly the tests gate's run (after
// trimming), or it is a test binding that names a ref and no run. A required
// and an optional criterion count alike, since either costs a repeated run.
func CoveredPreserveCriteria(ef *store.EvalFile, m *store.Manifest) []PreserveCoverage {
	if ef == nil || m == nil {
		return nil
	}
	tests, ok := m.Gates.Required["tests"]
	if !ok || !tests.On.Includes(store.NounFeature) {
		return nil
	}
	// A tests gate that is an attestation or has a blank run executes nothing,
	// so no criterion repeats it and the manifest proves no redundancy.
	testsRun := strings.TrimSpace(tests.Run)
	if tests.Kind != store.GateKindCommand || testsRun == "" {
		return nil
	}

	var out []PreserveCoverage
	for _, c := range ef.Criteria {
		if c.Polarity != store.PolarityPreserve {
			continue
		}
		if c.Binding.Type != store.BindingCommand && c.Binding.Type != store.BindingTest {
			continue
		}
		run := strings.TrimSpace(c.Binding.Run)
		switch {
		case run != "" && run == testsRun:
			out = append(out, PreserveCoverage{CriterionID: c.ID, ExactCommand: true})
		case run == "" && c.Binding.Type == store.BindingTest && !c.Binding.Ref.Empty():
			out = append(out, PreserveCoverage{CriterionID: c.ID})
		}
	}
	return out
}
