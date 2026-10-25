package views_test

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func TestWorkItemsBoardPreservesShelfOrderAndOrdersClosedByTerminalEvent(t *testing.T) {
	tree := workItemsBoardTree()
	evs := []events.Event{
		{ID: "EV-cancel", Type: "cancel.chore", At: boardAt(10), Targets: []string{"CH-cancel"}},
		{ID: "EV-done", Type: "sync.bug", At: boardAt(11), Targets: []string{"B-done"}},
	}

	got := views.WorkItemsBoardOf(tree, evs)

	require.Equal(t, []string{"B-task", "B-feature", "CH-open"}, cardIDs(got.Open))
	require.Equal(t, []string{"B-done", "CH-cancel", "CH-eventless"}, cardIDs(got.Closed))
	require.Equal(t, "2026-08-29T11:00:00Z", got.Closed[0].ClosedAt)
	require.Equal(t, "cancelled", got.Closed[1].Status)
	require.Empty(t, got.Closed[2].ClosedAt)
	require.Equal(t, []string{"E-foundation", "F-console", "T-render"}, got.Open[0].AttachmentIDs)
	require.Equal(t, "Render the board", got.Open[0].Attached.Title)
	require.Equal(t, []string{"E-foundation", "F-console", "T-render"}, relatedIDs(got.Attachments))
}

func TestAttachedWorkRollsDescendantsAndOpenCountExcludesTerminalWork(t *testing.T) {
	tree := workItemsBoardTree()

	require.Equal(t,
		[]string{"B-task", "B-feature", "B-done"},
		relatedIDs(views.AttachedWorkOf(tree, "F-console")),
	)
	require.Equal(t, 2, views.OpenAttachedWorkCount(tree, "F-console"))
	require.Equal(t, []string{"B-task"}, relatedIDs(views.AttachedWorkOf(tree, "T-render")))
}

func workItemsBoardTree() *store.Tree {
	epicKey := "foundation"
	featureKey := "foundation/console"
	taskKey := "foundation/console/render"
	bugTaskKey := "bugs/z-task"
	bugFeatureKey := "bugs/a-feature"
	bugDoneKey := "bugs/m-done"
	choreOpenKey := "chores/z-open"
	choreCancelKey := "chores/a-cancel"
	choreEventlessKey := "chores/m-eventless"
	task := &store.Task{
		ID: "T-render", Key: &taskKey, Title: "Render the board", Status: "ready", FeatureID: "F-console",
	}

	return &store.Tree{
		Manifest: &store.Manifest{},
		Epics: map[string]*store.Epic{
			"E-foundation": {ID: "E-foundation", Key: &epicKey, Title: "Foundation", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-console": {ID: "F-console", Key: &featureKey, Title: "Console", Status: "ready", EpicKey: epicKey},
		},
		Tasks: map[string]*store.Task{
			"T-render": task,
		},
		FeatureTasks: map[string][]*store.Task{
			"F-console": {task},
		},
		Bugs: map[string]*store.Bug{
			"B-task":    {WorkItem: store.WorkItem{ID: "B-task", Key: &bugTaskKey, Title: "Task bug", Status: "ready", AttachedTo: "T-render"}},
			"B-feature": {WorkItem: store.WorkItem{ID: "B-feature", Key: &bugFeatureKey, Title: "Feature bug", Status: "ready", AttachedTo: "F-console"}},
			"B-done":    {WorkItem: store.WorkItem{ID: "B-done", Key: &bugDoneKey, Title: "Done bug", Status: "done", AttachedTo: "F-console"}},
		},
		Chores: map[string]*store.Chore{
			"CH-open":      {WorkItem: store.WorkItem{ID: "CH-open", Key: &choreOpenKey, Title: "Open chore", Status: "ready"}},
			"CH-cancel":    {WorkItem: store.WorkItem{ID: "CH-cancel", Key: &choreCancelKey, Title: "Cancelled chore", Status: "cancelled"}},
			"CH-eventless": {WorkItem: store.WorkItem{ID: "CH-eventless", Key: &choreEventlessKey, Title: "Eventless chore", Status: "done"}},
		},
		BugOrder:   store.ShelfOrder{Order: []string{"B-task", "B-feature", "B-done"}},
		ChoreOrder: store.ShelfOrder{Order: []string{"CH-open", "CH-cancel", "CH-eventless"}},
	}
}

func boardAt(hour int) time.Time {
	return time.Date(2026, 8, 29, hour, 0, 0, 0, time.UTC)
}

func cardIDs(cards []views.WorkItemCard) []string {
	ids := make([]string, len(cards))
	for i, card := range cards {
		ids[i] = card.ID
	}
	return ids
}

func relatedIDs(nodes []views.RelatedNode) []string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.ID
	}
	return ids
}

func TestTransferredShelfHistoryLinksBothEnds(t *testing.T) {
	tree := workItemsBoardTree()
	before, after := tree.Chores["CH-cancel"], tree.Chores["CH-open"]
	// Node-page relationship resolution uses the same canonical IDs as a loaded
	// snapshot; the board-only fixture's abbreviated labels are not valid IDs.
	delete(tree.Chores, before.ID)
	delete(tree.Chores, after.ID)
	before.ID, after.ID = "CH-aaaaaa", "CH-bbbbbb"
	tree.Chores[before.ID], tree.Chores[after.ID] = before, after
	tree.ChoreOrder.Order = []string{after.ID, before.ID, "CH-eventless"}
	before.Resolution = store.ResolutionTransferred
	before.Started = true
	before.SupersededBy = &after.ID
	after.Supersedes = []string{before.ID}
	board := views.WorkItemsBoardOf(tree, nil)
	var original *views.WorkItemCard
	for i := range board.Closed {
		if board.Closed[i].ID == before.ID {
			original = &board.Closed[i]
			break
		}
	}
	require.NotNil(t, original)
	require.Equal(t, "transferred", original.Resolution)
	require.Equal(t, after.ID, original.SupersededBy)
	oldPage := views.NodePageOf(tree, nil, before, nil)
	newPage := views.NodePageOf(tree, nil, after, nil)
	require.True(t, oldPage.Node.Started)
	require.False(t, newPage.Node.Started, "replacement claims do not inherit the start boundary")
	require.Equal(t, after.ID, oldPage.Node.SupersededBy)
	require.Equal(t, []string{before.ID}, newPage.Node.Supersedes)
	require.Equal(t, []string{after.ID}, relatedIDs(oldPage.Relationships.SupersededBy))
	require.Equal(t, []string{before.ID}, relatedIDs(newPage.Relationships.Supersedes))
	require.Equal(t, store.NounChore, oldPage.Relationships.SupersededBy[0].Noun)
}

func TestShelfTransfersCloseOnlyOriginalsInEventOrder(t *testing.T) {
	tree := workItemsBoardTree()
	tree.Bugs["B-feature"].Status = "cancelled"
	evs := []events.Event{
		{ID: "EV-bugold", Type: "sync.bug", At: boardAt(9), Targets: []string{"B-done"}},
		{ID: "EV-bugnew", Type: "transfer.bug", At: boardAt(10), Targets: []string{"B-feature", "B-task"}, After: map[string]any{"id": "B-feature", "status": "cancelled", "replacement_id": "B-task", "replacement_status": "ready"}},
		{ID: "EV-chore", Type: "transfer.chore", At: boardAt(11), Targets: []string{"CH-cancel", "CH-open"}, After: map[string]any{"id": "CH-cancel", "status": "cancelled", "replacement_id": "CH-open", "replacement_status": "ready"}},
	}
	board := views.WorkItemsBoardOf(tree, evs)
	require.Equal(t, []string{"B-task", "CH-open"}, cardIDs(board.Open))
	require.Equal(t, []string{"CH-cancel", "B-feature", "B-done", "CH-eventless"}, cardIDs(board.Closed))
	require.Equal(t, "2026-08-29T11:00:00Z", board.Closed[0].ClosedAt)
	require.Equal(t, "2026-08-29T10:00:00Z", board.Closed[1].ClosedAt)
	// A later terminal snapshot with unavailable history must not borrow the
	// earlier transfer's timestamp for its replacement.
	tree.Bugs["B-task"].Status = "cancelled"
	tree.Chores["CH-open"].Status = "cancelled"
	board = views.WorkItemsBoardOf(tree, evs)
	require.Equal(t, []string{"CH-cancel", "B-feature", "B-done"}, cardIDs(board.Closed[:3]))
	for _, card := range board.Closed[3:] {
		require.Empty(t, card.ClosedAt)
	}
}
