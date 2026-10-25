package relationships_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestMentionRendererConformance(t *testing.T) {
	data, err := os.ReadFile("../../testdata/work-mentions.json")
	require.NoError(t, err)
	var cases []struct {
		Name     string
		Markdown string
		Mentions []struct {
			ID   string
			Line int
		}
	}
	require.NoError(t, json.Unmarshal(data, &cases))
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			idx := relationships.New()
			relationships.AddDocuments(&store.Tree{}, idx, []relationships.Document{{Path: ".tiller/design/example.md", Content: []byte(tc.Markdown)}})
			got := map[string][]int{}
			for _, edge := range idx.Edges {
				if edge.Kind == "mentions" {
					for _, source := range edge.Sources {
						got[edge.To] = append(got[edge.To], source.Line)
					}
				}
			}
			want := map[string][]int{}
			for _, mention := range tc.Mentions {
				want[mention.ID] = append(want[mention.ID], mention.Line)
			}
			// Distinct occurrences on one line share a source location, not a duplicate assertion.
			for id, lines := range want {
				unique := []int{}
				for _, line := range lines {
					if len(unique) == 0 || unique[len(unique)-1] != line {
						unique = append(unique, line)
					}
				}
				want[id] = unique
			}
			require.Equal(t, want, got)
		})
	}
}

func TestDocumentsKeepIdentityOwnershipAndUnknownMentions(t *testing.T) {
	epicKey, featureKey := "engine", "engine/load"
	tree := &store.Tree{Epics: map[string]*store.Epic{"E-j8m4q2": {ID: "E-j8m4q2", Key: &epicKey}}, Features: map[string]*store.Feature{"F-79dh3q": {ID: "F-79dh3q", Key: &featureKey, EpicKey: epicKey}}}
	idx := relationships.BuildStructural(tree)
	file := ".tiller/epics/engine/features/load/feature.eval.md"
	relationships.AddDocuments(tree, idx, []relationships.Document{{Path: file, Content: []byte("F-zzzzzz\n\nF-zzzzzz")}, {Path: ".tiller/orientation.md", Content: []byte("F-79dh3q")}})
	require.Equal(t, "F-79dh3q", idx.Vertices["doc:"+file].DocumentOwner)
	require.True(t, idx.Vertices["doc:.tiller/orientation.md"].Generated)
	require.True(t, idx.Vertices["F-zzzzzz"].Unresolved)
	require.Nil(t, idx.Vertices["F-zzzzzz"].Destination)
	require.True(t, idx.Complete, "a fully indexed dangling mention is not incomplete extraction")
	var ownership, coordinate, mention bool
	for _, edge := range idx.Edges {
		if edge.Kind == "has_document" && edge.From == "F-79dh3q" && edge.To == "doc:"+file {
			ownership = true
		}
		if edge.Kind == "at_path" && edge.From == "doc:"+file && edge.To == "path:"+file {
			coordinate = true
		}
		if edge.Kind == "mentions" && edge.To == "F-zzzzzz" {
			mention = true
			require.Equal(t, []relationships.Source{{File: file, Line: 1}, {File: file, Line: 3}}, edge.Sources)
		}
		require.False(t, edge.Kind == "mentions" && edge.From == "F-79dh3q", "ownership must not fabricate work-to-work mentions")
	}
	require.True(t, ownership)
	require.True(t, coordinate)
	require.True(t, mention)
}

// Filesystem-valid punctuation in a literal corpus filename is part of the
// coordinate, not citation or URL syntax. Each such document keeps its own
// nonempty identity and destination, and its mentions start from it. No edge
// ever has an empty endpoint.
func TestPunctuatedDocumentFilenamesKeepDistinctIdentities(t *testing.T) {
	epicKey := "engine"
	tree := &store.Tree{Epics: map[string]*store.Epic{"E-j8m4q2": {ID: "E-j8m4q2", Key: &epicKey}}}
	idx := relationships.BuildStructural(tree)
	files := []string{".tiller/design/notes#1.md", ".tiller/design/notes?1.md", ".tiller/design/notes:1.md", ".tiller/design/plain.md"}
	documents := make([]relationships.Document, 0, len(files))
	for _, file := range files {
		documents = append(documents, relationships.Document{Path: file, Content: []byte("See E-j8m4q2.\n")})
	}
	relationships.AddDocuments(tree, idx, documents)

	mentionedBy := map[string]bool{}
	for _, edge := range idx.Edges {
		require.NotEmpty(t, edge.From, "edge %s has an empty source endpoint", edge.ID)
		require.NotEmpty(t, edge.To, "edge %s has an empty target endpoint", edge.ID)
		if edge.Kind == "mentions" && edge.To == "E-j8m4q2" {
			mentionedBy[edge.From] = true
		}
	}
	for _, file := range files {
		id := "doc:" + file
		vertex, ok := idx.Vertices[id]
		require.True(t, ok, "missing document vertex for %s", file)
		require.Equal(t, file, vertex.Path)
		require.NotNil(t, vertex.Destination)
		require.Equal(t, file, vertex.Destination.Path)
		require.True(t, mentionedBy[id], "the mention from %s must start at its own document", file)
		resolved, err := idx.Resolve(id)
		require.NoError(t, err)
		require.Equal(t, id, resolved.ID)
	}
	require.Equal(t, "doc:.tiller/design/plain.md", relationships.DocumentID(".tiller/design/plain.md"), "ordinary ids are unchanged")
}

// A document coordinate that cannot be a confined repository path is
// diagnosed, never indexed under an empty identity.
func TestInvalidDocumentCoordinateIsDiagnosedWithoutPhantomEdges(t *testing.T) {
	epicKey := "engine"
	tree := &store.Tree{Epics: map[string]*store.Epic{"E-j8m4q2": {ID: "E-j8m4q2", Key: &epicKey}}}
	idx := relationships.BuildStructural(tree)
	before := len(idx.Edges)
	relationships.AddDocuments(tree, idx, []relationships.Document{
		{Path: "../outside.md", Content: []byte("E-j8m4q2")},
		{Path: "/abs/outside.md", Content: []byte("E-j8m4q2")},
	})
	require.Len(t, idx.Edges, before, "no edge from an invalid document")
	codes := 0
	for _, diagnostic := range idx.Diagnostics {
		if diagnostic.Code == "invalid_path" && diagnostic.Facet == "documents" {
			codes++
		}
	}
	require.Equal(t, 2, codes)
}
