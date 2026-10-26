package views_test

import (
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// enumerate runs the candidate enumeration for reader over tree.
func enumerate(tree *store.Tree, reader string, in workflow.Input) []views.NextCandidate {
	return views.CandidatesOf(tree, gen.DependencyOrder(tree), views.NextInput{Reader: reader, Workflow: in})
}

// candidatesOfKind keeps the candidates of one kind, in enumeration order.
func candidatesOfKind(cands []views.NextCandidate, kind string) []views.NextCandidate {
	var out []views.NextCandidate
	for _, c := range cands {
		if c.Kind == kind {
			out = append(out, c)
		}
	}
	return out
}

// candidateNodeIDs lists each candidate's node id ("" when it has none).
func candidateNodeIDs(cands []views.NextCandidate) []string {
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		id := ""
		if c.Node != nil {
			id = c.Node.ID
		}
		out = append(out, id)
	}
	return out
}

// candidatesFor keeps every candidate naming the node id.
func candidatesFor(cands []views.NextCandidate, id string) []views.NextCandidate {
	var out []views.NextCandidate
	for _, c := range cands {
		if c.Node != nil && c.Node.ID == id {
			out = append(out, c)
		}
	}
	return out
}

func TestEnumerateBaselineIsExclusive(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Manifest.Project.Kind = store.KindBrownfield

	cands := enumerate(tree, "jc", workflow.Input{})
	require.Len(t, cands, 1, "the draft epic's approval waits behind the baseline")
	require.Equal(t, "derive_baseline", cands[0].Kind)
	require.Equal(t, "agent", cands[0].Actor)
	require.Equal(t, "derive-baseline", cands[0].Skill)
	require.Empty(t, cands[0].Command)
	require.Nil(t, cands[0].Node)

	tree.Baseline.Docs = []store.BaselineDoc{
		{Path: "system.md", Describes: []string{"."}},
		{Path: "billing.md", Describes: []string{"billing"}},
	}
	cands = enumerate(tree, "jc", workflow.Input{})
	require.Len(t, cands, 2)
	for i, path := range []string{"system.md", "billing.md"} {
		require.Equal(t, "confirm_design", cands[i].Kind)
		require.Equal(t, "human", cands[i].Actor)
		require.Equal(t, "tiller approve design "+path, cands[i].Command)
		require.Contains(t, cands[i].Action, path)
	}
}

func TestEnumerateOwnWorkCheckedFirst(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f, inProgress := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	f.Status = "active"
	inProgress.Owner, inProgress.Status, inProgress.Started = "jc", "in_progress", true
	checked := &store.Task{ID: "T-bbbbb0", Key: strPtr("epic-0/work/second"), Status: "check", Owner: "jc", Started: true, FeatureID: f.ID}
	tree.Tasks[checked.ID] = checked
	tree.FeatureTasks[f.ID] = append(tree.FeatureTasks[f.ID], checked)
	f.Order = []string{inProgress.ID, checked.ID}
	tree.Bugs = map[string]*store.Bug{
		"B-aaaaa0": {WorkItem: store.WorkItem{ID: "B-aaaaa0", Key: strPtr("bugs/crash"), Title: "Crash", Status: "ready", Owner: "jc"}},
	}

	cont := candidatesOfKind(enumerate(tree, "jc", workflow.Input{}), "continue")
	require.Equal(t, []string{checked.ID, inProgress.ID, "B-aaaaa0"}, candidateNodeIDs(cont))
	for _, c := range cont {
		require.Equal(t, "agent", c.Actor)
		require.Equal(t, "execute", c.Skill)
		require.Equal(t, "continue", c.Action)
		require.Empty(t, c.Command)
		require.Empty(t, c.Branch)
		require.Empty(t, c.Reason, "the ranker writes every continue reason")
	}
	checkedRow, _ := rowFor(t, nextFor(tree, "jc", workflow.Input{}), checked.ID)
	require.Contains(t, checkedRow.Reason, "finish any outstanding checks")
	require.Empty(t, candidatesOfKind(enumerate(tree, "robin", workflow.Input{}), "continue"),
		"another reader's own work is never continued")
}

func TestEnumerateOwnObservedTaskCarriesItsBranch(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")

	got := candidatesFor(enumerate(tree, "jc", observedOn(task.ID, "t/x", "jc")), task.ID)
	require.Len(t, got, 1, "an observed task is continued, never also offered as a claim")
	require.Equal(t, "continue", got[0].Kind)
	require.Equal(t, "agent", got[0].Actor)
	require.Equal(t, "t/x", got[0].Branch)
	require.Empty(t, got[0].Command)
	require.Empty(t, got[0].Skill, "the work continues on its branch, not from this view")
	require.Empty(t, got[0].Reason, "the ranker writes every continue reason")
	row, _ := rowFor(t, nextFor(tree, "jc", observedOn(task.ID, "t/x", "jc")), task.ID)
	require.Equal(t, "your task, in flight on its branch", row.Reason)
}

func TestEnumerateExcludesOthersInFlightWork(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	require.Empty(t, candidatesFor(enumerate(tree, "jc", observedOn(task.ID, "t/x", "robin")), task.ID))

	task.Owner = "jc"
	require.Empty(t, candidatesFor(enumerate(tree, "jc", observedOn(task.ID, "t/x", "robin")), task.ID),
		"an owned task in flight on another contributor's branch is theirs to continue")
}

func TestEnumerateClaimableListsEveryEligibleTask(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f, first := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	for _, id := range []string{"T-bbbbb0", "T-ccccc0"} {
		task := &store.Task{ID: id, Key: strPtr("epic-0/work/" + id), Status: "ready", FeatureID: f.ID}
		tree.Tasks[id] = task
		tree.FeatureTasks[f.ID] = append(tree.FeatureTasks[f.ID], task)
	}
	f.Order = []string{"T-ccccc0", first.ID, "T-bbbbb0"}

	claims := candidatesOfKind(enumerate(tree, "jc", workflow.Input{}), "claim_task")
	pool := gen.UnclaimedTasks(tree, gen.DependencyOrder(tree), nil)
	want := make([]string, 0, len(pool))
	for _, task := range pool {
		want = append(want, task.ID)
	}
	require.Equal(t, []string{"T-ccccc0", first.ID, "T-bbbbb0"}, want)
	require.Equal(t, want, candidateNodeIDs(claims))
	for i, c := range claims {
		require.Equal(t, "agent", c.Actor)
		require.Equal(t, "execute", c.Skill)
		require.Equal(t, "claim and start", c.Action)
		require.Equal(t, "tiller claim task "+*tree.Tasks[want[i]].Key, c.Command)
		require.Equal(t, claims[0].Arm(), c.Arm(), "every claim sits in one arm")
	}
}

func TestEnumerateQuestionHeldTaskYieldsOnlyItsQuestion(t *testing.T) {
	for _, owner := range []string{"", "jc"} {
		t.Run("owner="+owner, func(t *testing.T) {
			tree := treeWithDraftEpics(t, 1)
			tree.Epics["E-aaaaa0"].Status = "ready"
			_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
			task.Owner = owner
			holdClaimableNode(tree, "Q-aaaaa0", task.ID)

			cands := enumerate(tree, "jc", workflow.Input{})
			require.Empty(t, candidatesFor(cands, task.ID))
			answers := candidatesOfKind(cands, "answer_question")
			require.Len(t, answers, 1)
			require.Equal(t, "Q-aaaaa0", answers[0].Node.ID)
			require.Equal(t, "human", answers[0].Actor)
			require.Equal(t, "answer", answers[0].Action)
			require.Equal(t, `tiller answer question Q-aaaaa0 --answer "..."`, answers[0].Command)
		})
	}
}

func TestEnumerateDependencyBlockedWorkIsAbsent(t *testing.T) {
	for _, owner := range []string{"", "jc"} {
		t.Run("owner="+owner, func(t *testing.T) {
			tree := treeWithDraftEpics(t, 1)
			tree.Epics["E-aaaaa0"].Status = "ready"
			f, first := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
			blocked := &store.Task{ID: "T-bbbbb0", Key: strPtr("epic-0/work/second"), Status: "ready", Owner: owner,
				FeatureID: f.ID, DependsOn: []string{first.ID}}
			tree.Tasks[blocked.ID] = blocked
			tree.FeatureTasks[f.ID] = append(tree.FeatureTasks[f.ID], blocked)

			cands := enumerate(tree, "jc", workflow.Input{})
			require.Empty(t, candidatesFor(cands, blocked.ID))
			require.Len(t, candidatesFor(cands, first.ID), 1)
		})
	}
}

func TestEnumerateResolveBlockerOnTheReadersHeldFeature(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	task.Owner = "jc"
	in := workflow.Input{Eligibility: map[string]string{f.ID: "accepted basis changed"}}

	blockers := candidatesOfKind(enumerate(tree, "jc", in), "resolve_blocker")
	require.Equal(t, []string{f.ID}, candidateNodeIDs(blockers))
	require.Equal(t, "human", blockers[0].Actor)
	require.Equal(t, "resolve the blocker on", blockers[0].Action)
	require.Empty(t, blockers[0].Command)
	require.Contains(t, blockers[0].Reason, "accepted basis changed")
	require.Empty(t, candidatesOfKind(enumerate(tree, "robin", in), "resolve_blocker"))

	holdClaimableNode(tree, "Q-aaaaa0", f.ID)
	cands := enumerate(tree, "jc", in)
	require.Empty(t, candidatesOfKind(cands, "resolve_blocker"), "the holding question is the candidate")
	require.Len(t, candidatesOfKind(cands, "answer_question"), 1)
}

// reviewPhaseTree walks workflowFeature's closeout to the human review and
// merge phase at feature or epic grain.
func reviewPhaseTree(t *testing.T, grain string) (*store.Tree, workflow.Input) {
	t.Helper()
	tree, f, in := workflowFeature(t, grain)
	if grain == store.ReviewGrainEpic {
		f.ReviewGrain = ""
		tree.Epics["E-aaaaa0"].ReviewGrain = grain
	}
	completeMachine(f)
	f.Gates["evals"].Criteria["manual"] = store.Criterion{Status: "pass"}
	f.Gates["evals"].Criteria["rubric"] = store.Criterion{Status: "pass"}
	gate := f.Gates["evals"]
	gate.Status = "pass"
	f.Gates["evals"] = gate
	pr := 42
	if grain == store.ReviewGrainEpic {
		tree.Epics["E-aaaaa0"].PR = &pr
	} else {
		f.PR = &pr
	}
	return tree, in
}

func TestEnumerateEveryQueueEntryIsACandidate(t *testing.T) {
	shelf := &store.Tree{
		Manifest: &store.Manifest{},
		Bugs: map[string]*store.Bug{
			"B-prop00": {WorkItem: store.WorkItem{ID: "B-prop00", Key: strPtr("bugs/proposed"), Status: "proposed"}},
			"B-return": {WorkItem: store.WorkItem{ID: "B-return", Key: strPtr("bugs/returned"), Status: "ready", ResolvedBy: "F-gone00"}},
		},
		Chores: map[string]*store.Chore{
			"CH-prop00": {WorkItem: store.WorkItem{ID: "CH-prop00", Key: strPtr("chores/proposed"), Status: "proposed"}},
		},
		Tweaks: map[string]*store.Tweak{
			"TW-prop00": {WorkItem: store.WorkItem{ID: "TW-prop00", Key: strPtr("tweaks/proposed"), Status: "proposed"}},
		},
	}
	checkTree, _ := awaitingCheckTree()
	featureReview, featureIn := reviewPhaseTree(t, store.ReviewGrainFeature)
	epicReview, epicIn := reviewPhaseTree(t, store.ReviewGrainEpic)

	fixtures := []struct {
		tree *store.Tree
		in   workflow.Input
	}{
		{attentionSummaryTree(), workflow.Input{}},
		{shelf, workflow.Input{}},
		{checkTree, workflowContracts(checkTree)},
		{featureReview, featureIn},
		{epicReview, epicIn},
	}
	seen := map[string]bool{}
	for _, fx := range fixtures {
		entries := views.QueueOfInput(fx.tree, views.QueueInput{Workflow: fx.in}).Entries
		cands := enumerate(fx.tree, "", fx.in)
		for _, entry := range entries {
			seen[entry.Kind] = true
			var matches []views.NextCandidate
			for _, c := range candidatesFor(cands, entry.ID) {
				if c.Kind == entry.Kind && c.Command == entry.Command {
					matches = append(matches, c)
				}
			}
			require.Lenf(t, matches, 1, "queue entry %s %s maps to exactly one candidate", entry.Kind, entry.ID)
			match := matches[0]
			require.Equal(t, entry.Noun, match.Node.Noun)
			require.Equal(t, entry.Ref, match.Node.Ref)
			require.Equal(t, views.QueueKindArms[entry.Kind], match.Arm())
			actor := entry.Actor
			if actor == "" {
				actor = "human"
			}
			require.Equal(t, actor, match.Actor, entry.Kind)
			if actor == "agent" {
				require.Equal(t, "execute", match.Skill, entry.Kind)
			} else {
				require.Empty(t, match.Skill, entry.Kind)
			}
			require.NotEmpty(t, match.Action, entry.Kind)
		}
	}
	for _, kind := range views.QueueKinds {
		require.Truef(t, seen[kind], "the fixtures produce no %s entry", kind)
		_, ok := views.QueueKindArms[kind]
		require.Truef(t, ok, "queue kind %s has no arm", kind)
	}
}

func TestEnumerateSpecifiesEveryFrontierDraft(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	first := draftFeatureUnder(t, tree, "F-aaaaa0", "epic-0/first", "epic-0")
	second := draftFeatureUnder(t, tree, "F-bbbbb0", "epic-0/second", "epic-0")
	blocked := draftFeatureUnder(t, tree, "F-ccccc0", "epic-0/blocked", "epic-0")
	blocked.DependsOn = []string{first.ID}
	tree.Epics["E-aaaaa0"].Order = []string{second.ID, first.ID, blocked.ID}

	specs := candidatesOfKind(enumerate(tree, "", workflow.Input{}), "specify")
	require.Equal(t, []string{second.ID, first.ID}, candidateNodeIDs(specs))
	for _, c := range specs {
		require.Equal(t, "agent", c.Actor)
		require.Equal(t, "specify", c.Skill)
		require.Equal(t, "specify", c.Action)
		require.Equal(t, "draft, with every dependency met", c.Reason)
	}

	order := gen.DependencyOrder(tree)
	frontier := views.FrontierFeatures(tree, order)
	require.Len(t, frontier, 2)
	require.Equal(t, frontier[0].ID, specs[0].Node.ID, "specify moves follow frontier order")
}

func TestEnumerateInReviewMergeWaits(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	f.Status, f.ReviewGrain = "active", store.ReviewGrainTask
	pr := 7
	task.Status, task.Owner, task.Started, task.PR = "in_review", "jc", true, &pr
	tree.Chores = map[string]*store.Chore{
		"CH-aaaaa0": {WorkItem: store.WorkItem{ID: "CH-aaaaa0", Key: strPtr("chores/bump"), Status: "in_review", Owner: "jc"}},
	}

	merges := candidatesOfKind(enumerate(tree, "jc", workflow.Input{}), "merge_task")
	require.Equal(t, []string{task.ID, "CH-aaaaa0"}, candidateNodeIDs(merges))
	for _, c := range merges {
		require.Equal(t, "human", c.Actor)
		require.Equal(t, "merge", c.Action)
		require.Equal(t, "tiller sync", c.Command)
	}
	require.Equal(t, "PR #7 awaits your merge", merges[0].Reason)
	require.Equal(t, "in review, waiting on your merge", merges[1].Reason)

	f.ReviewGrain = store.ReviewGrainFeature
	task.PR = nil
	for _, c := range candidatesOfKind(enumerate(tree, "jc", workflow.Input{}), "merge_task") {
		require.NotEqual(t, task.ID, c.Node.ID, "a coarse task whose feature records no review is not a human wait")
	}
}

func TestEnumerateEmptyTree(t *testing.T) {
	tree := &store.Tree{Manifest: &store.Manifest{}, ToBeDesign: []string{"vision.md"}}
	cands := enumerate(tree, "jc", workflow.Input{})
	require.Len(t, cands, 1)
	require.Equal(t, "decompose", cands[0].Kind)
	require.Equal(t, "agent", cands[0].Actor)
	require.Equal(t, "decompose", cands[0].Skill)
	require.Contains(t, cands[0].Reason, "vision.md")

	tree.ToBeDesign = nil
	require.Empty(t, enumerate(tree, "jc", workflow.Input{}))
}

// Closeout and aggregate merge rows keep their reason to one line: the row
// names the closeout phase or the merge wait, and the Needs you card keeps the
// advice's full reason.
func TestEnumerateCloseoutReasonsStayOneLine(t *testing.T) {
	t.Run("recovered review", func(t *testing.T) {
		tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
		pr := 155
		f.PR, f.RecoveredReview = &pr, recoveryFixtureRecord()
		advice := workflow.AdviceOf(tree, f, in)
		require.NotNil(t, advice.RecoveredReview)
		require.Greater(t, len(advice.Reason), 80, "the fixture's full advice is long")

		rows := candidatesFor(enumerate(tree, "", in), f.ID)
		require.Len(t, rows, 1)
		require.Equal(t, "workflow_closeout", rows[0].Kind)
		require.Equal(t, "shared closeout of a recovered review: "+strings.ReplaceAll(advice.Phase, "_", " "), rows[0].Reason)
		require.NotContains(t, rows[0].Reason, rows[0].Node.Ref)
		if rows[0].Command != "" {
			require.NotContains(t, rows[0].Reason, rows[0].Command)
		}
	})
	t.Run("ordinary closeout", func(t *testing.T) {
		tree, f, in := workflowFeature(t, store.ReviewGrainFeature)
		completeMachine(f)
		advice := workflow.AdviceOf(tree, f, in)
		rows := candidatesFor(enumerate(tree, "", in), f.ID)
		require.Len(t, rows, 1)
		require.Equal(t, "workflow_closeout", rows[0].Kind)
		require.Equal(t, "shared closeout: "+strings.ReplaceAll(advice.Phase, "_", " "), rows[0].Reason)
	})
	t.Run("feature check", func(t *testing.T) {
		tree, f, in := workflowFeature(t, store.ReviewGrainTask)
		advice := workflow.AdviceOf(tree, f, in)
		rows := candidatesFor(enumerate(tree, "", in), f.ID)
		require.Len(t, rows, 1)
		require.Equal(t, "feature_check", rows[0].Kind)
		require.Equal(t, "shared closeout: "+strings.ReplaceAll(advice.Phase, "_", " "), rows[0].Reason)
	})
	t.Run("aggregate merge with a PR", func(t *testing.T) {
		tree, in := reviewPhaseTree(t, store.ReviewGrainFeature)
		rows := candidatesFor(enumerate(tree, "", in), "F-aaaaa0")
		require.Len(t, rows, 1)
		require.Equal(t, "merge_feature", rows[0].Kind)
		require.Equal(t, "PR #42 awaits merge", rows[0].Reason, "no reader, no \"your\"")
		rows = candidatesFor(enumerate(tree, "jc", in), "F-aaaaa0")
		require.Equal(t, "PR #42 awaits your merge", rows[0].Reason)
	})
	t.Run("epic merge", func(t *testing.T) {
		tree, in := reviewPhaseTree(t, store.ReviewGrainEpic)
		rows := candidatesFor(enumerate(tree, "", in), "E-aaaaa0")
		require.Len(t, rows, 1)
		require.Equal(t, "merge_epic", rows[0].Kind)
		require.Equal(t, "PR #42 awaits merge", rows[0].Reason)
		rows = candidatesFor(enumerate(tree, "jc", in), "E-aaaaa0")
		require.Equal(t, "PR #42 awaits your merge", rows[0].Reason)
	})
	t.Run("aggregate merge on a review branch", func(t *testing.T) {
		tree, in := reviewPhaseTree(t, store.ReviewGrainFeature)
		f := tree.Features["F-aaaaa0"]
		f.PR, f.ReviewRef = nil, &store.ReviewRef{Branch: "tiller/reviews/features/epic-0/acceptance"}
		rows := candidatesFor(enumerate(tree, "", in), f.ID)
		require.Len(t, rows, 1)
		require.Equal(t, "merge_feature", rows[0].Kind)
		require.Equal(t, "in review on branch tiller/reviews/features/epic-0/acceptance, waiting on merge", rows[0].Reason)
	})
}
