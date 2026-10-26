package mutate_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func syncDeps() mutate.Deps {
	return testDeps()
}

// --- git helpers (a real temp repo, the only way to exercise vcs ancestry and
// blob fingerprints; the seam tests run here rather than against a stub) ---

func gitRunM(t testing.TB, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
}

func gitInitM(t testing.TB, dir string) {
	t.Helper()
	testgit.Init(t, dir)
}

func gitCommitAllM(t testing.TB, dir, msg string) {
	t.Helper()
	gitRunM(t, dir, "add", "-A")
	gitRunM(t, dir, "commit", "-m", msg)
}

// commitFile writes rel under root with content and commits it, so a test can
// advance HEAD without touching any file a recorded basis covers.
func commitFile(t *testing.T, root, rel, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(content+"\n"), 0o644))
	gitCommitAllM(t, root, "commit "+rel)
}

func setFeatureBranch(t *testing.T, root, epicKey, featKey, branch string) {
	t.Helper()
	p := filepath.Join(root, ".tiller", "epics", epicKey, "features", featKey, "feature.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Branch = branch
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, out, 0o644))
}

const featBranch = "tiller/sync-fixture"

// gitOutM runs a git command in dir and returns its trimmed output, so a test can
// name the exact commit a merge produced.
func gitOutM(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoErrorf(t, err, "git %s", strings.Join(args, " "))
	return strings.TrimSpace(string(out))
}

// layFeatureBranchGit lays an approved feature and reserved task in a real Git
// repo, commits the clean tree on main, and cuts a fixture branch with one work
// commit. The feature remains ready until a caller starts its task. HEAD returns
// to main with the branch unmerged and the working tree clean, so a caller can
// exercise a --no-ff merge, a fast-forward, a squash, or no merge. It is laid
// once per manifest and copied into each test.
func layFeatureBranchGit(t *testing.T, buildRun, testsRun string) string {
	t.Helper()
	return featureBranchRepos.Copy(t, formatGated(buildRun, testsRun), func(t testing.TB, root string) {
		layGatedRepo(t, root, buildRun, testsRun)
		driveFeatureBranch(t, root)
	})
}

// featureBranchRepos holds one laid layFeatureBranchGit fixture per manifest
// text.
var featureBranchRepos = testgit.Templates{Name: "feature branch repo"}

// driveFeatureBranch walks a freshly laid gated repo to layFeatureBranchGit's
// state.
func driveFeatureBranch(t testing.TB, root string) {
	t.Helper()
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	gitCommitAllM(t, root, "lay tree")
	gitRunM(t, root, "branch", "-M", "main")

	// Build the branch topology while the tree is clean.
	gitRunM(t, root, "checkout", "-b", featBranch)
	require.NoError(t, os.WriteFile(filepath.Join(root, "work.txt"), []byte("work\n"), 0o644))
	gitCommitAllM(t, root, "feature work")
	gitRunM(t, root, "checkout", "main")
}

// layActiveFeatureGit adds the classic local --no-ff merge to
// layFeatureBranchGit. The fixture branch is merged but not recorded on
// feature.yml; execution state is still supplied by the caller.
func layActiveFeatureGit(t *testing.T, buildRun, testsRun string) string {
	t.Helper()
	root := layFeatureBranchGit(t, buildRun, testsRun)
	gitRunM(t, root, "merge", "--no-ff", "-m", "merge feature", featBranch)
	return root
}

// submitTask puts one task under the feature into review, runs both grains'
// gates, and records the feature's working branch on feature.yml: the state sync
// observes. A PR always represents a task, so a task in review is the only shape
// that puts anything under a feature in front of a merge, and this is what the
// feature-grain submit fixture became when that verb went task-only.
//
// The feature-grain gates are run too, because the feature's rollup will not
// fire until they are recorded, and every caller here is about a feature that
// should cross once its task does.
//
// A red required gate leaves the task at in_progress, since check advances only
// on green. The red-gate fixtures still need it in review, so the transition is
// driven directly: advisory enforcement is exactly the case where work reaches
// review and then a merge with a gate that was never green.
func submitTask(t *testing.T, root string) {
	t.Helper()
	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key:   "form-engine/certification-form-loading/transcribe-the-schema",
		Title: "Transcribe the schema",
	})
	require.NoError(t, err)
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})

	task := resolveNode(t, load(t, root), store.NounTask, "T-aaaaaa")
	res, err := mutate.Check(load(t, root), testDeps(), vcs.New(root), task, mutate.CheckInput{})
	require.NoError(t, err)
	if !res.Passed {
		driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "check"})
	}
	stageSyncReview(t, root, "T-aaaaaa", featBranch)

	feat := resolveNode(t, load(t, root), store.NounFeature, "form-engine/certification-form-loading")
	_, err = mutate.Check(load(t, root), testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)
	setFeatureBranch(t, root, "form-engine", "certification-form-loading", featBranch)
}

// stageSyncReview isolates observation tests from submission's working-branch
// guards. These fixtures deliberately arrange merge topology before review state.
func stageSyncReview(t *testing.T, root, id, branch string) {
	t.Helper()
	p := taskYMLPath(t, root, id)
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	var tk store.Task
	require.NoError(t, yaml.Unmarshal(data, &tk))
	tk.Status, tk.Branch = "in_review", branch
	tk.Started = true
	tk.PR = nil
	tk.ReviewRef = nil
	if branch != "" {
		exists, err := vcs.New(root).RefExists("refs/heads/" + branch)
		require.NoError(t, err)
		if exists {
			tk.ReviewRef = &store.ReviewRef{Branch: branch, Head: store.RevisionRef{
				Kind: store.KindGitCommit, Value: "git:" + gitOutM(t, root, "rev-parse", "refs/heads/"+branch),
			}}
		}
	}
	out, err := yaml.Marshal(&tk)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, out, 0o644))
}

// layTaskGrainGit lays the task PR grain shape: an active feature whose branch is
// recorded, with two of its tasks sitting in in_review and no merge anywhere. It
// is the grain where a wrongly observed feature branch does the most damage,
// because one bad observation carries every in_review task under it to done at
// once.
//
// The feature already carries transcribe-the-schema (T-aaaaaa): the specify
// stage is one act, so layFeatureBranchGit's own writeStage2Output laid it and
// the plan gate flipped it to ready with the feature (cli-surface.md,
// "Specifying a feature"). Only the SECOND task is added here, and it forces its
// own id because testDeps' fixedMint hands every node the same T-aaaaaa. Before
// this repair the loop added two tasks and forced T-aaaaaa onto the first of
// them, so three task folders shared two ids: store.loadTasks keys Tree.Tasks by
// id, so the colliding folder was dropped from the loaded tree while its folder
// sat on disk at proposed forever, and every driveTo on T-aaaaaa moved
// transcribe-the-schema instead of the folder the loop had just written. The
// fixture happened to read right (two tasks in review) only because the dropped
// folder was invisible to every reader.
func layTaskGrainGit(t *testing.T) string {
	t.Helper()
	root := layFeatureBranchGit(t, "true", "true")

	seq := 0
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "T" {
			return "T-aaaaab"
		}
		seq++
		return fmt.Sprintf("%s-tg%02d", prefix, seq)
	}
	_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key:   "form-engine/certification-form-loading/validate-the-schema",
		Title: "Validate the schema",
	})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), deps, "T-aaaaab", "")
	require.NoError(t, err)
	writeTaskSpec(t, root, "validate-the-schema", "## Approach\n\nValidate the transcribed schema against the form model.")

	for _, id := range []string{"T-aaaaaa", "T-aaaaab"} {
		// T-aaaaaa rode the plan gate to ready already, so its approve is the
		// idempotent no-op the mutation contract promises; T-aaaaab is the
		// mid-flight addition that really moves proposed->ready here.
		driveTo(t, root, store.NounTask, id, mutate.TransitionInput{Verb: "approve"})
		driveTo(t, root, store.NounTask, id, mutate.TransitionInput{Verb: "start"})

		task := resolveNode(t, load(t, root), store.NounTask, id)
		_, err := mutate.Check(load(t, root), testDeps(), vcs.New(root), task, mutate.CheckInput{})
		require.NoError(t, err)
		stageSyncReview(t, root, id, featBranch)
	}
	setFeatureBranch(t, root, "form-engine", "certification-form-loading", featBranch)

	// The fixture's whole point is two in_review tasks under one feature and
	// nothing else, which is exactly what the id collision used to hide. The
	// folder count is compared against the loaded count on purpose: a tree read
	// alone cannot see a dropped folder, because the drop happens in the id-keyed
	// map the reader is holding.
	tree := load(t, root)
	entries, err := os.ReadDir(filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", store.TaskDirName))
	require.NoError(t, err)
	require.Len(t, entries, len(tree.Tasks), "every task folder on disk must own a distinct id")
	require.Len(t, tree.Tasks, 2, "two task folders, two ids, no folder dropped by a collision")
	for _, tk := range tree.Tasks {
		require.Equal(t, "in_review", tk.Status, "every task the fixture laid is in review")
	}
	return root
}

// setIntegrationBranch declares git.integration_branch in the repo's manifest,
// the field sync reads before falling back to the branch the checkout is on.
func setIntegrationBranch(t *testing.T, root, branch string) {
	t.Helper()
	p := filepath.Join(root, ".tiller", "tiller.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	data = append(data, []byte("git:\n  integration_branch: \""+branch+"\"\n")...)
	require.NoError(t, os.WriteFile(p, data, 0o644))
}

// declareIntegrationBranch is what every assertion test needs and no observation
// test does. --accept-fast-forward refuses outright unless the ref the run
// resolved is the manifest's git.integration_branch, so a fixture that leans on
// the HEAD fallback cannot use the flag at all. `tiller init` seeds the field, so
// declaring it here is the fixture catching up with what a real project carries;
// the HEAD-fallback tests deliberately do not call it.
func declareIntegrationBranch(t *testing.T, root string) {
	t.Helper()
	setIntegrationBranch(t, root, "main")
}

// layActiveFeatureGitWithEval is layActiveFeatureGit with a caller-supplied
// feature.eval.md, so a test can drive a feature with a required criterion that is
// not satisfied through to the done crossing and observe advisory enforcement.
func layActiveFeatureGitWithEval(t *testing.T, buildRun, testsRun, evalMD string) string {
	t.Helper()
	root := writeGatedRepo(t, buildRun, testsRun)
	gitInitM(t, root)

	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)

	dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Certification form loading\n\nReal spec prose, no longer a stub.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"), []byte(evalMD), 0o644))
	// The specify stage is one act: the explosion is part of what `spec feature`
	// validates, so the fixture lays the task the tests go on to name.
	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key: "form-engine/certification-form-loading/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), testDeps(), "T-aaaaaa", "")
	require.NoError(t, err)
	writeTaskSpec(t, root, "transcribe-the-schema", "## Approach\n\nTranscribe the JSON Schema into the form model.")

	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	gitCommitAllM(t, root, "lay tree")
	gitRunM(t, root, "branch", "-M", "main")

	gitRunM(t, root, "checkout", "-b", featBranch)
	require.NoError(t, os.WriteFile(filepath.Join(root, "work.txt"), []byte("work\n"), 0o644))
	gitCommitAllM(t, root, "feature work")
	gitRunM(t, root, "checkout", "main")
	gitRunM(t, root, "merge", "--no-ff", "-m", "merge feature", featBranch)

	return root
}

// requiredManualEvalMD is a feature.eval.md whose single required criterion is a
// manual sign-off, so the evals gate stays pending (needs_attestation) until a
// human attests it: the unsatisfied-required-gate case for advisory enforcement.
const requiredManualEvalMD = `---
node: F-aaaaaa
criteria:
  - id: C1
    statement: A reviewer confirms the rendered layout matches the source
    polarity: introduce
    binding: { type: manual }
    required: true
---

# Acceptance
`

// TestSyncPendingEvalsRecordsPolicyViolation is the A2 acceptance: a feature merged
// to done while a required gate is not green (its evals gate is pending because a
// required manual criterion is unattested) still reaches done, AND the non-green
// required gate is recorded as a policy_violations entry under advisory enforcement,
// naming the gate, the observed revision, and a status (cli-surface.md, The done
// crossing; artifact-schema.md, Gates).
func TestSyncPendingEvalsRecordsPolicyViolation(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGitWithEval(t, "true", "true", requiredManualEvalMD)
	submitTask(t, root)
	require.Equal(t, store.GateStatusPending, load(t, root).Features["F-aaaaaa"].GatesBlock.Gates["evals"].Status,
		"a required manual criterion leaves the evals gate pending")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.Equal(t, "done", fc[0].Status, "the human merge is the authority even with a non-green required gate")
	require.Contains(t, fc[0].PolicyViolations, "evals")

	f := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "done", f.Status)
	require.NotEmpty(t, f.PolicyViolations)
	var found bool
	for _, pv := range f.PolicyViolations {
		if pv.Gate == "evals" {
			found = true
			require.Equal(t, store.GateStatusPending, pv.Status, "the violation records the observed non-green status")
			require.Equal(t, store.KindGitCommit, pv.Observed.Kind, "the violation names the observed merge revision")
		}
	}
	require.True(t, found, "the unsatisfied evals gate is recorded as a policy_violation")
}

// TestSyncCleanDoneRecordsNoViolation is the clean complement: a feature whose
// every required gate is green at the merge (its eval criteria all satisfied)
// reaches done with no policy_violations recorded.
func TestSyncCleanDoneRecordsNoViolation(t *testing.T) {
	t.Parallel()
	greenMD := "---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: The suite passes\n    polarity: introduce\n    binding: { type: command, run: \"true\" }\n    required: true\n---\n"
	root := layActiveFeatureGitWithEval(t, "true", "true", greenMD)
	submitTask(t, root)
	require.Equal(t, store.GateStatusPass, load(t, root).Features["F-aaaaaa"].GatesBlock.Gates["evals"].Status)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.Equal(t, "done", fc[0].Status)
	require.Empty(t, fc[0].PolicyViolations, "a clean done over green required gates records no violation")
	require.Empty(t, load(t, root).Features["F-aaaaaa"].PolicyViolations)
}

// --- active-basis seam ---

// TestActiveBasisGuardRefusesUntilRefresh exercises the closed git seam: once a
// human records a feature's accepted basis (a git_blob of feature.md), editing and
// committing feature.md makes check and start refuse with active_basis (exit-1
// blocked), and approve --refresh-basis re-records the basis so check no longer
// refuses (cli-surface.md, "Speccing"). Those two verbs are the guard's whole
// surface; see the start clause for why submit is not among them.
func TestActiveBasisGuardRefusesUntilRefresh(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	gitInitM(t, root)

	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	gitCommitAllM(t, root, "lay tree")
	gitRunM(t, root, "branch", "-M", "main")

	// Record the accepted basis (git_blob) at the committed feature.md/eval.
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve", RefreshBasis: true})

	// Edit feature.md and commit, changing its blob.
	featDir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading")
	require.NoError(t, os.WriteFile(filepath.Join(featDir, "feature.md"),
		[]byte("# Certification form loading\n\nRevised: now also renders child forms.\n"), 0o644))
	gitCommitAllM(t, root, "revise spec")

	// check refuses with active_basis naming feature.md, and records nothing.
	//
	// The "records nothing" half is asserted through the EVENT STREAM, and the
	// choice matters: this fixture's manifest declares no gates at all, so an
	// empty gates block is the same map whether the guard bit or not and would
	// pin nothing. A check that ran to completion always appends one
	// check.feature event, whatever it found, so its absence is a property a
	// bypassed guard genuinely violates. No check has run in this fixture yet, so
	// the stream carries none to begin with.
	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")
	_, err = mutate.Check(tree, testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.Error(t, err)
	var ab *mutate.ActiveBasisError
	require.ErrorAs(t, err, &ab)
	require.Equal(t, []string{"feature.md"}, ab.Drifted)
	require.Equal(t, store.NounFeature, ab.Noun)
	require.False(t, hasEvent(t, load(t, root), "check.feature"),
		"a refused check writes no record of having run")

	// Task start enforces the parent feature's accepted basis too.
	_, err = mutate.Transition(tree, testDeps(), tree.Tasks["T-aaaaaa"], mutate.TransitionInput{Verb: "start"})
	require.ErrorAs(t, err, &ab)
	require.Equal(t, []string{"feature.md"}, ab.Drifted)

	// Refresh the basis after review; check no longer refuses for active_basis.
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve", RefreshBasis: true})
	tree = load(t, root)
	feat = resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")
	_, err = mutate.Check(tree, testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err, "after refresh-basis check no longer refuses on active_basis")
}

// --- sync to done ---

// TestSyncRecordsFeatureDone is the feature crossing: a task under it merged on
// the recorded branch, every non-cancelled task is now done, and the feature
// rolls up to done with an event written (cli-surface.md, "The done crossing").
func TestSyncRecordsFeatureDone(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.Equal(t, "done", fc[0].Status)
	require.Empty(t, fc[0].PolicyViolations, "a green merge records no policy violations")

	reloaded := load(t, root)
	require.Equal(t, "done", reloaded.Features["F-aaaaaa"].Status)
	require.NoFileExists(t, filepath.Join(root, ".tiller", "board.md"))

	require.True(t, hasEvent(t, reloaded, "sync.feature"), "a sync.feature event was recorded")
}

// TestSyncRecordsVerifiedAgainst confirms the done-crossing completion record: when
// sync crosses a feature to done it populates verified_against with git_blob refs of
// feature.md and feature.eval.md (the basis stale_spec drift reads), and a create
// feature with an empty surface records surfaces as empty (artifact-schema.md, Drift
// control hooks; The done crossing).
func TestSyncRecordsVerifiedAgainst(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)

	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	f := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "done", f.Status)

	va := f.VerifiedAgainst
	require.Equal(t, store.KindGitBlob, va.Spec.Kind, "spec is recorded as a git_blob ref")
	require.True(t, strings.HasPrefix(va.Spec.Value, "git:"), "the spec value carries the git blob prefix")
	require.Equal(t, ".tiller/epics/form-engine/features/certification-form-loading/feature.md", filepath.ToSlash(va.Spec.Path))

	require.Equal(t, store.KindGitBlob, va.Eval.Kind, "eval is recorded as a git_blob ref")
	require.True(t, strings.HasPrefix(va.Eval.Value, "git:"), "the eval value carries the git blob prefix")
	require.Equal(t, ".tiller/epics/form-engine/features/certification-form-loading/feature.eval.md", filepath.ToSlash(va.Eval.Path))

	require.Equal(t, store.KindGitCommit, va.Commit.Kind)
	require.NotEmpty(t, va.Commit.Value, "the completion record pins the integration commit so stale_spec can say since when")

	require.Empty(t, va.Surfaces, "a create feature with an empty surface records surfaces []")
}

// TestSyncRedGateRecordsPolicyViolations confirms advisory enforcement: a feature
// merged with a red required gate still crosses to done, and the red gate is
// recorded honestly as a policy_violations entry (cli-surface.md, "The done
// crossing").
func TestSyncRedGateRecordsPolicyViolations(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "false", "true") // build gate fails
	submitTask(t, root)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.Equal(t, "done", fc[0].Status, "the human merge is the authority even with a red gate")
	require.Contains(t, fc[0].PolicyViolations, "build")

	reloaded := load(t, root)
	f := reloaded.Features["F-aaaaaa"]
	require.Equal(t, "done", f.Status)
	require.NotEmpty(t, f.PolicyViolations)
	var gates []string
	for _, pv := range f.PolicyViolations {
		gates = append(gates, pv.Gate)
		require.Equal(t, store.GateStatusFail, pv.Status)
		require.Equal(t, store.KindGitCommit, pv.Observed.Kind)
	}
	require.Contains(t, gates, "build")
}

// TestSyncTaskGrainRollup confirms the whole crossing path, which is now the only
// one: a feature stays active while its task PR merges; sync crosses the
// in_review task to done and then rolls the parent feature up to done once every
// non-cancelled task is done and the feature gates have been checked.
func TestSyncTaskGrainRollup(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 3, "the task crosses, the feature rolls up, and the epic behind it")

	reloaded := load(t, root)
	require.Equal(t, "done", reloaded.Tasks["T-aaaaaa"].Status, "the merged task crosses to done")
	require.Equal(t, "done", reloaded.Features["F-aaaaaa"].Status, "the parent feature rolls up to done")

	require.True(t, hasEvent(t, reloaded, "sync.task"), "a sync.task event was recorded")
	require.True(t, hasEvent(t, reloaded, "sync.feature"), "a sync.feature rollup event was recorded")
}

// TestSyncUnmergedBranchIsNoOp confirms a task whose branch carries a commit
// not yet in the integration branch is left untouched: sync records no crossing,
// the task stays in review, and the feature stays active.
func TestSyncUnmergedBranchIsNoOp(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")

	// Build a genuinely unmerged branch: a commit off main that main never sees.
	gitRunM(t, root, "checkout", "-b", "tiller/never-merged")
	require.NoError(t, os.WriteFile(filepath.Join(root, "unmerged.txt"), []byte("nope\n"), 0o644))
	gitCommitAllM(t, root, "unmerged work")
	gitRunM(t, root, "checkout", "main")

	submitTask(t, root)
	stageSyncReview(t, root, "T-aaaaaa", "tiller/never-merged")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "an unmerged branch crosses nothing")
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status)
}

// crossingsOf narrows a sync result to one grain. Every one-epic-one-feature
// fixture in this file now rolls its epic up in the same run the last feature
// crosses (sync.go's epic pass), which is real behavior asserted where it
// belongs, in TestSyncRecordsEpicDoneWhenLastFeatureCrossesAndReopensOnNewWork.
// A test about a feature's own crossing therefore says so, rather than counting
// every crossing and silently changing meaning when a grain is added above it.
func crossingsOf(res mutate.SyncResult, noun string) []mutate.Crossing {
	var out []mutate.Crossing
	for _, c := range res.Crossings {
		if c.Noun == noun {
			out = append(out, c)
		}
	}
	return out
}

// crossedIDs lists the ids sync crossed, in the order it recorded them, so a test
// can assert both what moved and what did not without unpacking the whole
// crossing struct.
func crossedIDs(res mutate.SyncResult) []string {
	out := make([]string, 0, len(res.Crossings))
	for _, c := range res.Crossings {
		out = append(out, c.ID)
	}
	return out
}

// hasEvent reports whether the tree's event stream carries an event of the type.
func hasEvent(t *testing.T, tree *store.Tree, typ string) bool {
	t.Helper()
	evs, err := events.List(tree)
	require.NoError(t, err)
	for _, e := range evs {
		if e.Type == typ {
			return true
		}
	}
	return false
}

// findEvent returns the first event of the type in the tree's stream, failing the
// test when there is none.
func findEvent(t *testing.T, tree *store.Tree, typ string) events.Event {
	t.Helper()
	evs, err := events.List(tree)
	require.NoError(t, err)
	for _, e := range evs {
		if e.Type == typ {
			return e
		}
	}
	t.Fatalf("no %s event in the stream", typ)
	return events.Event{}
}

// --- the integration branch sync observes ---

// TestSyncRefusesWhenIntoIsTheFeaturesOwnBranch is the dry-run-3 defect, pinned as
// a regression. An agent standing on its own feature branch ran sync, and because
// Into defaulted to the literal "HEAD" the ancestry test asked whether the branch
// was an ancestor of itself. It always is, so sync recorded a done crossing no
// merge had earned, silently, at exit 0. The branch here is genuinely unmerged;
// sync must refuse rather than observe, and must not write anything.
func TestSyncRefusesWhenIntoIsTheFeaturesOwnBranch(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	submitTask(t, root)

	// Exactly the posture execute leaves an agent in when it says sync is next.
	gitRunM(t, root, "checkout", featBranch)
	require.Equal(t, featBranch, gitOutM(t, root, "branch", "--show-current"))

	merged, err := vcs.New(root).IsMergedInto(featBranch, "main")
	require.NoError(t, err)
	require.False(t, merged, "the branch is genuinely not merged into main")

	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.Error(t, err, "a branch is always its own ancestor, so this can never be evidence of a merge")
	var refused *mutate.IntoIsFeatureBranchError
	require.ErrorAs(t, err, &refused)
	require.Equal(t, featBranch, refused.Into)
	require.Equal(t, "HEAD", refused.Source, "the refusal names where the value came from")
	require.Equal(t, featBranch, refused.Branch)
	require.Equal(t, "T-aaaaaa", refused.NodeID)

	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status,
		"a refused sync records nothing")
	require.False(t, hasEvent(t, load(t, root), "sync.feature"))
}

// TestSyncRefusesAnExplicitFeatureBranchInto confirms the refusal is unconditional
// on where the value came from. Passing the feature's own branch to --into, in any
// spelling, is refused for the same reason the defaulted value is: there is no
// legitimate reading of "this branch was merged into itself".
func TestSyncRefusesAnExplicitFeatureBranchInto(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	submitTask(t, root)

	for _, into := range []string{featBranch, "refs/heads/" + featBranch} {
		_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{Into: into})
		var refused *mutate.IntoIsFeatureBranchError
		require.ErrorAsf(t, err, &refused, "--into %s", into)
		require.Equal(t, "--into", refused.Source)
	}

	// The manifest field is refused on the same terms.
	setIntegrationBranch(t, root, featBranch)
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	var refused *mutate.IntoIsFeatureBranchError
	require.ErrorAs(t, err, &refused)
	require.Equal(t, "git.integration_branch", refused.Source)
}

// TestSyncFastForwardIsNotObserved pins the cost of the merge witness, stated as a
// test rather than left as a surprise. A fast-forward merge puts the branch's
// commits on main's own first-parent line, which is byte for byte what a branch
// that never diverged leaves behind: same DAG, same refs, same trees. The two
// repositories below prove it, and because no observation separates them, sync
// declines to guess and leaves the node in review with the reason on its line.
//
// Reading the first case as a merge is what used to make the second case record a
// done crossing that no merge earned, under a completely correct configuration.
// The tool's promise is that done means the human's merge, so the ambiguity is
// resolved toward refusing, not toward crossing.
//
// This is not the last word on the fast-forward case, it is the no-flag half of
// it. The human's route out is `tiller sync --accept-fast-forward <feature>`,
// which crosses the same node on their assertion and records it as such
// (TestSyncAcceptFastForwardCrossesTheContainedBranch). What this test pins is
// that observation alone never crosses either repository, so nothing about the
// flag may be allowed to soften what plain sync will and will not believe.
func TestSyncFastForwardIsNotObserved(t *testing.T) {
	t.Parallel()
	merged := layFeatureBranchGit(t, "true", "true")
	gitRunM(t, merged, "merge", "--ff-only", featBranch)
	submitTask(t, merged)

	// The same shape, reached without any merge at all: a recorded branch that
	// carries no commits of its own, which is what an agent that branched and then
	// committed its work elsewhere leaves behind.
	vacuous := layFeatureBranchGit(t, "true", "true")
	gitRunM(t, vacuous, "branch", "-f", featBranch, "main")
	submitTask(t, vacuous)

	for _, root := range []string{merged, vacuous} {
		require.Equal(t, gitOutM(t, root, "rev-parse", featBranch), gitOutM(t, root, "rev-parse", "main"),
			"both leave the branch and main on one commit; nothing in git tells them apart")
		merged, err := vcs.New(root).IsMergedInto(featBranch, "main")
		require.NoError(t, err)
		require.True(t, merged, "and both satisfy ancestry, which is why ancestry alone is not evidence")

		res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
		require.NoError(t, err)
		require.Empty(t, res.Crossings, "containment with no merge commit to witness it crosses nothing")
		require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
		require.Len(t, res.Awaiting, 1)
		require.True(t, res.Awaiting[0].NoMergeWitness, "and the readout says which case this is")
		require.False(t, res.Awaiting[0].BranchMissing, "the branch is right there")
	}
}

// TestSyncVacuousBranchWithoutTheFlagNeverCrossesAtTaskGrain is the same defect at
// the grain that makes it worst. Under pr_grain [feature, task] a feature branch
// that carries nothing would carry every one of its in_review tasks to done at
// once, with no merge anywhere in the repository.
//
// "Without the flag" is load-bearing in the name. A named vacuous branch IS
// crossable by --accept-fast-forward, deliberately, because it sits in the same
// contained-but-unwitnessed set and the human asserting carries the
// responsibility (TestSyncAcceptFastForwardCrossesTheVacuousBranchToo). What must
// never change is this: unasked, sync crosses none of it.
func TestSyncVacuousBranchWithoutTheFlagNeverCrossesAtTaskGrain(t *testing.T) {
	t.Parallel()
	root := layTaskGrainGit(t)
	gitRunM(t, root, "branch", "-f", featBranch, "main")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "no merge happened, so no task crosses")
	for _, tk := range load(t, root).Tasks {
		require.Equal(t, "in_review", tk.Status)
	}
}

// TestSyncMergeCommitCrossesOnAncestry pins the classic local merge: a --no-ff
// merge commit is observed by ancestry, exactly as before this repair, and the
// crossing records that basis with no matched commit. The observation belongs to
// the TASK, because a PR always represents a task; the feature above it crosses
// on the arithmetic and records rollup.
func TestSyncMergeCommitCrossesOnAncestry(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	tc := crossingsOf(res, store.NounTask)
	require.Len(t, tc, 1)
	require.Equal(t, "ancestry", tc[0].Basis)
	require.Empty(t, tc[0].MatchedCommit, "ancestry needs no commit to point at")

	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.Equal(t, "rollup", fc[0].Basis,
		"no merge is observed at feature grain, so the crossing is arithmetic over its tasks")
	require.Empty(t, fc[0].MatchedCommit)

	ev := findEvent(t, load(t, root), "sync.task")
	require.Equal(t, "ancestry", ev.After["basis"], "every sync crossing records its basis")
	require.NotContains(t, ev.After, "merge_commit")
}

// TestSyncResolutionPrefersFlagThenManifestThenHead walks the resolution order.
// --into wins outright; with no flag the manifest field answers even from a
// checkout standing somewhere else; with neither, the branch the checkout is on
// answers, which is what keeps every project that works today working.
func TestSyncResolutionPrefersFlagThenManifestThenHead(t *testing.T) {
	t.Parallel()
	// --into beats a manifest field that names a branch with no merge on it.
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)
	gitRunM(t, root, "branch", "elsewhere", "HEAD~1")
	setIntegrationBranch(t, root, "elsewhere")
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{Into: "main"})
	require.NoError(t, err)
	require.Len(t, crossingsOf(res, store.NounFeature), 1, "--into main observes the merge the manifest field would have missed")
	require.Equal(t, "main", res.Into)
	require.Equal(t, "--into", res.IntoSource)

	// The manifest field answers from a checkout standing on an unrelated branch.
	root = layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)
	setIntegrationBranch(t, root, "main")
	gitRunM(t, root, "checkout", "-b", "scratch")
	res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, crossingsOf(res, store.NounFeature), 1)
	require.Equal(t, "git.integration_branch", res.IntoSource)

	// With neither, the branch the checkout is on answers.
	root = layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)
	res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, crossingsOf(res, store.NounFeature), 1)
	require.Equal(t, "main", res.Into)
	require.Equal(t, "HEAD", res.IntoSource)
}

// TestSyncRefusesALiteralHeadOnADetachedCheckout closes the hole the self-merge
// guard left open. The guard canonicalizes a literal "HEAD" through the branch
// HEAD points at, which on a detached checkout is no branch at all; it then had
// nothing to compare and waved the command through with its one check disabled,
// while `is-ancestor branch HEAD` stayed trivially true. It is the same
// unanswerable question a bare detached HEAD refuses on, so it gets the same
// refusal, and "@" is the same request spelled differently.
func TestSyncRefusesALiteralHeadOnADetachedCheckout(t *testing.T) {
	t.Parallel()
	for _, into := range []string{"HEAD", "@"} {
		root := layFeatureBranchGit(t, "true", "true")
		submitTask(t, root)
		gitRunM(t, root, "checkout", "--detach", featBranch)

		merged, err := vcs.New(root).IsMergedInto(featBranch, "main")
		require.NoError(t, err)
		require.False(t, merged, "the branch is genuinely not merged into main")

		_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{Into: into})
		require.Error(t, err, "--into %s on a detached checkout names no branch", into)
		var unresolved *mutate.IntoUnresolvedError
		require.ErrorAs(t, err, &unresolved)
		require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
	}
}

// TestSyncOnAMasterProjectIsUnchanged is the backward-compatibility case that
// forced the resolution order. A project that integrates on master, with a
// manifest written before integration_branch existed, running sync from master,
// must observe exactly what it observed before: master.
func TestSyncOnAMasterProjectIsUnchanged(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	gitRunM(t, root, "branch", "-M", "master")
	submitTask(t, root)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, crossingsOf(res, store.NounFeature), 1, "a master project keeps working with no manifest edit")
	require.Equal(t, "master", res.Into)
	require.Equal(t, "HEAD", res.IntoSource)
	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status)
}

// TestSyncRefusesOnADetachedHeadWithNothingDeclared pins the one deliberate
// behavior change for an existing project. A HEAD detached at a feature branch tip
// reproduces the self-merge defect with no branch name left to catch it by, so
// sync refuses rather than guess.
func TestSyncRefusesOnADetachedHeadWithNothingDeclared(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)
	gitRunM(t, root, "checkout", "--detach", "main")

	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.Error(t, err)
	var unresolved *mutate.IntoUnresolvedError
	require.ErrorAs(t, err, &unresolved)
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)

	// Either breadcrumb the refusal names clears it.
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{Into: "main"})
	require.NoError(t, err)
	require.Len(t, crossingsOf(res, store.NounFeature), 1)
}

// --- squash merges, and the constraints that keep the content fallback honest ---

// squashMergeFeature squashes the feature branch onto main the way GitHub's
// default merge button does, and returns the squash commit's full sha.
func squashMergeFeature(t *testing.T, root string) string {
	t.Helper()
	gitRunM(t, root, "merge", "--squash", featBranch)
	gitCommitAllM(t, root, "squash merge feature (#41)")
	return gitOutM(t, root, "rev-parse", "HEAD")
}

// TestSyncObservesASquashMerge is the second half of the repair: GitHub's default
// merge button makes ancestry permanently false, which used to strand the node in
// in_review silently at exit 0. The branch's net change is present in main under
// the same stable patch-id, so the crossing is recorded on content evidence and
// says so.
func TestSyncObservesASquashMerge(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	squash := squashMergeFeature(t, root)
	submitTask(t, root)

	merged, err := vcs.New(root).IsMergedInto(featBranch, "main")
	require.NoError(t, err)
	require.False(t, merged, "a squash merge leaves ancestry permanently false")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	tc := crossingsOf(res, store.NounTask)
	require.Len(t, tc, 1)
	require.Equal(t, "done", tc[0].Status)
	require.Equal(t, "content", tc[0].Basis)
	require.Equal(t, "git:"+squash, tc[0].MatchedCommit)

	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.Equal(t, "done", fc[0].Status)
	require.Equal(t, "rollup", fc[0].Basis, "the squash carried the task; the feature is arithmetic")
	require.Empty(t, fc[0].MatchedCommit, "a rollup points at no commit, because it observed none")
	require.Empty(t, res.Awaiting, "nothing is left in review")

	reloaded := load(t, root)
	require.Equal(t, "done", reloaded.Features["F-aaaaaa"].Status)

	ev := findEvent(t, reloaded, "sync.task")
	require.Equal(t, "content", ev.After["basis"], "the weaker evidence is recorded on the event")
	require.Equal(t, "git:"+squash, ev.After["merge_commit"])

	fev := findEvent(t, reloaded, "sync.feature")
	require.Equal(t, "rollup", fev.After["basis"])
	require.NotContains(t, fev.After, "merge_commit",
		"a rollup observed nothing, so it points the auditor at no commit")
}

// TestSyncSquashCarriesTasksToDone confirms the squash reaches the task grain: a
// squash-merged feature branch carries its in_review task PRs to done exactly as
// an ancestry-merged one does, and the task crossing records the content basis
// with the commit it matched. The feature above it rolls up, on arithmetic
// rather than on that evidence.
func TestSyncSquashCarriesTasksToDone(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	squash := squashMergeFeature(t, root)

	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{Key: "form-engine/certification-form-loading/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, "T-aaaaaa")
	_, err = mutate.Check(tree, testDeps(), vcs.New(root), task, mutate.CheckInput{})
	require.NoError(t, err)
	stageSyncReview(t, root, "T-aaaaaa", featBranch)

	feat := resolveNode(t, load(t, root), store.NounFeature, "form-engine/certification-form-loading")
	_, err = mutate.Check(load(t, root), testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)
	setFeatureBranch(t, root, "form-engine", "certification-form-loading", featBranch)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	tc := crossingsOf(res, store.NounTask)
	require.Len(t, tc, 1, "the task crosses on the squash")
	require.Equal(t, "content", tc[0].Basis)
	require.Equal(t, "git:"+squash, tc[0].MatchedCommit)

	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1, "and the feature rolls up over it")
	require.Equal(t, "rollup", fc[0].Basis)
	require.Empty(t, fc[0].MatchedCommit)
	require.Equal(t, "done", load(t, root).Tasks["T-aaaaaa"].Status)
}

// TestSyncContentFallbackIgnoresADifferentChange is the constraint that matters:
// the fallback cannot fire for work the integration branch does not contain. Here
// main gained a commit touching the very same file with different content, and the
// branch stays in review.
func TestSyncContentFallbackIgnoresADifferentChange(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	require.NoError(t, os.WriteFile(filepath.Join(root, "work.txt"), []byte("something else entirely\n"), 0o644))
	gitCommitAllM(t, root, "different work on main")
	submitTask(t, root)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "an unmerged branch crosses nothing, however close the content")
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
}

// TestSyncContentFallbackNeedsTheWholeNetChange pins the single-commit constraint:
// partial overlap never matches. main here carries a cherry-pick of the branch's
// first commit, so part of the branch's work is genuinely present, but the branch's
// whole net change is not equal to any one commit main gained.
func TestSyncContentFallbackNeedsTheWholeNetChange(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	first := gitOutM(t, root, "rev-parse", featBranch)

	gitRunM(t, root, "checkout", featBranch)
	require.NoError(t, os.WriteFile(filepath.Join(root, "more.txt"), []byte("more work\n"), 0o644))
	gitCommitAllM(t, root, "second feature commit")
	gitRunM(t, root, "checkout", "main")
	// Move main off the branch point first, so the cherry-pick is a genuinely new
	// commit rather than the branch's own commit arriving by fast-forward.
	require.NoError(t, os.WriteFile(filepath.Join(root, "elsewhere.txt"), []byte("main moves on\n"), 0o644))
	gitCommitAllM(t, root, "unrelated main work")
	gitRunM(t, root, "cherry-pick", first)

	submitTask(t, root)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "half the branch's work in main is not the branch landing")
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
}

// TestSyncContentFallbackIsWhitespaceFaithful pins the constraint that keeps the
// fallback from fabricating outright. `git patch-id` strips whitespace before
// hashing, so a branch that nests a YAML key under its parent and an integration
// branch that instead outdents the same key to the top level reduce to one id,
// even though they produce two different files and neither contains the other's
// change. Crossing that would be a done for work that landed nowhere, which is
// exactly the class of defect this whole repair exists to kill, so the patch text
// itself is compared before any match is believed.
func TestSyncContentFallbackIsWhitespaceFaithful(t *testing.T) {
	t.Parallel()
	base := "root:\n  a: 1\n  b: 2\n"
	branchWants := "root:\n  a: 1\n    b: 2\n" // nested under a
	mainGot := "root:\n  a: 1\nb: 2\n"         // outdented to the top level

	root := layFeatureBranchGit(t, "true", "true")
	cfg := filepath.Join(root, "config.yml")
	require.NoError(t, os.WriteFile(cfg, []byte(base), 0o644))
	gitCommitAllM(t, root, "add config")

	// Recut the branch off main so its whole net change is the one edit under
	// test, which is what the fallback compares.
	gitRunM(t, root, "branch", "-f", featBranch, "main")
	gitRunM(t, root, "checkout", featBranch)
	require.NoError(t, os.WriteFile(cfg, []byte(branchWants), 0o644))
	gitCommitAllM(t, root, "nest b under a")

	gitRunM(t, root, "checkout", "main")
	require.NoError(t, os.WriteFile(cfg, []byte(mainGot), 0o644))
	gitCommitAllM(t, root, "outdent b to the top level")

	// The premise: git itself reduces the two changes to one patch-id.
	rev := vcs.New(root)
	mergeBase, err := rev.MergeBase(featBranch, "main")
	require.NoError(t, err)
	branchID, err := rev.PatchID(mergeBase, featBranch)
	require.NoError(t, err)
	mainID, err := rev.PatchID("main^1", "main")
	require.NoError(t, err)
	require.Equal(t, branchID, mainID, "patch-id ignores whitespace, so these two collide")
	require.NotEqual(t, gitOutM(t, root, "show", "main:config.yml"),
		gitOutM(t, root, "show", featBranch+":config.yml"),
		"and yet the two files genuinely differ")

	submitTask(t, root)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "a colliding patch-id is not the branch's change landing")
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
}

// TestSyncContentFallbackIsMisattributionNotFabrication pins the residual the
// design accepted, so it cannot drift unnoticed. A human who cherry-picks a
// single-commit branch onto the integration branch and abandons the branch leaves
// content indistinguishable from a squash, and sync crosses on content evidence.
// The failure mode is misattribution (which act put the work there), never
// fabrication: the change really is in main. That is categorically weaker than the
// defect this repair kills, which recorded done for work integrated nowhere.
func TestSyncContentFallbackIsMisattributionNotFabrication(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	// Move main off the branch point first, so the cherry-pick is a genuinely new
	// commit and ancestry stays false; otherwise the branch's own commit would
	// simply arrive on main and the stronger evidence would answer.
	require.NoError(t, os.WriteFile(filepath.Join(root, "elsewhere.txt"), []byte("main moves on\n"), 0o644))
	gitCommitAllM(t, root, "unrelated main work")
	gitRunM(t, root, "cherry-pick", gitOutM(t, root, "rev-parse", featBranch))
	picked := gitOutM(t, root, "rev-parse", "HEAD")
	submitTask(t, root)

	merged, err := vcs.New(root).IsMergedInto(featBranch, "main")
	require.NoError(t, err)
	require.False(t, merged, "a cherry-pick leaves ancestry false, exactly as a squash does")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	tc := crossingsOf(res, store.NounTask)
	require.Len(t, tc, 1, "the branch's whole net change is in main, by a human's hand")
	require.Equal(t, "content", tc[0].Basis)
	require.Equal(t, "git:"+picked, tc[0].MatchedCommit)
	require.Len(t, crossingsOf(res, store.NounFeature), 1, "and the feature rolls up over it")
}

// TestSyncContentFallbackIgnoresAnEmptyNetPatch pins the first constraint: a
// branch whose commits net out to no change at all carries no evidence, and an
// empty patch must never be read as a match against anything.
func TestSyncContentFallbackIgnoresAnEmptyNetPatch(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")

	// Undo the branch's own work on the branch: two commits, zero net change.
	gitRunM(t, root, "checkout", featBranch)
	require.NoError(t, os.Remove(filepath.Join(root, "work.txt")))
	gitCommitAllM(t, root, "revert the feature work")
	gitRunM(t, root, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(root, "elsewhere.txt"), []byte("main moves on\n"), 0o644))
	gitCommitAllM(t, root, "unrelated main work")

	submitTask(t, root)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "a branch with no net change is no evidence of anything")
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
}

// TestSyncContentFallbackIsBounded pins the scan bound: a squash that has scrolled
// past the newest fifty first-parent commits of the integration branch is out of
// reach, so an ancient coincidental match cannot fire and the walk stays cheap.
func TestSyncContentFallbackIsBounded(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	squashMergeFeature(t, root)
	for i := 0; i < 50; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(root, "churn.txt"),
			[]byte(fmt.Sprintf("commit %d\n", i)), 0o644))
		gitCommitAllM(t, root, fmt.Sprintf("later main work %d", i))
	}
	submitTask(t, root)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "the squash is older than the bound")
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
}

// --- the zero-crossing readout ---

// TestSyncNamesTheNodesStillInReview is the companion to the two detections: a
// branch deleted after its merge is still unobservable (there is nothing left to
// diff), so sync names the node and says the branch is gone rather than reporting
// nothing and exiting as if all were well.
func TestSyncNamesTheNodesStillInReview(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)
	gitRunM(t, root, "branch", "-D", featBranch)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err, "an unobservable merge is a report, not a refusal")
	require.Empty(t, res.Crossings)
	require.Len(t, res.Awaiting, 1)
	require.Equal(t, store.NounTask, res.Awaiting[0].Noun)
	require.Equal(t, "T-aaaaaa", res.Awaiting[0].ID)
	require.Equal(t, featBranch, res.Awaiting[0].Branch,
		"a task borrows its parent feature's branch, which is where its work merges")
	require.True(t, res.Awaiting[0].BranchMissing, "the local branch was deleted after the merge")
}

// TestSyncAwaitingSeparatesNotMergedFromBranchDeleted confirms the two silent
// cases stay distinguishable: a branch that still exists and simply has not landed
// is awaiting without the missing-branch flag.
func TestSyncAwaitingSeparatesNotMergedFromBranchDeleted(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	submitTask(t, root)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings)
	require.Len(t, res.Awaiting, 1)
	require.False(t, res.Awaiting[0].BranchMissing, "the branch is there; the merge is not")
	require.False(t, res.Awaiting[0].NoMergeWitness, "and it is genuinely not contained either")
}

// TestSyncAwaitingNamesEachTaskByItsOwnKey pins one envelope invariant. The
// branch is borrowed from the parent feature, because that really is the branch
// the task's work merges on; the key is the task's own, so the noun/key pair an
// entry emits resolves back to the task the entry is about.
func TestSyncAwaitingNamesEachTaskByItsOwnKey(t *testing.T) {
	t.Parallel()
	root := layTaskGrainGit(t)

	tree := load(t, root)
	res, err := mutate.Sync(tree, syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings)
	require.Len(t, res.Awaiting, 2)
	for _, a := range res.Awaiting {
		require.Equal(t, store.NounTask, a.Noun)
		require.Equal(t, tree.Tasks[a.ID].Branch, a.Branch, "each task names its recorded branch")

		n, rerr := resolve.NewResolver(load(t, root)).Resolve(resolve.Ref{Noun: a.Noun, Raw: a.Key})
		require.NoError(t, rerr, "the emitted noun/key pair must resolve")
		require.Equal(t, a.ID, n.NodeID(), "and must resolve to the entry's own node")
	}
}

// TestSyncAwaitingRanksSiblingTasksByDependency pins the awaiting list against
// the same rule every other sibling-task list follows (artifact-schema.md,
// Tasks: Ordering): it reads in the order depends_on records. The fixture's
// alfa follows zulu, and alfa leads by both key and id, so a list taking the
// group as the loader held it, or falling back to id, reads them backwards.
func TestSyncAwaitingRanksSiblingTasksByDependency(t *testing.T) {
	t.Parallel()
	root := submitChainedTasks(t, layFeatureBranchGit(t, "true", "true"))

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	ids := make([]string, 0, len(res.Awaiting))
	for _, a := range res.Awaiting {
		ids = append(ids, a.ID)
	}
	require.Equal(t, []string{"T-zulu00", "T-alfa00"}, ids,
		"zulu leads the alfa that records it as a dependency")
}

// TestSyncCrossingsRankSiblingTasksByDependency pins the same rule on the other
// task list a sync run reports, the crossings it just recorded. Every task in
// the set crosses in the one act, so the order is the report's alone; it is the
// order the human reads back, and it is the same rank every other surface shows
// them in.
func TestSyncCrossingsRankSiblingTasksByDependency(t *testing.T) {
	t.Parallel()
	root := submitChainedTasks(t, layActiveFeatureGit(t, "true", "true"))

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	ids := make([]string, 0, len(res.Crossings))
	for _, c := range crossingsOf(res, store.NounTask) {
		ids = append(ids, c.ID)
	}
	require.Equal(t, []string{"T-zulu00", "T-alfa00"}, ids,
		"zulu leads the alfa that records it as a dependency")
}

// submitChainedTasks adds two tasks under the fixture feature and drives both
// to in_review. alfa records that it follows zulu, and alfa leads zulu by both
// canonical key and id, so a list taking the group as the loader held it, or
// falling back to id, reads the pair backwards.
func submitChainedTasks(t *testing.T, root string) string {
	t.Helper()
	base := "form-engine/certification-form-loading/"
	mint := func(taskID string) mutate.Deps {
		d := testDeps()
		seq := 0
		d.Mint = func(prefix string) string {
			if prefix == "T" {
				return taskID
			}
			seq++
			return fmt.Sprintf("%s-ch%s%d", prefix, taskID[2:5], seq)
		}
		return d
	}
	_, err := mutate.AddTask(load(t, root), mint("T-zulu00"), mutate.TaskInput{Key: base + "zulu", Title: "Zulu"})
	require.NoError(t, err)
	_, err = mutate.AddTask(load(t, root), mint("T-alfa00"), mutate.TaskInput{
		Key: base + "alfa", Title: "Alfa", DependsOn: []string{"T-zulu00"}})
	require.NoError(t, err)

	for _, id := range []string{"T-zulu00", "T-alfa00"} {
		_, err := mutate.ClaimTask(load(t, root), testDeps(), id, "")
		require.NoError(t, err)
		stageSyncReview(t, root, id, featBranch)
	}
	setFeatureBranch(t, root, "form-engine", "certification-form-loading", featBranch)
	return root
}

// TestSyncAwaitingExcludesNodesThatJustCrossed confirms the readout reads the
// post-crossing state, so a sync that did its job reports nothing awaiting.
func TestSyncAwaitingExcludesNodesThatJustCrossed(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, crossingsOf(res, store.NounFeature), 1)
	require.Empty(t, res.Awaiting)
}

// TestSyncRollupWithNoObservableBranchRecordsTheRollupBasis pins the one crossing
// that rests on neither kind of merge evidence. The task crossed on a merge, the
// branch was then deleted, and the feature-level gates were checked afterwards, so
// the rollup fires in a run where the feature branch is no longer observable at
// all. It still records a basis, so no reader ever has to interpret an absence.
func TestSyncRollupWithNoObservableBranchRecordsTheRollupBasis(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")

	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{Key: "form-engine/certification-form-loading/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, "T-aaaaaa")
	_, err = mutate.Check(tree, testDeps(), vcs.New(root), task, mutate.CheckInput{})
	require.NoError(t, err)
	stageSyncReview(t, root, "T-aaaaaa", featBranch)
	setFeatureBranch(t, root, "form-engine", "certification-form-loading", featBranch)

	// The first sync crosses the task on ancestry; the feature gates are not
	// checked yet, so the rollup does not fire.
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Crossings, 1)
	require.Equal(t, "ancestry", res.Crossings[0].Basis)
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status)

	// The branch is deleted after the merge, then the feature gates are checked.
	gitRunM(t, root, "branch", "-D", featBranch)
	feat := resolveNode(t, load(t, root), store.NounFeature, "form-engine/certification-form-loading")
	_, err = mutate.Check(load(t, root), testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)

	res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.Equal(t, "rollup", fc[0].Basis, "the tasks carried it, not a branch this run could observe")
	require.Empty(t, fc[0].MatchedCommit)
	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status)
}

// --- the assertion path: sync --accept-fast-forward ---
//
// Everything below exercises the one crossing Tiller does not observe. A
// fast-forward merge and a branch that never diverged leave byte-identical
// repositories (TestSyncFastForwardIsNotObserved proves it with two of them), so
// the node strands with no route to done and no flag on observation can free it.
// --accept-fast-forward widens the BASIS sync will accept, on nodes the human
// names, and never the SET of branches it will cross: the promotion is keyed off
// NoMergeWitness, which observeMerge sets only after `merge-base --is-ancestor`
// answered yes. The three refusal tests below are that safety property stated as
// behavior rather than as a comment.

// laySecondFeature adds a second feature to the already-approved form-engine epic
// under caller-supplied ids, puts a task under it into review, and records the
// branch name on its feature.yml WITHOUT creating the git ref. The caller creates
// whichever branch shape it is exercising, which is the whole point: these tests
// need two features in one repository sitting in different observation buckets.
func laySecondFeature(t *testing.T, root, localKey, featureID, taskID string) string {
	t.Helper()
	seq := 0
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "F" {
			return featureID
		}
		seq++
		return fmt.Sprintf("%s-2nd%02d", prefix, seq)
	}
	key := "form-engine/" + localKey
	_, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: key, Title: "Second thing", ChangeType: "create",
	})
	require.NoError(t, err)

	dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", localKey)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Second thing\n\nReal spec prose, no longer a stub.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte("---\nnode: "+featureID+"\ncriteria:\n  - id: C1\n    statement: The second thing works\n    polarity: introduce\n    binding: { type: test, ref: spec/second_spec.rb }\n---\n\n# Acceptance\n"), 0o644))

	// The task explosion is part of what `spec feature` validates, so it is laid
	// before the gate rather than after the start.
	taskSeq := 0
	taskDeps := testDeps()
	taskDeps.Mint = func(prefix string) string {
		if prefix == "T" {
			return taskID
		}
		taskSeq++
		return fmt.Sprintf("%s-second%02d", prefix, taskSeq)
	}
	_, err = mutate.AddTask(load(t, root), taskDeps, mutate.TaskInput{Key: key + "/do-the-second-thing", Title: "Do the second thing"})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), taskDeps, taskID, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, store.TaskDirName, "do-the-second-thing", store.TaskProseFile),
		[]byte("# Do the second thing\n\n## Approach\n\nDo it in the second place.\n"), 0o644))

	driveTo(t, root, store.NounFeature, key, mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, key, mutate.TransitionInput{Verb: "approve"})

	driveTo(t, root, store.NounTask, taskID, mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounTask, taskID, mutate.TransitionInput{Verb: "start"})

	task := resolveNode(t, load(t, root), store.NounTask, taskID)
	res, cerr := mutate.Check(load(t, root), testDeps(), vcs.New(root), task, mutate.CheckInput{})
	require.NoError(t, cerr)
	if !res.Passed {
		driveTo(t, root, store.NounTask, taskID, mutate.TransitionInput{Verb: "check"})
	}
	stageSyncReview(t, root, taskID, "tiller/sync-"+localKey)

	feat := resolveNode(t, load(t, root), store.NounFeature, key)
	_, err = mutate.Check(load(t, root), testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)

	branch := "tiller/sync-" + localKey
	setFeatureBranch(t, root, "form-engine", localKey, branch)
	return branch
}

// taskYMLPath is one task's task.yml on disk, located through the task's own
// canonical key rather than a guessed folder name.
func taskYMLPath(t *testing.T, root, taskID string) string {
	t.Helper()
	tk := load(t, root).Tasks[taskID]
	require.NotNil(t, tk, "no task %s in the tree", taskID)
	require.NotNil(t, tk.Key, "task %s carries no key", taskID)
	seg := strings.Split(*tk.Key, "/")
	require.Len(t, seg, 3)
	return filepath.Join(root, store.TaskRelDir(seg[0], seg[1], seg[2]), store.TaskStateFile)
}

// taskDoneBasis reads one task's recorded done_basis straight off its task.yml,
// so the durable answer is asserted rather than the in-memory result view.
func taskDoneBasis(t *testing.T, root, taskID string) string {
	t.Helper()
	data, err := os.ReadFile(taskYMLPath(t, root, taskID))
	require.NoError(t, err)
	var tk store.Task
	require.NoError(t, yaml.Unmarshal(data, &tk))
	return tk.DoneBasis
}

// TestSyncAcceptFastForwardCrossesTheContainedBranch is the flag's reason to
// exist. main fast-forwarded onto the feature branch, which is a real merge that
// leaves nothing to witness it, so observation strands the node forever. The human
// names the feature, and the crossing is recorded on their word: basis
// human_attested on the crossing, on the node, and on the event, with no matched
// commit, because nothing was matched.
func TestSyncAcceptFastForwardCrossesTheContainedBranch(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	gitRunM(t, root, "merge", "--ff-only", featBranch)
	submitTask(t, root)

	// Naming a node twice is one assertion, not two.
	declareIntegrationBranch(t, root)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa", "T-aaaaaa"},
	})
	require.NoError(t, err)
	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.Equal(t, "done", fc[0].Status)
	require.Equal(t, "rollup", fc[0].Basis, "the feature remains an arithmetic rollup")
	require.Empty(t, fc[0].MatchedCommit, "nothing was matched; nothing is pointed at")
	require.Equal(t, []string{"T-aaaaaa"}, res.Attested, "the named ids are deduped, in the order named")
	require.Empty(t, res.Awaiting, "nothing is left stranded")

	reloaded := load(t, root)
	require.Equal(t, "done", reloaded.Features["F-aaaaaa"].Status)
	require.Equal(t, "human_attested", reloaded.Tasks["T-aaaaaa"].DoneBasis)

	ev := findEvent(t, reloaded, "sync.task")
	require.Equal(t, "human_attested", ev.After["basis"], "and so does the event stream")
	require.NotContains(t, ev.After, "merge_commit")
	require.Equal(t, "jc", ev.Contributor, "the event's owner is the attester")
}

// TestSyncAcceptFastForwardRefusesWorkTheIntegrationBranchLacks is the safety
// property, and the single most important test in this batch. The flag may widen
// what Tiller accepts as a reason; it may never widen what it will cross. Here the
// branch is genuinely unmerged, main does not contain a line of it, and naming it
// refuses the whole command before any write.
func TestSyncAcceptFastForwardRefusesWorkTheIntegrationBranchLacks(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	submitTask(t, root)

	merged, err := vcs.New(root).IsMergedInto(featBranch, "main")
	require.NoError(t, err)
	require.False(t, merged, "main does not contain this work by any reading")

	declareIntegrationBranch(t, root)
	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa", "T-aaaaaa"},
	})
	require.Error(t, err, "no human assertion crosses work the integration branch does not carry")
	var refused *mutate.AttestNotEligibleError
	require.ErrorAs(t, err, &refused)
	require.Equal(t, "main", refused.Into)
	require.Len(t, refused.Nodes, 1, "one node named twice is one line in the refusal")
	require.Equal(t, mutate.AttestReasonNotContained, refused.Nodes[0].Reason)
	require.Equal(t, "T-aaaaaa", refused.Nodes[0].NodeID)
	require.Equal(t, featBranch, refused.Nodes[0].Branch)

	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status, "a refused sync records nothing")
	require.False(t, hasEvent(t, load(t, root), "sync.feature"))
}

// layOrphanedStateCommitGit is the field case issue #43 exists for, and the
// mirror of issue #42's own trigger (internal/core/review's ForTask): featBranch is
// pushed and its pushed tip merged with --no-ff into main, then the LOCAL
// branch gains one more commit that never reaches the remote, a state commit
// left behind after the PR that carried the earlier tip already merged. It
// returns the repo root and the bare remote's path, with HEAD back on main
// and featBranch not checked out anywhere, so a caller can add a worktree
// (or not) before asserting.
func layOrphanedStateCommitGit(t *testing.T, buildRun, testsRun string) (root, bare string) {
	t.Helper()
	root = layFeatureBranchGit(t, buildRun, testsRun)
	bare = t.TempDir()
	gitRunM(t, bare, "init", "--bare")
	gitRunM(t, root, "remote", "add", "origin", bare)
	gitRunM(t, root, "push", "-u", "origin", featBranch)

	gitRunM(t, root, "merge", "--no-ff", "-m", "merge feature", featBranch)

	gitRunM(t, root, "checkout", featBranch)
	require.NoError(t, os.WriteFile(filepath.Join(root, "state.txt"), []byte("state commit\n"), 0o644))
	gitCommitAllM(t, root, "state commit")
	gitRunM(t, root, "checkout", "main")

	submitTask(t, root)
	declareIntegrationBranch(t, root)
	return root, bare
}

// TestSyncAcceptFastForwardPrescribesRecoveryWhenTheRemoteTipIsContained is the
// honest half of issue #43: the local tip is not contained in main (an
// unpushed state commit sits on top of it), but the tip the PR actually
// carried IS contained, by ancestry, so the refusal has something real to
// prescribe. Nothing holds featBranch checked out anywhere, so the
// remediation names the plain `git branch -f` form.
func TestSyncAcceptFastForwardPrescribesRecoveryWhenTheRemoteTipIsContained(t *testing.T) {
	t.Parallel()
	root, _ := layOrphanedStateCommitGit(t, "true", "true")
	g := vcs.New(root)
	remoteTip, err := g.ShortSHA("origin/" + featBranch)
	require.NoError(t, err)
	localTip, err := g.ShortSHA(featBranch)
	require.NoError(t, err)

	_, err = mutate.Sync(load(t, root), syncDeps(), g, mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa"},
	})
	require.Error(t, err, "an unpushed state commit is still an unmerged local tip")
	var refused *mutate.AttestNotEligibleError
	require.ErrorAs(t, err, &refused)
	require.Len(t, refused.Nodes, 1)
	node := refused.Nodes[0]
	require.Equal(t, mutate.AttestReasonNotContained, node.Reason)
	require.NotNil(t, node.Recovery, "the pushed tip main already contains is a real remedy to prescribe")
	require.Equal(t, remoteTip, node.Recovery.RemoteTip)
	require.Equal(t, localTip, node.Recovery.LocalTip)
	require.Equal(t, "origin/"+featBranch, node.Recovery.Upstream)
	require.Empty(t, node.Recovery.Worktree, "nothing holds featBranch checked out in this fixture")
}

// TestSyncAcceptFastForwardRecoveryNamesTheWorktreeThatHoldsTheBranch is the
// other arm: when a worktree holds featBranch checked out, `git branch -f`
// would itself refuse, so the recovery has to name the worktree instead.
func TestSyncAcceptFastForwardRecoveryNamesTheWorktreeThatHoldsTheBranch(t *testing.T) {
	t.Parallel()
	root, _ := layOrphanedStateCommitGit(t, "true", "true")
	wtDir := t.TempDir()
	gitRunM(t, root, "worktree", "add", wtDir, featBranch)
	wantWorktree, err := filepath.EvalSymlinks(wtDir)
	require.NoError(t, err)

	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa"},
	})
	require.Error(t, err)
	var refused *mutate.AttestNotEligibleError
	require.ErrorAs(t, err, &refused)
	require.Len(t, refused.Nodes, 1)
	require.NotNil(t, refused.Nodes[0].Recovery)
	gotWorktree, err := filepath.EvalSymlinks(refused.Nodes[0].Recovery.Worktree)
	require.NoError(t, err)
	require.Equal(t, wantWorktree, gotWorktree)
}

// TestSyncAcceptFastForwardRecoveryStaysNilWhenNothingIsMergedEitherWay is the
// contract's own safety valve: featBranch is pushed, so an upstream resolves,
// but neither tip has been merged into main by any reading. Inventing a
// remedy here would be worse than the bare refusal, so Recovery stays nil and
// the refusal carries nothing beyond what it always has.
func TestSyncAcceptFastForwardRecoveryStaysNilWhenNothingIsMergedEitherWay(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	bare := t.TempDir()
	gitRunM(t, bare, "init", "--bare")
	gitRunM(t, root, "remote", "add", "origin", bare)
	gitRunM(t, root, "push", "-u", "origin", featBranch)
	submitTask(t, root)
	declareIntegrationBranch(t, root)

	merged, err := vcs.New(root).IsMergedInto("origin/"+featBranch, "main")
	require.NoError(t, err)
	require.False(t, merged, "the pushed tip was never merged either")

	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa"},
	})
	require.Error(t, err)
	var refused *mutate.AttestNotEligibleError
	require.ErrorAs(t, err, &refused)
	require.Len(t, refused.Nodes, 1)
	require.Equal(t, mutate.AttestReasonNotContained, refused.Nodes[0].Reason)
	require.Nil(t, refused.Nodes[0].Recovery, "no honest remedy exists when neither tip landed on main")
}

// TestSyncAcceptFastForwardRefusesAnIntoThatIsNotTheDeclaredBranch is the other
// half of the safety property, and it is the half that was missing. "The
// integration branch already contains this work" is a claim about two refs, and
// attestContained is airtight about only one of them. Every spelling below leaves
// the work sitting on an unmerged branch and main carrying none of it, and each
// one used to record done:
//
//   - an alternate spelling of the feature's own branch, where containment is the
//     trivial "a branch contains itself" and the name-based self-merge guard
//     cannot reduce the form typed;
//   - a release cut taken off the feature branch, which is a genuinely different
//     branch that genuinely contains the work, and which no amount of name
//     canonicalization could ever catch.
//
// The rule that closes both is stated positively: the assertion is measured
// against the declared integration branch or it is refused.
func TestSyncAcceptFastForwardRefusesAnIntoThatIsNotTheDeclaredBranch(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		into string
		lay  func(t *testing.T, root string)
	}{
		{name: "feature branch under a spelling canonicalBranch cannot reduce", into: "heads/" + featBranch},
		{name: "feature branch by rev expression", into: featBranch + "^0"},
		{name: "a release cut taken off the feature branch", into: "release-cut",
			lay: func(t *testing.T, root string) { gitRunM(t, root, "branch", "release-cut", featBranch) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := layFeatureBranchGit(t, "true", "true")
			submitTask(t, root)
			declareIntegrationBranch(t, root)
			if tc.lay != nil {
				tc.lay(t, root)
			}

			merged, err := vcs.New(root).IsMergedInto(featBranch, "main")
			require.NoError(t, err)
			require.False(t, merged, "main carries none of this work in any of these cases")

			_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
				Into:              tc.into,
				AcceptFastForward: []string{"T-aaaaaa"},
			})
			require.Error(t, err, "an assertion measured against anything but the declared trunk is refused")
			var refused *mutate.AttestIntoNotDeclaredError
			require.ErrorAs(t, err, &refused)
			require.Equal(t, "main", refused.Declared)
			require.Equal(t, tc.into, refused.Into)
			require.Equal(t, "--into", refused.Source)

			reloaded := load(t, root)
			require.Equal(t, "in_review", reloaded.Tasks["T-aaaaaa"].Status, "nothing was recorded")
			require.False(t, hasEvent(t, reloaded, "sync.task"))
		})
	}
}

// TestSyncAcceptFastForwardNeedsADeclaredIntegrationBranch is the case name
// canonicalization cannot reach at all. A project that declares no
// git.integration_branch resolves into from the branch the checkout is standing
// on, and that branch can be a scratch branch cut off the feature itself, which
// contains the work for the most trivial reason there is. There is no ref to
// compare against, so the flag refuses rather than assert about a trunk nobody
// named. Plain observation is untouched and still walks the HEAD fallback.
func TestSyncAcceptFastForwardNeedsADeclaredIntegrationBranch(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	submitTask(t, root)
	gitRunM(t, root, "checkout", "-b", "scratch", featBranch)

	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa"},
	})
	require.Error(t, err)
	var refused *mutate.AttestIntoNotDeclaredError
	require.ErrorAs(t, err, &refused)
	require.Empty(t, refused.Declared, "there is nothing declared, which is the whole refusal")
	require.Equal(t, "scratch", refused.Into)
	require.Equal(t, "HEAD", refused.Source)
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)

	// The same run without the flag is unchanged: it still observes, still walks
	// the HEAD fallback, and still names the stranded node.
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings)
	require.Equal(t, "scratch", res.Into)
	require.Equal(t, "HEAD", res.IntoSource)
}

// TestSyncAsksAboutTheBranchNotATagOfTheSameName covers git's rev-parse
// precedence, which resolves a bare name from refs/tags before refs/heads. A tag
// sharing the feature branch's name answered every question observeMerge asked,
// so an entirely unmerged branch read as contained in main with no witness, which
// is the one observation an assertion may act on. git warns about the ambiguity on
// stderr and Tiller never sees it.
//
// Both halves matter. The observation itself must stop reporting the node as "in
// main already", because that readout is what hands the human the assertion
// command; and the flag must refuse it.
func TestSyncAsksAboutTheBranchNotATagOfTheSameName(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	submitTask(t, root)
	declareIntegrationBranch(t, root)
	gitRunM(t, root, "tag", featBranch, "main")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings)
	require.Len(t, res.Awaiting, 1)
	require.False(t, res.Awaiting[0].NoMergeWitness,
		"the branch is not contained in main; only a tag of the same name is")
	require.False(t, res.Awaiting[0].BranchMissing, "the branch itself resolves perfectly well")

	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa"},
	})
	require.Error(t, err)
	var refused *mutate.AttestNotEligibleError
	require.ErrorAs(t, err, &refused)
	require.Equal(t, mutate.AttestReasonNotContained, refused.Nodes[0].Reason)
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
}

// TestSyncAcceptFastForwardRefusesAMissingBranch pins the bucket this work
// deliberately leaves alone. A branch deleted after its merge reads as "branch
// missing locally", not as containment, so it is not in the candidate set and the
// flag refuses it. Recovering that case would rest on the recorded review_ref.head
// and is a separate, evidence-based change.
func TestSyncAcceptFastForwardRefusesAMissingBranch(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	gitRunM(t, root, "merge", "--ff-only", featBranch)
	submitTask(t, root)
	gitRunM(t, root, "branch", "-D", featBranch)

	declareIntegrationBranch(t, root)
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa"},
	})
	require.Error(t, err)
	var refused *mutate.AttestNotEligibleError
	require.ErrorAs(t, err, &refused)
	require.Len(t, refused.Nodes, 1)
	require.Equal(t, mutate.AttestReasonBranchMissing, refused.Nodes[0].Reason)

	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
	require.False(t, hasEvent(t, load(t, root), "sync.feature"))
}

// TestSyncAcceptFastForwardCrossesTheVacuousBranchToo pins an accepted cost so
// nobody quietly "fixes" it later. A branch that carries no commits of its own is
// in the same contained-but-unwitnessed set as a fast-forward merge, and git holds
// nothing that separates them, so the flag can cross it. That is the price of the
// assertion, and it is paid by the human who named the node.
func TestSyncAcceptFastForwardCrossesTheVacuousBranchToo(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	gitRunM(t, root, "branch", "-f", featBranch, "main")
	submitTask(t, root)

	declareIntegrationBranch(t, root)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa"},
	})
	require.NoError(t, err)
	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1, "the human asserted it, and carries that")
	require.Equal(t, "rollup", fc[0].Basis)
	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status)
}

// TestSyncAcceptFastForwardCrossesOnlyNamedNodes is why the flag takes node
// references instead of sweeping the candidate set. Two features are stranded in
// exactly the same way and git cannot tell them apart; the human merged one of
// them. Naming is what makes the human, not the flag, the thing that selects.
func TestSyncAcceptFastForwardCrossesOnlyNamedNodes(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	gitRunM(t, root, "merge", "--ff-only", featBranch)
	submitTask(t, root)

	other := laySecondFeature(t, root, "second-thing", "F-bbbbbb", "T-bbbbbb")
	gitRunM(t, root, "branch", "-f", other, "main")

	declareIntegrationBranch(t, root)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa"},
	})
	require.NoError(t, err)
	// The named feature's task crosses on the assertion and the feature rolls up
	// behind it; nothing under the unnamed feature moves at all.
	require.Len(t, res.Crossings, 2, "only the named node's subtree crosses")
	require.Equal(t, []string{"T-aaaaaa", "F-aaaaaa"}, crossedIDs(res))

	reloaded := load(t, root)
	require.Equal(t, "done", reloaded.Features["F-aaaaaa"].Status)
	require.Equal(t, "active", reloaded.Features["F-bbbbbb"].Status)
	require.Equal(t, "in_review", reloaded.Tasks["T-bbbbbb"].Status,
		"the unnamed node is still the human's to answer for")
	require.Len(t, res.Awaiting, 1)
	require.Equal(t, "T-bbbbbb", res.Awaiting[0].ID)
	require.True(t, res.Awaiting[0].NoMergeWitness, "and it is still reported as the stranded case")
}

// TestSyncAcceptFastForwardRefusesBeforeAnyWrite confirms the refusal is
// all-or-nothing and runs ahead of every write, the same posture as the
// integration-branch guard. One eligible node and one that is not: nothing
// crosses, including the eligible one, and no event is appended.
func TestSyncAcceptFastForwardRefusesBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")

	// Cut the second, genuinely unmerged branch while the working tree is still
	// clean, so committing its work does not sweep up either feature's .tiller
	// state.
	gitRunM(t, root, "checkout", "-b", "tiller/sync-second-thing")
	require.NoError(t, os.WriteFile(filepath.Join(root, "second.txt"), []byte("unmerged\n"), 0o644))
	gitCommitAllM(t, root, "second feature work")
	gitRunM(t, root, "checkout", "main")

	gitRunM(t, root, "merge", "--ff-only", featBranch)
	submitTask(t, root)
	other := laySecondFeature(t, root, "second-thing", "F-bbbbbb", "T-bbbbbb")

	merged, err := vcs.New(root).IsMergedInto(other, "main")
	require.NoError(t, err)
	require.False(t, merged, "the second branch carries work main has never seen")

	declareIntegrationBranch(t, root)
	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa", "T-bbbbbb"},
	})
	require.Error(t, err)
	var refused *mutate.AttestNotEligibleError
	require.ErrorAs(t, err, &refused)
	require.Len(t, refused.Nodes, 1, "the eligible node is not listed; the ineligible one is")
	require.Equal(t, "T-bbbbbb", refused.Nodes[0].NodeID)
	require.Equal(t, mutate.AttestReasonNotContained, refused.Nodes[0].Reason)

	reloaded := load(t, root)
	require.Equal(t, "in_review", reloaded.Tasks["T-aaaaaa"].Status, "nothing was recorded, not even the eligible half")
	require.Equal(t, "in_review", reloaded.Tasks["T-bbbbbb"].Status)
	require.False(t, hasEvent(t, reloaded, "sync.task"))
	require.False(t, hasEvent(t, reloaded, "sync.feature"))
}

// TestSyncAcceptFastForwardRefusesANodeWithNothingToCross covers the third
// ineligibility. The branch is contained and unwitnessed, so the node is in the
// candidate set by observation, but the feature is still active with no in_review
// task under it: there is no crossing for the assertion to be about, and saying so
// is better than accepting an assertion that would do nothing.
func TestSyncAcceptFastForwardRefusesANodeWithNothingToCross(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	gitRunM(t, root, "merge", "--ff-only", featBranch)
	setFeatureBranch(t, root, "form-engine", "certification-form-loading", featBranch)
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status)

	declareIntegrationBranch(t, root)
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"F-aaaaaa"},
	})
	require.Error(t, err)
	var refused *mutate.AttestNotEligibleError
	require.ErrorAs(t, err, &refused)
	require.Len(t, refused.Nodes, 1)
	require.Equal(t, mutate.AttestReasonNothingToCross, refused.Nodes[0].Reason)
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status)
}

// TestSyncAcceptFastForwardCarriesTasksToDone runs the assertion at task PR grain,
// where one observation moves several nodes. The human names the feature; its
// in_review tasks cross on human_attested, their ledger entries record it, and the
// parent feature rolls up on the same basis rather than on rollup, because what
// carried it was the assertion and the record should say so.
func TestSyncAcceptFastForwardCarriesTasksToDone(t *testing.T) {
	t.Parallel()
	root := layTaskGrainGit(t)
	gitRunM(t, root, "merge", "--ff-only", featBranch)

	feat := resolveNode(t, load(t, root), store.NounFeature, "form-engine/certification-form-loading")
	_, err := mutate.Check(load(t, root), testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)

	declareIntegrationBranch(t, root)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa", "T-aaaaab"},
	})
	require.NoError(t, err)
	asserted := crossingsOf(res, store.NounTask)
	require.Len(t, asserted, 2, "both explicitly named tasks cross")
	for _, c := range asserted {
		require.Equal(t, "human_attested", c.Basis, "the assertion carried every one of them")
	}

	reloaded := load(t, root)
	for _, id := range []string{"T-aaaaaa", "T-aaaaab"} {
		require.Equal(t, "done", reloaded.Tasks[id].Status)
		require.Equal(t, "human_attested", taskDoneBasis(t, root, id),
			"a task carries the field in its ledger entry on the same terms")
	}
	require.Equal(t, "done", reloaded.Features["F-aaaaaa"].Status)
	require.Equal(t, "rollup", reloaded.Features["F-aaaaaa"].DoneBasis)
}

// TestSyncAcceptFastForwardNeverDowngradesObservedEvidence pins the rule that an
// assertion never replaces evidence Tiller already has. Naming a genuinely
// merge-committed feature is harmless: it crosses on ancestry, and the record says
// ancestry. The human's word is written only where nothing else can be.
func TestSyncAcceptFastForwardNeverDowngradesObservedEvidence(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)

	declareIntegrationBranch(t, root)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa"},
	})
	require.NoError(t, err)
	tc := crossingsOf(res, store.NounTask)
	require.Len(t, tc, 1)
	require.Equal(t, "ancestry", tc[0].Basis, "a merge commit witnesses this; no assertion is needed")
	require.Equal(t, "ancestry", taskDoneBasis(t, root, "T-aaaaaa"))

	// The feature above it records rollup, not human_attested: the assertion was
	// never what carried it, so nothing about this crossing rests on the human's
	// word and the record must not say it does.
	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.Equal(t, "rollup", fc[0].Basis)
	require.Equal(t, "rollup", load(t, root).Features["F-aaaaaa"].DoneBasis)

	require.Equal(t, []string{"T-aaaaaa"}, res.Attested,
		"what was asserted is still recorded, even where the observation answered")
}

// TestSyncAcceptFastForwardDoesNotDowngradeASquash is the same rule against the
// weaker observation. Content evidence is still evidence, and it outranks a word.
func TestSyncAcceptFastForwardDoesNotDowngradeASquash(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	squash := squashMergeFeature(t, root)
	submitTask(t, root)

	declareIntegrationBranch(t, root)
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"T-aaaaaa"},
	})
	require.NoError(t, err)
	tc := crossingsOf(res, store.NounTask)
	require.Len(t, tc, 1)
	require.Equal(t, "content", tc[0].Basis)
	require.Equal(t, "git:"+squash, tc[0].MatchedCommit)
	require.Equal(t, "content", taskDoneBasis(t, root, "T-aaaaaa"))

	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.Equal(t, "rollup", fc[0].Basis, "the content observation answered, so no word was needed")
	require.Equal(t, "rollup", load(t, root).Features["F-aaaaaa"].DoneBasis)
}

// TestSyncAcceptFastForwardRejectsAnUnknownFeature is the core's own guard on its
// input. The CLI resolves a reference to a feature id before calling, so this is
// unreachable in practice; it is here so a caller that skips that step gets a
// misuse rather than a silently ignored assertion.
func TestSyncAcceptFastForwardRejectsAnUnknownFeature(t *testing.T) {
	t.Parallel()
	root := layFeatureBranchGit(t, "true", "true")
	gitRunM(t, root, "merge", "--ff-only", featBranch)
	submitTask(t, root)

	declareIntegrationBranch(t, root)
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{
		AcceptFastForward: []string{"F-zzzzzz"},
	})
	require.Error(t, err)
	var coded interface{ ExitCode() int }
	require.ErrorAs(t, err, &coded)
	require.Equal(t, 2, coded.ExitCode(), "naming a node that does not exist is misuse")
	require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
}

// TestSyncWithoutTheFlagIsUnchanged is the regression fence around every guarantee
// the previous batch bought. With the flag absent, and with an empty flag, the
// fast-forward fixture crosses nothing, stays in review, and still reports itself
// as the contained-but-unwitnessed case.
func TestSyncWithoutTheFlagIsUnchanged(t *testing.T) {
	t.Parallel()
	for _, named := range [][]string{nil, {}} {
		root := layFeatureBranchGit(t, "true", "true")
		gitRunM(t, root, "merge", "--ff-only", featBranch)
		submitTask(t, root)

		res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{AcceptFastForward: named})
		require.NoError(t, err)
		require.Empty(t, res.Crossings, "without the assertion there is nothing to cross on")
		require.Empty(t, res.Attested)
		require.Equal(t, "in_review", load(t, root).Tasks["T-aaaaaa"].Status)
		require.Len(t, res.Awaiting, 1)
		require.True(t, res.Awaiting[0].NoMergeWitness)
		require.False(t, res.Awaiting[0].BranchMissing)
	}
}

// TestSyncRecordsTheDoneBasisOnEveryCrossing pins the basis enum PER GRAIN, which
// is where the two grains part company. Every crossing sync records writes its
// basis to done_basis, so an auditor can always tell an asserted crossing from an
// observed one without inferring anything from a missing field. What each grain
// is allowed to say differs: a task carries ancestry, content, or human_attested,
// because a PR represents a task and a task's merge is the thing observed; a
// feature carries only rollup or human_attested, because no merge is ever
// observed at feature grain (artifact-schema.md, Done basis).
//
// The feature arms are the load-bearing half. Writing the observation basis onto
// a feature would claim provenance the model no longer permits, in a committed
// file and an immutable event, on the ordinary arc rather than an edge case.
func TestSyncRecordsTheDoneBasisOnEveryCrossing(t *testing.T) {
	t.Parallel()
	// ancestry: a --no-ff merge commit, on the task.
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "ancestry", taskDoneBasis(t, root, "T-aaaaaa"))
	require.Equal(t, "rollup", load(t, root).Features["F-aaaaaa"].DoneBasis,
		"the feature crossed because its tasks did, not because a merge was observed for it")

	// content: a squash merge, again on the task.
	root = layFeatureBranchGit(t, "true", "true")
	squashMergeFeature(t, root)
	submitTask(t, root)
	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "content", taskDoneBasis(t, root, "T-aaaaaa"))
	require.Equal(t, "rollup", load(t, root).Features["F-aaaaaa"].DoneBasis,
		"a squash is still a task merge; the feature is still arithmetic")

	// rollup, with the branch gone entirely: the arm that shows the feature basis
	// is not merely inherited from an observation that happened to be present.
	root = layActiveFeatureGit(t, "true", "true")
	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{Key: "form-engine/certification-form-loading/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	task := resolveNode(t, load(t, root), store.NounTask, "T-aaaaaa")
	_, err = mutate.Check(load(t, root), testDeps(), vcs.New(root), task, mutate.CheckInput{})
	require.NoError(t, err)
	stageSyncReview(t, root, "T-aaaaaa", featBranch)
	setFeatureBranch(t, root, "form-engine", "certification-form-loading", featBranch)

	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "ancestry", taskDoneBasis(t, root, "T-aaaaaa"),
		"the task crossed on the merge the branch witnessed")

	gitRunM(t, root, "branch", "-D", featBranch)
	feat := resolveNode(t, load(t, root), store.NounFeature, "form-engine/certification-form-loading")
	_, err = mutate.Check(load(t, root), testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)
	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "rollup", load(t, root).Features["F-aaaaaa"].DoneBasis)
}

// layEpicRollupGit lays the one-epic-one-feature-one-task tree with the feature
// branch merged, the task in review, and the feature-grain gates checked: the
// state where one sync crosses the task, rolls the feature up, and (once the
// epic rollup lands) rolls the epic up behind it. It is TestSyncTaskGrainRollup's
// own setup, lifted so the epic rollup test drives the identical shape rather
// than a second hand-built approximation of it.
func layEpicRollupGit(t *testing.T) string {
	t.Helper()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)
	return root
}

// addDraftFeature adds one draft feature under the epic and leaves it there: the
// mechanical "new work landed beneath a done node" the reopen story rests on
// (artifact-schema.md, Epics States).
func addDraftFeature(t *testing.T, root, localKey, featureID string) {
	t.Helper()
	seq := 0
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "F" {
			return featureID
		}
		seq++
		return fmt.Sprintf("%s-late%02d", prefix, seq)
	}
	_, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/" + localKey, Title: "Late arrival", ChangeType: "create",
	})
	require.NoError(t, err)
}

// TestSyncRecordsEpicDoneWhenLastFeatureCrossesAndReopensOnNewWork is the epic
// rollup: epic done is a computed fact sync records, not a gate a human crosses
// (artifact-schema.md, Epics States: "The done crossing is not a gate at all").
// One epic, one feature, all its tasks done: sync's feature rollup already
// records the feature; the epic crossing is the new behavior under test.
func TestSyncRecordsEpicDoneWhenLastFeatureCrossesAndReopensOnNewWork(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	e := load(t, root).Epics["E-aaaaaa"]
	require.Equal(t, "done", e.Status)
	require.Equal(t, "rollup", e.DoneBasis)
	require.NotEmpty(t, e.VerifiedAgainst.Spec.Value,
		"epic completion record: the epic.md blob")
	require.Equal(t, store.KindGitBlob, e.VerifiedAgainst.Spec.Kind)
	require.Equal(t, store.KindGitCommit, e.VerifiedAgainst.Commit.Kind)
	require.NotEmpty(t, e.VerifiedAgainst.Commit.Value, "the completion record pins the integration commit so stale_spec can say since when")
	require.NotNil(t, e.VerifiedAgainst.Surfaces,
		"epic completion record: surfaces, the empty list for no surface")
	require.Zero(t, e.VerifiedAgainst.Eval,
		"an epic has no eval contract of its own; its bar is the sum of its features'")

	var epicCrossing bool
	for _, c := range res.Crossings {
		if c.Noun == store.NounEpic {
			epicCrossing = true
			require.Equal(t, "E-aaaaaa", c.ID)
			require.Equal(t, "done", c.Status)
			require.Equal(t, "rollup", c.Basis)
		}
	}
	require.True(t, epicCrossing, "the epic rollup is reported as a crossing")
	require.True(t, hasEvent(t, load(t, root), "sync.epic"), "the crossing writes an event")

	// Reopen is mechanical: a draft feature added under the done epic returns it
	// to ready on the next sync. artifact-schema.md, Epics, States.
	addDraftFeature(t, root, "late-arrival", "F-bbbbbb")
	res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	reopened := load(t, root).Epics["E-aaaaaa"]
	require.Equal(t, "ready", reopened.Status, "new work beneath a done epic reopens it")
	require.Empty(t, reopened.DoneBasis, "a reopened epic carries no done basis")
	require.Zero(t, reopened.VerifiedAgainst.Spec,
		"a reopened epic carries no completion record")
	require.Len(t, res.Reopened, 1, "the reverse is reported")
	require.Equal(t, "E-aaaaaa", res.Reopened[0].ID)
}

// TestSyncEpicRollupIgnoresCancelledFeatures pins how the epic rollup treats a
// cancelled sibling: it neither counts toward the crossing nor blocks it, the
// same rule the feature rollup applies to a cancelled task, so cancelling work
// under an epic never wedges the epic (artifact-schema.md, Epics States).
func TestSyncEpicRollupIgnoresCancelledFeatures(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	addDraftFeature(t, root, "abandoned", "F-bbbbbb")

	// While the sibling is merely draft it holds the epic back, even though the
	// first feature crosses in this same run.
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status)
	require.Equal(t, "ready", load(t, root).Epics["E-aaaaaa"].Status,
		"a live sibling feature holds the rollup")

	// Cancelled, it stops counting and the epic rolls up on the next sync.
	setFeatureStatusOnDisk(t, root, "form-engine", "abandoned", "cancelled")
	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "done", load(t, root).Epics["E-aaaaaa"].Status)
}

// TestSyncEpicWithNoFeaturesNeverRollsUp pins the zero-feature edge. The spec
// requires at least one non-cancelled feature for the crossing, so a ready epic
// nobody has decomposed yet is waiting on features rather than vacuously done
// (artifact-schema.md, Epics States).
func TestSyncEpicWithNoFeaturesNeverRollsUp(t *testing.T) {
	t.Parallel()
	root := writeGatedRepo(t, "true", "true")
	gitInitM(t, root)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	gitCommitAllM(t, root, "lay tree")
	gitRunM(t, root, "branch", "-M", "main")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Crossings, "an epic with no features has nothing to have completed")
	require.Equal(t, "ready", load(t, root).Epics["E-aaaaaa"].Status)
}

// TestSyncNeverUnarchivesAnEpic is the guard on the recompute's one dangerous
// edge. archive is the human's deliberate close of a done epic
// (artifact-schema.md, Epics States), so work landing under an archived epic
// must not quietly overturn that decision the way it reopens a merely-done one.
// The rollup owns ready and done; archived is the human's.
func TestSyncNeverUnarchivesAnEpic(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "done", load(t, root).Epics["E-aaaaaa"].Status)

	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "archive"})
	require.Equal(t, "archived", load(t, root).Epics["E-aaaaaa"].Status)

	addDraftFeature(t, root, "late-arrival", "F-bbbbbb")
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Reopened, "an archived epic is the human's decision, not the rollup's")
	require.Equal(t, "archived", load(t, root).Epics["E-aaaaaa"].Status)
}

// TestSyncReopensADoneFeatureThatGainedATask is the feature-grain half of the
// same recompute: a task added under a done feature returns it to active on the
// next sync (artifact-schema.md, Features States), and the epic above it follows
// in the same run. Its completion record clears with the status, because the
// record describes a done the feature no longer is.
func TestSyncReopensADoneFeatureThatGainedATask(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status)
	require.NotEmpty(t, load(t, root).Features["F-aaaaaa"].VerifiedAgainst.Spec.Value)

	seq := 0
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "T" {
			return "T-bbbbbb"
		}
		seq++
		return fmt.Sprintf("%s-late%02d", prefix, seq)
	}
	_, err = mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key:   "form-engine/certification-form-loading/one-more-thing",
		Title: "One more thing",
	})
	require.NoError(t, err)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Reopened, 2, "the feature reopens and the epic above it follows")

	reloaded := load(t, root)
	f := reloaded.Features["F-aaaaaa"]
	require.Equal(t, "active", f.Status)
	require.Empty(t, f.DoneBasis, "a reopened feature carries no done basis")
	require.Zero(t, f.VerifiedAgainst.Spec, "a reopened feature carries no completion record")
	require.Equal(t, "ready", reloaded.Epics["E-aaaaaa"].Status)
}

// setFeatureReviewSurfaceOnDisk hand-lays a feature-grain review surface onto
// feature.yml: review_grain, pr, the cheap way (rgSetFeatureStatus's own
// idiom, submit_node_test.go) to stage the coarse-grain state a real
// SubmitNode round would otherwise take a whole gated epic to reach, since
// reopenFeature's own clearing rule is
// independent of how the surface got recorded.
func setFeatureReviewSurfaceOnDisk(t *testing.T, root, epicKey, featKey, grain string, pr int) {
	t.Helper()
	p := filepath.Join(root, ".tiller", "epics", epicKey, "features", featKey, "feature.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.ReviewGrain = grain
	doc.PR = &pr
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, out, 0o644))
}

// setEpicReviewSurfaceOnDisk is setFeatureReviewSurfaceOnDisk's epic-grain
// twin: review_grain, branch, and pr hand-laid onto epic.yml, staging the
// "epic-grain epic already submitted its own PR" state directly.
func setEpicReviewSurfaceOnDisk(t *testing.T, root, epicKey, branch string, pr int) {
	t.Helper()
	p := filepath.Join(root, ".tiller", "epics", epicKey, "epic.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	var doc store.Epic
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.ReviewGrain = "epic"
	doc.Branch = branch
	doc.PR = &pr
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, out, 0o644))
}

// TestSyncReopenClearsTheConsumedReviewSurfaceOnAFeature is final-review fix
// round item 2, the controller's ruling: a merge consumes the review
// surface it represents, so reopenFeature must clear PR and ReviewRef (the
// events keep the full history) while leaving Branch, the deterministic
// workbench name, alone. Before this fix all three fields survived a
// reopen untouched, which left three things silently wrong: queueMergeFeatures
// kept naming a "tiller sync" for a PR that already merged and closed out a
// round of work, the coarse finish line never fired again for the new
// round (hasRecordedReview still read true), and Configure's own
// Changeability guard kept refusing `configure feature --review-grain` on
// a node whose review window had, in fact, reopened.
func TestSyncReopenClearsTheConsumedReviewSurfaceOnAFeature(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	first := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "done", first.Status)
	require.NotEmpty(t, first.Branch, "the feature's own workbench branch is already on record")

	// Stage the feature-grain submit this fixture's own task-grain flow never
	// runs: a recorded PR, as if `submit feature` had closed out this round.
	setFeatureReviewSurfaceOnDisk(t, root, "form-engine", "certification-form-loading", "feature", 99)

	// A new task lands under the done feature: the mechanical "new work
	// landed" trigger reopenFeature responds to (TestSyncReopensADoneFeatureThatGainedATask).
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "T" {
			return "T-bbbbbb"
		}
		return fixedMint(prefix)
	}
	_, err = mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key: "form-engine/certification-form-loading/one-more-thing", Title: "One more thing",
	})
	require.NoError(t, err)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Reopened, 2, "the feature reopens and the epic above it follows")

	reopened := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "active", reopened.Status)
	require.Nil(t, reopened.PR, "reopen consumes the merged review surface: pr clears on disk")
	require.Nil(t, reopened.ReviewRef, "and so does the review_ref fallback")
	require.Equal(t, featBranch, reopened.Branch,
		"Branch survives: the deterministic workbench name is not a review record")

	// The changeability lock releases: configure feature --review-grain now
	// succeeds instead of refusing with ReviewGrainLockedError, because the
	// node's own review surface no longer exists on disk.
	_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "form-engine/certification-form-loading", ReviewGrain: "feature"})
	require.NoError(t, err, "the reopen must have cleared the review surface Configure's guard locks on")
}

// TestSyncReopenClearsTheConsumedReviewSurfaceOnAnEpic mirrors the feature
// test above one grain up: reopenEpic clears PR and ReviewRef on the same
// ruling, and Branch, the epic-grain workbench every feature under an
// epic-grained epic shares, survives untouched.
func TestSyncReopenClearsTheConsumedReviewSurfaceOnAnEpic(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "done", load(t, root).Epics["E-aaaaaa"].Status)

	// Stage the epic-grain submit this fixture's own task-grain flow never
	// runs: a recorded branch and PR, as if `submit epic` had closed out
	// this round.
	setEpicReviewSurfaceOnDisk(t, root, "form-engine", "tiller/form-engine", 77)

	addDraftFeature(t, root, "late-arrival", "F-bbbbbb")
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Reopened, 1)

	reopened := load(t, root).Epics["E-aaaaaa"]
	require.Equal(t, "ready", reopened.Status)
	require.Nil(t, reopened.PR, "reopen consumes the merged review surface: pr clears on disk")
	require.Nil(t, reopened.ReviewRef, "and so does the review_ref fallback")
	require.Equal(t, "tiller/form-engine", reopened.Branch,
		"Branch survives: the epic workbench every feature under it shares is not a review record")

	deps := testDeps()
	_, err = mutate.Configure(load(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounEpic, Key: "form-engine", ReviewGrain: "epic"})
	require.NoError(t, err, "the reopen must have cleared the review surface Configure's guard locks on")
}

// TestSyncNeverCrossesADraftEpic is a safety property, not a formality.
// approve is the epic's one remaining human gate after the accept verb left, so
// the `ready` guard on the crossing arm is the only thing standing between a
// never-approved decomposition and a `done` epic. The failure it forbids is
// concrete: an agent lands and merges the feature work before the human has
// approved the tree, and sync carries the epic draft -> done, reporting a body
// of work complete that never passed a human's eye at all.
//
// The status is asserted explicitly rather than by the absence of a crossing,
// so a refactor that drops the guard fails here whatever else it changes about
// the result shape.
func TestSyncNeverCrossesADraftEpic(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	setEpicStatusOnDisk(t, root, "form-engine", "ready", "draft")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	// The work beneath it really is done: the feature crossed in this very run,
	// so the rollup arithmetic is satisfied and only the gate is holding.
	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status)
	require.Equal(t, "draft", load(t, root).Epics["E-aaaaaa"].Status,
		"an unapproved epic must not cross; approve is its one human gate")
	require.Empty(t, crossingsOf(res, store.NounEpic))
	require.False(t, hasEvent(t, load(t, root), "sync.epic"))
}

// setEpicStatusOnDisk rewrites an epic.yml's status field, the cheap way to
// stage an epic in a state the lifecycle verbs cannot legally return it to.
func setEpicStatusOnDisk(t *testing.T, root, epicKey, from, to string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "epic.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: "+from, "status: "+to, 1)
	require.NotEqualf(t, string(data), out, "expected a status: %s line to rewrite", from)
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// TestSyncReopenStalesTheGatesSoTheSecondCrossingIsNotSilentlyClean walks the
// whole reopen cycle, which is the only way to see the defect it pins: cross,
// add work, reopen, complete the new work, and re-cross with no `check` in
// between. Without staling, the second crossing reads the FIRST run's gate
// results, which are still green against a revision that predates the new work,
// so recordPolicyViolations finds nothing to say and a done is recorded clean on
// evidence that cannot speak to what it certifies.
//
// A reopen is the same event `resume` is (work is live again under a node whose
// gates described it at rest), so it takes the same treatment through the same
// function, and the assertions below are about that shared rule: gates stale on
// the reopen, and the re-crossing loud rather than silent.
func TestSyncReopenStalesTheGatesSoTheSecondCrossingIsNotSilentlyClean(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)

	// First crossing: the task merges, the feature rolls up on green gates.
	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	first := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "done", first.Status)
	require.Equal(t, store.GateStatusPass, first.Gates["build"].Status, "the first crossing was genuinely green")
	require.Empty(t, first.PolicyViolations, "a clean first crossing records nothing")

	// New work lands beneath the done feature, so it reopens.
	addLateTask(t, root, "T-bbbbbb", "one-more-thing", "One more thing")
	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	reopened := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "active", reopened.Status)
	require.NotEmpty(t, reopened.Gates, "the gate records stay on the node; only their status changes")
	for name, g := range reopened.Gates {
		require.Equalf(t, store.GateStatusStale, g.Status,
			"gate %q must not still read green over work it never saw", name)
	}
	require.Contains(t, reopened.Gates, "evals",
		"the evals gate is the one carrying human judgment, so it is named rather than swept up in the loop")

	// The boundary of the shared rule: the gate status stales AND so does every
	// per-criterion status inside the evals gate. The criterion grain is where a
	// human's attestation lives, and the eval re-evaluation preserves a settled
	// pass across re-runs, so leaving criteria alone let one ordinary `check`
	// return the gate to green on a sign-off given before this work existed.
	require.Equal(t, store.CriterionStatusStale, reopened.Gates["evals"].Criteria["C1"].Status,
		"a human attestation must not ride across a reopen")

	// The new task reaches done with NO intervening `check feature`, and the
	// feature re-crosses. Because the gates are stale rather than green, the
	// crossing records policy_violations naming them instead of passing in
	// silence: advisory enforcement doing its job.
	setTaskStatusOnDisk(t, root, "T-bbbbbb", "done")
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	recrossed := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "done", recrossed.Status)
	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.NotEmpty(t, fc[0].PolicyViolations,
		"a crossing over stale gates is recorded honestly, not as a clean done")
	require.Contains(t, fc[0].PolicyViolations, "build")

	// And the violations describe THIS crossing alone. They are cleared on the
	// reopen rather than appended to, so a node reopened and re-crossed many
	// times does not accumulate one entry per cycle for the same gate.
	byGate := map[string]int{}
	for _, v := range recrossed.PolicyViolations {
		byGate[v.Gate]++
	}
	for gate, n := range byGate {
		require.Equalf(t, 1, n, "gate %q has %d violation entries; a reopen clears rather than appends", gate, n)
	}
}

// addLateTask adds one approved-and-in-flight task under the fixture feature,
// the "new work landed beneath a done node" the recompute reacts to. localKey
// names the task's own folder under the feature.
func addLateTask(t *testing.T, root, taskID, localKey, title string) {
	t.Helper()
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "T" {
			return taskID
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key: "form-engine/certification-form-loading/" + localKey, Title: title,
	})
	require.NoError(t, err)
}

// setTaskStatusOnDisk rewrites one task's status in its own task.yml, the cheap
// way to complete new work without driving a second real merge.
func setTaskStatusOnDisk(t *testing.T, root, taskID, status string) {
	t.Helper()
	path := taskYMLPath(t, root, taskID)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var tk store.Task
	require.NoError(t, yaml.Unmarshal(data, &tk))
	tk.Status = status
	if status == "in_progress" || status == "check" || status == "in_review" || status == "done" {
		tk.Started = true
		if tk.Owner == "" {
			tk.Owner = "jc"
		}
	}
	out, err := yaml.Marshal(tk)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// TestSyncSkipsAnEpicWithNoProseRatherThanFailingTheRun pins the blast radius of
// one malformed node AND that the skip is reported. sync observes a whole
// project, so an epic whose own epic.md is gone (nothing to build a completion
// record against) must not turn every other node's earned crossing into a tool
// failure. But it must not vanish either: the work beneath it really is
// complete, so an unreported skip leaves a node that never crosses while the run
// prints its clean line, the exact indistinguishability cli-surface.md's
// done-crossing section settles against.
func TestSyncSkipsAnEpicWithNoProseRatherThanFailingTheRun(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	removeFromIntegration(t, root, ".tiller/epics/form-engine/epic.md")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err, "one unrecordable epic must not take the whole sync down")

	reloaded := load(t, root)
	require.Equal(t, "done", reloaded.Tasks["T-aaaaaa"].Status, "the task still crossed")
	require.Equal(t, "done", reloaded.Features["F-aaaaaa"].Status, "the feature still rolled up")
	require.Equal(t, "ready", reloaded.Epics["E-aaaaaa"].Status,
		"the epic did not cross, and says so by staying ready")
	require.Empty(t, crossingsOf(res, store.NounEpic))

	require.Len(t, res.Skipped, 1, "the skip is reported, never silent")
	require.Equal(t, store.NounEpic, res.Skipped[0].Noun)
	require.Equal(t, "E-aaaaaa", res.Skipped[0].ID)
	require.Equal(t, "form-engine", res.Skipped[0].Key)
	require.False(t, res.Skipped[0].Surface, "missing prose is not a surface to unlink")
	require.Equal(t, filepath.Join(".tiller", "epics", "form-engine", "epic.md"), res.Skipped[0].Path,
		"the offending path is the whole diagnosis, and the fix")

	// It is a skip, not a refusal: restoring the path on integration lets the
	// epic cross on the very next run, with nothing else to undo.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "form-engine", "epic.md"),
		[]byte("# Form Engine\n\nRestored prose.\n"), 0o644))
	commitPaths(t, root, "restore epic prose", ".tiller/epics/form-engine/epic.md")
	res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Skipped)
	require.Len(t, crossingsOf(res, store.NounEpic), 1)
	require.Equal(t, "done", load(t, root).Epics["E-aaaaaa"].Status)
}

// TestSyncSkipsAnEpicWhoseSurfacePathDoesNotResolve pins the OTHER half of the
// same catch, and the reason it is deliberately the whole completion record
// rather than the prose file alone. recordEpicVerified pins two kinds of path:
// epic.md, and one fingerprint for every surface.paths entry. A surface
// path that is absent at HEAD fails on identical terms, so it gets the identical
// answer: skipped, named, and the run carries on.
func TestSyncSkipsAnEpicWhoseSurfacePathDoesNotResolve(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	setEpicSurfacePath(t, root, "form-engine", "app/models/nowhere")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err, "a bad surface path is one node's problem, not the run's")

	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status)
	require.Equal(t, "ready", load(t, root).Epics["E-aaaaaa"].Status)
	require.Len(t, res.Skipped, 1)
	require.Equal(t, "app/models/nowhere", res.Skipped[0].Path,
		"the surface path is named, not the prose file that was fine")
	require.True(t, res.Skipped[0].Surface, "a recorded surface path is the unlink --surface case")
}

// removeFromIntegration deletes rel and commits only that deletion on the
// checked-out integration branch, so the integration commit sync records
// against has no such file while every other uncommitted change stays put.
func removeFromIntegration(t *testing.T, root, rel string) {
	t.Helper()
	gitRunM(t, root, "rm", "-q", "--", rel)
	gitRunM(t, root, "commit", "-m", "remove "+rel)
}

// setEpicSurfacePath declares one surface path on an epic.yml, the brownfield
// field recordEpicVerified fingerprints at the crossing.
func setEpicSurfacePath(t *testing.T, root, epicKey, surfacePath string) {
	t.Helper()
	p := filepath.Join(root, ".tiller", "epics", epicKey, "epic.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	var doc store.Epic
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Surface = store.Surface{Paths: []string{surfacePath}}
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, out, 0o644))
}

// TestSyncReopenClearsPolicyViolationsRatherThanAccumulating pins the clear
// itself, which needs a fixture that would visibly GROW without it. The
// stale-gates test above cannot do that job: its first crossing is clean, so
// there is nothing recorded to double, and its per-gate count passes whether or
// not the clear exists.
//
// Two full reopen/re-cross cycles is what it takes. Every re-crossing after a
// reopen runs over stale gates, so each one records a violation per required
// gate; without the clear those pile up in a field whose readers take it for the
// current record, and by the second cycle every gate carries two entries for
// what is really one condition.
func TestSyncReopenClearsPolicyViolationsRatherThanAccumulating(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	sync := func() {
		t.Helper()
		_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
		require.NoError(t, err)
	}
	violations := func() []store.PolicyViolation {
		t.Helper()
		return load(t, root).Features["F-aaaaaa"].PolicyViolations
	}

	// Cycle 0: the first crossing, clean over green gates.
	sync()
	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status)
	require.Empty(t, violations())

	// Two cycles, each landing a task, reopening, completing it and re-crossing
	// over the gates that reopen staled.
	for i, taskID := range []string{"T-bbbbbb", "T-cccccc"} {
		addLateTask(t, root, taskID, fmt.Sprintf("round-%d", i+1), fmt.Sprintf("Round %d of more work", i+1))
		sync()
		require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status)
		require.Emptyf(t, violations(), "cycle %d: the reopen clears what the superseded crossing recorded", i+1)

		setTaskStatusOnDisk(t, root, taskID, "done")
		sync()
		require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status)
		require.NotEmptyf(t, violations(), "cycle %d: a crossing over stale gates records violations", i+1)
	}

	// The pin: after two cycles the field describes THIS crossing, not the sum of
	// every crossing the node ever had. Without the clear each gate would carry
	// two entries here, one per cycle.
	byGate := map[string]int{}
	for _, v := range violations() {
		byGate[v.Gate]++
	}
	require.NotEmpty(t, byGate)
	for gate, n := range byGate {
		require.Equalf(t, 1, n,
			"gate %q carries %d violation entries after two reopen cycles; a reopen clears rather than appends", gate, n)
	}
}

// TestCheckRecomputesAStaleMechanicalCriterionRatherThanFreezingIt is the
// companion boundary to the reopen test above, and used to pin the opposite
// finding before criteria gained a stale value of their own: this test used to
// show the mitigation was loud right after the reopen and quiet again after one
// ordinary check, because the criterion enum had nothing but pass to fall back
// to. Now C1 reads stale the instant the feature reopens
// (TestStaleGatesStalesCriteriaToo covers the rule directly), and this test pins
// what happens to a stale criterion Tiller CAN re-verify: gates.EvaluateCriteria
// never consults keepPrior for a test or command binding, it reruns the command
// every time regardless of the prior status, so one ordinary check recomputes C1
// fresh off the current revision and it comes back pass on real evidence, not on
// a carried-over judgment. That is exactly right: the leak this task closes is
// the human one, a manual or required eval criterion that only keepPrior could
// preserve unexamined. A mechanically re-checkable criterion was never that risk,
// so staling it and then immediately re-earning it on the next check is the
// system working, not a residual gap.
//
// C1 is `required: true` here on purpose, unlike the shared fixture the reopen
// test above uses: with an unrequired criterion, GateStatusFromCriteria skips it
// entirely and the gate reads pass no matter what C1 says, so a version of this
// test that only checked the gate-level status could not tell a genuine
// recomputation from a frozen one carried straight through from the reopen. This
// version checks C1's own status after the check, which fails loudly if
// EvaluateCriteria is ever changed to freeze a stale command or test binding
// instead of rerunning it.
func TestCheckRecomputesAStaleMechanicalCriterionRatherThanFreezingIt(t *testing.T) {
	t.Parallel()
	greenMD := "---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: The suite passes\n    polarity: introduce\n    binding: { type: command, run: \"true\" }\n    required: true\n---\n"
	root := layActiveFeatureGitWithEval(t, "true", "true", greenMD)
	submitTask(t, root)

	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status, "the first crossing is genuinely green")

	addLateTask(t, root, "T-bbbbbb", "unseen-work", "Work the attestation never saw")
	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	reopened := load(t, root).Features["F-aaaaaa"].Gates["evals"]
	require.Equal(t, store.GateStatusStale, reopened.Status, "the reopen stales the gate")
	require.Equal(t, store.CriterionStatusStale, reopened.Criteria["C1"].Status,
		"the criterion under it stales the same instant, before any check has touched it")

	// One ordinary check, with no human judgment of any kind. C1 is required, so
	// the gate can read pass here only if C1 itself was actually recomputed: a
	// frozen stale value would leave the gate pending, not pass.
	feat := resolveNode(t, load(t, root), store.NounFeature, "form-engine/certification-form-loading")
	_, err = mutate.Check(load(t, root), testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)
	after := load(t, root).Features["F-aaaaaa"].Gates["evals"]
	require.Equal(t, store.CriterionStatusPass, after.Criteria["C1"].Status,
		"a command-binding criterion reruns on every check regardless of staleness; this pass is freshly earned, not carried over")
	require.Equal(t, store.GateStatusPass, after.Status,
		"the gate rolls up from C1's freshly recomputed pass, the only way it could read pass here")

	// And so the re-crossing is silent: nothing here was preserved across the
	// reopen, everything was reverified against the new work.
	setTaskStatusOnDisk(t, root, "T-bbbbbb", "done")
	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.NotContains(t, fc[0].PolicyViolations, "evals",
		"a genuinely re-verified gate records no violation, which is correct here")
}

// TestReopenStalesAHumanAttestedCriterionAndCheckReturnsItToNeedsAttestation is
// the end-to-end proof of the leak this task closes, driven through the real
// human path rather than a unit test on StaleGates alone: a required MANUAL
// criterion, the kind no machine can re-verify, gets a human's attested pass,
// the feature crosses done on it, new work lands underneath, and the reopen
// must not let that attestation ride. Before criteria could go stale, this
// exact sequence left C1 reading pass straight through the reopen (the same
// prior status keepPrior preserves on a re-run), so nothing forced a second
// look. Nothing in the existing coverage pinned this: TestStaleGatesStalesCriteriaToo
// exercises StaleGates directly with no check involved, and the reopen test
// above only carries a test-binding, non-required criterion through a check. A
// future change that let keepPrior treat stale as settled (adding it to the
// pass/needs_attestation disjunction) would go undetected by every other test
// in this file; this one catches it because it is the only one that attests a
// required manual criterion and then checks past a reopen.
func TestReopenStalesAHumanAttestedCriterionAndCheckReturnsItToNeedsAttestation(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGitWithEval(t, "true", "true", requiredManualEvalMD)
	submitTask(t, root)

	feat := resolveNode(t, load(t, root), store.NounFeature, "form-engine/certification-form-loading")
	_, err := mutate.Attest(load(t, root), testDeps(), feat, mutate.AttestInput{
		Criterion: "C1", Status: store.CriterionStatusPass,
	})
	require.NoError(t, err)
	settled := load(t, root).Features["F-aaaaaa"].Gates["evals"]
	require.Equal(t, store.CriterionStatusPass, settled.Criteria["C1"].Status,
		"the human attestation settles the required manual criterion")
	require.Equal(t, store.GateStatusPass, settled.Status, "the gate rolls up green on the attested criterion")

	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status, "the attested criterion crosses the feature clean")

	// New work lands beneath the done feature, so it reopens.
	addLateTask(t, root, "T-bbbbbb", "unseen-work", "Work the human's attestation never saw")
	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	reopened := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "active", reopened.Status, "new work reopens the feature")
	require.Equal(t, store.CriterionStatusStale, reopened.Gates["evals"].Criteria["C1"].Status,
		"the human's attested pass must not ride across the reopen")

	// One ordinary check, with no new human judgment of any kind.
	feat = resolveNode(t, load(t, root), store.NounFeature, "form-engine/certification-form-loading")
	_, err = mutate.Check(load(t, root), testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)

	checked := load(t, root).Features["F-aaaaaa"].Gates["evals"]
	require.Equal(t, store.CriterionStatusNeedsAttestation, checked.Criteria["C1"].Status,
		"a required manual criterion recomputes to needs_attestation, never back to the stale attestation's pass")
	require.NotEqual(t, store.GateStatusPass, checked.Status,
		"the gate must not read green again on judgment given before this work existed")
}

// TestSyncSkipsAFeatureWithNoProseRatherThanFailingTheRun is the feature-grain
// half of the epic skip above, on identical terms. sync observes a whole
// project, so a feature whose own prose is gone (nothing to build a completion
// record against) must not turn every other node's earned crossing into a tool
// failure, and it must not vanish either: the tasks beneath it really are done,
// so an unreported skip leaves a node that never crosses while the run prints
// its clean line.
func TestSyncSkipsAFeatureWithNoProseRatherThanFailingTheRun(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	removeFromIntegration(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.md")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err, "one unrecordable feature must not take the whole sync down")

	reloaded := load(t, root)
	require.Equal(t, "done", reloaded.Tasks["T-aaaaaa"].Status, "the task still crossed")
	require.Equal(t, "active", reloaded.Features["F-aaaaaa"].Status,
		"the feature did not cross, and says so by staying active")
	require.Equal(t, "ready", reloaded.Epics["E-aaaaaa"].Status,
		"the epic cannot roll up over a feature that did not cross")
	require.Empty(t, crossingsOf(res, store.NounFeature))

	require.Len(t, res.Skipped, 1, "the skip is reported, never silent")
	require.Equal(t, store.NounFeature, res.Skipped[0].Noun)
	require.Equal(t, "F-aaaaaa", res.Skipped[0].ID)
	require.Equal(t, "form-engine/certification-form-loading", res.Skipped[0].Key)
	require.Equal(t, filepath.Join(".tiller", "epics", "form-engine", "features",
		"certification-form-loading", "feature.md"), res.Skipped[0].Path,
		"the offending path is the whole diagnosis, and the fix")

	// It is a skip, not a refusal: restoring the path on integration lets the
	// feature roll up on the very next run, and the epic follows it.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "form-engine",
		"features", "certification-form-loading", "feature.md"),
		[]byte("# Certification form loading\n\nRestored prose.\n"), 0o644))
	commitPaths(t, root, "restore feature prose", ".tiller/epics/form-engine/features/certification-form-loading/feature.md")
	res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Skipped)
	require.Len(t, crossingsOf(res, store.NounFeature), 1)
	require.Equal(t, "done", load(t, root).Features["F-aaaaaa"].Status)
	require.Equal(t, "done", load(t, root).Epics["E-aaaaaa"].Status)
}

// TestSyncSkipsAFeatureWhoseSurfacePathDoesNotResolve pins the other half of the
// same catch at feature grain. recordVerified pins three kinds of path:
// feature.md, feature.eval.md, and one fingerprint for every
// surface.paths entry. A surface path absent at HEAD fails on identical terms,
// so it gets the identical answer: skipped, named, and the run carries on.
func TestSyncSkipsAFeatureWhoseSurfacePathDoesNotResolve(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	setFeatureSurfacePath(t, root, "form-engine", "certification-form-loading", "app/models/nowhere")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err, "a bad surface path is one node's problem, not the run's")

	require.Equal(t, "done", load(t, root).Tasks["T-aaaaaa"].Status)
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status)
	require.Len(t, res.Skipped, 1)
	require.Equal(t, "app/models/nowhere", res.Skipped[0].Path,
		"the surface path is named, not the prose that was fine")
}

// setFeatureSurfacePath declares surface paths on a feature.yml, the
// brownfield field recordVerified fingerprints at the crossing.
func setFeatureSurfacePath(t *testing.T, root, epicKey, featKey string, surfacePaths ...string) {
	t.Helper()
	p := filepath.Join(root, ".tiller", "epics", epicKey, "features", featKey, "feature.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Surface = store.Surface{Paths: surfacePaths}
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, out, 0o644))
}

// These round-trip regressions protect durable dependency edges and the approved
// spec basis from being erased when lifecycle, gate, and sync writes update the
// same task document.
func TestATaskWritePreservesTheFieldsNoVerbTouches(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key:   "form-engine/certification-form-loading/transcribe-the-schema",
		Title: "Transcribe the schema",
	})
	require.NoError(t, err)
	seedTaskDependsOn(t, root, "T-aaaaaa")

	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "approve"})
	basis := pinnedTaskBasis(t, root, "T-aaaaaa")
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	requireTaskBasisIntact(t, root, "T-aaaaaa", basis, "after start")

	task := resolveNode(t, load(t, root), store.NounTask, "T-aaaaaa")
	_, err = mutate.Check(load(t, root), testDeps(), vcs.New(root), task, mutate.CheckInput{})
	require.NoError(t, err)
	requireTaskBasisIntact(t, root, "T-aaaaaa", basis, "after check")

	stageSyncReview(t, root, "T-aaaaaa", featBranch)

	feat := resolveNode(t, load(t, root), store.NounFeature, "form-engine/certification-form-loading")
	_, err = mutate.Check(load(t, root), testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)
	setFeatureBranch(t, root, "form-engine", "certification-form-loading", featBranch)

	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "done", load(t, root).Tasks["T-aaaaaa"].Status, "the arc reached the sync crossing")
	requireTaskBasisIntact(t, root, "T-aaaaaa", basis, "after sync")
}

// Eval also persists task gates and must preserve the dependency and spec basis.
func TestATaskEvalPreservesTheFieldsNoVerbTouches(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGitWithEval(t, "true", "true", requiredManualEvalMD)
	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key:   "form-engine/certification-form-loading/transcribe-the-schema",
		Title: "Transcribe the schema",
	})
	require.NoError(t, err)
	seedTaskDependsOn(t, root, "T-aaaaaa")
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "approve"})
	basis := pinnedTaskBasis(t, root, "T-aaaaaa")
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})

	task := resolveNode(t, load(t, root), store.NounTask, "T-aaaaaa")
	_, err = mutate.Eval(load(t, root), testDeps(), vcs.New(root), task, mutate.EvalInput{})
	require.NoError(t, err)
	require.Contains(t, load(t, root).Tasks["T-aaaaaa"].Gates, "evals", "the eval run recorded its gate")
	requireTaskBasisIntact(t, root, "T-aaaaaa", basis, "after eval")
}

// seededDependsOn names the completed sibling used by the round-trip fixtures.
var seededDependsOn = []string{"T-9wq4m1"}

// seedTaskDependsOn completes a real sibling through sync, then links the task to
// it so dependency enforcement and the persisted edge agree.
func seedTaskDependsOn(t *testing.T, root, taskID string) {
	t.Helper()
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "T" {
			return seededDependsOn[0]
		}
		return fixedMint(prefix)
	}
	_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key:   "form-engine/certification-form-loading/schema-prerequisite",
		Title: "Schema prerequisite",
	})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), deps, seededDependsOn[0], "")
	require.NoError(t, err)
	stageSyncReview(t, root, seededDependsOn[0], featBranch)
	_, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "done", load(t, root).Tasks[seededDependsOn[0]].Status)
	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounTask, Node: taskID, DependsOn: seededDependsOn,
	})
	require.NoError(t, err)
}

// pinnedTaskBasis reads back what the approve gate recorded, the value every
// later write of the same document must carry through untouched.
func pinnedTaskBasis(t *testing.T, root, taskID string) store.RevisionRef {
	t.Helper()
	data, err := os.ReadFile(taskYMLPath(t, root, taskID))
	require.NoError(t, err)
	var tk store.Task
	require.NoError(t, yaml.Unmarshal(data, &tk))
	require.NotNil(t, tk.AcceptedAgainst, "the approve gate pins accepted_against")
	require.Equal(t, store.KindGitBlob, tk.AcceptedAgainst.Spec.Kind)
	return tk.AcceptedAgainst.Spec
}

// requireTaskBasisIntact reads the task back off disk and fails naming the verb
// that dropped or rewrote either field.
func requireTaskBasisIntact(t *testing.T, root, taskID string, basis store.RevisionRef, when string) {
	t.Helper()
	data, err := os.ReadFile(taskYMLPath(t, root, taskID))
	require.NoError(t, err)
	var tk store.Task
	require.NoError(t, yaml.Unmarshal(data, &tk))
	require.Equal(t, seededDependsOn, tk.DependsOn, "depends_on dropped %s", when)
	require.NotNil(t, tk.AcceptedAgainst, "accepted_against dropped %s", when)
	require.Equal(t, basis, tk.AcceptedAgainst.Spec, "accepted_against changed %s", when)
}

// TestSyncRecordsAFileSurfaceAsAGitBlobFingerprint pins the Lantern finding of
// 2026-08-07: link --surface accepts existing files and directories, so the
// done crossing must record either rather than skip it forever. A file
// fingerprints as a git_blob whose inventory is the file itself.
func TestSyncRecordsAFileSurfaceAsAGitBlobFingerprint(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "chrome.swift"), []byte("struct Chrome {}\n"), 0o644))
	gitRunM(t, root, "add", "-A")
	gitRunM(t, root, "commit", "-m", "add chrome.swift")
	setFeatureSurfacePath(t, root, "form-engine", "certification-form-loading", "chrome.swift")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Skipped, "a file surface is recordable, not a skip")

	f := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "done", f.Status)
	require.Len(t, f.VerifiedAgainst.Surfaces, 1)
	sf := f.VerifiedAgainst.Surfaces[0]
	require.Equal(t, store.KindGitBlob, sf.Kind)
	blob, err := vcs.New(root).BlobHash("chrome.swift")
	require.NoError(t, err)
	require.Equal(t, "git:"+blob, sf.Value)
	require.Equal(t, []string{"chrome.swift"}, sf.Inventory)
}

// TestSyncRecordsMixedSurfaceGrains covers a surface list naming a directory
// and a file side by side, the shape the Lantern feature actually carried: each
// entry fingerprints at its own grain, in declaration order.
func TestSyncRecordsMixedSurfaceGrains(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "a.swift"), []byte("struct A {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "chrome.swift"), []byte("struct Chrome {}\n"), 0o644))
	gitRunM(t, root, "add", "-A")
	gitRunM(t, root, "commit", "-m", "add code")
	setFeatureSurfacePath(t, root, "form-engine", "certification-form-loading", "src", "chrome.swift")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Skipped)

	f := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "done", f.Status)
	require.Len(t, f.VerifiedAgainst.Surfaces, 2)
	require.Equal(t, store.KindGitTree, f.VerifiedAgainst.Surfaces[0].Kind)
	require.Equal(t, []string{"src/a.swift"}, f.VerifiedAgainst.Surfaces[0].Inventory)
	require.Equal(t, store.KindGitBlob, f.VerifiedAgainst.Surfaces[1].Kind)
	require.Equal(t, []string{"chrome.swift"}, f.VerifiedAgainst.Surfaces[1].Inventory)
}

// TestSyncRecordsAnEpicFileSurfaceAsAGitBlobFingerprint is the epic grain of
// the same rule Task 1's feature test pins: both recorders share
// surfaceFingerprint, so a file surface crosses rather than skipping.
func TestSyncRecordsAnEpicFileSurfaceAsAGitBlobFingerprint(t *testing.T) {
	t.Parallel()
	root := layEpicRollupGit(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "chrome.swift"), []byte("struct Chrome {}\n"), 0o644))
	gitRunM(t, root, "add", "-A")
	gitRunM(t, root, "commit", "-m", "add chrome.swift")
	setEpicSurfacePath(t, root, "form-engine", "chrome.swift")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Empty(t, res.Skipped)

	e := load(t, root).Epics["E-aaaaaa"]
	require.Equal(t, "done", e.Status)
	require.Len(t, e.VerifiedAgainst.Surfaces, 1)
	require.Equal(t, store.KindGitBlob, e.VerifiedAgainst.Surfaces[0].Kind)
	require.Equal(t, []string{"chrome.swift"}, e.VerifiedAgainst.Surfaces[0].Inventory)
}

func syncBaselineFixture(t *testing.T, surfaces []string) (string, []byte) {
	t.Helper()
	root := layEpicRollupGit(t)
	designWrite(t, root, "app/models/form.rb", "class Form; end\n")
	designWrite(t, root, "application/main.rb", "class Application; end\n")
	designWrite(t, root, "app/models2/other.rb", "class Other; end\n")
	manifest := designBytes(t, root, ".tiller/tiller.yml")
	designWrite(t, root, ".tiller/tiller.yml", string(manifest)+"\ninputs:\n  code:\n    - {root: '.', mode: surface}\n")
	for _, doc := range []struct {
		path, describes string
	}{
		{"root.md", "."},
		{"app.md", "app"},
		{"form.md", "app/models/form.rb"},
		{"pending.md", "app/models"},
		{"sibling.md", "application"},
		{"sibling-prefix.md", "app/models2"},
	} {
		designWrite(t, root, ".tiller/design/"+doc.path, "# As-is\n")
		linkDesign(t, root, doc.path, doc.describes)
	}
	_, err := mutate.ApproveDesign(load(t, root), testDeps(), []string{"app.md"})
	require.NoError(t, err)
	// External ledger coordinates are legal, but the project root does not
	// contain them. No filesystem observation belongs to the touched-doc join.
	ledger := load(t, root).Baseline
	ledger.Docs = append(ledger.Docs, store.BaselineDoc{Path: "external.md", Describes: []string{"../legacy"}})
	data, err := yaml.Marshal(ledger)
	require.NoError(t, err)
	designWrite(t, root, store.BaselineRelPath, string(data))
	feature := resolveNode(t, load(t, root), store.NounFeature, "F-aaaaaa")
	_, err = mutate.SetSurface(load(t, root), testDeps(), feature, surfaces)
	require.NoError(t, err)
	gitCommitAllM(t, root, "record described surfaces")
	return root, designBytes(t, root, store.BaselineRelPath)
}

func TestSyncTouchedDesignOverlapAndReadOnly(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		surfaces []string
		want     []string
	}{
		{"descendants and duplicate overlaps", []string{"app/models", "app/models/form.rb"}, []string{"app.md", "form.md", "pending.md", "root.md"}},
		{"containing surface", []string{"app"}, []string{"app.md", "form.md", "pending.md", "root.md", "sibling-prefix.md"}},
		{"project root excludes external", []string{"."}, []string{"app.md", "form.md", "pending.md", "root.md", "sibling-prefix.md", "sibling.md"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, ledger := syncBaselineFixture(t, tc.surfaces)
			res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			features := crossingsOf(res, store.NounFeature)
			require.Len(t, features, 1)
			require.Equal(t, tc.want, features[0].TouchedDesign)
			for _, crossing := range res.Crossings {
				if crossing.Noun != store.NounFeature {
					require.Empty(t, crossing.TouchedDesign, crossing.Noun)
				}
			}
			require.Equal(t, ledger, designBytes(t, root, store.BaselineRelPath))
			res, err = mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Empty(t, res.Crossings, "a no-op sync cannot repeat the touched-doc notice")
			require.Equal(t, ledger, designBytes(t, root, store.BaselineRelPath))
		})
	}
}

func TestSyncTouchedDesignExcludesTaskOnlySkippedAndReopened(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"task-only", "skipped", "reopened"} {
		t.Run(mode, func(t *testing.T) {
			root, ledger := syncBaselineFixture(t, []string{"app"})
			if mode == "reopened" {
				_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
				require.NoError(t, err)
			}
			if mode == "skipped" {
				removeFromIntegration(t, root, ".tiller/epics/form-engine/features/certification-form-loading/feature.md")
			} else {
				deps := testDeps()
				deps.Mint = func(prefix string) string { return prefix + "-bbbbbb" }
				_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
					Key: "form-engine/certification-form-loading/more-work", Title: "More work",
				})
				require.NoError(t, err)
			}
			res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
			require.NoError(t, err)
			require.Empty(t, crossingsOf(res, store.NounFeature))
			for _, crossing := range res.Crossings {
				require.Empty(t, crossing.TouchedDesign)
			}
			switch mode {
			case "task-only":
				require.Len(t, crossingsOf(res, store.NounTask), 1)
			case "skipped":
				require.Len(t, res.Skipped, 1)
			case "reopened":
				require.Len(t, res.Reopened, 2)
			}
			require.Equal(t, ledger, designBytes(t, root, store.BaselineRelPath))
		})
	}
}
