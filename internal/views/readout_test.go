package views_test

// Readout tests cover both moment-specific content and the tree-wide,
// queue-derived Needs attention summaries shared by every moment.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func TestApprovedShelfReadoutReportsFreshLegalMoves(t *testing.T) {
	for noun, id := range map[string]string{store.NounBug: "B-aaaaaa", store.NounChore: "CH-aaaaaa", store.NounTweak: "TW-aaaaaa"} {
		t.Run(noun, func(t *testing.T) {
			work := store.WorkItem{ID: id, Status: "ready", Title: "Button text"}
			tree := &store.Tree{
				Manifest: &store.Manifest{},
				Bugs:     map[string]*store.Bug{},
				Chores:   map[string]*store.Chore{},
				Tweaks:   map[string]*store.Tweak{},
			}
			switch noun {
			case store.NounBug:
				tree.Bugs[work.ID] = &store.Bug{WorkItem: work}
			case store.NounChore:
				tree.Chores[work.ID] = &store.Chore{WorkItem: work}
			case store.NounTweak:
				tree.Tweaks[work.ID] = &store.Tweak{WorkItem: work}
			}
			in := views.ReadoutInput{Act: &views.GateAct{
				Nodes: []views.GateActNode{{Noun: noun, ID: work.ID, Changed: true}},
			}}
			readout := views.ReadoutOfInput(tree, views.MomentApprove, in)
			require.Equal(t, "cancel, return, start", checkLabels(readout)["Now legal on the node(s) just approved"])
		})
	}
}

// treeWithDraftEpics builds a tree carrying n draft epics and nothing else,
// enough to exercise QueueOf's approve_epic group (queue.go's first group)
// without a full fixture. The zero-value Manifest is required, not
// decorative: queueAttestGates (queue.go) reads t.Manifest.Git.PRGrain
// unconditionally on every call, so a nil Manifest panics QueueOf before it
// ever reaches the epics this test cares about.
func treeWithDraftEpics(t *testing.T, n int) *store.Tree {
	t.Helper()
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Epics:    map[string]*store.Epic{},
		Features: map[string]*store.Feature{},
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("E-aaaaa%d", i)
		key := fmt.Sprintf("epic-%d", i)
		tree.Epics[id] = &store.Epic{ID: id, Key: &key, Title: key, Status: "draft"}
	}
	return tree
}

// feat builds a minimal *store.Feature for depcheck_test.go's fixtures.
// DependencyShape (depcheck.go) compares two features' EpicKey fields
// directly, never resolving either through tree.Epics, so epicID only needs
// to distinguish one owning epic from another within a single fixture; it
// does not need to equal any real epic's stored Key.
func feat(t *testing.T, id, key, epicID string) *store.Feature {
	t.Helper()
	return &store.Feature{ID: id, Key: &key, EpicKey: epicID}
}

// featureKeyOf is the canonical key of the feature a fixture task hangs off,
// the prefix its own canonical key carries (artifact-schema.md, Keys and paths).
func featureKeyOf(t *testing.T, tree *store.Tree, featureID string) string {
	t.Helper()
	f := tree.Features[featureID]
	require.NotNil(t, f)
	require.NotNil(t, f.Key)
	return *f.Key
}

// attentionSummaryTree carries every queue kind at once. The fixture's
// concrete ids, keys, prose, gate names, criterion ids, and mutation commands
// are deliberately distinctive so the companion leak test can prove that the
// readout exposes only exact category counts and queue drill-down commands.
func attentionSummaryTree() *store.Tree {
	criterion := func(id string) store.GatesBlock {
		return store.GatesBlock{Gates: map[string]store.Gate{
			"evals": {Criteria: map[string]store.Criterion{
				id: {Status: store.CriterionStatusNeedsAttestation},
			}},
		}}
	}

	epicAKey, epicBKey, workKey := "private-epic-alpha", "private-epic-beta", "private-work"
	planKey := "private-work/private-plan"
	batchAKey, batchBKey := "private-work/private-batch-alpha", "private-work/private-batch-beta"
	reviewKey := "private-work/private-review"
	criterionAKey, criterionBKey := "private-work/private-criterion-alpha", "private-work/private-criterion-beta"
	checkKey := "private-work/private-check"
	batchATaskKey, batchBTaskKey := batchAKey+"/private-task", batchBKey+"/private-task"
	reviewTaskKey, checkTaskKey := reviewKey+"/private-task", checkKey+"/private-task"

	batchATask := &store.Task{ID: "T-aaaaaa", Key: &batchATaskKey, Title: "Private batch task alpha", Status: "proposed", FeatureID: "F-bbbbbb"}
	batchBTask := &store.Task{ID: "T-bbbbbb", Key: &batchBTaskKey, Title: "Private batch task beta", Status: "proposed", FeatureID: "F-cccccc"}
	reviewTask := &store.Task{
		ID: "T-cccccc", Key: &reviewTaskKey, Title: "Private review task", Status: "in_review", FeatureID: "F-dddddd",
		GatesBlock: criterion("PRIVATE_TASK_CRITERION"),
	}
	checkTask := &store.Task{ID: "T-dddddd", Key: &checkTaskKey, Title: "Private completed task", Status: "done", FeatureID: "F-gggggg"}

	return &store.Tree{
		Manifest: &store.Manifest{Gates: store.GatesConfig{
			Required: map[string]store.GateSpec{
				"evals": {Kind: "command", On: store.StringList{"feature"}},
			},
			Optional: map[string]store.GateSpec{
				"private_gate_alpha": {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
				"private_gate_beta":  {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
			},
		}},
		Amendments: map[string]*store.Amendment{
			"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("private-amendment"), Title: "Private amendment title", Status: store.AmendmentStatusProposed},
		},
		Epics: map[string]*store.Epic{
			"E-aaaaaa": {ID: "E-aaaaaa", Key: &epicAKey, Title: "Private epic alpha", Status: "draft"},
			"E-bbbbbb": {ID: "E-bbbbbb", Key: &epicBKey, Title: "Private epic beta", Status: "draft"},
			"E-cccccc": {ID: "E-cccccc", Key: &workKey, Title: "Private work epic", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-aaaaaa": {ID: "F-aaaaaa", Key: &planKey, EpicKey: workKey, Title: "Private plan title", Status: "specced"},
			"F-bbbbbb": {
				ID: "F-bbbbbb", Key: &batchAKey, EpicKey: workKey, Title: "Private batch alpha", Status: "ready",
			},
			"F-cccccc": {
				ID: "F-cccccc", Key: &batchBKey, EpicKey: workKey, Title: "Private batch beta", Status: "ready",
			},
			"F-dddddd": {
				ID: "F-dddddd", Key: &reviewKey, EpicKey: workKey, Title: "Private review feature", Status: "active",
			},
			"F-eeeeee": {
				ID: "F-eeeeee", Key: &criterionAKey, EpicKey: workKey, Title: "Private criterion alpha", Status: "done",
				GatesBlock: criterion("PRIVATE_FEATURE_CRITERION_ALPHA"),
			},
			"F-ffffff": {
				ID: "F-ffffff", Key: &criterionBKey, EpicKey: workKey, Title: "Private criterion beta", Status: "done",
				GatesBlock: criterion("PRIVATE_FEATURE_CRITERION_BETA"),
			},
			"F-gggggg": {
				ID: "F-gggggg", Key: &checkKey, EpicKey: workKey, Title: "Private feature check title", Status: "active",
			},
		},
		Tasks: map[string]*store.Task{
			batchATask.ID: batchATask,
			batchBTask.ID: batchBTask,
			reviewTask.ID: reviewTask,
			checkTask.ID:  checkTask,
		},
		FeatureTasks: map[string][]*store.Task{
			"F-bbbbbb": {batchATask},
			"F-cccccc": {batchBTask},
			"F-dddddd": {reviewTask},
			"F-gggggg": {checkTask},
		},
		Questions: map[string]*store.Question{
			"Q-aaaaaa": {ID: "Q-aaaaaa", Title: "Private question alpha", Statement: "PRIVATE QUESTION STATEMENT ALPHA", Status: "open"},
			"Q-bbbbbb": {ID: "Q-bbbbbb", Title: "Private question beta", Statement: "PRIVATE QUESTION STATEMENT BETA", Status: "open"},
			"Q-cccccc": {ID: "Q-cccccc", Title: "Private question gamma", Statement: "PRIVATE QUESTION STATEMENT GAMMA", Status: "open"},
		},
	}
}

func TestNeedsAttentionSummarizesEveryQueueCategoryExactly(t *testing.T) {
	tree := attentionSummaryTree()
	q := views.QueueOf(tree)
	require.Len(t, q.Entries, 15, "the fixture carries all eight kinds; criterion is split by noun")
	kinds := map[string]bool{}
	for _, entry := range q.Entries {
		kinds[entry.Kind] = true
	}
	require.Equal(t, map[string]bool{
		"approve_amendment": true,
		"approve_epic":      true,
		"approve_feature":   true,
		"approve_tasks":     true,
		"answer_question":   true,
		"attest_gate":       true,
		"attest_criterion":  true,
		"workflow_closeout": true,
	}, kinds)

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})

	commands := make([]string, 0, len(r.NeedsYou))
	for _, entry := range r.NeedsYou {
		commands = append(commands, entry.Command)
	}
	require.Equal(t, []string{
		"tiller queue --kind approve_amendment",
		"tiller queue --kind approve_epic",
		"tiller queue --kind approve_feature",
		"tiller queue --kind approve_tasks",
		"tiller queue --kind answer_question",
		"tiller queue --kind attest_gate",
		"tiller queue --kind attest_criterion --noun feature",
		"tiller queue --kind attest_criterion --noun task",
		"tiller queue --kind workflow_closeout",
	}, commands)
}

func TestReadoutNeedsAttentionCountsShelfQueueKinds(t *testing.T) {
	proposedBug, returnedBug, proposedChore := "bugs/proposed", "bugs/returned", "chores/proposed"
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Bugs: map[string]*store.Bug{
			"B-prop00": {WorkItem: store.WorkItem{ID: "B-prop00", Key: &proposedBug, Status: "proposed"}},
			"B-return": {WorkItem: store.WorkItem{ID: "B-return", Key: &returnedBug, Status: "ready", ResolvedBy: "F-gone00"}},
		},
		Chores: map[string]*store.Chore{
			"CH-prop00": {WorkItem: store.WorkItem{ID: "CH-prop00", Key: &proposedChore, Status: "proposed"}},
		},
	}

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	require.Equal(t, []views.ReadoutAction{
		{Text: "1 bug awaits triage", Command: "tiller queue --kind approve_bug"},
		{Text: "1 chore awaits triage", Command: "tiller queue --kind approve_chore"},
		{Text: "1 shelf item has a cancelled carrier", Command: "tiller queue --kind carrier_cancelled"},
	}, r.NeedsYou)
}

func TestNeedsAttentionQueueFilterPreservesOrderAndReturnsANonNilEmptySlice(t *testing.T) {
	input := views.Queue{Entries: []views.QueueEntry{
		{Kind: "approve_epic", Noun: "epic", ID: "first"},
		{Kind: "attest_criterion", Noun: "feature", ID: "second"},
		{Kind: "attest_criterion", Noun: "task", ID: "third"},
		{Kind: "attest_criterion", Noun: "feature", ID: "fourth"},
	}}

	got := views.FilterQueue(input, views.QueueFilter{Kind: "attest_criterion", Noun: "feature"})
	require.Equal(t, []views.QueueEntry{input.Entries[1], input.Entries[3]}, got.Entries,
		"filtering keeps the queue's existing order")

	empty := views.FilterQueue(input, views.QueueFilter{Kind: "feature_check", Noun: "task"})
	require.NotNil(t, empty.Entries, "empty filters stay an array in JSON")
	require.Empty(t, empty.Entries)
}

func TestNeedsAttentionRejectsConcreteQueueItemsAndMutationCommands(t *testing.T) {
	tree := attentionSummaryTree()
	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})
	var summary strings.Builder
	for _, action := range r.NeedsYou {
		summary.WriteString(action.Text)
		summary.WriteByte('\n')
		summary.WriteString(action.Command)
		summary.WriteByte('\n')
	}
	got := summary.String()

	for _, entry := range views.QueueOf(tree).Entries {
		for _, concrete := range []string{entry.ID, entry.Key, entry.Ref, entry.Prose, entry.Gate, entry.Command} {
			if concrete != "" {
				require.NotContainsf(t, got, concrete, "count summaries must not leak concrete queue entry %q", concrete)
			}
		}
	}
	for _, question := range tree.Questions {
		require.NotContains(t, got, question.Statement, "question statements stay behind the queue drill-down")
	}
	for _, mutation := range []string{"tiller approve ", "tiller answer ", "tiller attest ", "tiller check feature "} {
		require.NotContainsf(t, got, mutation, "readout commands enumerate; they never mutate via %q", mutation)
	}
}

func TestReadoutUsesQueueInputToSuppressBasisDriftedFeatureChecks(t *testing.T) {
	tree, feature := awaitingCheckTree()

	clean := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Workflow: workflowContracts(tree)})
	require.Len(t, clean.NeedsYou, 1)
	require.Equal(t, "tiller queue --kind feature_check", clean.NeedsYou[0].Command)

	drifted := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{
		QueueInput: views.QueueInput{BasisDrifted: map[string]bool{feature.ID: true}},
		Workflow:   workflowContracts(tree),
	})
	require.Empty(t, drifted.NeedsYou,
		"the readout uses the caller's live basis answer just like /api/queue; it must not advertise a refusing check")
}

// checkLabels flattens a readout's Checks into a label->result map, the
// shape every test below reads its assertions from rather than indexing
// r.Checks positionally, so a reordering of the checks a content function
// appends never breaks a test that does not care about order.
func checkLabels(r views.Readout) map[string]string {
	labels := map[string]string{}
	for _, c := range r.Checks {
		labels[c.Label] = c.Result
	}
	return labels
}

// actionTexts and actionCommands flatten a slice of ReadoutAction down to the
// one field a test cares about, the same "read the shape, not the position"
// discipline checkLabels applies to r.Checks.
func actionTexts(actions []views.ReadoutAction) []string {
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		out = append(out, a.Text)
	}
	return out
}

func actionCommands(actions []views.ReadoutAction) []string {
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		out = append(out, a.Command)
	}
	return out
}

func TestDecomposeReadoutProvesTheTreeRatherThanCountingIt(t *testing.T) {
	tree := treeWithDraftEpics(t, 3)
	in := views.ReadoutInput{
		Commit: "f70abe3",
		Ready:  true,
	}
	r := views.ReadoutOfInput(tree, views.MomentDecompose, in)

	require.Equal(t, "f70abe3", r.Artifact)
	require.Len(t, r.Entities, 3, "every epic is named, not just counted")

	labels := checkLabels(r)
	require.Equal(t, "passed", labels["Readiness"])
	require.Contains(t, labels["Dependency edges recorded"], "cross-epic",
		"the completeness bar is mechanical here, not advisory")
}

func TestDecomposeReadoutCapsItsEntityListAndSaysHowMany(t *testing.T) {
	tree := treeWithDraftEpics(t, 10)
	r := views.ReadoutOfInput(tree, views.MomentDecompose, views.ReadoutInput{})
	require.Len(t, r.Entities, 3, "capped so a large tree never lists every row")
	require.Equal(t, 7, r.Withheld)
}

// TestDecomposeReadoutFiltersDraftCountsAndCountsOpenQuestions pins
// task-3-report.md finding 3: draft epics and draft features are counted by
// Status rather than by tree size (one epic here is approved past draft and
// must not inflate the epic count), and open questions counts only the
// still-open ones, the same gen.QuestionOpen predicate the queue uses.
func TestDecomposeReadoutFiltersDraftCountsAndCountsOpenQuestions(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa1"].Status = "ready" // approved: must not count as draft

	tree.Features["F-aaaaa0"] = feat(t, "F-aaaaa0", "epic-0/a", "epic-0")
	tree.Features["F-aaaaa0"].Status = "draft"
	tree.Features["F-aaaaa1"] = feat(t, "F-aaaaa1", "epic-1/b", "epic-1")
	tree.Features["F-aaaaa1"].Status = "specced" // not draft: must not count

	tree.Questions = map[string]*store.Question{
		"Q-1": {ID: "Q-1", Status: "open"},
		"Q-2": {ID: "Q-2", Status: "answered"}, // closed: must not count
	}

	r := views.ReadoutOfInput(tree, views.MomentDecompose, views.ReadoutInput{})
	labels := checkLabels(r)
	require.Equal(t, "1", labels["Draft epics"], "only E-aaaaa0 is still draft")
	require.Equal(t, "1", labels["Draft features"])
	require.Equal(t, "1", labels["Open questions"])
}

// TestDecomposeReadoutMarksEpicSpecChecksNotEvaluatedWhenNil and
// TestDecomposeReadoutRendersRealEpicSpecCountsWhenSupplied pin
// task-3-report.md finding 1's nil-versus-empty discipline applied to
// EpicSpecs, the second I/O-derived field this round of fixes adds: a caller
// that never read epic.md must not have its silence read back as a real
// "0 of N". The companion "Acceptance bars valid" line went with epic.eval.md,
// since an epic has no acceptance contract of its own.
func TestDecomposeReadoutMarksEpicSpecChecksNotEvaluatedWhenNil(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	r := views.ReadoutOfInput(tree, views.MomentDecompose, views.ReadoutInput{})
	labels := checkLabels(r)
	require.Equal(t, "not evaluated", labels["Epic specs written"])
	_, hadBars := labels["Acceptance bars valid"]
	require.False(t, hadBars, "an epic has no acceptance bar of its own to validate")
}

func TestDecomposeReadoutRendersRealEpicSpecCountsWhenSupplied(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	in := views.ReadoutInput{EpicSpecs: map[string]views.EpicSpecStatus{
		"E-aaaaa0": {ProseWritten: true},
		// E-aaaaa1 carries no entry: a real, honest zero, not "not evaluated".
	}}
	r := views.ReadoutOfInput(tree, views.MomentDecompose, in)
	labels := checkLabels(r)
	require.Equal(t, "1 of 2", labels["Epic specs written"])
}

// TestFrontierSkipsTerminalFeatures pins the behavior fix round 1, finding
// 1 was about, restated for the one-arm frontier: the walk is first-match-wins,
// so an arm that matched a terminal feature would capture the route from every
// live feature behind it and the agent would be sent at work that can never
// happen. Both terminal states are pinned, ahead of the draft feature in walk
// order, so a widened arm (anything looser than "is draft") fails here.
func TestFrontierSkipsTerminalFeatures(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	// Both terminal features must sit AHEAD of the draft one in the frontier
	// walk, since that is what lets an unfiltered walk stop on one of them.
	// The draft feature RECORDS a dependency on each, so the ordering this
	// test rests on survives any change to how ties break: key order alone
	// would put them ahead here too, and that coincidence is exactly what
	// would leave this test passing while guarding nothing.
	cancelled := feat(t, "F-aaaaa0", "epic-0/aaa-cancelled", "epic-0")
	cancelled.Status = "cancelled"
	tree.Features[cancelled.ID] = cancelled

	done := feat(t, "F-aaaaa1", "epic-0/bbb-done", "epic-0")
	done.Status = "done"
	tree.Features[done.ID] = done

	draft := feat(t, "F-aaaaa2", "epic-0/zzz-draft", "epic-0")
	draft.Status = "draft"
	draft.DependsOn = []string{cancelled.ID, done.ID}
	tree.Features[draft.ID] = draft

	// The premise itself, asserted rather than assumed: the two terminal
	// features really do lead the draft one in the order the walk consumes.
	require.Equal(t, []string{"F-aaaaa0", "F-aaaaa1", "F-aaaaa2"},
		gen.DependencyOrder(tree).Features("epic-0"),
		"the walk must actually meet the terminal features first for the filter to be load-bearing")

	require.Equal(t, []string{"F-aaaaa2"}, frontierIDs(tree),
		"the terminal features are skipped; the frontier is the live draft feature behind them")

	// And through the readout, the surface the agent actually reads.
	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	require.Equal(t, []string{"specify"}, nextKinds(r))
	require.Equal(t, []string{"F-aaaaa2"}, nextNodeIDs(r), "the cancelled and done features are never moves")
}

// TestFrontierSkipsARetiredAncestor extends the same skip-not-stop rule
// to a draft feature that is not itself terminal but sits under a cancelled
// epic: `tiller spec feature` would refuse it with ParentEpicRetiredError
// (transition.go's spec case, issue #36), so routing an agent there would
// send it at a command that fails when copied, the same failure mode
// queueApproveFeatures and queueApproveTaskBatches now suppress on the queue
// side (queue.go, issue #37). epic-0 is cancelled and holds a draft feature
// that would otherwise lead the walk; epic-1 is live and holds the draft
// feature the frontier should actually name.
func TestFrontierSkipsARetiredAncestor(t *testing.T) {
	tree := treeWithDraftEpics(t, 2) // E-aaaaa0 (epic-0), E-aaaaa1 (epic-1)
	tree.Epics["E-aaaaa0"].Status = "cancelled"
	tree.Epics["E-aaaaa1"].Status = "ready"

	held := feat(t, "F-aaaaa0", "epic-0/held", "epic-0")
	held.Status = "draft"
	tree.Features[held.ID] = held

	live := feat(t, "F-aaaaa1", "epic-1/live", "epic-1")
	live.Status = "draft"
	tree.Features[live.ID] = live

	require.Equal(t, []string{"F-aaaaa1"}, frontierIDs(tree), "the held feature is skipped for the live one behind it")

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	require.Equal(t, []string{"F-aaaaa1"}, nextNodeIDs(r), "a feature held by a retired ancestor is never a move")
}

// TestOrientReadoutNamesTheHumanWaitWhenOnlyAnInReviewTaskRemains pins
// task-3-report.md finding 2: orient/SKILL.md's clean-wait bullet, "a node
// sits cleanly in in_review only waiting on a merge, that is a human wait,
// not a seam". The feature here is past draft (so the specify frontier does
// not claim the route first), and its one task is
// in_review with nothing else ready or in progress anywhere in the tree, so
// the only move is the human's merge of that task, never an agent seam.
func TestOrientReadoutNamesTheHumanWaitWhenOnlyAnInReviewTaskRemains(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	tree.Features["F-aaaaa0"] = f

	tree.Tasks = map[string]*store.Task{}
	tree.FeatureTasks = map[string][]*store.Task{}
	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: "F-aaaaa0"}
	tree.Tasks[task.ID] = task
	tree.FeatureTasks["F-aaaaa0"] = []*store.Task{task}

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})

	top := topNext(t, r)
	require.Equal(t, "merge_task", top.Kind)
	require.Equal(t, "T-aaaaa0", top.Node.ID)
	require.Equal(t, "human", top.Actor, "an in_review-only task is a human wait, not an agent seam")
	require.Equal(t, "in review, waiting on merge", top.Reason, "no reader, so no one's merge in particular")
}

func TestSpecifyReadoutListsEveryPlanAwaitingApproval(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	for i, id := range []string{"F-bat001", "F-bat002", "F-bat003"} {
		key := fmt.Sprintf("epic-0/batch-%d", i+1)
		f := feat(t, id, key, "epic-0")
		f.Status = "specced"
		tree.Features[id] = f
	}

	r := views.ReadoutOfInput(tree, views.MomentSpecify, views.ReadoutInput{Contracts: map[string]*store.EvalFile{}})
	// Every plan gate ranks ahead of the still-draft epic's own approval.
	require.Equal(t, []string{"F-bat001", "F-bat002", "F-bat003", "E-aaaaa0"}, nextNodeIDs(r))
	for _, c := range r.Next.Candidates[:3] {
		require.Equal(t, "approve_feature", c.Kind)
		require.Equal(t, "human", c.Actor)
	}
}

func TestSpecifyReadoutKeepsSinglePlanNext(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	f := feat(t, "F-solo01", "epic-0/solo", "epic-0")
	f.Status = "specced"
	tree.Features[f.ID] = f

	r := views.ReadoutOfInput(tree, views.MomentSpecify, views.ReadoutInput{Contributor: "jc", Contracts: map[string]*store.EvalFile{}})
	top := topNext(t, r)
	require.Equal(t, "approve_feature", top.Kind)
	require.Equal(t, "F-solo01", top.Node.ID)
	require.Equal(t, "its feature plan awaits your approval", top.Reason)
}

// Next is one list whatever the moment: orient and specify end on the same
// ranked moves.
func TestNextIsTheSameOnEveryMoment(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	for _, feature := range []*store.Feature{
		feat(t, "F-bat001", "epic-0/alpha", "epic-0"),
		feat(t, "F-bat002", "epic-0/bravo", "epic-0"),
	} {
		feature.Status = "specced"
		tree.Features[feature.ID] = feature
	}

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	specify := views.ReadoutOfInput(tree, views.MomentSpecify, views.ReadoutInput{})
	require.Equal(t, specify.Next, r.Next)
	require.Equal(t, "F-bat001", topNext(t, r).Node.ID)
}

func TestSpecifyReadoutKeepsAmendmentAheadOfSeveralPlans(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	for _, feature := range []*store.Feature{
		feat(t, "F-bat001", "epic-0/alpha", "epic-0"),
		feat(t, "F-bat002", "epic-0/bravo", "epic-0"),
	} {
		feature.Status = "specced"
		tree.Features[feature.ID] = feature
	}
	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("iphone-only"), Status: store.AmendmentStatusProposed},
	}

	r := views.ReadoutOfInput(tree, views.MomentSpecify, views.ReadoutInput{Contracts: map[string]*store.EvalFile{}})
	top := topNext(t, r)
	require.Equal(t, "approve_amendment", top.Kind)
	require.Equal(t, "A-aaaaaa", top.Node.ID)
}

// TestSpecifyReadoutReportsContractsValidAndCriteriaBoundByType pins specify's
// two distinctive checks against a batch of four
// specced features: one whose contract validates with a mix of binding
// types, one whose contract is malformed (no statement), one whose contract
// was never read at all (in.Contracts carries no entry), and one whose
// contract validates with a single manual criterion. The entity cap fires in
// the same fixture, so this test also pins that a large specced batch never
// lists every row.
func TestSpecifyReadoutReportsContractsValidAndCriteriaBoundByType(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	addSpecced := func(id, keySuffix string) {
		f := feat(t, id, "epic-0/"+keySuffix, "epic-0")
		f.Status = "specced"
		tree.Features[id] = f
	}
	addSpecced("F-aaaaa0", "a")
	addSpecced("F-aaaaa1", "b")
	addSpecced("F-aaaaa2", "c")
	addSpecced("F-aaaaa3", "d")

	in := views.ReadoutInput{Contracts: map[string]*store.EvalFile{
		"F-aaaaa0": {Criteria: []store.EvalCriterion{
			{ID: "C1", Statement: "the form validates", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/a_spec.rb"}}},
			{ID: "C2", Statement: "the pdf renders", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/b_spec.rb"}}},
			{ID: "C3", Statement: "the build stays green", Binding: store.EvalBinding{Type: store.BindingCommand, Run: "make check"}},
		}},
		"F-aaaaa1": {Criteria: []store.EvalCriterion{
			{ID: "C1", Statement: "", Binding: store.EvalBinding{Type: store.BindingManual}}, // no statement: fails ValidateContract
		}},
		// F-aaaaa2 carries no entry at all: its contract was never read.
		"F-aaaaa3": {Criteria: []store.EvalCriterion{
			{ID: "C1", Statement: "a reviewer confirms the layout", Binding: store.EvalBinding{Type: store.BindingManual}},
		}},
	}}

	r := views.ReadoutOfInput(tree, views.MomentSpecify, in)

	require.Len(t, r.Entities, 3, "capped so a large specced batch never lists every row")
	require.Equal(t, 1, r.Withheld)

	labels := checkLabels(r)
	require.Equal(t, "2 of 4", labels["Contracts valid"], "an invalid contract and an unread contract both fail the count")
	require.Equal(t, "2 test, 1 command, 1 manual", labels["Criteria bound by type"])
}

// TestSpecifyReadoutMarksContractChecksNotEvaluatedWhenContractsIsNil and
// TestSpecifyReadoutRendersARealZeroWhenContractsIsEmptyButNotNil pin
// task-3-report.md finding 1: a caller that never populated Contracts must
// read differently from a caller that populated it and genuinely found
// nothing. The reviewer's reproduction was exactly the nil, unwired-caller
// case rendering "0 of 1" as if it were a real answer.
func TestSpecifyReadoutMarksContractChecksNotEvaluatedWhenContractsIsNil(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	f := feat(t, "F-aaaaa0", "epic-0/a", "epic-0")
	f.Status = "specced"
	tree.Features["F-aaaaa0"] = f

	r := views.ReadoutOfInput(tree, views.MomentSpecify, views.ReadoutInput{}) // Contracts is nil

	labels := checkLabels(r)
	require.Equal(t, "not evaluated", labels["Contracts valid"], "an unwired caller must not render a false 0 of 1")
	require.Equal(t, "not evaluated", labels["Criteria bound by type"])
}

func TestSpecifyReadoutRendersARealZeroWhenContractsIsEmptyButNotNil(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	f := feat(t, "F-aaaaa0", "epic-0/a", "epic-0")
	f.Status = "specced"
	tree.Features["F-aaaaa0"] = f

	// The caller ran the check and genuinely read no contract for this
	// feature: a real, non-nil, empty map, distinct from never checking.
	in := views.ReadoutInput{Contracts: map[string]*store.EvalFile{}}
	r := views.ReadoutOfInput(tree, views.MomentSpecify, in)

	labels := checkLabels(r)
	require.Equal(t, "0 of 1", labels["Contracts valid"], "a real, checked zero renders as a real zero, not not evaluated")
	require.Equal(t, "none recorded", labels["Criteria bound by type"])
}

// chainedProposedTasksTree lays one ready feature carrying three proposed
// tasks whose recorded depends_on chain runs bravo, alfa, charlie. That
// sequence is none of the three a surface could fall back to: not their key
// order (alfa, bravo, charlie), not their id order (T-aaaaa0, T-aaaaa1,
// T-aaaaa2), and not the order the group is held in, which the fixture sets to
// key order the way os.ReadDir hands the loader task folders. The feature is
// ready, past its own plan gate, which is the state where a proposed task is
// the human's own decision and so reaches the approve_tasks queue kind.
func chainedProposedTasksTree(t *testing.T) *store.Tree {
	t.Helper()
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	f := feat(t, "F-aaaaa0", "epic-0/x", "epic-0")
	f.Status = "ready"
	tree.Features["F-aaaaa0"] = f

	tree.Tasks = map[string]*store.Task{}
	tree.FeatureTasks = map[string][]*store.Task{}
	add := func(id, localKey string, dependsOn ...string) {
		key := featureKeyOf(t, tree, "F-aaaaa0") + "/" + localKey
		task := &store.Task{ID: id, Key: &key, Status: "proposed", Title: localKey, FeatureID: "F-aaaaa0", DependsOn: dependsOn}
		tree.Tasks[id] = task
		tree.FeatureTasks["F-aaaaa0"] = append(tree.FeatureTasks["F-aaaaa0"], task)
	}
	add("T-aaaaa2", "alfa", "T-aaaaa1")
	add("T-aaaaa1", "bravo")
	add("T-aaaaa0", "charlie", "T-aaaaa2")
	return tree
}

// TestExecuteReadoutNamesTasksInRankedOrder preserves the produced-list order
// contract independently of Needs attention, which no longer exposes concrete
// tasks. The chain makes dependency order differ from key, id, and storage
// order, so falling back to any of them fails.
//
// execute is the surviving seam whose produced list runs through orderedTasks,
// so the pin moves here. The chain runs bravo, alfa, charlie: none of key order
// (alfa, bravo, charlie), id order (T-aaaaa0, T-aaaaa1, T-aaaaa2), or the order
// the group is held in, so a surface falling back to any of the three fails.
func TestExecuteReadoutNamesTasksInRankedOrder(t *testing.T) {
	tree := chainedProposedTasksTree(t)
	pr := 41
	for _, tk := range tree.Tasks {
		tk.Status = "in_review"
		tk.PR = &pr
	}

	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})

	refs := make([]string, 0, len(r.Entities))
	for _, e := range r.Entities {
		refs = append(refs, e.Ref)
	}
	require.Equal(t, []string{
		"epic-0/x/bravo (T-aaaaa1)",
		"epic-0/x/alfa (T-aaaaa2)",
		"epic-0/x/charlie (T-aaaaa0)",
	}, refs, "the in-review batch reads in dependency order")
}

// TestExecuteReadoutReportsGatesGreenWithEvidenceAndReviewRef pins execute's
// two distinctive checks against four in_review
// tasks: two carry a required gate that is pass and backed by evidence, one
// carries a required gate that is pass with no evidence at all (the hollow
// pass the check exists to catch), and one carries a required gate that is
// red. Review surfaces are split between the PR path and the git-only
// ReviewRef fallback so both count. The entity cap fires in the same
// fixture.
func TestExecuteReadoutReportsGatesGreenWithEvidenceAndReviewRef(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	f := feat(t, "F-aaaaa0", "epic-0/x", "epic-0")
	f.Status = "active"
	tree.Features["F-aaaaa0"] = f

	pr := 41
	greenGate := map[string]store.Gate{
		"tests": {Required: true, Status: store.GateStatusPass,
			Evidence: []store.Evidence{{Kind: "command_output", Ref: "artifacts/tiller/gates/tests.json"}}},
	}
	hollowGate := map[string]store.Gate{
		"tests": {Required: true, Status: store.GateStatusPass}, // pass, no evidence recorded
	}
	failGate := map[string]store.Gate{
		"tests": {Required: true, Status: store.GateStatusFail},
	}

	tree.Tasks = map[string]*store.Task{}
	tree.FeatureTasks = map[string][]*store.Task{}
	addInReview := func(id string, gates map[string]store.Gate, pr *int, ref *store.ReviewRef) {
		task := &store.Task{
			ID: id, Status: "in_review", FeatureID: "F-aaaaa0", PR: pr, ReviewRef: ref,
			GatesBlock: store.GatesBlock{Gates: gates},
		}
		tree.Tasks[id] = task
		tree.FeatureTasks["F-aaaaa0"] = append(tree.FeatureTasks["F-aaaaa0"], task)
	}
	addInReview("T-aaaaa0", greenGate, &pr, nil)
	addInReview("T-aaaaa1", hollowGate, nil, &store.ReviewRef{Branch: "tiller/epic-0/x"})
	addInReview("T-aaaaa2", failGate, nil, nil)
	addInReview("T-aaaaa3", greenGate, &pr, nil)

	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})

	require.Len(t, r.Entities, 3, "capped so a large in-review batch never lists every row")
	require.Equal(t, 1, r.Withheld)

	labels := checkLabels(r)
	require.Equal(t, "2 of 4", labels["Gates green with evidence"], "a pass with no recorded evidence must not count as green")
	require.Equal(t, "3 of 4", labels["Current submissions recorded"])
}

// TestExecuteReadoutDoesNotCountATaskWithNoRequiredGatesAsGreen pins
// task-3-report.md finding 4: a task carrying only an optional, non-required
// gate (pass, with evidence) has no required gate to call green, so it must
// not pass vacuously the way an empty required set otherwise would.
func TestExecuteReadoutDoesNotCountATaskWithNoRequiredGatesAsGreen(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	f := feat(t, "F-aaaaa0", "epic-0/x", "epic-0")
	f.Status = "active"
	tree.Features["F-aaaaa0"] = f

	optionalOnly := map[string]store.Gate{
		"code_review": {Required: false, Status: store.GateStatusPass,
			Evidence: []store.Evidence{{Kind: "attestation", Ref: "AT-1"}}},
	}
	tree.Tasks = map[string]*store.Task{}
	tree.FeatureTasks = map[string][]*store.Task{}
	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: "F-aaaaa0", GatesBlock: store.GatesBlock{Gates: optionalOnly}}
	tree.Tasks[task.ID] = task
	tree.FeatureTasks["F-aaaaa0"] = []*store.Task{task}

	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})

	labels := checkLabels(r)
	require.Equal(t, "0 of 1", labels["Gates green with evidence"], "a task with no required gates has nothing to call green")
}

// TestExecuteReadoutCarriesUnpushedReviewOntoItsEntity pins the review-state
// warning's data-layer half (issue #42): a caller-computed UnpushedReview
// entry, matched to an in-review task by id, pairs with the branch the tree
// already carries on the task itself (a pure read, no I/O
// views is not allowed to perform) and lands on that task's own entity, never
// a sibling's. The command layer (internal/cli) renders the actual wording;
// this only proves the fact reaches the right entity with the right values.
func TestExecuteReadoutCarriesUnpushedReviewOntoItsEntity(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	f := feat(t, "F-aaaaa0", "epic-0/x", "epic-0")
	f.Status = "active"
	f.Branch = "tiller/epic-0/x"
	tree.Features["F-aaaaa0"] = f

	pr := 41
	tree.Tasks = map[string]*store.Task{}
	tree.FeatureTasks = map[string][]*store.Task{}
	warned := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: "F-aaaaa0", PR: &pr, Branch: "tiller/epic-0/x/warned"}
	clean := &store.Task{ID: "T-aaaaa1", Status: "in_review", FeatureID: "F-aaaaa0", PR: &pr, Branch: "tiller/epic-0/x/clean"}
	tree.Tasks[warned.ID] = warned
	tree.Tasks[clean.ID] = clean
	tree.FeatureTasks["F-aaaaa0"] = []*store.Task{warned, clean}

	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{
		UnpushedReviews: []views.UnpushedReview{
			{Node: "T-aaaaa0", Commit: "abc1234", Upstream: "origin/tiller/epic-0/x/warned"},
		},
	})

	require.Len(t, r.Entities, 2)
	byRef := map[string]views.ReadoutEntity{}
	for _, e := range r.Entities {
		byRef[e.Ref] = e
	}

	warnedEntity, ok := byRef[gen.Ref(tree, warned)]
	require.True(t, ok)
	require.NotNil(t, warnedEntity.Unpushed, "the matched task carries the warning's facts")
	require.Equal(t, "abc1234", warnedEntity.Unpushed.Commit)
	require.Equal(t, "tiller/epic-0/x/warned", warnedEntity.Unpushed.Branch, "the branch comes from the task, not its feature")
	require.Equal(t, "origin/tiller/epic-0/x/warned", warnedEntity.Unpushed.Upstream)

	cleanEntity, ok := byRef[gen.Ref(tree, clean)]
	require.True(t, ok)
	require.Nil(t, cleanEntity.Unpushed, "an unmatched sibling task carries no warning")
}

// TestExecuteReadoutOmitsUnpushedReviewWithoutAnEntry proves the additive
// constraint at the data layer: with no UnpushedReviews at all (every other
// existing test in this file, unedited), an in-review entity's Unpushed field
// stays nil and the rest of the entity is exactly what it was before this
// task.
func TestExecuteReadoutOmitsUnpushedReviewWithoutAnEntry(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	f := feat(t, "F-aaaaa0", "epic-0/x", "epic-0")
	f.Status = "active"
	f.Branch = "tiller/epic-0/x"
	tree.Features["F-aaaaa0"] = f

	pr := 41
	tree.Tasks = map[string]*store.Task{}
	tree.FeatureTasks = map[string][]*store.Task{}
	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: "F-aaaaa0", PR: &pr}
	tree.Tasks[task.ID] = task
	tree.FeatureTasks["F-aaaaa0"] = []*store.Task{task}

	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})

	require.Len(t, r.Entities, 1)
	require.Nil(t, r.Entities[0].Unpushed)
}

// --- amend's own content ---------------------------------

// TestAmendReadoutListsProposedAmendmentsWithOpCounts pins amendContent's
// Produced list and its "Operations staged" check: every proposed amendment
// named with how many operations it has staged, id order (the withdrawn
// amendment must not appear at all), and the tree-wide total.
func TestAmendReadoutListsProposedAmendmentsWithOpCounts(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.Amendments = map[string]*store.Amendment{
		"A-bbbbbb": {
			ID: "A-bbbbbb", Key: strPtr("later-reshape"), Title: "Later reshape",
			Status:     store.AmendmentStatusProposed,
			Operations: []store.AmendmentOp{{Op: store.AmendmentOpRevise, Target: "F-aaaaaa"}},
		},
		"A-aaaaaa": {
			ID: "A-aaaaaa", Key: strPtr("iphone-only"), Title: "Remove iPad",
			Status: store.AmendmentStatusProposed,
			Operations: []store.AmendmentOp{
				{Op: store.AmendmentOpAddTask, Key: "form-engine/loading/iphone-acceptance", Title: "iPhone acceptance"},
				{Op: store.AmendmentOpCancel, Target: "T-aaaaaa", Reason: "superseded"},
			},
		},
		"A-cccccc": {ID: "A-cccccc", Key: strPtr("dropped-one"), Status: store.AmendmentStatusWithdrawn},
	}

	r := views.ReadoutOfInput(tree, views.MomentAmend, views.ReadoutInput{})

	require.Equal(t, views.MomentAmend, r.Moment)
	require.Len(t, r.Entities, 2, "the withdrawn amendment is not proposed and does not appear")
	require.Equal(t, "iphone-only (A-aaaaaa)", r.Entities[0].Ref)
	require.Equal(t, "2 operations staged", r.Entities[0].Note)
	require.Equal(t, "later-reshape (A-bbbbbb)", r.Entities[1].Ref)
	require.Equal(t, "1 operation staged", r.Entities[1].Note)

	labels := checkLabels(r)
	require.Equal(t, "3", labels["Operations staged"], "the tree-wide total across every proposed amendment")
	require.Equal(t, "not evaluated", labels["Preconditions"], "no caller ever fed a live read in this test")
}

// TestAmendReadoutCapsItsEntityListAndSaysHowMany pins the same entityCap
// discipline decomposeContent and specifyContent already prove: a large tree
// never lists every proposed amendment.
func TestAmendReadoutCapsItsEntityListAndSaysHowMany(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.Amendments = map[string]*store.Amendment{}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("A-aaaaa%d", i)
		key := fmt.Sprintf("amend-%d", i)
		tree.Amendments[id] = &store.Amendment{ID: id, Key: &key, Status: store.AmendmentStatusProposed}
	}

	r := views.ReadoutOfInput(tree, views.MomentAmend, views.ReadoutInput{})

	require.Len(t, r.Entities, 3)
	require.Equal(t, 2, r.Withheld)
}

// TestAmendReadoutIsHonestWithNoProposedAmendments pins the zero case: no
// Produced section (empty Entities, zero Withheld), no NeedsYou action, and the
// terminal Next line still closes the block, the same honest-empty rendering
// every other moment's content function leaves when it finds nothing to name.
func TestAmendReadoutIsHonestWithNoProposedAmendments(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)

	r := views.ReadoutOfInput(tree, views.MomentAmend, views.ReadoutInput{})

	require.Empty(t, r.Entities)
	require.Zero(t, r.Withheld)
	require.Empty(t, r.NeedsYou, "nothing proposed, nothing queued")
	labels := checkLabels(r)
	require.Equal(t, "0", labels["Operations staged"])
	require.Equal(t, "not evaluated", labels["Preconditions"])
	require.True(t, r.Next.Unplanned, "an empty tree closes on how work enters")
	require.NotEmpty(t, r.Next.Hint)
}

// TestAmendReadoutPreconditionsReflectsTheCallersLiveRead pins the
// caller-fed discipline (ReadoutInput.AmendmentPreconditions's own doc
// comment, matching Contracts and EpicSpecs): nil renders "not evaluated"
// (proven above), and a real map renders the real valid/stale tally across
// every proposed amendment, an unknown-status precondition counting toward
// neither.
func TestAmendReadoutPreconditionsReflectsTheCallersLiveRead(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("iphone-only"), Status: store.AmendmentStatusProposed},
	}
	in := views.ReadoutInput{AmendmentPreconditions: map[string][]mutate.PreconditionValidity{
		"A-aaaaaa": {
			{Index: 0, Status: mutate.PreconditionValid},
			{Index: 1, Status: mutate.PreconditionStale},
			{Index: 1, Status: mutate.PreconditionStale},
			{Index: 2, Status: mutate.PreconditionUnknown},
		},
	}}

	r := views.ReadoutOfInput(tree, views.MomentAmend, in)

	labels := checkLabels(r)
	require.Equal(t, "1 valid, 2 stale", labels["Preconditions"], "the unknown entry counts toward neither")
}

// TestAmendReadoutNeedsAttentionUsesTheQueueSeam confirms amend's own
// attention summary rides the same queue-derived path every other moment uses,
// while the concrete amendment stays behind the enumeration command.
func TestAmendReadoutNeedsAttentionUsesTheQueueSeam(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("iphone-only"), Title: "Remove iPad", Status: store.AmendmentStatusProposed},
	}

	r := views.ReadoutOfInput(tree, views.MomentAmend, views.ReadoutInput{})

	require.Equal(t, []views.ReadoutAction{{
		Text: "1 proposed amendment awaits your approval", Command: "tiller queue --kind approve_amendment",
	}}, r.NeedsYou)
	require.NotContains(t, r.NeedsYou[0].Text, "iphone-only")
	require.NotContains(t, r.NeedsYou[0].Command, "tiller approve")
}

// The six tests below pin that each gate's Verification answers something
// about the act, never a tally of the tree that would merely restate the
// independent Needs attention count below it.
func TestApproveContentProvesTheActNotTheQueue(t *testing.T) {
	tree := treeWithDraftEpics(t, 2) // E-aaaaa0 (epic-0), E-aaaaa1 (epic-1)
	// The gate's own store.Load reload (renderReadoutFor) would see epic-0
	// already ready by the time this runs; epic-1 was never touched.
	tree.Epics["E-aaaaa0"].Status = "ready"

	in := views.ReadoutInput{Act: &views.GateAct{
		Nodes: []views.GateActNode{{Noun: store.NounEpic, ID: "E-aaaaa0", Changed: true}},
	}}
	r := views.ReadoutOfInput(tree, views.MomentApprove, in)

	labels := checkLabels(r)
	require.Equal(t, "1 of 1", labels["Moved to ready"])
	require.Equal(t, "cancel", labels["Now legal on the node(s) just approved"],
		"a ready epic's own legal moves (lifecycle.NextMoves), not a count of what else is pending; "+
			"done is not among them, because it arrives by rollup rather than by verb")
	_, hadQueueTally := labels["Epics awaiting approval"]
	require.False(t, hadQueueTally, "round-1's queue-tally check must not come back")

	// Needs attention still counts the other draft epic, but keeps its concrete
	// identity behind the queue enumeration seam.
	require.Equal(t, []views.ReadoutAction{{
		Text: "1 epic awaits your approval", Command: "tiller queue --kind approve_epic",
	}}, r.NeedsYou)
	require.NotContains(t, r.NeedsYou[0].Command, "epic-1")
}

// TestApproveContentCountsAlreadyReadyNodesSeparately confirms a batch of
// mixed outcomes (one real move, one idempotent no-op) reports both, the
// "what transitioned, what was already in the target state" half of the
// design table's approve row.
func TestApproveContentCountsAlreadyReadyNodesSeparately(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"

	in := views.ReadoutInput{Act: &views.GateAct{Nodes: []views.GateActNode{
		{Noun: store.NounEpic, ID: "E-aaaaa0", Changed: true},
		{Noun: store.NounEpic, ID: "E-aaaaa1", Changed: false},
	}}}
	r := views.ReadoutOfInput(tree, views.MomentApprove, in)

	labels := checkLabels(r)
	require.Equal(t, "1 of 2", labels["Moved to ready"])
	require.Equal(t, "1", labels["Already ready (no change)"])
}

// TestApproveContentReportsAmendmentAppliedOpsByKind pins the deferred item
// this task closes: `approve amendment` used to pass act:nil, so its own
// readout carried no Verification section at all. With a real
// AmendmentApplied act it reports what the batch applied, tallied by op
// kind in mutate's own fixed order, beside the plain "Approval recorded"
// headline (not wrong for this act, only generic, so it stays unlike
// BasisRefresh's and AssumptionReviewed's own overrides).
func TestApproveContentReportsAmendmentAppliedOpsByKind(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	in := views.ReadoutInput{Act: &views.GateAct{AmendmentApplied: &views.GateActAmendmentApplied{
		Applied: []views.GateActAmendmentOp{
			{Op: store.AmendmentOpAddTask},
			{Op: store.AmendmentOpCancel},
			{Op: store.AmendmentOpCancel},
		},
	}}}
	r := views.ReadoutOfInput(tree, views.MomentApprove, in)

	require.Equal(t, "Approval recorded", r.Headline)
	labels := checkLabels(r)
	require.Equal(t, "3", labels["Operations applied"])
	require.Equal(t, "2 cancel, 1 add-task", labels["By kind"], "mutate's own fixed op order, not staged or arrival order")
}

// TestAmendmentOpBreakdownIncludesAddEpicAndAddQuestion pins task-3's
// inherited assignment from task-2's review: amendmentOpBreakdown's fixed
// order list used to name only revise, supersede, cancel, add-feature, and
// add-task, silently dropping add-epic and add-question from the "By kind"
// tally even though they appeared, correctly, in "Operations applied" and in
// humanApproveAmendment's own per-operation line. Both now have to show up
// here, in the same fixed order every other entry follows.
func TestAmendmentOpBreakdownIncludesAddEpicAndAddQuestion(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	in := views.ReadoutInput{Act: &views.GateAct{AmendmentApplied: &views.GateActAmendmentApplied{
		Applied: []views.GateActAmendmentOp{
			{Op: store.AmendmentOpAddQuestion},
			{Op: store.AmendmentOpAddEpic},
			{Op: store.AmendmentOpRevise},
		},
	}}}
	r := views.ReadoutOfInput(tree, views.MomentApprove, in)

	labels := checkLabels(r)
	require.Equal(t, "3", labels["Operations applied"])
	require.Equal(t, "1 revise, 1 add-epic, 1 add-question", labels["By kind"],
		"add-epic and add-question no longer silently drop from the tally")
}

// TestAmendmentOpBreakdownIncludesCorrectEval keeps an approved eval
// correction in the "By kind" tally, last in the store's fixed kind order.
func TestAmendmentOpBreakdownIncludesCorrectEval(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	in := views.ReadoutInput{Act: &views.GateAct{AmendmentApplied: &views.GateActAmendmentApplied{
		Applied: []views.GateActAmendmentOp{
			{Op: store.AmendmentOpCorrectEval},
			{Op: store.AmendmentOpCancel},
		},
	}}}
	r := views.ReadoutOfInput(tree, views.MomentApprove, in)

	labels := checkLabels(r)
	require.Equal(t, "2", labels["Operations applied"])
	require.Equal(t, "1 cancel, 1 correct-eval", labels["By kind"])
}

// TestNoAcceptMomentRemains pins the gate's removal from the readout side: the
// four human gates are approve, answer, attest, and sync (readout_gate.go's
// map), and "accept" is not a moment at all. An unknown moment falls back to
// its own name with no content function behind it, which is what this asserts
// rather than a rendering of the old block.
func TestNoAcceptMomentRemains(t *testing.T) {
	key := "checkout"
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Epics:    map[string]*store.Epic{"E-checkout": {ID: "E-checkout", Key: &key, Status: "done"}},
		Features: map[string]*store.Feature{
			"F-cart": {ID: "F-cart", EpicKey: key, Status: "done"},
		},
	}
	in := views.ReadoutInput{
		Act: &views.GateAct{Nodes: []views.GateActNode{{Noun: store.NounEpic, ID: "E-checkout", Changed: true}}},
	}
	r := views.ReadoutOfInput(tree, views.Moment("accept"), in)

	require.Equal(t, "accept", r.Headline, "no headline prose: accept is not a moment")
	require.Empty(t, checkLabels(r), "no content function answers for it either")
}

// TestAnswerContentReportsWhatItUnblocked confirms the one check no gate
// content function had before this fix: whether the nodes this specific
// question blocked are now actually clear, not just a count of what else is
// still open.
func TestAnswerContentReportsWhatItUnblocked(t *testing.T) {
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Features: map[string]*store.Feature{
			"F-a": {ID: "F-a", Status: "ready"},
			"F-b": {ID: "F-b", Status: "ready"},
		},
		Questions: map[string]*store.Question{
			"Q-answered": {ID: "Q-answered", Statement: "answered one", Status: "answered", Blocks: []string{"F-a", "F-b"}},
			"Q-open":     {ID: "Q-open", Statement: "still open", Status: "open", Blocks: []string{"F-b"}},
		},
	}
	in := views.ReadoutInput{Act: &views.GateAct{
		Answered: &views.GateActAnswer{Blocks: []string{"F-a", "F-b"}},
	}}
	r := views.ReadoutOfInput(tree, views.MomentAnswer, in)

	labels := checkLabels(r)
	require.Equal(t, "1 of 2", labels["Nodes this answer cleared of every open question"],
		"F-a is now clear; F-b is still held by the other open question")
	require.Equal(t, "1", labels["Open questions remaining"])
}

// TestCancelContentReportsWhatItReleased: a cancel releases holds exactly as
// an answer does, so its readout reports which of the cancelled question's
// nodes are now clear of every open question, and how many remain open.
func TestCancelContentReportsWhatItReleased(t *testing.T) {
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Features: map[string]*store.Feature{
			"F-a": {ID: "F-a", Status: "ready"},
			"F-b": {ID: "F-b", Status: "ready"},
		},
		Questions: map[string]*store.Question{
			"Q-cancelled": {ID: "Q-cancelled", Statement: "moot one", Status: "cancelled", CancelReason: "moot", Blocks: []string{"F-a", "F-b"}},
			"Q-open":      {ID: "Q-open", Statement: "still open", Status: "open", Blocks: []string{"F-b"}},
		},
	}
	in := views.ReadoutInput{Act: &views.GateAct{
		Cancelled: &views.GateActCancel{Blocks: []string{"F-a", "F-b"}},
	}}
	r := views.ReadoutOfInput(tree, views.MomentCancelQuestion, in)

	require.Equal(t, "Question cancelled", r.Headline)
	labels := checkLabels(r)
	require.Equal(t, "1 of 2", labels["Nodes this cancel cleared of every open question"])
	require.Equal(t, "1", labels["Open questions remaining"])

	in.Act.Cancelled.Blocks = nil
	labels = checkLabels(views.ReadoutOfInput(tree, views.MomentCancelQuestion, in))
	require.NotContains(t, labels, "Nodes this cancel cleared of every open question",
		"a question that blocked nothing reports no cleared nodes")
	require.Equal(t, "1", labels["Open questions remaining"])
}

func TestAnswerContentUsesOperativeAncestorQuestionBlockers(t *testing.T) {
	t.Run("an epic question still holds a descendant feature", func(t *testing.T) {
		epicKey := "widgets"
		tree := &store.Tree{
			Manifest: &store.Manifest{},
			Epics: map[string]*store.Epic{
				"E-root": {ID: "E-root", Key: &epicKey, Status: "ready"},
			},
			Features: map[string]*store.Feature{
				"F-child": {ID: "F-child", EpicKey: epicKey, Status: "ready"},
			},
			Questions: map[string]*store.Question{
				"Q-answered": {ID: "Q-answered", Statement: "answered one", Status: "answered", Blocks: []string{"F-child"}},
				"Q-epic":     {ID: "Q-epic", Statement: "still open at the epic", Status: "open", Blocks: []string{"E-root"}},
			},
		}
		in := views.ReadoutInput{Act: &views.GateAct{
			Answered: &views.GateActAnswer{Blocks: []string{"F-child"}},
		}}

		labels := checkLabels(views.ReadoutOfInput(tree, views.MomentAnswer, in))
		require.Equal(t, "0 of 1", labels["Nodes this answer cleared of every open question"],
			"the open epic question remains operative on its descendant feature")
	})

	t.Run("a legacy feature key resolves before checking inherited blockers", func(t *testing.T) {
		epicKey := "widgets"
		featureKey := "widgets/gizmo"
		tree := &store.Tree{
			Manifest: &store.Manifest{},
			Epics: map[string]*store.Epic{
				"E-root": {ID: "E-root", Key: &epicKey, Status: "ready"},
			},
			Features: map[string]*store.Feature{
				"F-child": {ID: "F-child", Key: &featureKey, EpicKey: epicKey, Status: "ready"},
			},
			Questions: map[string]*store.Question{
				"Q-answered": {
					ID: "Q-answered", Statement: "legacy answered one", Status: "answered",
					Blocks: []string{featureKey},
				},
				"Q-epic": {
					ID: "Q-epic", Statement: "still open at the epic", Status: "open",
					Blocks: []string{"E-root"},
				},
			},
		}
		in := views.ReadoutInput{Act: &views.GateAct{
			Answered: &views.GateActAnswer{Blocks: []string{featureKey}},
		}}

		labels := checkLabels(views.ReadoutOfInput(tree, views.MomentAnswer, in))
		require.Equal(t, "0 of 1", labels["Nodes this answer cleared of every open question"],
			"legacy recorded keys resolve to the feature before testing its inherited holds")
	})

	t.Run("a feature assumption does not clear its descendant task", func(t *testing.T) {
		epicKey := "widgets"
		tree := &store.Tree{
			Manifest: &store.Manifest{},
			Epics: map[string]*store.Epic{
				"E-root": {ID: "E-root", Key: &epicKey, Status: "ready"},
			},
			Features: map[string]*store.Feature{
				"F-assumed": {
					ID: "F-assumed", EpicKey: epicKey, Status: "ready",
					AssumptionBasis: store.AssumptionBasis{
						"Q-epic": {Reason: "Use the documented fallback"},
					},
				},
			},
			Tasks: map[string]*store.Task{
				"T-child": {ID: "T-child", FeatureID: "F-assumed", Status: "ready"},
			},
			Questions: map[string]*store.Question{
				"Q-answered": {
					ID: "Q-answered", Statement: "answered one", Status: "answered",
					Blocks: []string{"F-assumed", "T-child"},
				},
				"Q-epic": {
					ID: "Q-epic", Statement: "still open at the epic", Status: "open",
					Blocks: []string{"E-root"},
				},
			},
		}
		in := views.ReadoutInput{Act: &views.GateAct{
			Answered: &views.GateActAnswer{Blocks: []string{"F-assumed", "T-child"}},
		}}

		labels := checkLabels(views.ReadoutOfInput(tree, views.MomentAnswer, in))
		require.Equal(t, "1 of 2", labels["Nodes this answer cleared of every open question"],
			"the assumption clears the feature, but task guards ignore feature assumptions")
	})
}

// TestAttestContentReportsWhatRemainsUnmetOnThisNodeOnly is finding 2's own
// fix, verified: attest's Verification now answers at the node grain the
// design table asks for, excluding the target this call itself just
// satisfied, and never counting another node's unmet gates or criteria.
func TestAttestContentReportsWhatRemainsUnmetOnThisNodeOnly(t *testing.T) {
	gizmoKey, otherKey := "widgets/gizmo", "widgets/other"
	criteria := func(c map[string]store.Criterion) store.GatesBlock {
		return store.GatesBlock{Gates: map[string]store.Gate{"evals": {Criteria: c}}}
	}
	tree := &store.Tree{
		Manifest: &store.Manifest{
			Gates: store.GatesConfig{Optional: map[string]store.GateSpec{
				"code_review": {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
			}},
		},
		Features: map[string]*store.Feature{
			"F-gizmo": {ID: "F-gizmo", Key: &gizmoKey, Status: "active"},
			"F-other": {ID: "F-other", Key: &otherKey, Status: "active"},
		},
		// The review gates hang off a PR, and a PR always represents a task, so the
		// two nodes under attestation are tasks.
		Tasks: map[string]*store.Task{
			"T-gizmo": {ID: "T-gizmo", Status: "in_review", FeatureID: "F-gizmo", GatesBlock: criteria(map[string]store.Criterion{
				"C1": {Status: store.CriterionStatusNeedsAttestation},
				"C2": {Status: store.CriterionStatusPass},
			})},
			"T-other": {ID: "T-other", Status: "in_review", FeatureID: "F-other", GatesBlock: criteria(map[string]store.Criterion{
				"C9": {Status: store.CriterionStatusNeedsAttestation},
			})},
		},
	}
	// T-gizmo's code_review gate is unstored (defaults to pending), so
	// without the exclusion this call's own target would still count itself
	// as unmet; the exclusion is what a fresh tree read alone cannot do.
	in := views.ReadoutInput{Act: &views.GateAct{
		Attested: &views.GateActAttest{NodeID: "T-gizmo", TargetKind: "gate", TargetID: "code_review"},
	}}
	r := views.ReadoutOfInput(tree, views.MomentAttest, in)

	labels := checkLabels(r)
	require.Equal(t, "0", labels["Gates still awaiting attestation on this node"], "the just-attested gate is excluded")
	require.Equal(t, "1", labels["Criteria still awaiting attestation on this node"], "C1 remains; T-other's C9 must not be counted")
}

// TestSyncContentReportsBasisAndWhatStayedInReview confirms syncContent
// reads mutate.Sync's own Crossings and Awaiting verbatim (carried in as
// act.Crossed/act.Awaiting) rather than a bare in_review census that could
// never have named a basis or a stranded reason.
func TestSyncContentReportsBasisAndWhatStayedInReview(t *testing.T) {
	tree := &store.Tree{Manifest: &store.Manifest{}}
	in := views.ReadoutInput{Act: &views.GateAct{
		Crossed: []views.GateActCrossing{
			{Basis: "ancestry"},
			{Basis: "content"},
		},
		Awaiting: []views.GateActAwaiting{
			{NoMergeWitness: true},
		},
	}}
	r := views.ReadoutOfInput(tree, views.MomentSync, in)

	labels := checkLabels(r)
	require.Equal(t, "2", labels["Crossings recorded"])
	require.Equal(t, "1 ancestry, 1 content", labels["Basis"])
	require.Equal(t, "1", labels["Still in review"])
}

// The tests below drive ReadoutOfInput end to end, seams and gates alike,
// because the point being pinned is that every moment's block ends on the
// same ranked list (views.NextOf), never a second routing brain.

// TestApproveReadoutNamesTheFollowingSeamAsNextReusingOrientsOwnRouting is
// the gate-grain half: the identical tree TestOrientReadoutHasNoRouteCheck
// drives (a ready epic and one draft feature), read through MomentApprove
// instead of MomentOrient, ends on the same move.
func TestApproveReadoutNamesTheFollowingSeamAsNextReusingOrientsOwnRouting(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "draft"
	tree.Features["F-aaaaa0"] = f

	in := views.ReadoutInput{Act: &views.GateAct{
		Nodes: []views.GateActNode{{Noun: store.NounEpic, ID: "E-aaaaa0", Changed: true}},
	}}
	r := views.ReadoutOfInput(tree, views.MomentApprove, in)

	top := topNext(t, r)
	require.Equal(t, "specify", top.Kind)
	require.Equal(t, "F-aaaaa0", top.Node.ID, "the move names the node, not just the seam")
	orient := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	require.Equal(t, orient.Next, r.Next)
}

// TestAttestReadoutNamesTheHumanWaitAsNextOnAGate confirms a clean in_review
// wait (nothing ready, in progress, or draft; one task merely waiting on a
// merge) reaches Next named, not called a seam, and as the human's move,
// since only a human merges.
func TestAttestReadoutNamesTheHumanWaitAsNextOnAGate(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "active"
	tree.Features["F-aaaaa0"] = f

	tree.Tasks = map[string]*store.Task{}
	tree.FeatureTasks = map[string][]*store.Task{}
	task := &store.Task{ID: "T-aaaaa0", Status: "in_review", FeatureID: "F-aaaaa0"}
	tree.Tasks[task.ID] = task
	tree.FeatureTasks["F-aaaaa0"] = []*store.Task{task}

	in := views.ReadoutInput{Act: &views.GateAct{
		Attested: &views.GateActAttest{NodeID: "F-aaaaa0", TargetKind: "gate", TargetID: "code_review"},
	}}
	r := views.ReadoutOfInput(tree, views.MomentAttest, in)

	top := topNext(t, r)
	require.Equal(t, "human", top.Actor, "a merge is the human's to make")
	require.Equal(t, "T-aaaaa0", top.Node.ID)
	require.Contains(t, top.Reason, "waiting on merge")
}

// TestSyncReadoutIsCaughtUpWhenNothingIsLegal pins the terminal case: no
// live feature waiting on the frontier and nothing ready, in progress, or in
// review leaves no legal move, and the block still closes on Next as caught
// up (JC's call, 2026-08-05, the block's last section is always Next).
func TestSyncReadoutIsCaughtUpWhenNothingIsLegal(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	r := views.ReadoutOfInput(tree, views.MomentSync, views.ReadoutInput{Act: &views.GateAct{}})
	require.True(t, r.Next.CaughtUp)
	require.Empty(t, r.Next.Candidates)
}

// TestTopClaimIsTheDependencyRankedFirstFeature is the non-empty
// sibling of the terminal above. Both features and their tasks are ready but
// ownerless, so both are startable; the claimable arm (issue #92) names the
// dependency-ranked first one rather than falling through to a caught-up pool
// line, which no longer renders for a startable pool. The dependency
// deliberately inverts feature-key order: alpha depends on zulu, making zulu
// the first pool entry and the arm's pick.
func TestTopClaimIsTheDependencyRankedFirstFeature(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	base := feat(t, "F-zzzzz0", "epic-0/zulu-base", "epic-0")
	base.Status = "ready"
	dependent := feat(t, "F-aaaaa0", "epic-0/alpha-dependent", "epic-0")
	dependent.Status = "ready"
	dependent.DependsOn = []string{base.ID}
	tree.Features[base.ID] = base
	tree.Features[dependent.ID] = dependent

	baseTaskKey := "epic-0/zulu-base/wire-base"
	baseTask := &store.Task{
		ID: "T-zzzzz0", Key: &baseTaskKey, Status: "ready", FeatureID: base.ID,
	}
	dependentTaskKey := "epic-0/alpha-dependent/wire-dependent"
	dependentTask := &store.Task{
		ID: "T-aaaaa0", Key: &dependentTaskKey, Status: "ready", FeatureID: dependent.ID,
	}
	tree.Tasks = map[string]*store.Task{
		baseTask.ID:      baseTask,
		dependentTask.ID: dependentTask,
	}
	tree.FeatureTasks = map[string][]*store.Task{
		base.ID:      {baseTask},
		dependent.ID: {dependentTask},
	}

	// Both features are startable now that the claimable arm exists (issue
	// #92), so this tree is no longer a caught-up terminal: the route names
	// the dependency-ranked first one as an execute seam with its claim
	// command, and the pool line is not rendered.
	next := nextFor(tree, "jc", workflow.Input{})
	require.False(t, next.CaughtUp, "approved, unclaimed work is a move, not pool work")
	top := next.Candidates[0]
	require.Equal(t, "claim_task", top.Kind)
	require.Equal(t, "execute", top.Skill)
	require.Equal(t, baseTask.ID, top.Node.ID, "the dependency-ranked first execution unit")

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{Contributor: "jc"})
	require.Equal(t, []string{baseTask.ID}, nextNodeIDs(r), "the dependent task waits on its dependency, so it is not a move")
	require.Equal(t, top, topNext(t, r), "the readout lists the same top move")
	require.Equal(t, "tiller claim task "+baseTaskKey, top.Command)
}

// TestSpecifyReadoutNamesThePlanGateAsNext pins the plan-gate arm at the
// specify moment. `tiller spec feature` leaves the feature specced and its
// tasks proposed, and the single act that moves them is the human's
// `approve feature` (cli-surface.md, "Specifying a feature"), so that is what
// specify's block ends on.
//
// The tree is the one that separates the two answers: the feature just specced
// AND a later draft feature. Route used to walk past a specced feature and
// land on the draft one, so a Next taken straight off it read "specify:
// epic-0/zebra (F-aaaaa1) is draft" here, telling the agent to open the next
// specification while the plan it just wrote sat unapproved: the Acme
// incident (JC, 2026-08-05). The plan gate ranks ahead of the frontier now.
func TestSpecifyReadoutNamesThePlanGateAsNext(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	specced := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	specced.Status = "specced"
	tree.Features["F-aaaaa0"] = specced

	later := feat(t, "F-aaaaa1", "epic-0/zebra", "epic-0")
	later.Status = "draft"
	tree.Features["F-aaaaa1"] = later

	r := views.ReadoutOfInput(tree, views.MomentSpecify, views.ReadoutInput{})

	top := topNext(t, r)
	require.Equal(t, "human", top.Actor, "the plan gate is the human's")
	require.Equal(t, "approve_feature", top.Kind)
	require.Equal(t, "F-aaaaa0", top.Node.ID, "the feature just specced, not the next draft one")
}

// TestPlanGateOutranksTheFrontierOnEveryMoment is the globalization the
// Acme incident demanded: the identical specced-plus-draft tree read
// through orient and through a gate moment must name the plan gate too, not
// just specify's own readout. Before the plan-gate arm lived in Route, only
// MomentSpecify carried the answer (specifyNext), and every other surface
// told the reader to open the next spec.
func TestPlanGateOutranksTheFrontierOnEveryMoment(t *testing.T) {
	for _, m := range []views.Moment{views.MomentOrient, views.MomentSync} {
		tree := treeWithDraftEpics(t, 1)
		tree.Epics["E-aaaaa0"].Status = "ready"

		specced := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
		specced.Status = "specced"
		tree.Features["F-aaaaa0"] = specced

		later := feat(t, "F-aaaaa1", "epic-0/zebra", "epic-0")
		later.Status = "draft"
		tree.Features["F-aaaaa1"] = later

		in := views.ReadoutInput{}
		if m == views.MomentSync {
			in.Act = &views.GateAct{}
		}
		r := views.ReadoutOfInput(tree, m, in)

		top := topNext(t, r)
		require.Equal(t, "approve_feature", top.Kind, "moment %s", m)
		require.Equal(t, "F-aaaaa0", top.Node.ID, "moment %s: the pending gate, never the next draft", m)
	}
}

// TestTaskInHandOutranksThePlanGate pins the arm order: actionable work the
// reader holds beats the human wait, so an agent mid-feature is never parked
// on someone else's approve.
func TestTaskInHandOutranksThePlanGate(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	specced := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	specced.Status = "specced"
	tree.Features["F-aaaaa0"] = specced

	active := feat(t, "F-aaaaa1", "epic-0/active", "epic-0")
	active.Status = "active"
	tree.Features["F-aaaaa1"] = active
	task := &store.Task{ID: "T-aaaaa0", Status: "ready", Owner: "jc", FeatureID: "F-aaaaa1"}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{"F-aaaaa1": {task}}

	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"continue", "approve_feature"}, candidateKinds(next.Candidates))
	require.Equal(t, "T-aaaaa0", next.Candidates[0].Node.ID)
}

// strPtr is a small helper for a fixture that needs a *string Key field (an
// amendment's or a hand-built task's - both are the identical &foo pattern
// every other fixture in this file inlines for a node's own Key field, so
// one name serves both rather than two identically-bodied twins).
func strPtr(s string) *string { return &s }

// TestAmendmentWaitOutranksThePlanGate pins the amendment wait's own place in
// Route: a proposed amendment sits ahead of the plan gate,
// because a staged reshape can invalidate the very plan a human is about to
// approve (a supersede or a cancel naming the specced feature the plan gate
// would otherwise name next).
func TestAmendmentWaitOutranksThePlanGate(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	specced := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	specced.Status = "specced"
	tree.Features["F-aaaaa0"] = specced

	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("iphone-only"), Status: store.AmendmentStatusProposed},
	}

	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"approve_amendment", "approve_feature"}, candidateKinds(next.Candidates),
		"the amendment wait ranks ahead of the plan gate")
	top := next.Candidates[0]
	require.Equal(t, "human", top.Actor, "a gate is the human's move, the same as the plan gate")
	require.Empty(t, top.Skill)
	require.Equal(t, "iphone-only (A-aaaaaa)", top.Node.Ref)
	require.Equal(t, "its staged changes await your approval", top.Reason)
}

// TestTaskInHandOutranksTheAmendmentWait pins the arm directly above the
// amendment wait: actionable work the reader already holds still beats a
// proposed amendment, the identical rule TestTaskInHandOutranksThePlanGate
// pins one arm further down.
func TestTaskInHandOutranksTheAmendmentWait(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	active := feat(t, "F-aaaaa1", "epic-0/active", "epic-0")
	active.Status = "active"
	tree.Features["F-aaaaa1"] = active
	task := &store.Task{ID: "T-aaaaa0", Status: "ready", Owner: "jc", FeatureID: "F-aaaaa1"}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{"F-aaaaa1": {task}}

	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("iphone-only"), Status: store.AmendmentStatusProposed},
	}

	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"continue", "approve_amendment"}, candidateKinds(next.Candidates))
	require.Equal(t, "T-aaaaa0", next.Candidates[0].Node.ID)
}

// TestAmendmentWaitNamesTheFirstByIDOrder pins the tiebreak among several
// proposed amendments: the first by id order, regardless of which one a key
// would sort first (the fixture's "first-one" key sits on the higher id, so a
// key-sorted pick would get this wrong).
func TestAmendmentWaitNamesTheFirstByIDOrder(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	tree.Amendments = map[string]*store.Amendment{
		"A-bbbbbb": {ID: "A-bbbbbb", Key: strPtr("first-one"), Status: store.AmendmentStatusProposed},
		"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("later-one"), Status: store.AmendmentStatusProposed},
	}

	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"A-aaaaaa", "A-bbbbbb"}, candidateNodeIDs(candidatesOfKind(next.Candidates, "approve_amendment")),
		"A-aaaaaa ranks first by id though its key sorts second")
}

// TestAmendmentWaitIgnoresApprovedAndWithdrawnAmendments pins the status
// filter: only a proposed amendment is a wait, the same terminal-status rule
// every other queue and route derivation in this package applies.
func TestAmendmentWaitIgnoresApprovedAndWithdrawnAmendments(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("approved-one"), Status: store.AmendmentStatusApproved},
		"A-bbbbbb": {ID: "A-bbbbbb", Key: strPtr("withdrawn-one"), Status: store.AmendmentStatusWithdrawn},
	}

	require.Empty(t, candidatesOfKind(nextFor(tree, "jc", workflow.Input{}).Candidates, "approve_amendment"))
}

// TestEpiclessTreeNamesTheAmendmentWaitBeforeDecompose pins issue #137's
// amendment half: an intake amendment staged against an empty tree is a
// human wait the route must surface, not hide behind decompose.
func TestEpiclessTreeNamesTheAmendmentWaitBeforeDecompose(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.ToBeDesign = []string{"roadmap.md"}
	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("first-intake"), Status: store.AmendmentStatusProposed},
	}

	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"approve_amendment", "decompose"}, candidateKinds(next.Candidates))
	require.Equal(t, "first-intake (A-aaaaaa)", next.Candidates[0].Node.Ref)
	require.Equal(t, "its staged changes await your approval", next.Candidates[0].Reason)
}

func TestEpiclessTreeIgnoresSettledAmendmentsAndRoutesDecompose(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.ToBeDesign = []string{"roadmap.md"}
	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("approved-one"), Status: store.AmendmentStatusApproved},
		"A-bbbbbb": {ID: "A-bbbbbb", Key: strPtr("withdrawn-one"), Status: store.AmendmentStatusWithdrawn},
	}

	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"decompose"}, candidateKinds(next.Candidates))
	require.Contains(t, next.Candidates[0].Reason, "no epics recorded yet")
}

// TestSpecifyReadoutSuppressesThePlanGateUnderARetiredAncestor mirrors
// queueApproveFeatures' suppression (queue.go, issue #37) on this Next arm: a
// specced feature under a cancelled epic is not named here, because the
// copied command, `tiller approve feature`, now refuses with
// ParentEpicRetiredError (issue #36), and a Next line recommending a command
// that fails when copied is the same bug the queue row was. The positive
// sibling, the same feature under a live epic, is
// TestSpecifyReadoutNamesThePlanGateAsNext above.
func TestSpecifyReadoutSuppressesThePlanGateUnderARetiredAncestor(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "cancelled"

	specced := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	specced.Status = "specced"
	tree.Features["F-aaaaa0"] = specced

	r := views.ReadoutOfInput(tree, views.MomentSpecify, views.ReadoutInput{})

	require.NotContains(t, nextKinds(r), "approve_feature",
		"a feature held by a retired ancestor epic is never offered as the plan gate")
}

// TestFrontierSkipsADraftWhoseDependenciesAreUnfinished pins the strict
// dependency gate (JC's call, 2026-08-05): a draft feature is suggested only
// once every recorded depends_on target is done (or cancelled), and the walk
// runs the feature-grain flow order, so the qualified draft wins even when
// the gated one's epic sorts first: the Acme shape, where the frontier
// named audit-engine/selection-engine while its dependencies sat draft and
// active in later-keyed epics.
func TestFrontierSkipsADraftWhoseDependenciesAreUnfinished(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"

	// The cross edges run BOTH ways (epic-0's draft needs epic-1's in-flight
	// work, epic-1's draft rests on epic-0's done work), so the implied epic
	// graph cycles and epic rank degrades to key order with epic-0 first,
	// the exact degradation that put audit-engine at Acme's frontier. The
	// feature grain stays acyclic, and the flow walk reads it honestly.
	gated := feat(t, "F-gated0", "epic-0/selection", "epic-0")
	gated.Status = "draft"
	gated.DependsOn = []string{"F-nflght"}
	tree.Features["F-gated0"] = gated

	doneBase := feat(t, "F-done00", "epic-0/done", "epic-0")
	doneBase.Status = "done"
	tree.Features["F-done00"] = doneBase

	inflight := feat(t, "F-nflght", "epic-1/inflight", "epic-1")
	inflight.Status = "active"
	tree.Features["F-nflght"] = inflight

	ok := feat(t, "F-ok0000", "epic-1/base", "epic-1")
	ok.Status = "draft"
	ok.DependsOn = []string{"F-done00"}
	tree.Features["F-ok0000"] = ok

	specs := candidatesOfKind(nextFor(tree, "jc", workflow.Input{}).Candidates, "specify")
	require.Equal(t, []string{"F-ok0000"}, candidateNodeIDs(specs),
		"the dependency-satisfied draft is the frontier, though the gated one's epic leads by key")
}

// TestFrontierCountsADoneOrCancelledDependencyAsSatisfied pins the gate's
// terminal reading: done satisfies, cancelled neither counts nor blocks (the
// same rule epicRollupReady applies one level up), so a draft resting only on
// those still routes.
func TestFrontierCountsADoneOrCancelledDependencyAsSatisfied(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	done := feat(t, "F-done00", "epic-0/done", "epic-0")
	done.Status = "done"
	tree.Features["F-done00"] = done

	cancelled := feat(t, "F-cancl0", "epic-0/cancelled", "epic-0")
	cancelled.Status = "cancelled"
	tree.Features["F-cancl0"] = cancelled

	draft := feat(t, "F-draft0", "epic-0/next", "epic-0")
	draft.Status = "draft"
	draft.DependsOn = []string{"F-done00", "F-cancl0", "F-dangling"}
	tree.Features["F-draft0"] = draft

	top := topMove(t, tree, "jc", workflow.Input{})
	require.Equal(t, "specify", top.Kind)
	require.Equal(t, "F-draft0", top.Node.ID,
		"done and cancelled dependencies satisfy the gate, and a dangling id never gates")
}

// TestReadoutIsCaughtUpWhenEveryDraftIsGated pins the strict gate through
// the readout: drafts exist but every one waits on unfinished work, and work
// blocked by an unmet dependency is not a legal move.
func TestReadoutIsCaughtUpWhenEveryDraftIsGated(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	inflight := feat(t, "F-nflght", "epic-0/inflight", "epic-0")
	inflight.Status = "active"
	tree.Features["F-nflght"] = inflight

	gated := feat(t, "F-gated0", "epic-0/next", "epic-0")
	gated.Status = "draft"
	gated.DependsOn = []string{"F-nflght"}
	tree.Features["F-gated0"] = gated

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})

	require.True(t, r.Next.CaughtUp, "a draft behind unfinished work is not a move, and nothing else is legal: %v", nextNodeIDs(r))
	require.Empty(t, r.Next.Candidates)
}

// TestOrientReadoutStaysQuietOnAnHonestGraph is the absent half: no cycle, no
// line, so every clean orient stays exactly as long as it was.
func TestOrientReadoutStaysQuietOnAnHonestGraph(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	for _, c := range r.Checks {
		require.NotEqual(t, "Dependency order", c.Label)
	}
}

// TestRouteFollowsRootSoftOrderOverKeyOrder pins the routing half of the soft
// order stack at the views level: two independent draft
// features under two independent epics, neither depending on the other, so
// the recorded graph picks between them not at all and the tiebreak decides.
// With no root order recorded the tiebreak falls all the way to key, which
// routes to epic-0's feature first (its key sorts ahead of epic-1's).
// Recording a root soft order that lists epic-1 first moves its feature to
// the front instead, on Route directly and on the readout's own Next line,
// which is the one path a human or agent actually reads the routing walk
// from. TestFeatureFlowFollowsEpicRankThenSoftOrder in
// internal/gen/order_test.go already pins the same rule one layer down, at
// gen.Order.FeatureFlow itself (rewired by Task 6); this test's value is
// pinning it here too, at the surface Route and the readout consume, since
// the views Route walk was left untouched by that task and could in
// principle have drifted from what FeatureFlow now says.
func TestRouteFollowsRootSoftOrderOverKeyOrder(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"
	a := feat(t, "F-aaaaa0", "epic-0/one", "epic-0")
	a.Status = "draft"
	tree.Features["F-aaaaa0"] = a
	b := feat(t, "F-bbbbb0", "epic-1/one", "epic-1")
	b.Status = "draft"
	tree.Features["F-bbbbb0"] = b

	// No root order recorded: key order stands, so epic-0's feature leads.
	top := topMove(t, tree, "", workflow.Input{})
	require.Equal(t, "specify", top.Kind)
	require.Equal(t, "F-aaaaa0", top.Node.ID,
		"no soft order recorded, so key order ranks epic-0's feature first")

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	require.Equal(t, []string{"F-aaaaa0", "F-bbbbb0"}, nextNodeIDs(r))

	// The root soft order inverts the epics' key order: epic-1 now leads, and
	// routing follows it to that epic's feature instead.
	tree.ProjectOrder = store.ProjectOrder{Epics: []string{"E-aaaaa1", "E-aaaaa0"}}
	top = topMove(t, tree, "", workflow.Input{})
	require.Equal(t, "specify", top.Kind)
	require.Equal(t, "F-bbbbb0", top.Node.ID,
		"the root soft order lists epic-1 first, so ranking follows it over key order")

	r = views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	require.Equal(t, []string{"F-bbbbb0", "F-aaaaa0"}, nextNodeIDs(r))
}

// TestDependencyOrderCheckNamesAnOverriddenSoftOrder pins the readout half of
// the same stack: epic-0 depends on epic-1, so the
// recorded edge (never violated) forces epic-1 to render first regardless of
// what the root soft order asks for. Listing epic-0 first in that soft order
// is therefore a request the dependency layer overrides, and the "Dependency
// order" check line names the pair rather than staying silent about it, the
// same "silence was the bug" rule dependencyOrderCheck's cycle fallback
// already lives by.
func TestDependencyOrderCheckNamesAnOverriddenSoftOrder(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"
	tree.Epics["E-aaaaa0"].DependsOn = []string{"E-aaaaa1"}
	tree.ProjectOrder = store.ProjectOrder{Epics: []string{"E-aaaaa0", "E-aaaaa1"}}

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	var found *views.ReadoutCheck
	for i := range r.Checks {
		if r.Checks[i].Label == "Dependency order" {
			found = &r.Checks[i]
		}
	}
	require.NotNil(t, found,
		"epic-1 rendered ahead of epic-0 though the soft order names epic-0 first, and that inversion is worth a line")
	// The full string, not three order-agnostic Contains: refX (the
	// soft-listed-earlier epic) must lead and refY (the one dependencies
	// rendered ahead of it) must follow, in that order, or a refX/refY swap
	// would pass unnoticed.
	require.Equal(t,
		"soft order for epic-0 (E-aaaaa0) and epic-1 (E-aaaaa1) is overridden by recorded dependencies",
		found.Result)
}

// TestDependencyOrderCheckStaysQuietOnAConsistentSoftOrder is the contrast
// case: the same depends_on edge, but the root soft order already lists
// epic-1 (the one the edge forces first) ahead of epic-0, so the rendered
// order and the recorded soft order agree. Nothing was overridden, so the
// check renders no line at all, the same absent-means-clean rule the cycle
// fallback's own quiet-graph test pins.
func TestDependencyOrderCheckStaysQuietOnAConsistentSoftOrder(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"
	tree.Epics["E-aaaaa0"].DependsOn = []string{"E-aaaaa1"}
	tree.ProjectOrder = store.ProjectOrder{Epics: []string{"E-aaaaa1", "E-aaaaa0"}}

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	for _, c := range r.Checks {
		require.NotEqual(t, "Dependency order", c.Label,
			"the soft order already agrees with the enforced order, so nothing was overridden")
	}
}

// TestSpecifyReadoutFallsBackToTheRouteWithNothingSpecced covers the plan
// gate's empty arm: a specify readout run over a tree with no specced feature
// has no plan gate to name, so the block ends on the frontier draft.
func TestSpecifyReadoutFallsBackToTheRouteWithNothingSpecced(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	draft := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	draft.Status = "draft"
	tree.Features["F-aaaaa0"] = draft

	r := views.ReadoutOfInput(tree, views.MomentSpecify, views.ReadoutInput{})

	top := topNext(t, r)
	require.Equal(t, "specify", top.Kind)
	require.Equal(t, "agent", top.Actor)
}

// TestDecomposeReadoutNamesTheFrontierAsNext pins the other half of the pair
// the single-tree model renames: what follows an approved decomposition is
// the feature frontier, the first live feature in dependency order whose turn
// it is, not a planning layer between the tree and its features
// (artifact-schema.md, One tree, no effort axis). The epic is approved here
// because a draft epic is its own wait ahead of the frontier (issue #93,
// route_epic_gate_test.go).
func TestDecomposeReadoutNamesTheFrontierAsNext(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	first := feat(t, "F-aaaaa0", "epic-0/alpha", "epic-0")
	first.Status = "draft"
	tree.Features["F-aaaaa0"] = first

	second := feat(t, "F-aaaaa1", "epic-0/beta", "epic-0")
	second.Status = "draft"
	tree.Features["F-aaaaa1"] = second

	r := views.ReadoutOfInput(tree, views.MomentDecompose, views.ReadoutInput{})

	require.Equal(t, []string{"specify", "specify"}, nextKinds(r))
	require.Equal(t, []string{"F-aaaaa0", "F-aaaaa1"}, nextNodeIDs(r), "the frontier ranks in dependency order")
}

// TestSpecifyNextTeachesTheWriteThenAssertArc pins that a draft's move names
// the specify skill rather than a bare `tiller spec feature`: the bare
// command sent a dry run bouncing between spec and approve (JC, 2026-07-30),
// and the skill carries the write-then-assert arc.
func TestSpecifyNextTeachesTheWriteThenAssertArc(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	draft := feat(t, "F-aaaaa0", "epic-0/alpha", "epic-0")
	draft.Status = "draft"
	tree.Features["F-aaaaa0"] = draft

	r := views.ReadoutOfInput(tree, views.MomentDecompose, views.ReadoutInput{})

	top := topNext(t, r)
	require.Equal(t, "specify", top.Skill)
	require.Empty(t, top.Command, "the move is the skill, not a bare spec command")
}

// TestNextRanksATaskInHandAheadOfTheFrontier pins the ranking on
// the tree that decides it: a feature whose plan is approved (its task ready)
// sits AHEAD, by key, of a feature still in draft. Ranking the frontier first
// here answers "go specify epic-0/zebra" while an approved task waits, which is
// the opposite of finishing what the last gate started.
func TestNextRanksATaskInHandAheadOfTheFrontier(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	ready := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	ready.Status = "ready"
	tree.Features["F-aaaaa0"] = ready

	draft := feat(t, "F-aaaaa1", "epic-0/zebra", "epic-0")
	draft.Status = "draft"
	tree.Features["F-aaaaa1"] = draft

	task := &store.Task{ID: "T-aaaaa0", Status: "ready", Owner: "jc", FeatureID: "F-aaaaa0"}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{"F-aaaaa0": {task}}

	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"continue", "specify"}, candidateKinds(next.Candidates))
	require.Equal(t, "T-aaaaa0", next.Candidates[0].Node.ID)
}

func questionHeldRouteTree(t *testing.T) (*store.Tree, *store.Feature, *store.Task, *store.Task) {
	t.Helper()
	tree := treeWithDraftEpics(t, 1)
	epic := tree.Epics["E-aaaaa0"]
	epic.Status = "ready"
	legalEpicKey := "epic-1"
	legalEpic := &store.Epic{ID: "E-legal", Key: &legalEpicKey, Status: "ready"}
	tree.Epics[legalEpic.ID] = legalEpic
	tree.ProjectOrder.Epics = []string{epic.ID, legalEpic.ID}

	feature := feat(t, "F-route", "epic-0/routing", "epic-0")
	feature.Status = "ready"
	legalFeature := feat(t, "F-legal", "epic-1/routing-legal", "epic-1")
	legalFeature.Status = "ready"
	tree.Features[feature.ID] = feature
	tree.Features[legalFeature.ID] = legalFeature

	held := &store.Task{
		ID: "T-held", Key: strPtr("epic-0/routing/zeta-held"),
		Status: "ready", Owner: "jc", FeatureID: feature.ID,
	}
	legal := &store.Task{
		ID: "T-legal", Key: strPtr("epic-1/routing-legal/alpha-legal"),
		Status: "ready", Owner: "jc", FeatureID: legalFeature.ID,
	}
	tree.Tasks = map[string]*store.Task{held.ID: held, legal.ID: legal}
	tree.FeatureTasks = map[string][]*store.Task{
		feature.ID: {held}, legalFeature.ID: {legal},
	}
	tree.Questions = map[string]*store.Question{
		"Q-zulu": {
			ID: "Q-zulu", Title: "Deployment shape",
			Statement: "Which detailed deployment shape should this route use?",
			Status:    "open", Blocks: []string{epic.ID, feature.ID},
		},
		"Q-alpha": {
			ID: "Q-alpha", Title: "Architecture boundary",
			Statement: "Which detailed architecture boundary should this route use?",
			Status:    "open", Blocks: []string{feature.ID},
		},
		"Q-middle": {
			ID: "Q-middle", Title: "Migration sequence",
			Statement: "Which detailed migration sequence should this route use?",
			Status:    "open", Blocks: []string{feature.ID},
		},
		"Q-answered": {
			ID: "Q-answered", Title: "Answered detail",
			Statement: "This answered statement must not hold the route.",
			Status:    "answered", Blocks: []string{held.ID},
		},
	}
	return tree, feature, held, legal
}

// TestAQuestionHeldTaskYieldsToItsQuestionsAndTheLaterLegalTask pins a
// question hold: the soft-order leader is held by three open questions, so
// it is no move; the later legal task still leads, and each holding question
// is a move in queue order. The answered question holds nothing.
func TestAQuestionHeldTaskYieldsToItsQuestionsAndTheLaterLegalTask(t *testing.T) {
	tree, _, held, legal := questionHeldRouteTree(t)

	next := nextFor(tree, "jc", workflow.Input{})
	require.False(t, next.CaughtUp)
	top := next.Candidates[0]
	require.Equal(t, "continue", top.Kind)
	require.Equal(t, legal.ID, top.Node.ID,
		"the later legal task remains the top move when the soft-order leader is held")
	require.Empty(t, candidatesFor(next.Candidates, held.ID), "a question-held task is not a move")
	answers := candidatesOfKind(next.Candidates, "answer_question")
	require.Equal(t, []string{"Q-alpha", "Q-middle", "Q-zulu"}, candidateNodeIDs(answers),
		"every holding question is a move; the answered one is not")
	require.Equal(t, `tiller answer question Q-alpha --answer "..."`, answers[0].Command)
	require.Equal(t, "Architecture boundary", answers[0].Node.Title)

	readout := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{Contributor: "jc"})
	ids := nextNodeIDs(readout)
	require.Contains(t, ids, legal.ID)
	require.NotContains(t, ids, held.ID, "a question-held task is not a move; its questions are")
	require.Contains(t, ids, "Q-alpha")
}

// TestCaughtUpOnATreeHoldingACancelledTaskLessFeature pins that a cancelled
// feature with no tasks is never a move: the frontier skips it, and with
// nothing else legal the reader is caught up.
func TestCaughtUpOnATreeHoldingACancelledTaskLessFeature(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	cancelled := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	cancelled.Status = "cancelled"
	tree.Features["F-aaaaa0"] = cancelled

	next := nextFor(tree, "jc", workflow.Input{})
	require.True(t, next.CaughtUp)
	require.Empty(t, next.Candidates)

	// And the readout closes caught up.
	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})
	require.True(t, r.Next.CaughtUp)
}

// TestCaughtUpReportsTheReaderRatherThanCensusingTheTree pins that a ready
// task claimed by someone else is no move for any other reader, while the
// block's own "Tasks by state" census still prints "1 ready". Two readers on
// one tree: the owner of a second checkout, and a checkout that resolved no
// owner at all (a fresh clone or CI). The holder is asserted too, so the
// tree is known to carry work rather than to be quietly empty.
func TestCaughtUpReportsTheReaderRatherThanCensusingTheTree(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "ready"
	tree.Features["F-aaaaa0"] = f

	task := &store.Task{ID: "T-aaaaa0", Status: "ready", Owner: "dana", FeatureID: "F-aaaaa0"}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{"F-aaaaa0": {task}}

	top := topMove(t, tree, "dana", workflow.Input{})
	require.Equal(t, "continue", top.Kind, "the holder continues the work, so the tree really does carry a ready task")
	require.Equal(t, task.ID, top.Node.ID)

	for _, reader := range []string{"jc", ""} {
		next := nextFor(tree, reader, workflow.Input{})
		require.True(t, next.CaughtUp, "reader %q holds nothing", reader)
		require.Empty(t, next.Hint, "dana's claimed task is not held unclaimed work for reader %q", reader)

		r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{Contributor: reader})
		require.Equal(t, "1 ready", checkLabels(r)["Tasks by state"], "the census reports the tree for reader %q", reader)
		require.True(t, r.Next.CaughtUp, "dana's task is not reader %q's move, and nothing else is legal: %v", reader, nextNodeIDs(r))
	}
}

// routeArmFixtures builds one tree per arm of the retired route walk
// (decompose via the empty tree, own task, amendment wait, plan gate,
// specify, in_review wait, blocked frontier, nothing due), keyed by arm name.
// The TestTopMoveNames* tests below and the baseline suite drive them.
var routeArmFixtures = map[string]func(t *testing.T) *store.Tree{
	"empty tree": func(t *testing.T) *store.Tree {
		tree := treeWithDraftEpics(t, 0)
		tree.ToBeDesign = []string{"roadmap.md"}
		return tree
	},
	"execute": func(t *testing.T) *store.Tree {
		tree := treeWithDraftEpics(t, 1)
		tree.Epics["E-aaaaa0"].Status = "ready"
		ready := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
		ready.Status = "ready"
		tree.Features["F-aaaaa0"] = ready
		task := &store.Task{ID: "T-aaaaa0", Key: strPtr("epic-0/loading/step"), Status: "ready", Owner: "jc", FeatureID: "F-aaaaa0"}
		tree.Tasks = map[string]*store.Task{task.ID: task}
		tree.FeatureTasks = map[string][]*store.Task{"F-aaaaa0": {task}}
		return tree
	},
	"amendment wait": func(t *testing.T) *store.Tree {
		tree := treeWithDraftEpics(t, 1)
		tree.Epics["E-aaaaa0"].Status = "ready"
		specced := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
		specced.Status = "specced"
		tree.Features["F-aaaaa0"] = specced
		tree.Amendments = map[string]*store.Amendment{
			"A-aaaaaa": {ID: "A-aaaaaa", Key: strPtr("iphone-only"), Status: store.AmendmentStatusProposed},
		}
		return tree
	},
	"plan gate": func(t *testing.T) *store.Tree {
		tree := treeWithDraftEpics(t, 1)
		tree.Epics["E-aaaaa0"].Status = "ready"
		specced := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
		specced.Status = "specced"
		tree.Features["F-aaaaa0"] = specced
		return tree
	},
	"specify": func(t *testing.T) *store.Tree {
		tree := treeWithDraftEpics(t, 1)
		tree.Epics["E-aaaaa0"].Status = "ready"
		draft := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
		draft.Status = "draft"
		tree.Features["F-aaaaa0"] = draft
		return tree
	},
	"in_review wait": func(t *testing.T) *store.Tree {
		tree := treeWithDraftEpics(t, 1)
		tree.Epics["E-aaaaa0"].Status = "ready"
		active := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
		active.Status = "active"
		tree.Features["F-aaaaa0"] = active
		task := &store.Task{ID: "T-aaaaa0", Key: strPtr("epic-0/loading/step"), Status: "in_review", FeatureID: "F-aaaaa0"}
		tree.Tasks = map[string]*store.Task{task.ID: task}
		tree.FeatureTasks = map[string][]*store.Task{"F-aaaaa0": {task}}
		return tree
	},
	"blocked frontier": func(t *testing.T) *store.Tree {
		tree := treeWithDraftEpics(t, 1)
		tree.Epics["E-aaaaa0"].Status = "ready"
		inflight := feat(t, "F-nflght", "epic-0/inflight", "epic-0")
		inflight.Status = "active"
		tree.Features["F-nflght"] = inflight
		gated := feat(t, "F-gated0", "epic-0/next", "epic-0")
		gated.Status = "draft"
		gated.DependsOn = []string{"F-nflght"}
		tree.Features["F-gated0"] = gated
		return tree
	},
	"no route due": func(t *testing.T) *store.Tree {
		tree := treeWithDraftEpics(t, 1)
		tree.Epics["E-aaaaa0"].Status = "ready"
		cancelled := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
		cancelled.Status = "cancelled"
		tree.Features["F-aaaaa0"] = cancelled
		return tree
	},
}

// TestTopMoveNamesTheOwnTask pins the reader's own ready task as the top
// move, carrying the node a caller links to.
func TestTopMoveNamesTheOwnTask(t *testing.T) {
	top := topMove(t, routeArmFixtures["execute"](t), "jc", workflow.Input{})
	require.Equal(t, "continue", top.Kind)
	require.Equal(t, "agent", top.Actor)
	require.Equal(t, "execute", top.Skill)
	require.Equal(t, store.NounTask, top.Node.Noun)
	require.Equal(t, "T-aaaaa0", top.Node.ID)
}

// TestTopMoveNamesTheAmendmentWaitsAmendment pins the amendment wait's node:
// the proposed amendment, ahead of the plan gate.
func TestTopMoveNamesTheAmendmentWaitsAmendment(t *testing.T) {
	top := topMove(t, routeArmFixtures["amendment wait"](t), "jc", workflow.Input{})
	require.Equal(t, "approve_amendment", top.Kind)
	require.Equal(t, "human", top.Actor)
	require.Equal(t, store.NounAmendment, top.Node.Noun)
	require.Equal(t, "A-aaaaaa", top.Node.ID)
}

// TestTopMoveNamesThePlanGatesFeature pins the plan gate's node: the specced
// feature one approve away from ready.
func TestTopMoveNamesThePlanGatesFeature(t *testing.T) {
	top := topMove(t, routeArmFixtures["plan gate"](t), "jc", workflow.Input{})
	require.Equal(t, "approve_feature", top.Kind)
	require.Equal(t, "human", top.Actor)
	require.Equal(t, store.NounFeature, top.Node.Noun)
	require.Equal(t, "F-aaaaa0", top.Node.ID)
}

// TestTopMoveNamesTheFrontiersFeature pins the specify move's node: the
// draft feature the frontier accepts.
func TestTopMoveNamesTheFrontiersFeature(t *testing.T) {
	top := topMove(t, routeArmFixtures["specify"](t), "jc", workflow.Input{})
	require.Equal(t, "specify", top.Kind)
	require.Equal(t, "specify", top.Skill)
	require.Equal(t, store.NounFeature, top.Node.Noun)
	require.Equal(t, "F-aaaaa0", top.Node.ID)
}

// TestTopMoveNamesTheInReviewTask pins the merge wait's node: the task
// sitting in_review, a human move rather than a seam.
func TestTopMoveNamesTheInReviewTask(t *testing.T) {
	top := topMove(t, routeArmFixtures["in_review wait"](t), "jc", workflow.Input{})
	require.Equal(t, "merge_task", top.Kind)
	require.Equal(t, "human", top.Actor)
	require.Empty(t, top.Skill)
	require.Equal(t, store.NounTask, top.Node.Noun)
	require.Equal(t, "T-aaaaa0", top.Node.ID)
}

// TestABlockedFrontierIsNoMove pins that work behind an unmet
// dependency is not a legal move, so a tree whose only draft waits on work
// in flight is caught up rather than naming the gated draft.
func TestABlockedFrontierIsNoMove(t *testing.T) {
	next := nextFor(routeArmFixtures["blocked frontier"](t), "jc", workflow.Input{})
	require.True(t, next.CaughtUp)
	require.Empty(t, next.Candidates)
}

// TestTopMoveNamesNoNodeForAnEmptyTree and TestNothingDueIsCaughtUp pin the
// two arms that name no node: decompose acts on the design docs, and a tree
// with nothing legal has no move at all.
func TestTopMoveNamesNoNodeForAnEmptyTree(t *testing.T) {
	top := topMove(t, routeArmFixtures["empty tree"](t), "jc", workflow.Input{})
	require.Equal(t, "decompose", top.Kind)
	require.Equal(t, "no epics recorded yet; 1 to-be design doc awaits decomposition, starting with .tiller/design/roadmap.md", top.Reason)
	require.Nil(t, top.Node, "an undecomposed tree names no node")
}

func TestNothingDueIsCaughtUp(t *testing.T) {
	next := nextFor(routeArmFixtures["no route due"](t), "jc", workflow.Input{})
	require.True(t, next.CaughtUp)
	require.Empty(t, next.Candidates)
}

// TestExecuteReadoutSuppressesTheParkedFeatureUnderARetiredEpic is the Next
// arm's own suppression, the sibling of queue.go's queueFeatureChecks
// suppression: a feature otherwise awaiting check must not offer `tiller
// check feature` once its own epic is cancelled or archived, since gen.FeatureActivity
// reads only feature/task facts and answers awaiting_check regardless of the
// epic, and the command now refuses with ParentEpicRetiredError
// (whole-branch review, issue #36). This fixture's feature is also owner-less
// and non-terminal, so it sits in the Unclaimed pool: the caught-up terminal
// this tree reaches now names it in PoolSignpost's own sentence (issue #92),
// by full reference, "F-aaaaaa" included, which is fine, since PoolSignpost's
// whole point is honestly naming why a pool feature cannot start. Only the
// specific suppressed check command must never appear.
func TestExecuteReadoutSuppressesTheParkedFeatureUnderARetiredEpic(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	epicKey := "foundation"
	tree.Epics["E-aaaaaa"] = &store.Epic{ID: "E-aaaaaa", Key: &epicKey, Status: "cancelled"}
	key := "foundation/repo-tooling"
	f := &store.Feature{ID: "F-aaaaaa", Key: &key, EpicKey: epicKey, Status: "active"}
	tree.Features[f.ID] = f
	tree.Manifest.Gates.Required = map[string]store.GateSpec{
		"evals": {Kind: "command", On: store.StringList{"feature"}},
	}
	tk := &store.Task{ID: "T-aaaaa0", Status: "done", FeatureID: f.ID}
	tree.Tasks = map[string]*store.Task{tk.ID: tk}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {tk}}

	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})

	for _, c := range r.Next.Candidates {
		require.NotEqual(t, "tiller check feature F-aaaaaa", c.Command,
			"a feature whose epic is retired must not offer a check command that would now refuse")
		require.NotContains(t, c.Reason,
			"has every task done; its required gates have no recorded result",
			"the suppressed check-awaiting sentence must not appear either")
	}
}

// Multiple outstanding reviews stay visible in the uncapped summary while
// Next is the same ranked list as the correction block's.
func TestExecuteReadoutSummarizesEveryOutstandingReviewWithoutCompetingNextActions(t *testing.T) {
	tree, first, in := workflowFeature(t, store.ReviewGrainTask)
	second, task := readyFeatureWithTask(t, tree, "F-bbbbbb", "epic-0/second", "epic-0")
	second.Status = "active"
	task.Status = "done"
	in.Contracts[second.ID] = in.Contracts[first.ID]
	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc", Workflow: in})
	labels := checkLabels(r)
	require.Contains(t, labels, "Resolved review "+gen.Ref(tree, first))
	require.Contains(t, labels, "Resolved review "+gen.Ref(tree, second))
	correction := views.CorrectionOfInput(tree, views.MomentExecute, views.CorrectionInput{Contributor: "jc", Workflow: in})
	require.Equal(t, correction.Next, r.Next)
	require.Equal(t, "tiller check feature "+first.ID, topNext(t, r).Command)
}

// TestFrontierPrefersTheSoftOrderedEpicAcrossFlowLayers pins the issue #46
// fix: eligibility comes from the dependency gate alone, and among eligible
// drafts the recorded soft order outranks flow position. The fixture is the
// Acme shape reduced to three epics: epic-0 holds a done feature, epic-1
// (soft-listed first) holds a draft whose one dependency is that done
// feature (flow layer 1), and epic-2 holds a dependency-free draft (flow
// layer 0). Before the fix the layer-0 draft won regardless of the recorded
// order; the soft-listed epic's eligible draft must win now.
func TestFrontierPrefersTheSoftOrderedEpicAcrossFlowLayers(t *testing.T) {
	tree := treeWithDraftEpics(t, 3)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"
	tree.Epics["E-aaaaa2"].Status = "ready"

	done := feat(t, "F-ddddd0", "epic-0/base", "epic-0")
	done.Status = "done"
	tree.Features["F-ddddd0"] = done

	deep := feat(t, "F-eeeee1", "epic-1/next", "epic-1")
	deep.Status = "draft"
	deep.DependsOn = []string{"F-ddddd0"}
	tree.Features["F-eeeee1"] = deep

	free := feat(t, "F-fffff2", "epic-2/lane", "epic-2")
	free.Status = "draft"
	tree.Features["F-fffff2"] = free

	tree.ProjectOrder = store.ProjectOrder{Epics: []string{"E-aaaaa1"}}

	require.Equal(t, []string{"F-eeeee1", "F-fffff2"}, frontierIDs(tree),
		"the soft-listed epic's eligible draft outranks a dependency-free draft in an earlier flow layer")

	top := topMove(t, tree, "", workflow.Input{})
	require.Equal(t, "F-eeeee1", top.Node.ID,
		"the top move is the soft-preferred candidate, not the layer-0 one")
}

// TestFrontierKeepsFlowOrderWithoutASoftOrder is the byte-stability guard
// beside the fix: the identical tree with no recorded soft order keeps the
// pure flow answer, the dependency-free layer-0 draft.
func TestFrontierKeepsFlowOrderWithoutASoftOrder(t *testing.T) {
	tree := treeWithDraftEpics(t, 3)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"
	tree.Epics["E-aaaaa2"].Status = "ready"

	done := feat(t, "F-ddddd0", "epic-0/base", "epic-0")
	done.Status = "done"
	tree.Features["F-ddddd0"] = done

	deep := feat(t, "F-eeeee1", "epic-1/next", "epic-1")
	deep.Status = "draft"
	deep.DependsOn = []string{"F-ddddd0"}
	tree.Features["F-eeeee1"] = deep

	free := feat(t, "F-fffff2", "epic-2/lane", "epic-2")
	free.Status = "draft"
	tree.Features["F-fffff2"] = free

	require.Equal(t, []string{"F-fffff2", "F-eeeee1"}, frontierIDs(tree),
		"no soft order recorded: flow position decides, the layer-0 draft leads")
}

// TestFrontierPrefersTheFeatureSoftOrdinalWithinTheEpic extends the same
// rule one grain down: within the soft-listed epic, the epic's recorded
// feature order outranks flow position among its own eligible drafts.
func TestFrontierPrefersTheFeatureSoftOrdinalWithinTheEpic(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"

	done := feat(t, "F-ddddd0", "epic-0/base", "epic-0")
	done.Status = "done"
	tree.Features["F-ddddd0"] = done

	deep := feat(t, "F-eeeee1", "epic-1/deep", "epic-1")
	deep.Status = "draft"
	deep.DependsOn = []string{"F-ddddd0"}
	tree.Features["F-eeeee1"] = deep

	free := feat(t, "F-fffff1", "epic-1/free", "epic-1")
	free.Status = "draft"
	tree.Features["F-fffff1"] = free

	tree.ProjectOrder = store.ProjectOrder{Epics: []string{"E-aaaaa1"}}
	tree.Epics["E-aaaaa1"].Order = []string{"F-eeeee1", "F-fffff1"}

	require.Equal(t, []string{"F-eeeee1", "F-fffff1"}, frontierIDs(tree),
		"the epic's recorded feature order outranks flow position among its eligible drafts")
}

// TestFrontierUnlistedEpicsKeepFlowOrderAmongThemselves pins the fallback
// tier: a recorded soft order that names neither candidate's epic leaves the
// unlisted candidates in flow order among themselves.
func TestFrontierUnlistedEpicsKeepFlowOrderAmongThemselves(t *testing.T) {
	tree := treeWithDraftEpics(t, 3)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"
	tree.Epics["E-aaaaa2"].Status = "ready"

	done := feat(t, "F-ddddd0", "epic-0/base", "epic-0")
	done.Status = "done"
	tree.Features["F-ddddd0"] = done

	deep := feat(t, "F-eeeee1", "epic-1/next", "epic-1")
	deep.Status = "draft"
	deep.DependsOn = []string{"F-ddddd0"}
	tree.Features["F-eeeee1"] = deep

	free := feat(t, "F-fffff2", "epic-2/lane", "epic-2")
	free.Status = "draft"
	tree.Features["F-fffff2"] = free

	tree.ProjectOrder = store.ProjectOrder{Epics: []string{"E-aaaaa0"}}

	require.Equal(t, []string{"F-fffff2", "F-eeeee1"}, frontierIDs(tree),
		"the listed epic has no draft; unlisted candidates keep flow order among themselves")
}

// TestFrontierGatesOnTheOwningEpicsRecordedEdge pins the second half of the
// issue #46 fix: an epic-grain depends_on edge gates the epic's features the
// way a feature-grain edge gates the feature, so a soft order preferring the
// dependent epic cannot rank around the recorded edge. epic-1 depends on
// epic-0 (unfinished); epic-1's dependency-free draft is soft-preferred but
// ineligible, so the frontier stays on epic-0's draft, and once every draft
// is gated nothing is a move, however soft-preferred.
func TestFrontierGatesOnTheOwningEpicsRecordedEdge(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"
	tree.Epics["E-aaaaa1"].DependsOn = []string{"E-aaaaa0"}

	base := feat(t, "F-bbbbb0", "epic-0/base", "epic-0")
	base.Status = "draft"
	tree.Features["F-bbbbb0"] = base

	dep := feat(t, "F-ccccc1", "epic-1/dependent", "epic-1")
	dep.Status = "draft"
	tree.Features["F-ccccc1"] = dep

	tree.ProjectOrder = store.ProjectOrder{Epics: []string{"E-aaaaa1", "E-aaaaa0"}}

	require.Equal(t, []string{"F-bbbbb0"}, frontierIDs(tree),
		"the recorded epic-grain edge gates epic-1's draft; the soft order cannot rank around it")

	// With epic-0's draft in flight, nothing is eligible, and epic-1's
	// soft-preferred draft is still no move.
	base.Status = "active"
	require.Empty(t, frontierIDs(tree))
	require.True(t, nextFor(tree, "", workflow.Input{}).CaughtUp)
}

// TestFrontierListedFeatureOutranksUnlistedAtAnyListDepth pins the sentinel
// against the review-caught overflow: the unlisted rank is absolute, never
// derived from a list's length, so a feature listed deep in a long epic
// Order still outranks an unlisted sibling. The fixture makes the derived
// sentinel's collision concrete: one epic, one root list entry, and an
// Order whose eligible listed candidate sits at ordinal three, which a
// sentinel sized from the epic tier (three, here) would tie or lose to.
func TestFrontierListedFeatureOutranksUnlistedAtAnyListDepth(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"

	for i, id := range []string{"F-aaaaa0", "F-bbbbb0", "F-ccccc0"} {
		f := feat(t, id, fmt.Sprintf("epic-0/done-%d", i), "epic-0")
		f.Status = "done"
		tree.Features[id] = f
	}
	listed := feat(t, "F-ddddd0", "epic-0/zz-listed", "epic-0")
	listed.Status = "draft"
	// One settled dependency puts the listed candidate a flow layer deeper
	// than its unlisted sibling, so flow position favors the unlisted one
	// and only the sentinel's absoluteness keeps the listed one ahead.
	listed.DependsOn = []string{"F-aaaaa0"}
	tree.Features["F-ddddd0"] = listed
	unlisted := feat(t, "F-eeeee0", "epic-0/aa-unlisted", "epic-0")
	unlisted.Status = "draft"
	tree.Features["F-eeeee0"] = unlisted

	tree.ProjectOrder = store.ProjectOrder{Epics: []string{"E-aaaaa0"}}
	tree.Epics["E-aaaaa0"].Order = []string{"F-aaaaa0", "F-bbbbb0", "F-ccccc0", "F-ddddd0"}

	require.Equal(t, []string{"F-ddddd0", "F-eeeee0"}, frontierIDs(tree),
		"a feature listed at any depth outranks an unlisted sibling, whatever the list lengths")
}

// TestFrontierGatesOnTheOwningEpicsEdgeWithoutASoftOrder guards the gate's
// reach: the owning epic's recorded depends_on gates its features whether or
// not any soft order is recorded. epic-1 depends on epic-0, whose one
// feature is still in flight; with no ProjectOrder at all, epic-1's
// dependency-free draft is still not a move.
func TestFrontierGatesOnTheOwningEpicsEdgeWithoutASoftOrder(t *testing.T) {
	tree := treeWithDraftEpics(t, 2)
	tree.Epics["E-aaaaa0"].Status = "ready"
	tree.Epics["E-aaaaa1"].Status = "ready"
	tree.Epics["E-aaaaa1"].DependsOn = []string{"E-aaaaa0"}

	active := feat(t, "F-bbbbb0", "epic-0/inflight", "epic-0")
	active.Status = "active"
	tree.Features["F-bbbbb0"] = active

	dep := feat(t, "F-ccccc1", "epic-1/dependent", "epic-1")
	dep.Status = "draft"
	tree.Features["F-ccccc1"] = dep

	require.Empty(t, frontierIDs(tree), "the owning epic's unfinished edge gates its draft with no soft order recorded")
	require.True(t, nextFor(tree, "", workflow.Input{}).CaughtUp, "the gated draft is no move")
}

// grainNotesTree is issue #146's mixed-grain feature: the feature now resolves
// to feature grain while task alfa is in flight, started at startedGrain.
func grainNotesTree(t *testing.T, startedGrain string) *store.Tree {
	t.Helper()
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status, f.ReviewGrain = "active", store.ReviewGrainFeature
	tree.Features["F-aaaaa0"] = f
	key := "epic-0/loading/alfa"
	task := &store.Task{ID: "T-aaaaa0", Key: &key, Status: "in_review", Started: true, FeatureID: "F-aaaaa0", StartedGrain: startedGrain}
	tree.Tasks = map[string]*store.Task{task.ID: task}
	tree.FeatureTasks = map[string][]*store.Task{"F-aaaaa0": {task}}
	return tree
}

func TestReadoutGrainNotesNameStaleTask(t *testing.T) {
	tree := grainNotesTree(t, store.ReviewGrainTask)
	pr := 41
	tree.Tasks["T-aaaaa0"].PR = &pr
	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{})
	require.Equal(t, []string{"epic-0/loading/alfa (T-aaaaa0) rides task grain; its PR must merge before submit feature epic-0/loading (F-aaaaa0)."}, r.GrainNotes)
	require.Contains(t, r.GrainNotes[0], "rides task grain; its PR must merge before submit feature")
}

// With no PR recorded (remoteless, or not yet submitted) the note names
// either review surface the task may take.
func TestReadoutGrainNotesWithoutPRNameEitherReview(t *testing.T) {
	r := views.ReadoutOfInput(grainNotesTree(t, store.ReviewGrainTask), views.MomentExecute, views.ReadoutInput{})
	require.Equal(t, []string{"epic-0/loading/alfa (T-aaaaa0) rides task grain; its PR or review ref must merge before submit feature epic-0/loading (F-aaaaa0)."}, r.GrainNotes)
}

func TestReadoutGrainNotesEmptyForLegacyTask(t *testing.T) {
	r := views.ReadoutOfInput(grainNotesTree(t, ""), views.MomentExecute, views.ReadoutInput{})
	require.Empty(t, r.GrainNotes)
	r = views.ReadoutOfInput(grainNotesTree(t, store.ReviewGrainFeature), views.MomentExecute, views.ReadoutInput{})
	require.Empty(t, r.GrainNotes, "a task that started at the coarse grain rides the aggregate")
}
