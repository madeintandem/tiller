package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func TestQueueCarriesShelfApprovalAndCancelledCarrierEntries(t *testing.T) {
	bugA, bugB := "bugs/alpha", "bugs/zulu"
	chore := "chores/bump-go"
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Bugs: map[string]*store.Bug{
			"B-alpha0": {WorkItem: store.WorkItem{ID: "B-alpha0", Key: &bugA, Title: "Alpha", Status: "proposed"}},
			"B-zulu00": {WorkItem: store.WorkItem{ID: "B-zulu00", Key: &bugB, Title: "Zulu", Status: "ready", ResolvedBy: "F-miss00"}},
		},
		Chores: map[string]*store.Chore{
			"CH-bump00": {WorkItem: store.WorkItem{ID: "CH-bump00", Key: &chore, Title: "Bump Go", Status: "proposed"}},
		},
		BugOrder: store.ShelfOrder{Order: []string{"B-zulu00", "B-alpha0"}},
	}

	entries := views.QueueOf(tree).Entries
	require.Equal(t, []string{"approve_bug", "approve_chore", "carrier_cancelled"}, queueEntryKinds(entries))
	require.Equal(t, "tiller approve bug bugs/alpha", entries[0].Command)
	require.Equal(t, "tiller approve chore chores/bump-go", entries[1].Command)
	require.Equal(t, "tiller resolve bug bugs/zulu --clear", entries[2].Command)
	require.Equal(t, gen.ActivityCarrierCancelled, gen.WorkActivity(tree, tree.Bugs["B-zulu00"]))
}

func TestQueueApprovalExcludesProposedShelfItemWithLiveCarrier(t *testing.T) {
	bugKey := "bugs/carried"
	carrierKey := "work/carrier"
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Features: map[string]*store.Feature{
			"F-live000": {ID: "F-live000", Key: &carrierKey, Status: "active"},
		},
		Bugs: map[string]*store.Bug{
			"B-carried": {WorkItem: store.WorkItem{
				ID: "B-carried", Key: &bugKey, Title: "Carried bug",
				Status: "proposed", ResolvedBy: "F-live000",
			}},
		},
	}

	require.Equal(t, gen.ActivityCarried, gen.WorkActivity(tree, tree.Bugs["B-carried"]))
	require.Empty(t, entriesOfKind(views.QueueOf(tree), "approve_bug"))
}

func TestQueueAttestGateIncludesAnInReviewBug(t *testing.T) {
	key := "bugs/blank-on-reload"
	tree := &store.Tree{
		Manifest: &store.Manifest{Gates: store.GatesConfig{Optional: map[string]store.GateSpec{
			"code_review": {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
		}}},
		Bugs: map[string]*store.Bug{
			"B-aaaaaa": {WorkItem: store.WorkItem{ID: "B-aaaaaa", Key: &key, Title: "Blank on reload", Status: "in_review"}},
		},
	}

	got := entriesOfKind(views.QueueOf(tree), "attest_gate")
	require.Len(t, got, 1)
	require.Equal(t, store.NounBug, got[0].Noun)
	require.Equal(t, "tiller attest bug bugs/blank-on-reload --gate code_review --status pass", got[0].Command)
}
