package views_test

// Task 8 of the Review Grain build (docs/plans/2026-08-26-review-grain-design.md,
// "Surfaces": the console's own quiet badge). These tests pin the node header
// payload's two new additive fields: review_grain, the RESOLVED grain in force
// for work under an epic or a feature (store.ResolvedReviewGrain), and
// review_grain_source, naming which of the three homes (docs/plans/2026-08-26-
// review-grain-design.md, "The setting": feature, epic, project) it actually
// came from, or "default" when none of the three is set and the value is the
// hardcoded task fallback. Both are present for an epic and a feature, the
// two review-grain homes the design names besides the project default; a
// task, a question, and an amendment carry neither, since none of them owns
// work of its own to resolve a grain for.

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

func TestNodeProjectsReviewGrainAndSource(t *testing.T) {
	epicKey := "form-engine"
	featureKey := "form-engine/nrca-form-loading"

	treeWithEpic := func(e *store.Epic) *store.Tree {
		return &store.Tree{Epics: map[string]*store.Epic{e.ID: e}}
	}

	cases := []struct {
		name       string
		tree       *store.Tree
		node       store.Node
		wantGrain  string
		wantSource string
	}{
		{
			name:       "epic with its own explicit grain",
			tree:       &store.Tree{},
			node:       &store.Epic{ID: "E-aaaaaa", Key: &epicKey, Status: "ready", ReviewGrain: "epic"},
			wantGrain:  "epic",
			wantSource: "epic",
		},
		{
			name:       "epic inherits the project default",
			tree:       &store.Tree{Manifest: &store.Manifest{Execution: store.ExecutionBlock{ReviewGrain: "feature"}}},
			node:       &store.Epic{ID: "E-bbbbbb", Status: "ready"},
			wantGrain:  "feature",
			wantSource: "project",
		},
		{
			name:       "epic with nothing set resolves task by default",
			tree:       &store.Tree{},
			node:       &store.Epic{ID: "E-cccccc", Status: "ready"},
			wantGrain:  "task",
			wantSource: "default",
		},
		{
			name: "feature's own explicit grain beats its epic's",
			tree: treeWithEpic(&store.Epic{
				ID: "E-dddddd", Key: &epicKey, Status: "ready", ReviewGrain: "epic",
			}),
			node: &store.Feature{
				ID: "F-aaaaaa", Key: &featureKey, Status: "ready", EpicKey: epicKey, ReviewGrain: "feature",
			},
			wantGrain:  "feature",
			wantSource: "feature",
		},
		{
			// Fix round 1, Important 2 (the reviewer's named ambiguous case,
			// verified by hand, previously unpinned by any test): an explicit
			// feature-level override back to "task" under an epic-grain epic
			// resolves grain "task" - the same value the hardcoded default
			// would also produce - but the source must still read "feature",
			// the explicit setting that actually won, never "default".
			name: "feature's own explicit task grain beats its epic's, and sources as feature, not default",
			tree: treeWithEpic(&store.Epic{
				ID: "E-gggggg", Key: &epicKey, Status: "ready", ReviewGrain: "epic",
			}),
			node: &store.Feature{
				ID: "F-eeeeee", Key: &featureKey, Status: "ready", EpicKey: epicKey, ReviewGrain: "task",
			},
			wantGrain:  "task",
			wantSource: "feature",
		},
		{
			name: "feature inherits its epic's explicit grain",
			tree: treeWithEpic(&store.Epic{
				ID: "E-eeeeee", Key: &epicKey, Status: "ready", ReviewGrain: "epic",
			}),
			node:       &store.Feature{ID: "F-bbbbbb", Key: &featureKey, Status: "ready", EpicKey: epicKey},
			wantGrain:  "epic",
			wantSource: "epic",
		},
		{
			name: "feature inherits the project default over an unset epic",
			tree: func() *store.Tree {
				tr := treeWithEpic(&store.Epic{ID: "E-ffffff", Key: &epicKey, Status: "ready"})
				tr.Manifest = &store.Manifest{Execution: store.ExecutionBlock{ReviewGrain: "feature"}}
				return tr
			}(),
			node:       &store.Feature{ID: "F-cccccc", Key: &featureKey, Status: "ready", EpicKey: epicKey},
			wantGrain:  "feature",
			wantSource: "project",
		},
		{
			name:       "feature with a dangling epic key and no manifest resolves task by default",
			tree:       &store.Tree{},
			node:       &store.Feature{ID: "F-dddddd", Key: &featureKey, Status: "ready", EpicKey: epicKey},
			wantGrain:  "task",
			wantSource: "default",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := views.Node(tc.tree, tc.node)
			require.Equal(t, tc.wantGrain, v.ReviewGrain)
			require.Equal(t, tc.wantSource, v.ReviewGrainSource)
			out, err := json.Marshal(v)
			require.NoError(t, err)
			require.Contains(t, string(out), `"review_grain":"`+tc.wantGrain+`"`)
			require.Contains(t, string(out), `"review_grain_source":"`+tc.wantSource+`"`)
		})
	}
}

// TestNodeOmitsReviewGrainForNounsWithNoWorkUnderThem pins the negative case
// the table above never exercises: a task, a question, and an amendment all
// resolve no grain of their own (a task's is inherited context for its
// feature's page to explain, not a fact the task's own page states about
// itself), so both fields stay empty, and so absent from the JSON envelope,
// exactly like every other additive field this package leaves silent rather
// than emitting a zero value.
func TestNodeOmitsReviewGrainForNounsWithNoWorkUnderThem(t *testing.T) {
	tree := &store.Tree{}
	nodes := []store.Node{
		&store.Task{ID: "T-aaaaaa", Status: "ready", FeatureID: "F-missing"},
		&store.Question{ID: "Q-aaaaaa", Status: "open"},
		&store.Amendment{ID: "AM-aaaaaa", Status: store.AmendmentStatusProposed},
	}
	for _, n := range nodes {
		v := views.Node(tree, n)
		require.Empty(t, v.ReviewGrain, "%s should carry no review_grain", n.Noun())
		require.Empty(t, v.ReviewGrainSource, "%s should carry no review_grain_source", n.Noun())
		out, err := json.Marshal(v)
		require.NoError(t, err)
		require.NotContains(t, string(out), `"review_grain"`)
	}
}

// TestNodeProjectsPRAndReviewRefAtCoarserGrain pins the softened schema rule
// (docs/plans/2026-08-26-review-grain-design.md, Schema and the PR record: "a
// PR represents exactly one node, the task at task grain, the feature or
// epic at coarser grain"): an epic or a feature carries its OWN recorded pr
// or review_ref when it is itself the reviewed node, the identical fields a
// task has always carried, reused rather than a second shape.
func TestNodeProjectsPRAndReviewRefAtCoarserGrain(t *testing.T) {
	pr := 101
	epicKey := "form-engine"
	featureKey := "form-engine/nrca-form-loading"

	t.Run("epic-grain epic carries its own recorded PR", func(t *testing.T) {
		e := &store.Epic{ID: "E-aaaaaa", Key: &epicKey, Status: "in_review", ReviewGrain: "epic", PR: &pr}
		v := views.Node(&store.Tree{}, e)
		require.NotNil(t, v.PR)
		require.Equal(t, 101, *v.PR)
		require.Nil(t, v.ReviewRef)
		out, err := json.Marshal(v)
		require.NoError(t, err)
		require.Contains(t, string(out), `"pr":101`)
	})

	t.Run("feature-grain feature carries its own recorded review_ref", func(t *testing.T) {
		f := &store.Feature{
			ID: "F-aaaaaa", Key: &featureKey, Status: "in_review", ReviewGrain: "feature",
			ReviewRef: &store.ReviewRef{
				Branch: "tiller/form-engine/nrca-form-loading",
				Head:   store.RevisionRef{Kind: store.KindGitCommit, Value: "git:e5f9d28"},
			},
		}
		v := views.Node(&store.Tree{}, f)
		require.NotNil(t, v.ReviewRef)
		require.Equal(t, "tiller/form-engine/nrca-form-loading", v.ReviewRef.Branch)
		require.Equal(t, "git:e5f9d28", v.ReviewRef.Head)
		require.Nil(t, v.PR)
		out, err := json.Marshal(v)
		require.NoError(t, err)
		require.Contains(t, string(out),
			`"review_ref":{"branch":"tiller/form-engine/nrca-form-loading","head":"git:e5f9d28"}`)
	})
}
