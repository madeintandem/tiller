package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestConfigureFeatureRecordsTheGrain drives `configure feature <key>
// --review-grain feature` end to end: init, add contributor (initOwnedRepo, layered
// through layReadTree), add epic and feature, then configure. It asserts both
// the persisted feature.yml field and the human confirmation line.
func TestConfigureFeatureRecordsTheGrain(t *testing.T) {
	layReadTree(t)

	out, _, code := execCLI(t, "configure", "feature", "form-engine/certification-form-loading",
		"--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "review grain: feature recorded on")
	require.Contains(t, out, "certification-form-loading")
	require.Contains(t, out, "resolved grain here is now: feature (feature explicit)")

	tree := loadTestTree(t)
	f := tree.Features[soleFeatureID(t)]
	require.Equal(t, "feature", f.ReviewGrain)
}

// TestConfigureReviewGrainChangeSaysCommitBeforeStarting pins the commit
// nudge: a grain that actually changed says to commit it to integration before
// starting tasks, while a re-record of the same grain (no change) does not.
func TestConfigureReviewGrainChangeSaysCommitBeforeStarting(t *testing.T) {
	layReadTree(t)

	out, _, code := execCLI(t, "configure", "project", "--review-grain", "epic", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "Commit this change to integration before starting tasks")

	out, _, code = execCLI(t, "configure", "project", "--review-grain", "epic", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "(no change)")
	require.NotContains(t, out, "Commit this change to integration")
}

// TestConfigureBareInvocationRefusesNamingTheThreeTargets pins the design's
// own rule: a bare `tiller configure --review-grain <g>` refuses at exit 2
// because the omitted-target reading would be the widest possible scope, and
// the message names all three legal targets.
func TestConfigureBareInvocationRefusesNamingTheThreeTargets(t *testing.T) {
	layReadTree(t)

	stdout, stderr, code := execCLI(t, "configure", "--review-grain", "feature", "--contributor", "jc")
	message := stdout + stderr
	require.Equal(t, 2, code, message)
	require.Contains(t, message, "project")
	require.Contains(t, message, "epic")
	require.Contains(t, message, "feature")
}

func TestConfigureProjectOutsideRepoPreservesNotARepoError(t *testing.T) {
	t.Chdir(t.TempDir())

	stdout, stderr, code := execCLI(t, "configure", "project", "--review-grain", "feature", "--contributor", "jc", "--json")
	output := stdout + stderr
	require.Equal(t, 3, code, output)
	require.Contains(t, output, `"reason": "not_a_repo"`)
}

// TestConfigureProjectRecordsTheGrain pins the project home: no key, writes
// execution.review_grain into tiller.yml, and reads back "project default".
func TestConfigureProjectRecordsTheGrain(t *testing.T) {
	layReadTree(t)

	out, _, code := execCLI(t, "configure", "project", "--review-grain", "epic", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "review grain: epic recorded for the project")
	require.Contains(t, out, "resolved grain here is now: epic (project default)")

	tree := loadTestTree(t)
	require.Equal(t, "epic", tree.Manifest.Execution.ReviewGrain)
}

func TestConfigureProjectRefusesFlowExecutionWithoutWriting(t *testing.T) {
	layReadTree(t)
	root, err := os.Getwd()
	require.NoError(t, err)
	path := filepath.Join(root, ".tiller", "tiller.yml")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	before = append(bytes.TrimRight(before, "\n"), []byte("\nexecution: {review_grain: task}\n")...)
	require.NoError(t, os.WriteFile(path, before, 0o644))
	eventsBefore := countEvents(t)

	stdout, stderr, code := execCLI(t, "configure", "project", "--review-grain", "feature", "--contributor", "jc", "--json")
	output := stdout + stderr
	require.Equal(t, 2, code, output)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, eventsBefore, countEvents(t))
	require.Contains(t, output, `"reason": "unsupported_manifest_shape"`)
}

func TestConfigureProjectRefusesUnsupportedExecutionShapesBeforeLoadOrNoOp(t *testing.T) {
	for _, tc := range []struct {
		name  string
		line  string
		grain string
	}{
		{name: "scalar before load", line: "execution: feature", grain: "task"},
		{name: "equal flow mapping before no-op", line: "execution: {review_grain: feature}", grain: "feature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layReadTree(t)
			root, err := os.Getwd()
			require.NoError(t, err)
			path := filepath.Join(root, ".tiller", "tiller.yml")
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			before = append(bytes.TrimRight(before, "\n"), []byte("\n"+tc.line+"\n")...)
			require.NoError(t, os.WriteFile(path, before, 0o644))
			eventsBefore := countEvents(t)

			stdout, stderr, code := execCLI(t, "configure", "project", "--review-grain", tc.grain, "--contributor", "jc", "--json")
			output := stdout + stderr
			require.Equal(t, 2, code, output)

			after, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			require.Equal(t, before, after)
			require.Equal(t, eventsBefore, countEvents(t))
			require.Contains(t, output, `"reason": "unsupported_manifest_shape"`)
		})
	}
}

// TestConfigureEpicRecordsTheGrainAndOffersTheNextGate pins the epic home and
// the **Next** block: a draft epic still awaits its own approve gate, so
// configuring it prints the next runnable command.
func TestConfigureEpicRecordsTheGrainAndOffersTheNextGate(t *testing.T) {
	layReadTree(t)

	out, _, code := execCLI(t, "configure", "epic", "form-engine", "--review-grain", "epic", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "review grain: epic recorded on")
	require.Contains(t, out, "resolved grain here is now: epic (epic explicit)")
	require.Contains(t, out, "**Next**")
	require.Contains(t, out, "tiller approve epic form-engine")

	tree := loadTestTree(t)
	require.Equal(t, "epic", tree.Epics[soleEpicID(t)].ReviewGrain)
}

// TestConfigureFeatureOmitsNextBlockOnceApproved pins the other half of the
// **Next** rule: once the feature's own approve gate is behind it, there is
// no gate left to name, so no **Next** block prints.
func TestConfigureFeatureOmitsNextBlockOnceApproved(t *testing.T) {
	laySpeccableFeature(t)
	_, _, code := execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "configure", "feature", "form-engine/certification-form-loading",
		"--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.NotContains(t, out, "**Next**")
}

// TestConfigureFeatureNamesTheAnswerWhileAQuestionHoldsThePlan: a specced
// feature still sits ahead of its approve, but an open question holding the
// plan makes that approve refuse, so the **Next**
// block names the answer rather than a command that exits 1 when copied.
func TestConfigureFeatureNamesTheAnswerWhileAQuestionHoldsThePlan(t *testing.T) {
	laySpeccableFeature(t)
	_, _, code := execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	writeBlockingQuestion(t, "Q-aaaaaa", soleTaskID(t))

	out, _, code := execCLI(t, "configure", "feature", "form-engine/certification-form-loading",
		"--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "**Next**")
	require.Contains(t, out, `tiller answer question Q-aaaaaa --answer "..."`)
	require.NotContains(t, out, "tiller approve feature")
}

// TestConfigureRefusesOnceTheNodesOwnReviewSurfaceExists pins Changeability
// (design doc): once a feature's own PR is recorded, its grain is locked, and
// the refusal is a blocked business answer (exit 1, ok:false, data block),
// never a tool failure.
func TestConfigureRefusesOnceTheNodesOwnReviewSurfaceExists(t *testing.T) {
	layApprovedFeatureWithTask(t)
	_, _, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
	require.Equal(t, 0, code)

	tree := loadTestTree(t)
	feature := tree.Features[soleFeatureID(t)]
	pr := 99
	feature.PR = &pr
	featureYAML, err := yaml.Marshal(feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.yml"), featureYAML, 0o644))

	out, _, code := execCLI(t, "configure", "feature", cliFeatureKey,
		"--review-grain", "feature", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, "\"ok\": false")
	require.Contains(t, out, "\"reason\": \"review_grain_locked\"")
	require.NotContains(t, out, "\"error\"")
}

// TestConfigureUnknownTargetAndMissingKeyAreMisuse rounds out the CLI
// grammar: an unrecognized target, and a target that needs a key but did not
// get one, are both exit-2 misuse.
func TestConfigureUnknownTargetAndMissingKeyAreMisuse(t *testing.T) {
	layReadTree(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "unknown target", args: []string{"configure", "bogus", "--review-grain", "task", "--contributor", "jc"}},
		{name: "epic missing key", args: []string{"configure", "epic", "--review-grain", "task", "--contributor", "jc"}},
		{name: "project with a key", args: []string{"configure", "project", "form-engine", "--review-grain", "task", "--contributor", "jc"}},
		{name: "missing review-grain", args: []string{"configure", "epic", "form-engine", "--contributor", "jc"}},
		{name: "invalid review-grain value", args: []string{"configure", "epic", "form-engine", "--review-grain", "bogus", "--contributor", "jc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := execCLI(t, tc.args...)
			require.Equal(t, 2, code, stdout+stderr)
		})
	}
}

// TestConfigureJSONEnvelopeKeepsIDAndKeySeparate pins the --json contract:
// id and key are separate structured fields, never glued together.
func TestConfigureJSONEnvelopeKeepsIDAndKeySeparate(t *testing.T) {
	layReadTree(t)

	out, _, code := execCLI(t, "configure", "feature", "form-engine/certification-form-loading",
		"--review-grain", "feature", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "\"id\":")
	require.Contains(t, out, "\"key\": \"form-engine/certification-form-loading\"")
	require.Contains(t, out, "\"review_grain\": \"feature\"")
}

func TestConfigureReviewGrainWorkInFlightRendersStableBlockedAnswer(t *testing.T) {
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
			args := []string{"configure", "feature", cliFeatureKey,
				"--review-grain", "feature", "--contributor", "jc"}
			if jsonOutput {
				args = append(args, "--json")
			}

			stdout, stderr, code := execCLI(t, args...)
			output := stdout + stderr

			require.Equal(t, 1, code, output)
			if jsonOutput {
				require.Contains(t, output, `"reason": "review_grain_work_in_flight"`)
				require.Contains(t, output, `"blocker_noun": "task"`)
				require.Contains(t, output, `"blocker_id": "`+taskID+`"`)
				require.Contains(t, output, `"blocker_status": "in_review"`)
				require.Contains(t, output, `"before_grain": "task"`)
				require.Contains(t, output, `"after_grain": "feature"`)
				require.NotContains(t, output, `"reason": "error"`)
				return
			}
			require.Equal(t, "Blocked: changing review grain on "+gen.RefByID(tree, featureID)+
				" would retarget "+gen.RefByID(tree, taskID)+
				", currently in_review; finish and sync that review first.\n", output)
		})
	}
}

func TestConfigureReviewGrainBranchLockedRendersStableBlockedAnswer(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		name := "human"
		if jsonOutput {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			layApprovedFeatureWithTask(t)
			out, _, code := execCLI(t, "configure", "project", "--review-grain", "feature", "--contributor", "jc")
			require.Equal(t, 0, code, out)
			_, _, code = execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
			require.Equal(t, 0, code)
			mustRun(t, "cancel", "task", cliTaskKey, "--reason", "retire task without erasing the aggregate branch", "--contributor", "jc")
			tree := loadTestTree(t)
			epicID := soleEpicID(t)
			featureID := soleFeatureID(t)
			args := []string{"configure", "epic", "form-engine",
				"--review-grain", "epic", "--contributor", "jc"}
			if jsonOutput {
				args = append(args, "--json")
			}

			stdout, stderr, code := execCLI(t, args...)
			output := stdout + stderr

			require.Equal(t, 1, code, output)
			if jsonOutput {
				require.Contains(t, output, `"reason": "review_grain_branch_locked"`)
				require.Contains(t, output, `"blocker_id": "`+featureID+`"`)
				require.NotContains(t, output, `"reason": "error"`)
				return
			}
			require.Contains(t, output, gen.RefByID(tree, epicID))
			require.Contains(t, output, gen.RefByID(tree, featureID))
		})
	}
}

func TestReviewGrainChangeBlockedGenericFallbackPreservesReason(t *testing.T) {
	body := classify(&mutate.ReviewGrainChangeBlockedError{
		Reason: mutate.ReasonReviewGrainBranchLocked,
	})

	require.Equal(t, 2, body.Code)
	require.Equal(t, mutate.ReasonReviewGrainBranchLocked, body.Reason)
}
