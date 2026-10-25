package mutate_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestAttestBugGateWritesTheShelfNode(t *testing.T) {
	t.Parallel()
	root := layBug(t, "blank-on-reload")
	tree := load(t, root)
	bug := tree.Bugs["B-aaaaaa"]

	res, err := mutate.Attest(tree, testDeps(), bug, mutate.AttestInput{
		Gate: "code_review", Status: store.GateStatusPass,
	})
	require.NoError(t, err)
	require.Equal(t, store.NounBug, res.Noun)
	require.Equal(t, "code_review", res.TargetID)
	require.Equal(t, []string{`gate "code_review" is not declared in tiller.yml, which declares no attestable gates; recorded anyway`}, res.Warnings,
		"a manifest with no attestable gates still records the attestation, with a warning")

	saved := load(t, root).Bugs["B-aaaaaa"]
	require.Equal(t, store.GateStatusPass, saved.Gates["code_review"].Status)
	require.Equal(t, store.GateKindAttestation, saved.Gates["code_review"].Kind)
	require.Len(t, saved.Attestations, 1)
	require.Equal(t, "gate", saved.Attestations[0].Target.Kind)
}

func TestAttestBugCriterionIsUnsupportedAndWritesNothing(t *testing.T) {
	t.Parallel()
	root := layBug(t, "blank-on-reload")
	tree := load(t, root)
	before, err := events.List(tree)
	require.NoError(t, err)

	_, err = mutate.Attest(tree, testDeps(), tree.Bugs["B-aaaaaa"], mutate.AttestInput{
		Criterion: "C1", Status: store.GateStatusPass,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "criterion")
	require.Contains(t, err.Error(), "bug")
	after, listErr := events.List(load(t, root))
	require.NoError(t, listErr)
	require.Len(t, after, len(before))
	require.Empty(t, load(t, root).Bugs["B-aaaaaa"].Attestations)
}
