package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// nestProject moves everything a fixture laid at the repository root into sub
// and commits the move, turning a root project into a nested one (a Tiller root
// below the repository root) without disturbing the merge history sync reads.
func nestProject(t *testing.T, repo, sub string) string {
	t.Helper()
	gitCommitAllM(t, repo, "fixture state")
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
	gitCommitAllM(t, repo, "nest the project")
	return project
}

// A nested project records its file and directory surfaces from the
// integration commit in its own coordinates. Repository-root siblings sharing
// each surface's name must not stand in for the project's own paths.
func TestSyncRecordsNestedProjectSurfacesInProjectCoordinates(t *testing.T) {
	t.Parallel()
	repo := layEpicRollupGit(t)
	project := nestProject(t, repo, "project")
	for path, data := range map[string]string{
		"project/src/a.swift":  "struct A {}\n",
		"project/chrome.swift": "struct Chrome {}\n",
		"src/a.swift":          "root sibling\n",
		"src/b.swift":          "root sibling\n",
		"chrome.swift":         "root chrome\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(repo, path)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(repo, path), []byte(data), 0o644))
	}
	gitCommitAllM(t, repo, "add code")
	setFeatureSurfacePath(t, project, "form-engine", "nrca-form-loading", "src", "chrome.swift")

	res, err := mutate.Sync(load(t, project), syncDeps(), vcs.New(project), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Skipped, "a nested project's surfaces are recordable, not a skip")

	f := load(t, project).Features["F-aaaaaa"]
	require.Equal(t, "done", f.Status)
	require.Len(t, f.VerifiedAgainst.Surfaces, 2)
	dir, file := f.VerifiedAgainst.Surfaces[0], f.VerifiedAgainst.Surfaces[1]
	require.Equal(t, store.KindGitTree, dir.Kind)
	require.Equal(t, "git:"+gitOutM(t, repo, "rev-parse", "HEAD:project/src"), dir.Value)
	require.Equal(t, []string{"src/a.swift"}, dir.Inventory)
	require.Equal(t, store.KindGitBlob, file.Kind)
	require.Equal(t, "git:"+gitOutM(t, repo, "rev-parse", "HEAD:project/chrome.swift"), file.Value)
	require.Equal(t, []string{"chrome.swift"}, file.Inventory)
}
