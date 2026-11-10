package views_test

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func startPage(tree *store.Tree, task *store.Task) views.NodePage {
	return views.NodePageOf(tree, []events.Event{}, task, nil)
}

func TestNodePageReadyUnclaimedTaskShowsClaimThenStart(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")

	page := startPage(tree, task)
	require.NotNil(t, page.Start)
	require.Equal(t, "agent", page.Start.Actor)
	require.Equal(t, "claim and start", page.Start.Action)
	require.Equal(t, "tiller claim task "+*task.Key, page.Start.Command)
	require.Equal(t, "tiller start task "+*task.Key, page.Start.Then)

	command, _ := views.ClaimMove(task)
	require.Equal(t, command, page.Start.Command, "the node page and Up Next print one claim command")
}

func TestStartOfClaimedTaskOffersStartToItsOwnerOnly(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	task.Owner = "alice"

	start := views.StartOf(tree, task, "alice", workflow.Input{})
	require.NotNil(t, start)
	require.Equal(t, "start", start.Action)
	require.Equal(t, "tiller start task "+*task.Key, start.Command)
	require.Empty(t, start.Then)

	require.Nil(t, views.StartOf(tree, task, "bob", workflow.Input{}),
		"start refuses anyone but the owner, and Up Next offers it to nobody else")
	require.Nil(t, views.StartOf(tree, task, "", workflow.Input{}),
		"an unpersonalized reader holds no task, as on Up Next")
	require.Nil(t, startPage(tree, task).Start, "NodePageOf knows no reader")
}

func TestStartOfUnclaimedTaskIsOfferedToAnyReader(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")

	for _, reader := range []string{"", "bob"} {
		start := views.StartOf(tree, task, reader, workflow.Input{})
		require.NotNil(t, start, reader)
		require.Equal(t, "claim and start", start.Action)
	}
}

func TestNodePageTaskNotReadyOrHeldHasNoStart(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")

	task.Status = "in_progress"
	require.Nil(t, startPage(tree, task).Start)

	task.Status = "ready"
	holdClaimableNode(tree, "Q-aaaaa0", task.ID)
	require.Nil(t, startPage(tree, task).Start, "an open question holds the task")
}

func TestNodePageTaskUnderHeldFeatureHasNoStart(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	f.Blocked = true
	require.Nil(t, startPage(tree, task).Start)
}

func TestStartOfHonorsObservedEligibilityAndBranches(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")

	require.NotNil(t, views.StartOf(tree, task, "", workflow.Input{}))
	require.Nil(t, views.StartOf(tree, task, "", workflow.Input{Eligibility: map[string]string{f.ID: "basis drifted"}}),
		"an observed basis failure on the feature holds the claim")
	require.Nil(t, views.StartOf(tree, task, "", observedOn(task.ID, "t/x", "robin")),
		"work already in flight on a branch is not offered for claiming")

	task.Owner = "robin"
	require.NotNil(t, views.StartOf(tree, task, "robin", workflow.Input{}))
	require.Nil(t, views.StartOf(tree, task, "robin", workflow.Input{Eligibility: map[string]string{f.ID: "basis drifted"}}),
		"an observed basis failure on the feature holds the owner's start too")
	require.Nil(t, views.StartOf(tree, task, "robin", workflow.Input{Eligibility: map[string]string{task.ID: "basis drifted"}}),
		"an observed basis failure on the task holds its start")
}

func TestNodePageStartIsOmittedFromJSONWhenNil(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	_, task := readyFeatureWithTask(t, tree, "F-aaaaa0", "epic-0/work", "epic-0")
	raw, err := json.Marshal(startPage(tree, task))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"start":{"actor":"agent","action":"claim and start","command":"tiller claim task `)

	task.Status = "in_progress"
	raw, err = json.Marshal(startPage(tree, task))
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"start"`)
}
