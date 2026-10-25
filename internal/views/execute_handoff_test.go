package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func handoffTree(t *testing.T) (*store.Tree, *store.Feature, *store.Task, *store.Task) {
	t.Helper()
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	feature := feat(t, "F-aaaaa0", "epic-0/handoff", "epic-0")
	feature.Status = "active"
	feature.Branch = "tiller/epic-0/handoff"
	tree.Features[feature.ID] = feature
	ready := &store.Task{ID: "T-aaaaa0", Key: strPtr("epic-0/handoff/alpha"), Status: "ready", Owner: "jc", FeatureID: feature.ID}
	checked := &store.Task{ID: "T-aaaaa1", Key: strPtr("epic-0/handoff/zeta"), Status: "check", Owner: "jc", FeatureID: feature.ID,
		GatesBlock: store.GatesBlock{Gates: map[string]store.Gate{
			"tests": {Required: true, Status: store.GateStatusPass, Evidence: []store.Evidence{{Kind: "command_output", Ref: "artifacts/tiller/gates/tests.txt"}}},
		}},
	}
	tree.Tasks = map[string]*store.Task{ready.ID: ready, checked.ID: checked}
	tree.FeatureTasks = map[string][]*store.Task{feature.ID: {ready, checked}}
	return tree, feature, ready, checked
}

func TestHandoffRoutePrioritizesCheckedWorkBeforeEarlierReadyWork(t *testing.T) {
	tree, _, ready, checked := handoffTree(t)
	checked.Status = "ready"
	require.Equal(t, ready.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
	checked.Status = "check"
	require.Equal(t, checked.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID, "checking work keeps its submission ahead of starting another task")
}

func TestHandoffRouteKeepsCheckedWorkBeforeReadySibling(t *testing.T) {
	tree, _, first, second := handoffTree(t)
	first.Status, second.Status = "check", "ready"
	require.Equal(t, first.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
}

func TestTaskDependenciesWaitForMergeButCoarseDependenciesRouteToAssembly(t *testing.T) {
	tree, feature, dependent, prerequisite := handoffTree(t)
	prerequisite.Status = "in_review"
	prerequisite.Branch = "tiller/epic-0/handoff/zeta"
	prerequisite.ReviewRef = &store.ReviewRef{Branch: prerequisite.Branch, Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:abc1234"}}
	dependent.DependsOn = []string{prerequisite.ID}
	taskGrain := nextFor(tree, "jc", workflow.Input{}).Candidates
	require.Empty(t, candidatesFor(taskGrain, dependent.ID), "task review must merge before dependent execution")

	feature.ReviewGrain = store.ReviewGrainFeature
	coarse := topMove(t, tree, "jc", workflow.Input{})
	require.Equal(t, dependent.ID, coarse.Node.ID, "execute assembles a submitted coarse prerequisite before starting the dependent")
	require.Equal(t, "continue", coarse.Kind)
	require.Equal(t, "execute", coarse.Skill)

	prerequisite.Status = "in_progress"
	require.Equal(t, prerequisite.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID, "requested changes must finish before dependent work")
}

func TestHandoffRoutePreservesOwnershipAndQuestionHolds(t *testing.T) {
	tree, _, ready, checked := handoffTree(t)
	checked.Owner = "pat"
	require.Equal(t, ready.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
	checked.Owner = "jc"
	question := &store.Question{ID: "Q-hold", Title: "Publishing scope", Status: "open", Blocks: []string{checked.ID}}
	tree.Questions = map[string]*store.Question{question.ID: question}
	held := nextFor(tree, "jc", workflow.Input{}).Candidates
	require.Equal(t, ready.ID, held[0].Node.ID)
	require.Empty(t, candidatesFor(held, checked.ID), "a question-held task is no move")
	require.Equal(t, []string{"answer_question"}, candidateKinds(candidatesFor(held, question.ID)), "the hold stays visible as its question's move")
	question.Status = "answered"
	require.Equal(t, checked.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
}

func TestHandoffRouteRecoversCheckedShelfWorkWithoutStartingCarriedWork(t *testing.T) {
	tree := shelfRouteTree()
	checked := tree.Chores["CH-aaaaaa"]
	checked.Owner, checked.Status, checked.ResolvedBy = "jc", "check", "F-aaaaaa"
	require.Equal(t, "B-aaaaaa", topMove(t, tree, "jc", workflow.Input{}).Node.ID)
	checked.ResolvedBy = ""
	require.Equal(t, checked.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
}

func TestHandoffReadoutIncludesCheckedEvidenceWithoutSubmission(t *testing.T) {
	tree, _, _, checked := handoffTree(t)
	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc"})
	require.Len(t, r.Entities, 1)
	require.Contains(t, r.Entities[0].Ref, checked.ID)
	require.Equal(t, "1 of 1", checkLabels(r)["Gates green with evidence"])
	require.Equal(t, "0 of 1", checkLabels(r)["Current submissions recorded"])
}

func TestHandoffReadoutDoesNotVerifyIncompleteRequiredEvidence(t *testing.T) {
	for name, gate := range map[string]store.Gate{
		"failed":        {Required: true, Status: store.GateStatusFail},
		"stale":         {Required: true, Status: store.GateStatusStale, Evidence: []store.Evidence{{Kind: "command_output", Ref: "old.txt"}}},
		"unevidenced":   {Required: true, Status: store.GateStatusPass},
		"optional only": {Required: false, Status: store.GateStatusPass, Evidence: []store.Evidence{{Kind: "command_output", Ref: "optional.txt"}}},
	} {
		t.Run(name, func(t *testing.T) {
			tree, _, _, checked := handoffTree(t)
			checked.Gates = map[string]store.Gate{"tests": gate}
			r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})
			require.Len(t, r.Entities, 1, "incomplete checks remain visible rather than vanishing from the population")
			require.Equal(t, "0 of 1", checkLabels(r)["Gates green with evidence"])
			require.Equal(t, "0 of 1", checkLabels(r)["Current submissions recorded"])
		})
	}
}

func TestHandoffReadoutRetainedPRDoesNotCountAsResubmission(t *testing.T) {
	tree, _, _, task := handoffTree(t)
	pr := 41
	task.PR, task.Status = &pr, "in_progress"
	before := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})
	require.Len(t, before.Entities, 1)
	require.Equal(t, "0 of 1", checkLabels(before)["Current submissions recorded"])
	task.Status = "in_review"
	after := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})
	require.Equal(t, "1 of 1", checkLabels(after)["Current submissions recorded"])
}

func TestHandoffReadoutCountsMixedStatesBeyondDisplayCap(t *testing.T) {
	tree, feature, resumed, checked := handoffTree(t)
	pr := 41
	resumed.Status, resumed.PR = "in_progress", &pr
	for _, id := range []string{"T-aaaaa2", "T-aaaaa3"} {
		task := &store.Task{ID: id, Status: "in_review", FeatureID: feature.ID, PR: &pr, GatesBlock: checked.GatesBlock}
		tree.Tasks[id] = task
		tree.FeatureTasks[feature.ID] = append(tree.FeatureTasks[feature.ID], task)
	}
	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})
	require.Len(t, r.Entities, 3)
	require.Equal(t, 1, r.Withheld)
	require.Contains(t, r.Entities[0].Ref, checked.ID, "the publication still owed is not hidden behind older reviews")
	require.Equal(t, "3 of 4", checkLabels(r)["Gates green with evidence"])
	require.Equal(t, "2 of 4", checkLabels(r)["Current submissions recorded"])
}

func TestHandoffReadoutIdentifiesCoarseReviewRatherThanInventingTaskPR(t *testing.T) {
	tree, feature, ready, riding := handoffTree(t)
	feature.ReviewGrain = store.ReviewGrainFeature
	ready.Status = "cancelled"
	riding.Status, riding.ReviewRef = "in_review", &store.ReviewRef{Branch: feature.Branch}
	before := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc", Workflow: workflowContracts(tree)})
	require.Len(t, before.Entities, 1)
	require.Contains(t, before.Entities[0].Note, feature.ID, "a riding ref must identify the actual review surface")
	require.Equal(t, "agent", topNext(t, before).Actor)
	pr := 42
	feature.PR = &pr
	after := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc", Workflow: workflowContracts(tree)})
	require.Contains(t, after.Entities[0].Note, "42", "the recorded parent PR is the reviewable artifact, not a new task PR")
}

func TestHandoffReadoutsShareCheckedRouteWithoutLosingCorrectionEvents(t *testing.T) {
	tree, _, _, checked := handoffTree(t)
	full := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc"})
	correction := views.CorrectionOfInput(tree, views.MomentExecute, views.CorrectionInput{Contributor: "jc", Events: []events.Event{
		eventAt(1, "check.task", []string{checked.ID}, map[string]any{"status": "in_progress"}, map[string]any{"status": "check"}),
	}})
	require.Equal(t, full.Next, correction.Next)
	top := topNext(t, full)
	require.Equal(t, checked.ID, top.Node.ID)
	require.Equal(t, "agent", top.Actor)
	require.Len(t, correction.Changed, 1)
	require.Contains(t, correction.Changed[0], checked.ID)
}

func TestHandoffCheckedPriorityRetainsTaskDependencyAndSoftOrder(t *testing.T) {
	tree := chainedProposedTasksTree(t)
	feature := tree.Features["F-aaaaa0"]
	for _, task := range tree.Tasks {
		task.Status, task.Owner = "check", "jc"
	}
	require.Equal(t, "T-aaaaa1", topMove(t, tree, "jc", workflow.Input{}).Node.ID)
	for _, task := range tree.Tasks {
		task.DependsOn = nil
	}
	feature.Order = []string{"T-aaaaa0", "T-aaaaa2", "T-aaaaa1"}
	require.Equal(t, "T-aaaaa0", topMove(t, tree, "jc", workflow.Input{}).Node.ID)
}

func TestHandoffReadoutIncludesShelfPhasesButExcludesLiveCarriers(t *testing.T) {
	tree := shelfRouteTree()
	pr := 41
	resumed := tree.Bugs["B-aaaaaa"]
	resumed.Status, resumed.PR = "in_progress", &pr
	checked := tree.Chores["CH-aaaaaa"]
	checked.Status, checked.Owner = "check", "pat"
	checked.Gates = map[string]store.Gate{
		"tests": {Required: true, Status: store.GateStatusPass, Evidence: []store.Evidence{{Kind: "command_output", Ref: "tests.txt"}}},
	}
	tree.Bugs["B-checked"] = &store.Bug{WorkItem: store.WorkItem{ID: "B-checked", Status: "check", Branch: "tiller/bugs/checked"}}
	tree.Bugs["B-carried"] = &store.Bug{WorkItem: store.WorkItem{
		ID: "B-carried", Status: "check", ResolvedBy: "F-aaaaaa", GatesBlock: checked.GatesBlock,
	}}
	tree.Chores["CH-submitted"] = &store.Chore{WorkItem: store.WorkItem{
		ID: "CH-submitted", Status: "in_review", GatesBlock: checked.GatesBlock,
		ReviewRef: &store.ReviewRef{Branch: "tiller/chores/submitted", Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:abc1234"}},
	}}
	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc",
		UnpushedReviews: []views.UnpushedReview{{Node: "B-checked", Commit: "abc1234", Upstream: "origin/tiller/bugs/checked"}},
	})
	require.Len(t, r.Entities, 3)
	require.Contains(t, r.Entities[0].Ref, "B-checked")
	require.Contains(t, r.Entities[1].Ref, checked.ID, "another owner's execution is still part of the project-wide report")
	require.Contains(t, r.Entities[2].Ref, resumed.ID)
	require.NotNil(t, r.Entities[0].Unpushed)
	require.Equal(t, "tiller/bugs/checked", r.Entities[0].Unpushed.Branch)
	require.Equal(t, 1, r.Withheld)
	require.Equal(t, "2 of 4", checkLabels(r)["Gates green with evidence"])
	require.Equal(t, "1 of 4", checkLabels(r)["Current submissions recorded"])
	tree.Features["F-aaaaaa"].Status = "cancelled"
	uncarried := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})
	require.Equal(t, "3 of 5", checkLabels(uncarried)["Gates green with evidence"])
	require.Equal(t, "1 of 5", checkLabels(uncarried)["Current submissions recorded"])
	require.Equal(t, 2, uncarried.Withheld)
}

func TestHandoffReadoutEmptyExecutionAndAbsentChecks(t *testing.T) {
	tree, _, ready, checked := handoffTree(t)
	checked.Gates = nil
	active := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})
	require.Len(t, active.Entities, 1)
	require.Equal(t, "0 of 1", checkLabels(active)["Gates green with evidence"])
	require.Equal(t, "0 of 1", checkLabels(active)["Current submissions recorded"])
	for _, status := range []string{"proposed", "done", "cancelled"} {
		checked.Status = status
		empty := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})
		require.Empty(t, empty.Entities)
		require.Zero(t, empty.Withheld)
		require.Equal(t, "0 of 0", checkLabels(empty)["Gates green with evidence"])
		require.Equal(t, "0 of 0", checkLabels(empty)["Current submissions recorded"])
	}
	require.Equal(t, "ready", ready.Status, "ready work never joined the execution population")
}

func TestHandoffReadoutIdentifiesEpicOfflineReviewAndCurrentTaskRef(t *testing.T) {
	tree, _, ready, riding := handoffTree(t)
	epic := tree.Epics["E-aaaaa0"]
	epic.ReviewGrain = store.ReviewGrainEpic
	ready.Status = "cancelled"
	riding.Status = "in_review"
	riding.ReviewRef = &store.ReviewRef{Branch: "tiller/epic-0", Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:abc1234"}}
	before := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc", Workflow: workflowContracts(tree)})
	require.Equal(t, "1 of 1", checkLabels(before)["Current submissions recorded"])
	require.Contains(t, before.Entities[0].Note, epic.ID)
	require.NotContains(t, before.Entities[0].Note, "PR #")
	require.Equal(t, "agent", topNext(t, before).Actor)
	epic.ReviewRef = &store.ReviewRef{Branch: "tiller/epic-0", Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:def5678"}}
	after := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc", Workflow: workflowContracts(tree)})
	require.Contains(t, after.Entities[0].Note, epic.ID)
	require.Contains(t, after.Entities[0].Note, "def5678")
	require.NotContains(t, after.Entities[0].Note, "PR #")
	top := topNext(t, after)
	require.Equal(t, "human", top.Actor)
	require.Contains(t, top.Reason, epic.ReviewRef.Branch)
	require.NotContains(t, top.Reason, "PR")
}
