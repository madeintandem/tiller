package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin the human lines of the structural blocked arms
// (transitionBlockedView), which until now were covered only through their
// JSON reason codes: each arm must name the blocked node by the shared
// reference convention, "<key-path> (<id>)". The blocker errors carry bare
// ids, so these lines are where the id-to-ref resolution through the tree is
// proven at the CLI surface. A blocked business answer prints on stderr for a
// single-node command, so every assertion reads stdout+stderr. Minted ids are
// random and read off disk.

// soleEpicID reads the one epic's minted id off disk.
func soleEpicID(t *testing.T) string {
	t.Helper()
	tree := loadTestTree(t)
	require.Len(t, tree.Epics, 1)
	for id := range tree.Epics {
		return id
	}
	return ""
}

// TestSpecIncompleteBlockedLineNamesFeatureByRef pins the spec_incomplete arm:
// speccing a feature whose spec files are still stubs is refused with a human
// line naming the feature by its full reference, never a bare id.
func TestSpecIncompleteBlockedLineNamesFeatureByRef(t *testing.T) {
	layReadTree(t) // stub feature.md, no eval: the spec is incomplete
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")

	stdout, stderr, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 1, code, "an incomplete spec is a blocked business answer (exit 1)")
	require.Contains(t, stdout+stderr,
		fmt.Sprintf("Blocked: feature form-engine/certification-form-loading (%s) is not specced", soleFeatureID(t)))
	// The line names the folder the files live in and points at the guide: a
	// dry run bounced between spec and approve because nothing said where to
	// write or that spec asserts rather than writes (JC, 2026-07-30).
	require.Contains(t, stdout+stderr,
		"write feature.md, feature.eval.md under .tiller/epics/form-engine/features/certification-form-loading/")
	require.Contains(t, stdout+stderr, "run `tiller guide specify` for the walkthrough")
}

// TestSpeccedFeatureWithUnwrittenOutputIsNotCalledUnspecced pins #145's wording:
// re-spec and the plan gate re-validate a specced feature, and the blocked line
// must not claim the feature "is not specced" when its status says it is.
func TestSpeccedFeatureWithUnwrittenOutputIsNotCalledUnspecced(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	_, _, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), store.TaskDirName, "transcribe-the-schema", store.TaskProseFile), nil, 0o644))

	for _, verb := range []string{"spec", "approve"} {
		stdout, stderr, code := execCLI(t, verb, "feature", "form-engine/certification-form-loading", "--contributor", "jc")
		require.Equal(t, 1, code, verb)
		require.NotContains(t, stdout+stderr, "is not specced", verb)
		require.Contains(t, stdout+stderr, fmt.Sprintf(
			"Blocked: feature form-engine/certification-form-loading (%s) is specced, but its specify output no longer passes; write tasks/transcribe-the-schema/task.md", soleFeatureID(t)), verb)
	}
}

// TestQuestionBlockedLineNamesTaskByRef pins the open_question_blocker arm: a
// ready task held by an open question refuses start with a human line naming
// the task by its borrowed feature path plus id, then the blocker by id plus
// bounded title, without expanding the full statement.
func TestQuestionBlockedLineNamesTaskByRef(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	_, _, code := execCLI(t, "approve", "task", ids[0], "--contributor", "jc")
	require.Equal(t, 0, code)
	writeBlockingQuestion(t, "Q-aaaaaa", ids[0])

	stdout, stderr, code := execCLI(t, "start", "task", ids[0], "--contributor", "jc")
	require.Equal(t, 1, code, "an open question holding the task is a blocked business answer (exit 1)")
	human := stdout + stderr
	require.Contains(t, human, fmt.Sprintf(
		`Blocked: task form-engine/certification-form-loading/schema-transcription (%s) is held by 1 open question(s): Q-aaaaaa ("Canonical schema version"); answer them, or cancel one that no longer needs a decision with tiller cancel question <Q-id> --reason "...".`,
		ids[0]))
	require.NotContains(t, human, task4LongQuestionStatement)
}

func TestTaskDependencyBlockedLineOmitsAbsentAggregate(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	late := "form-engine/certification-form-loading/late-task"
	_, stderr, code := execCLI(t, "add", "task", late, "--title", "Late task", "--contributor", "jc",
		"--depends-on", "form-engine/certification-form-loading/schema-transcription")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = execCLI(t, "approve", "task", late, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	claimTaskAsOwner(t, late, "jc")

	stdout, stderr, code := execCLI(t, "start", "task", late, "--contributor", "jc", "--json")
	require.Equal(t, 1, code, stderr)
	data := jsonData(t, stdout)
	require.Equal(t, "task_dependency_unfinished", data["reason"])
	require.Equal(t, ids[0], data["dependency"])
	require.Equal(t, "proposed", data["dependency_status"])
	require.NotContains(t, data, "branch")

	stdout, stderr, code = execCLI(t, "start", "task", late, "--contributor", "jc")
	require.Equal(t, 1, code, stderr)
	human := stdout + stderr
	require.Contains(t, human, fmt.Sprintf("form-engine/certification-form-loading/schema-transcription (%s)", ids[0]))
	require.Contains(t, human, "wait")
	require.NotContains(t, human, "assemble")
	require.NotContains(t, human, "into  before")
}
