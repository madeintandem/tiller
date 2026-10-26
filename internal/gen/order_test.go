package gen

// Tests for the dependency-ordering authority (plan
// 2026-07-23-console-chart-table-pass, Task A1). DependencyOrder is the one
// place that walks recorded depends_on into a Kahn topological order; every
// later read surface (tiller tree, ls, the console queue and
// streams) is meant to consult it instead of re-deriving its own alphabetical
// sort, so these tests pin the algorithm itself: layered topological order
// with a key-ascending tiebreak, cycle degrade, and the epic/feature-only
// scope (tasks and questions never pass through this).

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// epic is a small constructor so a fixture reads as data, not boilerplate.
func epic(id, key string, dependsOn ...string) *store.Epic {
	k := key
	return &store.Epic{ID: id, Key: &k, Title: key, Status: "ready", DependsOn: dependsOn}
}

// feature is the feature-grain equivalent of epic.
func feature(id, key, epicKey string, dependsOn ...string) *store.Feature {
	k := key
	return &store.Feature{ID: id, Key: &k, EpicKey: epicKey, Title: key, Status: "draft", DependsOn: dependsOn}
}

// task is the task-grain equivalent of epic and feature: one sibling under a
// feature, carrying the recorded depends_on the sibling sort ranks on.
func task(id, key, featureID string, dependsOn ...string) *store.Task {
	k := key
	return &store.Task{ID: id, Key: &k, Title: key, Status: "proposed", FeatureID: featureID, DependsOn: dependsOn}
}

// taskIDs projects a ranked task slice onto its ids, so an ordering assertion
// reads as a sequence rather than a struct dump.
func taskIDs(tasks []*store.Task) []string {
	out := make([]string, 0, len(tasks))
	for _, tk := range tasks {
		out = append(out, tk.ID)
	}
	return out
}

func taskOrderTree(tasks []*store.Task) *store.Tree {
	tree := &store.Tree{Tasks: map[string]*store.Task{}, FeatureTasks: map[string][]*store.Task{}}
	for _, task := range tasks {
		tree.Tasks[task.ID] = task
		tree.FeatureTasks[task.FeatureID] = append(tree.FeatureTasks[task.FeatureID], task)
	}
	return tree
}

// TestOrderTasksRanksSiblingsByDependency is the sibling-task case of the same
// layered sort epics and features run (cli-surface.md, tasks
// un-exempted; artifact-schema.md, Tasks: Ordering). The fixture is chosen so
// key order and dependency order disagree: "render" depends on "schema" but
// sorts before it alphabetically, and "docs" carries no edge at all, so it
// must still land in key order among the layer-zero peers.
func TestOrderTasksRanksSiblingsByDependency(t *testing.T) {
	tasks := []*store.Task{
		task("T-render", "form-engine/loading/render", "F-loading", "T-schema"),
		task("T-schema", "form-engine/loading/schema", "F-loading"),
		task("T-docs", "form-engine/loading/docs", "F-loading"),
	}
	require.Equal(t, []string{"T-docs", "T-schema", "T-render"}, taskIDs(OrderTasks(taskOrderTree(tasks), tasks, nil)),
		"schema leads the task that depends on it, though render sorts first by key; docs, edge-free, keeps key order among the peers")
}

// TestOrderTasksCycleDegradesToKeyOrder is the task-grain case of the cycle
// rule epics and features hold (the condensation semantics, JC's routing
// batch): a cycle's members emit as one batch at the layer the cycle's
// external dependencies allow, key-ascending among themselves, instead of the
// old rule's sink-to-the-tail. This cycle has no external dependency at all,
// so it sits in layer zero and its min-key member (aaa) places the batch
// ahead of the edge-free zzz, exactly where an ordinary node named aaa would
// sort.
func TestOrderTasksCycleDegradesToKeyOrder(t *testing.T) {
	tasks := []*store.Task{
		task("T-aaa", "form-engine/loading/aaa", "F-loading", "T-bbb"),
		task("T-bbb", "form-engine/loading/bbb", "F-loading", "T-aaa"),
		task("T-zzz", "form-engine/loading/zzz", "F-loading"),
	}
	require.NotPanics(t, func() {
		require.Equal(t, []string{"T-aaa", "T-bbb", "T-zzz"}, taskIDs(OrderTasks(taskOrderTree(tasks), tasks, nil)),
			"the dependency-free cycle sits in layer zero at its min member key; zzz follows by key, never jumping a cycle it does not depend on")
	})
}

// TestOrderTasksDependentOfACycleFollowsIt pins the honesty the condensation
// upgrade buys: a task depending on a cycle member must emit AFTER the cycle.
// Under the old sink-to-the-tail rule all three landed in one key-ascending
// tail, which put aaa ahead of the very tasks it depends on, a silently
// violated recorded edge.
func TestOrderTasksDependentOfACycleFollowsIt(t *testing.T) {
	tasks := []*store.Task{
		task("T-aaa", "form-engine/loading/aaa", "F-loading", "T-bbb"),
		task("T-bbb", "form-engine/loading/bbb", "F-loading", "T-ccc"),
		task("T-ccc", "form-engine/loading/ccc", "F-loading", "T-bbb"),
	}
	require.Equal(t, []string{"T-bbb", "T-ccc", "T-aaa"}, taskIDs(OrderTasks(taskOrderTree(tasks), tasks, nil)),
		"the cycle emits first, key-ascending; the task that depends into it follows, despite sorting first by key")
}

// TestOrderTasksIgnoresOutOfSetDependsOn confirms an edge naming something the
// slice does not hold (a dangling id, or the cross-feature target
// mutate/depends.go refuses to record) does not hold its task back: the
// dependent still ranks by key among the others rather than sinking to the
// cycle tail.
func TestOrderTasksIgnoresOutOfSetDependsOn(t *testing.T) {
	tasks := []*store.Task{
		task("T-aaa", "form-engine/loading/aaa", "F-loading", "T-elsewhere"),
		task("T-bbb", "form-engine/loading/bbb", "F-loading"),
	}
	require.Equal(t, []string{"T-aaa", "T-bbb"}, taskIDs(OrderTasks(taskOrderTree(tasks), tasks, nil)))
}

// TestOrderTasksLeavesTheInputAlone pins that ranking is a read: a caller
// holding the tree's own FeatureTasks slice (the shape every surface passes)
// gets a new slice back and its own is untouched, so ordering one surface
// cannot reorder the store under another.
func TestOrderTasksLeavesTheInputAlone(t *testing.T) {
	in := []*store.Task{
		task("T-render", "form-engine/loading/render", "F-loading", "T-schema"),
		task("T-schema", "form-engine/loading/schema", "F-loading"),
	}
	_ = OrderTasks(taskOrderTree(in), in, nil)
	require.Equal(t, []string{"T-render", "T-schema"}, taskIDs(in))
}

// TestOrderTasksKeepsEveryTaskWhenTwoShareAnID pins the no-drop rule against
// the shape a bad merge leaves: two task folders whose task.yml claims the
// same id. Both are in the feature's group, so both must reach the rendered
// list, in the order the caller held them, rather than one being rendered
// twice and the other lost.
func TestOrderTasksKeepsEveryTaskWhenTwoShareAnID(t *testing.T) {
	clash := task("T-dup", "form-engine/loading/alfa", "F-loading")
	twin := task("T-dup", "form-engine/loading/zulu", "F-loading", "T-solo")
	other := task("T-solo", "form-engine/loading/mike", "F-loading")

	tasks := []*store.Task{clash, twin, other}
	tree := taskOrderTree(tasks)
	tree.Features = map[string]*store.Feature{"F-loading": feature("F-loading", "form-engine/loading", "form-engine")}
	got := OrderTasks(tree, tasks, nil)

	require.Len(t, got, 3, "no task is dropped")
	require.Same(t, clash, got[0])
	require.Same(t, twin, got[1], "the same-id pair keeps the caller's order")
	require.Same(t, other, got[2], "and ranks after, since alfa decides T-dup's key")
	require.True(t, DependencyOrder(tree).Less(store.NounTask, "T-dup", "T-solo"), "global ordering also keeps the first supplied dependency body")
	cached := DependencyOrder(tree).Tasks("F-loading")
	require.Len(t, cached, 3)
	require.Same(t, clash, cached[0])
	require.Same(t, twin, cached[1])
	require.Same(t, other, cached[2])
}

// tasksAcrossFeaturesTree lays two features under one epic, each with two
// tasks, and fights every fallback a half-pinned ordering test could pass on.
// rendering-modes depends on schema-engine, so the feature rank reverses the
// feature keys; within each feature "alpha" depends on "zed", so the sibling
// edge reverses the task keys; and each id is chosen so id-ascending order
// reverses the whole expected sequence too. The one order that satisfies the
// assertions below is feature rank then sibling rank:
//
//	want:  T-zzz9, T-zzz1, T-aaa9, T-aaa1
//	key:   T-aaa1, T-aaa9, T-zzz1, T-zzz9
//	id:    T-aaa1, T-aaa9, T-zzz1, T-zzz9
func tasksAcrossFeaturesTree() *store.Tree {
	early := []*store.Task{
		task("T-zzz9", "form-engine/schema-engine/zed", "F-schema"),
		task("T-zzz1", "form-engine/schema-engine/alpha", "F-schema", "T-zzz9"),
	}
	late := []*store.Task{
		task("T-aaa9", "form-engine/rendering-modes/zed", "F-render"),
		task("T-aaa1", "form-engine/rendering-modes/alpha", "F-render", "T-aaa9"),
	}
	return &store.Tree{
		Manifest: &store.Manifest{Project: store.ProjectBlock{Key: "acme"}},
		Epics: map[string]*store.Epic{
			"E-form-engine": epic("E-form-engine", "form-engine"),
		},
		Features: map[string]*store.Feature{
			"F-render": feature("F-render", "form-engine/rendering-modes", "form-engine", "F-schema"),
			"F-schema": feature("F-schema", "form-engine/schema-engine", "form-engine"),
		},
		Tasks: map[string]*store.Task{
			"T-zzz9": early[0], "T-zzz1": early[1],
			"T-aaa9": late[0], "T-aaa1": late[1],
		},
		FeatureTasks: map[string][]*store.Task{"F-schema": early, "F-render": late},
	}
}

// TestDependencyOrderLessRanksTasksByFeatureThenSibling pins the tree-wide
// comparator every whole-tree task list sorts through (ls, the board's lenses,
// the console's streams): a task under an earlier-ranked feature leads one
// under a later-ranked feature even though its key and its id both sort after,
// and within one feature the recorded sibling edge decides against both.
func TestDependencyOrderLessRanksTasksByFeatureThenSibling(t *testing.T) {
	order := DependencyOrder(tasksAcrossFeaturesTree())

	require.True(t, order.Less(store.NounTask, "T-zzz9", "T-aaa1"),
		"schema-engine's tasks lead rendering-modes', though rendering-modes sorts first by both key and id")
	require.False(t, order.Less(store.NounTask, "T-aaa1", "T-zzz9"))
	require.True(t, order.Less(store.NounTask, "T-zzz9", "T-zzz1"),
		"within a feature the recorded edge decides, though zed sorts after alpha by key and T-zzz9 after T-zzz1 by id")
	require.False(t, order.Less(store.NounTask, "T-zzz1", "T-zzz9"))
}

// TestDependencyOrderLessMatchesTaskOrder confirms Less agrees with the ranked
// sequence for every ordered pair, at task grain, the same property
// TestDependencyOrderLessMatchesEpicsOrder pins one grain up. Agreement with a
// sequence is the whole of what an in-place sort.Slice swap needs and is what
// makes the comparator a strict total order over this set: a relation that
// answers "is a before b in this list" is irreflexive, antisymmetric, and
// transitive by construction. Asserting those three directly would pin
// nothing, since any rank-based comparator has them; agreement is the part
// that can be wrong, and is wrong the moment the rank is missing and the id
// fallback answers instead.
func TestDependencyOrderLessMatchesTaskOrder(t *testing.T) {
	order := DependencyOrder(tasksAcrossFeaturesTree())
	// The ranked sequence: parser's feature leads by dependency, and within
	// each feature the recorded edge leads. Both fight the ids, so an id
	// fallback disagrees with this list at almost every pair.
	ranked := []string{"T-zzz9", "T-zzz1", "T-aaa9", "T-aaa1"}
	for i, a := range ranked {
		for j, b := range ranked {
			require.Equal(t, i < j, order.Less(store.NounTask, a, b),
				"Less(%s, %s) must agree with their positions %d and %d", a, b, i, j)
		}
	}
}

// acmeEpicsTree lays the real Acme shape (plan 2026-07-23, Task A1 steps):
// two roots, a four-wide first layer, then three single-epic layers. The keys
// within the first layer are already alphabetical (form-engine, migration,
// notifications, payments) so the fixture doubles as the tie-break pin, and
// audit-engine (alphabetically first among all nine keys) landing
// second-to-last is the fixture's proof that the order is not just
// alphabetical.
func acmeEpicsTree() *store.Tree {
	return &store.Tree{
		Manifest: &store.Manifest{Project: store.ProjectBlock{Key: "acme"}},
		Epics: map[string]*store.Epic{
			"E-foundation":    epic("E-foundation", "foundation"),
			"E-ops-security":  epic("E-ops-security", "ops-security"),
			"E-form-engine":   epic("E-form-engine", "form-engine", "E-foundation"),
			"E-migration":     epic("E-migration", "migration", "E-foundation", "E-ops-security"),
			"E-notifications": epic("E-notifications", "notifications", "E-ops-security"),
			"E-payments":      epic("E-payments", "payments", "E-foundation"),
			"E-lifecycle":     epic("E-lifecycle", "lifecycle", "E-form-engine", "E-migration"),
			"E-audit-engine":  epic("E-audit-engine", "audit-engine", "E-lifecycle"),
			"E-reporting":     epic("E-reporting", "reporting", "E-audit-engine"),
		},
		Features: map[string]*store.Feature{},
	}
}

// TestDependencyOrderEpicsAcmeShape pins the layered Kahn order over the real
// Acme epic graph: both roots first (key-ascending between them), then the
// four-wide layer (already alphabetical, so the tiebreak is visible only by
// elimination), then the three single-member layers in dependency sequence.
func TestDependencyOrderEpicsAcmeShape(t *testing.T) {
	order := DependencyOrder(acmeEpicsTree())

	require.Equal(t, []string{
		"E-foundation", "E-ops-security",
		"E-form-engine", "E-migration", "E-notifications", "E-payments",
		"E-lifecycle",
		"E-audit-engine",
		"E-reporting",
	}, order.Epics(), "foundation-first: dependencies before what rests on them, not alphabetical (audit-engine sorts first alphabetically but seventh here)")
}

// TestDependencyOrderEpicsNoEdgesIsKeyOrder confirms that with no recorded
// depends_on at all, every epic is in the same (zeroth) layer and the whole
// order degrades to plain key ascending, so a dependency-free tree reads
// exactly as it did before this authority existed.
func TestDependencyOrderEpicsNoEdgesIsKeyOrder(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-zzz": epic("E-zzz", "zzz-last"),
			"E-aaa": epic("E-aaa", "aaa-first"),
			"E-mmm": epic("E-mmm", "mmm-middle"),
		},
		Features: map[string]*store.Feature{},
	}
	order := DependencyOrder(tree)
	require.Equal(t, []string{"E-aaa", "E-mmm", "E-zzz"}, order.Epics())
}

// TestDependencyOrderEpicsCycleDegradesToKeyOrder confirms a cycle (a
// recorded-graph shape real trees produce; Acme manufactured one at epic
// grain from perfectly legal feature edges) never errors: alpha and beta
// depend on each other, so they emit as one batch at the layer their external
// dependencies allow (none here, so layer zero), key-ascending between
// themselves, with edge-free gamma beside them in the same layer sorted by
// its own key.
func TestDependencyOrderEpicsCycleDegradesToKeyOrder(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-beta":  epic("E-beta", "beta", "E-alpha"),
			"E-alpha": epic("E-alpha", "alpha", "E-beta"),
			"E-gamma": epic("E-gamma", "gamma"),
		},
		Features: map[string]*store.Feature{},
	}
	order := DependencyOrder(tree)
	require.Equal(t, []string{"E-alpha", "E-beta", "E-gamma"}, order.Epics(),
		"the dependency-free cycle batches at its min member key (alpha); gamma follows by key")
}

// TestDependencyOrderEpicDependentOfACycleFollowsIt pins the condensation
// win at epic grain against the shape Acme actually rendered wrong: an epic
// depending on a cycle member used to land in the same key-ascending tail as
// the cycle itself, so post-launch rendered BEFORE the reporting epic it
// depends on. The dependent must follow the cycle now.
func TestDependencyOrderEpicDependentOfACycleFollowsIt(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-post":   epic("E-post", "aaa-post-launch", "E-report"),
			"E-report": epic("E-report", "reporting", "E-audit"),
			"E-audit":  epic("E-audit", "audit-engine", "E-report"),
		},
		Features: map[string]*store.Feature{},
	}
	order := DependencyOrder(tree)
	require.Equal(t, []string{"E-audit", "E-report", "E-post"}, order.Epics(),
		"the audit<->reporting cycle emits first; the epic depending into it follows, despite sorting first by key")
}

// TestDependencyOrderIgnoresDanglingDependsOn confirms an epic depending on an
// id no longer in the tree (a dangling edge, the kind a delete or a stale
// fixture can leave behind) is not held back by it: the edge is simply not
// counted, the same graceful-degrade the tree's other reverse-edge lookups
// already practice.
func TestDependencyOrderIgnoresDanglingDependsOn(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-solo": epic("E-solo", "solo", "E-nonexistent"),
		},
		Features: map[string]*store.Feature{},
	}
	order := DependencyOrder(tree)
	require.Equal(t, []string{"E-solo"}, order.Epics())
}

// TestDependencyOrderFeaturesWithinEpicFollowDependencies pins the scoped view
// Features(epicKey) returns: form-engine's own features in their recorded
// dependency order, not key order. rendering-modes depends on schema-engine,
// but "rendering-modes" < "schema-engine" alphabetically, so a key sort would
// get this exactly backwards.
func TestDependencyOrderFeaturesWithinEpicFollowDependencies(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-form-engine": epic("E-form-engine", "form-engine"),
		},
		Features: map[string]*store.Feature{
			"F-rendering": feature("F-rendering", "form-engine/rendering-modes", "form-engine", "F-schema"),
			"F-schema":    feature("F-schema", "form-engine/schema-engine", "form-engine"),
		},
	}
	order := DependencyOrder(tree)
	require.Equal(t, []string{"F-schema", "F-rendering"}, order.Features("form-engine"))
}

// TestDependencyOrderFeaturesUnknownEpicKeyIsEmpty confirms an epic key with no
// features (a childless epic, or a key Order never saw) returns an empty
// scoped list rather than panicking.
func TestDependencyOrderFeaturesUnknownEpicKeyIsEmpty(t *testing.T) {
	order := DependencyOrder(acmeEpicsTree())
	require.Empty(t, order.Features("lifecycle"), "lifecycle carries no features in this fixture")
	require.Empty(t, order.Features("no-such-epic"))
}

// TestDependencyOrderFeaturesCycleDegradesToKeyOrder mirrors the epic cycle
// test at feature grain: two features depending on each other degrade to key
// order between themselves.
func TestDependencyOrderFeaturesCycleDegradesToKeyOrder(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{"E-widgets": epic("E-widgets", "widgets")},
		Features: map[string]*store.Feature{
			"F-second": feature("F-second", "widgets/second", "widgets", "F-first"),
			"F-first":  feature("F-first", "widgets/first", "widgets", "F-second"),
		},
	}
	order := DependencyOrder(tree)
	require.Equal(t, []string{"F-first", "F-second"}, order.Features("widgets"))
}

// TestDependencyOrderLessRanksFeaturesAcrossEpicsByEpicRank is the cross-epic
// proof queue.go's tree-wide feature groups (every specced feature, regardless
// of epic) depend on: Less compares a feature owned by an early-ranked epic
// against one owned by a late-ranked epic correctly even when their canonical
// keys disagree, because a feature's key is prefixed by its own epic's key
// (so plain alphabetical-by-feature-key order already happens to group by
// epic key, the very thing this test must NOT be fooled by). audit-engine
// sorts alphabetically before foundation (a < f) but depends (transitively) on
// foundation, so foundation's feature must still lead.
func TestDependencyOrderLessRanksFeaturesAcrossEpicsByEpicRank(t *testing.T) {
	tree := acmeEpicsTree()
	tree.Features["F-foundation-core"] = feature("F-foundation-core", "foundation/core", "foundation")
	tree.Features["F-audit-report"] = feature("F-audit-report", "audit-engine/report", "audit-engine")

	order := DependencyOrder(tree)
	require.True(t, order.Less(store.NounFeature, "F-foundation-core", "F-audit-report"),
		"foundation's feature leads audit-engine's despite audit-engine sorting alphabetically first")
	require.False(t, order.Less(store.NounFeature, "F-audit-report", "F-foundation-core"))
}

// TestDependencyOrderImpliedCrossEpicFeatureEdge is the reviewer's scenario
// from the #27 A1 fix review: no epic-level depends_on records any edge
// between alpha-epic and beta-epic, and alpha-epic sorts first alphabetically,
// but alpha-epic's own feature depends on beta-epic's feature. Before the fix,
// the layered Kahn pass's inSet filter drops any feature edge whose target
// sits outside the epic being ranked, so epic order saw no signal at all and fell
// back to key order: alpha-epic (and its dependent feature) would render
// before beta-epic (and the feature it depends on), silently violating a
// recorded edge. The fix derives an implied epic(dependent) depends-on
// epic(dependency) edge from every cross-epic feature edge before ranking
// epics, so beta-epic (the dependency) must now lead alpha-epic (the
// dependent), and Less must agree at feature grain the same way.
func TestDependencyOrderImpliedCrossEpicFeatureEdge(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-alpha": epic("E-alpha", "alpha-epic"),
			"E-beta":  epic("E-beta", "beta-epic"),
		},
		Features: map[string]*store.Feature{
			"F-dependent":  feature("F-dependent", "alpha-epic/needs-it", "alpha-epic", "F-dependency"),
			"F-dependency": feature("F-dependency", "beta-epic/provides-it", "beta-epic"),
		},
	}
	order := DependencyOrder(tree)

	require.Equal(t, []string{"E-beta", "E-alpha"}, order.Epics(),
		"beta-epic owns the recorded dependency; it must lead even though alpha-epic sorts first by key and no epic-level edge names either epic")
	require.True(t, order.Less(store.NounFeature, "F-dependency", "F-dependent"),
		"the dependency feature must rank before the dependent, agreeing with the epic order that now carries the implied edge")
	require.False(t, order.Less(store.NounFeature, "F-dependent", "F-dependency"))
}

// TestDependencyOrderImpliedCrossEpicCycleDegradesToKeyOrder confirms that
// when two epics' features depend on each other across the epic boundary in
// both directions (alpha's feature depends on beta's, and beta's depends on
// alpha's, with no epic-level depends_on at all), the implied edges this
// creates form a cycle between the two epics exactly like an explicit
// epic-level cycle, and the existing cycle rule applies unchanged: no panic,
// no error, both epics degrade to key order between themselves.
func TestDependencyOrderImpliedCrossEpicCycleDegradesToKeyOrder(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-alpha": epic("E-alpha", "alpha-epic"),
			"E-beta":  epic("E-beta", "beta-epic"),
		},
		Features: map[string]*store.Feature{
			"F-a": feature("F-a", "alpha-epic/a", "alpha-epic", "F-b"),
			"F-b": feature("F-b", "beta-epic/b", "beta-epic", "F-a"),
		},
	}

	require.NotPanics(t, func() {
		order := DependencyOrder(tree)
		require.Equal(t, []string{"E-alpha", "E-beta"}, order.Epics(),
			"the implied alpha<->beta edges form a cycle; both degrade to key order (alpha-epic < beta-epic) rather than panicking or erroring")
	})
}

// acmeShapeTree is the fixture the routing batch was built from: two epics
// whose FEATURES form a perfectly acyclic dependency graph, while the
// cross-epic edges among them lift to an implied epic-grain cycle
// (alpha's a1 needs beta's b1, beta's b2 needs alpha's a2). This is the
// Acme finding in miniature: 8 of 11 real epics degraded to key order from
// legal feature edges, so the honest order exists only at feature grain.
func acmeShapeTree() *store.Tree {
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-alpha": epic("E-alpha", "alpha"),
			"E-beta":  epic("E-beta", "beta"),
		},
		Features: map[string]*store.Feature{
			"F-a1": feature("F-a1", "alpha/needs-beta", "alpha", "F-b1"),
			"F-a2": feature("F-a2", "alpha/base", "alpha"),
			"F-b1": feature("F-b1", "beta/base", "beta"),
			"F-b2": feature("F-b2", "beta/needs-alpha", "beta", "F-a2"),
		},
	}
}

// TestFeatureFlowStaysHonestWhenEpicRankDegrades pins the routing order the
// frontier walks: one global layered order over the feature-grain graph,
// which stays acyclic and honest even while the implied epic graph cycles
// and the epic-grouped render order degrades to key order. The grouped
// order here reads alpha's a1 BEFORE the beta feature it depends on (the
// per-epic pass cannot see the cross-epic edge); the flow order must not.
func TestFeatureFlowStaysHonestWhenEpicRankDegrades(t *testing.T) {
	order := DependencyOrder(acmeShapeTree())

	require.Equal(t, []string{"E-alpha", "E-beta"}, order.Epics(),
		"the implied epic cycle degrades epic rank to key order")
	require.Equal(t, []string{"F-a2", "F-b1", "F-a1", "F-b2"}, order.FeatureFlow(),
		"the flow interleaves epics honestly: both bases first (key-ascending), then the two dependents")
}

// TestCyclesNamesTheDegradedGroups pins fix 3's data source: Order records
// which nodes degraded, per grain, in emitted order, so readiness and the
// readouts can say so instead of the silence the old rule kept.
func TestCyclesNamesTheDegradedGroups(t *testing.T) {
	cycles := DependencyOrder(acmeShapeTree()).Cycles()

	require.Len(t, cycles, 1, "one degraded group: the implied epic cycle; the feature grain is acyclic and contributes none")
	require.Equal(t, store.NounEpic, cycles[0].Noun)
	require.Equal(t, []string{"E-alpha", "E-beta"}, cycles[0].Members)
}

// TestCyclesIsEmptyOnAnAcyclicTree pins the quiet side: a clean tree reports
// no groups at all, which is what lets the readouts render the degradation
// line only when there is something to say.
func TestCyclesIsEmptyOnAnAcyclicTree(t *testing.T) {
	require.Empty(t, DependencyOrder(acmeEpicsTree()).Cycles())
}

// TestCyclesNamesAFeatureGrainGroup covers the grain the flow pass owns: a
// within-epic feature cycle is reported as a feature group, once, from the
// flow pass rather than duplicated per epic-scoped pass.
func TestCyclesNamesAFeatureGrainGroup(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{"E-widgets": epic("E-widgets", "widgets")},
		Features: map[string]*store.Feature{
			"F-second": feature("F-second", "widgets/second", "widgets", "F-first"),
			"F-first":  feature("F-first", "widgets/first", "widgets", "F-second"),
		},
	}
	cycles := DependencyOrder(tree).Cycles()

	require.Len(t, cycles, 1)
	require.Equal(t, store.NounFeature, cycles[0].Noun)
	require.Equal(t, []string{"F-first", "F-second"}, cycles[0].Members)
}

// TestDependencyOrderImpliedFeatureDependsOnEpicEdge covers the second
// recorded form a cross-epic edge can take (fix loop 2): a feature's
// depends_on naming an EPIC id directly, which mutate/depends.go's
// resolveDependsOn allows (an E- target resolves for any linked node). Here
// alpha-epic's feature depends on E-beta itself, no epic-level edges exist,
// and alpha-epic sorts first by key, so before the extension the edge carried
// no weight anywhere: it is not a feature id (the feature-to-feature lift
// skipped it) and the per-epic pass's inSet filter dropped it. The dependency
// epic must lead regardless of the key tiebreak.
func TestDependencyOrderImpliedFeatureDependsOnEpicEdge(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-alpha": epic("E-alpha", "alpha-epic"),
			"E-beta":  epic("E-beta", "beta-epic"),
		},
		Features: map[string]*store.Feature{
			"F-dependent": feature("F-dependent", "alpha-epic/needs-beta", "alpha-epic", "E-beta"),
		},
	}
	order := DependencyOrder(tree)

	require.Equal(t, []string{"E-beta", "E-alpha"}, order.Epics(),
		"alpha-epic's feature depends on E-beta directly; the implied edge must outrank alpha-epic's earlier key")
	require.True(t, order.Less(store.NounEpic, "E-beta", "E-alpha"))
}

// TestDependencyOrderImpliedEpicDependsOnFeatureEdge covers the reverse
// recorded form (fix loop 2): an EPIC's depends_on naming a FEATURE id, which
// resolveDependsOn also allows (an F- target resolves for a linked epic).
// alpha-epic itself depends on beta-epic's feature, no other edges, alpha
// sorting first by key. Before the extension the epic Kahn pass's inSet
// filter silently dropped the F- id, so key order won; the feature's owning
// epic must lead instead.
func TestDependencyOrderImpliedEpicDependsOnFeatureEdge(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-alpha": epic("E-alpha", "alpha-epic", "F-provider"),
			"E-beta":  epic("E-beta", "beta-epic"),
		},
		Features: map[string]*store.Feature{
			"F-provider": feature("F-provider", "beta-epic/provides-it", "beta-epic"),
		},
	}
	order := DependencyOrder(tree)

	require.Equal(t, []string{"E-beta", "E-alpha"}, order.Epics(),
		"alpha-epic depends on beta-epic's feature; the implied edge lifts to beta-epic and must outrank alpha-epic's earlier key")
	require.True(t, order.Less(store.NounEpic, "E-beta", "E-alpha"))
}

// TestDependencyOrderEpicDependsOnOwnFeatureIsNotACycle guards the lift
// against the self-edge hazard: an epic depending on one of its own features
// (a natural record: the epic is gated on its own child, and nothing in
// mutate refuses it since only node-depends-on-itself is a self-dependency)
// must NOT produce an implied epic-depends-on-itself edge, which would trap
// the epic in the never-reaches-zero cycle path and demote it below every
// acyclic epic for no reason. The order must stay plain key order.
func TestDependencyOrderEpicDependsOnOwnFeatureIsNotACycle(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-alpha": epic("E-alpha", "alpha-epic", "F-own"),
			"E-beta":  epic("E-beta", "beta-epic"),
		},
		Features: map[string]*store.Feature{
			"F-own": feature("F-own", "alpha-epic/own-child", "alpha-epic"),
		},
	}
	order := DependencyOrder(tree)

	require.Equal(t, []string{"E-alpha", "E-beta"}, order.Epics(),
		"depending on your own feature is not a cross-epic edge; alpha-epic must keep its key-order place, not degrade to the cycle tail")
}

// TestDependencyOrderLessMatchesEpicsOrder confirms Less agrees with Epics for
// every pair, the property every in-place comparator swap relies on.
func TestDependencyOrderLessMatchesEpicsOrder(t *testing.T) {
	tree := acmeEpicsTree()
	order := DependencyOrder(tree)
	ids := order.Epics()
	for i := range ids {
		for j := range ids {
			want := i < j
			got := order.Less(store.NounEpic, ids[i], ids[j])
			require.Equal(t, want, got, "Less(%s, %s) should agree with Epics() position", ids[i], ids[j])
		}
	}
}

// TestDependencyOrderLessFallsBackForUnrankedNoun confirms Less never panics
// and always returns a total order for the noun it does not rank (question:
// the ledger item records no depends_on) or an id it never saw, a task id
// included: it falls back to plain id comparison.
func TestDependencyOrderLessFallsBackForUnrankedNoun(t *testing.T) {
	order := DependencyOrder(acmeEpicsTree())
	require.True(t, order.Less(store.NounQuestion, "Q-aaaaaa", "Q-bbbbbb"))
	require.False(t, order.Less(store.NounQuestion, "Q-bbbbbb", "Q-aaaaaa"))
	require.True(t, order.Less(store.NounTask, "T-aaaaaa", "T-bbbbbb"), "no task in this fixture is ranked")
	require.False(t, order.Less(store.NounEpic, "E-not-in-tree-a", "E-not-in-tree-a"))
}

// TestEpicCycleMembersRenderByFeatureFlow pins the phase 1 rule of the soft
// order design (docs/spec/cli-surface.md): a multi-member
// epic cycle group emits its members by the minimum featureFlow index of
// their own features, not by key, because the feature graph stays acyclic
// and honest when the epic lift cycles (the Acme field case: audit-engine
// led form-engine alphabetically while every recorded edge said otherwise).
func TestEpicCycleMembersRenderByFeatureFlow(t *testing.T) {
	// Feature chain: form/f1 -> lifecycle/l1 -> audit/a1 -> form/f2.
	// Implied epic edges: lifecycle->form, audit->lifecycle, form->audit,
	// one three-member cycle. Key order is audit, form, lifecycle; flow
	// order of first features is form (0), lifecycle (1), audit (2).
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-audit": epic("E-audit", "audit-engine"),
			"E-form":  epic("E-form", "form-engine"),
			"E-life":  epic("E-life", "lifecycle"),
		},
		Features: map[string]*store.Feature{
			"F-f1": feature("F-f1", "form-engine/definition", "form-engine"),
			"F-l1": feature("F-l1", "lifecycle/workflow", "lifecycle", "F-f1"),
			"F-a1": feature("F-a1", "audit-engine/selection", "audit-engine", "F-l1"),
			"F-f2": feature("F-f2", "form-engine/pdf", "form-engine", "F-a1"),
		},
	}
	o := DependencyOrder(tree)
	require.Equal(t, []string{"E-form", "E-life", "E-audit"}, o.Epics(),
		"cycle members emit by their features' flow positions, not key order")
	require.Len(t, o.Cycles(), 1)
	require.Equal(t, []string{"E-form", "E-life", "E-audit"}, o.Cycles()[0].Members,
		"Cycles() reports members in emitted order, which is now flow-derived")
}

// TestEpicCycleFeaturelessMemberEmitsLast: an epic with no features has no
// flow position; it emits after every ranked member, key-ascending among
// featureless peers.
func TestEpicCycleFeaturelessMemberEmitsLast(t *testing.T) {
	// bare-epic joins the cycle via an explicit epic edge in each direction.
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-form": epic("E-form", "form-engine", "E-bare"),
			"E-bare": epic("E-bare", "aaa-bare", "E-form"),
		},
		Features: map[string]*store.Feature{
			"F-f1": feature("F-f1", "form-engine/definition", "form-engine"),
		},
	}
	o := DependencyOrder(tree)
	require.Equal(t, []string{"E-form", "E-bare"}, o.Epics(),
		"a featureless cycle member sorts after flow-ranked members despite leading by key")
}

// TestEpicCycleRankIgnoresStatus: the flow rank uses all features, done
// included, so the render does not reshuffle as work completes.
func TestEpicCycleRankIgnoresStatus(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-audit": epic("E-audit", "audit-engine"),
			"E-form":  epic("E-form", "form-engine"),
			"E-life":  epic("E-life", "lifecycle"),
		},
		Features: map[string]*store.Feature{
			"F-f1": feature("F-f1", "form-engine/definition", "form-engine"),
			"F-l1": feature("F-l1", "lifecycle/workflow", "lifecycle", "F-f1"),
			"F-a1": feature("F-a1", "audit-engine/selection", "audit-engine", "F-l1"),
			"F-f2": feature("F-f2", "form-engine/pdf", "form-engine", "F-a1"),
		},
	}
	tree.Features["F-f1"].Status = "done"
	o := DependencyOrder(tree)
	require.Equal(t, []string{"E-form", "E-life", "E-audit"}, o.Epics())
}

// TestDependencyOrderIsDeterministic confirms two computations over the same
// tree agree byte for byte, the property every generated view in this package
// already holds (TestBoardIsDeterministic, TestTreeDeterministic): map
// iteration backs the input, so any leak of that randomness would make this
// flaky.
func TestDependencyOrderIsDeterministic(t *testing.T) {
	tree := acmeEpicsTree()
	a := DependencyOrder(tree)
	b := DependencyOrder(tree)
	require.Equal(t, a.Epics(), b.Epics())
	require.Equal(t, a.Features("form-engine"), b.Features("form-engine"))
}

// The tests below pin the soft order stack (docs/spec/cli-surface.md): recorded
// depends_on first (never violated), then the recorded soft order (listed ids
// precede unlisted, informing render and routing without ever gating), then,
// inside an epic cycle group only, the flow-derived rank phase 1 established,
// and finally key. Every test drives DependencyOrder or OrderTasks, the public
// surface, never softTie directly.

// TestSoftOrderBreaksEpicLayerTies: two independent epics; soft order
// inverts their key order.
func TestSoftOrderBreaksEpicLayerTies(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-a": epic("E-a", "alpha"),
			"E-z": epic("E-z", "zulu"),
		},
		ProjectOrder: store.ProjectOrder{Epics: []string{"E-z", "E-a"}},
	}
	require.Equal(t, []string{"E-z", "E-a"}, DependencyOrder(tree).Epics())
}

// TestDependsOnOutranksSoftOrder: soft order asking for the dependent first
// changes nothing; the recorded edge wins.
func TestDependsOnOutranksSoftOrder(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-a": epic("E-a", "alpha", "E-z"), // alpha depends on zulu
			"E-z": epic("E-z", "zulu"),
		},
		ProjectOrder: store.ProjectOrder{Epics: []string{"E-a", "E-z"}},
	}
	require.Equal(t, []string{"E-z", "E-a"}, DependencyOrder(tree).Epics())
}

// TestListedSiblingsPrecedeUnlisted: a partial list leads its batch; the
// unlisted keep key order among themselves. Three features under one epic,
// no edges among them; the epic's recorded Order names only the middle one.
func TestListedSiblingsPrecedeUnlisted(t *testing.T) {
	e := epic("E-only", "only")
	e.Order = []string{"F-mid"}
	tree := &store.Tree{
		Epics: map[string]*store.Epic{"E-only": e},
		Features: map[string]*store.Feature{
			"F-alpha": feature("F-alpha", "only/alpha", "only"),
			"F-mid":   feature("F-mid", "only/mid", "only"),
			"F-zed":   feature("F-zed", "only/zed", "only"),
		},
	}
	order := DependencyOrder(tree)
	require.Equal(t, []string{"F-mid", "F-alpha", "F-zed"}, order.Features("only"),
		"F-mid leads because it is listed; alpha and zed, both unlisted, keep key order between themselves")
}

// TestSoftOrderRanksCycleMembersAboveFlow: inside an epic cycle group the
// soft ordinal outranks the phase 1 flow rank. Reuses the Acme-shaped
// three-member cycle fixture (TestEpicCycleMembersRenderByFeatureFlow), whose
// flow order ranks E-form, E-life, E-audit; listing E-audit in the recorded
// soft order must move it to the front of the batch regardless.
func TestSoftOrderRanksCycleMembersAboveFlow(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-audit": epic("E-audit", "audit-engine"),
			"E-form":  epic("E-form", "form-engine"),
			"E-life":  epic("E-life", "lifecycle"),
		},
		Features: map[string]*store.Feature{
			"F-f1": feature("F-f1", "form-engine/definition", "form-engine"),
			"F-l1": feature("F-l1", "lifecycle/workflow", "lifecycle", "F-f1"),
			"F-a1": feature("F-a1", "audit-engine/selection", "audit-engine", "F-l1"),
			"F-f2": feature("F-f2", "form-engine/pdf", "form-engine", "F-a1"),
		},
		ProjectOrder: store.ProjectOrder{Epics: []string{"E-audit"}},
	}
	order := DependencyOrder(tree)
	require.Equal(t, []string{"E-audit", "E-form", "E-life"}, order.Epics(),
		"E-audit is listed in the soft order, so it leads the cycle batch though its own features flow last; form then life keep the phase 1 flow rank between themselves")
}

// TestFeatureFlowFollowsEpicRankThenSoftOrder: the routing walk's layer
// tiebreak is owning epic's rank, then the feature's soft ordinal, then key.
// Two epics, each with two independent (edge-free) draft features: the root
// soft order inverts the epics' key order, and alpha's own Order inverts its
// two features, while zulu records none and keeps key order among its own.
func TestFeatureFlowFollowsEpicRankThenSoftOrder(t *testing.T) {
	ea := epic("E-a", "alpha")
	ea.Order = []string{"F-a2", "F-a1"}
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-a": ea,
			"E-z": epic("E-z", "zulu"),
		},
		Features: map[string]*store.Feature{
			"F-a1": feature("F-a1", "alpha/one", "alpha"),
			"F-a2": feature("F-a2", "alpha/two", "alpha"),
			"F-z1": feature("F-z1", "zulu/one", "zulu"),
			"F-z2": feature("F-z2", "zulu/two", "zulu"),
		},
		ProjectOrder: store.ProjectOrder{Epics: []string{"E-z", "E-a"}},
	}
	order := DependencyOrder(tree)
	require.Equal(t, []string{"F-z1", "F-z2", "F-a2", "F-a1"}, order.FeatureFlow(),
		"zulu's features lead alpha's because the root soft order ranks E-z first; "+
			"within zulu key order stands (no Order recorded), within alpha the recorded Order inverts F-a1 and F-a2")
}

// TestOrderTasksHonorsSoftList: OrderTasks with a soft list ranks listed
// tasks first within a dependency layer; a dependency still outranks it.
func TestOrderTasksHonorsSoftList(t *testing.T) {
	tasks := []*store.Task{
		task("T-alpha", "form-engine/loading/alpha", "F-loading"),
		task("T-mid", "form-engine/loading/mid", "F-loading"),
		task("T-zed", "form-engine/loading/zed", "F-loading", "T-mid"),
	}
	require.Equal(t, []string{"T-alpha", "T-mid", "T-zed"},
		taskIDs(OrderTasks(taskOrderTree(tasks), tasks, []string{"T-zed", "T-alpha"})),
		"T-zed leads the soft list but depends on T-mid, so the recorded edge still places it last; "+
			"T-alpha, listed second, still leads the unlisted T-mid within the zero layer")
}

func TestLegacyDependenciesMatchIDsAcrossOrderingAndFeatureFlow(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-alpha": epic("E-alpha", "alpha"),
			"E-zulu":  epic("E-zulu", "zulu"),
		},
		Features: map[string]*store.Feature{
			"F-alpha": feature("F-alpha", "alpha/work", "alpha", "zulu/work"),
			"F-zulu":  feature("F-zulu", "zulu/work", "zulu"),
		},
		ProjectOrder: store.ProjectOrder{Epics: []string{"E-alpha", "E-zulu"}},
	}
	legacy := DependencyOrder(tree)
	require.Equal(t, []string{"E-zulu", "E-alpha"}, legacy.Epics())
	require.Equal(t, []string{"F-zulu", "F-alpha"}, legacy.FeatureFlow())
	tree.Features["F-alpha"].DependsOn = []string{"F-zulu"}
	ids := DependencyOrder(tree)
	require.Equal(t, ids.Epics(), legacy.Epics())
	require.Equal(t, ids.FeatureFlow(), legacy.FeatureFlow())
}

func TestLegacyTaskDependenciesRespectSoftOrderAndSubsets(t *testing.T) {
	alpha := task("T-alpha", "app/work/alpha", "F-work", "app/work/zulu")
	zulu := task("T-zulu", "app/work/zulu", "F-work")
	mid := task("T-mid", "app/work/mid", "F-work")
	tasks := []*store.Task{alpha, zulu, mid}
	tree := taskOrderTree(tasks)
	f := feature("F-work", "app/work", "app")
	f.Order = []string{"T-alpha"}
	tree.Features = map[string]*store.Feature{f.ID: f}
	order := DependencyOrder(tree)
	require.Equal(t, []string{"T-mid", "T-zulu", "T-alpha"}, taskIDs(order.Tasks(f.ID)))
	copied := order.Tasks(f.ID)
	copied[0] = alpha
	require.Equal(t, []string{"T-mid", "T-zulu", "T-alpha"}, taskIDs(order.Tasks(f.ID)))
	require.Equal(t, []string{"T-alpha", "T-zulu", "T-mid"}, taskIDs(tree.FeatureTasks[f.ID]))
	require.Nil(t, order.Tasks("F-absent"))
	require.Equal(t, []string{"T-mid", "T-zulu", "T-alpha"}, taskIDs(OrderTasks(tree, tasks, []string{"T-alpha"})))
	require.Equal(t, []string{"T-alpha", "T-mid"}, taskIDs(OrderTasks(tree, []*store.Task{alpha, mid}, []string{"T-alpha"})))
	alpha.DependsOn = []string{"T-zulu"}
	require.Equal(t, []string{"T-mid", "T-zulu", "T-alpha"}, taskIDs(OrderTasks(tree, tasks, []string{"T-alpha"})))
	require.Equal(t, taskIDs(order.Tasks(f.ID)), taskIDs(DependencyOrder(tree).Tasks(f.ID)))
}

func TestAmbiguousLegacyDependencyDoesNotChooseAnOrderingTarget(t *testing.T) {
	tree := &store.Tree{Features: map[string]*store.Feature{
		"F-alpha": feature("F-alpha", "app/alpha", "app", "app/zulu"),
		"F-one":   feature("F-one", "app/zulu", "app"),
		"F-two":   feature("F-two", "app/zulu", "app"),
	}}
	require.Equal(t, []string{"F-alpha", "F-one", "F-two"}, DependencyOrder(tree).FeatureFlow())
}

func TestTaskSubsetCannotDisambiguateLegacyDependency(t *testing.T) {
	alpha := task("T-alpha", "app/work/alpha", "F-work", "app/work/zulu")
	one := task("T-one", "app/work/zulu", "F-work")
	two := task("T-two", "app/work/zulu", "F-work")
	tree := taskOrderTree([]*store.Task{alpha, one, two})
	tree.Features = map[string]*store.Feature{"F-work": feature("F-work", "app/work", "app")}
	require.Equal(t, []string{"T-alpha", "T-one", "T-two"}, taskIDs(DependencyOrder(tree).Tasks("F-work")))
	require.Equal(t, []string{"T-alpha", "T-one"}, taskIDs(OrderTasks(tree, []*store.Task{alpha, one}, nil)))
}

func TestDependencyOrderTasksPreservesCycleSoftOrder(t *testing.T) {
	alpha := task("T-alpha", "app/work/alpha", "F-work", "app/work/zulu")
	mid := task("T-mid", "app/work/mid", "F-work", "T-zulu")
	zulu := task("T-zulu", "app/work/zulu", "F-work", "app/work/mid")
	tasks := []*store.Task{alpha, mid, zulu}
	tree := taskOrderTree(tasks)
	f := feature("F-work", "app/work", "app")
	f.Order = []string{"T-alpha", "T-zulu"}
	tree.Features = map[string]*store.Feature{f.ID: f}

	order := DependencyOrder(tree)
	want := []string{"T-zulu", "T-mid", "T-alpha"}
	require.Equal(t, want, taskIDs(order.Tasks(f.ID)))
	require.Equal(t, want, taskIDs(OrderTasks(tree, tasks, f.Order)))
	require.Equal(t, []CycleGroup{{Noun: store.NounTask, Members: []string{"T-zulu", "T-mid"}}}, order.Cycles())
}

func TestDependencyOrderTasksIgnoresIneligibleTargets(t *testing.T) {
	alpha := task("T-alpha", "app/work/alpha", "F-work", "T-other", "app/work/missing")
	zulu := task("T-zulu", "app/work/zulu", "F-work")
	other := task("T-other", "app/other/task", "F-other")
	orphan := task("T-orphan", "absent/task", "F-absent")
	tree := taskOrderTree([]*store.Task{alpha, zulu, other, orphan})
	tree.Features = map[string]*store.Feature{
		"F-work":  feature("F-work", "app/work", "app"),
		"F-other": feature("F-other", "app/other", "app"),
	}
	order := DependencyOrder(tree)
	require.Equal(t, []string{"T-alpha", "T-zulu"}, taskIDs(order.Tasks("F-work")))
	require.Equal(t, []string{"T-other"}, taskIDs(order.Tasks("F-other")))
	require.Nil(t, order.Tasks("F-absent"), "a task group cannot invent its missing feature")
}
