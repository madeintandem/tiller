package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// TestActiveBasisFilesAreRepositoryPaths pins one form for a row's files
// across the drift report: an active_basis finding names its pinned files by
// their path from the repository root, the same form stale_spec already uses,
// never a bare file name.
func TestActiveBasisFilesAreRepositoryPaths(t *testing.T) {
	fKey := "docs/handbook"
	tKey := "docs/handbook/write-intro"
	tree := &store.Tree{
		Features: map[string]*store.Feature{"F-aaaaaa": {ID: "F-aaaaaa", Key: &fKey, EpicKey: "docs", Status: "ready"}},
		Tasks:    map[string]*store.Task{"T-bbbbbb": {ID: "T-bbbbbb", Key: &tKey, FeatureID: "F-aaaaaa", Status: "ready"}},
	}
	rep := drift.Report{Candidates: []drift.Candidate{
		{Marker: drift.MarkerActiveBasis, Noun: store.NounFeature, ID: "F-aaaaaa", Key: fKey, Files: []string{"feature.md", "feature.eval.md"}},
		{Marker: drift.MarkerActiveBasis, Noun: store.NounTask, ID: "T-bbbbbb", Key: tKey, Files: []string{"task.md"}},
		{Marker: drift.MarkerStaleSpec, Noun: store.NounFeature, ID: "F-aaaaaa", Key: fKey, Files: []string{".tiller/epics/docs/features/handbook/feature.md"}},
	}}

	v := views.DriftReportOf(tree, rep)
	basis := v.Candidates[drift.MarkerActiveBasis]
	require.Len(t, basis, 2)
	require.Equal(t, []string{
		".tiller/epics/docs/features/handbook/feature.md",
		".tiller/epics/docs/features/handbook/feature.eval.md",
	}, basis[0].Files)
	require.Equal(t, []string{".tiller/epics/docs/features/handbook/tasks/write-intro/task.md"}, basis[1].Files)
	require.Equal(t, []string{".tiller/epics/docs/features/handbook/feature.md"}, v.Candidates[drift.MarkerStaleSpec][0].Files,
		"a path that is already repository-relative is left as it is")
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
