package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// This file tests EvaluateAmendmentPreconditions and EvaluateAmendmentSource
// (amendment_validity.go), Task 4's and Task 5's exports: `show amendment`'s
// live-validity reads, reusing approve's own comparisons rather than a
// second copy of either.

// heldDeps is gitDeps with a sequential minter: staging an add mints a real
// node now, and the fixed minter's E-, F-, and T-aaaaaa collide with the
// fixture's own nodes. Share one value across a test so the sequence never
// repeats an id.
func heldDeps(root string) mutate.Deps {
	d := gitDeps(root)
	d.Mint = seqDeps().Mint
	return d
}

// soleFeatureIDM returns stageFixture's one feature id.
func soleFeatureIDM(t *testing.T, root string) string {
	t.Helper()
	tree := load(t, root)
	require.Len(t, tree.Features, 1)
	for id := range tree.Features {
		return id
	}
	return ""
}

// TestEvaluateAmendmentPreconditionsReportsValidAndStale stages one op
// carrying a precondition that holds against the live tree (target_status,
// matching the feature's actual draft status) beside one that does not
// (key_absent naming the key the feature itself already holds), and asserts
// each comes back with its own LIVE status: exactly the fixture show's
// rendering test needs.
func TestEvaluateAmendmentPreconditionsReportsValidAndStale(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	featureID := soleFeatureIDM(t, root)
	deps := gitDeps(root)

	a := stage(t, root, deps, loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:     store.AmendmentOpRevise,
		Target: featureID,
		Preconditions: map[string]string{
			"target_status": "draft",
			"key_absent":    "form-engine/certification-form-loading",
		},
	})

	got, err := mutate.EvaluateAmendmentPreconditions(load(t, root), deps, a)
	require.NoError(t, err)
	require.Len(t, got, 2)

	byName := map[string]mutate.PreconditionValidity{}
	for _, pv := range got {
		byName[pv.Precondition] = pv
	}

	valid := byName["target_status"]
	require.Equal(t, mutate.PreconditionValid, valid.Status)
	require.Equal(t, "draft", valid.Actual)
	require.Equal(t, featureID, valid.Target)

	stale := byName["key_absent"]
	require.Equal(t, mutate.PreconditionStale, stale.Status)
	require.Equal(t, featureID, stale.Actual, "key_absent's stale Actual names the holder")
	require.NotEmpty(t, stale.Note, "a stale key_absent explains who holds the key")
}

// TestEvaluateAmendmentPreconditionsReportsUnknownForAStagedKey stages an
// add-task op (whose subject is its own staged key) carrying a target_status
// precondition, a combination no evaluator can compare: an add has no target.
// Its key_absent reads valid although staging created the task under that
// key, because the only holder is the operation's own held draft.
func TestEvaluateAmendmentPreconditionsReportsUnknownForAStagedKey(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	// An amendment adds tasks only to an approved feature.
	setFeatureStatusOnDisk(t, root, "form-engine", "certification-form-loading", "ready")
	deps := heldDeps(root)

	const newKey = "form-engine/certification-form-loading/extra-task"
	a := stage(t, root, deps, loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:    store.AmendmentOpAddTask,
		Key:   newKey,
		Title: "Extra task",
		Preconditions: map[string]string{
			"target_status": "done",
			"key_absent":    newKey,
		},
	})

	got, err := mutate.EvaluateAmendmentPreconditions(load(t, root), deps, a)
	require.NoError(t, err)
	byName := map[string]mutate.PreconditionValidity{}
	for _, pv := range got {
		byName[pv.Precondition] = pv
	}

	unknown := byName["target_status"]
	require.Equal(t, mutate.PreconditionUnknown, unknown.Status)
	require.NotEmpty(t, unknown.Note)
	require.Empty(t, unknown.Actual, "an unknown precondition has nothing yet to compare")

	valid := byName["key_absent"]
	require.Equal(t, mutate.PreconditionValid, valid.Status, "only the operation's own held draft holds the key")
}

// TestEvaluateAmendmentPreconditionsHandlesAStagedAddEpic pins that add-epic
// resolves through this same path exactly as add-feature and add-task do:
// resolveAmendment (amendment_approve.go) has to treat add-epic's key as
// "nothing to resolve, not a target" the same way it treats the other two add
// kinds, or every show amendment on a batch carrying a staged epic would
// error instead of rendering.
func TestEvaluateAmendmentPreconditionsHandlesAStagedAddEpic(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	deps := heldDeps(root)

	a := stage(t, root, deps, loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:    store.AmendmentOpAddEpic,
		Key:   "hardening",
		Title: "Hardening",
		Preconditions: map[string]string{
			"key_absent": "hardening",
		},
	})

	got, err := mutate.EvaluateAmendmentPreconditions(load(t, root), deps, a)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, mutate.PreconditionValid, got[0].Status, "only the operation's own held epic holds the key")
	require.Equal(t, store.AmendmentOpAddEpic, got[0].Op)
	require.Equal(t, "hardening", got[0].Target)
}

// TestEvaluateAmendmentPreconditionsHandlesAStagedAddQuestion pins the
// identical regression one grain over: resolveAmendment must treat
// add-question's own blank Target as "nothing to resolve" too,
// or show amendment on a batch carrying a staged question would error instead
// of rendering. A question carries no key, so nothing here stages a
// key_absent precondition against one; the empty result itself is the
// assertion that resolution did not error.
func TestEvaluateAmendmentPreconditionsHandlesAStagedAddQuestion(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	deps := gitDeps(root)

	a := stage(t, root, deps, loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:        store.AmendmentOpAddQuestion,
		Title:     "Authentication provider",
		Statement: "Which auth provider?",
	})

	got, err := mutate.EvaluateAmendmentPreconditions(load(t, root), deps, a)
	require.NoError(t, err, "a staged add-question must not error resolution")
	require.Empty(t, got, "no preconditions were staged on it")
}

// TestEvaluateAmendmentPreconditionsNeverWrites proves the read leaves the
// tree untouched, the same no-writes discipline approve's own phase 1 keeps:
// a caller (show amendment) must be able to run this on every invocation
// without it ever being the write path.
func TestEvaluateAmendmentPreconditionsNeverWrites(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	featureID := soleFeatureIDM(t, root)
	deps := gitDeps(root)
	a := stage(t, root, deps, loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpRevise, Target: featureID,
		Preconditions: map[string]string{"target_status": "draft"},
	})

	before := snapshotDir(t, root)
	_, err := mutate.EvaluateAmendmentPreconditions(load(t, root), deps, a)
	require.NoError(t, err)
	after := snapshotDir(t, root)
	requireNoWrites(t, before, after)
}

// TestEvaluateAmendmentPreconditionsPropagatesResolutionFailure asserts that
// a batch that cannot even resolve (a vanished target) surfaces its error
// rather than silently reporting empty or fabricated validity: the same
// distinction resolveAmendment's own doc comment draws between a comparison
// that came out false and a comparison that could not run at all.
func TestEvaluateAmendmentPreconditionsPropagatesResolutionFailure(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	featureID := soleFeatureIDM(t, root)
	deps := gitDeps(root)
	a := stage(t, root, deps, loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpRevise, Target: featureID,
	})
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{{Op: store.AmendmentOpRevise, Target: "F-vanished"}})

	_, err := mutate.EvaluateAmendmentPreconditions(load(t, root), deps, a)
	require.Error(t, err)
}

// --- EvaluateAmendmentSource: show amendment's live read of Task 5's one
// amendment-level fact, amendment_approve_test.go's own
// sourceFixture reused here since both files' tests need the identical
// intake-minted shape (a real feedback artifact, an amendment whose Source
// cites its current fingerprint). ---

// TestEvaluateAmendmentSourceReportsValid mirrors
// TestEvaluateAmendmentPreconditionsReportsValidAndStale for the
// amendment-level twin: a fresh intake artifact reads valid, Actual matching
// the recorded fingerprint.
func TestEvaluateAmendmentSourceReportsValid(t *testing.T) {
	t.Parallel()
	root, deps, a, _ := sourceFixture(t, "the review body\n")

	got, err := mutate.EvaluateAmendmentSource(load(t, root), deps, a)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, mutate.PreconditionValid, got.Status)
	require.Equal(t, a.Source.Revision, got.Actual)
}

// TestEvaluateAmendmentSourceReportsStaleOnAnEdit pins the same live
// comparison approve's own checkSource refuses on (evalSource, shared),
// reported here without refusing: the whole point of show is a human
// reviewing the batch BEFORE running approve.
func TestEvaluateAmendmentSourceReportsStaleOnAnEdit(t *testing.T) {
	t.Parallel()
	root, deps, a, relPath := sourceFixture(t, "the review body\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, relPath), []byte("edited\n"), 0o644))
	live := blobOf(t, root, relPath)

	got, err := mutate.EvaluateAmendmentSource(load(t, root), deps, a)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, mutate.PreconditionStale, got.Status)
	require.Equal(t, live, got.Actual)
}

// TestEvaluateAmendmentSourceReportsStaleOnAVanishedArtifactWithANote pins
// the distinct-vanish reporting checkSource's own doc comment states: no
// live hash to report, so Actual stays empty and Note explains why rather
// than a blank value standing in for a comparison that never ran.
func TestEvaluateAmendmentSourceReportsStaleOnAVanishedArtifactWithANote(t *testing.T) {
	t.Parallel()
	root, deps, a, relPath := sourceFixture(t, "the review body\n")
	require.NoError(t, os.Remove(filepath.Join(root, relPath)))

	got, err := mutate.EvaluateAmendmentSource(load(t, root), deps, a)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, mutate.PreconditionStale, got.Status)
	require.Empty(t, got.Actual, "a vanished artifact has no live hash")
	require.NotEmpty(t, got.Note, "the note explains the vanish, distinct from a changed hash")
}

// TestEvaluateAmendmentSourceIsNilForAHandAmendment proves the honest-absence
// contract (EvaluateAmendmentSource's own doc comment): a hand-opened
// amendment carries no Source at all, so the evaluator returns (nil, nil)
// rather than a PreconditionUnknown-carrying value for a fact that does not
// exist.
func TestEvaluateAmendmentSourceIsNilForAHandAmendment(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	deps := gitDeps(root)
	a := loadAmendment(t, root, amendmentID)

	got, err := mutate.EvaluateAmendmentSource(load(t, root), deps, a)
	require.NoError(t, err)
	require.Nil(t, got, "a hand-opened amendment carries no Source to evaluate")
}

// TestEvaluateAmendmentSourceNeverWrites mirrors
// TestEvaluateAmendmentPreconditionsNeverWrites for the Source-side read: a
// re-hash is a filesystem read, never a write.
func TestEvaluateAmendmentSourceNeverWrites(t *testing.T) {
	t.Parallel()
	root, deps, a, _ := sourceFixture(t, "the review body\n")
	before := snapshotDir(t, root)
	_, err := mutate.EvaluateAmendmentSource(load(t, root), deps, a)
	require.NoError(t, err)
	requireNoWrites(t, before, snapshotDir(t, root))
}

// --- EvaluateAmendmentOpSources: show amendment's live read of
// checkOpSources' own per-op provenance guard (#57, amendment_approve.go),
// reused here beside EvaluateAmendmentPreconditions' and
// EvaluateAmendmentSource's own live reads above. ---

// TestEvaluateAmendmentOpSourcesReportsEveryBlankedOp is the full-report pin:
// checkOpSources itself refuses on the FIRST sourceless operation, but this
// evaluator collects every one, the same evaluator-beside-checker
// relationship EvaluateAmendmentPreconditions keeps with checkPreconditions.
// Three sourced ops are staged (three independent add-epics, so nothing here
// depends on a same-batch parent), then the second and third are hand-blanked
// (writeAmendmentOps, the same shape TestApproveAmendmentRefusesAHandBlankedOpSourceWithNoWrites
// uses for the single-defect refusal), and both must come back, not only the
// one that sorts first.
func TestEvaluateAmendmentOpSourcesReportsEveryBlankedOp(t *testing.T) {
	t.Parallel()
	root, deps, a, _ := sourceFixture(t, "the review body\n")
	deps.Mint = seqDeps().Mint

	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "billing", Title: "Billing",
		Source: "review.md#L1-2",
	})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "shipping", Title: "Shipping",
		Source: "review.md#L3-4",
	})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "support", Title: "Support",
		Source: "review.md#L5-6",
	})

	ops := append([]store.AmendmentOp(nil), a.Operations...)
	ops[1].Source = ""
	ops[2].Source = ""
	a = writeAmendmentOps(t, root, a, ops)

	got := mutate.EvaluateAmendmentOpSources(a)
	require.Len(t, got, 2, "every blanked op is reported, not only the first")
	require.Equal(t, 1, got[0].Index)
	require.Equal(t, store.AmendmentOpAddEpic, got[0].Op)
	require.Equal(t, 2, got[1].Index)
	require.Equal(t, store.AmendmentOpAddEpic, got[1].Op)
}

// TestEvaluateAmendmentOpSourcesTreatsWhitespaceAsBlank pins the shared
// predicate: checkOpSources runs strings.TrimSpace before comparing to "", so
// this evaluator must flag a whitespace-only source exactly like an empty
// one.
func TestEvaluateAmendmentOpSourcesTreatsWhitespaceAsBlank(t *testing.T) {
	t.Parallel()
	root, deps, a, _ := sourceFixture(t, "the review body\n")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "billing", Title: "Billing",
		Source: "review.md#L1-2",
	})

	ops := append([]store.AmendmentOp(nil), a.Operations...)
	ops[0].Source = "   "
	a = writeAmendmentOps(t, root, a, ops)

	got := mutate.EvaluateAmendmentOpSources(a)
	require.Len(t, got, 1)
	require.Equal(t, 0, got[0].Index)
	require.Equal(t, store.AmendmentOpAddEpic, got[0].Op)
}

// TestEvaluateAmendmentOpSourcesReportsNoneWhenEverySourced is the positive
// pin beside the two refusal-shaped cases above: every op sourced cleanly
// comes back empty.
func TestEvaluateAmendmentOpSourcesReportsNoneWhenEverySourced(t *testing.T) {
	t.Parallel()
	root, deps, a, _ := sourceFixture(t, "the review body\n")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "billing", Title: "Billing",
		Source: "review.md#L1-2",
	})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "billing/invoicing", Title: "Invoicing",
		ChangeType: "create", Source: "review.md#L3-4",
	})

	got := mutate.EvaluateAmendmentOpSources(a)
	require.Empty(t, got)
}

// TestEvaluateAmendmentOpSourcesIsNilForAHandAmendment mirrors
// TestEvaluateAmendmentSourceIsNilForAHandAmendment: a hand-opened amendment
// (amendment.Source == nil) never required a per-op citation to begin with
// (checkOpSources' own pass-through), so the evaluator returns nil rather
// than flagging an op whose blank Source was always legal there.
func TestEvaluateAmendmentOpSourcesIsNilForAHandAmendment(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	a := stage(t, root, seqDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening",
	})
	require.Nil(t, a.Source, "a hand-opened amendment carries no Source")

	got := mutate.EvaluateAmendmentOpSources(a)
	require.Nil(t, got, "nothing here was ever required to carry a per-op citation")
}

// --- EvaluateAmendmentShape: show amendment's live read of resolveAmendment's
// own two shape-only refusals: a duplicate staged key and a
// staged-epic replacement. Neither predicate is gated on amendment.Source
// (unlike EvaluateAmendmentOpSources): resolveAmendment itself runs both
// checks unconditionally, on a hand-opened amendment exactly as on an
// intake-minted one, so this evaluator does too. ---

// TestEvaluateAmendmentShapeReportsEveryDuplicateStagedKey is the
// full-report pin: resolveAmendment itself refuses on the FIRST duplicate
// key it finds, but this evaluator collects every one, the same
// evaluator-beside-checker relationship EvaluateAmendmentOpSources keeps
// with checkOpSources. Two independent duplicate pairs are laid so both
// have to come back, not only the first pair's second occurrence. Staging
// now refuses a repeated key itself (the first stage created the node, so the
// second is key_exists), so the batch is laid by hand in the shape an
// amendment staged before schema 15 can still carry.
func TestEvaluateAmendmentShapeReportsEveryDuplicateStagedKey(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	a := writeAmendmentOps(t, root, loadAmendment(t, root, amendmentID), []store.AmendmentOp{
		{Op: store.AmendmentOpAddEpic, Key: "billing", Title: "Billing one"},
		{Op: store.AmendmentOpAddEpic, Key: "billing", Title: "Billing two"},
		{Op: store.AmendmentOpAddEpic, Key: "shipping", Title: "Shipping one"},
		{Op: store.AmendmentOpAddEpic, Key: "shipping", Title: "Shipping two"},
	})

	got := mutate.EvaluateAmendmentShape(a)
	require.Len(t, got, 2, "both duplicate pairs are reported, not only the first")
	require.Equal(t, 1, got[0].Index)
	require.Equal(t, store.AmendmentOpAddEpic, got[0].Op)
	require.Equal(t, mutate.ReasonDuplicateStagedKey, got[0].Kind)
	require.Equal(t, 3, got[1].Index)
	require.Equal(t, store.AmendmentOpAddEpic, got[1].Op)
	require.Equal(t, mutate.ReasonDuplicateStagedKey, got[1].Kind)
}

// TestEvaluateAmendmentShapeReportsEveryOccurrenceWhenOneKeyRepeatsThreeTimes
// is duplicateStagedKey's own edge case beside the two-independent-pairs case
// above: ONE key staged THREE times, not two keys staged twice each. Every
// occurrence after the first is a defect on its own terms (duplicateStagedKey
// asks "does ANY earlier operation already stage this key", not "does the
// immediately preceding one"), so both the second and the third operation
// are flagged, and the first stays clean.
func TestEvaluateAmendmentShapeReportsEveryOccurrenceWhenOneKeyRepeatsThreeTimes(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	// Laid by hand, as above: staging refuses the repeat itself.
	a := writeAmendmentOps(t, root, loadAmendment(t, root, amendmentID), []store.AmendmentOp{
		{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening one"},
		{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening two"},
		{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening three"},
	})

	got := mutate.EvaluateAmendmentShape(a)
	require.Len(t, got, 2, "operation 0 stages the key cleanly; 1 and 2 both repeat it")
	require.Equal(t, 1, got[0].Index)
	require.Equal(t, mutate.ReasonDuplicateStagedKey, got[0].Kind)
	require.Equal(t, 2, got[1].Index)
	require.Equal(t, mutate.ReasonDuplicateStagedKey, got[1].Kind)
}

// TestEvaluateAmendmentShapeReportsStagedEpicReplacement pins the
// order-independent predicate (stagedEpicKey, amendment_approve.go): a
// supersede or a cancel naming a staged add-epic key as its By or
// SupersededBy is exactly the shape resolveAmendment's own
// stagedEpicReplacementErr refuses. Staging itself never permits this
// (resolveStageReplacement's own deliberate epic exclusion, amendment.go),
// so the batch is hand-laid the same way
// TestApproveAmendmentRefusesAStagedAddEpicKeyAsAReplacementWithNoWrites
// does; both a supersede and a cancel are staged so the evaluator's own
// full-report posture covers both op kinds this predicate applies to.
func TestEvaluateAmendmentShapeReportsStagedEpicReplacement(t *testing.T) {
	t.Parallel()
	root, deps, featureID, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "epic-replacement")
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{
		{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"},
		{Op: store.AmendmentOpSupersede, Target: featureID, By: "hardening"},
		{Op: store.AmendmentOpCancel, Target: taskID, Reason: "replaced", SupersededBy: "hardening"},
	})

	got := mutate.EvaluateAmendmentShape(a)
	require.Len(t, got, 2, "both the supersede and the cancel are reported")
	require.Equal(t, 1, got[0].Index)
	require.Equal(t, store.AmendmentOpSupersede, got[0].Op)
	require.Equal(t, mutate.ReasonStagedEpicReplacement, got[0].Kind)
	require.Equal(t, 2, got[1].Index)
	require.Equal(t, store.AmendmentOpCancel, got[1].Op)
	require.Equal(t, mutate.ReasonStagedEpicReplacement, got[1].Kind)
}

// TestEvaluateAmendmentShapeReportsALaterAddEpic pins stagedEpicKey's own
// order independence (amendment_approve.go): a supersede staged BEFORE the
// add-epic it names as its replacement is exactly as much a defect as the
// common, epic-first shape TestEvaluateAmendmentShapeReportsStagedEpicReplacement
// pins above, because an epic is never a legal replacement in ANY position
// (resolveStageReplacement's own deliberate exclusion, amendment.go) — there
// is no "earlier" question to ask here the way there is for a duplicate key.
// This used to be the one place the evaluator's report diverged from
// resolveAmendment's own refusal (a narrower, ordering-aware predicate that
// needed a second, wider fallback inside resolveAmendment just to keep its
// own behavior unchanged); the fallback is gone now that both sides share
// this one order-independent predicate, so the evaluator catches the
// reversed order for free and approve refuses it through the SAME
// predicate, not a parallel one.
func TestEvaluateAmendmentShapeReportsALaterAddEpic(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	a := openAmendment(t, root, deps, "reversed-order")
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{
		{Op: store.AmendmentOpSupersede, Target: featureID, By: "hardening"},
		{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"},
	})

	got := mutate.EvaluateAmendmentShape(a)
	require.Len(t, got, 1, "the supersede is flagged even though the add-epic staging its key appears AFTER it")
	require.Equal(t, 0, got[0].Index)
	require.Equal(t, store.AmendmentOpSupersede, got[0].Op)
	require.Equal(t, mutate.ReasonStagedEpicReplacement, got[0].Kind)

	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Equal(t, "staged_epic_replacement", reasonOf(t, err),
		"approve refuses the identical batch through the SAME shared predicate, byte-identical to before the ordering fix")
	requireNoWrites(t, before, snapshotDir(t, root))
}

// TestEvaluateAmendmentShapeReportsNoneForACleanBatch is the positive pin
// beside the refusal-shaped cases above: a batch with no duplicate keys and
// no staged-epic replacements comes back empty.
func TestEvaluateAmendmentShapeReportsNoneForACleanBatch(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	a := loadAmendment(t, root, amendmentID)
	deps := seqDeps()
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"})
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "hardening/onboarding", Title: "Onboarding", ChangeType: "create"})

	got := mutate.EvaluateAmendmentShape(a)
	require.Empty(t, got)
}

// --- Folder sources (intake of a folder): one amendment, one Source naming the
// folder, a per-file fingerprint list. evalSource is shared by propose,
// approve, and show, so one verdict serves all three. ---

// folderSourceFixture is folderSourceDraft handed over to proposed.
func folderSourceFixture(t *testing.T) (root string, deps mutate.Deps, a *store.Amendment, dir string) {
	t.Helper()
	root, deps, a, dir = folderSourceDraft(t)
	_, err := mutate.ProposeAmendment(load(t, root), deps, a)
	require.NoError(t, err)
	return root, deps, loadAmendment(t, root, a.ID), dir
}

// folderSourceDraft lands two files under .tiller/design/specs/ and opens a
// draft amendment whose Source is the folder with per-file revisions, with one
// operation staged so it is not empty.
func folderSourceDraft(t *testing.T) (root string, deps mutate.Deps, a *store.Amendment, dir string) {
	t.Helper()
	root = writeOwnedRepo(t)
	deps = gitDeps(root)

	dir = filepath.Join(".tiller", "design", "specs")
	require.NoError(t, os.MkdirAll(filepath.Join(root, dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, dir, "a.md"), []byte("alpha\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, dir, "sub", "b.md"), []byte("beta\n"), 0o644))

	var files []store.AmendmentSourceFile
	for _, rel := range []string{filepath.Join(dir, "a.md"), filepath.Join(dir, "sub", "b.md")} {
		files = append(files, store.AmendmentSourceFile{Path: filepath.ToSlash(rel), Revision: blobOf(t, root, rel)})
	}
	res, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{
		Key: "specs-intake", Title: "Intake: specs",
		Source: &store.AmendmentSource{Path: filepath.ToSlash(dir), Files: files},
	})
	require.NoError(t, err)
	a = stage(t, root, deps, loadAmendment(t, root, res.ID), mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "audit", Title: "Audit", Source: ".tiller/design/specs/a.md#L1-1",
	})
	return root, deps, a, dir
}

// TestFolderSourceStaleness pins decision 4's fingerprint rule: an untouched
// folder reads valid; a changed, removed, or added file reads stale with a note
// naming the first differing path and what happened; and the same verdict
// refuses approve.
func TestFolderSourceStaleness(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(t *testing.T, root, dir string)
		path   string
		what   string
	}{
		{"changed", func(t *testing.T, root, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(root, dir, "sub", "b.md"), []byte("edited\n"), 0o644))
		}, ".tiller/design/specs/sub/b.md", "changed"},
		{"removed", func(t *testing.T, root, dir string) {
			require.NoError(t, os.Remove(filepath.Join(root, dir, "a.md")))
		}, ".tiller/design/specs/a.md", "removed"},
		{"added", func(t *testing.T, root, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(root, dir, "c.md"), []byte("new\n"), 0o644))
		}, ".tiller/design/specs/c.md", "added"},
	}
	t.Run("valid when untouched", func(t *testing.T) {
		t.Parallel()
		root, deps, a, _ := folderSourceFixture(t)
		got, err := mutate.EvaluateAmendmentSource(load(t, root), deps, a)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, mutate.PreconditionValid, got.Status)
		_, err = mutate.ApproveAmendment(load(t, root), deps, a)
		require.NoError(t, err)
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, deps, a, dir := folderSourceFixture(t)
			tc.mutate(t, root, dir)

			got, err := mutate.EvaluateAmendmentSource(load(t, root), deps, a)
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, mutate.PreconditionStale, got.Status)
			require.Contains(t, got.Note, tc.path)
			require.Contains(t, got.Note, tc.what)

			_, err = mutate.ApproveAmendment(load(t, root), deps, a)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.path)
		})
	}
}

// TestFolderSourceIgnoresOSJunk: a Finder-written .DS_Store beside the landed
// files is not a document, the same filter the import applies.
func TestFolderSourceIgnoresOSJunk(t *testing.T) {
	t.Parallel()
	root, deps, a, dir := folderSourceFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, dir, ".DS_Store"), []byte("x"), 0o644))
	got, err := mutate.EvaluateAmendmentSource(load(t, root), deps, a)
	require.NoError(t, err)
	require.Equal(t, mutate.PreconditionValid, got.Status)
}

// TestFolderSourceStalenessRefusesPropose: propose runs the same source check,
// so a folder that moved while the amendment was a draft cannot be handed over.
func TestFolderSourceStalenessRefusesPropose(t *testing.T) {
	t.Parallel()
	root, deps, a, dir := folderSourceDraft(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, dir, "c.md"), []byte("new\n"), 0o644))

	_, err := mutate.ProposeAmendment(load(t, root), deps, a)
	require.Error(t, err)
	require.Contains(t, err.Error(), ".tiller/design/specs/c.md")
}
