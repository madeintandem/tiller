package relationships_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestProvenanceSeparatesExternalPathsAndOpaqueCitations(t *testing.T) {
	tree := contractTree()
	tree.Features["F-79dh3q"].Source = "design/origin.md#L7-12"
	tree.Epics["E-j8m4q2"].Source = "Discussed docs/origin.md in the meeting"
	tree.Baseline = store.BaselineLedger{Docs: []store.BaselineDoc{{Path: "origin.md", Describes: []string{"internal/store", "../legacy/internal/store"}, DerivedFrom: []store.RevisionRef{{Kind: store.KindGitBlob, Path: "docs/input.md", Value: "git:1111111"}}}}}
	tree.Manifest = &store.Manifest{Inputs: store.InputsBlock{Code: []store.CodeRoot{{Root: "../legacy", Mode: store.CodeModeReference}}}}
	key := "reshape"
	tree.Amendments = map[string]*store.Amendment{"A-aaaaaa": {ID: "A-aaaaaa", Key: &key, Status: store.AmendmentStatusProposed, Implements: "ADR 14", TriggeredBy: []string{"design/origin.md"}, Source: &store.AmendmentSource{Path: ".tiller/feedback/request.md", Revision: "git:2222222"}, Operations: []store.AmendmentOp{{Op: store.AmendmentOpRevise, Target: "F-79dh3q", Source: ".tiller/feedback/request.md#L3-L4"}}}}
	idx := relationships.BuildStructural(tree)
	require.Contains(t, idx.Vertices, "path:internal/store")
	require.Contains(t, idx.Vertices, "external_path:../legacy/internal/store")
	require.Nil(t, idx.Vertices["external_path:../legacy/internal/store"].Destination)
	require.NotContains(t, idx.Vertices, "doc:docs/origin.md")
	var citation, opaque, revision, operation bool
	for _, edge := range idx.Edges {
		if edge.Kind == "sourced_from" && edge.From == "F-79dh3q" {
			citation = true
			require.Equal(t, "doc:.tiller/design/origin.md", edge.To)
			require.Equal(t, "design/origin.md#L7-12", edge.Sources[0].Raw)
			require.Equal(t, 7, edge.Sources[0].CitationLine)
			require.Equal(t, ".tiller/design/origin.md", edge.Sources[0].CitationPath)
			require.Zero(t, edge.Sources[0].Line)
		}
		if edge.Kind == "implements" {
			opaque = true
			require.Equal(t, "ADR 14", idx.Vertices[edge.To].Raw)
			require.True(t, idx.Vertices[edge.To].Unresolved)
		}
		if edge.Kind == "derived_from" {
			revision = true
			require.Equal(t, "git:1111111", edge.Sources[0].Revision.Value)
		}
		if edge.Kind == "sourced_from" && edge.From == "A-aaaaaa" {
			for _, s := range edge.Sources {
				if s.Operation == 1 {
					operation = true
					require.Equal(t, ".tiller/feedback/request.md#L3-L4", s.Raw)
					require.Equal(t, ".tiller/feedback/request.md", s.CitationPath)
					require.Equal(t, 3, s.CitationLine)
					require.NotNil(t, s.Revision)
					require.Equal(t, "git:2222222", s.Revision.Value)
				}
			}
		}
	}
	require.True(t, citation)
	require.True(t, opaque)
	require.True(t, revision)
	require.True(t, operation)
}

func TestSourceCitationRangesPreserveAuthoredCoordinates(t *testing.T) {
	for _, locator := range []string{"L3", "L3-58", "L3-L58", "L3-3", "L3-L3"} {
		t.Run(locator, func(t *testing.T) {
			tree := contractTree()
			raw := "design/origin.md#" + locator
			tree.Features["F-79dh3q"].Source = raw
			index := relationships.BuildStructural(tree)
			edges := index.Adjacent("F-79dh3q", "sourced_from")
			require.Len(t, edges, 1)
			require.Equal(t, "doc:.tiller/design/origin.md", edges[0].To)
			require.Len(t, edges[0].Sources, 1)
			source := edges[0].Sources[0]
			require.Equal(t, raw, source.Raw)
			require.Equal(t, ".tiller/design/origin.md", source.CitationPath)
			require.Equal(t, 3, source.CitationLine)
			require.Equal(t, raw, tree.Features["F-79dh3q"].Source)
			require.Empty(t, index.Diagnostics)
		})
	}
}

func TestSourceCitationsAcceptSeveralRangesInOneDocument(t *testing.T) {
	for _, locator := range []string{"L3-L58,L321", "L3-L58, L321", "L3-L58,#L321", "L3-L58, #L321", "L3, L9-12, #L40-L41"} {
		t.Run(locator, func(t *testing.T) {
			tree := contractTree()
			raw := "design/origin.md#" + locator
			tree.Features["F-79dh3q"].Source = raw
			index := relationships.BuildStructural(tree)
			edges := index.Adjacent("F-79dh3q", "sourced_from")
			require.Len(t, edges, 1)
			require.Equal(t, "doc:.tiller/design/origin.md", edges[0].To)
			source := edges[0].Sources[0]
			require.Equal(t, raw, source.Raw)
			require.Equal(t, ".tiller/design/origin.md", source.CitationPath)
			require.Equal(t, 3, source.CitationLine, "the citation anchors at its first range")
			require.Empty(t, index.Diagnostics)
		})
	}
}

func TestSourceCitationsRejectUnsafeAndUnsupportedLocators(t *testing.T) {
	for _, raw := range []string{
		"../origin.md#L3-L58",
		"/origin.md#L3-L58",
		"design/../../origin.md#L3-L58",
		"design/../origin.md#L3-L58",
		`design\origin.md#L3-L58`,
		"https://example.com/origin.md#L3-L58",
		"design/\x00origin.md#L3-L58",
		"design/origin.md#L0-L58",
		"design/origin.md#L3-L0",
		"design/origin.md#L3-0",
		"design/origin.md#L58-L3",
		"design/origin.md#L58-3",
		"design/origin.md#L999999999999999999999",
		"design/origin.md#L3-L999999999999999999999",
		"design/origin.md#L3-999999999999999999999",
		"design/origin.md#L3-L58#extra",
		"design/origin.md#",
		"design/origin.md#L3-L58 ",
		"design/origin.md#L3,",
		"design/origin.md#L3,,L5",
		"design/origin.md#,L3",
		"design/origin.md#L3 ,L5",
		"design/origin.md#L3,L0",
		"design/origin.md#L3,L9-L5",
		"design/origin.md#L3, other.md#L5",
		"design/origin.md#L3, #",
		"Discussed design/origin.md",
		"Discussed design/origin.md#L3-L58 in the meeting",
		"cat design/origin.md",
	} {
		t.Run(raw, func(t *testing.T) {
			tree := contractTree()
			tree.Features["F-79dh3q"].Source = raw
			index := relationships.BuildStructural(tree)
			edges := index.Adjacent("F-79dh3q", "sourced_from")
			require.Len(t, edges, 1)
			target := index.Vertices[edges[0].To]
			require.Equal(t, "reference", target.Kind)
			require.True(t, target.Unresolved)
			require.Nil(t, target.Destination)
			require.Equal(t, raw, target.Raw)
			require.Equal(t, raw, edges[0].Sources[0].Raw)
			require.Empty(t, edges[0].Sources[0].CitationPath)
			require.Zero(t, edges[0].Sources[0].CitationLine)
			require.Len(t, index.Diagnostics, 1)
			require.Equal(t, "unresolved_reference", index.Diagnostics[0].Code)
			require.True(t, index.Complete, "an opaque assertion is indexed, not omitted")
		})
	}
}

func TestBaselineCoverageUsesSegmentBoundariesAndNeverAliasesExternalPaths(t *testing.T) {
	tree := contractTree()
	tree.Manifest = &store.Manifest{Inputs: store.InputsBlock{Code: []store.CodeRoot{{Root: "../legacy", Mode: store.CodeModeReference}}}}
	tree.Baseline = store.BaselineLedger{Docs: []store.BaselineDoc{
		{Path: "local.md", Describes: []string{"internal/store"}},
		{Path: "external.md", Describes: []string{"../legacy/internal/store"}},
	}}
	index := relationships.BuildStructural(tree)
	result, err := relationships.Query(context.Background(), index, relationships.QueryInput{From: []string{"path:internal/store/write.go"}, Follow: []string{"covered_by"}, Depth: 1})
	require.NoError(t, err)
	require.Contains(t, result.Matched, "doc:.tiller/design/local.md")
	require.NotContains(t, result.Matched, "doc:.tiller/design/external.md")
	require.Len(t, result.Edges, 1)
	require.True(t, result.Edges[0].Derived)
	require.Equal(t, "internal/store", result.Edges[0].DeclaredScope)
	unrelated, err := relationships.Query(context.Background(), index, relationships.QueryInput{From: []string{"path:internal/storage.go"}, Follow: []string{"covered_by"}, Depth: 1})
	require.NoError(t, err)
	require.NotContains(t, unrelated.Matched, "doc:.tiller/design/local.md")
}

func TestOpaqueQuestionCitationsDoNotCreateSharedIdentity(t *testing.T) {
	tree := contractTree()
	tree.Questions = map[string]*store.Question{
		"Q-aaaaaa": {ID: "Q-aaaaaa", Source: "Discussed in planning"},
		"Q-bbbbbb": {ID: "Q-bbbbbb", Source: "Discussed in planning"},
	}
	index := relationships.BuildStructural(tree)
	targets := map[string]string{}
	for _, edge := range index.Edges {
		if edge.Kind == "sourced_from" {
			targets[edge.From] = edge.To
		}
	}
	require.NotEmpty(t, targets["Q-aaaaaa"])
	require.NotEmpty(t, targets["Q-bbbbbb"])
	require.NotEqual(t, targets["Q-aaaaaa"], targets["Q-bbbbbb"])
	result, err := relationships.Query(context.Background(), index, relationships.QueryInput{From: []string{"Q-aaaaaa"}, Follow: []string{"sourced_from", "source_for"}, Depth: 2})
	require.NoError(t, err)
	require.NotContains(t, result.Matched, "Q-bbbbbb")
}

func TestRevisionObservationJSONOmitsAbsentTimeAndPreservesPrecision(t *testing.T) {
	bare, err := json.Marshal(relationships.RevisionOf(store.RevisionRef{
		Kind: store.KindGitBlob, Value: "git:abc1234", Path: "docs/source.md",
	}))
	require.NoError(t, err)
	var absent map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(bare, &absent))
	require.NotContains(t, absent, "observed_at")

	when := time.Date(2026, time.September, 19, 10, 15, 30, 123456789, time.FixedZone("CDT", -5*60*60))
	stamped, err := json.Marshal(relationships.RevisionOf(store.RevisionRef{ObservedAt: when}))
	require.NoError(t, err)
	var recorded struct {
		ObservedAt time.Time `json:"observed_at"`
	}
	require.NoError(t, json.Unmarshal(stamped, &recorded))
	require.True(t, when.Equal(recorded.ObservedAt), "revision evidence must retain the exact recorded instant")
}
