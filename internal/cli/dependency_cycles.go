package cli

import (
	"strings"

	"github.com/madeintandem/tiller/internal/core/readiness"
	"github.com/madeintandem/tiller/internal/store"
)

// cycleDiagnosis is one blocking cycle as readiness words it, the shape
// `tiller check readiness` carries in gaps.
type cycleDiagnosis = readiness.Gap

// blockingCycles is readiness's own diagnosis of every execution dependency
// cycle recorded in tree, the gaps `tiller check readiness` reports.
func blockingCycles(tree *store.Tree) []cycleDiagnosis {
	return readiness.DependencyDiagnostics(tree).Gaps
}

// cyclesCreated reloads the tree a command just wrote and returns the
// blocking cycles readiness reports now that it did not report in before, so
// the command that creates a deadlock says so while it can still be undone.
// It is a printed diagnosis, never a refusal: the write already succeeded,
// and a reload failure reports nothing rather than failing it.
func cyclesCreated(root string, before []cycleDiagnosis) []cycleDiagnosis {
	fresh, err := store.Load(root)
	if err != nil {
		return nil
	}
	known := make(map[string]bool, len(before))
	for _, gap := range before {
		known[gap.Message] = true
	}
	var created []cycleDiagnosis
	for _, gap := range blockingCycles(fresh) {
		if !known[gap.Message] {
			created = append(created, gap)
		}
	}
	return created
}

// withCycleDiagnosis closes a command's confirmation with each cycle's
// readiness message.
func withCycleDiagnosis(human string, cycles []cycleDiagnosis) string {
	if len(cycles) == 0 {
		return human
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(human, "\n"))
	b.WriteString("\n")
	for _, cycle := range cycles {
		b.WriteString("\nBlocking dependency cycle: " + cycle.Message)
	}
	return b.String()
}
