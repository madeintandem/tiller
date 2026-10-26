package mutate_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/gates"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// gatedManifestTmpl is a manifest carrying the required command gates check runs.
// The build/tests commands are substituted so a test can make them pass or fail;
// the evals gate is the `tiller eval` recursion point check no-ops until T13.
const gatedManifestTmpl = `schema_version: 12
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
gates:
  required:
    build: { kind: command, run: "%s", on: [feature, task] }
    tests: { kind: command, run: "%s", on: [feature, task] }
    evals: { kind: command, run: "tiller eval ${node.noun} ${node.id}", on: [feature, task] }
`

// fixedRev is the injected against revision: a git_commit RevisionRef the stub
// returns, so the recorded gate block is golden-stable without a real git repo.
var fixedRev = store.RevisionRef{
	Kind:       store.KindGitCommit,
	Value:      "git:abc1234",
	ObservedAt: time.Date(2026, 6, 27, 14, 30, 0, 0, time.UTC),
}

// stubRevisions is the read-only git stub. CurrentCommit returns the fixed against
// revision, and the tree is reported clean so the recorded gate `against` is the
// HEAD git_commit (the dirty-tree fingerprint path is exercised against a real
// repo, where it can be observed honestly).
//
// WorktreeBlobHash and WorktreeBlobMatches are the observations it does not
// stub, because the basis a gate run is guarded against was recorded through
// real git: the accepted_against pins on the fixture's feature and task carry
// real blob hashes, so answering a constant here would report every approved
// node as drifted. root is the repo the hashes are read from.
type stubRevisions struct {
	commit store.RevisionRef
	root   string
}

func (s stubRevisions) Branch() (string, error)         { return "main", nil }
func (s stubRevisions) Head() (string, error)           { return "abc1234", nil }
func (s stubRevisions) BlobHash(string) (string, error) { return "", nil }
func (s stubRevisions) WorktreeBlobHash(path string) (string, error) {
	return vcs.New(s.root).WorktreeBlobHash(path)
}
func (s stubRevisions) WorktreeBlobMatches(path, hash string) (bool, error) {
	return vcs.New(s.root).WorktreeBlobMatches(path, hash)
}
func (s stubRevisions) TreeHash(string) (string, error) { return "", nil }
func (s stubRevisions) CurrentCommit() (store.RevisionRef, error) {
	return s.commit, nil
}
func (s stubRevisions) IsDirty() (bool, error) { return false, nil }
func (s stubRevisions) WorktreeFingerprint() (store.RevisionRef, error) {
	return store.RevisionRef{Kind: store.KindSHA256, Value: "sha256:stub"}, nil
}
func (s stubRevisions) CodeIdentity() (string, error) { return "", nil }

// The integration commit a stub sync pins: one fixed commit, whose committed
// content the stub reports as empty just as it does for HEAD.
func (s stubRevisions) RefCommit(string) (string, error) {
	return "abc1234000000000000000000000000000000000", nil
}
func (s stubRevisions) CommitRef(string) (store.RevisionRef, error) { return s.commit, nil }
func (s stubRevisions) BlobHashAt(string, string) (string, error)   { return "", nil }
func (s stubRevisions) TreeHashAt(string, string) (string, error)   { return "", nil }
func (s stubRevisions) TreeInventoryAt(string, string) ([]string, error) {
	return nil, nil
}
func (s stubRevisions) IsMergedInto(string, string) (bool, error) { return false, nil }

// The merge-observation surface. HeadBranch answers a real branch name so a stub
// sync still resolves an integration branch the way a checkout on main would;
// the rest answer "nothing to compare", so neither the merge witness nor the
// content fallback ever fires against a stub and these tests stay about what they
// are about. The merge topology itself is exercised in sync_test.go against real
// git repositories, which is the only honest way to test it.
func (s stubRevisions) HeadBranch() (string, error)    { return "main", nil }
func (s stubRevisions) RefExists(string) (bool, error) { return false, nil }
func (s stubRevisions) FirstParentContains(string, string) (bool, error) {
	return false, nil
}
func (s stubRevisions) MergeBase(string, string) (string, error) {
	return "", nil
}
func (s stubRevisions) PatchID(string, string) (string, error)     { return "", nil }
func (s stubRevisions) PatchDigest(string, string) (string, error) { return "", nil }
func (s stubRevisions) FirstParentPatchIDs(string, int) ([]vcs.CommitPatch, error) {
	return nil, nil
}

// stubRev is the stub git window rooted at one repo, so the blob hashes the
// basis guard reads come from that repo while every other observation stays
// stubbed.
func stubRev(root string) stubRevisions { return stubRevisions{commit: fixedRev, root: root} }

// writeGatedRepo lays a temp repo whose manifest carries the required gates, with
// build and tests bound to the given shell commands. It is a real git repo with
// one commit from the start (rather than a plain directory a caller git-inits
// later): approve and sync both read HEAD now (the commit anchor,
// artifact-schema.md "Accepted basis"), and HEAD cannot resolve before a first
// commit exists, so the repo has to be born with one rather than gaining it
// after the fixture has already driven a node through approve.
//
// The repo is laid once per manifest and copied into each test.
func writeGatedRepo(t *testing.T, buildRun, testsRun string) string {
	t.Helper()
	return gatedRepos.Copy(t, formatGated(buildRun, testsRun), func(t testing.TB, dir string) {
		layGatedRepo(t, dir, buildRun, testsRun)
	})
}

// gatedRepos holds one laid gated repo per manifest text.
var gatedRepos = testgit.Templates{Name: "gated repo"}

// layGatedRepo lays the gated repo into dir, for writeGatedRepo and for the
// templates that build on it.
func layGatedRepo(t testing.TB, dir, buildRun, testsRun string) {
	t.Helper()
	tdir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(tdir, 0o755))
	body := []byte(formatGated(buildRun, testsRun))
	require.NoError(t, os.WriteFile(filepath.Join(tdir, "tiller.yml"), body, 0o644))
	gitInitM(t, dir)
	gitCommitAllM(t, dir, "init")
}

func formatGated(buildRun, testsRun string) string {
	return fmt.Sprintf(gatedManifestTmpl, buildRun, testsRun)
}

// layGatedTask drives a gated repo to one started task (in_progress), the fixture
// the task-grain check runs against. It is laid once per manifest and copied.
func layGatedTask(t *testing.T, buildRun, testsRun string) (root, taskID string) {
	t.Helper()
	root = gatedTasks.Copy(t, formatGated(buildRun, testsRun), func(t testing.TB, dir string) {
		layGatedRepo(t, dir, buildRun, testsRun)
		driveGatedTask(t, dir)
	})
	return root, gatedTaskID
}

// gatedTasks holds one laid layGatedTask fixture per manifest text.
var gatedTasks = testgit.Templates{Name: "gated task"}

// gatedTaskID is the task driveGatedTask lays: the fixed minter gives every
// task the same id.
const gatedTaskID = "T-aaaaaa"

// driveGatedTask walks an already-written repo forward to one started task
// (in_progress) and returns its id, so fixtures with different manifests share
// one drive.
func driveGatedTask(t testing.TB, root string) (taskID string) {
	t.Helper()
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/04-form-engine.md"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create", Source: "design/04-form-engine.md"})
	require.NoError(t, err)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{Key: "form-engine/certification-form-loading/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	driveTo(t, root, store.NounTask, gatedTaskID, mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounTask, gatedTaskID, mutate.TransitionInput{Verb: "start"})
	return gatedTaskID
}

// TestCheckTaskGreenAdvancesAndRecordsGolden is the schema-shape golden: a task
// whose build and tests pass records a full gates block (status, kind, run_by,
// run_at, the against revision, and evidence), the eval recursion is recorded
// pending, and the task advances in_progress -> check.
func TestCheckTaskGreenAdvancesAndRecordsGolden(t *testing.T) {
	t.Parallel()
	root, taskID := layGatedTask(t, "true", "true")

	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, taskID)
	res, err := mutate.Check(tree, testDeps(), stubRev(root), task, mutate.CheckInput{})
	require.NoError(t, err)
	require.True(t, res.Passed)
	require.Equal(t, "check", res.Status)
	require.True(t, res.Changed)

	reloaded := load(t, root)
	got := reloaded.Tasks[taskID]
	require.Equal(t, "check", got.Status, "a green task advances to check")

	block, err := yaml.Marshal(got.GatesBlock)
	require.NoError(t, err)

	goldenPath := filepath.Join("testdata", "check_task_gates.yml")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile(goldenPath, block, 0o644))
	}
	golden, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "golden file missing; inspect the produced block")
	emptyOutputDigest := fmt.Sprintf("%x", sha256.Sum256(nil))
	expected := strings.ReplaceAll(string(golden), "<empty-output-sha256>", emptyOutputDigest)
	require.Equal(t, expected, string(block))

	// The fixture commands produce no output, so each evidence reference carries
	// the SHA-256 of the empty combined-output stream.
	require.FileExists(t, filepath.Join(root, "artifacts", "tiller", "gates",
		"build-abc1234-out-"+emptyOutputDigest+".txt"))
	require.FileExists(t, filepath.Join(root, "artifacts", "tiller", "gates",
		"tests-abc1234-out-"+emptyOutputDigest+".txt"))
}

func TestCheckDoesNotRecordAPassWithoutEvidence(t *testing.T) {
	t.Parallel()
	root, taskID := layGatedTask(t, "true", "true")
	dir := filepath.Join(root, "artifacts", "tiller", "gates")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0o755)) })

	before := load(t, root).Tasks[taskID]
	_, err := mutate.Check(load(t, root), testDeps(), vcs.New(root), before, mutate.CheckInput{})
	require.Error(t, err)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, "evidence_unwritable", env.Reason)

	after := load(t, root).Tasks[taskID]
	require.Equal(t, before.Status, after.Status)
	require.Empty(t, after.Gates)
}

// TestCheckTaskRedDoesNotAdvance confirms a failing required gate records fail,
// reports Passed=false, and leaves the task in_progress (no advance to check).
func TestCheckTaskRedDoesNotAdvance(t *testing.T) {
	t.Parallel()
	root, taskID := layGatedTask(t, "false", "true")

	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, taskID)
	res, err := mutate.Check(tree, testDeps(), stubRev(root), task, mutate.CheckInput{})
	require.NoError(t, err, "a red gate is a recorded result, not a Go error")
	require.False(t, res.Passed)
	require.Equal(t, "in_progress", res.Status)

	reloaded := load(t, root)
	got := reloaded.Tasks[taskID]
	require.Equal(t, "in_progress", got.Status, "a red task does not advance")
	require.Equal(t, store.GateStatusFail, got.GatesBlock.Gates["build"].Status)
	require.Equal(t, store.GateStatusPass, got.GatesBlock.Gates["tests"].Status)
	require.Equal(t, "jc", got.GatesBlock.Gates["build"].RunBy)
	require.Equal(t, fixedRev.Value, got.GatesBlock.Gates["build"].Against.Value)
}

// TestCheckTaskIllegalState confirms a task in the wrong state is refused with the
// engine's typed illegal-transition error before any gate runs. The fixture stops
// at ready (not started), where check is not legal.
func TestCheckTaskIllegalState(t *testing.T) {
	t.Parallel()
	root := writeGatedRepo(t, "true", "true")
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{Key: "form-engine/certification-form-loading/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "approve"})

	tree := load(t, root)
	require.Equal(t, "ready", tree.Tasks["T-aaaaaa"].Status)
	task := resolveNode(t, tree, store.NounTask, "T-aaaaaa")

	_, err = mutate.Check(tree, testDeps(), stubRev(root), task, mutate.CheckInput{})
	require.Error(t, err)
	var ill *lifecycle.IllegalTransitionError
	require.ErrorAs(t, err, &ill)
	require.Equal(t, "ready", ill.From)
	require.Equal(t, "check", ill.Verb)

	// No gates block was recorded on the refused task.
	require.Empty(t, load(t, root).Tasks["T-aaaaaa"].GatesBlock.Gates)
}

// TestCheckRejectsAnExistingInvalidContract proves the derived eval path
// validates before any command gate or mutation runs. The active feature's
// accepted eval basis is refreshed directly so the test reaches contract
// validation rather than the separate active-basis guard.
func TestCheckRejectsAnExistingInvalidContract(t *testing.T) {
	t.Parallel()
	root, _ := layGatedTask(t, "touch command-ran", "touch command-ran")
	evalPath := filepath.Join(root, featureRelDirT, "feature.eval.md")
	require.NoError(t, os.WriteFile(evalPath,
		[]byte("---\nnode: F-aaaaaa\ncriteria: []\n---\n"), 0o644))

	ymlPath := filepath.Join(root, featureRelDirT, "feature.yml")
	data, err := os.ReadFile(ymlPath)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.NotNil(t, doc.AcceptedAgainst)
	evalHash, err := vcs.New(root).WorktreeBlobHash(filepath.Join(featureRelDirT, "feature.eval.md"))
	require.NoError(t, err)
	doc.AcceptedAgainst.Eval.Value = "git:" + evalHash
	data, err = yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(ymlPath, data, 0o644))

	tree := load(t, root)
	beforeEvents, err := events.List(tree)
	require.NoError(t, err)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")

	_, err = mutate.Check(tree, testDeps(), stubRev(root), feat, mutate.CheckInput{})
	require.Error(t, err)
	var invalid *store.EvalContractError
	require.ErrorAs(t, err, &invalid)
	require.Equal(t, "eval_contract_invalid", invalid.Reason())
	require.NoFileExists(t, filepath.Join(root, "command-ran"), "no command gate ran")

	afterEvents, listErr := events.List(load(t, root))
	require.NoError(t, listErr)
	require.Len(t, afterEvents, len(beforeEvents), "a rejected contract records no event")
}

// TestCheckFeatureRecordsNoStateChange confirms a feature-grain check records the
// gates block onto feature.yml without moving the feature's state. The feature
// carries an accepted basis (approve records it), so the check runs against a real
// git window: the working-tree feature.md/eval still match the basis, so the
// active-basis guard passes and the gates run.
func TestCheckFeatureRecordsNoStateChange(t *testing.T) {
	t.Parallel()
	root, _ := layGatedTask(t, "true", "true")
	gitInitM(t, root)
	gitCommitAllM(t, root, "lay tree")

	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")
	before := feat.NodeStatus()
	res, err := mutate.Check(tree, testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)
	require.True(t, res.Passed)
	require.Equal(t, before, res.Status, "check does not move a feature's state")

	reloaded := load(t, root)
	f := reloaded.Features["F-aaaaaa"]
	require.Equal(t, before, f.Status)
	require.Equal(t, store.GateStatusPass, f.GatesBlock.Gates["build"].Status)
}

// TestCheckFeatureSucceedsOnADoneFeatureUnderAnArchivedEpic pins the guard's
// terminal-self exception (whole-branch review, issue #36): checkFeature runs
// no legality gate at all (unlike checkTask), so `tiller check feature` can
// reach a feature whose status is already done, exactly the shape an archived
// epic's ordinary content sits in once sync's rollup crossed it. Refusing that
// check with ParentEpicRetiredError would name a remedy, `tiller cancel
// feature ...`, that is illegal from done, breaking the promise that a
// refusal's named remedy is always reachable. guardFeature's epic arm has to
// skip a feature that is itself terminal (done or cancelled), mirroring
// gen.RetiredAncestor's own terminal-self rule (gen/held.go's terminalStates).
// The done status is hand-laid directly onto feature.yml (setFeatureStatusOnDisk),
// the cheap legacy-tree shape rather than a full merge-observed rollup, since
// what this pins is the guard's read of a resting state, not how it arrived.
func TestCheckFeatureSucceedsOnADoneFeatureUnderAnArchivedEpic(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := layDraftPlan(t, deps, "alfa")
	setFeatureStatusOnDisk(t, root, "form-engine", "certification-form-loading", "done")
	setEpicStatus(t, root, "form-engine", "draft", "archived")

	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")

	_, err := mutate.Check(tree, deps, stubRev(root), feat, mutate.CheckInput{})
	require.NoError(t, err)
}

// TestCheckTaskRefusesUnderRetiredEpicLiveFeature is task 4's net 2 (issue
// #40), the check half: checkTask (check.go) calls guardTask on its own path
// entirely outside mutate.Transition, exactly the reasoning
// TestCheckTaskRefusesUnderDraftFeature and TestCheckTaskRefusesUnderCancelledFeature
// (transition_test.go) already give for the pre-gate and feature-cancelled
// arms; this is the retired-EPIC arm's own net at the check grain, which
// nothing exercised before (TestTaskVerbsRefuseUnderLiveFeatureCancelledEpic,
// transition_test.go, pins the identical shape for the start verb alone).
// The feature stays active (live) throughout; only its epic is cancelled,
// after the task is already started, so the epic answer is what must fire
// even though the nearer parent is still perfectly live.
func TestCheckTaskRefusesUnderRetiredEpicLiveFeature(t *testing.T) {
	t.Parallel()
	root, taskID := layGatedTask(t, "true", "true")
	driveTo(t, root, store.NounEpic, "form-engine",
		mutate.TransitionInput{Verb: "cancel", Reason: "programme killed"})

	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, taskID)
	_, err := mutate.Check(tree, testDeps(), stubRev(root), task, mutate.CheckInput{})

	var pe *mutate.ParentEpicRetiredError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, store.NounTask, pe.Noun)
	require.Equal(t, "cancelled", pe.EpicState)
	require.Equal(t, "in_progress", load(t, root).Tasks[taskID].Status, "a refused check must not move the task")
}

// recordingCheckProgress is a minimal gates.Progress fake that records which
// gates started and finished, in whatever order Check drives them, so a test
// can pin that CheckInput.Progress actually reaches every command gate a
// check run shells (issue #45's heartbeat sits behind this same threading).
type recordingCheckProgress struct {
	started, finished []string
}

func (p *recordingCheckProgress) GateStarted(name, _ string) {
	p.started = append(p.started, name)
}
func (p *recordingCheckProgress) GateStillRunning(string, time.Duration) {}
func (p *recordingCheckProgress) GateFinished(name, _ string, _ time.Duration) {
	p.finished = append(p.finished, name)
}

var _ gates.Progress = (*recordingCheckProgress)(nil)

// TestCheckThreadsProgressToGateRuns confirms CheckInput.Progress is not
// merely accepted but actually reaches gates.RunGate for every command gate a
// check evaluates: a nil Progress stays legal (every other test in this file
// passes CheckInput{}), and a non-nil one sees Started and Finished for both
// the manifest's own command gates (build, tests) and the eval contract's own
// mechanical criterion (eval-C1, the fixture's test binding composed onto the
// tests runner), the same threading the evals recursion uses.
func TestCheckThreadsProgressToGateRuns(t *testing.T) {
	t.Parallel()
	root, taskID := layGatedTask(t, "true", "true")

	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, taskID)
	p := &recordingCheckProgress{}
	res, err := mutate.Check(tree, testDeps(), stubRev(root), task, mutate.CheckInput{Progress: p})
	require.NoError(t, err)
	require.True(t, res.Passed)

	want := []string{"build", "tests", "eval-C1"}
	require.ElementsMatch(t, want, p.started)
	require.ElementsMatch(t, want, p.finished)
}
