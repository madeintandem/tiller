package mutate_test

import (
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin SetOrder, the soft order's one writer
// (docs/plans/2026-08-08-soft-order-design.md): a recorded statement of
// intended sequence that arranges rendering and steers routing, and never
// gates a legal action.

// orderRepo lays two epics (form-engine with two features, payments with one)
// and three tasks, two under form-engine/schema-engine and one under
// form-engine/certification-form-loading: enough siblings at every grain to record an
// order, and a cross-parent pair at both feature and task grain to prove the
// split-parent refusal.
func orderRepo(t *testing.T) string {
	t.Helper()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)

	mint := func(suffix string) mutate.Deps {
		d := testDeps()
		d.Mint = func(prefix string) string { return prefix + "-" + suffix }
		return d
	}

	_, err = mutate.AddFeature(load(t, root), mint("bbbbbb"), mutate.FeatureInput{
		Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "create"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), mint("cccccc"), mutate.FeatureInput{
		Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)

	_, err = mutate.AddEpic(load(t, root), mint("dddddd"), mutate.EpicInput{Key: "payments", Title: "Payments"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), mint("eeeeee"), mutate.FeatureInput{
		Key: "payments/checkout", Title: "Checkout", ChangeType: "create"})
	require.NoError(t, err)

	_, err = mutate.AddTask(load(t, root), mint("first0"), mutate.TaskInput{
		Key: "form-engine/schema-engine/first", Title: "First"})
	require.NoError(t, err)
	_, err = mutate.AddTask(load(t, root), mint("second"), mutate.TaskInput{
		Key: "form-engine/schema-engine/second", Title: "Second"})
	require.NoError(t, err)
	_, err = mutate.AddTask(load(t, root), mint("other0"), mutate.TaskInput{
		Key: "form-engine/certification-form-loading/other", Title: "Other"})
	require.NoError(t, err)

	return root
}

// TestSetOrderEpicGrainWritesOrderYMLAndResult pins the root grain: the
// resolved ids land in .tiller/order.yml, in the stated sequence, and the
// result echoes them with no parent (the root has none).
func TestSetOrderEpicGrainWritesOrderYMLAndResult(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)

	res, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounEpic, Nodes: []string{"payments", "form-engine"},
	})
	require.NoError(t, err)
	require.Equal(t, store.NounEpic, res.Noun)
	require.Empty(t, res.Parent, "epic grain has no parent to name; it is the root")
	require.Equal(t, []string{"E-dddddd", "E-aaaaaa"}, res.Order)
	require.True(t, res.Changed)

	require.FileExists(t, filepath.Join(root, ".tiller", "order.yml"))
	tree := load(t, root)
	require.Equal(t, []string{"E-dddddd", "E-aaaaaa"}, tree.ProjectOrder.Epics)
}

// TestSetOrderFeatureGrainWritesEpicOrderAndEvent pins the feature grain: the
// list lands on the parent epic's own order field, and one order.features
// event records it on the epic's stream.
func TestSetOrderFeatureGrainWritesEpicOrderAndEvent(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)

	res, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Nodes: []string{"form-engine/certification-form-loading", "form-engine/schema-engine"},
	})
	require.NoError(t, err)
	require.Equal(t, store.NounFeature, res.Noun)
	require.Equal(t, "E-aaaaaa", res.Parent)
	require.Equal(t, []string{"F-cccccc", "F-bbbbbb"}, res.Order)
	require.True(t, res.Changed)

	tree := load(t, root)
	require.Equal(t, []string{"F-cccccc", "F-bbbbbb"}, tree.Epics["E-aaaaaa"].Order)

	evs, err := events.List(tree)
	require.NoError(t, err)
	ordered := eventsOfType(evs, "order.features")
	require.Len(t, ordered, 1)
	require.Contains(t, ordered[0].Targets, "E-aaaaaa", "the event rides the parent epic's stream")
	require.Equal(t, []any{"F-cccccc", "F-bbbbbb"}, ordered[0].After["order"])
}

// TestSetOrderTaskGrainWritesFeatureOrderAndEvent pins the task grain: the
// list lands on the parent feature's own order field, and one order.tasks
// event records it on the feature's stream.
func TestSetOrderTaskGrainWritesFeatureOrderAndEvent(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)

	res, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounTask, Nodes: []string{"T-second", "T-first0"},
	})
	require.NoError(t, err)
	require.Equal(t, store.NounTask, res.Noun)
	require.Equal(t, "F-bbbbbb", res.Parent)
	require.Equal(t, []string{"T-second", "T-first0"}, res.Order)
	require.True(t, res.Changed)

	tree := load(t, root)
	require.Equal(t, []string{"T-second", "T-first0"}, tree.Features["F-bbbbbb"].Order)

	evs, err := events.List(tree)
	require.NoError(t, err)
	ordered := eventsOfType(evs, "order.tasks")
	require.Len(t, ordered, 1)
	require.Contains(t, ordered[0].Targets, "F-bbbbbb", "the event rides the parent feature's stream")
	require.Equal(t, []any{"T-second", "T-first0"}, ordered[0].After["order"])
}

// TestSetOrderReplacesNotUnions pins replace semantics: a second call with a
// different list overwrites the first outright (not a union), records its own
// event, and a third call restating the identical list is a no-change success
// that records no new event.
func TestSetOrderReplacesNotUnions(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)

	first, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Nodes: []string{"form-engine/schema-engine"},
	})
	require.NoError(t, err)
	require.True(t, first.Changed)
	require.Equal(t, []string{"F-bbbbbb"}, first.Order)

	second, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Nodes: []string{"form-engine/certification-form-loading"},
	})
	require.NoError(t, err)
	require.True(t, second.Changed)
	require.Equal(t, []string{"F-cccccc"}, second.Order, "the second list replaces the first outright")

	tree := load(t, root)
	require.Equal(t, []string{"F-cccccc"}, tree.Epics["E-aaaaaa"].Order)

	evs, err := events.List(tree)
	require.NoError(t, err)
	require.Len(t, eventsOfType(evs, "order.features"), 2, "each real change records its own event")

	third, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Nodes: []string{"form-engine/certification-form-loading"},
	})
	require.NoError(t, err)
	require.False(t, third.Changed, "an identical list is a no-change success")

	evsAfter, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, eventsOfType(evsAfter, "order.features"), 2, "the no-op repeat records no new event")
}

// TestSetOrderRefusesDuplicateEntry pins the duplicate guard: the same node
// named twice, by whatever spelling, refuses rather than silently folding.
func TestSetOrderRefusesDuplicateEntry(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Nodes: []string{"form-engine/schema-engine", "F-bbbbbb"},
	})
	require.ErrorContains(t, err, "F-bbbbbb is listed twice in the order")
}

// TestSetOrderRefusesSplitParent pins the shared-parent guard at feature
// grain: two features under different epics cannot share one recorded order.
func TestSetOrderRefusesSplitParent(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Nodes: []string{"form-engine/schema-engine", "payments/checkout"},
	})
	require.ErrorContains(t, err, "order features must share one parent")
	require.ErrorContains(t, err, "F-bbbbbb is under E-aaaaaa")
	require.ErrorContains(t, err, "F-eeeeee under E-dddddd")
}

// TestSetOrderRefusesSplitParentAtTaskGrain pins the same guard one grain
// down: two tasks under different features cannot share one recorded order.
func TestSetOrderRefusesSplitParentAtTaskGrain(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounTask, Nodes: []string{"T-first0", "T-other0"},
	})
	require.ErrorContains(t, err, "order tasks must share one parent")
	require.ErrorContains(t, err, "T-first0 is under F-bbbbbb")
	require.ErrorContains(t, err, "T-other0 under F-cccccc")
}

// TestSetOrderRefusesUnorderableNoun pins the noun guard: only epic, feature,
// task, bug, and chore rank; a question keeps id order and refuses plainly.
func TestSetOrderRefusesUnorderableNoun(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounQuestion, Nodes: []string{"Q-aaaaaa"},
	})
	require.ErrorContains(t, err, "order ranks epics, features, tasks, bugs, chores, and tweaks; a question keeps id order")
}

// TestSetOrderRefusesEmptyWithoutClear pins the empty-list guard: nothing to
// record and no --clear is a plain refusal, not a silent no-op.
func TestSetOrderRefusesEmptyWithoutClear(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounEpic,
	})
	require.ErrorContains(t, err, "order needs at least one node, or --clear")
}

// TestSetOrderClearEpicGrain pins --clear at the root: the recorded epic
// sequence goes back to nil.
func TestSetOrderClearEpicGrain(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounEpic, Nodes: []string{"form-engine", "payments"},
	})
	require.NoError(t, err)

	res, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounEpic, Clear: true,
	})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Nil(t, res.Order)

	tree := load(t, root)
	require.Nil(t, tree.ProjectOrder.Epics)
}

// TestSetOrderClearFeatureGrain pins --clear at feature grain: --epic names
// whose list to clear, since no siblings are given to infer a parent from.
func TestSetOrderClearFeatureGrain(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Nodes: []string{"form-engine/schema-engine", "form-engine/certification-form-loading"},
	})
	require.NoError(t, err)

	res, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Clear: true, ParentEpic: "form-engine",
	})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Nil(t, res.Order)
	require.Equal(t, "E-aaaaaa", res.Parent)

	tree := load(t, root)
	require.Nil(t, tree.Epics["E-aaaaaa"].Order)

	evs, err := events.List(tree)
	require.NoError(t, err)
	require.Len(t, eventsOfType(evs, "order.features"), 2, "the set and the clear each record their own event")
}

// TestSetOrderClearTaskGrain pins --clear at task grain: --feature names
// whose list to clear.
func TestSetOrderClearTaskGrain(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounTask, Nodes: []string{"T-first0", "T-second"},
	})
	require.NoError(t, err)

	res, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounTask, Clear: true, ParentFeature: "form-engine/schema-engine",
	})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Nil(t, res.Order)
	require.Equal(t, "F-bbbbbb", res.Parent)

	tree := load(t, root)
	require.Nil(t, tree.Features["F-bbbbbb"].Order)
}

// TestSetOrderRefusesClearWithNodes pins the guard whole-branch review found
// missing: SetOrder routed to setOrderClear only when Nodes was empty, so a
// non-empty Nodes list with Clear set fell through to the record path with
// Clear silently unexamined, recording a list instead of refusing. `tiller
// order feature some-feature --clear` must refuse, not record a one-element
// order.
func TestSetOrderRefusesClearWithNodes(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Nodes: []string{"form-engine/schema-engine"}, Clear: true,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "order takes a node list or --clear, never both")
	require.ErrorContains(t, err, "order feature <node> <node> ...")
	require.ErrorContains(t, err, "order feature --clear")

	tree := load(t, root)
	require.Nil(t, tree.Epics["E-aaaaaa"].Order, "the refused call must not have recorded anything")
}

// TestSetOrderRefusesParentFlagWithNodes pins the same gap's other half: a
// parent flag (ParentEpic or ParentFeature) scopes --clear, which names no
// siblings to infer a parent from; naming one alongside a Nodes list, with
// or without Clear, has nothing to scope and must refuse rather than pass
// through unexamined.
func TestSetOrderRefusesParentFlagWithNodes(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Nodes: []string{"form-engine/schema-engine"}, ParentEpic: "form-engine",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "--epic and --feature scope --clear alone")
	require.ErrorContains(t, err, "order feature <node> <node> ...")
	require.ErrorContains(t, err, "order feature --clear")

	tree := load(t, root)
	require.Nil(t, tree.Epics["E-aaaaaa"].Order, "the refused call must not have recorded anything")

	_, err = mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounTask, Nodes: []string{"T-first0"}, ParentFeature: "form-engine/schema-engine",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "--epic and --feature scope --clear alone")
}

// TestSetOrderClearRefusesMissingParent pins the guard --clear needs at
// feature and task grain: with no siblings to infer a parent from, the parent
// flag is required and its absence refuses plainly.
func TestSetOrderClearRefusesMissingParent(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)

	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Clear: true,
	})
	require.ErrorContains(t, err, "order feature --clear needs --epic to name whose list to clear")

	_, err = mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounTask, Clear: true,
	})
	require.ErrorContains(t, err, "order task --clear needs --feature to name whose list to clear")
}

// TestSetOrderKeysAndIDsMixResolveToSameList pins that a key and an id naming
// the same node resolve to the same stored ids, whatever spelling the caller
// used for each.
func TestSetOrderKeysAndIDsMixResolveToSameList(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	res, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounFeature, Nodes: []string{"F-cccccc", "form-engine/schema-engine"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"F-cccccc", "F-bbbbbb"}, res.Order, "a key and an id resolve to the same stored ids")
}

// TestSetOrderEpicGrainRecordsNoEvent pins the deliberate v1 gap: epic grain
// writes .tiller/order.yml but records no node event, because there is no
// project-grain event stream to record it on (order.yml under git is the
// record).
func TestSetOrderEpicGrainRecordsNoEvent(t *testing.T) {
	t.Parallel()
	root := orderRepo(t)
	before, err := events.List(load(t, root))
	require.NoError(t, err)
	beforeCount := len(before)

	_, err = mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{
		Noun: store.NounEpic, Nodes: []string{"form-engine", "payments"},
	})
	require.NoError(t, err)

	after, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, after, beforeCount, "epic grain writes order.yml but records no event")
}
