package mutate_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func layTwoOwnerBug(t *testing.T) string {
	t.Helper()
	root := writeTwoOwnerRepo(t)
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "B" {
			return "B-aaaaaa"
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "alpha", Title: "A"})
	require.NoError(t, err)
	return root
}

func TestClaimReleaseReassignBug(t *testing.T) {
	t.Parallel()
	root := layTwoOwnerBug(t)
	res, err := mutate.ClaimWork(load(t, root), testDeps(), store.NounBug, "alpha", "")
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "bug", res.Noun)
	require.Equal(t, "jc", res.Owner)
	require.Equal(t, "jc", load(t, root).Bugs["B-aaaaaa"].Owner)
	require.Equal(t, "claim.bug", lastEvent(t, root, "claim.bug").Type)

	again, err := mutate.ClaimWork(load(t, root), testDeps(), store.NounBug, "B-aaaaaa", "")
	require.NoError(t, err)
	require.False(t, again.Changed)

	pat := testDeps()
	pat.Contributor = "pat"
	_, err = mutate.ClaimWork(load(t, root), pat, store.NounBug, "alpha", "")
	var held *mutate.ClaimHeldError
	require.ErrorAs(t, err, &held)
	require.Equal(t, store.NounBug, held.Noun)
	take, err := mutate.ClaimWork(load(t, root), pat, store.NounBug, "alpha", "jc")
	require.NoError(t, err)
	require.True(t, take.Takeover)
	require.Equal(t, "jc", take.From)

	_, err = mutate.ReleaseWork(load(t, root), testDeps(), store.NounBug, "alpha")
	var notClaimant *mutate.NotClaimantError
	require.ErrorAs(t, err, &notClaimant)
	rel, err := mutate.ReleaseWork(load(t, root), pat, store.NounBug, "alpha")
	require.NoError(t, err)
	require.Equal(t, "pat", rel.From)
	require.Equal(t, "", load(t, root).Bugs["B-aaaaaa"].Owner)

	re, err := mutate.ReassignWork(load(t, root), testDeps(), store.NounBug, "alpha", "pat", "pat owns the form")
	require.NoError(t, err)
	require.Equal(t, "pat", re.To)
	require.Equal(t, "pat", load(t, root).Bugs["B-aaaaaa"].Owner)
}

func TestClaimWorkRefusesWrongNoun(t *testing.T) {
	t.Parallel()
	root := layTwoOwnerBug(t)
	_, err := mutate.ClaimWork(load(t, root), testDeps(), store.NounChore, "B-aaaaaa", "")
	require.Error(t, err)
}
