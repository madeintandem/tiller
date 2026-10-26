package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// These tests cover adopting an aggregate: a hand-built aggregate at the derived name
// was refused as "X conflicts with X", and the only recovery deadlocked a
// dependent task start. start task --adopt-aggregate records the existing
// ref at exactly the derived name; nothing ever renames or deletes a ref.

const adoptAgg = "tiller/reviews/features/pay/checkout"

// adoptStaleGrainFixture is the issue's reproduction: alfa ran at task grain
// and was submitted, the grain change then landed, and bravo depends on alfa.
// HEAD is left on alfa's branch, so a ref created at HEAD holds alfa's work.
func adoptStaleGrainFixture(t *testing.T) (string, mutate.Deps) {
	t.Helper()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "alfa", "bravo")
	_, err := mutate.Link(load(t, root), deps, mutate.LinkInput{Noun: store.NounTask, Node: "pay/checkout/bravo", DependsOn: []string{"pay/checkout/alfa"}})
	require.NoError(t, err)
	gitCommitAllM(t, root, "plan")

	rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "alfa", 1)

	feature := featureByKey(load(t, root), "pay/checkout")
	feature.ReviewGrain = "feature"
	data, err := yaml.Marshal(feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "feature.yml"), data, 0o644))
	gitCommitAllM(t, root, "grain merged")

	driveWith(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "approve"})
	return root, deps
}

// adoptFeatureGrainFixture is a fresh feature-grain feature with alfa ready.
func adoptFeatureGrainFixture(t *testing.T, taskKeys ...string) (string, mutate.Deps) {
	t.Helper()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", taskKeys...)
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: "feature"})
	require.NoError(t, err)
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "approve"})
	return root, deps
}

func requireSafetyReason(t *testing.T, err error, reason string) *mutate.BranchSafetyError {
	t.Helper()
	var safety *mutate.BranchSafetyError
	require.ErrorAs(t, err, &safety)
	require.Equal(t, reason, safety.Reason, safety.Error())
	return safety
}

func TestStartRefusesUnrecordedAggregateByName(t *testing.T) {
	t.Parallel()
	root, deps := adoptStaleGrainFixture(t)
	gitRunM(t, root, "branch", adoptAgg)

	err := tryTransition(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start"})
	safety := requireSafetyReason(t, err, "aggregate_unrecorded")
	require.Equal(t, adoptAgg, safety.Branch)
	require.Contains(t, safety.Error(), "exists but is not recorded on feature")
	require.Contains(t, safety.Error(), "--adopt-aggregate")
	require.Contains(t, safety.Error(), "Tiller will not delete refs")
	require.Empty(t, featureByKey(load(t, root), "pay/checkout").Branch)
}

// clearStartedGrain models a task started before task.yml recorded its grain:
// it resolves through its parents, so an aggregate may hold it.
// A task that recorded task grain never enters one (started_grain_test.go).
func clearStartedGrain(t *testing.T, root, taskKey string) {
	t.Helper()
	task := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/"+taskKey).(*store.Task)
	task.StartedGrain = ""
	data, err := yaml.Marshal(task)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "tasks", taskKey, "task.yml"), data, 0o644))
}

func TestStartAdoptAggregateRecordsAndStartsDependent(t *testing.T) {
	t.Parallel()
	root, deps := adoptStaleGrainFixture(t)
	clearStartedGrain(t, root, "alfa")
	gitRunM(t, root, "branch", adoptAgg)

	res := driveWith(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start", AdoptAggregate: true})
	require.Equal(t, adoptAgg, featureByKey(load(t, root), "pay/checkout").Branch)
	require.Equal(t, adoptAgg, res.BaseBranch)
	require.Equal(t, adoptAgg, res.AggregateBranch)
	require.NotEmpty(t, res.AdoptedCommits)
}

func TestStartAdoptAggregateStillRequiresPrerequisite(t *testing.T) {
	t.Parallel()
	root, deps := adoptStaleGrainFixture(t)
	clearStartedGrain(t, root, "alfa")
	gitRunM(t, root, "branch", adoptAgg, "main")

	err := tryTransition(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start", AdoptAggregate: true})
	var dep *mutate.TaskDependencyBlockedError
	require.ErrorAs(t, err, &dep)
	require.False(t, dep.AggregateUnrecorded, "an adopted aggregate is recorded by this start")
	require.Empty(t, dep.DependencyStartedGrain)
	require.Contains(t, dep.Error(), "submit that prerequisite and merge its complete branch into aggregate")
	require.Equal(t, adoptAgg, dep.Branch)
	require.Empty(t, featureByKey(load(t, root), "pay/checkout").Branch, "a refused start writes nothing")
}

func TestStartAdoptAggregateRefusesMissingRef(t *testing.T) {
	t.Parallel()
	root, deps := adoptFeatureGrainFixture(t, "alfa")
	err := tryTransition(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start", AdoptAggregate: true})
	requireSafetyReason(t, err, "aggregate_adopt_missing")
	require.Empty(t, featureByKey(load(t, root), "pay/checkout").Branch)
}

func TestStartAdoptAggregateRefusesRecordedBranch(t *testing.T) {
	t.Parallel()
	root, deps := adoptFeatureGrainFixture(t, "alfa", "bravo")
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})
	gitRunM(t, root, "branch", adoptAgg)
	driveWith(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "approve"})

	err := tryTransition(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start", AdoptAggregate: true})
	requireSafetyReason(t, err, "aggregate_adopt_recorded")
}

func TestStartAdoptAggregateRefusesTaskGrain(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "approve"})
	gitRunM(t, root, "branch", adoptAgg)

	err := tryTransition(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start", AdoptAggregate: true})
	requireSafetyReason(t, err, "aggregate_adopt_grain")
	require.Empty(t, featureByKey(load(t, root), "pay/checkout").Branch)
}

func TestStartAdoptAggregateRefusesPrefixConflict(t *testing.T) {
	t.Parallel()
	root, deps := adoptFeatureGrainFixture(t, "alfa")
	gitRunM(t, root, "branch", "tiller/reviews/features/pay")

	err := tryTransition(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start", AdoptAggregate: true})
	safety := requireSafetyReason(t, err, "branch_ref_conflict")
	require.Equal(t, "tiller/reviews/features/pay", safety.Conflict)
}

func TestStartAdoptAggregateRefusesTaskBranchName(t *testing.T) {
	t.Parallel()
	root, deps := adoptFeatureGrainFixture(t, "alfa", "bravo")
	gitRunM(t, root, "branch", adoptAgg)
	bravo := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/bravo").(*store.Task)
	bravo.Branch, bravo.Started, bravo.Status = adoptAgg, true, "in_progress"
	data, err := yaml.Marshal(bravo)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "tasks", "bravo", "task.yml"), data, 0o644))

	err = tryTransition(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start", AdoptAggregate: true})
	requireSafetyReason(t, err, "aggregate_adopt_task_branch")
	require.Empty(t, featureByKey(load(t, root), "pay/checkout").Branch)
}

func TestStartAdoptAggregateAtEpicGrain(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: store.NounEpic, Key: "pay", ReviewGrain: "epic"})
	require.NoError(t, err)
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "approve"})
	agg := "tiller/reviews/epics/pay"
	gitRunM(t, root, "branch", agg)

	res := driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start", AdoptAggregate: true})
	tree := load(t, root)
	require.Equal(t, agg, epicByKey(tree, "pay").Branch)
	require.Equal(t, agg, featureByKey(tree, "pay/checkout").Branch)
	require.Equal(t, agg, res.AggregateBranch)
	require.Empty(t, res.AdoptedCommits, "a ref at integration holds nothing beyond it")
}

func TestStartAdoptAggregateRefusesNonTaskNoun(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "blank-on-reload", Title: "Blank on reload"})
	require.NoError(t, err)
	err = tryTransition(t, root, deps, store.NounBug, "blank-on-reload", mutate.TransitionInput{Verb: "start", AdoptAggregate: true})
	safety := requireSafetyReason(t, err, "aggregate_adopt_grain")
	require.Equal(t, store.NounBug, safety.Noun)
}

func TestStartDependencyRefusalNamesUnrecordedAggregate(t *testing.T) {
	t.Parallel()
	root, deps := adoptStaleGrainFixture(t)
	clearStartedGrain(t, root, "alfa")
	gitRunM(t, root, "branch", adoptAgg)
	gitRunM(t, root, "branch", "-m", adoptAgg, "aside/checkout")

	err := tryTransition(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start"})
	var dep *mutate.TaskDependencyBlockedError
	require.ErrorAs(t, err, &dep)
	require.True(t, dep.AggregateUnrecorded, "the aggregate this start named was never recorded")
	require.Empty(t, dep.DependencyStartedGrain, "a legacy prerequisite keeps the adopt remedy")
	require.Contains(t, dep.Error(), "--adopt-aggregate")
	require.Equal(t, adoptAgg, dep.Branch)
	require.Empty(t, featureByKey(load(t, root), "pay/checkout").Branch)
}

// A minted start's dependency refusal names adoption only when assembly could
// unblock it: a ready same-feature prerequisite must be started first.
func TestStartDependencyRefusalReadyPrerequisiteKeepsWaitText(t *testing.T) {
	t.Parallel()
	root, deps := adoptFeatureGrainFixture(t, "alfa", "bravo")
	_, err := mutate.Link(load(t, root), deps, mutate.LinkInput{Noun: store.NounTask, Node: "pay/checkout/bravo", DependsOn: []string{"pay/checkout/alfa"}})
	require.NoError(t, err)
	driveWith(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "approve"})

	err = tryTransition(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start"})
	var dep *mutate.TaskDependencyBlockedError
	require.ErrorAs(t, err, &dep)
	require.False(t, dep.AggregateUnrecorded)
	require.Contains(t, dep.Error(), "wait for integration")
	require.NotContains(t, dep.Error(), "--adopt-aggregate")
}

// A cross-feature prerequisite can never be assembled into this aggregate,
// so only waiting works. An in_progress same-feature prerequisite is the same.
func TestStartDependencyRefusalCrossFeaturePrerequisiteKeepsWaitText(t *testing.T) {
	t.Parallel()
	root, deps := adoptFeatureGrainFixture(t, "alfa")
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})
	rgAddFeature(t, root, deps, "pay", "refund")
	rgAddTask(t, root, deps, "pay", "refund", "delta")
	driveWith(t, root, deps, store.NounFeature, "pay/refund", mutate.TransitionInput{Verb: "spec"})
	driveWith(t, root, deps, store.NounFeature, "pay/refund", mutate.TransitionInput{Verb: "approve"})
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{Noun: store.NounFeature, Key: "pay/refund", ReviewGrain: "feature"})
	require.NoError(t, err)
	driveWith(t, root, deps, store.NounTask, "pay/refund/delta", mutate.TransitionInput{Verb: "approve"})
	// link refuses a cross-feature task edge, so stage a legacy one by hand.
	tree := load(t, root)
	alfa := resolveNode(t, tree, store.NounTask, "pay/checkout/alfa").(*store.Task)
	delta := resolveNode(t, tree, store.NounTask, "pay/refund/delta").(*store.Task)
	delta.DependsOn = []string{alfa.ID}
	data, err := yaml.Marshal(delta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "pay", "features", "refund", "tasks", "delta", "task.yml"), data, 0o644))

	err = tryTransition(t, root, deps, store.NounTask, "pay/refund/delta", mutate.TransitionInput{Verb: "start"})
	var dep *mutate.TaskDependencyBlockedError
	require.ErrorAs(t, err, &dep)
	require.False(t, dep.AggregateUnrecorded)
	require.Equal(t, "tiller/reviews/features/pay/refund", dep.Branch, "the start minted this feature's aggregate")
	require.Contains(t, dep.Error(), "wait for integration")
	require.NotContains(t, dep.Error(), "--adopt-aggregate")
}

func TestStartDependencyRefusalInProgressPrerequisiteKeepsWaitText(t *testing.T) {
	t.Parallel()
	root, deps := adoptFeatureGrainFixture(t, "alfa", "bravo")
	_, err := mutate.Link(load(t, root), deps, mutate.LinkInput{Noun: store.NounTask, Node: "pay/checkout/bravo", DependsOn: []string{"pay/checkout/alfa"}})
	require.NoError(t, err)
	tree := load(t, root)
	alfa := resolveNode(t, tree, store.NounTask, "pay/checkout/alfa").(*store.Task)
	alfa.Status, alfa.Started, alfa.Branch = "in_progress", true, "tiller/pay/checkout/alfa"
	data, err := yaml.Marshal(alfa)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "tasks", "alfa", "task.yml"), data, 0o644))
	driveWith(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "approve"})

	err = tryTransition(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start"})
	var dep *mutate.TaskDependencyBlockedError
	require.ErrorAs(t, err, &dep)
	require.False(t, dep.AggregateUnrecorded)
	require.Contains(t, dep.Error(), "wait for integration")
}
