package mutate_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests cover the recovery contract for an approval cut short: a cancel,
// a correct-eval, or a move-feature that landed under an amendment still
// reading proposed is recognised on the re-approve by its state plus the
// event the first run recorded for it, and skipped rather than refused.

// interruptBeforeStatusWrite approves the amendment and then puts its status
// back to proposed. The status write is the last write in the transaction,
// after every node file and every event file, so this is exactly the tree a
// process killed just before that last write leaves behind.
func interruptBeforeStatusWrite(t *testing.T, root string, deps mutate.Deps, a *store.Amendment) *store.Amendment {
	t.Helper()
	approve(t, root, deps, a)
	setAmendmentStatus(t, root, a.ID, store.AmendmentStatusProposed)
	return loadAmendment(t, root, a.ID)
}

// changedPaths names every path that differs between two snapshotDir
// fingerprints.
func changedPaths(before, after map[string]string) []string {
	var changed []string
	for rel, fingerprint := range after {
		if prior, known := before[rel]; !known || prior != fingerprint {
			changed = append(changed, rel)
		}
	}
	for rel := range before {
		if _, ok := after[rel]; !ok {
			changed = append(changed, rel)
		}
	}
	sort.Strings(changed)
	return changed
}

func amendmentRel(t *testing.T, a *store.Amendment) string {
	t.Helper()
	return filepath.Join(".tiller", "amendments", a.ID+"-"+deref(t, a.Key)+".yml")
}

func eventsRecorded(t *testing.T, root, typ string) int {
	t.Helper()
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	return len(eventsOfType(evs, typ))
}

// removeEvents deletes every recorded event of one type, leaving a node in its
// landed state with nothing in the log saying this amendment put it there.
func removeEvents(t *testing.T, root, typ string) {
	t.Helper()
	located, err := events.ListLocated(load(t, root))
	require.NoError(t, err)
	removed := 0
	for _, l := range located {
		if l.Event.Type == typ {
			require.NoError(t, os.Remove(filepath.Join(root, filepath.FromSlash(l.Path))))
			removed++
		}
	}
	require.Positive(t, removed, "the fixture must have recorded a %s event to remove", typ)
}

// requireFinishedWithoutRework re-approves a half-applied amendment and
// requires that the run left the tree as it was apart from the amendment's own
// status, reported the one operation as already applied, and left one event of
// the operation's type and one approval event in the log.
func requireFinishedWithoutRework(t *testing.T, root string, deps mutate.Deps, a *store.Amendment, eventType string) mutate.AppliedOperation {
	t.Helper()
	before := snapshotDir(t, root)
	res := approve(t, root, deps, a)

	require.Equal(t, store.AmendmentStatusApproved, res.Status)
	require.Equal(t, store.AmendmentStatusApproved, loadAmendment(t, root, a.ID).Status)
	// Every committed mutation advances the generation counter; beside it the
	// second run writes the amendment's status and nothing else.
	require.Equal(t, []string{filepath.Join(".tiller", ".cache", "generation"), amendmentRel(t, a)},
		changedPaths(before, snapshotDir(t, root)))
	require.Equal(t, 1, eventsRecorded(t, root, eventType), "no duplicate operation event")
	require.Equal(t, 1, eventsRecorded(t, root, "amendment_approved"), "no duplicate approval event")

	require.Len(t, res.Applied, 1)
	require.Equal(t, mutate.AppliedAlready, res.Applied[0].Outcome)
	return res.Applied[0]
}

func requireStillRefuses(t *testing.T, root string, deps mutate.Deps, a *store.Amendment) error {
	t.Helper()
	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Error(t, err)
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusProposed, loadAmendment(t, root, a.ID).Status)
	return err
}

func TestReapproveSkipsACancelThisAmendmentAlreadyApplied(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	// The guard the first run consumed: the target is no longer at this status.
	status := load(t, root).Tasks[taskID].Status
	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "no longer wanted",
		Preconditions: map[string]string{"target_status": status}})
	a = interruptBeforeStatusWrite(t, root, deps, a)

	op := requireFinishedWithoutRework(t, root, deps, a, "cancel.task")
	require.Equal(t, taskID, op.NodeID)
	require.Equal(t, "cancelled", op.Status)
	require.Equal(t, "cancelled", load(t, root).Tasks[taskID].Status)
}

func TestReapproveRefusesACancelledTargetWithoutThisAmendmentsEvent(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "no longer wanted"})
	a = interruptBeforeStatusWrite(t, root, deps, a)
	removeEvents(t, root, "cancel.task")

	err := requireStillRefuses(t, root, deps, a)
	require.Equal(t, "already_cancelled", reasonOf(t, err))
}

// A cancel event another amendment recorded is not this amendment's evidence.
func TestReapproveRefusesATargetAnotherAmendmentCancelled(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "iphone-only")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "no longer wanted"})

	other := openAmendment(t, root, deps, "drop-ipad")
	other = stage(t, root, deps, other, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "dropped elsewhere"})
	approve(t, root, deps, other)

	err := requireStillRefuses(t, root, deps, a)
	require.Equal(t, "already_cancelled", reasonOf(t, err))
}

func TestReapproveSkipsACorrectEvalThisAmendmentAlreadyApplied(t *testing.T) {
	t.Parallel()
	root, deps, featureID := correctableFixture(t)
	writeEval(t, root, verifiedEvalMD(featureID, "exit 0"))
	a := stageCorrection(t, root, deps, "eval-bindings")
	a = interruptBeforeStatusWrite(t, root, deps, a)
	pinned := load(t, root).Features[featureID].VerifiedAgainst.Eval

	op := requireFinishedWithoutRework(t, root, deps, a, "correct-eval.feature")
	require.Equal(t, featureID, op.NodeID)
	require.Equal(t, "done", op.Status)
	require.Equal(t, pinned, load(t, root).Features[featureID].VerifiedAgainst.Eval)
}

func TestReapproveRefusesACorrectedPinWithoutThisAmendmentsEvent(t *testing.T) {
	t.Parallel()
	root, deps, featureID := correctableFixture(t)
	writeEval(t, root, verifiedEvalMD(featureID, "exit 0"))
	a := stageCorrection(t, root, deps, "eval-bindings")
	a = interruptBeforeStatusWrite(t, root, deps, a)
	removeEvents(t, root, "correct-eval.feature")

	requireStillRefuses(t, root, deps, a)
}

func TestReapproveSkipsAMoveThisAmendmentAlreadyApplied(t *testing.T) {
	t.Parallel()
	root, deps, featureID, taskID := moveFixture(t)
	a := openAmendment(t, root, deps, "relocate")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpMoveFeature, Target: featureID, Key: "destination/renamed", Reason: "Keep one outcome together"})
	a = interruptBeforeStatusWrite(t, root, deps, a)

	op := requireFinishedWithoutRework(t, root, deps, a, "move.feature")
	require.Equal(t, featureID, op.NodeID)
	require.Equal(t, "destination/renamed", op.Key)
	tree := load(t, root)
	require.Equal(t, "destination/renamed", *tree.Features[featureID].Key)
	require.Equal(t, "destination/renamed/task", *tree.Tasks[taskID].Key)
}

func TestReapproveRefusesAMovedFeatureWithoutThisAmendmentsEvent(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := moveFixture(t)
	a := openAmendment(t, root, deps, "relocate")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpMoveFeature, Target: featureID, Key: "destination/renamed", Reason: "Keep one outcome together"})
	a = interruptBeforeStatusWrite(t, root, deps, a)
	removeEvents(t, root, "move.feature")

	err := requireStillRefuses(t, root, deps, a)
	var stale *mutate.AmendmentPreconditionsStaleError
	require.ErrorAs(t, err, &stale)
}
