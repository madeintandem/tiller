package mutate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func TestWorkflowObservesHistoricalReviewWithoutItsLiveAggregateBranch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		return strings.TrimSpace(string(out))
	}
	git("init", "-b", "main")
	git("config", "user.name", "Recovery test")
	git("config", "user.email", "recovery@example.test")
	require.NoError(t, os.WriteFile(filepath.Join(root, "work.txt"), []byte("base\n"), 0o644))
	git("add", ".")
	git("commit", "-m", "base")
	git("switch", "-c", "historical-review")
	require.NoError(t, os.WriteFile(filepath.Join(root, "work.txt"), []byte("reviewed\n"), 0o644))
	git("commit", "-am", "reviewed work")
	head := git("rev-parse", "HEAD")
	git("switch", "main")
	git("merge", "--no-ff", "historical-review", "-m", "integrate review")
	result := git("rev-parse", "HEAD")
	git("branch", "-D", "historical-review")
	epicKey, featureKey := "work", "work/review"
	feature := &store.Feature{ID: "F-review", Key: &featureKey, EpicKey: epicKey, Status: "active", ReviewGrain: store.ReviewGrainFeature, Branch: "historical-review",
		RecoveredReview: &store.RecoveredReview{Branch: "historical-review", Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + head}, MergeCommit: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + result}, IntegrationBranch: "main", Basis: "ancestry", RecoveredBy: "jc", Reason: "Authorized historical review"}}
	task := &store.Task{ID: "T-review", FeatureID: feature.ID, Status: "in_review", Branch: "deleted-task", ReviewRef: &store.ReviewRef{Branch: "deleted-task", Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + head}}}
	tree := &store.Tree{Root: root, Manifest: &store.Manifest{Git: store.GitBlock{IntegrationBranch: "main"}},
		Epics:    map[string]*store.Epic{"E-work": {ID: "E-work", Key: &epicKey, Status: "ready"}},
		Features: map[string]*store.Feature{feature.ID: feature}, Tasks: map[string]*store.Task{task.ID: task}, FeatureTasks: map[string][]*store.Task{feature.ID: {task}}}
	feature.RecoveredReview.Tasks = []store.RecoveredTask{{ID: task.ID, Path: "task.yml"}}
	in := ObserveWorkflow(tree)
	require.Empty(t, in.Assembly[feature.ID])
	require.Empty(t, in.AssemblyErrors[feature.ID])
	require.Empty(t, in.RecoveryErrors[feature.ID])
	advice := workflow.AdviceOf(tree, feature, in)
	require.NotNil(t, advice)
	require.Equal(t, "sync", advice.Action)
	require.False(t, advice.AcceptanceReady, "an unread current contract must remain independent")
	git("reset", "--hard", "HEAD^1")
	in = ObserveWorkflow(tree)
	require.NotEmpty(t, in.RecoveryErrors[feature.ID], "a result removed from integration is no longer an observed witness")
	require.Equal(t, "inspect_recovery", workflow.AdviceOf(tree, feature, in).Action)
	require.Empty(t, in.AssemblyErrors[feature.ID], "invalid recovery must never fall back to the deleted live aggregate")
}

func TestObserveWorkflowForNodesReadsOnlyTheNamedReviews(t *testing.T) {
	t.Parallel()
	epicKey, keyA, keyB := "work", "work/a", "work/b"
	a := &store.Feature{ID: "F-a", Key: &keyA, EpicKey: epicKey, Status: "done", ReviewGrain: store.ReviewGrainFeature}
	b := &store.Feature{ID: "F-b", Key: &keyB, EpicKey: epicKey, Status: "done", ReviewGrain: store.ReviewGrainFeature}
	tree := &store.Tree{Root: t.TempDir(), Manifest: &store.Manifest{},
		Epics:    map[string]*store.Epic{"E-work": {ID: "E-work", Key: &epicKey, Status: "ready"}},
		Features: map[string]*store.Feature{a.ID: a, b.ID: b}, Tasks: map[string]*store.Task{}, FeatureTasks: map[string][]*store.Task{}}

	in := ObserveWorkflowForNodes(tree, []store.Node{a})
	require.Contains(t, in.ContractErrors, a.ID, "the named review's contract is read")
	require.NotContains(t, in.ContractErrors, b.ID, "an unrelated feature is never read")

	require.Empty(t, ObserveWorkflowForNodes(tree, nil).ContractErrors, "no owners, no observation")
	require.Equal(t, ObserveWorkflowForNode(tree, a).ContractErrors, in.ContractErrors)
}
