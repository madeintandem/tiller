package views

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func streamsTree() *store.Tree {
	epicKey, featureKey := "widgets", "widgets/shared"
	tree := &store.Tree{
		Manifest: &store.Manifest{Contributors: []store.Contributor{{ID: "past", Name: "Past contributor"}, {ID: "current", Name: "Current owner"}}},
		Epics:    map[string]*store.Epic{"E-widgets": {ID: "E-widgets", Key: &epicKey, Status: "ready"}},
		Features: map[string]*store.Feature{"F-shared": {ID: "F-shared", Key: &featureKey, EpicKey: epicKey, Status: "active"}},
		Tasks:    map[string]*store.Task{}, FeatureTasks: map[string][]*store.Task{},
		Contributors: map[string][]store.Contributor{"E-widgets": {{ID: "past", Name: "Past contributor"}}, "F-shared": {{ID: "past", Name: "Past contributor"}}},
	}
	for _, status := range []string{"proposed", "ready", "in_progress", "check", "in_review", "done", "cancelled"} {
		key := featureKey + "/" + strings.ReplaceAll(status, "_", "-")
		task := &store.Task{ID: "T-" + status, Key: &key, FeatureID: "F-shared", Status: status, Owner: "current"}
		if status == "done" || status == "cancelled" {
			task.Owner, task.Started = "past", true
		}
		tree.Tasks[task.ID] = task
		tree.FeatureTasks["F-shared"] = append(tree.FeatureTasks["F-shared"], task)
	}
	return tree
}

func contributorStreamByID(t *testing.T, streams Streams, id string) ContributorStream {
	t.Helper()
	for _, owner := range streams.Contributors {
		if owner.Contributor.ID == id {
			return owner
		}
	}
	t.Fatalf("owner %s missing", id)
	return ContributorStream{}
}

func TestStreamsCurrentAssignmentsDoNotFollowContributorHistory(t *testing.T) {
	tree := streamsTree()
	tree.Tasks["T-check"].Blocked = true
	streams := StreamsOf(tree)
	past := contributorStreamByID(t, streams, "past")
	require.Empty(t, past.Active)
	require.Empty(t, past.Blocked)
	require.Empty(t, past.GatesInReview)
	current := contributorStreamByID(t, streams, "current")
	var ids []string
	for _, item := range current.Active {
		require.Equal(t, store.NounTask, item.Noun)
		ids = append(ids, item.ID)
	}
	require.ElementsMatch(t, []string{"T-proposed", "T-ready", "T-in_progress", "T-check"}, ids)
	require.Len(t, current.GatesInReview, 1)
	require.Equal(t, "T-in_review", current.GatesInReview[0].ID)
	require.Len(t, current.Blocked, 1)
	require.Equal(t, "T-check", current.Blocked[0].ID)
}

func TestStreamsUseRegisteredRosterNotAncestorParticipants(t *testing.T) {
	tree := streamsTree()
	tree.Manifest.Contributors = []store.Contributor{{ID: "current", Name: "Current engineer"}}
	streams := StreamsOf(tree)
	require.Len(t, streams.Contributors, 1)
	require.Equal(t, "current", streams.Contributors[0].Contributor.ID)
	require.Equal(t, []store.Contributor{{ID: "past", Name: "Past contributor"}}, tree.Contributors["F-shared"])
}

func TestStreamsUnclaimedUsesTaskEligibilityAndSoftOrder(t *testing.T) {
	tree := streamsTree()
	first := tree.Tasks["T-ready"]
	first.Owner = ""
	key := "widgets/shared/aaa-second"
	second := &store.Task{ID: "T-second", Key: &key, FeatureID: "F-shared", Status: "ready"}
	tree.Tasks[second.ID] = second
	tree.FeatureTasks["F-shared"] = append(tree.FeatureTasks["F-shared"], second)
	tree.Features["F-shared"].Order = []string{first.ID, second.ID}
	for _, status := range []string{"ready", "active"} {
		tree.Features["F-shared"].Status = status
		pool := StreamsOf(tree).Unclaimed
		require.Len(t, pool, 2)
		require.Equal(t, first.ID, pool[0].ID)
		require.Equal(t, second.ID, pool[1].ID)
		require.Equal(t, store.NounTask, pool[0].Noun)
		require.Equal(t, "F-shared", pool[0].FeatureID)
		data, err := json.Marshal(pool[0])
		require.NoError(t, err)
		require.NotContains(t, string(data), "task_count")
	}
	second.DependsOn = []string{first.ID}
	pool := StreamsOf(tree).Unclaimed
	require.Len(t, pool, 1)
	require.Equal(t, first.ID, pool[0].ID)
	tree.Questions = map[string]*store.Question{"Q-hold": {ID: "Q-hold", Status: "open", Blocks: []string{first.ID}}}
	require.Empty(t, StreamsOf(tree).Unclaimed)
	delete(tree.Questions, "Q-hold")
	in := workflow.Input{Eligibility: map[string]string{first.ID: "basis drift"}}
	require.Empty(t, StreamsOfInput(tree, in).Unclaimed)
	second.DependsOn = nil
	in.Eligibility = map[string]string{"F-shared": "basis drift"}
	require.Empty(t, StreamsOfInput(tree, in).Unclaimed)
}

func TestStreamsEmptyUnclaimedIsOmitted(t *testing.T) {
	data, err := json.Marshal(StreamsOf(streamsTree()))
	require.NoError(t, err)
	var wire map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &wire))
	require.NotContains(t, wire, "unclaimed")
}

func TestStreamsTaskConditionsRemainIndependentAndTerminalHistoryDisappears(t *testing.T) {
	tree := streamsTree()
	task := tree.Tasks["T-ready"]
	task.Blocked = true
	task.DependsOn = []string{"T-check"}
	tree.Epics["E-widgets"].Status = "cancelled"
	tree.Questions = map[string]*store.Question{"Q-hold": {ID: "Q-hold", Title: "Scope", Status: "open", Blocks: []string{task.ID, "F-shared"}}}
	current := contributorStreamByID(t, StreamsOf(tree), "current")
	var rows []BlockedItem
	for _, row := range current.Blocked {
		if row.ID == task.ID {
			rows = append(rows, row)
		}
	}
	require.Len(t, rows, 3)
	require.Equal(t, []string{"T-check"}, rows[0].DependsOn)
	require.Equal(t, []string{gen.Ref(tree, tree.Tasks["T-check"])}, rows[0].DependsOnRefs)
	require.NotNil(t, rows[1].Held)
	require.Equal(t, "E-widgets", rows[1].Held.ID)
	require.Len(t, rows[2].QuestionHolds, 1)
	require.Equal(t, "Q-hold", rows[2].QuestionHolds[0].Question)
	task.Status = "cancelled"
	for _, row := range contributorStreamByID(t, StreamsOf(tree), "current").Blocked {
		require.NotEqual(t, task.ID, row.ID)
	}
}

func TestStreamsAssignmentAndReviewOrderFollowDependencies(t *testing.T) {
	tree := streamsTree()
	tree.Tasks["T-check"].DependsOn = []string{"T-ready"}
	current := contributorStreamByID(t, StreamsOf(tree), "current")
	positions := map[string]int{}
	for i, item := range current.Active {
		positions[item.ID] = i
	}
	require.Contains(t, positions, "T-ready")
	require.Contains(t, positions, "T-check")
	require.Less(t, positions["T-ready"], positions["T-check"])
	tree.Tasks["T-check"].Status = "in_review"
	tree.Tasks["T-ready"].Status = "in_review"
	current = contributorStreamByID(t, StreamsOf(tree), "current")
	positions = map[string]int{}
	for i, item := range current.GatesInReview {
		positions[item.ID] = i
	}
	require.Contains(t, positions, "T-ready")
	require.Contains(t, positions, "T-check")
	require.Less(t, positions["T-ready"], positions["T-check"])
}

func TestStreamsShelfAssignmentsExcludeCarriedAndTerminalWork(t *testing.T) {
	tree := streamsTree()
	tree.Bugs = map[string]*store.Bug{
		"B-live":    {WorkItem: store.WorkItem{ID: "B-live", Status: "check", Owner: "current", Blocked: true}},
		"B-carried": {WorkItem: store.WorkItem{ID: "B-carried", Status: "ready", Owner: "current", ResolvedBy: "F-shared"}},
		"B-history": {WorkItem: store.WorkItem{ID: "B-history", Status: "done", Owner: "past", Blocked: true}},
		"B-review":  {WorkItem: store.WorkItem{ID: "B-review", Status: "in_review", Owner: "current", GatesBlock: store.GatesBlock{Gates: map[string]store.Gate{"tests": {Status: store.GateStatusPass}}}}},
	}
	tree.Chores = map[string]*store.Chore{"CH-proposal": {WorkItem: store.WorkItem{ID: "CH-proposal", Status: "proposed", Owner: "current"}}}
	tree.Tweaks = map[string]*store.Tweak{
		"TW-check":   {WorkItem: store.WorkItem{ID: "TW-check", Status: "check", Owner: "current"}},
		"TW-carried": {WorkItem: store.WorkItem{ID: "TW-carried", Status: "ready", Owner: "current", ResolvedBy: "F-shared"}},
		"TW-history": {WorkItem: store.WorkItem{ID: "TW-history", Status: "done", Owner: "past"}},
	}
	streams := StreamsOf(tree)
	current := contributorStreamByID(t, streams, "current")
	var ids []string
	for _, row := range current.Active {
		ids = append(ids, row.ID)
	}
	require.Contains(t, ids, "B-live")
	require.Contains(t, ids, "CH-proposal")
	require.Contains(t, ids, "TW-check")
	require.NotContains(t, ids, "TW-carried")
	require.NotContains(t, ids, "B-carried")
	require.NotContains(t, ids, "B-review")
	require.Len(t, current.Blocked, 1)
	require.Equal(t, "B-live", current.Blocked[0].ID)
	require.Equal(t, "B-review", current.GatesInReview[len(current.GatesInReview)-1].ID)
	require.Contains(t, current.GatesInReview[len(current.GatesInReview)-1].Gates, "tests")
	require.Empty(t, contributorStreamByID(t, streams, "past").Blocked)
}

func TestQuestionStreamsKeepActorAttributionAndFullStatement(t *testing.T) {
	tree := streamsTree()
	statement := strings.Repeat("A full question statement. ", 8)
	tree.Questions = map[string]*store.Question{"Q-open": {ID: "Q-open", Title: "Scope", Statement: statement, Status: "open", Owner: "past"}}
	past := contributorStreamByID(t, StreamsOf(tree), "past")
	require.Len(t, past.OpenQuestions, 1)
	require.Equal(t, "Scope", past.OpenQuestions[0].Title)
	require.Equal(t, statement, past.OpenQuestions[0].Statement)
	require.Empty(t, past.Active)
}
