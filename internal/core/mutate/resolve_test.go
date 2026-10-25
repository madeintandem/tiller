package mutate_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestResolveSetsAndClearsTheCarrier(t *testing.T) {
	t.Parallel()
	root := layShelfPair(t)
	res, err := mutate.Resolve(load(t, root), testDeps(), mutate.ResolveInput{Noun: store.NounBug, Node: "alpha", By: "form-engine/loading"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "F-aaaaaa", res.ResolvedBy)
	tree := load(t, root)
	b := tree.Bugs["B-aaaaaa"]
	require.Equal(t, "F-aaaaaa", b.ResolvedBy)
	require.Equal(t, "proposed", b.Status, "resolve moves no status")
	require.Equal(t, gen.ActivityCarried, gen.WorkActivity(tree, b))
	ev := lastEvent(t, root, "resolve.bug")
	require.Equal(t, "F-aaaaaa", ev.After["resolved_by"])

	cleared, err := mutate.Resolve(load(t, root), testDeps(), mutate.ResolveInput{Noun: store.NounBug, Node: "alpha", Clear: true})
	require.NoError(t, err)
	require.Equal(t, "F-aaaaaa", cleared.Before)
	require.Equal(t, "", load(t, root).Bugs["B-aaaaaa"].ResolvedBy)
}

func TestResolveRefusals(t *testing.T) {
	t.Parallel()
	root := layShelfPair(t)
	node := mutate.ResolveInput{Noun: store.NounBug, Node: "alpha"}
	_, err := mutate.Resolve(load(t, root), testDeps(), node)
	require.Equal(t, "resolve_by_required", reasonOf(t, err))

	both := node
	both.By = "form-engine/loading"
	both.Clear = true
	_, err = mutate.Resolve(load(t, root), testDeps(), both)
	require.Equal(t, "bad_args", reasonOf(t, err))

	in := node
	in.By = "CH-aaaaaa"
	_, err = mutate.Resolve(load(t, root), testDeps(), in)
	require.Equal(t, "carrier_target", reasonOf(t, err))

	driveTo(t, root, store.NounFeature, "form-engine/loading", mutate.TransitionInput{Verb: "cancel", Reason: "dropped"})
	in.By = "F-aaaaaa"
	_, err = mutate.Resolve(load(t, root), testDeps(), in)
	require.Equal(t, "carrier_retired", reasonOf(t, err))

	driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "cancel", Reason: "no", Resolution: "wontfix"})
	in.By = "form-engine"
	_, err = mutate.Resolve(load(t, root), testDeps(), in)
	var terminal *mutate.WorkTerminalError
	require.ErrorAs(t, err, &terminal)
}
