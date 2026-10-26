package views_test

// These tests pin ranked next's grain-aware moves (task-5-brief.md,
// docs/spec/cli-surface.md): a coarse-grain feature's own
// finish line (gates, then submit) ranks ahead of the generic in_review wait,
// an in_review task under an unrecorded coarse-grain PR is not a human wait
// at all, and once that PR IS recorded the wait names the reviewed node's
// own merge rather than the task's. Fixtures build the store structs
// directly, the same pattern routeArmFixtures (readout_test.go) uses.

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// TestNextCoarseFeatureMidRunRoutesToNextReadyTaskUnchanged proves the
// coarse finish-line arm does not shadow the existing ready-task arm: a
// feature-grain feature with one ready task still in hand and one task
// already in_review routes straight to the ready task, exactly as it always
// has, rather than jumping ahead to a finish-line reason.
func TestNextCoarseFeatureMidRunRoutesToNextReadyTaskUnchanged(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	f.ReviewGrain = store.ReviewGrainFeature
	tree.Features["F-aaaaa0"] = f

	ready := &store.Task{ID: "T-ready0", Status: "ready", Owner: "jc", FeatureID: f.ID}
	inReview := &store.Task{ID: "T-revie0", Status: "in_review", FeatureID: f.ID}
	tree.Tasks = map[string]*store.Task{ready.ID: ready, inReview.ID: inReview}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {ready, inReview}}

	next := nextFor(tree, "jc", workflowContracts(tree))

	require.Equal(t, "continue", next.Candidates[0].Kind)
	require.Equal(t, "T-ready0", next.Candidates[0].Node.ID, "the ready task still wins over the coarse finish line")
	require.Empty(t, workflowActions(next), "the feature has not reached its finish line")
}

// Missing required aggregate records need check, not a redundant eval loop.
func TestNextCoarseFinishLineChecksUnrecordedGates(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	f.ReviewGrain = store.ReviewGrainFeature
	tree.Features["F-aaaaa0"] = f
	tree.Manifest.Gates.Required = map[string]store.GateSpec{
		"evals": {Kind: "command", On: store.StringList{"feature"}},
	}

	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: f.ID}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {task}}

	top := topMove(t, tree, "jc", workflowContracts(tree))

	require.Equal(t, "execute", top.Skill)
	require.Equal(t, "check", top.Workflow.Action)
	require.Equal(t, "tiller check feature "+f.ID, top.Workflow.Command)
	require.Equal(t, store.NounFeature, top.Node.Noun)
	require.Equal(t, f.ID, top.Node.ID)
}

// TestNextCoarseFinishLineNamesSubmitWhenGatesGreen is the finish-line
// arm's other branch: the same all-in_review feature, but its required gate
// is now recorded pass, so the top move names the submit instead of the gates.
func TestNextCoarseFinishLineNamesSubmitWhenGatesGreen(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	f.ReviewGrain = store.ReviewGrainFeature
	f.GatesBlock = store.GatesBlock{Gates: map[string]store.Gate{
		"evals": {Required: true, Status: store.GateStatusPass},
	}}
	tree.Features["F-aaaaa0"] = f
	tree.Manifest.Gates.Required = map[string]store.GateSpec{
		"evals": {Kind: "command", On: store.StringList{"feature"}},
	}

	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: f.ID}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {task}}

	top := topMove(t, tree, "jc", workflowContracts(tree))

	require.Equal(t, "execute", top.Skill)
	require.Equal(t, "submit", top.Workflow.Action)
	require.True(t, top.Workflow.AcceptanceReady)
}

// TestNextCoarseWaitNamesAwaitingYourMergeOnceFeaturePRRecorded is the wait
// arm's grain-aware text: once the feature's own PR is recorded, an
// in_review task under it IS a human wait again, but the wording names the
// feature's PR rather than the task's, and the finish-line arm above no
// longer intercepts (the feature has already submitted).
func TestNextCoarseWaitNamesAwaitingYourMergeOnceFeaturePRRecorded(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	f.ReviewGrain = store.ReviewGrainFeature
	pr := 42
	f.PR = &pr
	tree.Features["F-aaaaa0"] = f

	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: f.ID}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {task}}

	next := nextFor(tree, "jc", workflowContracts(tree))

	top := next.Candidates[0]
	require.Equal(t, "human", top.Actor, "a merge wait is the human's move, not a seam")
	require.Equal(t, task.ID, top.Node.ID)
	require.Equal(t, "rides the review of epic-0/loading (F-aaaaa0); PR #42 awaits your merge", top.Reason)
	require.NotContains(t, workflowActions(next), "check")
	require.NotContains(t, workflowActions(next), "submit")
}

// TestNextCoarseWaitNamesAwaitingYourMergeForAnEpicOneLevelUp is the same
// wait text one grain up: an epic-grain epic with its own PR recorded, and a
// task under one of its features sitting in_review, waits on the epic's own
// merge.
func TestNextCoarseWaitNamesAwaitingYourMergeForAnEpicOneLevelUp(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	epic := tree.Epics["E-aaaaa0"]
	epic.Status = "ready"
	epic.ReviewGrain = store.ReviewGrainEpic
	pr := 7
	epic.PR = &pr

	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	tree.Features["F-aaaaa0"] = f

	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: f.ID}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {task}}

	top := topMove(t, tree, "jc", workflowContracts(tree))

	require.Equal(t, "human", top.Actor)
	require.Equal(t, task.ID, top.Node.ID)
	require.Equal(t, "rides the review of epic-0 (E-aaaaa0); PR #7 awaits your merge", top.Reason)
}

// An unrecorded aggregate is not yet a human merge wait. Its ready unclaimed
// sibling remains available as an individual task under the active feature.
func TestNextSkipsACoarseInReviewTaskWithAnUnrecordedGrainPR(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	epic := tree.Epics["E-aaaaa0"]
	epic.Status = "ready"
	epic.ReviewGrain = store.ReviewGrainEpic

	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	tree.Features["F-aaaaa0"] = f

	reviewing := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: f.ID}
	notReady := &store.Task{ID: "T-bbbbb0", Status: "ready", FeatureID: f.ID}
	tree.Tasks = map[string]*store.Task{reviewing.ID: reviewing, notReady.ID: notReady}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {reviewing, notReady}}

	next := nextFor(tree, "jc", workflowContracts(tree))

	require.Equal(t, "claim_task", next.Candidates[0].Kind)
	require.Equal(t, notReady.ID, next.Candidates[0].Node.ID)
	require.Empty(t, candidatesOfKind(next.Candidates, "merge_task"), "an unrecorded aggregate is no one's merge")
}

// Fix round 1 (code review findings 1 and 2).

// TestNextCoarseFinishLineSuppressedUnderARetiredAncestorEpic is finding 1:
// a feature-grain feature whose own epic has been cancelled must not answer
// the coarse finish line, the same gen.RetiredAncestor guard the
// specced-feature wait arm (readout.go) and queueFeatureChecks (queue.go)
// already apply, because `tiller check feature`/`tiller eval feature`/
// `tiller submit feature` now refuse outright under a retired ancestor
// (transition.go's guardFeature epicRetired arm), and a move naming
// a command that would refuse is worse than naming nothing.
func TestNextCoarseFinishLineSuppressedUnderARetiredAncestorEpic(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "cancelled"
	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	f.ReviewGrain = store.ReviewGrainFeature
	tree.Features["F-aaaaa0"] = f

	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: f.ID}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {task}}

	next := nextFor(tree, "jc", workflowContracts(tree))

	require.Empty(t, workflowActions(next),
		"a retired ancestor epic suppresses the finish line the same way it suppresses queueFeatureChecks")
}

// The epic finish line names the riding feature whose check remains.
func TestNextCoarseEpicFinishLineChecksUnrecordedGates(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	epic := tree.Epics["E-aaaaa0"]
	epic.Status = "ready"
	epic.ReviewGrain = store.ReviewGrainEpic
	tree.Manifest.Gates.Required = map[string]store.GateSpec{
		"evals": {Kind: "command", On: store.StringList{"feature"}},
	}

	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	tree.Features["F-aaaaa0"] = f
	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: f.ID}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {task}}

	top := topMove(t, tree, "jc", workflowContracts(tree))

	require.Equal(t, "execute", top.Skill)
	require.Equal(t, "check", top.Workflow.Action)
	require.Equal(t, f.ID, top.Workflow.FeatureID)
	require.Equal(t, store.NounEpic, top.Node.Noun)
	require.Equal(t, epic.ID, top.Node.ID)
}

// TestNextCoarseEpicFinishLineNamesSubmitWhenGatesGreen is the epic arm's
// other phase: the same all-ready epic, but the live feature's required
// gate is now recorded pass, so the top move names the epic's own submit instead.
func TestNextCoarseEpicFinishLineNamesSubmitWhenGatesGreen(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	epic := tree.Epics["E-aaaaa0"]
	epic.Status = "ready"
	epic.ReviewGrain = store.ReviewGrainEpic
	tree.Manifest.Gates.Required = map[string]store.GateSpec{
		"evals": {Kind: "command", On: store.StringList{"feature"}},
	}

	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	f.GatesBlock = store.GatesBlock{Gates: map[string]store.Gate{
		"evals": {Required: true, Status: store.GateStatusPass},
	}}
	tree.Features["F-aaaaa0"] = f
	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: f.ID}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {task}}

	top := topMove(t, tree, "jc", workflowContracts(tree))

	require.Equal(t, "execute", top.Skill)
	require.Equal(t, "submit", top.Workflow.Action)
	require.Equal(t, "tiller guide execute", top.Workflow.Command)
}

// TestNextCoarseEpicFinishLineDoesNotFireWhileAFeatureIsStillInProgress is
// the epic arm's no-arm case: one live feature under the epic has reached
// in_review, but a second live feature still has a task in_progress, so the
// epic has not reached its own finish line (nothing else is legal in this
// fixture, so the reader is caught up).
func TestNextCoarseEpicFinishLineDoesNotFireWhileAFeatureIsStillInProgress(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	epic := tree.Epics["E-aaaaa0"]
	epic.Status = "ready"
	epic.ReviewGrain = store.ReviewGrainEpic

	ready := feat(t, "F-ready00", "epic-0/loading", "epic-0")
	ready.Status = "active"
	tree.Features[ready.ID] = ready
	inFlight := feat(t, "F-inflght", "epic-0/next", "epic-0")
	inFlight.Status = "active"
	tree.Features[inFlight.ID] = inFlight

	doneTask := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: ready.ID}
	stillGoing := &store.Task{ID: "T-bbbbb0", Status: "in_progress", FeatureID: inFlight.ID}
	tree.Tasks = map[string]*store.Task{doneTask.ID: doneTask, stillGoing.ID: stillGoing}
	tree.FeatureTasks = map[string][]*store.Task{ready.ID: {doneTask}, inFlight.ID: {stillGoing}}

	next := nextFor(tree, "jc", workflowContracts(tree))

	require.True(t, next.CaughtUp, "one feature still in progress means the epic has not reached its own finish line yet")
}

// TestNextCoarseEpicFinishLineSuppressedWhenEpicItselfIsRetired proves the
// epic arm reuses finding 1's guard rather than a bespoke epic-status check:
// a cancelled epic's own live-feature walk (epicLiveFeaturesForRoute)
// excludes every feature under it via gen.RetiredAncestor, so the arm never
// fires for a retired epic even though its ReviewGrain still resolves to
// epic.
func TestNextCoarseEpicFinishLineSuppressedWhenEpicItselfIsRetired(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	epic := tree.Epics["E-aaaaa0"]
	epic.Status = "cancelled"
	epic.ReviewGrain = store.ReviewGrainEpic

	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	tree.Features["F-aaaaa0"] = f
	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: f.ID}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {task}}

	require.Empty(t, workflowActions(nextFor(tree, "jc", workflowContracts(tree))))
}

func TestNextCoarseEpicFinishLineAllowsIndependentTaskOwners(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	epic := tree.Epics["E-aaaaa0"]
	epic.Status = "ready"
	epic.ReviewGrain = store.ReviewGrainEpic

	alpha := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	alpha.Status = "active"
	beta := feat(t, "F-bbbbb0", "epic-0/saving", "epic-0")
	beta.Status = "active"
	tree.Features[alpha.ID] = alpha
	tree.Features[beta.ID] = beta
	alphaTask := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: alpha.ID, Owner: "worker-a", Started: true}
	betaTask := &store.Task{ID: "T-bbbbb0", Status: "in_review", FeatureID: beta.ID, Owner: "worker-b", Started: true}
	tree.Tasks = map[string]*store.Task{alphaTask.ID: alphaTask, betaTask.ID: betaTask}
	tree.FeatureTasks = map[string][]*store.Task{alpha.ID: {alphaTask}, beta.ID: {betaTask}}

	for _, owner := range []string{"worker-a", "worker-b", "unrelated-reviewer"} {
		top := topMove(t, tree, owner, workflowContracts(tree))
		require.Equal(t, epic.ID, top.Node.ID)
		require.Equal(t, "submit", top.Workflow.Action)
	}
}

func TestNextLegacyTaskPRWinsOverCurrentCoarseGrain(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	f.ReviewGrain = store.ReviewGrainFeature
	tree.Features[f.ID] = f

	pr := 41
	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: f.ID, PR: &pr}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {task}}

	next := nextFor(tree, "jc", workflowContracts(tree))

	top := next.Candidates[0]
	require.Equal(t, "merge_task", top.Kind)
	require.Equal(t, "T-aaaaa0", top.Node.ID)
	require.Equal(t, "PR #41 awaits your merge", top.Reason)
	require.NotContains(t, workflowActions(next), "submit")
}

func TestNextCoarseEpicFinishLineWaitsForActiveOverrides(t *testing.T) {
	for _, grain := range []string{store.ReviewGrainTask, store.ReviewGrainFeature} {
		t.Run(grain, func(t *testing.T) {
			tree := treeWithDraftEpics(t, 1)
			epic := tree.Epics["E-aaaaa0"]
			epic.Status = "ready"
			epic.ReviewGrain = store.ReviewGrainEpic

			alpha := feat(t, "F-aaaaa0", "epic-0/alpha", "epic-0")
			alpha.Status = "active"
			beta := feat(t, "F-bbbbb0", "epic-0/beta", "epic-0")
			beta.Status = "active"
			beta.ReviewGrain = grain
			if grain == store.ReviewGrainFeature {
				pr := 88
				beta.PR = &pr
			}
			tree.Features = map[string]*store.Feature{alpha.ID: alpha, beta.ID: beta}

			alphaTask := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: alpha.ID}
			betaTask := &store.Task{ID: "T-bbbbb0", Status: "in_review", FeatureID: beta.ID}
			if grain == store.ReviewGrainTask {
				pr := 41
				betaTask.PR = &pr
			}
			tree.Tasks = map[string]*store.Task{alphaTask.ID: alphaTask, betaTask.ID: betaTask}
			tree.FeatureTasks = map[string][]*store.Task{
				alpha.ID: {alphaTask},
				beta.ID:  {betaTask},
			}

			next := nextFor(tree, "jc", workflowContracts(tree))
			require.NotEmpty(t, next.Candidates)
			require.Equal(t, "merge_task", next.Candidates[0].Kind, "the override's own review is what waits")
			require.Equal(t, betaTask.ID, next.Candidates[0].Node.ID)
			for _, c := range next.Candidates {
				if c.Node != nil && c.Node.ID == epic.ID && c.Workflow != nil {
					require.NotEqual(t, "submit", c.Workflow.Action,
						"an active override must finish its own review before the epic finish line appears")
				}
			}
		})
	}
}

func TestNextCoarseEpicFinishLineIgnoresCompletedOverrideReadiness(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	epic := tree.Epics["E-aaaaa0"]
	epic.Status = "ready"
	epic.ReviewGrain = store.ReviewGrainEpic

	alpha := feat(t, "F-aaaaa0", "epic-0/alpha", "epic-0")
	alpha.Status = "active"
	beta := feat(t, "F-bbbbb0", "epic-0/beta", "epic-0")
	beta.Status = "done"
	beta.ReviewGrain = store.ReviewGrainFeature
	tree.Features = map[string]*store.Feature{alpha.ID: alpha, beta.ID: beta}

	alphaTask := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: alpha.ID}
	betaTask := &store.Task{ID: "T-bbbbb0", Status: "ready", FeatureID: beta.ID}
	tree.Tasks = map[string]*store.Task{alphaTask.ID: alphaTask, betaTask.ID: betaTask}
	tree.FeatureTasks = map[string][]*store.Task{
		alpha.ID: {alphaTask},
		beta.ID:  {betaTask},
	}

	top := topMove(t, tree, "jc", workflowContracts(tree))

	require.Equal(t, "execute", top.Skill)
	require.Equal(t, epic.ID, top.Node.ID)
	require.Equal(t, "submit", top.Workflow.Action)
}

func TestNextCoarseFinishLinesAllowDoneTaskRetainingHistoricalPR(t *testing.T) {
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			tree := treeWithDraftEpics(t, 1)
			epic := tree.Epics["E-aaaaa0"]
			epic.Status = "ready"
			if grain == store.ReviewGrainEpic {
				epic.ReviewGrain = store.ReviewGrainEpic
			}

			f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
			f.Status = "active"
			if grain == store.ReviewGrainFeature {
				f.ReviewGrain = store.ReviewGrainFeature
			}
			tree.Features[f.ID] = f

			pr := 41
			task := &store.Task{ID: "T-aaaaa0", Status: "done", FeatureID: f.ID, PR: &pr}
			tree.Tasks = map[string]*store.Task{task.ID: task}
			tree.FeatureTasks = map[string][]*store.Task{f.ID: {task}}

			top := topMove(t, tree, "jc", workflowContracts(tree))

			require.Equal(t, "execute", top.Skill)
			require.Equal(t, grain, top.Node.Noun)
			require.Equal(t, "submit", top.Workflow.Action)
		})
	}
}

// workflowActions lists the workflow action of every ranked move that
// carries workflow advice, in rank order.
func workflowActions(next views.Next) []string {
	out := []string{}
	for _, c := range next.Candidates {
		if c.Workflow != nil {
			out = append(out, c.Workflow.Action)
		}
	}
	return out
}
