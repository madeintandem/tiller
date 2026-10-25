package mutate

// This file pins task 4's net 1 and net 4 (issue #40, the six test nets the
// #37-#39 reviews deferred). Both nets call guardParentState directly, and it
// is unexported, so neither can live in transition_test.go or check_test.go
// (package mutate_test): they belong in the mutate package's own test binary
// instead, exactly the precedent sync_internal_test.go already set for a
// mutate-internal test that needs gen (mutate already imports gen in its own
// production files: add.go, sync.go, transition.go, so the test-side import
// below follows the same legal direction, not a new one).

import (
	"errors"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// guardMatrixFixture lays one epic, one feature under it, and one task under
// that feature, each grain's status set independently, mirroring
// gen/held_test.go's heldTaskFixture exactly: the two nets are the same
// truth-table idiom one package apart, and building the fixture the same way
// makes the two files readable side by side.
func guardMatrixFixture(epicStatus, featureStatus, taskStatus string) (*store.Tree, *store.Feature, *store.Task) {
	epicKey := "form-engine"
	featKey := "form-engine/nrca-form-loading"
	epic := &store.Epic{ID: "E-aaaaaa", Key: &epicKey, Status: epicStatus}
	feat := &store.Feature{ID: "F-aaaaaa", Key: &featKey, Status: featureStatus, EpicKey: epicKey}
	task := &store.Task{ID: "T-aaaaaa", FeatureID: feat.ID, Status: taskStatus}
	tree := &store.Tree{
		Epics:    map[string]*store.Epic{epic.ID: epic},
		Features: map[string]*store.Feature{feat.ID: feat},
		Tasks:    map[string]*store.Task{task.ID: task},
	}
	return tree, feat, task
}

// classifyGuardErr sorts guardParentState's answer into one of the four
// classes the mapping documents: nil, feature-cancelled, epic-retired, or
// ungated. Any other error type fails the test outright: the whole point of
// the matrix is that these four classes are exhaustive.
func classifyGuardErr(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return "nil"
	}
	var pc *ParentCancelledError
	if errors.As(err, &pc) {
		return "feature-cancelled"
	}
	var pe *ParentEpicRetiredError
	if errors.As(err, &pe) {
		return "epic-retired"
	}
	var pg *ParentPlanUngatedError
	if errors.As(err, &pg) {
		return "ungated"
	}
	t.Fatalf("guardParentState returned an unrecognized error type: %T (%v)", err, err)
	return ""
}

// classifyHeld sorts gen.RetiredAncestor's answer into "nil", store.NounFeature,
// or store.NounEpic (the only two nouns a Held can ever name for a task).
func classifyHeld(h *gen.Held) string {
	if h == nil {
		return "nil"
	}
	return h.Noun
}

// guardClassAsPredClass translates a guardParentState answer class into the
// gen.RetiredAncestor class the documented mapping says it must agree with
// (the context bullet quoting the post-#38 order, and guardParentState's own
// doc comment cross-referencing gen.RetiredAncestor as the read-side twin of
// the identical walk): guard nil and guard ungated both answer predicate nil
// (a pre-gate feature under a live epic holds nothing; RetiredAncestor's own
// doc comment calls this "the pre-gate exclusion"), guard feature-cancelled
// answers predicate feature, and guard epic-retired answers predicate epic.
func guardClassAsPredClass(guardClass string) string {
	switch guardClass {
	case "feature-cancelled":
		return store.NounFeature
	case "epic-retired":
		return store.NounEpic
	case "ungated", "nil":
		return "nil"
	default:
		return "unrecognized:" + guardClass
	}
}

// TestGuardParentStateAgreesWithRetiredAncestorAcrossTheMatrix is net 1 (issue
// #40): every feature-status x epic-status cell calls guardParentState and
// gen.RetiredAncestor directly on the SAME tree, so the two independently
// maintained walks are pinned to actually agree under the documented mapping
// rather than each only being pinned in isolation (gen/held_test.go pins
// RetiredAncestor alone; transition_test.go pins guardParentState alone
// through mutate.Transition's exported surface, and the two never share one
// assertion). The task's own status is held fixed at "in_progress"
// throughout: guardParentState never reads it except to name it in an error,
// and RetiredAncestor's terminal-self check reads only the TASK's own status
// (not the feature's, gen/held.go's retiredAncestorOfTask), so a live,
// non-terminal task status isolates the matrix to the feature x epic
// combinations the mapping is actually about.
func TestGuardParentStateAgreesWithRetiredAncestorAcrossTheMatrix(t *testing.T) {
	t.Parallel()
	featureStates := []string{"draft", "specced", "ready", "active", "done", "cancelled"}
	epicStates := []string{"ready", "cancelled", "archived"} // "ready" stands in for any live epic status

	for _, fs := range featureStates {
		for _, es := range epicStates {
			t.Run("feature="+fs+",epic="+es, func(t *testing.T) {
				tree, feat, task := guardMatrixFixture(es, fs, "in_progress")

				guardClass := classifyGuardErr(t, guardParentState(tree, feat, task))
				predClass := classifyHeld(gen.RetiredAncestor(tree, task))

				require.Equal(t, guardClassAsPredClass(guardClass), predClass,
					"feature=%s epic=%s: guard answered %q (mapping to predicate class %q) but RetiredAncestor answered %q",
					fs, es, guardClass, guardClassAsPredClass(guardClass), predClass)
			})
		}
	}
}

// TestGuardParentStateDoneFeatureFallsThroughLikeReadyAndActive is net 4
// (issue #40): guardParentState's own doc comment says ready, active, and
// done all fall through the guard the same way under a live epic ("a done
// parent is the reopen story's business, not this arm's"), but nothing
// called the guard directly over a done feature before this — every other
// test that reaches a done feature does so through guardFEATURE
// (TestCheckFeatureSucceedsOnADoneFeatureUnderAnArchivedEpic, check_test.go),
// a different function at a different grain, and a done feature's own
// live-task rollup arithmetic never produces a live task under it through any
// verb a fixture can drive. The task's ready status is hand-set directly onto
// the in-memory fixture, mirroring setTaskStatus's on-disk version in
// transition_test.go: the guard has to answer for whatever a tree holds, not
// only what today's verbs can produce.
func TestGuardParentStateDoneFeatureFallsThroughLikeReadyAndActive(t *testing.T) {
	t.Parallel()
	tree, feat, task := guardMatrixFixture("ready", "done", "ready")
	require.Nil(t, guardParentState(tree, feat, task),
		"a done feature under a live epic falls through the guard exactly like ready or active")
}
