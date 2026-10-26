package views

// These tests pin views.BuildBoardOf (build.go, task-12-brief.md): the Build
// view's server projection. They ride hand-built in-memory trees, the same
// house style streams_test.go and nodepage_test.go's siblingTree already use
// for this package's other order-sensitive projections, rather than driving
// mutate or a real repo, since BuildBoardOf is a pure function of a loaded
// store.Tree.

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// buildOrderTree lays two epics and two features where dependency order
// disagrees with key order at both grains, the identical "the dependency
// leads though the dependent's key sorts first" shape streams_test.go's
// streamsDependencyTree and streamsBlockedDependencyTree already pin for the
// streams lenses (issue #27): alpha depends on zeta at epic grain, and
// alpha/alpha-feature depends on alpha/zeta-feature at feature grain.
func buildOrderTree() *store.Tree {
	zetaKey, alphaKey := "zeta", "alpha"
	zFeatKey, aFeatKey := "alpha/zeta-feature", "alpha/alpha-feature"
	return &store.Tree{
		Manifest: &store.Manifest{Contributors: []store.Contributor{{ID: "jc", Name: "JC Grubbs"}}},
		Epics: map[string]*store.Epic{
			"E-zeta":  {ID: "E-zeta", Key: &zetaKey, Title: "Zeta", Status: "ready"},
			"E-alpha": {ID: "E-alpha", Key: &alphaKey, Title: "Alpha", Status: "ready", DependsOn: []string{"E-zeta"}},
		},
		Features: map[string]*store.Feature{
			"F-zfeat": {ID: "F-zfeat", Key: &zFeatKey, Title: "Zeta feature", Status: "draft", EpicKey: "alpha"},
			"F-afeat": {ID: "F-afeat", Key: &aFeatKey, Title: "Alpha feature", Status: "specced", EpicKey: "alpha", DependsOn: []string{"F-zfeat"}},
		},
		Tasks:     map[string]*store.Task{},
		Questions: map[string]*store.Question{},
	}
}

// TestBuildBoardOfOrdersEpicsAndFeaturesByDependencyNotKey pins the epic
// panel's own order and one epic's own feature order against key order:
// both must follow the recorded dependency, not the string each name sorts
// by.
func TestBuildBoardOfOrdersEpicsAndFeaturesByDependencyNotKey(t *testing.T) {
	board := BuildBoardOf(buildOrderTree())
	require.Len(t, board.Epics, 2)
	require.Equal(t, "E-zeta", board.Epics[0].ID, "zeta (the dependency) leads alpha, though alpha's key sorts first")
	require.Equal(t, "E-alpha", board.Epics[1].ID)
	require.Empty(t, board.Epics[0].Features, "zeta has no features of its own")

	alpha := board.Epics[1]
	require.Len(t, alpha.Features, 2)
	require.Equal(t, "F-zfeat", alpha.Features[0].ID, "the zeta-feature (the dependency) leads the alpha-feature that depends on it")
	require.Equal(t, "F-afeat", alpha.Features[1].ID)
	require.Equal(t, map[string]int{"draft": 1, "specced": 1}, alpha.FeatureCounts,
		"feature_counts is keyed by literal stored status, not a folded legend bucket")
}

// buildSoftOrderTree lays two epics and two features under one of them, both
// pairs edge-free (no depends_on at either grain), so key order alone would
// decide between them — except each grain also carries a recorded soft
// order (store.ProjectOrder.Epics at epic grain, store.Epic.Order at
// feature grain) that lists the key-losing sibling first. gen.
// DependencyOrder's own tie stack puts the soft order ahead of the key
// fallback (docs/spec/cli-surface.md), one tier this
// package's other fixture, buildOrderTree, never exercises: a fixture built
// only from depends_on edges cannot tell "BuildBoardOf reads the shared
// order" from "BuildBoardOf reads the shared order's key-only fallback",
// since a naive re-sort by key alone would pass it too.
func buildSoftOrderTree() *store.Tree {
	alphaKey, zuluKey := "alpha", "zulu"
	zFeatKey, aFeatKey := "zulu/zulu-feature", "zulu/alpha-feature"
	return &store.Tree{
		Manifest: &store.Manifest{Contributors: []store.Contributor{{ID: "jc", Name: "JC Grubbs"}}},
		Epics: map[string]*store.Epic{
			"E-alpha": {ID: "E-alpha", Key: &alphaKey, Title: "Alpha", Status: "ready"},
			"E-zulu": {ID: "E-zulu", Key: &zuluKey, Title: "Zulu", Status: "ready",
				// The recorded feature order lists the zulu feature ahead of the
				// alpha feature, though "zulu/alpha-feature" sorts first by key.
				Order: []string{"F-zfeat", "F-afeat"}},
		},
		Features: map[string]*store.Feature{
			"F-zfeat": {ID: "F-zfeat", Key: &zFeatKey, Title: "Zulu feature", Status: "draft", EpicKey: "zulu"},
			"F-afeat": {ID: "F-afeat", Key: &aFeatKey, Title: "Alpha feature", Status: "draft", EpicKey: "zulu"},
		},
		Tasks:     map[string]*store.Task{},
		Questions: map[string]*store.Question{},
		// The recorded root order lists zulu ahead of alpha, though "alpha"
		// sorts first by key.
		ProjectOrder: store.ProjectOrder{Epics: []string{"E-zulu", "E-alpha"}},
	}
}

// TestBuildBoardOfOrdersEpicsAndFeaturesBySoftOrderNotKey pins the soft-order
// tier of the shared computation at both grains BuildBoardOf serves: absent
// any recorded dependency, the recorded intent (`tiller order`) still
// outranks a plain key sort. A regression that swapped order.Epics() or
// order.Features() for a bare key sort would still pass buildOrderTree's
// dependency-edge fixture but would fail this one.
func TestBuildBoardOfOrdersEpicsAndFeaturesBySoftOrderNotKey(t *testing.T) {
	board := BuildBoardOf(buildSoftOrderTree())
	require.Len(t, board.Epics, 2)
	require.Equal(t, "E-zulu", board.Epics[0].ID, "the recorded root order lists zulu first, though alpha's key sorts first")
	require.Equal(t, "E-alpha", board.Epics[1].ID)

	zulu := board.Epics[0]
	require.Len(t, zulu.Features, 2)
	require.Equal(t, "F-zfeat", zulu.Features[0].ID,
		"the epic's recorded feature order lists the zulu feature first, though the alpha feature's key sorts first")
	require.Equal(t, "F-afeat", zulu.Features[1].ID)
}

// TestBuildBoardOfOrdersTasksByDependency reuses nodepage_test.go's
// siblingTree, the fixture already proven to catch a projection that falls
// back to key or id order: the loader feature's three tasks rank
// T-000004, T-000005, T-000003, neither their key order (...bravo is
// "alfa"'s key, sorting differently) nor their id order. Pinning the exact
// same sequence NodePageOf's Relationships.Tasks already renders for this
// feature (TestNodePageTasksFollowSiblingOrder) is the point: the board and
// the node page — and so `ls` — must never disagree about a feature's own
// task order.
func TestBuildBoardOfOrdersTasksByDependency(t *testing.T) {
	board := BuildBoardOf(siblingTree())
	require.Len(t, board.Epics, 1)
	require.Len(t, board.Epics[0].Features, 3)

	var loader *BuildFeature
	for i := range board.Epics[0].Features {
		if board.Epics[0].Features[i].ID == "F-0000fa" {
			loader = &board.Epics[0].Features[i]
		}
	}
	require.NotNil(t, loader, "the loader feature must be on the board")

	ids := make([]string, len(loader.Tasks))
	for i, tk := range loader.Tasks {
		ids[i] = tk.ID
		require.Equal(t, store.NounTask, tk.Noun)
	}
	require.Equal(t, []string{"T-000004", "T-000005", "T-000003"}, ids,
		"bravo leads the alfa that depends on it, and charlie follows the alfa it depends on")
}

// buildRollupTree lays one feature with three tasks: a dependency-free task,
// a done task that depends on it, and a cancelled task that records no
// edge.
func buildRollupTree() *store.Tree {
	epicKey := "widgets"
	featKey := "widgets/gizmo"
	bravoKey := "widgets/gizmo/bravo-task"
	alfaKey := "widgets/gizmo/alfa-task"
	charlieKey := "widgets/gizmo/charlie-cancelled"

	bravo := &store.Task{ID: "T-bravo0", Key: &bravoKey, Title: "Bravo", Status: "ready", Owner: "jc", FeatureID: "F-gizmo0"}
	alfa := &store.Task{ID: "T-alfa00", Key: &alfaKey, Title: "Alfa", Status: "done", Owner: "jc", FeatureID: "F-gizmo0", DependsOn: []string{"T-bravo0"}}
	charlie := &store.Task{ID: "T-charl0", Key: &charlieKey, Title: "Charlie", Status: "cancelled", Owner: "jc", FeatureID: "F-gizmo0"}

	return &store.Tree{
		Manifest: &store.Manifest{Contributors: []store.Contributor{{ID: "jc", Name: "JC Grubbs"}}},
		Epics: map[string]*store.Epic{
			"E-widget": {ID: "E-widget", Key: &epicKey, Title: "Widgets", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-gizmo0": {ID: "F-gizmo0", Key: &featKey, Title: "Gizmo", Status: "active", EpicKey: "widgets"},
		},
		Tasks:        map[string]*store.Task{"T-bravo0": bravo, "T-alfa00": alfa, "T-charl0": charlie},
		FeatureTasks: map[string][]*store.Task{"F-gizmo0": {alfa, bravo, charlie}},
		Questions:    map[string]*store.Question{},
	}
}

// TestBuildBoardOfTaskRollupExcludesCancelled pins the counting rule this
// file's own package comment documents: task_done and task_total both drop
// a cancelled task (mirroring gen.FeatureActivity's identical exclusion),
// while the feature's own Tasks list stays the complete, unfiltered
// dependency order (bravo and the edge-free cancelled task share the first
// layer, key order breaking the tie; alfa follows the bravo it depends on).
func TestBuildBoardOfTaskRollupExcludesCancelled(t *testing.T) {
	board := BuildBoardOf(buildRollupTree())
	require.Len(t, board.Epics, 1)
	require.Len(t, board.Epics[0].Features, 1)
	f := board.Epics[0].Features[0]

	require.Equal(t, 1, f.TaskDone, "alfa is done; the cancelled task never counts as done")
	require.Equal(t, 2, f.TaskTotal, "bravo and alfa; the cancelled task is not scope the feature still carries")

	require.Len(t, f.Tasks, 3, "the expand list is the full task list, unfiltered")
	ids := make([]string, len(f.Tasks))
	for i, tk := range f.Tasks {
		ids[i] = tk.ID
	}
	require.Equal(t, []string{"T-bravo0", "T-charl0", "T-alfa00"}, ids)
}

func buildTreeWithAttachedWork() *store.Tree {
	epicKey := "build"
	featureKey := "build/schema-engine"
	otherFeatureKey := "build/unrelated"
	taskKey := "build/schema-engine/parse"
	directBugKey := "bugs/direct"
	childChoreKey := "chores/child"
	doneBugKey := "bugs/done"
	otherBugKey := "bugs/other"

	task := &store.Task{
		ID: "T-000001", Key: &taskKey, Title: "Parse", Status: "ready", FeatureID: "F-000001",
	}

	return &store.Tree{
		Manifest: &store.Manifest{},
		Epics: map[string]*store.Epic{
			"E-000001": {ID: "E-000001", Key: &epicKey, Title: "Build", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-000001": {ID: "F-000001", Key: &featureKey, Title: "Schema engine", Status: "active", EpicKey: epicKey},
			"F-000002": {ID: "F-000002", Key: &otherFeatureKey, Title: "Unrelated", Status: "active", EpicKey: epicKey},
		},
		Tasks: map[string]*store.Task{
			"T-000001": task,
		},
		FeatureTasks: map[string][]*store.Task{
			"F-000001": {task},
		},
		Bugs: map[string]*store.Bug{
			"B-direct": {WorkItem: store.WorkItem{ID: "B-direct", Key: &directBugKey, Title: "Direct", Status: "proposed", AttachedTo: "F-000001"}},
			"B-done":   {WorkItem: store.WorkItem{ID: "B-done", Key: &doneBugKey, Title: "Done", Status: "done", AttachedTo: "T-000001"}},
			"B-other":  {WorkItem: store.WorkItem{ID: "B-other", Key: &otherBugKey, Title: "Other", Status: "ready", AttachedTo: "F-000002"}},
		},
		Chores: map[string]*store.Chore{
			"CH-child": {WorkItem: store.WorkItem{ID: "CH-child", Key: &childChoreKey, Title: "Child", Status: "ready", AttachedTo: "T-000001"}},
		},
		BugOrder:   store.ShelfOrder{Order: []string{"B-direct", "B-done", "B-other"}},
		ChoreOrder: store.ShelfOrder{Order: []string{"CH-child"}},
	}
}

func TestBuildFeatureCountsOpenAttachedShelfWorkAcrossTasks(t *testing.T) {
	got := BuildBoardOf(buildTreeWithAttachedWork())
	require.Len(t, got.Epics, 1)
	require.Len(t, got.Epics[0].Features, 2)

	require.Equal(t, 2, got.Epics[0].Features[0].OpenWorkItems)
}

// buildHeldTree lays a cancelled epic with one live feature under it,
// mirroring streams_test.go's own streamsHeldTree fixture (issue #39):
// gen.RetiredAncestor answers non-nil for the live feature.
func buildHeldTree() *store.Tree {
	epicKey := "widgets"
	liveFeatKey := "widgets/live-feature"
	epic := &store.Epic{ID: "E-widget", Key: &epicKey, Title: "Widgets", Status: "cancelled"}
	live := &store.Feature{ID: "F-live00", Key: &liveFeatKey, Title: "Live feature", Status: "active", EpicKey: epicKey}
	return &store.Tree{
		Manifest:  &store.Manifest{Contributors: []store.Contributor{{ID: "jc", Name: "JC Grubbs"}}},
		Epics:     map[string]*store.Epic{epic.ID: epic},
		Features:  map[string]*store.Feature{live.ID: live},
		Tasks:     map[string]*store.Task{},
		Questions: map[string]*store.Question{},
	}
}

func TestBuildBoardOfExcludesCancelledEpicWithLiveFeature(t *testing.T) {
	board := BuildBoardOf(buildHeldTree())
	require.Equal(t, []BuildEpic{}, board.Epics)
}

// buildManualBlockedTree lays one active feature carrying the manual
// blocked flag (store.Feature.Blocked), under a live epic, so nothing
// retired sits above it and Held stays nil: the manual flag and a held
// ancestor are distinct facts a card never folds into one (issue #39).
func buildManualBlockedTree() *store.Tree {
	epicKey := "widgets"
	featKey := "widgets/blocked-feature"
	epic := &store.Epic{ID: "E-widget", Key: &epicKey, Title: "Widgets", Status: "ready"}
	blocked := &store.Feature{ID: "F-block0", Key: &featKey, Title: "Blocked feature", Status: "active", EpicKey: epicKey, Blocked: true}
	return &store.Tree{
		Manifest:  &store.Manifest{Contributors: []store.Contributor{{ID: "jc", Name: "JC Grubbs"}}},
		Epics:     map[string]*store.Epic{epic.ID: epic},
		Features:  map[string]*store.Feature{blocked.ID: blocked},
		Tasks:     map[string]*store.Task{},
		Questions: map[string]*store.Question{},
	}
}

// TestBuildBoardOfManualBlockedFlagStaysInItsStatusColumn pins the manual
// blocked flag (Blocked, wired straight through views.Blocked — no new
// derivation) as a card condition orthogonal to status, mirroring
// streams.go's BlockedItem register (issue #39): a blocked active feature
// still serves status "active" (so a status-filtered column still carries
// it) and carries Blocked: true beside that status, distinct from Held,
// which stays nil since nothing retired sits above this feature.
func TestBuildBoardOfManualBlockedFlagStaysInItsStatusColumn(t *testing.T) {
	board := BuildBoardOf(buildManualBlockedTree())
	require.Len(t, board.Epics, 1)
	require.Len(t, board.Epics[0].Features, 1)
	f := board.Epics[0].Features[0]

	require.True(t, f.Blocked)
	require.Equal(t, "active", f.Status,
		"blocked is a condition, not a lifecycle state: the feature still serves its stored status, so it still lands in that status column")
	require.Nil(t, f.Held, "the manual flag and a held ancestor are distinct facts (issue #39); nothing retired sits above this feature")

	raw, err := json.Marshal(f)
	require.NoError(t, err)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &got))
	var blockedVal bool
	require.NoError(t, json.Unmarshal(got["blocked"], &blockedVal))
	require.True(t, blockedVal, "a manually blocked feature carries \"blocked\": true on the wire")
}

// TestBuildBoardOfJSONShapeOmitsAbsentFields pins the wire contract's
// omitempty behaviors this file's own additions introduce: a keyless epic
// (the degenerate folder-backed case decompositionOf and childFeatures
// already guard defensively) carries no "key" and no "feature_counts",
// features stays an empty array rather than being omitted or null, a live
// unblocked feature carries no "held" and no "blocked", and — the
// drift-mechanism call this file's own package comment defends — no feature
// row ever carries a "drift" key at all.
func TestBuildBoardOfJSONShapeOmitsAbsentFields(t *testing.T) {
	normalKey := "normal"
	featKey := "normal/only-feature"
	tree := &store.Tree{
		Manifest: &store.Manifest{Contributors: []store.Contributor{{ID: "jc", Name: "JC Grubbs"}}},
		Epics: map[string]*store.Epic{
			"E-keyless": {ID: "E-keyless", Status: "draft"},
			"E-normal":  {ID: "E-normal", Key: &normalKey, Title: "Normal", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-only00": {ID: "F-only00", Key: &featKey, Title: "Only feature", Status: "draft", EpicKey: "normal"},
		},
		Tasks:     map[string]*store.Task{},
		Questions: map[string]*store.Question{},
	}

	raw, err := json.Marshal(BuildBoardOf(tree))
	require.NoError(t, err)

	var payload struct {
		Epics []map[string]json.RawMessage `json:"epics"`
	}
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Len(t, payload.Epics, 2)

	var keyless, normal map[string]json.RawMessage
	for _, e := range payload.Epics {
		var id string
		require.NoError(t, json.Unmarshal(e["id"], &id))
		switch id {
		case "E-keyless":
			keyless = e
		case "E-normal":
			normal = e
		}
	}
	require.NotNil(t, keyless, "the keyless epic still rides the payload")
	require.NotNil(t, normal)

	_, hasKey := keyless["key"]
	require.False(t, hasKey, "a keyless epic omits key rather than emitting an empty string")
	_, hasCounts := keyless["feature_counts"]
	require.False(t, hasCounts, "a featureless epic omits feature_counts rather than an empty object")
	var keylessFeatures []json.RawMessage
	require.NoError(t, json.Unmarshal(keyless["features"], &keylessFeatures))
	require.Empty(t, keylessFeatures, "features is an empty array, not omitted or null, for a childless epic")

	_, hasKey = normal["key"]
	require.True(t, hasKey)
	var counts map[string]int
	require.NoError(t, json.Unmarshal(normal["feature_counts"], &counts))
	require.Equal(t, map[string]int{"draft": 1}, counts)

	var features []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(normal["features"], &features))
	require.Len(t, features, 1)
	_, hasHeld := features[0]["held"]
	require.False(t, hasHeld, "a live feature omits held rather than emitting null")
	_, hasBlocked := features[0]["blocked"]
	require.False(t, hasBlocked, "an unblocked feature omits blocked rather than emitting false")
	_, hasDrift := features[0]["drift"]
	require.False(t, hasDrift,
		"the projection carries no drift field at all (task-12-report.md's cost call): the client joins /api/drift by node id instead")

	var tasks []json.RawMessage
	require.NoError(t, json.Unmarshal(features[0]["tasks"], &tasks))
	require.Empty(t, tasks, "tasks is an empty array, not omitted or null, for a taskless feature")
}

// buildOrphanFeatureTree lays one real epic with one real feature, plus a
// second feature whose EpicKey names no epic in the tree at all - the
// degenerate shape store.Tree.EpicKey documents as set from directory
// nesting on Load (never itself stored), so tiller's own verbs cannot
// produce it; only a merge artifact or hand-edited YAML can. It still needs
// a guard (this repo's own history with silently-invisible nodes), which is
// what the test below is.
func buildOrphanFeatureTree() *store.Tree {
	epicKey := "widgets"
	homeFeatKey := "widgets/gizmo"
	orphanFeatKey := "nowhere/ghost"
	epic := &store.Epic{ID: "E-widget", Key: &epicKey, Title: "Widgets", Status: "ready"}
	home := &store.Feature{ID: "F-home00", Key: &homeFeatKey, Title: "Gizmo", Status: "active", EpicKey: epicKey}
	orphan := &store.Feature{ID: "F-ghost0", Key: &orphanFeatKey, Title: "Ghost feature", Status: "draft", EpicKey: "nowhere"}
	return &store.Tree{
		Manifest:  &store.Manifest{Contributors: []store.Contributor{{ID: "jc", Name: "JC Grubbs"}}},
		Epics:     map[string]*store.Epic{epic.ID: epic},
		Features:  map[string]*store.Feature{home.ID: home, orphan.ID: orphan},
		Tasks:     map[string]*store.Task{},
		Questions: map[string]*store.Question{},
	}
}

// TestBuildBoardOfOrphanFeatureIsInvisible pins the DOCUMENTED behavior for a
// feature whose EpicKey names no epic in the tree (task-23-brief.md item 6,
// carrying forward Task 12's own note to Task 14, progress.md: "orphan
// features (EpicKey naming no epic) are invisible on the board - same blind
// spot as gen.Tree, consistent not regressive, but this repo's history with
// silently-invisible nodes says check it"). buildEpicOf reaches a tree's
// features only through order.Features(epicKey), scoped to a REAL epic's own
// key; an orphan's own group in gen.Order's featuresByEpic map is never
// looked up by anyone, because no epic carries its key. So the orphan is not
// merely uncounted, it renders on no epic's card list, in no epic's
// FeatureCounts, and nowhere else on the board - the identical blind spot
// gen.Tree's own epicNode walk shares (both call order.Features(key) the same
// way), so this is not a Build-specific regression, but it is a conscious
// choice from here forward: change it only on purpose, not by accident.
func TestBuildBoardOfOrphanFeatureIsInvisible(t *testing.T) {
	board := BuildBoardOf(buildOrphanFeatureTree())
	require.Len(t, board.Epics, 1, "the orphan claims no epic row of its own")

	widgets := board.Epics[0]
	require.Equal(t, "E-widget", widgets.ID)
	require.Len(t, widgets.Features, 1, "only the real epic's own feature renders")
	require.Equal(t, "F-home00", widgets.Features[0].ID)
	require.Equal(t, map[string]int{"active": 1}, widgets.FeatureCounts,
		"the orphan's draft status never reaches any epic's count, its own included, since it has none")

	for _, e := range board.Epics {
		for _, f := range e.Features {
			require.NotEqual(t, "F-ghost0", f.ID, "the orphan renders under no epic on the board")
		}
	}
}

// TestBuildBoardOfIsDeterministic guards against map-iteration nondeterminism
// leaking into the served order, the same property TestStreamsAreDeterministic
// pins for the streams projection.
func TestBuildBoardOfIsDeterministic(t *testing.T) {
	tree := siblingTree()
	first := BuildBoardOf(tree)
	for i := 0; i < 20; i++ {
		require.Equal(t, first, BuildBoardOf(tree), "read %d disagreed with the first", i)
	}
}

func TestBuildContributorsAreStartedOwnersOnly(t *testing.T) {
	tree := buildRollupTree()
	tree.Tasks["T-alfa00"].Owner = "past"
	tree.Tasks["T-alfa00"].Started = true
	tree.Tasks["T-charl0"].Owner = "past"
	tree.Tasks["T-charl0"].Started = true
	added := []*store.Task{
		{ID: "T-check0", Key: new("widgets/gizmo/check"), Title: "Checked", Status: "check", Owner: "reviewer", Started: true, FeatureID: "F-gizmo0"},
		{ID: "T-propos", Key: new("widgets/gizmo/proposed"), Title: "Proposed", Status: "proposed", Owner: "jc", FeatureID: "F-gizmo0"},
		{ID: "T-pool00", Key: new("widgets/gizmo/pool"), Title: "Unclaimed", Status: "ready", FeatureID: "F-gizmo0"},
	}
	for _, task := range added {
		tree.Tasks[task.ID] = task
		tree.FeatureTasks[task.FeatureID] = append(tree.FeatureTasks[task.FeatureID], task)
	}
	tree.ProjectContributors()
	card := BuildBoardOf(tree).Epics[0].Features[0]
	require.Equal(t, []store.Contributor{{ID: "past", Name: "past"}, {ID: "reviewer", Name: "reviewer"}}, card.Contributors)
}

// TestBuildColumnShowsReadyFeatureWithWorkInFlightAsActive pins the board's
// display column: a feature whose stored status is still ready (its start
// lives on an unmerged review branch) lands in Active, and feature_counts
// agrees with the columns, once any task under it is in flight.
func TestBuildColumnShowsReadyFeatureWithWorkInFlightAsActive(t *testing.T) {
	tree := buildRollupTree()
	tree.Features["F-gizmo0"].Status = "ready"
	tree.Tasks["T-alfa00"].Status = "ready"
	card := BuildBoardOf(tree).Epics[0]
	require.Equal(t, "ready", card.Features[0].Status)
	require.Equal(t, "ready", card.Features[0].Column)
	require.Equal(t, map[string]int{"ready": 1}, card.FeatureCounts)

	tree.Tasks["T-bravo0"].Status = "in_review"
	card = BuildBoardOf(tree).Epics[0]
	require.Equal(t, "ready", card.Features[0].Status, "the stored status is never replaced")
	require.Equal(t, "active", card.Features[0].Column)
	require.Equal(t, map[string]int{"active": 1}, card.FeatureCounts)
}
