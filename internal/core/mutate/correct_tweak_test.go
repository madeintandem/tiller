package mutate_test

import (
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestTweakOrderRenameAndRemovalPreserveIdentityAndScrubReferences(t *testing.T) {
	t.Parallel()
	root := layTweak(t, "alpha")
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "TW" {
			return "TW-bbbbbb"
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounTweak, Key: "dependent", Title: "Dependent", DependsOn: []string{"TW-aaaaaa"}})
	require.NoError(t, err)
	_, err = mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{Noun: store.NounTweak, Nodes: []string{"dependent", "alpha"}})
	require.NoError(t, err)
	require.Equal(t, []string{"TW-bbbbbb", "TW-aaaaaa"}, load(t, root).TweakOrder.Order)
	require.Equal(t, "order.tweaks", lastEvent(t, root, "order.tweaks").Type)
	_, err = mutate.RenameKey(load(t, root), testDeps(), mutate.RenameKeyInput{Noun: store.NounTweak, Node: "alpha", Key: "adjusted"})
	require.NoError(t, err)
	require.Equal(t, "tweaks/adjusted", *load(t, root).Tweaks["TW-aaaaaa"].Key)
	require.NoDirExists(t, filepath.Join(root, ".tiller/tweaks/alpha"))
	require.FileExists(t, filepath.Join(root, ".tiller/tweaks/adjusted/tweak.md"))
	_, err = mutate.RenameKey(load(t, root), testDeps(), mutate.RenameKeyInput{Noun: store.NounTweak, Node: "dependent", Key: "adjusted"})
	require.Error(t, err, "an existing tweak key must not be overwritten")
	_, err = mutate.RemoveDraft(load(t, root), testDeps(), mutate.RemoveInput{Noun: store.NounTweak, Node: "adjusted", Reason: "unneeded"})
	require.NoError(t, err)
	tree := load(t, root)
	require.NotContains(t, tree.Tweaks, "TW-aaaaaa")
	require.Empty(t, tree.Tweaks["TW-bbbbbb"].DependsOn)
	require.Equal(t, []string{"TW-bbbbbb"}, tree.TweakOrder.Order)
	require.NoDirExists(t, filepath.Join(root, ".tiller/tweaks/adjusted"))
	require.Equal(t, "remove.proposed.tweak", lastEvent(t, root, "remove.proposed.tweak").Type)
	_, err = mutate.SetOrder(tree, testDeps(), mutate.SetOrderInput{Noun: store.NounTweak, Clear: true})
	require.NoError(t, err)
	require.Empty(t, load(t, root).TweakOrder.Order)
}
