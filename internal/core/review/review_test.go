package review_test

// These tests pin ForTask directly at its new home (moved out of
// internal/cli/unpushed_review.go per the review-fix wave), the composition
// every one of the four review-state surfaces (submit's confirmation,
// status's oracle, the execute readout, and the node page) now shares. The
// CLI's own end-to-end tests (lifecycle_cmds_test.go's
// TestSubmitConfirmationWarnsWhenTheStateCommitIsUnpushed and its paired
// silent case, readout_test.go's TestReadoutGathersUnpushedReviewsOnlyForExecute)
// and the serve package's node-page tests already cover this behavior
// through those callers; these tests pin ForTask's own contract in
// isolation, an in-memory tree against a real git repo rather than a full
// CLI walk.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/review"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// reviewTree builds a minimal in-memory tree, one task on one feature, so a
// test can flip the task's status, PR, and branch with no
// disk I/O: ForTask reads a *store.Tree directly, never a loaded repo.
func reviewTree(status string, pr *int, branch string) *store.Tree {
	return &store.Tree{
		Features: map[string]*store.Feature{
			"F-000001": {ID: "F-000001", Branch: "tiller/reviews/features/form-engine/schema-engine"},
		},
		Tasks: map[string]*store.Task{
			"T-000001": {ID: "T-000001", FeatureID: "F-000001", Status: status, Owner: "jc", Started: true, PR: pr, Branch: branch},
		},
	}
}

func prNum(n int) *int { return &n }

// gitRepoWithBranch lays a real git repo with an initial commit, a bare
// remote, and branch checked out from it (uncommitted to it, unpushed), so
// a caller can push it, or not, and add a further local commit, or not, to
// drive every arm of ForTask's own git reads.
func gitRepoWithBranch(t *testing.T, branch string) string {
	t.Helper()
	root := t.TempDir()
	testgit.Init(t, root)
	require.NoError(t, os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644))
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "initial")

	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")
	gitRun(t, root, "remote", "add", "origin", bare)

	gitRun(t, root, "checkout", "-b", branch)
	return root
}

// TestForTaskOnAnUnpushedStateCommit pins the positive case: an in-review
// task with a recorded PR, whose own branch is pushed but then holds a
// further local commit its remote-tracking counterpart does not, answers
// Facts naming that exact commit, branch, and upstream.
func TestForTaskOnAnUnpushedStateCommit(t *testing.T) {
	branch := "tiller/form-engine/schema-engine"
	root := gitRepoWithBranch(t, branch)
	gitRun(t, root, "push", "-u", "origin", branch)

	require.NoError(t, os.WriteFile(filepath.Join(root, "state.txt"), []byte("state\n"), 0o644))
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "state commit")
	head := gitOut(t, root, "rev-parse", "--short", "HEAD")

	tree := reviewTree("in_review", prNum(41), branch)
	facts := review.ForTask(tree, "T-000001", vcs.New(root))

	require.NotNil(t, facts)
	require.Equal(t, head, facts.Commit)
	require.Equal(t, branch, facts.Branch)
	require.Equal(t, "origin/"+branch, facts.Upstream)
}

// TestForTaskNilWhenPushed pins the all-clear pair: the identical setup with
// nothing advancing past the push, so the local tip is already on the
// remote and there is nothing to report.
func TestForTaskNilWhenPushed(t *testing.T) {
	branch := "tiller/form-engine/schema-engine"
	root := gitRepoWithBranch(t, branch)
	gitRun(t, root, "push", "-u", "origin", branch)

	tree := reviewTree("in_review", prNum(41), branch)
	require.Nil(t, review.ForTask(tree, "T-000001", vcs.New(root)))
}

// TestForTaskNilWithNoRecordedBranch pins the third omission condition
// (alongside no upstream and already-pushed): a task that has not yet
// recorded a branch at all (empty Branch) has nothing for UnpushedTip to
// even ask about, and ForTask returns nil before touching git.
func TestForTaskNilWithNoRecordedBranch(t *testing.T) {
	root := t.TempDir() // no git needed: ForTask never reaches a git read
	tree := reviewTree("in_review", prNum(41), "")
	require.Nil(t, review.ForTask(tree, "T-000001", vcs.New(root)))
}

// TestForTaskNilWithoutARecordedPR pins the offline-path constraint: a
// remote-less submit's branch-ref fallback leaves PR nil, and ForTask
// returns nil before ever touching git, the mechanical form of "the offline
// path never warns."
func TestForTaskNilWithoutARecordedPR(t *testing.T) {
	branch := "tiller/form-engine/schema-engine"
	root := gitRepoWithBranch(t, branch)
	gitRun(t, root, "push", "-u", "origin", branch)
	require.NoError(t, os.WriteFile(filepath.Join(root, "state.txt"), []byte("state\n"), 0o644))
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "state commit")

	tree := reviewTree("in_review", nil, branch)
	require.Nil(t, review.ForTask(tree, "T-000001", vcs.New(root)))
}

// TestForTaskNilForANonTaskID pins the narrowing every caller (the node
// page in particular) relies on instead of type-asserting first: an id that
// names no task in tree.Tasks is a harmless no-op, not a panic or a
// mismatched read.
func TestForTaskNilForANonTaskID(t *testing.T) {
	branch := "tiller/form-engine/schema-engine"
	root := gitRepoWithBranch(t, branch)

	tree := reviewTree("in_review", prNum(41), branch)
	require.Nil(t, review.ForTask(tree, "E-000001", vcs.New(root)))
}

// TestForTaskNilOnAVcsError pins the degrade-silently posture every caller
// of this report, not a gate, relies on: a root with no git repository at
// all cannot answer UnpushedTip no matter what it asks, and ForTask returns
// nil rather than propagating the error.
func TestForTaskNilOnAVcsError(t *testing.T) {
	root := t.TempDir() // no git init at all
	tree := reviewTree("in_review", prNum(41), "tiller/form-engine/schema-engine")
	require.Nil(t, review.ForTask(tree, "T-000001", vcs.New(root)))
}

// TestForTaskResolvesTheBranchNotAShadowingTag plants the f351377 hazard
// ForTask's own doc comment names as "the one hazard a second,
// uncentralized copy of this composition would silently risk losing": a tag
// sharing the task branch's exact name. git's rev-parse resolves a bare
// name from refs/tags before refs/heads, so an unqualified ShortSHA(tk.Branch)
// would name the TAG's commit in the warning instead of the branch's own;
// ForTask's real call already goes through vcs.BranchRef to prevent exactly
// that. The tag here points at the repo's initial commit, deliberately
// different from the branch's own unpushed tip, so a regression that dropped
// the qualification would make this test observe the wrong (and wrong-count)
// commit rather than merely fail to notice a coincidence.
func TestForTaskResolvesTheBranchNotAShadowingTag(t *testing.T) {
	branch := "tiller/form-engine/schema-engine"
	root := gitRepoWithBranch(t, branch)
	initial := gitOut(t, root, "rev-parse", "--short", "HEAD")
	gitRun(t, root, "push", "-u", "origin", branch)

	// A tag named identically to the branch, planted at the initial commit -
	// a real git repo lets a tag and a branch share a name without conflict,
	// which is exactly what makes the ambiguity possible.
	gitRun(t, root, "tag", branch, "HEAD")

	require.NoError(t, os.WriteFile(filepath.Join(root, "state.txt"), []byte("state\n"), 0o644))
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "state commit")
	head := gitOut(t, root, "rev-parse", "--short", "HEAD")
	require.NotEqual(t, initial, head, "the tag and the branch's real tip must name different commits")

	tree := reviewTree("in_review", prNum(41), branch)
	facts := review.ForTask(tree, "T-000001", vcs.New(root))

	require.NotNil(t, facts)
	require.Equal(t, head, facts.Commit, "the branch's own unpushed tip, not the shadowing tag's commit")
	require.NotEqual(t, initial, facts.Commit)
}
