package views_test

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// topNext is the first move a readout's **Next:** list shows.
func topNext(t *testing.T, r views.Readout) views.NextCandidate {
	t.Helper()
	short := r.Next.Shortlist()
	require.NotEmpty(t, short, "the block closes on at least one ranked move")
	return short[0]
}

// nextKinds and nextNodeIDs flatten every ranked move down to the one field
// a test cares about, in rank order.
func nextKinds(r views.Readout) []string {
	out := []string{}
	for _, c := range r.Next.Candidates {
		out = append(out, c.Kind)
	}
	return out
}

func nextNodeIDs(r views.Readout) []string {
	out := []string{}
	for _, c := range r.Next.Candidates {
		if c.Node != nil {
			out = append(out, c.Node.ID)
		}
	}
	return out
}

// A gate readout ends on the ranked list NextOf derives for the same reader,
// observations, and recent events: the readout ranks nothing of its own.
func TestReadoutNextIsTheShortlist(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	for _, id := range []string{"F-aaaaa0", "F-aaaaa1"} {
		f := feat(t, id, "epic-0/"+id, "epic-0")
		f.Status = "draft"
		tree.Features[id] = f
	}
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	// The reader just worked on the second feature, so it outranks the first
	// one's plan order only when Recent and Now reach the ranking.
	recent := []events.Event{{ID: "EV-aaaaaa", Type: "spec.feature", Contributor: "jc", At: now.Add(-time.Hour), Targets: []string{"F-aaaaa1"}}}

	in := views.ReadoutInput{
		Contributor: "jc", Recent: recent, Now: now,
		Act: &views.GateAct{Nodes: []views.GateActNode{{Noun: store.NounEpic, ID: "E-aaaaa0", Changed: true}}},
	}
	r := views.ReadoutOfInput(tree, views.MomentApprove, in)

	require.Equal(t, views.NextOf(tree, views.NextInput{Reader: "jc", Recent: recent, Now: now}), r.Next)
	top := topNext(t, r)
	require.Equal(t, "specify", top.Kind)
	require.Equal(t, "F-aaaaa1", top.Node.ID, "the reader's current feature ranks first")

	// The control: with no recent events, plan order leads.
	in.Recent = nil
	control := views.ReadoutOfInput(tree, views.MomentApprove, in)
	require.Equal(t, "F-aaaaa0", topNext(t, control).Node.ID)
}

// The correction block closes on the same ranked list as the full block.
func TestCorrectionNextIsTheReadoutNext(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "draft"
	tree.Features[f.ID] = f
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	full := views.ReadoutOfInput(tree, views.MomentSpecify, views.ReadoutInput{Contributor: "jc", Now: now})
	correction := views.CorrectionOfInput(tree, views.MomentSpecify, views.CorrectionInput{Contributor: "jc", Now: now})
	require.Equal(t, full.Next, correction.Next)
}

// Orient no longer carries a Route check (plan ruling 7): the ranked
// **Next:** list carries the same answer.
func TestOrientReadoutHasNoRouteCheck(t *testing.T) {
	tree := treeWithDraftEpics(t, 1)
	tree.Epics["E-aaaaa0"].Status = "ready"
	f := feat(t, "F-aaaaa0", "epic-0/loading", "epic-0")
	f.Status = "draft"
	tree.Features["F-aaaaa0"] = f

	r := views.ReadoutOfInput(tree, views.MomentOrient, views.ReadoutInput{})

	_, hasRoute := checkLabels(r)["Route"]
	require.False(t, hasRoute)
	top := topNext(t, r)
	require.Equal(t, "specify", top.Kind, "a draft feature's move is to specify it")
	require.Equal(t, "F-aaaaa0", top.Node.ID, "the move names the node, not just the seam")
}
