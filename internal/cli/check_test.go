package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// gatedGitManifest is a manifest carrying the required command gates, with build
// and tests bound to a substitutable shell command so a test can make the run pass
// or fail. The evals gate is the `tiller eval` recursion point, recorded pending.
const gatedGitManifest = `schema_version: 12
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
    tests: { kind: command, run: "true", on: [feature, task] }
    evals: { kind: command, run: "tiller eval ${node.noun} ${node.id}", on: [feature, task] }
`

// gitRun runs a git command in dir and fails the test on error.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
}

// initGatedGitRepo lays a temp git repo with one commit (so HEAD exists for the
// against revision) and a gated manifest, then chdirs into it. buildRun is the
// shell command the build gate runs.
func initGatedGitRepo(t *testing.T, buildRun string) string {
	t.Helper()
	dir := t.TempDir()
	testgit.Init(t, dir)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "tiller.yml"),
		[]byte(fmt.Sprintf(gatedGitManifest, buildRun)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644))
	gitRun(t, dir, "add", "README.md")
	gitRun(t, dir, "commit", "-m", "init")

	t.Chdir(dir)
	return dir
}

// initEvalSeededGitRepo lays a temp git repo with one commit, then runs the real
// `tiller init` so the manifest is exactly the seeded one (evals required gate
// active, build and tests commented). It is the fixture for confirming a freshly
// inited project drives end to end through the default acceptance gate.
func initEvalSeededGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testgit.Init(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644))
	gitRun(t, dir, "add", "README.md")
	gitRun(t, dir, "commit", "-m", "init")

	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "Test Project", "--key", "test", "--kind", "greenfield")
	require.Equal(t, 0, code)
	// init seeds no owner now; a human registers jc, then join records the
	// identity the gate runs attribute to.
	registerContributor(t, "jc", "JC Grubbs")
	_, _, code = execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)
	return dir
}

// TestCheckFeaturePassesThroughSeededEvalsGate is the Fix 1c end-to-end: a freshly
// inited project (evals required gate active) drives a specced, green feature
// through `tiller check` without the active evals gate blocking it. The gate is
// recorded required, and its status is now DERIVED from the eval contract's
// criteria rather than a bare pending seam: the feature's single criterion is not
// required, so no required criterion is unsatisfied and the gate is green.
func TestCheckFeaturePassesThroughSeededEvalsGate(t *testing.T) {
	initEvalSeededGitRepo(t)
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/nrca-form-loading",
		"--title", "NRCA form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	claimTaskAsOwner(t, cliFeatureKey+"/transcribe-the-schema", "jc")
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	_, stderr, code := execCLI(t, "start", "task", cliFeatureKey+"/transcribe-the-schema", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	out, _, code := execCLI(t, "check", "feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, "a specced, green feature passes through the active evals gate")
	require.Contains(t, out, "\"passed\": true")
	require.Contains(t, out, "\"name\": \"evals\"")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	g := tree.Features[soleFeatureID(t)].GatesBlock.Gates["evals"]
	require.True(t, g.Required, "the seeded evals gate is required")
	require.Equal(t, store.GateStatusPass, g.Status, "evals is derived from criteria; no required criterion is unsatisfied")
	require.Contains(t, g.Criteria, "C1", "check records the evals criteria map, not an empty seam")
}

// driveToStartedTask lays a one-epic-one-feature tree and walks it to a started
// task (in_progress), the fixture the task-grain check runs against.
func driveToStartedTask(t *testing.T) string {
	t.Helper()
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/nrca-form-loading",
		"--title", "NRCA form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")

	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")

	_, _, code = execCLI(t, "add", "task", "form-engine/nrca-form-loading/transcribe-the-schema", "--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)
	claimTaskAsOwner(t, tid, "jc")
	execCLI(t, "approve", "task", tid, "--contributor", "jc")
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code, "start task must actually succeed, or every caller of this fixture is testing a false premise")
	return tid
}

// TestCheckTaskGreenExits0 confirms a task whose required command gates pass exits
// 0, advances to the check state, and reports each gate green with provenance.
func TestCheckTaskGreenExits0(t *testing.T) {
	initGatedGitRepo(t, "true")
	tid := driveToStartedTask(t)

	out, _, code := execCLI(t, "check", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"passed\": true")
	require.Contains(t, out, "\"status\": \"check\"")
	require.Contains(t, out, "\"name\": \"build\"")
	require.Contains(t, out, "\"status\": \"pass\"")
	require.Contains(t, out, "artifacts/tiller/gates/build-")

	// The gates block is recorded on the task with full provenance.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	task := tree.Tasks[tid]
	require.Equal(t, "check", task.Status)
	require.Equal(t, store.GateStatusPass, task.GatesBlock.Gates["build"].Status)
	// The evals gate is derived from the eval contract's criteria; the feature's
	// single criterion is not required, so the gate is green (not a bare seam).
	require.Equal(t, store.GateStatusPass, task.GatesBlock.Gates["evals"].Status)
	require.Equal(t, "jc", task.GatesBlock.Gates["build"].RunBy)
	// The .tiller tree (the code under test) is uncommitted here, so the working
	// tree is dirty and the honest provenance is a sha256 working-tree fingerprint,
	// not the HEAD commit, which does not contain the tested code.
	require.Equal(t, store.KindSHA256, task.GatesBlock.Gates["build"].Against.Kind)
	require.True(t, strings.HasPrefix(task.GatesBlock.Gates["build"].Against.Value, "sha256:"))
}

// TestCheckTaskRedExits1 confirms a red required gate is a blocked business answer:
// exit 1, ok:false with a data block (never an error block), the task left
// in_progress, and the failing gate recorded.
func TestCheckTaskRedExits1(t *testing.T) {
	initGatedGitRepo(t, "false")
	tid := driveToStartedTask(t)

	out, _, code := execCLI(t, "check", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"passed\": false")
	require.Contains(t, out, "\"status\": \"in_progress\"")
	require.NotContains(t, out, "\"error\"")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	task := tree.Tasks[tid]
	require.Equal(t, "in_progress", task.Status, "a red task does not advance")
	require.Equal(t, store.GateStatusFail, task.GatesBlock.Gates["build"].Status)
}

// TestCheckReadinessStillWorks confirms the readiness subcommand is unaffected by
// the node-grain RunE wired onto the check command.
func TestCheckReadinessStillWorks(t *testing.T) {
	initGatedGitRepo(t, "true")
	// No design/ yet, so readiness reports not-ready (exit 1) with structured gaps.
	out, _, code := execCLI(t, "check", "readiness", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "design")
}

// TestCheckReadinessReadyNamesDecompose confirms a ready project does not just
// report "ready": it names the next move (issue #6). With design inputs present
// and a coherent manifest, readiness passes (exit 0) and the human output routes
// the user on to decompose.
func TestCheckReadinessReadyNamesDecompose(t *testing.T) {
	dir := t.TempDir()
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "00-index.md"), []byte("# Index\n"), 0o644))
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield", "--design", src)
	require.Equal(t, 0, code)
	// A ready project needs an owner in the roster; a human registers jc, then
	// join records the identity.
	registerContributor(t, "jc", "jc")
	_, _, code = execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "check", "readiness")
	require.Equal(t, 0, code, "design inputs plus a coherent manifest is ready")
	require.Contains(t, out, "Ready", "the ready report still names readiness")
	require.Contains(t, out, "Next (tiller guide decompose):", "a ready project with a to-be doc names decompose, from the shared route")
}

// TestCheckBadArgs confirms a check with a single positional arg is misuse.
func TestCheckBadArgs(t *testing.T) {
	initGatedGitRepo(t, "true")
	_, _, code := execCLI(t, "check", "feature", "--json")
	require.Equal(t, 2, code)
}

// evalsGate reads the recorded evals gate off the sole feature on disk.
func evalsGate(t *testing.T) store.Gate {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	feat := tree.Features[soleFeatureID(t)]
	require.NotNil(t, feat)
	return feat.GatesBlock.Gates["evals"]
}

// layActiveFeatureWithEvalCLI walks a one-epic-one-feature tree to an active
// feature whose feature.eval.md is evalMD, the fixture the evals-gate clobber
// tests run check against. Its first task is explicitly claimed and started,
// activating the feature after approval has recorded the accepted basis.
//
// spec feature requires at least one live task with a written task.md
// (validateSpec), so one is cut here before spec runs: without it the feature
// never leaves draft, and the parent-plan gate (guardParentState) would then
// refuse every task-grain call a caller downstream goes on to make. Its key,
// "first-slice-of-the-feature", is the one
// TestSeededGrainTaskGreensWhileContractGatesFeature itself names, so that
// caller's own `add task` of the same key is the idempotent no-op the add
// verbs promise (laySpeccableFeature's own doc comment states the identical
// convention); every other caller here never names a task at all, so the one
// it inherits is inert for them.
func layActiveFeatureWithEvalCLI(t *testing.T, evalMD string) {
	t.Helper()
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", cliFeatureKey,
		"--title", "NRCA form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.md"),
		[]byte("# NRCA form loading\n\nReal spec prose, no longer a stub.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.eval.md"), []byte(evalMD), 0o644))
	addSpeccedTask(t, "first-slice-of-the-feature", "First slice of the feature")
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", cliFeatureKey, "--contributor", "jc")
	execCLI(t, "approve", "feature", cliFeatureKey, "--contributor", "jc")
	claimTaskAsOwner(t, cliFeatureKey+"/first-slice-of-the-feature", "jc")
	_, stderr, code := execCLI(t, "start", "task", cliFeatureKey+"/first-slice-of-the-feature", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
}

// requiredManualEvalMD is a feature.eval.md whose single required criterion is a
// manual sign-off, so eval leaves it needs_attestation and only a human attest
// flips it to pass.
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

// TestCheckKeepsAttestedEvalsGreen is the A1 anti-clobber: after `tiller eval` and
// a human `tiller attest` satisfy the required eval criterion, the evals gate is
// pass with the criterion in its criteria map; running `tiller check` afterward
// must LEAVE evals pass and the criteria map intact, never reset it to pending
// with an empty map by re-running the gate as a bare command.
func TestCheckKeepsAttestedEvalsGreen(t *testing.T) {
	initGatedGitRepo(t, "true")
	layActiveFeatureWithEvalCLI(t, requiredManualEvalMD)

	// eval: the required manual criterion is needs_attestation, the gate not green.
	_, _, code := execCLI(t, "eval", "feature", cliFeatureKey, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Equal(t, store.CriterionStatusNeedsAttestation, evalsCriterion(t, "C1").Status)

	// attest: the human signs off; the gate is green with the criterion pass.
	_, _, code = execCLI(t, "attest", "feature", cliFeatureKey, "--criterion", "C1", "--status", "pass", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Equal(t, store.CriterionStatusPass, evalsCriterion(t, "C1").Status)
	require.Equal(t, store.GateStatusPass, evalsGate(t).Status)

	// check MUST NOT clobber the attested result.
	_, _, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	g := evalsGate(t)
	require.Equal(t, store.GateStatusPass, g.Status, "check must not reset the attested evals gate to pending")
	require.Contains(t, g.Criteria, "C1", "the criteria map survives check")
	require.Equal(t, store.CriterionStatusPass, g.Criteria["C1"].Status, "the attested criterion is not downgraded")
}

// TestCheckEvalsKeepsTeethUnsatisfied confirms a feature with an unsatisfied
// required criterion shows the evals gate un-green through BOTH eval and check: a
// required manual criterion is pending across both, and a failing required command
// criterion is fail across both. check evaluates the criteria the same way eval
// does rather than recording a bare pass.
func TestCheckEvalsKeepsTeethUnsatisfied(t *testing.T) {
	t.Run("pending manual", func(t *testing.T) {
		initGatedGitRepo(t, "true")
		contract := strings.Replace(requiredManualEvalMD, "---\n\n# Acceptance",
			"  - id: C2\n    statement: Optional visual polish review\n    binding: { type: manual }\n    required: false\n---\n\n# Acceptance", 1)
		layActiveFeatureWithEvalCLI(t, contract)

		_, _, code := execCLI(t, "eval", "feature", cliFeatureKey, "--contributor", "jc", "--json")
		require.Equal(t, 1, code)
		require.Equal(t, store.GateStatusPending, evalsGate(t).Status)

		out, _, code := execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc", "--json")
		require.Equal(t, 1, code, "pending required acceptance is not a passed check")
		require.Contains(t, out, "\"ok\": false")
		require.Contains(t, out, "\"passed\": false")
		require.NotContains(t, out, "\"error\"")
		g := evalsGate(t)
		require.Equal(t, store.GateStatusPending, g.Status, "check keeps the evals gate pending, not reset to a bare command pass")
		require.Equal(t, store.CriterionStatusNeedsAttestation, g.Criteria["C1"].Status)

		_, errOut, code := execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
		require.Equal(t, 1, code)
		require.Contains(t, errOut, "C1 (needs_attestation)")
		require.Contains(t, errOut, fmt.Sprintf("tiller attest feature %s --criterion C1 --status pass", soleFeatureID(t)))
		require.NotContains(t, errOut, "C2", "optional criteria must not be presented as required acceptance")

		tid := soleTaskID(t)
		taskOut, _, code := execCLI(t, "check", "task", tid, "--contributor", "jc", "--json")
		require.Equal(t, 1, code)
		require.Contains(t, taskOut, "\"status\": \"in_progress\"", "pending required gates must not advance the task")
	})

	t.Run("fail command", func(t *testing.T) {
		initGatedGitRepo(t, "true")
		failMD := "---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: The suite passes\n    polarity: introduce\n    binding: { type: command, run: \"false\" }\n    required: true\n---\n"
		layActiveFeatureWithEvalCLI(t, failMD)

		_, _, code := execCLI(t, "eval", "feature", cliFeatureKey, "--contributor", "jc", "--json")
		require.Equal(t, 1, code)
		require.Equal(t, store.GateStatusFail, evalsGate(t).Status)

		out, _, code := execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc", "--json")
		require.Equal(t, 1, code, "a red required eval criterion blocks the check")
		require.Contains(t, out, "\"ok\": false")
		require.Equal(t, store.GateStatusFail, evalsGate(t).Status, "check records the evals gate fail, not a bare pass")
	})
}

func TestCheckRubricRequiresHumanAttestation(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	layActiveFeatureWithEvalCLI(t, evalFeatureEvalMD)

	_, _, code := execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Equal(t, store.CriterionStatusPending, evalsCriterion(t, "C2").Status)
	_, _, code = execCLI(t, "attest", "feature", cliFeatureKey, "--criterion", "C1", "--status", "pass", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rubric.json"), []byte(rubricOutputPass), 0o644))
	_, _, code = execCLI(t, "eval", "feature", cliFeatureKey,
		"--criterion", "C2", "--record-rubric-output", "rubric.json", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "a passing rubric is evidence, not human acceptance")
	require.Contains(t, out, "\"passed\": false")
	require.Equal(t, store.CriterionStatusNeedsAttestation, evalsCriterion(t, "C2").Status)
	_, errOut, code := execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "C2 (needs_attestation)")
	require.Contains(t, errOut, fmt.Sprintf("tiller attest feature %s --criterion C2 --status pass", soleFeatureID(t)))
	require.NotContains(t, errOut, "--criterion C1", "settled criteria need no new attestation")

	_, _, code = execCLI(t, "attest", "feature", cliFeatureKey, "--criterion", "C2", "--status", "pass", "--contributor", "jc")
	require.Equal(t, 0, code)
	out, _, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"passed\": true")
	require.Equal(t, store.GateStatusPass, evalsGate(t).Status)
}

func TestCheckUnconfiguredRequiredGateDoesNotAdvanceTask(t *testing.T) {
	initGatedGitRepo(t, "")
	tid := driveToStartedTask(t)

	out, _, code := execCLI(t, "check", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"passed\": false")
	require.Contains(t, out, "\"status\": \"in_progress\"")
	_, errOut, code := execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "build: pending", "a missing command is unfinished configuration, not a failed command")
}

// fillSeededBuildAndTests uncomments the seeded manifest's build and tests
// templates with a passing command, exactly the fill-in move the template
// invites a team to make. It anchors on the commented lines init rendered, so
// the seeded evals line stays byte-identical and a template change fails loudly.
func fillSeededBuildAndTests(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	path := filepath.Join(cwd, ".tiller", "tiller.yml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	manifest := string(raw)
	const buildTmpl = `# build: { kind: command, run: "<build command>", on: [feature, task] }`
	const testsTmpl = `# tests: { kind: command, run: "<test command>",  on: [feature, task] }`
	require.Contains(t, manifest, buildTmpl, "the seeded manifest carries the build template")
	require.Contains(t, manifest, testsTmpl, "the seeded manifest carries the tests template")
	manifest = strings.Replace(manifest, buildTmpl, `build: { kind: command, run: "true", on: [feature, task] }`, 1)
	manifest = strings.Replace(manifest, testsTmpl, `tests: { kind: command, run: "true", on: [feature, task] }`, 1)
	require.NoError(t, os.WriteFile(path, []byte(manifest), 0o644))
}

// TestSeededGrainTaskGreensWhileContractGatesFeature is the seeded-default half
// of the eval-grain decision (issue #11; the engine half is pinned in
// core/mutate gate_scoping_test.go). Under the manifest exactly as `tiller init`
// seeds it, with build and tests filled in as the template invites, a
// mid-feature task greens on build and tests alone and records no evals gate,
// while `tiller check feature` is still blocked by the unsatisfied acceptance
// contract. This is the dry run's failure inverted: the contract gates the
// feature, and a task greens on its own build and tests.
func TestSeededGrainTaskGreensWhileContractGatesFeature(t *testing.T) {
	initEvalSeededGitRepo(t)
	fillSeededBuildAndTests(t)

	// A contract whose required criterion cannot pass yet: the dry-run shape,
	// where the criterion belongs to work beyond this one task.
	failMD := "---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: The whole feature's acceptance holds\n    polarity: introduce\n    binding: { type: command, run: \"false\" }\n    required: true\n---\n\n# Acceptance\n"
	layActiveFeatureWithEvalCLI(t, failMD)

	_, _, code := execCLI(t, "add", "task", cliFeatureKey+"/first-slice-of-the-feature", "--title", "First slice of the feature", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)

	// The mid-feature task greens on build and tests alone.
	out, _, code := execCLI(t, "check", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, "a mid-feature task greens on build and tests alone")
	require.Contains(t, out, "\"passed\": true")
	require.Contains(t, out, "\"status\": \"check\"")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	got := tree.Tasks[tid].GatesBlock.Gates
	require.Contains(t, got, "build")
	require.Contains(t, got, "tests")
	require.NotContains(t, got, "evals", "the seeded default scopes the contract away from tasks")

	// The contract still gates the feature: its check is blocked on the red
	// required evals gate.
	fout, _, fcode := execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc", "--json")
	require.Equal(t, 1, fcode, "the feature still answers to its acceptance contract")
	require.Contains(t, fout, "\"ok\": false")
	require.Equal(t, store.GateStatusFail, evalsGate(t).Status, "the seeded evals gate records the contract's verdict on the feature")
}

// TestCheckBlockedOutputNamesFailureDetail is item 1.6 from the dry-run plan:
// the blocked line alone forced two follow-up commands every time. A red evals
// gate now appends its per-criterion detail, and a red command gate names its
// evidence path on one line.
func TestCheckBlockedOutputNamesFailureDetail(t *testing.T) {
	t.Run("red evals names criteria", func(t *testing.T) {
		initGatedGitRepo(t, "true")
		twoCritMD := "---\nnode: F-aaaaaa\ncriteria:\n" +
			"  - id: C1\n    statement: The suite passes\n    polarity: introduce\n    binding: { type: command, run: \"false\" }\n    required: true\n" +
			"  - id: C2\n    statement: A reviewer confirms the layout\n    polarity: introduce\n    binding: { type: manual }\n    required: true\n---\n"
		layActiveFeatureWithEvalCLI(t, twoCritMD)

		_, errOut, code := execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
		require.Equal(t, 1, code)
		require.Contains(t, errOut, "evals: C1 (fail), C2 (needs_attestation)",
			"the blocked output names the per-criterion detail")
	})

	t.Run("red command gate names evidence", func(t *testing.T) {
		initGatedGitRepo(t, "false")
		tid := driveToStartedTask(t)

		_, errOut, code := execCLI(t, "check", "task", tid, "--contributor", "jc")
		require.Equal(t, 1, code)
		require.Contains(t, errOut, "build: fail (evidence: artifacts/tiller/gates/build-",
			"the blocked output names the command gate's evidence path")
	})
}

// TestCheckGateProgress is issue #45: gate progress is human feedback, never
// evidence. In human mode each gate's started/finished lines land on stderr,
// distinct from the stdout confirmation, so a healthy multi-minute gate is
// distinguishable from a wedged one without contaminating anything recorded.
// Under --json the sink is nil: stderr stays empty and the envelope is the
// same shape the pre-task tests above already pin (TestCheckTaskGreenExits0
// et al. are unedited, which is the byte-stability proof).
func TestCheckGateProgress(t *testing.T) {
	t.Run("human mode lines land on stderr, not stdout", func(t *testing.T) {
		initGatedGitRepo(t, "true")
		tid := driveToStartedTask(t)

		out, errOut, code := execCLI(t, "check", "task", tid, "--contributor", "jc")
		require.Equal(t, 0, code)

		require.Contains(t, errOut, "gate build: running (true)...")
		require.Contains(t, errOut, "gate build: pass (")
		require.Contains(t, errOut, "gate tests: running (true)...")
		require.Contains(t, errOut, "gate tests: pass (")
		require.NotContains(t, out, "gate build:", "progress is stderr-only; it never rides the stdout confirmation")
		require.NotContains(t, out, "gate tests:", "progress is stderr-only; it never rides the stdout confirmation")
	})

	t.Run("json mode stays machine-clean", func(t *testing.T) {
		initGatedGitRepo(t, "true")
		tid := driveToStartedTask(t)

		out, errOut, code := execCLI(t, "check", "task", tid, "--contributor", "jc", "--json")
		require.Equal(t, 0, code)
		require.Empty(t, errOut, "--json passes a nil Progress sink, so nothing renders on stderr")
		require.Contains(t, out, "\"passed\": true")
		require.Contains(t, out, "\"name\": \"build\"")
		require.NotContains(t, out, "gate build:", "progress lines never contaminate the JSON envelope")
	})
}

func TestCheckHumanReviewLineNamesAggregate(t *testing.T) {
	res := mutate.CheckResult{
		Noun: store.NounFeature, ID: "F-a", Status: "active", AggregateBranch: "tiller/reviews/features/a",
		Gates: []mutate.GateOutcome{{Name: "evals", Required: true, Status: store.GateStatusPending, Criteria: map[string]string{"c1": store.CriterionStatusNeedsAttestation}}},
	}
	out := humanCheck(res, "a (F-a)")
	require.Contains(t, out, "on branch tiller/reviews/features/a")
	res.AggregateBranch = ""
	require.NotContains(t, humanCheck(res, "a (F-a)"), "on branch")
}

// reusedGateOutcomes is a hand-built pair of reused gates, one the node's own
// record and one a riding task's, so the rendering tests need no repo.
func reusedGateOutcomes(at time.Time) []mutate.GateOutcome {
	return []mutate.GateOutcome{
		{Name: "build", Required: true, Kind: store.GateKindCommand, Status: store.GateStatusPass, Reused: true, ReusedRunAt: at},
		{Name: "tests", Required: true, Kind: store.GateKindCommand, Status: store.GateStatusPass, Reused: true, ReusedFrom: "T-abcdef", ReusedRunAt: at},
	}
}

// TestCheckHumanNamesReusedGates pins the line that tells a reader a green came
// from a recorded pass, naming when it really ran and, for a pass a riding task
// carried, which task.
func TestCheckHumanNamesReusedGates(t *testing.T) {
	at := time.Date(2026, 10, 21, 14, 5, 0, 0, time.FixedZone("EDT", -4*60*60))
	stamp := at.UTC().Format("2006-01-02 15:04 UTC")

	t.Run("green", func(t *testing.T) {
		res := mutate.CheckResult{Noun: store.NounTask, ID: "T-a", Status: "check", Passed: true, Gates: reusedGateOutcomes(at)}
		out := humanCheck(res, "a (T-a)")
		require.Contains(t, out, "Reused: build (run "+stamp+"), tests (T-abcdef, run "+stamp+").")
		require.True(t, strings.HasPrefix(out, "Checked task a (T-a)"), "the Reused line follows the first line")
	})

	t.Run("blocked", func(t *testing.T) {
		gs := append(reusedGateOutcomes(at), mutate.GateOutcome{Name: "evals", Required: true, Status: store.GateStatusFail})
		res := mutate.CheckResult{Noun: store.NounTask, ID: "T-a", Status: "in_progress", Gates: gs}
		out := humanCheck(res, "a (T-a)")
		require.Contains(t, out, "Reused: build (run "+stamp+"), tests (T-abcdef, run "+stamp+").")
		require.Contains(t, out, "evals: fail")
	})

	t.Run("nothing reused prints no line", func(t *testing.T) {
		res := mutate.CheckResult{Noun: store.NounTask, ID: "T-a", Status: "check", Passed: true}
		require.NotContains(t, humanCheck(res, "a (T-a)"), "Reused:")
	})
}

// TestCheckHumanNamesNotRun pins the line for gates a --gate run left alone, and
// that each advisory warning prints on its own line.
func TestCheckHumanNamesNotRun(t *testing.T) {
	res := mutate.CheckResult{
		Noun: store.NounTask, ID: "T-a", Status: "in_progress",
		Gates: []mutate.GateOutcome{
			{Name: "build", Required: true, Status: store.GateStatusPending, NotRun: true},
			{Name: "tests", Required: true, Status: store.GateStatusPass},
		},
		Warnings: []string{"first advisory", "second advisory"},
	}
	out := humanCheck(res, "a (T-a)")
	require.Contains(t, out, "Not run: build (excluded by --gate).")
	require.Contains(t, out, "Warning: first advisory\n")
	require.Contains(t, out, "Warning: second advisory")
}

// TestCheckJSONReuseFields drives real checks on unchanged code and reads the
// reuse fields off the envelope: a second check reuses, --rerun does not, and a
// --gate run reports the gate it left alone as not_run.
func TestCheckJSONReuseFields(t *testing.T) {
	initGatedGitRepo(t, "true")
	tid := driveToStartedTask(t)

	_, _, code := execCLI(t, "check", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "check", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"reused\": true", "the second check on unchanged code reuses the recorded pass")

	out, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc", "--rerun", "--json")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "\"reused\"", "--rerun runs every gate for real")

	out, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc", "--rerun", "--gate", "tests", "--json")
	require.Contains(t, out, "\"not_run\": true")
	require.Contains(t, out, "\"name\": \"build\"")
}

// TestCheckHumanBlockedNamesStaledGate drives a blocked --gate run: build sits
// outside the set and --rerun forbids reusing its pass, so the pass is marked
// stale and the blocked headline names build rather than an empty list.
func TestCheckHumanBlockedNamesStaledGate(t *testing.T) {
	initGatedGitRepo(t, "true")
	tid := driveToStartedTask(t)

	_, _, code := execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)

	stdout, stderr, code := execCLI(t, "check", "task", tid, "--contributor", "jc", "--rerun", "--gate", "tests")
	require.Equal(t, 1, code)
	out := stdout + stderr
	require.Contains(t, out, "has unsatisfied required gate(s): build, evals.")
	require.Contains(t, out, "build: stale (evidence: ")
	require.Contains(t, out, "Not run: build, evals (excluded by --gate).")
}

// TestCheckJSONViewFields covers the projection: warnings ride the envelope
// only when present, and a riding task's pass names its source node.
func TestCheckJSONViewFields(t *testing.T) {
	view := checkView(mutate.CheckResult{Noun: store.NounChore, ID: "CH-a", Warnings: []string{"no command gate applies"}})
	b, err := json.Marshal(view)
	require.NoError(t, err)
	require.Contains(t, string(b), "\"warnings\":[\"no command gate applies\"]")

	b, err = json.Marshal(checkView(mutate.CheckResult{}))
	require.NoError(t, err)
	require.NotContains(t, string(b), "warnings")

	view = checkView(mutate.CheckResult{Gates: reusedGateOutcomes(time.Now())})
	require.True(t, view.Gates[0].Reused)
	require.Empty(t, view.Gates[0].ReusedFrom)
	require.Equal(t, "T-abcdef", view.Gates[1].ReusedFrom)
}

// TestCheckFlagsRefusedOnReadiness: --rerun and --gate mean nothing to the
// project-level readiness gate, so naming them there is misuse.
func TestCheckFlagsRefusedOnReadiness(t *testing.T) {
	initGatedGitRepo(t, "true")
	for _, flag := range [][]string{{"--rerun"}, {"--gate", "build"}} {
		argv := append([]string{"check", "readiness", "--json"}, flag...)
		out, _, code := execCLI(t, argv...)
		require.Equalf(t, 2, code, "%v", flag)
		require.Contains(t, out, "readiness_flag")
	}
}

// TestCheckBlankGateRefused: a blank --gate value is misuse before any gate runs.
func TestCheckBlankGateRefused(t *testing.T) {
	initGatedGitRepo(t, "true")
	tid := driveToStartedTask(t)
	out, _, code := execCLI(t, "check", "task", tid, "--contributor", "jc", "--gate", "  ", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, "blank_gate")
	require.Contains(t, out, "--gate")
}
