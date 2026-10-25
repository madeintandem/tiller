package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestReadoutEpicSpecsRefusesEscapingProse(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	private := filepath.Join(outside, "private.md")
	require.NoError(t, os.WriteFile(private, []byte("# Confidential real prose\n"), 0o644))
	key := "bounded"
	epics := filepath.Join(root, ".tiller", "epics", key)
	require.NoError(t, os.MkdirAll(epics, 0o755))
	require.NoError(t, os.Symlink(private, filepath.Join(epics, "epic.md")))
	tree := &store.Tree{Root: root, Epics: map[string]*store.Epic{
		"E-aaaaaa": {ID: "E-aaaaaa", Key: &key},
	}}

	got := readoutEpicSpecs(tree)
	require.False(t, got["E-aaaaaa"].ProseWritten, "outside prose must not qualify a repository spec")
}

func TestImportDocsRefusesEscapingDestinationBeforeCreatingDirectories(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(source, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "nested", "design.md"), []byte("imported prose\n"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".tiller", "design")))

	copied, err := importDocs(root, source, filepath.Join(".tiller", "design"))
	require.Error(t, err)
	require.Zero(t, copied)
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries, "an unsafe import must not create outside subdirectories")
}

func TestRepoRootRetainsUnsafeNearestCheckoutForLoadRefusal(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "tiller.yml"), []byte("schema_version: 7\n"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".tiller")))
	t.Chdir(root)

	got, err := repoRoot()
	require.NoError(t, err)
	require.Equal(t, root, got)
	_, err = store.LoadManifest(got)
	require.Error(t, err)
}

func TestRepoRootWalksPastAnUnrelatedTillerFile(t *testing.T) {
	parent := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(parent, ".tiller"), []byte("not a project\n"), 0o644))
	cwd := filepath.Join(parent, "work")
	require.NoError(t, os.Mkdir(cwd, 0o755))
	t.Chdir(cwd)

	got, err := repoRoot()
	require.NoError(t, err)
	require.Equal(t, cwd, got, "a regular file named .tiller does not make its directory a project")
}

// skipUnlessPermissionsEnforced skips a test whose fixture relies on a mode-000
// directory being unreadable. Root bypasses directory permissions, and only
// Unix permission bits are modeled here, so the test would pass vacuously.
func skipUnlessPermissionsEnforced(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("mode-000 directories are not unreadable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
}

// TestUnreadableNestedMarkerRefusesInsteadOfMutatingParent pins bug hunt
// defect 06 (an unreadable nested marker redirects a mutation to the parent).
// A child .tiller that cannot be inspected is a boundary refusal, not absence:
// an ordinary mutation from the child answers an environment error and leaves
// both projects exactly as they were.
func TestUnreadableNestedMarkerRefusesInsteadOfMutatingParent(t *testing.T) {
	skipUnlessPermissionsEnforced(t)
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.Mkdir(child, 0o755))
	for _, dir := range []string{parent, child} {
		t.Chdir(dir)
		_, stderr, code := execCLI(t, "init", "--name", "P", "--key", "p", "--kind", "greenfield")
		require.Equal(t, 0, code, stderr)
		_, stderr, code = execCLI(t, "add", "contributor", "jc", "--name", "JC")
		require.Equal(t, 0, code, stderr)
	}
	locked := filepath.Join(child, ".tiller")
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	parentFiles, parentDirs := snapshotWholeTree(t, filepath.Join(parent, ".tiller"))
	childEntries, err := os.ReadDir(child)
	require.NoError(t, err)

	t.Chdir(child)
	out, _, code := execCLI(t, "add", "chore", "upkeep", "--title", "Upkeep", "--contributor", "jc", "--json")
	require.Equal(t, 3, code, out)
	require.Contains(t, out, `"reason": "project_marker_unreadable"`)

	afterFiles, afterDirs := snapshotWholeTree(t, filepath.Join(parent, ".tiller"))
	require.Equal(t, parentFiles, afterFiles, "the parent project must not absorb the child's mutation")
	require.ElementsMatch(t, parentDirs, afterDirs)
	require.NoError(t, os.Chmod(locked, 0o755))
	afterEntries, err := os.ReadDir(child)
	require.NoError(t, err)
	require.Equal(t, len(childEntries), len(afterEntries))
	require.NoDirExists(t, filepath.Join(locked, "chores"), "the child project is unchanged too")
}

// TestRepoRootDiscoversProjectFromOrdinarySubdirectory keeps genuine absence
// as absence: a subdirectory with no marker walks up to its project.
func TestRepoRootDiscoversProjectFromOrdinarySubdirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte("schema_version: 1\n"), 0o644))
	cwd := filepath.Join(root, "src", "deep")
	require.NoError(t, os.MkdirAll(cwd, 0o755))
	t.Chdir(cwd)

	got, err := repoRoot()
	require.NoError(t, err)
	require.Equal(t, root, got)
}
