package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const cliFeatureKey = "form-engine/certification-form-loading"

// cliTaskKey is the canonical key of the task the CLI sync fixtures add under
// cliFeatureKey, the node the human-surface assertions name.
const cliTaskKey = cliFeatureKey + "/transcribe-the-schema"

// driveFeatureToReadyCLI prepares an approved feature with one claimed task
// through the CLI, the fixture the sync and active-basis tests build on.
func driveFeatureToReadyCLI(t *testing.T) {
	t.Helper()
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	claimTaskAsOwner(t, cliTaskKey, "jc")
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", cliFeatureKey, "--contributor", "jc")
	execCLI(t, "approve", "feature", cliFeatureKey, "--contributor", "jc")
}

// submitTaskOnFeatureCLI starts and checks out the feature's task, then submits
// its review and records the feature gates needed for rollup.
func submitTaskOnFeatureCLI(t *testing.T, key string) string {
	t.Helper()
	_, _, code := execCLI(t, "add", "task", key+"/transcribe-the-schema", "--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := taskIDForFeatureCLI(t, key)
	_, _, code = execCLI(t, "approve", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	checkoutTaskBranchCLI(t, tid)
	dir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "work.txt"), []byte(key+"\n"), 0o644))
	gitRun(t, dir, "add", "work.txt")
	gitRun(t, dir, "commit", "-m", "task work")
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, 0, code)

	// The feature-grain gates must be recorded for the rollup to fire behind the
	// task's crossing.
	_, _, code = execCLI(t, "check", "feature", key, "--contributor", "jc")
	require.Equal(t, 0, code)
	return tid
}

// taskIDForFeatureCLI reads the id of the one task under the named feature, the
// multi-feature companion to soleTaskID.
func taskIDForFeatureCLI(t *testing.T, key string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	fid := featureIDByKey(t, key)
	ids := []string{}
	for id, tk := range tree.Tasks {
		if tk.FeatureID == fid {
			ids = append(ids, id)
		}
	}
	require.Len(t, ids, 1, "expected exactly one task under %s", key)
	return ids[0]
}

// taskStatusCLI reads one task's status off disk.
func taskStatusCLI(t *testing.T, id string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Contains(t, tree.Tasks, id)
	return tree.Tasks[id].Status
}

// setBuildGateCommandCLI rewrites the manifest's build gate command, so a fixture
// can walk a task through green gates and then make the feature-grain run red,
// which is what the advisory-enforcement tests need: a node in review under a
// gate that is not green at the merge.
func setBuildGateCommandCLI(t *testing.T, dir, run string) {
	t.Helper()
	p := filepath.Join(dir, ".tiller", "tiller.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	out := strings.Replace(string(data), `build: { kind: command, run: "true"`, `build: { kind: command, run: "`+run+`"`, 1)
	require.NotEqual(t, string(data), out, "expected a build gate line to rewrite")
	require.NoError(t, os.WriteFile(p, []byte(out), 0o644))
}

// mergeTaskBranchCLI includes the task's state writes in the human's local merge.
func mergeTaskBranchCLI(t *testing.T, dir, branch string) {
	t.Helper()
	require.Equal(t, branch, gitOut(t, dir, "branch", "--show-current"))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "record task review")
	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "merge", "--no-ff", "-m", "merge task", branch)
}

func fastForwardTaskBranchCLI(t *testing.T, dir, branch string) {
	t.Helper()
	require.Equal(t, branch, gitOut(t, dir, "branch", "--show-current"))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "record task review")
	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "merge", "--ff-only", branch)
}

// driveFastForwardedTaskToInReviewCLI leaves a submitted task stranded after
// fast-forward integration, with no merge commit witnessing that integration.
func driveFastForwardedTaskToInReviewCLI(t *testing.T, dir string) string {
	t.Helper()
	driveFeatureToReadyCLI(t)
	tid := submitTaskOnFeatureCLI(t, cliFeatureKey)
	branch := loadTestTree(t).Tasks[tid].Branch
	fastForwardTaskBranchCLI(t, dir, branch)
	return branch
}

// declareIntegrationBranchCLI appends git.integration_branch to the fixture
// manifest, which is what `tiller init` seeds into a real project and what
// --accept-fast-forward requires before it will assert anything. The assertion is
// a claim about the project's trunk, so Tiller measures it against the declared
// branch or refuses; the fixtures that lean on the HEAD fallback deliberately do
// not call this, because that fallback is a path this file still pins.
func declareIntegrationBranchCLI(t *testing.T, dir string) {
	t.Helper()
	p := filepath.Join(dir, ".tiller", "tiller.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, append(data, []byte("git:\n  integration_branch: \"main\"\n")...), 0o644))
}

// secondFeatureKey is a second feature under the same epic, so one sync run can
// carry an observed crossing and an asserted one at the same time.
const secondFeatureKey = "form-engine/second-slice"

// driveSecondFeatureToReadyCLI prepares the second feature's approved plan
// through the real command chain, spec and all.
func driveSecondFeatureToReadyCLI(t *testing.T) {
	t.Helper()
	_, _, code := execCLI(t, "add", "feature", secondFeatureKey, "--title", "Second slice", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	dir := filepath.Join(cwd, ".tiller", "epics", "form-engine", "features", "second-slice")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Second slice\n\nStand up the second slice so one sync carries two nodes.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte("---\nnode: F-bbbbbb\ncriteria:\n  - id: C1\n    statement: The second slice loads\n    polarity: introduce\n    binding: { type: test, ref: spec/second_spec.rb }\n---\n\n# Acceptance\n"), 0o644))
	// The explosion is part of the stage-2 output `spec feature` validates.
	// It carries the same local key submitTaskOnFeatureCLI adds later, so that
	// add is the idempotent no-op and the feature keeps its one task.
	_, _, code = execCLI(t, "add", "task", secondFeatureKey+"/transcribe-the-schema",
		"--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	claimTaskAsOwner(t, secondFeatureKey+"/transcribe-the-schema", "jc")
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, store.TaskDirName, "transcribe-the-schema", store.TaskProseFile),
		[]byte("# Transcribe the schema\n\n## Approach\n\nTranscribe it.\n"), 0o644))
	_, _, code = execCLI(t, "spec", "feature", secondFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "feature", secondFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
}

// featureIDByKey reads one feature's minted id off disk by its canonical key, the
// multi-feature companion to soleFeatureID.
func featureIDByKey(t *testing.T, key string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	for id, f := range tree.Features {
		if f.Key != nil && *f.Key == key {
			return id
		}
	}
	t.Fatalf("no feature keyed %s in the tree", key)
	return ""
}

// featureDoneBasisCLI reads the done_basis the crossing wrote onto a feature.
func featureDoneBasisCLI(t *testing.T, key string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	return tree.Features[featureIDByKey(t, key)].DoneBasis
}

// TestSyncCrossesFeatureToDoneCLI is the end-to-end done crossing: a feature whose
// branch was locally merged crosses to done through `tiller sync`, with a success
// envelope and no GitHub.
func TestSyncCrossesFeatureToDoneCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)

	tid := submitTaskOnFeatureCLI(t, cliFeatureKey)
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"noun\": \"feature\"")
	require.Contains(t, out, "\"status\": \"done\"")
	require.NotContains(t, jsonData(t, out), "board")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, "done", tree.Features[soleFeatureID(t)].Status)
	require.NoFileExists(t, filepath.Join(dir, ".tiller", "board.md"))
}

// TestSyncRedGatePolicyViolationsCLI confirms advisory enforcement at the CLI:
// merging with a red required gate still records done and surfaces the gate in
// policy_violations.
func TestSyncRedGatePolicyViolationsCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)

	// The task reaches review on green gates, because check advances it only on
	// green; the build gate then goes red before the feature-grain run, which is
	// the state a merge under advisory enforcement observes.
	tid := submitTaskOnFeatureCLI(t, cliFeatureKey)
	setBuildGateCommandCLI(t, dir, "false")
	_, _, code := execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 1, code, "a red required gate is a blocked business answer")
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)
	require.Equal(t, "in_review", taskStatusCLI(t, tid))

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"status\": \"done\"")
	require.Contains(t, out, "\"policy_violations\"")
	require.Contains(t, out, "build")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	f := tree.Features[soleFeatureID(t)]
	require.Equal(t, "done", f.Status)
	require.NotEmpty(t, f.PolicyViolations)
}

// TestSyncNoMergeIsCleanCLI confirms a sync with nothing merged is a clean success
// with no crossings or generated board.
func TestSyncNoMergeIsCleanCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "lay tree")

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"crossings\": []")
	require.NotContains(t, jsonData(t, out), "board")
	require.NoFileExists(t, filepath.Join(dir, ".tiller", "board.md"))
}

func TestSyncTaskMergeDoesNotCrossItsSiblingCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)
	first := soleTaskID(t)
	addSpeccedTask(t, "render-the-form", "Render the form")
	sibling := taskIDByKey(t, cliFeatureKey+"/render-the-form")
	mustRun(t, "approve", "task", sibling, "--contributor", "jc")

	for _, id := range []string{first, sibling} {
		claimTaskAsOwner(t, id, "jc")
		mustRun(t, "start", "task", id, "--contributor", "jc")
		checkoutTaskBranchCLI(t, id)
		work := id + ".txt"
		require.NoError(t, os.WriteFile(filepath.Join(dir, work), []byte(id+"\n"), 0o644))
		gitRun(t, dir, "add", work)
		gitRun(t, dir, "commit", "-m", "implement "+id)
		mustRun(t, "check", "task", id, "--contributor", "jc")
		mustRun(t, "submit", "task", id, "--contributor", "jc")
		gitRun(t, dir, "checkout", "main")
	}
	mustRun(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	firstBranch := loadTestTree(t).Tasks[first].Branch
	require.NotEqual(t, firstBranch, loadTestTree(t).Tasks[sibling].Branch)
	gitRun(t, dir, "checkout", firstBranch)
	// Both submission records travel with this state commit, but only the first
	// task's implementation is merged. Metadata is not proof of sibling work.
	mergeTaskBranchCLI(t, dir, firstBranch)
	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, []string{"task"}, syncCrossings(t, out))
	require.Equal(t, "done", taskStatusCLI(t, first))
	require.Equal(t, "in_review", taskStatusCLI(t, sibling))
	require.Equal(t, "active", soleFeatureStatus(t))
	require.NoFileExists(t, filepath.Join(dir, sibling+".txt"))
}

// TestSyncHumanNamesCrossingsByRef pins the human sync rendering:
// each crossing line names its node by the shared reference convention, and a
// task crossing, which carries only its id, borrows its parent feature's key
// path through the tree humanSync resolves against, the one loaded BEFORE the
// sync mutation. The fixture walks one task to in_review and checks the
// feature, so the observed merge records both the task crossing and the parent
// rollup; the head must count both.
func TestSyncHumanNamesCrossingsByRef(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)

	// Walk one task to in_review under the active feature, the real command
	// chain: add, approve, start, check green, submit with a PR.
	_, _, code := execCLI(t, "add", "task", cliFeatureKey+"/transcribe-the-schema", "--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)
	_, _, code = execCLI(t, "approve", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	checkoutTaskBranchCLI(t, tid)
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, 0, code)

	// Feature-level gates checked so the parent rollup fires at the sync.
	_, _, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)

	// The branch start recorded is merged locally; sync observes it by ancestry.
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)
	fid := soleFeatureID(t)

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	// Three, one per grain: the task on the merge, the feature over its tasks,
	// and the epic over its features, all in one pass.
	require.Contains(t, out, "Sync: recorded 3 done crossing(s):")
	// The task line carries its own key path; a bare "T-x -> done" or the
	// degenerate "T-x (T-x)" must never render.
	require.Contains(t, out, fmt.Sprintf("- task %s (%s) -> done", cliTaskKey, tid))
	require.NotContains(t, out, fmt.Sprintf("%s (%s)", tid, tid))
	// The rollup lines name each node by its own key path.
	require.Contains(t, out, fmt.Sprintf("- feature %s (%s) -> done", cliFeatureKey, fid))
	require.Contains(t, out, fmt.Sprintf("- epic %s (%s) -> done", "form-engine", soleEpicID(t)))
}

// driveTaskToInReviewOnItsBranchCLI leaves the submitted task on its own branch.
func driveTaskToInReviewOnItsBranchCLI(t *testing.T, dir string) string {
	t.Helper()
	driveFeatureToReadyCLI(t)
	tid := submitTaskOnFeatureCLI(t, cliFeatureKey)
	return loadTestTree(t).Tasks[tid].Branch
}

// Observing a working branch against itself cannot establish integration.
func TestSyncRefusesOnTheTasksOwnBranchCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	branch := driveTaskToInReviewOnItsBranchCLI(t, dir)

	// The task branch is genuinely not merged into main.
	require.Equal(t, branch, gitOut(t, dir, "branch", "--show-current"))
	require.Equal(t, "in_review", taskStatusCLI(t, soleTaskID(t)))

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"reason\": \"into_is_feature_branch\"")
	require.Contains(t, out, "\"into\": \""+branch+"\"")
	require.Contains(t, out, "\"source\": \"HEAD\"")
	require.Contains(t, out, "\"branch\": \""+branch+"\"")
	require.NotContains(t, out, "\"error\"", "a blocked answer carries a data block, never an error block")

	// Nothing was recorded: the refusal runs before any observation and any write.
	require.Equal(t, "in_review", taskStatusCLI(t, soleTaskID(t)))
	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.False(t, hasEventCLI(t, tree, "sync.feature"), "a refused sync writes no crossing event")

	// The human line names the branch and how to proceed.
	_, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 1, code)

	// --into naming that same branch is refused too: there is no legitimate
	// reading of "this branch was merged into itself", whatever asked for it.
	gitRun(t, dir, "checkout", "main")
	out, _, code = execCLI(t, "sync", "--contributor", "jc", "--into", branch, "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"reason\": \"into_is_feature_branch\"")
	require.Contains(t, out, "\"source\": \"--into\"")
}

// TestSyncNamesNodesStillInReviewCLI pins S3 at the CLI: a sync that records
// nothing while work sits in review says so and names the nodes plus the causes,
// instead of printing the clean-run line and exiting 0 as if all were well. The
// exit code stays 0 on purpose: a node waiting on a merge is a true report of the
// world rather than a refusal, so the loudness lives in the text and in the
// awaiting data block.
func TestSyncNamesNodesStillInReviewCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	branch := driveTaskToInReviewOnItsBranchCLI(t, dir)
	gitRun(t, dir, "checkout", "main") // observe from the integration branch; nothing merged

	tid := soleTaskID(t)
	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Sync: no crossings recorded against main; 1 node(s) still in review:")
	// The awaiting node is the task, named by its own canonical key path and id,
	// on the branch its work merges on.
	require.Contains(t, out, fmt.Sprintf("  - task %s (%s) on %s", cliTaskKey, tid, branch))
	// The causes named have to be causes that can actually be true here. The
	// branch is not merged and still exists, so a readout claiming it was "merged
	// into a branch other than main" or "deleted after the merge" would be
	// pointing the reader away from the truth.
	require.Contains(t, out, "Not merged yet, or merged in a shape sync cannot read:")
	require.Contains(t, out, "a fast-forward")
	require.NotContains(t, out, "merged into a branch other than main")
	require.NotContains(t, out, "no merged branches to record", "the clean-run line must not fire while work waits")

	jout, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, jout, "\"awaiting\"")
	require.Contains(t, jout, "\"branch\": \""+branch+"\"")
	require.NotContains(t, jout, "\"branch_missing\"", "the branch is still there, so nothing claims otherwise")

	// Delete the local branch after the merge, the case squash detection cannot
	// reach: it is separated from "not merged yet" rather than reported alike.
	gitRun(t, dir, "branch", "-D", branch)
	out, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "(branch missing locally)")

	jout, _, code = execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, jout, "\"branch_missing\": true")
	// A branch that is simply gone has no assertion route, so the breadcrumb for
	// the one that does must not appear here.
	out, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "--accept-fast-forward")
}

// TestSyncAcceptFastForwardConfirmationCLI is the whole recovery at the human
// surface. A fast-forward merge leaves the node stranded with no route to done by
// observation, the readout says so and hands over the command, and running that
// command records the crossing on the human's word.
//
// The confirmation is pinned line by line on purpose. An assertion rendered as an
// observation would be the exact fabrication plain sync refuses to commit, so the
// asserted block has its own header, names every node, prints the basis, and says
// in as many words what the record rests on. The observed header must not appear
// at all, and neither must the zero-crossing line.
func TestSyncAcceptFastForwardConfirmationCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFastForwardedTaskToInReviewCLI(t, dir)
	tid := soleTaskID(t)

	declareIntegrationBranchCLI(t, dir)

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "(in main already, but no merge commit witnesses it)")
	require.Contains(t, out, "crosses only on the human's assertion, which is theirs to type: tiller sync --accept-fast-forward <node>.")
	require.Equal(t, "in_review", taskStatusCLI(t, soleTaskID(t)))

	out, _, code = execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", tid)
	require.Equal(t, 0, code)
	require.Contains(t, out, "Recorded on your assertion, not observation:")
	require.Contains(t, out, fmt.Sprintf("  - task %s (%s) -> done", cliTaskKey, tid))
	require.Contains(t, out, "    basis: human_attested")
	// The trailer names the branch tip rather than "this work": on the accepted
	// vacuous-branch case there is no work on the branch at all, and overstating
	// the observation is the one thing this block exists to avoid.
	require.Contains(t, out, "Tiller observed only that main already contains the branch tip, which a fast-forward "+
		"merge and a branch that never diverged both leave, so this rests on your word rather than on evidence.")
	require.NotContains(t, out, "Sync: no crossings recorded", "a run that recorded an assertion is not a zero-crossing run")

	// Parent rollups are observations, not human assertions over their own refs.
	observedBlock := out[:strings.Index(out, "Recorded on your assertion")]
	require.Contains(t, observedBlock, "Sync: recorded 2 done crossing(s):")
	require.Contains(t, observedBlock, "- epic form-engine")
	require.NotContains(t, observedBlock, tid, "the asserted task is never inside the observed count")

	require.Equal(t, "done", soleFeatureStatus(t))
	require.Equal(t, "human_attested", loadTestTree(t).Tasks[tid].DoneBasis)
	require.Equal(t, "rollup", featureDoneBasisCLI(t, cliFeatureKey))
}

// TestSyncAcceptFastForwardJSONCLI pins the machine surface: the crossing's basis
// is the single discriminator and carries the schema's token, and `attested`
// carries what the human named. The token is asserted against a real crossing
// rather than against the constant that renders it, so the CLI cannot drift from
// the core or from artifact-schema.md without this failing.
func TestSyncAcceptFastForwardJSONCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFastForwardedTaskToInReviewCLI(t, dir)
	declareIntegrationBranchCLI(t, dir)
	tid := soleTaskID(t)

	// Plain sync carries no attested block at all: the field records an input the
	// human did not give.
	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "\"attested\"")

	out, _, code = execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", tid, "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"basis\": \"human_attested\"")
	require.Contains(t, out, "\"attested\": [")
	require.Contains(t, out, "\""+tid+"\"")
	require.Contains(t, out, "\"status\": \"done\"")
	require.Contains(t, out, "\"into\": \"main\"")
	require.NotContains(t, out, "\"matched_commit\"", "an assertion matched no commit")
	require.NotContains(t, out, "\"human_attested\": true", "basis is the one discriminator; no boolean beside it")
}

// TestSyncAcceptFastForwardRefusalCLI is requirement 1 made visible instead of
// silent. The flag widens the basis Tiller will accept and never the set of
// branches it will cross, so a named node whose work main does not contain
// refuses the whole command, before any write, with a blocked data block and
// never an error block.
func TestSyncAcceptFastForwardRefusalCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	branch := driveTaskToInReviewOnItsBranchCLI(t, dir)
	gitRun(t, dir, "checkout", "main") // genuinely unmerged: main does not contain it
	declareIntegrationBranchCLI(t, dir)
	tid := soleTaskID(t)

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", tid, "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"reason\": \"not_attestable\"")
	require.Contains(t, out, "\"reason\": \"not_contained\"")
	require.Contains(t, out, "\"into\": \"main\"")
	require.Contains(t, out, "\"id\": \""+tid+"\"")
	require.Contains(t, out, "\"branch\": \""+branch+"\"")
	require.NotContains(t, out, "\"error\"", "a blocked answer carries a data block, never an error block")

	// Nothing was recorded: the classification runs before any write.
	require.Equal(t, "in_review", taskStatusCLI(t, soleTaskID(t)))
	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.False(t, hasEventCLI(t, tree, "sync.feature"), "a refused sync writes no crossing event")

	_, errOut, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", tid)
	require.Equal(t, 1, code)
	// The headline names the set, never one of its three conditions: a
	// containment headline was false for nothing_to_cross, which is reachable only
	// once containment has already been established.
	require.Contains(t, errOut, "Blocked: 1 named node(s) are outside the set --accept-fast-forward may cross against main.")
	require.Contains(t, errOut, fmt.Sprintf("  - task %s (%s) on %s: not contained in main", cliTaskKey, tid, branch))
	require.Contains(t, errOut, "Nothing was recorded, including for the named nodes that were eligible.")
	// The trailer restates the candidate set rather than naming one fix: the
	// remedy for a missing branch is not the remedy for unmerged work, and the
	// per-node line above is what says which condition failed.
	require.Contains(t, errOut, "--accept-fast-forward closes only a merge main already contains, on a "+
		"branch that still resolves, for a node with something in review to cross.")
}

// layOrphanedStateCommitCLI is the field case the accept-fast-forward recovery exists for, driven
// through the real command chain: the fixture task's branch is pushed and
// its pushed tip merged with --no-ff into main, exactly like a normal PR
// merge, and then the local branch gains one more commit that never reaches
// the remote, a state commit left behind after the PR that carried the
// earlier tip already merged (the same shape the own fixture
// produces, from the other side of one merge). It leaves the checkout on
// main, the task in review, and the integration branch declared, and returns
// the branch name.
func layOrphanedStateCommitCLI(t *testing.T, dir string) string {
	t.Helper()
	driveFeatureToReadyCLI(t)
	tid := submitTaskOnFeatureCLI(t, cliFeatureKey)
	branch := loadTestTree(t).Tasks[tid].Branch
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "record task review")

	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")
	gitRun(t, dir, "remote", "add", "origin", bare)
	gitRun(t, dir, "push", "-u", "origin", branch)

	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "merge", "--no-ff", "-m", "merge feature", branch)

	gitRun(t, dir, "checkout", branch)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.txt"), []byte("state commit\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "state commit")
	gitRun(t, dir, "checkout", "main")

	declareIntegrationBranchCLI(t, dir)
	return branch
}

// TestSyncAcceptFastForwardRecoveryRendersBranchDashFArmCLI is the
// field case rendered end to end: a state commit left unpushed after its PR
// already merged refuses --accept-fast-forward as not_contained, and the
// refusal now prescribes the honest fix instead of leaving the human to
// invent one. Nothing holds the branch checked out anywhere, so the plain
// `git branch -f` form renders, on both the JSON and the human surface.
func TestSyncAcceptFastForwardRecoveryRendersBranchDashFArmCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	branch := layOrphanedStateCommitCLI(t, dir)
	tid := soleTaskID(t)
	remoteTip := gitOut(t, dir, "rev-parse", "--short", "origin/"+branch)
	localTip := gitOut(t, dir, "rev-parse", "--short", branch)

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", tid, "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "\"reason\": \"not_contained\"")
	require.Contains(t, out, "\"recovery\": {")
	require.Contains(t, out, "\"remote_tip\": \""+remoteTip+"\"")
	require.Contains(t, out, "\"local_tip\": \""+localTip+"\"")
	require.Contains(t, out, "\"upstream\": \"origin/"+branch+"\"")
	require.NotContains(t, out, "\"worktree\"", "nothing holds the branch checked out in this fixture")
	require.Contains(t, out, "git branch recovery/"+cliTaskKey+"-in-review "+localTip)
	require.Contains(t, out, "git branch -f "+branch+" origin/"+branch)
	require.Contains(t, out, "tiller sync --accept-fast-forward "+cliTaskKey)

	_, errOut, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", tid)
	require.Equal(t, 1, code)
	require.Contains(t, errOut, fmt.Sprintf("recovery for %s (%s) on %s:", cliTaskKey, tid, branch))
	require.Contains(t, errOut, fmt.Sprintf(
		"the merged remote tip %s is contained in main; the local tip %s is not.", remoteTip, localTip))
	require.Contains(t, errOut, "1. preserve the local tip: git branch recovery/"+cliTaskKey+"-in-review "+localTip)
	require.Contains(t, errOut, "2. realign the branch: git branch -f "+branch+" origin/"+branch)
	require.NotContains(t, errOut, "would refuse", "nothing holds the branch checked out, so branch -f is honest")
	require.Contains(t, errOut, "3. rerun: tiller sync --accept-fast-forward "+cliTaskKey)
}

// TestSyncAcceptFastForwardRecoveryRendersWorktreeArmCLI is the other arm: a
// worktree holds the orphaned branch checked out, the shape that would make
// `git branch -f` itself refuse, so the recovery names
// `git -C <worktree> reset --hard` instead and says why, on both surfaces.
func TestSyncAcceptFastForwardRecoveryRendersWorktreeArmCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	branch := layOrphanedStateCommitCLI(t, dir)
	wtDir := t.TempDir()
	gitRun(t, dir, "worktree", "add", wtDir, branch)
	wantWorktree, err := filepath.EvalSymlinks(wtDir)
	require.NoError(t, err)

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", soleTaskID(t), "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "\"worktree\": \""+wantWorktree+"\"")
	require.Contains(t, out, "git -C "+wantWorktree+" reset --hard origin/"+branch)

	_, errOut, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", soleTaskID(t))
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "2. realign the branch: git -C "+wantWorktree+" reset --hard origin/"+branch)
	require.Contains(t, errOut, "(the branch is checked out in that worktree; git branch -f would refuse)")
}

func TestSyncFastForwardAssertionNamesTaskNotParentCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFastForwardedTaskToInReviewCLI(t, dir)
	declareIntegrationBranchCLI(t, dir)
	tid := soleTaskID(t)
	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", cliFeatureKey, "--json")
	require.Equal(t, 1, code, out)
	require.Equal(t, "in_review", taskStatusCLI(t, tid))
	out, _, code = execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", tid, "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, "done", taskStatusCLI(t, tid))
	require.Equal(t, "human_attested", loadTestTree(t).Tasks[tid].DoneBasis)
}

// TestSyncAcceptFastForwardIntoRefusalCLI is the second half of the safety
// property at the command surface. The flag asserts about the project's declared
// trunk, so a run that resolved anything else refuses before a node is even
// classified, with a blocked data block and its own reason token.
//
// The fixture is the one that used to record done for work main never saw: the
// feature branch is genuinely unmerged, and --into names a release cut taken off
// it, which contains the work for a reason that has nothing to do with the trunk.
func TestSyncAcceptFastForwardIntoRefusalCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	branch := driveTaskToInReviewOnItsBranchCLI(t, dir)
	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "branch", "release-cut", branch)
	declareIntegrationBranchCLI(t, dir)

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--into", "release-cut",
		"--accept-fast-forward", soleTaskID(t), "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"reason\": \"into_not_integration_branch\"")
	require.Contains(t, out, "\"into\": \"release-cut\"")
	require.Contains(t, out, "\"source\": \"--into\"")
	require.Contains(t, out, "\"declared\": \"main\"")
	require.NotContains(t, out, "\"error\"", "a blocked answer carries a data block, never an error block")
	require.Equal(t, "in_review", taskStatusCLI(t, soleTaskID(t)))

	_, errOut, code := execCLI(t, "sync", "--contributor", "jc", "--into", "release-cut",
		"--accept-fast-forward", soleTaskID(t))
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "Blocked: --accept-fast-forward asserts about main, the declared integration "+
		"branch, and this run resolved release-cut (from --into).")
	require.Contains(t, errOut, "Re-run with --into main, or drop --accept-fast-forward to observe merges into release-cut.")

	// Plain observation against the same ref is untouched: only the assertion is
	// held to the declared branch.
	_, _, code = execCLI(t, "sync", "--contributor", "jc", "--into", "release-cut")
	require.Equal(t, 0, code)
	require.Equal(t, "in_review", taskStatusCLI(t, soleTaskID(t)))
}

// TestSyncAcceptFastForwardNeedsADeclaredBranchCLI is the same refusal seen from
// its other side: a project that declares no git.integration_branch has no trunk
// for the assertion to be about, so the flag refuses and names the one line to
// add. Plain sync still walks the HEAD fallback, which is the compatibility spine
// for every project that predates the field.
func TestSyncAcceptFastForwardNeedsADeclaredBranchCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFastForwardedTaskToInReviewCLI(t, dir)

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", soleTaskID(t), "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "\"reason\": \"into_not_integration_branch\"")
	require.Contains(t, out, "\"source\": \"HEAD\"")
	require.NotContains(t, out, "\"declared\"", "there is nothing declared, which is the whole refusal")
	require.Equal(t, "in_review", taskStatusCLI(t, soleTaskID(t)))

	_, errOut, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", soleTaskID(t))
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "Set git.integration_branch in .tiller/tiller.yml, then re-run.")

	_, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code, "observation is untouched by the flag's requirement")
	require.Equal(t, "in_review", taskStatusCLI(t, soleTaskID(t)))
}

// Unknown references remain usage errors, including unknown task IDs.
func TestSyncAcceptFastForwardUnknownRefIsMisuseCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFastForwardedTaskToInReviewCLI(t, dir)

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", "form-engine/no-such-thing", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, "\"ok\": false")

	out, _, code = execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", "T-zzzzzz", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, "\"ok\": false")

	// A refused call is a no-op: the stranded node is still stranded.
	require.Equal(t, "in_review", taskStatusCLI(t, soleTaskID(t)))
	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.False(t, hasEventCLI(t, tree, "sync.feature"))
}

// TestSyncMixedObservedAndAssertedCLI is the readout when one feature was merged
// with a merge commit and another was fast-forwarded. Both blocks print, the
// observed header counts the observed crossing alone, and the asserted node sits
// under its own header with its basis on the line. A reader must never have to
// work out which of the two a given node was.
func TestSyncMixedObservedAndAssertedCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)
	driveSecondFeatureToReadyCLI(t)

	observedTask := submitTaskOnFeatureCLI(t, cliFeatureKey)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "one.txt"), []byte("one\n"), 0o644))
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[observedTask].Branch)
	assertedID := submitTaskOnFeatureCLI(t, secondFeatureKey)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "two.txt"), []byte("two\n"), 0o644))
	fastForwardTaskBranchCLI(t, dir, loadTestTree(t).Tasks[assertedID].Branch)
	declareIntegrationBranchCLI(t, dir)
	observedID := featureIDByKey(t, cliFeatureKey)

	ledger := linkSyncBaselineCLI(t, dir, [][2]string{
		{cliFeatureKey, "one.txt"}, {secondFeatureKey, "two.txt"},
	})
	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--accept-fast-forward", assertedID)
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "Sync: recorded 4 done crossing(s):")
	require.Contains(t, out, fmt.Sprintf("  - feature %s (%s) -> done", cliFeatureKey, observedID))
	require.Contains(t, out, "Recorded on your assertion, not observation:")
	require.Contains(t, out, fmt.Sprintf("  - task %s/transcribe-the-schema (%s) -> done", secondFeatureKey, assertedID))
	require.Contains(t, out, "    basis: human_attested")
	notice := "as-is docs touched: design/as-is/one.txt.md, design/as-is/project.md, design/as-is/two.txt.md; run tiller drift"
	require.Equal(t, 1, strings.Count(out, "as-is docs touched:"))
	require.Contains(t, out, notice)
	require.Greater(t, strings.Index(out, notice), strings.LastIndex(out, "-> done"),
		"one union follows both observed and asserted crossings")
	currentLedger, err := os.ReadFile(filepath.Join(dir, store.BaselineRelPath))
	require.NoError(t, err)
	require.Equal(t, ledger, currentLedger)

	// The asserted node is below the assertion header, not inside the observed
	// count: the observed block ends where that header begins.
	observedBlock := out[:strings.Index(out, "Recorded on your assertion")]
	require.Contains(t, observedBlock, observedID)
	require.NotContains(t, observedBlock, assertedID)

	require.Equal(t, "rollup", featureDoneBasisCLI(t, cliFeatureKey))
	require.Equal(t, "rollup", featureDoneBasisCLI(t, secondFeatureKey))
	require.Equal(t, "human_attested", loadTestTree(t).Tasks[assertedID].DoneBasis)

	jout, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, jout, "\"crossings\": []", "both nodes are done; a second sync crosses nothing")
	require.NotContains(t, jout, "touched_design", "a no-op does not repeat prior notices")
}

// TestSyncRecordsTheBasisItCrossedOnCLI confirms an ancestry crossing says so in
// the envelope and carries no matched commit, so the weaker content evidence is
// always distinguishable from the strong path in the data an agent reads back.
func TestSyncRecordsTheBasisItCrossedOnCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)

	tid := submitTaskOnFeatureCLI(t, cliFeatureKey)
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)
	ledger := linkSyncBaselineCLI(t, dir, [][2]string{{cliFeatureKey, "work.txt"}})

	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	var envelope struct {
		Data SyncView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	featureSeen := false
	for _, crossing := range envelope.Data.Crossings {
		if crossing.Noun == store.NounFeature {
			featureSeen = true
			require.Equal(t, []string{"as-is/project.md", "as-is/work.txt.md"}, crossing.TouchedDesign)
		} else {
			require.Empty(t, crossing.TouchedDesign, crossing.Noun)
		}
	}
	require.True(t, featureSeen, "the JSON includes a real feature done crossing")
	currentLedger, err := os.ReadFile(filepath.Join(dir, store.BaselineRelPath))
	require.NoError(t, err)
	require.Equal(t, ledger, currentLedger)
	require.Contains(t, out, "\"basis\": \"ancestry\"")
	require.NotContains(t, out, "\"matched_commit\"", "only a content crossing names the commit it matched")
	require.Contains(t, out, "\"into\": \"main\"")
	// Where `into` came from is on the success envelope too, not only the
	// refusal. A project with no manifest field is silently observing whatever
	// branch it is standing on, and this is the one field that says so.
	require.Contains(t, out, "\"into_source\": \"HEAD\"")

	// The human line for an ancestry crossing is unchanged: no content annotation.
	require.Equal(t, "done", soleFeatureStatus(t))
}

// TestActiveBasisRefusesCheckCLI exercises the closed seam through the CLI: after a
// recorded basis, an edited-and-committed feature.md makes `tiller check` refuse
// with an exit-1 active_basis blocked answer (ok:false, a data block, no error
// block), cleared by approve --refresh-basis.
func TestActiveBasisRefusesCheckCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "lay tree")

	// Record the accepted basis at the committed feature.md.
	_, _, code := execCLI(t, "approve", "feature", cliFeatureKey, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, code)

	// Edit feature.md and commit, drifting the basis.
	mdPath := filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("# Certification form loading\n\nRevised spec.\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "revise spec")

	out, _, code := execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"active_basis\"")
	require.Contains(t, out, "feature.md")
	require.NotContains(t, out, "\"error\"")

	// Refresh the basis after review; check no longer refuses.
	_, _, code = execCLI(t, "approve", "feature", cliFeatureKey, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
}

// TestSyncRendersTheReopenedNodesInBothSurfaces covers the reverse direction's
// whole emitted contract, which Tasks 9 and 11 will consume: the human block
// naming what moved back, and the `reopened` array in the --json data block. It
// matters that both exist at all. A sync that quietly walks a done feature and
// its epic back to work while printing only a crossing count would be a command
// changing state the developer never sees, and reopening is the one thing sync
// does on a run nobody asked it to do anything on.
//
// Both are asserted apart from `crossings` on purpose: every entry there means
// "this reached done", and a caller must never have to read a status field to
// learn which direction an entry went.
//
// The test drives two full reopen cycles, because that is the only honest way
// to see both surfaces do the work: a reopen is idempotent, so the run that
// renders it is the one where new work first lands, and there is exactly one
// such run per cycle.
func TestSyncRendersTheReopenedNodesInBothSurfaces(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)

	addWorkedTask := func(localKey, title string) string {
		t.Helper()
		_, _, code := execCLI(t, "add", "task", cliFeatureKey+"/"+localKey, "--title", title, "--contributor", "jc")
		require.Equal(t, 0, code)
		tid := taskIDByKey(t, cliFeatureKey+"/"+localKey)
		for _, argv := range [][]string{
			{"approve", "task", tid, "--contributor", "jc"},
			{"start", "task", tid, "--contributor", "jc"},
			{"check", "task", tid, "--contributor", "jc"},
			{"submit", "task", tid, "--pr", "41", "--contributor", "jc"},
		} {
			_, _, code = execCLI(t, argv...)
			require.Equalf(t, 0, code, "%v", argv)
			if argv[0] == "start" {
				checkoutTaskBranchCLI(t, tid)
			}
		}
		return tid
	}

	addWorkedTask("transcribe-the-schema", "Transcribe the schema")
	_, _, code := execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[soleTaskID(t)].Branch)

	// First sync: the task crosses, the feature rolls up, the epic follows.
	_, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	fid, eid := soleFeatureID(t), soleEpicID(t)
	require.Equal(t, "done", featureStatusCLI(t, cliFeatureKey))

	// Cycle 1, the human surface. New work lands under the done feature, so this
	// run walks both it and the epic above it back.
	_, _, code = execCLI(t, "add", "task", cliFeatureKey+"/one-more-thing", "--title", "One more thing", "--contributor", "jc")
	require.Equal(t, 0, code)
	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Reopened by work that landed beneath them (2):")
	require.Contains(t, out, fmt.Sprintf("  - feature %s (%s) -> active", cliFeatureKey, fid))
	require.Contains(t, out, fmt.Sprintf("  - epic %s (%s) -> ready", "form-engine", eid))
	require.NotContains(t, out, "Sync: recorded",
		"nothing crossed on this run; a reopen is never counted as a crossing")

	// The reverse is a recompute, not a repeat: the very next sync, with the
	// tree unchanged, moves nothing and says nothing.
	quiet, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NotContains(t, quiet, "Reopened by work")

	// Cycle 2, the JSON surface. Finish the outstanding task so the tree
	// re-crosses, then land another one so a reopen renders again.
	tid := taskIDByKey(t, cliFeatureKey+"/one-more-thing")
	claimTaskAsOwner(t, tid, "jc")
	for _, argv := range [][]string{
		{"approve", "task", tid, "--contributor", "jc"},
		{"start", "task", tid, "--contributor", "jc"},
		{"check", "task", tid, "--contributor", "jc"},
		{"submit", "task", tid, "--pr", "41", "--contributor", "jc"},
	} {
		_, _, code = execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
		if argv[0] == "start" {
			checkoutTaskBranchCLI(t, tid)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "second-task.txt"), []byte("independent work\n"), 0o644))
			gitRun(t, dir, "add", "second-task.txt")
			gitRun(t, dir, "commit", "-m", "second task work")
		}
	}
	// The new task cannot borrow the first task's observed merge.
	unmerged, _, code := execCLI(t, "sync", "--into", "main", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, unmerged)
	require.Empty(t, syncCrossings(t, unmerged))
	require.Equal(t, "in_review", taskStatusCLI(t, tid))
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)
	_, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Equal(t, "done", featureStatusCLI(t, cliFeatureKey), "the tree re-crossed")

	_, _, code = execCLI(t, "add", "task", cliFeatureKey+"/and-another", "--title", "And another", "--contributor", "jc")
	require.Equal(t, 0, code)

	jout, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	var env struct {
		Data struct {
			Crossings []map[string]any `json:"crossings"`
			Reopened  []struct {
				Noun   string `json:"noun"`
				ID     string `json:"id"`
				Key    string `json:"key"`
				Status string `json:"status"`
			} `json:"reopened"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(jout), &env))
	require.Empty(t, env.Data.Crossings, "a reopen never lands in the crossings array")
	require.Len(t, env.Data.Reopened, 2)
	require.Equal(t, "feature", env.Data.Reopened[0].Noun)
	require.Equal(t, fid, env.Data.Reopened[0].ID)
	require.Equal(t, cliFeatureKey, env.Data.Reopened[0].Key)
	require.Equal(t, "active", env.Data.Reopened[0].Status)
	require.Equal(t, "epic", env.Data.Reopened[1].Noun)
	require.Equal(t, eid, env.Data.Reopened[1].ID)
	require.Equal(t, "form-engine", env.Data.Reopened[1].Key)
	require.Equal(t, "ready", env.Data.Reopened[1].Status)
}

// featureStatusCLI reads one feature's stored status off disk by key.
func featureStatusCLI(t *testing.T, key string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	for _, f := range tree.Features {
		if f.Key != nil && *f.Key == key {
			return f.Status
		}
	}
	t.Fatalf("no feature %s", key)
	return ""
}

// taskIDByKey is the id of the task at one canonical three-segment key. Folders
// carry no add order, so a test names the task it means rather than reaching for
// "the last one written".
func taskIDByKey(t *testing.T, canonical string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	for _, tk := range tree.Tasks {
		if tk.Key != nil && *tk.Key == canonical {
			return tk.ID
		}
	}
	t.Fatalf("no task at key %s", canonical)
	return ""
}

// TestSyncNamesTheSkippedEpicInBothSurfaces is the rendering half of the skip.
// The node's work really is complete, so nothing else on any surface will ever
// explain why it is still sitting at ready: the status oracle shows a ready
// epic, and without this block sync prints its
// clean-run line over the top of it. cli-surface.md's done-crossing section
// settles that a crossing which did not happen must be named rather than left
// indistinguishable from a clean run, and this is that rule at epic grain.
func TestSyncNamesTheSkippedEpicInBothSurfaces(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)

	_, _, code := execCLI(t, "add", "task", cliFeatureKey+"/transcribe-the-schema", "--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := taskIDByKey(t, cliFeatureKey+"/transcribe-the-schema")
	for _, argv := range [][]string{
		{"approve", "task", tid, "--contributor", "jc"},
		{"start", "task", tid, "--contributor", "jc"},
		{"check", "task", tid, "--contributor", "jc"},
		{"submit", "task", tid, "--pr", "41", "--contributor", "jc"},
		{"check", "feature", cliFeatureKey, "--contributor", "jc"},
	} {
		_, _, code = execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
		if argv[0] == "start" {
			checkoutTaskBranchCLI(t, tid)
		}
	}

	// The epic's prose goes missing before the merge is observed, so its rollup
	// is earned on this very run and cannot be recorded.
	epicMD := filepath.Join(dir, ".tiller", "epics", "form-engine", "epic.md")
	require.NoError(t, os.Remove(epicMD))
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)
	eid := soleEpicID(t)

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Earned a crossing but could not be recorded (1):")
	require.Contains(t, out, fmt.Sprintf("  - epic form-engine (%s): git could not resolve .tiller/epics/form-engine/epic.md", eid))
	require.Contains(t, out, "will cross on the next sync once its path does resolve")

	// The second run is the one the finding was about: nothing left to cross,
	// nothing in review, so this is where a silent skip would hide behind the
	// clean-run line. It must still name the epic, and must not claim a clean run.
	quiet, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	var env struct {
		Data struct {
			Crossings []map[string]any `json:"crossings"`
			Skipped   []struct {
				Noun string `json:"noun"`
				ID   string `json:"id"`
				Key  string `json:"key"`
				Path string `json:"path"`
			} `json:"skipped"`
			Readout string `json:"readout"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(quiet), &env))
	require.Empty(t, env.Data.Crossings, "the epic did not cross; it is not a crossing")
	require.Len(t, env.Data.Skipped, 1)
	require.Equal(t, "epic", env.Data.Skipped[0].Noun)
	require.Equal(t, eid, env.Data.Skipped[0].ID)
	require.Equal(t, "form-engine", env.Data.Skipped[0].Key)
	require.Equal(t, ".tiller/epics/form-engine/epic.md", env.Data.Skipped[0].Path)

	human, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NotContains(t, human, "Sync: no merged branches to record",
		"the clean-run line must not print over a node that could not be recorded")
	require.Contains(t, human, "Earned a crossing but could not be recorded (1):")

	require.NotContains(t, human, "is not under version control")

	// Restoring the file on integration lets the epic cross on the next run, so
	// the block was a diagnosis rather than a dead end.
	require.NoError(t, os.WriteFile(epicMD, []byte("# Form Engine\n\nRestored prose.\n"), 0o644))
	gitRun(t, dir, "add", "--", epicMD)
	gitRun(t, dir, "commit", "-m", "restore epic prose")
	fixed, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NotContains(t, fixed, "could not be recorded")
	require.Contains(t, fixed, fmt.Sprintf("- epic form-engine (%s) -> done", eid))
}

// TestSyncRecordsIntegrationProseDespiteAWorkingTreeDelete is defect 03 at the
// CLI surface: a completion record reads the integration commit, so prose that
// integration holds is recorded even when the invoking checkout has deleted it.
// The working-tree delete is not a skip and is not recorded as anything else.
func TestSyncRecordsIntegrationProseDespiteAWorkingTreeDelete(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)

	_, _, code := execCLI(t, "add", "task", cliTaskKey, "--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := taskIDByKey(t, cliTaskKey)
	for _, argv := range [][]string{
		{"approve", "task", tid, "--contributor", "jc"},
		{"start", "task", tid, "--contributor", "jc"},
		{"check", "task", tid, "--contributor", "jc"},
		{"submit", "task", tid, "--pr", "41", "--contributor", "jc"},
		{"check", "feature", cliFeatureKey, "--contributor", "jc"},
	} {
		_, _, code = execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
		if argv[0] == "start" {
			checkoutTaskBranchCLI(t, tid)
		}
	}
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)

	const epicRel = ".tiller/epics/form-engine/epic.md"
	want := "git:" + gitOut(t, dir, "rev-parse", "main:"+epicRel)
	require.NoError(t, os.Remove(filepath.Join(dir, epicRel)))

	eid := soleEpicID(t)
	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "could not be recorded")
	require.Contains(t, out, fmt.Sprintf("- epic form-engine (%s) -> done", eid))
	require.Equal(t, want, loadTestTree(t).Epics[eid].VerifiedAgainst.Spec.Value)
}

// checksDueTree builds a minimal tree carrying one feature at the key and id
// the ChecksDue fixtures below reference, so gen.RefByID resolves it to a full
// reference ("foundation/repo-tooling (F-dhzdb2)") rather than falling back to
// the bare id.
func checksDueTree() *store.Tree {
	key := "foundation/repo-tooling"
	return &store.Tree{
		Features: map[string]*store.Feature{
			"F-dhzdb2": {ID: "F-dhzdb2", Key: &key},
		},
	}
}

// checksDueFixture is the ChecksDue entry the tests below assert the
// confirmation and JSON envelope name: one feature with every task done and a
// required feature-grain gate left unrecorded (mutate's own
// TestChecksDueNamesAwaitingCheckFeaturesOnly proves the core computes this
// shape; these tests are the rendering half).
var checksDueFixture = []mutate.CheckDue{
	{Noun: store.NounFeature, ID: "F-dhzdb2", Key: "foundation/repo-tooling"},
}

// TestSyncHumanNamesChecksDueOnCleanRun is the zero-crossing outcome with
// nothing else to report: no crossings, nothing in review, nothing reopened,
// nothing skipped, and one feature parked awaiting its check. The clean-run
// line and the checks-due block are not a contradiction, since there really is
// nothing to merge, so both render together.
func TestSyncHumanNamesChecksDueOnCleanRun(t *testing.T) {
	tree := checksDueTree()
	out := humanSync(tree, mutate.SyncResult{ChecksDue: checksDueFixture})
	require.Contains(t, out, "Awaiting feature check: 1 feature(s) with every task done and required gates unrecorded:")
	require.Contains(t, out, "  - feature foundation/repo-tooling (F-dhzdb2); run: tiller check feature F-dhzdb2")
}

// TestSyncHumanNamesChecksDueSurvivesCleanRunSuppression hits humanSync's
// OTHER zero-crossing return: nothing in review, but something reopened, so
// the clean-run line is suppressed ("no merged branches to record" would
// contradict a reopen this run just recorded). The checks-due block is not
// that line, so it is not part of what gets suppressed and still renders.
func TestSyncHumanNamesChecksDueSurvivesCleanRunSuppression(t *testing.T) {
	tree := checksDueTree()
	otherKey := "form-engine/certification-form-loading"
	tree.Features["F-bbbbbb"] = &store.Feature{ID: "F-bbbbbb", Key: &otherKey}
	res := mutate.SyncResult{
		ChecksDue: checksDueFixture,
		Reopened:  []mutate.Reopening{{Noun: store.NounFeature, ID: "F-bbbbbb", Key: otherKey, Status: "active"}},
	}
	out := humanSync(tree, res)
	require.Contains(t, out, "Reopened by work that landed beneath them (1):")
	require.NotContains(t, out, "Sync: no merged branches to record")
	require.Contains(t, out, "Awaiting feature check: 1 feature(s)")
	require.Contains(t, out, "run: tiller check feature F-dhzdb2")
}

// TestSyncHumanNamesChecksDueOnCrossingPath is the crossing branch: a task
// crosses this run and, in the same walk, another feature is left parked
// awaiting its check. Both blocks render, because the crossing that reads as
// done and the feature strand it can leave behind are exactly the two facts
// the incident this feature fixes says must never separate.
func TestSyncHumanNamesChecksDueOnCrossingPath(t *testing.T) {
	tree := checksDueTree()
	taskKey := "form-engine/certification-form-loading/transcribe-the-schema"
	tree.Tasks = map[string]*store.Task{"T-aaaaaa": {ID: "T-aaaaaa", Key: &taskKey}}
	res := mutate.SyncResult{
		Crossings: []mutate.Crossing{
			{Noun: store.NounTask, ID: "T-aaaaaa", Key: taskKey, Status: "done", Basis: "ancestry"},
		},
		ChecksDue: checksDueFixture,
	}
	out := humanSync(tree, res)
	require.Contains(t, out, "Sync: recorded 1 done crossing(s):")
	require.Contains(t, out, "Awaiting feature check: 1 feature(s) with every task done and required gates unrecorded:")
	require.Contains(t, out, "run: tiller check feature F-dhzdb2")
}

// TestSyncHumanOmitsChecksDueBlockWhenEmpty confirms joinSyncBlocks' empty-
// string skip actually makes the new block invisible on the ordinary run: an
// empty ChecksDue must not print a header with a zero count or leave a
// dangling blank block.
func TestSyncHumanOmitsChecksDueBlockWhenEmpty(t *testing.T) {
	tree := checksDueTree()
	out := humanSync(tree, mutate.SyncResult{})
	require.NotContains(t, out, "Awaiting feature check")
}

// TestSyncViewChecksDueJSON is the machine surface: the envelope carries
// checks_due with noun/id/key, present only when ChecksDue is non-empty
// (omitempty keeps an ordinary sync's envelope unchanged).
func TestSyncViewChecksDueJSON(t *testing.T) {
	raw, err := json.Marshal(syncView(mutate.SyncResult{ChecksDue: checksDueFixture}))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"checks_due"`)
	require.Contains(t, string(raw), `"noun":"feature"`)
	require.Contains(t, string(raw), `"id":"F-dhzdb2"`)
	require.Contains(t, string(raw), `"key":"foundation/repo-tooling"`)

	emptyRaw, err := json.Marshal(syncView(mutate.SyncResult{}))
	require.NoError(t, err)
	require.NotContains(t, string(emptyRaw), "checks_due",
		"omitempty keeps an ordinary sync's envelope byte-for-byte as it was before ChecksDue existed")
}

// TestSyncAcceptFastForwardResolvesBugRefs pins the flag's widened reference
// class: it names features, bugs, and chores, so a shelf key reaches the core
// instead of being turned away by the resolver as a feature that does not
// exist. Misuse is still misuse: a reference that resolves to nothing at all
// exits 2 exactly as it always did.
func TestSyncAcceptFastForwardResolvesBugRefs(t *testing.T) {
	initOwnedRepo(t)
	// The flag refuses outright unless the run resolved the DECLARED
	// integration branch, and that refusal lands before any node is
	// classified, so without this the assertion below would never reach the
	// resolver it is about (initOwnedRepo chdirs into the repo, so "." is it).
	declareIntegrationBranchCLI(t, ".")
	mustRun(t, "add", "bug", "alpha", "--title", "Alpha", "--contributor", "jc")
	// An unknown ref is still misuse; a bug ref reaches the core (which then
	// refuses on eligibility, exit 1, since nothing is in review).
	_, _, code := execCLI(t, "sync", "--accept-fast-forward", "nope", "--contributor", "jc")
	require.Equal(t, 2, code)
	out, _, code := execCLI(t, "sync", "--accept-fast-forward", "bugs/alpha", "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "B-")
	mustRun(t, "add", "tweak", "spacing", "--title", "Spacing", "--contributor", "jc")
	tree := loadCwdTree(t)
	var tweakID string
	for id := range tree.Tweaks {
		tweakID = id
	}
	for _, ref := range []string{"tweaks/spacing", "spacing", tweakID} {
		out, _, code = execCLI(t, "sync", "--accept-fast-forward", ref, "--contributor", "jc", "--json")
		require.Equal(t, 1, code, out)
		require.Contains(t, out, tweakID)
	}
}

func linkSyncBaselineCLI(t *testing.T, dir string, features [][2]string) []byte {
	t.Helper()
	manifestPath := filepath.Join(dir, ".tiller/tiller.yml")
	manifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, append(manifest, []byte("\ninputs:\n  code:\n    - {root: '.', mode: surface}\n")...), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller/design/as-is"), 0o755))
	docs := [][2]string{{"as-is/project.md", "."}}
	for _, feature := range features {
		out, _, code := execCLI(t, "link", "feature", feature[0], "--surface", feature[1], "--contributor", "jc")
		require.Equal(t, 0, code, out)
		docs = append(docs, [2]string{"as-is/" + feature[1] + ".md", feature[1]})
	}
	for _, doc := range docs {
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller/design", doc[0]), []byte("# As-is\n"), 0o644))
		out, _, code := execCLI(t, "link", "design", doc[0], "--describes", doc[1], "--contributor", "jc")
		require.Equal(t, 0, code, out)
	}
	out, _, code := execCLI(t, "approve", "design", "as-is/project.md", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	ledger, err := os.ReadFile(filepath.Join(dir, store.BaselineRelPath))
	require.NoError(t, err)
	return ledger
}

// layEvalContractMismatchCLI merges a feature's task, then refreshes the
// feature's eval in the working tree and repins accepted_against.eval to it
// without committing, so the integration commit's eval is not the accepted
// one. It returns the feature id with the accepted and integration blobs.
func layEvalContractMismatchCLI(t *testing.T) (string, string, string) {
	t.Helper()
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)
	tid := submitTaskOnFeatureCLI(t, cliFeatureKey)
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)

	featDir := filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading")
	evalRel := filepath.Join(featDir, "feature.eval.md")
	require.NoError(t, os.WriteFile(filepath.Join(dir, evalRel),
		[]byte("---\nnode: x\ncriteria: []\n---\n\n# Refreshed, uncommitted eval\n"), 0o644))
	accepted := "git:" + gitOut(t, dir, "hash-object", evalRel)
	verified := "git:" + gitOut(t, dir, "rev-parse", "main:"+filepath.ToSlash(evalRel))
	require.NotEqual(t, accepted, verified)

	ymlPath := filepath.Join(dir, featDir, "feature.yml")
	data, err := os.ReadFile(ymlPath)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.AcceptedAgainst.Eval.Value = accepted
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(ymlPath, out, 0o644))
	return featureIDByKey(t, cliFeatureKey), accepted, verified
}

// Final review item 1 at the CLI: an eval contract mismatch at a feature
// crossing rides the crossing in JSON and prints an advisory block naming the
// feature, while the crossing itself still succeeds.
func TestSyncNamesAnEvalContractMismatchCLI(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		fid, accepted, verified := layEvalContractMismatchCLI(t)
		out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
		require.Equal(t, 0, code, out)
		var envelope struct {
			Data SyncView `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &envelope))
		seen := false
		for _, c := range envelope.Data.Crossings {
			if c.Noun != store.NounFeature {
				require.Nil(t, c.EvalContractMismatch, c.Noun)
				continue
			}
			seen = true
			require.Equal(t, fid, c.ID)
			require.Equal(t, "done", c.Status)
			require.Equal(t, &EvalContractMismatchView{Accepted: accepted, Verified: verified}, c.EvalContractMismatch)
		}
		require.True(t, seen)
	})
	t.Run("human", func(t *testing.T) {
		fid, accepted, verified := layEvalContractMismatchCLI(t)
		out, _, code := execCLI(t, "sync", "--contributor", "jc")
		require.Equal(t, 0, code, out)
		require.Contains(t, out, fmt.Sprintf("  - feature %s (%s) -> done", cliFeatureKey, fid))
		require.Contains(t, out, "Eval contract differs from the accepted one (advisory):")
		require.Contains(t, out, fmt.Sprintf("  - feature %s (%s): accepted %s, integration %s",
			cliFeatureKey, fid, accepted, verified))
	})
}

// A matching contract adds nothing to either surface.
func TestSyncMatchingEvalContractIsSilentCLI(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)
	tid := submitTaskOnFeatureCLI(t, cliFeatureKey)
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)
	out, _, code := execCLI(t, "sync", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "\"status\": \"done\"")
	require.NotContains(t, out, "eval_contract_mismatch")
}
