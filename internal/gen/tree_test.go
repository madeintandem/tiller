package gen

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// Shared ordering fixture: dependency order opposes key order in loading;
// key order decides the independent review tasks in export.
func reversedSiblingTree() *store.Tree {
	epicKey, loadKey, exportKey := "form-engine", "form-engine/loading", "form-engine/export"
	makeTask := func(id, key, feature, status string, deps ...string) *store.Task {
		return &store.Task{ID: id, Key: &key, FeatureID: feature, Status: status, Owner: "jc", DependsOn: deps}
	}
	alpha := makeTask("T-aaaa01", loadKey+"/zulu", "F-loadng", "in_progress")
	mike := makeTask("T-mmmm03", loadKey+"/mike", "F-loadng", "in_progress", alpha.ID)
	bravo := makeTask("T-bbbb02", loadKey+"/alfa", "F-loadng", "in_progress", mike.ID)
	charlie := makeTask("T-cccc03", exportKey+"/yankee", "F-export", "in_review")
	delta := makeTask("T-dddd04", exportKey+"/bravo", "F-export", "in_review")
	return &store.Tree{
		Manifest: &store.Manifest{Project: store.ProjectBlock{Key: "acme"}, Contributors: []store.Contributor{{ID: "jc", Name: "JC"}}},
		Epics:    map[string]*store.Epic{"E-formen": {ID: "E-formen", Key: &epicKey, Status: "ready"}},
		Features: map[string]*store.Feature{
			"F-loadng": {ID: "F-loadng", Key: &loadKey, EpicKey: epicKey, Status: "active"},
			"F-export": {ID: "F-export", Key: &exportKey, EpicKey: epicKey, Status: "active"},
		},
		Tasks:        map[string]*store.Task{alpha.ID: alpha, mike.ID: mike, bravo.ID: bravo, charlie.ID: charlie, delta.ID: delta},
		FeatureTasks: map[string][]*store.Task{"F-loadng": {bravo, mike, alpha}, "F-export": {delta, charlie}},
	}
}

// mapTree builds an in-memory tree with fixed ids, keys, and titles, so the tree
// model is exercised deterministically without touching disk. foundation is an
// approved epic with two features (one carrying an eval, neither in flight, so it
// is idle); form-engine is an approved epic whose one feature is active and
// carries two tasks, which makes form-engine derived-active.
func mapTree() *store.Tree {
	foundKey := "foundation"
	formKey := "form-engine"
	dbKey := "foundation/db-core"
	authnKey := "foundation/authn"
	loadKey := "form-engine/nrca-form-loading"
	// The two tasks are held once and pointed at from both indexes, the shape the
	// store leaves on load: Tasks resolves by id, FeatureTasks groups per feature.
	task01Key := "form-engine/nrca-form-loading/schema-transcription"
	task02Key := "form-engine/nrca-form-loading/wire-validation"
	task01 := &store.Task{ID: "T-task01", Key: &task01Key, Title: "Transcribe the schema", Status: "in_progress", Owner: "jc", FeatureID: "F-loadng"}
	task02 := &store.Task{ID: "T-task02", Key: &task02Key, Title: "Wire validation", Status: "proposed", Owner: "jc", FeatureID: "F-loadng"}
	return &store.Tree{
		Manifest: &store.Manifest{Project: store.ProjectBlock{Key: "acme"}},
		Epics: map[string]*store.Epic{
			"E-founda": {ID: "E-founda", Key: &foundKey, Title: "Foundation", Status: "ready"},
			"E-formen": {ID: "E-formen", Key: &formKey, Title: "Form Engine", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-dbcore": {ID: "F-dbcore", Key: &dbKey, Title: "Database core", Status: "ready", EpicKey: "foundation", HasEval: true},
			"F-authnn": {ID: "F-authnn", Key: &authnKey, Title: "Authentication", Status: "specced", EpicKey: "foundation"},
			"F-loadng": {ID: "F-loadng", Key: &loadKey, Title: "NRCA form loading", Status: "active", EpicKey: "form-engine"},
		},
		Tasks:        map[string]*store.Task{"T-task01": task01, "T-task02": task02},
		FeatureTasks: map[string][]*store.Task{"F-loadng": {task01, task02}},
		Questions:    map[string]*store.Question{},
	}
}

// nodeByKey finds a top-level or child node by its canonical key, so a test can
// assert on a node without depending on slice position.
func nodeByKey(nodes []TreeNode, key string) TreeNode {
	for _, n := range nodes {
		if n.Key == key {
			return n
		}
	}
	return TreeNode{}
}

// TestTreeBuildsDefinitionAxis is the core projection: the whole-project map is
// the epic -> feature -> task hierarchy in declared (key, then id) order, each
// node carrying its identity, status, owner, and the prose drill target path.
func TestTreeBuildsDefinitionAxis(t *testing.T) {
	m := Tree(mapTree())
	require.Equal(t, "acme", m.Project)

	// Top level is the two epics in key order: form-engine before foundation.
	require.Len(t, m.Nodes, 2)
	require.Equal(t, "form-engine", m.Nodes[0].Key)
	require.Equal(t, "foundation", m.Nodes[1].Key)

	found := m.Nodes[1]
	require.Equal(t, store.NounEpic, found.Noun)
	require.Equal(t, "E-founda", found.ID)
	require.Equal(t, "Foundation", found.Title)
	require.Equal(t, "ready", found.Status)
	require.Empty(t, found.Owner)
	require.Equal(t, []store.Contributor{}, found.Contributors)
	require.Equal(t, ".tiller/epics/foundation/epic.md", found.Path)

	// foundation's features sort by key: authn before db-core, each pointing at
	// its feature.md prose file.
	require.Len(t, found.Children, 2)
	require.Equal(t, "foundation/authn", found.Children[0].Key)
	require.Equal(t, "foundation/db-core", found.Children[1].Key)
	require.Equal(t, store.NounFeature, found.Children[1].Noun)
	require.Equal(t, ".tiller/epics/foundation/features/db-core/feature.md", found.Children[1].Path)

	// form-engine's one feature carries its two tasks, each a folder node whose
	// path is its own task.md implementation spec.
	fe := m.Nodes[0]
	require.Len(t, fe.Children, 1)
	loading := fe.Children[0]
	require.Len(t, loading.Children, 2)
	require.Equal(t, store.NounTask, loading.Children[0].Noun)
	require.Equal(t, "T-task01", loading.Children[0].ID)
	require.Equal(t, "form-engine/nrca-form-loading/schema-transcription", loading.Children[0].Key)
	require.Equal(t, "Transcribe the schema", loading.Children[0].Title)
	require.Equal(t, ".tiller/epics/form-engine/features/nrca-form-loading/tasks/schema-transcription/task.md",
		loading.Children[0].Path)
	require.Equal(t, "T-task02", loading.Children[1].ID)
}

func TestTreeSeparatesAncestorContributorsFromTaskOwners(t *testing.T) {
	tree := mapTree()
	tree.Contributors = map[string][]store.Contributor{
		"E-formen": {{ID: "past", Name: "Past contributor"}},
		"F-loadng": {{ID: "past", Name: "Past contributor"}},
	}
	model := Tree(tree)
	epic := nodeByKey(model.Nodes, "form-engine")
	require.Equal(t, tree.Contributors["E-formen"], epic.Contributors)
	require.Equal(t, tree.Contributors["F-loadng"], epic.Children[0].Contributors)
	require.Empty(t, epic.Owner)
	require.Empty(t, epic.Children[0].Owner)
	require.Equal(t, "jc", epic.Children[0].Children[0].Owner)
	data, err := json.Marshal(model)
	require.NoError(t, err)
	var wire struct {
		Nodes []map[string]json.RawMessage `json:"nodes"`
	}
	require.NoError(t, json.Unmarshal(data, &wire))
	for _, ancestor := range wire.Nodes {
		require.NotContains(t, ancestor, "owner")
		require.Contains(t, ancestor, "contributors")
	}
	require.JSONEq(t, "[]", string(wire.Nodes[1]["contributors"]))
	require.JSONEq(t, `[{"id":"past","name":"Past contributor"}]`, string(wire.Nodes[0]["contributors"]))
}

// TestTreeChildrenFollowSiblingOrder confirms a feature's task children render
// in dependency order, key order deciding among tasks that record no edge.
// reversedSiblingTree's loading chain runs opposite to its keys and its ids
// both, so a map that took the group as held, or fell back to sorting by id,
// would visibly disagree; its export pair records nothing, so key order stands
// there.
func TestTreeChildrenFollowSiblingOrder(t *testing.T) {
	fe := nodeByKey(Tree(reversedSiblingTree()).Nodes, "form-engine")

	loading := nodeByKey(fe.Children, "form-engine/loading")
	require.Len(t, loading.Children, 3)
	require.Equal(t, []string{"T-aaaa01", "T-mmmm03", "T-bbbb02"},
		[]string{loading.Children[0].ID, loading.Children[1].ID, loading.Children[2].ID},
		"the recorded chain decides: zulu, then the mike that follows it, then the alfa that follows mike")

	export := nodeByKey(fe.Children, "form-engine/export")
	require.Len(t, export.Children, 2)
	require.Equal(t, "T-dddd04", export.Children[0].ID, "no edges recorded, so the key-leading task leads, though its id sorts second")
	require.Equal(t, "T-cccc03", export.Children[1].ID)
}

// TestTreeFilesFootprint pins the --files footprint each node carries: the prose
// .md first, then the .yml, then feature.eval.md when a feature has one. An
// epic's footprint is its narrative and its state and nothing else, since it
// carries no eval contract of its own (artifact-schema.md, Epics). A task owns
// its own two files and contributes neither to its feature's footprint.
func TestTreeFilesFootprint(t *testing.T) {
	m := Tree(mapTree())

	found := nodeByKey(m.Nodes, "foundation")
	require.Equal(t, []string{
		".tiller/epics/foundation/epic.md",
		".tiller/epics/foundation/epic.yml",
	}, found.Files)

	// db-core carries an eval: md, yml, eval.md.
	db := nodeByKey(found.Children, "foundation/db-core")
	require.Equal(t, []string{
		".tiller/epics/foundation/features/db-core/feature.md",
		".tiller/epics/foundation/features/db-core/feature.yml",
		".tiller/epics/foundation/features/db-core/feature.eval.md",
	}, db.Files)

	// authn carries no eval: md, yml only.
	authn := nodeByKey(found.Children, "foundation/authn")
	require.Equal(t, []string{
		".tiller/epics/foundation/features/authn/feature.md",
		".tiller/epics/foundation/features/authn/feature.yml",
	}, authn.Files)

	// the active feature has tasks but no eval: md, yml, and nothing of its
	// children's, because a task owns its own folder.
	loading := nodeByKey(m.Nodes, "form-engine").Children[0]
	require.Equal(t, []string{
		".tiller/epics/form-engine/features/nrca-form-loading/feature.md",
		".tiller/epics/form-engine/features/nrca-form-loading/feature.yml",
	}, loading.Files)
	require.Equal(t, []string{
		".tiller/epics/form-engine/features/nrca-form-loading/tasks/schema-transcription/task.md",
		".tiller/epics/form-engine/features/nrca-form-loading/tasks/schema-transcription/task.yml",
	}, loading.Children[0].Files)
}

// TestTreeEpicActivity confirms each epic node carries its derived working
// status: form-engine is active (an in-flight child feature) and foundation is
// idle (no child in flight). A feature node never carries an activity.
func TestTreeEpicActivity(t *testing.T) {
	m := Tree(mapTree())
	require.Equal(t, EpicActivityActive, nodeByKey(m.Nodes, "form-engine").Activity)
	require.Equal(t, EpicActivityIdle, nodeByKey(m.Nodes, "foundation").Activity)
	require.Equal(t, "", nodeByKey(nodeByKey(m.Nodes, "foundation").Children, "foundation/db-core").Activity)
}

// TestSubtreeRoots confirms the scope arg roots the map at one epic or feature,
// and that a task (or any non-axis noun) has no subtree.
func TestSubtreeRoots(t *testing.T) {
	tr := mapTree()

	m, ok := Subtree(tr, tr.Epics["E-founda"])
	require.True(t, ok)
	require.Len(t, m.Nodes, 1)
	require.Equal(t, "foundation", m.Nodes[0].Key)
	require.Len(t, m.Nodes[0].Children, 2)

	m, ok = Subtree(tr, tr.Features["F-loadng"])
	require.True(t, ok)
	require.Len(t, m.Nodes, 1)
	require.Equal(t, store.NounFeature, m.Nodes[0].Noun)
	require.Equal(t, "form-engine/nrca-form-loading", m.Nodes[0].Key)
	require.Len(t, m.Nodes[0].Children, 2)

	_, ok = Subtree(tr, tr.Tasks["T-task01"])
	require.False(t, ok, "a task roots no subtree of its own")
}

// TestTreeDeterministic confirms two builds over the same tree are identical, the
// property the goldens and the future console rely on.
func TestTreeDeterministic(t *testing.T) {
	tr := mapTree()
	require.Equal(t, Tree(tr), Tree(tr))
}

// TestTreeEpicsFollowDependencyOrderNotKey pins Task A1 (issue #27) at the
// `tiller tree` grain: the top-level epics come out in DependencyOrder, not
// plain key order. "alpha" depends on "zeta", so key order (alpha before
// zeta) and dependency order (zeta, the foundation, before alpha, what rests
// on it) disagree; a map still sorting by key alone would fail this.
func TestTreeEpicsFollowDependencyOrderNotKey(t *testing.T) {
	tree := acmeEpicsTree()
	m := Tree(tree)

	keys := make([]string, len(m.Nodes))
	for i, n := range m.Nodes {
		keys[i] = n.Key
	}
	require.Equal(t, DependencyOrder(tree).Epics(), idsOf(m.Nodes), "the whole-project map follows DependencyOrder")
	require.Equal(t, []string{
		"foundation", "ops-security",
		"form-engine", "migration", "notifications", "payments",
		"lifecycle", "audit-engine", "reporting",
	}, keys, "foundation-first: audit-engine sorts alphabetically first but lands seventh")
}

// TestTreeFeaturesFollowDependencyOrderNotKey pins the same rule one level
// down: an epic's features order by DependencyOrder too. rendering-modes
// depends on schema-engine but sorts before it alphabetically, so a features
// slice still sorted by key would list them backwards.
func TestTreeFeaturesFollowDependencyOrderNotKey(t *testing.T) {
	tree := &store.Tree{
		Manifest: &store.Manifest{Project: store.ProjectBlock{Key: "acme"}},
		Epics: map[string]*store.Epic{
			"E-form-engine": epic("E-form-engine", "form-engine"),
		},
		Features: map[string]*store.Feature{
			"F-rendering": feature("F-rendering", "form-engine/rendering-modes", "form-engine", "F-schema"),
			"F-schema":    feature("F-schema", "form-engine/schema-engine", "form-engine"),
		},
	}
	fe := nodeByKey(Tree(tree).Nodes, "form-engine")
	require.Len(t, fe.Children, 2)
	require.Equal(t, "form-engine/schema-engine", fe.Children[0].Key)
	require.Equal(t, "form-engine/rendering-modes", fe.Children[1].Key)
}

// idsOf projects a TreeNode slice onto its ids, so a test can compare the
// map's top-level order against DependencyOrder.Epics() directly.
func idsOf(nodes []TreeNode) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

// TestTreeKeylessTaskNamesNoFile pins the empty-local-segment answer at the
// third of the three places that derive a folder from a key. lastSegment("") is
// "", and path.Join drops an empty segment, so a naive join would print a
// task.md directly under the FEATURE: a path that is some other node's or
// nobody's, on a row claiming to be this task's footprint. The row renders by
// id with no path and no files instead, which is what Label and ProsePath
// already answer for a keyless node.
func TestTreeKeylessTaskNamesNoFile(t *testing.T) {
	tr := mapTree()
	keyless := &store.Task{ID: "T-nokey", Title: "No key recorded", Status: "proposed", FeatureID: "F-loadng"}
	tr.Tasks["T-nokey"] = keyless
	tr.FeatureTasks["F-loadng"] = append(tr.FeatureTasks["F-loadng"], keyless)

	loading := nodeByKey(Tree(tr).Nodes, "form-engine").Children[0]
	var got TreeNode
	for _, c := range loading.Children {
		if c.ID == "T-nokey" {
			got = c
		}
	}
	require.Equal(t, "T-nokey", got.ID, "the row still renders")
	require.Empty(t, got.Path, "a task naming no folder drills nowhere")
	require.Empty(t, got.Files, "and claims no footprint")
	require.NotContains(t, got.Path, "nrca-form-loading/tasks/task.md",
		"never the feature-level path an empty segment would collapse to")
}
