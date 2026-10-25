package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// evalFeatureEvalMD is a feature.eval.md carrying one manual and one eval criterion,
// the fixture the eval handoff tests run against. The node field is a placeholder;
// Tiller validates a recorded rubric output against the resolved feature id, not
// against this field.
const evalFeatureEvalMD = `---
node: F-aaaaaa
criteria:
  - id: C1
    statement: A reviewer confirms the rendered PDF matches the source layout
    polarity: introduce
    binding: { type: manual }
    required: true
  - id: C2
    statement: The generated copy reads naturally for a homeowner
    polarity: introduce
    binding: { type: eval, run: "Grade whether the copy reads naturally." }
    required: true
---

# Acceptance
`

// rubricOutputPass is a well-formed graded rubric output for C2 with a pass verdict.
const rubricOutputPass = `{
  "schema": "tiller.rubric_output.v1",
  "criterion": "C2",
  "verdict": "pass",
  "rationale": "The copy reads naturally and clearly."
}
`

// layEvalFeature lays a one-epic-one-feature tree in an owned (non-git) repo and
// writes a real spec plus the eval fixture onto the feature, the fixture the eval
// and attest tests resolve against. It returns the feature's canonical key.
func layEvalFeature(t *testing.T) string {
	t.Helper()
	initOwnedRepo(t)
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/nrca-form-loading",
		"--title", "NRCA form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)

	dir := featureDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"), []byte(evalFeatureEvalMD), 0o644))
	return "form-engine/nrca-form-loading"
}

// evalsCriterion reads one recorded evals-gate criterion off the feature on disk.
func evalsCriterion(t *testing.T, critID string) store.Criterion {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	feat := tree.Features[soleFeatureID(t)]
	require.NotNil(t, feat)
	return feat.GatesBlock.Gates["evals"].Criteria[critID]
}

// TestEvalEmitRubrics confirms --emit-rubrics returns a success envelope whose data
// is a rubric packet carrying only the eval-binding criterion, with the schema id
// and the output path the agent writes its grade to.
func TestEvalEmitRubrics(t *testing.T) {
	key := layEvalFeature(t)

	out, _, code := execCLI(t, "eval", "feature", key, "--emit-rubrics", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"schema\": \"tiller.rubric_packet.v1\"")
	require.Contains(t, out, "\"id\": \"C2\"")
	require.NotContains(t, out, "\"id\": \"C1\"", "only eval bindings are in the packet")
	require.Contains(t, out, "artifacts/tiller/evals/")
	require.Contains(t, out, "-C2.json")
}

// TestEvalEmitRubricsHumanSurfacesOutputPath confirms --emit-rubrics in HUMAN
// (non-json) mode surfaces, for each eval-binding criterion, the output_path the
// agent must write its graded packet to and the rubric text, so grading does not
// require --json (B4).
func TestEvalEmitRubricsHumanSurfacesOutputPath(t *testing.T) {
	key := layEvalFeature(t)

	out, _, code := execCLI(t, "eval", "feature", key, "--emit-rubrics", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "C2", "the human emit names the eval criterion")
	require.Contains(t, out, "artifacts/tiller/evals/", "the human emit surfaces the output path")
	require.Contains(t, out, "-C2.json", "the path is the criterion's rubric-output file")
	require.Contains(t, out, "Grade whether the copy reads naturally", "the human emit surfaces the rubric")
	require.NotContains(t, out, "C1", "only eval-binding criteria are in the packet")
}

// TestEvalRecordRubricOutputNeedsAttestation confirms a recorded well-formed output
// moves the required eval criterion to needs_attestation and does NOT satisfy it:
// the evals gate stays pending, the agent's rubric alone never makes it pass. The
// record mutation itself succeeded, so it exits 0 (ok:true): the not-yet-clean state
// is reported in the data (passed:false, needs_attestation), never via a failure exit
// (B5). The exit code is unambiguous: 0 means "recorded", not "failed".
func TestEvalRecordRubricOutputNeedsAttestation(t *testing.T) {
	key := layEvalFeature(t)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "rubric.json"), []byte(rubricOutputPass), 0o644))

	out, _, code := execCLI(t, "eval", "feature", key,
		"--criterion", "C2", "--record-rubric-output", "rubric.json", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, "the record mutation succeeded; the not-yet-clean state is in the data, not a failure exit")
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"passed\": false")
	require.Contains(t, out, "\"recorded\": \"C2\"")
	require.Contains(t, out, "needs_attestation")

	// The criterion is recorded needs_attestation, never pass, and the gate is not
	// green: an agent rubric without a human attest does not satisfy a required eval.
	c := evalsCriterion(t, "C2")
	require.Equal(t, store.CriterionStatusNeedsAttestation, c.Status)
	require.NotEqual(t, store.CriterionStatusPass, c.Status)
	require.Len(t, c.Evidence, 1)
	require.Equal(t, "rubric_output", c.Evidence[0].Kind)
	require.Contains(t, c.Evidence[0].Ref, "artifacts/tiller/evals/")

	tree, err := store.Load(cwd)
	require.NoError(t, err)
	feat := tree.Features[soleFeatureID(t)]
	require.NotEqual(t, store.GateStatusPass, feat.GatesBlock.Gates["evals"].Status,
		"the evals gate is not green on an unattested rubric")

	// The canonical evidence file was persisted under artifacts/tiller/evals/.
	matches, _ := filepath.Glob(filepath.Join(cwd, "artifacts", "tiller", "evals", "*-C2.json"))
	require.Len(t, matches, 1)
}

func TestEvalRecordFailedRubricOutputSucceedsWithoutPassingAcceptance(t *testing.T) {
	key := layEvalFeature(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	output := `{
  "schema": "tiller.rubric_output.v1",
  "criterion": "C2",
  "verdict": "fail",
  "rationale": "The copy is unnatural and confusing."
}`
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "rubric.json"), []byte(output), 0o644))

	out, _, code := execCLI(t, "eval", "feature", key,
		"--criterion", "C2", "--record-rubric-output", "rubric.json", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, "recording a failed verdict is still a successful mutation")
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"passed\": false")
	require.Contains(t, out, "\"recorded\": \"C2\"")

	c := evalsCriterion(t, "C2")
	require.Equal(t, store.CriterionStatusFail, c.Status)
	require.Len(t, c.Evidence, 1)
	require.Equal(t, "rubric_output", c.Evidence[0].Kind)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Equal(t, store.GateStatusFail, tree.Features[soleFeatureID(t)].GatesBlock.Gates["evals"].Status)
	evidence, err := os.ReadFile(filepath.Join(cwd, c.Evidence[0].Ref))
	require.NoError(t, err)
	require.Equal(t, output, string(evidence), "the persisted evidence retains the failed verdict")
}

// TestEvalRecordRubricOutputRefusesEvidenceSymlinkEscape confirms rubric
// evidence stays inside the checkout: an artifacts directory that is a symlink
// to an outside directory refuses the record instead of writing through it.
func TestEvalRecordRubricOutputRefusesEvidenceSymlinkEscape(t *testing.T) {
	key := layEvalFeature(t)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "rubric.json"), []byte(rubricOutputPass), 0o644))
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(cwd, "artifacts")))

	_, _, code := execCLI(t, "eval", "feature", key,
		"--criterion", "C2", "--record-rubric-output", "rubric.json", "--contributor", "jc", "--json")
	require.NotEqual(t, 0, code, "an escaping evidence path refuses the record")
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries, "no evidence, directory, or temporary file may land outside the checkout")
	require.Empty(t, evalsCriterion(t, "C2").Evidence, "nothing is recorded when the evidence write is refused")
}

// TestEvalRequiredUnsatisfiedIsNotPassed is the defense-in-depth contract: a
// default eval run over a feature whose required manual criterion is
// needs_attestation must report passed:false (exit 1), never a false all-clear,
// because a required criterion that is not satisfied means acceptance is not met.
func TestEvalRequiredUnsatisfiedIsNotPassed(t *testing.T) {
	initGatedGitRepo(t, "true")
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/nrca-form-loading",
		"--title", "NRCA", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)

	evalMD := "---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: A reviewer confirms the layout\n    polarity: introduce\n    binding: { type: manual }\n    required: true\n---\n"
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.eval.md"), []byte(evalMD), 0o644))

	out, _, code := execCLI(t, "eval", "feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "a required criterion in needs_attestation is not passed")
	require.Contains(t, out, "\"passed\": false")
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "needs_attestation")

	// The recorded criterion status is honest: needs_attestation, not pass.
	c := evalsCriterion(t, "C1")
	require.Equal(t, store.CriterionStatusNeedsAttestation, c.Status)
}

// TestEvalNoCriteriaIsCleanNoOp confirms eval on a feature with no acceptance
// contract yet (unspecced, no feature.eval.md) is a clean no-op: no required
// criteria to fail, so it reports passed (exit 0) rather than crashing.
func TestEvalNoCriteriaIsCleanNoOp(t *testing.T) {
	initGatedGitRepo(t, "true")
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/nrca-form-loading",
		"--title", "NRCA", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "eval", "feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, "no required criteria means a clean no-op, not a failure")
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"passed\": true")
}

// TestEvalRejectsAnExistingInvalidContract distinguishes a missing draft
// contract from an authored file that cannot serve as an acceptance contract.
// Direct eval must fail closed before it can record a green evals gate.
func TestEvalRejectsAnExistingInvalidContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "broken frontmatter", body: "---\ncriteria: [\n---\n"},
		{name: "empty criteria", body: "---\nnode: F-aaaaaa\ncriteria: []\n---\n"},
		{
			name: "duplicate criterion ids, first failing and second passing",
			body: "---\nnode: F-aaaaaa\ncriteria:\n" +
				"  - id: C1\n    statement: First fails\n    polarity: introduce\n    binding: { type: command, run: \"false\" }\n    required: true\n" +
				"  - id: C1\n    statement: Second passes\n    polarity: introduce\n    binding: { type: command, run: \"touch invalid-eval-command-ran\" }\n    required: true\n---\n",
		},
		{
			name: "invalid command criterion",
			body: "---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    polarity: introduce\n    binding: { type: command, run: \"touch invalid-eval-command-ran\" }\n    required: true\n---\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := layEvalFeature(t)
			require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.eval.md"), []byte(tc.body), 0o644))

			cwd, err := os.Getwd()
			require.NoError(t, err)
			tree, err := store.Load(cwd)
			require.NoError(t, err)
			ymlPath := filepath.Join(featureDir(t), "feature.yml")
			beforeYML, err := os.ReadFile(ymlPath)
			require.NoError(t, err)
			beforeEvents, err := events.List(tree)
			require.NoError(t, err)

			out, _, code := execCLI(t, "eval", "feature", key, "--contributor", "jc", "--json")
			require.Equal(t, 2, code)
			require.Contains(t, out, `"reason": "eval_contract_invalid"`)
			require.NotContains(t, out, `"passed": true`)

			require.NoFileExists(t, filepath.Join(cwd, "invalid-eval-command-ran"))
			afterYML, err := os.ReadFile(ymlPath)
			require.NoError(t, err)
			require.Equal(t, beforeYML, afterYML, "invalid eval must not rewrite feature.yml")

			tree, err = store.Load(cwd)
			require.NoError(t, err)
			afterEvents, err := events.List(tree)
			require.NoError(t, err)
			require.Equal(t, beforeEvents, afterEvents, "invalid eval must not append an event")
			require.NotEqual(t, store.GateStatusPass, tree.Features[soleFeatureID(t)].Gates["evals"].Status)
		})
	}
}

// TestEvalEmitRubricsAllowsMissingDraftContract preserves lazy stage-2
// authoring: a draft with no feature.eval.md still has a legal, read-only empty
// rubric response, while an existing malformed file is rejected above.
func TestEvalEmitRubricsAllowsMissingDraftContract(t *testing.T) {
	initOwnedRepo(t)
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/nrca-form-loading",
		"--title", "NRCA form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)

	ymlPath := filepath.Join(featureDir(t), "feature.yml")
	before, err := os.ReadFile(ymlPath)
	require.NoError(t, err)

	out, _, code := execCLI(t, "eval", "feature", "form-engine/nrca-form-loading",
		"--emit-rubrics", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"ok": true`)
	require.Contains(t, out, `"criteria": []`)
	require.NoFileExists(t, filepath.Join(featureDir(t), "feature.eval.md"))

	after, err := os.ReadFile(ymlPath)
	require.NoError(t, err)
	require.Equal(t, before, after, "emit-rubrics is read-only")
}

// TestEvalModeConflict confirms combining --emit-rubrics with --record-rubric-output
// is misuse (exit 2).
func TestEvalModeConflict(t *testing.T) {
	key := layEvalFeature(t)
	_, _, code := execCLI(t, "eval", "feature", key,
		"--emit-rubrics", "--criterion", "C2", "--record-rubric-output", "rubric.json", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
}

// TestEvalRerunConflicts: --rerun forces a default run, so combining it with
// either rubric mode is misuse (exit 2).
func TestEvalRerunConflicts(t *testing.T) {
	key := layEvalFeature(t)
	for _, argv := range [][]string{
		{"eval", "feature", key, "--rerun", "--emit-rubrics", "--contributor", "jc", "--json"},
		{"eval", "feature", key, "--rerun", "--criterion", "C2", "--record-rubric-output", "rubric.json", "--contributor", "jc", "--json"},
	} {
		out, _, code := execCLI(t, argv...)
		require.Equal(t, 2, code)
		require.Contains(t, out, "eval_mode_conflict")
	}
}

// TestEvalReusedCriterion: a second default run on unchanged code reuses the
// command criterion and says so; --rerun runs it for real.
func TestEvalReusedCriterion(t *testing.T) {
	runEvalCommandBinding(t, "true", 0, store.GateStatusPass)

	out, _, code := execCLI(t, "eval", "feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"reused\": true")

	out, _, code = execCLI(t, "eval", "feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--rerun", "--json")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "\"reused\"")
}

// TestEvalRenderingOfReusedCriteria: the default run names reused criteria and
// carries the flag; record mode never reports a reuse from an earlier run.
func TestEvalRenderingOfReusedCriteria(t *testing.T) {
	res := mutate.EvalResult{
		Noun: store.NounFeature, ID: "F-a", Passed: true,
		Criteria: []mutate.CriterionOutcome{
			{ID: "C1", Status: store.CriterionStatusPass, Required: true, Reused: true},
			{ID: "C2", Status: store.CriterionStatusPass, Required: true},
		},
	}
	require.Contains(t, humanEval(res, "a (F-a)"), "Reused: C1.")
	require.True(t, evalView(res).Criteria[0].Reused)

	res.Recorded = "C2"
	require.NotContains(t, humanEval(res, "a (F-a)"), "Reused:")
	require.False(t, evalView(res).Criteria[0].Reused, "record mode leaves reused unreported")
}

// TestEvalRecordNeedsCriterion confirms --record-rubric-output without --criterion
// is misuse (exit 2).
func TestEvalRecordNeedsCriterion(t *testing.T) {
	key := layEvalFeature(t)
	_, _, code := execCLI(t, "eval", "feature", key,
		"--record-rubric-output", "rubric.json", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
}

// TestEpicGrainIsRefusedByEvalCheckAndAttest is the A3 surface removed. An epic
// has no eval file and no gates block: its acceptance bar is the sum of its
// features' contracts, each enforced at feature grain (artifact-schema.md,
// Epics; Gates). So all three verbs refuse the epic noun as misuse (exit 2)
// naming the nouns they do take, rather than recording a gate block on a node
// that has nowhere to keep one.
func TestEpicGrainIsRefusedByEvalCheckAndAttest(t *testing.T) {
	initGatedGitRepo(t, "true")
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)

	for _, argv := range [][]string{
		{"eval", "epic", "form-engine", "--contributor", "jc", "--json"},
		{"check", "epic", "form-engine", "--contributor", "jc", "--json"},
		{"attest", "epic", "form-engine", "--criterion", "EC1", "--status", "pass", "--contributor", "jc", "--json"},
	} {
		out, _, code := execCLI(t, argv...)
		require.Equalf(t, 2, code, "%s epic must be misuse", argv[0])
		require.Contains(t, out, "unknown_noun")
		require.Contains(t, out, "feature, task")
	}
}

// TestEvalDefaultRunCommandBinding confirms a default eval run executes a command
// binding and records the result: a passing command leaves no required criterion
// failing (exit 0), a failing one is a blocked answer (exit 1).
func TestEvalDefaultRunCommandBinding(t *testing.T) {
	t.Run("green", func(t *testing.T) {
		runEvalCommandBinding(t, "true", 0, store.GateStatusPass)
	})
	t.Run("red", func(t *testing.T) {
		runEvalCommandBinding(t, "false", 1, store.GateStatusFail)
	})
}

// runEvalCommandBinding lays a git-backed feature whose single required criterion is
// a command binding running `run`, evaluates it, and asserts the exit code and the
// recorded criterion status.
func runEvalCommandBinding(t *testing.T, run string, wantCode int, wantStatus string) {
	t.Helper()
	initGatedGitRepo(t, "true")
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/nrca-form-loading",
		"--title", "NRCA", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)

	evalMD := "---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: The suite passes\n    polarity: introduce\n    binding: { type: command, run: \"" + run + "\" }\n    required: true\n---\n"
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.eval.md"), []byte(evalMD), 0o644))

	_, _, code = execCLI(t, "eval", "feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, wantCode, code)

	c := evalsCriterion(t, "C1")
	require.Equal(t, wantStatus, c.Status)
}

// TestEvalDistinctCriterionIdsKeepIndependentResults confirms unique ids stay
// independent: a failing C1 and a passing C2 are recorded separately, and the
// required failure keeps the run unsatisfied.
func TestEvalDistinctCriterionIdsKeepIndependentResults(t *testing.T) {
	key := layEvalFeature(t)
	body := "---\nnode: F-aaaaaa\ncriteria:\n" +
		"  - id: C1\n    statement: First fails\n    polarity: introduce\n    binding: { type: command, run: \"false\" }\n    required: true\n" +
		"  - id: C2\n    statement: Second passes\n    polarity: introduce\n    binding: { type: command, run: \"true\" }\n    required: true\n---\n"
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.eval.md"), []byte(body), 0o644))

	out, _, code := execCLI(t, "eval", "feature", key, "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, `"passed": false`)
	require.Equal(t, store.CriterionStatusFail, evalsCriterion(t, "C1").Status)
	require.Equal(t, store.CriterionStatusPass, evalsCriterion(t, "C2").Status)
}
