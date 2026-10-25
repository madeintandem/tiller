package gen

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func questionBlockerFixture() (*store.Tree, *store.Epic, *store.Feature, *store.Task) {
	epicKey := "routing"
	featureKey := "routing/decision"
	taskKey := "routing/decision/walk"
	epic := &store.Epic{ID: "E-route", Key: &epicKey, Status: "ready"}
	feature := &store.Feature{
		ID: "F-decision", Key: &featureKey, Status: "active", EpicKey: epicKey,
	}
	task := &store.Task{
		ID: "T-walk", Key: &taskKey, Status: "ready", FeatureID: feature.ID,
	}
	tree := &store.Tree{
		Epics:        map[string]*store.Epic{epic.ID: epic},
		Features:     map[string]*store.Feature{feature.ID: feature},
		Tasks:        map[string]*store.Task{task.ID: task},
		FeatureTasks: map[string][]*store.Task{feature.ID: {task}},
		Questions:    map[string]*store.Question{},
	}
	return tree, epic, feature, task
}

func addBlockingQuestion(tree *store.Tree, id, status string, blocks ...string) {
	tree.Questions[id] = &store.Question{
		ID:        id,
		Title:     "Choose routing semantics",
		Statement: "Which detailed routing semantics should the implementation use?",
		Status:    status,
		Blocks:    blocks,
	}
}

func TestEpicQuestionBlockersMatchCanonicalIDAndLegacyKey(t *testing.T) {
	for _, target := range []string{"E-route", "routing"} {
		t.Run(target, func(t *testing.T) {
			tree, epic, _, _ := questionBlockerFixture()
			addBlockingQuestion(tree, "Q-epic", "open", target)

			blockers := EpicQuestionBlockers(tree, epic)
			require.Equal(t, []QuestionBlocker{{
				Question: "Q-epic", Title: "Choose routing semantics",
				Statement: "Which detailed routing semantics should the implementation use?",
				Blocks:    []string{target}, TargetID: epic.ID, TargetNoun: store.NounEpic,
			}}, blockers)
		})
	}
}

func TestFeatureQuestionBlockersMatchDirectAndEpicTargetsByIDAndLegacyKey(t *testing.T) {
	cases := []struct {
		name       string
		block      string
		targetID   string
		targetNoun string
	}{
		{name: "feature id", block: "F-decision", targetID: "F-decision", targetNoun: store.NounFeature},
		{name: "feature legacy key", block: "routing/decision", targetID: "F-decision", targetNoun: store.NounFeature},
		{name: "epic id", block: "E-route", targetID: "E-route", targetNoun: store.NounEpic},
		{name: "epic legacy key", block: "routing", targetID: "E-route", targetNoun: store.NounEpic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, _, feature, _ := questionBlockerFixture()
			addBlockingQuestion(tree, "Q-target", "open", tc.block)

			blockers := FeatureQuestionBlockers(tree, feature)
			require.Len(t, blockers, 1)
			require.Equal(t, tc.targetID, blockers[0].TargetID)
			require.Equal(t, tc.targetNoun, blockers[0].TargetNoun)
			require.Equal(t, []string{tc.block}, blockers[0].Blocks)
		})
	}
}

func TestTaskQuestionBlockersMatchDirectAndAncestorTargetsByIDAndLegacyKey(t *testing.T) {
	cases := []struct {
		name       string
		block      string
		targetID   string
		targetNoun string
	}{
		{name: "task id", block: "T-walk", targetID: "T-walk", targetNoun: store.NounTask},
		{name: "task legacy key", block: "routing/decision/walk", targetID: "T-walk", targetNoun: store.NounTask},
		{name: "feature id", block: "F-decision", targetID: "F-decision", targetNoun: store.NounFeature},
		{name: "feature legacy key", block: "routing/decision", targetID: "F-decision", targetNoun: store.NounFeature},
		{name: "epic id", block: "E-route", targetID: "E-route", targetNoun: store.NounEpic},
		{name: "epic legacy key", block: "routing", targetID: "E-route", targetNoun: store.NounEpic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, _, feature, task := questionBlockerFixture()
			addBlockingQuestion(tree, "Q-target", "open", tc.block)

			blockers := TaskQuestionBlockers(tree, feature, task)
			require.Len(t, blockers, 1)
			require.Equal(t, tc.targetID, blockers[0].TargetID)
			require.Equal(t, tc.targetNoun, blockers[0].TargetNoun)
			require.Equal(t, []string{tc.block}, blockers[0].Blocks)
		})
	}
}

func TestTaskQuestionBlockersCountOneQuestionOnceAndPreferTheNearestTarget(t *testing.T) {
	tree, _, feature, task := questionBlockerFixture()
	addBlockingQuestion(tree, "Q-many-levels", "open", "routing", feature.ID, "routing/decision/walk", task.ID)

	blockers := TaskQuestionBlockers(tree, feature, task)
	require.Len(t, blockers, 1)
	require.Equal(t, "Q-many-levels", blockers[0].Question)
	require.Equal(t, task.ID, blockers[0].TargetID)
	require.Equal(t, store.NounTask, blockers[0].TargetNoun)
	require.Equal(t, []string{"routing", feature.ID, "routing/decision/walk", task.ID}, blockers[0].Blocks)
}

func TestTaskQuestionBlockersPreferFeatureOverEpicForOneMultiLevelQuestion(t *testing.T) {
	tree, _, feature, task := questionBlockerFixture()
	addBlockingQuestion(tree, "Q-feature-and-epic", "open", "routing", feature.ID)

	blockers := TaskQuestionBlockers(tree, feature, task)
	require.Len(t, blockers, 1)
	require.Equal(t, feature.ID, blockers[0].TargetID)
	require.Equal(t, store.NounFeature, blockers[0].TargetNoun)
}

func TestQuestionBlockersUseQuestionOpenForLiveness(t *testing.T) {
	tree, epic, _, _ := questionBlockerFixture()
	addBlockingQuestion(tree, "Q-open", "open", epic.ID)
	addBlockingQuestion(tree, "Q-empty-status", "", epic.ID)
	addBlockingQuestion(tree, "Q-answered", "answered", epic.ID)
	addBlockingQuestion(tree, "Q-resolved", "resolved", epic.ID)
	addBlockingQuestion(tree, "Q-cancelled", "cancelled", epic.ID)

	blockers := EpicQuestionBlockers(tree, epic)
	require.Len(t, blockers, 2)
	require.Equal(t, []string{"Q-empty-status", "Q-open"}, []string{blockers[0].Question, blockers[1].Question})
}

func TestFeatureQuestionBlockersHonorAssumptionBasisButTaskBlockersDoNot(t *testing.T) {
	tree, _, feature, task := questionBlockerFixture()
	addBlockingQuestion(tree, "Q-assumed", "open", feature.ID)
	feature.AssumptionBasis = store.AssumptionBasis{"Q-assumed": store.Assumption{}}

	require.Empty(t, FeatureQuestionBlockers(tree, feature))
	taskBlockers := TaskQuestionBlockers(tree, feature, task)
	require.Len(t, taskBlockers, 1)
	require.Equal(t, "Q-assumed", taskBlockers[0].Question)
	require.Equal(t, feature.ID, taskBlockers[0].TargetID)
	require.Equal(t, store.NounFeature, taskBlockers[0].TargetNoun)
}

func TestTaskQuestionBlockersSortByQuestionID(t *testing.T) {
	tree, _, feature, task := questionBlockerFixture()
	addBlockingQuestion(tree, "Q-zebra", "open", task.ID)
	addBlockingQuestion(tree, "Q-alpha", "open", feature.ID)
	addBlockingQuestion(tree, "Q-middle", "open", "routing")

	blockers := TaskQuestionBlockers(tree, feature, task)
	require.Len(t, blockers, 3)
	require.Equal(t, []string{"Q-alpha", "Q-middle", "Q-zebra"}, []string{
		blockers[0].Question, blockers[1].Question, blockers[2].Question,
	})
}

func TestPlanQuestionBlockers(t *testing.T) {
	ids := func(blockers []QuestionBlocker) []string {
		out := make([]string, len(blockers))
		for i, b := range blockers {
			out[i] = b.Question
		}
		return out
	}
	addTask := func(tree *store.Tree, feature *store.Feature, id, status string) *store.Task {
		key := "routing/decision/" + id
		task := &store.Task{ID: id, Key: &key, Status: status, FeatureID: feature.ID}
		tree.Tasks[id] = task
		tree.FeatureTasks[feature.ID] = append(tree.FeatureTasks[feature.ID], task)
		return task
	}

	t.Run("no questions", func(t *testing.T) {
		tree, _, feature, _ := questionBlockerFixture()
		require.Nil(t, PlanQuestionBlockers(tree, feature))
	})
	t.Run("nil tree or feature", func(t *testing.T) {
		tree, _, feature, _ := questionBlockerFixture()
		require.Nil(t, PlanQuestionBlockers(nil, feature))
		require.Nil(t, PlanQuestionBlockers(tree, nil))
	})
	t.Run("question on the feature", func(t *testing.T) {
		tree, _, feature, _ := questionBlockerFixture()
		addBlockingQuestion(tree, "Q-feature", "open", feature.ID)
		blockers := PlanQuestionBlockers(tree, feature)
		require.Equal(t, []string{"Q-feature"}, ids(blockers))
		require.Equal(t, store.NounFeature, blockers[0].TargetNoun)
	})
	t.Run("question on the epic", func(t *testing.T) {
		tree, epic, feature, _ := questionBlockerFixture()
		addBlockingQuestion(tree, "Q-epic", "open", epic.ID)
		blockers := PlanQuestionBlockers(tree, feature)
		require.Equal(t, []string{"Q-epic"}, ids(blockers))
		require.Equal(t, store.NounEpic, blockers[0].TargetNoun)
	})
	t.Run("question on one task", func(t *testing.T) {
		tree, _, feature, task := questionBlockerFixture()
		addBlockingQuestion(tree, "Q-task", "open", task.ID)
		blockers := PlanQuestionBlockers(tree, feature)
		require.Equal(t, []string{"Q-task"}, ids(blockers))
		require.Equal(t, task.ID, blockers[0].TargetID)
	})
	t.Run("question on two tasks appears once", func(t *testing.T) {
		tree, _, feature, task := questionBlockerFixture()
		second := addTask(tree, feature, "T-second", "proposed")
		addBlockingQuestion(tree, "Q-both", "open", task.ID, second.ID)
		require.Equal(t, []string{"Q-both"}, ids(PlanQuestionBlockers(tree, feature)))
	})
	t.Run("feature question is not repeated through its tasks", func(t *testing.T) {
		tree, _, feature, _ := questionBlockerFixture()
		addTask(tree, feature, "T-second", "proposed")
		addBlockingQuestion(tree, "Q-feature", "open", feature.ID)
		require.Equal(t, []string{"Q-feature"}, ids(PlanQuestionBlockers(tree, feature)))
	})
	t.Run("first-seen order puts feature questions before task questions", func(t *testing.T) {
		tree, _, feature, task := questionBlockerFixture()
		addBlockingQuestion(tree, "Q-aaa-task", "open", task.ID)
		addBlockingQuestion(tree, "Q-zzz-feature", "open", feature.ID)
		require.Equal(t, []string{"Q-zzz-feature", "Q-aaa-task"}, ids(PlanQuestionBlockers(tree, feature)))
	})
	t.Run("question on a cancelled task holds nothing", func(t *testing.T) {
		tree, _, feature, task := questionBlockerFixture()
		task.Status = "cancelled"
		addBlockingQuestion(tree, "Q-gone", "open", task.ID)
		require.Empty(t, PlanQuestionBlockers(tree, feature))
	})
	t.Run("answered question holds nothing", func(t *testing.T) {
		tree, _, feature, task := questionBlockerFixture()
		addBlockingQuestion(tree, "Q-done", "answered", task.ID)
		require.Empty(t, PlanQuestionBlockers(tree, feature))
	})
}
