package gen

// This file is the dependency-ordering authority (issue #27, plan
// 2026-07-23-console-chart-table-pass, Task A1): one computation, run once
// per loaded tree, that every read surface here and in views (tiller tree,
// ls, the console queue and streams) consults in place of the
// ad hoc alphabetical-by-key sort each used to run at its own call site, so
// dependency order cannot drift per surface. It lives in gen alongside Tree;
// views imports gen to share the same ordering.
//
// The algorithm is a Kahn topological order over the recorded depends_on graph,
// computed in layers over the graph's strongly-connected-component
// condensation: every round, every component with zero remaining (in-set)
// dependencies is emitted as one batch, and a component emits its members key-
// ascending. A tie within a layer, or a node with no recorded edges at all,
// breaks by canonical key ascending, so a dependency-free tree reads exactly as
// the old alphabetical list did, and for any acyclic input every component is a
// singleton and the output is identical to the plain layered Kahn this
// replaced. The full tie stack, run identically at epic, feature, and task
// grain, is: recorded depends_on edges first (hard, never violated), then the
// recorded soft order (listed ids precede unlisted ones; intent, not
// constraint, so it never gates), then, inside an epic cycle group only, the
// flow-derived rank described below, and finally canonical key ascending as the
// last resort (docs/plans/2026-08-08-soft-order-design.md, "The rule: one
// precedence stack, render and routing alike"). A cycle never errors here: its
// members emit as one batch at the layer the cycle's external dependencies
// allow, and anything depending into the cycle emits after it. Within the
// batch, an epic cycle's members emit by the minimum featureFlow index of their
// own features (an epic with no features last, key-ascending among such peers),
// since the feature graph stays acyclic and honest even where the epic lift
// cycles (docs/plans/2026-08-08-soft-order-design.md, phase 1); a feature or
// task cycle's members emit key-ascending, there being no finer grain to
// consult. The old rule sank a cycle, and everything downstream of it, into one
// key-ascending tail after every acyclic node, which let a dependent render
// ahead of the cycle it depends on (Acme rendered post-launch before the
// reporting epic it depends on); the condensation keeps every recorded edge
// honored in rendered order except the edges inside the cycle itself, where no
// honest order exists. Which nodes degraded is recorded rather than discarded:
// Cycles() names each degraded group, per grain, so readiness and the readouts
// can surface the fallback instead of keeping it silent (JC's routing batch,
// 2026-08-05; the cycles came from legal feature edges, not a modeling mistake,
// so silence was the bug).
//
// Order ranks epics, features, and tasks. Questions keep id order: the ledger
// noun records no depends_on, so it has no edge to rank. A tree-wide list of
// one noun asks Order.Less; callers holding this order use Order.Tasks for a
// feature's complete sibling group. OrderTasks runs the same layered sort for
// a standalone group or a true subset, whose dependencies outside that set
// must not constrain its order.
//
// Epic rank is computed over more than the recorded epic-to-epic edges,
// though: the per-epic layered Kahn pass only ever sees one epic's own
// siblings (an edge to a feature outside that set does not gate order
// there), and its epic pass only ever counts epic-to-epic ids, so any other
// recorded cross-epic edge carries no weight of its own unless something
// lifts it to epic grain first. addImpliedEpicEdges is that lift, covering
// every cross-epic form mutate/depends.go can record: a feature depending on
// another epic's feature, a feature depending on an epic directly, and an
// epic depending on another epic's feature. Each becomes an implied
// epic-depends-on-epic edge before the epic Kahn pass runs, so a dependent
// always renders after what it depends on, the same guarantee an explicit
// epic-level edge already gave. A recorded edge must never be silently
// violated in rendered order (#27 A1 fix
// review). Per-epic feature ordering itself is untouched by this: within-epic
// edges are still honored locally by the existing per-epic pass, and
// cross-epic edges are honored transitively through the epic rank they now
// feed rather than by widening any single epic's own local sort. If the
// implied edges plus the explicit epic edges form a cycle, the cycle rule
// above applies: the members batch at their layer, ranked by their features'
// flow order, and the group is named by Cycles(). A real tree manufactures
// these implied cycles from legal feature edges (Acme: 8 of 11 epics in one
// group while the feature grain stayed fully acyclic), which is why the
// feature-grain FeatureFlow order exists as the routing walk: it reads the
// recorded feature edges directly and owes nothing to the epic lift.

import (
	"sort"

	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
)

// Order is one tree's dependency order: the epic order, the feature order, and
// the task order, each computed once so every caller in a single read gets the
// same answer.
type Order struct {
	epics    []string
	epicRank map[string]int

	features       []string
	featureRank    map[string]int
	featuresByEpic map[string][]string

	taskRank       map[string]int
	tasksByFeature map[string][]*store.Task

	// featureFlow is the routing walk's order: one global layered order over
	// every feature and its recorded edges, ignoring epic grouping. The
	// epic-grouped features order above stays the RENDER order (tree, board,
	// the navigator all group by epic); the flow is what the frontier walks,
	// because the feature-grain graph is the recorded truth and stays honest
	// even when the implied epic graph cycles and epic rank degrades. Where
	// depends_on leaves the choice open, this pass's own tie is composite:
	// the owning epic's already-settled global rank first (an orphan feature
	// ranks after every ranked epic's own features), then, among two
	// features under the same epic, that epic's recorded soft order, then
	// key (docs/plans/2026-08-08-soft-order-design.md, "Sort integration").
	// It is a second pass over the same graph the base flow below computes;
	// the base flow's own plain tie is what epicFlowRank and Cycles() read,
	// kept separate so feature-level soft order cannot leak into the rank
	// that answers only what the recorded graph itself says.
	featureFlow []string

	// cycles records every degraded group Cycles() reports, in grain order
	// (epic groups first, then feature, then task), each group's members in
	// emitted order: flow-ranked for an epic group, key-ascending for a
	// feature or task group.
	cycles []CycleGroup
}

// CycleGroup names one degraded dependency group: same-noun nodes whose
// recorded (or, for epics, implied) depends_on edges form a cycle, so no
// honest order exists among them and their rendered order fell back to a
// lower tier. Reported member order is per grain, following the same
// tiebreak stack each grain's own pass runs on: an epic group ranks by the
// root soft order first, then by its own features' flow order
// (docs/plans/2026-08-08-soft-order-design.md, phase 1), then key ascending;
// a task group ranks by its owning feature's recorded soft order first, then
// key ascending, both from the tied pass OrderTasks runs; a feature group
// ranks by key ascending alone, because Cycles() reports the base flow
// pass's groups (the one pass that sees every feature edge at once, run with
// no tiebreak beneath key) rather than the later routing pass that layers
// feature-level soft order on top of the same graph. Members are ids in
// emitted order.
type CycleGroup struct {
	Noun    string
	Members []string
}

// DependencyOrder computes the dependency order once for a loaded tree.
// Callers that need to order siblings should hold on to the result for the
// life of the read rather than recomputing it per call site.
func DependencyOrder(t *store.Tree) *Order {
	return dependencyOrder(t, t)
}

// dependencyOrder orders t's subset while resolving legacy references against
// the complete snapshot, so filtering cannot turn ambiguous keys into edges.
func dependencyOrder(t, resolveAgainst *store.Tree) *Order {
	o := &Order{
		epicRank:       map[string]int{},
		featureRank:    map[string]int{},
		featuresByEpic: map[string][]string{},
		taskRank:       map[string]int{},
		tasksByFeature: map[string][]*store.Task{},
	}
	nodes := make([]store.Node, 0, len(t.Epics)+len(t.Features)+len(t.Tasks))
	for _, epic := range t.Epics {
		nodes = append(nodes, epic)
	}
	featureIDs := make([]string, 0, len(t.Features))
	for id := range t.Features {
		featureIDs = append(featureIDs, id)
	}
	sort.Strings(featureIDs)
	for _, id := range featureIDs {
		nodes = append(nodes, t.Features[id])
		// Preserve each group's input order: duplicate-id fixtures use the
		// first supplied assertion body, just as standalone OrderTasks does.
		for _, task := range t.FeatureTasks[id] {
			nodes = append(nodes, task)
		}
	}
	index := relationships.BuildDependencies(resolveAgainst, nodes)

	featureDeps := make(map[string][]string, len(t.Features))
	byEpicKey := map[string][]string{}
	for id, f := range t.Features {
		featureDeps[id] = dependencyTargets(index, id)
		byEpicKey[f.EpicKey] = append(byEpicKey[f.EpicKey], id)
	}
	featureKeyOf := func(id string) string { return keyStr(t.Features[id].Key) }

	// The base flow pass sees every feature and every recorded edge at once,
	// which the per-epic passes below cannot (their inSet filter drops
	// cross-epic targets), so it is also the one pass that can name a
	// feature-grain cycle completely; the scoped passes deliberately do not
	// report, or a within-epic cycle would be counted twice. It runs before
	// the epic pass because the epic cycle tiebreak below ranks a cycle
	// member by its own features' flow positions, so the flow must already
	// exist when that pass runs. Its own tie stays the plain (nil, nil)
	// default rather than any soft order: this is the rank epicFlowRank and
	// Cycles() read, and it must answer only what the recorded graph itself
	// says. The routing flow (o.featureFlow) is a second, later pass over
	// this same graph that layers the soft order stack on top.
	baseFlow, featureCycles := layeredTopoOrderTied(featureIDs, featureDeps, featureKeyOf, nil, nil)

	epicIDs := make([]string, 0, len(t.Epics))
	epicDeps := make(map[string][]string, len(t.Epics))
	epicIDByKey := make(map[string]string, len(t.Epics))
	for id, e := range t.Epics {
		epicIDs = append(epicIDs, id)
		// These slices belong to this ordering pass; implied edges never mutate
		// the relationship index or the loaded record's backing array.
		epicDeps[id] = dependencyTargets(index, id)
		if k := keyStr(e.Key); k != "" {
			epicIDByKey[k] = id
		}
	}
	addImpliedEpicEdges(t, epicDeps, featureDeps, epicIDByKey)

	flowIndex := make(map[string]int, len(baseFlow))
	for i, id := range baseFlow {
		flowIndex[id] = i
	}
	// epicFlowRank is the phase 1 rank: the earliest flow position any of the
	// epic's own features holds. ok is false for an epic with no features.
	epicFlowRank := func(eid string) (int, bool) {
		ek := keyStr(t.Epics[eid].Key)
		best, ok := 0, false
		for _, fid := range byEpicKey[ek] {
			if idx, in := flowIndex[fid]; in && (!ok || idx < best) {
				best, ok = idx, true
			}
		}
		return best, ok
	}
	epicKeyOf := func(id string) string { return keyStr(t.Epics[id].Key) }
	// flowThenKey is the tiebreak among an epic cycle's own members: flow
	// rank ascending where the graph gives one, key ascending among peers
	// with none, featureless peers always last. It plays no part in an
	// acyclic layer, where layeredTopoOrderTied's nil layer arg keeps the
	// existing lead-member keyTie.
	flowThenKey := func(a, b string) bool {
		ra, aok := epicFlowRank(a)
		rb, bok := epicFlowRank(b)
		if aok != bok {
			return aok // ranked members precede featureless ones
		}
		if aok && ra != rb {
			return ra < rb
		}
		return keyTie(epicKeyOf)(a, b)
	}
	// The epic pass's own ties carry the soft order stack: a listed epic
	// precedes an unlisted one at the layer grain, and inside a cycle batch a
	// listed epic still outranks the flow-derived rank
	// (docs/plans/2026-08-08-soft-order-design.md, "Sort integration").
	epicLayer := softTie(t.ProjectOrder.Epics, keyTie(epicKeyOf))
	epicCycleTie := softTie(t.ProjectOrder.Epics, flowThenKey)
	var epicCycles [][]string
	o.epics, epicCycles = layeredTopoOrderTied(epicIDs, epicDeps, epicKeyOf, epicLayer, epicCycleTie)
	for _, c := range epicCycles {
		o.cycles = append(o.cycles, CycleGroup{Noun: store.NounEpic, Members: c})
	}
	for i, id := range o.epics {
		o.epicRank[id] = i
	}

	for _, c := range featureCycles {
		o.cycles = append(o.cycles, CycleGroup{Noun: store.NounFeature, Members: c})
	}

	// The routing flow is a second pass over the same feature graph the base
	// flow above walked, its own tie composite: the owning epic's
	// now-settled global rank first (an orphan feature, whose EpicKey names
	// no ranked epic, ranks after every ranked epic's own features), then,
	// among two features under the same epic, that epic's recorded soft
	// order, then key (design doc, "Sort integration"). The graph and its
	// strongly-connected components are identical to the base flow pass, so
	// this pass finds the same cycle groups already reported from that pass;
	// reporting them again here would double the group, so the second
	// return is discarded.
	featureEpicRank := func(id string) (int, bool) {
		eid, ok := epicIDByKey[t.Features[id].EpicKey]
		if !ok {
			return 0, false
		}
		r, ok := o.epicRank[eid]
		return r, ok
	}
	routingTie := func(a, b string) bool {
		ra, aok := featureEpicRank(a)
		rb, bok := featureEpicRank(b)
		if aok != bok {
			return aok // features under a ranked epic precede orphans
		}
		if aok && ra != rb {
			return ra < rb
		}
		if aok {
			eid := epicIDByKey[t.Features[a].EpicKey]
			return softTie(t.Epics[eid].Order, keyTie(featureKeyOf))(a, b)
		}
		return keyTie(featureKeyOf)(a, b)
	}
	o.featureFlow, _ = layeredTopoOrderTied(featureIDs, featureDeps, featureKeyOf, routingTie, routingTie)

	// A feature's global order groups by its owning epic's rank first (so a
	// feature under an early epic always leads one under a later epic, even
	// when their canonical keys disagree, since a feature's key is prefixed by
	// its epic's key and so cannot be trusted to reflect epic rank on its
	// own), then by that epic's own local dependency order, itself tied by
	// that epic's recorded soft order ahead of key. A keyless epic claims no
	// features, the same guard childFeatures and decompositionOf already
	// apply.
	consumed := make(map[string]bool, len(byEpicKey))
	// featureTieFor is the per-epic (and orphan) local tie: that epic's own
	// soft order ahead of key, or plain key alone for an orphan group, since
	// no epic node exists there to carry an Order.
	featureTieFor := func(ek string) tieLess {
		var soft []string
		if eid, ok := epicIDByKey[ek]; ok {
			soft = t.Epics[eid].Order
		}
		return softTie(soft, keyTie(featureKeyOf))
	}
	for _, eid := range o.epics {
		ek := keyStr(t.Epics[eid].Key)
		if ek == "" || len(byEpicKey[ek]) == 0 {
			continue
		}
		consumed[ek] = true
		tie := featureTieFor(ek)
		local, _ := layeredTopoOrderTied(byEpicKey[ek], featureDeps, featureKeyOf, tie, tie)
		o.featuresByEpic[ek] = local
		o.features = append(o.features, local...)
	}
	// Any feature whose EpicKey named no epic Order just ranked (an orphan, or
	// a dangling layout link) still needs a deterministic place: its own group,
	// key-ascending across groups, appended after every epic-owned feature.
	orphanKeys := make([]string, 0, len(byEpicKey)-len(consumed))
	for ek := range byEpicKey {
		if !consumed[ek] {
			orphanKeys = append(orphanKeys, ek)
		}
	}
	sort.Strings(orphanKeys)
	for _, ek := range orphanKeys {
		tie := featureTieFor(ek)
		local, _ := layeredTopoOrderTied(byEpicKey[ek], featureDeps, featureKeyOf, tie, tie)
		o.featuresByEpic[ek] = local
		o.features = append(o.features, local...)
	}
	for i, id := range o.features {
		o.featureRank[id] = i
	}

	// A task's tree-wide rank is its owning feature's rank first, then its own
	// sibling rank within that feature, the same two-level rule a feature's
	// global rank already follows through its epic. A task key is prefixed by
	// its feature's key, so key order alone would group siblings correctly and
	// then order them alphabetically, which is exactly what depends_on
	// overrides (artifact-schema.md, Tasks: Ordering). Ranking walks the ranked
	// features rather than t.Tasks so the sibling sort runs per feature, the
	// scope a task's edges are recorded in; a task in t.Tasks that no feature
	// group holds is left unranked, and Less falls back to id for it the same
	// way it does for an unranked epic.
	rank := 0
	for _, fid := range o.features {
		ranked, taskCycles := orderTasksWithCycles(index, t.FeatureTasks[fid], t.Features[fid].Order)
		o.tasksByFeature[fid] = ranked
		for _, c := range taskCycles {
			o.cycles = append(o.cycles, CycleGroup{Noun: store.NounTask, Members: c})
		}
		for _, tk := range ranked {
			o.taskRank[tk.ID] = rank
			rank++
		}
	}

	return o
}

// RankedNoun reports whether Less answers for a noun from rank rather than
// from the id fallback. A mixed-noun list (the board's lenses, the console's
// streams, the queue's attestation group) asks this before reaching for Less,
// so the set of ranked nouns is named once here rather than spelled out at
// each of those call sites.
func RankedNoun(noun string) bool {
	switch noun {
	case store.NounEpic, store.NounFeature, store.NounTask:
		return true
	default:
		return false
	}
}

// OrderTasks ranks one feature's sibling tasks through the same layered Kahn
// computation Epics and Features rank on (cli-surface.md, issue #27;
// artifact-schema.md, Tasks: Ordering). Callers hand it the slice they already
// hold (a tree's FeatureTasks group, or a filtered subset of one) and get a
// new slice back; the input is not reordered in place. soft is the owning
// feature's recorded task order (Feature.Order): listed ids precede unlisted
// ones within a dependency layer, and a recorded depends_on edge still
// outranks it (docs/plans/2026-08-08-soft-order-design.md). nil is legal and
// falls straight through to key order, the same as an empty list.
// The complete tree resolves legacy keys before this slice limits ordering.
//
// A depends_on entry naming something outside the slice handed in is filtered
// by the layered Kahn pass's own inSet pass, so it gates nothing here. That covers
// the cross-feature task edge, which this model does not carry: mutate's
// resolveDependsOn refuses to record one. A filtered subset (one feature's
// proposed batch) is a caller narrowing its own set rather than an edge being
// dropped.
//
// Ranking is per id, but the result carries every supplied task. Same-id
// entries retain their input order for in-memory callers; store.Load rejects
// duplicate IDs in a persisted tree.
func OrderTasks(tree *store.Tree, tasks []*store.Task, soft []string) []*store.Task {
	nodes := make([]store.Node, len(tasks))
	for n, task := range tasks {
		nodes[n] = task
	}
	out, _ := orderTasksWithCycles(relationships.BuildDependencies(tree, nodes), tasks, soft)
	return out
}

// orderTasksWithCycles is OrderTasks plus the degraded groups the sort found,
// which DependencyOrder's task pass collects into Cycles(); the exported
// wrapper drops them, since a caller ranking one ad hoc slice has no Cycles()
// to feed.
func orderTasksWithCycles(index *relationships.Index, tasks []*store.Task, soft []string) ([]*store.Task, [][]string) {
	byID := make(map[string][]*store.Task, len(tasks))
	ids := make([]string, 0, len(tasks))
	deps := make(map[string][]string, len(tasks))
	for _, tk := range tasks {
		if _, known := byID[tk.ID]; !known {
			ids = append(ids, tk.ID)
			deps[tk.ID] = dependencyTargets(index, tk.ID)
		}
		byID[tk.ID] = append(byID[tk.ID], tk)
	}
	keyOf := func(id string) string { return keyStr(byID[id][0].Key) }
	tie := softTie(soft, keyTie(keyOf))
	ranked, cycles := layeredTopoOrderTied(ids, deps, keyOf, tie, tie)
	out := make([]*store.Task, 0, len(tasks))
	for _, id := range ranked {
		out = append(out, byID[id]...)
	}
	return out, cycles
}

// dependencyTargets consumes canonical shared facts; unresolved or ineligible
// endpoints remain inspectable in the index but cannot constrain sorting.
func dependencyTargets(index *relationships.Index, id string) []string {
	var targets []string
	for _, edgeID := range index.Forward[id] {
		edge := index.Edges[edgeID]
		if edge.Kind == "depends_on" && !index.Vertices[edge.To].Unresolved {
			targets = append(targets, edge.To)
		}
	}
	return targets
}

// Epics returns every epic id in dependency order: topological layer first (a
// layer's members have every recorded epic-to-epic dependency already
// emitted by an earlier layer), canonical key ascending within a layer, and
// a cycle's own members ranked by their features' flow order (an epic with
// no features last, key-ascending among such peers).
func (o *Order) Epics() []string {
	return append([]string(nil), o.epics...)
}

// Features returns epicKey's own features in dependency order, the same
// topological-layer-then-key rule Epics applies, scoped to one epic's
// children (the tree's Feature.EpicKey layout link). An epic key with no
// features, or one Order never saw, returns nil.
func (o *Order) Features(epicKey string) []string {
	if fs := o.featuresByEpic[epicKey]; len(fs) > 0 {
		return append([]string(nil), fs...)
	}
	return nil
}

// Tasks returns a feature's complete sibling group in dependency order.
// The returned slice is a copy; task records remain shared with the tree.
// Unknown features and features without tasks return nil.
func (o *Order) Tasks(featureID string) []*store.Task {
	return append([]*store.Task(nil), o.tasksByFeature[featureID]...)
}

// FeatureFlow returns every feature id in the routing walk's order: one
// global layered order over the recorded feature-grain edges, ignoring epic
// grouping. This is what the frontier walks; the epic-grouped Features order
// stays the render order. The two agree wherever the implied epic graph is
// acyclic; where it cycles, the flow is the one that still honors every
// recorded edge (the Acme finding: an implied-epic cycle from legal
// feature edges degraded epic rank to key order while the feature grain
// stayed fully acyclic). Where depends_on leaves the choice open, ties break
// by the owning epic's global rank, then that epic's own soft order, then
// key (docs/plans/2026-08-08-soft-order-design.md).
func (o *Order) FeatureFlow() []string {
	return append([]string(nil), o.featureFlow...)
}

// Cycles returns every degraded dependency group this order carries: epic
// groups first (recorded plus implied edges), then feature groups (from the
// flow pass, which is the one pass that sees every feature edge at once),
// then task groups per feature. Empty on a tree whose recorded graph is
// honest at every grain, which is what lets a surface stay quiet then.
func (o *Order) Cycles() []CycleGroup {
	out := make([]CycleGroup, 0, len(o.cycles))
	for _, c := range o.cycles {
		out = append(out, CycleGroup{Noun: c.Noun, Members: append([]string(nil), c.Members...)})
	}
	return out
}

// Less reports whether a sorts before b for noun, the shared comparator every
// sibling swap in gen and views uses in place of a bespoke sort. It answers
// from rank for store.NounEpic, store.NounFeature, and store.NounTask, the
// three nouns Order ranks; any other noun, or an id Order never ranked (a
// dangling reference, a fixture Order was never built from), falls back to
// comparing the ids themselves.
//
// What that guarantees, precisely: Less never panics, it is irreflexive and
// antisymmetric over every pair, and over a set whose ids Order all ranked it
// is a strict total order, which is the case every caller here is in, since
// each sorts a list drawn from the tree Order was built from. It is NOT
// transitive over a set that mixes ranked and unranked ids, because rank and
// id can then order two pairs against each other; a caller sorting such a set
// would be sorting a list Order was not built from, and sort.Slice has no
// defined result for it.
//
// A tree-wide list of one noun is what this is for; a caller holding one
// feature's own tasks uses Tasks instead. A list mixing
// nouns must settle grain before asking (views.grainOf, or in this package the
// noun word each board line leads with), for the same transitivity reason.
func (o *Order) Less(noun, aID, bID string) bool {
	switch noun {
	case store.NounEpic:
		if ra, aok := o.epicRank[aID]; aok {
			if rb, bok := o.epicRank[bID]; bok {
				return ra < rb
			}
		}
	case store.NounFeature:
		if ra, aok := o.featureRank[aID]; aok {
			if rb, bok := o.featureRank[bID]; bok {
				return ra < rb
			}
		}
	case store.NounTask:
		if ra, aok := o.taskRank[aID]; aok {
			if rb, bok := o.taskRank[bID]; bok {
				return ra < rb
			}
		}
	}
	return aID < bID
}

// addImpliedEpicEdges augments epicDeps in place with one implied edge per
// recorded cross-epic depends_on entry, in every form mutate/depends.go can
// record one (resolveDependsOn accepts an epic or a feature target for any
// linked epic or feature, so all three cross-epic shapes exist on disk):
//
//   - feature -> feature: F1 depends on F2 owned by a different epic, so
//     epic(F1) depends on epic(F2);
//   - feature -> epic: F depends on epic E directly, so epic(F) depends on E;
//   - epic -> feature: epic X depends on feature F, so X depends on epic(F).
//     X's recorded F- id itself stays in epicDeps too, where the epic Kahn
//     pass's inSet filter ignores it, exactly as before.
//
// epicIDByKey resolves a feature's EpicKey (the key its owning epic carries)
// to that epic's id, the same lookup childFeatures and decompositionOf
// already key their own grouping by. A feature whose EpicKey names no epic
// (an orphan), a dependency id that resolves to nothing (dangling), or an
// edge whose two ends land in the same epic (a within-epic feature edge,
// already honored by the per-epic pass, or an epic depending on its own
// feature, which must not become a self-edge that traps the epic in the
// cycle tail) contributes no implied edge. A pair of epics linked by more
// than one such edge, or already epic-level linked besides, simply adds more
// than one edge instance between the same two epics; the layered Kahn pass's
// remaining-count bookkeeping treats duplicate edges between the same pair
// as harmless, so no de-duplication is needed here.
func addImpliedEpicEdges(t *store.Tree, epicDeps, featureDeps map[string][]string, epicIDByKey map[string]string) {
	// epicOf lifts one depends_on target id to the epic that answers for it:
	// the epic itself for an E- id, the owning epic for a feature id, and ""
	// for anything dangling or orphaned.
	epicOf := func(dep string) string {
		if _, ok := t.Epics[dep]; ok {
			return dep
		}
		if f, ok := t.Features[dep]; ok {
			return epicIDByKey[f.EpicKey]
		}
		return ""
	}

	for id, f := range t.Features {
		e1 := epicIDByKey[f.EpicKey]
		if e1 == "" {
			continue
		}
		for _, dep := range featureDeps[id] {
			if e2 := epicOf(dep); e2 != "" && e2 != e1 {
				epicDeps[e1] = append(epicDeps[e1], e2)
			}
		}
	}
	for id := range t.Epics {
		for _, dep := range epicDeps[id] {
			target, ok := t.Features[dep]
			if !ok {
				continue
			}
			if e2 := epicIDByKey[target.EpicKey]; e2 != "" && e2 != id {
				epicDeps[id] = append(epicDeps[id], e2)
			}
		}
	}
}

// tieLess orders two ids where the recorded graph is silent: across
// components within a layer (by lead member) and among one multi-member
// component's members. nil means the canonical (keyOf, id) compare.
type tieLess func(a, b string) bool

// keyTie is the canonical last-resort tiebreak: key ascending, id ascending.
func keyTie(keyOf func(string) string) tieLess {
	return func(a, b string) bool {
		ka, kb := keyOf(a), keyOf(b)
		if ka != kb {
			return ka < kb
		}
		return a < b
	}
}

// softTie ranks listed ids by ordinal ahead of unlisted ids, falling back
// for exact peers. Intent, not constraint: it is a tiebreak below the
// dependency layers, never an edge.
func softTie(soft []string, fallback tieLess) tieLess {
	ord := make(map[string]int, len(soft))
	for i, id := range soft {
		ord[id] = i
	}
	return func(a, b string) bool {
		ra, aok := ord[a]
		rb, bok := ord[b]
		if aok != bok {
			return aok
		}
		if aok && ra != rb {
			return ra < rb
		}
		return fallback(a, b)
	}
}

// layeredTopoOrderTied computes a Kahn topological order over ids, with the
// two tiebreaks exposed: layer orders components within a layer by their
// lead member, cycle orders members within one multi-member component.
// Either nil falls back to keyTie(keyOf), the canonical (keyOf, id) compare,
// which is what keeps every acyclic input byte-identical to the plain walk
// when no caller-supplied tiebreak is given.
//
// Mechanics: Kosaraju condensation first (two DFS passes over the in-set
// edges), then the same layered Kahn as ever over the condensed DAG, which
// by construction cannot cycle, so every component is emitted and no tail
// exists. Each component's members are sorted by cycle first; the component's
// own position in its layer batch then compares its lead (first) member
// against the other batch members' leads, by layer. For a singleton, both
// default to the (keyOf, id) tiebreak the plain walk used, which is what
// keeps the output byte-identical for every acyclic input (the pre-existing
// order tests are the proof).
func layeredTopoOrderTied(ids []string, dependsOn map[string][]string, keyOf func(string) string, layer, cycle tieLess) ([]string, [][]string) {
	if layer == nil {
		layer = keyTie(keyOf)
	}
	if cycle == nil {
		cycle = keyTie(keyOf)
	}
	inSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		inSet[id] = true
	}

	// The in-set edge lists, both directions: adj follows the dependency
	// (id -> what it depends on), radj the dependents. Duplicate edge
	// instances are kept as instances; the remaining-count bookkeeping below
	// pairs each increment with exactly one decrement, so they stay harmless
	// exactly as in the plain walk.
	adj := make(map[string][]string, len(ids))
	radj := make(map[string][]string, len(ids))
	for _, id := range ids {
		for _, dep := range dependsOn[id] {
			if inSet[dep] {
				adj[id] = append(adj[id], dep)
				radj[dep] = append(radj[dep], id)
			}
		}
	}

	// Kosaraju pass 1: iterative DFS finish order over adj.
	visited := make(map[string]bool, len(ids))
	finish := make([]string, 0, len(ids))
	type frame struct {
		id   string
		next int
	}
	for _, start := range ids {
		if visited[start] {
			continue
		}
		visited[start] = true
		stack := []frame{{id: start}}
		for len(stack) > 0 {
			f := &stack[len(stack)-1]
			if f.next < len(adj[f.id]) {
				n := adj[f.id][f.next]
				f.next++
				if !visited[n] {
					visited[n] = true
					stack = append(stack, frame{id: n})
				}
				continue
			}
			finish = append(finish, f.id)
			stack = stack[:len(stack)-1]
		}
	}

	// Kosaraju pass 2: reverse finish order over radj assigns components.
	sccOf := make(map[string]int, len(ids))
	var members [][]string
	assigned := make(map[string]bool, len(ids))
	for i := len(finish) - 1; i >= 0; i-- {
		root := finish[i]
		if assigned[root] {
			continue
		}
		idx := len(members)
		assigned[root] = true
		comp := []string{}
		work := []string{root}
		for len(work) > 0 {
			n := work[len(work)-1]
			work = work[:len(work)-1]
			comp = append(comp, n)
			sccOf[n] = idx
			for _, m := range radj[n] {
				if !assigned[m] {
					assigned[m] = true
					work = append(work, m)
				}
			}
		}
		members = append(members, comp)
	}

	// A multi-member component sorts by cycle, the caller's degraded-order
	// tiebreak (flow-derived rank for epics, key for everything else). A
	// singleton sorts by keyTie explicitly rather than by cycle: a
	// one-member batch is always acyclic output, and this branch is what
	// keeps a caller's cycle tie provably unable to touch it.
	sortByKey := func(batch []string) {
		less := keyTie(keyOf)
		if len(batch) > 1 {
			less = cycle
		}
		sort.Slice(batch, func(i, j int) bool { return less(batch[i], batch[j]) })
	}
	for i := range members {
		sortByKey(members[i])
	}
	// A component's batch position is its lead member's, compared by layer:
	// for the default keyTie, exactly the (keyOf, id) tiebreak the plain walk
	// used.
	superLess := func(a, b int) bool {
		return layer(members[a][0], members[b][0])
	}

	// The layered Kahn, over components.
	remaining := make([]int, len(members))
	dependents := make([][]int, len(members))
	for _, id := range ids {
		for _, dep := range adj[id] {
			if a, b := sccOf[id], sccOf[dep]; a != b {
				remaining[a]++
				dependents[b] = append(dependents[b], a)
			}
		}
	}

	var frontier []int
	for i := range members {
		if remaining[i] == 0 {
			frontier = append(frontier, i)
		}
	}
	sort.Slice(frontier, func(i, j int) bool { return superLess(frontier[i], frontier[j]) })

	order := make([]string, 0, len(ids))
	var cycles [][]string
	for len(frontier) > 0 {
		var next []int
		for _, s := range frontier {
			order = append(order, members[s]...)
			if len(members[s]) > 1 {
				cycles = append(cycles, append([]string(nil), members[s]...))
			}
			for _, d := range dependents[s] {
				remaining[d]--
				if remaining[d] == 0 {
					next = append(next, d)
				}
			}
		}
		sort.Slice(next, func(i, j int) bool { return superLess(next[i], next[j]) })
		frontier = next
	}
	return order, cycles
}
