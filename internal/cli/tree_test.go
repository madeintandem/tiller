package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// layMapTree lays a deterministic one-epic tree for the tree command: foundation
// with two features (authn and db-core), db-core carrying one task, and a
// db-core feature.eval.md so the --files footprint's conditional eval line is
// exercised (authn has none, so both branches render). No epic.eval.md: an epic
// carries no eval contract, so its footprint is its prose and its state alone
// (artifact-schema.md, Epics). The features sort by key (authn before db-core)
// so the map is stable across runs; the only random value is the minted ids,
// which the goldens normalize.
func layMapTree(t *testing.T) {
	t.Helper()
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "foundation", "--title", "Foundation", "--contributor", "jc")
	mustRun(t, "add", "feature", "foundation/db-core", "--title", "Database core",
		"--change-type", "create", "--contributor", "jc")
	mustRun(t, "add", "feature", "foundation/authn", "--title", "Authentication foundation",
		"--change-type", "create", "--contributor", "jc")
	mustRun(t, "add", "task", "foundation/db-core/define-the-schema", "--title", "Define the schema", "--contributor", "jc")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	eval := []byte("---\nnode: x\ncriteria: []\n---\n")
	require.NoError(t, os.WriteFile(filepath.Join(cwd, ".tiller", "epics", "foundation",
		"features", "db-core", "feature.eval.md"), eval, 0o644))
}

// mustRun runs a CLI command and fails the test if it does not exit 0.
func mustRun(t *testing.T, args ...string) {
	t.Helper()
	_, _, code := execCLI(t, args...)
	require.Equalf(t, 0, code, "%v should exit 0", args)
}

// readGolden reads a committed golden relative to the package directory. It is
// called before layMapTree chdirs into the temp repo, so the testdata path still
// resolves, the same ordering the other read goldens use.
func readGolden(t *testing.T, name string) string {
	t.Helper()
	golden, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(golden)
}

// TestTreeDefaultGolden pins the default whole-project map: one line per node
// carrying its local key segment and title, sized to the tree's own widest
// label; the drill paths live under --files.
func TestTreeDefaultGolden(t *testing.T) {
	golden := readGolden(t, "tree_default.txt")
	layMapTree(t)
	out, _, code := execCLI(t, "tree")
	require.Equal(t, 0, code)
	require.Equal(t, golden, normalizeIDs(out))
}

// TestTreeFilesGolden pins the --files map: each node expands to its full file
// footprint (the prose .md, the .yml, and the *.eval.md when a feature has one).
func TestTreeFilesGolden(t *testing.T) {
	golden := readGolden(t, "tree_files.txt")
	layMapTree(t)
	out, _, code := execCLI(t, "tree", "--files")
	require.Equal(t, 0, code)
	require.Equal(t, golden, normalizeIDs(out))
}

// TestTreeStatusGolden pins the --status map: each node is annotated with its
// stored status.
func TestTreeStatusGolden(t *testing.T) {
	golden := readGolden(t, "tree_status.txt")
	layMapTree(t)
	out, _, code := execCLI(t, "tree", "--status")
	require.Equal(t, 0, code)
	require.Equal(t, golden, normalizeIDs(out))
}

// TestTreeSubtreeGolden pins the rooted subtree map for `tree epic foundation`:
// the epic and its descendants, with no other epic at the top level.
func TestTreeSubtreeGolden(t *testing.T) {
	golden := readGolden(t, "tree_subtree.txt")
	layMapTree(t)
	out, _, code := execCLI(t, "tree", "epic", "foundation")
	require.Equal(t, 0, code)
	require.Equal(t, golden, normalizeIDs(out))
}

// TestTreeJSONGolden pins the structured --json envelope, the tooling shape: the
// project key and the nested nodes, each carrying identity, status, owner, the
// epic activity, the prose path, the file footprint, and its children.
func TestTreeJSONGolden(t *testing.T) {
	golden := readGolden(t, "tree.json")
	layMapTree(t)
	out, _, code := execCLI(t, "tree", "--json")
	require.Equal(t, 0, code)
	require.Equal(t, golden, normalizeIDs(out))
}

// TestTreeNodePaths pins where the drill paths live after the tree change: the
// default map is for orienting and spends no columns on paths (real trees
// averaged 144 columns with them and drowned a laptop terminal), while
// --files is the drill view, where every node's footprint names its own
// folder's files.
func TestTreeNodePaths(t *testing.T) {
	layMapTree(t)
	out, _, code := execCLI(t, "tree")
	require.Equal(t, 0, code)
	require.NotContains(t, out, ".tiller/", "the default map carries no paths")

	out, _, code = execCLI(t, "tree", "--files")
	require.Equal(t, 0, code)
	require.Contains(t, out, ".tiller/epics/foundation/epic.md")
	require.Contains(t, out, ".tiller/epics/foundation/features/db-core/feature.md")
	require.Contains(t, out, ".tiller/epics/foundation/features/db-core/tasks/define-the-schema/task.md")
}

// TestTreeFeatureSubtree confirms a feature roots its own subtree: the feature at
// the top and its task beneath it, with no sibling feature.
func TestTreeFeatureSubtree(t *testing.T) {
	layMapTree(t)
	out, _, code := execCLI(t, "tree", "feature", "foundation/db-core")
	require.Equal(t, 0, code)
	require.Contains(t, out, "db-core")
	require.Contains(t, out, "Define the schema")
	require.NotContains(t, out, "authn", "a feature subtree excludes its siblings")
}

// TestTreeTaskRootExits2 confirms a task is not a tree root: tree
// refuses it as misuse (exit 2), because a task has no subtree.
func TestTreeTaskRootExits2(t *testing.T) {
	layMapTree(t)
	tid := soleTaskID(t)
	out, _, code := execCLI(t, "tree", "task", tid, "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, "unscoped_noun")
}

// TestTreeOneArgIsMisuse confirms a lone noun with no node is rejected (exit 2),
// the same arg-count misuse status enforces.
func TestTreeOneArgIsMisuse(t *testing.T) {
	layMapTree(t)
	_, _, code := execCLI(t, "tree", "feature")
	require.Equal(t, 2, code)
}

// TestTreeOutsideRepoExits3 confirms tree refuses outside a Tiller repo with an
// environment error (exit 3).
func TestTreeOutsideRepoExits3(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "tree", "--json")
	require.Equal(t, 3, code)
}

// TestTreeEpicsFollowDependencyOrder pins Task A1 end to end at the
// CLI surface: `tiller tree`'s top-level epics come out in dependency order,
// not key order. "alpha" records --depends-on "zeta", so key order (alpha
// before zeta) and dependency order (zeta, the foundation, before alpha, what
// rests on it) disagree; layMapTree's single-epic fixture never exercises this
// (nothing to depend on), so this stands up its own two-epic tree.
func TestTreeEpicsFollowDependencyOrder(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "zeta", "--title", "Zeta", "--contributor", "jc")
	mustRun(t, "add", "epic", "alpha", "--title", "Alpha", "--depends-on", "zeta", "--contributor", "jc")

	out, _, code := execCLI(t, "tree")
	require.Equal(t, 0, code)
	zetaIdx := strings.Index(out, "zeta")
	alphaIdx := strings.Index(out, "alpha")
	require.GreaterOrEqual(t, zetaIdx, 0, "zeta is listed")
	require.Greater(t, alphaIdx, zetaIdx,
		"zeta (the dependency) leads alpha (what depends on it) even though alpha sorts first alphabetically")

	// The --json envelope carries the same order the rendered map does.
	out, _, code = execCLI(t, "tree", "--json")
	require.Equal(t, 0, code)
	zetaIdx = strings.Index(out, `"key": "zeta"`)
	alphaIdx = strings.Index(out, `"key": "alpha"`)
	require.GreaterOrEqual(t, zetaIdx, 0)
	require.Greater(t, alphaIdx, zetaIdx)
}
