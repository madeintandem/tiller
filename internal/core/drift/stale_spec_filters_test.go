package drift_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// objectCounts reads git's own object database counts, so a test can prove a
// read wrote nothing there.
func objectCounts(t *testing.T, root string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "count-objects", "-v").Output()
	require.NoError(t, err)
	return string(out)
}

// Under `* text=auto` a spec checked out with CRLF line endings is the same
// content as its LF committed blob. stale_spec hashes the working tree the way
// `git add` would, so the unchanged file reads clean, a real edit still reads
// stale, and neither read writes an object.
func TestStaleSpecHashesTheWorkingTreeThroughCheckinConversion(t *testing.T) {
	root := scratchProjectGit(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitattributes"), []byte("* text=auto\n"), 0o644))
	gitCommitAll(t, root, "normalize line endings")
	deps := seqDeps()
	epicRes, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)
	approveEpic(t, root, deps, epicRes.ID)
	featID, _ := driveFeatureToDoneGit(t, root, deps, "form-engine", "feat-done", "task-a", nil)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})
	gitCommitAll(t, root, "settle")

	spec := featurePath(root, "form-engine", "feat-done", "feature.md")
	lf, err := os.ReadFile(spec)
	require.NoError(t, err)
	require.NotContains(t, string(lf), "\r")
	require.NoError(t, os.WriteFile(spec, []byte(strings.ReplaceAll(string(lf), "\n", "\r\n")), 0o644))

	before := objectCounts(t, root)
	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec), "a CRLF checkout of the verified spec is not an edit")
	require.Empty(t, uncoveredFor(rep, drift.MarkerStaleSpec))
	require.Equal(t, before, objectCounts(t, root), "hashing the working tree writes no object")

	edited := strings.ReplaceAll(string(lf), "Real specification prose", "Rewritten specification prose")
	require.NoError(t, os.WriteFile(spec, []byte(strings.ReplaceAll(edited, "\n", "\r\n")), 0o644))
	rep, err = drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	cs := candidatesFor(rep, drift.MarkerStaleSpec)
	require.Len(t, cs, 1, "a real content edit is still stale")
	require.Equal(t, featID, cs[0].ID)
	require.Equal(t, before, objectCounts(t, root), "hashing the working tree writes no object")
}
