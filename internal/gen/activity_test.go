package gen

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func candidateTree() *store.Tree {
	tree := reversedSiblingTree()
	for _, task := range tree.Tasks {
		task.Status, task.Owner = "ready", ""
		task.DependsOn = nil
	}
	tree.Features["F-loadng"].Status = "ready"
	tree.Features["F-loadng"].Order = []string{"T-aaaa01", "T-bbbb02", "T-mmmm03"}
	return tree
}

func TestEligibleTaskPreservesStructuralStartGuards(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*store.Tree, *store.Task)
	}{
		{"task pause", func(_ *store.Tree, task *store.Task) { task.Blocked = true }},
		{"feature pause", func(tree *store.Tree, _ *store.Task) { tree.Features["F-loadng"].Blocked = true }},
		{"epic pause", func(tree *store.Tree, _ *store.Task) { tree.Epics["E-formen"].Blocked = true }},
		{"feature not approved", func(tree *store.Tree, _ *store.Task) { tree.Features["F-loadng"].Status = "specced" }},
		{"closed epic", func(tree *store.Tree, _ *store.Task) { tree.Epics["E-formen"].Status = "done" }},
		{"missing task prerequisite", func(_ *store.Tree, task *store.Task) { task.DependsOn = []string{"T-missing"} }},
		{"live feature prerequisite", func(tree *store.Tree, _ *store.Task) { tree.Features["F-loadng"].DependsOn = []string{"F-export"} }},
		{"ancestor question", func(tree *store.Tree, _ *store.Task) {
			tree.Questions = map[string]*store.Question{"Q-hold": {ID: "Q-hold", Status: "open", Blocks: []string{"E-formen"}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := candidateTree()
			task := tree.Tasks["T-aaaa01"]
			require.True(t, EligibleTask(tree, task))
			tc.change(tree, task)
			require.False(t, EligibleTask(tree, task))
		})
	}
}

func TestEligibleTaskAcceptsCoarseSubmittedPrerequisiteOnly(t *testing.T) {
	tree := candidateTree()
	task := tree.Tasks["T-bbbb02"]
	task.DependsOn = []string{"T-aaaa01"}
	dep := tree.Tasks["T-aaaa01"]
	dep.Status = "in_review"
	dep.ReviewRef = &store.ReviewRef{Branch: "task-branch"}
	tree.Features["F-loadng"].ReviewGrain = store.ReviewGrainTask
	require.False(t, EligibleTask(tree, task))
	tree.Features["F-loadng"].ReviewGrain = store.ReviewGrainFeature
	require.True(t, EligibleTask(tree, task))
	dep.ReviewRef = nil
	require.False(t, EligibleTask(tree, task))
	dep.Status = "cancelled"
	require.True(t, EligibleTask(tree, task))
}

// featureTree builds a one-feature tree whose manifest requires the evals gate
// at feature grain, the Acme configuration the incident ran under.
func featureTree(status string, gates map[string]store.Gate, tasks ...*store.Task) (*store.Tree, *store.Feature) {
	key := "foundation/repo-tooling"
	f := &store.Feature{ID: "F-aaaaaa", Key: &key, Status: status,
		GatesBlock: store.GatesBlock{Gates: gates}}
	tree := &store.Tree{
		Manifest: &store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
			"evals": {Kind: "command", On: store.StringList{"feature"}},
		}}},
		Features: map[string]*store.Feature{f.ID: f},
		Tasks:    map[string]*store.Task{},
	}
	for _, tk := range tasks {
		tk.FeatureID = f.ID
		tree.Tasks[tk.ID] = tk
	}
	return tree, f
}

func taskWithStatus(id, status string) *store.Task {
	return &store.Task{ID: id, Status: status}
}

func TestFeatureActivityTruthTable(t *testing.T) {
	checked := map[string]store.Gate{"evals": {Required: true, Status: store.GateStatusPending}}

	cases := []struct {
		name   string
		status string
		gates  map[string]store.Gate
		tasks  []*store.Task
		want   string
	}{
		{"no tasks at all is no phase", "active", nil, nil, ""},
		{"a task still in flight is no phase", "active", nil,
			[]*store.Task{taskWithStatus("T-1", "done"), taskWithStatus("T-2", "in_progress")}, ""},
		{"all done, gate unrecorded, awaits check", "active", nil,
			[]*store.Task{taskWithStatus("T-1", "done"), taskWithStatus("T-2", "done")}, FeatureActivityAwaitingCheck},
		{"all done, gate recorded but not green, awaits sync", "active", checked,
			[]*store.Task{taskWithStatus("T-1", "done")}, FeatureActivityAwaitingSync},
		{"a stale recorded result still counts as checked", "active",
			map[string]store.Gate{"evals": {Required: true, Status: store.GateStatusStale}},
			[]*store.Task{taskWithStatus("T-1", "done")}, FeatureActivityAwaitingSync},
		{"cancelled tasks are excluded from both sides", "active", nil,
			[]*store.Task{taskWithStatus("T-1", "done"), taskWithStatus("T-2", "cancelled")}, FeatureActivityAwaitingCheck},
		{"cancelled-only set is vacuous, no phase", "active", nil,
			[]*store.Task{taskWithStatus("T-1", "cancelled")}, ""},
		{"a non-active feature has no phase however its tasks stand", "ready", nil,
			[]*store.Task{taskWithStatus("T-1", "done")}, ""},
		{"done feature has no phase", "done", checked,
			[]*store.Task{taskWithStatus("T-1", "done")}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, f := featureTree(tc.status, tc.gates, tc.tasks...)
			require.Equal(t, tc.want, FeatureActivity(tree, f))
		})
	}
}

func TestFeatureActivityWithNoRequiredFeatureGatesIsAwaitingSync(t *testing.T) {
	tree, f := featureTree("active", nil, taskWithStatus("T-1", "done"))
	tree.Manifest.Gates.Required = nil
	require.Equal(t, FeatureActivityAwaitingSync, FeatureActivity(tree, f),
		"an empty required set is vacuously checked, matching featureGatesChecked today")
}

func TestFeatureDisplayAppendsThePhase(t *testing.T) {
	tree, f := featureTree("active", nil, taskWithStatus("T-1", "done"))
	require.Equal(t, "active, awaiting check", FeatureDisplay(tree, f))

	f.GatesBlock.Gates = map[string]store.Gate{"evals": {Status: store.GateStatusPending}}
	require.Equal(t, "active, awaiting sync", FeatureDisplay(tree, f))

	tree2, f2 := featureTree("active", nil, taskWithStatus("T-1", "done"), taskWithStatus("T-2", "in_progress"))
	require.Equal(t, "active", FeatureDisplay(tree2, f2), "no phase, bare stored status")
}

func TestStringListIncludes(t *testing.T) {
	require.True(t, store.StringList{"feature", "task"}.Includes("feature"))
	require.False(t, store.StringList{"task"}.Includes("feature"))
	require.False(t, store.StringList(nil).Includes("feature"))
}

func TestFeatureBoardStatusShowsReadyWorkInFlightAsActive(t *testing.T) {
	for _, tc := range []struct {
		feature, task, want string
	}{
		{"ready", "ready", "ready"},
		{"ready", "proposed", "ready"},
		{"ready", "in_progress", "active"},
		{"ready", "check", "active"},
		{"ready", "in_review", "active"},
		{"ready", "done", "ready"},
		{"specced", "in_progress", "specced"},
		{"active", "ready", "active"},
		{"done", "done", "done"},
	} {
		t.Run(tc.feature+"/"+tc.task, func(t *testing.T) {
			tree := candidateTree()
			feature := tree.Features["F-loadng"]
			feature.Status = tc.feature
			tree.Tasks["T-aaaa01"].Status = tc.task
			require.Equal(t, tc.want, FeatureBoardStatus(tree, feature))
		})
	}
}
