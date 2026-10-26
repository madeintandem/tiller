package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

const task4LongQuestionStatement = "Which schema version is canonical, how should every implementation migrate, and what compatibility guarantees must remain while the rollout is in progress?"

// featureDir is the on-disk folder of the one feature layReadTree lays down.
func featureDir(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(cwd, ".tiller", "epics", "form-engine", "features", "certification-form-loading")
}

func setFeatureReviewGrainOnDisk(t *testing.T, grain string) {
	t.Helper()
	path := filepath.Join(featureDir(t), "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(data), "\nreview_grain:")
	require.NoError(t, os.WriteFile(path, append(data, []byte("review_grain: "+grain+"\n")...), 0o644))
}

// writeRealSpec replaces the feature's stub feature.md with real prose and adds a
// feature.eval.md carrying one criterion.
func writeRealSpec(t *testing.T) {
	t.Helper()
	dir := featureDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Certification form loading\n\nLoad the CERT-LTI-02-A form from its JSON Schema and render it.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: The form loads\n    polarity: introduce\n    binding: { type: test, ref: spec/form_spec.rb }\n---\n\n# Acceptance\n"), 0o644))
}

// writeTaskSpec replaces one task's stub task.md with a real implementation
// spec, the non-stub state `spec feature` and the approve gate both read.
func writeTaskSpec(t *testing.T, localKey, body string) {
	t.Helper()
	path := filepath.Join(featureDir(t), store.TaskDirName, localKey, store.TaskProseFile)
	require.NoError(t, os.WriteFile(path, []byte("# "+localKey+"\n\n"+body+"\n"), 0o644))
}

// addSpeccedTask adds one task under the fixture feature and writes its
// implementation spec, the pair the merged specify stage requires.
func addSpeccedTask(t *testing.T, localKey, title string) {
	t.Helper()
	_, _, code := execCLI(t, "add", "task", "form-engine/certification-form-loading/"+localKey,
		"--title", title, "--contributor", "jc")
	require.Equal(t, 0, code)
	writeTaskSpec(t, localKey, "## Approach\n\nThe implementation spec for "+localKey+".")
}

// laySpeccableFeature lays a one-epic-one-feature tree and writes the whole
// second-stage output onto the feature: the spec, the contract, and one task
// with a written task.md. `spec feature` validates all three, because the
// specify stage is one act with one output (cli-surface.md, "Specifying a
// feature"), so a fixture stopping at the prose pair is refused at the gate.
//
// The task it lays is the one the fixtures downstream go on to name, so their
// own `add task` of that key is the idempotent no-op the add verbs promise.
func laySpeccableFeature(t *testing.T) {
	t.Helper()
	layReadTree(t)
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
}

// TestSpecFeatureExitCodesAcrossTheInputSpace pins the split cli-surface.md
// draws over the stage-2 output at the surface that carries it, the process exit
// code, and over the whole space of ways the stage can be short rather than the
// one that is easiest to reach. A missing or stub file is the not-written-yet
// blocked answer (exit 1, ok:false with a data block); a file that is written but
// malformed is misuse (exit 2, an error block naming the fault).
func TestSpecFeatureExitCodesAcrossTheInputSpace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lay    func(t *testing.T)
		code   int
		reason string
	}{
		{
			name: "prose missing", code: 1, reason: "spec_incomplete",
			lay: func(t *testing.T) { layReadTree(t) },
		},
		{
			name: "prose written, no contract", code: 1, reason: "spec_incomplete",
			lay: func(t *testing.T) {
				layReadTree(t)
				require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.md"),
					[]byte("# Certification form loading\n\nReal spec prose.\n"), 0o644))
			},
		},
		{
			name: "contract with no criteria", code: 2, reason: "eval_contract_invalid",
			lay: func(t *testing.T) {
				layReadTree(t)
				writeRealSpec(t)
				require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.eval.md"),
					[]byte("---\nnode: F-aaaaaa\ncriteria: []\n---\n\n# Acceptance\n"), 0o644))
				addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
			},
		},
		{
			name: "criterion with no binding", code: 2, reason: "eval_contract_invalid",
			lay: func(t *testing.T) {
				layReadTree(t)
				writeRealSpec(t)
				require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.eval.md"),
					[]byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: It should feel nice\n---\n\n# Acceptance\n"), 0o644))
				addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
			},
		},
		{
			name: "no tasks", code: 1, reason: "spec_incomplete",
			lay: func(t *testing.T) {
				layReadTree(t)
				writeRealSpec(t)
			},
		},
		{
			name: "task.md still a stub", code: 1, reason: "spec_incomplete",
			lay: func(t *testing.T) {
				layReadTree(t)
				writeRealSpec(t)
				_, _, code := execCLI(t, "add", "task", "form-engine/certification-form-loading/transcribe-the-schema",
					"--title", "Transcribe the schema", "--contributor", "jc")
				require.Equal(t, 0, code)
			},
		},
		{
			name: "the whole stage written", code: 0, reason: "",
			lay: func(t *testing.T) { laySpeccableFeature(t) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.lay(t)
			out, _, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc", "--json")
			require.Equal(t, tc.code, code, out)
			if tc.reason != "" {
				require.Contains(t, out, tc.reason)
			}
			want := "draft"
			if tc.code == 0 {
				want = "specced"
			}
			require.Equal(t, want, loadTestTree(t).Features[soleFeatureID(t)].Status)
		})
	}
}

// taskYMLPath is the on-disk path to one task's task.yml under the fixture
// feature, the file setTaskStatus edits directly.
func taskYMLPath(t *testing.T, localKey string) string {
	t.Helper()
	return filepath.Join(featureDir(t), store.TaskDirName, localKey, store.TaskStateFile)
}

// setTaskStatus hand-lays a task status onto task.yml, the shape a pre-guard
// tree can already hold on disk (the field trees do: a task approved
// before this guard existed). The guard must bite on what is there, not only
// on what today's verbs can produce (mirrors mutate/transition_test.go's own
// helper of the same name, one layer down).
func setTaskStatus(t *testing.T, localKey, from, to string) {
	t.Helper()
	path := taskYMLPath(t, localKey)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: "+from, "status: "+to, 1)
	require.NotEqual(t, string(data), out, "the replace must land or the fixture lies")
	switch to {
	case "in_progress", "check", "in_review", "done":
		if strings.Contains(out, "started: false") {
			out = strings.Replace(out, "started: false", "started: true", 1)
		} else if !strings.Contains(out, "started: true") {
			out += "started: true\n"
		}
	}
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// TestTaskVerbsRefuseUnderUngatedFeature pins the guard at the surface an
// agent reads: the exit code and the reason string. approve and start under a
// pre-plan-gate feature are blocked business answers (exit 1) naming
// parent_plan_ungated, and the batch form refuses the same way; after the
// plan gate both verbs work exactly as before.
func TestTaskVerbsRefuseUnderUngatedFeature(t *testing.T) {
	laySpeccableFeature(t)
	claimTaskAsOwner(t, cliTaskKey, "jc")
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")

	out, stderr, code := execCLI(t, "approve", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "approve under a draft feature is a blocked answer: out=%s stderr=%s", out, stderr)
	require.Contains(t, out, `"reason": "parent_plan_ungated"`)

	// The human line is carried on the report, not the --json payload
	// (emitReport discards `human` in JSON mode;
	// TestSpecIncompleteBlockedLineNamesFeatureByRef asserts a blocked human
	// line the same way), so it is asserted on a plain-mode repeat of the
	// same refused call; the task never moved, so the repeat refuses
	// identically.
	_, stderrHuman, codeHuman := execCLI(t, "approve", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--contributor", "jc")
	require.Equal(t, 1, codeHuman)
	require.Contains(t, stderrHuman, "tiller guide specify")

	// The batch form (approve task --feature) applies the same guard per
	// node; its human lines all print to stdout via emitBatch
	// (TestApproveBatchBlockedHumanLines pins that stream choice), so the
	// assertion reads both the way that test does.
	outBatch, stderrBatch, codeBatch := execCLI(t, "approve", "task", "--feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 1, codeBatch, "the batch form refuses the same way")
	require.Contains(t, outBatch+stderrBatch, "Blocked")

	execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")

	// Both approves above were refused, so the task itself never reached
	// ready; start's own refusal is pinned at the state the field
	// trees can already hold on disk, a task already ready under a feature
	// that has not yet taken the plan gate (setTaskStatus mirrors the
	// identical fixture at mutate/transition_test.go's
	// TestStartTaskRefusesUnderDraftFeature).
	setTaskStatus(t, "transcribe-the-schema", "proposed", "ready")

	_, stderr3, code3 := execCLI(t, "start", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--contributor", "jc")
	require.Equal(t, 1, code3, "specced is still pre-gate; start refuses")
	require.Contains(t, stderr3, "tiller approve feature")

	_, _, code4 := execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code4)
	_, _, code5 := execCLI(t, "start", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--contributor", "jc")
	require.Equal(t, 0, code5, "past the plan gate the guard is silent")
}

// TestApproveTaskOnAnAlreadyReadyTaskStillRefusesUnderUngatedFeature pins task
// 4's net 5, the v0.2.1 idempotent-retry behavior change no
// earlier test caught: approve on a task already sitting at ready is
// lifecycle.Legal's own safe-retry case (idempotent, "a verb whose target
// equals the current state" — Legal finds the approve->ready row and matches
// on To alone, regardless of From). transitionTask's approve arm calls
// guardParentState right after the legality check, ahead of lifecycle.Apply,
// so a RETRIED approve under a feature that never took the plan gate refuses
// parent_plan_ungated exactly like the first approve does, rather than the
// safe-retry rule quietly turning it into an exit-0 no-op. The task is
// hand-set to ready (setTaskStatus, the identical pre-guard-tree shape
// TestTaskVerbsRefuseUnderUngatedFeature's own start assertion uses one
// status later): no legitimate path reaches ready under a still-ungated
// feature, since the FIRST approve already refuses under one.
func TestApproveTaskOnAnAlreadyReadyTaskStillRefusesUnderUngatedFeature(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	setTaskStatus(t, "transcribe-the-schema", "proposed", "ready")

	out, _, code := execCLI(t, "approve", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "an idempotent retry must not bypass the parent-gate guard: out=%s", out)
	require.Contains(t, out, `"reason": "parent_plan_ungated"`)
	require.Equal(t, "ready", loadTestTree(t).Tasks[soleTaskID(t)].Status, "a refused approve must not move the task")
}

// TestTaskVerbsRefuseUnderCancelledFeature pins the guard at the agent's
// surface: exit 1, the reason string, the remedy naming cancel task, and the
// feature's key riding as its own structured field on both parent-guard
// blocks (the id-and-key rule; the ungated block gains it here too).
func TestTaskVerbsRefuseUnderCancelledFeature(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")

	out, stderr, code := execCLI(t, "approve", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "approve under a draft feature is a blocked answer: out=%s stderr=%s", out, stderr)
	require.Contains(t, out, `"reason": "parent_plan_ungated"`)
	require.Contains(t, out, `"feature_key": "form-engine/certification-form-loading"`)

	execCLI(t, "cancel", "feature", "form-engine/certification-form-loading", "--contributor", "jc", "--reason", "scope cut")

	out2, stderr2, code2 := execCLI(t, "approve", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--contributor", "jc", "--json")
	require.Equal(t, 1, code2, "approve under a cancelled feature is a blocked answer: out=%s stderr=%s", out2, stderr2)
	require.Contains(t, out2, `"reason": "parent_feature_cancelled"`)
	require.Contains(t, out2, `"feature_key": "form-engine/certification-form-loading"`)

	// Neither approve above ever moved the task, so it is still proposed;
	// start requires ready, so the fixture is hand-advanced the same way
	// TestTaskVerbsRefuseUnderUngatedFeature's own start assertion is (a task
	// already ready under a feature is exactly the pre-existing-tree shape
	// the and #35's field trees can already hold on disk).
	setTaskStatus(t, "transcribe-the-schema", "proposed", "ready")

	_, stderr3, code3 := execCLI(t, "start", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--contributor", "jc")
	require.Equal(t, 1, code3, "a cancelled parent is terminal; start refuses")
	require.Contains(t, stderr3, "tiller cancel task")
}

// epicYMLPath is the on-disk path to the fixture epic's epic.yml, the file
// setEpicStatus edits directly (mirrors taskYMLPath one grain up).
func epicYMLPath(t *testing.T, epicKey string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(cwd, ".tiller", "epics", epicKey, "epic.yml")
}

// setEpicStatus hand-lays an epic status onto epic.yml, mirroring setTaskStatus
// (and mutate/transition_test.go's own helper of the same name, one layer
// down): archived only arrives for real via a full done rollup followed by a
// human's archive, a path this package's fixtures cannot cheaply drive, and the
// guard has to read whatever status a legacy tree already holds on disk
// regardless of how it got there.
func setEpicStatus(t *testing.T, epicKey, from, to string) {
	t.Helper()
	path := epicYMLPath(t, epicKey)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: "+from, "status: "+to, 1)
	require.NotEqual(t, string(data), out, "the replace must land or the fixture lies")
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// TestFeatureVerbsRefuseUnderRetiredEpic pins the guard at the feature grain,
// the agent's surface: exit 1, the reason string, and the epic's own state and
// key riding as structured fields. Both of the epic's terminal states refuse
// the same way (mutate.ParentEpicRetiredError documents why: cancel and
// archive share terminality and remedy). cancelled is reached through the real
// verb; archived only arrives via a full done rollup this package's fixtures
// cannot cheaply drive, so that case hand-lays epic.yml the same way
// setTaskStatus already hand-lays a task's status one grain down.
func TestFeatureVerbsRefuseUnderRetiredEpic(t *testing.T) {
	for _, tc := range []struct {
		name      string
		epicState string
		retire    func(t *testing.T)
	}{
		{
			name:      "cancelled",
			epicState: "cancelled",
			retire: func(t *testing.T) {
				execCLI(t, "cancel", "epic", "form-engine", "--contributor", "jc", "--reason", "programme killed")
			},
		},
		{
			name:      "archived",
			epicState: "archived",
			retire: func(t *testing.T) {
				setEpicStatus(t, "form-engine", "draft", "archived")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			laySpeccableFeature(t)
			tc.retire(t)

			out, stderr, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc", "--json")
			require.Equal(t, 1, code, "spec under a retired epic is a blocked answer: out=%s stderr=%s", out, stderr)
			require.Contains(t, out, `"reason": "parent_epic_retired"`)
			require.Contains(t, out, fmt.Sprintf(`"epic_state": "%s"`, tc.epicState))
			require.Contains(t, out, `"epic_key": "form-engine"`)
			require.Equal(t, "draft", loadTestTree(t).Features[soleFeatureID(t)].Status, "a refused spec must not move the feature")
		})
	}
}

// TestTaskVerbsRefuseUnderLiveFeatureCancelledEpic preserves the grandparent
// guard before the first task start would activate an approved feature.
func TestTaskVerbsRefuseUnderLiveFeatureCancelledEpic(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	claimTaskAsOwner(t, cliTaskKey, "jc")
	execCLI(t, "cancel", "epic", "form-engine", "--contributor", "jc", "--reason", "programme killed")

	out, stderr, code := execCLI(t, "start", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "start under a live feature whose epic is cancelled is a blocked answer: out=%s stderr=%s", out, stderr)
	require.Contains(t, out, `"reason": "parent_epic_retired"`)
	tree := loadTestTree(t)
	require.Equal(t, "ready", tree.Features[soleFeatureID(t)].Status)
	require.Equal(t, "ready", tree.Tasks[soleTaskID(t)].Status)
	require.False(t, tree.Tasks[soleTaskID(t)].Started)

	// The human line is carried on the report, not the --json payload (the #35
	// arm's own test asserts it the same way: a plain-mode repeat of the same
	// refused call, since the task never moved).
	_, stderrHuman, codeHuman := execCLI(t, "start", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--contributor", "jc")
	require.Equal(t, 1, codeHuman)
	require.Contains(t, stderrHuman, "tiller cancel task")
}

// TestStatusReportsHeldByRetiredFeature pins task 3's status-oracle wiring:
// a live task under a cancelled feature reads held at both the
// human line and the --json envelope, before any doing verb ever runs.
func TestStatusReportsHeldByRetiredFeature(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "cancel", "feature", "form-engine/certification-form-loading", "--contributor", "jc", "--reason", "scope cut")

	out, _, code := execCLI(t, "status", "task", "form-engine/certification-form-loading/transcribe-the-schema")
	require.Equal(t, 0, code)
	require.Contains(t, out, "held:  feature form-engine/certification-form-loading (F-")
	require.Contains(t, out, "is cancelled; the doing verbs will refuse")

	jout, _, code := execCLI(t, "status", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, jout, `"held": {`)
	require.Contains(t, jout, `"noun": "feature"`)
	require.Contains(t, jout, `"state": "cancelled"`)
}

// TestStatusReportsHeldByRetiredEpic pins task 3's other grain (mirrors
// TestTaskVerbsRefuseUnderLiveFeatureCancelledEpic's fixture): a live feature
// (and the live task beneath it) sitting under a cancelled epic both read
// held naming the epic, never the feature.
func TestStatusReportsHeldByRetiredEpic(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	claimTaskAsOwner(t, cliTaskKey, "jc")
	_, startErr, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
	require.Equal(t, 0, code, startErr)
	execCLI(t, "cancel", "epic", "form-engine", "--contributor", "jc", "--reason", "programme killed")

	taskOut, _, code := execCLI(t, "status", "task", "form-engine/certification-form-loading/transcribe-the-schema")
	require.Equal(t, 0, code)
	require.Contains(t, taskOut, "held:  epic form-engine (E-")
	require.Contains(t, taskOut, "is cancelled; the doing verbs will refuse")

	featOut, _, code := execCLI(t, "status", "feature", "form-engine/certification-form-loading")
	require.Equal(t, 0, code)
	require.Contains(t, featOut, "held:  epic form-engine (E-")
}

// TestStatusDoesNotReportHeldForPreGateParent pins the scoping rule the whole
// feature turns on: a proposed task under a DRAFT feature is the ordinary
// mid-specify shape, never a held one, however much both verbs would refuse
// against an actually retired ancestor.
func TestStatusDoesNotReportHeldForPreGateParent(t *testing.T) {
	laySpeccableFeature(t)

	out, _, code := execCLI(t, "status", "task", "form-engine/certification-form-loading/transcribe-the-schema")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "held:")

	fout, _, code := execCLI(t, "status", "feature", "form-engine/certification-form-loading")
	require.Equal(t, 0, code)
	require.NotContains(t, fout, "held:")
}

// layFeatureReadyToStart walks the feature through its plan gate and leaves its
// task ready and unclaimed. Claim-gate tests choose the task's reservation.
func layFeatureReadyToStart(t *testing.T) string {
	t.Helper()
	layTwoOwnerFeature(t)
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	return soleFeatureID(t)
}

func TestStartTaskClaimGateRefusesUnclaimed(t *testing.T) {
	featureID := layFeatureReadyToStart(t)
	taskID := soleTaskID(t)

	out, _, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	data := jsonData(t, out)
	require.Equal(t, "task", data["noun"])
	require.Equal(t, taskID, data["id"])
	require.Equal(t, "work_unclaimed", data["reason"])
	require.Equal(t, "ready", data["current_state"])
	require.Equal(t, cliTaskKey, data["key"])
	require.Equal(t, "tiller claim task "+cliTaskKey, data["seam"])
	tree := loadTestTree(t)
	require.Equal(t, "ready", tree.Features[featureID].Status)
	require.Equal(t, "ready", tree.Tasks[taskID].Status)
	require.False(t, tree.Tasks[taskID].Started)
	require.Empty(t, tree.Tasks[taskID].Branch)
}

func TestStartTaskClaimGateRefusesForeignClaim(t *testing.T) {
	featureID := layFeatureReadyToStart(t)
	taskID := soleTaskID(t)
	claimTaskAsOwner(t, cliTaskKey, "pat")

	out, _, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	data := jsonData(t, out)
	require.Equal(t, "task", data["noun"])
	require.Equal(t, taskID, data["id"])
	require.Equal(t, "claim_held", data["reason"])
	require.Equal(t, "ready", data["current_state"])
	require.Equal(t, cliTaskKey, data["key"])
	require.Equal(t, "pat", data["holder"])
	tree := loadTestTree(t)
	require.Equal(t, "ready", tree.Features[featureID].Status)
	require.Equal(t, "ready", tree.Tasks[taskID].Status)
	require.False(t, tree.Tasks[taskID].Started)
	require.Equal(t, "pat", tree.Tasks[taskID].Owner)
	require.Empty(t, tree.Tasks[taskID].Branch)
}

func TestStartTaskClaimGateAllowsHolder(t *testing.T) {
	featureID := layFeatureReadyToStart(t)
	taskID := soleTaskID(t)
	claimTaskAsOwner(t, cliTaskKey, "jc")

	out, _, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Equal(t, "in_progress", jsonData(t, out)["status"])
	tree := loadTestTree(t)
	require.Equal(t, "active", tree.Features[featureID].Status)
	require.Empty(t, tree.Features[featureID].Branch)
	require.Equal(t, "in_progress", tree.Tasks[taskID].Status)
	require.True(t, tree.Tasks[taskID].Started)
	require.Equal(t, "jc", tree.Tasks[taskID].Owner)
}

func TestStartTaskReturnsItsBranchAndBase(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	walkFeatureToActive(t)
	tid := soleTaskID(t)
	out, _, code := execCLI(t, "start", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	branch := loadTestTree(t).Tasks[tid].Branch
	require.NotEmpty(t, branch)
	require.Equal(t, branch, data["branch"])
	require.Equal(t, "main", data["base_branch"])
	require.Empty(t, loadTestTree(t).Features[soleFeatureID(t)].Branch)
	require.Equal(t, "main", gitOut(t, dir, "branch", "--show-current"), "start records a branch without checking it out")
}

// checkoutTaskBranchCLI creates the task's recorded branch for a work-loop fixture.
func checkoutTaskBranchCLI(t *testing.T, taskID string) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	branch := loadTestTree(t).Tasks[taskID].Branch
	require.NotEmpty(t, branch)
	gitRun(t, dir, "checkout", "-b", branch)
	return branch
}

// assembleTaskForReviewCLI merges the submitted task, including its state commit,
// into a distinct aggregate branch. It never performs the final integration merge.
func assembleTaskForReviewCLI(t *testing.T, taskID, aggregate string) {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	branch := loadTestTree(t).Tasks[taskID].Branch
	require.NotEmpty(t, aggregate)
	require.NotEqual(t, branch, aggregate)
	require.Equal(t, branch, gitOut(t, dir, "branch", "--show-current"))
	base := gitOut(t, dir, "rev-parse", "main")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "record submitted task")
	gitRun(t, dir, "checkout", "-b", aggregate, base)
	gitRun(t, dir, "merge", "--no-ff", "-m", "assemble reviewed task", branch)
}

// TestSubmitConfirmationWarnsWhenTheStateCommitIsUnpushed is the field case
// mechanically reproduced (Lantern's T-m66b11): the recorded feature
// branch is pushed and tracked, then a further local commit (the state
// commit a submit's own writes would normally be folded into) never reaches
// the remote before submit runs, submit's own "normal immediate condition"
// per the brief. The exact wording names the commit and branch on submit's
// own human confirmation and, unchanged since it is the same recorded task,
// on status's oracle too (both its human line and its --json
// unpushed_review object), proving the one shared helper renders identically
// at both call sites.
func TestSubmitConfirmationWarnsWhenTheStateCommitIsUnpushed(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")
	gitRun(t, dir, "remote", "add", "origin", bare)

	tid := walkTaskToCheck(t)
	branch := loadTestTree(t).Tasks[tid].Branch
	gitRun(t, dir, "push", "-u", "origin", branch)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.txt"), []byte("state\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "state commit")
	head := gitOut(t, dir, "rev-parse", "--short", "HEAD")
	wantWarning := fmt.Sprintf("state commit %s is not on the PR; push %s before merging or the crossing cannot be observed", head, branch)

	humanOut, _, code := execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, 0, code, humanOut)
	require.Contains(t, humanOut, wantWarning)

	statusHuman, _, code := execCLI(t, "status", "task", tid)
	require.Equal(t, 0, code)
	require.Contains(t, statusHuman, wantWarning, "the task status renderer names the same warning verbatim")

	jsonOut, _, code := execCLI(t, "status", "task", tid, "--json")
	require.Equal(t, 0, code)
	require.Contains(t, jsonOut, "\"unpushed_review\"")
	require.Contains(t, jsonOut, fmt.Sprintf("\"commit\": \"%s\"", head))
	require.Contains(t, jsonOut, fmt.Sprintf("\"branch\": \"%s\"", branch))
	require.Contains(t, jsonOut, "\"upstream\": \"origin/"+branch+"\"")
}

// TestSubmitConfirmationIsSilentWhenTheBranchIsPushedEven is the paired
// all-clear the brief's fixture pair names: the identical setup, but with
// nothing advancing past the push, so the local tip is already on the
// remote and neither submit's human line, status's human line, nor either
// --json envelope carries a word about it.
func TestSubmitConfirmationIsSilentWhenTheBranchIsPushedEven(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")
	gitRun(t, dir, "remote", "add", "origin", bare)

	tid := walkTaskToCheck(t)
	branch := loadTestTree(t).Tasks[tid].Branch
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "state commit")
	gitRun(t, dir, "push", "-u", "origin", branch)

	humanOut, _, code := execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, 0, code, humanOut)
	require.NotContains(t, humanOut, "is not on the PR")

	statusHuman, _, code := execCLI(t, "status", "task", tid)
	require.Equal(t, 0, code)
	require.NotContains(t, statusHuman, "is not on the PR")

	jsonOut, _, code := execCLI(t, "status", "task", tid, "--json")
	require.Equal(t, 0, code)
	require.NotContains(t, jsonOut, "unpushed_review")
}

// TestSubmitJSONCarriesUnpushedReview proves submit's own --json envelope
// (not merely status's oracle, a separate call site in lifecycle_cmds.go)
// gains the additive unpushed_review object when the helper reports
// unpushed, and that a clean push leaves it absent.
// This test exercises a real push boundary rather than a fixed payload.
// Both states matter for review handoff.
func TestSubmitJSONCarriesUnpushedReview(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")
	gitRun(t, dir, "remote", "add", "origin", bare)

	tid := walkTaskToCheck(t)
	branch := loadTestTree(t).Tasks[tid].Branch
	gitRun(t, dir, "push", "-u", "origin", branch)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.txt"), []byte("state\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "state commit")
	head := gitOut(t, dir, "rev-parse", "--short", "HEAD")

	jsonOut, _, code := execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, jsonOut)
	require.Contains(t, jsonOut, "\"unpushed_review\"")
	require.Contains(t, jsonOut, fmt.Sprintf("\"commit\": \"%s\"", head))
	require.Contains(t, jsonOut, fmt.Sprintf("\"branch\": \"%s\"", branch))
}

// TestIllegalTransitionExits1 is the recoverable-error contract: an illegal move
// is a BLOCKED business answer (exit 1) with ok:false and a data block carrying
// current_state and legal_next, never an error block, so a stuck agent corrects
// itself.
func TestIllegalTransitionExits1(t *testing.T) {
	laySpeccableFeature(t)
	out, _, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "an illegal transition is a blocked business answer (exit 1)")
	require.Contains(t, out, "\"ok\": false")
	data := jsonData(t, out)
	require.Equal(t, "illegal_transition", data["reason"])
	require.Equal(t, "proposed", data["current_state"])
	require.Contains(t, data["legal_next"], "approve")
	require.Equal(t, "proposed", loadTestTree(t).Tasks[soleTaskID(t)].Status)
	require.NotContains(t, out, "\"error\"")
}

// TestSpecBlockedWhenIncomplete confirms speccing a freshly added feature (stub
// md, no eval) is a blocked answer (exit 1) naming the missing files.
func TestSpecBlockedWhenIncomplete(t *testing.T) {
	layReadTree(t) // no real spec/eval written
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	out, _, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"reason\": \"spec_incomplete\"")
	require.Contains(t, out, "feature.md")
	require.Contains(t, out, "feature.eval.md")
	require.NotContains(t, out, "\"error\"")
}

// writeMalformedEval replaces the feature's eval contract with a structurally
// wrong one: a single criterion that is just prose with no binding. The spec
// prose itself is real, so only the contract is at fault.
func writeMalformedEval(t *testing.T, evalBody string) {
	t.Helper()
	dir := featureDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Certification form loading\n\nReal spec prose.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"), []byte(evalBody), 0o644))
}

// TestSpecRejectsMalformedEval is the headline acceptance-contract fix: a
// feature.eval.md whose criteria do not carry teeth is refused as misuse (exit 2)
// with an error block naming the offending criterion and pointing at the shape,
// not silently accepted as a false all-clear.
func TestSpecRejectsMalformedEval(t *testing.T) {
	cases := []struct {
		name string
		eval string
		want string
	}{
		{
			name: "criterion is just prose",
			eval: "---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: The form should feel nice\n---\n\n# Acceptance\n",
			want: "C1",
		},
		{
			name: "unknown binding type",
			eval: "---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: It works\n    binding: { type: smoke }\n---\n\n# Acceptance\n",
			want: "C1",
		},
		{
			name: "command binding missing run",
			eval: "---\nnode: F-aaaaaa\ncriteria:\n  - id: C2\n    statement: The suite passes\n    binding: { type: command }\n---\n\n# Acceptance\n",
			want: "C2",
		},
		{
			name: "empty contract",
			eval: "---\nnode: F-aaaaaa\ncriteria: []\n---\n\n# Acceptance\n",
			want: "no criteria",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			layReadTree(t)
			execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
			writeMalformedEval(t, tc.eval)

			out, _, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc", "--json")
			require.Equal(t, 2, code, "a malformed contract is misuse (exit 2)")
			require.Contains(t, out, "\"ok\": false")
			require.Contains(t, out, "\"reason\": \"eval_contract_invalid\"")
			require.Contains(t, out, "\"error\"")
			require.Contains(t, out, tc.want)
			require.Contains(t, out, "tiller guide specify")
		})
	}
}

// TestStartBlockedByOpenQuestion preserves inherited question holds at task start.
// Recording a feature assumption is a separate act and never clears a task hold.
func TestStartBlockedByOpenQuestion(t *testing.T) {
	for _, noun := range []string{"feature", "epic"} {
		t.Run(noun, func(t *testing.T) {
			layFeatureReadyToStart(t)
			claimTaskAsOwner(t, cliTaskKey, "jc")
			fid := soleFeatureID(t)
			tid := soleTaskID(t)
			blockedID := fid
			if noun == "epic" {
				blockedID = soleEpicID(t)
			}
			writeBlockingQuestion(t, "Q-zzzzzz", blockedID)

			out, _, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc", "--json")
			require.Equal(t, 1, code, out)
			require.Contains(t, out, "\"reason\": \"open_question_blocker\"")
			require.Contains(t, out, "Q-zzzzzz")
			require.NotContains(t, out, "\"error\"")
			require.Contains(t, out, `"title": "Canonical schema version"`)
			require.Contains(t, out, `"statement": "`+task4LongQuestionStatement+`"`)
			blockedStdout, blockedStderr, blockedCode := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
			require.Equal(t, 1, blockedCode)
			blockedHuman := blockedStdout + blockedStderr
			require.Contains(t, blockedHuman, `Q-zzzzzz ("Canonical schema version")`)
			require.NotContains(t, blockedHuman, task4LongQuestionStatement)

			stdout, stderr, code := execCLI(t, "approve", "feature", cliFeatureKey,
				"--assume-question", "Q-zzzzzz", "--reason", "proceeding under the documented assumption", "--contributor", "jc", "--json")
			require.Equal(t, 0, code, stdout+stderr)
			tree := loadTestTree(t)
			require.Contains(t, tree.Features[fid].AssumptionBasis, "Q-zzzzzz")
			require.Equal(t, "ready", tree.Features[fid].Status)
			require.Equal(t, "ready", tree.Tasks[tid].Status)
			require.False(t, tree.Tasks[tid].Started)
			require.Empty(t, tree.Tasks[tid].Branch)
			require.Empty(t, tree.Features[fid].Branch)

			out, _, code = execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc", "--json")
			require.Equal(t, 1, code, out)
			require.Equal(t, "open_question_blocker", jsonData(t, out)["reason"])
			_, _, code = execCLI(t, "start", "task", cliTaskKey,
				"--assume-question", "Q-zzzzzz", "--reason", "not a task assumption", "--contributor", "jc")
			require.Equal(t, 2, code, "start does not accept assumption flags")

			mustRun(t, "answer", "question", "Q-zzzzzz", "--answer", "Use the 2025 revision.", "--contributor", "jc")
			mustRun(t, "start", "task", cliTaskKey, "--contributor", "jc")
			tree = loadTestTree(t)
			require.Equal(t, "active", tree.Features[fid].Status)
			require.Equal(t, "in_progress", tree.Tasks[tid].Status)
			require.True(t, tree.Tasks[tid].Started)
		})
	}
}

// TestAssumeQuestionRequiresReason confirms an assumption without a reason is
// misuse (exit 2) before any work.
func TestAssumeQuestionRequiresReason(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")

	_, _, code := execCLI(t, "approve", "feature", cliFeatureKey,
		"--assume-question", "Q-zzzzzz", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
}

// TestReturnAndResume confirms return reverses a pre-work gate and resume re-enters
// from review, both requiring a reason.
func TestReturnAndResume(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")

	// return: ready -> specced.
	out, _, code := execCLI(t, "return", "feature", "form-engine/certification-form-loading", "--reason", "scope was wrong", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"status\": \"specced\"")

}

// TestResumeReEntersATaskFromReview is resume's own arc, split out because it
// needs a real git repo: resume is the requested-changes path off a task's PR,
// so the task goes in_review -> in_progress while its feature stays active.
func TestResumeReEntersATaskFromReview(t *testing.T) {
	initGatedGitRepo(t, "true")
	tid := walkTaskToCheck(t)
	_, _, code := execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, 0, code)
	before := loadTestTree(t).Tasks[tid]

	out, _, code := execCLI(t, "resume", "task", tid, "--reason", "review requested changes", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"status\": \"in_progress\"")
	require.Equal(t, "active", soleFeatureStatus(t), "the feature stays active while its task loops back")
	after := loadTestTree(t).Tasks[tid]
	require.Equal(t, before.Branch, after.Branch)
	require.Equal(t, before.PR, after.PR)
}

func TestSubmitTaskRefusesAnotherTasksBranch(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	tid := walkTaskToCheck(t)
	branch := loadTestTree(t).Tasks[tid].Branch
	gitRun(t, dir, "checkout", "main")
	addSpeccedTask(t, "render-the-form", "Render the form")
	sibling := taskIDByKey(t, cliFeatureKey+"/render-the-form")
	mustRun(t, "approve", "task", sibling, "--contributor", "jc")
	claimTaskAsOwner(t, sibling, "jc")
	mustRun(t, "start", "task", sibling, "--contributor", "jc")
	otherBranch := checkoutTaskBranchCLI(t, sibling)
	require.NotEqual(t, branch, otherBranch)

	out, _, code := execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Equal(t, "branch_mismatch", jsonData(t, out)["reason"])
	task := loadTestTree(t).Tasks[tid]
	require.Equal(t, "check", task.Status)
	require.Nil(t, task.PR)
	require.Equal(t, branch, task.Branch)
	require.Equal(t, otherBranch, gitOut(t, dir, "branch", "--show-current"))

	gitRun(t, dir, "checkout", branch)
	mustRun(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, "in_review", taskStatusCLI(t, tid))
	require.Equal(t, "in_progress", taskStatusCLI(t, sibling))
}

// TestReturnRequiresReason confirms return without a reason is rejected.
func TestReturnRequiresReason(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")

	_, _, code := execCLI(t, "return", "feature", "form-engine/certification-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, "a missing required --reason is misuse")
}

// TestCancelRequiresReasonFlag confirms cancel now demands --reason the same
// way return and resume already do: cobra refuses the command before it ever
// reaches the core, naming the flag the human forgot.
func TestCancelRequiresReasonFlag(t *testing.T) {
	layReadTree(t)

	_, stderr, code := execCLI(t, "cancel", "feature", "form-engine/certification-form-loading", "--contributor", "jc")

	require.Equal(t, 2, code, "a missing required flag is misuse")
	require.Contains(t, stderr, "reason", "the refusal must name the flag the human forgot")
}

// TestStatusSurfacesLegalNext confirms status <node> now carries the lifecycle
// engine's legal next moves (the Phase 2 seam closed here).
func TestStatusSurfacesLegalNext(t *testing.T) {
	laySpeccableFeature(t)
	out, _, code := execCLI(t, "status", "feature", "form-engine/certification-form-loading", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"legal_next\"")
	require.Contains(t, out, "spec")
}

// TestStatusEpicShowsDerivedActive is the heart of this change at the CLI
// surface: a freshly approved epic (stored 'ready') with a child feature in flight
// (active) is shown as derived-active. The oracle keeps status "ready" and adds an
// "activity": "active" field; epic.yml is never rewritten to a stored 'active'.
func TestStatusEpicShowsDerivedActive(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	claimTaskAsOwner(t, cliTaskKey, "jc")
	_, startErr, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
	require.Equal(t, 0, code, startErr)

	out, _, code := execCLI(t, "status", "epic", "form-engine", "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	require.Equal(t, "ready", data["status"])
	require.Equal(t, "active", data["activity"])
	require.Equal(t, []any{map[string]any{"id": "jc", "name": "JC Grubbs"}}, data["contributors"])

	// The stored status on disk stays 'ready': the 'active' is derived, not persisted.
	require.Equal(t, "ready", soleEpicStatus(t))
}

// TestStartRejectsAggregateNouns confirms activation happens only through tasks.
func TestStartRejectsAggregateNouns(t *testing.T) {
	layFeatureReadyToStart(t)
	for _, tc := range []struct {
		noun string
		ref  string
	}{
		{"epic", "form-engine"},
		{"feature", cliFeatureKey},
	} {
		out, _, code := execCLI(t, "start", tc.noun, tc.ref, "--contributor", "jc", "--json")
		require.Equal(t, 2, code, out)
		require.Contains(t, out, "\"ok\": false")
	}
	require.Equal(t, "ready", soleEpicStatus(t))
	require.Equal(t, "ready", soleFeatureStatus(t))
	require.False(t, loadTestTree(t).Tasks[soleTaskID(t)].Started)
}

// soleEpicStatus reads the one epic's stored status off disk.
func soleEpicStatus(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Len(t, tree.Epics, 1)
	for _, e := range tree.Epics {
		return e.Status
	}
	return ""
}

// TestHappyChain walks approval, reservation, and task start, asserting that
// starting the task activates its parent without changing the epic's stored state.
func TestHappyChain(t *testing.T) {
	laySpeccableFeature(t)
	claimTaskAsOwner(t, cliTaskKey, "jc")

	_, _, code := execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)

	_, _, code = execCLI(t, "add", "task", "form-engine/certification-form-loading/transcribe-the-schema", "--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)

	tid := soleTaskID(t)
	_, _, code = execCLI(t, "approve", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)

	// Final states off disk.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	for _, e := range tree.Epics {
		require.Equal(t, "ready", e.Status)
	}
	require.Equal(t, "active", tree.Features[soleFeatureID(t)].Status)
	require.Equal(t, "in_progress", tree.Tasks[tid].Status)
}

// soleTaskID reads the one task's minted id off disk.
func soleTaskID(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Len(t, tree.Tasks, 1)
	for id := range tree.Tasks {
		return id
	}
	return ""
}

// writeBlockingQuestion writes an open question that blocks the named node.
func writeBlockingQuestion(t *testing.T, qid, blocks string) {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	dir := filepath.Join(cwd, ".tiller", "decisions")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	body := "questions:\n  - id: " + qid + "\n    title: Canonical schema version\n    statement: " + task4LongQuestionStatement + "\n    status: open\n    blocks: [" + blocks + "]\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "open-questions.yml"), []byte(body), 0o644))
}

// walkFeatureToActive prepares the one-feature tree for its first task start.
// The task is ready and claimed; callers may still configure review grain.
func walkFeatureToActive(t *testing.T) {
	t.Helper()
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	claimTaskAsOwner(t, cliTaskKey, "jc")
	_, _, code = execCLI(t, "spec", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
}

// walkTaskToCheck walks the shared fixture past walkFeatureToActive to one task
// sitting at check under the active feature, the state submit is aimed at now
// that a PR always represents a task. It returns the task id.
func walkTaskToCheck(t *testing.T) string {
	t.Helper()
	walkFeatureToActive(t)
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
	return tid
}

// TestSubmitWithoutPRRecordsBranchRefWhenNoRemote is the offline path:
// in a repo with no git remote, submit without --pr succeeds, moves the task to
// in_review, and records the current branch and head commit as the review ref,
// with pr left absent (never a faked number). The envelope carries the review
// ref and the human line names the branch and short sha.
func TestSubmitWithoutPRRecordsBranchRefWhenNoRemote(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	tid := walkTaskToCheck(t)

	head := gitOut(t, dir, "rev-parse", "--short", "HEAD")

	out, _, code := execCLI(t, "submit", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, "submit without --pr succeeds when the repo has no remote: %s", out)
	require.Contains(t, out, "\"status\": \"in_review\"")
	require.Contains(t, out, "\"review_ref\"")
	require.Contains(t, out, "\"branch\": \""+loadTestTree(t).Tasks[tid].Branch+"\"")
	require.Contains(t, out, "\"head\": \"git:"+head+"\"")
	require.NotContains(t, out, "\"pr\"")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	tk := tree.Tasks[tid]
	require.Equal(t, "in_review", tk.Status)
	require.NotNil(t, tk.ReviewRef, "the review ref is persisted on the task")
	require.Equal(t, tk.Branch, tk.ReviewRef.Branch)
	require.Equal(t, "git:"+head, tk.ReviewRef.Head.Value)
	require.Equal(t, store.KindGitCommit, tk.ReviewRef.Head.Kind)
	require.Nil(t, tk.PR, "pr stays absent in the offline case")

}

// TestSubmitWithoutPRWithRemoteIsMisuse pins the normal-path guard: when the
// repo has a remote, submit without --pr is a misuse (exit 2) whose message
// names --pr, and nothing mutates. An explicit --pr then works unchanged.
func TestSubmitWithoutPRWithRemoteIsMisuse(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	// A configured remote is enough; the path never resolves and no network is
	// touched.
	gitRun(t, dir, "remote", "add", "origin", "/tmp/anything.git")
	tid := walkTaskToCheck(t)

	out, _, code := execCLI(t, "submit", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, "submit without --pr is a misuse when the repo has a remote")
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"reason\": \"pr_required\"")
	require.Contains(t, out, "--pr")
	require.Contains(t, out, "review surface")

	// Nothing mutated: the task is still at check, no pr, no review ref.
	tree, err := store.Load(dir)
	require.NoError(t, err)
	tk := tree.Tasks[tid]
	require.Equal(t, "check", tk.Status, "a misuse mutates nothing")
	require.Nil(t, tk.PR)
	require.Nil(t, tk.ReviewRef)

	// An explicit --pr keeps working unchanged: pr is recorded, no review ref.
	out, _, code = execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"status\": \"in_review\"")
	require.Contains(t, out, "\"pr\": 41")
	require.NotContains(t, out, "review_ref")

	tree, err = store.Load(dir)
	require.NoError(t, err)
	tk = tree.Tasks[tid]
	require.Equal(t, "in_review", tk.Status)
	require.NotNil(t, tk.PR)
	require.Equal(t, 41, *tk.PR)
	require.Nil(t, tk.ReviewRef)
}

// TestResumeRefusesTheFeatureNoun is resume's own half of the grammar
// checksum at the CLI boundary: a resumed review is always a task's own, so
// resume takes the task noun alone and the feature noun is misuse (exit 2),
// naming the noun that works. The noun mismatch fires before the address is
// even looked up: an unresolvable feature key still answers the noun
// refusal, not not_found.
//
// Before the review-grain build this test also covered submit feature, since
// a feature noun there was categorically wrong (task_only_verb). It no
// longer is: `submit feature` is now a legitimate coarse-grain command
// (mutate.SubmitNode), so its own half of this checksum moved to
// TestSubmitFeatureUnderDefaultGrainIsWrongGrainMisuse, which pins the new
// split.
func TestResumeRefusesTheFeatureNoun(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	walkFeatureToActive(t)
	before := soleFeatureStatus(t)

	out, _, code := execCLI(t, "resume", "feature", cliFeatureKey,
		"--reason", "review asked for changes", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, "resume feature is misuse, not a blocked business answer: %s", out)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"error\"")
	require.Contains(t, out, "task", "the refusal names the noun that works")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, before, tree.Features[soleFeatureID(t)].Status, "a refused verb mutates nothing")

	out, _, code = execCLI(t, "resume", "feature", "form-engine/no-such-feature",
		"--reason", "x", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, "\"reason\": \"unknown_noun\"",
		"resume validates its noun before resolving the address")
}

// TestSubmitFeatureUnderDefaultGrainIsWrongGrainMisuse pins submit's new
// split (docs/spec/cli-surface.md): `submit feature` is
// now a legitimate coarse-grain command (mutate.SubmitNode), so the feature
// noun no longer refuses categorically the way resume's still does. Under
// the default (unconfigured) task grain it still refuses as misuse (exit 2),
// now naming the resolved grain rather than the old task-only-verb reason,
// and mutates nothing. An unresolvable feature key resolves before the CLI
// ever reads a grain to refuse on, so it answers not_found instead of the
// noun checksum resume's own sibling test still pins.
func TestSubmitFeatureUnderDefaultGrainIsWrongGrainMisuse(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	walkFeatureToActive(t)
	before := soleFeatureStatus(t)

	out, _, code := execCLI(t, "submit", "feature", cliFeatureKey, "--pr", "9", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, "submit feature under the default task grain is misuse: %s", out)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"reason\": \"wrong_review_grain\"")
	require.Contains(t, out, "task", "the refusal names the grain that resolved")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, before, tree.Features[soleFeatureID(t)].Status, "a refused submit mutates nothing")
	require.Nil(t, tree.Features[soleFeatureID(t)].PR, "a refused submit records no pr")

	out, _, code = execCLI(t, "submit", "feature", "form-engine/no-such-feature",
		"--pr", "9", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, "\"reason\": \"not_found\"",
		"submit feature resolves the address before it has a grain to refuse on")
}

// TestSubmitFeatureCLIRefusesBeforeTasksAreInReview is the brief's own
// CLI-grain refusal shape: `submit feature` under a resolved feature grain,
// before its task has reached in_review, exits 1 naming the task
// (mutate.TaskNotSubmittableError); the full submitFeature refusal matrix
// (wrong grain, terminal, gates not green, the idempotent/changed pair) is
// Task 4's own core coverage (submit_node_test.go), trusted rather than
// re-driven here.
func TestSubmitFeatureCLIRefusesBeforeTasksAreInReview(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	walkFeatureToActive(t)
	_, _, code := execCLI(t, "configure", "feature", cliFeatureKey, "--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "task", cliFeatureKey+"/transcribe-the-schema", "--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)
	_, _, code = execCLI(t, "approve", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "submit", "feature", cliFeatureKey, "--pr", "9", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"reason\": \"tasks_incomplete\"")
	require.Contains(t, out, tid)
	require.Contains(t, out, "check")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.Nil(t, tree.Features[soleFeatureID(t)].PR, "a blocked submit records nothing")
}

// TestSubmitTaskUnderCoarseGrainAcceptsARemotelessReviewRefWithARemote pins
// controller ruling C: under a coarse resolved grain (the task's feature
// resolves to feature or epic grain), `submit task` accepts the remote-less
// review ref even though the repo has a remote, because the task itself
// never carries a PR at that grain: the feature or epic PR is the review
// surface (docs/spec/cli-surface.md). The task still moves check->in_review
// exactly as task grain does, recording a review ref instead of a PR.
func TestSubmitTaskUnderCoarseGrainAcceptsARemotelessReviewRefWithARemote(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	gitRun(t, dir, "remote", "add", "origin", "/tmp/anything.git")
	walkFeatureToActive(t)
	_, _, code := execCLI(t, "configure", "feature", cliFeatureKey, "--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "task", cliFeatureKey+"/transcribe-the-schema", "--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)
	_, _, code = execCLI(t, "approve", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	checkoutTaskBranchCLI(t, tid)
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "submit", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.NotContains(t, out, "No remote")
	require.Contains(t, out, "feature review")
	require.Contains(t, out, cliFeatureKey)

	tree, err := store.Load(dir)
	require.NoError(t, err)
	tk := tree.Tasks[tid]
	require.Equal(t, "in_review", tk.Status)
	require.NotNil(t, tk.ReviewRef, "the review ref is persisted on the task")
	require.Contains(t, out, tk.ReviewRef.Branch)
	require.Nil(t, tk.PR, "the task carries no PR of its own under a coarse grain")
}

// TestSubmitTaskUnderCoarseGrainRefusesAnExplicitPR resolves the brief's own
// named ambiguity: the design says a task under a coarse grain records no PR
// of its own ever, so an explicit --pr there is refused (exit 2) rather than
// silently accepted or silently downgraded to a review ref.
func TestSubmitTaskUnderCoarseGrainRefusesAnExplicitPR(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	gitRun(t, dir, "remote", "add", "origin", "/tmp/anything.git")
	walkFeatureToActive(t)
	_, _, code := execCLI(t, "configure", "feature", cliFeatureKey, "--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "task", cliFeatureKey+"/transcribe-the-schema", "--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)
	_, _, code = execCLI(t, "approve", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	checkoutTaskBranchCLI(t, tid)
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)

	stdout, stderr, code := execCLI(t, "submit", "task", tid, "--pr", "5", "--contributor", "jc", "--json")
	message := stdout + stderr
	require.Equal(t, 2, code, message)
	require.Contains(t, message, "--pr")
	require.Contains(t, message, "feature")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, "check", tree.Tasks[tid].Status, "a refused submit mutates nothing")
}

// gitOut runs a git command in dir and returns its trimmed output, failing the
// test on error. It is the read half of gitRun, for asserting against real
// git state (a head sha) without hand-parsing.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

func TestRefreshBasisOnDoneFeatureIsABlockedAnswer(t *testing.T) {
	dir := layApprovedFeatureWithTask(t)
	path := filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	done := strings.Replace(string(raw), "status: ready", "status: done", 1)
	require.NotEqual(t, string(raw), done, "the done status edit must land")
	require.NoError(t, os.WriteFile(path, []byte(done), 0o644))

	stdout, _, code := execCLI(t, "approve", "feature", cliFeatureKey, "--refresh-basis", "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, stdout, `"ok": false`)
	require.Contains(t, stdout, `"current_state": "done"`)
	require.NotContains(t, stdout, `"error"`)
}

// TestRefreshBasisOnADeletedPinFailsAndTheRefusalSaidSo pins the pair that has
// to hold together: the guard treats a pinned file that is GONE as drift
// (mutate.checkBasis), and --refresh-basis re-records the pin by hashing the
// working tree, so on a deleted file it has nothing to hash and fails rather
// than clearing anything.
//
// The refusal used to name --refresh-basis as the move on both arms, walking a
// human straight into that failure. This
// drives the failure first, so the message's advice is grounded in what the
// command actually does, then confirms the restore-then-refresh sequence the
// message now names is the one that works.
func TestRefreshBasisOnADeletedPinFailsAndTheRefusalSaidSo(t *testing.T) {
	dir := layApprovedFeatureWithTask(t)
	evalPath := filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.eval.md")
	kept, err := os.ReadFile(evalPath)
	require.NoError(t, err)
	require.NoError(t, os.Remove(evalPath))

	// The named move, run on the deleted pin: it fails, and not as a blocked
	// business answer either.
	_, _, code := execCLI(t, "approve", "feature", cliFeatureKey, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 2, code, "--refresh-basis cannot hash a file that is gone")

	// So the refusal names restoring the file first, and still names the command
	// that clears the drift once it is back.
	stdout, stderr, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
	require.Equal(t, 1, code)
	out := stdout + stderr
	require.Contains(t, out, "restore feature.eval.md")
	require.Contains(t, out, "tiller approve feature "+cliFeatureKey+" --refresh-basis")

	// And the JSON side carries the same distinction structurally.
	out, _, code = execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, "\"reason\": \"active_basis\"")
	require.Contains(t, out, "\"missing\"")
	require.Contains(t, out, "feature.eval.md")

	// The sequence the message names works.
	require.NoError(t, os.WriteFile(evalPath, kept, 0o644))
	_, _, code = execCLI(t, "approve", "feature", cliFeatureKey, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
	require.Equal(t, 0, code)
}

// TestActiveBasisRefusalNamesARunnableCommand pins the ordinary edited-file arm
// of the same message. The old line read "run approve feature --refresh-basis",
// which names no node and so is not runnable as printed; every breadcrumb Tiller
// prints is an in-project command a reader can run (cli-surface.md, "Refusal
// breadcrumbs and help text are self-contained in the project").
func TestActiveBasisRefusalNamesARunnableCommand(t *testing.T) {
	dir := layApprovedFeatureWithTask(t)
	mdPath := filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("# Certification form loading\n\nRevised.\n"), 0o644))

	stdout, stderr, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
	require.Equal(t, 1, code)
	out := stdout + stderr
	require.Contains(t, out, "tiller approve feature "+cliFeatureKey+" --refresh-basis")
	require.NotContains(t, out, "restore", "nothing is missing on this arm")
}

// TestRefreshBasisReportsWhatItRecordedNotAMove pins the confirmation and the
// readout of a basis refresh, which moved nothing: `approve <noun> <node>
// --refresh-basis` re-records accepted_against and deliberately leaves the state
// alone (mutate's own refresh arms return before the legality check), so a
// confirmation reading "is now in_progress" and a block reading "Approval
// recorded / Moved to ready: 1 of 1" both describe a transition that did not
// happen.
func TestRefreshBasisReportsWhatItRecordedNotAMove(t *testing.T) {
	dir := layApprovedFeatureWithTask(t)
	tid := soleTaskID(t)
	mustRun(t, "start", "task", tid, "--contributor", "jc")

	taskMD := filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading",
		store.TaskDirName, "transcribe-the-schema", store.TaskProseFile)
	require.NoError(t, os.WriteFile(taskMD, []byte("# Transcribe the schema\n\n## Approach\n\nRevised after approval.\n"), 0o644))

	out, _, code := execCLI(t, "approve", "task", tid, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, code)

	require.NotContains(t, out, "is now in_progress", "nothing moved: the state was in_progress before this ran")
	require.NotContains(t, out, "Moved to ready")
	require.NotContains(t, out, "Approval recorded")
	require.Contains(t, out, "accepted basis re-recorded")
	require.Contains(t, out, "in_progress", "the confirmation still says where the node stands")
	require.Contains(t, out, "**Accepted basis re-recorded**", "the block's headline says what the act was")

	require.Equal(t, "in_progress", soleTaskStatus(t, tid))
}

// TestRefreshBasisReportsWhatItRecordedOnABatch is the batch half of inherited
// item 8, which review found pinned only on the single-node path: a verifier
// reverted batch.go's act to its pre-commit `&views.GateAct{}` and the whole
// uncached cli suite still passed, then drove the mutant binary and got back
// "**Approval recorded** / Moved to ready: 2 of 2" for a command that moved
// nothing.
//
// The per-node confirmation lines stay right under that mutation, so the only
// thing that goes wrong is the block a human reads, which is the harder failure
// to notice and the reason this needs its own test rather than trust.
func TestRefreshBasisReportsWhatItRecordedOnABatch(t *testing.T) {
	dir := layApprovedFeatureWithTask(t)
	first := soleTaskID(t)

	// A second pinned task, so the invocation below is a real multi-node batch.
	_, _, code := execCLI(t, "add", "task", cliFeatureKey+"/render-the-form",
		"--title", "Render the form", "--contributor", "jc")
	require.Equal(t, 0, code)
	writeTaskSpec(t, "render-the-form", "## Approach\n\nThe implementation spec.")
	second := taskIDByKey(t, cliFeatureKey+"/render-the-form")
	_, _, code = execCLI(t, "approve", "task", second, "--contributor", "jc")
	require.Equal(t, 0, code)

	for _, k := range []string{"transcribe-the-schema", "render-the-form"} {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading",
				store.TaskDirName, k, store.TaskProseFile),
			[]byte("# "+k+"\n\n## Approach\n\nRevised after approval.\n"), 0o644))
	}

	out, _, code := execCLI(t, "approve", "task", first, second, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, code)

	require.NotContains(t, out, "**Approval recorded**")
	require.NotContains(t, out, "Moved to ready")
	require.Contains(t, out, "**Accepted basis re-recorded**")
	require.Contains(t, out, "Nodes re-pinned: 2")
	// The per-node lines are the record of what happened to each node and are
	// unchanged by the block above them; they are asserted here because the
	// mutation this test exists to catch leaves them correct.
	require.Equal(t, 2, strings.Count(out, "accepted basis re-recorded; state unchanged at ready."))
}

// TestRefreshBasisOnAFeatureWithNoProposedTasksIsNotAnApproval covers the other
// batch arm review named: `approve task --feature <key>` on a feature with
// nothing proposed took a hardcoded empty act, so a --refresh-basis run there
// printed the plain approval block for a command that approved nothing and
// re-pinned nothing.
func TestRefreshBasisOnAFeatureWithNoProposedTasksIsNotAnApproval(t *testing.T) {
	layApprovedFeatureWithTask(t)
	// The plan gate already flipped the one task to ready, so nothing on this
	// feature is proposed.

	out, _, code := execCLI(t, "approve", "task", "--feature", cliFeatureKey,
		"--refresh-basis", "--contributor", "jc")
	require.Equal(t, 0, code, "zero proposed tasks is a plain success")
	require.Contains(t, out, "No proposed tasks on feature")
	require.NotContains(t, out, "**Approval recorded**", "nothing was approved")
	require.Contains(t, out, "**Accepted basis re-recorded**")
	require.Contains(t, out, "Nodes re-pinned: 0", "and the count says plainly that it reached nothing")
}

// TestActiveBasisRefusalOfATaskNamesTheTask pins small item 5 of the earlier
// findings. A task's guard compares its PARENT feature's spec and contract
// before its own task.md (mutate.guardTask), so a task verb can be refused for a
// pin that is not the task's. The refusal named only the feature, which left
// `tiller start task <id>` answering with a sentence that never mentioned the
// task and read as a refusal of some other command.
func TestActiveBasisRefusalOfATaskNamesTheTask(t *testing.T) {
	dir := layApprovedFeatureWithTask(t)
	tid := soleTaskID(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.md"),
		[]byte("# Certification form loading\n\nRevised.\n"), 0o644))

	stdout, stderr, code := execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 1, code)
	out := stdout + stderr
	require.Contains(t, out, "task "+cliFeatureKey+"/transcribe-the-schema (",
		"the refusal names the node the caller addressed")
	require.Contains(t, out, "on feature "+cliFeatureKey,
		"and the grain whose pin actually moved")
	require.Contains(t, out, "tiller approve feature "+cliFeatureKey+" --refresh-basis",
		"the remedy addresses the feature, since that is the pin to refresh")
}

// layFeatureWithAssumedQuestionCLI drives layApprovedFeatureWithTask's
// approved-but-not-started feature to carry an assumption_basis entry for one
// answered question: the shape approve --reviewed-assumption exists to
// clear. It returns the repo root and the minted question id.
func layFeatureWithAssumedQuestionCLI(t *testing.T) (dir, questionID string) {
	t.Helper()
	dir = layApprovedFeatureWithTask(t)

	out, _, code := execCLI(t, "add", "question", task4LongQuestionStatement, "--title", "Canonical schema version", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	questionID = extractID(t, out, "Q-")

	mustRun(t, "approve", "feature", cliFeatureKey,
		"--assume-question", questionID, "--reason", "proceeding under the documented assumption", "--contributor", "jc")
	mustRun(t, "answer", "question", questionID, "--answer", "The 2025 revision.", "--contributor", "jc")
	return dir, questionID
}

// TestApproveReviewedAssumptionStampsIt is the CLI happy path:
// --reviewed-assumption's confirmation names both the node and the question
// by the shared reference convention, --json carries the question id and the
// feature's own id/key structurally (plus the readout block additively), and
// the stamp lands on disk matching the question's answer revision. A second
// run against the same question is the idempotent no-change report. Both
// runs print the human-gate readout block (approve is one of the four gates
// that always do), and it must never claim "Moved to ready": this act moves
// no lifecycle state at all.
func TestApproveReviewedAssumptionStampsIt(t *testing.T) {
	dir, qid := layFeatureWithAssumedQuestionCLI(t)
	fid := featureIDByKey(t, cliFeatureKey)

	out, _, code := execCLI(t, "approve", "feature", cliFeatureKey, "--reviewed-assumption", qid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"id": "`+fid+`"`)
	require.Contains(t, out, `"key": "`+cliFeatureKey+`"`)
	require.Contains(t, out, `"question_id": "`+qid+`"`)
	require.Contains(t, out, `"changed": true`)
	require.Contains(t, out, `"readout":`, "approve is a human gate; the block rides additively in --json")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	entry := tree.Features[fid].AssumptionBasis[qid]
	require.Equal(t, tree.Questions[qid].AnswerRevision, entry.AnswerRevision)
	require.NotEmpty(t, entry.Reason, "reviewing in never erases why the human proceeded")

	// The event is re-runnable truth, not an invented verb, and carries the
	// prior stamp (Before) beside the new one (After).
	evs, err := events.List(tree)
	require.NoError(t, err)
	var reviewEv *events.Event
	for i := range evs {
		if evs[i].Type == "review.assumption" {
			reviewEv = &evs[i]
		}
	}
	require.NotNil(t, reviewEv, "a review.assumption event was recorded")
	require.Equal(t, fmt.Sprintf("approve feature %s --reviewed-assumption %s", fid, qid), reviewEv.Command)
	require.Contains(t, reviewEv.Before, "answer_revision")

	// The plain confirmation names both the node (its reference form) and the
	// question, followed by the gate's own readout block; the second call
	// against the same revision is a no-change report, not a refusal, and
	// still prints the block.
	out, _, code = execCLI(t, "approve", "feature", cliFeatureKey, "--reviewed-assumption", qid, "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "feature "+cliFeatureKey+" (")
	require.Contains(t, out, qid)
	require.Contains(t, out, "no change")
	require.Contains(t, out, "Assumption reviewed in")
	require.Contains(t, out, "Stamp already matched (no change)")
	require.Contains(t, out, "Unreviewed assumptions remaining on this node")
	require.NotContains(t, out, "Moved to ready", "this act moves no lifecycle state")
	require.NotContains(t, out, "Approval recorded", "the plain-approve headline is wrong for this act too")
}

// TestApproveReviewedAssumptionRefusesAnEmptyValue pins that
// `--reviewed-assumption ""` read by non-empty-string alone
// silently fell through to the ordinary transition arm and crossed the
// approval gate (reviewer verified: a specced feature moved to ready on
// this exact input, exit 0). Gating on whether the flag was passed at all
// (cmd.Flags().Changed) is what tells "not given" from "given empty".
func TestApproveReviewedAssumptionRefusesAnEmptyValue(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")

	stdout, stderr, code := execCLI(t, "approve", "feature", "form-engine/certification-form-loading",
		"--reviewed-assumption", "", "--contributor", "jc")
	require.Equal(t, 2, code, "an explicit empty value is misuse, not flag-absent")
	require.Contains(t, stdout+stderr, "--reviewed-assumption needs a question id")

	out, _, code := execCLI(t, "status", "feature", "form-engine/certification-form-loading")
	require.Equal(t, 0, code)
	require.Contains(t, out, "state: specced", "the approval gate must not have silently crossed")
	require.NotContains(t, out, "state: ready")
}

// TestApproveReviewedAssumptionExclusiveWithRefreshBasis pins one of the two
// named exclusivity refusals: --reviewed-assumption and --refresh-basis are
// two different acts under the same verb, and combining them is misuse
// (exit 2), not a silent pick of one over the other.
func TestApproveReviewedAssumptionExclusiveWithRefreshBasis(t *testing.T) {
	dir, qid := layFeatureWithAssumedQuestionCLI(t)

	stdout, stderr, code := execCLI(t, "approve", "feature", cliFeatureKey,
		"--reviewed-assumption", qid, "--refresh-basis", "--contributor", "jc")
	require.Equal(t, 2, code)
	require.Contains(t, stdout+stderr, "one node and one act at a time")

	// Nothing moved: neither act ran.
	tree, err := store.Load(dir)
	require.NoError(t, err)
	fid := featureIDByKey(t, cliFeatureKey)
	require.Empty(t, tree.Features[fid].AssumptionBasis[qid].AnswerRevision.Value)
}

// TestApproveReviewedAssumptionExclusiveWithMultipleNodes pins the other
// named exclusivity refusal: --reviewed-assumption is a single-node act, so a
// multi-node argument list (the general approve batch shape) is refused
// before any address even resolves.
func TestApproveReviewedAssumptionExclusiveWithMultipleNodes(t *testing.T) {
	_, qid := layFeatureWithAssumedQuestionCLI(t)

	stdout, stderr, code := execCLI(t, "approve", "feature", cliFeatureKey, "F-does-not-matter",
		"--reviewed-assumption", qid, "--contributor", "jc")
	require.Equal(t, 2, code)
	require.Contains(t, stdout+stderr, "one node and one act at a time")
}

// TestApproveEpicWithReviewGrainRecordsIt pins the approve-time review-grain
// flag at epic grain (docs/spec/cli-surface.md): approve's own --review-grain
// records the epic's review grain in the same act as the start gate.
func TestApproveEpicWithReviewGrainRecordsIt(t *testing.T) {
	laySpeccableFeature(t)

	out, _, code := execCLI(t, "approve", "epic", "form-engine", "--review-grain", "epic", "--contributor", "jc")
	require.Equal(t, 0, code, out)

	tree := loadTestTree(t)
	e := tree.Epics[soleEpicID(t)]
	require.Equal(t, "epic", e.ReviewGrain)
	require.Equal(t, "ready", e.Status, "the flag rides the same act as the gate; the gate still moves the epic")
}

// TestApproveFeatureWithReviewGrainRecordsIt pins the identical flag one
// grain down: the feature's own plan gate (specced->ready) records the
// feature's review grain in the same act.
func TestApproveFeatureWithReviewGrainRecordsIt(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", cliFeatureKey, "--contributor", "jc")

	out, _, code := execCLI(t, "approve", "feature", cliFeatureKey, "--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code, out)

	fid := featureIDByKey(t, cliFeatureKey)
	tree := loadTestTree(t)
	require.Equal(t, "feature", tree.Features[fid].ReviewGrain)
	require.Equal(t, "ready", tree.Features[fid].Status)
}

// TestApproveRefreshBasisAndReviewGrainRefuseTogether pins controller ruling
// B: the mutate layer ignores ReviewGrain on the refresh-basis path (neither
// transitionEpic nor transitionFeature nor transitionTask reads it there), so
// a silently dropped flag would be the exact silent-pass-over class tiller
// refuses elsewhere. Combining the two flags is misuse (exit 2) naming both,
// tested at both nouns the flag applies to.
func TestApproveRefreshBasisAndReviewGrainRefuseTogether(t *testing.T) {
	t.Run("feature", func(t *testing.T) {
		dir := layApprovedFeatureWithTask(t)

		stdout, stderr, code := execCLI(t, "approve", "feature", cliFeatureKey,
			"--refresh-basis", "--review-grain", "feature", "--contributor", "jc")
		message := stdout + stderr
		require.Equal(t, 2, code, message)
		require.Contains(t, message, "--refresh-basis")
		require.Contains(t, message, "--review-grain")
		require.Contains(t, message, "separate act")

		tree, err := store.Load(dir)
		require.NoError(t, err)
		require.Empty(t, tree.Features[soleFeatureID(t)].ReviewGrain, "nothing recorded on a refused combination")
	})

	t.Run("epic", func(t *testing.T) {
		laySpeccableFeature(t)

		stdout, stderr, code := execCLI(t, "approve", "epic", "form-engine",
			"--refresh-basis", "--review-grain", "epic", "--contributor", "jc")
		message := stdout + stderr
		require.Equal(t, 2, code, message)
		require.Contains(t, message, "--refresh-basis")
		require.Contains(t, message, "--review-grain")
		require.Contains(t, message, "separate act")

		tree := loadTestTree(t)
		e := tree.Epics[soleEpicID(t)]
		require.Empty(t, e.ReviewGrain)
		require.Equal(t, "draft", e.Status, "nothing recorded, nothing moved")
	})
}

// TestApproveReviewGrainRefusesOnTaskNoun pins a deliberate deviation beyond
// the brief's own text, in the same silent-pass-over spirit as ruling B: a
// task carries no review_grain field at all (store.Task has none), so
// transitionTask never reads TransitionInput.ReviewGrain, and a task-noun
// approve carrying the flag would otherwise be silently ignored rather than
// refused. Scoping --review-grain to epic and feature at the CLI boundary
// mirrors the existing --feature-flag noun scoping just above it.
func TestApproveReviewGrainRefusesOnTaskNoun(t *testing.T) {
	dir := layApprovedFeatureWithTask(t)
	tid := soleTaskID(t)

	stdout, stderr, code := execCLI(t, "approve", "task", tid, "--review-grain", "feature", "--contributor", "jc")
	message := stdout + stderr
	require.Equal(t, 2, code, message)
	require.Contains(t, message, "--review-grain")
	require.Contains(t, message, "task")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	require.Equal(t, "ready", tree.Tasks[tid].Status, "a refused flag combination mutates nothing")
}

// TestApproveReviewGrainRefusesWithReviewedAssumption is the review-grain
// sibling of TestApproveReviewedAssumptionExclusiveWithRefreshBasis: reviewing
// an assumption in is a dedicated act (runApproveReviewedAssumption) that
// never reads --review-grain either, so combining the two is the same
// silent-pass-over hazard ruling B names, extended to the flag this task adds.
func TestApproveReviewGrainRefusesWithReviewedAssumption(t *testing.T) {
	dir, qid := layFeatureWithAssumedQuestionCLI(t)

	stdout, stderr, code := execCLI(t, "approve", "feature", cliFeatureKey,
		"--reviewed-assumption", qid, "--review-grain", "feature", "--contributor", "jc")
	message := stdout + stderr
	require.Equal(t, 2, code, message)
	require.Contains(t, message, "--review-grain")
	require.Contains(t, message, "--reviewed-assumption")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	fid := featureIDByKey(t, cliFeatureKey)
	require.Empty(t, tree.Features[fid].AssumptionBasis[qid].AnswerRevision.Value, "neither act ran")
}

// TestApproveReviewGrainRefusesAnEmptyValue mirrors
// TestApproveReviewedAssumptionRefusesAnEmptyValue: gating on whether the
// flag was passed at all (cmd.Flags().Changed), not on a non-empty value, is
// what tells "not given" from "given empty" for --review-grain too.
func TestApproveReviewGrainRefusesAnEmptyValue(t *testing.T) {
	laySpeccableFeature(t)

	stdout, stderr, code := execCLI(t, "approve", "epic", "form-engine", "--review-grain", "", "--contributor", "jc")
	require.Equal(t, 2, code, "an explicit empty value is misuse, not flag-absent")
	require.Contains(t, stdout+stderr, "--review-grain needs a value")

	require.Equal(t, "draft", loadTestTree(t).Epics[soleEpicID(t)].Status, "the approval gate must not have silently crossed")
}

// TestApproveEpicReviewGrainLockedReasonMatchesConfigure pins the
// Review Grain follow-up (live verification, Task 10): the approve/transition
// error path had no mapping for mutate.ReviewGrainLockedError, so classify
// fell to its generic fallback, reason "error", exit 2, even though the
// human message already named the refusal correctly ("review surface").
// configure's identical refusal (reportConfigureError, configure.go) already
// carries reason "review_grain_locked"; approve's --json payload must match
// it, with the exit code left exactly as it was (2, the generic
// classification's own code, not configure's exit 1: this fix touches only
// the reason string, not the rendering shape).
//
// An epic has no stored `active` status (a review finding), so a
// submitted epic-grain epic (its own PR or review ref already recorded via
// `submit epic`) sits at `ready` exactly like an unsubmitted one; the cheap
// honest route to that fixture is a direct yml rewrite, the same pattern the
// mutate-level tests already use for the identical condition
// (transition_review_grain_test.go's setEpicReviewRefOnDisk).
func TestApproveEpicReviewGrainLockedReasonMatchesConfigure(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	epicYML := filepath.Join(cwd, ".tiller", "epics", "form-engine", "epic.yml")
	data, err := os.ReadFile(epicYML)
	require.NoError(t, err)
	rewritten := string(data) + "review_ref:\n  branch: tiller/form-engine\n  head:\n    kind: git_commit\n    value: \"git:abc\"\n"
	require.NoError(t, os.WriteFile(epicYML, []byte(rewritten), 0o644))

	stdout, _, code := execCLI(t, "approve", "epic", "form-engine", "--review-grain", "feature", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, "the exit code is unchanged by this fix: %s", stdout)
	require.Contains(t, stdout, "\"reason\": \"review_grain_locked\"")
	require.Contains(t, stdout, "review surface", "the message still names why")

	require.Equal(t, "", loadTestTree(t).Epics[soleEpicID(t)].ReviewGrain,
		"a refused re-approve must not write the grain")
}

func TestApproveReviewGrainWorkInFlightMatchesConfigureBlockedAnswer(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		name := "human"
		if jsonOutput {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			layApprovedFeatureWithTask(t)
			setTaskStatusOnDisk(t, "transcribe-the-schema", "in_review")
			tree := loadTestTree(t)
			featureID := soleFeatureID(t)
			taskID := soleTaskID(t)
			args := []string{"approve", "feature", cliFeatureKey,
				"--review-grain", "feature", "--contributor", "jc"}
			if jsonOutput {
				args = append(args, "--json")
			}

			stdout, stderr, code := execCLI(t, args...)
			output := stdout + stderr

			require.Equal(t, 1, code, output)
			if jsonOutput {
				require.Contains(t, output, `"reason": "review_grain_work_in_flight"`)
				require.Contains(t, output, `"blocker_id": "`+taskID+`"`)
				require.NotContains(t, output, `"reason": "error"`)
				return
			}
			require.Equal(t, "Blocked: changing review grain on "+gen.RefByID(tree, featureID)+
				" would retarget "+gen.RefByID(tree, taskID)+
				", currently in_review; finish and sync that review first.\n", output)
		})
	}
}

// layInProgressTaskUnderAssumedFeatureCLI is layFeatureWithAssumedQuestionCLI
// plus driving the one task to check, the state submit is legal from
// (lifecycle/table.go: submit's only row is {From: "check"}): the CLI mirror
// of the core package's layInProgressTaskUnderAssumedFeature
// (transition_test.go), built the same way.
func layInProgressTaskUnderAssumedFeatureCLI(t *testing.T) (dir, taskID, questionID string) {
	t.Helper()
	dir, questionID = layFeatureWithAssumedQuestionCLI(t)
	taskID = soleTaskID(t)
	mustRun(t, "start", "task", taskID, "--contributor", "jc")
	checkoutTaskBranchCLI(t, taskID)
	mustRun(t, "check", "task", taskID, "--contributor", "jc")
	return dir, taskID, questionID
}

// TestQuestionSubmitRefusesUnreviewedAnsweredAssumptionCLI is the CLI rendering of
// the core refusal (mutate.TestSubmitRefusesUnreviewedAnsweredAssumption):
// the blocked JSON data block carries the structured question (id and
// statement) and a remedy addressing the feature that CARRIES the entry,
// never the task submit was run on, and the human line names both nodes so a
// reader is not left hunting for which command applies to what.
func TestQuestionSubmitRefusesUnreviewedAnsweredAssumptionCLI(t *testing.T) {
	_, taskID, qid := layInProgressTaskUnderAssumedFeatureCLI(t)

	out, _, code := execCLI(t, "submit", "task", taskID, "--pr", "7", "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, `"reason": "answered_question"`)
	require.Contains(t, out, `"id": "`+qid+`"`, "the question's own id rides in the structured questions block")
	require.Contains(t, out, task4LongQuestionStatement, "and its statement, so a reader does not need a second lookup")
	require.Contains(t, out, `"title": "Canonical schema version"`)
	require.Contains(t, out, `"remedy": "tiller approve feature `+cliFeatureKey+` --reviewed-assumption `+qid+`"`)

	stdout, stderr, code := execCLI(t, "submit", "task", taskID, "--pr", "7", "--contributor", "jc")
	require.Equal(t, 1, code)
	humanOut := stdout + stderr
	require.Contains(t, humanOut, "task "+cliFeatureKey+"/transcribe-the-schema (",
		"the refusal names the node the caller addressed")
	require.Contains(t, humanOut, "feature "+cliFeatureKey+" (",
		"and the feature that carries the stale entry")
	require.Contains(t, humanOut, qid+` ("Canonical schema version")`,
		"the human review context names the question by id plus bounded title")
	require.NotContains(t, humanOut, task4LongQuestionStatement,
		"the human refusal keeps the full review statement on the structured wire only")
	require.Contains(t, humanOut, "tiller approve feature "+cliFeatureKey+" --reviewed-assumption "+qid,
		"the remedy addresses the feature, never the task: a task command for a feature's entry is a refusal loop")

	require.Equal(t, "check", soleTaskStatus(t, taskID), "a refused submit must not move the task")
}

// TestSubmitPassesOnceReviewedInCLI is the CLI proof that the remedy the
// refusal names actually works: running the exact command the refusal above
// prints clears the stale entry, and the identical submit that was just
// refused now succeeds.
func TestSubmitPassesOnceReviewedInCLI(t *testing.T) {
	_, taskID, qid := layInProgressTaskUnderAssumedFeatureCLI(t)

	_, _, code := execCLI(t, "submit", "task", taskID, "--pr", "7", "--contributor", "jc")
	require.Equal(t, 1, code, "sanity: the refusal is real before the remedy runs")

	mustRun(t, "approve", "feature", cliFeatureKey, "--reviewed-assumption", qid, "--contributor", "jc")

	out, _, code := execCLI(t, "submit", "task", taskID, "--pr", "7", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, "the exact remedy the refusal named must clear it")
	require.Contains(t, out, `"status": "in_review"`)
	require.Equal(t, "in_review", soleTaskStatus(t, taskID))
}

// --- Follow-up ---

// TestSubmitEpicCLINamesNoLiveTaskRatherThanTasksIncompleteWording is the
// CLI-level pin of item 6, which the final-review fix
// (commit 7657882) fixed only at the mutate layer
// (mutate.FeatureNotSubmittableError.Error()): reportSubmitNodeError's own
// rendering here still unconditionally printed "has a task not yet in_review
// or done" for every tasks_incomplete refusal, so a real `tiller submit epic
// <key>` against a live feature with zero tasks at all printed exactly the
// false statement item 6 already fixed underneath. Mirrors
// TaskNotSubmittableError's own conditional branch shape one grain down
// (`if taskBlk.TaskID != ""`). Checks both faces: the human line (plain
// output goes to stderr on a blocked, non-zero exit) and the --json data
// block's new additive `task` field, which must stay absent (omitempty) when
// there is no blocking task id to name.
func TestSubmitEpicCLINamesNoLiveTaskRatherThanTasksIncompleteWording(t *testing.T) {
	initGatedGitRepo(t, "true")
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "epic", "form-engine", "--review-grain", "epic", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", cliFeatureKey, "--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)

	_, errOut, code := execCLI(t, "submit", "epic", "form-engine", "--pr", "50", "--contributor", "jc")
	require.Equal(t, 1, code, errOut)
	require.Contains(t, errOut, "Blocked: epic")
	require.Contains(t, errOut, "has no live task to review.",
		"a zero-task feature has nothing to be short of in_review, so this is the wording, not the generic one")
	require.NotContains(t, errOut, "not yet in_review or done")

	out, _, code := execCLI(t, "submit", "epic", "form-engine", "--pr", "50", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"reason\": \"tasks_incomplete\"")
	require.Contains(t, out, "\"feature\":")
	require.NotContains(t, out, "\"task\":",
		"task is additive and omitempty; it must not render when there is no blocking task id")
}

func TestSubmitFeatureRefusesActiveBasisDriftAfterGreenCheck(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	walkFeatureToActive(t)
	_, _, code := execCLI(t, "configure", "feature", cliFeatureKey,
		"--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code)

	tid := soleTaskID(t)
	for _, argv := range [][]string{
		{"start", "task", tid, "--contributor", "jc"},
		{"check", "task", tid, "--contributor", "jc"},
		{"submit", "task", tid, "--contributor", "jc"},
		{"check", "feature", cliFeatureKey, "--contributor", "jc"},
	} {
		_, stderr, stepCode := execCLI(t, argv...)
		require.Equalf(t, 0, stepCode, "%v: %s", argv, stderr)
		if argv[0] == "start" {
			checkoutTaskBranchCLI(t, tid)
		}
	}
	assembleTaskForReviewCLI(t, tid, soleFeatureBranch(t))

	featurePath := filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.md")
	before, err := os.ReadFile(featurePath)
	require.NoError(t, err)
	after := strings.Replace(string(before),
		"Load the CERT-LTI-02-A form from its JSON Schema and render it.",
		"Build materially different behavior.", 1)
	require.NotEqual(t, string(before), after)
	require.NoError(t, os.WriteFile(featurePath, []byte(after), 0o644))

	out, _, code := execCLI(t, "submit", "feature", cliFeatureKey,
		"--pr", "77", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, `"reason": "active_basis"`)
	require.Contains(t, out, "feature.md")
	require.Nil(t, loadTestTree(t).Features[featureIDByKey(t, cliFeatureKey)].PR)
	queue, _ := runQueueCommandJSON(t, "--kind", "merge_feature")
	require.Empty(t, queue.Entries, "a refused coarse submit must not create a merge card")
}

func TestSubmitFeatureRefusesLegacyTaskReviewOpen(t *testing.T) {
	initGatedGitRepo(t, "true")
	walkFeatureToActive(t)
	tid := soleTaskID(t)
	for _, argv := range [][]string{
		{"start", "task", tid, "--contributor", "jc"},
		{"check", "task", tid, "--contributor", "jc"},
		{"submit", "task", tid, "--pr", "41", "--contributor", "jc"},
		{"check", "feature", cliFeatureKey, "--contributor", "jc"},
	} {
		_, stderr, stepCode := execCLI(t, argv...)
		require.Equalf(t, 0, stepCode, "%v: %s", argv, stderr)
		if argv[0] == "start" {
			checkoutTaskBranchCLI(t, tid)
		}
	}
	setFeatureReviewGrainOnDisk(t, store.ReviewGrainFeature)

	out, _, code := execCLI(t, "submit", "feature", cliFeatureKey,
		"--pr", "99", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, `"reason": "task_review_open"`)
	require.Contains(t, out, `"task": "`+tid+`"`)
	require.Contains(t, out, `"pr": 41`)
	require.Nil(t, loadTestTree(t).Features[featureIDByKey(t, cliFeatureKey)].PR)

	_, human, code := execCLI(t, "submit", "feature", cliFeatureKey,
		"--pr", "99", "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, human, "PR #41")
	require.Contains(t, human, "tiller sync")
}

func TestSubmitEpicRendersIndependentFeatureReviewOpen(t *testing.T) {
	initGatedGitRepo(t, "true")
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "configure", "epic", "form-engine",
		"--review-grain", "epic", "--contributor", "jc")
	require.Equal(t, 0, code)

	overrideKey := "form-engine/alpha"
	ridingKey := "form-engine/beta"
	for _, key := range []string{overrideKey, ridingKey} {
		_, _, code = execCLI(t, "add", "feature", key,
			"--title", key, "--change-type", "create", "--contributor", "jc")
		require.Equal(t, 0, code)
	}
	_, _, code = execCLI(t, "configure", "feature", overrideKey,
		"--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "submit", "epic", "form-engine",
		"--pr", "77", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, `"reason": "independent_review_open"`)
	require.Contains(t, out, `"feature_key": "`+overrideKey+`"`)
	require.Nil(t, loadTestTree(t).Epics[soleEpicID(t)].PR)

	_, human, code := execCLI(t, "submit", "epic", "form-engine",
		"--pr", "77", "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, human, "own review reach done")
	require.Contains(t, human, "before the epic PR opens")
}

func TestSubmitFeatureQuestionRefusalHumanNamesOnlySupportedRemedy(t *testing.T) {
	initGatedGitRepo(t, "true")
	walkFeatureToActive(t)
	_, _, code := execCLI(t, "configure", "feature", cliFeatureKey,
		"--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code)
	taskID := soleTaskID(t)
	for _, argv := range [][]string{
		{"start", "task", taskID, "--contributor", "jc"},
		{"check", "task", taskID, "--contributor", "jc"},
		{"submit", "task", taskID, "--contributor", "jc"},
		{"check", "feature", cliFeatureKey, "--contributor", "jc"},
		{"add", "question", "Which schema revision is canonical?", "--title", "Schema revision",
			"--blocks", cliFeatureKey, "--contributor", "jc"},
	} {
		_, stderr, stepCode := execCLI(t, argv...)
		require.Equalf(t, 0, stepCode, "%v: %s", argv, stderr)
		if argv[0] == "start" {
			checkoutTaskBranchCLI(t, taskID)
		}
	}

	_, human, code := execCLI(t, "submit", "feature", cliFeatureKey, "--pr", "77", "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, human, `answer the questions, or cancel one that no longer needs a decision with tiller cancel question <Q-id> --reason "...", and retry.`)
	require.NotContains(t, human, "--assume-question")

	jsonOut, _, code := execCLI(t, "submit", "feature", cliFeatureKey, "--pr", "77", "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, jsonOut, `"reason": "open_question_blocker"`)
}

func TestSubmitEpicQuestionRefusalHumanNamesOnlySupportedRemedy(t *testing.T) {
	initGatedGitRepo(t, "true")
	laySpeccableFeature(t)
	for _, argv := range [][]string{
		{"configure", "epic", "form-engine", "--review-grain", "epic", "--contributor", "jc"},
		{"approve", "epic", "form-engine", "--contributor", "jc"},
		{"spec", "feature", cliFeatureKey, "--contributor", "jc"},
		{"approve", "feature", cliFeatureKey, "--contributor", "jc"},
		{"claim", "task", cliTaskKey, "--contributor", "jc"},
	} {
		_, stderr, stepCode := execCLI(t, argv...)
		require.Equalf(t, 0, stepCode, "%v: %s", argv, stderr)
	}
	taskID := soleTaskID(t)
	for _, argv := range [][]string{
		{"start", "task", taskID, "--contributor", "jc"},
		{"check", "task", taskID, "--contributor", "jc"},
		{"submit", "task", taskID, "--contributor", "jc"},
		{"check", "feature", cliFeatureKey, "--contributor", "jc"},
		{"add", "question", "Which schema revision is canonical?", "--title", "Schema revision",
			"--blocks", cliFeatureKey, "--contributor", "jc"},
	} {
		_, stderr, stepCode := execCLI(t, argv...)
		require.Equalf(t, 0, stepCode, "%v: %s", argv, stderr)
		if argv[0] == "start" {
			checkoutTaskBranchCLI(t, taskID)
		}
	}

	_, human, code := execCLI(t, "submit", "epic", "form-engine", "--pr", "77", "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, human, `answer the questions, or cancel one that no longer needs a decision with tiller cancel question <Q-id> --reason "...", and retry.`)
	require.NotContains(t, human, "--assume-question")

	jsonOut, _, code := execCLI(t, "submit", "epic", "form-engine", "--pr", "77", "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, jsonOut, `"reason": "open_question_blocker"`)
}

func layBugCLI(t *testing.T) {
	t.Helper()
	initOwnedRepo(t)
	mustRun(t, "add", "bug", "alpha", "--title", "Alpha", "--contributor", "jc")
	require.NoError(t, os.WriteFile(filepath.Join(".tiller", "bugs", "alpha", "bug.md"), []byte("# Alpha\n\n## Reproduction\n\nx\n"), 0o644))
	gitRun(t, ".", "add", "-A")
	gitRun(t, ".", "commit", "-q", "-m", "lay bug")
}

func TestApproveReturnAndCancelBugCommands(t *testing.T) {
	layBugCLI(t)
	out, stderr, code := execCLI(t, "approve", "bug", "alpha", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "ready")
	_, stderr, code = execCLI(t, "return", "bug", "alpha", "--reason", "rework", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	out, _, code = execCLI(t, "cancel", "bug", "alpha", "--reason", "no", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "resolution_required"`)
	out, stderr, code = execCLI(t, "cancel", "bug", "alpha", "--reason", "no", "--resolution", "wontfix", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "cancelled")

	out, _, code = execCLI(t, "cancel", "task", "form-engine/x/y", "--reason", "no", "--resolution", "wontfix", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "bad_args"`)
}

func TestStartBugRefusesUnclaimedAsBlocked(t *testing.T) {
	layBugCLI(t)
	mustRun(t, "approve", "bug", "alpha", "--contributor", "jc")
	out, _, code := execCLI(t, "start", "bug", "alpha", "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, `"reason": "work_unclaimed"`)
	require.Contains(t, out, "tiller claim bug bugs/alpha")
}

func TestCheckSubmitResumeBugCommands(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	_ = dir
	mustRun(t, "add", "bug", "alpha", "--title", "Alpha", "--contributor", "jc")
	require.NoError(t, os.WriteFile(filepath.Join(".tiller", "bugs", "alpha", "bug.md"), []byte("# Alpha\n\n## Reproduction\n\nx\n"), 0o644))
	gitRun(t, ".", "add", "-A")
	gitRun(t, ".", "commit", "-q", "-m", "lay bug")
	mustRun(t, "approve", "bug", "alpha", "--contributor", "jc")
	mustRun(t, "claim", "bug", "alpha", "--contributor", "jc")
	mustRun(t, "start", "bug", "alpha", "--contributor", "jc")
	gitRun(t, dir, "checkout", "-b", "tiller/bugs/alpha")
	out, stderr, code := execCLI(t, "check", "bug", "alpha", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "check")

	// No remote: submit records a review ref; --pr is never refused for grain.
	out, stderr, code = execCLI(t, "submit", "bug", "alpha", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, "in_review", data["status"])
	require.NotNil(t, data["review_ref"])

	_, stderr, code = execCLI(t, "resume", "bug", "alpha", "--reason", "changes", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
}

// adoptAggregateFixture walks the shared feature to active at feature grain and
// hand-builds a ref at its derived aggregate name holding one commit beyond
// integration, the state the field run left behind.
func adoptAggregateFixture(t *testing.T) string {
	t.Helper()
	dir := initGatedGitRepo(t, "true")
	walkFeatureToActive(t)
	_, stderr, code := execCLI(t, "configure", "feature", cliFeatureKey, "--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	agg := "tiller/reviews/features/" + cliFeatureKey
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "plan")
	gitRun(t, dir, "checkout", "-b", agg)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assembled.txt"), []byte("hand-built\n"), 0o644))
	gitRun(t, dir, "add", "assembled.txt")
	gitRun(t, dir, "commit", "-m", "hand-built aggregate")
	gitRun(t, dir, "checkout", "main")
	return agg
}

// TestStartAdoptAggregateFlag pins the CLI surface: --adopt-aggregate
// records the existing ref and reports the commits it holds beyond
// integration, in JSON and in the human line.
func TestStartAdoptAggregateFlag(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		agg := adoptAggregateFixture(t)
		stdout, stderr, code := execCLI(t, "start", "task", cliTaskKey, "--adopt-aggregate", "--contributor", "jc", "--json")
		require.Equal(t, 0, code, stdout+stderr)
		data := jsonData(t, stdout)
		require.Equal(t, agg, data["aggregate_branch"])
		commits, ok := data["adopted_commits"].([]any)
		require.True(t, ok, stdout)
		require.Len(t, commits, 1)
		require.Contains(t, commits[0], "hand-built aggregate")
		require.Equal(t, agg, loadCwdTree(t).Features[soleFeatureID(t)].Branch)
	})
	t.Run("human", func(t *testing.T) {
		agg := adoptAggregateFixture(t)
		stdout, stderr, code := execCLI(t, "start", "task", cliTaskKey, "--adopt-aggregate", "--contributor", "jc")
		require.Equal(t, 0, code, stdout+stderr)
		require.Contains(t, stdout, "Adopted existing aggregate "+agg+"; it holds 1 commit(s) beyond integration:")
		require.Contains(t, stdout, "hand-built aggregate")
	})
	t.Run("plain start names the unrecorded ref", func(t *testing.T) {
		agg := adoptAggregateFixture(t)
		stdout, stderr, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
		require.Equal(t, 1, code, stdout+stderr)
		human := stdout + stderr
		require.Contains(t, human, fmt.Sprintf("branch %q exists but is not recorded on feature", agg))
		require.Contains(t, human, "--adopt-aggregate")
		require.NotContains(t, human, fmt.Sprintf("conflicts with %q", agg))
	})
}

// TestTaskDependencyBlockedLineNamesUnrecordedAggregate pins the refusal that
// used to send an agent to assemble into an aggregate nothing had recorded.
func TestTaskDependencyBlockedLineNamesUnrecordedAggregate(t *testing.T) {
	tree := &store.Tree{}
	data, human, ok := branchBlockedView(tree, &mutate.TaskDependencyBlockedError{
		TaskID: "T-bbbbbb", DependencyID: "T-aaaaaa", DependencyStatus: "in_review",
		Branch: "tiller/reviews/features/pay/checkout", AggregateUnrecorded: true,
	})
	require.True(t, ok)
	require.True(t, data.(branchBlockedData).AggregateUnrecorded)
	require.Equal(t, "Blocked: task T-bbbbbb requires T-aaaaaa; aggregate tiller/reviews/features/pay/checkout is not recorded and does not exist. Build it containing the prerequisite's complete branch, then start with --adopt-aggregate.", human)
}

// An adopted aggregate holding nothing beyond integration says so rather than
// ending on a colon with no list.
func TestStartAdoptAggregateHumanLineWithNoCommits(t *testing.T) {
	line := humanTransition(&store.Tree{}, "start", mutate.TransitionInput{Verb: "start", AdoptAggregate: true},
		mutate.TransitionResult{Noun: "task", Status: "in_progress", Changed: true, AggregateBranch: "tiller/reviews/epics/pay"}, "pay/checkout/alfa", nil)
	require.Contains(t, line, "Adopted existing aggregate tiller/reviews/epics/pay; it holds nothing beyond integration.")
	require.NotContains(t, line, "commit(s)")
}

// staleTaskGrainCLIFixture is the timeline at the CLI: the task starts
// and checks at task grain, then the feature's grain changes to feature, with
// a remote configured. It returns the task id.
func staleTaskGrainCLIFixture(t *testing.T) string {
	t.Helper()
	dir := initGatedGitRepo(t, "true")
	gitRun(t, dir, "remote", "add", "origin", "/tmp/anything.git")
	tid := walkTaskToCheck(t)
	setFeatureReviewGrainOnDisk(t, store.ReviewGrainFeature)
	return tid
}

func TestStaleTaskGrainTaskSubmitRefusalNamesStartedGrain(t *testing.T) {
	tid := staleTaskGrainCLIFixture(t)

	out, _, code := execCLI(t, "submit", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "pr_required"`)
	require.Contains(t, out, "resolves to review grain task (started at task grain)")

	out, _, code = execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, `"pr": 41`)
	require.Equal(t, 41, *loadTestTree(t).Tasks[tid].PR)
}

func TestSubmitFeatureTaskReviewOpenNamesStartedGrain(t *testing.T) {
	tid := staleTaskGrainCLIFixture(t)
	_, stderr, code := execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	stdout, stderr, code := execCLI(t, "submit", "feature", cliFeatureKey, "--pr", "99", "--contributor", "jc")
	require.Equal(t, 1, code)
	human := stdout + stderr
	require.Contains(t, human, "PR #41")
	require.Contains(t, human, "started at task grain, so it takes its own PR and must merge before this submit.")
}

// TestPRRequiredNamesTheGrainSource pins each source label the task form of
// pr_required carries; feature and epic submits keep the bare text.
func TestPRRequiredNamesTheGrainSource(t *testing.T) {
	epicKey, featureKey := "pay", "pay/checkout"
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Epics:    map[string]*store.Epic{"E-aaaaaa": {ID: "E-aaaaaa", Key: &epicKey}},
		Features: map[string]*store.Feature{"F-aaaaaa": {ID: "F-aaaaaa", Key: &featureKey, EpicKey: epicKey}},
	}
	f, e := tree.Features["F-aaaaaa"], tree.Epics["E-aaaaaa"]
	require.Equal(t, "started at task grain", taskGrainSourceLabel(tree, e, f, "started"))
	require.Equal(t, "feature "+gen.Ref(tree, f)+" in this checkout", taskGrainSourceLabel(tree, e, f, "feature"))
	require.Equal(t, "epic "+gen.Ref(tree, e)+" in this checkout", taskGrainSourceLabel(tree, e, f, "epic"))
	require.Equal(t, "project setting", taskGrainSourceLabel(tree, e, f, "project"))
	require.Equal(t, "default", taskGrainSourceLabel(tree, e, f, "default"))
}

// Controller ruling P2: a task-grain prerequisite never enters the aggregate,
// so the started-grain text wins. Start no longer sets AggregateUnrecorded for
// such a prerequisite; this pins the renderer's precedence should both arrive.
func TestTaskDependencyBlockedLineStartedGrainBeatsUnrecordedAggregate(t *testing.T) {
	data, human, ok := branchBlockedView(&store.Tree{}, &mutate.TaskDependencyBlockedError{
		TaskID: "T-bbbbbb", DependencyID: "T-aaaaaa", DependencyStatus: "in_review",
		Branch: "tiller/reviews/features/pay/checkout", AggregateUnrecorded: true, DependencyStartedGrain: "task",
	})
	require.True(t, ok)
	require.Equal(t, "task", data.(branchBlockedData).DependencyStartedGrain)
	require.Equal(t, "Blocked: task T-bbbbbb requires T-aaaaaa, which started at task grain; wait for its PR to merge and sync before starting.", human)
	require.Contains(t, human, "wait for its PR to merge and sync")
	require.NotContains(t, human, "assemble")
	require.NotContains(t, human, "--adopt-aggregate")
}

// A remoteless task-grain task holds only a review_ref. The refusal names
// that review, never "PR #0", and never asks to assemble the task into the
// aggregate.
func TestCoarseSubmitRefusesRemotelessTaskGrainReviewRef(t *testing.T) {
	for _, noun := range []string{store.NounFeature, store.NounEpic} {
		t.Run(noun, func(t *testing.T) {
			dir := initGatedGitRepo(t, "true")
			tid := walkTaskToCheck(t)
			_, stderr, code := execCLI(t, "submit", "task", tid, "--contributor", "jc")
			require.Equal(t, 0, code, stderr)
			key := cliFeatureKey
			if noun == store.NounEpic {
				key = "form-engine"
				path := filepath.Join(dir, ".tiller", "epics", "form-engine", "epic.yml")
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, append(data, []byte("review_grain: epic\n")...), 0o644))
			} else {
				setFeatureReviewGrainOnDisk(t, store.ReviewGrainFeature)
			}
			_, stderr, code = execCLI(t, "check", "feature", cliFeatureKey, "--contributor", "jc")
			require.Equal(t, 0, code, stderr)

			out, _, code := execCLI(t, "submit", noun, key, "--pr", "99", "--contributor", "jc", "--json")
			require.Equal(t, 1, code, out)
			require.Contains(t, out, `"reason": "task_review_open"`)
			require.Contains(t, out, `"review_branch": "`+loadTestTree(t).Tasks[tid].Branch+`"`)
			require.NotContains(t, out, `"pr"`)

			stdout, stderr, code := execCLI(t, "submit", noun, key, "--pr", "99", "--contributor", "jc")
			require.Equal(t, 1, code)
			human := stdout + stderr
			require.Contains(t, human, "has its own review open on "+loadTestTree(t).Tasks[tid].Branch)
			require.Contains(t, human, "started at task grain, so it takes its own PR or review ref and must merge before this submit.")
			require.NotContains(t, human, "PR #")
			require.NotContains(t, human, "into the aggregate")
		})
	}
}
