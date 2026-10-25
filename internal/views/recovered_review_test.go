package views_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func recoveryFixtureRecord() *store.RecoveredReview {
	return &store.RecoveredReview{
		Branch:            "tiller/reviews/features/epic-0/acceptance",
		Head:              store.RevisionRef{Kind: store.KindGitCommit, Value: "git:historical-head"},
		MergeCommit:       store.RevisionRef{Kind: store.KindGitCommit, Value: "git:integration-result"},
		IntegrationBranch: "main", Basis: "ancestry", RecoveredBy: "jc",
		RecoveredAt: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC), Reason: "Authorized historical recovery",
		Tasks:           []store.RecoveredTask{{ID: "T-aaaaa0", Path: "task.yml", State: store.RevisionRef{Kind: store.KindGitBlob, Value: "git:task-state"}}},
		PolicySnapshots: []store.HistoricalPolicySnapshot{{NodeID: "F-aaaaa0", Path: "feature.yml", Source: store.RevisionRef{Kind: store.KindGitBlob, Value: "git:policy-blob"}, Observations: []store.HistoricalPolicyObservation{{Gate: "evals", Status: "pending", UnresolvedCriteria: []string{"C8"}}}}},
	}
}

func TestRecoveredWorkflowPreservesIndependentCurrentAcceptance(t *testing.T) {
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		for _, basis := range []string{"ancestry", "content", "contained"} {
			t.Run(grain+"/"+basis, func(t *testing.T) {
				tree, f, in := workflowFeature(t, grain)
				pr := 155
				record := recoveryFixtureRecord()
				record.Basis = basis
				var owner store.Node = f
				if grain == store.ReviewGrainEpic {
					epic := tree.Epics["E-aaaaa0"]
					epic.ReviewGrain, epic.PR, epic.RecoveredReview = grain, &pr, record
					owner = epic
				} else {
					f.PR, f.RecoveredReview = &pr, record
				}
				in.AssemblyErrors = map[string]string{f.ID: "old aggregate branch was deleted"}
				in.Eligibility = map[string]string{f.ID: "current contract basis changed"}
				a := workflow.AdviceOf(tree, owner, in)
				require.NotNil(t, a)
				require.False(t, a.AcceptanceReady)
				require.NotEmpty(t, a.Outstanding)
				require.NotNil(t, a.RecoveredReview)
				if basis == "contained" {
					require.Equal(t, "accept_fast_forward", a.Action)
					require.Equal(t, "human", a.Actor)
				} else {
					require.Equal(t, "sync", a.Action)
					require.Equal(t, "tiller sync", a.Command)
				}
				queue := views.QueueOfInput(tree, views.QueueInput{Workflow: in, BasisDrifted: map[string]bool{f.ID: true}})
				require.NotEmpty(t, queue.Entries)
				for _, e := range queue.Entries {
					require.NotEqual(t, "merge_feature", e.Kind)
					require.NotEqual(t, "merge_epic", e.Kind)
					if e.ID == owner.NodeID() {
						require.Equal(t, a.Command, e.Command)
					}
				}
				route := topMove(t, tree, "jc", in)
				require.NotNil(t, route.Workflow)
				require.Equal(t, a.Action, route.Workflow.Action)
				v := views.Node(tree, owner)
				require.Equal(t, record.Head.Value, v.RecoveredReview.Head)
				require.Equal(t, "pending", v.RecoveredReview.PolicySnapshots[0].Observations[0].Status)
				require.Equal(t, []string{"C8"}, v.RecoveredReview.PolicySnapshots[0].Observations[0].UnresolvedCriteria)
				encoded, err := json.Marshal(v)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, json.Unmarshal(encoded, &payload))
				require.NotNil(t, payload["recovered_review"])
			})
		}
	}
}

func TestRecoveredInvalidEvidenceNeverSelectsOrdinarySubmission(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	f.RecoveredReview = recoveryFixtureRecord()
	in.RecoveryErrors = map[string]string{f.ID: "integration result no longer exists; restore the historical Git object"}
	a := workflow.AdviceOf(tree, f, in)
	require.Equal(t, "inspect_recovery", a.Action)
	require.Contains(t, a.Reason, "restore the historical Git object")
	require.Empty(t, a.Command)
	route := topMove(t, tree, "jc", in)
	require.NotNil(t, route.Workflow)
	require.Equal(t, "inspect_recovery", route.Workflow.Action)
	queue := views.QueueOfInput(tree, views.QueueInput{Workflow: in})
	require.NotEmpty(t, queue.Entries)
	require.Empty(t, queue.Entries[0].Command)
}

func TestSyncReadoutRetainsMissingReviewAsStranded(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	r := views.ReadoutOfInput(tree, views.MomentSync, views.ReadoutInput{Act: &views.GateAct{Awaiting: []views.GateActAwaiting{{ReviewMissing: true}}}})
	require.NotEmpty(t, r.Checks)
	require.Equal(t, "1", r.Checks[len(r.Checks)-1].Result)
}

func TestRecoveryHistoricalUnknownAndPendingSurviveCurrentGreen(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	completeMachine(f)
	for _, id := range []string{"manual", "rubric"} {
		f.Gates["evals"].Criteria[id] = store.Criterion{Status: store.CriterionStatusPass}
	}
	gate := f.Gates["evals"]
	gate.Status = store.GateStatusPass
	f.Gates["evals"] = gate
	f.RecoveredReview = recoveryFixtureRecord()
	f.ReviewRef = &store.ReviewRef{Branch: f.RecoveredReview.Branch, Head: f.RecoveredReview.Head}
	f.RecoveredReview.PolicySnapshots[0].Observations = append(f.RecoveredReview.PolicySnapshots[0].Observations, store.HistoricalPolicyObservation{Gate: "tests", Status: "unknown"})
	a := workflow.AdviceOf(tree, f, in)
	require.True(t, a.AcceptanceReady)
	require.Equal(t, "sync", a.Action)
	require.Equal(t, "pending", a.RecoveredReview.PolicySnapshots[0].Observations[0].Status)
	require.Equal(t, "unknown", a.RecoveredReview.PolicySnapshots[0].Observations[1].Status)
	view := views.Node(tree, f)
	require.Nil(t, view.PR)
	require.Equal(t, f.RecoveredReview.Branch, view.ReviewRef.Branch)
	require.Equal(t, f.RecoveredReview.Head.Value, view.ReviewRef.Head)
	require.Equal(t, store.GateStatusPass, view.Gates["evals"].Status)
	require.Equal(t, "pending", view.RecoveredReview.PolicySnapshots[0].Observations[0].Status)
	f.Status = "done"
	done := workflow.AdviceOf(tree, f, in)
	require.Equal(t, "done", done.Phase)
	require.NotNil(t, done.RecoveredReview)
	f.PolicyViolations = []store.PolicyViolation{{Gate: "evals", Status: "pending", SourceRevision: f.RecoveredReview.PolicySnapshots[0].Source, UnresolvedCriteria: []string{"C8"}}}
	status := views.NodeStatus(tree, f, in)
	require.True(t, status.Workflow.AcceptanceReady)
	require.Equal(t, f.RecoveredReview.Head.Value, status.RecoveredReview.Head)
	require.Len(t, status.PolicyViolations, 1)
	require.Equal(t, []string{"C8"}, status.PolicyViolations[0].UnresolvedCriteria)
	require.Equal(t, f.RecoveredReview.PolicySnapshots[0].Source.Value, status.PolicyViolations[0].SourceRevision)
}

func TestMissingReviewKeepsOrdinaryBehaviorWithoutInventingMerge(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	a := workflow.AdviceOf(tree, f, in)
	require.Equal(t, "check", a.Action)
	require.False(t, a.SubmissionRecorded)
	require.Nil(t, a.RecoveredReview)
	_, missing := workflow.ReviewStateOf(tree, tree.Tasks["T-aaaaa0"])
	require.True(t, missing)
	completeMachine(f)
	for _, id := range []string{"manual", "rubric"} {
		f.Gates["evals"].Criteria[id] = store.Criterion{Status: store.CriterionStatusPass}
	}
	gate := f.Gates["evals"]
	gate.Status = store.GateStatusPass
	f.Gates["evals"] = gate
	require.Equal(t, "submit", workflow.AdviceOf(tree, f, in).Action)
}

func TestContainedRecoveryStopsAssertingAfterFrozenRidersCross(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	f.RecoveredReview = recoveryFixtureRecord()
	f.RecoveredReview.Basis = "contained"
	before := workflow.AdviceOf(tree, f, in)
	require.Equal(t, "accept_fast_forward", before.Action)
	require.True(t, before.RecoveredCrossingsPending)
	tree.Tasks["T-aaaaa0"].Status = "done"
	after := workflow.AdviceOf(tree, f, in)
	require.False(t, after.RecoveredCrossingsPending)
	require.Equal(t, "check", after.Action)
	require.Equal(t, "tiller check feature "+f.ID, after.Command)
	require.False(t, after.AcceptanceReady)
	route := topMove(t, tree, "jc", in)
	require.Equal(t, after.Command, route.Workflow.Command)
	queue := views.QueueOfInput(tree, views.QueueInput{Workflow: in})
	require.NotEmpty(t, queue.Entries)
	for _, entry := range queue.Entries {
		require.NotContains(t, entry.Command, "--accept-fast-forward")
	}
	completeMachine(f)
	for _, id := range []string{"manual", "rubric"} {
		f.Gates["evals"].Criteria[id] = store.Criterion{Status: store.CriterionStatusPass}
	}
	gate := f.Gates["evals"]
	gate.Status = store.GateStatusPass
	f.Gates["evals"] = gate
	require.Equal(t, "sync", workflow.AdviceOf(tree, f, in).Action)
}

func TestRecoveredEpicCombinesAllParticipatingCurrentAcceptance(t *testing.T) {
	tree, first, in := workflowFeature(t, store.ReviewGrainEpic)
	epic := tree.Epics["E-aaaaa0"]
	epic.ReviewGrain, epic.RecoveredReview = store.ReviewGrainEpic, recoveryFixtureRecord()
	completeMachine(first)
	for _, id := range []string{"manual", "rubric"} {
		first.Gates["evals"].Criteria[id] = store.Criterion{Status: store.CriterionStatusPass}
	}
	gate := first.Gates["evals"]
	gate.Status = store.GateStatusPass
	first.Gates["evals"] = gate
	second := feat(t, "F-aaaaa1", "epic-0/second", "epic-0")
	second.Status, second.ReviewGrain = "active", store.ReviewGrainEpic
	second.Gates = map[string]store.Gate{}
	tree.Features[second.ID] = second
	task := &store.Task{ID: "T-aaaaa1", FeatureID: second.ID, Status: "in_review"}
	tree.Tasks[task.ID] = task
	tree.FeatureTasks[second.ID] = []*store.Task{task}
	in.Contracts[second.ID] = &store.EvalFile{Node: second.ID, Criteria: []store.EvalCriterion{{ID: "C2", Statement: "Second acceptance", Required: true, Binding: store.EvalBinding{Type: store.BindingManual}}}}
	epic.Order = []string{first.ID, second.ID}
	epic.RecoveredReview.Tasks = append(epic.RecoveredReview.Tasks, store.RecoveredTask{ID: task.ID, Path: "second/task.yml"})
	advice := workflow.AdviceOf(tree, epic, in)
	require.Equal(t, "sync", advice.Action)
	require.False(t, advice.AcceptanceReady)
	require.Len(t, advice.CurrentAcceptance, 2)
	require.True(t, advice.CurrentAcceptance[0].AcceptanceReady)
	require.False(t, advice.CurrentAcceptance[1].AcceptanceReady)
	require.NotEmpty(t, advice.Outstanding)
	for _, work := range advice.Outstanding {
		require.Equal(t, second.ID, work.NodeID)
	}
	route := topMove(t, tree, "jc", in)
	require.False(t, route.Workflow.AcceptanceReady)
	require.Equal(t, advice.CurrentAcceptance, route.Workflow.CurrentAcceptance)
	epicItems := []views.ListItem{views.Item(tree, epic)}
	views.EnrichRecoveredList(tree, store.NounEpic, epicItems, in)
	require.NotNil(t, epicItems[0].CurrentAcceptance)
	require.False(t, epicItems[0].CurrentAcceptance.AcceptanceReady)
	require.Len(t, epicItems[0].ParticipatingAcceptance, 2)
	featureItems := []views.ListItem{views.Item(tree, first), views.Item(tree, second)}
	views.EnrichRecoveredList(tree, store.NounFeature, featureItems, in)
	require.True(t, featureItems[0].CurrentAcceptance.AcceptanceReady)
	require.False(t, featureItems[1].CurrentAcceptance.AcceptanceReady)
	require.Equal(t, second.ID, featureItems[1].CurrentAcceptance.NodeID)
}

func TestMissingReviewRemedySurvivesCurrentEligibilityBlocker(t *testing.T) {
	tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
	in.Eligibility = map[string]string{f.ID: "current accepted basis drift"}
	advice := workflow.AdviceOf(tree, f, in)
	require.Equal(t, "resolve_blocker", advice.Action)
	require.True(t, advice.ReviewMissing)
	require.NotEmpty(t, advice.ReviewGuidance)
	require.Contains(t, advice.Reason, advice.ReviewGuidance)
	status := views.NodeStatus(tree, f, in)
	require.True(t, status.ReviewMissing)
	require.Equal(t, advice.ReviewGuidance, status.Workflow.ReviewGuidance)
	route := topMove(t, tree, "jc", in)
	require.NotNil(t, route.Workflow)
	require.True(t, route.Workflow.ReviewMissing)
	require.Contains(t, route.Reason, route.Workflow.ReviewGuidance)
}

func TestRecoveredStatusSeparatesHistoricalSourcesFromCrossingTimeWarnings(t *testing.T) {
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			tree, feature, in := workflowFeature(t, grain)
			record := recoveryFixtureRecord()
			feature.Status = "done"
			tree.Tasks["T-aaaaa0"].Status = "done"
			if grain == store.ReviewGrainEpic {
				epic := tree.Epics["E-aaaaa0"]
				epic.ReviewGrain, epic.Status, epic.RecoveredReview = grain, "done", record
			} else {
				feature.RecoveredReview = record
			}
			feature.PolicyViolations = []store.PolicyViolation{
				{Gate: "evals", Status: "pending", SourceRevision: record.PolicySnapshots[0].Source, UnresolvedCriteria: []string{"C8"}},
				{Gate: "evals", Status: "fail"},
			}
			status := views.NodeStatus(tree, feature, in)
			require.NotNil(t, status.Workflow.RecoveredReview)
			require.Equal(t, 1, status.Workflow.HistoricalViolations)
			require.Equal(t, 1, status.Workflow.CrossingViolations)
			require.Len(t, status.PolicyViolations, 2)
			require.NotEmpty(t, status.PolicyViolations[0].SourceRevision)
			require.Empty(t, status.PolicyViolations[1].SourceRevision)
			require.Equal(t, []string{"C8"}, status.PolicyViolations[0].UnresolvedCriteria)
			require.Equal(t, "fail", status.PolicyViolations[1].Status)
			items := []views.ListItem{views.Item(tree, feature)}
			views.EnrichRecoveredList(tree, store.NounFeature, items, in)
			require.NotNil(t, items[0].CurrentAcceptance)
			require.Equal(t, status.Workflow.AcceptanceReady, items[0].CurrentAcceptance.AcceptanceReady)
			require.Equal(t, 1, *items[0].HistoricalViolations)
			require.Equal(t, 1, *items[0].CrossingViolations)
		})
	}
}

func TestOrdinaryStoredViolationSummaryRetainsExistingHistoricalMeaning(t *testing.T) {
	tree, feature, in := workflowFeature(t, store.ReviewGrainFeature)
	feature.Status = "done"
	feature.PolicyViolations = []store.PolicyViolation{{Gate: "evals", Status: "pending"}, {Gate: "tests", Status: "fail"}}
	status := views.NodeStatus(tree, feature, in)
	require.Nil(t, status.Workflow.RecoveredReview)
	require.Equal(t, 2, status.Workflow.HistoricalViolations)
	require.Zero(t, status.Workflow.CrossingViolations)
	require.Len(t, status.PolicyViolations, 2)
}

func TestReviewMissingOnlyNamesSubmittedWorkAwaitingAnUnrecordedReview(t *testing.T) {
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			tree, f, _ := workflowFeature(t, grain)
			epic := tree.Epics["E-aaaaa0"]
			epic.ReviewGrain = grain
			task := tree.Tasks["T-aaaaa0"]
			missing := func(n store.Node) bool {
				_, m := workflow.ReviewStateOf(tree, n)
				return m
			}

			require.True(t, missing(task), "a submitted rider is stranded without its aggregate review")
			require.True(t, missing(f))
			require.Equal(t, grain == store.ReviewGrainEpic, missing(epic))

			task.Status = "in_progress"
			require.False(t, missing(task), "unfinished work has no review to miss")
			require.False(t, missing(f))
			require.False(t, missing(epic))
			require.False(t, views.NodeStatus(tree, f, workflow.Input{}).ReviewMissing)

			draft := feat(t, "F-aaaaa1", "epic-0/later", "epic-0")
			draft.Status, draft.ReviewGrain = "draft", grain
			tree.Features[draft.ID] = draft
			require.False(t, missing(draft), "a draft feature has nothing submitted")

			task.Status = "in_review"
			switch owner := workflow.ReviewOwnerOf(tree, task).(type) {
			case *store.Feature:
				owner.Status = "done"
			case *store.Epic:
				owner.Status = "done"
			}
			require.False(t, missing(task), "a closed review owner strands nothing")
		})
	}
}

func TestReviewMissingIgnoresATaskWithItsOwnPR(t *testing.T) {
	tree, f, _ := workflowFeature(t, store.ReviewGrainFeature)
	task := tree.Tasks["T-aaaaa0"]
	pr := 12
	task.PR = &pr
	_, missing := workflow.ReviewStateOf(tree, task)
	require.False(t, missing, "a task PR is task-owned history")
	_, missing = workflow.ReviewStateOf(tree, f)
	require.False(t, missing, "the feature has no rider awaiting its review")
}

func TestReviewMissingFollowsTheFeatureThatHoldsTheSubmittedRider(t *testing.T) {
	tree, first, _ := workflowFeature(t, store.ReviewGrainEpic)
	epic := tree.Epics["E-aaaaa0"]
	epic.ReviewGrain = store.ReviewGrainEpic
	tree.Tasks["T-aaaaa0"].Status = "in_progress"
	second := feat(t, "F-aaaaa1", "epic-0/second", "epic-0")
	second.Status, second.ReviewGrain = "active", store.ReviewGrainEpic
	tree.Features[second.ID] = second
	rider := &store.Task{ID: "T-aaaaa1", FeatureID: second.ID, Status: "in_review", Owner: "jc", Started: true}
	tree.Tasks[rider.ID] = rider
	tree.FeatureTasks[second.ID] = []*store.Task{rider}
	missing := func(n store.Node) bool {
		_, m := workflow.ReviewStateOf(tree, n)
		return m
	}
	require.True(t, missing(epic))
	require.True(t, missing(second))
	require.True(t, missing(rider))
	require.False(t, missing(first), "the idle sibling has nothing submitted")
}

func TestRecoveredOwnersAreDistinct(t *testing.T) {
	tree, first, _ := workflowFeature(t, store.ReviewGrainEpic)
	epic := tree.Epics["E-aaaaa0"]
	epic.ReviewGrain = store.ReviewGrainEpic
	epic.RecoveredReview = recoveryFixtureRecord()
	second := feat(t, "F-aaaaa1", "epic-0/second", "epic-0")
	second.Status, second.ReviewGrain = "active", store.ReviewGrainEpic
	tree.Features[second.ID] = second
	items := []views.ListItem{views.Item(tree, first), views.Item(tree, second)}
	require.NotNil(t, items[0].RecoveredReview)
	require.NotNil(t, items[1].RecoveredReview)

	owners := views.RecoveredOwners(tree, store.NounFeature, items)
	require.Len(t, owners, 1, "rows riding one recovered epic share one observation")
	require.Equal(t, epic.ID, owners[0].NodeID())
}

func TestCancelledFeatureRidesNoAggregateReview(t *testing.T) {
	tree, f, _ := workflowFeature(t, store.ReviewGrainEpic)
	epic := tree.Epics["E-aaaaa0"]
	epic.ReviewGrain = store.ReviewGrainEpic
	epic.RecoveredReview = recoveryFixtureRecord()
	require.Equal(t, epic.ID, workflow.ReviewOwnerOf(tree, f).NodeID())

	f.Status = "cancelled"
	require.Nil(t, workflow.ReviewOwnerOf(tree, f))
	require.Nil(t, workflow.ReviewOwnerOf(tree, tree.Tasks["T-aaaaa0"]))
	recovered, missing := workflow.ReviewStateOf(tree, f)
	require.Nil(t, recovered, "a cancelled feature does not carry the epic's recovered review")
	require.False(t, missing)
}

func TestCancelledFeatureKeepsItsOwnRecoveredReview(t *testing.T) {
	tree, f, _ := workflowFeature(t, store.ReviewGrainFeature)
	record := recoveryFixtureRecord()
	f.RecoveredReview = record
	f.Status = "cancelled"
	require.Equal(t, f.ID, workflow.ReviewOwnerOf(tree, f).NodeID(), "its own record is history, not an inheritance")
	require.Equal(t, f.ID, workflow.ReviewOwnerOf(tree, tree.Tasks["T-aaaaa0"]).NodeID())
	recovered, missing := workflow.ReviewStateOf(tree, f)
	require.NotNil(t, recovered)
	require.Equal(t, record.Branch, recovered.Branch)
	require.False(t, missing)
}
