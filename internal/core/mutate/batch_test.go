package mutate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const batchManifest = `schema_version: 16
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
  - { id: pat, name: Pat Nguyen }
git:
  integration_branch: main
gates:
  required:
    build: { kind: command, run: "true", on: [feature, task] }
    tests: { kind: command, run: "true", on: [feature, task] }
    evals: { kind: command, run: "tiller eval ${node.noun} ${node.id}", on: [feature, task] }
`

// The shelf nodes the batch fixture lays, by id. Alpha, beta, and delta are
// approved; gamma is still proposed.
const (
	batchBugAlpha   = "B-000001"
	batchBugBeta    = "B-000002"
	batchChoreGamma = "CH-000001"
	batchTweakDelta = "TW-000001"
	batchID         = "BT-000001"
	batchBranch     = "tiller/batches/core"
)

var batchRepo = testgit.Template{Name: "batch repo"}

// batchDeps mints sequential ids per class, so the fixture's ids are the
// constants above and a batch added afterwards is BT-000001.
func batchDeps() mutate.Deps {
	deps := testDeps()
	seqs := map[string]int{}
	deps.Mint = func(prefix string) string {
		if prefix == "EV" {
			return fixedMint(prefix)
		}
		seqs[prefix]++
		return fmt.Sprintf("%s-%06d", prefix, seqs[prefix])
	}
	return deps
}

// layBatchRepo lays two approved bugs, a proposed chore, and an approved
// tweak, each with a written contract, committed on main.
func layBatchRepo(t *testing.T) string {
	t.Helper()
	return batchRepo.Copy(t, func(t testing.TB, dir string) {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "tiller.yml"), []byte(batchManifest), 0o644))
		gitInitM(t, dir)
		gitCommitAllM(t, dir, "init")
		deps := batchDeps()
		for _, item := range []struct{ noun, key, severity string }{
			{store.NounBug, "alpha", "minor"}, {store.NounBug, "beta", "important"},
			{store.NounChore, "gamma", ""}, {store.NounTweak, "delta", ""},
		} {
			_, err := mutate.AddWorkItem(load(t, dir), deps, mutate.WorkItemInput{Noun: item.noun, Key: item.key, Title: "Fix " + item.key, Severity: item.severity})
			require.NoError(t, err)
			contract := filepath.Join(dir, store.ShelfRelDir(item.noun, item.key), store.ShelfProseFile(item.noun))
			require.NoError(t, os.WriteFile(contract, []byte("# Fix "+item.key+"\n\n## What\n\nIt is wrong.\n\n## Done when\n\nIt is right.\n"), 0o644))
		}
		gitCommitAllM(t, dir, "capture shelf work")
		for _, item := range []struct{ noun, key string }{{store.NounBug, "alpha"}, {store.NounBug, "beta"}, {store.NounTweak, "delta"}} {
			driveTo(t, dir, item.noun, item.key, mutate.TransitionInput{Verb: "approve"})
		}
		gitCommitAllM(t, dir, "approve shelf work")
	})
}

func addBatch(t *testing.T, root string, members ...string) mutate.BatchResult {
	t.Helper()
	res, err := mutate.AddBatch(load(t, root), batchDeps(), mutate.BatchInput{Key: "core", Title: "Core fixes", Members: members})
	require.NoError(t, err)
	return res
}

// layStartedBatch lays a batch over the three approved nodes, claimed and
// started by jc, with its branch checked out.
func layStartedBatch(t *testing.T) string {
	t.Helper()
	root := layBatchRepo(t)
	addBatch(t, root, batchBugAlpha, "bugs/beta", batchTweakDelta)
	_, err := mutate.ClaimWork(load(t, root), testDeps(), store.NounBatch, "core", "")
	require.NoError(t, err)
	started := driveTo(t, root, store.NounBatch, "core", mutate.TransitionInput{Verb: "start"})
	gitCommitAllM(t, root, "start batch")
	gitRunM(t, root, "checkout", "-b", started.Branch)
	return root
}

// layReviewedBatch drives the started batch through check and submit, then
// lands one fix per member on the batch branch and returns to main with that
// branch unmerged. Like layReviewedBugGit, main holds the submitted ledger
// snapshot and not the implementation.
func layReviewedBatch(t *testing.T) string {
	t.Helper()
	root := layStartedBatch(t)
	checkBatch(t, root)
	gitCommitAllM(t, root, "batch gate evidence")
	pr := 77
	driveTo(t, root, store.NounBatch, "core", mutate.TransitionInput{Verb: "submit", PR: &pr})
	gitCommitAllM(t, root, "batch in review")
	gitRunM(t, root, "branch", "-f", "main")
	for _, id := range []string{batchBugAlpha, batchBugBeta, batchTweakDelta} {
		landFix(t, root, id+".txt", id+" fix")
	}
	gitRunM(t, root, "checkout", "main")
	return root
}

func checkBatch(t *testing.T, root string) mutate.CheckResult {
	t.Helper()
	tree := load(t, root)
	res, err := mutate.Check(tree, testDeps(), vcs.New(root), tree.Batches[batchID], mutate.CheckInput{})
	require.NoError(t, err)
	return res
}

func requireBatchError(t *testing.T, err error, reason string) *mutate.BatchError {
	t.Helper()
	var refusal *mutate.BatchError
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, reason, refusal.Reason, refusal.Error())
	return refusal
}

func memberActivity(t *testing.T, root, id string) string {
	t.Helper()
	tree := load(t, root)
	w, ok := tree.WorkNode(id)
	require.True(t, ok)
	return gen.WorkActivity(tree, w)
}

func TestAddBatchCreatesAReadyBatchOverMixedShelves(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	res := addBatch(t, root, batchBugAlpha, "chores/gamma", batchTweakDelta, batchBugAlpha)
	require.True(t, res.Created)
	require.Equal(t, batchID, res.ID)
	require.Equal(t, "batches/core", res.Key)
	require.Equal(t, "ready", res.Status)
	require.Len(t, res.Members, 3, "a repeated member is one member")

	tree := load(t, root)
	batch := tree.Batches[batchID]
	require.Equal(t, "Core fixes", batch.Title)
	require.Empty(t, batch.Owner, "a batch is claimed with claim batch, not by creating it")
	require.FileExists(t, filepath.Join(root, ".tiller", "batches", "core", "batch.yml"))
	require.NoFileExists(t, filepath.Join(root, ".tiller", "batches", "core", "batch.md"))
	require.Equal(t, "proposed", tree.Chores[batchChoreGamma].Status, "a member keeps its stored status")
	require.Equal(t, batchID, tree.Chores[batchChoreGamma].BatchID)
	require.Equal(t, gen.ActivityBatched, memberActivity(t, root, batchBugAlpha))
	require.Empty(t, memberActivity(t, root, batchBugBeta), "a node outside the batch is untouched")
	require.Equal(t, "minor", gen.BatchSeverity(tree, batch))

	ev := lastEvent(t, root, "add.batch")
	require.Equal(t, []string{batchID}, ev.Targets, "the batch's event targets the batch alone")
	require.Equal(t, []any{batchBugAlpha, batchChoreGamma, batchTweakDelta}, ev.After["members"])
	joined := lastEvent(t, root, "batch.chore")
	require.Equal(t, []string{batchChoreGamma}, joined.Targets)
	require.Equal(t, batchID, joined.After["batch"])
	require.NotContains(t, joined.After, "status", "joining a batch moves no status on the member")

	again, err := mutate.AddBatch(load(t, root), batchDeps(), mutate.BatchInput{Key: "core", Title: "Core fixes", Members: []string{batchBugBeta}})
	require.NoError(t, err)
	require.False(t, again.Created, "re-adding a key is a no-op")
	require.Empty(t, load(t, root).Bugs[batchBugBeta].BatchID)
}

func TestAddBatchRefusesMembersThatCannotJoin(t *testing.T) {
	t.Parallel()
	tryAdd := func(root string, members ...string) error {
		_, err := mutate.AddBatch(load(t, root), batchDeps(), mutate.BatchInput{Key: "other", Title: "Other", Members: members})
		return err
	}
	t.Run("fewer than two", func(t *testing.T) {
		err := tryAdd(layBatchRepo(t), batchBugAlpha)
		require.ErrorContains(t, err, "two or more")
	})
	t.Run("not shelf work", func(t *testing.T) {
		err := tryAdd(layBatchRepo(t), batchBugAlpha, "some-epic")
		require.ErrorContains(t, err, "does not name a bug, chore, or tweak")
	})
	t.Run("cancelled", func(t *testing.T) {
		root := layBatchRepo(t)
		driveTo(t, root, store.NounBug, "beta", mutate.TransitionInput{Verb: "cancel", Reason: "no", Resolution: "wontfix"})
		requireBatchError(t, tryAdd(root, batchBugAlpha, batchBugBeta), mutate.ReasonBatchMemberTerminal)
	})
	t.Run("started on its own", func(t *testing.T) {
		root := layBatchRepo(t)
		_, err := mutate.ClaimWork(load(t, root), testDeps(), store.NounBug, "beta", "")
		require.NoError(t, err)
		driveTo(t, root, store.NounBug, "beta", mutate.TransitionInput{Verb: "start"})
		refusal := requireBatchError(t, tryAdd(root, batchBugAlpha, batchBugBeta), mutate.ReasonBatchMemberStarted)
		require.Equal(t, batchBugBeta, refusal.Member)
	})
	t.Run("carried", func(t *testing.T) {
		root := layBatchRepo(t)
		_, err := mutate.AddEpic(load(t, root), batchDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
		require.NoError(t, err)
		_, err = mutate.Resolve(load(t, root), testDeps(), mutate.ResolveInput{Noun: store.NounBug, Node: "beta", By: "form-engine"})
		require.NoError(t, err)
		requireBatchError(t, tryAdd(root, batchBugAlpha, batchBugBeta), mutate.ReasonBatchMemberCarried)
	})
	t.Run("holding a recovered review", func(t *testing.T) {
		root := layBatchRepo(t)
		path := filepath.Join(root, ".tiller", "bugs", "beta", "bug.yml")
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		recovered := "pr: 13\nrecovered_review:\n" +
			"  head: {kind: git_commit, value: \"git:1111111111111111111111111111111111111111\"}\n" +
			"  merge_commit: {kind: git_commit, value: \"git:2222222222222222222222222222222222222222\"}\n" +
			"  integration_branch: main\n  basis: ancestry\n  recovered_by: jc\n  recovered_at: 2026-11-19T12:00:00Z\n  reason: merged before submit\n"
		require.NoError(t, os.WriteFile(path, append(data, []byte(recovered)...), 0o644))
		requireBatchError(t, tryAdd(root, batchBugAlpha, batchBugBeta), mutate.ReasonBatchMemberRecovered)
	})
	t.Run("in another live batch", func(t *testing.T) {
		root := layBatchRepo(t)
		addBatch(t, root, batchBugAlpha, batchBugBeta)
		refusal := requireBatchError(t, tryAdd(root, batchBugBeta, batchTweakDelta), mutate.ReasonBatchMemberInBatch)
		require.Contains(t, refusal.Error(), batchID)
	})
	t.Run("claimed by another contributor", func(t *testing.T) {
		root := layBatchRepo(t)
		_, err := mutate.ClaimWork(load(t, root), depsAs(testDeps(), "pat"), store.NounBug, "beta", "")
		require.NoError(t, err)
		requireBatchError(t, tryAdd(root, batchBugAlpha, batchBugBeta), mutate.ReasonBatchMemberClaimed)
	})
}

func TestStartBatchRefusesUntilEveryMemberIsApproved(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	addBatch(t, root, batchBugAlpha, batchChoreGamma)
	start := func() (mutate.TransitionResult, error) {
		tree := load(t, root)
		return mutate.Transition(tree, testDeps(), tree.Batches[batchID], mutate.TransitionInput{Verb: "start"})
	}
	_, err := start()
	var unclaimed *mutate.WorkUnclaimedError
	require.ErrorAs(t, err, &unclaimed, "a batch needs its own claim")
	require.Equal(t, "tiller claim batch batches/core", unclaimed.Seam)

	_, err = mutate.ClaimWork(load(t, root), testDeps(), store.NounBatch, "core", "")
	require.NoError(t, err)
	_, err = start()
	refusal := requireBatchError(t, err, mutate.ReasonBatchMemberUnready)
	require.Equal(t, batchChoreGamma, refusal.Member)
	require.Equal(t, "tiller approve chore chores/gamma", refusal.Seam)
	require.Equal(t, "ready", load(t, root).Batches[batchID].Status)

	driveTo(t, root, store.NounChore, "gamma", mutate.TransitionInput{Verb: "approve"})
	res, err := start()
	require.NoError(t, err)
	require.Equal(t, "in_progress", res.Status)
	require.Equal(t, batchBranch, res.Branch)
	tree := load(t, root)
	require.True(t, tree.Batches[batchID].Started)
	require.Equal(t, batchBranch, tree.Batches[batchID].Branch)
	require.Empty(t, tree.Bugs[batchBugAlpha].Branch, "the branch is recorded on the batch, never on a member")
	require.False(t, tree.Bugs[batchBugAlpha].Started)
	require.Equal(t, "start.batch", lastEvent(t, root, "start.batch").Type)
}

func TestBatchMemberIsNotAnExecuteTargetOnItsOwn(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	addBatch(t, root, batchBugAlpha, batchBugBeta)

	_, err := mutate.ClaimWork(load(t, root), testDeps(), store.NounBug, "alpha", "")
	refusal := requireBatchError(t, err, mutate.ReasonWorkBatched)
	require.Equal(t, batchID, refusal.Batch)
	require.Equal(t, "tiller claim batch core", refusal.Seam)

	_, err = mutate.ReassignWork(load(t, root), testDeps(), store.NounBug, "alpha", "pat", "handoff")
	requireBatchError(t, err, mutate.ReasonWorkBatched)

	for _, verb := range []string{"start", "submit", "resume"} {
		tree := load(t, root)
		_, err := mutate.Transition(tree, testDeps(), tree.Bugs[batchBugAlpha], mutate.TransitionInput{Verb: verb, Reason: "why"})
		require.Error(t, err, verb)
	}
	tree := load(t, root)
	_, err = mutate.Transition(tree, testDeps(), tree.Bugs[batchBugAlpha], mutate.TransitionInput{Verb: "start"})
	refusal = requireBatchError(t, err, mutate.ReasonWorkBatched)
	require.Equal(t, "tiller start batch core", refusal.Seam)

	_, err = mutate.Check(tree, testDeps(), vcs.New(root), tree.Bugs[batchBugAlpha], mutate.CheckInput{})
	require.Error(t, err)

	_, err = mutate.Resolve(load(t, root), testDeps(), mutate.ResolveInput{Noun: store.NounBug, Node: "alpha", By: "anything"})
	requireBatchError(t, err, mutate.ReasonWorkBatched)

	// Triage verbs still act on the member itself.
	_, err = mutate.Grade(load(t, root), testDeps(), mutate.GradeInput{Noun: store.NounBug, Node: "alpha", Severity: "blocker"})
	require.NoError(t, err)
	res := driveTo(t, root, store.NounBug, "alpha", mutate.TransitionInput{Verb: "return", Reason: "needs another look"})
	require.Equal(t, "proposed", res.Status)
	require.Equal(t, batchID, load(t, root).Bugs[batchBugAlpha].BatchID, "a returned member stays in an unstarted batch")
}

func TestReturningAMemberRefusesOnceItsBatchStarted(t *testing.T) {
	t.Parallel()
	root := layStartedBatch(t)
	tree := load(t, root)
	_, err := mutate.Transition(tree, testDeps(), tree.Bugs[batchBugAlpha], mutate.TransitionInput{Verb: "return", Reason: "second thoughts"})
	refusal := requireBatchError(t, err, mutate.ReasonBatchStarted)
	require.Equal(t, "tiller batch core --remove "+batchBugAlpha, refusal.Seam)
}

func TestBatchRunsTheShelfGatesAndRecordsItsOwnReview(t *testing.T) {
	t.Parallel()
	root := layStartedBatch(t)
	res := checkBatch(t, root)
	require.True(t, res.Passed)
	require.Equal(t, "check", res.Status)
	names := map[string]bool{}
	for _, g := range res.Gates {
		names[g.Name] = true
	}
	require.Equal(t, map[string]bool{"build": true, "tests": true}, names, "a batch runs the gate set one shelf node runs")
	gitCommitAllM(t, root, "batch gate evidence")

	pr := 77
	submitted := driveTo(t, root, store.NounBatch, "core", mutate.TransitionInput{Verb: "submit", PR: &pr})
	require.Equal(t, "in_review", submitted.Status)
	tree := load(t, root)
	require.Equal(t, 77, *tree.Batches[batchID].PR)
	require.Equal(t, store.GateStatusPass, tree.Batches[batchID].Gates["build"].Status)
	for _, id := range []string{batchBugAlpha, batchBugBeta, batchTweakDelta} {
		member, _ := tree.WorkNode(id)
		require.Nil(t, member.Body().PR, "the PR is recorded on the batch, never on a member")
		require.Empty(t, member.Body().Gates)
		require.Equal(t, "ready", member.Body().Status)
	}

	// Requested changes resume the batch on the same branch and PR.
	resumed := driveTo(t, root, store.NounBatch, "core", mutate.TransitionInput{Verb: "resume", Reason: "review asked for a rename"})
	require.Equal(t, "in_progress", resumed.Status)
	tree = load(t, root)
	require.Equal(t, batchBranch, tree.Batches[batchID].Branch)
	require.Equal(t, 77, *tree.Batches[batchID].PR)
}

func TestSubmitBatchRefusesOffItsRecordedBranch(t *testing.T) {
	t.Parallel()
	root := layStartedBatch(t)
	checkBatch(t, root)
	gitCommitAllM(t, root, "checked")
	gitRunM(t, root, "checkout", "main")
	gitRunM(t, root, "merge", "--ff-only", batchBranch)
	tree := load(t, root)
	_, err := mutate.Transition(tree, testDeps(), tree.Batches[batchID], mutate.TransitionInput{Verb: "submit"})
	var unsafe *mutate.BranchSafetyError
	require.ErrorAs(t, err, &unsafe)
	require.Equal(t, "branch_mismatch", unsafe.Reason)
}

func TestSyncCrossesTheBatchAndEveryMemberOnOneMerge(t *testing.T) {
	t.Parallel()
	root := layReviewedBatch(t)

	waiting, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, waiting.Crossings)
	require.Len(t, waiting.Awaiting, 1, "members of an unmerged batch await no merge of their own")
	require.Equal(t, store.NounBatch, waiting.Awaiting[0].Noun)
	require.Equal(t, batchBranch, waiting.Awaiting[0].Branch)

	gitRunM(t, root, "merge", "--no-ff", "-m", "Merge pull request #77", batchBranch)
	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Awaiting)
	require.Empty(t, res.UnrecordedMerges, "a batched member is accounted for")
	require.Len(t, res.Crossings, 4)
	require.Equal(t, store.NounBatch, res.Crossings[0].Noun)
	require.Equal(t, "ancestry", res.Crossings[0].Basis)
	for _, crossing := range res.Crossings[1:] {
		require.Equal(t, "batched", crossing.Basis)
		require.Equal(t, batchID, crossing.Batch)
		require.Empty(t, crossing.PolicyViolations)
	}

	tree := load(t, root)
	require.Equal(t, "done", tree.Batches[batchID].Status)
	for _, id := range []string{batchBugAlpha, batchBugBeta, batchTweakDelta} {
		member, _ := tree.WorkNode(id)
		require.Equal(t, "done", member.Body().Status)
		require.Equal(t, "batched", member.Body().DoneBasis)
		require.False(t, member.Body().Started)
		require.Equal(t, batchID, member.Body().BatchID, "the pointer stays as history")
	}
	require.Equal(t, batchID, lastEvent(t, root, "sync.tweak").After["batch"])
	require.Equal(t, "proposed", tree.Chores[batchChoreGamma].Status, "a node outside the batch is untouched")
}

func TestSyncRecordsARedGateOnTheBatchCrossing(t *testing.T) {
	t.Parallel()
	root := layStartedBatch(t)
	// Reach review with no gate ever recorded, the advisory-enforcement case.
	path := filepath.Join(root, ".tiller", "batches", "core", "batch.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Batch
	require.NoError(t, yaml.Unmarshal(data, &doc))
	pr := 77
	doc.Status, doc.PR = "in_review", &pr
	writeYAMLTest(t, path, &doc)
	gitCommitAllM(t, root, "batch in review")
	gitRunM(t, root, "branch", "-f", "main")
	landFix(t, root, "more.txt", batchBugBeta+" fix")
	gitRunM(t, root, "checkout", "main")
	gitRunM(t, root, "merge", "--no-ff", "-m", "merge", batchBranch)

	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, []string{"build", "tests"}, res.Crossings[0].PolicyViolations)
	require.Len(t, load(t, root).Batches[batchID].PolicyViolations, 2)
	require.Empty(t, load(t, root).Bugs[batchBugAlpha].PolicyViolations)
}

func TestSyncAcceptsFastForwardOnABatch(t *testing.T) {
	t.Parallel()
	root := layReviewedBatch(t)
	gitRunM(t, root, "merge", "--ff-only", batchBranch)
	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "a fast-forward is unobservable")

	res, err = mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{batchID}})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 4)
	require.Equal(t, "human_attested", res.Crossings[0].Basis)
	require.Equal(t, "batched", res.Crossings[1].Basis)
}

func TestSyncDoesNotAssertAFastForwardOnAMember(t *testing.T) {
	t.Parallel()
	root := layReviewedBatch(t)
	gitRunM(t, root, "merge", "--ff-only", batchBranch)
	_, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{batchBugAlpha}})
	var ineligible *mutate.AttestNotEligibleError
	require.ErrorAs(t, err, &ineligible)
	require.Equal(t, "in_review", load(t, root).Batches[batchID].Status)
}

func TestMembershipEditsFollowTheBatchState(t *testing.T) {
	t.Parallel()
	root := layStartedBatch(t)
	edit := func(in mutate.BatchEditInput) (mutate.BatchResult, error) {
		in.Batch = "core"
		return mutate.EditBatch(load(t, root), batchDeps(), in)
	}

	_, err := edit(mutate.BatchEditInput{Add: []string{batchChoreGamma}})
	refusal := requireBatchError(t, err, mutate.ReasonBatchMemberUnready)
	require.Equal(t, batchChoreGamma, refusal.Member, "a started batch takes only approved members")

	_, err = edit(mutate.BatchEditInput{Add: []string{batchBugBeta}, Remove: []string{batchBugBeta}})
	require.ErrorContains(t, err, "--add and --remove both name "+batchBugBeta)

	checkBatch(t, root)
	driveTo(t, root, store.NounChore, "gamma", mutate.TransitionInput{Verb: "approve"})
	added, err := edit(mutate.BatchEditInput{Add: []string{batchChoreGamma}})
	require.NoError(t, err)
	require.True(t, added.Changed)
	require.True(t, added.GatesStaled, "a change after check stales the batch's gates")
	require.Len(t, added.Members, 4)
	tree := load(t, root)
	require.Equal(t, "check", tree.Batches[batchID].Status)
	require.Equal(t, store.GateStatusStale, tree.Batches[batchID].Gates["build"].Status)
	_, err = mutate.Transition(tree, testDeps(), tree.Batches[batchID], mutate.TransitionInput{Verb: "submit"})
	var notGreen *mutate.RequiredGatesNotGreenError
	require.ErrorAs(t, err, &notGreen, "stale gates block submit until check runs again")
	_, err = mutate.Check(tree, testDeps(), vcs.New(root), tree.Batches[batchID], mutate.CheckInput{})
	uncommitted := requireBatchError(t, err, mutate.ReasonBatchStateUncommitted)
	require.Equal(t, []string{".tiller/chores/gamma/chore.yml"}, uncommitted.Paths, "the new member's pointer has to ride the batch branch")
	gitCommitAllM(t, root, "add gamma to the batch")
	require.True(t, checkBatch(t, root).Passed)

	removed, err := edit(mutate.BatchEditInput{Remove: []string{batchBugBeta}})
	require.NoError(t, err)
	require.Len(t, removed.Warnings, 1)
	require.Contains(t, removed.Warnings[0], "yours to revert")
	require.Contains(t, removed.Warnings[0], batchBranch)
	require.Empty(t, load(t, root).Bugs[batchBugBeta].BatchID)
	require.Empty(t, memberActivity(t, root, batchBugBeta), "a removed member is back in its own queue")
	require.Equal(t, "ready", load(t, root).Bugs[batchBugBeta].Status)

	again, err := edit(mutate.BatchEditInput{Add: []string{batchChoreGamma}})
	require.NoError(t, err)
	require.False(t, again.Changed, "adding a member already there is a no-op")
	_, err = edit(mutate.BatchEditInput{Remove: []string{batchBugBeta}})
	requireBatchError(t, err, mutate.ReasonBatchNotMember)

	gitCommitAllM(t, root, "remove beta from the batch")
	checkBatch(t, root)
	gitCommitAllM(t, root, "batch gate evidence")
	driveTo(t, root, store.NounBatch, "core", mutate.TransitionInput{Verb: "submit"})
	_, err = edit(mutate.BatchEditInput{Remove: []string{batchChoreGamma}})
	refusal = requireBatchError(t, err, mutate.ReasonBatchInReview)
	require.Contains(t, refusal.Seam, "tiller resume batch core")
	tree = load(t, root)
	_, err = mutate.Transition(tree, testDeps(), tree.Chores[batchChoreGamma], mutate.TransitionInput{Verb: "cancel", Reason: "no", Resolution: "wontfix"})
	requireBatchError(t, err, mutate.ReasonBatchInReview)

	driveTo(t, root, store.NounBatch, "core", mutate.TransitionInput{Verb: "resume", Reason: "drop the chore"})
	_, err = edit(mutate.BatchEditInput{Remove: []string{batchChoreGamma}})
	require.NoError(t, err)
}

func TestABatchThatLosesItsLastMemberIsCancelled(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	addBatch(t, root, batchBugAlpha, batchBugBeta)
	res, err := mutate.EditBatch(load(t, root), batchDeps(), mutate.BatchEditInput{Batch: batchID, Remove: []string{batchBugAlpha, batchBugBeta}})
	require.NoError(t, err)
	require.True(t, res.Cancelled)
	tree := load(t, root)
	require.Equal(t, "cancelled", tree.Batches[batchID].Status)
	require.Equal(t, "ready", tree.Bugs[batchBugAlpha].Status)
	closed := lastEvent(t, root, "cancel.batch")
	require.Equal(t, "cancelled", closed.After["status"])
	require.Equal(t, "its last member left", closed.After["reason"])
	require.Equal(t, []string{batchID}, closed.Targets)

	_, err = mutate.EditBatch(load(t, root), batchDeps(), mutate.BatchEditInput{Batch: batchID, Add: []string{batchBugAlpha}})
	requireBatchError(t, err, mutate.ReasonBatchClosed)
}

func TestCancellingAMemberRemovesItFromItsBatch(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	addBatch(t, root, batchBugAlpha, batchBugBeta)
	cancel := func(key string) {
		driveTo(t, root, store.NounBug, key, mutate.TransitionInput{Verb: "cancel", Reason: "not a bug", Resolution: "invalid"})
	}
	cancel("alpha")
	tree := load(t, root)
	require.Equal(t, "cancelled", tree.Bugs[batchBugAlpha].Status)
	require.Empty(t, tree.Bugs[batchBugAlpha].BatchID)
	require.Equal(t, "ready", tree.Batches[batchID].Status, "one member remains")
	left := lastEvent(t, root, "cancel.bug")
	require.Equal(t, batchID, left.Before["batch"], "the member's own cancel says which batch it left")
	departure := lastEvent(t, root, "edit.batch")
	require.Equal(t, []string{batchID}, departure.Targets)
	require.Equal(t, []any{batchBugAlpha}, departure.After["removed"])

	cancel("beta")
	tree = load(t, root)
	require.Equal(t, "cancelled", tree.Batches[batchID].Status, "its last member left")
	ev := lastEvent(t, root, "cancel.batch")
	require.Equal(t, []string{batchID}, ev.Targets)
	require.Equal(t, []any{batchBugBeta}, ev.After["removed"])
}

func TestCancellingABatchReleasesItsMembersAndCancelsNone(t *testing.T) {
	t.Parallel()
	root := layStartedBatch(t)
	// A later clock than the add's, so the release is each member's last
	// batch event by time and not by a tie on a random event id.
	later := testDeps()
	later.Clock = fixedClock{t: time.Date(2026, 6, 27, 15, 0, 0, 0, time.UTC)}
	res := driveWith(t, root, later, store.NounBatch, "core", mutate.TransitionInput{Verb: "cancel", Reason: "reviewing them apart after all"})
	require.Equal(t, "cancelled", res.Status)
	tree := load(t, root)
	for _, id := range []string{batchBugAlpha, batchBugBeta, batchTweakDelta} {
		member, _ := tree.WorkNode(id)
		require.Equal(t, "ready", member.Body().Status)
		require.Empty(t, member.Body().BatchID)
		require.Empty(t, gen.WorkActivity(tree, member))
	}
	ev := lastEvent(t, root, "cancel.batch")
	require.Equal(t, []any{batchBugAlpha, batchBugBeta, batchTweakDelta}, ev.After["released"])
	require.Equal(t, []string{batchID}, ev.Targets, "a released member is named, never targeted, by the batch's cancel")
	released := lastEvent(t, root, "batch.tweak")
	require.Equal(t, batchID, released.Before["batch"])
	require.Equal(t, "", released.After["batch"])

	// A released member is free to join another batch or start on its own.
	_, err := mutate.AddBatch(tree, batchDeps(), mutate.BatchInput{Key: "second", Title: "Second", Members: []string{batchBugAlpha, batchBugBeta}})
	require.NoError(t, err)
}

func TestClaimBatchRefusesWhileAnotherContributorHoldsAMember(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	_, err := mutate.ClaimWork(load(t, root), depsAs(testDeps(), "pat"), store.NounBug, "beta", "")
	require.NoError(t, err)
	_, err = mutate.AddBatch(load(t, root), depsAs(batchDeps(), "pat"), mutate.BatchInput{Key: "core", Title: "Core fixes", Members: []string{batchBugAlpha, batchBugBeta}})
	require.NoError(t, err, "the creator's own claim is no obstacle")

	_, err = mutate.ClaimWork(load(t, root), testDeps(), store.NounBatch, "core", "")
	refusal := requireBatchError(t, err, mutate.ReasonBatchMemberClaimed)
	require.Equal(t, batchBugBeta, refusal.Member)

	claimed, err := mutate.ClaimWork(load(t, root), depsAs(testDeps(), "pat"), store.NounBatch, "core", "")
	require.NoError(t, err)
	require.Equal(t, "pat", claimed.Owner)
	require.Equal(t, "claim.batch", lastEvent(t, root, "claim.batch").Type)
}

func TestSyncWarnsWhenAMergeNamesAMemberOfAnUnsubmittedBatch(t *testing.T) {
	t.Parallel()
	root := layStartedBatch(t)
	fix := landFix(t, root, "fix.txt", batchBugAlpha+" Preserve capture order")
	gitRunM(t, root, "checkout", "main")
	gitRunM(t, root, "merge", "--no-ff", "-m", "Merge the fixes", batchBranch)

	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings)
	require.Len(t, res.UnrecordedMerges, 1, "the batch answers for its members")
	finding := res.UnrecordedMerges[0]
	require.Equal(t, store.NounBatch, finding.Noun)
	require.Equal(t, batchID, finding.ID)
	require.Equal(t, "in_progress", finding.Status)
	require.Equal(t, fix, finding.Commits[0].Commit)
	require.Equal(t, "tiller status batch "+batchID, finding.NextStep())
}

func TestRecoveryDoesNotApplyToABatch(t *testing.T) {
	t.Parallel()
	root := layStartedBatch(t)
	_, err := mutate.RecoverReview(load(t, root), testDeps(), mutate.RecoverReviewInput{Noun: store.NounBatch, Key: "core",
		Head: "1111111111111111111111111111111111111111", MergeCommit: "2222222222222222222222222222222222222222", Reason: "merged early"})
	require.ErrorContains(t, err, "does not apply to a batch")
}

func TestALiveBatchHoldsItsOwnerOnTheRoster(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	_, err := mutate.AddBatch(load(t, root), depsAs(batchDeps(), "pat"), mutate.BatchInput{Key: "core", Title: "Core fixes", Members: []string{batchBugAlpha, batchBugBeta}})
	require.NoError(t, err)
	_, err = mutate.ClaimWork(load(t, root), depsAs(testDeps(), "pat"), store.NounBatch, "core", "")
	require.NoError(t, err)

	_, err = mutate.RemoveContributor(load(t, root), testDeps(), "pat")
	var held *mutate.RemoveContributorHoldingsError
	require.ErrorAs(t, err, &held)
	require.Equal(t, 1, held.Holdings.Batches)
	require.Zero(t, held.Holdings.Bugs, "the members carry no claim of their own")
}

func TestAProposedBatchMemberIsNotRemovedOutFromUnderItsBatch(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	addBatch(t, root, batchBugAlpha, batchChoreGamma)
	_, err := mutate.RemoveDraft(load(t, root), testDeps(), mutate.RemoveInput{Noun: store.NounChore, Node: "gamma", Reason: "not needed"})
	var blocked *mutate.RemoveBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Contains(t, blocked.Detail, "tiller batch core --remove "+batchChoreGamma)

	_, err = mutate.RemoveDraft(load(t, root), testDeps(), mutate.RemoveInput{Noun: store.NounBatch, Node: "core", Reason: "no"})
	require.ErrorContains(t, err, "remove does not apply to a batch")
}

func TestAStartedBatchBranchIsReservedAgainstCollisions(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	addBatch(t, root, batchBugAlpha, batchBugBeta)
	_, err := mutate.ClaimWork(load(t, root), testDeps(), store.NounBatch, "core", "")
	require.NoError(t, err)
	gitRunM(t, root, "branch", batchBranch)
	tree := load(t, root)
	_, err = mutate.Transition(tree, testDeps(), tree.Batches[batchID], mutate.TransitionInput{Verb: "start"})
	var unsafe *mutate.BranchSafetyError
	require.ErrorAs(t, err, &unsafe, "start records a new branch, so an existing ref of that name refuses")
	require.Equal(t, "branch_ref_conflict", unsafe.Reason)
}

func TestCancelBatchTakesNoResolution(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	addBatch(t, root, batchBugAlpha, batchBugBeta)
	tree := load(t, root)
	_, err := mutate.Transition(tree, testDeps(), tree.Batches[batchID], mutate.TransitionInput{Verb: "cancel", Reason: "no", Resolution: "wontfix"})
	require.ErrorContains(t, err, "cancelling a batch releases its members and cancels none")
	require.Equal(t, "ready", load(t, root).Batches[batchID].Status)
}

// The Headway-shaped accident one layer down: the batch record reaches the
// integration branch in the merge, and the member state files that point at it
// do not, because add batch's member writes were never committed. Sync must
// not close the batch over bugs it cannot see.
func TestSyncDoesNotCrossABatchWhoseMembersNeverReachedIntegration(t *testing.T) {
	t.Parallel()
	root := layReviewedBatch(t)
	gitRunM(t, root, "merge", "--no-ff", "-m", "Merge pull request #77", batchBranch)
	// Integration as it would be had only .tiller/batches been committed.
	for _, member := range []struct{ noun, key string }{{store.NounBug, "alpha"}, {store.NounBug, "beta"}, {store.NounTweak, "delta"}} {
		path := filepath.Join(root, store.ShelfRelDir(member.noun, member.key), store.ShelfStateFile(member.noun))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), "batch: "+batchID+"\n", "", 1)), 0o644))
	}
	gitCommitAllM(t, root, "integration without the member pointers")

	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "a batch with no members to cross does not cross alone")
	require.Len(t, res.MemberlessBatches, 1)
	finding := res.MemberlessBatches[0]
	require.Equal(t, batchID, finding.ID)
	require.Equal(t, `tiller cancel batch core --reason "<why>"`, finding.NextStep())
	require.Contains(t, finding.Warning(), "were never committed on "+batchBranch)
	tree := load(t, root)
	require.Equal(t, "in_review", tree.Batches[batchID].Status)
	require.Equal(t, "ready", tree.Bugs[batchBugAlpha].Status)

	// The repair the warning names leaves every bug free to be recovered.
	driveTo(t, root, store.NounBatch, "core", mutate.TransitionInput{Verb: "cancel", Reason: "member pointers never merged"})
	require.Equal(t, "cancelled", load(t, root).Batches[batchID].Status)
}

func TestSubmitBatchRefusesWhileItsStateIsUncommitted(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	addBatch(t, root, batchBugAlpha, batchBugBeta)
	_, err := mutate.ClaimWork(load(t, root), testDeps(), store.NounBatch, "core", "")
	require.NoError(t, err)
	started := driveTo(t, root, store.NounBatch, "core", mutate.TransitionInput{Verb: "start"})
	gitRunM(t, root, "checkout", "-b", started.Branch)
	// Only the batch folder is committed, the mistake the guard exists for.
	gitRunM(t, root, "add", "--", ".tiller/batches")
	gitRunM(t, root, "commit", "-m", "start batch, members forgotten")

	tree := load(t, root)
	_, err = mutate.Check(tree, testDeps(), vcs.New(root), tree.Batches[batchID], mutate.CheckInput{})
	refusal := requireBatchError(t, err, mutate.ReasonBatchStateUncommitted)
	require.Equal(t, []string{".tiller/bugs/alpha/bug.yml", ".tiller/bugs/beta/bug.yml"}, refusal.Paths)
	require.Equal(t, "git add -- .tiller/bugs/alpha/bug.yml .tiller/bugs/beta/bug.yml", refusal.Seam)
	require.Contains(t, refusal.Error(), "Commit these on branch "+batchBranch)

	gitCommitAllM(t, root, "the member pointers")
	require.True(t, checkBatch(t, root).Passed)
	tree = load(t, root)
	_, err = mutate.Transition(tree, testDeps(), tree.Batches[batchID], mutate.TransitionInput{Verb: "submit"})
	refusal = requireBatchError(t, err, mutate.ReasonBatchStateUncommitted)
	require.Equal(t, []string{".tiller/batches/core/batch.yml"}, refusal.Paths, "the gate state check wrote is uncommitted")
	require.Equal(t, "check", load(t, root).Batches[batchID].Status)

	gitCommitAllM(t, root, "gate evidence")
	driveTo(t, root, store.NounBatch, "core", mutate.TransitionInput{Verb: "submit"})
}

// A sync interrupted after it crossed the batch and before every member heals
// on the next run: a member of a done batch crosses on that batch's done.
func TestSyncHealsAMemberLeftBehindByAnInterruptedRun(t *testing.T) {
	t.Parallel()
	root := layReviewedBatch(t)
	gitRunM(t, root, "merge", "--no-ff", "-m", "Merge pull request #77", batchBranch)
	_, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	// Put one member back the way an interrupted run would have left it.
	path := filepath.Join(root, ".tiller", "bugs", "beta", "bug.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	stale := strings.Replace(strings.Replace(string(data), "status: done", "status: ready", 1), "done_basis: batched\n", "", 1)
	require.NoError(t, os.WriteFile(path, []byte(stale), 0o644))

	tree := load(t, root)
	require.Equal(t, gen.ActivityBatched, gen.WorkActivity(tree, tree.Bugs[batchBugBeta]), "it still waits on its batch")
	_, err = mutate.Transition(tree, testDeps(), tree.Bugs[batchBugBeta], mutate.TransitionInput{Verb: "cancel", Reason: "no", Resolution: "wontfix"})
	requireBatchError(t, err, mutate.ReasonBatchClosed)

	res, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 1)
	require.Equal(t, batchBugBeta, res.Crossings[0].ID)
	require.Equal(t, "batched", res.Crossings[0].Basis)
	require.Equal(t, "done", load(t, root).Bugs[batchBugBeta].Status)
}

func TestCancellingAMemberAtCheckStalesTheBatchGates(t *testing.T) {
	t.Parallel()
	root := layStartedBatch(t)
	checkBatch(t, root)
	driveTo(t, root, store.NounTweak, "delta", mutate.TransitionInput{Verb: "cancel", Reason: "not needed", Resolution: "wontfix"})
	tree := load(t, root)
	require.Equal(t, "check", tree.Batches[batchID].Status)
	require.Equal(t, store.GateStatusStale, tree.Batches[batchID].Gates["build"].Status)
	require.Equal(t, store.GateStatusStale, lastEvent(t, root, "edit.batch").After["gates"])
}

func TestMembershipChangeWithNoRecordedGatesStalesNothing(t *testing.T) {
	t.Parallel()
	root := layStartedBatch(t)
	// Reach check with no gate recorded, as a project with no shelf gate does.
	path := filepath.Join(root, ".tiller", "batches", "core", "batch.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), "status: in_progress", "status: check", 1)), 0o644))
	res, err := mutate.EditBatch(load(t, root), batchDeps(), mutate.BatchEditInput{Batch: "core", Remove: []string{batchTweakDelta}})
	require.NoError(t, err)
	require.False(t, res.GatesStaled, "nothing was recorded, so there is nothing to run again")
}

func TestReassignBatchRefusesWhileAnotherContributorHoldsAMember(t *testing.T) {
	t.Parallel()
	root := layBatchRepo(t)
	_, err := mutate.ClaimWork(load(t, root), testDeps(), store.NounBug, "beta", "")
	require.NoError(t, err)
	addBatch(t, root, batchBugAlpha, batchBugBeta)
	_, err = mutate.ReassignWork(load(t, root), testDeps(), store.NounBatch, "core", "pat", "pat takes the batch")
	refusal := requireBatchError(t, err, mutate.ReasonBatchMemberClaimed)
	require.Equal(t, batchBugBeta, refusal.Member)
	require.Equal(t, "tiller release bug bugs/beta", refusal.Seam)

	_, err = mutate.ReleaseWork(load(t, root), testDeps(), store.NounBug, "beta")
	require.NoError(t, err, "a member may release the claim it brought")
	res, err := mutate.ReassignWork(load(t, root), testDeps(), store.NounBatch, "core", "pat", "pat takes the batch")
	require.NoError(t, err)
	require.Equal(t, "pat", res.To)
}

func TestResumeEditAndResubmitKeepTheSamePR(t *testing.T) {
	t.Parallel()
	root := layReviewedBatch(t)
	gitRunM(t, root, "checkout", batchBranch)
	driveTo(t, root, store.NounBatch, "core", mutate.TransitionInput{Verb: "resume", Reason: "drop the tweak"})
	_, err := mutate.EditBatch(load(t, root), batchDeps(), mutate.BatchEditInput{Batch: "core", Remove: []string{batchTweakDelta}})
	require.NoError(t, err)
	gitCommitAllM(t, root, "resume and drop delta")
	checkBatch(t, root)
	gitCommitAllM(t, root, "gate evidence")

	tree := load(t, root)
	other := 78
	_, err = mutate.Transition(tree, testDeps(), tree.Batches[batchID], mutate.TransitionInput{Verb: "submit", PR: &other})
	var unsafe *mutate.BranchSafetyError
	require.ErrorAs(t, err, &unsafe, "a batch keeps the PR it recorded")
	require.Equal(t, "branch_review_identity", unsafe.Reason)

	pr := 77
	res := driveTo(t, root, store.NounBatch, "core", mutate.TransitionInput{Verb: "submit", PR: &pr})
	require.Equal(t, "in_review", res.Status)
	require.Equal(t, 77, *load(t, root).Batches[batchID].PR)
	require.Len(t, load(t, root).BatchMembers(batchID), 2)
}

func TestAcceptFastForwardOnAMemberNamesItsBatch(t *testing.T) {
	t.Parallel()
	root := layReviewedBatch(t)
	gitRunM(t, root, "merge", "--ff-only", batchBranch)
	_, err := mutate.Sync(load(t, root), testDeps(), vcs.New(root), mutate.SyncInput{AcceptFastForward: []string{batchBugAlpha}})
	var ineligible *mutate.AttestNotEligibleError
	require.ErrorAs(t, err, &ineligible)
	require.Equal(t, mutate.AttestReasonBatchMember, ineligible.Nodes[0].Reason)
	require.Equal(t, batchID, ineligible.Nodes[0].Batch)
}
