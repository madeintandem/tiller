package relationships

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func queryFixture() *Index {
	index := New()
	for _, vertex := range []Vertex{
		{ID: "E-aaaaaa", Kind: "epic", Key: "engine", Status: "active"},
		{ID: "F-aaaaaa", Kind: "feature", Key: "engine/store", Status: "done", Contributors: []store.Contributor{{ID: "jc", Name: "JC"}}},
		{ID: "T-aaaaaa", Kind: "task", Key: "engine/store/write", Status: "done", Owner: "jc"},
		{ID: "Q-aaaaaa", Kind: "question", Status: "open", Owner: "jc"},
		{ID: "Q-bbbbbb", Kind: "question", Status: "answered", Owner: "jc"},
	} {
		index.AddVertex(vertex)
	}
	index.AddEdge("contains", "E-aaaaaa", "F-aaaaaa", Source{File: "feature.yml", Field: "layout"})
	index.AddEdge("contains", "F-aaaaaa", "T-aaaaaa", Source{File: "task.yml", Field: "layout"})
	index.AddEdge("blocks", "Q-aaaaaa", "T-aaaaaa", Source{File: "questions.yml", Field: "blocks"})
	index.AddEdge("blocks", "Q-bbbbbb", "E-aaaaaa", Source{File: "questions.yml", Field: "blocks"})
	index.AddEdge("depends_on", "T-aaaaaa", "F-aaaaaa", Source{File: "task.yml", Field: "depends_on"})
	index.AddEdge("depends_on", "F-aaaaaa", "T-aaaaaa", Source{File: "feature.yml", Field: "depends_on"})
	index.Finalize()
	return index
}

func TestQueryFiltersAfterUnionTraversalAndRetainsCanonicalEdges(t *testing.T) {
	input, err := ParseQuery([]string{"E-aaaaaa"}, []string{"contains,blocked_by"}, []string{"3"}, []string{"kind=question", "status=open"})
	require.NoError(t, err)
	result, err := Query(context.Background(), queryFixture(), input)
	require.NoError(t, err)
	require.Equal(t, []string{"Q-aaaaaa"}, result.Matched)
	depths := map[string]int{}
	for _, vertex := range result.Vertices {
		depths[vertex.ID] = vertex.Depth
	}
	require.Equal(t, map[string]int{"E-aaaaaa": 0, "F-aaaaaa": 1, "Q-bbbbbb": 1, "T-aaaaaa": 2, "Q-aaaaaa": 3}, depths)
	require.True(t, result.Vertices[0].Seed)
	for _, edge := range result.Edges {
		if edge.Kind == "blocks" {
			require.Equal(t, "Q-", edge.From[:2], "inverse traversal must not invert the recorded fact")
		}
	}
}

func TestQueryMultipleSeedsCyclesAndMinimumDepth(t *testing.T) {
	index := queryFixture()
	input, err := ParseQuery([]string{"T-aaaaaa", "engine", "E-aaaaaa"}, []string{"contains,depends_on,contained_by"}, []string{"1000000000"}, nil)
	require.NoError(t, err)
	result, err := Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Equal(t, []string{"E-aaaaaa", "T-aaaaaa"}, result.Starts)
	require.Len(t, result.Vertices, 3)
	require.Equal(t, "E-aaaaaa", result.Vertices[0].ID)
	require.Equal(t, "T-aaaaaa", result.Vertices[1].ID)
	require.True(t, result.Vertices[1].Seed)
	require.Equal(t, 1, result.Vertices[2].Depth)
	require.Len(t, result.Edges, 4, "each fact appears once even when reached from both ends or several seeds")

	input.Depth = 0
	input.Where = []QueryFilter{{Field: "kind", Value: "task"}}
	result, err = Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Equal(t, []string{"T-aaaaaa"}, result.Matched)
	require.Empty(t, result.Edges)
	require.Len(t, result.Vertices, 2)
}

func TestQueryEnclosingCoverageUsesSegmentsAndKeepsDeclaredScope(t *testing.T) {
	index := queryFixture()
	index.AddVertex(Vertex{ID: "path:internal/store", Kind: "path", Path: "internal/store"})
	index.AddVertex(Vertex{ID: "path:internal/storage", Kind: "path", Path: "internal/storage"})
	index.AddVertex(Vertex{ID: "external_path:internal/store", Kind: "external_path", Path: "internal/store"})
	index.AddEdge("covers", "F-aaaaaa", "path:internal/store", Source{File: "feature.yml", Field: "surface.paths", Basis: "work_surface"})
	index.AddEdge("covers", "T-aaaaaa", "path:internal/storage", Source{File: "task.yml", Field: "surface.paths"})
	index.AddEdge("covers", "E-aaaaaa", "external_path:internal/store", Source{File: "baseline.yml", Field: "describes"})
	index.Finalize()
	input, err := ParseQuery([]string{"path:internal/store/./write.go"}, []string{"covered_by"}, nil, []string{"status=done"})
	require.NoError(t, err)
	result, err := Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Equal(t, []string{"path:internal/store/write.go"}, result.Starts)
	require.Equal(t, []string{"F-aaaaaa"}, result.Matched)
	require.Len(t, result.Edges, 1)
	require.True(t, result.Edges[0].Derived)
	require.Equal(t, "internal/store", result.Edges[0].DeclaredScope)
	require.Equal(t, "path:internal/store/write.go", result.Edges[0].To)
	require.Equal(t, "work_surface", result.Edges[0].Sources[0].Basis)
	_, persisted := index.Vertices["path:internal/store/write.go"]
	require.False(t, persisted, "query must not mutate the shared index")

	input.From = []string{"path:internal/storage.go"}
	result, err = Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Empty(t, result.Matched)
	require.Empty(t, result.Edges)
	input.From = []string{"path:internal/store"}
	result, err = Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Len(t, result.Edges, 1)
	require.False(t, result.Edges[0].Derived)
}

func TestQueryRootScopeAndDanglingAssertionsRemainHonest(t *testing.T) {
	index := queryFixture()
	index.AddVertex(Vertex{ID: "path:.", Kind: "path", Path: "."})
	index.AddVertex(Vertex{ID: "F-ffffff", Kind: "reference", Raw: "F-ffffff", Unresolved: true})
	index.AddEdge("covers", "F-aaaaaa", "path:.", Source{File: "feature.yml", Field: "surface.paths"})
	index.AddEdge("depends_on", "F-aaaaaa", "F-ffffff", Source{File: "feature.yml", Field: "depends_on"})
	index.AddDiagnostic(Diagnostic{Code: "unresolved_reference", Facet: "structure", Message: "F-ffffff is not present"}, false)
	index.Finalize()
	input, err := ParseQuery([]string{"path:main.go"}, []string{"covered_by,depends_on"}, []string{"2"}, []string{"kind=reference"})
	require.NoError(t, err)
	result, err := Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Equal(t, []string{"F-ffffff"}, result.Matched)
	require.True(t, result.Complete, "an indexed dangling assertion is not failed extraction")
	var scopes []string
	for _, edge := range result.Edges {
		if edge.Derived {
			scopes = append(scopes, edge.DeclaredScope)
		}
	}
	require.Equal(t, []string{"."}, scopes)
	for _, vertex := range result.Vertices {
		if vertex.ID == "F-ffffff" {
			require.True(t, vertex.Unresolved)
			require.Nil(t, vertex.Destination)
			require.Equal(t, 2, vertex.Depth)
		}
	}
	input.From = []string{"F-ffffff"}
	input.Depth = 0
	result, err = Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Equal(t, []string{"F-ffffff"}, result.Matched, "indexed unresolved addresses can be inspected exactly")
}

func TestQueryDeterministicAcrossEquivalentInputsAndExactFilters(t *testing.T) {
	index := queryFixture()
	first, err := ParseQuery([]string{"T-aaaaaa", "E-aaaaaa"}, []string{"contains,depends_on,contained_by"}, []string{"3"}, []string{"status=done", "owner=jc"})
	require.NoError(t, err)
	second, err := ParseQuery([]string{"engine", "T-aaaaaa", "E-aaaaaa"}, []string{"contained_by,contains,depends_on,contains"}, []string{"3"}, []string{"owner=jc", "status=done"})
	require.NoError(t, err)
	want, err := Query(context.Background(), index, first)
	require.NoError(t, err)
	got, err := Query(context.Background(), index, second)
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.Equal(t, []string{"T-aaaaaa"}, got.Matched, "historical contribution is not scalar ownership")
	second.Where = []QueryFilter{{Field: "status", Value: "Done"}}
	got, err = Query(context.Background(), index, second)
	require.NoError(t, err)
	require.Empty(t, got.Matched, "equality is case-sensitive")
}

func TestQueryEmptyIncompleteAndUnknownStartsAreDistinct(t *testing.T) {
	index := queryFixture()
	index.AddDiagnostic(Diagnostic{Code: "history_unavailable", Facet: "history", Message: "history unreadable"}, true)
	index.Finalize()
	input, err := ParseQuery([]string{"E-aaaaaa"}, []string{"contains"}, nil, []string{"owner=not-jc"})
	require.NoError(t, err)
	result, err := Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Empty(t, result.Matched)
	require.NotNil(t, result.Matched)
	require.False(t, result.Complete)
	require.Equal(t, "history", result.Diagnostics[0].Facet)

	input.From = []string{"F-ffffff"}
	_, err = Query(context.Background(), index, input)
	var missing *resolve.ErrNotFound
	require.ErrorAs(t, err, &missing)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Query(ctx, index, input)
	require.True(t, errors.Is(err, context.Canceled))
}

func TestQueryDiagnosticsIncludeFilteredBridgesButNotUnreachedWarnings(t *testing.T) {
	tree := &store.Tree{Features: map[string]*store.Feature{
		"F-aaaaaa": {ID: "F-aaaaaa", DependsOn: []string{"F-bbbbbb"}},
		"F-bbbbbb": {ID: "F-bbbbbb", DependsOn: []string{"F-zzzzzz"}},
		"F-cccccc": {ID: "F-cccccc", Source: "unsupported reference"},
	}}
	index := BuildStructural(tree)
	input := QueryInput{From: []string{"F-aaaaaa"}, Follow: []string{"depends_on"}, Depth: 1, Where: []QueryFilter{{Field: "kind", Value: "task"}}}
	result, err := Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Empty(t, result.Matched)
	require.Len(t, result.Diagnostics, 1)
	require.Equal(t, []string{"F-bbbbbb", "F-zzzzzz"}, result.Diagnostics[0].Subjects)
	input.Depth = 0
	result, err = Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Empty(t, result.Diagnostics)
	input.From = []string{"F-zzzzzz"}
	result, err = Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Len(t, result.Diagnostics, 1, "an unresolved endpoint can inspect its own assertion")
}

func TestQueryDocumentSeedKeepsContractLossExplainedOnUnrelatedQueries(t *testing.T) {
	tree := &store.Tree{Features: map[string]*store.Feature{
		"F-aaaaaa": {ID: "F-aaaaaa", Key: new("work/alpha")},
		"F-bbbbbb": {ID: "F-bbbbbb", Key: new("work/bravo")},
	}}
	index := BuildStructural(tree)
	file := ".tiller/epics/work/features/bravo/feature.eval.md"
	AddDocuments(tree, index, []Document{{Path: file}})
	AddContracts(tree, index, nil, map[string]string{"F-bbbbbb": "malformed frontmatter"})
	index.Finalize()
	for _, start := range []string{"doc:" + file, "F-aaaaaa"} {
		result, err := Query(context.Background(), index, QueryInput{From: []string{start}, Follow: []string{"document_of"}, Depth: 0})
		require.NoError(t, err)
		require.False(t, result.Complete)
		require.Len(t, result.Diagnostics, 1)
		require.Equal(t, "contracts", result.Diagnostics[0].Facet)
		require.ElementsMatch(t, []string{"F-bbbbbb", "doc:" + file}, result.Diagnostics[0].Subjects)
	}
}

func TestQueryJSONDistinguishesReferenceIssuesFromGlobalExtractionLoss(t *testing.T) {
	tree := &store.Tree{Features: map[string]*store.Feature{
		"F-aaaaaa": {ID: "F-aaaaaa", Key: new("work/alpha"), Source: "unsupported selected reference"},
		"F-bbbbbb": {ID: "F-bbbbbb", Key: new("work/bravo"), Source: "unsupported unrelated reference"},
	}}
	index := BuildStructural(tree)
	AddContracts(tree, index, nil, map[string]string{"F-bbbbbb": "malformed frontmatter"})
	index.Finalize()
	result, err := Query(context.Background(), index, QueryInput{From: []string{"F-aaaaaa"}, Follow: []string{"sourced_from"}, Depth: 0})
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	var response struct {
		Complete    bool `json:"complete"`
		Diagnostics []struct {
			Code       string   `json:"code"`
			Subjects   []string `json:"subjects"`
			Incomplete bool     `json:"incomplete"`
		} `json:"diagnostics"`
	}
	require.NoError(t, json.Unmarshal(encoded, &response))
	require.False(t, response.Complete)
	require.Len(t, response.Diagnostics, 2)
	byCode := map[string]bool{}
	for _, diagnostic := range response.Diagnostics {
		byCode[diagnostic.Code] = diagnostic.Incomplete
		if diagnostic.Code == "unresolved_reference" {
			require.Contains(t, diagnostic.Subjects, "F-aaaaaa")
			require.NotContains(t, diagnostic.Subjects, "F-bbbbbb")
		} else {
			require.Contains(t, diagnostic.Subjects, "F-bbbbbb")
		}
	}
	require.Equal(t, map[string]bool{"unresolved_reference": false, "contract_unavailable": true}, byCode)
}

func TestParseQueryRefusesMalformedParameters(t *testing.T) {
	for _, test := range []struct {
		name                       string
		from, follow, depth, where []string
		reason                     string
	}{
		{name: "missing from", follow: []string{"contains"}, reason: "query_from_required"},
		{name: "empty seed", from: []string{""}, follow: []string{"contains"}, reason: "query_from_invalid"},
		{name: "missing follow", from: []string{"E-aaaaaa"}, reason: "query_follow_required"},
		{name: "unknown direction", from: []string{"E-aaaaaa"}, follow: []string{"contains,nope"}, reason: "query_follow_invalid"},
		{name: "empty direction", from: []string{"E-aaaaaa"}, follow: []string{"contains,"}, reason: "query_follow_invalid"},
		{name: "negative depth", from: []string{"E-aaaaaa"}, follow: []string{"contains"}, depth: []string{"-1"}, reason: "query_depth_invalid"},
		{name: "overflow depth", from: []string{"E-aaaaaa"}, follow: []string{"contains"}, depth: []string{"999999999999999999999"}, reason: "query_depth_invalid"},
		{name: "repeated depth", from: []string{"E-aaaaaa"}, follow: []string{"contains"}, depth: []string{"1", "2"}, reason: "query_depth_invalid"},
		{name: "unknown field", from: []string{"E-aaaaaa"}, follow: []string{"contains"}, where: []string{"title=hello"}, reason: "query_where_invalid"},
		{name: "missing equals", from: []string{"E-aaaaaa"}, follow: []string{"contains"}, where: []string{"kind"}, reason: "query_where_invalid"},
		{name: "empty equality", from: []string{"E-aaaaaa"}, follow: []string{"contains"}, where: []string{"status="}, reason: "query_where_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseQuery(test.from, test.follow, test.depth, test.where)
			var misuse *QueryError
			require.ErrorAs(t, err, &misuse)
			require.Equal(t, test.reason, misuse.Reason())
		})
	}
}

func TestContributorFilterTraversesNoncontributingBridges(t *testing.T) {
	tree := &store.Tree{
		Epics: map[string]*store.Epic{"E-root00": {ID: "E-root00", Key: new("root")}},
		Features: map[string]*store.Feature{
			"F-seed00": {ID: "F-seed00", EpicKey: "root", DependsOn: []string{"F-bridge"}},
			"F-bridge": {ID: "F-bridge", EpicKey: "root", DependsOn: []string{"F-histor"}},
			"F-histor": {ID: "F-histor", EpicKey: "root"},
		},
		Tasks: map[string]*store.Task{
			"T-old000": {ID: "T-old000", FeatureID: "F-histor", Status: "cancelled", Owner: "past", Started: true},
			"T-ready0": {ID: "T-ready0", FeatureID: "F-histor", Status: "ready", Owner: "current"},
		},
	}
	tree.ProjectContributors()
	index := BuildStructural(tree)
	input, err := ParseQuery([]string{"F-seed00"}, []string{"depends_on"}, []string{"2"}, []string{"contributor=past"})
	require.NoError(t, err)
	result, err := Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Equal(t, []string{"F-histor"}, result.Matched)
	require.Len(t, result.Vertices, 3, "nonmatching ancestors remain traversal bridges")
	require.Equal(t, "F-bridge", result.Vertices[1].ID)
	require.Empty(t, result.Vertices[1].Owner)

	input.Where = []QueryFilter{{Field: "owner", Value: "past"}}
	result, err = Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Empty(t, result.Matched, "contributors never create ancestor scalar ownership")

	input.From = []string{"E-root00"}
	input.Follow = []string{"contains"}
	input.Where = []QueryFilter{{Field: "owner", Value: "current"}}
	result, err = Query(context.Background(), index, input)
	require.NoError(t, err)
	require.Equal(t, []string{"T-ready0"}, result.Matched, "ownerless ancestors do not prune live assignments")
}
