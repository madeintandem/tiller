package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// draftSourceFixture is sourceFixture before the hand-over: an intake-minted
// amendment still in draft, its Source matching the artifact on disk.
func draftSourceFixture(t *testing.T, body string) (root string, deps mutate.Deps, a *store.Amendment, relPath string) {
	t.Helper()
	root = writeOwnedRepo(t)
	deps = gitDeps(root)
	relPath = filepath.Join(".tiller", "feedback", "review.md")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "feedback"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, relPath), []byte(body), 0o644))
	res, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{
		Key: "review-intake", Title: "Intake: review.md",
		Source: &store.AmendmentSource{Path: relPath, Revision: blobOf(t, root, relPath)},
	})
	require.NoError(t, err)
	return root, deps, loadAmendment(t, root, res.ID), relPath
}

func proposedEvents(t *testing.T, root string) int {
	t.Helper()
	n := 0
	for _, ev := range listEvents(t, root) {
		if ev.Type == "amendment_proposed" {
			n++
		}
	}
	return n
}

func TestProposeMovesDraftToProposed(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	res, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{Key: "drop-ipad", Title: "Drop iPad"})
	require.NoError(t, err)
	a := loadAmendment(t, root, res.ID)
	require.Equal(t, store.AmendmentStatusDraft, a.Status)
	a = stage(t, root, deps, a, mutate.StageInput{Op: store.AmendmentOpCancel, Target: featureID, Reason: "out of scope"})

	out, err := mutate.ProposeAmendment(load(t, root), deps, a)
	require.NoError(t, err)
	require.Equal(t, store.AmendmentStatusProposed, out.Status)
	require.Equal(t, res.ID, out.ID)
	require.Equal(t, "drop-ipad", out.Key)
	require.False(t, out.Empty)
	require.Equal(t, store.AmendmentStatusProposed, loadAmendment(t, root, res.ID).Status)

	var found int
	for _, ev := range listEvents(t, root) {
		if ev.Type != "amendment_proposed" {
			continue
		}
		found++
		require.Equal(t, store.AmendmentStatusProposed, ev.After["status"])
		require.Equal(t, false, ev.After["empty"])
	}
	require.Equal(t, 1, found)
}

func TestProposeIsIdempotent(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	res, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{Key: "drop-ipad", Title: "Drop iPad"})
	require.NoError(t, err)
	_, err = mutate.ProposeAmendment(load(t, root), deps, loadAmendment(t, root, res.ID))
	require.NoError(t, err)

	before := snapshotDir(t, root)
	out, err := mutate.ProposeAmendment(load(t, root), deps, loadAmendment(t, root, res.ID))
	require.NoError(t, err)
	require.Equal(t, store.AmendmentStatusProposed, out.Status)
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, 1, proposedEvents(t, root))
}

func TestProposeRefusesATerminalAmendment(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	res, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{Key: "drop-ipad", Title: "Drop iPad"})
	require.NoError(t, err)
	setAmendmentStatus(t, root, res.ID, store.AmendmentStatusWithdrawn)
	_, err = mutate.ProposeAmendment(load(t, root), deps, loadAmendment(t, root, res.ID))
	require.Equal(t, "amendment_not_open", reasonOf(t, err))
}

func TestProposeRefusesStaleSource(t *testing.T) {
	t.Parallel()
	root, deps, a, relPath := draftSourceFixture(t, "the review body\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, relPath), []byte("the EDITED review body\n"), 0o644))

	before := snapshotDir(t, root)
	_, err := mutate.ProposeAmendment(load(t, root), deps, a)
	require.Equal(t, "source_changed", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusDraft, loadAmendment(t, root, a.ID).Status)
}

func TestProposeRefusesMissingCitation(t *testing.T) {
	t.Parallel()
	root, deps, a, _ := draftSourceFixture(t, "the review body\n")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "billing", Title: "Billing", Source: "review.md#L1-2",
	})
	ops := append([]store.AmendmentOp(nil), a.Operations...)
	ops[0].Source = ""
	a = writeAmendmentOps(t, root, a, ops)

	before := snapshotDir(t, root)
	_, err := mutate.ProposeAmendment(load(t, root), deps, a)
	require.Equal(t, "op_source_required", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusDraft, loadAmendment(t, root, a.ID).Status)
}

func TestProposeWarnsOnStalePrecondition(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	res, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{Key: "drop-ipad", Title: "Drop iPad"})
	require.NoError(t, err)
	a := loadAmendment(t, root, res.ID)
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: featureID, Reason: "out of scope",
		Preconditions: map[string]string{"target_status": "done"}})

	out, err := mutate.ProposeAmendment(load(t, root), deps, a)
	require.NoError(t, err)
	require.Equal(t, store.AmendmentStatusProposed, out.Status)
	require.Len(t, out.Warnings, 1)
	require.Contains(t, out.Warnings[0], "operation 0")
}

func TestProposeAcceptsEmpty(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	res, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{Key: "nothing", Title: "Nothing"})
	require.NoError(t, err)
	out, err := mutate.ProposeAmendment(load(t, root), deps, loadAmendment(t, root, res.ID))
	require.NoError(t, err)
	require.True(t, out.Empty)
	for _, ev := range listEvents(t, root) {
		if ev.Type == "amendment_proposed" {
			require.Equal(t, true, ev.After["empty"])
		}
	}
}

func TestApproveRefusesEmpty(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	a := openAmendment(t, root, deps, "nothing")
	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Equal(t, "amendment_empty", reasonOf(t, err))
	require.EqualError(t, err, "amendment "+a.ID+" proposes nothing; drop it: tiller withdraw amendment nothing --reason \"<why>\"")
	requireNoWrites(t, before, snapshotDir(t, root))
}
