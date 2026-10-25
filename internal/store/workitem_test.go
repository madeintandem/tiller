package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShelfHelpers(t *testing.T) {
	require.Equal(t, "bugs", ShelfDir(NounBug))
	require.Equal(t, "chores", ShelfDir(NounChore))
	require.Equal(t, "bug.md", ShelfProseFile(NounBug))
	require.Equal(t, "chore.yml", ShelfStateFile(NounChore))
	require.Equal(t, filepath.Join(".tiller", "bugs", "blank-on-reload"), ShelfRelDir(NounBug, "blank-on-reload"))
	require.Equal(t, "chores/bump-rails", ShelfCanonicalKey(NounChore, "bump-rails"))
	require.Equal(t, filepath.Join(".tiller", "bugs", "order.yml"), ShelfOrderRelPath(NounBug))
	require.True(t, IsWorkNoun(NounBug))
	require.True(t, IsWorkNoun(NounChore))
	require.False(t, IsWorkNoun(NounTask))
	require.True(t, IsReservedEpicKey("bugs"))
	require.True(t, IsReservedEpicKey("chores"))
	require.True(t, ValidResolution(ResolutionWontfix))
	require.True(t, ValidResolution(ResolutionDuplicate))
	require.False(t, ValidResolution("fixed"))
}

func TestWorkNodeShape(t *testing.T) {
	key := "bugs/blank-on-reload"
	b := &Bug{WorkItem: WorkItem{ID: "B-aaaaaa", Key: &key, Status: "ready"}}
	require.Equal(t, NounBug, b.Noun())
	require.Equal(t, "B-aaaaaa", b.NodeID())
	require.Equal(t, "ready", b.NodeStatus())
	w, ok := AsWorkNode(b)
	require.True(t, ok)
	require.Same(t, &b.WorkItem, w.Body())
	_, ok = AsWorkNode(&Task{})
	require.False(t, ok)
	c, err := NewWorkNode(NounChore)
	require.NoError(t, err)
	require.Equal(t, NounChore, c.Noun())
	_, err = NewWorkNode(NounTask)
	require.Error(t, err)
}

func TestLoadReadsBothShelves(t *testing.T) {
	tree, err := Load(fixtureTreeRoot)
	require.NoError(t, err)
	require.Len(t, tree.Bugs, 1)
	require.Len(t, tree.Chores, 1)
	b := tree.Bugs["B-7k2p9x"]
	require.NotNil(t, b)
	require.Equal(t, "bugs/blank-on-reload", *b.Key)
	require.Equal(t, "ready", b.Status)
	require.Equal(t, "F-79dh3q", b.AttachedTo)
	require.Equal(t, []string{"CH-4m2x9q"}, b.DependsOn)
	require.NotNil(t, b.AcceptedAgainst)
	require.Equal(t, "git_blob", b.AcceptedAgainst.Spec.Kind)
	c := tree.Chores["CH-4m2x9q"]
	require.NotNil(t, c)
	require.Equal(t, "chores/bump-rails", *c.Key)
	require.Equal(t, []string{"B-7k2p9x"}, tree.BugOrder.Order)
	require.Empty(t, tree.ChoreOrder.Order)
	w, ok := tree.WorkNode("CH-4m2x9q")
	require.True(t, ok)
	require.Equal(t, NounChore, w.Noun())
	require.Len(t, tree.WorkNodesOf(NounBug), 1)
	// The rest of the fixture is untouched by the shelves.
	require.Len(t, tree.Tasks, 1)
}

func TestLoadRefusesShelfKeyOutsideItsShelf(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	p := filepath.Join(root, ".tiller", "bugs", "blank-on-reload", "bug.yml")
	writeFile(t, p, "id: B-7k2p9x\nkey: blank-on-reload\nstatus: ready\nblocked: false\n")
	_, err := Load(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be canonical under bugs/")
}

func TestLoadRefusesBadResolution(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	p := filepath.Join(root, ".tiller", "bugs", "blank-on-reload", "bug.yml")
	writeFile(t, p, "id: B-7k2p9x\nkey: bugs/blank-on-reload\nstatus: cancelled\nblocked: false\nresolution: fixed\n")
	_, err := Load(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolution")
}
