package drift

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStaleSpecDetailSaysTheWorkWasFinished pins the plain wording of a
// stale_spec finding's sentence: a file changed or was deleted after the work
// was finished, never "since it was verified at done".
func TestStaleSpecDetailSaysTheWorkWasFinished(t *testing.T) {
	changed := staleSpecItem{path: ".tiller/epics/e/features/f/feature.md", label: "feature.md"}
	gone := staleSpecItem{path: "app/zone.rb", label: "app/zone.rb"}

	detail, context := staleSpecDetail(staleSpecFinding{changed: []staleSpecItem{changed}})
	require.Equal(t, "feature.md changed after the work was finished", detail)
	require.Empty(t, context)

	detail, context = staleSpecDetail(staleSpecFinding{vanished: []staleSpecItem{gone}, changed: []staleSpecItem{changed}})
	require.Equal(t, "app/zone.rb deleted after the work was finished", detail)
	require.Equal(t, []string{"feature.md also changed after the work was finished"}, context)
}
