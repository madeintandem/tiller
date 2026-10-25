package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const bugBranch = "tiller/bugs/alpha"

// layReviewedBugGit lays a bug driven to in_review on its own branch, with
// main clean: the shelf twin of layFeatureBranchGit + layTaskGrainGit.
func layReviewedBugGit(t *testing.T) string {
	t.Helper()
	return layReviewedBugGitGates(t, "true", "true")
}

// layReviewedBugGitGates is layReviewedBugGit over a chosen pair of gate
// commands, so a caller can put the bug in review with a required gate that
// was never green. A red required gate leaves check at in_progress, since
// check advances only on green, so the transition is driven directly instead:
// advisory enforcement is exactly the case where work reaches review and then
// a merge with a gate that never passed (submitTask does the same one grain up).
func layReviewedBugGitGates(t *testing.T, buildRun, testsRun string) string {
	t.Helper()
	root := layStartedBug(t, buildRun, testsRun)
	gitCommitAllM(t, root, "bug started")
	gitRunM(t, root, "branch", "-M", "main")
	tree := load(t, root)
	res, err := mutate.Check(tree, testDeps(), vcs.New(root), tree.Bugs["B-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	if res.Passed {
		pr := 41
		driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "submit", PR: &pr})
	} else {
		stageShelfReview(t, root, store.NounBug, "alpha", 41)
	}
	gitCommitAllM(t, root, "bug in review")

	gitRunM(t, root, "checkout", "-b", bugBranch)
	require.NoError(t, os.WriteFile(filepath.Join(root, "fix.txt"), []byte("fix\n"), 0o644))
	gitCommitAllM(t, root, "fix the bug")
	gitRunM(t, root, "checkout", "main")
	return root
}

func TestSyncCrossesBugOnItsOwnMergedBranch(t *testing.T) {
	t.Parallel()
	root := layReviewedBugGit(t)
	gitRunM(t, root, "merge", "--no-ff", "-m", "merge fix", bugBranch)
	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 1)
	c := res.Crossings[0]
	require.Equal(t, "bug", c.Noun)
	require.Equal(t, "B-aaaaaa", c.ID)
	require.Equal(t, "done", c.Status)
	require.Equal(t, "ancestry", c.Basis)
	require.Empty(t, c.PolicyViolations,
		"every required gate a shelf node actually runs was green, and the evals gate it never runs is not a finding")
	b := load(t, root).Bugs["B-aaaaaa"]
	require.Equal(t, "done", b.Status)
	require.Equal(t, "ancestry", b.DoneBasis)
	require.Empty(t, b.PolicyViolations)
	require.Equal(t, "sync.bug", lastEvent(t, root, "sync.bug").Type)
}

// TestSyncRedGateOnABugRecordsExactlyThatGate is the shelf twin of
// TestSyncRedGateRecordsPolicyViolations, and it pins the scoping as well as
// the honesty: the crossing records the red tests gate and nothing else. The
// fixture manifest binds evals on [feature, task], the grain a shelf node
// inherits, but a shelf node has no contract and the eval recursion gate is
// deliberately never run on one, so naming it here would manufacture a finding
// out of a gate that was never asked for an answer.
func TestSyncRedGateOnABugRecordsExactlyThatGate(t *testing.T) {
	t.Parallel()
	root := layReviewedBugGitGates(t, "true", "false")
	gitRunM(t, root, "merge", "--no-ff", "-m", "merge fix", bugBranch)
	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 1)
	require.Equal(t, "done", res.Crossings[0].Status, "the human merge is the authority even with a red gate")
	require.Equal(t, []string{"tests"}, res.Crossings[0].PolicyViolations)

	b := load(t, root).Bugs["B-aaaaaa"]
	require.Len(t, b.PolicyViolations, 1)
	require.Equal(t, "tests", b.PolicyViolations[0].Gate)
	require.Equal(t, store.GateStatusFail, b.PolicyViolations[0].Status)
}

func TestSyncListsUnmergedBugAsAwaiting(t *testing.T) {
	t.Parallel()
	root := layReviewedBugGit(t)
	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings)
	require.Len(t, res.Awaiting, 1)
	require.Equal(t, "bug", res.Awaiting[0].Noun)
	require.Equal(t, bugBranch, res.Awaiting[0].Branch)
	require.Equal(t, "in_review", load(t, root).Bugs["B-aaaaaa"].Status)
}

func TestSyncAcceptsFastForwardOnABug(t *testing.T) {
	t.Parallel()
	root := layReviewedBugGit(t)
	gitRunM(t, root, "merge", "--ff-only", bugBranch)
	// Declared after the merge and left uncommitted, the way
	// declareIntegrationBranch does it for the feature-grain assertion tests:
	// store.Load reads the working tree, and a commit here would put main a
	// commit ahead of the bug branch, which is exactly what a fast-forward
	// cannot be.
	setIntegrationBranch(t, root, "main")
	_, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "in_review", load(t, root).Bugs["B-aaaaaa"].Status, "a fast-forward is unobservable")
	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{"B-aaaaaa"}})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 1)
	require.Equal(t, "human_attested", res.Crossings[0].Basis)
	require.Equal(t, []string{"B-aaaaaa"}, res.Attested)
}

func TestSyncCrossesCarriedBugWhenCarrierIsDone(t *testing.T) {
	t.Parallel()
	// A feature branch with one in_review task and a bug carried by that
	// task: one merge crosses the task, and the same run crosses the bug.
	root := layTaskGrainGit(t)
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "B" {
			return "B-cccccc"
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "carried", Title: "Carried"})
	require.NoError(t, err)
	_, err = mutate.Resolve(load(t, root), testDeps(), mutate.ResolveInput{Noun: store.NounBug, Node: "carried", By: "T-aaaaaa"})
	require.NoError(t, err)
	gitCommitAllM(t, root, "carried bug")
	gitRunM(t, root, "merge", "--no-ff", "-m", "merge feature", featBranch)

	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	var bugCrossing *mutate.Crossing
	for i := range res.Crossings {
		if res.Crossings[i].ID == "B-cccccc" {
			bugCrossing = &res.Crossings[i]
		}
	}
	require.NotNil(t, bugCrossing, "the carried bug crosses in the run that crosses its carrier")
	require.Equal(t, "carried", bugCrossing.Basis)
	b := load(t, root).Bugs["B-cccccc"]
	require.Equal(t, "done", b.Status)
	require.Equal(t, "carried", b.DoneBasis)
	require.Equal(t, "T-aaaaaa", b.ResolvedBy)
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	var ev *events.Event
	for i := range evs {
		if evs[i].Type == "sync.bug" {
			ev = &evs[i]
		}
	}
	require.NotNil(t, ev)
	require.Equal(t, "carried", ev.After["basis"])
	require.Equal(t, "T-aaaaaa", ev.After["carrier"])
}

// TestSyncTreatsACancelledCarrierAsAbsent is the other half of the recompute
// (docs/spec/artifact-schema.md, Resolution: "a carrier cancelled afterwards
// returns the bug to the queue"). A carrier that will never merge must not
// park the node forever: the derived phase says carrier_cancelled, and the
// pass stops treating the node as carried, so its own in_review branch is
// observed and listed as awaiting exactly as an unresolved node's would be.
func TestSyncTreatsACancelledCarrierAsAbsent(t *testing.T) {
	t.Parallel()
	root := layReviewedBugGit(t)

	// A carrier for the bug already in review on its own branch, then cancelled.
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/loading", Title: "Loading", ChangeType: "create"})
	require.NoError(t, err)
	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{Key: "form-engine/loading/wire", Title: "Wire it"})
	require.NoError(t, err)
	_, err = mutate.Resolve(load(t, root), testDeps(), mutate.ResolveInput{Noun: store.NounBug, Node: "alpha", By: "T-aaaaaa"})
	require.NoError(t, err)
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "cancel", Reason: "not doing it"})
	gitCommitAllM(t, root, "carrier cancelled")

	tree := load(t, root)
	require.Equal(t, gen.ActivityCarrierCancelled, gen.WorkActivity(tree, tree.Bugs["B-aaaaaa"]))

	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	for _, c := range res.Crossings {
		require.NotEqual(t, "B-aaaaaa", c.ID, "a carrier that will never merge crosses nothing")
	}
	require.Equal(t, "in_review", load(t, root).Bugs["B-aaaaaa"].Status)
	var awaited *mutate.Awaiting
	for i := range res.Awaiting {
		if res.Awaiting[i].ID == "B-aaaaaa" {
			awaited = &res.Awaiting[i]
		}
	}
	require.NotNil(t, awaited, "with no carrier left, the node awaits its own merge again")
	require.Equal(t, bugBranch, awaited.Branch)
	require.Equal(t, "T-aaaaaa", load(t, root).Bugs["B-aaaaaa"].ResolvedBy,
		"the pointer stays on the record; the human clears or re-points it")
}

// TestSyncAcceptsFastForwardOnABugWhoseCarrierWasCancelled is the assertion
// path's share of the same recompute. Once the carrier can no longer carry,
// the node is back on its own branch for every part of the pass, and the
// fast-forward assertion is the one part where refusing it would strand the
// node for good: it is exactly the node the zero-crossing readout has just
// named, with the command it just handed the human.
func TestSyncAcceptsFastForwardOnABugWhoseCarrierWasCancelled(t *testing.T) {
	t.Parallel()
	root := layReviewedBugGit(t)
	// The fast-forward lands first: everything below is left uncommitted, the
	// way TestSyncAcceptsFastForwardOnABug leaves its integration branch, since
	// a commit on main here would put main a commit ahead of the bug branch,
	// which is what a fast-forward cannot be.
	gitRunM(t, root, "merge", "--ff-only", bugBranch)

	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/loading", Title: "Loading", ChangeType: "create"})
	require.NoError(t, err)
	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{Key: "form-engine/loading/wire", Title: "Wire it"})
	require.NoError(t, err)
	_, err = mutate.Resolve(load(t, root), testDeps(), mutate.ResolveInput{Noun: store.NounBug, Node: "alpha", By: "T-aaaaaa"})
	require.NoError(t, err)
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "cancel", Reason: "not doing it"})
	setIntegrationBranch(t, root, "main")

	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{"B-aaaaaa"}})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 1)
	require.Equal(t, "B-aaaaaa", res.Crossings[0].ID)
	require.Equal(t, "human_attested", res.Crossings[0].Basis,
		"the assertion is about the node's own branch, which is what it has again")
	require.Equal(t, []string{"B-aaaaaa"}, res.Attested)
}

func TestSyncLeavesCarriedBugWhenCarrierIsNotDone(t *testing.T) {
	t.Parallel()
	root := layTaskGrainGit(t)
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "B" {
			return "B-cccccc"
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "carried", Title: "Carried"})
	require.NoError(t, err)
	_, err = mutate.Resolve(load(t, root), testDeps(), mutate.ResolveInput{Noun: store.NounBug, Node: "carried", By: "T-aaaaaa"})
	require.NoError(t, err)
	gitCommitAllM(t, root, "carried bug")
	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings)
	require.Equal(t, "proposed", load(t, root).Bugs["B-cccccc"].Status)
	for _, a := range res.Awaiting {
		require.NotEqual(t, "B-cccccc", a.ID, "a carried node is not awaiting its own merge")
	}
}

// stageShelfReview writes a shelf node's review state straight to disk. Submit
// refuses a node whose latest required result is red, so a fixture that needs a
// red node under review (one submitted before a later failed recheck was
// recorded, or a hand edit) stages it here; sync must still honor the human
// merge of such a node and record the gate as a policy violation.
func stageShelfReview(t *testing.T, root, noun, localKey string, pr int) {
	t.Helper()
	p := filepath.Join(root, store.ShelfRelDir(noun, localKey), store.ShelfStateFile(noun))
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc["status"] = "in_review"
	doc["pr"] = pr
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, out, 0o644))
}
