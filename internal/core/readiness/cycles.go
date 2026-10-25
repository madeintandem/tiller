package readiness

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// DependencyDiagnostics is the shared read-only cycle report used by readiness,
// orient/decompose, and the console. Ready here concerns dependencies only.
func DependencyDiagnostics(t *store.Tree) Report {
	classified := gen.ClassifyCycles(t)
	report := Report{Ready: len(classified.Blocking) == 0, Gaps: []Gap{}, Advisories: []Gap{}}
	for _, cycle := range classified.Blocking {
		labels := make([]string, 0, len(cycle.Members))
		for _, id := range cycle.Members {
			labels = append(labels, cycleMemberLabel(t, id))
		}
		noun := cycle.Noun + "s"
		fallback := "grouped rendering uses deterministic fallback order"
		if cycle.Noun == "mixed" {
			noun = "epic/feature completion obligations for"
		} else if cycle.Noun == store.NounFeature || cycle.Noun == store.NounTask {
			fallback = "their rendered order falls back to key order among themselves"
		}
		obligations := "epic completion requires its non-cancelled features and features inherit epic prerequisites"
		if len(cycle.ReviewEpics) > 0 {
			reviews := make([]string, 0, len(cycle.ReviewEpics))
			for _, id := range cycle.ReviewEpics {
				reviews = append(reviews, cycleMemberLabel(t, id))
			}
			obligations += "; epic review for " + strings.Join(reviews, ", ") +
				" completes riding features only after aggregate merge; submission requires all riding tasks submitted and independent task/feature-grain overrides done"
		}
		report.Gaps = append(report.Gaps, Gap{
			Reason:  ReasonDependencyCycle,
			Message: fmt.Sprintf("%s %s form a dependency cycle; %s; recorded depends_on: %s; %s; review the recorded prerequisites and break the execution loop", noun, strings.Join(labels, ", "), fallback, cycleEdges(t, cycle.Edges), obligations),
		})
	}
	for _, cycle := range classified.Advisory {
		labels := make([]string, 0, len(cycle.Members))
		for _, id := range cycle.Members {
			labels = append(labels, cycleMemberLabel(t, id))
		}
		report.Advisories = append(report.Advisories, Gap{
			Reason:  ReasonProjectedEpicOrderCycle,
			Message: fmt.Sprintf("epics %s have a projected grouping-only ordering cycle; features may interleave without an execution deadlock; grouped rendering follows their features' flow order with recorded soft-order preference; recorded depends_on: %s; keep valid prerequisites", strings.Join(labels, ", "), cycleEdges(t, cycle.Edges)),
		})
	}
	return report
}

func cycleMemberLabel(t *store.Tree, id string) string {
	if t.Epics[id] != nil {
		return memberLabel(t, store.NounEpic, id)
	}
	if t.Features[id] != nil {
		return memberLabel(t, store.NounFeature, id)
	}
	return memberLabel(t, store.NounTask, id)
}

func cycleEdges(t *store.Tree, edges []gen.DependencyEdge) string {
	labels := make([]string, 0, len(edges))
	for _, edge := range edges {
		labels = append(labels, cycleMemberLabel(t, edge.Source)+" depends_on "+cycleMemberLabel(t, edge.Target))
	}
	return strings.Join(labels, "; ")
}
