package views_test

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// TestNodeCarriesBasisCommitDoneBasisAndProposedBy pins the three facts the
// node header pass (2026-08-30) put on the wire. The store already recorded
// every one of them: the basis commit (HEAD at approval, HEAD at done) that
// BasisOf and VerifiedAgainstOf used to drop, the done_basis a sync crossing
// writes, and the proposed_by an add stamps on a task or a shelf node. The
// commit is the one sha on a node page GitHub can actually show (the spec and
// eval pins are blob fingerprints), done_basis answers "done how?" beside the
// state, and proposed_by is the only person a proposed shelf node has.
func TestNodeCarriesBasisCommitDoneBasisAndProposedBy(t *testing.T) {
	featureKey := "habit-engine/swiftdata-model"
	f := &store.Feature{
		ID: "F-n7oy52", Key: &featureKey, Status: "done", DoneBasis: "rollup",
		AcceptedAgainst: store.BasisRefs{
			Spec:   store.RevisionRef{Kind: "git_blob", Value: "git:bedf333"},
			Commit: store.RevisionRef{Kind: "git_commit", Value: "git:7458e30"},
		},
		VerifiedAgainst: store.VerifiedAgainst{
			Spec:   store.RevisionRef{Kind: "git_blob", Value: "git:bedf333"},
			Commit: store.RevisionRef{Kind: "git_commit", Value: "git:828a8b0"},
		},
	}
	fv := views.Node(&store.Tree{}, f)
	require.Equal(t, "git:7458e30", fv.AcceptedAgainst.Commit)
	require.Equal(t, "git:828a8b0", fv.VerifiedAgainst.Commit)
	require.Equal(t, "rollup", fv.DoneBasis)
	fRaw, err := json.Marshal(fv)
	require.NoError(t, err)
	require.Contains(t, string(fRaw), `"commit":"git:7458e30"`)
	require.Contains(t, string(fRaw), `"commit":"git:828a8b0"`)
	require.Contains(t, string(fRaw), `"done_basis":"rollup"`)

	taskKey := "habit-engine/swiftdata-model/package-foundation"
	tk := &store.Task{ID: "T-tj6hor", Key: &taskKey, Status: "done", ProposedBy: "jc", DoneBasis: "ancestry"}
	tv := views.Node(&store.Tree{}, tk)
	require.Equal(t, "jc", tv.ProposedBy)
	require.Equal(t, "ancestry", tv.DoneBasis)
	tRaw, err := json.Marshal(tv)
	require.NoError(t, err)
	require.Contains(t, string(tRaw), `"proposed_by":"jc"`)
	require.Contains(t, string(tRaw), `"done_basis":"ancestry"`)

	epicKey := "habit-engine"
	e := &store.Epic{ID: "E-tslnrm", Key: &epicKey, Status: "done", DoneBasis: "rollup"}
	require.Equal(t, "rollup", views.Node(&store.Tree{}, e).DoneBasis)

	bugKey := "bugs/journal-nav"
	b := &store.Bug{}
	b.ID = "B-8lpm8p"
	b.Key = &bugKey
	b.Status = "proposed"
	b.ProposedBy = "jc"
	bv := views.Node(&store.Tree{}, b)
	require.Equal(t, "jc", bv.ProposedBy)

	// A fresh draft carries none of the three, and the keys stay absent.
	draftKey := "x/y"
	dv := views.Node(&store.Tree{}, &store.Feature{ID: "F-000000", Key: &draftKey, Status: "draft"})
	dRaw, err := json.Marshal(dv)
	require.NoError(t, err)
	require.NotContains(t, string(dRaw), `"done_basis"`)
	require.NotContains(t, string(dRaw), `"proposed_by"`)
	require.NotContains(t, string(dRaw), `"commit"`)
}

// TestBasisOfKeepsACommitOnlyPin pins the nil guard against the struct it
// guards: a basis that records only its commit is still a basis, not an
// absent block. No writer produces one today (approve always pins the spec),
// so this is the guard staying honest rather than a live case.
func TestBasisOfKeepsACommitOnlyPin(t *testing.T) {
	v := views.BasisOf(store.BasisRefs{Commit: store.RevisionRef{Kind: "git_commit", Value: "git:7458e30"}})
	require.NotNil(t, v)
	require.Equal(t, "git:7458e30", v.Commit)
	require.Nil(t, views.BasisOf(store.BasisRefs{}))
}
