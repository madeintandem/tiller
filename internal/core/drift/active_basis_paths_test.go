package drift_test

import (
	"os"
	"testing"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// TestActiveBasisNamesFilesByRepositoryPath pins one form for a file name
// across a basis finding: its Files and the sentence that leads it both name
// the pinned file by its path from the repository root, the form stale_spec
// already uses, never the bare file name the pin records.
func TestActiveBasisNamesFilesByRepositoryPath(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
	deps := seqDeps()
	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, _ = layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")

	const spec = ".tiller/epics/form-engine/features/feat-active/feature.md"
	const eval = ".tiller/epics/form-engine/features/feat-active/feature.eval.md"
	appendToFile(t, featurePath(root, "form-engine", "feat-active", "feature.md"), "\nEdited after approval.\n")

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	cs := candidatesFor(rep, drift.MarkerActiveBasis)
	require.Len(t, cs, 1)
	require.Equal(t, []string{spec}, cs[0].Files)
	require.Equal(t, spec+" changed since the accepted basis", cs[0].Detail)

	require.NoError(t, os.Remove(featurePath(root, "form-engine", "feat-active", "feature.eval.md")))
	rep, err = drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	cs = candidatesFor(rep, drift.MarkerActiveBasis)
	require.Len(t, cs, 1)
	require.Equal(t, eval+" deleted since the accepted basis", cs[0].Detail)
	require.Equal(t, []string{spec + " also changed since the accepted basis"}, cs[0].Context)
}
