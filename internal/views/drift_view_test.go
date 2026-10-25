package views_test

// Tests for the drift-panel payload (JC's chip-to-panel ask, 2026-08-06):
// the node page carries candidate grain beside the one-per-marker badges,
// and CandidateView carries the clearing move where one exists, composed
// once for every surface.

import (
	"os/exec"
	"testing"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func driftTree(t *testing.T) *store.Tree {
	t.Helper()
	key := "form-engine/nrca-form-loading"
	return &store.Tree{
		Features: map[string]*store.Feature{
			"F-aaaaaa": {ID: "F-aaaaaa", Key: &key, Status: "active"},
		},
	}
}

// TestCandidateViewComposesTheClearingMove pins the remedy per marker: a
// runnable command where one exists (ui-surface.md's own promise, "reported
// on that node's page with its clearing move beside it"), empty where none
// does, because inventing a command is the one thing the console must never
// do.
func TestCandidateViewComposesTheClearingMove(t *testing.T) {
	tree := driftTree(t)
	cases := []struct {
		name   string
		cand   drift.Candidate
		remedy string
	}{
		{
			name:   "active_basis names refresh-basis",
			cand:   drift.Candidate{Marker: drift.MarkerActiveBasis, Noun: store.NounFeature, ID: "F-aaaaaa", Key: "form-engine/nrca-form-loading"},
			remedy: "tiller approve feature form-engine/nrca-form-loading --refresh-basis",
		},
		{
			name:   "answered_question names reviewed-assumption with the question id",
			cand:   drift.Candidate{Marker: drift.MarkerAnsweredQuestion, Noun: store.NounFeature, ID: "F-aaaaaa", Key: "form-engine/nrca-form-loading", Question: "Q-6kjmgp"},
			remedy: "tiller approve feature form-engine/nrca-form-loading --reviewed-assumption Q-6kjmgp",
		},
		{
			name:   "missing_surface names the link template",
			cand:   drift.Candidate{Marker: drift.MarkerMissingSurface, Noun: store.NounFeature, ID: "F-aaaaaa", Key: "form-engine/nrca-form-loading"},
			remedy: "tiller link feature form-engine/nrca-form-loading --surface <path>",
		},
		{
			name:   "stale_spec carries no command, so no remedy is invented",
			cand:   drift.Candidate{Marker: drift.MarkerStaleSpec, Noun: store.NounFeature, ID: "F-aaaaaa", Key: "form-engine/nrca-form-loading"},
			remedy: "",
		},
		{
			name:   "orphan_eval carries no command either",
			cand:   drift.Candidate{Marker: drift.MarkerOrphanEval, Noun: store.NounFeature, ID: "F-aaaaaa", Key: "form-engine/nrca-form-loading"},
			remedy: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := drift.Report{Candidates: []drift.Candidate{tc.cand}}
			v := views.DriftReportOf(tree, rep)
			rows := v.Candidates[tc.cand.Marker]
			require.Len(t, rows, 1)
			require.Equal(t, tc.remedy, rows[0].Remedy)
		})
	}
}

// TestNodeDriftDetailOfCarriesBothGrainsAndNilsWhenEmpty pins the panel's
// payload: candidates and uncovered rows ride together (the serve handler
// used to discard uncovered for want of a detail area), and a node with
// neither carries no detail block at all rather than an empty one.
func TestNodeDriftDetailOfCarriesBothGrainsAndNilsWhenEmpty(t *testing.T) {
	tree := driftTree(t)
	cands := []drift.Candidate{{Marker: drift.MarkerActiveBasis, Noun: store.NounFeature, ID: "F-aaaaaa", Key: "form-engine/nrca-form-loading", Detail: "feature.md no longer matches"}}
	unc := []drift.Uncovered{{Marker: drift.MarkerOrphanEval, Noun: store.NounFeature, ID: "F-aaaaaa", Key: "form-engine/nrca-form-loading", Reason: "ref_unreadable", Detail: "not a single path"}}

	d := views.NodeDriftDetailOf(tree, cands, unc)
	require.NotNil(t, d)
	require.Len(t, d.Candidates, 1)
	require.Equal(t, "tiller approve feature form-engine/nrca-form-loading --refresh-basis", d.Candidates[0].Remedy)
	require.Len(t, d.Uncovered, 1)
	require.Equal(t, "ref_unreadable", d.Uncovered[0].Reason)

	require.Nil(t, views.NodeDriftDetailOf(tree, nil, nil),
		"no findings, no block: the wire stays quiet the same way the chip does")
}

func TestDesignRemedyPreservesLiteralShellArgument(t *testing.T) {
	doc := "-it's $(printf injected); model.md"
	rep := drift.Report{Candidates: []drift.Candidate{{Marker: drift.MarkerShiftedDesign, Noun: "design", Key: doc}}}
	view := views.DriftReportOf(driftTree(t), rep)
	remedy := view.Candidates[drift.MarkerShiftedDesign][0].Remedy
	// Parse the runnable command as a shell would, without invoking Tiller.
	out, err := exec.Command("sh", "-c", "set -- "+remedy+"; printf '%s\\n' \"$@\"").CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "tiller\napprove\ndesign\n--\n"+doc+"\n", string(out))
}
