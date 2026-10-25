package relationships

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestStructuralNormalizesDependenciesAndRetainsEveryAssertion(t *testing.T) {
	tr := &store.Tree{
		Epics: map[string]*store.Epic{"E-aaaaaa": {ID: "E-aaaaaa", Key: new("app")}},
		Features: map[string]*store.Feature{
			"F-aaaaaa": {ID: "F-aaaaaa", Key: new("app/alpha"), EpicKey: "app", DependsOn: []string{"app/zulu", "F-zzzzzz", "F-missng"}},
			"F-zzzzzz": {ID: "F-zzzzzz", Key: new("app/zulu"), EpicKey: "app"},
		},
	}
	before, err := json.Marshal(tr)
	require.NoError(t, err)
	index := BuildStructural(tr)
	deps := index.Adjacent("F-aaaaaa", "depends_on")
	require.Len(t, deps, 2)
	reverse := index.Adjacent("F-zzzzzz", "depended_on_by")
	require.Len(t, reverse, 1)
	require.Equal(t, "F-aaaaaa", reverse[0].From)
	require.Len(t, reverse[0].Sources, 2)
	missing, err := index.Resolve("F-missng")
	require.NoError(t, err)
	require.True(t, missing.Unresolved)
	require.Empty(t, missing.Title)
	require.Nil(t, missing.Destination)
	require.True(t, index.Complete, "a recorded dangling reference is not incomplete extraction")
	after, err := json.Marshal(tr)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestStructuralAmbiguousAndIneligibleDependenciesNeverChooseTargets(t *testing.T) {
	tr := &store.Tree{
		Features: map[string]*store.Feature{
			"F-source": {ID: "F-source", DependsOn: []string{"app/twin"}},
			"F-first":  {ID: "F-first", Key: new("app/twin")},
			"F-second": {ID: "F-second", Key: new("app/twin")},
		},
		Tasks: map[string]*store.Task{
			"T-source": {ID: "T-source", FeatureID: "F-source", DependsOn: []string{"app/other/task"}},
			"T-other":  {ID: "T-other", Key: new("app/other/task"), FeatureID: "F-other"},
		},
	}
	index := BuildStructural(tr)
	require.Empty(t, index.Adjacent("F-first", "depended_on_by"))
	require.Empty(t, index.Adjacent("F-second", "depended_on_by"))
	require.Empty(t, index.Adjacent("T-other", "depended_on_by"))
	require.False(t, index.Complete)
	for _, source := range []string{"F-source", "T-source"} {
		edges := index.Adjacent(source, "depends_on")
		require.Len(t, edges, 1)
		require.True(t, index.Vertices[edges[0].To].Unresolved)
		require.NotEmpty(t, edges[0].Sources[0].Raw)
	}
}

func TestStructuralSupersessionReconcilesAndReportsContradictions(t *testing.T) {
	tr := &store.Tree{Features: map[string]*store.Feature{
		"F-old": {ID: "F-old", SupersededBy: new("F-new")},
		"F-new": {ID: "F-new", Supersedes: []string{"F-old"}},
	}}
	index := BuildStructural(tr)
	edges := index.Adjacent("F-old", "superseded_by")
	require.Len(t, edges, 1)
	require.Equal(t, "F-new", edges[0].From)
	require.Len(t, edges[0].Sources, 2)
	tr.Features["F-rival"] = &store.Feature{ID: "F-rival", Supersedes: []string{"F-old"}}
	index = BuildStructural(tr)
	require.Len(t, index.Adjacent("F-old", "superseded_by"), 2, "contradictions stay inspectable rather than repaired")
	found := false
	for _, d := range index.Diagnostics {
		if d.Code == "conflicting_supersession" {
			found = true
			require.Contains(t, d.Subjects, "F-old")
			require.Len(t, d.Subjects, 2)
		}
	}
	require.True(t, found)
}

func TestStructuralRecordedQuestionsShelvesAndScopesAreNotLifecyclePolicy(t *testing.T) {
	tr := &store.Tree{
		Features:  map[string]*store.Feature{"F-work": {ID: "F-work", Status: "done", Surface: store.Surface{Paths: []string{"internal/store/../store"}}, AssumptionBasis: store.AssumptionBasis{"Q-open": {}}}},
		Questions: map[string]*store.Question{"Q-open": {ID: "Q-open", Status: "answered", Blocks: []string{"F-work"}}},
		Bugs:      map[string]*store.Bug{"B-found": {WorkItem: store.WorkItem{ID: "B-found", Status: "cancelled", AttachedTo: "F-work", ResolvedBy: "F-work", DuplicateOf: "B-other", DependsOn: []string{"chores/cleanup"}}}},
		Chores:    map[string]*store.Chore{"CH-clean": {WorkItem: store.WorkItem{ID: "CH-clean", Key: new("chores/cleanup")}}},
	}
	index := BuildStructural(tr)
	for _, direction := range []string{"attached_to", "resolved_by", "duplicate_of", "depends_on"} {
		require.Len(t, index.Adjacent("B-found", direction), 1)
	}
	require.Len(t, index.Adjacent("F-work", "blocked_by"), 1)
	require.Len(t, index.Adjacent("F-work", "assumes"), 1)
	covers := index.Adjacent("F-work", "covers")
	require.Len(t, covers, 1)
	require.Equal(t, "path:internal/store", covers[0].To)
	require.Equal(t, "work_surface", covers[0].Sources[0].Basis)
}

func TestSupersessionCycleWarningsIncludeEveryParticipant(t *testing.T) {
	tree := &store.Tree{Features: map[string]*store.Feature{
		"F-aaaaaa": {ID: "F-aaaaaa", Supersedes: []string{"F-bbbbbb"}},
		"F-bbbbbb": {ID: "F-bbbbbb", Supersedes: []string{"F-cccccc"}},
		"F-cccccc": {ID: "F-cccccc", Supersedes: []string{"F-aaaaaa"}},
		"F-other":  {ID: "F-other"},
	}}
	index := BuildStructural(tree)
	for _, id := range []string{"F-aaaaaa", "F-bbbbbb", "F-cccccc"} {
		diagnostics := index.DiagnosticsFor(id)
		require.Len(t, diagnostics, 1)
		require.Equal(t, []string{"F-aaaaaa", "F-bbbbbb", "F-cccccc"}, diagnostics[0].Subjects)
	}
	require.Empty(t, index.DiagnosticsFor("F-other"))
}

func TestShelfTransferLineageIsReciprocalAndKeepsNouns(t *testing.T) {
	tree := &store.Tree{
		Bugs: map[string]*store.Bug{
			"B-old000": {WorkItem: store.WorkItem{ID: "B-old000", Status: "cancelled", SupersededBy: new("B-new000")}},
			"B-new000": {WorkItem: store.WorkItem{ID: "B-new000", Status: "ready", Supersedes: []string{"B-old000"}}},
		},
		Chores: map[string]*store.Chore{
			"CH-old00": {WorkItem: store.WorkItem{ID: "CH-old00", Status: "cancelled", SupersededBy: new("CH-new00")}},
			"CH-new00": {WorkItem: store.WorkItem{ID: "CH-new00", Status: "ready", Supersedes: []string{"CH-old00"}}},
		},
		Tweaks: map[string]*store.Tweak{
			"TW-old00": {WorkItem: store.WorkItem{ID: "TW-old00", Status: "cancelled", SupersededBy: new("TW-new00")}},
			"TW-new00": {WorkItem: store.WorkItem{ID: "TW-new00", Status: "ready", Supersedes: []string{"TW-old00"}}},
		},
	}
	index := BuildStructural(tree)
	var supersedes RelationKind
	for _, kind := range index.Catalog() {
		if kind.Name == "supersedes" {
			supersedes = kind
		}
	}
	require.Equal(t, "supersedes", supersedes.Name)
	for _, pair := range [][2]string{{"B-old000", "B-new000"}, {"CH-old00", "CH-new00"}, {"TW-old00", "TW-new00"}} {
		forward := index.Adjacent(pair[1], "supersedes")
		reverse := index.Adjacent(pair[0], "superseded_by")
		require.Len(t, forward, 1)
		require.Equal(t, forward, reverse)
		require.Equal(t, pair[0], forward[0].To)
		require.Len(t, forward[0].Sources, 2)
		require.Equal(t, index.Vertices[pair[0]].Kind, index.Vertices[pair[1]].Kind)
		require.Contains(t, supersedes.FromKinds, index.Vertices[forward[0].From].Kind)
		require.Contains(t, supersedes.ToKinds, index.Vertices[forward[0].To].Kind)
	}
	require.Empty(t, index.Diagnostics)
}

func TestTweakRelationshipsIncludeCrossShelfDependenciesAndDocuments(t *testing.T) {
	tweak := &store.Tweak{WorkItem: store.WorkItem{
		ID: "TW-polish", Key: new("tweaks/polish"), Title: "Polish button", Owner: "jc",
		AttachedTo: "F-work", ResolvedBy: "F-work", DuplicateOf: "TW-other",
		DependsOn: []string{"CH-clean", "bugs/found", "tweaks/other", "TW-missing"},
	}}
	tree := &store.Tree{
		Features: map[string]*store.Feature{"F-work": {ID: "F-work", Key: new("app/work")}},
		Bugs:     map[string]*store.Bug{"B-found": {WorkItem: store.WorkItem{ID: "B-found", Key: new("bugs/found"), DependsOn: []string{"TW-polish"}}}},
		Chores:   map[string]*store.Chore{"CH-clean": {WorkItem: store.WorkItem{ID: "CH-clean", Key: new("chores/clean")}}},
		Tweaks: map[string]*store.Tweak{
			"TW-polish": tweak,
			"TW-other":  {WorkItem: store.WorkItem{ID: "TW-other", Key: new("tweaks/other")}},
		},
	}
	index := BuildStructural(tree)
	require.Equal(t, "tweak", index.Vertices[tweak.ID].Kind)
	require.Equal(t, ".tiller/tweaks/polish/tweak.yml", index.Vertices[tweak.ID].Path)
	require.Len(t, index.Adjacent(tweak.ID, "depends_on"), 4)
	for name, target := range map[string]string{"attached_to": "F-work", "resolved_by": "F-work", "duplicate_of": "TW-other"} {
		edges := index.Adjacent(tweak.ID, name)
		require.Len(t, edges, 1)
		require.Equal(t, target, edges[0].To)
	}
	require.Equal(t, "tweak", index.Vertices["TW-missing"].Kind)
	require.True(t, index.Vertices["TW-missing"].Unresolved)
	require.Len(t, index.Adjacent(tweak.ID, "depended_on_by"), 1)
	for _, raw := range []string{"TW-other", "tweaks/other"} {
		tweak.DependsOn = []string{raw}
		dependencies := BuildDependencies(tree, []store.Node{tweak})
		require.Equal(t, "TW-other", dependencies.Adjacent(tweak.ID, "depends_on")[0].To)
		require.Empty(t, dependencies.Diagnostics)
	}
	file, ok := NodePath(tree, tweak, "md")
	require.True(t, ok)
	require.Equal(t, ".tiller/tweaks/polish/tweak.md", file)
	_, ok = NodePath(tree, tweak, "eval.md")
	require.False(t, ok)
	AddDocuments(tree, index, []Document{{Path: file}})
	require.Len(t, index.Adjacent(tweak.ID, "has_document"), 1)
	for _, kind := range index.Catalog() {
		switch kind.Name {
		case "depends_on", "supersedes", "duplicate_of":
			require.Contains(t, kind.ToKinds, "tweak")
			require.Contains(t, kind.FromKinds, "tweak")
		case "attached_to", "resolved_by", "has_document", "sourced_from":
			require.Contains(t, kind.FromKinds, "tweak")
		}
	}
}
