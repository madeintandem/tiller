package relationships_test

import (
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func contractTree() *store.Tree {
	epicKey, featureKey := "engine", "engine/load"
	return &store.Tree{Epics: map[string]*store.Epic{"E-j8m4q2": {ID: "E-j8m4q2", Key: &epicKey}}, Features: map[string]*store.Feature{"F-79dh3q": {ID: "F-79dh3q", Key: &featureKey, EpicKey: epicKey}}}
}

func TestContractsRetainPathClaimsAndOpaqueReferences(t *testing.T) {
	tree := contractTree()
	idx := relationships.BuildStructural(tree)
	contract := &store.EvalFile{Node: "F-79dh3q", Criteria: []store.EvalCriterion{
		{ID: "C1", Statement: "file", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"internal/store/missing_test.go"}, Run: "must never execute"}},
		{ID: "C2", Statement: "selector", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"TestStore"}}},
		{ID: "C3", Statement: "rubric", Binding: store.EvalBinding{Type: store.BindingEval, Ref: store.BindingRefs{"Review internal/store for clarity"}}},
		{ID: "C4", Statement: "glob", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"internal/**/*.go"}}},
		{ID: "C5", Statement: "baseline", Polarity: store.PolarityPreserve, Binding: store.EvalBinding{Type: store.BindingManual}, Baseline: &store.EvalBaseline{Surface: "internal/store", Artifact: "testdata/old.json", CapturedAt: store.RevisionRef{Kind: store.KindGitTree, Path: "internal/store", Value: "git:1111111"}}},
	}}
	relationships.AddContracts(tree, idx, map[string]*store.EvalFile{"F-79dh3q": contract}, nil)
	var preserves relationships.RelationKind
	for _, kind := range idx.Catalog() {
		if kind.Name == "preserves" {
			preserves = kind
		}
	}
	require.Equal(t, "preserves", preserves.Name)
	var covers []relationships.Edge
	preserve := map[string]relationships.Source{}
	opaque := map[string]string{}
	for _, edge := range idx.Edges {
		if edge.Kind == "covers" && edge.From == "F-79dh3q" {
			covers = append(covers, edge)
		}
		if edge.Kind == "preserves" {
			require.Equal(t, "F-79dh3q", edge.From)
			require.Contains(t, preserves.FromKinds, idx.Vertices[edge.From].Kind)
			require.Contains(t, preserves.ToKinds, idx.Vertices[edge.To].Kind)
			preserve[edge.Sources[0].Role] = edge.Sources[0]
		}
		for _, source := range edge.Sources {
			if source.Classification != "" {
				opaque[source.Raw] = source.Classification
			}
		}
	}
	require.Len(t, covers, 1)
	require.Equal(t, "path:internal/store/missing_test.go", covers[0].To)
	require.Equal(t, "acceptance_binding", covers[0].Sources[0].Basis)
	require.Equal(t, "C1", covers[0].Sources[0].Criterion)
	require.Equal(t, "opaque", opaque["TestStore"])
	require.Equal(t, "opaque", opaque["Review internal/store for clarity"])
	require.Equal(t, "opaque_path", opaque["internal/**/*.go"])
	require.Len(t, preserve, 2)
	require.Equal(t, "git:1111111", preserve["surface"].Revision.Value)
	require.Equal(t, "git:1111111", preserve["artifact"].Revision.Value)
	require.NotContains(t, idx.Vertices, "path:internal/**/*.go")
}

// A test binding that lists several files covers each path-shaped one: one
// covers edge per item, each recording which list entry it came from.
func TestContractsCoverEachPathInARefList(t *testing.T) {
	tree := contractTree()
	idx := relationships.BuildStructural(tree)
	contract := &store.EvalFile{Node: "F-79dh3q", Criteria: []store.EvalCriterion{
		{ID: "C1", Statement: "list", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/a_spec.rb", "TestStore", "spec/b_spec.rb"}}},
		{ID: "C2", Statement: "scalar", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/c_spec.rb"}}},
	}}
	relationships.AddContracts(tree, idx, map[string]*store.EvalFile{"F-79dh3q": contract}, nil)

	fields := map[string]string{}
	for _, edge := range idx.Edges {
		if edge.Kind != "covers" || edge.From != "F-79dh3q" {
			continue
		}
		require.Len(t, edge.Sources, 1)
		fields[edge.To] = edge.Sources[0].Field
		require.Equal(t, strings.TrimPrefix(edge.To, "path:"), edge.Sources[0].Scope)
		require.Equal(t, strings.TrimPrefix(edge.To, "path:"), edge.Sources[0].Raw)
	}
	require.Equal(t, map[string]string{
		"path:spec/a_spec.rb": "criteria[0].binding.ref[0]",
		"path:spec/b_spec.rb": "criteria[0].binding.ref[2]",
		"path:spec/c_spec.rb": "criteria[1].binding.ref",
	}, fields)
}

func TestContractsDoNotBorrowMismatchedOwnership(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contract *store.EvalFile
		failure  string
	}{
		{"read failure", nil, "permission denied"},
		{"misowned draft", &store.EvalFile{Node: "F-zzzzzz"}, ""},
		{"invalid criterion", &store.EvalFile{Node: "F-79dh3q", Criteria: []store.EvalCriterion{{ID: "C1"}}}, ""},
		{"mismatched", &store.EvalFile{Node: "F-zzzzzz", Criteria: []store.EvalCriterion{{ID: "C1", Statement: "foreign", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/foreign.go"}}}}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := contractTree()
			idx := relationships.BuildStructural(tree)
			contracts := map[string]*store.EvalFile{}
			errs := map[string]string{}
			if tc.contract != nil {
				contracts["F-79dh3q"] = tc.contract
			}
			if tc.failure != "" {
				errs["F-79dh3q"] = tc.failure
			}
			relationships.AddContracts(tree, idx, contracts, errs)
			require.False(t, idx.Complete)
			require.NotEmpty(t, idx.Diagnostics)
			for _, diagnostic := range idx.Diagnostics {
				require.Equal(t, "contracts", diagnostic.Facet)
				require.ElementsMatch(t, []string{"F-79dh3q", "doc:.tiller/epics/engine/features/load/feature.eval.md"}, diagnostic.Subjects)
			}
			for _, edge := range idx.Edges {
				require.NotEqual(t, "covers", edge.Kind)
			}
		})
	}
}

func TestPreserveEvidenceRetainsDistinctRevisionsOfOnePath(t *testing.T) {
	tree := contractTree()
	index := relationships.BuildStructural(tree)
	contract := &store.EvalFile{Node: "F-79dh3q"}
	for n, revision := range []string{"git:1111111", "git:2222222"} {
		id := []string{"C1", "C2"}[n]
		contract.Criteria = append(contract.Criteria, store.EvalCriterion{ID: id, Statement: "preserve behavior", Polarity: store.PolarityPreserve, Binding: store.EvalBinding{Type: store.BindingManual}, Baseline: &store.EvalBaseline{Surface: "internal/store", CapturedAt: store.RevisionRef{Kind: store.KindGitTree, Value: revision, Path: "internal/store"}}})
	}
	relationships.AddContracts(tree, index, map[string]*store.EvalFile{"F-79dh3q": contract}, nil)
	var revisions []string
	for _, edge := range index.Edges {
		if edge.Kind == "preserves" && edge.To == "path:internal/store" {
			for _, source := range edge.Sources {
				revisions = append(revisions, source.Revision.Value)
			}
		}
	}
	require.ElementsMatch(t, []string{"git:1111111", "git:2222222"}, revisions)
}

func TestAbsentAndDraftContractsRemainCompleteWithoutInventingCoverage(t *testing.T) {
	for _, draft := range []bool{false, true} {
		tree := contractTree()
		index := relationships.BuildStructural(tree)
		contracts := map[string]*store.EvalFile{}
		if draft {
			contracts["F-79dh3q"] = &store.EvalFile{Node: "F-79dh3q"}
		}
		relationships.AddContracts(tree, index, contracts, nil)
		require.True(t, index.Complete)
		require.Empty(t, index.Diagnostics)
		require.Empty(t, index.Adjacent("F-79dh3q", "covers"))
		require.Empty(t, index.Adjacent("F-79dh3q", "preserves"))
		if draft {
			require.Len(t, index.Adjacent("F-79dh3q", "has_document"), 1)
		} else {
			require.Empty(t, index.Adjacent("F-79dh3q", "has_document"))
		}
	}
}

func TestInvalidContractPathIsNotAnUnwrittenDraft(t *testing.T) {
	tree := contractTree()
	tree.Features["F-79dh3q"].Key = new("../escape")
	index := relationships.BuildStructural(tree)
	relationships.AddContracts(tree, index, nil, nil)
	require.False(t, index.Complete)
	require.Equal(t, "contract_path_unavailable", index.Diagnostics[0].Code)
	require.Equal(t, []string{"F-79dh3q"}, index.Diagnostics[0].Subjects)
}
