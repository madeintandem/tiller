package views_test

// A draft an open amendment holds is a proposal, visible but inert, and it
// changes no rollup (artifact-schema.md, Held drafts). These tests pin what
// that means on the Report: a held feature is not approved scope and takes no
// place on an approved epic's progress bar until its amendment is approved,
// the release is dated by the approval the stream recorded, and a withdrawal
// is never read as one.

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// holdTree is one epic (E-aaaaaa, key E1) in epicStatus holding the done
// feature F-aaaaaa plus the listed draft features. open maps a draft the tree
// still holds to the open amendment holding it.
func holdTree(epicStatus string, drafts []string, open map[string]string) *store.Tree {
	tr := burnUpTree(epicStatus, map[string]string{"F-aaaaaa": "done"})
	tr.Epics["E-aaaaaa"].Title = "Epic one"
	for _, id := range drafts {
		tr.Features[id] = &store.Feature{ID: id, EpicKey: "E1", Status: "draft"}
	}
	tr.Amendments = map[string]*store.Amendment{}
	tr.AmendmentHolds = map[string]string{}
	for featureID, amendmentID := range open {
		tr.Amendments[amendmentID] = &store.Amendment{ID: amendmentID, Status: store.AmendmentStatusProposed}
		tr.AmendmentHolds[featureID] = amendmentID
	}
	return tr
}

// shippedEpic is the history every hold scenario starts from: the epic
// approved and its one feature added in week 0, both done in week 1.
func shippedEpic() []events.Event {
	return []events.Event{
		statusEv("approve.epic", inWeek(0), "ready", "E-aaaaaa"),
		statusEv("add.feature", inWeek(0), "draft", "F-aaaaaa"),
		statusEv("sync.feature", inWeek(1), "done", "F-aaaaaa"),
		statusEv("sync.epic", inWeek(1), "done", "E-aaaaaa"),
	}
}

// stagedAddEv is the add.feature event staging an add-feature writes: the
// held draft and its amendment as targets, the amendment named in after.
func stagedAddEv(at time.Time, feature, amendment string) events.Event {
	ev := statusEv("add.feature", at, "draft", feature, amendment)
	ev.After["key"] = "E1/staged"
	ev.After["amendment"] = amendment
	ev.After["operation_index"] = 0
	ev.After["epic"] = "E-aaaaaa"
	return ev
}

// amendmentEv is an amendment's own approval or withdrawal event.
func amendmentEv(typ string, at time.Time, amendment, status string) events.Event {
	burnUpSeq++
	after := map[string]any{"status": status}
	if typ == "amendment_approved" {
		after["amendment"] = amendment
	}
	return events.Event{ID: evID(burnUpSeq), Type: typ, At: at, Targets: []string{amendment}, After: after}
}

func holdReport(tr *store.Tree, evs []events.Event) views.Report {
	return views.ReportOf(tr, views.ReportInput{Events: evs, Now: burnUpNow, Location: time.UTC})
}

func TestReportHeldDraftUnderDoneEpicIsNotApprovedScope(t *testing.T) {
	tr := holdTree("done", []string{"F-bbbbbb"}, map[string]string{"F-bbbbbb": "A-aaaaaa"})
	evs := append(shippedEpic(), stagedAddEv(inWeek(2), "F-bbbbbb", "A-aaaaaa"))
	rep := holdReport(tr, evs)

	require.Equal(t, []int{1, 1, 1, 1}, scopes(rep.BurnUp))
	require.Equal(t, []int{0, 1, 1, 1}, dones(rep.BurnUp))
	require.Equal(t, 1, rep.KPIs.FeaturesTotal)
	require.Equal(t, 1, rep.KPIs.FeaturesDone)
	require.Equal(t, []views.ReportEpic{{Title: "Epic one", Done: 1}}, rep.Epics,
		"a held draft must not make a finished epic read as unfinished")
}

func TestReportHeldDraftEntersScopeWhenItsAmendmentIsApproved(t *testing.T) {
	tr := holdTree("done", []string{"F-bbbbbb"}, nil)
	evs := append(shippedEpic(),
		stagedAddEv(inWeek(2), "F-bbbbbb", "A-aaaaaa"),
		amendmentEv("amendment_approved", inWeek(3), "A-aaaaaa", "approved"),
	)
	rep := holdReport(tr, evs)

	require.Equal(t, []int{1, 1, 1, 2}, scopes(rep.BurnUp), "scope starts at the approval, not at staging")
	require.Equal(t, []views.ReportEpic{{Title: "Epic one", Done: 1, NotStarted: 1}}, rep.Epics)
	require.Equal(t, 2, rep.KPIs.FeaturesTotal)
}

// A withdrawal removes the drafts it held, so they are gone from the tree and
// from every week. Should a draft outlive a withdrawn amendment anyway, the
// withdrawal is still not an approval: the draft enters scope only where the
// tree puts it now, never at the withdrawal.
func TestReportWithdrawalIsNeverAnApproval(t *testing.T) {
	withdrawn := append(shippedEpic(),
		stagedAddEv(inWeek(1), "F-bbbbbb", "A-aaaaaa"),
		amendmentEv("amendment_withdrawn", inWeek(2), "A-aaaaaa", "withdrawn"),
	)

	removed := holdReport(holdTree("done", nil, nil), withdrawn)
	require.Equal(t, []int{1, 1, 1, 1}, scopes(removed.BurnUp))
	require.Equal(t, []views.ReportEpic{{Title: "Epic one", Done: 1}}, removed.Epics)

	survived := holdReport(holdTree("done", []string{"F-bbbbbb"}, nil), withdrawn)
	require.Equal(t, []int{1, 1, 1, 2}, scopes(survived.BurnUp))
}

// One amendment's approval releases only what that amendment staged, and a
// draft removed from it by hand is gone with no trace in scope.
func TestReportApprovalReleasesOnlyItsOwnHeldDrafts(t *testing.T) {
	tr := holdTree("ready", []string{"F-bbbbbb", "F-dddddd"}, map[string]string{"F-dddddd": "A-bbbbbb"})
	evs := append(shippedEpic(),
		stagedAddEv(inWeek(1), "F-bbbbbb", "A-aaaaaa"),
		stagedAddEv(inWeek(1), "F-cccccc", "A-aaaaaa"), // removed by hand before approval
		stagedAddEv(inWeek(1), "F-dddddd", "A-bbbbbb"),
		amendmentEv("amendment_approved", inWeek(2), "A-aaaaaa", "approved"),
	)
	rep := holdReport(tr, evs)

	require.Equal(t, []int{1, 1, 2, 2}, scopes(rep.BurnUp))
	require.Equal(t, []views.ReportEpic{{Title: "Epic one", Done: 1, NotStarted: 1}}, rep.Epics)
}

// A draft added by hand under an approved epic is no proposal: it is scope
// from the add, as before.
func TestReportUnheldDraftUnderApprovedEpicIsScope(t *testing.T) {
	tr := holdTree("ready", []string{"F-bbbbbb"}, nil)
	evs := append(shippedEpic(), statusEv("add.feature", inWeek(2), "draft", "F-bbbbbb"))
	rep := holdReport(tr, evs)

	require.Equal(t, []int{1, 1, 2, 2}, scopes(rep.BurnUp))
	require.Equal(t, []views.ReportEpic{{Title: "Epic one", Done: 1, NotStarted: 1}}, rep.Epics)
}

// A node an approval materialized, as amendments did before staging created
// held drafts, carries its amendment on an add written in the approval's own
// batch. It was never held, whichever of the two events sorts first.
func TestReportAddWrittenByAnApprovalIsScopeAtOnce(t *testing.T) {
	at := inWeek(2)
	for name, evs := range map[string][]events.Event{
		"approval sorts first": {
			amendmentEv("amendment_approved", at, "A-aaaaaa", "approved"),
			stagedAddEv(at, "F-bbbbbb", "A-aaaaaa"),
		},
		"add sorts first": {
			stagedAddEv(at, "F-bbbbbb", "A-aaaaaa"),
			amendmentEv("amendment_approved", at.Add(time.Millisecond), "A-aaaaaa", "approved"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			rep := holdReport(holdTree("ready", []string{"F-bbbbbb"}, nil), append(shippedEpic(), evs...))
			require.Equal(t, []int{1, 1, 2, 2}, scopes(rep.BurnUp))
		})
	}
}

// A hold the stream never recorded is still a hold: the draft stays out of
// every week while the tree says an open amendment holds it.
func TestReportHoldWithNoStagingEventCoversEveryWeek(t *testing.T) {
	tr := holdTree("ready", []string{"F-bbbbbb"}, map[string]string{"F-bbbbbb": "A-aaaaaa"})
	rep := holdReport(tr, shippedEpic())

	require.Equal(t, []int{1, 1, 1, 1}, scopes(rep.BurnUp))
	require.Equal(t, []views.ReportEpic{{Title: "Epic one", Done: 1}}, rep.Epics)
}

// Held drafts stay in the draft totals: a staged epic is a ghost row, and the
// feature staged under it counts among its planned features.
func TestReportHeldDraftsStayInDraftTotals(t *testing.T) {
	tr := holdTree("done", []string{"F-bbbbbb"}, map[string]string{"F-bbbbbb": "A-aaaaaa", "E-bbbbbb": "A-aaaaaa"})
	key := "E2"
	tr.Epics["E-bbbbbb"] = &store.Epic{ID: "E-bbbbbb", Key: &key, Title: "Epic two", Status: "draft"}
	tr.Features["F-bbbbbb"].EpicKey = key
	tr.ProjectOrder = store.ProjectOrder{Epics: []string{"E-aaaaaa", "E-bbbbbb"}}
	rep := holdReport(tr, shippedEpic())

	require.Equal(t, []views.ReportEpic{
		{Title: "Epic one", Done: 1},
		{Title: "Epic two", Planned: true, PlannedFeatures: 1},
	}, rep.Epics)
	require.Equal(t, []int{1, 1, 1, 1}, scopes(rep.BurnUp))
}
