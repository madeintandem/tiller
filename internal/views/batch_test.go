package views_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// batchTree holds one batch jc owns over a ready bug and a proposed chore,
// beside a ready bug jc owns that is in no batch.
func batchTree() *store.Tree {
	batchKey, alphaKey, gammaKey, aloneKey := "batches/core", "bugs/alpha", "chores/gamma", "bugs/alone"
	return &store.Tree{
		Manifest: &store.Manifest{Gates: store.GatesConfig{Optional: map[string]store.GateSpec{
			"code_review": {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
		}}},
		Batches: map[string]*store.Batch{
			"BT-aaaaaa": {WorkItem: store.WorkItem{ID: "BT-aaaaaa", Key: &batchKey, Title: "Core fixes", Status: "ready", Owner: "jc"}},
		},
		Bugs: map[string]*store.Bug{
			"B-aaaaaa": {WorkItem: store.WorkItem{ID: "B-aaaaaa", Key: &alphaKey, Title: "Alpha", Status: "ready", Severity: "important", BatchID: "BT-aaaaaa"}},
			"B-bbbbbb": {WorkItem: store.WorkItem{ID: "B-bbbbbb", Key: &aloneKey, Title: "Alone", Status: "ready", Owner: "jc"}},
		},
		Chores: map[string]*store.Chore{
			"CH-aaaaaa": {WorkItem: store.WorkItem{ID: "CH-aaaaaa", Key: &gammaKey, Title: "Gamma", Status: "proposed", BatchID: "BT-aaaaaa"}},
		},
	}
}

func startBatch(tree *store.Tree, status string) *store.Batch {
	batch := tree.Batches["BT-aaaaaa"]
	batch.Status, batch.Started, batch.Branch = status, true, "tiller/batches/core"
	tree.Chores["CH-aaaaaa"].Status = "ready"
	return batch
}

func TestBatchedPhaseFollowsTheBatch(t *testing.T) {
	tree := batchTree()
	member := tree.Bugs["B-aaaaaa"]
	require.Equal(t, gen.ActivityBatched, gen.WorkActivity(tree, member))
	require.True(t, gen.WorkLeftQueue(tree, member))
	require.Empty(t, gen.WorkActivity(tree, tree.Bugs["B-bbbbbb"]))
	require.Empty(t, gen.WorkActivity(tree, tree.Batches["BT-aaaaaa"]), "a batch is never itself batched")

	tree.Batches["BT-aaaaaa"].Status = "done"
	require.Equal(t, gen.ActivityBatched, gen.WorkActivity(tree, member), "a done batch is the crossing sync is about to record")
	tree.Batches["BT-aaaaaa"].Status = "cancelled"
	require.Empty(t, gen.WorkActivity(tree, member), "a cancelled batch reviews nothing")
}

func TestNextOffersABatchAsOneUnitAndNeverItsMembers(t *testing.T) {
	tree := batchTree()
	// With a member still proposed the batch cannot start, so the move is the
	// human's triage of that member and the batch is not offered as work.
	waiting := nextFor(tree, "jc", workflow.Input{})
	require.Empty(t, candidatesFor(waiting.Candidates, "BT-aaaaaa"), "start batch would refuse batch_member_unready")
	triage := candidatesFor(waiting.Candidates, "CH-aaaaaa")
	require.Len(t, triage, 1)
	require.Equal(t, "human", triage[0].Actor)

	tree.Chores["CH-aaaaaa"].Status = "ready"
	next := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, []string{"BT-aaaaaa", "B-bbbbbb"}, candidateNodeIDs(candidatesOfKind(next.Candidates, "continue")))
	require.Empty(t, candidatesFor(next.Candidates, "B-aaaaaa"))
	top := next.Candidates[0]
	require.Equal(t, store.NounBatch, top.Node.Noun)
	require.Equal(t, "execute", top.Skill)
	require.Equal(t, "your batch is claimed and ready to start", top.Reason)

	startBatch(tree, "in_review")
	wait := candidatesFor(nextFor(tree, "jc", workflow.Input{}).Candidates, "BT-aaaaaa")
	require.Len(t, wait, 1)
	require.Equal(t, "merge_task", wait[0].Kind)
	require.Equal(t, "human", wait[0].Actor)
	require.Empty(t, candidatesFor(nextFor(tree, "jc", workflow.Input{}).Candidates, "CH-aaaaaa"))
}

func TestReadyQueueLineCountsABatchOnceAndNotItsMembers(t *testing.T) {
	tree := batchTree()
	require.Equal(t, "queue: 1 bug, 0 chores, 0 tweaks ready", views.ReadyShelfQueueLine(tree), "a batch with an unapproved member is not ready to start")
	tree.Chores["CH-aaaaaa"].Status = "ready"
	require.Equal(t, "queue: 1 bug, 0 chores, 0 tweaks, 1 batch ready", views.ReadyShelfQueueLine(tree))
	startBatch(tree, "in_progress")
	require.Equal(t, "queue: 1 bug, 0 chores, 0 tweaks ready", views.ReadyShelfQueueLine(tree))
}

func TestQueueStillAsksForAProposedMembersApproval(t *testing.T) {
	entries := views.QueueOf(batchTree()).Entries
	var approvals []string
	for _, e := range entries {
		if e.Kind == "approve_chore" {
			approvals = append(approvals, e.ID)
		}
	}
	require.Equal(t, []string{"CH-aaaaaa"}, approvals, "start batch waits on it")
}

func TestQueueAttestGateNamesTheBatchNotItsMembers(t *testing.T) {
	tree := batchTree()
	startBatch(tree, "in_review")
	gates := queueGatesOnly(views.QueueOf(tree).Entries)
	require.Len(t, gates, 1)
	require.Equal(t, store.NounBatch, gates[0].Noun)
	require.Equal(t, "tiller attest batch batches/core --gate code_review --status <pass|fail>", gates[0].Command)
}

func TestExecuteReadoutListsABatchAsOneEntity(t *testing.T) {
	tree := batchTree()
	startBatch(tree, "in_progress")
	r := views.ReadoutOfInput(tree, views.MomentExecute, views.ReadoutInput{Contributor: "jc"})
	require.Len(t, r.Entities, 1)
	require.Contains(t, r.Entities[0].Ref, "BT-aaaaaa")
	require.Equal(t, "0 of 1", checkLabels(r)["Gates green with evidence"])
}

func TestMemberViewsNameTheBatchAndBatchViewsListTheMembers(t *testing.T) {
	tree := batchTree()
	pr := 77
	batch := startBatch(tree, "in_review")
	batch.PR = &pr
	member := tree.Bugs["B-aaaaaa"]

	status := views.NodeStatus(tree, member, workflow.Input{})
	require.Equal(t, "ready", status.Status, "the stored status is the member's own")
	require.Equal(t, gen.ActivityBatched, status.Activity)
	require.Empty(t, status.LegalNext, "the batch owns start, return closed when it started, and cancel closed when it went to review")
	require.NotNil(t, status.Batch)
	require.Equal(t, "BT-aaaaaa", status.Batch.ID)
	require.Equal(t, "in_review", status.Batch.Status)
	require.Equal(t, "tiller/batches/core", status.Batch.Branch)
	require.Equal(t, 77, *status.Batch.PR)

	shown := views.Node(tree, member)
	require.Equal(t, "BT-aaaaaa", shown.Batch.ID)
	require.Empty(t, shown.NextOwner, "nothing moves the member itself while its batch is in review")

	batch.Status = "in_progress"
	require.Equal(t, []string{"cancel"}, views.NodeStatus(tree, member, workflow.Input{}).LegalNext)
	require.Equal(t, "human", views.Node(tree, member).NextOwner)
	batch.Status = "in_review"
	require.Equal(t, "BT-aaaaaa", views.Item(tree, member).Batch)

	onBatch := views.NodeStatus(tree, batch, workflow.Input{})
	require.Nil(t, onBatch.Batch)
	require.Equal(t, "Core fixes", onBatch.Title)
	require.Equal(t, "important", onBatch.Severity, "a batch displays its highest member severity")
	require.Len(t, onBatch.Members, 2)
	require.Equal(t, "B-aaaaaa", onBatch.Members[0].ID)
	require.Equal(t, "ready", onBatch.Members[0].Status)
	require.Equal(t, 2, views.Item(tree, batch).Members)
	require.Len(t, views.Node(tree, batch).Members, 2)

	alone := views.NodeStatus(tree, tree.Bugs["B-bbbbbb"], workflow.Input{})
	require.Nil(t, alone.Batch)
	require.Contains(t, alone.LegalNext, "start")
}

func TestWhoseMoveCountsABatchOnceAndNotItsMembers(t *testing.T) {
	tree := batchTree()
	// The batch and the lone bug are the agent's to start. The ready member
	// adds nothing, and the proposed one still waits on the human's triage.
	require.Equal(t, views.WhoseMoveCounts{Agent: 2, Human: 1}, views.WhoseMoveOf(tree))
}

func TestCollectListsBatches(t *testing.T) {
	tree := batchTree()
	nodes := views.Collect(tree, store.NounBatch)
	require.Len(t, nodes, 1)
	require.True(t, views.Matches(tree, nodes[0], views.ListFilters{Status: "unfinished"}))
}

// The Fix board draws a batch as one card listing its members, so the board
// carries every batch with its member rows, in key order, and each member's
// own card still names the batch by id.
func TestWorkItemsBoardCarriesBatchesWithTheirMembers(t *testing.T) {
	tree := batchTree()
	doneKey := "batches/done"
	tree.Batches["BT-bbbbbb"] = &store.Batch{WorkItem: store.WorkItem{ID: "BT-bbbbbb", Key: &doneKey, Title: "Done fixes", Status: "done"}}
	evs := []events.Event{{ID: "EV-add", Type: "add.batch", At: time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC), Targets: []string{"BT-aaaaaa"}}}

	board := views.WorkItemsBoardOf(tree, evs)

	require.Len(t, board.Batches, 2)
	core := board.Batches[0]
	require.Equal(t, "BT-aaaaaa", core.ID)
	require.Equal(t, store.NounBatch, core.Noun)
	require.Equal(t, "batches/core (BT-aaaaaa)", core.Ref)
	require.Equal(t, "Core fixes", core.Title)
	require.Equal(t, "ready", core.Status)
	require.Equal(t, "jc", core.Owner)
	require.Equal(t, "important", core.Severity, "a batch shows its highest member severity")
	require.Equal(t, 2, core.Members)
	require.Equal(t, "EV-add", core.AddedEvent)
	require.NotEmpty(t, core.AddedAt)
	require.Len(t, core.MemberList, 2)
	require.Equal(t, "B-aaaaaa", core.MemberList[0].ID)
	require.Equal(t, "ready", core.MemberList[0].Status)
	require.Equal(t, "proposed", core.MemberList[1].Status)
	require.Equal(t, "BT-bbbbbb", board.Batches[1].ID)
	require.Empty(t, board.Batches[1].MemberList)

	raw, err := json.Marshal(views.WorkItemsBoardOf(&store.Tree{}, nil))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"batches":[]`, "the board's batch list is always an array")
}

// History and Just happened name an event's target by its title, and a batch
// has one.
func TestRecapTargetCarriesABatchTitle(t *testing.T) {
	at := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	evs := []events.Event{{ID: "EV-add", Type: "add.batch", At: at, Contributor: "jc", Targets: []string{"BT-aaaaaa"}}}

	recap := views.RecapOf(batchTree(), evs, at.Add(-time.Hour), "24h", "jc")

	require.Len(t, recap.Groups, 1)
	target := recap.Groups[0].Entries[0].Targets[0]
	require.Equal(t, store.NounBatch, target.Noun)
	require.Equal(t, "Core fixes", target.Title)
}

// A related row, such as a feature's Fix tab, draws a member with the phase
// its batch gives it, so it never reads as work waiting to start.
func TestRelatedRowCarriesTheBatchedPhase(t *testing.T) {
	tree := batchTree()
	tree.Bugs["B-aaaaaa"].AttachedTo = "F-aaaaaa"
	tree.Bugs["B-bbbbbb"].AttachedTo = "F-aaaaaa"

	rows := views.AttachedWorkOf(tree, "F-aaaaaa")

	require.Len(t, rows, 2)
	byID := map[string]views.RelatedNode{rows[0].ID: rows[0], rows[1].ID: rows[1]}
	require.Equal(t, gen.ActivityBatched, byID["B-aaaaaa"].Activity)
	require.Empty(t, byID["B-bbbbbb"].Activity)
}
