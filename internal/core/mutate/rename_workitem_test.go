package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestRenameBugTitleAtAnyStateAndKeyWhileProposed(t *testing.T) {
	t.Parallel()
	root := layBug(t, "alpha")
	res, err := mutate.RenameTitle(load(t, root), testDeps(), mutate.RenameTitleInput{Noun: store.NounBug, Node: "alpha", Title: "Alpha, retitled"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "Alpha, retitled", load(t, root).Bugs["B-aaaaaa"].Title)
	require.Equal(t, "rename.title.bug", lastEvent(t, root, "rename.title.bug").Type)

	moved, err := mutate.RenameKey(load(t, root), testDeps(), mutate.RenameKeyInput{Noun: store.NounBug, Node: "alpha", Key: "blank-on-reload"})
	require.NoError(t, err)
	require.Equal(t, "bugs/alpha", moved.OldKey)
	require.Equal(t, "bugs/blank-on-reload", moved.Key)
	require.NoDirExists(t, filepath.Join(root, ".tiller", "bugs", "alpha"))
	require.FileExists(t, filepath.Join(root, ".tiller", "bugs", "blank-on-reload", "bug.md"))
	b := load(t, root).Bugs["B-aaaaaa"]
	require.Equal(t, "bugs/blank-on-reload", *b.Key)
	require.Equal(t, "rename.key.bug", lastEvent(t, root, "rename.key.bug").Type)

	_, err = mutate.RenameKey(load(t, root), testDeps(), mutate.RenameKeyInput{Noun: store.NounBug, Node: "B-aaaaaa", Key: "bugs/other"})
	require.Error(t, err, "the new key is a local segment; the shelf prefix is implied")
	driveTo(t, root, store.NounBug, "B-aaaaaa", mutate.TransitionInput{Verb: "approve"})
	_, err = mutate.RenameKey(load(t, root), testDeps(), mutate.RenameKeyInput{Noun: store.NounBug, Node: "B-aaaaaa", Key: "other"})
	require.Error(t, err, "key moves stop at the proposal boundary")
	_, err = mutate.RenameTitle(load(t, root), testDeps(), mutate.RenameTitleInput{Noun: store.NounBug, Node: "B-aaaaaa", Title: "Still fine"})
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(root, ".tiller", "bugs", "blank-on-reload", "bug.yml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "title: Still fine")
}
