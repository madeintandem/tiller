package gen

// This file pins the held-by-retired-ancestor predicate ("annotate,
// don't subtract"): RetiredAncestor re-derives, from the loaded tree alone, the
// identical parent-then-epic walk mutate.guardParentState runs ahead of the
// doing task verbs, so the read surfaces can say WHY those verbs would refuse
// without importing mutate at all.

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// heldTaskFixture composes one epic, one feature under it (EpicKey wired the
// way store.Load sets it from the directory layout), and one task under that
// feature, each grain's status set independently so the table below can drive
// every combination guardParentState's own switch cares about.
func heldTaskFixture(epicStatus, featureStatus, taskStatus string) (*store.Tree, *store.Task) {
	epicKey := "form-engine"
	featKey := "form-engine/certification-form-loading"
	epic := &store.Epic{ID: "E-aaaaaa", Key: &epicKey, Status: epicStatus}
	feat := &store.Feature{ID: "F-aaaaaa", Key: &featKey, Status: featureStatus, EpicKey: epicKey}
	task := &store.Task{ID: "T-aaaaaa", FeatureID: feat.ID, Status: taskStatus}
	tree := &store.Tree{
		Epics:    map[string]*store.Epic{epic.ID: epic},
		Features: map[string]*store.Feature{feat.ID: feat},
		Tasks:    map[string]*store.Task{task.ID: task},
	}
	return tree, task
}

// TestRetiredAncestorTaskTruthTable pins the table over a
// task: a cancelled feature is checked before the epic, exactly the order
// guardParentState's own switch runs, so a feature cancellation wins over an
// epic retirement one below it (its remedy, cancel the task, is reachable,
// the deepest actionable node). Past that, the epic question runs BEFORE the
// pre-gate check (draft/specced): a pre-gate parent is otherwise a live route
// to the gate, but that route is a dead end exactly when the epic itself is
// retired, so the epic answer outranks it there (JC's call
// 2026-08-03). A pre-gate feature under a LIVE epic still holds nothing, and
// a task that is itself terminal is never held.
func TestRetiredAncestorTaskTruthTable(t *testing.T) {
	cases := []struct {
		name          string
		epicStatus    string
		featureStatus string
		taskStatus    string
		wantNoun      string // "" means RetiredAncestor must answer nil
		wantState     string
	}{
		{
			name:       "live task under cancelled feature is held, names the feature",
			epicStatus: "ready", featureStatus: "cancelled", taskStatus: "in_progress",
			wantNoun: store.NounFeature, wantState: "cancelled",
		},
		{
			name:       "live task under live feature under cancelled epic is held, names the epic",
			epicStatus: "cancelled", featureStatus: "active", taskStatus: "in_progress",
			wantNoun: store.NounEpic, wantState: "cancelled",
		},
		{
			name:       "live task under cancelled feature under cancelled epic is held, names the FEATURE",
			epicStatus: "cancelled", featureStatus: "cancelled", taskStatus: "in_progress",
			wantNoun: store.NounFeature, wantState: "cancelled",
		},
		{
			name:       "proposed task under draft feature is NOT held: pre-gate, not retired",
			epicStatus: "draft", featureStatus: "draft", taskStatus: "proposed",
			wantNoun: "",
		},
		{
			name:       "proposed task under draft feature under cancelled epic is held, names the EPIC (issue #38: epic outranks the pre-gate arm)",
			epicStatus: "cancelled", featureStatus: "draft", taskStatus: "proposed",
			wantNoun: store.NounEpic, wantState: "cancelled",
		},
		{
			name:       "proposed task under specced feature under archived epic is held, names the EPIC",
			epicStatus: "archived", featureStatus: "specced", taskStatus: "proposed",
			wantNoun: store.NounEpic, wantState: "archived",
		},
		{
			name:       "cancelled task under cancelled feature is NOT held: it finished, it was never left holding anything",
			epicStatus: "ready", featureStatus: "cancelled", taskStatus: "cancelled",
			wantNoun: "",
		},
		{
			name:       "everything live is nil",
			epicStatus: "ready", featureStatus: "active", taskStatus: "in_progress",
			wantNoun: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, task := heldTaskFixture(tc.epicStatus, tc.featureStatus, tc.taskStatus)
			held := RetiredAncestor(tree, task)
			if tc.wantNoun == "" {
				require.Nil(t, held)
				return
			}
			require.NotNil(t, held)
			require.Equal(t, tc.wantNoun, held.Noun)
			require.Equal(t, tc.wantState, held.State)
			require.NotEmpty(t, held.ID)
			require.NotEmpty(t, held.Ref)
		})
	}
}

// TestRetiredAncestorFeatureUnderArchivedEpic pins the seventh table row at
// feature grain: a live feature has no feature ancestor to check, only its
// epic, and archived retires it exactly like cancelled does.
func TestRetiredAncestorFeatureUnderArchivedEpic(t *testing.T) {
	epicKey := "form-engine"
	featKey := "form-engine/certification-form-loading"
	epic := &store.Epic{ID: "E-aaaaaa", Key: &epicKey, Status: "archived"}
	feat := &store.Feature{ID: "F-aaaaaa", Key: &featKey, Status: "active", EpicKey: epicKey}
	tree := &store.Tree{
		Epics:    map[string]*store.Epic{epic.ID: epic},
		Features: map[string]*store.Feature{feat.ID: feat},
	}

	held := RetiredAncestor(tree, feat)
	require.NotNil(t, held)
	require.Equal(t, store.NounEpic, held.Noun)
	require.Equal(t, "archived", held.State)
	require.Equal(t, "E-aaaaaa", held.ID)
	require.Contains(t, held.Ref, "form-engine")
}

// TestRetiredAncestorFeatureItselfCancelledIsNil confirms the terminal rule at
// feature grain too: a cancelled feature under a cancelled epic already
// finished, so it is never held, however retired its epic reads.
func TestRetiredAncestorFeatureItselfCancelledIsNil(t *testing.T) {
	epicKey := "form-engine"
	featKey := "form-engine/certification-form-loading"
	epic := &store.Epic{ID: "E-aaaaaa", Key: &epicKey, Status: "cancelled"}
	feat := &store.Feature{ID: "F-aaaaaa", Key: &featKey, Status: "cancelled", EpicKey: epicKey}
	tree := &store.Tree{
		Epics:    map[string]*store.Epic{epic.ID: epic},
		Features: map[string]*store.Feature{feat.ID: feat},
	}

	require.Nil(t, RetiredAncestor(tree, feat))
}

// TestRetiredAncestorEpicIsAlwaysNil confirms an epic has no ancestor to
// retire under: RetiredAncestor answers nil for one regardless of its own
// status, since an epic sits at the top of the tree.
func TestRetiredAncestorEpicIsAlwaysNil(t *testing.T) {
	epic := &store.Epic{ID: "E-aaaaaa", Status: "ready"}
	tree := &store.Tree{Epics: map[string]*store.Epic{epic.ID: epic}}
	require.Nil(t, RetiredAncestor(tree, epic))
}
