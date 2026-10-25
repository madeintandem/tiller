package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func workflowContracts(tree *store.Tree) workflow.Input {
	in := workflow.Input{Contracts: map[string]*store.EvalFile{}}
	for id := range tree.Features {
		in.Contracts[id] = &store.EvalFile{Node: id, Criteria: []store.EvalCriterion{{ID: "optional", Statement: "Optional observation", Binding: store.EvalBinding{Type: store.BindingManual}}}}
	}
	return in
}

func workflowFeature(t *testing.T, grain string) (*store.Tree, *store.Feature, workflow.Input) {
	t.Helper()
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f := feat(t, "F-aaaaa0", "epic-0/acceptance", "epic-0")
	f.Status, f.ReviewGrain = "active", grain
	f.Gates = map[string]store.Gate{}
	tree.Features[f.ID] = f
	task := &store.Task{ID: "T-aaaaa0", FeatureID: f.ID, Status: "in_review", Owner: "jc", Started: true}
	if grain == store.ReviewGrainTask {
		task.Status = "done"
	}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {task}}
	tree.Manifest.Gates.Required = map[string]store.GateSpec{
		"tests": {Kind: store.GateKindCommand, Run: "true", On: store.StringList{store.NounFeature}},
		"evals": {Kind: store.GateKindCommand, On: store.StringList{store.NounFeature}},
	}
	in := workflowContracts(tree)
	in.Contracts[f.ID].Criteria = []store.EvalCriterion{
		{ID: "machine", Statement: "Machine acceptance", Required: true, Binding: store.EvalBinding{Type: store.BindingCommand, Run: "true"}},
		{ID: "manual", Statement: "Human acceptance", Required: true, Binding: store.EvalBinding{Type: store.BindingManual}},
		{ID: "rubric", Statement: "Rubric acceptance", Required: true, Binding: store.EvalBinding{Type: store.BindingEval, Ref: store.BindingRefs{"rubric.md"}}},
		{ID: "optional", Statement: "Optional rubric", Binding: store.EvalBinding{Type: store.BindingEval, Ref: store.BindingRefs{"optional.md"}}},
	}
	return tree, f, in
}

func completeMachine(f *store.Feature) {
	f.Gates["tests"] = store.Gate{Status: store.GateStatusPass}
	f.Gates["evals"] = store.Gate{Status: store.GateStatusPending, Criteria: map[string]store.Criterion{
		"machine": {Status: store.CriterionStatusPass},
		"manual":  {Status: store.CriterionStatusNeedsAttestation},
		"rubric":  {Status: store.CriterionStatusPending},
	}}
}

func TestWorkflowKeepsMixedWorkThroughChecksRubricsAndHumanAcceptance(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	decision := topMove(t, tree, "jc", in)
	require.Equal(t, "checks", decision.Workflow.Phase)
	require.Equal(t, "agent", decision.Workflow.Actor)
	require.Len(t, decision.Workflow.Outstanding, 5)
	require.False(t, decision.Workflow.AcceptanceReady)

	completeMachine(f)
	decision = topMove(t, tree, "jc", in)
	require.Equal(t, "rubric", decision.Workflow.Phase)
	require.Equal(t, "grade_rubric", decision.Workflow.Action)
	require.Len(t, decision.Workflow.Outstanding, 2)

	f.Gates["evals"].Criteria["rubric"] = store.Criterion{Status: store.CriterionStatusNeedsAttestation, Evidence: []store.Evidence{{Kind: "rubric_output", Ref: "grade.json"}}}
	decision = topMove(t, tree, "jc", in)
	require.Empty(t, decision.Skill)
	require.Equal(t, "acceptance", decision.Workflow.Phase)
	require.Equal(t, "human", decision.Workflow.Actor)
	require.Len(t, decision.Workflow.Outstanding, 2)
	for _, work := range decision.Workflow.Outstanding {
		require.NotEqual(t, "optional", work.ID)
	}

	// A stale grade must be prepared again, not handed straight to a human.
	f.Gates["evals"].Criteria["rubric"] = store.Criterion{Status: store.CriterionStatusStale, Evidence: []store.Evidence{{Kind: "rubric_output", Ref: "old.json"}}}
	require.Equal(t, "rubric", topMove(t, tree, "jc", in).Workflow.Phase)
}

func TestWorkflowContractUncertaintyNeverBecomesHumanOnlyOrAcceptanceReady(t *testing.T) {
	for _, state := range []string{"unread", "unavailable", "invalid"} {
		t.Run(state, func(t *testing.T) {
			tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
			completeMachine(f)
			switch state {
			case "unread":
				in.Contracts = nil
			case "unavailable":
				in.ContractErrors = map[string]string{f.ID: "read failed"}
			case "invalid":
				in.Contracts[f.ID] = &store.EvalFile{}
			}
			a := workflow.AdviceOf(tree, f, in)
			require.Equal(t, state, a.ContractStatus)
			require.Equal(t, "agent", a.Actor)
			require.False(t, a.AcceptanceReady)
			require.Equal(t, "repair_contract", a.Action)
		})
	}
}

func TestWorkflowManifestScopeAndInconsistentAggregate(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	completeMachine(f)
	for _, id := range []string{"manual", "rubric"} {
		f.Gates["evals"].Criteria[id] = store.Criterion{Status: store.CriterionStatusPass}
	}
	for _, status := range []string{store.GateStatusPending, store.GateStatusFail, store.GateStatusStale} {
		gate := f.Gates["evals"]
		gate.Status = status
		f.Gates["evals"] = gate
		a := workflow.AdviceOf(tree, f, in)
		require.False(t, a.AcceptanceReady)
		require.Equal(t, "check", a.Action)
	}
	// Current manifest scope wins over both old Required flags and old records.
	tree.Manifest.Gates.Required["evals"] = store.GateSpec{Kind: store.GateKindCommand, On: store.StringList{store.NounTask}}
	f.Gates["evals"] = store.Gate{Required: true, Status: store.GateStatusFail}
	a := workflow.AdviceOf(tree, f, in)
	require.True(t, a.AcceptanceReady)
	require.Equal(t, "submit", a.Action)
	delete(f.Gates, "tests")
	a = workflow.AdviceOf(tree, f, in)
	require.False(t, a.AcceptanceReady)
	require.Equal(t, "check", a.Action)
}

func TestTaskGrainCloseoutSharesNextFullCorrectionAndNodeAdvice(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainTask)
	completeMachine(f)
	f.Gates["evals"].Criteria["rubric"] = store.Criterion{Status: store.CriterionStatusPass}
	decision := topMove(t, tree, "jc", in)
	require.Equal(t, f.ID, decision.Node.ID)
	require.Equal(t, "acceptance", decision.Workflow.Phase)
	require.NotContains(t, decision.Workflow.Reason, "draft")
	require.Contains(t, decision.Workflow.Reason, "integration-branch")
	full := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc", Workflow: in})
	correction := views.CorrectionOfInput(tree, views.MomentExecute, views.CorrectionInput{Contributor: "jc", Workflow: in})
	require.Equal(t, full.Next, correction.Next)
	require.Equal(t, "human", topNext(t, full).Actor)
	status := views.NodeStatus(tree, f, in)
	require.Equal(t, decision.Workflow, status.Workflow)
	require.NotContains(t, status.LegalNext, "sync")

	// Human attestation is sufficient manual evidence; no command artifact exists.
	f.Gates["evals"].Criteria["manual"] = store.Criterion{Status: store.CriterionStatusPass}
	gate := f.Gates["evals"]
	gate.Status = store.GateStatusPass
	f.Gates["evals"] = gate
	f.Attestations = []store.Attestation{{ID: "AT-human", Target: store.AttestationTarget{Kind: "criterion", ID: "manual"}, Status: "pass"}}
	decision = topMove(t, tree, "jc", in)
	require.Equal(t, "sync", decision.Workflow.Action)
	require.Equal(t, "tiller sync", decision.Workflow.Command)
	require.True(t, decision.Workflow.AcceptanceReady)

	f.Status = "done"
	f.PolicyViolations = []store.PolicyViolation{{Gate: "evals", Status: "pending"}}
	status = views.NodeStatus(tree, f, in)
	require.Empty(t, status.LegalNext)
	require.Equal(t, "done", status.Workflow.Phase)
	require.Equal(t, 1, status.Workflow.HistoricalViolations)
	readyFeatureWithTask(t, tree, "F-later0", "epic-0/later", "epic-0")
	require.Equal(t, "claim_task", topMove(t, tree, "jc", in).Kind)
}

func TestWorkflowCloseoutIsSharedAndRetainsEligibilityGuards(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainTask)
	require.Equal(t, f.ID, topMove(t, tree, "unrelated-reviewer", in).Node.ID)
	in.Eligibility = map[string]string{f.ID: "accepted basis changed"}
	a := workflow.AdviceOf(tree, f, in)
	require.Equal(t, "blocked", a.Phase)
	require.Empty(t, a.Command)
	tree.Epics["E-aaaaa0"].Status = "cancelled"
	require.Equal(t, "blocked", workflow.AdviceOf(tree, f, in).Phase)
	require.Empty(t, candidatesFor(nextFor(tree, "jc", in).Candidates, f.ID))
}

func TestWorkflowMachineRecoveryPrecedesEarlierAuthoredRubric(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	completeMachine(f)
	criteria := in.Contracts[f.ID].Criteria
	criteria[0], criteria[2] = criteria[2], criteria[0]
	f.Gates["evals"].Criteria["machine"] = store.Criterion{Status: "fail"}
	a := workflow.AdviceOf(tree, f, in)
	require.Equal(t, "checks", a.Phase)
	require.Len(t, a.Outstanding, 3)
}

func TestWorkflowRequiredAttestationGateDoesNotBecomeRepeatedCheck(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	completeMachine(f)
	for _, id := range []string{"manual", "rubric"} {
		f.Gates["evals"].Criteria[id] = store.Criterion{Status: "pass"}
	}
	gate := f.Gates["evals"]
	gate.Status = "pass"
	f.Gates["evals"] = gate
	tree.Manifest.Gates.Required["review"] = store.GateSpec{Kind: "attestation", On: store.StringList{"feature"}}
	a := workflow.AdviceOf(tree, f, in)
	require.Equal(t, "human", a.Actor)
	require.Equal(t, "attest", a.Action)
	require.Equal(t, []workflow.Work{{Kind: "gate", ID: "review", Status: "missing", Actor: "human"}}, a.Outstanding)
	f.Gates["review"] = store.Gate{Status: "pass"}
	a = workflow.AdviceOf(tree, f, in)
	require.True(t, a.AcceptanceReady)
	require.Equal(t, "submit", a.Action)
}

func TestResolvedReviewAcceptanceStaysVisibleBeyondTaskCap(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	green := store.GatesBlock{Gates: map[string]store.Gate{"tests": {Required: true, Status: "pass", Evidence: []store.Evidence{{Kind: "command_output", Ref: "test.txt"}}}}}
	tree.Tasks = map[string]*store.Task{}
	tree.FeatureTasks[f.ID] = nil
	for _, id := range []string{"T-aaaaa0", "T-aaaaa1", "T-aaaaa2", "T-aaaaa3", "T-aaaaa4"} {
		task := &store.Task{ID: id, FeatureID: f.ID, Status: "in_review", ReviewRef: &store.ReviewRef{Branch: "implementation"}, GatesBlock: green}
		tree.Tasks[id] = task
		tree.FeatureTasks[f.ID] = append(tree.FeatureTasks[f.ID], task)
	}
	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc", Workflow: in})
	require.Len(t, r.Entities, 3)
	require.Equal(t, 2, r.Withheld)
	require.Equal(t, "5 of 5", checkLabels(r)["Gates green with evidence"])
	require.Equal(t, "5 of 5", checkLabels(r)["Current submissions recorded"])
	require.Contains(t, checkLabels(r), "Resolved review "+gen.Ref(tree, f))
	require.Equal(t, "tiller check feature "+f.ID, topNext(t, r).Command)
	a := workflow.AdviceOf(tree, f, in)
	require.False(t, a.AcceptanceReady)
	require.False(t, a.SubmissionRecorded)
}

func TestNodeAdvicePreservesLifecycleUntilItsResolvedReviewIsReady(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainTask)
	task := tree.Tasks["T-aaaaa0"]
	require.Nil(t, workflow.AdviceOf(tree, task, in))
	task.Status = "ready"
	require.Nil(t, workflow.AdviceOf(tree, f, in))
	epic := tree.Epics["E-aaaaa0"]
	epic.ReviewGrain = store.ReviewGrainEpic
	f.ReviewGrain = ""
	task.Status = "in_review"
	_, siblingTask := readyFeatureWithTask(t, tree, "F-bbbbb0", "epic-0/sibling", "epic-0")
	siblingTask.Status = "ready"
	require.Nil(t, workflow.AdviceOf(tree, f, in))
	require.Nil(t, workflow.AdviceOf(tree, task, in))
}

func TestReadyTaskUnderDoneRollupRequiresExplicitAssignment(t *testing.T) {
	for _, parent := range []string{"feature", "epic", "both"} {
		t.Run(parent, func(t *testing.T) {
			tree, f, in := workflowFeature(t, store.ReviewGrainTask)
			if parent != "epic" {
				f.Status = "done"
			}
			if parent != "feature" {
				tree.Epics["E-aaaaa0"].Status = "done"
			}
			task := tree.Tasks["T-aaaaa0"]
			task.Owner, task.Status, task.Started = "", "ready", false
			require.False(t, views.EligibleTask(tree, task, in))
			require.False(t, gen.EligibleTask(tree, task))
			require.False(t, gen.AssignedTaskEligible(tree, task))
			require.Empty(t, candidatesFor(nextFor(tree, "jc", in).Candidates, task.ID))

			task.Owner = "jc"
			decision := topMove(t, tree, "jc", in)
			require.Equal(t, "continue", decision.Kind)
			require.Equal(t, task.ID, decision.Node.ID)
			require.False(t, views.EligibleTask(tree, task, in), "assignment never broadens the pool")
			require.Empty(t, candidatesFor(nextFor(tree, "other", in).Candidates, task.ID))
		})
	}
}

func TestAssignedRollupContinuationRetainsStartGuards(t *testing.T) {
	for _, guard := range []string{"feature_review", "epic_review", "blocked", "dependency", "question", "basis", "retired"} {
		t.Run(guard, func(t *testing.T) {
			tree, f, in := workflowFeature(t, store.ReviewGrainTask)
			f.Status = "done"
			task := tree.Tasks["T-aaaaa0"]
			task.Owner, task.Status, task.Started = "jc", "ready", false
			switch guard {
			case "feature_review":
				f.ReviewGrain = store.ReviewGrainFeature
			case "epic_review":
				f.ReviewGrain = ""
				tree.Epics["E-aaaaa0"].ReviewGrain = store.ReviewGrainEpic
			case "blocked":
				tree.Epics["E-aaaaa0"].Blocked = true
			case "dependency":
				task.DependsOn = []string{"T-missing"}
			case "question":
				holdClaimableNode(tree, "Q-hold", f.ID)
			case "basis":
				in.Eligibility = map[string]string{task.ID: "accepted basis changed"}
			case "retired":
				tree.Epics["E-aaaaa0"].Status = "archived"
			}
			require.Empty(t, candidatesFor(nextFor(tree, "jc", in).Candidates, task.ID))
		})
	}
}

func TestEpicAdviceAllowsIndependentTaskOwnersAndUnrelatedReviewer(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	epic := tree.Epics["E-aaaaa0"]
	epic.ReviewGrain, f.ReviewGrain = store.ReviewGrainEpic, ""
	completeMachine(f)
	for _, id := range []string{"manual", "rubric"} {
		f.Gates["evals"].Criteria[id] = store.Criterion{Status: "pass"}
	}
	gate := f.Gates["evals"]
	gate.Status = "pass"
	f.Gates["evals"] = gate
	sibling, task := readyFeatureWithTask(t, tree, "F-bbbbb0", "epic-0/sibling", "epic-0")
	sibling.Status = "active"
	sibling.Gates = f.Gates
	task.Status, task.Owner, task.Started = "in_review", "other", true
	in.Contracts[sibling.ID] = in.Contracts[f.ID]
	decision := topMove(t, tree, "reviewer", in)
	require.NotNil(t, decision.Workflow)
	require.Equal(t, epic.ID, decision.Node.ID)
	require.Equal(t, "submit", decision.Workflow.Action)
	require.Equal(t, decision.Workflow, workflow.AdviceOf(tree, epic, in))
}
