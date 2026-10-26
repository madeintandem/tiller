package views_test

// depcheck_test.go pins DependencyShape's one job: telling a tree where every
// dependency stays inside its own epic (dry run 3's shape, invisible to any
// prose scan) from a tree where dependencies actually cross epic boundaries.

import (
	"testing"

	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func TestDependencyShapeCountsCrossEpicEdges(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	// Two features, one per epic, with a cross-epic edge between them.
	tree.Features["F-aaaaa0"] = feat(t, "F-aaaaa0", "epic-0/a", "E-aaaaa0")
	tree.Features["F-aaaaa1"] = feat(t, "F-aaaaa1", "epic-1/b", "E-aaaaa1")
	tree.Features["F-aaaaa1"].DependsOn = []string{"F-aaaaa0"}

	got := views.DependencyShape(tree)
	require.Equal(t, 1, got.Edges)
	require.Equal(t, 1, got.CrossEpic)
	require.Equal(t, 0, got.EpicsWithNoCrossEpic)
}

func TestDependencyShapeFlagsATreeWhoseEdgesAllStayInsideTheirEpic(t *testing.T) {
	// This is dry run 3's shape: every edge intra-epic, nothing crossing, which
	// left every dependency-ordered surface under-ordering with no signal.
	tree := treeWithDraftEpics(t, 2)
	tree.Features["F-aaaaa0"] = feat(t, "F-aaaaa0", "epic-0/a", "E-aaaaa0")
	tree.Features["F-aaaaa2"] = feat(t, "F-aaaaa2", "epic-0/c", "E-aaaaa0")
	tree.Features["F-aaaaa2"].DependsOn = []string{"F-aaaaa0"}

	got := views.DependencyShape(tree)
	require.Equal(t, 1, got.Edges)
	require.Equal(t, 0, got.CrossEpic)
	require.Equal(t, 2, got.EpicsWithNoCrossEpic)
	require.Equal(t, 2, got.Epics)
}

func TestDependencyShapeCountsAnUnresolvableTargetAsAnEdgeNotACrossing(t *testing.T) {
	// depcheck.go's guard: a DependsOn entry naming an id absent from
	// t.Features still gets recorded (the dependency was written down), but
	// must not inflate CrossEpic, since an unresolved target's epic is
	// unknown rather than confirmed different. A
	// regression that miscounted a dangling edge as cross-epic would pass
	// every other test here, since none of them exercise a missing target.
	tree := treeWithDraftEpics(t, 1)
	tree.Features["F-aaaaa0"] = feat(t, "F-aaaaa0", "epic-0/a", "E-aaaaa0")
	tree.Features["F-aaaaa0"].DependsOn = []string{"F-missing"}

	got := views.DependencyShape(tree)
	require.Equal(t, 1, got.Edges)
	require.Equal(t, 0, got.CrossEpic)
}

func TestDependencyShapeCountsAnEpicTouchedOnlyAsATargetNotStarved(t *testing.T) {
	// epic-0 here is never a DependsOn source, only ever named as a target:
	// F-aaaaa0 carries no outbound edges of its own. DependencyShape's doc
	// comment calls out marking both ends of a crossing edge, not just the
	// depending side, as the deliberate choice that keeps a target-only epic
	// out of EpicsWithNoCrossEpic. An implementation that touched only the
	// source epic would count epic-0 as having no cross-epic edge (1, not 0)
	// even though another epic depends on it directly; this pins the 0.
	tree := treeWithDraftEpics(t, 2)
	tree.Features["F-aaaaa0"] = feat(t, "F-aaaaa0", "epic-0/a", "E-aaaaa0")
	tree.Features["F-aaaaa1"] = feat(t, "F-aaaaa1", "epic-1/b", "E-aaaaa1")
	tree.Features["F-aaaaa1"].DependsOn = []string{"F-aaaaa0"}

	got := views.DependencyShape(tree)
	require.Equal(t, 0, got.EpicsWithNoCrossEpic, "epic-0 is target-only and must still count as touched")
}
