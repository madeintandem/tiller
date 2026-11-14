package views_test

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// moveTree is two epics, E-aaaaaa (key E1) and E-bbbbbb (key E2), with one
// ready feature currently under the epic keyed under.
func moveTree(e1, e2, under string) *store.Tree {
	tr := reportTree("p", "main")
	k1, k2 := "E1", "E2"
	tr.Epics["E-aaaaaa"] = &store.Epic{ID: "E-aaaaaa", Key: &k1, Status: e1}
	tr.Epics["E-bbbbbb"] = &store.Epic{ID: "E-bbbbbb", Key: &k2, Status: e2}
	tr.Features["F-aaaaaa"] = &store.Feature{ID: "F-aaaaaa", EpicKey: under, Status: "ready"}
	return tr
}

// moveEv is the move.feature event an approved amendment writes: the feature,
// then both epics and the amendment as targets, with the stable epic ids in
// before and after and no status.
func moveEv(at time.Time, feature, from, to string) events.Event {
	burnUpSeq++
	return events.Event{
		ID: evID(burnUpSeq), Type: "move.feature", At: at,
		Targets: []string{feature, from, to, "A-aaaaaa"},
		Before:  map[string]any{"key": "old/key", "epic": from},
		After:   map[string]any{"key": "new/key", "epic": to, "amendment": "A-aaaaaa"},
	}
}

// A feature that sat under an approved epic for weeks stays in those weeks'
// scope after it moves to an epic approved later: the move changes scope from
// the move onward and never rewrites the weeks before it.
func TestReportBurnUpKeepsEarlierScopeAfterMoveToLaterApprovedEpic(t *testing.T) {
	tr := moveTree("ready", "ready", "E2")
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("add.epic", inWeek(0), "draft", "E-bbbbbb"),
		statusEv("add.feature", inWeek(0), "draft", "F-aaaaaa"),
		statusEv("approve.epic", inWeek(2), "ready", "E-bbbbbb"),
		moveEv(inWeek(3), "F-aaaaaa", "E-aaaaaa", "E-bbbbbb"),
	}
	require.Equal(t, []int{1, 1, 1, 1}, scopes(burnUp(t, tr, evs)))
}

// The inverse move must not backdate scope either: a feature that waited
// under a draft epic was a proposal in those weeks, whatever epic holds it now.
func TestReportBurnUpDoesNotBackdateScopeAfterMoveToEarlierApprovedEpic(t *testing.T) {
	tr := moveTree("ready", "ready", "E1")
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("add.epic", inWeek(0), "draft", "E-bbbbbb"),
		statusEv("add.feature", inWeek(0), "draft", "F-aaaaaa"),
		statusEv("approve.epic", inWeek(2), "ready", "E-bbbbbb"),
		moveEv(inWeek(3), "F-aaaaaa", "E-bbbbbb", "E-aaaaaa"),
	}
	require.Equal(t, []int{0, 0, 1, 1}, scopes(burnUp(t, tr, evs)))
}

// A move stamped exactly on a Monday belongs to the week it opens, as every
// other change does: the week before still reads the old parent.
func TestReportBurnUpMoveOnMondayBoundaryBelongsToNextWeek(t *testing.T) {
	tr := moveTree("ready", "draft", "E2")
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("add.epic", inWeek(0), "draft", "E-bbbbbb"),
		statusEv("add.feature", inWeek(0), "draft", "F-aaaaaa"),
		moveEv(burnUpWeek0.AddDate(0, 0, 14), "F-aaaaaa", "E-aaaaaa", "E-bbbbbb"),
	}
	require.Equal(t, []int{1, 1, 0, 0}, scopes(burnUp(t, tr, evs)))
}

// A destination epic an amendment staged and approved together with the move
// did not exist in the earlier weeks; the feature still counted under its old
// approved parent then.
func TestReportBurnUpKeepsEarlierScopeAfterMoveToNewlyStagedEpic(t *testing.T) {
	tr := moveTree("ready", "ready", "E2")
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("add.feature", inWeek(0), "draft", "F-aaaaaa"),
		statusEv("add.epic", inWeek(2), "draft", "E-bbbbbb", "A-aaaaaa"),
		statusEv("approve.epic", inWeek(3), "ready", "E-bbbbbb", "A-aaaaaa"),
		moveEv(inWeek(3), "F-aaaaaa", "E-aaaaaa", "E-bbbbbb"),
	}
	require.Equal(t, []int{1, 1, 1, 1}, scopes(burnUp(t, tr, evs)))
}

// A move that keeps the parent, and a direct key rename, change no scope.
func TestReportBurnUpSameParentRenameChangesNothing(t *testing.T) {
	tr := moveTree("ready", "draft", "E1")
	burnUpSeq++
	rename := events.Event{
		ID: evID(burnUpSeq), Type: "rename.key.feature", At: inWeek(1).Add(time.Hour),
		Targets: []string{"F-aaaaaa"},
		Before:  map[string]any{"key": "E1/old", "epic": "E1"},
		After:   map[string]any{"key": "E1/new", "epic": "E1"},
	}
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("add.epic", inWeek(0), "draft", "E-bbbbbb"),
		statusEv("add.feature", inWeek(1), "draft", "F-aaaaaa"),
		rename,
		moveEv(inWeek(2), "F-aaaaaa", "E-aaaaaa", "E-aaaaaa"),
	}
	require.Equal(t, []int{0, 1, 1, 1}, scopes(burnUp(t, tr, evs)))
}

// The last week reads the tree's parent even when the recorded moves end
// somewhere else, as a hand-moved folder leaves them, so the burn-up's final
// week still agrees with epic progress.
func TestReportBurnUpFinalWeekUsesTheCurrentParent(t *testing.T) {
	tr := moveTree("ready", "draft", "E1")
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("add.epic", inWeek(0), "draft", "E-bbbbbb"),
		statusEv("add.feature", inWeek(0), "draft", "F-aaaaaa"),
		moveEv(inWeek(1), "F-aaaaaa", "E-aaaaaa", "E-bbbbbb"),
	}
	rep := views.ReportOf(tr, views.ReportInput{Events: evs, Now: burnUpNow, Location: time.UTC})
	require.Equal(t, []int{1, 0, 0, 1}, scopes(rep.BurnUp))
	approved := 0
	for _, epic := range rep.Epics {
		if !epic.Planned {
			approved += epic.Done + epic.InProgress + epic.NotStarted
		}
	}
	require.Equal(t, approved, rep.KPIs.FeaturesTotal)
}
