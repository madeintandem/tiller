package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// TestActiveBasisViewNamesFilesByRepositoryPath pins one form for a file name
// on the node page's basis row: the drifted and missing files are named by
// their path from the repository root, as the drift report's files and
// sentence name them (drift's active_basis_paths_test.go), never by the bare
// name the pin records.
func TestActiveBasisViewNamesFilesByRepositoryPath(t *testing.T) {
	fKey := "docs/handbook"
	tKey := "docs/handbook/write-intro"
	tree := &store.Tree{
		Features: map[string]*store.Feature{"F-aaaaaa": {ID: "F-aaaaaa", Key: &fKey, EpicKey: "docs", Status: "ready"}},
		Tasks:    map[string]*store.Task{"T-bbbbbb": {ID: "T-bbbbbb", Key: &tKey, FeatureID: "F-aaaaaa", Status: "ready"}},
	}

	v := views.ActiveBasisOf(tree, &mutate.ActiveBasisError{
		NodeID: "F-aaaaaa", Noun: store.NounFeature,
		Drifted: []string{"feature.md", "feature.eval.md"}, Missing: []string{"feature.eval.md"},
	})
	require.Equal(t, []string{
		".tiller/epics/docs/features/handbook/feature.md",
		".tiller/epics/docs/features/handbook/feature.eval.md",
	}, v.Drifted)
	require.Equal(t, []string{".tiller/epics/docs/features/handbook/feature.eval.md"}, v.Missing)

	task := views.ActiveBasisOf(tree, &mutate.ActiveBasisError{NodeID: "T-bbbbbb", Noun: store.NounTask, Drifted: []string{"task.md"}})
	require.Equal(t, []string{".tiller/epics/docs/features/handbook/tasks/write-intro/task.md"}, task.Drifted)
}

// TestDesignRowsLeadWithTheDocumentTitle pins the design row's title, carried
// from the sweep, beside its path key.
func TestDesignRowsLeadWithTheDocumentTitle(t *testing.T) {
	rep := drift.Report{
		Candidates: []drift.Candidate{{Marker: drift.MarkerStaleDesign, Noun: store.NounDesign, Key: "routing.md", Title: "Routing design"}},
		Uncovered:  []drift.Uncovered{{Marker: drift.MarkerShiftedDesign, Noun: store.NounDesign, Key: "routing.md", Title: "Routing design", Reason: "unconfirmed"}},
	}
	v := views.DriftReportOf(&store.Tree{}, rep)
	require.Equal(t, "Routing design", v.Candidates[drift.MarkerStaleDesign][0].Title)
	require.Equal(t, "routing.md", v.Candidates[drift.MarkerStaleDesign][0].Key)
	require.Equal(t, "Routing design", v.Uncovered[0].Title)
}
