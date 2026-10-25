package relationships_test

import (
	"context"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func amendmentTree() *store.Tree {
	epicKey, featureKey, amendmentKey := "work", "work/reused", "change"
	return &store.Tree{
		Epics:    map[string]*store.Epic{"E-111111": {ID: "E-111111", Key: &epicKey}},
		Features: map[string]*store.Feature{"F-222222": {ID: "F-222222", Key: &featureKey, EpicKey: epicKey}},
		Amendments: map[string]*store.Amendment{"A-333333": {
			ID: "A-333333", Key: &amendmentKey, Status: store.AmendmentStatusProposed,
		}},
	}
}

func amendmentEvent(id, typ string, second int, targets []string, after map[string]any) events.Event {
	return events.Event{ID: id, Type: typ, At: time.Date(2026, 9, 19, 12, 0, second, 0, time.UTC), Targets: targets, After: after}
}

func TestAmendmentHistoricalKeyNeverUsesCurrentHolder(t *testing.T) {
	tree := amendmentTree()
	a := tree.Amendments["A-333333"]
	a.Operations = []store.AmendmentOp{{Op: store.AmendmentOpRevise, Target: "work/reused"}}
	index := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, index, nil, "")
	require.False(t, index.Complete)
	require.Empty(t, index.Amendments.Relations["F-222222"])
	edges := index.Adjacent(a.ID, "amends")
	require.Len(t, edges, 1, "the unresolved assertion must remain inspectable")
	endpoint := index.Vertices[edges[0].To]
	require.True(t, endpoint.Unresolved)
	require.Equal(t, "work/reused", endpoint.Raw)
	require.Nil(t, endpoint.Destination)
	require.Equal(t, 1, edges[0].Sources[0].Operation)
	require.Equal(t, "target", edges[0].Sources[0].Role)
	index.Finalize()
	result, err := relationships.Query(context.Background(), index, relationships.QueryInput{From: []string{endpoint.ID}, Follow: []string{"amended_by"}, Depth: 0})
	require.NoError(t, err)
	var endpointWarnings []relationships.Diagnostic
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "unresolved_amendment_endpoint" {
			endpointWarnings = append(endpointWarnings, diagnostic)
		}
	}
	require.Len(t, endpointWarnings, 1)
	require.ElementsMatch(t, []string{a.ID, endpoint.ID}, endpointWarnings[0].Subjects)
}

// stagedEpicAmendment stages a new epic, a feature under it, and a question
// blocking that feature, with staging history and no application.
func stagedEpicAmendment(status string) (*store.Tree, []events.Event) {
	tree := amendmentTree()
	a := tree.Amendments["A-333333"]
	a.Status = status
	a.Operations = []store.AmendmentOp{
		{Op: store.AmendmentOpAddEpic, Key: "fresh"},
		{Op: store.AmendmentOpAddFeature, Key: "fresh/thing"},
		{Op: store.AmendmentOpAddQuestion, Statement: "Which layout?", Blocks: []string{"fresh/thing"}},
	}
	evs := []events.Event{amendmentEvent("EV-111111", "amendment_opened", 1, []string{a.ID}, nil)}
	for i, op := range a.Operations {
		evs = append(evs, amendmentEvent("EV-22222"+string(rune('0'+i)), "amendment_staged", 2+i, []string{a.ID}, map[string]any{"index": i, "op": op.Op, "key": op.Key}))
	}
	return tree, evs
}

func unresolvedEndpointDiagnostics(index *relationships.Index) []relationships.Diagnostic {
	var out []relationships.Diagnostic
	for _, diagnostic := range index.Diagnostics {
		if diagnostic.Code == "unresolved_amendment_endpoint" {
			out = append(out, diagnostic)
		}
	}
	return out
}

func TestProposedAmendmentReferencesToItsOwnStagedKeysAreNotDiagnosed(t *testing.T) {
	tree, evs := stagedEpicAmendment(store.AmendmentStatusProposed)
	index := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, index, evs, "")
	require.Empty(t, unresolvedEndpointDiagnostics(index), "a staged key is expected to be unmaterialized before approval")
	require.True(t, index.Complete)
	parent := index.Amendments.Operations["A-333333"][1].Parent
	require.Equal(t, "fresh", parent.Raw)
	require.Empty(t, parent.ID, "a pending endpoint never fabricates an identity")
}

func TestApprovedAmendmentStagedKeyWithoutCreationIsStillDiagnosed(t *testing.T) {
	tree, evs := stagedEpicAmendment(store.AmendmentStatusApproved)
	index := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, index, evs, "")
	require.NotEmpty(t, unresolvedEndpointDiagnostics(index))
}

func TestAmendmentLegacyParentTracksRemovalAndReuse(t *testing.T) {
	tree := amendmentTree()
	a := tree.Amendments["A-333333"]
	a.Operations = []store.AmendmentOp{{Op: store.AmendmentOpAddTask, Key: "work/reused/new-task"}}
	evs := []events.Event{
		amendmentEvent("EV-111111", "add.feature", 1, []string{"F-aaaaaa"}, map[string]any{"key": "work/reused"}),
		amendmentEvent("EV-222222", "amendment_opened", 2, []string{a.ID}, nil),
		amendmentEvent("EV-333333", "amendment_staged", 3, []string{a.ID}, map[string]any{"index": 0, "op": "add-task", "key": a.Operations[0].Key}),
		amendmentEvent("EV-444444", "remove.feature", 4, []string{"F-aaaaaa"}, nil),
		amendmentEvent("EV-555555", "add.feature", 5, []string{"F-222222"}, map[string]any{"key": "work/reused"}),
	}
	evs[3].Before = map[string]any{"removed": []map[string]string{{"id": "F-aaaaaa", "key": "work/reused"}}}
	index := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, index, evs, "")
	parent := index.Amendments.Operations[a.ID][0].Parent
	require.Equal(t, "F-aaaaaa", parent.ID)
	require.True(t, parent.Unresolved)
	require.Empty(t, index.Amendments.Relations["F-222222"])
	edges := index.Adjacent(a.ID, "amends")
	require.Len(t, edges, 1)
	require.Equal(t, "F-aaaaaa", edges[0].To)
	require.Contains(t, edges[0].Sources[0].Evidence, "EV-333333")
	require.Contains(t, edges[0].Sources[0].Evidence, "EV-111111")
}

func TestAmendmentEqualTimeParentEvidenceStaysUnresolved(t *testing.T) {
	tree := amendmentTree()
	a := tree.Amendments["A-333333"]
	a.Operations = []store.AmendmentOp{{Op: store.AmendmentOpAddTask, Key: "work/reused/new-task"}}
	evs := []events.Event{
		amendmentEvent("EV-111111", "add.feature", 1, []string{"F-222222"}, map[string]any{"key": "work/reused"}),
		amendmentEvent("EV-222222", "amendment_opened", 1, []string{a.ID}, nil),
		amendmentEvent("EV-333333", "amendment_staged", 1, []string{a.ID}, map[string]any{"index": 0, "op": "add-task", "key": a.Operations[0].Key}),
	}
	index := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, index, evs, "")
	require.Empty(t, index.Amendments.Operations[a.ID][0].Parent.ID)
	require.Empty(t, index.Amendments.Relations["F-222222"])
	require.False(t, index.Complete)
}

func TestAmendmentProposalAndApplicationEvidenceRemainDistinct(t *testing.T) {
	tree := amendmentTree()
	a := tree.Amendments["A-333333"]
	a.Operations = []store.AmendmentOp{{Op: store.AmendmentOpSupersede, Target: "F-222222", By: "F-444444"}}
	tree.Features["F-444444"] = &store.Feature{ID: "F-444444", EpicKey: "work"}
	evs := []events.Event{
		amendmentEvent("EV-111111", "amendment_opened", 1, []string{a.ID}, nil),
		amendmentEvent("EV-222222", "amendment_staged", 2, []string{a.ID}, map[string]any{"index": 0, "op": "supersede"}),
	}
	index := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, index, evs, "")
	require.Empty(t, index.Adjacent("F-444444", "supersedes"), "a replacement proposal is not applied lineage")
	for _, edge := range index.Adjacent(a.ID, "amends") {
		for _, source := range edge.Sources {
			require.Equal(t, "proposed", source.State)
			require.Equal(t, 1, source.Operation)
			require.Contains(t, source.Evidence, "EV-222222")
		}
	}
	require.Len(t, index.Amendments.Relations["E-111111"], 1, "ancestor counts count amendments, not endpoints")
	for _, effect := range index.Amendments.Relations["E-111111"][0].Effects {
		require.Equal(t, "descendant", effect.Scope)
	}

	evs = append(evs, amendmentEvent("EV-333333", "superseded", 3, []string{"F-222222"}, map[string]any{"amendment": a.ID, "operation_index": 0, "superseded_by": "F-444444"}))
	applied := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, applied, evs, "")
	for _, edge := range applied.Adjacent(a.ID, "amends") {
		for _, source := range edge.Sources {
			require.Equal(t, "applied", source.State, "interrupted application can leave the amendment proposed")
			require.Contains(t, source.Evidence, "EV-333333")
		}
	}
}

func TestAmendmentCorrectEvalJoinsItsApplicationEvent(t *testing.T) {
	tree := amendmentTree()
	a := tree.Amendments["A-333333"]
	a.Operations = []store.AmendmentOp{{Op: store.AmendmentOpCorrectEval, Target: "F-222222", Reason: "the binding moved"}}
	evs := []events.Event{
		amendmentEvent("EV-111111", "amendment_opened", 1, []string{a.ID}, nil),
		amendmentEvent("EV-222222", "amendment_staged", 2, []string{a.ID}, map[string]any{"index": 0, "op": "correct-eval"}),
		amendmentEvent("EV-333333", "correct-eval.feature", 3, []string{"F-222222", a.ID}, map[string]any{"amendment": a.ID, "operation_index": 0, "eval": "git:bbb"}),
	}
	index := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, index, evs, "")
	for _, edge := range index.Adjacent(a.ID, "amends") {
		if edge.To != "F-222222" {
			continue
		}
		require.Len(t, edge.Sources, 1)
		require.Equal(t, "applied", edge.Sources[0].State)
		require.Equal(t, 1, edge.Sources[0].Operation)
		require.Contains(t, edge.Sources[0].Evidence, "EV-333333")
		return
	}
	t.Fatal("the corrected feature has no amends edge")
}

// A correct-eval event whose operation was hand-removed still surfaces its
// target as unjoined applied evidence, with the warning, like every other kind.
func TestAmendmentUnjoinedCorrectEvalEventRemainsVisible(t *testing.T) {
	tree := amendmentTree()
	a := tree.Amendments["A-333333"]
	evs := []events.Event{
		amendmentEvent("EV-111111", "correct-eval.feature", 1, []string{"F-222222", a.ID}, map[string]any{"amendment": a.ID, "operation_index": 0}),
	}
	index := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, index, evs, "")
	require.False(t, index.Complete, "an unjoined application event warns")
	require.Len(t, index.Amendments.Relations["F-222222"], 1)
	effects := index.Amendments.Relations["F-222222"][0].Effects
	require.Len(t, effects, 1)
	require.Equal(t, store.AmendmentOpCorrectEval, effects[0].Operation)
	require.Equal(t, "applied", effects[0].State)
	for _, edge := range index.Adjacent(a.ID, "amends") {
		if edge.To != "F-222222" {
			continue
		}
		require.Len(t, edge.Sources, 1)
		require.Zero(t, edge.Sources[0].Operation)
		require.Equal(t, "applied", edge.Sources[0].State)
		require.Equal(t, []string{"EV-111111"}, edge.Sources[0].Evidence)
		return
	}
	t.Fatal("the orphaned correction's target disappeared")
}

func TestAmendmentUnjoinedEvidenceAndHistoryFailureRemainVisible(t *testing.T) {
	tree := amendmentTree()
	a := tree.Amendments["A-333333"]
	evs := []events.Event{
		amendmentEvent("EV-111111", "revise.feature", 1, []string{"F-222222"}, map[string]any{"amendment": a.ID}),
		amendmentEvent("EV-222222", "revise.feature", 2, []string{"F-222222"}, map[string]any{"amendment": a.ID}),
	}
	index := relationships.BuildStructural(tree)
	relationships.AddAmendments(tree, index, evs, "history unreadable")
	require.False(t, index.Complete)
	require.Contains(t, index.Amendments.Warning, "history unreadable")
	require.Len(t, index.Amendments.Relations["F-222222"], 1)
	for _, edge := range index.Adjacent(a.ID, "amends") {
		if edge.To != "F-222222" {
			continue
		}
		require.Len(t, edge.Sources, 1)
		require.Zero(t, edge.Sources[0].Operation, "unjoined application must not invent an operation number")
		require.Equal(t, "applied", edge.Sources[0].State)
		require.ElementsMatch(t, []string{"EV-111111", "EV-222222"}, edge.Sources[0].Evidence)
		return
	}
	t.Fatal("known application endpoint disappeared")
}

func TestAmendmentMoveRetainsBothParentIdentities(t *testing.T) {
	tree := amendmentTree()
	a := tree.Amendments["A-333333"]
	a.Status = store.AmendmentStatusApproved
	newKey, destinationKey := "delivery/reused", "delivery"
	tree.Features["F-222222"].Key = &newKey
	tree.Features["F-222222"].EpicKey = destinationKey
	tree.Epics["E-444444"] = &store.Epic{ID: "E-444444", Key: &destinationKey}
	a.Operations = []store.AmendmentOp{{Op: store.AmendmentOpMoveFeature, Target: "F-222222", Key: newKey, Move: &store.FeatureMovePreconditions{SourceKey: "work/reused", SourceEpicID: "E-111111", DestinationEpicID: "E-444444", DestinationEpicKey: destinationKey}}}
	evs := []events.Event{
		amendmentEvent("EV-111111", "amendment_opened", 1, []string{a.ID}, nil),
		amendmentEvent("EV-222222", "amendment_staged", 2, []string{a.ID}, map[string]any{"index": 0, "op": "move-feature", "key": newKey}),
		amendmentEvent("EV-333333", "move.feature", 3, []string{"F-222222", "E-111111", "E-444444"}, map[string]any{"amendment": a.ID, "operation_index": 0, "key": newKey, "epic": "E-444444"}),
		amendmentEvent("EV-444444", "amendment_approved", 4, []string{a.ID}, nil),
	}
	index := relationships.AmendmentIdentities(tree, evs)
	require.Empty(t, index.Warning)
	roles := map[string]string{}
	for _, effect := range index.Effects[a.ID] {
		roles[effect.Role] = effect.Node.ID
		require.Equal(t, "applied", effect.State)
	}
	require.Equal(t, map[string]string{"target": "F-222222", "move_source_parent": "E-111111", "move_destination_parent": "E-444444"}, roles)
}
