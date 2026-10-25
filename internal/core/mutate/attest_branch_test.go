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
	"gopkg.in/yaml.v3"
)

const attestBranchEval = `---
node: %s
criteria:
  - id: C1
    statement: A reviewer confirms the rendered layout
    polarity: introduce
    binding: { type: manual }
    required: true
---

# Acceptance
`

// layAttestAggregate lays a feature at feature grain whose aggregate branch is
// recorded by its first task start, declares an attestation gate, and leaves
// the working copy on integration with the recorded state merged there. It
// returns the root and the aggregate branch name.
func layAttestAggregate(t *testing.T, grain string) (root string, deps mutate.Deps, aggregate string) {
	t.Helper()
	deps = seqDeps()
	root = rgLayFeature(t, deps, "pay", "checkout", "alfa")
	f, err := os.OpenFile(filepath.Join(root, ".tiller", "tiller.yml"), os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString("  optional:\n    code_review: { kind: attestation, on: pr }\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	if grain == store.ReviewGrainEpic {
		rgSetEpicReviewGrain(t, root, "pay", store.ReviewGrainEpic)
	} else {
		_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
			Noun: store.NounFeature, Key: "pay/checkout", ReviewGrain: grain})
		require.NoError(t, err)
	}
	if grain == store.ReviewGrainTask {
		return root, deps, ""
	}
	rgDriveTaskToInReview(t, root, deps, "pay", "checkout", "alfa", 101)
	tree := load(t, root)
	feature := featureByKey(tree, "pay/checkout")
	aggregate = feature.Branch
	if grain == store.ReviewGrainEpic {
		aggregate = epicByKey(tree, "pay").Branch
	}
	require.NotEmpty(t, aggregate)
	gitRunM(t, root, "checkout", "main")
	gitRunM(t, root, "merge", "--ff-only", aggregate)
	return root, deps, aggregate
}

func attestGate(t *testing.T, root string, deps mutate.Deps) (mutate.AttestResult, error) {
	t.Helper()
	tree := load(t, root)
	return mutate.Attest(tree, deps, resolveNode(t, tree, store.NounFeature, "pay/checkout"), mutate.AttestInput{Gate: "code_review", Status: store.GateStatusPass})
}

func featureYML(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout", "feature.yml"))
	require.NoError(t, err)
	return string(data)
}

func requireAttestMismatch(t *testing.T, err error, aggregate, current string) *mutate.BranchSafetyError {
	t.Helper()
	var refusal *mutate.BranchSafetyError
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, "attest_branch_mismatch", refusal.Reason)
	require.Equal(t, aggregate, refusal.Branch)
	require.Equal(t, current, refusal.CurrentBranch)
	return refusal
}

func TestAttestFeatureRefusedOffAggregateBranch(t *testing.T) {
	t.Parallel()
	root, deps, aggregate := layAttestAggregate(t, store.ReviewGrainFeature)
	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "pay/checkout")
	require.NoError(t, os.WriteFile(filepath.Join(attestFeatureDir(root), "feature.eval.md"),
		[]byte(strings.Replace(attestBranchEval, "%s", feat.NodeID(), 1)), 0o644))
	tree = load(t, root)
	before := featureYML(t, root)

	_, err := mutate.Attest(tree, deps, resolveNode(t, tree, store.NounFeature, "pay/checkout"), mutate.AttestInput{Criterion: "C1", Status: store.GateStatusPass})

	refusal := requireAttestMismatch(t, err, aggregate, "main")
	require.Equal(t, feat.NodeID(), refusal.NodeID)
	require.Contains(t, err.Error(), "check out")
	require.Equal(t, before, featureYML(t, root))
}

func attestFeatureDir(root string) string {
	return filepath.Join(root, ".tiller", "epics", "pay", "features", "checkout")
}

func TestAttestFeatureGateRefusedOffAggregateBranch(t *testing.T) {
	t.Parallel()
	root, deps, aggregate := layAttestAggregate(t, store.ReviewGrainFeature)
	before := featureYML(t, root)

	_, err := attestGate(t, root, deps)

	requireAttestMismatch(t, err, aggregate, "main")
	require.Equal(t, before, featureYML(t, root))
}

func TestAttestFeaturePassesOnAggregateBranch(t *testing.T) {
	t.Parallel()
	root, deps, aggregate := layAttestAggregate(t, store.ReviewGrainFeature)
	gitRunM(t, root, "checkout", aggregate)

	res, err := attestGate(t, root, deps)

	require.NoError(t, err)
	require.True(t, res.Changed)
}

func TestAttestFeaturePassesInLinkedWorktreeOnAggregate(t *testing.T) {
	t.Parallel()
	root, deps, aggregate := layAttestAggregate(t, store.ReviewGrainFeature)
	wt := filepath.Join(t.TempDir(), "agg")
	gitRunM(t, root, "worktree", "add", wt, aggregate)

	res, err := attestGate(t, wt, deps)

	require.NoError(t, err)
	require.True(t, res.Changed)
}

func TestAttestFeatureRefusalNamesWorktree(t *testing.T) {
	t.Parallel()
	root, deps, aggregate := layAttestAggregate(t, store.ReviewGrainFeature)
	wt := filepath.Join(t.TempDir(), "agg")
	gitRunM(t, root, "worktree", "add", wt, aggregate)

	_, err := attestGate(t, root, deps)

	refusal := requireAttestMismatch(t, err, aggregate, "main")
	want, evalErr := filepath.EvalSymlinks(wt)
	require.NoError(t, evalErr)
	got, evalErr := filepath.EvalSymlinks(refusal.Worktree)
	require.NoError(t, evalErr)
	require.Equal(t, want, got)
	require.Contains(t, err.Error(), "run this attest in the worktree at "+refusal.Worktree)
}

func TestAttestFeatureRefusalWhenAggregateRefMissing(t *testing.T) {
	t.Parallel()
	root, deps, aggregate := layAttestAggregate(t, store.ReviewGrainFeature)
	gitRunM(t, root, "branch", "-D", aggregate)

	_, err := attestGate(t, root, deps)

	refusal := requireAttestMismatch(t, err, aggregate, "main")
	require.True(t, refusal.RefMissing)
	require.Contains(t, err.Error(), "git fetch && git switch")
}

func TestAttestFeatureRefusedOnDetachedHead(t *testing.T) {
	t.Parallel()
	root, deps, aggregate := layAttestAggregate(t, store.ReviewGrainFeature)
	gitRunM(t, root, "checkout", "--detach", aggregate)

	_, err := attestGate(t, root, deps)

	requireAttestMismatch(t, err, aggregate, "HEAD")
}

func TestAttestFeatureAllowedAtTaskGrain(t *testing.T) {
	t.Parallel()
	root, deps, _ := layAttestAggregate(t, store.ReviewGrainTask)

	res, err := attestGate(t, root, deps)

	require.NoError(t, err)
	require.True(t, res.Changed)
}

func rewriteFeature(t *testing.T, root string, edit func(*store.Feature)) {
	t.Helper()
	path := filepath.Join(attestFeatureDir(root), "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	edit(&doc)
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

func TestAttestFeatureAllowedForDoneFeature(t *testing.T) {
	t.Parallel()
	root, deps, _ := layAttestAggregate(t, store.ReviewGrainFeature)
	rewriteFeature(t, root, func(f *store.Feature) { f.Status = "done" })

	res, err := attestGate(t, root, deps)

	require.NoError(t, err)
	require.True(t, res.Changed)
}

func TestAttestFeatureAllowedForRecoveredReview(t *testing.T) {
	t.Parallel()
	root, deps, aggregate := layAttestAggregate(t, store.ReviewGrainFeature)
	tree := load(t, root)
	// Staged in memory: a valid on-disk record needs a full historical
	// evidence set, and the guard reads only whether the record is present.
	featureByKey(tree, "pay/checkout").RecoveredReview = &store.RecoveredReview{Branch: aggregate}

	res, err := mutate.Attest(tree, deps, resolveNode(t, tree, store.NounFeature, "pay/checkout"), mutate.AttestInput{Gate: "code_review", Status: store.GateStatusPass})

	require.NoError(t, err)
	require.True(t, res.Changed)
}

func TestAttestFeatureRefusedAtEpicGrain(t *testing.T) {
	t.Parallel()
	root, deps, aggregate := layAttestAggregate(t, store.ReviewGrainEpic)
	require.Equal(t, "main", currentBranchOf(t, root))

	_, err := attestGate(t, root, deps)

	requireAttestMismatch(t, err, aggregate, "main")
}

func TestAttestTaskUnguarded(t *testing.T) {
	t.Parallel()
	root, deps, _ := layAttestAggregate(t, store.ReviewGrainFeature)
	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, "pay/checkout/alfa")

	res, err := mutate.Attest(tree, deps, task, mutate.AttestInput{Gate: "code_review", Status: store.GateStatusPass})

	require.NoError(t, err)
	require.True(t, res.Changed)
}

func currentBranchOf(t *testing.T, root string) string {
	t.Helper()
	b, err := vcs.New(root).Branch()
	require.NoError(t, err)
	return b
}
