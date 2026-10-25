package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// Issue #152 item 1: the plan gate refuses while an open question holds the
// plan, so the human answers at the gate instead of an agent waiting on the
// question mid-run. The refusal covers approve from specced (feature) and from
// proposed (task) and nothing else.

const planGateFeature = "form-engine/nrca-form-loading"

// requirePlanHeld asserts err is the plan-gate question refusal naming the
// feature, with the approve verb and the given questions.
func requirePlanHeld(t *testing.T, err error, questions ...string) {
	t.Helper()
	var blk *mutate.BlockedByQuestionsError
	require.ErrorAs(t, err, &blk)
	require.Equal(t, "F-aaaaaa", blk.NodeID)
	require.Equal(t, store.NounFeature, blk.Noun)
	require.Equal(t, "approve", blk.Verb)
	require.Equal(t, "specced", blk.CurrentState)
	got := make([]string, len(blk.Blockers))
	for i, b := range blk.Blockers {
		got[i] = b.Question
	}
	require.Equal(t, questions, got)
}

func TestApproveFeatureRefusesHeldPlan(t *testing.T) {
	t.Parallel()
	root := laySpeccedFeature(t)
	writeOpenQuestion(t, root, "Q-zzzzzz", "T-aaaaaa")
	before, err := events.List(load(t, root))
	require.NoError(t, err)

	err = tryTransition(t, root, testDeps(), store.NounFeature, planGateFeature, mutate.TransitionInput{Verb: "approve"})

	requirePlanHeld(t, err, "Q-zzzzzz")
	tree := load(t, root)
	require.Equal(t, "specced", tree.Features["F-aaaaaa"].Status, "a refused plan gate must not move the feature")
	require.Equal(t, "proposed", tree.Tasks["T-aaaaaa"].Status, "a refused plan gate must not move a task")
	after, err := events.List(tree)
	require.NoError(t, err)
	require.Equal(t, len(before), len(after), "a refused plan gate writes no event")
}

func TestApproveFeatureRefusesEpicQuestion(t *testing.T) {
	t.Parallel()
	root := laySpeccedFeature(t)
	writeOpenQuestion(t, root, "Q-zzzzzz", "E-aaaaaa")

	err := tryTransition(t, root, testDeps(), store.NounFeature, planGateFeature, mutate.TransitionInput{Verb: "approve"})

	requirePlanHeld(t, err, "Q-zzzzzz")
	tree := load(t, root)
	require.Equal(t, "specced", tree.Features["F-aaaaaa"].Status)
	require.Equal(t, "proposed", tree.Tasks["T-aaaaaa"].Status)
}

func TestApproveFeatureAfterAnswer(t *testing.T) {
	t.Parallel()
	root := laySpeccedFeature(t)
	writeOpenQuestion(t, root, "Q-zzzzzz", "T-aaaaaa")
	err := tryTransition(t, root, testDeps(), store.NounFeature, planGateFeature, mutate.TransitionInput{Verb: "approve"})
	requirePlanHeld(t, err, "Q-zzzzzz")

	_, err = mutate.AnswerQuestion(load(t, root), testDeps(), stubRev(root), mutate.AnswerInput{
		Node: "Q-zzzzzz", Answer: "Use the 2020-12 schema.",
	})
	require.NoError(t, err)

	res := driveTo(t, root, store.NounFeature, planGateFeature, mutate.TransitionInput{Verb: "approve"})
	require.Equal(t, "ready", res.Status)
	tree := load(t, root)
	require.Equal(t, "ready", tree.Features["F-aaaaaa"].Status)
	require.Equal(t, "ready", tree.Tasks["T-aaaaaa"].Status)
}

func TestApproveFeatureBrokenSpecReportedFirst(t *testing.T) {
	t.Parallel()
	root := laySpeccedFeature(t)
	writeOpenQuestion(t, root, "Q-zzzzzz", "T-aaaaaa")
	breakContract(t, root)

	err := tryTransition(t, root, testDeps(), store.NounFeature, planGateFeature, mutate.TransitionInput{Verb: "approve"})

	var ce *store.EvalContractError
	require.ErrorAs(t, err, &ce, "a broken spec is reported before a held plan: the spec has to be fixed regardless")
	var blk *mutate.BlockedByQuestionsError
	require.NotErrorAs(t, err, &blk)
}

// TestRefreshBasisIgnoresQuestions: --refresh-basis on a live feature is a
// correction move, so an open question on one of its tasks must not refuse it
// (the correction a mid-run answer needs, decision 6).
func TestRefreshBasisIgnoresQuestions(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	gitInitM(t, root)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, planGateFeature, mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, planGateFeature, mutate.TransitionInput{Verb: "approve"})
	gitCommitAllM(t, root, "lay tree")
	gitRunM(t, root, "branch", "-M", "main")
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status)

	// Move the basis, then hold one task with an open question.
	featDir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "nrca-form-loading")
	require.NoError(t, os.WriteFile(filepath.Join(featDir, "feature.md"),
		[]byte("# NRCA form loading\n\nRevised after the answer.\n"), 0o644))
	gitCommitAllM(t, root, "revise spec")
	writeOpenQuestion(t, root, "Q-zzzzzz", "T-aaaaaa")

	res := driveTo(t, root, store.NounFeature, planGateFeature, mutate.TransitionInput{Verb: "approve", RefreshBasis: true})

	require.True(t, res.Changed)
	require.Equal(t, "active", res.Status, "refresh-basis never moves the feature")
}

// TestReapproveReadyFeatureWithQuestion: the idempotent re-approve of a ready
// feature has no plan left to gate, so an open question does not turn it into a
// refusal.
func TestReapproveReadyFeatureWithQuestion(t *testing.T) {
	t.Parallel()
	root := laySpeccedFeature(t)
	driveTo(t, root, store.NounFeature, planGateFeature, mutate.TransitionInput{Verb: "approve"})
	writeOpenQuestion(t, root, "Q-zzzzzz", "F-aaaaaa")
	before, err := events.List(load(t, root))
	require.NoError(t, err)

	res := driveTo(t, root, store.NounFeature, planGateFeature, mutate.TransitionInput{Verb: "approve"})

	require.False(t, res.Changed, "re-approving a ready feature is a no-change success")
	require.Equal(t, "ready", res.Status)
	after, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Equal(t, len(before), len(after))
}

// TestApproveTaskRefusesHeldTask: a task added after the plan gate has its own
// plan gate (proposed to ready), and an open question naming it refuses that
// approve while a sibling with no question approves.
func TestApproveTaskRefusesHeldTask(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, planGateFeature, mutate.TransitionInput{Verb: "approve"})
	for _, key := range []string{"bravo", "charlie"} {
		_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
			Key: planGateFeature + "/" + key, Title: "Task " + key})
		require.NoError(t, err)
		writeTaskSpec(t, root, key, "## Approach\n\nA late addition: "+key+".")
	}
	tree := load(t, root)
	ids := map[string]string{}
	for id, tk := range tree.Tasks {
		ids[localKeyT(*tk.Key)] = id
	}
	require.Equal(t, "proposed", tree.Tasks[ids["bravo"]].Status)
	writeOpenQuestion(t, root, "Q-zzzzzz", ids["bravo"])

	err := tryTransition(t, root, deps, store.NounTask, ids["bravo"], mutate.TransitionInput{Verb: "approve"})

	var blk *mutate.BlockedByQuestionsError
	require.ErrorAs(t, err, &blk)
	require.Equal(t, ids["bravo"], blk.NodeID)
	require.Equal(t, store.NounTask, blk.Noun)
	require.Equal(t, "approve", blk.Verb)
	require.Equal(t, "proposed", blk.CurrentState)
	require.Len(t, blk.Blockers, 1)
	require.Equal(t, "Q-zzzzzz", blk.Blockers[0].Question)
	require.Equal(t, "proposed", load(t, root).Tasks[ids["bravo"]].Status, "a refused approve must not move the task")

	require.NoError(t, tryTransition(t, root, deps, store.NounTask, ids["charlie"], mutate.TransitionInput{Verb: "approve"}))
	require.Equal(t, "ready", load(t, root).Tasks[ids["charlie"]].Status)
}
