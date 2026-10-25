package views_test

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func strp(s string) *string { return &s }

// mapTree is the smallest tree with every shape the map draws: two epics
// where the second depends on the first, two features under the first epic
// (the second depending on the first), one feature under the second epic
// depending across epics, two tasks under one feature (the second depending
// on the first, exercising gen.OrderTasks's sibling ordering rather than a
// single-task list that never calls its comparator), one open question
// holding a feature, a bug found in a feature and carried by a task, and a
// chore.
func mapTree() *store.Tree {
	e1 := &store.Epic{ID: "E-1", Key: strp("foundation"), Title: "Foundation", Status: "ready"}
	e2 := &store.Epic{ID: "E-2", Key: strp("audit"), Title: "Audit", Status: "draft", DependsOn: []string{"E-1"}}
	f1 := &store.Feature{ID: "F-1", Key: strp("foundation/skeleton"), EpicKey: "foundation", Title: "Skeleton", Status: "ready"}
	f2 := &store.Feature{ID: "F-2", Key: strp("foundation/ci"), EpicKey: "foundation", Title: "CI", Status: "draft", DependsOn: []string{"F-1"}}
	f3 := &store.Feature{ID: "F-3", Key: strp("audit/trail"), EpicKey: "audit", Title: "Trail", Status: "draft", DependsOn: []string{"foundation/ci"}}
	t1 := &store.Task{ID: "T-1", Key: strp("foundation/skeleton/init"), FeatureID: "F-1", Title: "Init", Status: "in_progress", Owner: "jc", Started: true}
	t2 := &store.Task{ID: "T-2", Key: strp("foundation/skeleton/wire-up"), FeatureID: "F-1", Title: "Wire up", Status: "draft", DependsOn: []string{"T-1"}}
	q := &store.Question{ID: "Q-1", Title: "Which CI?", Statement: "Which CI runner?", Status: "open", Blocks: []string{"F-2"}}
	b := &store.Bug{}
	b.ID, b.Key, b.Title, b.Status = "B-1", strp("bugs/blank"), "Blank on reload", "proposed"
	b.AttachedTo, b.ResolvedBy = "F-2", "T-1"
	c := &store.Chore{}
	c.ID, c.Key, c.Title, c.Status = "CH-1", strp("chores/bump"), "Bump Go", "ready"
	tree := &store.Tree{
		Manifest:     &store.Manifest{},
		Epics:        map[string]*store.Epic{e1.ID: e1, e2.ID: e2},
		Features:     map[string]*store.Feature{f1.ID: f1, f2.ID: f2, f3.ID: f3},
		Tasks:        map[string]*store.Task{t1.ID: t1, t2.ID: t2},
		FeatureTasks: map[string][]*store.Task{f1.ID: {t1, t2}},
		Questions:    map[string]*store.Question{q.ID: q},
		Bugs:         map[string]*store.Bug{b.ID: b},
		Chores:       map[string]*store.Chore{c.ID: c},
	}
	tree.ProjectContributors()
	return tree
}

func ids(nodes []views.MapNode) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID
	}
	return out
}

func TestMapOfNodesRideInDependencyOrderWithContainersAndHolds(t *testing.T) {
	v := views.MapOf(mapTree(), nil)

	// Epics in dependency order, each epic's features after it in order, a
	// feature's tasks after it (T-2 depends on T-1, so it rides after it),
	// then the shelf: bugs, then chores.
	require.Equal(t, []string{"E-1", "F-1", "T-1", "T-2", "F-2", "E-2", "F-3", "B-1", "CH-1"}, ids(v.Nodes))

	byID := map[string]views.MapNode{}
	for _, n := range v.Nodes {
		byID[n.ID] = n
	}
	require.Equal(t, "", byID["E-1"].Parent)
	require.Equal(t, "E-1", byID["F-1"].Parent)
	require.Equal(t, "F-1", byID["T-1"].Parent)
	require.Equal(t, "F-1", byID["T-2"].Parent)
	require.Equal(t, "", byID["B-1"].Parent, "a shelf node has no container")
	require.Equal(t, "foundation/skeleton", byID["F-1"].Key)
	require.Equal(t, "jc", byID["T-1"].Owner)
	require.Equal(t, []store.Contributor{{ID: "jc", Name: "jc"}}, byID["E-1"].Contributors)
	require.Equal(t, byID["E-1"].Contributors, byID["F-1"].Contributors)
	require.NotNil(t, byID["F-2"].Contributors)
	require.Empty(t, byID["F-2"].Contributors)
	require.Empty(t, byID["F-1"].Owner)
	require.Equal(t, 1, byID["F-2"].Holds, "the open question holds F-2")
	require.Equal(t, 0, byID["F-1"].Holds)

	raw, err := json.Marshal(v)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"parent":"E-1"`)
	require.Contains(t, string(raw), `"holds":1`)
	require.NotContains(t, string(raw), `"holds":0`, "holds is omitempty")
}

func TestMapOfOrdersShelvesByDependencyThenSoftOrderThenKey(t *testing.T) {
	bug := func(id, key string, dependsOn ...string) *store.Bug {
		b := &store.Bug{}
		b.ID, b.Key, b.Title, b.Status = id, strp(key), id, "ready"
		b.DependsOn = dependsOn
		return b
	}
	chore := func(id, key string) *store.Chore {
		c := &store.Chore{}
		c.ID, c.Key, c.Title, c.Status = id, strp(key), id, "ready"
		return c
	}

	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Bugs: map[string]*store.Bug{
			"B-dependent":    bug("B-dependent", "bugs/alpha", "B-prerequisite"),
			"B-key-fallback": bug("B-key-fallback", "bugs/bravo"),
			"B-prerequisite": bug("B-prerequisite", "bugs/yankee"),
			"B-soft":         bug("B-soft", "bugs/zulu"),
		},
		Chores: map[string]*store.Chore{
			"CH-key-fallback": chore("CH-key-fallback", "chores/alpha"),
			"CH-soft":         chore("CH-soft", "chores/zulu"),
		},
		BugOrder:   store.ShelfOrder{Order: []string{"B-soft"}},
		ChoreOrder: store.ShelfOrder{Order: []string{"CH-soft"}},
	}

	require.Equal(t, []string{
		"B-soft",
		"B-key-fallback",
		"B-prerequisite",
		"B-dependent",
		"CH-soft",
		"CH-key-fallback",
	}, ids(views.MapOf(tree, nil).Nodes))
}

// The badge counts the questions that name the card, not the questions
// holding the work under it. Operative holds inherit downward, so one
// epic-level question used to badge the epic, every feature inside it, and
// every task inside those, and the map read as a project drowning in open
// questions where there was one.
func TestMapOfBadgesOnlyTheNodeAQuestionNames(t *testing.T) {
	tree := mapTree()
	q := &store.Question{ID: "Q-2", Title: "Which cloud?", Statement: "Which cloud?", Status: "open", Blocks: []string{"E-1"}}
	tree.Questions[q.ID] = q

	byID := map[string]views.MapNode{}
	for _, n := range views.MapOf(tree, nil).Nodes {
		byID[n.ID] = n
	}
	require.Equal(t, 1, byID["E-1"].Holds, "the question names the epic")
	require.Equal(t, 0, byID["F-1"].Holds, "a hold on the epic is not a hold on the feature inside it")
	require.Equal(t, 0, byID["T-1"].Holds, "nor on the task inside that")
	require.Equal(t, 1, byID["F-2"].Holds, "the question naming F-2 still badges it")
}

func TestMapOfEdgesResolveByIDOrKeyAndDropDangling(t *testing.T) {
	tree := mapTree()
	// One dangling dependency, which must be dropped rather than drawn to nowhere.
	tree.Features["F-3"].DependsOn = append(tree.Features["F-3"].DependsOn, "F-gone")

	v := views.MapOf(tree, nil)

	require.ElementsMatch(t, []views.MapEdge{
		{From: "E-2", To: "E-1", Kind: "depends_on"},
		{From: "F-2", To: "F-1", Kind: "depends_on"},
		{From: "F-3", To: "F-2", Kind: "depends_on"}, // recorded by key, resolved to the id
		{From: "T-2", To: "T-1", Kind: "depends_on"},
		{From: "B-1", To: "F-2", Kind: "attached_to"},
		{From: "B-1", To: "T-1", Kind: "resolved_by"},
	}, v.Edges)

	raw, err := json.Marshal(v)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"edges":[`)
	require.Contains(t, string(raw), "F-gone", "the semantic graph retains unresolved assertions")
}

func TestMapOfEmptyTreeHasEmptyListsNotNulls(t *testing.T) {
	raw, err := json.Marshal(views.MapOf(&store.Tree{Manifest: &store.Manifest{}}, nil))
	require.NoError(t, err)
	var payload struct {
		Nodes         []views.MapNode         `json:"nodes"`
		Edges         []views.MapEdge         `json:"edges"`
		Relationships views.RelationshipGraph `json:"relationships"`
	}
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.NotNil(t, payload.Nodes)
	require.Empty(t, payload.Nodes)
	require.NotNil(t, payload.Edges)
	require.Empty(t, payload.Edges)
	require.NotNil(t, payload.Relationships.Vertices)
	require.Empty(t, payload.Relationships.Vertices)
}
