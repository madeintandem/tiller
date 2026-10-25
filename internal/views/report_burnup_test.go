package views_test

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// burnUpWeek0 is the Monday of the first of four weeks; burnUpNow falls in
// the fourth.
var (
	burnUpWeek0 = time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	burnUpNow   = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
)

// inWeek is an hour into week n.
func inWeek(n int) time.Time { return burnUpWeek0.AddDate(0, 0, 7*n).Add(time.Hour) }

var burnUpSeq int

// evID is a unique event id per counter value (base 26, six letters).
func evID(n int) string {
	id := []byte("aaaaaa")
	for i := len(id) - 1; i >= 0 && n > 0; i-- {
		id[i] = byte('a' + n%26)
		n /= 26
	}
	return "EV-" + string(id)
}

func statusEv(typ string, at time.Time, status string, targets ...string) events.Event {
	burnUpSeq++
	return events.Event{
		ID: evID(burnUpSeq), Type: typ, At: at,
		Targets: targets, After: map[string]any{"status": status},
	}
}

func burnUpTree(epicStatus string, features map[string]string) *store.Tree {
	tr := reportTree("p", "main")
	key := "E1"
	tr.Epics["E-aaaaaa"] = &store.Epic{ID: "E-aaaaaa", Key: &key, Status: epicStatus}
	for id, status := range features {
		tr.Features[id] = &store.Feature{ID: id, EpicKey: key, Status: status}
	}
	return tr
}

func burnUp(t *testing.T, tr *store.Tree, evs []events.Event) []views.ReportBurnUpWeek {
	t.Helper()
	rep := views.ReportOf(tr, views.ReportInput{Events: evs, Now: burnUpNow, Location: time.UTC})
	require.Len(t, rep.Weeks, 4)
	require.Equal(t, "2026-09-28", rep.Weeks[0])
	return rep.BurnUp
}

func scopes(b []views.ReportBurnUpWeek) []int {
	out := make([]int, len(b))
	for i, w := range b {
		out[i] = w.Scope
	}
	return out
}

func dones(b []views.ReportBurnUpWeek) []int {
	out := make([]int, len(b))
	for i, w := range b {
		out[i] = w.Done
	}
	return out
}

func TestReportBurnUpCountsOnlyApprovedEpics(t *testing.T) {
	tr := burnUpTree("ready", map[string]string{"F-aaaaaa": "ready", "F-bbbbbb": "ready"})
	evs := []events.Event{
		statusEv("add.epic", inWeek(0), "draft", "E-aaaaaa"),
		statusEv("add.feature", inWeek(0), "draft", "F-aaaaaa"),
		statusEv("add.feature", inWeek(0), "draft", "F-bbbbbb"),
		statusEv("approve.epic", inWeek(2), "ready", "E-aaaaaa"),
	}
	require.Equal(t, []int{0, 0, 2, 2}, scopes(burnUp(t, tr, evs)))
}

func TestReportBurnUpAddsAndCancels(t *testing.T) {
	tr := burnUpTree("ready", map[string]string{"F-aaaaaa": "cancelled", "F-bbbbbb": "ready"})
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("add.feature", inWeek(0), "draft", "F-aaaaaa"),
		statusEv("add.feature", inWeek(1), "draft", "F-bbbbbb"),
		statusEv("cancel.feature", inWeek(2), "cancelled", "F-aaaaaa", "A-aaaaaa"),
	}
	require.Equal(t, []int{1, 2, 1, 1}, scopes(burnUp(t, tr, evs)))
}

func TestReportBurnUpDone(t *testing.T) {
	tr := burnUpTree("ready", map[string]string{"F-aaaaaa": "done"})
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("add.feature", inWeek(0), "draft", "F-aaaaaa"),
		statusEv("sync.feature", inWeek(3), "done", "F-aaaaaa"),
	}
	require.Equal(t, []int{0, 0, 0, 1}, dones(burnUp(t, tr, evs)))
}

func TestReportBurnUpFinalWeekMatchesCurrentTree(t *testing.T) {
	// F-aaaaaa went done by event but was reopened with no event; F-bbbbbb is
	// done in the tree. Only the tree's done feature counts in the last week.
	tr := burnUpTree("ready", map[string]string{"F-aaaaaa": "active", "F-bbbbbb": "done"})
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("sync.feature", inWeek(1), "done", "F-aaaaaa"),
		statusEv("sync.feature", inWeek(2), "done", "F-bbbbbb"),
	}
	b := burnUp(t, tr, evs)
	require.Equal(t, 1, b[3].Done)
	require.Equal(t, 2, b[3].Scope)
	require.Equal(t, []int{0, 1, 2, 1}, dones(b))
}

func TestReportBurnUpCancelledEpicLeavesScope(t *testing.T) {
	tr := burnUpTree("cancelled", map[string]string{"F-aaaaaa": "ready", "F-bbbbbb": "ready"})
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("add.feature", inWeek(0), "draft", "F-aaaaaa"),
		statusEv("add.feature", inWeek(0), "draft", "F-bbbbbb"),
		statusEv("cancel.epic", inWeek(2), "cancelled", "E-aaaaaa"),
	}
	require.Equal(t, []int{2, 2, 0, 0}, scopes(burnUp(t, tr, evs)))
}

func TestReportBurnUpMondayBoundaryBelongsToNextWeek(t *testing.T) {
	tr := burnUpTree("ready", map[string]string{"F-aaaaaa": "ready"})
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		// Exactly Monday 00:00 of week 2: counts in week 2, not week 1.
		statusEv("add.feature", burnUpWeek0.AddDate(0, 0, 14), "draft", "F-aaaaaa"),
	}
	b := burnUp(t, tr, evs)
	require.Equal(t, []int{0, 0, 1, 1}, scopes(b))
}

func TestReportBurnUpKPIs(t *testing.T) {
	tr := burnUpTree("ready", map[string]string{"F-aaaaaa": "done", "F-bbbbbb": "ready"})
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("sync.feature", inWeek(1), "done", "F-aaaaaa"),
	}
	rep := views.ReportOf(tr, views.ReportInput{Events: evs, Now: burnUpNow, Location: time.UTC})
	require.Equal(t, 1, rep.KPIs.FeaturesDone)
	require.Equal(t, 2, rep.KPIs.FeaturesTotal)
}

func TestReportEventAfterNowCountsAtNow(t *testing.T) {
	tr := burnUpTree("ready", map[string]string{"F-aaaaaa": "done"})
	evs := []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("sync.feature", burnUpNow.Add(time.Minute), "done", "F-aaaaaa"),
	}
	rep := views.ReportOf(tr, views.ReportInput{Events: evs, Now: burnUpNow, Location: time.UTC})
	require.Equal(t, []int{0, 0, 0, 1}, dones(rep.BurnUp))
	require.Equal(t, 1, rep.KPIs.FeaturesDone)
	require.Equal(t, 1, rep.KPIs.FeaturesTotal)
	require.Equal(t, 1, rep.Epics[0].Done)
}
