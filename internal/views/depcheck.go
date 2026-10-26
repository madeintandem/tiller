package views

// depcheck.go counts recorded feature dependencies without interpreting prose.
// These diagnostics describe graph shape, not planning quality or completeness:
// independent epics may legitimately have no cross-epic edges.

import "github.com/madeintandem/tiller/internal/store"

// DepShape reports recorded feature edges, cross-epic edges, and epics untouched
// by a cross-epic edge. decomposeContent renders these neutral counts for review.
type DepShape struct {
	Edges                int
	CrossEpic            int
	EpicsWithNoCrossEpic int
	Epics                int
}

// DependencyShape walks every feature's DependsOn and compares the owning
// epic (Feature.EpicKey) of the edge's two ends. This is a feature-to-feature
// comparison only: it never resolves either end through t.Epics, because the
// two features already carry their own EpicKey, and requiring an extra
// lookup through the epic map would only give a dangling or renamed epic key
// a second way to hide a real cross-epic edge.
//
// An edge naming a target id absent from t.Features still counts toward
// Edges (the dependency was recorded) but never toward CrossEpic: its
// target's epic is unknown, not confirmed different, and counting it cross
// would manufacture a signal the tree never recorded.
//
// EpicsWithNoCrossEpic counts epics untouched by a cross-epic edge at either end.
// Both independent work and work with missing edges can have that shape; only
// an audit against the requirements can distinguish them.
func DependencyShape(t *store.Tree) DepShape {
	shape := DepShape{Epics: len(t.Epics)}
	touched := map[string]bool{}
	for _, f := range t.Features {
		for _, dep := range f.DependsOn {
			shape.Edges++
			target, ok := t.Features[dep]
			if !ok {
				continue
			}
			if f.EpicKey != target.EpicKey {
				shape.CrossEpic++
				touched[f.EpicKey] = true
				touched[target.EpicKey] = true
			}
		}
	}
	// Epics - len(touched) rather than a per-epic membership walk: touched is
	// keyed by the same raw EpicKey strings the loop above just compared, so
	// it never needs a second resolution through t.Epics either. The clamp
	// guards the pathological case of a dangling EpicKey naming no real epic
	// still landing in touched, which must not push the count negative.
	shape.EpicsWithNoCrossEpic = shape.Epics - len(touched)
	if shape.EpicsWithNoCrossEpic < 0 {
		shape.EpicsWithNoCrossEpic = 0
	}
	return shape
}
