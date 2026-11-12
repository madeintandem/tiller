package mutate_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// The hold is exclusive (decision 7): a draft one open amendment holds is
// reached only through that amendment. These tests walk every door other than
// add, which amendment_test.go already covers: rename's re-parent, a staged
// move, another amendment's operations, a dependency edge, and withdraw and
// remove on a tree where an unheld child already sits under a held epic.

// doorsFixture is stageFixture plus a task-free hand feature (form-engine/old),
// a held epic (newe) staged on the first amendment, and a second open
// amendment. The first amendment's id sorts below the second's, so it is the
// one the hold index favors if both ever claim one node.
func doorsFixture(t *testing.T) (root string, deps mutate.Deps, first, second, heldEpic, oldFeature string) {
	t.Helper()
	root, first = stageFixture(t)
	deps = correctionDeps(root)
	old, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{Key: "form-engine/old", Title: "Old", ChangeType: "create"})
	require.NoError(t, err)
	heldEpic = stageHeld(t, root, deps, first, mutate.StageInput{Op: store.AmendmentOpAddEpic, Key: "newe", Title: "New epic"}).Op.Node
	res, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{Key: "second", Title: "Second"})
	require.NoError(t, err)
	require.Less(t, first, res.ID)
	return root, deps, first, res.ID, heldEpic, old.ID
}

// --- C1: nothing puts an unheld node under a held epic ---

// TestRenameRefusesToReparentUnderAnEpicHeldElsewhere: a hand rename may not
// move live work under a draft an amendment holds, since withdrawing that
// amendment would delete it. A feature the same amendment holds may move
// there.
func TestRenameRefusesToReparentUnderAnEpicHeldElsewhere(t *testing.T) {
	t.Parallel()
	root, deps, first, _, _, _ := doorsFixture(t)
	before := snapshotDir(t, root)

	_, err := mutate.RenameKey(load(t, root), deps, mutate.RenameKeyInput{Noun: store.NounFeature, Node: "form-engine/old", Key: "newe/old"})
	require.Equal(t, "parent_held_elsewhere", reasonOf(t, err))
	require.ErrorContains(t, err, first)
	requireNoWrites(t, before, snapshotDir(t, root))

	held := stageHeld(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "form-engine/mine", Title: "Mine", ChangeType: "create"}).Op.Node
	_, err = mutate.RenameKey(load(t, root), deps, mutate.RenameKeyInput{Noun: store.NounFeature, Node: "form-engine/mine", Key: "newe/mine"})
	require.NoError(t, err, "a draft moves under a parent its own amendment holds")
	require.Equal(t, "newe", load(t, root).Features[held].EpicKey)
}

// TestStageMoveRefusesADestinationHeldElsewhere: a staged move into an epic a
// different amendment holds is refused at staging; the holding amendment may
// stage the same move.
func TestStageMoveRefusesADestinationHeldElsewhere(t *testing.T) {
	t.Parallel()
	root, deps, first, second, _, _ := doorsFixture(t)
	move := mutate.StageInput{Op: store.AmendmentOpMoveFeature, Target: "form-engine/old", Key: "newe/old", Reason: "it belongs there"}

	_, err := mutate.StageOperation(load(t, root), deps, loadAmendment(t, root, second), move)
	require.Equal(t, "parent_held_elsewhere", reasonOf(t, err))
	require.ErrorContains(t, err, first)
	require.Empty(t, loadAmendment(t, root, second).Operations)

	stageHeld(t, root, deps, first, move)
}

// TestApproveRefusesAMoveIntoAnEpicHeldElsewhere: approval checks the move's
// destination again, so a hand-edited amendment cannot land live work under
// another amendment's draft.
func TestApproveRefusesAMoveIntoAnEpicHeldElsewhere(t *testing.T) {
	t.Parallel()
	root, deps, first, second, _, oldID := doorsFixture(t)
	staged := stageHeld(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpMoveFeature, Target: "form-engine/old", Key: "newe/old", Reason: "it belongs there"})
	writeAmendmentOps(t, root, loadAmendment(t, root, second), []store.AmendmentOp{staged.Op})
	setAmendmentStatus(t, root, second, store.AmendmentStatusProposed)
	before := snapshotDir(t, root)

	_, err := mutate.ApproveAmendment(load(t, root), deps, loadAmendment(t, root, second))
	require.Equal(t, "parent_held_elsewhere", reasonOf(t, err))
	require.ErrorContains(t, err, "operation 0")
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, "form-engine", load(t, root).Features[oldID].EpicKey)
}

// layUnheldChildUnderHeldEpic builds, by hand, the state the guards above
// now prevent: an epic and a feature under it made by plain adds, and the
// first amendment's operations edited to claim only the epic. It returns the
// epic and feature ids.
func layUnheldChildUnderHeldEpic(t *testing.T, root string, deps mutate.Deps, first string) (epicID, featureID string) {
	t.Helper()
	epic, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "adopted", Title: "Adopted"})
	require.NoError(t, err)
	feature, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{Key: "adopted/live", Title: "Live", ChangeType: "create"})
	require.NoError(t, err)
	a := loadAmendment(t, root, first)
	ops := append(append([]store.AmendmentOp(nil), a.Operations...),
		store.AmendmentOp{Op: store.AmendmentOpAddEpic, Key: "adopted", Title: "Adopted", Node: epic.ID})
	writeAmendmentOps(t, root, a, ops)
	tree := load(t, root)
	require.Equal(t, first, tree.AmendmentHolds[epic.ID])
	require.NotContains(t, tree.AmendmentHolds, feature.ID)
	return epic.ID, feature.ID
}

// TestWithdrawRefusesAnUnheldChildUnderAHeldEpic: withdraw removes only what
// the amendment holds. A child it does not hold under one of its epics stops
// the withdraw, named, with nothing written.
func TestWithdrawRefusesAnUnheldChildUnderAHeldEpic(t *testing.T) {
	t.Parallel()
	root, deps, first, _, _, _ := doorsFixture(t)
	_, featureID := layUnheldChildUnderHeldEpic(t, root, deps, first)
	before := snapshotDir(t, root)

	_, err := mutate.WithdrawAmendment(load(t, root), deps, loadAmendment(t, root, first), "not now")
	require.Equal(t, "parent_held_elsewhere", reasonOf(t, err))
	require.ErrorContains(t, err, "adopted/live")
	require.NotContains(t, err.Error(), "remove refuses")
	requireNoWrites(t, before, snapshotDir(t, root))
	require.NotNil(t, load(t, root).Features[featureID])
}

// TestRemoveHeldEpicRefusesAnUnheldChild: a hand removal of a held epic is
// bound the same way, since it would otherwise delete live work as a held
// draft.
func TestRemoveHeldEpicRefusesAnUnheldChild(t *testing.T) {
	t.Parallel()
	root, deps, first, _, _, _ := doorsFixture(t)
	_, featureID := layUnheldChildUnderHeldEpic(t, root, deps, first)
	before := snapshotDir(t, root)

	_, err := mutate.RemoveDraft(load(t, root), deps, mutate.RemoveInput{Noun: store.NounEpic, Node: "adopted", Reason: "not now"})
	require.Equal(t, "parent_held_elsewhere", reasonOf(t, err))
	require.ErrorContains(t, err, "adopted/live")
	requireNoWrites(t, before, snapshotDir(t, root))
	require.NotNil(t, load(t, root).Features[featureID])
}

// --- I1: another amendment cannot act on a held draft ---

// TestStageRefusesAnOperationReachingADraftHeldElsewhere: every operation
// kind that names an existing node refuses one a different amendment holds,
// naming the holder; the holder itself may name its own draft.
func TestStageRefusesAnOperationReachingADraftHeldElsewhere(t *testing.T) {
	t.Parallel()
	root, deps, first, second, _, _ := doorsFixture(t)
	stageHeld(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "form-engine/held", Title: "Held", ChangeType: "create"})
	live := "form-engine/certification-form-loading"

	cases := map[string]mutate.StageInput{
		"cancel target":       {Op: store.AmendmentOpCancel, Target: "form-engine/held", Reason: "no"},
		"revise target":       {Op: store.AmendmentOpRevise, Target: "form-engine/held"},
		"supersede target":    {Op: store.AmendmentOpSupersede, Target: "form-engine/held", By: live},
		"supersede by":        {Op: store.AmendmentOpSupersede, Target: live, By: "form-engine/held"},
		"cancel superseded":   {Op: store.AmendmentOpCancel, Target: live, Reason: "replaced", SupersededBy: "form-engine/held"},
		"move target":         {Op: store.AmendmentOpMoveFeature, Target: "form-engine/held", Key: "form-engine/held-two", Reason: "rename"},
		"correct-eval target": {Op: store.AmendmentOpCorrectEval, Target: "form-engine/held", Reason: "fix"},
		"question blocks":     {Op: store.AmendmentOpAddQuestion, Title: "Which?", Statement: "Which one?", Blocks: []string{"form-engine/held"}},
	}
	before := snapshotDir(t, root)
	for name, in := range cases {
		_, err := mutate.StageOperation(load(t, root), deps, loadAmendment(t, root, second), in)
		require.Equal(t, "held_by_amendment", reasonOf(t, err), name)
		require.ErrorContains(t, err, first, name)
		require.ErrorContains(t, err, "stage this on that amendment", name)
	}
	requireNoWrites(t, before, snapshotDir(t, root))

	stageHeld(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Which?", Statement: "Which one?", Blocks: []string{"form-engine/held"}})
	stageHeld(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: live, Reason: "replaced", SupersededBy: "form-engine/held"})
}

// TestApproveRefusesAnOperationOnADraftHeldElsewhere: approval checks the
// same thing again, so a hand-edited cancel of another amendment's draft
// refuses with nothing written, and the holder is not wedged.
func TestApproveRefusesAnOperationOnADraftHeldElsewhere(t *testing.T) {
	t.Parallel()
	root, deps, first, second, _, _ := doorsFixture(t)
	held := stageHeld(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "form-engine/held", Title: "Held", ChangeType: "create"}).Op.Node
	writeAmendmentOps(t, root, loadAmendment(t, root, second), []store.AmendmentOp{
		{Op: store.AmendmentOpCancel, Target: held, Reason: "no"}})
	setAmendmentStatus(t, root, second, store.AmendmentStatusProposed)
	before := snapshotDir(t, root)

	_, err := mutate.ApproveAmendment(load(t, root), deps, loadAmendment(t, root, second))
	require.Equal(t, "held_by_amendment", reasonOf(t, err))
	require.ErrorContains(t, err, "operation 0")
	require.ErrorContains(t, err, first)
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, "draft", load(t, root).Features[held].Status)
}

// --- I2: a renamed draft frees its staged key ---

// TestRenamedHeldDraftFreesItsStagedKey: the key an add was staged under is
// history once its draft is renamed, so staging that key again is a second,
// distinct draft, and the amendment proposes and approves with both.
func TestRenamedHeldDraftFreesItsStagedKey(t *testing.T) {
	t.Parallel()
	root, deps, first, _, heldEpic, _ := doorsFixture(t)
	_, err := mutate.RenameKey(load(t, root), deps, mutate.RenameKeyInput{Noun: store.NounEpic, Node: "newe", Key: "renamed"})
	require.NoError(t, err)
	again := stageHeld(t, root, deps, first, mutate.StageInput{Op: store.AmendmentOpAddEpic, Key: "newe", Title: "New epic again"}).Op.Node

	_, err = mutate.ProposeAmendment(load(t, root), deps, loadAmendment(t, root, first))
	require.NoError(t, err)
	_, err = mutate.ApproveAmendment(load(t, root), deps, loadAmendment(t, root, first))
	require.NoError(t, err)
	tree := load(t, root)
	require.Equal(t, "ready", tree.Epics[heldEpic].Status)
	require.Equal(t, "ready", tree.Epics[again].Status)
}

// --- I3: approval promotes only what the amendment holds ---

// TestProposeAndApproveRefuseANodeAnotherAmendmentHolds: two amendments
// naming one node (a hand edit or a merge) cannot both promote it. The hold
// index gives it to one; the other is refused at hand-over and at approval,
// naming the real holder, and the node is untouched.
func TestProposeAndApproveRefuseANodeAnotherAmendmentHolds(t *testing.T) {
	t.Parallel()
	root, deps, first, second, heldEpic, _ := doorsFixture(t)
	writeAmendmentOps(t, root, loadAmendment(t, root, second), []store.AmendmentOp{
		{Op: store.AmendmentOpAddEpic, Key: "newe", Title: "New epic", Node: heldEpic}})
	require.Equal(t, first, load(t, root).AmendmentHolds[heldEpic])

	_, err := mutate.ProposeAmendment(load(t, root), deps, loadAmendment(t, root, second))
	require.Equal(t, "held_node_missing", reasonOf(t, err))
	require.ErrorContains(t, err, first)

	setAmendmentStatus(t, root, second, store.AmendmentStatusProposed)
	before := snapshotDir(t, root)
	_, err = mutate.ApproveAmendment(load(t, root), deps, loadAmendment(t, root, second))
	require.Equal(t, "held_node_missing", reasonOf(t, err))
	require.ErrorContains(t, err, first)
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, "draft", load(t, root).Epics[heldEpic].Status)
}

// --- I4: a dependency on a held draft waits for approval ---

// TestLinkRefusesLiveWorkDependingOnADraftHeldElsewhere: live work does not
// take a dependency on a draft, since the edge would act before the
// amendment is approved. A draft may depend on live work, and on another
// draft of its own amendment.
func TestLinkRefusesLiveWorkDependingOnADraftHeldElsewhere(t *testing.T) {
	t.Parallel()
	root, deps, first, _, _, _ := doorsFixture(t)
	stageHeld(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "form-engine/held", Title: "Held", ChangeType: "create"})
	stageHeld(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "form-engine/held-two", Title: "Held two", ChangeType: "create"})
	before := snapshotDir(t, root)

	_, err := mutate.Link(load(t, root), deps, mutate.LinkInput{Noun: store.NounFeature, Node: "form-engine/old", DependsOn: []string{"form-engine/held"}})
	require.Equal(t, "held_by_amendment", reasonOf(t, err))
	require.ErrorContains(t, err, first)
	require.ErrorContains(t, err, "after the amendment is approved")
	_, err = mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/fresh", Title: "Fresh", ChangeType: "create", DependsOn: []string{"form-engine/held"}})
	require.Equal(t, "held_by_amendment", reasonOf(t, err))
	_, err = mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "fresh", Title: "Fresh", DependsOn: []string{"newe"}})
	require.Equal(t, "held_by_amendment", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))

	_, err = mutate.Link(load(t, root), deps, mutate.LinkInput{Noun: store.NounFeature, Node: "form-engine/held", DependsOn: []string{"form-engine/old"}})
	require.NoError(t, err, "a draft may depend on live work")
	_, err = mutate.Link(load(t, root), deps, mutate.LinkInput{Noun: store.NounFeature, Node: "form-engine/held-two", DependsOn: []string{"form-engine/held"}})
	require.NoError(t, err, "two drafts of one amendment may depend on each other")
}

// TestAddTaskRefusesASiblingDependencyOnAHeldTask: a plain task under an
// approved feature may not depend on a sibling an amendment holds.
func TestAddTaskRefusesASiblingDependencyOnAHeldTask(t *testing.T) {
	t.Parallel()
	root, deps, first, _, _, _ := doorsFixture(t)
	setEpicStatus(t, root, "form-engine", "draft", "ready")
	setFeatureStatusOnDisk(t, root, "form-engine", "certification-form-loading", "ready")
	stageHeld(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpAddTask, Key: "form-engine/certification-form-loading/held", Title: "Held"})

	_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key: "form-engine/certification-form-loading/fresh", Title: "Fresh",
		DependsOn: []string{"form-engine/certification-form-loading/held"}})
	require.Equal(t, "held_by_amendment", reasonOf(t, err))
	_, err = mutate.Link(load(t, root), deps, mutate.LinkInput{Noun: store.NounTask,
		Node: "form-engine/certification-form-loading/schema-transcription", DependsOn: []string{"form-engine/certification-form-loading/held"}})
	require.Equal(t, "held_by_amendment", reasonOf(t, err))
}

// --- M4: a staged move's destination is a reference ---

// TestRemoveRefusesAHeldEpicAStagedMoveLandsIn: removing a held epic that a
// staged move names as its destination would leave a move that can never
// apply, so the removal refuses like any other open reference.
func TestRemoveRefusesAHeldEpicAStagedMoveLandsIn(t *testing.T) {
	t.Parallel()
	root, deps, first, _, _, _ := doorsFixture(t)
	stageHeld(t, root, deps, first, mutate.StageInput{
		Op: store.AmendmentOpMoveFeature, Target: "form-engine/old", Key: "newe/old", Reason: "it belongs there"})
	before := snapshotDir(t, root)

	_, err := mutate.RemoveDraft(load(t, root), deps, mutate.RemoveInput{Noun: store.NounEpic, Node: "newe", Reason: "not now"})
	require.Equal(t, "proposed_amendment_reference", reasonOf(t, err))
	requireNoWrites(t, before, snapshotDir(t, root))
	for _, op := range loadAmendment(t, root, first).Operations {
		require.Empty(t, op.Removed)
	}
}
