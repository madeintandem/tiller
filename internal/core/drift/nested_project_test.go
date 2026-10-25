package drift_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// nestDriftProject moves a root fixture into sub and commits the move, so the
// Tiller project sits below the repository root.
func nestDriftProject(t *testing.T, repo, sub string) string {
	t.Helper()
	gitCommitAll(t, repo, "fixture state")
	project := filepath.Join(repo, sub)
	entries, err := os.ReadDir(repo)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(project, 0o755))
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		require.NoError(t, os.Rename(filepath.Join(repo, entry.Name()), filepath.Join(project, entry.Name())))
	}
	gitCommitAll(t, repo, "nest the project")
	return project
}

// A nested project's recorded surfaces are re-read in the same project
// coordinates sync recorded them in: an untouched surface compares clean, and
// its deletion is still the vanish candidate.
func TestStaleSpecReadsNestedProjectSurfacesInProjectCoordinates(t *testing.T) {
	repo, featID := scratchDoneFeatureWithSurface(t)
	setManifestChecks(t, repo, []string{drift.MarkerStaleSpec})
	project := nestDriftProject(t, repo, "project")

	rep, err := drift.Run(loadTree(t, project), vcs.NewHeadSnapshot(vcs.New(project)))
	require.NoError(t, err)
	require.Empty(t, uncoveredFor(rep, drift.MarkerStaleSpec), "the recorded surface resolves inside the project")
	require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec))
	require.Equal(t, 2, rep.Compared, "the feature and its rolled-up epic both compare clean")

	removeAndCommit(t, project, "app/calc/zone.rb")
	rep, err = drift.Run(loadTree(t, project), vcs.NewHeadSnapshot(vcs.New(project)))
	require.NoError(t, err)
	cs := candidatesFor(rep, drift.MarkerStaleSpec)
	require.Len(t, cs, 1)
	require.Equal(t, featID, cs[0].ID)
	require.Contains(t, cs[0].Files, "app/calc/zone.rb")
}
