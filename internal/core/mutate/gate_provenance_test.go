package mutate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// TestCheckDirtyTreeRecordsWorktreeFingerprint confirms the honest gate
// provenance fix: when the working tree is dirty (the code under test is
// uncommitted, the usual case for `check`), the recorded `against` is a sha256
// working-tree fingerprint, not the HEAD commit, which does not contain the
// tested code (artifact-schema.md, The stored gate block).
func TestCheckDirtyTreeRecordsWorktreeFingerprint(t *testing.T) {
	t.Parallel()
	root, _ := layGatedTask(t, "true", "true")
	gitInitM(t, root)
	// writeGatedRepo already committed tiller.yml alone; everything driveGatedTask
	// wrote after that (the epic, the feature, its task) stays uncommitted, so the
	// working tree is already dirty here. Committing one more file (a README) just
	// gives HEAD a second, later commit to be compared against.
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("hi\n"), 0o644))
	gitRunM(t, root, "add", "README.md")
	gitRunM(t, root, "commit", "-m", "init")
	gitRunM(t, root, "branch", "-M", "main")

	tree := load(t, root)
	require.True(t, mustDirty(t, root), "the .tiller tree is uncommitted, so the tree is dirty")
	task := resolveNode(t, tree, store.NounTask, "T-aaaaaa")
	_, err := mutate.Check(tree, testDeps(), vcs.New(root), task, mutate.CheckInput{})
	require.NoError(t, err)

	got := load(t, root).Tasks["T-aaaaaa"].GatesBlock.Gates["build"]
	require.Equal(t, store.KindSHA256, got.Against.Kind, "a dirty tree records a sha256 working-tree fingerprint")
	require.True(t, strings.HasPrefix(got.Against.Value, "sha256:"), "the against value carries the sha256 prefix")
}

// TestCheckCleanTreeRecordsHeadCommit confirms the clean-tree path is unchanged:
// when nothing is uncommitted, HEAD does contain the tested code, so the recorded
// `against` is the HEAD git_commit.
func TestCheckCleanTreeRecordsHeadCommit(t *testing.T) {
	t.Parallel()
	root, _ := layGatedTask(t, "true", "true")
	gitInitM(t, root)
	gitCommitAllM(t, root, "lay tree")
	gitRunM(t, root, "branch", "-M", "main")

	tree := load(t, root)
	require.False(t, mustDirty(t, root), "a fully committed tree is clean")
	task := resolveNode(t, tree, store.NounTask, "T-aaaaaa")
	_, err := mutate.Check(tree, testDeps(), vcs.New(root), task, mutate.CheckInput{})
	require.NoError(t, err)

	got := load(t, root).Tasks["T-aaaaaa"].GatesBlock.Gates["build"]
	require.Equal(t, store.KindGitCommit, got.Against.Kind, "a clean tree pins the HEAD commit")
	require.True(t, strings.HasPrefix(got.Against.Value, "git:"), "the against value carries the git commit prefix")
}

func mustDirty(t *testing.T, root string) bool {
	t.Helper()
	d, err := vcs.New(root).IsDirty()
	require.NoError(t, err)
	return d
}
