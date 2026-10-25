package readinput_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const relationshipContractFixture = "---\nnode: F-79dh3q\ncriteria:\n  - id: C1\n    statement: works\n    binding:\n      type: test\n      ref: src/one_test.go\n---\nF-aaaaaa\n"

func TestRelationshipsCachedParsesObserveSameMetadataEdits(t *testing.T) {
	tree, file := relationshipInputTree(t)
	physical := filepath.Join(tree.Root, file)
	before := []byte(relationshipContractFixture)
	require.NoError(t, os.WriteFile(physical, before, 0644))
	info, err := os.Stat(physical)
	require.NoError(t, err)
	options := readinput.RelationshipOptions{EventsLoaded: true}
	first, err := readinput.Relationships(context.Background(), tree, options)
	require.NoError(t, err)
	second, err := readinput.Relationships(context.Background(), tree, options)
	require.NoError(t, err)
	require.Equal(t, first, second, "unchanged bytes produce the same request-local index")
	after := []byte(strings.ReplaceAll(strings.ReplaceAll(string(before), "one_test.go", "two_test.go"), "F-aaaaaa", "F-bbbbbb"))
	require.Len(t, after, len(before))
	require.NoError(t, os.WriteFile(physical, after, 0644))
	require.NoError(t, os.Chtimes(physical, info.ModTime(), info.ModTime()))
	changed, err := readinput.Relationships(context.Background(), tree, options)
	require.NoError(t, err)
	require.Contains(t, changed.Vertices, "path:src/two_test.go")
	require.NotContains(t, changed.Vertices, "path:src/one_test.go")
	require.Contains(t, changed.Vertices, "F-bbbbbb")
	require.NotContains(t, changed.Vertices, "F-aaaaaa")
}

func TestRelationshipsCachedContractFailureRepairsAndDeletionRemainsFresh(t *testing.T) {
	tree, file := relationshipInputTree(t)
	physical := filepath.Join(tree.Root, file)
	require.NoError(t, os.WriteFile(physical, []byte("---\nnode: [\n---\n"), 0644))
	options := readinput.RelationshipOptions{EventsLoaded: true}
	for range 2 {
		broken, err := readinput.Relationships(context.Background(), tree, options)
		require.NoError(t, err)
		require.False(t, broken.Complete)
		require.Equal(t, "contract_unavailable", broken.DiagnosticsFor("F-79dh3q")[0].Code)
	}
	require.NoError(t, os.WriteFile(physical, []byte(relationshipContractFixture), 0644))
	repaired, err := readinput.Relationships(context.Background(), tree, options)
	require.NoError(t, err)
	require.True(t, repaired.Complete)
	require.Contains(t, repaired.Vertices, "path:src/one_test.go")
	require.NoError(t, os.Remove(physical))
	deleted, err := readinput.Relationships(context.Background(), tree, options)
	require.NoError(t, err)
	require.True(t, deleted.Complete)
	require.NotContains(t, deleted.Vertices, "path:src/one_test.go")
	require.NotContains(t, deleted.Vertices, "F-aaaaaa")
	require.NotContains(t, deleted.Vertices, relationships.DocumentID(file))
}

func TestRelationshipsCachedContractsRecheckCurrentOwnerAndPath(t *testing.T) {
	tree, file := relationshipInputTree(t)
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, file), []byte(relationshipContractFixture), 0644))
	options := readinput.RelationshipOptions{EventsLoaded: true}
	warm, err := readinput.Relationships(context.Background(), tree, options)
	require.NoError(t, err)
	require.True(t, warm.Complete)
	delete(tree.Features, "F-79dh3q")
	tree.Features["F-aaaaaa"] = &store.Feature{ID: "F-aaaaaa", Key: new("engine/moved"), EpicKey: "engine"}
	moved := ".tiller/epics/engine/features/moved/feature.eval.md"
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(tree.Root, moved)), 0755))
	require.NoError(t, os.Rename(filepath.Join(tree.Root, file), filepath.Join(tree.Root, moved)))
	current, err := readinput.Relationships(context.Background(), tree, options)
	require.NoError(t, err)
	require.False(t, current.Complete, "cached parsing never grants a former owner's contract to the current owner")
	require.Equal(t, "F-aaaaaa", current.Vertices[relationships.DocumentID(moved)].DocumentOwner)
	require.False(t, current.Vertices["F-aaaaaa"].Unresolved, "the mention resolves against the current tree")
	require.NotContains(t, current.Vertices, relationships.DocumentID(file))
	require.NotContains(t, current.Vertices, "path:src/one_test.go")
	var mismatch bool
	for _, diagnostic := range current.Diagnostics {
		if diagnostic.Code == "contract_owner_mismatch" {
			mismatch = true
			require.Equal(t, moved, diagnostic.Source.File)
			require.ElementsMatch(t, []string{"F-aaaaaa", relationships.DocumentID(moved)}, diagnostic.Subjects)
		}
	}
	require.True(t, mismatch)
}

func TestRelationshipsSuppliedObservationsNeverEnterContentCache(t *testing.T) {
	tree, file := relationshipInputTree(t)
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, file), []byte(relationshipContractFixture), 0644))
	warm, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true})
	require.NoError(t, err)
	require.Contains(t, warm.Vertices, "path:src/one_test.go")
	supplied := &store.EvalFile{Node: "F-79dh3q", Criteria: []store.EvalCriterion{{ID: "C1", Statement: "supplied", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"src/supplied.go"}}}}}
	observed, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true, Contracts: map[string]*store.EvalFile{"F-79dh3q": supplied}})
	require.NoError(t, err)
	require.Contains(t, observed.Vertices, "path:src/supplied.go")
	require.NotContains(t, observed.Vertices, "path:src/one_test.go")
	supplied.Criteria[0].Binding.Ref[0] = "src/mutated.go"
	unsupplied, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true})
	require.NoError(t, err)
	require.Contains(t, unsupplied.Vertices, "path:src/one_test.go")
	require.NotContains(t, unsupplied.Vertices, "path:src/mutated.go")
	failed, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true, ContractErrors: map[string]string{"F-79dh3q": "observed failure"}})
	require.NoError(t, err)
	require.False(t, failed.Complete)
	missing, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true, ContractMissing: map[string]bool{"F-79dh3q": true}})
	require.NoError(t, err)
	require.True(t, missing.Complete)
	require.NotContains(t, missing.Vertices, "path:src/one_test.go")
	restored, err := readinput.Relationships(context.Background(), tree, readinput.RelationshipOptions{EventsLoaded: true})
	require.NoError(t, err)
	require.Contains(t, restored.Vertices, "path:src/one_test.go")
}

func TestRelationshipsConcurrentReadersReturnIndependentIndexes(t *testing.T) {
	tree, file := relationshipInputTree(t)
	require.NoError(t, os.WriteFile(filepath.Join(tree.Root, file), []byte(relationshipContractFixture), 0644))
	options := readinput.RelationshipOptions{EventsLoaded: true}
	warm, err := readinput.Relationships(context.Background(), tree, options)
	require.NoError(t, err)
	var readers sync.WaitGroup
	for range 12 {
		readers.Go(func() {
			for range 4 {
				index, err := readinput.Relationships(context.Background(), tree, options)
				if !assert.NoError(t, err) || !assert.NotNil(t, index) {
					return
				}
				assert.Equal(t, warm, index)
				clear(index.Vertices)
				clear(index.Edges)
			}
		})
	}
	readers.Wait()
}
