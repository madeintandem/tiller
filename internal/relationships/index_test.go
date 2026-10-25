package relationships

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestIndexDeduplicatesFactsWithoutDroppingRevisionEvidence(t *testing.T) {
	index := New()
	index.AddVertex(Vertex{ID: "doc:.tiller/design/a.md", Kind: "document"})
	index.AddVertex(Vertex{ID: "path:src/a.go", Kind: "path"})
	one := Source{File: ".tiller/baseline.yml", Field: "derived_from", Revision: RevisionOf(store.RevisionRef{Kind: "git_blob", Value: "git:111"})}
	two := Source{File: ".tiller/baseline.yml", Field: "derived_from", Revision: RevisionOf(store.RevisionRef{Kind: "git_blob", Value: "git:222"})}
	index.AddEdge("derived_from", "doc:.tiller/design/a.md", "path:src/a.go", two, one, one)
	index.AddEdge("derived_from", "doc:.tiller/design/a.md", "path:src/a.go", two)
	index.Finalize()
	forward := index.Adjacent("doc:.tiller/design/a.md", "derived_from")
	reverse := index.Adjacent("path:src/a.go", "derivation_of")
	require.Len(t, forward, 1)
	require.Equal(t, forward, reverse)
	require.Len(t, forward[0].Sources, 2)
	require.Equal(t, "git:111", forward[0].Sources[0].Revision.Value)
}

func TestIndexCoordinatesStayDistinctAndUnknownAddressesRefuse(t *testing.T) {
	index := New()
	index.AddVertex(Vertex{ID: "external_path:legacy/src/a.go", Kind: "external_path", Path: "legacy/src/a.go"})
	v, err := index.Resolve("path:src/./module/../a.go")
	require.NoError(t, err)
	require.Equal(t, "path:src/a.go", v.ID)
	require.Nil(t, v.Destination)
	_, err = index.Resolve("doc:.tiller/missing.md")
	require.Error(t, err)
	for _, raw := range []string{"../escape", "a/../../escape", "/absolute", "C:\\absolute", "C:", "https://example.com/path", "file:///etc/passwd", "a\x00b.md", "line\nbreak.md", ""} {
		_, err := PathID(raw)
		require.Error(t, err, raw)
	}
	// Literal filenames keep filesystem-valid punctuation as part of the name.
	for raw, want := range map[string]string{
		"notes#1.md":             "path:notes#1.md",
		"docs/notes?1.md":        "path:docs/notes?1.md",
		"notes:1.md":             "path:notes:1.md",
		"docs/./a:b/../c#d.md":   "path:docs/c#d.md",
		".tiller/design/plan.md": "path:.tiller/design/plan.md",
	} {
		got, err := PathID(raw)
		require.NoError(t, err, raw)
		require.Equal(t, want, got, raw)
	}
}

func TestIndexUpgradesDocumentPlaceholderWithoutInventingOtherTargets(t *testing.T) {
	index := New()
	index.AddVertex(Vertex{ID: "doc:.tiller/a.md", Kind: "document", Raw: ".tiller/a.md", Unresolved: true, DocumentOwner: "F-owner"})
	index.AddVertex(Vertex{ID: "doc:.tiller/a.md", Kind: "document", Path: ".tiller/a.md", Destination: &Destination{Kind: "file", Path: ".tiller/a.md"}})
	v, err := index.Resolve("doc:.tiller/./a.md")
	require.NoError(t, err)
	require.False(t, v.Unresolved)
	require.Equal(t, "F-owner", v.DocumentOwner)
	require.NotNil(t, v.Destination)
}

func TestNodePathUsesPhysicalAncestryInsteadOfStaleCanonicalPrefixes(t *testing.T) {
	feature := &store.Feature{ID: "F-owner", Key: new("old/spec"), EpicKey: "actual"}
	task := &store.Task{ID: "T-child", Key: new("old/other/work"), FeatureID: feature.ID}
	tree := &store.Tree{Features: map[string]*store.Feature{feature.ID: feature}}
	got, ok := NodePath(tree, task, "md")
	require.True(t, ok)
	require.Equal(t, ".tiller/epics/actual/features/spec/tasks/work/task.md", got)
	got, ok = NodePath(tree, feature, "eval.md")
	require.True(t, ok)
	require.Equal(t, ".tiller/epics/actual/features/spec/feature.eval.md", got)
}

func TestDiagnosticsScopeAndNormalizeWithoutLosingIncompleteExplanations(t *testing.T) {
	index := New()
	for _, subjects := range [][]string{{"F-b", "", "F-a", "F-b"}, {"F-a", "F-b"}} {
		index.AddDiagnostic(Diagnostic{Code: "unresolved_reference", Subjects: subjects, Source: &Source{File: "feature.yml", Line: 2}}, false)
	}
	index.AddDiagnostic(Diagnostic{Code: "unresolved_reference", Subjects: []string{"F-a", "F-b"}, Source: &Source{File: "feature.yml", Line: 3}}, false)
	index.AddDiagnostic(Diagnostic{Code: "contract_invalid", Facet: "contracts", Subjects: []string{"F-other"}}, true)
	index.AddDiagnostic(Diagnostic{Code: "history_unavailable", Facet: "amendments"}, true)
	// Deduplication must not discard the incomplete bit of an otherwise equal fact.
	index.AddDiagnostic(Diagnostic{Code: "contract_invalid", Facet: "contracts", Subjects: []string{"F-other"}}, false)
	index.Finalize()
	require.Len(t, index.Diagnostics, 4)
	own := index.DiagnosticsFor("F-a")
	require.Len(t, own, 4)
	for _, diagnostic := range own {
		if diagnostic.Code == "unresolved_reference" {
			require.Equal(t, []string{"F-a", "F-b"}, diagnostic.Subjects)
		}
	}
	unrelated := index.DiagnosticsFor("F-unrelated")
	require.Len(t, unrelated, 2)
	require.False(t, index.Complete)
	for _, diagnostic := range unrelated {
		require.True(t, diagnostic.Incomplete)
	}
	index.Finalize()
	require.Equal(t, own, index.DiagnosticsFor("F-a"))
	require.Equal(t, unrelated, index.DiagnosticsFor("F-unrelated"))
	require.Equal(t, []Diagnostic{}, New().DiagnosticsFor("F-a"))
}
