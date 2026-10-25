package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// Staging a correct-eval onto an open amendment (the apply side is approval's
// own business). These drive the lantern fixture because the auto-pin has to
// hash a real feature.eval.md in a real git working tree.

func TestStageOperationCorrectEvalPinsTheEvalRevisionToTheWorkingTree(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	a := openAmendment(t, root, deps, "eval-bindings")

	res, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{
		Op: store.AmendmentOpCorrectEval, Target: "app-experience/fast-logging",
		Reason: "the binding file moved", Retire: []string{"C2"},
	})
	require.NoError(t, err)
	require.Equal(t, store.AmendmentOpCorrectEval, res.Op.Op)
	require.Equal(t, featureID, res.Op.Target, "a key target resolves to its id")
	require.Equal(t, "the binding file moved", res.Op.Reason)
	require.Equal(t, []string{"C2"}, res.Op.Retire)
	require.Equal(t, map[string]string{"eval_revision": blobOf(t, root, lanternEvalRel)}, res.Op.Preconditions)

	reloaded := loadAmendment(t, root, a.ID)
	require.Len(t, reloaded.Operations, 1)
	require.Equal(t, res.Op, reloaded.Operations[0], "what is on disk is what staging returned")

	// The pin is the value approval compares against: right after staging the
	// precondition reads fresh, and an edit to the contract afterwards stales it.
	validity, err := mutate.EvaluateAmendmentPreconditions(load(t, root), deps, reloaded)
	require.NoError(t, err)
	require.Len(t, validity, 1)
	require.Equal(t, "eval_revision", validity[0].Precondition)
	require.Equal(t, mutate.PreconditionValid, validity[0].Status, validity[0].Note)

	require.NoError(t, os.WriteFile(filepath.Join(root, lanternEvalRel), []byte("# edited after staging\n"), 0o644))
	validity, err = mutate.EvaluateAmendmentPreconditions(load(t, root), deps, reloaded)
	require.NoError(t, err)
	require.Equal(t, mutate.PreconditionStale, validity[0].Status)
}

func TestStageOperationCorrectEvalStoresTrimmedRetireIDs(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	a := openAmendment(t, root, deps, "eval-bindings")

	res, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{
		Op: store.AmendmentOpCorrectEval, Target: "app-experience/fast-logging", Reason: "r",
		Retire: []string{" C2", "C3 "},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"C2", "C3"}, res.Op.Retire)
	require.Equal(t, []string{"C2", "C3"}, loadAmendment(t, root, a.ID).Operations[0].Retire)
}

func TestStageOperationCorrectEvalKeepsASuppliedEvalRevision(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	a := openAmendment(t, root, deps, "eval-bindings")

	res, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{
		Op: store.AmendmentOpCorrectEval, Target: "app-experience/fast-logging", Reason: "r",
		Preconditions: map[string]string{"eval_revision": "git:reviewedbytes"},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"eval_revision": "git:reviewedbytes"}, res.Op.Preconditions)
}

func TestStageOperationCorrectEvalPinsBesideOtherSuppliedPreconditions(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	a := openAmendment(t, root, deps, "eval-bindings")

	res, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{
		Op: store.AmendmentOpCorrectEval, Target: "app-experience/fast-logging", Reason: "r",
		Preconditions: map[string]string{"target_status": "done"},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"target_status": "done",
		"eval_revision": blobOf(t, root, lanternEvalRel),
	}, res.Op.Preconditions)
}

func TestStageOperationCorrectEvalDoesNotMutateTheCallersPreconditions(t *testing.T) {
	t.Parallel()
	root, deps, _, _ := lanternFixture(t)
	a := openAmendment(t, root, deps, "eval-bindings")
	supplied := map[string]string{"target_status": "done"}

	_, err := mutate.StageOperation(load(t, root), deps, a, mutate.StageInput{
		Op: store.AmendmentOpCorrectEval, Target: "app-experience/fast-logging", Reason: "r",
		Preconditions: supplied,
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"target_status": "done"}, supplied)
}

func TestStageOperationCorrectEvalRefusals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		in         mutate.StageInput
		prepare    func(t *testing.T, root string)
		wantReason string
		wantMsg    string
	}{
		{
			name:       "a task target",
			in:         mutate.StageInput{Op: store.AmendmentOpCorrectEval, Target: "app-experience/fast-logging/ipad-logging", Reason: "r"},
			wantReason: "correct_eval_not_feature",
			wantMsg:    `^correct-eval targets a done feature's acceptance contract; T-\w+ is a task$`,
		},
		{
			name:       "an epic target",
			in:         mutate.StageInput{Op: store.AmendmentOpCorrectEval, Target: "app-experience", Reason: "r"},
			wantReason: "epic_target",
		},
		{
			name:       "an unknown target",
			in:         mutate.StageInput{Op: store.AmendmentOpCorrectEval, Target: "app-experience/nope", Reason: "r"},
			wantReason: "unknown_target",
		},
		{
			name:       "no reason",
			in:         mutate.StageInput{Op: store.AmendmentOpCorrectEval, Target: "app-experience/fast-logging"},
			wantReason: "amendment_shape_invalid",
		},
		{
			name: "a feature with no eval file",
			in:   mutate.StageInput{Op: store.AmendmentOpCorrectEval, Target: "app-experience/fast-logging", Reason: "r"},
			prepare: func(t *testing.T, root string) {
				require.NoError(t, os.Remove(filepath.Join(root, lanternEvalRel)))
			},
			wantReason: "correct_eval_no_contract",
			wantMsg:    "feature.eval.md",
		},
		{
			name: "a by on a correct-eval",
			in: mutate.StageInput{Op: store.AmendmentOpCorrectEval, Target: "app-experience/fast-logging", Reason: "r",
				By: "app-experience/fast-logging"},
			wantReason: "supersede_by_misplaced",
		},
		{
			name: "a retire on a revise",
			in: mutate.StageInput{Op: store.AmendmentOpRevise, Target: "app-experience/fast-logging",
				Retire: []string{"C1"}},
			wantReason: "retire_misplaced",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, deps, _, _ := lanternFixture(t)
			a := openAmendment(t, root, deps, "eval-bindings")
			if tc.prepare != nil {
				tc.prepare(t, root)
			}
			before := snapshotDir(t, root)

			_, err := mutate.StageOperation(load(t, root), deps, a, tc.in)
			require.Error(t, err)
			require.Equal(t, tc.wantReason, reasonOf(t, err))
			var coder interface{ ExitCode() int }
			require.ErrorAs(t, err, &coder)
			require.Equal(t, 2, coder.ExitCode())
			if tc.wantMsg != "" {
				require.Regexp(t, tc.wantMsg, err.Error())
			}
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}
