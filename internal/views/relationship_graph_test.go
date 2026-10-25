package views

import (
	"testing"

	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBacklinksRetainDistinctCitationsWithoutDuplicatingExistingPanels(t *testing.T) {
	index := relationships.New()
	index.AddVertex(relationships.Vertex{ID: "F-source", Kind: "feature"})
	index.AddVertex(relationships.Vertex{ID: "F-target", Kind: "feature"})
	index.AddVertex(relationships.Vertex{ID: "doc:.tiller/source.md", Kind: "document", DocumentOwner: "F-source"})
	index.AddVertex(relationships.Vertex{ID: "doc:.tiller/target.md", Kind: "document", DocumentOwner: "F-target"})
	index.AddEdge("has_document", "F-target", "doc:.tiller/target.md")
	index.AddEdge("mentions", "doc:.tiller/source.md", "F-target", relationships.Source{File: ".tiller/source.md", Line: 2}, relationships.Source{File: ".tiller/source.md", Line: 7})
	index.AddEdge("sourced_from", "doc:.tiller/source.md", "doc:.tiller/target.md", relationships.Source{File: ".tiller/source.md", Field: "source"})
	index.AddEdge("depends_on", "F-source", "F-target")
	index.Finalize()
	got := BacklinksOf(index, "F-target")
	require.Equal(t, 1, got.Count, "one source identity even when it reaches the work and its owned document")
	require.Len(t, got.Groups, 2)
	require.Equal(t, "mentioned_by", got.Groups[0].Kind)
	require.Len(t, got.Groups[0].Sources, 1)
	require.Equal(t, "F-source", got.Groups[0].Sources[0].OwnerVertex.ID)
	require.Equal(t, []int{2, 7}, []int{got.Groups[0].Sources[0].Edges[0].Sources[0].Line, got.Groups[0].Sources[0].Edges[0].Sources[1].Line})
	require.Equal(t, "source_for", got.Groups[1].Kind)
	require.True(t, got.Complete)
}

func TestBacklinksScopeWarningsToNodeAndOwnedDocumentsWhileMapStaysGlobal(t *testing.T) {
	tree := &store.Tree{Features: map[string]*store.Feature{
		"F-owner": {ID: "F-owner", Key: new("work/owner")},
		"F-other": {ID: "F-other", Key: new("work/other")},
	}}
	index := relationships.BuildStructural(tree)
	file := ".tiller/epics/work/features/owner/notes.md"
	relationships.AddDocuments(tree, index, []relationships.Document{{Path: file, Content: []byte("F-zzzzzz\n")}})
	index.Finalize()
	owner := BacklinksOf(index, "F-owner")
	require.True(t, owner.Complete)
	require.Len(t, owner.Diagnostics, 1)
	require.Equal(t, "unresolved_mention", owner.Diagnostics[0].Code)
	require.Equal(t, file, owner.Diagnostics[0].Source.File)
	require.ElementsMatch(t, []string{"doc:" + file, "F-zzzzzz"}, owner.Diagnostics[0].Subjects)
	require.Empty(t, BacklinksOf(index, "F-other").Diagnostics)
	require.Equal(t, owner.Diagnostics, RelationshipGraphOf(index).Diagnostics)

	relationships.AddContracts(tree, index, nil, map[string]string{"F-other": "malformed frontmatter"})
	index.Finalize()
	owner = BacklinksOf(index, "F-owner")
	require.False(t, owner.Complete)
	require.Len(t, owner.Diagnostics, 2, "global extraction loss must stay explained on unrelated pages")
}
