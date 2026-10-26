package views_test

// These tests pin issue #23 on the surviving parentage filters:
// ResolveParentFilters rewrites the caller's raw --epic or --feature value to
// the stable identity Matches compares against, so a caller naming a parent by
// its canonical key finds the same rows as naming it by id rather than
// silently matching nothing.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// epicFilterTree lays a minimal tree: one epic with one feature under it, so
// an --epic filter has exactly one row to find, whichever way the caller names
// the epic.
func epicFilterTree() *store.Tree {
	epicKey := "form-engine"
	featKey := "form-engine/schema-engine"
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-000001": {ID: "E-000001", Key: &epicKey, Title: "Form Engine", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-000001": {
				ID: "F-000001", Key: &featKey, Title: "Schema engine", Status: "draft",
				EpicKey: "form-engine",
			},
		},
	}
}

// featureIDsByEpic resolves f through ResolveParentFilters and runs the same
// Collect-then-Matches loop the CLI's ls and serve's ListHandler both run,
// returning the matching feature ids.
func featureIDsByEpic(t *testing.T, tree *store.Tree, epicRaw string) []string {
	t.Helper()
	f := views.ListFilters{Epic: epicRaw}
	require.NoError(t, views.ResolveParentFilters(tree, &f))
	var ids []string
	for _, n := range views.Collect(tree, store.NounFeature) {
		if views.Matches(tree, n, f) {
			ids = append(ids, n.NodeID())
		}
	}
	return ids
}

// TestResolveParentFiltersEpicKeyEqualsID pins the fix: an --epic filter named
// by the epic's canonical key lists the same rows as one named by its id.
func TestResolveParentFiltersEpicKeyEqualsID(t *testing.T) {
	tree := epicFilterTree()

	byKey := featureIDsByEpic(t, tree, "form-engine")
	byID := featureIDsByEpic(t, tree, "E-000001")

	require.Equal(t, []string{"F-000001"}, byKey, "the key filter should find the epic's feature")
	require.Equal(t, byID, byKey, "a key filter and an id filter must find the same rows")
}

// TestResolveParentFiltersEpicNotFound confirms an unresolved --epic value
// returns the resolver's typed not-found error rather than silently matching
// nothing.
func TestResolveParentFiltersEpicNotFound(t *testing.T) {
	tree := epicFilterTree()
	f := views.ListFilters{Epic: "no-such"}

	err := views.ResolveParentFilters(tree, &f)

	var nf *resolve.ErrNotFound
	require.ErrorAs(t, err, &nf)
}

// siblingListTree lays two features whose task keys reverse id order, and
// records a depends_on chain across the first feature's three tasks so its
// ranked order (bravo, alfa, charlie) is neither their key order (alfa, bravo,
// charlie) nor their id order (charlie, bravo, alfa). The parser's two tasks
// record nothing, so key order decides there, and they carry the lowest ids
// under a later feature key, so they must still sort last. Both indexes hold
// the same *Task values, the way store.Load fills them.
func siblingListTree() *store.Tree {
	loaderKey := "form-engine/loader"
	parserKey := "form-engine/parser"
	laKey := "form-engine/loader/alfa"
	lbKey := "form-engine/loader/bravo"
	lcKey := "form-engine/loader/charlie"
	paKey := "form-engine/parser/alfa"
	pbKey := "form-engine/parser/bravo"
	la := &store.Task{ID: "T-000005", Key: &laKey, Title: "Load 5", FeatureID: "F-0000fa", DependsOn: []string{"T-000004"}}
	lb := &store.Task{ID: "T-000004", Key: &lbKey, Title: "Load 4", FeatureID: "F-0000fa"}
	lc := &store.Task{ID: "T-000003", Key: &lcKey, Title: "Load 3", FeatureID: "F-0000fa", DependsOn: []string{"T-000005"}}
	pa := &store.Task{ID: "T-000002", Key: &paKey, Title: "Parse 2", FeatureID: "F-0000fb"}
	pb := &store.Task{ID: "T-000001", Key: &pbKey, Title: "Parse 1", FeatureID: "F-0000fb"}
	return &store.Tree{
		Features: map[string]*store.Feature{
			"F-0000fa": {ID: "F-0000fa", Key: &loaderKey, Title: "Loader", EpicKey: "form-engine"},
			"F-0000fb": {ID: "F-0000fb", Key: &parserKey, Title: "Parser", EpicKey: "form-engine"},
		},
		Tasks: map[string]*store.Task{
			"T-000005": la, "T-000004": lb, "T-000003": lc,
			"T-000002": pa, "T-000001": pb,
		},
		FeatureTasks: map[string][]*store.Task{
			"F-0000fa": {la, lb, lc},
			"F-0000fb": {pa, pb},
		},
	}
}

// epicOrderTree lays two epics where key order and dependency order disagree:
// "alpha" depends on "zeta", so key order lists alpha first (a < z) but
// dependency order must list zeta, the dependency, first.
func epicOrderTree() *store.Tree {
	zetaKey := "zeta"
	alphaKey := "alpha"
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-zeta":  {ID: "E-zeta", Key: &zetaKey, Title: "Zeta", Status: "ready"},
			"E-alpha": {ID: "E-alpha", Key: &alphaKey, Title: "Alpha", Status: "ready", DependsOn: []string{"E-zeta"}},
		},
	}
}

// TestSortListItemsEpicsFollowDependencyOrderNotKey pins Task A1 (issue #27)
// on `ls epic`'s row order: DependencyOrder, not plain key ascending.
func TestSortListItemsEpicsFollowDependencyOrderNotKey(t *testing.T) {
	tree := epicOrderTree()
	items := []views.ListItem{}
	for _, n := range views.Collect(tree, store.NounEpic) {
		items = append(items, views.Item(tree, n))
	}
	views.SortListItems(tree, store.NounEpic, items)

	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	require.Equal(t, []string{"E-zeta", "E-alpha"}, ids,
		"zeta (the dependency) leads alpha, though alpha's key sorts first alphabetically")
}

// featureCheckListTree lays one active feature whose one task is done and
// whose manifest requires the evals gate at feature grain but records none,
// the awaiting-check shape gen.FeatureActivity fires on (mirrors
// internal/gen/activity_test.go's featureTree).
func featureCheckListTree() (*store.Tree, *store.Feature, *store.Task) {
	key := "form-engine/schema-engine"
	f := &store.Feature{ID: "F-000001", Key: &key, Status: "active", EpicKey: "form-engine"}
	tk := &store.Task{ID: "T-000001", FeatureID: f.ID, Status: "done"}
	tree := &store.Tree{
		Manifest: &store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
			"evals": {Kind: "command", On: store.StringList{"feature"}},
		}}},
		Features: map[string]*store.Feature{f.ID: f},
		Tasks:    map[string]*store.Task{tk.ID: tk},
	}
	return tree, f, tk
}

// TestItemCarriesFeatureActivity pins Task 8: a list row's Activity field
// carries the feature's derived rollup phase (gen.FeatureActivity), the same
// answer the status oracle gives, so the console's list rows can show the
// phase without a second fetch. A task carries no phase, so its row's
// Activity stays empty.
func TestItemCarriesFeatureActivity(t *testing.T) {
	tree, f, tk := featureCheckListTree()

	fItem := views.Item(tree, f)
	require.Equal(t, "awaiting_check", fItem.Activity)

	tItem := views.Item(tree, tk)
	require.Equal(t, "", tItem.Activity)
}

// TestSortListItemsTasksFollowDependencyThenKeyOrder pins `ls task`'s row
// order (artifact-schema.md, Tasks: Ordering): the loader's tasks lead the
// parser's because their feature does, within the loader the recorded chain
// decides, and within the parser, which records nothing, key order does. Both
// a plain key sort and a plain id sort disagree with this sequence.
func TestSortListItemsTasksFollowDependencyThenKeyOrder(t *testing.T) {
	tree := siblingListTree()
	items := []views.ListItem{}
	for _, n := range views.Collect(tree, store.NounTask) {
		items = append(items, views.Item(tree, n))
	}
	views.SortListItems(tree, store.NounTask, items)

	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	require.Equal(t, []string{"T-000004", "T-000005", "T-000003", "T-000002", "T-000001"}, ids,
		"the loader's chain (bravo, alfa, charlie) then the parser's edge-free pair in key order")
}

// questionListTree lays one task a question can block, an answered question
// naming that task plus a dangling target, and an open question that carries
// only a due hint, the exact shapes the three additive
// question-row fields (answer, due, blocks_refs) need to prove: an answered
// question with an answer and a resolvable blocking edge, an open question
// with a due date and no answer, and a blocking edge that does not resolve.
func questionListTree() *store.Tree {
	epicKey := "form-engine"
	featKey := "form-engine/schema-engine"
	taskKey := "form-engine/schema-engine/parse-the-schema"
	answer := "JSON Schema"
	tk := &store.Task{ID: "T-000001", Key: &taskKey, Title: "Parse the schema", FeatureID: "F-000001"}
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-000001": {ID: "E-000001", Key: &epicKey, Title: "Form Engine", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-000001": {ID: "F-000001", Key: &featKey, Title: "Schema engine", EpicKey: "form-engine"},
		},
		Tasks:        map[string]*store.Task{"T-000001": tk},
		FeatureTasks: map[string][]*store.Task{"F-000001": {tk}},
		Questions: map[string]*store.Question{
			"Q-000001": {ID: "Q-000001", Title: "Schema dialect", Statement: "Which schema dialect?", Status: "answered",
				Owner: "jc", Answer: &answer, Blocks: []string{"T-000001", "Q-dangling"}},
			"Q-000002": {ID: "Q-000002", Title: "Decision timing", Statement: "When do we decide?", Status: "open",
				Owner: "jc", Due: store.QuestionDue{Date: "2026-09-01"}},
		},
	}
}

// TestItemAnsweredQuestionCarriesAnswerAndBlocksRefs pins the answered arm:
// Answer carries the recorded text, BlocksRefs resolves the real task edge to
// a full link row (RelatedNode, the exact reuse relatedByIDs already applies
// on the node page), and the dangling second edge degrades to a bare-id row
// rather than disappearing (nodepage.go's own relatedByIDs doc comment:
// "nothing hidden, least of all a broken edge").
func TestItemAnsweredQuestionCarriesAnswerAndBlocksRefs(t *testing.T) {
	tree := questionListTree()
	item := views.Item(tree, tree.Questions["Q-000001"])

	require.NotNil(t, item.Answer)
	require.Equal(t, "JSON Schema", *item.Answer)
	require.Empty(t, item.Due, "this question recorded no due hint")

	require.Len(t, item.BlocksRefs, 2)

	resolved := item.BlocksRefs[0]
	require.Equal(t, "task", resolved.Noun)
	require.Equal(t, "T-000001", resolved.ID)
	require.Equal(t, gen.Ref(tree, tree.Tasks["T-000001"]), resolved.Ref)
	require.Equal(t, "Parse the schema", resolved.Title)

	dangling := item.BlocksRefs[1]
	require.Equal(t, "Q-dangling", dangling.ID)
	require.Equal(t, "Q-dangling", dangling.Ref, "a dangling target's ref is its bare id")
	require.Empty(t, dangling.Noun, "a dangling target resolves to no noun")
}

// TestItemOpenQuestionCarriesDueAndNoAnswer pins the open arm: Due carries
// gen.QuestionDue's date-or-hint answer, Answer stays nil (never an empty
// string) until answered, and a question naming no blocks carries an empty,
// omitted BlocksRefs.
func TestItemOpenQuestionCarriesDueAndNoAnswer(t *testing.T) {
	tree := questionListTree()
	item := views.Item(tree, tree.Questions["Q-000002"])

	require.Nil(t, item.Answer, "an open question carries no answer")
	require.Equal(t, "2026-09-01", item.Due)
	require.Empty(t, item.BlocksRefs, "this question names no blocking edges")
}

func TestItemQuestionUsesPersistedAndBoundedLegacyTitles(t *testing.T) {
	tree := questionListTree()
	titledStatement := strings.Repeat("This is the full titled question statement. ", 6)
	legacyStatement := strings.Repeat("界", 81)
	tree.Questions["Q-titled"] = &store.Question{
		ID: "Q-titled", Title: "Canonical schema", Statement: titledStatement, Status: "open",
	}
	tree.Questions["Q-legacy"] = &store.Question{
		ID: "Q-legacy", Statement: legacyStatement, Status: "open",
	}

	titled := views.Item(tree, tree.Questions["Q-titled"])
	require.Equal(t, "Canonical schema", titled.Title)
	require.Equal(t, titledStatement, titled.Statement)

	legacy := views.Item(tree, tree.Questions["Q-legacy"])
	require.Equal(t, strings.Repeat("界", 79)+"…", legacy.Title)
	require.Equal(t, legacyStatement, legacy.Statement)
}

// TestItemQuestionFieldsAreOmittedFromOtherNouns pins the envelope rule
// (additive, omitempty): a task row, which carries none
// of the three question-only fields, marshals with none of their keys
// present, so an existing caller's payload is byte-identical to before this
// task.
func TestItemQuestionFieldsAreOmittedFromOtherNouns(t *testing.T) {
	tree := questionListTree()
	item := views.Item(tree, tree.Tasks["T-000001"])

	raw, err := json.Marshal(item)
	require.NoError(t, err)
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &obj))

	require.NotContains(t, obj, "answer")
	require.NotContains(t, obj, "due")
	require.NotContains(t, obj, "blocks_refs")
}

// TestItemAnsweredQuestionJSONShape pins the wire shape for the answered
// question row byte for byte: answer, due (absent), and blocks_refs with its
// resolved and dangling rows both present.
func TestItemAnsweredQuestionJSONShape(t *testing.T) {
	tree := questionListTree()
	item := views.Item(tree, tree.Questions["Q-000001"])

	raw, err := json.Marshal(item)
	require.NoError(t, err)
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &obj))

	require.JSONEq(t, `"JSON Schema"`, string(obj["answer"]))
	require.NotContains(t, obj, "due", "this question recorded no due hint")
	require.Contains(t, obj, "blocks_refs")

	var refs []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(obj["blocks_refs"], &refs))
	require.Len(t, refs, 2)
	require.JSONEq(t, `"task"`, string(refs[0]["noun"]))
	require.JSONEq(t, `"T-000001"`, string(refs[0]["id"]))
	require.JSONEq(t, `""`, string(refs[1]["noun"]), "a dangling row's noun is the RelatedNode zero value, not omitted")
	require.JSONEq(t, `"Q-dangling"`, string(refs[1]["id"]))
}

// TestCollectAndSortShelfNouns pins Collect's shelf-noun dispatch and
// SortListItems' shelf ordering (gen.OrderWorkNodes over the recorded soft
// order), plus Item's carriage of attached_to for a shelf row.
func TestCollectAndSortShelfNouns(t *testing.T) {
	ka, kb := "bugs/alpha", "bugs/bravo"
	tree := &store.Tree{
		Epics: map[string]*store.Epic{}, Features: map[string]*store.Feature{}, Tasks: map[string]*store.Task{},
		Questions: map[string]*store.Question{}, Amendments: map[string]*store.Amendment{},
		Bugs: map[string]*store.Bug{
			"B-aaaaaa": {WorkItem: store.WorkItem{ID: "B-aaaaaa", Key: &ka, Title: "Alpha", Status: "ready", AttachedTo: "F-79dh3q"}},
			"B-bbbbbb": {WorkItem: store.WorkItem{ID: "B-bbbbbb", Key: &kb, Title: "Bravo", Status: "ready"}},
		},
		Chores:   map[string]*store.Chore{},
		BugOrder: store.ShelfOrder{Order: []string{"B-bbbbbb"}},
	}
	nodes := views.Collect(tree, store.NounBug)
	require.Len(t, nodes, 2)
	items := []views.ListItem{views.Item(tree, nodes[0]), views.Item(tree, nodes[1])}
	views.SortListItems(tree, store.NounBug, items)
	require.Equal(t, "B-bbbbbb", items[0].ID)
	require.Equal(t, "Alpha", items[1].Title)
	require.Equal(t, "F-79dh3q", items[1].AttachedTo)
}

func TestListSeparatesContributorsFromUnfinishedOwners(t *testing.T) {
	tree := epicFilterTree()
	tree.Tasks = map[string]*store.Task{
		"T-history": {ID: "T-history", FeatureID: "F-000001", Status: "cancelled", Owner: "past", Started: true},
		"T-ready00": {ID: "T-ready00", FeatureID: "F-000001", Status: "ready", Owner: "current"},
		"T-check00": {ID: "T-check00", FeatureID: "F-000001", Status: "check", Owner: "current", Started: true},
	}
	tree.ProjectContributors()
	feature := tree.Features["F-000001"]
	require.True(t, views.Matches(tree, feature, views.ListFilters{Contributor: "past"}))
	require.False(t, views.Matches(tree, feature, views.ListFilters{Owner: "past"}))
	require.False(t, views.Matches(tree, feature, views.ListFilters{Status: "unfinished"}))
	require.False(t, views.Matches(tree, tree.Tasks["T-history"], views.ListFilters{Owner: "past", Status: "unfinished"}))
	require.True(t, views.Matches(tree, tree.Tasks["T-ready00"], views.ListFilters{Owner: "current", Status: "unfinished"}))
	require.True(t, views.Matches(tree, tree.Tasks["T-check00"], views.ListFilters{Owner: "current", Status: "unfinished"}))
	require.False(t, views.Matches(tree, tree.Tasks["T-ready00"], views.ListFilters{Contributor: "current"}))

	empty := epicFilterTree()
	for _, projection := range []any{
		views.Node(empty, empty.Features["F-000001"]),
		views.Item(empty, empty.Features["F-000001"]),
	} {
		raw, err := json.Marshal(projection)
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &fields))
		require.JSONEq(t, `[]`, string(fields["contributors"]))
		require.NotContains(t, fields, "owner")
	}
}

func TestItemCancelReason(t *testing.T) {
	tree := questionListTree()
	tree.Questions["Q-cancel"] = &store.Question{ID: "Q-cancel", Statement: "Gone?", Status: "cancelled", CancelReason: "folded into the plan"}

	item := views.Item(tree, tree.Questions["Q-cancel"])
	require.NotNil(t, item.CancelReason)
	require.Equal(t, "folded into the plan", *item.CancelReason)

	open := views.Item(tree, tree.Questions["Q-000002"])
	require.Nil(t, open.CancelReason, "an open question carries no cancel reason")
}
