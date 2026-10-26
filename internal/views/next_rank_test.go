package views_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// nextFor ranks tree's next moves for reader at locateNow.
func nextFor(tree *store.Tree, reader string, in workflow.Input, evs ...events.Event) views.Next {
	return views.NextOf(tree, views.NextInput{Reader: reader, Workflow: in, Recent: evs, Now: locateNow})
}

// rowFor is the one ranked row naming the node id, and its index.
func rowFor(t *testing.T, next views.Next, id string) (views.NextCandidate, int) {
	t.Helper()
	for i, c := range next.Candidates {
		if c.Node != nil && c.Node.ID == id {
			return c, i
		}
	}
	require.Failf(t, "no ranked row", "node %s", id)
	return views.NextCandidate{}, -1
}

// readyEpics builds n approved epics (epic-0 first in plan order).
func readyEpics(t *testing.T, n int) *store.Tree {
	t.Helper()
	tree := treeWithDraftEpics(t, n)
	for _, e := range tree.Epics {
		e.Status = "ready"
	}
	return tree
}

// addTask adds one task to feature f with the given status and owner.
func addTask(tree *store.Tree, f *store.Feature, id, slug, status, owner string) *store.Task {
	task := &store.Task{ID: id, Key: strPtr(*f.Key + "/" + slug), Status: status, Owner: owner, FeatureID: f.ID,
		Started: status == "in_progress" || status == "check"}
	tree.Tasks[task.ID] = task
	tree.FeatureTasks[f.ID] = append(tree.FeatureTasks[f.ID], task)
	f.Order = append(f.Order, task.ID)
	return task
}

// datedQuestion opens a question blocking node with the given due value.
func datedQuestion(tree *store.Tree, id, node string, due store.QuestionDue) {
	holdClaimableNode(tree, id, node)
	tree.Questions[id].Due = due
}

func TestRankUnblockNearbyLeads(t *testing.T) {
	tree := readyEpics(t, 1)
	f, held := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	f.Status = "active"
	f.Order = []string{held.ID}
	mine := addTask(tree, f, "T-ccccc0", "mine", "in_progress", "jc")
	addTask(tree, f, "T-bbbbb0", "added", "proposed", "")
	holdClaimableNode(tree, "Q-aaaaa0", held.ID)

	next := nextFor(tree, "jc", workflow.Input{}, locateEvent("a", "start.task", "jc", 1, mine.ID))
	require.Equal(t, "jc", next.Reader)
	require.GreaterOrEqual(t, len(next.Candidates), 3)
	top := next.Candidates[:3]
	require.Equal(t, []string{"answer_question", "approve_tasks", "continue"}, []string{top[0].Kind, top[1].Kind, top[2].Kind})
	require.Equal(t, []int{1, 1, 2}, []int{top[0].Tier, top[1].Tier, top[2].Tier})
	require.Equal(t, "Q-aaaaa0", top[0].Node.ID)
	require.Equal(t, fmt.Sprintf("holds %s in your current feature; Which schema?", gen.Ref(tree, held)), top[0].Reason)
	require.Equal(t, f.ID, top[1].Node.ID)
	require.Equal(t, "in your current feature", top[1].Reason, "a move on the current feature itself names only where it sits")
	require.Equal(t, mine.ID, top[2].Node.ID)
	require.Equal(t, "your task is in progress", top[2].Reason)
}

func TestRankKeepsTopContinue(t *testing.T) {
	tree := readyEpics(t, 1)
	f, mine := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	f.Status = "active"
	mine.Owner, mine.Status, mine.Started = "jc", "in_progress", true
	f.Order = []string{mine.ID}
	for i := 0; i < 6; i++ {
		task := addTask(tree, f, fmt.Sprintf("T-held0%d", i), fmt.Sprintf("held-%d", i), "ready", "")
		holdClaimableNode(tree, fmt.Sprintf("Q-held0%d", i), task.ID)
	}

	next := nextFor(tree, "jc", workflow.Input{}, locateEvent("a", "start.task", "jc", 0, mine.ID))
	require.Len(t, next.Candidates, 7)
	for _, c := range next.Candidates[:6] {
		require.Equal(t, 1, c.Tier)
	}
	short := next.Shortlist()
	require.Len(t, short, 5)
	for _, c := range short[:4] {
		require.Equal(t, "answer_question", c.Kind)
	}
	require.Equal(t, "continue", short[4].Kind, "the top Continue takes the last place")
	require.Equal(t, mine.ID, short[4].Node.ID)
}

func TestRankNearbyFeatureThenEpic(t *testing.T) {
	tree := readyEpics(t, 2)
	early, earlyTask := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/early", "epic-0")
	early.Order = []string{earlyTask.ID}
	current, first := readyFeatureWithTask(t, tree, "F-aaaaa1", "epic-1/current", "epic-1")
	current.Order = []string{first.ID}
	second := addTask(tree, current, "T-bbbbb1", "second", "ready", "")
	_, sibling := readyFeatureWithTask(t, tree, "F-ccccc1", "epic-1/sibling", "epic-1")

	unread := nextFor(tree, "", workflow.Input{})
	require.Equal(t, earlyTask.ID, unread.Candidates[0].Node.ID, "epic-0 is earlier in plan order")

	next := nextFor(tree, "jc", workflow.Input{}, locateEvent("a", "approve.feature", "jc", 1, current.ID))
	ids := candidateNodeIDs(next.Candidates)
	require.Equal(t, []string{first.ID, second.ID, sibling.ID, earlyTask.ID}, ids)
	for _, c := range next.Candidates[:2] {
		require.Equal(t, 3, c.Tier)
		require.Equal(t, "next in your current feature", c.Reason)
	}
	require.Equal(t, 3, next.Candidates[2].Tier)
	require.Equal(t, "in your current epic", next.Candidates[2].Reason)
	require.Equal(t, 5, next.Candidates[3].Tier)
}

func TestRankRecentlyTouchedMostRecentFirst(t *testing.T) {
	tree := readyEpics(t, 5)
	current, _ := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/current", "epic-0")
	nine, nineTask := readyFeatureWithTask(t, tree, "F-aaaaa1", "epic-1/nine", "epic-1")
	two, twoTask := readyFeatureWithTask(t, tree, "F-aaaaa2", "epic-2/two", "epic-2")
	one, oneTask := readyFeatureWithTask(t, tree, "F-aaaaa3", "epic-3/one", "epic-3")
	zero, zeroTask := readyFeatureWithTask(t, tree, "F-aaaaa4", "epic-4/zero", "epic-4")
	_, cousin := readyFeatureWithTask(t, tree, "F-bbbbb1", "epic-1/cousin", "epic-1")

	at := func(id string, ago time.Duration, target string) events.Event {
		return events.Event{ID: id, Type: "spec.feature", Contributor: "jc", At: locateNow.Add(-ago), Targets: []string{target}}
	}
	day := 24 * time.Hour
	next := nextFor(tree, "jc", workflow.Input{},
		at("cur", time.Hour, current.ID),
		at("zero", 2*time.Hour, zero.ID),
		at("one", day+time.Hour, one.ID),
		at("two", 2*day, two.ID),
		at("nine", 9*day, nine.ID),
	)

	want := []struct {
		task   *store.Task
		reason string
	}{
		{zeroTask, fmt.Sprintf("in %s, which you worked on today", gen.Ref(tree, zero))},
		{oneTask, fmt.Sprintf("in %s, which you worked on yesterday", gen.Ref(tree, one))},
		{twoTask, fmt.Sprintf("in %s, which you worked on 2 days ago", gen.Ref(tree, two))},
		{nineTask, fmt.Sprintf("in %s, which you worked on 9 days ago", gen.Ref(tree, nine))},
	}
	last := -1
	for _, w := range want {
		row, i := rowFor(t, next, w.task.ID)
		require.Equal(t, 4, row.Tier, w.task.ID)
		require.Equal(t, w.reason, row.Reason)
		require.Greater(t, i, last, "most recent first")
		last = i
	}

	row, _ := rowFor(t, next, cousin.ID)
	require.Equal(t, 4, row.Tier, "an untouched feature in a touched epic ranks by the epic's recency")
	require.Equal(t, fmt.Sprintf("in %s, which you worked on 9 days ago", gen.Ref(tree, tree.Epics["E-aaaaa1"])), row.Reason)
}

func TestRankEverythingElseKeepsArmThenPlanOrder(t *testing.T) {
	tree := readyEpics(t, 1)
	draftFeatureUnder(t, tree, "F-aaaaa0", "epic-0/draft", "epic-0")
	specced := draftFeatureUnder(t, tree, "F-bbbbb0", "epic-0/specced", "epic-0")
	specced.Status = "specced"
	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaa0": {ID: "A-aaaaa0", Key: strPtr("rework"), Title: "Rework", Status: store.AmendmentStatusProposed},
	}

	next := nextFor(tree, "", workflow.Input{})
	kinds := make([]string, 0, len(next.Candidates))
	for _, c := range next.Candidates {
		kinds = append(kinds, c.Kind)
		require.Equal(t, 5, c.Tier)
	}
	require.Equal(t, []string{"approve_amendment", "approve_feature", "specify"}, kinds)
}

func TestRankOneRowPerNode(t *testing.T) {
	tree := readyEpics(t, 1)
	tree.Manifest.Gates.Optional = map[string]store.GateSpec{
		"design_review": {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
	}
	f, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	f.Status, f.ReviewGrain = "active", store.ReviewGrainTask
	pr := 7
	task.Status, task.Owner, task.Started, task.PR = "in_review", "jc", true, &pr

	both := views.CandidatesOf(tree, gen.DependencyOrder(tree), views.NextInput{Reader: "jc"})
	require.Len(t, candidatesFor(both, task.ID), 2, "the task is admitted by the merge and the attest arms")

	next := nextFor(tree, "jc", workflow.Input{}, locateEvent("a", "submit.task", "jc", 0, task.ID))
	require.Len(t, candidatesFor(next.Candidates, task.ID), 1)
	row, _ := rowFor(t, next, task.ID)
	require.Equal(t, "merge_task", row.Kind, "the merge arm ranks ahead of attest")
	require.Equal(t, 1, row.Tier)

	specTree := readyEpics(t, 1)
	specced := draftFeatureUnder(t, specTree, "F-aaaaa0", "epic-0/plan", "epic-0")
	specced.Status = "specced"
	next = nextFor(specTree, "jc", workflow.Input{}, locateEvent("a", "spec.feature", "jc", 0, specced.ID))
	rows := candidatesFor(next.Candidates, specced.ID)
	require.Len(t, rows, 1)
	require.Equal(t, "approve_feature", rows[0].Kind)
	require.Equal(t, 1, rows[0].Tier)
}

func TestNextWithoutReaderIsPlanOrder(t *testing.T) {
	tree := readyEpics(t, 2)
	early, earlyTask := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/early", "epic-0")
	early.Order = []string{earlyTask.ID}
	current, owned := readyFeatureWithTask(t, tree, "F-aaaaa1", "epic-1/current", "epic-1")
	current.Status = "active"
	owned.Owner, owned.Status, owned.Started = "jc", "in_progress", true
	addTask(tree, current, "T-bbbbb1", "next", "ready", "")
	specced := draftFeatureUnder(t, tree, "F-ccccc1", "epic-1/plan", "epic-1")
	specced.Status = "specced"
	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaa0": {ID: "A-aaaaa0", Key: strPtr("rework"), Title: "Rework", Status: store.AmendmentStatusProposed},
	}

	next := nextFor(tree, "", workflow.Input{},
		locateEvent("a", "start.task", "jc", 0, owned.ID),
		locateEvent("b", "spec.feature", "jc", 3, specced.ID),
	)
	require.Equal(t, "", next.Reader)
	require.NotEmpty(t, next.Candidates)
	for _, c := range next.Candidates {
		require.Equal(t, 5, c.Tier, "%s %s", c.Kind, c.Node.ID)
		require.NotContains(t, c.Reason, "your", "%s %s", c.Kind, c.Node.ID)
	}
	require.Equal(t, "approve_amendment", next.Candidates[0].Kind)
	_, earlyAt := rowFor(t, next, earlyTask.ID)
	_, nextAt := rowFor(t, next, "T-bbbbb1")
	require.Less(t, earlyAt, nextAt, "plan order, not the events")
}

func TestNextCaughtUp(t *testing.T) {
	tree := readyEpics(t, 1)
	_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	task.Blocked = true

	next := nextFor(tree, "jc", workflow.Input{})
	require.True(t, next.CaughtUp)
	require.False(t, next.Unplanned)
	require.NotNil(t, next.Candidates)
	require.Empty(t, next.Candidates)
	signpost := views.PoolSignpost(tree, gen.DependencyOrder(tree), workflow.Input{})
	require.NotEmpty(t, signpost)
	require.Equal(t, signpost, next.Hint)
	raw, err := json.Marshal(next)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"candidates":[]`)

	empty := &store.Tree{Manifest: &store.Manifest{}}
	next = nextFor(empty, "jc", workflow.Input{})
	require.True(t, next.CaughtUp)
	require.True(t, next.Unplanned)
	require.Equal(t, views.UnplannedReason(empty), next.Hint)

	busy := readyEpics(t, 1)
	readyFeatureWithTask(t, busy, "F-aaaaa0", "epic-0/work", "epic-0")
	next = nextFor(busy, "jc", workflow.Input{})
	require.False(t, next.CaughtUp)
	require.Empty(t, next.Hint)
}

func TestNextCarriesDependencyDiagnosticsOnlyWithFindings(t *testing.T) {
	tree := readyEpics(t, 1)
	f, first := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	require.Nil(t, nextFor(tree, "jc", workflow.Input{}).DependencyDiagnostics)

	second := addTask(tree, f, "T-bbbbb0", "second", "ready", "")
	first.DependsOn = []string{second.ID}
	second.DependsOn = []string{first.ID}
	diagnostics := nextFor(tree, "jc", workflow.Input{}).DependencyDiagnostics
	require.NotNil(t, diagnostics)
	require.NotEmpty(t, diagnostics.Gaps)
}

func TestTopAgentSkipsHumanMoves(t *testing.T) {
	next := views.Next{Candidates: []views.NextCandidate{
		{Actor: "human", Kind: "answer_question"},
		{Actor: "human", Kind: "approve_feature"},
		{Actor: "agent", Kind: "claim_task", Skill: "execute"},
		{Actor: "agent", Kind: "specify", Skill: "specify"},
	}}
	top, ok := next.TopAgent()
	require.True(t, ok)
	require.Equal(t, "claim_task", top.Kind)

	_, ok = views.Next{Candidates: next.Candidates[:2]}.TopAgent()
	require.False(t, ok)
	require.Len(t, views.Next{Candidates: next.Candidates[:2]}.Shortlist(), 2, "fewer than five lists what exists")
}

func TestNextUnplannedKeepsItsHintBesideCandidates(t *testing.T) {
	tree := &store.Tree{Manifest: &store.Manifest{}, Questions: map[string]*store.Question{
		"Q-aaaaa0": {ID: "Q-aaaaa0", Title: "Which market?", Statement: "Which market first?", Status: "open"},
	}}

	next := nextFor(tree, "jc", workflow.Input{})
	require.Len(t, next.Candidates, 1)
	require.Equal(t, "answer_question", next.Candidates[0].Kind)
	require.False(t, next.CaughtUp)
	require.True(t, next.Unplanned, "the zero-day guidance stays beside the open question")
	require.Equal(t, views.UnplannedReason(tree), next.Hint)

	tree.Bugs = map[string]*store.Bug{
		"B-aaaaa0": {WorkItem: store.WorkItem{ID: "B-aaaaa0", Key: strPtr("bugs/crash"), Title: "Crash", Status: "ready", Owner: "jc"}},
	}
	next = nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"continue", "answer_question"}, candidateKinds(next.Candidates))
	require.False(t, next.Unplanned, "live work beside the question is not the no-work-planned state")
	require.Empty(t, next.Hint)
	tree.Bugs = nil

	tree.Manifest.Project.Kind = store.KindBrownfield
	next = nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, "derive_baseline", next.Candidates[0].Kind)
	require.False(t, next.Unplanned, "a pending baseline is not the no-work-planned end state")
	require.Empty(t, next.Hint)
}

func TestShortlistKeepsTheTopContinueNodeThroughDedupe(t *testing.T) {
	tree := readyEpics(t, 1)
	f, checking := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	f.Status = "active"
	f.Order = []string{checking.ID}
	checking.Owner, checking.Status, checking.Started = "jc", "check", true
	checking.GatesBlock = store.GatesBlock{Gates: map[string]store.Gate{
		"evals": {Criteria: map[string]store.Criterion{"manual": {Status: store.CriterionStatusNeedsAttestation}}},
	}}
	other := addTask(tree, f, "T-other0", "other", "in_progress", "jc")
	for i := 0; i < 6; i++ {
		task := addTask(tree, f, fmt.Sprintf("T-held0%d", i), fmt.Sprintf("held-%d", i), "ready", "")
		holdClaimableNode(tree, fmt.Sprintf("Q-held0%d", i), task.ID)
	}

	next := nextFor(tree, "jc", workflow.Input{}, locateEvent("a", "submit.task", "jc", 0, checking.ID))
	row, at := rowFor(t, next, checking.ID)
	require.Equal(t, "attest_criterion", row.Kind, "dedupe keeps the tier 1 row for the checked task")
	require.Greater(t, at, 4)
	otherRow, otherAt := rowFor(t, next, other.ID)
	require.Equal(t, 2, otherRow.Tier)
	require.Greater(t, otherAt, at)

	short := next.Shortlist()
	require.Len(t, short, 5)
	require.Equal(t, checking.ID, short[4].Node.ID, "the top Continue node keeps its place, not another task")
	require.Equal(t, "attest_criterion", short[4].Kind)
}

func TestShortlistReturnsACopy(t *testing.T) {
	next := views.Next{Candidates: []views.NextCandidate{{Actor: "human", Kind: "answer_question"}}}
	short := next.Shortlist()
	short[0].Kind = "changed"
	require.Equal(t, "answer_question", next.Candidates[0].Kind)
}

func TestRankContinueReasons(t *testing.T) {
	tree := readyEpics(t, 1)
	f, observed := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	f.Status = "active"
	f.Order = []string{observed.ID}
	checking := addTask(tree, f, "T-check0", "check", "check", "jc")
	claimed := addTask(tree, f, "T-claim0", "claimed", "ready", "jc")
	tree.Bugs = map[string]*store.Bug{
		"B-aaaaa0": {WorkItem: store.WorkItem{ID: "B-aaaaa0", Key: strPtr("bugs/crash"), Title: "Crash", Status: "in_progress", Owner: "jc"}},
	}
	tree.Chores = map[string]*store.Chore{
		"CH-aaaaa0": {WorkItem: store.WorkItem{ID: "CH-aaaaa0", Key: strPtr("chores/tidy"), Title: "Tidy", Status: "check", Owner: "jc"}},
	}

	next := nextFor(tree, "jc", observedOn(observed.ID, "t/x", "jc"))
	for id, want := range map[string]string{
		checking.ID: "your task is in check; finish any outstanding checks, then publish and submit it",
		"CH-aaaaa0": "your chore is in check; finish any outstanding checks, then publish and submit it",
		claimed.ID:  "your task is claimed and ready to start",
		observed.ID: "your task, in flight on its branch",
		"B-aaaaa0":  "your bug is in progress",
	} {
		row, _ := rowFor(t, next, id)
		require.Equal(t, 2, row.Tier, id)
		require.Equal(t, want, row.Reason, id)
	}
}

func TestRankHoldThroughTheCurrentEpic(t *testing.T) {
	tree, in := reviewPhaseTree(t, store.ReviewGrainEpic)
	f := tree.Features["F-aaaaa0"]
	next := nextFor(tree, "jc", in, locateEvent("a", "submit.task", "jc", 0, "T-aaaaa0"))
	row, _ := rowFor(t, next, "E-aaaaa0")
	require.Equal(t, "merge_epic", row.Kind)
	require.Equal(t, 1, row.Tier)
	require.Equal(t, fmt.Sprintf("holds %s, your current feature; PR #42 awaits your merge", gen.Ref(tree, f)), row.Reason)
}

// Tier 4 ranks by whole elapsed days, the grain its copy speaks in: two
// features touched minutes apart today keep plan order.
func TestRankSameDayTouchesKeepPlanOrder(t *testing.T) {
	tree := readyEpics(t, 3)
	current, _ := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/current", "epic-0")
	early, earlyTask := readyFeatureWithTask(t, tree, "F-aaaaa1", "epic-1/early", "epic-1")
	late, lateTask := readyFeatureWithTask(t, tree, "F-aaaaa2", "epic-2/late", "epic-2")

	at := func(id string, ago time.Duration, target string) events.Event {
		return events.Event{ID: id, Type: "spec.feature", Contributor: "jc", At: locateNow.Add(-ago), Targets: []string{target}}
	}
	next := nextFor(tree, "jc", workflow.Input{},
		at("cur", time.Minute, current.ID),
		at("late", 10*time.Minute, late.ID),
		at("early", 3*time.Hour, early.ID),
	)
	earlyRow, earlyAt := rowFor(t, next, earlyTask.ID)
	lateRow, lateAt := rowFor(t, next, lateTask.ID)
	require.Equal(t, 4, earlyRow.Tier)
	require.Equal(t, 4, lateRow.Tier)
	require.Less(t, earlyAt, lateAt, "same-day touches fall through to plan order")
	require.Equal(t, fmt.Sprintf("in %s, which you worked on today", gen.Ref(tree, early)), earlyRow.Reason)
}

// A human decision placed by locality keeps what it waits on after the tier
// reason; an agent move keeps the tier reason alone.
func TestRankHumanLocalityReasonsKeepTheirBase(t *testing.T) {
	tree := readyEpics(t, 2)
	current, currentTask := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/current", "epic-0")
	sibling := draftFeatureUnder(t, tree, "F-bbbbb0", "epic-0/sibling", "epic-0")
	sibling.Status = "specced"
	touched, _ := readyFeatureWithTask(t, tree, "F-aaaaa1", "epic-1/touched", "epic-1")
	other := draftFeatureUnder(t, tree, "F-bbbbb1", "epic-1/other", "epic-1")
	other.Status = "specced"

	next := nextFor(tree, "jc", workflow.Input{},
		locateEvent("a", "spec.feature", "jc", 0, current.ID),
		locateEvent("b", "spec.feature", "jc", 2, touched.ID),
	)
	row, _ := rowFor(t, next, sibling.ID)
	require.Equal(t, 3, row.Tier)
	require.Equal(t, "in your current epic; its feature plan awaits your approval", row.Reason)
	row, _ = rowFor(t, next, other.ID)
	require.Equal(t, 4, row.Tier)
	require.Equal(t, fmt.Sprintf("in %s, which you worked on 2 days ago; its feature plan awaits your approval",
		gen.Ref(tree, tree.Epics["E-aaaaa1"])), row.Reason)
	row, _ = rowFor(t, next, currentTask.ID)
	require.Equal(t, "next in your current feature", row.Reason, "an agent move keeps its tier reason alone")
}
