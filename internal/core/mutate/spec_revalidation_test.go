package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// Issue #145: a specced feature's stage-2 output can change after spec. Re-running
// spec and the plan gate both re-validate it rather than trusting the earlier spec.

const featureDir = ".tiller/epics/form-engine/features/certification-form-loading"

// laySpeccedFeature walks the shared fixture to a specced feature under an
// approved epic.
func laySpeccedFeature(t *testing.T) string {
	t.Helper()
	root := laySpeccableFeature(t, "create")
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	return root
}

// breakContract leaves feature.eval.md written but invalid: an empty criteria list.
func breakContract(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, featureDir, "feature.eval.md"),
		[]byte("---\nnode: F-aaaaaa\ncriteria: []\n---\n"), 0o644))
}

// unwriteOnlyTask empties the feature's only task.md, which reads as not written.
func unwriteOnlyTask(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, store.TaskRelDir("form-engine", "certification-form-loading", "transcribe-the-schema"), store.TaskProseFile)
	require.NoError(t, os.WriteFile(path, nil, 0o644))
}

func TestReSpecOfAnUnchangedSpeccedFeatureIsAQuietNoOp(t *testing.T) {
	t.Parallel()
	root := laySpeccedFeature(t)
	res := driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	require.False(t, res.Changed)
	require.Equal(t, "specced", res.Status)
}

func TestReSpecRefusesAContractBrokenAfterSpec(t *testing.T) {
	t.Parallel()
	root := laySpeccedFeature(t)
	breakContract(t, root)

	err := trySpec(t, root)
	var ce *store.EvalContractError
	require.ErrorAs(t, err, &ce, "a contract malformed after spec is the same misuse as before it")
	require.Equal(t, 2, ce.ExitCode())
	require.Equal(t, "specced", load(t, root).Features["F-aaaaaa"].Status)
}

func TestReSpecRefusesATaskUnwrittenAfterSpec(t *testing.T) {
	t.Parallel()
	root := laySpeccedFeature(t)
	unwriteOnlyTask(t, root)

	err := trySpec(t, root)
	var inc *mutate.SpecIncompleteError
	require.ErrorAs(t, err, &inc)
	require.Equal(t, "specced", load(t, root).Features["F-aaaaaa"].Status)
}

func TestPlanGateRefusesAContractBrokenAfterSpec(t *testing.T) {
	t.Parallel()
	root := laySpeccedFeature(t)
	breakContract(t, root)

	err := tryTransition(t, root, testDeps(), store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	var ce *store.EvalContractError
	require.ErrorAs(t, err, &ce, "the plan gate never approves an invalid contract")
	tree := load(t, root)
	require.Equal(t, "specced", tree.Features["F-aaaaaa"].Status)
	require.Equal(t, "proposed", tree.Tasks["T-aaaaaa"].Status, "a refused plan gate moves no task")
}

func TestPlanGateRefusesATaskUnwrittenAfterSpec(t *testing.T) {
	t.Parallel()
	root := laySpeccedFeature(t)
	unwriteOnlyTask(t, root)

	err := tryTransition(t, root, testDeps(), store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	var inc *mutate.SpecIncompleteError
	require.ErrorAs(t, err, &inc)
	require.Equal(t, "specced", load(t, root).Features["F-aaaaaa"].Status)
}
