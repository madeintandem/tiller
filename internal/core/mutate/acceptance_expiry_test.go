package mutate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// These tests pin defect 02, old task attestation reused after a contract
// change: a changed feature eval contract expires the acceptance verdicts its
// unfinished child tasks recorded against it, whether the new contract is
// accepted by a direct basis refresh or by an amendment's revise. Attestation
// history stays, terminal tasks and unrelated features are untouched, and a
// spec-only edit expires nothing.

const judgedContractTmpl = `---
node: %s
criteria:
  - id: C1
    statement: %s
    polarity: introduce
    binding: { type: manual }
    required: true
  - id: C2
    statement: %s
    polarity: introduce
    binding: { type: eval, run: "Grade whether the copy reads naturally." }
    required: true
---

# Acceptance
`

const (
	c1Statement = "A reviewer confirms the layout"
	c2Statement = "The copy reads naturally"
)

// judgedFixture is one epic with two features. The accept feature carries a
// manual criterion (C1) and a rubric criterion (C2); its live task and its
// retired task were both accepted against that contract, and the retired one
// was then cancelled. The other feature's bystander task was accepted against
// its own copy of the same contract.
type judgedFixture struct {
	root      string
	deps      mutate.Deps
	featureID string
	otherID   string
	live      string
	retired   string
	bystander string
}

func (f judgedFixture) featureDir(key string) string {
	return filepath.Join(f.root, ".tiller", "epics", "app", "features", key)
}

func (f judgedFixture) taskYML(t *testing.T, id string) string {
	t.Helper()
	return taskYMLPath(t, f.root, id)
}

func (f judgedFixture) taskBytes(t *testing.T, id string) []byte {
	t.Helper()
	data, err := os.ReadFile(f.taskYML(t, id))
	require.NoError(t, err)
	return data
}

func (f judgedFixture) writeContract(t *testing.T, key, featureID, c1, c2 string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(f.featureDir(key), "feature.eval.md"),
		[]byte(fmt.Sprintf(judgedContractTmpl, featureID, c1, c2)), 0o644))
}

func (f judgedFixture) check(t *testing.T, id string) mutate.CheckResult {
	t.Helper()
	tree := load(t, f.root)
	res, err := mutate.Check(tree, f.deps, vcs.New(f.root), tree.Tasks[id], mutate.CheckInput{})
	require.NoError(t, err)
	return res
}

// accept drives one approved task to check on a human acceptance: the rubric
// output is recorded for C2, both criteria are attested, and check runs.
func (f judgedFixture) accept(t *testing.T, id string) {
	t.Helper()
	driveWith(t, f.root, f.deps, store.NounTask, id, mutate.TransitionInput{Verb: "start"})
	rubric := filepath.Join(t.TempDir(), "rubric.json")
	require.NoError(t, os.WriteFile(rubric, []byte(`{"schema": "tiller.rubric_output.v1", "criterion": "C2", "verdict": "pass", "rationale": "Reads naturally."}`), 0o644))
	tree := load(t, f.root)
	_, err := mutate.Eval(tree, f.deps, vcs.New(f.root), tree.Tasks[id], mutate.EvalInput{Criterion: "C2", RecordRubricOutput: rubric})
	require.NoError(t, err)
	for _, c := range []string{"C1", "C2"} {
		tree = load(t, f.root)
		_, err = mutate.Attest(tree, f.deps, tree.Tasks[id], mutate.AttestInput{Criterion: c, Status: store.GateStatusPass})
		require.NoError(t, err)
	}
	res := f.check(t, id)
	require.True(t, res.Passed, "an attested contract plus green gates passes")
	require.Equal(t, "check", res.Status)
}

func layJudgedFeature(t *testing.T, f *judgedFixture, key string, taskKeys ...string) (featureID string, taskIDs []string) {
	t.Helper()
	feat, err := mutate.AddFeature(load(t, f.root), f.deps, mutate.FeatureInput{Key: "app/" + key, Title: key, ChangeType: "create"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(f.featureDir(key), "feature.md"), []byte("# "+key+"\n\nShip "+key+".\n"), 0o644))
	f.writeContract(t, key, feat.ID, c1Statement, c2Statement)
	for _, tk := range taskKeys {
		res, err := mutate.AddTask(load(t, f.root), f.deps, mutate.TaskInput{Key: "app/" + key + "/" + tk, Title: tk})
		require.NoError(t, err)
		_, err = mutate.ClaimTask(load(t, f.root), f.deps, res.ID, "")
		require.NoError(t, err)
		writeTaskSpecAt(t, f.root, "app", key, tk, "## Approach\n\nBuild "+tk+".")
		taskIDs = append(taskIDs, res.ID)
	}
	driveWith(t, f.root, f.deps, store.NounFeature, "app/"+key, mutate.TransitionInput{Verb: "spec"})
	driveWith(t, f.root, f.deps, store.NounFeature, "app/"+key, mutate.TransitionInput{Verb: "approve"})
	return feat.ID, taskIDs
}

func layJudgedFixture(t *testing.T) judgedFixture {
	t.Helper()
	root := writeGatedRepo(t, "true", "true")
	seq := 0
	f := judgedFixture{root: root, deps: amendDepsCounting(root, &seq)}
	_, err := mutate.AddEpic(load(t, root), f.deps, mutate.EpicInput{Key: "app", Title: "App"})
	require.NoError(t, err)
	driveWith(t, root, f.deps, store.NounEpic, "app", mutate.TransitionInput{Verb: "approve"})

	var tasks []string
	f.featureID, tasks = layJudgedFeature(t, &f, "accept", "live", "retired")
	f.live, f.retired = tasks[0], tasks[1]
	f.otherID, tasks = layJudgedFeature(t, &f, "other", "bystander")
	f.bystander = tasks[0]

	for _, id := range []string{f.live, f.retired, f.bystander} {
		f.accept(t, id)
	}
	driveWith(t, root, f.deps, store.NounTask, f.retired, mutate.TransitionInput{Verb: "cancel", Reason: "no longer needed"})
	gitCommitAllM(t, root, "accepted")
	return f
}

// requireExpired asserts the task's acceptance no longer counts: the evals
// gate and every criterion in it are stale, the attestations that produced the
// old verdict are still on record, and a fresh check does not reuse them.
func requireExpired(t *testing.T, f judgedFixture, id string, attestations int) {
	t.Helper()
	tk := load(t, f.root).Tasks[id]
	require.Equal(t, store.GateStatusStale, tk.Gates["evals"].Status)
	for c, crit := range tk.Gates["evals"].Criteria {
		require.Equal(t, store.CriterionStatusStale, crit.Status, "criterion %s", c)
	}
	require.Len(t, tk.Attestations, attestations, "attestation history is preserved")

	res := f.check(t, id)
	require.False(t, res.Passed, "an expired verdict cannot pass a recheck")
	criteria := load(t, f.root).Tasks[id].Gates["evals"].Criteria
	require.Equal(t, store.CriterionStatusNeedsAttestation, criteria["C1"].Status, "the manual criterion needs a new attestation")
	require.Equal(t, store.CriterionStatusPending, criteria["C2"].Status, "the rubric criterion needs new rubric evidence")
}

func TestDirectRefreshExpiresUnfinishedChildAcceptance(t *testing.T) {
	t.Parallel()
	f := layJudgedFixture(t)
	retired := f.taskBytes(t, f.retired)
	bystander := f.taskBytes(t, f.bystander)
	attestations := len(load(t, f.root).Tasks[f.live].Attestations)

	f.writeContract(t, "accept", f.featureID, c1Statement+" on iPhone", c2Statement)
	driveWith(t, f.root, f.deps, store.NounFeature, "app/accept", mutate.TransitionInput{Verb: "approve", RefreshBasis: true})

	requireExpired(t, f, f.live, attestations)
	require.Equal(t, retired, f.taskBytes(t, f.retired), "a cancelled task keeps its history")
	require.Equal(t, bystander, f.taskBytes(t, f.bystander), "another feature's task is untouched")

	evs, err := events.List(load(t, f.root))
	require.NoError(t, err)
	var named int
	for _, ev := range eventsOfType(evs, "approve.feature") {
		if containsStr(ev.Targets, f.live) {
			named++
			require.Contains(t, ev.Targets, f.featureID)
		}
	}
	require.Equal(t, 1, named, "the refresh event names the task whose acceptance expired")

	// New acceptance restores submission.
	gitRunM(t, f.root, "checkout", "-b", load(t, f.root).Tasks[f.live].Branch)
	pr := 9
	err = tryTransition(t, f.root, f.deps, store.NounTask, f.live, mutate.TransitionInput{Verb: "submit", PR: &pr})
	var notGreen *mutate.RequiredGatesNotGreenError
	require.ErrorAs(t, err, &notGreen)
	tree := load(t, f.root)
	_, err = mutate.Attest(tree, f.deps, tree.Tasks[f.live], mutate.AttestInput{Criterion: "C1", Status: store.GateStatusPass})
	require.NoError(t, err)
	tree = load(t, f.root)
	_, err = mutate.Attest(tree, f.deps, tree.Tasks[f.live], mutate.AttestInput{Criterion: "C2", Status: store.GateStatusPass})
	require.NoError(t, err)
	require.True(t, f.check(t, f.live).Passed)
	require.NoError(t, tryTransition(t, f.root, f.deps, store.NounTask, f.live, mutate.TransitionInput{Verb: "submit", PR: &pr}))
}

func TestDirectRefreshOfAnUnchangedContractKeepsAcceptance(t *testing.T) {
	t.Parallel()
	f := layJudgedFixture(t)
	live := f.taskBytes(t, f.live)

	spec := filepath.Join(f.featureDir("accept"), "feature.md")
	require.NoError(t, os.WriteFile(spec, []byte("# accept\n\nShip accept, reworded.\n"), 0o644))
	driveWith(t, f.root, f.deps, store.NounFeature, "app/accept", mutate.TransitionInput{Verb: "approve", RefreshBasis: true})

	require.Equal(t, live, f.taskBytes(t, f.live), "a spec-only edit leaves the judgment standing")
	require.True(t, f.check(t, f.live).Passed)
}

// A child written concurrently with the refresh refuses the whole refresh: the
// feature pin, the child, and the event ledger stay as they were.
func TestDirectRefreshRefusesAChildSnapshotConflict(t *testing.T) {
	t.Parallel()
	f := layJudgedFixture(t)
	f.writeContract(t, "accept", f.featureID, c1Statement+" on iPhone", c2Statement)
	feature := filepath.Join(f.featureDir("accept"), "feature.yml")
	before, err := os.ReadFile(feature)
	require.NoError(t, err)
	eventsBefore := eventCount(t, f.root)

	deps := f.deps
	touched := concurrentlyTouch(t, f.taskYML(t, f.live), &deps)
	tree := load(t, f.root)
	_, err = mutate.Transition(tree, deps, tree.Features[f.featureID], mutate.TransitionInput{Verb: "approve", RefreshBasis: true})
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)

	after, err := os.ReadFile(feature)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, *touched, f.taskBytes(t, f.live), "the concurrent child edit is not overwritten")
	require.Equal(t, eventsBefore, eventCount(t, f.root))
}

// concurrentlyTouch makes every event id minted through deps append a comment
// to path, a stand-in for another process editing the file between the
// mutation's read and its commit. It returns the file's latest bytes.
func concurrentlyTouch(t *testing.T, path string, deps *mutate.Deps) *[]byte {
	t.Helper()
	latest := new([]byte)
	mint := deps.Mint
	n := 0
	deps.Mint = func(prefix string) string {
		if prefix == "EV" {
			n++
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			data = append(data, []byte(fmt.Sprintf("# concurrent edit %d\n", n))...)
			require.NoError(t, os.WriteFile(path, data, 0o644))
			*latest = data
		}
		return mint(prefix)
	}
	return latest
}

func TestAmendmentReviseExpiresUnfinishedChildAcceptance(t *testing.T) {
	t.Parallel()
	f := layJudgedFixture(t)
	retired := f.taskBytes(t, f.retired)
	bystander := f.taskBytes(t, f.bystander)
	attestations := len(load(t, f.root).Tasks[f.live].Attestations)

	f.writeContract(t, "accept", f.featureID, c1Statement, c2Statement+" to a first-time user")
	a := openAmendment(t, f.root, f.deps, "copy")
	a = stage(t, f.root, f.deps, a, mutate.StageInput{Op: store.AmendmentOpRevise, Target: f.featureID})
	approve(t, f.root, f.deps, a)

	requireExpired(t, f, f.live, attestations)
	require.Equal(t, retired, f.taskBytes(t, f.retired))
	require.Equal(t, bystander, f.taskBytes(t, f.bystander))

	evs, err := events.List(load(t, f.root))
	require.NoError(t, err)
	revised := eventsOfType(evs, "revise.feature")
	require.Len(t, revised, 1)
	require.Contains(t, revised[0].Targets, f.live)
}

func TestAmendmentReviseOfASpecOnlyEditKeepsAcceptance(t *testing.T) {
	t.Parallel()
	f := layJudgedFixture(t)
	live := f.taskBytes(t, f.live)
	require.NoError(t, os.WriteFile(filepath.Join(f.featureDir("accept"), "feature.md"), []byte("# accept\n\nReworded.\n"), 0o644))
	a := openAmendment(t, f.root, f.deps, "reword")
	a = stage(t, f.root, f.deps, a, mutate.StageInput{Op: store.AmendmentOpRevise, Target: f.featureID})
	approve(t, f.root, f.deps, a)
	require.Equal(t, live, f.taskBytes(t, f.live))
}

// A child the same batch cancels is terminal by the time the revise runs, so
// its recorded verdict stays as history rather than being staled.
func TestAmendmentReviseWithChildCancelLeavesTheCancelledChildsVerdict(t *testing.T) {
	t.Parallel()
	f := layJudgedFixture(t)
	f.writeContract(t, "accept", f.featureID, c1Statement+" on iPhone", c2Statement)
	a := openAmendment(t, f.root, f.deps, "drop-live")
	a = stage(t, f.root, f.deps, a, mutate.StageInput{Op: store.AmendmentOpRevise, Target: f.featureID})
	a = stage(t, f.root, f.deps, a, mutate.StageInput{Op: store.AmendmentOpCancel, Target: f.live, Reason: "out of scope"})
	approve(t, f.root, f.deps, a)

	tk := load(t, f.root).Tasks[f.live]
	require.Equal(t, "cancelled", tk.Status)
	require.Equal(t, store.GateStatusPass, tk.Gates["evals"].Status, "a cancelled task's verdict is history")
}

// A child the same batch revises takes both edits: its own re-pinned basis and
// the expiry of its acceptance.
func TestAmendmentReviseWithChildReviseAppliesBothEdits(t *testing.T) {
	t.Parallel()
	f := layJudgedFixture(t)
	priorPin := load(t, f.root).Tasks[f.live].AcceptedAgainst.Spec.Value
	f.writeContract(t, "accept", f.featureID, c1Statement+" on iPhone", c2Statement)
	writeTaskSpecAt(t, f.root, "app", "accept", "live", "## Approach\n\nBuild live for iPhone.")
	a := openAmendment(t, f.root, f.deps, "iphone")
	a = stage(t, f.root, f.deps, a, mutate.StageInput{Op: store.AmendmentOpRevise, Target: f.live})
	a = stage(t, f.root, f.deps, a, mutate.StageInput{Op: store.AmendmentOpRevise, Target: f.featureID})
	approve(t, f.root, f.deps, a)

	tk := load(t, f.root).Tasks[f.live]
	require.NotEqual(t, priorPin, tk.AcceptedAgainst.Spec.Value, "the child's own revise landed")
	require.Equal(t, store.GateStatusStale, tk.Gates["evals"].Status, "the feature revise expired its acceptance")
}

// A child changed between the batch's read and its commit refuses the whole
// approval: feature, children, amendment, and events are unchanged.
func TestAmendmentReviseRefusesAChildSnapshotConflict(t *testing.T) {
	t.Parallel()
	f := layJudgedFixture(t)
	f.writeContract(t, "accept", f.featureID, c1Statement+" on iPhone", c2Statement)
	a := openAmendment(t, f.root, f.deps, "conflict")
	a = stage(t, f.root, f.deps, a, mutate.StageInput{Op: store.AmendmentOpRevise, Target: f.featureID})

	livePath := f.taskYML(t, f.live)
	rel, err := filepath.Rel(f.root, livePath)
	require.NoError(t, err)
	before := snapshotDir(t, f.root)
	deps := f.deps
	touched := concurrentlyTouch(t, livePath, &deps)

	_, err = mutate.ApproveAmendment(load(t, f.root), deps, a)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)

	after := snapshotDir(t, f.root)
	delete(before, rel)
	delete(after, rel)
	requireNoWrites(t, before, after)
	require.Equal(t, *touched, f.taskBytes(t, f.live))
	require.False(t, strings.Contains(string(*touched), "status: stale"), "the child's verdict was not expired")
	require.Equal(t, store.AmendmentStatusProposed, loadAmendment(t, f.root, a.ID).Status)
}
