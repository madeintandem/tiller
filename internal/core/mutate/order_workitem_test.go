package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func layThreeBugs(t *testing.T) string {
	t.Helper()
	root := writeOwnedRepo(t)
	deps := testDeps()
	// The shared sequenceMinter serves every prefix from one list, EV
	// included, so a plain sequenceMinter("aaaaaa", "bbbbbb", "cccccc")
	// would let each add's own EV mint eat into the sequence meant for the
	// next bug. Reserving the sequence to the "B" prefix alone is the
	// prefix-keyed closure transition_workitem_test.go's layBug already
	// uses for the identical reason.
	bugIDs := sequenceMinter("aaaaaa", "bbbbbb", "cccccc")
	deps.Mint = func(prefix string) string {
		if prefix == "B" {
			return bugIDs(prefix)
		}
		return fixedMint(prefix)
	}
	for _, k := range []string{"alpha", "bravo", "charlie"} {
		_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: k, Title: k})
		require.NoError(t, err)
	}
	return root
}

func TestOrderBugsWritesShelfOrderFile(t *testing.T) {
	t.Parallel()
	root := layThreeBugs(t)
	res, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{Noun: store.NounBug, Nodes: []string{"charlie", "B-aaaaaa"}})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "", res.Parent)
	require.Equal(t, []string{"B-cccccc", "B-aaaaaa"}, res.Order)
	raw, err := os.ReadFile(filepath.Join(root, ".tiller", "bugs", "order.yml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "- B-cccccc")
	require.Equal(t, []string{"B-cccccc", "B-aaaaaa"}, load(t, root).BugOrder.Order)
	require.Equal(t, "order.bugs", lastEvent(t, root, "order.bugs").Type)

	// Ranked reads follow the list where edges leave the choice open.
	tree := load(t, root)
	ranked := gen.OrderWorkNodes(tree, tree.WorkNodesOf(store.NounBug), tree.ShelfOrderOf(store.NounBug))
	require.Equal(t, "B-cccccc", ranked[0].NodeID())

	cleared, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{Noun: store.NounBug, Clear: true})
	require.NoError(t, err)
	require.True(t, cleared.Changed)
	require.Empty(t, load(t, root).BugOrder.Order)
}

func TestOrderBugsRefusesMixedShelvesAndParentFlags(t *testing.T) {
	t.Parallel()
	root := layThreeBugs(t)
	_, err := mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{Noun: store.NounBug, Clear: true, ParentEpic: "x"})
	require.Error(t, err)
	deps := testDeps()
	deps.Mint = func(string) string { return "CH-aaaaaa" }
	_, err = mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounChore, Key: "bump", Title: "Bump"})
	require.NoError(t, err)
	_, err = mutate.SetOrder(load(t, root), testDeps(), mutate.SetOrderInput{Noun: store.NounBug, Nodes: []string{"alpha", "CH-aaaaaa"}})
	require.Error(t, err, "a chore is not a bug; the resolver's noun mismatch refuses")
}
