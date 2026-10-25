package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func readyFeatureWithTask(t *testing.T, tree *store.Tree, id, key, epicKey string) (*store.Feature, *store.Task) {
	t.Helper()
	f := feat(t, id, key, epicKey)
	f.Status = "ready"
	tree.Features[id] = f
	taskKey := key + "/first"
	task := &store.Task{ID: "T-" + id[2:], Key: &taskKey, Status: "ready", FeatureID: id}
	if tree.Tasks == nil {
		tree.Tasks = map[string]*store.Task{}
	}
	if tree.FeatureTasks == nil {
		tree.FeatureTasks = map[string][]*store.Task{}
	}
	tree.Tasks[task.ID] = task
	tree.FeatureTasks[id] = append(tree.FeatureTasks[id], task)
	return f, task
}

func holdClaimableNode(tree *store.Tree, questionID, nodeID string) {
	if tree.Questions == nil {
		tree.Questions = map[string]*store.Question{}
	}
	tree.Questions[questionID] = &store.Question{ID: questionID, Title: "Which schema?", Statement: "Which schema does this work read?", Status: "open", Blocks: []string{nodeID}}
}

func TestClaimableTaskOutranksUnrelatedPlanningButNotAmendmentReview(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	draftFeatureUnder(t, tree, "F-aaaaa1", "epic-1/planning", "epic-1")
	for _, owner := range []string{"jc", "other", ""} {
		top := topMove(t, tree, owner, workflow.Input{})
		require.Equal(t, "claim_task", top.Kind)
		require.Equal(t, task.ID, top.Node.ID)
		require.Equal(t, "tiller claim task "+*task.Key, top.Command)
		require.Contains(t, top.Reason, "tiller start task "+*task.Key)
	}
	f.Status = "active"
	require.Equal(t, "claim_task", topMove(t, tree, "jc", workflow.Input{}).Kind)
	a := &store.Amendment{ID: "A-aaaaa0", Status: store.AmendmentStatusProposed}
	tree.Amendments = map[string]*store.Amendment{a.ID: a}
	require.Equal(t, a.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
}

func TestClaimableTasksRespectDependenciesSoftOrderAndReservations(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f, first := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	second := &store.Task{ID: "T-bbbbb0", Key: strPtr("epic-0/work/second"), Status: "ready", FeatureID: f.ID}
	third := &store.Task{ID: "T-ccccc0", Key: strPtr("epic-0/work/third"), Status: "ready", FeatureID: f.ID, DependsOn: []string{first.ID}}
	tree.Tasks[second.ID], tree.Tasks[third.ID] = second, third
	tree.FeatureTasks[f.ID] = append(tree.FeatureTasks[f.ID], second, third)
	f.Order = []string{third.ID, second.ID, first.ID}
	route := func() string {
		return topMove(t, tree, "reader", workflow.Input{}).Node.ID
	}
	require.Equal(t, second.ID, route(), "unmet dependency wins over soft placement")
	second.Owner = "someone-else"
	require.Equal(t, first.ID, route(), "one reservation never reserves sibling tasks")
	first.Status, first.Started, first.Owner = "done", true, "historical-owner"
	require.Equal(t, third.ID, route())
	require.Equal(t, third.ID, views.StreamsOf(tree).Unclaimed[0].ID, "pool and routing share eligible task ordering")
}

func TestClaimableTaskHonorsParentAndTaskEligibility(t *testing.T) {
	for _, hold := range []string{"task question", "feature question", "epic question", "feature assumption", "task dependency", "feature dependency", "draft epic", "draft feature", "retired epic", "paused task", "paused feature", "task basis", "feature basis"} {
		t.Run(hold, func(t *testing.T) {
			tree := treeWithDraftEpics(t, 1)
			epic := tree.Epics["E-aaaaa0"]
			epic.Status = "ready"
			f, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
			in := workflow.Input{Eligibility: map[string]string{}}
			switch hold {
			case "task question":
				holdClaimableNode(tree, "Q-aaaaa0", task.ID)
			case "feature question", "feature assumption":
				holdClaimableNode(tree, "Q-aaaaa0", f.ID)
				if hold == "feature assumption" {
					f.AssumptionBasis = store.AssumptionBasis{"Q-aaaaa0": store.Assumption{}}
				}
			case "epic question":
				holdClaimableNode(tree, "Q-aaaaa0", epic.ID)
			case "task dependency":
				task.DependsOn = []string{"T-absent"}
			case "feature dependency":
				f.DependsOn = []string{"F-absent"}
			case "draft epic":
				epic.Status = "draft"
			case "draft feature":
				f.Status = "draft"
			case "retired epic":
				epic.Status = "cancelled"
			case "paused task":
				task.Blocked = true
			case "paused feature":
				f.Blocked = true
			case "task basis":
				in.Eligibility[task.ID] = "basis changed"
			case "feature basis":
				in.Eligibility[f.ID] = "basis changed"
			}
			require.False(t, views.EligibleTask(tree, task, in))
			require.Empty(t, candidatesOfKind(nextFor(tree, "jc", in).Candidates, "claim_task"))
			require.Empty(t, views.StreamsOfInput(tree, in).Unclaimed)
		})
	}
}

func TestCheckedAssignmentPrecedesUnclaimedSiblingAndHistoricalContributorHasNoPersonalWork(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f, ready := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	f.Status = "active"
	checked := &store.Task{ID: "T-bbbbb0", Status: "check", Owner: "worker", Started: true, FeatureID: f.ID}
	historical := &store.Task{ID: "T-ccccc0", Status: "cancelled", Owner: "former", Started: true, FeatureID: f.ID}
	tree.Tasks[checked.ID], tree.Tasks[historical.ID] = checked, historical
	tree.FeatureTasks[f.ID] = append(tree.FeatureTasks[f.ID], checked, historical)
	tree.Contributors = map[string][]store.Contributor{f.ID: {{ID: "former", Name: "Former"}, {ID: "worker", Name: "Worker"}}}
	mine := topMove(t, tree, "worker", workflow.Input{})
	require.Equal(t, "continue", mine.Kind)
	require.Equal(t, checked.ID, mine.Node.ID)
	former := topMove(t, tree, "former", workflow.Input{})
	require.Equal(t, ready.ID, former.Node.ID)
	require.Equal(t, "claim_task", former.Kind, "a shared offer does not assign the historical contributor another owner's work")
}

func executionPreferenceTree(t *testing.T) (*store.Tree, *store.Task, *store.Task) {
	t.Helper()
	tree := treeWithDraftEpics(t, 1)
	epic := tree.Epics["E-aaaaa0"]
	epic.Status = "ready"
	foundation, foundationTask := readyFeatureWithTask(t, tree, "F-found0", "epic-0/foundation", "epic-0")
	pipeline, pipelineTask := readyFeatureWithTask(t, tree, "F-pipel0", "epic-0/pipeline", "epic-0")
	webhooks, webhookTask := readyFeatureWithTask(t, tree, "F-hooks0", "epic-0/webhooks", "epic-0")
	editor, editorTask := readyFeatureWithTask(t, tree, "F-edito0", "epic-0/editor", "epic-0")
	history, historyTask := readyFeatureWithTask(t, tree, "F-histo0", "epic-0/history", "epic-0")
	pipeline.DependsOn = []string{foundation.ID}
	editor.DependsOn = []string{pipeline.ID}
	webhooks.DependsOn = []string{pipeline.ID}
	history.DependsOn = []string{webhooks.ID}
	for _, f := range []*store.Feature{foundation, pipeline, webhooks} {
		f.Status = "done"
	}
	for _, task := range []*store.Task{foundationTask, pipelineTask, webhookTask} {
		task.Status = "done"
	}
	epic.Order = []string{history.ID, editor.ID}
	return tree, editorTask, historyTask
}

func TestExecutionPreferenceAfterCompletedPrerequisites(t *testing.T) {
	for _, owner := range []string{"", "jc"} {
		t.Run("owner="+owner, func(t *testing.T) {
			tree, editor, history := executionPreferenceTree(t)
			editor.Owner, history.Owner = owner, owner
			order := gen.DependencyOrder(tree)
			before := order.Features("epic-0")
			require.True(t, order.Less(store.NounTask, editor.ID, history.ID), "historical prerequisite depth remains intact")
			top := topMove(t, tree, "jc", workflow.Input{})
			require.Equal(t, history.ID, top.Node.ID, "completed prerequisite depth must not outrank recorded intent")
			if owner == "" {
				pool := views.UnclaimedOf(tree, order)
				require.Equal(t, []string{history.ID, editor.ID}, []string{pool[0].ID, pool[1].ID})
			}
			require.Equal(t, before, gen.DependencyOrder(tree).Features("epic-0"))
			require.Equal(t, []string{"F-hooks0"}, tree.Features[history.FeatureID].DependsOn)
		})
	}
}

func TestExecutionPreferenceSkipsHeldPreferredWork(t *testing.T) {
	for _, owner := range []string{"", "jc"} {
		for _, hold := range []string{"dependency", "question", "basis", "pause", "parent"} {
			t.Run(owner+"/"+hold, func(t *testing.T) {
				tree, editor, history := executionPreferenceTree(t)
				editor.Owner, history.Owner = owner, owner
				in := workflow.Input{}
				switch hold {
				case "dependency":
					tree.Features[history.FeatureID].DependsOn = []string{"F-unmet0"}
				case "question":
					holdClaimableNode(tree, "Q-histo0", history.FeatureID)
				case "basis":
					in.Eligibility = map[string]string{history.ID: "accepted basis changed"}
				case "pause":
					history.Blocked = true
				case "parent":
					tree.Features[history.FeatureID].Status = "specced"
				}
				next := nextFor(tree, "jc", in)
				require.Equal(t, editor.ID, next.Candidates[0].Node.ID)
				require.Empty(t, candidatesFor(next.Candidates, history.ID), "held preferred work is no move")
				if hold == "question" {
					require.Equal(t, []string{"answer_question"}, candidateKinds(candidatesFor(next.Candidates, "Q-histo0")),
						"the holding question is the move instead")
				}
			})
		}
	}
}

func TestExecutionPreferenceStaysWithinPriorityAndSiblingGroups(t *testing.T) {
	t.Run("checked first then preference within checked", func(t *testing.T) {
		tree, editor, history := executionPreferenceTree(t)
		editor.Owner, history.Owner = "jc", "jc"
		editor.Status = "check"
		require.Equal(t, editor.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
		history.Status = "check"
		require.Equal(t, history.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
	})
	t.Run("no preference retains historical traversal", func(t *testing.T) {
		tree, editor, _ := executionPreferenceTree(t)
		tree.Epics["E-aaaaa0"].Order = nil
		require.Equal(t, editor.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
	})
	t.Run("partial list and local task preference", func(t *testing.T) {
		tree, editor, history := executionPreferenceTree(t)
		epic := tree.Epics["E-aaaaa0"]
		epic.Order = []string{editor.FeatureID}
		historyFeature := tree.Features[history.FeatureID]
		historyFeature.Order = []string{history.ID}
		require.Equal(t, editor.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID, "local task ordinal cannot outrank another feature")
		extra := &store.Task{ID: "T-later0", Key: strPtr("epic-0/history/later"), FeatureID: history.FeatureID, Status: "ready", DependsOn: []string{history.ID}}
		tree.Tasks[extra.ID] = extra
		tree.FeatureTasks[history.FeatureID] = append(tree.FeatureTasks[history.FeatureID], extra)
		independent := &store.Task{ID: "T-indep0", Key: strPtr("epic-0/history/independent"), FeatureID: history.FeatureID, Status: "ready"}
		tree.Tasks[independent.ID] = independent
		tree.FeatureTasks[history.FeatureID] = append(tree.FeatureTasks[history.FeatureID], independent)
		history.Status = "done"
		historyFeature.Order = []string{extra.ID}
		epic.Order = []string{historyFeature.ID}
		require.Equal(t, extra.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
		pool := views.UnclaimedOf(tree, gen.DependencyOrder(tree))
		require.Equal(t, []string{extra.ID, independent.ID, editor.ID}, []string{pool[0].ID, pool[1].ID, pool[2].ID})
	})
	t.Run("cross epic preference", func(t *testing.T) {
		tree, editor, history := executionPreferenceTree(t)
		other := &store.Epic{ID: "E-other0", Key: strPtr("other"), Status: "ready"}
		tree.Epics[other.ID] = other
		tree.Features[history.FeatureID].EpicKey = "other"
		other.Order = []string{history.FeatureID}
		tree.ProjectOrder.Epics = []string{other.ID}
		require.Equal(t, history.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
		other.Blocked = true
		require.Equal(t, editor.ID, topMove(t, tree, "jc", workflow.Input{}).Node.ID)
	})
}

func TestExecutionPreferenceObservedQuestionHoldPreservesOtherGuards(t *testing.T) {
	for _, additionalHold := range []string{"", "feature basis", "task basis", "dependency"} {
		t.Run(additionalHold, func(t *testing.T) {
			tree, editor, history := executionPreferenceTree(t)
			tree.Root = t.TempDir()
			holdClaimableNode(tree, "Q-histo0", history.FeatureID)
			pin := store.RevisionRef{Kind: store.KindGitBlob, Value: "git:0123456789"}
			switch additionalHold {
			case "feature basis":
				tree.Features[history.FeatureID].AcceptedAgainst.Spec = pin
			case "task basis":
				history.AcceptedAgainst = &store.BasisRefs{Spec: pin}
			case "dependency":
				history.DependsOn = []string{"T-unmet0"}
			}
			in := mutate.ObserveWorkflow(tree)
			next := nextFor(tree, "jc", in)
			require.Equal(t, editor.ID, next.Candidates[0].Node.ID)
			require.Empty(t, candidatesFor(next.Candidates, history.ID), "held work is no move under real workflow observations")
			require.Equal(t, []string{"answer_question"}, candidateKinds(candidatesFor(next.Candidates, "Q-histo0")))
			pool := views.UnclaimedOfInput(tree, gen.DependencyOrder(tree), in)
			require.Len(t, pool, 1)
			require.Equal(t, editor.ID, pool[0].ID)
		})
	}
}
