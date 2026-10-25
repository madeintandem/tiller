package views_test

// TestNodeStatusCarriesHeldByRetiredAncestor pins task-3-brief.md's status
// oracle bullet (issue #36): NodeStatusView.Held carries gen.RetiredAncestor's
// answer verbatim, the same additive fact NodeView.Held carries for `show`,
// present only when there is one.

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func TestNodeStatusCarriesHeldByRetiredAncestor(t *testing.T) {
	featKey := "widgets/gadget"
	feat := &store.Feature{ID: "F-aaaaaa", Key: &featKey, Status: "cancelled"}
	task := &store.Task{ID: "T-aaaaaa", FeatureID: feat.ID, Status: "in_progress"}
	tree := &store.Tree{
		Features: map[string]*store.Feature{feat.ID: feat},
		Tasks:    map[string]*store.Task{task.ID: task},
	}

	v := views.NodeStatus(tree, task, workflow.Input{})
	require.NotNil(t, v.Held)
	require.Equal(t, "feature", v.Held.Noun)
	require.Equal(t, "cancelled", v.Held.State)

	raw, err := json.Marshal(v)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"held":{"noun":"feature"`)

	// A live task under a live feature carries no held object at all.
	liveFeat := &store.Feature{ID: "F-bbbbbb", Status: "active"}
	liveTask := &store.Task{ID: "T-bbbbbb", FeatureID: liveFeat.ID, Status: "in_progress"}
	liveTree := &store.Tree{
		Features: map[string]*store.Feature{liveFeat.ID: liveFeat},
		Tasks:    map[string]*store.Task{liveTask.ID: liveTask},
	}
	lv := views.NodeStatus(liveTree, liveTask, workflow.Input{})
	require.Nil(t, lv.Held)
	lraw, err := json.Marshal(lv)
	require.NoError(t, err)
	require.NotContains(t, string(lraw), `"held"`)
}

func TestNodeStatusCarriesBidirectionalQuestionRelationships(t *testing.T) {
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
	answer := "Use the route table."
	questions := map[string]*store.Question{
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
		"Q-500-answered": {
			ID: "Q-500-answered", Title: "Choose the settled route",
			Statement: "Which route was already settled?",
			Status:    "answered", Answer: &answer, Blocks: []string{feature.ID},
		},
		"Q-600-empty": {
			ID: "Q-600-empty", Title: "Unrelated question",
			Statement: "Does this question hold nothing?", Status: "open",
		},
	}
	tree := &store.Tree{
		Epics:     map[string]*store.Epic{epic.ID: epic},
		Features:  map[string]*store.Feature{feature.ID: feature},
		Tasks:     map[string]*store.Task{task.ID: task},
		Questions: questions,
	}
	questionRow := func(id string) views.RelatedNode {
		q := questions[id]
		return views.RelatedNode{
			Noun: store.NounQuestion, ID: q.ID, Ref: q.ID, Status: q.Status,
			Title: q.Title, Statement: q.Statement,
		}
	}

	epicStatus := views.NodeStatus(tree, epic, workflow.Input{})
	require.NotNil(t, epicStatus.Relationships)
	require.Equal(t, []views.RelatedNode{
		questionRow("Q-100-epic"),
		questionRow("Q-400-multiple"),
	}, epicStatus.Relationships.BlockedByQuestions)

	featureStatus := views.NodeStatus(tree, feature, workflow.Input{})
	require.NotNil(t, featureStatus.Relationships)
	require.Equal(t, []views.RelatedNode{
		questionRow("Q-100-epic"),
		questionRow("Q-200-feature"),
		questionRow("Q-400-multiple"),
	}, featureStatus.Relationships.BlockedByQuestions,
		"feature status inherits the epic hold, dedupes the multi-grain hold, and excludes answered and assumed questions")
	featureRaw, err := json.Marshal(featureStatus)
	require.NoError(t, err)
	require.Contains(t, string(featureRaw), `"relationships":{"blocked_by_questions":[`,
		"relationships are additive structured status data, not CLI-only projection")

	taskStatus := views.NodeStatus(tree, task, workflow.Input{})
	require.NotNil(t, taskStatus.Relationships)
	require.Equal(t, []views.RelatedNode{
		questionRow("Q-100-epic"),
		questionRow("Q-200-feature"),
		questionRow("Q-300-assumed"),
		questionRow("Q-400-multiple"),
	}, taskStatus.Relationships.BlockedByQuestions,
		"a feature assumption clears only that feature's own blocker view, not its task's inherited hold")

	multipleStatus := views.NodeStatus(tree, questions["Q-400-multiple"], workflow.Input{})
	require.NotNil(t, multipleStatus.Relationships)
	require.Equal(t, []views.RelatedNode{
		{
			Noun: store.NounTask, ID: task.ID, Key: taskKey,
			Ref: taskKey + " (" + task.ID + ")", Status: task.Status, Title: task.Title,
		},
		{
			Noun: store.NounFeature, ID: feature.ID, Key: featureKey,
			Ref: featureKey + " (" + feature.ID + ")", Status: feature.Status, Title: feature.Title,
		},
		{
			Noun: store.NounEpic, ID: epic.ID, Key: epicKey,
			Ref: epicKey + " (" + epic.ID + ")", Status: epic.Status, Title: epic.Title,
		},
	}, multipleStatus.Relationships.Blocks,
		"question status preserves its durable forward targets in recorded order with composed refs")
	require.Equal(t, questions["Q-400-multiple"].Statement, multipleStatus.Statement)

	answeredStatus := views.NodeStatus(tree, questions["Q-500-answered"], workflow.Input{})
	require.NotNil(t, answeredStatus.Relationships)
	require.Equal(t, []views.RelatedNode{{
		Noun: store.NounFeature, ID: feature.ID, Key: featureKey,
		Ref: featureKey + " (" + feature.ID + ")", Status: feature.Status, Title: feature.Title,
	}}, answeredStatus.Relationships.Blocks,
		"answering removes the operative reverse hold without erasing the question's durable Blocks")

	emptyStatus := views.NodeStatus(tree, questions["Q-600-empty"], workflow.Input{})
	require.Nil(t, emptyStatus.Relationships)
	raw, err := json.Marshal(emptyStatus)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"relationships"`)
}

// TestSummaryCarriesRecordedRootOrderAndItsRefs pins Task 8: the project
// summary carries the recorded root soft order (store.ProjectOrder.Epics,
// .tiller/order.yml) the same carriage-only, machine-ids-plus-human-refs way
// NodeView.Order/OrderRefs already do, and the same wire posture ActiveEpics/
// ActiveEpicRefs already established here: Order rides the JSON envelope,
// OrderRefs stays CLI-only (json:"-"), because machine output carries ids
// structurally and never the composed human label (ActiveEpicRefs' own doc
// comment). A project that never ran `tiller order epic` carries neither.
func TestSummaryCarriesRecordedRootOrderAndItsRefs(t *testing.T) {
	paymentsKey := "payments"
	formEngineKey := "form-engine"
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Epics: map[string]*store.Epic{
			"E-payments":    {ID: "E-payments", Key: &paymentsKey, Status: "draft"},
			"E-form-engine": {ID: "E-form-engine", Key: &formEngineKey, Status: "draft"},
		},
		ProjectOrder: store.ProjectOrder{Epics: []string{"E-payments", "E-form-engine"}},
	}

	s := views.Summary(tree)
	require.Equal(t, []string{"E-payments", "E-form-engine"}, s.Order)
	require.Equal(t, []string{"payments (E-payments)", "form-engine (E-form-engine)"}, s.OrderRefs)

	raw, err := json.Marshal(s)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"order":["E-payments","E-form-engine"]`)
	require.NotContains(t, string(raw), `"order_refs"`, "the human refs stay CLI-only, like active_epic_refs")

	// A project with no recorded root order carries neither key.
	fresh := views.Summary(&store.Tree{Manifest: &store.Manifest{}})
	require.Nil(t, fresh.Order)
	require.Nil(t, fresh.OrderRefs)
	fraw, err := json.Marshal(fresh)
	require.NoError(t, err)
	require.NotContains(t, string(fraw), `"order":`)
}
