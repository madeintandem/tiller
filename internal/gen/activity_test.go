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

// TestFeatureCheckDueCoversUnrecordedAndStaleRequiredGates: a feature check is
// owed when every live task is done and a required feature gate was never run
// or went stale. A current fail or pending result is not a check owed, and
// neither is anything on a feature that is still in flight or already done.
func TestFeatureCheckDueCoversUnrecordedAndStaleRequiredGates(t *testing.T) {
	gate := func(status string) map[string]store.Gate {
		return map[string]store.Gate{"evals": {Required: true, Status: status}}
	}
	done := func() []*store.Task { return []*store.Task{taskWithStatus("T-1", "done")} }

	cases := []struct {
		name   string
		status string
		gates  map[string]store.Gate
		tasks  []*store.Task
		want   bool
	}{
		{"all done, gate never run", "active", nil, done(), true},
		{"all done, gate stale", "active", gate(store.GateStatusStale), done(), true},
		{"all done, gate failed", "active", gate(store.GateStatusFail), done(), false},
		{"all done, gate pending", "active", gate(store.GateStatusPending), done(), false},
		{"all done, gate green", "active", gate(store.GateStatusPass), done(), false},
		{"stale gate, a task still open", "active", gate(store.GateStatusStale),
			[]*store.Task{taskWithStatus("T-1", "done"), taskWithStatus("T-2", "in_progress")}, false},
		{"stale gate, the open task cancelled", "active", gate(store.GateStatusStale),
			[]*store.Task{taskWithStatus("T-1", "done"), taskWithStatus("T-2", "cancelled")}, true},
		{"stale gate, no live task", "active", gate(store.GateStatusStale),
			[]*store.Task{taskWithStatus("T-1", "cancelled")}, false},
		{"stale gate on a done feature", "done", gate(store.GateStatusStale), done(), false},
		{"stale gate on a ready feature", "ready", gate(store.GateStatusStale), done(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, f := featureTree(tc.status, tc.gates, tc.tasks...)
			require.Equal(t, tc.want, FeatureCheckDue(tree, f))
		})
	}
}

// TestFeatureCheckDueIgnoresAHeldTask: a held draft changes no rollup, so a
// task an open amendment holds does not keep a stale gate from being due.
func TestFeatureCheckDueIgnoresAHeldTask(t *testing.T) {
	held := taskWithStatus("T-2", "proposed")
	tree, f := featureTree("active", map[string]store.Gate{"evals": {Required: true, Status: store.GateStatusStale}},
		taskWithStatus("T-1", "done"), held)
	require.False(t, FeatureCheckDue(tree, f), "unheld, the proposed task is work still to do")
	tree.Amendments = map[string]*store.Amendment{"A-aaaaaa": {ID: "A-aaaaaa", Status: store.AmendmentStatusProposed}}
	tree.AmendmentHolds = map[string]string{held.ID: "A-aaaaaa"}
	require.True(t, FeatureCheckDue(tree, f))
}

// TestFeatureCheckDueLeavesAStaleAttestationToTheHuman: check never runs an
// attestation gate and carries an expired verdict forward as it is, so naming
// `tiller check feature` for it would name a command that changes nothing.
func TestFeatureCheckDueLeavesAStaleAttestationToTheHuman(t *testing.T) {
	tree, f := featureTree("active", map[string]store.Gate{
		"evals":   {Required: true, Status: store.GateStatusPass},
		"signoff": {Required: true, Kind: store.GateKindAttestation, Status: store.GateStatusStale},
	}, taskWithStatus("T-1", "done"))
	tree.Manifest.Gates.Required["signoff"] = store.GateSpec{Kind: store.GateKindAttestation, On: store.StringList{"feature"}}
	require.False(t, FeatureCheckDue(tree, f))

	f.Gates["evals"] = store.Gate{Required: true, Status: store.GateStatusStale}
	require.True(t, FeatureCheckDue(tree, f), "a stale command gate beside it is still a check owed")
}

// TestFeatureCheckDueIgnoresAGateTheFeatureGrainDoesNotRun: a stale record for
// a gate the manifest scopes to another grain is not this feature's to rerun.
func TestFeatureCheckDueIgnoresAGateTheFeatureGrainDoesNotRun(t *testing.T) {
	tree, f := featureTree("active", map[string]store.Gate{
		"evals": {Required: true, Status: store.GateStatusPass},
		"tests": {Required: true, Status: store.GateStatusStale},
	}, taskWithStatus("T-1", "done"))
	tree.Manifest.Gates.Required["tests"] = store.GateSpec{Kind: store.GateKindCommand, On: store.StringList{"task"}}
	require.False(t, FeatureCheckDue(tree, f))
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

func TestFeatureBoardStatusShowsFinishedWorkAwaitingReviewAsInReview(t *testing.T) {
	pr := 12
	for _, tc := range []struct {
		name    string
		feature string
		tasks   [3]string
		pr      *int
		ref     *store.ReviewRef
		want    string
	}{
		{"task PRs open, rest merged", "active", [3]string{"in_review", "done", "done"}, nil, nil, "in_review"},
		{"every task in review", "active", [3]string{"in_review", "in_review", "in_review"}, nil, nil, "in_review"},
		{"cancelled task ignored", "active", [3]string{"in_review", "cancelled", "done"}, nil, nil, "in_review"},
		{"one task still checking", "active", [3]string{"in_review", "check", "done"}, nil, nil, "active"},
		{"one task not started", "active", [3]string{"in_review", "ready", "done"}, nil, nil, "active"},
		{"all done awaiting check", "active", [3]string{"done", "done", "done"}, nil, nil, "active"},
		{"all done, feature PR recorded", "active", [3]string{"done", "done", "done"}, &pr, nil, "in_review"},
		{"all done, feature review ref recorded", "active", [3]string{"done", "done", "done"}, nil, &store.ReviewRef{Branch: "feature/loading"}, "in_review"},
		{"feature PR with work left", "active", [3]string{"done", "in_progress", "done"}, &pr, nil, "active"},
		{"every task cancelled", "active", [3]string{"cancelled", "cancelled", "cancelled"}, nil, nil, "active"},
		{"ready feature, riding tasks in review", "ready", [3]string{"in_review", "in_review", "done"}, nil, nil, "in_review"},
		{"done feature", "done", [3]string{"done", "done", "done"}, &pr, nil, "done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := candidateTree()
			feature := tree.Features["F-loadng"]
			feature.Status, feature.PR, feature.ReviewRef = tc.feature, tc.pr, tc.ref
			for i, id := range []string{"T-aaaa01", "T-bbbb02", "T-mmmm03"} {
				tree.Tasks[id].Status = tc.tasks[i]
			}
			require.Equal(t, tc.want, FeatureBoardStatus(tree, feature))
		})
	}
}

// TestBoardStatusReadsAReadyEpicWithWorkInFlightAsActive pins the epic half
// of the console's one displayed status: an approved epic stays stored ready,
// and reads active only while a task under it is in flight.
func TestBoardStatusReadsAReadyEpicWithWorkInFlightAsActive(t *testing.T) {
	tree := candidateTree()
	tree.Features["F-export"].Status = "ready"
	epic := tree.Epics["E-formen"]
	require.Equal(t, "ready", BoardStatus(tree, epic), "no task in flight")

	tree.Tasks["T-aaaa01"].Status = "in_progress"
	require.Equal(t, "active", BoardStatus(tree, epic))
	require.Equal(t, "ready", epic.Status, "the stored status is never replaced")

	epic.Status = "done"
	require.Equal(t, "done", BoardStatus(tree, epic), "only a ready epic reads active")
}

func TestBoardStatusIsFeatureBoardStatusForAFeatureAndStoredElsewhere(t *testing.T) {
	tree := candidateTree()
	tree.Tasks["T-aaaa01"].Status = "in_progress"
	require.Equal(t, "active", BoardStatus(tree, tree.Features["F-loadng"]))
	require.Equal(t, "in_progress", BoardStatus(tree, tree.Tasks["T-aaaa01"]))
}

func TestBoardColumnIsTheBoardStatusForEpicsAndFeaturesOnly(t *testing.T) {
	tree := candidateTree()
	tree.Tasks["T-aaaa01"].Status = "in_progress"
	require.Equal(t, "active", BoardColumn(tree, tree.Features["F-loadng"]))
	require.Equal(t, "active", BoardColumn(tree, tree.Epics["E-formen"]))
	require.Empty(t, BoardColumn(tree, tree.Tasks["T-aaaa01"]))
}

// TestRollupIgnoresAHeldTask: a held draft changes no rollup, so a held
// proposed task neither counts toward its feature's completion nor keeps the
// feature from the awaiting phases or the in-review board column.
func TestRollupIgnoresAHeldTask(t *testing.T) {
	held := taskWithStatus("T-2", "proposed")
	tree, f := featureTree("active", nil, taskWithStatus("T-1", "done"), held)
	require.Equal(t, "", FeatureActivity(tree, f), "unheld, the proposed task holds the rollup")
	tree.Amendments = map[string]*store.Amendment{"A-aaaaaa": {ID: "A-aaaaaa", Status: store.AmendmentStatusProposed}}
	tree.AmendmentHolds = map[string]string{held.ID: "A-aaaaaa"}
	require.Equal(t, FeatureActivityAwaitingCheck, FeatureActivity(tree, f))

	tree.Tasks["T-1"].Status = ReviewState
	require.Equal(t, ReviewState, FeatureBoardStatus(tree, f), "the held task does not keep the feature out of review")
}
