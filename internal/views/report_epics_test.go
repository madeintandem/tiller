package views

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// epicTree builds a tree from epics (id, key, status) and features
// (epic key, status).
func epicTree(epics [][3]string, feats [][2]string) *store.Tree {
	t := &store.Tree{
		Manifest:  &store.Manifest{},
		Epics:     map[string]*store.Epic{},
		Features:  map[string]*store.Feature{},
		Tasks:     map[string]*store.Task{},
		Bugs:      map[string]*store.Bug{},
		Chores:    map[string]*store.Chore{},
		Tweaks:    map[string]*store.Tweak{},
		Questions: map[string]*store.Question{},
	}
	for _, e := range epics {
		key := e[1]
		t.Epics[e[0]] = &store.Epic{ID: e[0], Key: &key, Title: "Title " + e[1], Status: e[2]}
	}
	for i, f := range feats {
		id := "F-" + string(rune('a'+i))
		key := f[0] + "/" + id
		t.Features[id] = &store.Feature{ID: id, Key: &key, Title: id, Status: f[1], EpicKey: f[0]}
	}
	return t
}

func TestReportEpicProgressSegments(t *testing.T) {
	tr := epicTree([][3]string{{"E-a", "a", "ready"}},
		[][2]string{{"a", "done"}, {"a", "done"}, {"a", "active"}, {"a", "specced"}, {"a", "ready"}, {"a", "cancelled"}})
	got := epicProgress(tr)
	require.Equal(t, []ReportEpic{{Title: "Title a", Done: 2, InProgress: 1, NotStarted: 2}}, got)
}

// The start that activates a feature is recorded on its task's branch, so the
// integration checkout sees a feature with work in flight as ready. Progress
// reads the board column, as Build does, and the observed tree when one is
// supplied, since in-flight task states live on branches too.
func TestReportEpicProgressCountsReadyFeatureWithWorkInFlight(t *testing.T) {
	tr := epicTree([][3]string{{"E-a", "a", "ready"}},
		[][2]string{{"a", "ready"}, {"a", "ready"}, {"a", "done"}})
	tr.Tasks["T-1"] = &store.Task{ID: "T-1", FeatureID: "F-a", Status: "in_progress"}
	tr.Tasks["T-2"] = &store.Task{ID: "T-2", FeatureID: "F-b", Status: "ready"}
	got := epicProgress(tr)
	require.Equal(t, []ReportEpic{{Title: "Title a", Done: 1, InProgress: 1, NotStarted: 1}}, got)
}

func TestReportEpicProgressReadsTheObservedTree(t *testing.T) {
	base := epicTree([][3]string{{"E-a", "a", "ready"}}, [][2]string{{"a", "ready"}})
	base.Tasks["T-1"] = &store.Task{ID: "T-1", FeatureID: "F-a", Status: "ready"}
	observed := *base
	observed.Tasks = map[string]*store.Task{"T-1": {ID: "T-1", FeatureID: "F-a", Status: "in_review"}}
	got := ReportOf(base, ReportInput{Now: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC), Location: time.UTC, Observed: &observed})
	require.Equal(t, []ReportEpic{{Title: "Title a", InProgress: 1}}, got.Epics)
	got = ReportOf(base, ReportInput{Now: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC), Location: time.UTC})
	require.Equal(t, []ReportEpic{{Title: "Title a", NotStarted: 1}}, got.Epics)
}

func TestReportEpicProgressGhostsDraftEpicsInOrder(t *testing.T) {
	tr := epicTree([][3]string{
		{"E-a", "a", "ready"}, {"E-b", "b", "ready"}, {"E-c", "c", "draft"}, {"E-d", "d", "cancelled"},
	}, [][2]string{{"c", "draft"}, {"c", "specced"}, {"c", "cancelled"}, {"a", "active"}})
	tr.ProjectOrder = store.ProjectOrder{Epics: []string{"E-b", "E-c", "E-a"}}
	got := epicProgress(tr)
	require.Equal(t, []ReportEpic{
		{Title: "Title b"},
		{Title: "Title c", Planned: true, PlannedFeatures: 2},
		{Title: "Title a", InProgress: 1},
	}, got)
}

func TestReportEpicProgressGhostsKeylessDraftEpic(t *testing.T) {
	tr := epicTree([][3]string{{"E-a", "a", "ready"}}, nil)
	tr.Epics["E-b"] = &store.Epic{ID: "E-b", Title: "Keyless", Status: "draft"}
	got := epicProgress(tr)
	require.Contains(t, got, ReportEpic{Title: "Keyless", Planned: true})
	require.Len(t, got, 2)
}

func TestReportEpicProgressGhostsDraftEpicMissingFromOrder(t *testing.T) {
	tr := epicTree([][3]string{{"E-a", "a", "ready"}, {"E-b", "b", "ready"}, {"E-c", "c", "draft"}},
		[][2]string{{"c", "draft"}})
	tr.ProjectOrder = store.ProjectOrder{Epics: []string{"E-b", "E-a"}}
	got := epicProgress(tr)
	require.Len(t, got, 3)
	require.Equal(t, []string{"Title b", "Title a"}, []string{got[0].Title, got[1].Title})
	require.Contains(t, got, ReportEpic{Title: "Title c", Planned: true, PlannedFeatures: 1})
}

func TestReportEpicProgressPlannedOnlyForDraft(t *testing.T) {
	tr := epicTree([][3]string{
		{"E-a", "a", "ready"}, {"E-b", "b", "done"}, {"E-c", "c", "archived"}, {"E-d", "d", "draft"},
	}, nil)
	planned := map[string]bool{}
	for _, e := range epicProgress(tr) {
		planned[e.Title] = e.Planned
	}
	require.Equal(t, map[string]bool{
		"Title a": false, "Title b": false, "Title c": false, "Title d": true,
	}, planned)
}

func TestReportDraftEpicsLeaveScopeAndKPIsAlone(t *testing.T) {
	approved := [][3]string{{"E-a", "a", "ready"}}
	feats := [][2]string{{"a", "done"}, {"a", "active"}}
	in := ReportInput{Now: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), Location: time.UTC}
	base := ReportOf(epicTree(approved, feats), in)

	withDrafts := ReportOf(epicTree(append(approved, [3]string{"E-b", "b", "draft"}),
		append(feats, [2]string{"b", "draft"}, [2]string{"b", "specced"})), in)

	require.Equal(t, base.BurnUp, withDrafts.BurnUp)
	require.Equal(t, base.KPIs, withDrafts.KPIs)
	require.Equal(t, 1, withDrafts.KPIs.FeaturesDone)
	require.Equal(t, 2, withDrafts.KPIs.FeaturesTotal)
	require.Len(t, withDrafts.Epics, 2)
	require.Equal(t, ReportEpic{Title: "Title b", Planned: true, PlannedFeatures: 2}, withDrafts.Epics[1])
}

func TestReportEpicProgressKeepsDoneAndEmpty(t *testing.T) {
	tr := epicTree([][3]string{{"E-a", "a", "done"}, {"E-b", "b", "ready"}, {"E-c", "c", "archived"}},
		[][2]string{{"a", "done"}})
	got := epicProgress(tr)
	require.Len(t, got, 3)
	require.Equal(t, ReportEpic{Title: "Title a", Done: 1}, got[0])
	require.Equal(t, ReportEpic{Title: "Title b"}, got[1])
	require.Equal(t, ReportEpic{Title: "Title c"}, got[2])
}
