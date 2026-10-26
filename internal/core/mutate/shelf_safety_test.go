package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func readyShelfSafety(t *testing.T) string {
	t.Helper()
	root := layBug(t, "guarded")
	driveTo(t, root, store.NounBug, "guarded", mutate.TransitionInput{Verb: "approve"})
	setWorkOwner(t, root, store.NounBug, "guarded", "jc")
	return root
}

func TestShelfStartRejectsUnsafeBranchBeforeFreezingOwner(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ pattern, existing string }{
		{"work", ""}, {"work/../${node.key}", ""},
		{"tiller/${node.key}", "tiller/bugs"},
		{"tiller/${node.key}", "tiller/bugs/guarded/old"},
		{"tiller/${node.key}", "tiller/bugs/guarded"},
	} {
		t.Run(tc.pattern+tc.existing, func(t *testing.T) {
			t.Parallel()
			root := readyShelfSafety(t)
			setTaskBranchPattern(t, root, tc.pattern)
			if tc.existing != "" {
				gitRunM(t, root, "branch", tc.existing)
			}
			before := snapshotDir(t, filepath.Join(root, ".tiller"))
			err := tryTransition(t, root, testDeps(), store.NounBug, "guarded", mutate.TransitionInput{Verb: "start"})
			var blocked *mutate.BranchSafetyError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
			require.False(t, load(t, root).Bugs["B-aaaaaa"].Started)
		})
	}
}

func TestShelfSubmitRejectsWrongBranchAndReviewHeadWithoutWrites(t *testing.T) {
	t.Parallel()
	root := readyShelfSafety(t)
	driveTo(t, root, store.NounBug, "guarded", mutate.TransitionInput{Verb: "start"})
	tree := load(t, root)
	bug := tree.Bugs["B-aaaaaa"]
	bug.Status = "check"
	data, err := yaml.Marshal(bug)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/bugs/guarded/bug.yml"), data, 0o644))
	before := snapshotDir(t, filepath.Join(root, ".tiller"))
	err = tryTransition(t, root, testDeps(), store.NounBug, "guarded", mutate.TransitionInput{Verb: "submit"})
	var blocked *mutate.BranchSafetyError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "branch_mismatch", blocked.Reason)
	require.Equal(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
	gitRunM(t, root, "checkout", "-b", bug.Branch)
	err = tryTransition(t, root, testDeps(), store.NounBug, "guarded", mutate.TransitionInput{Verb: "submit", ReviewRef: &store.ReviewRef{Branch: bug.Branch, Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:wrong"}}})
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "branch_review_head_mismatch", blocked.Reason)
	require.Equal(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
	head, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	res := driveTo(t, root, store.NounBug, "guarded", mutate.TransitionInput{Verb: "submit"})
	require.Equal(t, bug.Branch, res.ReviewRef.Branch)
	require.Equal(t, head.Value, res.ReviewRef.Head.Value)
}

func TestShelfStartRefusesLiveCarrierButPreservesRetiredFallback(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"draft", "done", "cancelled", "archived", "missing"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			root := readyShelfSafety(t)
			tree := load(t, root)
			bug := tree.Bugs["B-aaaaaa"]
			bug.ResolvedBy = "E-zzzzzz"
			data, err := yaml.Marshal(bug)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/bugs/guarded/bug.yml"), data, 0o644))
			if status != "missing" {
				path := filepath.Join(root, ".tiller/epics/carrier")
				require.NoError(t, os.MkdirAll(path, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(path, "epic.yml"), []byte("id: E-zzzzzz\nkey: carrier\nstatus: "+status+"\n"), 0o644))
			}
			before := snapshotDir(t, filepath.Join(root, ".tiller"))
			err = tryTransition(t, root, testDeps(), store.NounBug, "guarded", mutate.TransitionInput{Verb: "start"})
			if status == "draft" || status == "done" {
				var blocked *mutate.WorkCarriedError
				require.ErrorAs(t, err, &blocked)
				require.Contains(t, err.Error(), "resolve bug bugs/guarded --clear")
				require.Equal(t, before, snapshotDir(t, filepath.Join(root, ".tiller")))
			} else {
				require.NoError(t, err)
				require.True(t, load(t, root).Bugs["B-aaaaaa"].Started)
			}
		})
	}
}
