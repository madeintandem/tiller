package mutate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// These tests cover started grain: start persists the grain a task resolved, and a
// task started at task grain keeps it through a later coarse setting. It takes
// its own PR, merges, and syncs; the aggregate never absorbs it.

func readTaskYML(t *testing.T, root, taskKey string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "tasks", taskKey, "task.yml"))
	require.NoError(t, err)
	return string(data)
}

// staleTaskGrainFixture is the #146 timeline: alfa starts and checks at task
// grain on its own branch, then a commit sets the feature to feature grain.
// bravo depends on alfa. HEAD is left on alfa's branch.
func staleTaskGrainFixture(t *testing.T) (string, mutate.Deps, string) {
	t.Helper()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "alfa", "bravo")
	_, err := mutate.Link(load(t, root), deps, mutate.LinkInput{Noun: store.NounTask, Node: "pay/checkout/bravo", DependsOn: []string{"pay/checkout/alfa"}})
	require.NoError(t, err)
	gitCommitAllM(t, root, "plan")

	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "approve"})
	started := driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})
	require.Empty(t, started.AggregateBranch)
	gitRunM(t, root, "checkout", "-b", started.Branch)
	tree := load(t, root)
	res, err := mutate.Check(tree, deps, stubRev(root), resolveNode(t, tree, store.NounTask, "pay/checkout/alfa"), mutate.CheckInput{})
	require.NoError(t, err)
	require.True(t, res.Passed)
	gitCommitAllM(t, root, "alfa checked")

	feature := featureByKey(load(t, root), "pay/checkout")
	feature.ReviewGrain = "feature"
	data, err := yaml.Marshal(feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "feature.yml"), data, 0o644))
	gitCommitAllM(t, root, "grain merged")
	return root, deps, started.Branch
}

func TestStartRecordsStartedGrain(t *testing.T) {
	t.Parallel()
	root, deps := adoptFeatureGrainFixture(t, "alfa")
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})
	require.Contains(t, readTaskYML(t, root, "alfa"), "started_grain: feature")
	require.Equal(t, "feature", resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task).StartedGrain)
}

func TestStaleTaskGrainTaskSubmitsAsTaskGrainAfterGrainMerge(t *testing.T) {
	t.Parallel()
	root, deps, _ := staleTaskGrainFixture(t)
	require.Contains(t, readTaskYML(t, root, "alfa"), "started_grain: task")

	pr := 7
	res := driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "submit", PR: &pr})
	require.Equal(t, "in_review", res.Status)
	alfa := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").(*store.Task)
	require.NotNil(t, alfa.PR)
	require.Equal(t, 7, *alfa.PR)
	require.Empty(t, featureByKey(load(t, root), "pay/checkout").Branch, "a task-grain task records no aggregate")
}

// After its own PR merges, sync crosses the task done on its own branch's
// merge, not on an aggregate the task never rode.
func TestStaleTaskGrainTaskCrossesDoneOnItsOwnMerge(t *testing.T) {
	t.Parallel()
	root, deps, branch := staleTaskGrainFixture(t)
	pr := 7
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "submit", PR: &pr})
	gitCommitAllM(t, root, "alfa submitted")
	gitRunM(t, root, "checkout", "main")
	gitRunM(t, root, "merge", "--no-ff", "-m", "merge alfa PR", branch)

	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "done", resolveNode(t, load(t, root), store.NounTask, "pay/checkout/alfa").NodeStatus())
}

func TestDependentOfStaleTaskGrainPrerequisiteSaysWaitForMerge(t *testing.T) {
	t.Parallel()
	root, deps, _ := staleTaskGrainFixture(t)
	pr := 7
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "submit", PR: &pr})
	gitCommitAllM(t, root, "alfa submitted")
	driveWith(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "approve"})

	err := tryTransition(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start"})
	var dep *mutate.TaskDependencyBlockedError
	require.ErrorAs(t, err, &dep)
	require.Equal(t, "task", dep.DependencyStartedGrain)
	require.False(t, dep.AggregateUnrecorded, "a task-grain prerequisite never enters the aggregate, so no unrecorded aggregate can unblock it")
	require.Contains(t, dep.Error(), "wait for its PR to merge and sync")
	require.NotContains(t, dep.Error(), "--adopt-aggregate")
	require.NotContains(t, dep.Error(), "assemble")
	require.Empty(t, featureByKey(load(t, root), "pay/checkout").Branch, "a refused start writes nothing")
}

// A task-grain prerequisite never enters the aggregate, even when someone
// merged its branch into one by hand.
func TestStaleTaskGrainPrerequisiteNeverBasesTheAggregate(t *testing.T) {
	t.Parallel()
	root, deps, branch := staleTaskGrainFixture(t)
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "submit"})
	gitCommitAllM(t, root, "alfa submitted")
	gitRunM(t, root, "branch", adoptAgg, branch)
	driveWith(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "approve"})

	err := tryTransition(t, root, deps, store.NounTask, "pay/checkout/bravo", mutate.TransitionInput{Verb: "start", AdoptAggregate: true})
	var dep *mutate.TaskDependencyBlockedError
	require.ErrorAs(t, err, &dep)
	require.Equal(t, "task", dep.DependencyStartedGrain)
}

// soloStaleTaskGrainFixture is staleTaskGrainFixture with alfa alone and the
// coarse grain set on grainNoun (feature or epic). alfa is left submitted
// remoteless, holding only a review_ref.
func soloStaleTaskGrainFixture(t *testing.T, grainNoun string) (string, mutate.Deps) {
	t.Helper()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "alfa")
	gitCommitAllM(t, root, "plan")
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "approve"})
	started := driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})
	gitRunM(t, root, "checkout", "-b", started.Branch)
	tree := load(t, root)
	res, err := mutate.Check(tree, deps, stubRev(root), resolveNode(t, tree, store.NounTask, "pay/checkout/alfa"), mutate.CheckInput{})
	require.NoError(t, err)
	require.True(t, res.Passed)
	gitCommitAllM(t, root, "alfa checked")
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "submit"})
	gitCommitAllM(t, root, "alfa submitted")

	if grainNoun == store.NounEpic {
		epic := epicByKey(load(t, root), "pay")
		epic.ReviewGrain = "epic"
		data, err := yaml.Marshal(epic)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "pay", "epic.yml"), data, 0o644))
	} else {
		feature := featureByKey(load(t, root), "pay/checkout")
		feature.ReviewGrain = "feature"
		data, err := yaml.Marshal(feature)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "feature.yml"), data, 0o644))
	}
	gitCommitAllM(t, root, "grain merged")
	rgCheckFeatureGreen(t, root, deps, "pay", "checkout")
	gitCommitAllM(t, root, "feature checked")
	return root, deps
}

// A remoteless task-grain task holds a review_ref, not a PR. Coarse submit
// still refuses it as a task-owned review, never by asking for it to be
// assembled into the aggregate (R6: the aggregate never absorbs it).
func TestCoarseSubmitRefusesRemotelessTaskGrainReview(t *testing.T) {
	t.Parallel()
	for _, noun := range []string{store.NounFeature, store.NounEpic} {
		t.Run(noun, func(t *testing.T) {
			root, deps := soloStaleTaskGrainFixture(t, noun)
			key := "pay/checkout"
			if noun == store.NounEpic {
				key = "pay"
			}
			_, err := mutate.SubmitNode(load(t, root), deps, mutate.SubmitNodeInput{Noun: noun, Key: key, PR: intPtr(9)})
			var open *mutate.TaskReviewOpenError
			require.ErrorAs(t, err, &open, "%v", err)
			require.Nil(t, open.PR)
			require.Equal(t, "tiller/pay/checkout/alfa", open.ReviewBranch)
			require.Equal(t, store.ReviewGrainTask, open.StartedGrain)
			require.NotContains(t, err.Error(), "PR #")
			require.NotContains(t, err.Error(), "into the aggregate")
		})
	}
}

// A mixed feature with a recorded aggregate: bravo rides it, alfa started at
// task grain does not. bravo crosses on the aggregate's merge; alfa waits on
// its own branch and is never judged against the aggregate.
func TestSyncMixedFeatureJudgesEachTaskByItsOwnGrain(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := rgLayFeature(t, deps, "pay", "checkout", "alfa", "bravo")
	gitCommitAllM(t, root, "plan")
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "approve"})
	alfa := driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "start"})
	gitRunM(t, root, "checkout", "-b", alfa.Branch)
	tree := load(t, root)
	res, err := mutate.Check(tree, deps, stubRev(root), resolveNode(t, tree, store.NounTask, "pay/checkout/alfa"), mutate.CheckInput{})
	require.NoError(t, err)
	require.True(t, res.Passed)
	gitCommitAllM(t, root, "alfa checked")
	driveWith(t, root, deps, store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "submit"})
	gitCommitAllM(t, root, "alfa submitted")
	feature := featureByKey(load(t, root), "pay/checkout")
	feature.ReviewGrain = "feature"
	data, err := yaml.Marshal(feature)
	require.NoError(t, err)
	featurePath := filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "feature.yml")
	require.NoError(t, os.WriteFile(featurePath, data, 0o644))
	gitCommitAllM(t, root, "grain merged")

	rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "bravo", 2)
	agg := featureByKey(load(t, root), "pay/checkout").Branch
	require.Equal(t, adoptAgg, agg)
	feature = featureByKey(load(t, root), "pay/checkout")
	feature.ReviewRef = &store.ReviewRef{Branch: agg, Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + gitOutM(t, root, "rev-parse", "HEAD")}}
	data, err = yaml.Marshal(feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(featurePath, data, 0o644))
	gitCommitAllM(t, root, "feature review recorded")
	gitRunM(t, root, "checkout", "main")
	gitRunM(t, root, "merge", "--no-ff", "-m", "merge feature PR", agg)
	gitRunM(t, root, "checkout", alfa.Branch)
	commitFile(t, root, "alfa-followup.txt", "alfa's own PR moved on")
	gitRunM(t, root, "checkout", "main")

	sres, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	bravo := resolveNode(t, load(t, root), store.NounTask, "pay/checkout/bravo")
	require.Equal(t, []string{bravo.NodeID()}, crossedIDs(sres))
	var awaiting *mutate.Awaiting
	for i := range sres.Awaiting {
		if sres.Awaiting[i].Key == "pay/checkout/alfa" {
			awaiting = &sres.Awaiting[i]
		}
	}
	require.NotNil(t, awaiting)
	require.Equal(t, alfa.Branch, awaiting.Branch, "alfa waits on its own branch")
	require.False(t, awaiting.AggregateIncomplete, "alfa is never judged against the aggregate")
	require.False(t, awaiting.ReviewMissing)
	require.Equal(t, "active", featureByKey(load(t, root), "pay/checkout").Status, "alfa's own review still holds the rollup")
}

// Accepting a fast-forward of the aggregate cannot cross a task-grain task
// that someone merged into it by hand: nothing riding the aggregate crosses.
func TestSyncFastForwardAggregateIgnoresTaskGrainTask(t *testing.T) {
	t.Parallel()
	root, _, branch := staleTaskGrainFixture(t)
	driveWith(t, root, seqDeps(), store.NounTask, "pay/checkout/alfa", mutate.TransitionInput{Verb: "submit"})
	gitCommitAllM(t, root, "alfa submitted")
	gitRunM(t, root, "checkout", "-b", adoptAgg, branch)
	feature := featureByKey(load(t, root), "pay/checkout")
	feature.Branch = adoptAgg
	feature.ReviewRef = &store.ReviewRef{Branch: adoptAgg, Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + gitOutM(t, root, "rev-parse", "HEAD")}}
	data, err := yaml.Marshal(feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "feature.yml"), data, 0o644))
	gitCommitAllM(t, root, "feature review recorded")
	gitRunM(t, root, "checkout", "main")
	gitRunM(t, root, "merge", "--ff-only", adoptAgg)

	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{feature.ID}})
	var refused *mutate.AttestNotEligibleError
	require.ErrorAs(t, err, &refused)
	require.Equal(t, mutate.AttestReasonNothingToCross, refused.Nodes[0].Reason)
}

// R6 on the routing side: a remoteless task-grain task under a now-coarse
// feature holds its own review ref, not a PR. The aggregate never absorbs it,
// so neither assembly advice nor the riding-feature wording may name it; the
// route waits on the task's own merge.
func TestRemotelessTaskGrainReviewIsNeverAssembled(t *testing.T) {
	t.Parallel()
	root, _ := soloStaleTaskGrainFixture(t, store.NounFeature)
	tree := load(t, root)
	alfa := resolveNode(t, tree, store.NounTask, "pay/checkout/alfa").(*store.Task)
	require.Nil(t, alfa.PR)
	require.Equal(t, store.ReviewGrainTask, alfa.StartedGrain)
	feature := featureByKey(tree, "pay/checkout")
	in := mutate.ObserveWorkflow(tree)

	require.False(t, workflow.FeatureTasksReadyToSubmit(tree, feature), "a task-grain review holds the coarse submit")
	require.Empty(t, in.Assembly[feature.ID])
	if advice := workflow.AdviceOf(tree, feature, in); advice != nil {
		require.NotEqual(t, "assembly", advice.Phase, "%+v", advice)
		require.NotContains(t, advice.Reason, "assemble", "%+v", advice)
	}

	next := views.NextOf(tree, views.NextInput{Reader: alfa.Owner, Workflow: in})
	require.NotEmpty(t, next.Candidates)
	top := next.Candidates[0]
	require.Equal(t, "merge_task", top.Kind, "%+v", next.Candidates)
	require.Equal(t, alfa.ID, top.Node.ID, "%+v", next.Candidates)
	require.Contains(t, top.Reason, "waiting on your merge")
	for _, c := range next.Candidates {
		require.NotContains(t, c.Reason, "assemble", "%+v", c)
		if c.Workflow != nil {
			require.NotEqual(t, "assembly", c.Workflow.Phase, "%+v", c)
		}
	}

	readout := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: alfa.Owner, Workflow: in})
	var note string
	for _, entity := range readout.Entities {
		if strings.HasSuffix(entity.Ref, "pay/checkout/alfa") || strings.Contains(entity.Ref, alfa.ID) {
			note = entity.Note
		}
	}
	require.NotEmpty(t, note, "%+v", readout.Entities)
	require.NotContains(t, note, "riding feature review", note)
}
