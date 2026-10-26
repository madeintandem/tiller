package views

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin the console's uniform node page (ui-surface.md, "Node
// detail"): the exact JSON payload for a feature, and the relationship hub's
// behavior per noun. The tree is built in memory with fixed ids and the events
// with fixed timestamps, so every asserted byte is deterministic.

// pageTree lays a small graph: an epic with two features, one feature with two
// ledger tasks, a dangling dependency, and a successor that supersedes it, an
// open question blocking the first feature, and an answered one that must not.
func pageTree() *store.Tree {
	epicKey := "form-engine"
	f1Key := "form-engine/schema-engine"
	f2Key := "form-engine/form-loading"
	answer := "JSON Schema"
	successor := "F-222222"
	t333Key := "form-engine/schema-engine/parse"
	t444Key := "form-engine/schema-engine/validate"
	t333 := &store.Task{ID: "T-333333", Key: &t333Key, Title: "Parse the schema", Status: "proposed", FeatureID: "F-111111"}
	t444 := &store.Task{ID: "T-444444", Key: &t444Key, Title: "Validate the schema", Status: "proposed", FeatureID: "F-111111"}
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-aaa111": {ID: "E-aaa111", Key: &epicKey, Title: "Form Engine", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-111111": {
				ID: "F-111111", Key: &f1Key, Title: "Schema engine", Status: "specced",
				ChangeType: "create", EpicKey: "form-engine",
				DependsOn:    []string{"F-zzzzzz"},
				SupersededBy: &successor,
			},
			"F-222222": {
				ID: "F-222222", Key: &f2Key, Title: "Form loading", Status: "draft",
				ChangeType: "create", EpicKey: "form-engine",
				DependsOn:  []string{"F-111111"},
				Supersedes: []string{"F-111111", "F-yyyyyy"},
			},
		},
		Tasks: map[string]*store.Task{
			"T-333333": t333,
			"T-444444": t444,
		},
		FeatureTasks: map[string][]*store.Task{
			"F-111111": {t333, t444},
		},
		Questions: map[string]*store.Question{
			"Q-555555": {ID: "Q-555555", Statement: "Which schema dialect?", Status: "open", Owner: "jc",
				Source: "design/rfc-3.md", Blocks: []string{"F-111111"}},
			"Q-666666": {ID: "Q-666666", Statement: "Settled already?", Status: "answered", Owner: "jc",
				Answer: &answer, Blocks: []string{"F-111111"}},
		},
	}
}

func operativeBlockerPageTree() (*store.Tree, *store.Feature, *store.Task) {
	epicKey := "routing"
	featureKey := "routing/decision"
	taskKey := "routing/decision/walk"
	epic := &store.Epic{
		ID: "E-route", Key: &epicKey, Title: "Routing", Status: "ready",
	}
	feature := &store.Feature{
		ID: "F-decision", Key: &featureKey, Title: "Route decision", Status: "active",
		EpicKey: epicKey,
		AssumptionBasis: store.AssumptionBasis{
			"Q-300-assumed": store.Assumption{},
		},
	}
	task := &store.Task{
		ID: "T-walk", Key: &taskKey, Title: "Walk routes", Status: "ready",
		FeatureID: feature.ID,
	}
	tree := &store.Tree{
		Epics:        map[string]*store.Epic{epic.ID: epic},
		Features:     map[string]*store.Feature{feature.ID: feature},
		Tasks:        map[string]*store.Task{task.ID: task},
		FeatureTasks: map[string][]*store.Task{feature.ID: {task}},
		Questions: map[string]*store.Question{
			"Q-100-epic": {
				ID: "Q-100-epic", Title: "Choose the route root",
				Statement: "Which route root should every decision inherit?",
				Status:    "open", Blocks: []string{epicKey},
			},
			"Q-200-feature": {
				ID: "Q-200-feature", Title: "Choose the decision",
				Statement: "Which decision should this feature make?",
				Status:    "open", Blocks: []string{feature.ID},
			},
			"Q-300-assumed": {
				ID: "Q-300-assumed", Title: "Confirm the working assumption",
				Statement: "Can the feature proceed on its recorded assumption?",
				Status:    "open", Blocks: []string{feature.ID},
			},
			"Q-400-multiple": {
				ID: "Q-400-multiple", Title: "Choose the shared walk",
				Statement: "Which walk applies across all three grains?",
				Status:    "open", Blocks: []string{task.ID, feature.ID, epicKey},
			},
		},
	}
	return tree, feature, task
}

// pageEvents is a small stream around the first feature: its add, its spec,
// and an unrelated task add.
func pageEvents() []events.Event {
	at := func(min int) time.Time { return time.Date(2026, 7, 1, 10, min, 0, 0, time.UTC) }
	return []events.Event{
		{ID: "EV-000001", Type: "add.feature", At: at(0), Contributor: "jc",
			Command: "add feature form-engine/schema-engine", Targets: []string{"F-111111"}},
		{ID: "EV-000002", Type: "spec.feature", At: at(5), Contributor: "jc",
			Command: "spec feature form-engine/schema-engine", Targets: []string{"F-111111"}},
		{ID: "EV-000003", Type: "add.task", At: at(10), Contributor: "jc",
			Command: "add task F-111111", Targets: []string{"T-333333"}},
	}
}

func nodeOf(t *testing.T, tree *store.Tree, id string) store.Node {
	t.Helper()
	switch id[0] {
	case 'E':
		return tree.Epics[id]
	case 'F':
		return tree.Features[id]
	case 'T':
		return tree.Tasks[id]
	case 'Q':
		return tree.Questions[id]
	}
	t.Fatalf("unknown fixture id %q", id)
	return nil
}

// TestNodePageTaskCarriesItsOwnPath pins the task page: the reference is the
// task's own three-segment key, the hub climbs to both the parent feature and
// the grandparent epic, and the prose pointer names the task's own
// implementation spec (artifact-schema.md, Tasks).
func TestNodePageTaskCarriesItsOwnPath(t *testing.T) {
	tree := pageTree()
	page := NodePageOf(tree, pageEvents(), nodeOf(t, tree, "T-333333"), nil)

	require.Equal(t, "form-engine/schema-engine/parse (T-333333)", page.Ref)
	require.NotNil(t, page.Relationships.Feature)
	require.Equal(t, "form-engine/schema-engine (F-111111)", page.Relationships.Feature.Ref)
	require.NotNil(t, page.Relationships.Epic)
	require.Equal(t, "form-engine (E-aaa111)", page.Relationships.Epic.Ref)

	require.NotNil(t, page.Prose, "a task owns its implementation spec")
	require.Equal(t, ".tiller/epics/form-engine/features/schema-engine/tasks/parse/task.md", page.Prose.Path)
	_, err := json.Marshal(page)
	require.NoError(t, err)

	require.Len(t, page.History, 1)
	require.Equal(t, "add.task", page.History[0].Type)
}

func TestNodePageUnresolvedParentsAreAbsent(t *testing.T) {
	tree := pageTree()
	tree.Features["F-111111"].EpicKey = "missing-epic"
	feature := NodePageOf(tree, nil, tree.Features["F-111111"], nil)
	require.Nil(t, feature.Relationships.Epic)
	task := NodePageOf(tree, nil, tree.Tasks["T-333333"], nil)
	require.NotNil(t, task.Relationships.Feature)
	require.Nil(t, task.Relationships.Epic)

	tree.Tasks["T-333333"].FeatureID = "F-missing"
	task = NodePageOf(tree, nil, tree.Tasks["T-333333"], nil)
	require.Nil(t, task.Relationships.Feature)
	require.Nil(t, task.Relationships.Epic)
}

func TestNodePageDanglingLegacyReferencesKeepRecordedRows(t *testing.T) {
	tree := pageTree()
	f := tree.Features["F-111111"]
	f.DependsOn = []string{"missing/work"}
	f.Supersedes = []string{"missing/old"}
	page := NodePageOf(tree, nil, f, nil)
	require.Equal(t, []RelatedNode{{ID: "missing/work", Ref: "missing/work"}}, page.Relationships.DependsOn)
	require.Equal(t, []RelatedNode{{ID: "missing/old", Ref: "missing/old"}}, page.Relationships.Supersedes)

	shelf := nodePageShelfTree()
	shelf.Bugs["B-dangling"].AttachedTo = "missing/work/task"
	bug := NodePageOf(shelf, nil, shelf.Bugs["B-dangling"], nil)
	require.Equal(t, &RelatedNode{ID: "missing/work/task", Ref: "missing/work/task"}, bug.Relationships.AttachedTo)
}

func TestNodePageQuestionPreservesMixedTargetOrder(t *testing.T) {
	tree := pageTree()
	q := tree.Questions["Q-555555"]
	q.Blocks = []string{"F-222222", "missing/work", "form-engine", "T-missing", "form-engine/schema-engine"}
	page := NodePageOf(tree, nil, q, nil)
	var ids []string
	for _, target := range page.Relationships.Blocks {
		ids = append(ids, target.ID)
	}
	require.Equal(t, []string{"F-222222", "missing/work", "E-aaa111", "T-missing", "F-111111"}, ids)
	require.Equal(t, RelatedNode{ID: "missing/work", Ref: "missing/work"}, page.Relationships.Blocks[1])
	require.Equal(t, RelatedNode{ID: "T-missing", Ref: "T-missing"}, page.Relationships.Blocks[3])
}

func TestNodePageFeatureUsesOperativeQuestionBlockers(t *testing.T) {
	tree, feature, _ := operativeBlockerPageTree()

	page := NodePageOf(tree, nil, feature, nil)

	require.Equal(t, []RelatedNode{
		{
			Noun: store.NounQuestion, ID: "Q-100-epic", Ref: "Q-100-epic", Status: "open",
			Title: "Choose the route root", Statement: "Which route root should every decision inherit?",
		},
		{
			Noun: store.NounQuestion, ID: "Q-200-feature", Ref: "Q-200-feature", Status: "open",
			Title: "Choose the decision", Statement: "Which decision should this feature make?",
		},
		{
			Noun: store.NounQuestion, ID: "Q-400-multiple", Ref: "Q-400-multiple", Status: "open",
			Title: "Choose the shared walk", Statement: "Which walk applies across all three grains?",
		},
	}, page.Relationships.BlockedByQuestions,
		"the page inherits the epic-key blocker, filters the assumed blocker, and counts a multi-grain question once")
}

func TestNodePageTaskUsesOperativeQuestionBlockersWithoutInheritingFeatureAssumptions(t *testing.T) {
	tree, _, task := operativeBlockerPageTree()

	page := NodePageOf(tree, nil, task, nil)

	require.Equal(t, []RelatedNode{
		{
			Noun: store.NounQuestion, ID: "Q-100-epic", Ref: "Q-100-epic", Status: "open",
			Title: "Choose the route root", Statement: "Which route root should every decision inherit?",
		},
		{
			Noun: store.NounQuestion, ID: "Q-200-feature", Ref: "Q-200-feature", Status: "open",
			Title: "Choose the decision", Statement: "Which decision should this feature make?",
		},
		{
			Noun: store.NounQuestion, ID: "Q-300-assumed", Ref: "Q-300-assumed", Status: "open",
			Title: "Confirm the working assumption", Statement: "Can the feature proceed on its recorded assumption?",
		},
		{
			Noun: store.NounQuestion, ID: "Q-400-multiple", Ref: "Q-400-multiple", Status: "open",
			Title: "Choose the shared walk", Statement: "Which walk applies across all three grains?",
		},
	}, page.Relationships.BlockedByQuestions,
		"the task inherits feature and epic blockers, retains the feature-assumed question, and counts a multi-grain question once")
}

// TestNodePageEpicListsFeaturesByDependencyOrder pins the epic page: children
// in DependencyOrder, and the
// prose pointer naming the epic's epic.md with content left empty for the
// serving layer to fill. pageTree records form-loading --depends-on
// schema-engine, so schema-engine (the dependency) leads even though
// "form-engine/form-loading" sorts first by key.
func TestNodePageEpicListsFeaturesByDependencyOrder(t *testing.T) {
	tree := pageTree()
	page := NodePageOf(tree, nil, nodeOf(t, tree, "E-aaa111"), nil)

	require.Len(t, page.Relationships.Features, 2)
	require.Equal(t, "form-engine/schema-engine", page.Relationships.Features[0].Key)
	require.Equal(t, "form-engine/form-loading", page.Relationships.Features[1].Key)
	require.NotNil(t, page.Prose)
	require.Equal(t, ".tiller/epics/form-engine/epic.md", page.Prose.Path)
	require.Empty(t, page.Prose.Content, "views never read files; content is the handler's job")
	require.NotNil(t, page.History, "history is an array even with no events")
	require.Empty(t, page.History)
}

// TestNodePageFeatureSupersedes pins the supersession edges from the
// successor's side: supersedes resolves the recorded ids ref-sorted, with a
// dangling id degraded to a bare-id row (which sorts ahead of the composed
// references, capital letters before lowercase), and superseded_by stays
// empty on a feature nothing replaced.
func TestNodePageFeatureSupersedes(t *testing.T) {
	tree := pageTree()
	page := NodePageOf(tree, nil, nodeOf(t, tree, "F-222222"), nil)

	require.Len(t, page.Relationships.Supersedes, 2)
	require.Equal(t, "F-yyyyyy", page.Relationships.Supersedes[0].Ref)
	require.Equal(t, "form-engine/schema-engine (F-111111)", page.Relationships.Supersedes[1].Ref)
	require.Empty(t, page.Relationships.SupersededBy)
}

// TestNodePageQuestionListsBlocks pins the question page: the reference is the
// bare id (a question is id plus statement), blocks resolve to link rows in
// the ledger's recorded order, and the page carries no prose pointer because
// a question's prose is its statement and answer on the node itself. An open
// question has no answer, and the view never invents one.
func TestNodePageQuestionListsBlocks(t *testing.T) {
	tree := pageTree()
	page := NodePageOf(tree, nil, nodeOf(t, tree, "Q-555555"), nil)

	require.Equal(t, "Q-555555", page.Ref)
	require.Equal(t, "Which schema dialect?", page.Node.Statement)
	require.Len(t, page.Relationships.Blocks, 1)
	require.Equal(t, "form-engine/schema-engine (F-111111)", page.Relationships.Blocks[0].Ref)
	require.Equal(t, "Schema engine", page.Relationships.Blocks[0].Title)
	require.Nil(t, page.Prose, "a question owns no prose file")

	require.Nil(t, page.Node.Answer)
	got, err := json.Marshal(page)
	require.NoError(t, err)
	require.NotContains(t, string(got), `"answer"`)
}

func TestNodePageQuestionResolvesLegacyKeyBlocks(t *testing.T) {
	tree := pageTree()
	answer := "Keep every recorded target after the question closes."
	question := &store.Question{
		ID:        "Q-legacy-blocks",
		Title:     "Resolve legacy block targets",
		Statement: "Do old key-shaped block targets still resolve?",
		Status:    "answered",
		Answer:    &answer,
		Blocks: []string{
			"form-engine/schema-engine/parse",
			"F-222222",
			"form-engine",
			"form-engine/schema-engine",
			"F-missing",
		},
	}
	tree.Questions[question.ID] = question

	page := NodePageOf(tree, nil, question, nil)

	require.Equal(t, []RelatedNode{
		{
			Noun: store.NounTask, ID: "T-333333", Key: "form-engine/schema-engine/parse",
			Ref: "form-engine/schema-engine/parse (T-333333)", Status: "proposed", Title: "Parse the schema",
		},
		{
			Noun: store.NounFeature, ID: "F-222222", Key: "form-engine/form-loading",
			Ref: "form-engine/form-loading (F-222222)", Status: "draft", Title: "Form loading",
		},
		{
			Noun: store.NounEpic, ID: "E-aaa111", Key: "form-engine",
			Ref: "form-engine (E-aaa111)", Status: "ready", Title: "Form Engine",
		},
		{
			Noun: store.NounFeature, ID: "F-111111", Key: "form-engine/schema-engine",
			Ref: "form-engine/schema-engine (F-111111)", Status: "specced", Title: "Schema engine",
		},
		{ID: "F-missing", Ref: "F-missing"},
	}, page.Relationships.Blocks,
		"legacy epic, feature, and task keys resolve without changing the ledger's recorded order")
	require.Equal(t, []BlockRef{
		{ID: "T-333333", Key: "form-engine/schema-engine/parse"},
		{ID: "F-222222", Key: "form-engine/form-loading"},
		{ID: "E-aaa111", Key: "form-engine"},
		{ID: "F-111111", Key: "form-engine/schema-engine"},
		{ID: "F-missing"},
	}, page.Node.Blocks)
}

// TestNodePageQuestionFlatViewCarriesSourceAndBlocksAlongsideRelationships
// pins the serve half: the flat node.source/node.blocks fields (what
// the question's own record claims) ride the SAME page as
// relationships.blocks (the resolved graph edge Relationships.Blocks already
// carried), harmlessly, because they answer different questions
// (NodeView.Blocks' own doc comment, node.go). node.blocks carries the
// minimal id-plus-key BlockRef; relationships.blocks carries the full
// RelatedNode (noun, ref, status, title) for the browsable hub.
func TestNodePageQuestionFlatViewCarriesSourceAndBlocksAlongsideRelationships(t *testing.T) {
	tree := pageTree()
	page := NodePageOf(tree, nil, nodeOf(t, tree, "Q-555555"), nil)

	require.Equal(t, "design/rfc-3.md", page.Node.Source)
	require.Equal(t, []BlockRef{{ID: "F-111111", Key: "form-engine/schema-engine"}}, page.Node.Blocks)

	require.Len(t, page.Relationships.Blocks, 1, "the resolved-graph edge is untouched by this fix")
	require.Equal(t, "form-engine/schema-engine (F-111111)", page.Relationships.Blocks[0].Ref)
	require.Equal(t, "Schema engine", page.Relationships.Blocks[0].Title)

	got, err := json.Marshal(page)
	require.NoError(t, err)
	require.Contains(t, string(got), `"source":"design/rfc-3.md"`)
	require.Contains(t, string(got), `"blocks":[{"id":"F-111111","key":"form-engine/schema-engine"}]`,
		"the flat node.blocks field, id plus key")
}

// TestNodePageAnsweredQuestionCarriesAnswer pins the answered arm: the node
// view surfaces the recorded answer, the piece the console's question body
// renders beside the statement (ui-surface.md, "Node detail").
func TestNodePageAnsweredQuestionCarriesAnswer(t *testing.T) {
	tree := pageTree()
	page := NodePageOf(tree, nil, nodeOf(t, tree, "Q-666666"), nil)

	require.NotNil(t, page.Node.Answer)
	require.Equal(t, "JSON Schema", *page.Node.Answer)
	got, err := json.Marshal(page)
	require.NoError(t, err)
	require.Contains(t, string(got), `"answer":"JSON Schema"`)
}

func TestQuestionRelatedRowsUseTitleAndRetainFullStatement(t *testing.T) {
	titledStatement := strings.Repeat("This is the full titled question statement. ", 6)
	legacyStatement := strings.Repeat("界", 81)
	tree := pageTree()
	titled := &store.Question{
		ID: "Q-titled", Title: "Canonical schema", Statement: titledStatement, Status: "open",
	}
	legacy := &store.Question{
		ID: "Q-legacy", Statement: legacyStatement, Status: "open",
	}

	titledRow := relatedOf(tree, titled)
	require.Equal(t, "Canonical schema", titledRow.Title)
	require.Equal(t, titledStatement, titledRow.Statement)

	legacyRow := relatedOf(tree, legacy)
	require.Equal(t, strings.Repeat("界", 79)+"…", legacyRow.Title)
	require.Equal(t, legacyStatement, legacyRow.Statement)
}

// TestHistoryOrdersNewestFirstWithIDTiebreak pins the timeline order: later
// timestamps first, and the higher event id first when two share an instant,
// the reverse of the stream's canonical ascending order.
func TestHistoryOrdersNewestFirstWithIDTiebreak(t *testing.T) {
	at := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	evs := []events.Event{
		{ID: "EV-000002", Type: "second", At: at, Targets: []string{"F-111111"}},
		{ID: "EV-000001", Type: "first", At: at, Targets: []string{"F-111111"}},
		{ID: "EV-000003", Type: "third", At: at.Add(time.Hour), Targets: []string{"F-111111"}},
	}
	tree := pageTree()
	page := NodePageOf(tree, evs, nodeOf(t, tree, "F-111111"), nil)

	types := make([]string, len(page.History))
	for i, h := range page.History {
		types[i] = h.Type
	}
	require.Equal(t, []string{"third", "second", "first"}, types)
}

// siblingTree builds a graph whose sibling key order is the exact reverse of id
// order, the shape that catches a projection still sorting tasks by id. One
// epic owns three features, all active. "form-engine/loader" carries three
// in-progress tasks that record a depends_on chain, so their ranked order runs
// T-000004, T-000005, T-000003, which is neither their key order (T-000005,
// T-000004, T-000003) nor their id order (T-000003, T-000004, T-000005);
// "form-engine/parser" carries two in-review tasks that record no edge at all,
// so key order stands there and runs T-000002, T-000001. Both indexes point at
// the same *Task values the way store.Load fills them. The active lens draws
// the loader's tasks alongside the epic and the three features; the gates lens
// draws the parser's two in-review tasks and nothing else, because a PR always
// represents a task and a feature has no review state to be drawn in.
func siblingTree() *store.Tree {
	epicKey := "form-engine"
	loaderKey := "form-engine/loader"
	parserKey := "form-engine/parser"
	auditKey := "form-engine/audit"
	gates := store.GatesBlock{Gates: map[string]store.Gate{
		"tests": {Required: true, Status: store.GateStatusPass},
	}}

	laKey := "form-engine/loader/alfa"
	lbKey := "form-engine/loader/bravo"
	lcKey := "form-engine/loader/charlie"
	paKey := "form-engine/parser/alfa"
	pbKey := "form-engine/parser/bravo"

	la := &store.Task{ID: "T-000005", Key: &laKey, Title: "Load stage 5", Status: "in_progress", Owner: "jc", FeatureID: "F-0000fa",
		DependsOn: []string{"T-000004"}}
	lb := &store.Task{ID: "T-000004", Key: &lbKey, Title: "Load stage 4", Status: "in_progress", Owner: "jc", FeatureID: "F-0000fa"}
	lc := &store.Task{ID: "T-000003", Key: &lcKey, Title: "Load stage 3", Status: "in_progress", Owner: "jc", FeatureID: "F-0000fa",
		DependsOn: []string{"T-000005"}}
	pa := &store.Task{ID: "T-000002", Key: &paKey, Title: "Parse stage 2", Status: "in_review", Owner: "jc", FeatureID: "F-0000fb", GatesBlock: gates}
	pb := &store.Task{ID: "T-000001", Key: &pbKey, Title: "Parse stage 1", Status: "in_review", Owner: "jc", FeatureID: "F-0000fb", GatesBlock: gates}

	return &store.Tree{
		Manifest: &store.Manifest{Contributors: []store.Contributor{{ID: "jc", Name: "JC Grubbs"}}},
		Epics: map[string]*store.Epic{
			"E-000009": {ID: "E-000009", Key: &epicKey, Title: "Form Engine", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-0000fa": {ID: "F-0000fa", Key: &loaderKey, Title: "Loader", Status: "active", EpicKey: "form-engine"},
			"F-0000fb": {ID: "F-0000fb", Key: &parserKey, Title: "Parser", Status: "active", EpicKey: "form-engine", GatesBlock: gates},
			"F-0000fd": {ID: "F-0000fd", Key: &auditKey, Title: "Audit", Status: "active", EpicKey: "form-engine", GatesBlock: gates},
		},
		Tasks: map[string]*store.Task{
			"T-000005": la, "T-000004": lb, "T-000003": lc,
			"T-000002": pa, "T-000001": pb,
		},
		FeatureTasks: map[string][]*store.Task{
			"F-0000fa": {la, lb, lc},
			"F-0000fb": {pa, pb},
		},
		Questions: map[string]*store.Question{},
	}
}

// TestNodePageTasksFollowSiblingOrder pins that a feature page lists its tasks
// in the sibling order depends_on records (artifact-schema.md, Tasks:
// Ordering). The fixture's chain runs against both the key order the store
// holds the group in and the id order an older projection fell back to, so
// either fallback gets this feature visibly wrong.
func TestNodePageTasksFollowSiblingOrder(t *testing.T) {
	tree := siblingTree()
	page := NodePageOf(tree, nil, nodeOf(t, tree, "F-0000fa"), nil)

	ids := make([]string, len(page.Relationships.Tasks))
	for i, tk := range page.Relationships.Tasks {
		ids[i] = tk.ID
	}
	require.Equal(t, []string{"T-000004", "T-000005", "T-000003"}, ids,
		"bravo leads the alfa that depends on it, and charlie follows the alfa it depends on")
}

// TestNodePageTaskCarriesBothSiblingEdges pins that a task page carries the
// data the order above is computed from. The feature page ranks its tasks by
// depends_on (TestNodePageTasksFollowSiblingOrder), so a human who opens one of
// those tasks to ask why it sits where it does has to find the edge there, in
// both directions: the fixture's alfa follows bravo and is followed by charlie.
func TestNodePageTaskCarriesBothSiblingEdges(t *testing.T) {
	tree := siblingTree()
	page := NodePageOf(tree, nil, nodeOf(t, tree, "T-000005"), nil)

	require.Len(t, page.Relationships.DependsOn, 1, "alfa records the sibling it follows")
	require.Equal(t, "T-000004", page.Relationships.DependsOn[0].ID)
	require.Equal(t, "form-engine/loader/bravo (T-000004)", page.Relationships.DependsOn[0].Ref,
		"the edge is a full link row, not a bare id")

	require.Len(t, page.Relationships.DependedOnBy, 1, "and the sibling that follows it")
	require.Equal(t, "T-000003", page.Relationships.DependedOnBy[0].ID)
	require.Equal(t, "Load stage 3", page.Relationships.DependedOnBy[0].Title)

	// The other direction of the same pair, so neither edge is a coincidence of
	// one node's fields.
	bravo := NodePageOf(tree, nil, nodeOf(t, tree, "T-000004"), nil)
	require.Empty(t, bravo.Relationships.DependsOn, "bravo leads the chain")
	require.Len(t, bravo.Relationships.DependedOnBy, 1)
	require.Equal(t, "T-000005", bravo.Relationships.DependedOnBy[0].ID)
}

// taskSupersedesTree lays two tasks under one feature where the second
// supersedes the first (an amendment's supersede or a cancel's
// --superseded-by can both land a task-grain edge, mutate's
// amendment_approve.go), so the edge can be pinned at task grain the same way
// TestNodePageFeatureSupersedes pins it at feature grain. A third, dangling
// id in the supersedes list proves the same degrade-to-bare-id rule applies
// here too.
func taskSupersedesTree() *store.Tree {
	epicKey := "form-engine"
	featKey := "form-engine/loader"
	oldKey := "form-engine/loader/old-approach"
	newKey := "form-engine/loader/new-approach"
	replacement := "T-newtsk"
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-aaa111": {ID: "E-aaa111", Key: &epicKey, Title: "Form Engine", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-111111": {ID: "F-111111", Key: &featKey, Title: "Loader", Status: "active", EpicKey: "form-engine"},
		},
		Tasks: map[string]*store.Task{
			"T-oldtsk": {
				ID: "T-oldtsk", Key: &oldKey, Title: "Old approach", Status: "cancelled",
				FeatureID: "F-111111", SupersededBy: &replacement,
			},
			"T-newtsk": {
				ID: "T-newtsk", Key: &newKey, Title: "New approach", Status: "ready",
				FeatureID: "F-111111", Supersedes: []string{"T-oldtsk", "T-dangle"},
			},
		},
	}
}

// TestNodePageTaskSupersedes pins the deferred item this task closes: a
// task's own supersedes and superseded_by edges, rendered from the successor's
// side (a dangling id degraded to a bare-id row) and from the retired task's
// own side, the same both-directions coverage TestNodePageFeatureSupersedes
// already proves at feature grain.
func TestNodePageTaskSupersedes(t *testing.T) {
	tree := taskSupersedesTree()

	newer := NodePageOf(tree, nil, tree.Tasks["T-newtsk"], nil)
	require.Len(t, newer.Relationships.Supersedes, 2)
	require.Equal(t, "T-dangle", newer.Relationships.Supersedes[0].Ref)
	require.Equal(t, "form-engine/loader/old-approach (T-oldtsk)", newer.Relationships.Supersedes[1].Ref)
	require.Empty(t, newer.Relationships.SupersededBy)

	older := NodePageOf(tree, nil, tree.Tasks["T-oldtsk"], nil)
	require.Empty(t, older.Relationships.Supersedes)
	require.Len(t, older.Relationships.SupersededBy, 1)
	require.Equal(t, "form-engine/loader/new-approach (T-newtsk)", older.Relationships.SupersededBy[0].Ref)
}

// decompositionTree lays a small graph for the epic decomposition tests: one
// epic ("widgets") owning two features whose canonical keys sort the reverse
// of their id order, so a projection still sorting by id would get this
// fixture wrong. Each feature carries its own source, change type, and
// surface, and the second also carries a gate, proving the full node view
// rides along rather than the lightweight relationship row. A second epic
// owns no features, the childless case, and a third carries no key at all,
// the degenerate case that should not exist for a folder-backed noun but must
// still render an empty decomposition rather than omit the block.
func decompositionTree() *store.Tree {
	epicKey := "widgets"
	betaKey := "widgets/beta"
	gammaKey := "widgets/gamma"
	childlessKey := "widgets-childless"
	gates := store.GatesBlock{Gates: map[string]store.Gate{
		"tests": {Required: true, Status: store.GateStatusPass},
	}}
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-aaa111": {ID: "E-aaa111", Key: &epicKey, Title: "Widgets", Status: "ready"},
			"E-bbb222": {ID: "E-bbb222", Key: &childlessKey, Title: "Childless", Status: "ready"},
			"E-ccc333": {ID: "E-ccc333", Title: "Keyless", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-900001": {
				ID: "F-900001", Key: &betaKey, Title: "Beta", Status: "specced",
				ChangeType: "create", EpicKey: "widgets",
				Source:  "design/beta.md",
				Surface: store.Surface{Paths: []string{"internal/widgets/beta.go"}},
			},
			"F-100002": {
				ID: "F-100002", Key: &gammaKey, Title: "Gamma", Status: "active",
				ChangeType: "modify", EpicKey: "widgets",
				Source:     "design/gamma.md",
				Surface:    store.Surface{Paths: []string{"widgets"}},
				GatesBlock: gates,
			},
		},
		Questions: map[string]*store.Question{},
	}
}

// TestNodePageEpicDecompositionProjectsFullFeatureViewsInKeyOrder pins the
// decomposition's two defining rules: it carries the full Node projection
// (source, change type, surface, and gates all riding along, not just the
// lightweight relationship row childFeatures builds), and it orders that list
// by canonical key rather than id. The fixture's id order is F-100002 then
// F-900001, the reverse of its key order, so a sort still keyed on id would
// fail this.
func TestNodePageEpicDecompositionProjectsFullFeatureViewsInKeyOrder(t *testing.T) {
	tree := decompositionTree()
	page := NodePageOf(tree, nil, tree.Epics["E-aaa111"], nil)

	require.NotNil(t, page.Decomposition)
	require.Len(t, page.Decomposition.Features, 2)

	beta := page.Decomposition.Features[0]
	gamma := page.Decomposition.Features[1]

	require.Equal(t, "widgets/beta", beta.Key,
		"key order, not id order: F-900001 sorts after F-100002 by id")
	require.Equal(t, "widgets/gamma", gamma.Key)

	require.Equal(t, "create", beta.ChangeType)
	require.Equal(t, "design/beta.md", beta.Source)
	require.NotNil(t, beta.Surface)
	require.Equal(t, []string{"internal/widgets/beta.go"}, beta.Surface.Paths)
	require.Empty(t, beta.Gates)

	require.Equal(t, "modify", gamma.ChangeType)
	require.Equal(t, "design/gamma.md", gamma.Source)
	require.NotNil(t, gamma.Surface)
	require.Equal(t, []string{"widgets"}, gamma.Surface.Paths)
	require.NotEmpty(t, gamma.Gates, "the gate on the second feature must ride along too")
	require.Equal(t, "pass", gamma.Gates["tests"].Status)

	got, err := json.Marshal(page)
	require.NoError(t, err)
	require.Contains(t, string(got), `"decomposition":{"features":[`)
}

// decompositionDependencyTree lays two features under one epic where key
// order and dependency order disagree: "alpha" depends on "zeta", so key
// order lists alpha first (a < z) but dependency order must list zeta first.
func decompositionDependencyTree() *store.Tree {
	epicKey := "widgets"
	zetaKey := "widgets/zeta"
	alphaKey := "widgets/alpha"
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-aaa111": {ID: "E-aaa111", Key: &epicKey, Title: "Widgets", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-zeta": {ID: "F-zeta", Key: &zetaKey, Title: "Zeta", Status: "draft", EpicKey: "widgets"},
			"F-alpha": {
				ID: "F-alpha", Key: &alphaKey, Title: "Alpha", Status: "draft", EpicKey: "widgets",
				DependsOn: []string{"F-zeta"},
			},
		},
		Questions: map[string]*store.Question{},
	}
}

// TestNodePageEpicDecompositionFollowsDependencyOrderNotKey pins Task A1
// on decompositionOf specifically: it shares the Order
// computation with childFeatures (TestNodePageEpicListsFeaturesByDependencyOrder)
// but is a separate call site, so it earns its own pin.
func TestNodePageEpicDecompositionFollowsDependencyOrderNotKey(t *testing.T) {
	tree := decompositionDependencyTree()
	page := NodePageOf(tree, nil, tree.Epics["E-aaa111"], nil)

	require.Len(t, page.Decomposition.Features, 2)
	require.Equal(t, "widgets/zeta", page.Decomposition.Features[0].Key,
		"zeta (the dependency) leads alpha, though alpha sorts first by key")
	require.Equal(t, "widgets/alpha", page.Decomposition.Features[1].Key)
}

// TestNodePageEpicDecompositionEmptyForChildlessEpic pins that a childless
// epic still carries the decomposition block, with an empty, not null,
// features list: the features field has no omitempty precisely so a renderer
// never has to special case an epic with nothing under it yet.
func TestNodePageEpicDecompositionEmptyForChildlessEpic(t *testing.T) {
	tree := decompositionTree()
	page := NodePageOf(tree, nil, tree.Epics["E-bbb222"], nil)

	require.NotNil(t, page.Decomposition)
	require.Empty(t, page.Decomposition.Features)

	got, err := json.Marshal(page)
	require.NoError(t, err)
	require.Contains(t, string(got), `"decomposition":{"features":[]}`)
}

// TestNodePageEpicDecompositionEmptyForKeylessEpic pins the degenerate case a
// folder-backed epic should never reach: a nil key cannot match any feature's
// EpicKey, childFeatures' own guard refuses to even try, but the
// decomposition block itself must still render, empty, rather than being
// omitted. An epic page always carries the key.
func TestNodePageEpicDecompositionEmptyForKeylessEpic(t *testing.T) {
	tree := decompositionTree()
	page := NodePageOf(tree, nil, tree.Epics["E-ccc333"], nil)

	require.NotNil(t, page.Decomposition)
	require.Empty(t, page.Decomposition.Features)

	got, err := json.Marshal(page)
	require.NoError(t, err)
	require.Contains(t, string(got), `"decomposition":{"features":[]}`)
}

// TestNodePageNonEpicPagesCarryNoDecompositionKey pins the other side of the
// rule: every noun but epic omits the block entirely, key and all, rather
// than serializing an empty or null one.
func TestNodePageNonEpicPagesCarryNoDecompositionKey(t *testing.T) {
	tree := pageTree()
	for _, id := range []string{"F-111111", "T-333333", "Q-555555"} {
		page := NodePageOf(tree, nil, nodeOf(t, tree, id), nil)
		require.Nil(t, page.Decomposition, "noun %s must carry no decomposition", id)

		got, err := json.Marshal(page)
		require.NoError(t, err)
		require.NotContains(t, string(got), `"decomposition"`, "noun %s must carry no decomposition", id)
	}
}

// --- Task 5 (amend build): the amendment node page --------------------------

// amendmentPageTree lays a small graph for the amendment node page tests: one
// epic, one feature, and one task the amendment's operations can target, plus
// the amendment itself staging three operations that between them exercise
// every field AmendmentOperationPageView carries: a revise naming a
// resolvable target and a structural precondition, a supersede naming a
// resolvable target and a staged (not yet materialized) replacement, and an
// add-task naming a brand-new key with no existing target at all.
func amendmentPageTree() *store.Tree {
	epicKey := "form-engine"
	featKey := "form-engine/loader"
	taskKey := "form-engine/loader/old-task"
	amendKey := "iphone-only"
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-aaa111": {ID: "E-aaa111", Key: &epicKey, Title: "Form Engine", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-111111": {ID: "F-111111", Key: &featKey, Title: "Loader", Status: "active", EpicKey: "form-engine"},
		},
		Tasks: map[string]*store.Task{
			"T-222222": {ID: "T-222222", Key: &taskKey, Title: "Old task", Status: "proposed", FeatureID: "F-111111"},
		},
		Amendments: map[string]*store.Amendment{
			"A-333333": {
				ID: "A-333333", Key: &amendKey, Title: "Remove native iPad support",
				Status:       store.AmendmentStatusProposed,
				TriggeredBy:  []string{"docs/design/mobile.md"},
				AmendsDesign: []string{"docs/design/mobile.md"},
				Operations: []store.AmendmentOp{
					{Op: store.AmendmentOpRevise, Target: "F-111111",
						Preconditions: map[string]string{"spec_revision": "git:abc123"}},
					{Op: store.AmendmentOpSupersede, Target: "T-222222", By: "form-engine/loader/new-task"},
					{Op: store.AmendmentOpAddTask, Key: "form-engine/loader/new-task", Title: "New task"},
				},
			},
		},
	}
}

// TestNodePageAmendmentBlock pins the flat amendment page:
// the NodeView carries the amendment's own title (the one field its own
// switch arm in Node() has to fill), the prose ref names the flat
// .tiller/amendments/ narrative path with content left for serve to fill,
// and the amendment block carries every staged operation with its targets
// resolved to a RelatedNode where the live tree can (the revise and the
// supersede's own target) and degraded to a bare-id row where it cannot yet
// (the supersede's By, a staged key no add op in this batch has materialized).
// Preconditions carry only the structural name and expected value; Live stays
// nil, the typed slot the serve handler fills (mirroring Eval and
// ActiveBasis).
func TestNodePageAmendmentBlock(t *testing.T) {
	tree := amendmentPageTree()
	page := NodePageOf(tree, nil, tree.Amendments["A-333333"], nil)

	require.Equal(t, "amendment", page.Node.Noun)
	require.Equal(t, "A-333333", page.Node.ID)
	require.Equal(t, "iphone-only", page.Node.Key)
	require.Equal(t, "Remove native iPad support", page.Node.Title)
	require.Equal(t, "proposed", page.Node.Status)
	require.Equal(t, "iphone-only (A-333333)", page.Ref)

	require.NotNil(t, page.Prose, "an amendment owns a narrative file even though it is not folder-backed")
	require.Equal(t, ".tiller/amendments/A-333333-iphone-only.md", page.Prose.Path)
	require.Empty(t, page.Prose.Content, "views never read files; content is the handler's job")

	require.NotNil(t, page.Amendment)
	require.Equal(t, []string{"docs/design/mobile.md"}, page.Amendment.TriggeredBy)
	require.Equal(t, []string{"docs/design/mobile.md"}, page.Amendment.AmendsDesign)
	require.Len(t, page.Amendment.Operations, 3)

	revise := page.Amendment.Operations[0]
	require.Equal(t, "revise", revise.Op)
	require.NotNil(t, revise.Target)
	require.Equal(t, "form-engine/loader (F-111111)", revise.Target.Ref)
	require.Nil(t, revise.By)
	require.Len(t, revise.Preconditions, 1)
	require.Equal(t, "spec_revision", revise.Preconditions[0].Name)
	require.Equal(t, "git:abc123", revise.Preconditions[0].Expected)
	require.Nil(t, revise.Preconditions[0].Live, "the live read is the serve handler's own fill, not this pure projection's")

	supersede := page.Amendment.Operations[1]
	require.Equal(t, "supersede", supersede.Op)
	require.NotNil(t, supersede.Target)
	require.Equal(t, "form-engine/loader/old-task (T-222222)", supersede.Target.Ref)
	require.NotNil(t, supersede.By)
	require.Equal(t, "form-engine/loader/new-task", supersede.By.Ref,
		"a staged key naming an add op not yet materialized degrades to a bare-id row")

	addTask := page.Amendment.Operations[2]
	require.Equal(t, "add-task", addTask.Op)
	require.Equal(t, "form-engine/loader/new-task", addTask.Key)
	require.Equal(t, "New task", addTask.Title)
	require.Nil(t, addTask.Target, "an add op names no existing target")

	// The relationship hub carries nothing of its own for an amendment: its
	// edges live in the amendment block above, not duplicated here.
	require.Empty(t, page.Relationships.DependsOn)
	require.Empty(t, page.Relationships.Supersedes)
	require.Empty(t, page.Relationships.SupersededBy)

	got, err := json.Marshal(page)
	require.NoError(t, err)
	require.Contains(t, string(got), `"amendment":{`)
}

// TestNodePageAmendmentEmptyOperationsRendersEmptyArray pins the same
// always-an-array discipline DecompositionView.Features already follows: an
// amendment staging nothing yet still renders "operations":[], never null,
// so a renderer parses one shape whether anything is staged or not.
func TestNodePageAmendmentEmptyOperationsRendersEmptyArray(t *testing.T) {
	key := "empty-one"
	tree := &store.Tree{Amendments: map[string]*store.Amendment{
		"A-000001": {ID: "A-000001", Key: &key, Status: store.AmendmentStatusProposed},
	}}
	page := NodePageOf(tree, nil, tree.Amendments["A-000001"], nil)

	require.NotNil(t, page.Amendment)
	require.NotNil(t, page.Amendment.Operations, "operations serializes as [] not null")
	require.Empty(t, page.Amendment.Operations)

	got, err := json.Marshal(page.Amendment)
	require.NoError(t, err)
	require.JSONEq(t, `{"operations":[]}`, string(got))
}

// TestNodePageNonAmendmentPagesCarryNoAmendmentKey mirrors
// TestNodePageNonEpicPagesCarryNoDecompositionKey for the new block: every
// other noun's page omits "amendment" whole rather than serializing it null.
func TestNodePageNonAmendmentPagesCarryNoAmendmentKey(t *testing.T) {
	tree := pageTree()
	for _, id := range []string{"E-aaa111", "F-111111", "T-333333", "Q-555555"} {
		page := NodePageOf(tree, nil, nodeOf(t, tree, id), nil)
		require.Nil(t, page.Amendment, "noun %s must carry no amendment block", id)

		got, err := json.Marshal(page)
		require.NoError(t, err)
		require.NotContains(t, string(got), `"amendment"`, "noun %s must carry no amendment block", id)
	}
}

// --- Task 5 (intake build): the amendment page's own Source -----------------

// TestNodePageAmendmentCarriesSourceStructurally pins the one amendment-level
// addition the page projection carries: an
// intake-minted amendment's own Source, Path and Revision structural, Live
// left nil, the same typed-nil-slot the serve handler fills (mirroring every
// precondition's own Live, AmendmentPreconditionPageView's own doc comment).
func TestNodePageAmendmentCarriesSourceStructurally(t *testing.T) {
	key := "review-intake"
	tree := &store.Tree{Amendments: map[string]*store.Amendment{
		"A-444444": {
			ID: "A-444444", Key: &key, Title: "Intake: review.md",
			Status: store.AmendmentStatusProposed,
			Source: &store.AmendmentSource{Path: ".tiller/feedback/review.md", Revision: "git:abc123"},
		},
	}}
	page := NodePageOf(tree, nil, tree.Amendments["A-444444"], nil)

	require.NotNil(t, page.Amendment)
	require.NotNil(t, page.Amendment.Source, "an intake-minted amendment carries its Source on the page")
	require.Equal(t, ".tiller/feedback/review.md", page.Amendment.Source.Path)
	require.Equal(t, "git:abc123", page.Amendment.Source.Revision)
	require.Nil(t, page.Amendment.Source.Live, "the live read is the serve handler's own fill, not this pure projection's")

	got, err := json.Marshal(page.Amendment)
	require.NoError(t, err)
	require.Contains(t, string(got), `"source":{"path":".tiller/feedback/review.md","revision":"git:abc123"}`)
}

// TestNodePageAmendmentOmitsSourceForAHandAmendment pins the other side: a
// hand-opened amendment (amendmentPageTree's own fixture) carries no Source
// at all, and the page omits the key whole rather than serializing it null,
// the same convention TestNodePageNonAmendmentPagesCarryNoAmendmentKey pins
// one level up.
func TestNodePageAmendmentOmitsSourceForAHandAmendment(t *testing.T) {
	tree := amendmentPageTree()
	page := NodePageOf(tree, nil, tree.Amendments["A-333333"], nil)
	require.NotNil(t, page.Amendment)
	require.Nil(t, page.Amendment.Source, "a hand-opened amendment carries no Source")

	got, err := json.Marshal(page.Amendment)
	require.NoError(t, err)
	require.NotContains(t, string(got), `"source"`)
}

// --- Task 8: op-level Source and Blocks (Task 5's ledger addendum) ---------

// TestNodePageAmendmentOperationCarriesSourceAndBlocks pins the addendum
// Task 5's review assigned to Task 8: an operation's own Source citation and
// an add-question's Blocks both project onto AmendmentOperationPageView, the
// two fields the plan's own file list missed. Blocks resolves through
// relatedByIDs, the identical rule the question node page's own Blocks
// relationship already uses (relationshipsOf's *store.Question arm): an
// existing feature resolves in full, and a staged key naming an add op on
// this same amendment, not yet materialized, degrades to a bare-id row
// rather than disappearing, the same honest-not-hidden convention Target,
// By, and SupersededBy already keep on this page.
func TestNodePageAmendmentOperationCarriesSourceAndBlocks(t *testing.T) {
	epicKey := "form-engine"
	featKey := "form-engine/loader"
	amendKey := "iphone-only"
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-aaa111": {ID: "E-aaa111", Key: &epicKey, Title: "Form Engine", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-111111": {ID: "F-111111", Key: &featKey, Title: "Loader", Status: "active", EpicKey: "form-engine"},
		},
		Amendments: map[string]*store.Amendment{
			"A-999999": {
				ID: "A-999999", Key: &amendKey, Status: store.AmendmentStatusProposed,
				Source: &store.AmendmentSource{Path: ".tiller/feedback/review.md", Revision: "git:abc123"},
				Operations: []store.AmendmentOp{
					{Op: store.AmendmentOpAddFeature, Key: "form-engine/new-thing", Title: "New thing",
						ChangeType: "add", Source: "docs/feedback/opus-review.md#L1-9"},
					{Op: store.AmendmentOpAddQuestion, Title: "Authentication provider",
						Statement: "Which auth provider?",
						Blocks:    []string{"E-aaa111", "F-111111", "form-engine/new-thing"},
						Source:    "docs/feedback/opus-review.md#L20-24"},
				},
			},
		},
	}
	page := NodePageOf(tree, nil, tree.Amendments["A-999999"], nil)
	require.NotNil(t, page.Amendment)
	require.Len(t, page.Amendment.Operations, 2)

	addFeature := page.Amendment.Operations[0]
	require.Equal(t, "docs/feedback/opus-review.md#L1-9", addFeature.Source)
	require.Empty(t, addFeature.Blocks, "an add-feature op names no blocks")
	opGot, err := json.Marshal(addFeature)
	require.NoError(t, err)
	require.NotContains(t, string(opGot), `"blocks"`, "an op with no blocks omits the key rather than serializing null")

	addQuestion := page.Amendment.Operations[1]
	require.Equal(t, "Authentication provider", addQuestion.Title)
	require.Equal(t, "Which auth provider?", addQuestion.Statement)
	require.Equal(t, "docs/feedback/opus-review.md#L20-24", addQuestion.Source)
	require.Len(t, addQuestion.Blocks, 3)
	require.Equal(t, "form-engine (E-aaa111)", addQuestion.Blocks[0].Ref,
		"an existing epic resolves in full, the same as the question NODE page's own blocks relationship")
	require.Equal(t, "form-engine/loader (F-111111)", addQuestion.Blocks[1].Ref,
		"an existing feature resolves in full")
	require.Equal(t, "form-engine/new-thing", addQuestion.Blocks[2].Ref,
		"a staged key naming an add op on this same amendment, not yet materialized, degrades to a bare-id row")

	got, err := json.Marshal(page.Amendment)
	require.NoError(t, err)
	require.Contains(t, string(got), `"source":"docs/feedback/opus-review.md#L1-9"`)
	require.Contains(t, string(got), `"blocks":[`)
	require.Contains(t, string(got), `"title":"Authentication provider"`)
	require.Contains(t, string(got), `"statement":"Which auth provider?"`)
}

// --- #57: the amendment page's own source_missing / shape_error pre-flight -

// TestNodePageAmendmentOperationCarriesSourceMissingAndShapeError pins the
// console-side half of #57: the same two doc-only hand-edit defect classes
// `show amendment` already flags (mutate.EvaluateAmendmentOpSources,
// mutate.EvaluateAmendmentShape) now render on the amendment NODE page too.
// Both evaluators take no tree and never fail (their own doc comments,
// amendment_validity.go), so this pure projection computes them directly
// from the amendment already in hand, unconditionally, the identical
// composition runShowAmendment already keeps independent of the
// (tree-needing, error-prone) precondition evaluator (cli/amendment_cmds.go).
// One batch exercises all three shapes at once: a clean first add-epic, a
// second add-epic repeating its key (duplicate_staged_key), and an add-feature
// whose Source is whitespace only, the identical trim
// mutate.EvaluateAmendmentOpSources itself runs before calling it blank.
func TestNodePageAmendmentOperationCarriesSourceMissingAndShapeError(t *testing.T) {
	amendKey := "hardening"
	tree := &store.Tree{
		Amendments: map[string]*store.Amendment{
			"A-555555": {
				ID: "A-555555", Key: &amendKey, Status: store.AmendmentStatusProposed,
				Source: &store.AmendmentSource{Path: ".tiller/feedback/review.md", Revision: "git:abc123"},
				Operations: []store.AmendmentOp{
					{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "One", Source: "docs/feedback/review.md#L1"},
					{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Two", Source: "docs/feedback/review.md#L2"},
					{Op: store.AmendmentOpAddFeature, Key: "hardening/x", Title: "X", ChangeType: "create", Source: "   "},
				},
			},
		},
	}
	page := NodePageOf(tree, nil, tree.Amendments["A-555555"], nil)
	require.NotNil(t, page.Amendment)
	require.Len(t, page.Amendment.Operations, 3)

	op0, op1, op2 := page.Amendment.Operations[0], page.Amendment.Operations[1], page.Amendment.Operations[2]
	require.False(t, op0.SourceMissing)
	require.Empty(t, op0.ShapeError, "the first operation to stage the key is clean")
	require.False(t, op1.SourceMissing)
	require.Equal(t, "duplicate_staged_key", op1.ShapeError, "the second operation to stage the same key is flagged")
	require.True(t, op2.SourceMissing, "a whitespace-only source trims to blank, the identical rule EvaluateAmendmentOpSources runs")
	require.Empty(t, op2.ShapeError, "an add-feature is not a shape defect either predicate covers")

	got, err := json.Marshal(page.Amendment)
	require.NoError(t, err)
	require.Contains(t, string(got), `"shape_error":"duplicate_staged_key"`)
	require.Contains(t, string(got), `"source_missing":true`)
}

// TestNodePageAmendmentCleanIntakeBatchOmitsValidityFlags is the other side:
// an intake-minted amendment whose every op carries a real Source and no
// shape defect renders SourceMissing false and ShapeError blank for every
// operation, and the JSON omits source_missing, shape_error, and
// validity_error entirely rather than serializing a false or empty value,
// the same omitempty convention every other flag on this page keeps.
// ValidityError stays empty here too: it is node.go's fillAmendmentValidity
// that ever fills it, never this pure projection (amendmentPageOf's own doc
// comment).
func TestNodePageAmendmentCleanIntakeBatchOmitsValidityFlags(t *testing.T) {
	amendKey := "iphone-only"
	tree := &store.Tree{
		Amendments: map[string]*store.Amendment{
			"A-666666": {
				ID: "A-666666", Key: &amendKey, Status: store.AmendmentStatusProposed,
				Source: &store.AmendmentSource{Path: ".tiller/feedback/review.md", Revision: "git:abc123"},
				Operations: []store.AmendmentOp{
					{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening", Source: "docs/feedback/review.md#L1"},
					{Op: store.AmendmentOpAddFeature, Key: "hardening/x", Title: "X", ChangeType: "create", Source: "docs/feedback/review.md#L2"},
				},
			},
		},
	}
	page := NodePageOf(tree, nil, tree.Amendments["A-666666"], nil)
	require.NotNil(t, page.Amendment)
	for _, op := range page.Amendment.Operations {
		require.False(t, op.SourceMissing, "every op carries a real source")
		require.Empty(t, op.ShapeError, "no duplicate staged key and no staged-epic replacement here")
	}
	require.Empty(t, page.Amendment.ValidityError)

	got, err := json.Marshal(page.Amendment)
	require.NoError(t, err)
	require.NotContains(t, string(got), `"source_missing"`)
	require.NotContains(t, string(got), `"shape_error"`)
	require.NotContains(t, string(got), `"validity_error"`)
}

// --- Task 12: DriftBadgesOf and the Refusing rule --------------------------
//
// ui-surface.md's node-page chip sentence is the whole contract under test:
// the chip "takes the blocked palette register only where the condition
// actually refuses verbs, active_basis anywhere and an unreviewed answered
// assumption at a submittable task, and the neutral register the stale gates
// use otherwise". DriftBadgesOf and driftRefusing are exercised directly
// here (fast, in-process) rather than only through serve's slower HTTP/git
// fixtures, since this truth table is the single riskiest decision the whole
// task adds.

func candidate(marker, noun, id string) drift.Candidate {
	return drift.Candidate{Marker: marker, Noun: noun, ID: id}
}

func TestDriftBadgesOfActiveBasisAlwaysRefuses(t *testing.T) {
	feat := &store.Feature{ID: "F-1", Status: "active"}
	badges := DriftBadgesOf([]drift.Candidate{candidate(drift.MarkerActiveBasis, store.NounFeature, "F-1")}, feat)
	require.Equal(t, []DriftBadgeView{{Marker: drift.MarkerActiveBasis, Refusing: true}}, badges)

	task := &store.Task{ID: "T-1", Status: "done"}
	badges = DriftBadgesOf([]drift.Candidate{candidate(drift.MarkerActiveBasis, store.NounFeature, "F-1")}, task)
	require.Equal(t, []DriftBadgeView{{Marker: drift.MarkerActiveBasis, Refusing: true}}, badges,
		"active_basis refuses regardless of the PAGE node's own status: it reads 'anywhere'")
}

func TestDriftBadgesOfAnsweredQuestionRefusesOnlyAtASubmittableTask(t *testing.T) {
	cs := []drift.Candidate{candidate(drift.MarkerAnsweredQuestion, store.NounFeature, "F-1")}

	for _, status := range []string{"in_progress", "check"} {
		task := &store.Task{ID: "T-1", Status: status}
		badges := DriftBadgesOf(cs, task)
		require.Equal(t, []DriftBadgeView{{Marker: drift.MarkerAnsweredQuestion, Refusing: true}}, badges,
			"status %s is the submittable window", status)
	}

	for _, status := range []string{"ready", "in_review", "done", "cancelled"} {
		task := &store.Task{ID: "T-1", Status: status}
		badges := DriftBadgesOf(cs, task)
		require.Equal(t, []DriftBadgeView{{Marker: drift.MarkerAnsweredQuestion, Refusing: false}}, badges,
			"status %s is outside the submittable window (in_review already shipped its claim; resume is the way back)", status)
	}

	feat := &store.Feature{ID: "F-1", Status: "active"}
	badges := DriftBadgesOf(cs, feat)
	require.Equal(t, []DriftBadgeView{{Marker: drift.MarkerAnsweredQuestion, Refusing: false}}, badges,
		"the rule names a TASK specifically; a feature page never refuses on this marker")
}

func TestDriftBadgesOfEveryOtherMarkerNeverRefuses(t *testing.T) {
	task := &store.Task{ID: "T-1", Status: "in_progress"} // the one status that DOES refuse for answered_question
	for _, marker := range []string{drift.MarkerStaleSpec, drift.MarkerMissingSurface, drift.MarkerOrphanEval} {
		badges := DriftBadgesOf([]drift.Candidate{candidate(marker, store.NounFeature, "F-1")}, task)
		require.Equal(t, []DriftBadgeView{{Marker: marker, Refusing: false}}, badges, "marker %s never refuses", marker)
	}
}

// TestDriftBadgesOfDedupesToOneBadgePerMarker pins one chip
// regardless of how many markers touch the node, which starts from ONE
// badge per marker even when a single ForNode call produced more than one
// candidate for it (a task whose active_basis drifted at both its own grain
// and its parent feature's, TestActiveBasisNamesTaskOwnDriftBesideParentDrift's
// two-candidate shape in the drift package).
func TestDriftBadgesOfDedupesToOneBadgePerMarker(t *testing.T) {
	task := &store.Task{ID: "T-1", Status: "ready"}
	cs := []drift.Candidate{
		candidate(drift.MarkerActiveBasis, store.NounFeature, "F-1"),
		candidate(drift.MarkerActiveBasis, store.NounTask, "T-1"),
	}
	badges := DriftBadgesOf(cs, task)
	require.Equal(t, []DriftBadgeView{{Marker: drift.MarkerActiveBasis, Refusing: true}}, badges)
}

// TestDriftBadgesOfOrdersBySchemaNotArrival pins the title attribute's own
// need: badges render in drift.KnownMarkers' fixed schema order, never
// the incidental order candidates happened to arrive in.
func TestDriftBadgesOfOrdersBySchemaNotArrival(t *testing.T) {
	feat := &store.Feature{ID: "F-1", Status: "done"}
	cs := []drift.Candidate{
		candidate(drift.MarkerOrphanEval, store.NounFeature, "F-1"),
		candidate(drift.MarkerStaleSpec, store.NounFeature, "F-1"),
		candidate(drift.MarkerActiveBasis, store.NounFeature, "F-1"),
	}
	badges := DriftBadgesOf(cs, feat)
	require.Equal(t, []DriftBadgeView{
		{Marker: drift.MarkerStaleSpec, Refusing: false},
		{Marker: drift.MarkerActiveBasis, Refusing: true},
		{Marker: drift.MarkerOrphanEval, Refusing: false},
	}, badges)
}

func TestDriftBadgesOfEmptyCandidatesYieldsNoBadges(t *testing.T) {
	feat := &store.Feature{ID: "F-1", Status: "active"}
	require.Empty(t, DriftBadgesOf(nil, feat))
}

func TestRelatedNodeCarriesShelfTitle(t *testing.T) {
	bugKey := "bugs/blank-on-reload"
	choreKey := "chores/bump-go"
	bug := &store.Bug{WorkItem: store.WorkItem{
		ID: "B-aaaaaa", Key: &bugKey, Title: "Blank on reload", Status: "ready",
	}}
	chore := &store.Chore{WorkItem: store.WorkItem{
		ID: "CH-bbbbbb", Key: &choreKey, Title: "Bump Go", Status: "ready",
	}}
	tree := &store.Tree{
		Bugs:   map[string]*store.Bug{bug.ID: bug},
		Chores: map[string]*store.Chore{chore.ID: chore},
	}

	require.Equal(t, "Blank on reload", relatedOf(tree, bug).Title)
	require.Equal(t, "Bump Go", relatedOf(tree, chore).Title)
}

func TestTreeNodePageCarriesRolledUpAttachedShelfWork(t *testing.T) {
	tree := nodePageShelfTree()

	epic := NodePageOf(tree, nil, tree.Epics["E-foundation"], nil)
	require.Equal(t, []string{"B-child", "CH-direct"}, relatedIDs(epic.Relationships.AttachedWork))

	feature := NodePageOf(tree, nil, tree.Features["F-console"], nil)
	require.Equal(t, []string{"B-child", "CH-direct"}, relatedIDs(feature.Relationships.AttachedWork))

	task := NodePageOf(tree, nil, tree.Tasks["T-render"], nil)
	require.Equal(t, []string{"B-child"}, relatedIDs(task.Relationships.AttachedWork))
}

func TestWorkItemNodePageCarriesShelfRelationships(t *testing.T) {
	tree := nodePageShelfTree()
	tree.Bugs["B-dependent"].AttachedTo = "T-render"

	page := NodePageOf(tree, nil, tree.Bugs["B-dependent"], nil)

	require.Equal(t, "T-render", page.Relationships.AttachedTo.ID)
	require.Equal(t, "F-console", page.Relationships.ResolvedBy.ID)
	require.Equal(t, "B-child", page.Relationships.DuplicateOf.ID)
	require.Equal(t, []string{"B-child"}, relatedIDs(page.Relationships.DependsOn))
	require.Equal(t, []string{"B-follower"}, relatedIDs(page.Relationships.DependedOnBy))
	require.Nil(t, page.Relationships.AttachedWork)

	child := NodePageOf(tree, nil, tree.Bugs["B-child"], nil)
	require.Equal(t, []string{"B-dependent", "CH-direct"}, relatedIDs(child.Relationships.DependedOnBy))

	dangling := NodePageOf(tree, nil, tree.Bugs["B-dangling"], nil)
	require.Equal(t, &RelatedNode{ID: "T-missing", Ref: "T-missing"}, dangling.Relationships.AttachedTo)
}

func nodePageShelfTree() *store.Tree {
	epicKey := "foundation"
	featureKey := "foundation/console"
	taskKey := "foundation/console/render"
	childKey := "bugs/child"
	dependentKey := "bugs/dependent"
	followerKey := "bugs/follower"
	danglingKey := "bugs/dangling"
	choreKey := "chores/direct"
	task := &store.Task{
		ID: "T-render", Key: &taskKey, Title: "Render the console", Status: "ready", FeatureID: "F-console",
	}
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-foundation": {ID: "E-foundation", Key: &epicKey, Title: "Foundation", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-console": {
				ID: "F-console", Key: &featureKey, Title: "Console", Status: "ready", EpicKey: epicKey,
			},
		},
		Tasks: map[string]*store.Task{"T-render": task},
		FeatureTasks: map[string][]*store.Task{
			"F-console": {task},
		},
		Bugs: map[string]*store.Bug{
			"B-child": {
				WorkItem: store.WorkItem{
					ID: "B-child", Key: &childKey, Title: "Child bug", Status: "ready", AttachedTo: "T-render",
				},
			},
			"B-dependent": {
				WorkItem: store.WorkItem{
					ID: "B-dependent", Key: &dependentKey, Title: "Dependent bug", Status: "ready",
					ResolvedBy: "F-console", DuplicateOf: "B-child", DependsOn: []string{"B-child"},
				},
			},
			"B-follower": {
				WorkItem: store.WorkItem{
					ID: "B-follower", Key: &followerKey, Title: "Follower bug", Status: "ready",
					DependsOn: []string{"B-dependent"},
				},
			},
			"B-dangling": {
				WorkItem: store.WorkItem{
					ID: "B-dangling", Key: &danglingKey, Title: "Dangling bug", Status: "ready",
					AttachedTo: "T-missing",
				},
			},
		},
		Chores: map[string]*store.Chore{
			"CH-direct": {
				WorkItem: store.WorkItem{
					ID: "CH-direct", Key: &choreKey, Title: "Direct chore", Status: "ready",
					AttachedTo: "F-console", DependsOn: []string{"B-child"},
				},
			},
		},
	}
}

func relatedIDs(nodes []RelatedNode) []string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.ID
	}
	return ids
}

// A correct-eval operation reaches the console page with its reason and the
// criterion ids it retires, so a reviewer sees what the approval removes.
func TestNodePageAmendmentCorrectEvalCarriesRetire(t *testing.T) {
	tree := amendmentPageTree()
	tree.Amendments["A-333333"].Operations = []store.AmendmentOp{
		{Op: store.AmendmentOpCorrectEval, Target: "F-111111", Reason: "Loader tests split into two files.", Retire: []string{"C-2"}},
	}
	page := NodePageOf(tree, nil, tree.Amendments["A-333333"], nil)

	require.NotNil(t, page.Amendment)
	require.Len(t, page.Amendment.Operations, 1)
	op := page.Amendment.Operations[0]
	require.Equal(t, store.AmendmentOpCorrectEval, op.Op)
	require.NotNil(t, op.Target)
	require.Equal(t, "F-111111", op.Target.ID)
	require.Equal(t, "Loader tests split into two files.", op.Reason)
	require.Equal(t, []string{"C-2"}, op.Retire)

	got, err := json.Marshal(op)
	require.NoError(t, err)
	require.Contains(t, string(got), `"retire":["C-2"]`)
}
