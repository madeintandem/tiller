package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// epicByKey walks tree.Epics for the one whose recorded key equals key, the
// epic-grain twin of configure_test.go's featureByKey.
func epicByKey(tree *store.Tree, key string) *store.Epic {
	for _, e := range tree.Epics {
		if e.Key != nil && *e.Key == key {
			return e
		}
	}
	return nil
}

// eventOfType returns the last event of the given type in evs, or nil.
func eventOfType(evs []events.Event, typ string) *events.Event {
	var found *events.Event
	for i := range evs {
		if evs[i].Type == typ {
			found = &evs[i]
		}
	}
	return found
}

// TestApproveEpicRecordsReviewGrain pins approve epic --review-grain: the
// value lands on epic.yml in the same act as the draft->ready move, and the
// approve.epic event's After map carries it alongside status
// (docs/plans/2026-08-26-review-grain-design.md, "The setting").
func TestApproveEpicRecordsReviewGrain(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	tree := load(t, root)
	epic := resolveNode(t, tree, store.NounEpic, "pay")
	grain := "epic"

	res, err := mutate.Transition(tree, testDeps(), epic, mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "ready", res.Status)

	require.Equal(t, "epic", epicByKey(load(t, root), "pay").ReviewGrain)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	ev := eventOfType(evs, "approve.epic")
	require.NotNil(t, ev, "expected an approve.epic event")
	require.Equal(t, "epic", ev.After["review_grain"])
}

// TestApproveFeaturePlanRecordsReviewGrain pins approve feature
// --review-grain at the plan gate: the value lands on feature.yml alongside
// the normal specced->ready move, and the approve.feature event's After map
// carries it.
func TestApproveFeaturePlanRecordsReviewGrain(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	grain := "feature"

	res := driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})
	require.Equal(t, "ready", res.Status)

	require.Equal(t, "feature", featureByKey(load(t, root), "form-engine/certification-form-loading").ReviewGrain)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	ev := eventOfType(evs, "approve.feature")
	require.NotNil(t, ev, "expected an approve.feature event")
	require.Equal(t, "feature", ev.After["review_grain"])
}

// Invalid review grain refuses approval before any lifecycle write.
func TestApproveRefusesIllegalReviewGrain(t *testing.T) {
	t.Parallel()
	t.Run("bogus on an epic", func(t *testing.T) {
		root := writeOwnedRepo(t)
		mustAddEpic(t, root, "pay")
		grain := "bogus"

		err := tryTransition(t, root, testDeps(), store.NounEpic, "pay",
			mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})
		require.Error(t, err)
		require.Equal(t, "draft", epicByKey(load(t, root), "pay").Status,
			"a refused approve must not move the epic")
	})
}

// TestApproveWithoutReviewGrainLeavesItUntouched asserts a plain approve
// (no --review-grain) leaves a previously configured grain exactly as it
// was: absence of the flag is not the same as clearing the field.
func TestApproveWithoutReviewGrainLeavesItUntouched(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	_, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: store.NounEpic, Key: "pay", ReviewGrain: "task"})
	require.NoError(t, err)

	tree := load(t, root)
	epic := resolveNode(t, tree, store.NounEpic, "pay")
	res, err := mutate.Transition(tree, testDeps(), epic, mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)
	require.True(t, res.Changed)

	require.Equal(t, "task", epicByKey(load(t, root), "pay").ReviewGrain,
		"a plain approve must not touch a previously configured grain")
}

// TestNonApproveVerbNeverWritesReviewGrainOntoTheEvent is final-review fix
// round item 5: TransitionInput.ReviewGrain's own doc comment states "every
// other verb ignores this field", but transitionEvent's own guard used to
// test only in.ReviewGrain != nil and the noun, never the verb, so setting
// ReviewGrain alongside a non-approve verb still wrote review_grain onto
// the event's Before/After maps even though the mutation guards inside
// transitionEpic and approveFeaturePlan (each scoped to `case "approve"`)
// never touched the node's own field. No caller in this build does this
// today (the CLI's own approve command is the only writer, and it always
// pairs ReviewGrain with Verb: "approve"), so this pins the guard at the
// seam the doc comment promises rather than at any live defect.
func TestNonApproveVerbNeverWritesReviewGrainOntoTheEvent(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	driveTo(t, root, store.NounEpic, "pay", mutate.TransitionInput{Verb: "approve"})

	grain := "epic"
	tree := load(t, root)
	epic := resolveNode(t, tree, store.NounEpic, "pay")
	res, err := mutate.Transition(tree, testDeps(), epic,
		mutate.TransitionInput{Verb: "cancel", Reason: "no longer needed", ReviewGrain: &grain})
	require.NoError(t, err)
	require.Equal(t, "cancelled", res.Status)

	require.Empty(t, epicByKey(load(t, root), "pay").ReviewGrain,
		"a non-approve verb must not record review_grain on the node itself")

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	ev := eventOfType(evs, "cancel.epic")
	require.NotNil(t, ev, "expected a cancel.epic event")
	_, hasReviewGrain := ev.After["review_grain"]
	require.False(t, hasReviewGrain,
		"cancel is not approve; the event must not carry a review_grain change that never happened on disk")
}

// TestApproveFeaturePlanDoesNotLeakReviewGrainIntoTaskEvents pins the
// review finding: review_grain belongs on the epic or feature the plan gate
// is about, never on the tasks it moves alongside. store.Task has no
// ReviewGrain field, so an approve.task event carrying one would be a
// fabrication, not a record of the task's own state.
func TestApproveFeaturePlanDoesNotLeakReviewGrainIntoTaskEvents(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	grain := "feature"

	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})

	evs, err := events.List(load(t, root))
	require.NoError(t, err)

	featureEv := eventOfType(evs, "approve.feature")
	require.NotNil(t, featureEv, "expected an approve.feature event")
	require.Equal(t, "feature", featureEv.After["review_grain"], "the feature's own event carries the grain")

	taskEv := eventOfType(evs, "approve.task")
	require.NotNil(t, taskEv, "expected an approve.task event for the moved task")
	_, hasReviewGrain := taskEv.After["review_grain"]
	require.False(t, hasReviewGrain, "a task has no review_grain field; its approve event must not fabricate one")
}

// TestApproveFeaturePlanWithoutReviewGrainLeavesItUntouched is the feature
// arm's twin of TestApproveWithoutReviewGrainLeavesItUntouched above: a
// feature configured with a grain before the plan gate keeps it across a
// plain approve (no --review-grain).
func TestApproveFeaturePlanWithoutReviewGrainLeavesItUntouched(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "form-engine/certification-form-loading", ReviewGrain: "task"})
	require.NoError(t, err)

	res := driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve"})
	require.Equal(t, "ready", res.Status)

	require.Equal(t, "task", featureByKey(load(t, root), "form-engine/certification-form-loading").ReviewGrain,
		"a plain approve must not touch a previously configured grain")
}

// TestApproveEpicReviewGrainOverwriteRecordsBeforeAndAfter drives a grain set
// by configure through an approve carrying a different grain: the new value
// persists and the approve.epic event's Before carries the value it replaced,
// not just the value it landed on.
func TestApproveEpicReviewGrainOverwriteRecordsBeforeAndAfter(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	_, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: store.NounEpic, Key: "pay", ReviewGrain: "task"})
	require.NoError(t, err)

	tree := load(t, root)
	epic := resolveNode(t, tree, store.NounEpic, "pay")
	grain := "feature"
	res, err := mutate.Transition(tree, testDeps(), epic, mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "ready", res.Status)

	require.Equal(t, "feature", epicByKey(load(t, root), "pay").ReviewGrain)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	ev := eventOfType(evs, "approve.epic")
	require.NotNil(t, ev)
	require.Equal(t, "task", ev.Before["review_grain"], "the event's Before carries the prior grain")
	require.Equal(t, "feature", ev.After["review_grain"])
}

// TestApproveRefusesIllegalReviewGrainWritesNoEvent extends
// TestApproveRefusesIllegalReviewGrain with the event-ledger half of "no
// state change": a refused approve must not append any event either,
// mirroring configure_test.go's own no-op event-count assertions.
func TestApproveRefusesIllegalReviewGrainWritesNoEvent(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	evsDir := filepath.Join(root, ".tiller", "events")
	before := countFiles(t, evsDir)

	grain := "bogus"
	err := tryTransition(t, root, testDeps(), store.NounEpic, "pay",
		mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})
	require.Error(t, err)
	require.Equal(t, before, countFiles(t, evsDir), "a refused approve records no event")
}

// TestApproveEpicAlreadyReadyRecordsChangedGrain pins the mutated-flag path
// transitionEpic's doc comment promises: a re-approve of an already-ready
// epic is a legal idempotent no-op for status, but a different grain on that
// same call still counts as a change and still gets recorded.
//
// Both approve calls land in the same fixedClock instant, so events.List's
// tiebreak (event id) is what orders them; seqDeps' sequential minter keeps
// that order the same as call order, where testDeps' random EV ids would not.
func TestApproveEpicAlreadyReadyRecordsChangedGrain(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	deps := seqDeps()

	first := "task"
	tree := load(t, root)
	epic := resolveNode(t, tree, store.NounEpic, "pay")
	res, err := mutate.Transition(tree, deps, epic, mutate.TransitionInput{Verb: "approve", ReviewGrain: &first})
	require.NoError(t, err)
	require.Equal(t, "ready", res.Status)

	second := "epic"
	tree2 := load(t, root)
	epic2 := resolveNode(t, tree2, store.NounEpic, "pay")
	res2, err := mutate.Transition(tree2, deps, epic2, mutate.TransitionInput{Verb: "approve", ReviewGrain: &second})
	require.NoError(t, err)
	require.Equal(t, "ready", res2.Status, "status is already ready; the verb is a legal no-op there")
	require.True(t, res2.Changed, "a changed grain on an idempotent re-approve still counts as a change")

	require.Equal(t, "epic", epicByKey(load(t, root), "pay").ReviewGrain)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	var approveEvs []events.Event
	for _, e := range evs {
		if e.Type == "approve.epic" {
			approveEvs = append(approveEvs, e)
		}
	}
	require.Len(t, approveEvs, 2, "both approve calls recorded their own event")
	last := approveEvs[len(approveEvs)-1]
	require.Equal(t, "task", last.Before["review_grain"])
	require.Equal(t, "epic", last.After["review_grain"])
}

// TestApproveFeatureAlreadyReadyRecordsChangedGrain is the feature arm's
// twin of TestApproveEpicAlreadyReadyRecordsChangedGrain: a re-approve of an
// already-ready feature is a legal idempotent no-op for status, but a
// different grain on that same call still counts as a change and still gets
// recorded. Before the fix, transitionFeature's tail treated this call as an
// ordinary no-op (after == before, mutated never set) and silently dropped
// the grain: Changed=false, no event, no write.
func TestApproveFeatureAlreadyReadyRecordsChangedGrain(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve"})

	_, err := mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "form-engine/certification-form-loading", ReviewGrain: "task"})
	require.NoError(t, err)

	grain := "feature"
	res := driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})
	require.Equal(t, "ready", res.Status, "status is already ready; the verb is a legal no-op there")
	require.True(t, res.Changed, "a changed grain on an idempotent re-approve still counts as a change")

	require.Equal(t, "feature", featureByKey(load(t, root), "form-engine/certification-form-loading").ReviewGrain)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	var approveEvs []events.Event
	for _, e := range evs {
		if e.Type == "approve.feature" {
			approveEvs = append(approveEvs, e)
		}
	}
	require.Len(t, approveEvs, 2, "both approve calls recorded their own event")
	last := approveEvs[len(approveEvs)-1]
	require.Equal(t, "task", last.Before["review_grain"])
	require.Equal(t, "feature", last.After["review_grain"])
}

// TestApproveFeatureAlreadyReadyRefusesIllegalReviewGrain pins the
// invalid-value refusal on the ready re-approve path: an illegal grain
// refuses before any write, exactly as the plan gate's own validation does.
func TestApproveFeatureAlreadyReadyRefusesIllegalReviewGrain(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve"})

	evsDir := filepath.Join(root, ".tiller", "events")
	before := countFiles(t, evsDir)

	grain := "bogus"
	err := tryTransition(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "invalid_review_grain", r.Reason())

	require.Equal(t, "", featureByKey(load(t, root), "form-engine/certification-form-loading").ReviewGrain,
		"a refused approve must not write the grain")
	require.Equal(t, before, countFiles(t, evsDir), "a refused approve records no event")
}

// TestApproveFeatureAlreadyReadyRefusesOnceReviewSurfaceRecorded pins the
// changeability rule on the ready re-approve path: a feature that already
// carries its own recorded review_ref has nothing left to gate, the same
// refusal shape configure feature already returns for the identical
// condition (configure.go's refuseReviewSurfaceLocked).
func TestApproveFeatureAlreadyReadyRefusesOnceReviewSurfaceRecorded(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve"})
	setFeatureReviewRefOnDisk(t, root, "form-engine", "certification-form-loading")

	evsDir := filepath.Join(root, ".tiller", "events")
	before := countFiles(t, evsDir)

	grain := "task"
	err := tryTransition(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})
	require.Error(t, err)
	require.Contains(t, err.Error(), "review surface", "the refusal names why")

	require.Equal(t, "", featureByKey(load(t, root), "form-engine/certification-form-loading").ReviewGrain,
		"a refused approve must not write the grain")
	require.Equal(t, before, countFiles(t, evsDir), "a refused approve records no event")
}

// TestApproveFeatureAlreadyReadyWithoutReviewGrainStaysAQuietNoOp pins the
// nil case byte-identical to today: a plain re-approve (no --review-grain)
// of an already-ready feature stays a quiet no-op, Changed=false and no new
// event, exactly as it behaved before this fix.
func TestApproveFeatureAlreadyReadyWithoutReviewGrainStaysAQuietNoOp(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve"})

	evsDir := filepath.Join(root, ".tiller", "events")
	before := countFiles(t, evsDir)

	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")
	res, err := mutate.Transition(tree, deps, feat, mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)
	require.False(t, res.Changed, "a plain re-approve of an already-ready feature stays a quiet no-op")
	require.Equal(t, before, countFiles(t, evsDir), "a quiet no-op records no new event")
}

// setEpicReviewRefOnDisk rewrites an epic.yml to carry a recorded review_ref,
// the epic-grain twin of configure_test.go's setFeatureReviewRefOnDisk: the
// cheap way to stage an epic-grain epic whose own review surface already
// exists (submitted via SubmitNode, or by hand here) without driving the
// full submit arc, which for an epic requires every live feature under it to
// already be in_review or done with its own gates green.
func setEpicReviewRefOnDisk(t *testing.T, root, epicKey string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "epic.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := string(data) + "review_ref:\n  branch: tiller/" + epicKey + "\n  head:\n    kind: git_commit\n    value: \"git:abc\"\n"
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// TestApproveEpicAlreadyReadyRefusesOnceReviewSurfaceRecorded pins the
// Changeability rule (round 2) on the epic's own re-approve grain hook: an
// epic-grain epic has no stored `active` status, so a submitted epic (its
// own PR or review ref already recorded via `submit epic`) sits at `ready`
// exactly like an unsubmitted one. Before this fix, transitionEpic's
// approve-time grain write ran unconditionally, so a re-approve with
// --review-grain would silently overwrite a grain the Changeability rule
// says is locked. This mirrors TestApproveFeatureAlreadyReadyRefusesOnceReviewSurfaceRecorded,
// reusing the same refuseReviewSurfaceLocked shape configureEpic already
// runs for the identical condition.
func TestApproveEpicAlreadyReadyRefusesOnceReviewSurfaceRecorded(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	deps := seqDeps()

	tree := load(t, root)
	epic := resolveNode(t, tree, store.NounEpic, "pay")
	_, err := mutate.Transition(tree, deps, epic, mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)

	_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounEpic, Key: "pay", ReviewGrain: "task"})
	require.NoError(t, err)

	setEpicReviewRefOnDisk(t, root, "pay")

	evsDir := filepath.Join(root, ".tiller", "events")
	before := countFiles(t, evsDir)

	grain := "feature"
	err = tryTransition(t, root, deps, store.NounEpic, "pay",
		mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})
	require.Error(t, err)
	require.Contains(t, err.Error(), "review surface", "the refusal names why")

	require.Equal(t, "task", epicByKey(load(t, root), "pay").ReviewGrain,
		"a refused re-approve must not overwrite the locked grain")
	require.Equal(t, before, countFiles(t, evsDir), "a refused approve records no event")
}

// TestApproveEpicAlreadyReadySubmittedWithoutReviewGrainStaysAQuietNoOp is
// TestApproveEpicAlreadyReadyRefusesOnceReviewSurfaceRecorded's nil-flag
// twin: a plain re-approve (no --review-grain) of that same submitted epic
// stays exactly as it behaved before this fix, a quiet no-op with no event,
// because the new guard only ever runs when ReviewGrain is set.
func TestApproveEpicAlreadyReadySubmittedWithoutReviewGrainStaysAQuietNoOp(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	deps := seqDeps()

	tree := load(t, root)
	epic := resolveNode(t, tree, store.NounEpic, "pay")
	_, err := mutate.Transition(tree, deps, epic, mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)
	setEpicReviewRefOnDisk(t, root, "pay")

	evsDir := filepath.Join(root, ".tiller", "events")
	before := countFiles(t, evsDir)

	tree2 := load(t, root)
	epic2 := resolveNode(t, tree2, store.NounEpic, "pay")
	res, err := mutate.Transition(tree2, deps, epic2, mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)
	require.False(t, res.Changed, "a plain re-approve of an already-ready submitted epic stays a quiet no-op")
	require.Equal(t, before, countFiles(t, evsDir), "a quiet no-op records no new event")
}

// TestApproveEpicRefusesReviewGrainOnceTerminal pins the terminal half of the
// Changeability rule on the same hook: a cancelled or archived epic has
// nothing left to submit, so --review-grain refuses even though no PR or
// review_ref was ever recorded, matching refuseReviewSurfaceLocked's
// terminal arm exactly as configureEpic already does.
func TestApproveEpicRefusesReviewGrainOnceTerminal(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"cancelled", "archived"} {
		t.Run(status, func(t *testing.T) {
			root := writeOwnedRepo(t)
			mustAddEpic(t, root, "pay")
			setEpicStatusOnDisk(t, root, "pay", "draft", status)

			evsDir := filepath.Join(root, ".tiller", "events")
			before := countFiles(t, evsDir)

			grain := "task"
			err := tryTransition(t, root, testDeps(), store.NounEpic, "pay",
				mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})
			require.Error(t, err)
			require.Contains(t, err.Error(), "review surface", "the refusal names why")
			require.Contains(t, err.Error(), status, "the refusal names the terminal status")

			require.Equal(t, "", epicByKey(load(t, root), "pay").ReviewGrain,
				"a refused approve must not write the grain")
			require.Equal(t, before, countFiles(t, evsDir), "a refused approve records no event")
		})
	}
}

func TestApproveFeaturePlanRefusesReviewGrainWithAffectedTaskInFlightWithoutWriting(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	taskID := onlyTaskID(t, load(t, root))
	setTaskStatusOnDisk(t, root, taskID, "in_progress")
	before := snapshotDir(t, root)
	eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))

	grain := store.ReviewGrainFeature
	err := tryTransition(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})

	var blocked *mutate.ReviewGrainChangeBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, mutate.ReasonReviewGrainWorkInFlight, blocked.Reason)
	require.Equal(t, taskID, blocked.BlockerID)
	require.Equal(t, "specced", featureByKey(load(t, root), "form-engine/certification-form-loading").Status)
	require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")))
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestApproveFeatureAgainRefusesSameReviewGrainChangeAsConfigureWithoutWriting(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve"})
	taskID := onlyTaskID(t, load(t, root))
	setTaskStatusOnDisk(t, root, taskID, "in_review")
	before := snapshotDir(t, root)
	eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))

	grain := store.ReviewGrainFeature
	err := tryTransition(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})

	var blocked *mutate.ReviewGrainChangeBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, mutate.ReasonReviewGrainWorkInFlight, blocked.Reason)
	require.Equal(t, taskID, blocked.BlockerID)
	require.Empty(t, featureByKey(load(t, root), "form-engine/certification-form-loading").ReviewGrain)
	require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")))
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestApproveEpicRefusesReviewGrainThatWouldRetargetChildSurfaceWithoutWriting(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	mustAddEpic(t, root, "pay")
	mustAddFeature(t, root, "pay/checkout")
	setFeatureReviewRefOnDisk(t, root, "pay", "checkout")
	before := snapshotDir(t, root)
	eventsBefore := countFiles(t, filepath.Join(root, ".tiller", "events"))

	grain := store.ReviewGrainFeature
	err := tryTransition(t, root, testDeps(), store.NounEpic, "pay",
		mutate.TransitionInput{Verb: "approve", ReviewGrain: &grain})

	var blocked *mutate.ReviewGrainChangeBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, mutate.ReasonReviewGrainWorkInFlight, blocked.Reason)
	require.Equal(t, store.NounFeature, blocked.BlockerNoun)
	require.Equal(t, "draft", epicByKey(load(t, root), "pay").Status)
	require.Equal(t, eventsBefore, countFiles(t, filepath.Join(root, ".tiller", "events")))
	requireNoWrites(t, before, snapshotDir(t, root))
}
