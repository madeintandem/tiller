package views_test

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func TestTweakBoardOrdersLifecycleAndRollsUpAttachments(t *testing.T) {
	tree := workItemsBoardTree()
	readyKey, oldKey, newKey := "tweaks/ready", "tweaks/old", "tweaks/new"
	tree.Tweaks = map[string]*store.Tweak{
		"TW-aaaaaa": {WorkItem: store.WorkItem{ID: "TW-aaaaaa", Key: &readyKey, Title: "Tune rendering", Status: "check", AttachedTo: "T-render", Owner: "jc"}},
		"TW-bbbbbb": {WorkItem: store.WorkItem{ID: "TW-bbbbbb", Key: &oldKey, Status: "cancelled", Resolution: store.ResolutionTransferred}},
		"TW-cccccc": {WorkItem: store.WorkItem{ID: "TW-cccccc", Key: &newKey, Status: "ready"}},
	}
	tree.TweakOrder.Order = []string{"TW-cccccc", "TW-aaaaaa", "TW-bbbbbb"}
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	evs := []events.Event{{ID: "EV-transfer", Type: "transfer.tweak", At: at, Targets: []string{"TW-bbbbbb", "TW-cccccc"}, After: map[string]any{"id": "TW-bbbbbb"}}}
	board := views.WorkItemsBoardOf(tree, evs)
	require.Equal(t, []string{"TW-cccccc", "TW-aaaaaa"}, cardIDs(board.Open[len(board.Open)-2:]))
	require.Equal(t, "TW-bbbbbb", board.Closed[0].ID)
	require.Equal(t, "2026-09-01T12:00:00Z", board.Closed[0].ClosedAt)
	require.Empty(t, board.Open[len(board.Open)-2].ClosedAt, "a replacement is not closed by its transfer event")
	require.Contains(t, board.Open[len(board.Open)-1].AttachmentIDs, "F-console")
	require.Contains(t, board.Open[len(board.Open)-1].AttachmentIDs, "E-foundation")
	require.Equal(t, 3, views.OpenAttachedWorkCount(tree, "F-console"))
	require.Contains(t, relatedIDs(views.AttachedWorkOf(tree, "F-console")), "TW-aaaaaa")
	for _, kind := range []string{"sync.tweak", "cancel.tweak"} {
		t.Run(kind, func(t *testing.T) {
			evs = append(evs[:1:1], events.Event{ID: "EV-terminal", Type: kind, At: at.Add(time.Hour), Targets: []string{"TW-bbbbbb"}})
			require.Equal(t, "2026-09-01T13:00:00Z", views.WorkItemsBoardOf(tree, evs).Closed[0].ClosedAt)
		})
	}
}

func TestTweakListMapNodeAndQueueShareWorkItemBehavior(t *testing.T) {
	tree := shelfRouteTree()
	key := "tweaks/tune"
	work := &store.Tweak{WorkItem: store.WorkItem{ID: "TW-aaaaaa", Key: &key, Title: "Tune rendering", Status: "proposed", Owner: "jc", AttachedTo: "F-aaaaaa"}}
	tree.Tweaks = map[string]*store.Tweak{work.ID: work}
	require.Equal(t, []store.Node{work}, views.Collect(tree, store.NounTweak))
	require.True(t, views.Matches(tree, work, views.ListFilters{Status: "unfinished", Owner: "jc"}))
	page := views.NodePageOf(tree, nil, work, nil)
	require.Equal(t, store.NounTweak, page.Node.Noun)
	require.Equal(t, work.Title, page.Node.Title)
	require.NotNil(t, page.Relationships.AttachedTo)
	require.Equal(t, "F-aaaaaa", page.Relationships.AttachedTo.ID)
	mapped := views.MapOf(tree, nil)
	require.Equal(t, work.ID, mapped.Nodes[len(mapped.Nodes)-1].ID)
	require.Contains(t, mapped.Edges, views.MapEdge{From: work.ID, To: "F-aaaaaa", Kind: views.MapEdgeAttachedTo})
	require.Equal(t, 1, views.Summary(tree).Counts.Tweaks)
	approvals := entriesOfKind(views.QueueOf(tree), "approve_tweak")
	require.Len(t, approvals, 1)
	require.Equal(t, "tiller approve tweak tweaks/tune", approvals[0].Command)
	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc"})
	require.Contains(t, r.NeedsYou, views.ReadoutAction{Text: "1 tweak awaits triage", Command: "tiller queue --kind approve_tweak"})
	work.Status = "done"
	require.False(t, views.Matches(tree, work, views.ListFilters{Status: "unfinished"}))
	work.Status = "ready"
	require.Equal(t, "queue: 1 bug, 1 chore, 1 tweak ready", views.ReadyShelfQueueLine(tree))
	work.ResolvedBy = "F-aaaaaa"
	require.Equal(t, "queue: 1 bug, 1 chore, 0 tweaks ready", views.ReadyShelfQueueLine(tree))
}

func TestTweakReviewAndCarrierAttention(t *testing.T) {
	key := "tweaks/tune"
	work := &store.Tweak{WorkItem: store.WorkItem{ID: "TW-aaaaaa", Key: &key, Title: "Tune rendering", Status: "in_review", Owner: "jc"}}
	tree := &store.Tree{
		Manifest: &store.Manifest{Gates: store.GatesConfig{Optional: map[string]store.GateSpec{
			"code_review": {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
		}}},
		Tweaks: map[string]*store.Tweak{work.ID: work},
	}
	gates := entriesOfKind(views.QueueOf(tree), "attest_gate")
	require.Len(t, gates, 1)
	require.Equal(t, store.NounTweak, gates[0].Noun)
	require.Equal(t, "tiller attest tweak tweaks/tune --gate code_review --status pass", gates[0].Command)
	// Review still has agent legal moves (resume/sync), while the queue above
	// separately records the human attestation waiting on this same node.
	require.Equal(t, views.WhoseMoveCounts{Agent: 1}, views.WhoseMoveOf(tree))
	work.Status, work.ResolvedBy = "ready", "F-missing"
	returned := entriesOfKind(views.QueueOf(tree), "carrier_cancelled")
	require.Len(t, returned, 1)
	require.Equal(t, "tiller resolve tweak tweaks/tune --clear", returned[0].Command)
	require.Equal(t, "carrier_cancelled", views.Node(tree, work).Activity)
}
