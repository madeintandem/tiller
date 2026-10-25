package readinput_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func relationshipInputTree(t *testing.T) (*store.Tree, string) {
	t.Helper()
	root := t.TempDir()
	epic, feature := "engine", "engine/load"
	file := ".tiller/epics/engine/features/load/feature.eval.md"
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, file)), 0755))
	return &store.Tree{Root: root, Epics: map[string]*store.Epic{"E-j8m4q2": {ID: "E-j8m4q2", Key: &epic}}, Features: map[string]*store.Feature{"F-79dh3q": {ID: "F-79dh3q", Key: &feature, EpicKey: epic}}}, file
}

func TestRelationshipsUsesSuppliedContractsAndEvents(t *testing.T) {
	tree, file := relationshipInputTree(t)
	// The supplied contract is the already gathered request input. Re-parsing the
	// physical prose here would disagree with the caller's contract presentation.
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, file), []byte("not YAML frontmatter"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(tree.Root, ".tiller/events/2026"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, ".tiller/events/2026/bad.yml"), []byte("[broken"), 0644))
	supplied := &store.EvalFile{Node: "F-79dh3q", Criteria: []store.EvalCriterion{{ID: "C1", Statement: "works", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/store_test.go"}}}}}
	index, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true, Contracts: map[string]*store.EvalFile{"F-79dh3q": supplied}})
	require.NoError(t, err)
	require.True(t, index.Complete)
	require.Contains(t, index.Vertices, "path:spec/store_test.go")
}

func TestRelationshipsParsesCorpusContractsAndNamesIncompleteFacets(t *testing.T) {
	tree, file := relationshipInputTree(t)
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, file), []byte("---\nnode: F-79dh3q\ncriteria: []\n---\nF-zzzzzz\n"), 0644))
	index, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true, HistoryError: "events unavailable"})
	require.NoError(t, err)
	require.False(t, index.Complete)
	facets := map[string]bool{}
	for _, diagnostic := range index.Diagnostics {
		facets[diagnostic.Facet] = true
	}
	require.False(t, facets["contracts"], "an owned draft is not extraction loss")
	require.True(t, facets["amendments"])
	require.Contains(t, index.Vertices, "F-zzzzzz")
}

func TestRelationshipsRefusesUnsafeCorpusDespiteSuppliedInputs(t *testing.T) {
	tree, _ := relationshipInputTree(t)
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(tree.Root, ".tiller/design")))
	index, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true})
	require.Nil(t, index)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, "corpus_unreadable", env.Reason)
}

func TestRelationshipsCancellationIsNotPartialSuccess(t *testing.T) {
	tree, _ := relationshipInputTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	index, err := readinput.Relationships(ctx, tree, readinput.RelationshipOptions{EventsLoaded: true, AllowIncompleteCorpus: true})
	require.Nil(t, index)
	require.ErrorIs(t, err, context.Canceled)
}

func TestRelationshipsResolvesCitedLocalProseWithoutExpandingMentionCorpus(t *testing.T) {
	tree, _ := relationshipInputTree(t)
	tree.Features["F-79dh3q"].Source = "docs/source.md#L2-3"
	require.NoError(t, os.Mkdir(filepath.Join(tree.Root, "docs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, "docs/source.md"), []byte("source\nF-zzzzzz\nlast\n"), 0644))
	index, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true})
	require.NoError(t, err)
	document := index.Vertices["doc:docs/source.md"]
	require.False(t, document.Unresolved)
	require.Equal(t, "docs/source.md", document.Destination.Path)
	require.NotContains(t, index.Vertices, "F-zzzzzz", "outside prose is cited, never scanned for mentions")
	require.Empty(t, document.DocumentOwner)
}

// A citation never advertises a hidden coordinate as a readable file
// destination (bug hunt defect 07): the console serves no dot-prefixed path
// outside the .tiller/ corpus root, so the relationship index must not offer
// one, and the citation keeps its authored text without a destination.
func TestRelationshipsDoesNotAdvertiseHiddenCitedDocument(t *testing.T) {
	tree, _ := relationshipInputTree(t)
	tree.Features["F-79dh3q"].Source = ".notes/secret.md#L1"
	require.NoError(t, os.Mkdir(filepath.Join(tree.Root, ".notes"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, ".notes/secret.md"), []byte("super-secret\n"), 0644))
	index, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true})
	require.NoError(t, err)
	document := index.Vertices["doc:.notes/secret.md"]
	require.True(t, document.Unresolved)
	require.Nil(t, document.Destination)
	require.NotContains(t, index.Vertices, "path:.notes/secret.md")
	// The console's source link falls back to copy-only on this diagnostic.
	var refused bool
	for _, diagnostic := range index.Diagnostics {
		if diagnostic.Code == "unresolved_document" && diagnostic.Source != nil && diagnostic.Source.File == ".notes/secret.md" {
			refused = true
		}
	}
	require.True(t, refused, "the unserved citation is reported, not silently dropped")
	var found bool
	for _, edge := range index.Adjacent("F-79dh3q", "sourced_from") {
		found = true
		require.Empty(t, edge.Sources[0].CitationPath)
		require.Equal(t, ".notes/secret.md#L1", edge.Sources[0].Raw)
	}
	require.True(t, found)
}

func TestRelationshipsDoesNotAdvertiseInvalidCitationLine(t *testing.T) {
	tree, _ := relationshipInputTree(t)
	tree.Features["F-79dh3q"].Source = "docs/source.md#L99"
	require.NoError(t, os.Mkdir(filepath.Join(tree.Root, "docs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, "docs/source.md"), []byte("only one line"), 0644))
	index, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true})
	require.NoError(t, err)
	var found bool
	for _, edge := range index.Edges {
		if edge.Kind == "sourced_from" && edge.From == "F-79dh3q" {
			found = true
			require.Empty(t, edge.Sources[0].CitationPath)
			require.Equal(t, 99, edge.Sources[0].CitationLine)
			require.Equal(t, "docs/source.md#L99", edge.Sources[0].Raw)
		}
	}
	require.True(t, found)
}

func TestRelationshipsMissingObservationWinsWithoutBorrowingContract(t *testing.T) {
	tree, _ := relationshipInputTree(t)
	index, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{
		EventsLoaded:    true,
		ContractMissing: map[string]bool{"F-79dh3q": true},
		ContractErrors:  map[string]string{"F-79dh3q": "file does not exist"},
		Contracts:       map[string]*store.EvalFile{"F-79dh3q": {Node: "F-foreign"}},
	})
	require.NoError(t, err)
	require.True(t, index.Complete)
	require.Empty(t, index.Diagnostics)
	require.Empty(t, index.Adjacent("F-79dh3q", "has_document"))
}

func TestRelationshipsDistinguishesDraftFromMalformedAndMisownedCorpusContracts(t *testing.T) {
	for _, tc := range []struct {
		name, content, code string
	}{
		{"missing", "", ""},
		{"draft", "---\nnode: F-79dh3q\ncriteria: []\n---\nF-zzzzzz\n", ""},
		{"malformed", "---\nnode: [\n---\n", "contract_unavailable"},
		{"misowned draft", "---\nnode: F-foreign\ncriteria: []\n---\n", "contract_owner_mismatch"},
		{"invalid criterion", "---\nnode: F-79dh3q\ncriteria:\n  - id: C1\n---\n", "contract_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, file := relationshipInputTree(t)
			if tc.content != "" {
				require.NoError(t, os.WriteFile(filepath.Join(tree.Root, file), []byte(tc.content), 0644))
			}
			index, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true})
			require.NoError(t, err)
			require.Equal(t, tc.code == "", index.Complete)
			var contractDiagnostics []relationships.Diagnostic
			for _, diagnostic := range index.Diagnostics {
				if diagnostic.Facet == "contracts" {
					contractDiagnostics = append(contractDiagnostics, diagnostic)
				}
			}
			if tc.code == "" {
				require.Empty(t, contractDiagnostics)
			} else {
				require.Len(t, contractDiagnostics, 1)
				require.Equal(t, tc.code, contractDiagnostics[0].Code)
				require.ElementsMatch(t, []string{"F-79dh3q", "doc:" + file}, contractDiagnostics[0].Subjects)
			}
			if tc.name == "draft" {
				require.Contains(t, index.Vertices, "F-zzzzzz")
				require.Len(t, index.Adjacent("F-79dh3q", "has_document"), 1)
			}
		})
	}
}

func TestRelationshipsDegradedWalkDiscardsPartialCorpusButRetainsSuppliedFacts(t *testing.T) {
	tree, file := relationshipInputTree(t)
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, ".tiller/a.md"), []byte("F-zzzzzz\n"), 0644))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(tree.Root, ".tiller/z-unsafe")))
	tree.Features["F-79dh3q"].Source = ".tiller/zz-unexamined.md#L2"
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, ".tiller/zz-unexamined.md"), []byte("present\nunexamined\n"), 0644))
	tree.Features["F-other"] = &store.Feature{ID: "F-other", Key: new("engine/other"), EpicKey: "engine"}
	supplied := &store.EvalFile{Node: "F-79dh3q", Criteria: []store.EvalCriterion{{ID: "C1", Statement: "works", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/store_test.go"}}}}}
	index, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{
		EventsLoaded: true, AllowIncompleteCorpus: true, Contracts: map[string]*store.EvalFile{"F-79dh3q": supplied},
	})
	require.NoError(t, err)
	require.False(t, index.Complete)
	require.NotContains(t, index.Vertices, "F-zzzzzz")
	require.NotContains(t, index.Vertices, "doc:.tiller/a.md")
	require.Contains(t, index.Vertices, "path:spec/store_test.go")
	require.Len(t, index.Adjacent("F-79dh3q", "contained_by"), 1)
	require.Len(t, index.Adjacent("F-79dh3q", "sourced_from"), 1)
	for _, id := range []string{"doc:.tiller/zz-unexamined.md", "doc:" + file} {
		require.True(t, index.Vertices[id].Unresolved)
		require.Nil(t, index.Vertices[id].Destination)
	}
	require.Len(t, index.Diagnostics, 1, "one global failure explains unexamined documents and contracts without false missing warnings")
	require.Equal(t, "corpus_unreadable", index.Diagnostics[0].Code)
	require.Empty(t, index.Diagnostics[0].Subjects)
	require.Empty(t, index.Adjacent("F-79dh3q", "sourced_from")[0].Sources[0].CitationPath)
}

func TestRelationshipsUnreadableCitationStaysUnresolvedAndAttributed(t *testing.T) {
	tree, _ := relationshipInputTree(t)
	tree.Features["F-79dh3q"].Source = "docs/source.md#L2"
	tree.Features["F-other"] = &store.Feature{ID: "F-other", Key: new("engine/other"), EpicKey: "engine", Source: "docs/source.md"}
	require.NoError(t, os.Mkdir(filepath.Join(tree.Root, "docs"), 0755))
	outside := filepath.Join(t.TempDir(), "unsafe.md")
	require.NoError(t, os.WriteFile(outside, []byte("F-zzzzzz\n"), 0644))
	require.NoError(t, os.Symlink(outside, filepath.Join(tree.Root, "docs/source.md")))
	options := readinput.RelationshipOptions{EventsLoaded: true, AllowIncompleteCorpus: true}
	index, err := readinput.Relationships(context.Background(), tree, options)
	require.NoError(t, err)
	require.False(t, index.Complete)
	require.True(t, index.Vertices["doc:docs/source.md"].Unresolved)
	require.Nil(t, index.Vertices["doc:docs/source.md"].Destination)
	require.NotContains(t, index.Vertices, "F-zzzzzz")
	require.Len(t, index.Diagnostics, 1)
	require.Equal(t, "document_unreadable", index.Diagnostics[0].Code)
	require.Equal(t, []string{"F-79dh3q", "F-other", "doc:docs/source.md"}, index.Diagnostics[0].Subjects)
	for _, edge := range index.Adjacent("F-79dh3q", "sourced_from") {
		require.Empty(t, edge.Sources[0].CitationPath)
	}
	options.AllowIncompleteCorpus = false
	index, err = readinput.Relationships(context.Background(), tree, options)
	require.Error(t, err)
	require.Nil(t, index)
}

func TestRelationshipsDegradationDoesNotBypassGenerationRefusal(t *testing.T) {
	tree, _ := relationshipInputTree(t)
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(tree.Root, ".tiller/unsafe")))
	require.NoError(t, os.Mkdir(filepath.Join(tree.Root, ".tiller/.cache"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, ".tiller/.cache/generation"), []byte("new-generation"), 0644))
	index, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true, AllowIncompleteCorpus: true})
	require.Nil(t, index)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)
}
