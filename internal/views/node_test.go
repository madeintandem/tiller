package views_test

// These tests pin Task 6 of the serve read-model correction batch
// (docs/spec/cli-surface.md): NodeView projects a node's source provenance, its
// work surface, and whichever review target the store recorded. The store
// already carries these fields (Task 1 put Source on epic and feature); this
// pins that views.Node actually copies them through, and stays silent when a
// node carries none of them.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// TestNodeProjectsSourceSurfaceAndReviewTargets table-drives views.Node over
// the additive fields Task 6 adds: an epic's source and surface, a feature's
// mutually exclusive review targets (PR vs. ReviewRef), a task's ReviewRef,
// and a bare draft feature that carries none of it. Each case checks both the
// struct fields and the marshaled JSON, so a key that should stay silent is
// asserted absent from the payload, not just zero on the struct.
func TestNodeProjectsSourceSurfaceAndReviewTargets(t *testing.T) {
	epicKey := "form-engine"
	prFeatureKey := "form-engine/certification-form-loading"
	draftFeatureKey := "form-engine/draft-only"
	pr := 42

	cases := []struct {
		name  string
		node  store.Node
		check func(t *testing.T, v views.NodeView, raw string)
	}{
		{
			name: "epic carries source and a surface",
			node: &store.Epic{
				ID: "E-aaaaaa", Key: &epicKey, Status: "ready",
				Source: "design/04-form-engine.md",
				Surface: store.Surface{
					Paths: []string{"internal/formengine"},
				},
			},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.Equal(t, "design/04-form-engine.md", v.Source)
				require.NotNil(t, v.Surface)
				require.Equal(t, []string{"internal/formengine"}, v.Surface.Paths)
				require.Contains(t, raw, `"source":"design/04-form-engine.md"`)
				require.Contains(t, raw, `"surface":{"paths":["internal/formengine"]}`)
			},
		},
		{
			// This pins that Surface materializes off Paths alone, independent
			// of the node's other fields: case 1 above pairs it with Source,
			// this one sets nothing else.
			name: "epic surface materializes from paths alone",
			node: &store.Epic{
				ID: "E-bbbbbb", Status: "ready",
				Surface: store.Surface{Paths: []string{"formengine"}},
			},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.NotNil(t, v.Surface)
				require.Equal(t, []string{"formengine"}, v.Surface.Paths)
				require.Contains(t, raw, `"surface":{"paths":["formengine"]}`)
			},
		},
		{
			name: "active feature carries source, surface, and branch but no review target",
			node: &store.Feature{
				ID: "F-aaaaaa", Key: &prFeatureKey, Status: "active",
				Source:  "design/04-form-engine.md",
				Surface: store.Surface{Paths: []string{"internal/formengine"}},
				Branch:  "tiller/form-engine/certification-form-loading",
			},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.Equal(t, "design/04-form-engine.md", v.Source)
				require.NotNil(t, v.Surface)
				require.Equal(t, "tiller/form-engine/certification-form-loading", v.Branch)
				// A PR represents exactly one node, the reviewed grain's own
				// (docs/spec/cli-surface.md): this fixture sets no ReviewGrain
				// and records no PR or ReviewRef of its own, so it projects
				// neither, and its Branch stays the task-grain workbench rather
				// than a review surface. node_review_grain_test.go pins the
				// coarser-grain case where a feature DOES carry its own
				// PR/ReviewRef.
				require.Nil(t, v.PR)
				require.Nil(t, v.ReviewRef)
				require.NotContains(t, raw, `"pr":`)
				require.NotContains(t, raw, `"review_ref"`)
			},
		},
		{
			name: "task with PR set carries no review_ref",
			node: &store.Task{
				ID: "T-bbbbbb", Status: "in_review", FeatureID: "F-aaaaaa", PR: &pr,
			},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.NotNil(t, v.PR)
				require.Equal(t, 42, *v.PR)
				require.Nil(t, v.ReviewRef)
				require.NotContains(t, raw, `"review_ref"`)
			},
		},
		{
			name: "task with ReviewRef set carries no pr, head renders the compact token",
			node: &store.Task{
				ID: "T-aaaaaa", Status: "in_review", FeatureID: "F-aaaaaa",
				ReviewRef: &store.ReviewRef{
					Branch: "tiller/form-engine/certification-form-loading",
					Head:   store.RevisionRef{Kind: store.KindGitCommit, Value: "git:e5f9d28"},
				},
			},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.NotNil(t, v.ReviewRef)
				require.Equal(t, "tiller/form-engine/certification-form-loading", v.ReviewRef.Branch)
				require.Equal(t, "git:e5f9d28", v.ReviewRef.Head)
				require.Contains(t, raw,
					`"review_ref":{"branch":"tiller/form-engine/certification-form-loading","head":"git:e5f9d28"}`)
			},
		},
		{
			name: "bare draft feature omits every new field",
			node: &store.Feature{ID: "F-cccccc", Key: &draftFeatureKey, Status: "draft"},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.Empty(t, v.Source)
				require.Nil(t, v.Surface)
				require.Nil(t, v.ReviewRef)
				require.Empty(t, v.PRURL)
				require.NotContains(t, raw, `"source"`)
				require.NotContains(t, raw, `"surface"`)
				require.NotContains(t, raw, `"review_ref"`)
				require.NotContains(t, raw, `"pr_url"`)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := views.Node(&store.Tree{}, tc.node)
			out, err := json.Marshal(v)
			require.NoError(t, err)
			tc.check(t, v, string(out))
		})
	}
}

// TestNodeProjectsBothDirectionsOfASupersession pins the read surface behind
// the schema's own promise about a reshape: "Both leave the tree answering what
// replaced this structurally" (artifact-schema.md, Evolving a project; the same
// sentence store.Task.Supersedes quotes). An applied amendment writes both legs
// onto the two nodes, and until this the CLI's `show` payload carried neither,
// so the only surfaces that could answer were the console's node page and the
// amendment's own render. Both legs are bare ids, the shape depends_on already
// uses: the envelope carries ids structurally and a reader resolves them.
func TestNodeProjectsBothDirectionsOfASupersession(t *testing.T) {
	retiredKey := "care/reminders/local-notifications"
	replacementKey := "care/reminders/push-notifications"
	retiredFeatureKey := "care/local-reminders"
	replacement := "T-bbbbbb"
	replacementFeature := "F-bbbbbb"

	cases := []struct {
		name  string
		node  store.Node
		check func(t *testing.T, v views.NodeView, raw string)
	}{
		{
			name: "the cancelled task answers what replaced it",
			node: &store.Task{
				ID: "T-aaaaaa", Key: &retiredKey, Status: "cancelled", FeatureID: "F-aaaaaa",
				SupersededBy: &replacement,
			},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.Equal(t, "T-bbbbbb", v.SupersededBy)
				require.Empty(t, v.Supersedes)
				require.Contains(t, raw, `"superseded_by":"T-bbbbbb"`)
				require.NotContains(t, raw, `"supersedes"`)
			},
		},
		{
			name: "the replacement task answers what it replaced",
			node: &store.Task{
				ID: replacement, Key: &replacementKey, Status: "proposed", FeatureID: "F-aaaaaa",
				Supersedes: []string{"T-aaaaaa"},
			},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.Equal(t, []string{"T-aaaaaa"}, v.Supersedes)
				require.Empty(t, v.SupersededBy)
				require.Contains(t, raw, `"supersedes":["T-aaaaaa"]`)
				require.NotContains(t, raw, `"superseded_by"`)
			},
		},
		{
			name: "a feature carries the same pair one grain up",
			node: &store.Feature{
				ID: "F-aaaaaa", Key: &retiredFeatureKey, Status: "done", EpicKey: "care",
				Supersedes: []string{"F-cccccc"}, SupersededBy: &replacementFeature,
			},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.Equal(t, []string{"F-cccccc"}, v.Supersedes)
				require.Equal(t, "F-bbbbbb", v.SupersededBy)
				require.Contains(t, raw, `"supersedes":["F-cccccc"]`)
				require.Contains(t, raw, `"superseded_by":"F-bbbbbb"`)
			},
		},
		{
			name: "a node in no supersession carries neither key",
			node: &store.Task{ID: "T-cccccc", Status: "ready", FeatureID: "F-aaaaaa"},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.Empty(t, v.Supersedes)
				require.Empty(t, v.SupersededBy)
				require.NotContains(t, raw, `"supersedes"`)
				require.NotContains(t, raw, `"superseded_by"`)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := views.Node(&store.Tree{}, tc.node)
			out, err := json.Marshal(v)
			require.NoError(t, err)
			tc.check(t, v, string(out))
		})
	}
}

// featureCheckNodeTree lays one active feature whose one task is done and
// whose manifest requires the evals gate at feature grain but records none,
// the awaiting-check shape gen.FeatureActivity fires on (mirrors
// internal/gen/activity_test.go's featureTree).
func featureCheckNodeTree() (*store.Tree, *store.Feature, *store.Task) {
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

// TestNodeCarriesFeatureActivity pins Task 8: NodeView.Activity carries the
// feature's derived rollup phase (gen.FeatureActivity), the same answer the
// status oracle and the list row give, so the node page can show the phase
// without a second fetch. A task carries no phase, so its Activity stays
// empty and the key is omitted from the marshaled JSON.
func TestNodeCarriesFeatureActivity(t *testing.T) {
	tree, f, tk := featureCheckNodeTree()

	fv := views.Node(tree, f)
	require.Equal(t, "awaiting_check", fv.Activity)
	fRaw, err := json.Marshal(fv)
	require.NoError(t, err)
	require.Contains(t, string(fRaw), `"activity":"awaiting_check"`)

	tv := views.Node(tree, tk)
	require.Equal(t, "", tv.Activity)
	tRaw, err := json.Marshal(tv)
	require.NoError(t, err)
	require.NotContains(t, string(tRaw), `"activity"`)
}

func TestSurfaceViewCarriesNoModules(t *testing.T) {
	v := views.SurfaceOf(store.Surface{Paths: []string{"app/models/calc"}})
	require.NotNil(t, v)

	// The modules field was cut from the schema (artifact-schema.md, the
	// brownfield hinge): it had no writer and no checkable meaning, so the JSON
	// contract must not keep advertising it. Asserting the exact marshaled
	// shape, not a substring absence, is the honest form of that: a fixture
	// that never sets modules would satisfy a "no modules substring" check
	// forever regardless of what else the view marshals, so it would catch
	// nothing if this view ever grew some other key the contract never
	// sanctioned. The exact shape catches that too.
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	require.JSONEq(t, `{"paths":["app/models/calc"]}`, string(raw))
}

// TestNodeProjectsNextOwner pins task A2's additive field: NodeView.NextOwner
// carries exactly the answer views.NextOwner computes for the node (Node must
// not re-derive its own rule), present in the marshaled JSON when the node
// has a legal next move and omitted entirely, never an empty string, at a
// terminal state.
func TestNodeProjectsNextOwner(t *testing.T) {
	cases := []struct {
		name string
		node store.Node
		want string
	}{
		{"a ready task is the agent's to start", &store.Task{ID: "T-aaaaaa", Status: "ready"}, "agent"},
		{"a specced feature awaits human approval", &store.Feature{ID: "F-aaaaaa", Status: "specced"}, "human"},
		{"a done feature is terminal", &store.Feature{ID: "F-bbbbbb", Status: "done"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := views.Node(&store.Tree{}, tc.node)
			require.Equal(t, tc.want, v.NextOwner)
			require.Equal(t, views.NextOwner(tc.node), v.NextOwner,
				"Node must carry views.NextOwner's answer verbatim, not a second derivation")

			out, err := json.Marshal(v)
			require.NoError(t, err)
			if tc.want == "" {
				require.NotContains(t, string(out), `"next_owner"`)
			} else {
				require.Contains(t, string(out), `"next_owner":"`+tc.want+`"`)
			}
		})
	}
}

// TestNodeCarriesHeldByRetiredAncestor pins the NodeView bullet
// (issue #36): the additive held object is gen.RetiredAncestor's answer
// verbatim, present only when there is one, absent from the marshaled JSON
// (never a null "held" key) for a node with nothing retired above it.
func TestNodeCarriesHeldByRetiredAncestor(t *testing.T) {
	epicKey := "widgets"
	featKey := "widgets/gadget"
	epic := &store.Epic{ID: "E-aaaaaa", Key: &epicKey, Status: "cancelled"}
	feat := &store.Feature{ID: "F-aaaaaa", Key: &featKey, Status: "cancelled"}
	task := &store.Task{ID: "T-aaaaaa", FeatureID: feat.ID, Status: "in_progress"}
	tree := &store.Tree{
		Epics:    map[string]*store.Epic{epic.ID: epic},
		Features: map[string]*store.Feature{feat.ID: feat},
		Tasks:    map[string]*store.Task{task.ID: task},
	}

	v := views.Node(tree, task)
	require.NotNil(t, v.Held)
	require.Equal(t, "feature", v.Held.Noun)
	require.Equal(t, "F-aaaaaa", v.Held.ID)
	require.Equal(t, "cancelled", v.Held.State)
	require.Contains(t, v.Held.Ref, "widgets/gadget")

	raw, err := json.Marshal(v)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"held":{"noun":"feature","id":"F-aaaaaa"`)
	require.Contains(t, string(raw), `"state":"cancelled"`)

	// A live node with nothing retired above it carries no held object at all.
	liveFeat := &store.Feature{ID: "F-bbbbbb", Status: "active"}
	lv := views.Node(&store.Tree{}, liveFeat)
	require.Nil(t, lv.Held)
	lraw, err := json.Marshal(lv)
	require.NoError(t, err)
	require.NotContains(t, string(lraw), `"held"`)
}

// TestGatesProjectsGateLevelEvidenceOrderedAndOmitsWhenEmpty pins Task 7 of the
// serve read-model correction batch: store.Gate already carries an Evidence
// list recorded by `tiller check`, and this pins that views.Gates copies it
// onto GateView in the stored order (never re-sorted), while a gate that
// recorded none omits the key entirely from the marshaled JSON: never null,
// never an empty array.
func TestGatesProjectsGateLevelEvidenceOrderedAndOmitsWhenEmpty(t *testing.T) {
	block := store.GatesBlock{
		Gates: map[string]store.Gate{
			"tests": {
				Required: true,
				Kind:     store.GateKindCommand,
				Status:   store.GateStatusPass,
				Evidence: []store.Evidence{
					{Kind: "command_output", Ref: "artifacts/tiller/gates/tests-abc1234-run2.txt"},
					{Kind: "command_output", Ref: "artifacts/tiller/gates/tests-abc1234-run1.txt"},
				},
			},
			"build": {
				Required: true,
				Kind:     store.GateKindCommand,
				Status:   store.GateStatusPass,
			},
		},
	}

	out := views.Gates(block)
	require.Equal(t, []views.EvidenceView{
		{Kind: "command_output", Ref: "artifacts/tiller/gates/tests-abc1234-run2.txt"},
		{Kind: "command_output", Ref: "artifacts/tiller/gates/tests-abc1234-run1.txt"},
	}, out["tests"].Evidence, "evidence order must survive the projection verbatim")
	require.Nil(t, out["build"].Evidence)

	testsRaw, err := json.Marshal(out["tests"])
	require.NoError(t, err)
	require.Contains(t, string(testsRaw),
		`"evidence":[{"kind":"command_output","ref":"artifacts/tiller/gates/tests-abc1234-run2.txt"},`+
			`{"kind":"command_output","ref":"artifacts/tiller/gates/tests-abc1234-run1.txt"}]`)

	buildRaw, err := json.Marshal(out["build"])
	require.NoError(t, err)
	require.NotContains(t, string(buildRaw), "evidence",
		"a gate with no recorded evidence must omit the key, never null, never []")
}

// TestBlockedReadsEveryGrainThatCarriesTheFlag pins views.Blocked across the
// three node types that record it. A task joined them when it became a folder
// node (artifact-schema.md, Tasks); a question carries no such field.
func TestBlockedReadsEveryGrainThatCarriesTheFlag(t *testing.T) {
	cases := []struct {
		name string
		node store.Node
		want bool
	}{
		{"blocked epic", &store.Epic{ID: "E-1", Blocked: true}, true},
		{"unblocked epic", &store.Epic{ID: "E-2"}, false},
		{"blocked feature", &store.Feature{ID: "F-1", Blocked: true}, true},
		{"unblocked feature", &store.Feature{ID: "F-2"}, false},
		{"blocked task", &store.Task{ID: "T-1", Blocked: true}, true},
		{"unblocked task", &store.Task{ID: "T-2"}, false},
		{"question carries no flag", &store.Question{ID: "Q-1"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, views.Blocked(c.node))
		})
	}
}

// TestEpicKeyPlacesATaskUnderItsEpic pins the axis `ls --epic` filters on. A
// task's canonical key begins with the epic segment, so the filter can answer
// for it; before it did not, and `ls task --epic <key>` returned nothing while
// the epic held tasks, which reads as a fact about the epic rather than a
// missing filter.
func TestEpicKeyPlacesATaskUnderItsEpic(t *testing.T) {
	taskKey := "form-engine/certification-form-loading/schema-transcription"
	require.Equal(t, "form-engine", views.EpicKey(&store.Task{ID: "T-1", Key: &taskKey}))

	// A feature still answers from its recorded linkage, and a task with no key
	// recorded places nowhere rather than guessing.
	featKey := "form-engine/certification-form-loading"
	require.Equal(t, "form-engine", views.EpicKey(&store.Feature{ID: "F-1", Key: &featKey, EpicKey: "form-engine"}))
	require.Equal(t, "", views.EpicKey(&store.Task{ID: "T-2"}))
}

// TestNodeCarriesRecordedOrderAndItsRefs pins Task 8: the recorded soft order
// rides `show` and the node page as carriage only, the raw list `tiller
// order` wrote (store.Epic.Order, store.Feature.Order), verbatim and
// index-aligned to its human refs (OrderRefs), never the full computed
// sibling order Relationships.Features/Tasks already carries. A dangling id
// degrades to its bare id rather than disappearing (gen.RefByID's own rule),
// and a noun with nothing to sequence, or one that never ran `tiller order`,
// carries neither key.
func TestNodeCarriesRecordedOrderAndItsRefs(t *testing.T) {
	epicKey := "form-engine"
	loaderKey := "form-engine/loader"
	parserKey := "form-engine/parser"
	tree := &store.Tree{
		Epics: map[string]*store.Epic{
			"E-aaaaaa": {ID: "E-aaaaaa", Key: &epicKey, Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-loader": {ID: "F-loader", Key: &loaderKey, Status: "draft", EpicKey: "form-engine"},
			"F-parser": {ID: "F-parser", Key: &parserKey, Status: "draft", EpicKey: "form-engine"},
		},
	}

	cases := []struct {
		name  string
		node  store.Node
		check func(t *testing.T, v views.NodeView, raw string)
	}{
		{
			name: "an epic carries its recorded feature order and refs, in recorded sequence",
			node: &store.Epic{ID: "E-aaaaaa", Key: &epicKey, Status: "ready", Order: []string{"F-parser", "F-loader"}},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.Equal(t, []string{"F-parser", "F-loader"}, v.Order)
				require.Equal(t, []string{
					"form-engine/parser (F-parser)",
					"form-engine/loader (F-loader)",
				}, v.OrderRefs)
				require.Contains(t, raw, `"order":["F-parser","F-loader"]`)
				require.Contains(t, raw,
					`"order_refs":["form-engine/parser (F-parser)","form-engine/loader (F-loader)"]`)
			},
		},
		{
			name: "a dangling recorded id degrades to its bare id rather than disappearing",
			node: &store.Feature{ID: "F-aaaaaa", Status: "draft", Order: []string{"T-ghost"}},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.Equal(t, []string{"T-ghost"}, v.Order)
				require.Equal(t, []string{"T-ghost"}, v.OrderRefs)
			},
		},
		{
			name: "an epic that never ran tiller order carries neither key",
			node: &store.Epic{ID: "E-bbbbbb", Status: "draft"},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.Nil(t, v.Order)
				require.Nil(t, v.OrderRefs)
				require.NotContains(t, raw, `"order":`)
				require.NotContains(t, raw, `"order_refs":`)
			},
		},
		{
			name: "a task carries no order: a leaf noun has no children to sequence",
			node: &store.Task{ID: "T-aaaaaa", Status: "draft", FeatureID: "F-aaaaaa"},
			check: func(t *testing.T, v views.NodeView, raw string) {
				require.Nil(t, v.Order)
				require.Nil(t, v.OrderRefs)
				require.NotContains(t, raw, `"order":`)
				require.NotContains(t, raw, `"order_refs":`)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := views.Node(tree, tc.node)
			out, err := json.Marshal(v)
			require.NoError(t, err)
			tc.check(t, v, string(out))
		})
	}
}

// TestNodeProjectsQuestionSourceAndBlocks pins issue #55: a question's Source
// citation and its Blocks list live on store.Question (internal/store/node.go)
// but were dropped from the read envelope entirely, so `show question --json`
// could not say why a question exists (Source) or what it holds (Blocks) even
// though both were recorded. Blocks resolves through views.BlockRef, the
// minimal id-plus-key pair blockRefsOf (node.go) computes by calling the SAME
// relatedByIDs resolution the node page's own Relationships.Blocks already
// calls (nodepage.go's relationshipsOf, *store.Question arm): one
// computation, not a second lookup. A dangling id degrades to a bare id with
// no key, relatedByIDs' own rule, and a question with neither carries neither
// key (omitempty).
func TestNodeProjectsQuestionSourceAndBlocks(t *testing.T) {
	featKey := "form-engine/schema-engine"
	tree := &store.Tree{
		Features: map[string]*store.Feature{
			"F-111111": {ID: "F-111111", Key: &featKey, Status: "specced", EpicKey: "form-engine"},
		},
	}

	t.Run("a sourced, blocking question carries both", func(t *testing.T) {
		q := &store.Question{
			ID: "Q-aaaaaa", Statement: "Keep or trim the legacy image parsers?",
			Status: "open", Owner: "jc",
			Source: ".tiller/feedback/opus-review.md#L24-L26",
			Blocks: []string{"F-111111"},
		}
		v := views.Node(tree, q)
		require.Equal(t, ".tiller/feedback/opus-review.md#L24-L26", v.Source)
		require.Equal(t, []views.BlockRef{{ID: "F-111111", Key: "form-engine/schema-engine"}}, v.Blocks)

		raw, err := json.Marshal(v)
		require.NoError(t, err)
		require.Contains(t, string(raw), `"source":".tiller/feedback/opus-review.md#L24-L26"`)
		require.Contains(t, string(raw), `"blocks":[{"id":"F-111111","key":"form-engine/schema-engine"}]`)
	})

	t.Run("a dangling block degrades to its bare id, the same rule relatedByIDs already applies", func(t *testing.T) {
		q := &store.Question{ID: "Q-bbbbbb", Statement: "Ghost target?", Status: "open", Blocks: []string{"F-zzzzzz"}}
		v := views.Node(tree, q)
		require.Equal(t, []views.BlockRef{{ID: "F-zzzzzz"}}, v.Blocks)

		raw, err := json.Marshal(v)
		require.NoError(t, err)
		require.Contains(t, string(raw), `"blocks":[{"id":"F-zzzzzz"}]`)
	})

	t.Run("a blockless, sourceless question carries neither key", func(t *testing.T) {
		q := &store.Question{ID: "Q-cccccc", Statement: "No provenance, holds nothing.", Status: "open"}
		v := views.Node(tree, q)
		require.Empty(t, v.Source)
		require.Nil(t, v.Blocks)

		raw, err := json.Marshal(v)
		require.NoError(t, err)
		require.NotContains(t, string(raw), `"source"`)
		require.NotContains(t, string(raw), `"blocks"`)
	})
}

func TestQuestionNodeAndStatusViewsUseTitleAndRetainFullStatement(t *testing.T) {
	titledStatement := strings.Repeat("This is the full titled question statement. ", 6)
	legacyStatement := strings.Repeat("界", 81)
	tree := &store.Tree{}
	cases := []struct {
		name      string
		question  *store.Question
		wantTitle string
	}{
		{
			name: "persisted title",
			question: &store.Question{
				ID: "Q-titled", Title: "Canonical schema", Statement: titledStatement, Status: "open",
			},
			wantTitle: "Canonical schema",
		},
		{
			name: "legacy bounded fallback",
			question: &store.Question{
				ID: "Q-legacy", Statement: legacyStatement, Status: "open",
			},
			wantTitle: strings.Repeat("界", 79) + "…",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := views.Node(tree, tc.question)
			require.Equal(t, tc.wantTitle, node.Title)
			require.Equal(t, tc.question.Statement, node.Statement)

			status := views.NodeStatus(tree, tc.question, workflow.Input{})
			require.Equal(t, tc.wantTitle, status.Title)
			require.Equal(t, tc.question.Statement, status.Statement)
		})
	}
}

// TestNodeViewCarriesWorkItemFields pins the shelf-node default arm: Node,
// Owner, and NodeGates all read a bug or chore through store.AsWorkNode, and
// Activity derives the carrier phase against the live tree the same way
// gen.WorkActivity does, which is why the carrier itself has to be in the
// tree the view is built from.
func TestNodeViewCarriesWorkItemFields(t *testing.T) {
	k := "bugs/alpha"
	tk := "form-engine/loading/wire"
	pr := 9
	b := &store.Bug{WorkItem: store.WorkItem{
		ID: "B-aaaaaa", Key: &k, Title: "Alpha", Status: "ready", Owner: "jc",
		AttachedTo: "F-79dh3q", ResolvedBy: "T-k3p9w6", PR: &pr, Branch: "tiller/bugs/alpha",
	}}
	carrier := &store.Task{ID: "T-k3p9w6", Key: &tk, Status: "in_progress"}
	tree := &store.Tree{
		Bugs:  map[string]*store.Bug{"B-aaaaaa": b},
		Tasks: map[string]*store.Task{"T-k3p9w6": carrier},
	}
	v := views.Node(tree, b)
	require.Equal(t, "bug", v.Noun)
	require.Equal(t, "F-79dh3q", v.AttachedTo)
	require.Equal(t, "T-k3p9w6", v.ResolvedBy)
	require.Equal(t, "carried", v.Activity)
	require.Equal(t, "tiller/bugs/alpha", v.Branch)
	require.Equal(t, 9, *v.PR)
	require.Equal(t, "jc", views.Owner(b))
	st := views.NodeStatus(tree, b, workflow.Input{})
	require.ElementsMatch(t, []string{"cancel", "return", "start"}, st.LegalNext)

	// The phase recomputes on the carrier's fate, so a cancelled carrier
	// changes the answer with nothing on the bug itself rewritten.
	carrier.Status = "cancelled"
	require.Equal(t, "carrier_cancelled", views.Node(tree, b).Activity)
	require.Equal(t, "T-k3p9w6", views.Node(tree, b).ResolvedBy, "the pointer stays; a human clears or re-points it")
}

func TestNodeCancelReason(t *testing.T) {
	tree := &store.Tree{}
	c := views.Node(tree, &store.Question{ID: "Q-aaaaaa", Statement: "Gone?", Status: "cancelled", CancelReason: "folded into the plan"})
	require.NotNil(t, c.CancelReason)
	require.Equal(t, "folded into the plan", *c.CancelReason)
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"cancel_reason":"folded into the plan"`)

	o := views.Node(tree, &store.Question{ID: "Q-bbbbbb", Statement: "Open?", Status: "open"})
	require.Nil(t, o.CancelReason)
	raw, err = json.Marshal(o)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "cancel_reason")
}
