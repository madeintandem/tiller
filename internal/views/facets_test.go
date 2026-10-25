package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// facetTree is a small two-epic tree: Payments is live with one active
// feature and two tasks (jc in progress, bob done); Audit is cancelled
// with one cancelled feature and one cancelled task of jc's. A task owned by
// "ghost" sits off the roster.
func facetTree() *store.Tree {
	f1 := &store.Feature{ID: "F-000001", Key: new("payments/checkout"), Title: "Checkout", Status: "active", EpicKey: "payments"}
	f2 := &store.Feature{ID: "F-000002", Key: new("audit/export"), Title: "Export", Status: "cancelled", EpicKey: "audit"}
	t1 := &store.Task{ID: "T-000001", Key: new("payments/checkout/cart"), Title: "Cart", Status: "in_progress", Owner: "jc", FeatureID: "F-000001"}
	t2 := &store.Task{ID: "T-000002", Key: new("payments/checkout/pay"), Title: "Pay", Status: "done", Owner: "bob", FeatureID: "F-000001"}
	t3 := &store.Task{ID: "T-000003", Key: new("audit/export/csv"), Title: "CSV", Status: "cancelled", Owner: "jc", FeatureID: "F-000002"}
	t4 := &store.Task{ID: "T-000004", Key: new("payments/checkout/refund"), Title: "Refund", Status: "ready", Owner: "ghost", FeatureID: "F-000001"}
	people := []store.Contributor{{ID: "bob", Name: "Bob Chen"}, {ID: "jc", Name: "JC Grubbs"}}
	return &store.Tree{
		Manifest: &store.Manifest{Contributors: people},
		Epics: map[string]*store.Epic{
			"E-000001": {ID: "E-000001", Key: new("payments"), Title: "Payments", Status: "ready"},
			"E-000002": {ID: "E-000002", Key: new("audit"), Title: "Audit Engine", Status: "cancelled"},
		},
		Features:     map[string]*store.Feature{"F-000001": f1, "F-000002": f2},
		Tasks:        map[string]*store.Task{"T-000001": t1, "T-000002": t2, "T-000003": t3, "T-000004": t4},
		FeatureTasks: map[string][]*store.Task{"F-000001": {t1, t2, t4}, "F-000002": {t3}},
		Contributors: map[string][]store.Contributor{"F-000001": people, "E-000001": people},
	}
}

func TestResolveParentFiltersLeniently(t *testing.T) {
	tree := facetTree()

	byKey := views.ListFilters{Epic: "payments", Feature: "payments/checkout"}
	require.Empty(t, views.ResolveParentFiltersLeniently(tree, &byKey))
	require.Equal(t, "payments", byKey.Epic, "an epic resolves to its key, as the strict resolver does")
	require.Equal(t, "F-000001", byKey.Feature, "a feature resolves to its id")

	byID := views.ListFilters{Epic: "E-000001"}
	require.Empty(t, views.ResolveParentFiltersLeniently(tree, &byID))
	require.Equal(t, "payments", byID.Epic)

	stale := views.ListFilters{Epic: "nope", Feature: "payments/checkout"}
	unresolved := views.ResolveParentFiltersLeniently(tree, &stale)
	require.Equal(t, map[string]string{views.AxisEpic: "nope"}, unresolved)
	require.Equal(t, "nope", stale.Epic, "an unresolved value is left as typed")
	require.Equal(t, "F-000001", stale.Feature, "the other axis still resolves")

	none := views.ListFilters{}
	require.NotNil(t, views.ResolveParentFiltersLeniently(tree, &none), "the map is never nil")
}
func facetByAxis(t *testing.T, facets []views.Facet, axis string) []views.FacetOption {
	t.Helper()
	for _, f := range facets {
		if f.Axis == axis {
			return f.Options
		}
	}
	t.Fatalf("no %s facet in %v", axis, facets)
	return nil
}

func values(opts []views.FacetOption) []string {
	out := make([]string, len(opts))
	for i, o := range opts {
		out[i] = o.Value
	}
	return out
}

func TestFilterAxesPerNoun(t *testing.T) {
	require.Equal(t, []string{"status", "contributor"}, views.FilterAxes(store.NounEpic))
	require.Equal(t, []string{"status", "contributor", "epic"}, views.FilterAxes(store.NounFeature))
	require.Equal(t, []string{"status", "owner", "epic", "feature"}, views.FilterAxes(store.NounTask))
	require.Equal(t, []string{"status", "owner"}, views.FilterAxes(store.NounQuestion))
	require.Equal(t, []string{"status"}, views.FilterAxes(store.NounAmendment))
	require.Equal(t, []string{"status", "owner"}, views.FilterAxes(store.NounBug))
	require.Equal(t, []string{"status", "owner"}, views.FilterAxes(store.NounChore))
	require.Equal(t, []string{"status", "owner"}, views.FilterAxes(store.NounTweak))
	require.Nil(t, views.FilterAxes("widget"))

	table := views.FilterAxesByNoun()
	table[store.NounEpic][0] = "mutated"
	require.Equal(t, "status", views.FilterAxes(store.NounEpic)[0], "the returned table is a copy")
}

func TestFacetsFollowTheNounsAxes(t *testing.T) {
	facets := views.Facets(facetTree(), store.NounEpic, views.ListFilters{}, "", nil)
	require.Len(t, facets, 2)
	require.Equal(t, "status", facets[0].Axis)
	require.Equal(t, "contributor", facets[1].Axis)
}

func TestFacetsStatusLifecycleOrderWithUnfinished(t *testing.T) {
	opts := facetByAxis(t, views.Facets(facetTree(), store.NounTask, views.ListFilters{}, "", nil), views.AxisStatus)
	require.Equal(t, []string{"unfinished", "ready", "in_progress", "done", "cancelled"}, values(opts))
	require.Equal(t, 2, opts[0].Count, "unfinished counts ready and in_progress")
	require.Equal(t, "in_progress", opts[2].Label)
}

func TestFacetsCountAgainstTheOtherAxes(t *testing.T) {
	facets := views.Facets(facetTree(), store.NounTask, views.ListFilters{Owner: "jc"}, "", nil)

	status := facetByAxis(t, facets, views.AxisStatus)
	require.Equal(t, []string{"unfinished", "in_progress", "cancelled"}, values(status), "status counts only jc's tasks")

	owner := facetByAxis(t, facets, views.AxisOwner)
	require.Equal(t, []string{"bob", "ghost", "jc"}, values(owner), "the owner facet ignores its own value")
	require.True(t, owner[2].Selected)
	require.Equal(t, 2, owner[2].Count)
}

func TestFacetsKeepTheSelectedValueAtZero(t *testing.T) {
	facets := views.Facets(facetTree(), store.NounTask, views.ListFilters{Status: "specced"}, "", nil)

	status := facetByAxis(t, facets, views.AxisStatus)
	last := status[len(status)-1]
	require.Equal(t, views.FacetOption{Value: "specced", Label: "specced", Count: 0, Selected: true}, last,
		"a carried status illegal on tasks stays visible, after the lifecycle order")
	require.Empty(t, facetByAxis(t, facets, views.AxisOwner), "nothing matches specced, so no owner has a count")
	require.NotNil(t, facetByAxis(t, facets, views.AxisOwner), "options are an empty array, never null")
}

func TestFacetsPeopleLabelsAndYou(t *testing.T) {
	owner := facetByAxis(t, views.Facets(facetTree(), store.NounTask, views.ListFilters{}, "jc", nil), views.AxisOwner)
	require.Equal(t, []string{"jc", "bob", "ghost"}, values(owner), "You first, then by name")
	require.True(t, owner[0].You)
	require.Equal(t, "JC Grubbs", owner[0].Label)
	require.Equal(t, "Bob Chen", owner[1].Label)
	require.Equal(t, "ghost", owner[2].Label, "an unlisted person shows as the bare id")

	noViewer := facetByAxis(t, views.Facets(facetTree(), store.NounTask, views.ListFilters{}, "", nil), views.AxisOwner)
	for _, o := range noViewer {
		require.False(t, o.You)
	}

	contributor := facetByAxis(t, views.Facets(facetTree(), store.NounFeature, views.ListFilters{}, "", nil), views.AxisContributor)
	require.Equal(t, []string{"bob", "jc"}, values(contributor))
	require.Equal(t, 1, contributor[0].Count)
}

func TestFacetsParents(t *testing.T) {
	facets := views.Facets(facetTree(), store.NounTask, views.ListFilters{}, "", nil)

	epics := facetByAxis(t, facets, views.AxisEpic)
	require.Equal(t, []views.FacetOption{
		{Value: "E-000001", Label: "Payments", Key: "payments", Count: 3},
		{Value: "E-000002", Label: "Audit Engine", Key: "audit", Count: 1, Cancelled: true},
	}, epics, "cancelled parents sort last")

	features := facetByAxis(t, facets, views.AxisFeature)
	require.Equal(t, []views.FacetOption{
		{Value: "F-000001", Label: "Checkout", Key: "payments/checkout", Parent: "E-000001", Count: 3},
		{Value: "F-000002", Label: "Export", Key: "audit/export", Parent: "E-000002", Count: 1, Cancelled: true},
	}, features)
}

func TestFacetsSelectAParentByKeyOrReportItStale(t *testing.T) {
	tree := facetTree()

	byKey := views.ListFilters{Epic: "payments"} // an old key-based URL, resolved
	epics := facetByAxis(t, views.Facets(tree, store.NounTask, byKey, "", nil), views.AxisEpic)
	require.True(t, epics[0].Selected)
	require.Equal(t, "E-000001", epics[0].Value, "the option still carries the id the next pick writes")

	stale := views.ListFilters{Epic: "nope"} // left raw by the lenient resolver
	unresolved := views.ResolveParentFiltersLeniently(tree, &stale)
	epics = facetByAxis(t, views.Facets(tree, store.NounTask, stale, "", unresolved), views.AxisEpic)
	require.Equal(t, views.FacetOption{Value: "nope", Label: "nope", Selected: true}, epics[len(epics)-1])
}

func TestLenientParentsResolveIndependentlyAndStrictStaysStrict(t *testing.T) {
	for _, f := range []views.ListFilters{
		{Epic: "payments", Feature: "missing"},
		{Epic: "missing", Feature: "payments/checkout"},
		{Epic: "missing", Feature: "also-missing"},
		{Epic: "F-000001", Feature: "E-000001"},
	} {
		raw := f
		unresolved := views.ResolveParentFiltersLeniently(facetTree(), &f)
		require.NotEmpty(t, unresolved)
		for axis, value := range unresolved {
			if axis == views.AxisEpic {
				require.Equal(t, raw.Epic, value)
				require.Equal(t, raw.Epic, f.Epic)
			} else {
				require.Equal(t, raw.Feature, value)
				require.Equal(t, raw.Feature, f.Feature)
			}
		}
		require.Error(t, views.ResolveParentFilters(facetTree(), &raw))
	}
}

func TestAllNounAxesAndStatusLifecycles(t *testing.T) {
	assignment := []string{"proposed", "ready", "in_progress", "check", "in_review", "done", "cancelled"}
	for _, tc := range []struct {
		noun     string
		axes     []string
		statuses []string
	}{
		{store.NounEpic, []string{"status", "contributor"}, []string{"draft", "ready", "done", "archived", "cancelled"}},
		{store.NounFeature, []string{"status", "contributor", "epic"}, []string{"draft", "specced", "ready", "active", "done", "cancelled"}},
		{store.NounTask, []string{"status", "owner", "epic", "feature"}, assignment},
		{store.NounBug, []string{"status", "owner"}, assignment},
		{store.NounChore, []string{"status", "owner"}, assignment},
		{store.NounTweak, []string{"status", "owner"}, assignment},
		{store.NounQuestion, []string{"status", "owner"}, []string{"open", "answered", "cancelled"}},
		{store.NounAmendment, []string{"status"}, []string{"proposed", "approved", "withdrawn"}},
	} {
		t.Run(tc.noun, func(t *testing.T) {
			tree := &store.Tree{
				Epics: map[string]*store.Epic{}, Features: map[string]*store.Feature{}, Tasks: map[string]*store.Task{},
				Bugs: map[string]*store.Bug{}, Chores: map[string]*store.Chore{},
				Tweaks:    map[string]*store.Tweak{},
				Questions: map[string]*store.Question{}, Amendments: map[string]*store.Amendment{},
			}
			for _, status := range tc.statuses {
				switch tc.noun {
				case store.NounEpic:
					tree.Epics[status] = &store.Epic{ID: status, Status: status}
				case store.NounFeature:
					tree.Features[status] = &store.Feature{ID: status, Status: status}
				case store.NounTask:
					tree.Tasks[status] = &store.Task{ID: status, Status: status}
				case store.NounBug:
					tree.Bugs[status] = &store.Bug{WorkItem: store.WorkItem{ID: status, Status: status}}
				case store.NounChore:
					tree.Chores[status] = &store.Chore{WorkItem: store.WorkItem{ID: status, Status: status}}
				case store.NounTweak:
					tree.Tweaks[status] = &store.Tweak{WorkItem: store.WorkItem{ID: status, Status: status}}
				case store.NounQuestion:
					tree.Questions[status] = &store.Question{ID: status, Status: status}
				case store.NounAmendment:
					tree.Amendments[status] = &store.Amendment{ID: status, Status: status}
				}
			}
			facets := views.Facets(tree, tc.noun, views.ListFilters{}, "", nil)
			axes := make([]string, len(facets))
			for i, facet := range facets {
				axes[i] = facet.Axis
				require.NotNil(t, facet.Options)
			}
			require.Equal(t, tc.axes, axes)
			expected := tc.statuses
			if tc.noun == store.NounTask || store.IsWorkNoun(tc.noun) {
				expected = append([]string{"unfinished"}, expected...)
				require.Equal(t, 5, facets[0].Options[0].Count)
			}
			require.Equal(t, expected, values(facets[0].Options))
		})
	}
}

func pickFilter(f views.ListFilters, axis, value string) views.ListFilters {
	switch axis {
	case views.AxisStatus:
		f.Status = value
	case views.AxisOwner:
		f.Owner = value
	case views.AxisContributor:
		f.Contributor = value
	case views.AxisEpic:
		f.Epic = value
	case views.AxisFeature:
		f.Feature = value
	}
	return f
}

func TestFacetCountsAgreeWithPickingMatches(t *testing.T) {
	tree := facetTree()
	for _, noun := range []string{store.NounEpic, store.NounFeature, store.NounTask} {
		for _, f := range []views.ListFilters{{}, {Status: "unfinished", Owner: "jc"}, {Epic: "payments"}, {Feature: "F-000001"}, {Contributor: "bob"}, {Status: "specced", Owner: "missing"}} {
			for _, facet := range views.Facets(tree, noun, f, "", nil) {
				for _, option := range facet.Options {
					picked := pickFilter(f, facet.Axis, option.Value)
					if facet.Axis == views.AxisEpic && option.Key != "" {
						picked.Epic = option.Key
					}
					count := 0
					for _, node := range views.Collect(tree, noun) {
						if views.Matches(tree, node, picked) {
							count++
						}
					}
					require.Equal(t, count, option.Count, "%s %s %s with %+v", noun, facet.Axis, option.Value, f)
				}
			}
		}
	}
}

func TestFacetParentsDependencyThenCancellationOrder(t *testing.T) {
	tree := facetTree()
	tree.Epics["E-000003"] = &store.Epic{ID: "E-000003", Key: new("aaa"), Title: "Dependent", Status: "ready"}
	tree.Features["F-000003"] = &store.Feature{ID: "F-000003", Key: new("aaa/aaa"), EpicKey: "aaa", DependsOn: []string{"F-000001"}, Status: "active"}
	tree.Features["F-000004"] = &store.Feature{ID: "F-000004", Key: new("payments/aaa"), EpicKey: "payments", DependsOn: []string{"F-000001"}, Status: "active"}
	for _, id := range []string{"F-000003", "F-000004"} {
		feature := tree.Features[id]
		task := &store.Task{ID: "T-" + id, Key: new(*feature.Key + "/task"), FeatureID: id, Status: "ready"}
		tree.Tasks[task.ID] = task
		tree.FeatureTasks[id] = []*store.Task{task}
	}
	facets := views.Facets(tree, store.NounTask, views.ListFilters{Epic: "missing", Feature: "missing"}, "", map[string]string{views.AxisEpic: "missing", views.AxisFeature: "missing"})
	require.Equal(t, []string{"missing"}, values(facetByAxis(t, facets, views.AxisEpic)))
	facets = views.Facets(tree, store.NounTask, views.ListFilters{}, "", nil)
	require.Equal(t, []string{"E-000001", "E-000003", "E-000002"}, values(facetByAxis(t, facets, views.AxisEpic)))
	require.Equal(t, []string{"F-000001", "F-000004", "F-000003", "F-000002"}, values(facetByAxis(t, facets, views.AxisFeature)))
}

func TestFacetPeopleCaseInsensitiveTiesAndSelectedZero(t *testing.T) {
	tree := facetTree()
	tree.Manifest.Contributors = []store.Contributor{{ID: "jc", Name: "alice"}, {ID: "bob", Name: "ALICE"}}
	opts := facetByAxis(t, views.Facets(tree, store.NounTask, views.ListFilters{}, "", nil), views.AxisOwner)
	require.Equal(t, []string{"bob", "jc", "ghost"}, values(opts))
	facets := views.Facets(tree, store.NounTask, views.ListFilters{Status: "specced", Owner: "jc", Epic: "payments", Feature: "F-000001"}, "jc", nil)
	for _, axis := range []string{views.AxisOwner, views.AxisEpic, views.AxisFeature} {
		opts = facetByAxis(t, facets, axis)
		require.Len(t, opts, 1)
		require.True(t, opts[0].Selected)
		require.Zero(t, opts[0].Count)
	}
	require.True(t, facetByAxis(t, facets, views.AxisOwner)[0].You)
}

func TestLenientParentAmbiguityDoesNotBlockOtherParent(t *testing.T) {
	tree := facetTree()
	tree.Features["F-000003"] = &store.Feature{ID: "F-000003", Key: new("payments/checkout"), EpicKey: "payments"}
	f := views.ListFilters{Epic: "E-000001", Feature: "payments/checkout"}
	require.Equal(t, map[string]string{views.AxisFeature: "payments/checkout"}, views.ResolveParentFiltersLeniently(tree, &f))
	require.Equal(t, "payments", f.Epic)
	require.Equal(t, "payments/checkout", f.Feature)
}

func TestContributorSelectedAtZeroAndUnknownStatusOrder(t *testing.T) {
	tree := facetTree()
	facets := views.Facets(tree, store.NounFeature, views.ListFilters{Status: "done", Contributor: "jc"}, "", nil)
	require.Equal(t, []views.FacetOption{{Value: "jc", Label: "JC Grubbs", Selected: true}}, facetByAxis(t, facets, views.AxisContributor))
	tree.Tasks["T-000005"] = &store.Task{ID: "T-000005", Status: "zebra"}
	tree.Tasks["T-000006"] = &store.Task{ID: "T-000006", Status: "aardvark"}
	opts := facetByAxis(t, views.Facets(tree, store.NounTask, views.ListFilters{Status: "obsolete"}, "", nil), views.AxisStatus)
	require.Equal(t, []string{"unfinished", "ready", "in_progress", "done", "cancelled", "aardvark", "obsolete", "zebra"}, values(opts))
	require.True(t, opts[len(opts)-2].Selected)
	require.Zero(t, opts[len(opts)-2].Count)
}

func TestShelfAndQuestionPeopleCountsAgreeWithMatches(t *testing.T) {
	for _, noun := range []string{store.NounBug, store.NounChore, store.NounQuestion, store.NounAmendment} {
		tree := &store.Tree{
			Bugs: map[string]*store.Bug{
				"B-000001": {WorkItem: store.WorkItem{ID: "B-000001", Status: "ready", Owner: "jc"}},
				"B-000002": {WorkItem: store.WorkItem{ID: "B-000002", Status: "done", Owner: "bob"}},
			},
			Chores: map[string]*store.Chore{
				"CH-000001": {WorkItem: store.WorkItem{ID: "CH-000001", Status: "check", Owner: "jc"}},
				"CH-000002": {WorkItem: store.WorkItem{ID: "CH-000002", Status: "cancelled", Owner: "bob"}},
			},
			Questions: map[string]*store.Question{
				"Q-000001": {ID: "Q-000001", Status: "open", Owner: "jc"},
				"Q-000002": {ID: "Q-000002", Status: "answered", Owner: "bob"},
			},
			Amendments: map[string]*store.Amendment{
				"A-000001": {ID: "A-000001", Status: "proposed"},
				"A-000002": {ID: "A-000002", Status: "approved"},
			},
		}
		for _, f := range []views.ListFilters{{}, {Owner: "jc"}, {Status: "unfinished"}, {Status: "missing", Owner: "bob"}} {
			for _, facet := range views.Facets(tree, noun, f, "jc", nil) {
				for _, option := range facet.Options {
					count := 0
					for _, node := range views.Collect(tree, noun) {
						if views.Matches(tree, node, pickFilter(f, facet.Axis, option.Value)) {
							count++
						}
					}
					require.Equal(t, count, option.Count, "%s %s %s", noun, facet.Axis, option.Value)
				}
			}
		}
	}
}

func TestFacetsUnresolvedParentsBlockOtherAxesButOfferRecovery(t *testing.T) {
	tree := facetTree()
	f := views.ListFilters{Epic: "payments"}
	unresolved := map[string]string{views.AxisEpic: "payments"}
	tree.Epics["E-000001"].Key = nil
	facets := views.Facets(tree, store.NounTask, f, "", unresolved)
	require.Empty(t, facetByAxis(t, facets, views.AxisStatus))
	require.Empty(t, facetByAxis(t, facets, views.AxisOwner))
	require.Empty(t, facetByAxis(t, facets, views.AxisFeature))
	require.Equal(t, []views.FacetOption{
		{Value: "E-000002", Label: "Audit Engine", Key: "audit", Count: 1, Cancelled: true},
		{Value: "payments", Label: "payments", Selected: true},
	}, facetByAxis(t, facets, views.AxisEpic))
	f.Feature = "F-000001"
	unresolved[views.AxisFeature] = "F-000001"
	facets = views.Facets(tree, store.NounTask, f, "", unresolved)
	require.Equal(t, []views.FacetOption{{Value: "payments", Label: "payments", Selected: true}}, facetByAxis(t, facets, views.AxisEpic))
	require.Equal(t, []views.FacetOption{{Value: "F-000001", Label: "F-000001", Selected: true}}, facetByAxis(t, facets, views.AxisFeature))
}

func TestFacetsOmitMissingParentsUnlessSelectedAtZero(t *testing.T) {
	tree := facetTree()
	delete(tree.Features, "F-000001")
	delete(tree.Epics, "E-000001")
	facets := views.Facets(tree, store.NounTask, views.ListFilters{}, "", nil)
	require.Equal(t, []string{"F-000002"}, values(facetByAxis(t, facets, views.AxisFeature)))
	require.Equal(t, []string{"E-000002"}, values(facetByAxis(t, facets, views.AxisEpic)))
	f := views.ListFilters{Feature: "F-000001"}
	for _, unresolved := range []map[string]string{nil, {views.AxisFeature: "F-000001"}} {
		facets = views.Facets(tree, store.NounTask, f, "", unresolved)
		opts := facetByAxis(t, facets, views.AxisFeature)
		require.Equal(t, views.FacetOption{Value: "F-000001", Label: "F-000001", Selected: true}, opts[len(opts)-1])
	}
	f = views.ListFilters{Epic: "payments"}
	for _, unresolved := range []map[string]string{nil, {views.AxisEpic: "payments"}} {
		facets = views.Facets(tree, store.NounTask, f, "", unresolved)
		opts := facetByAxis(t, facets, views.AxisEpic)
		require.Equal(t, views.FacetOption{Value: "payments", Label: "payments", Selected: true}, opts[len(opts)-1])
	}
}
