package views

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// evalFixture is a *.eval.md parsed into memory, exercising every binding
// family in authored order: a bare manual criterion (C1, no ref or run), a
// test criterion bound by ref (C2), a not-required command criterion bound by
// run (C3, proving Required renders false rather than omitting the key), an
// eval (AI-graded rubric) criterion bound by run (C4), and a required preserve
// criterion carrying a full pre-change baseline (C5, artifact-schema.md, "The
// eval model", the brownfield modify example).
func evalFixture() *store.EvalFile {
	return &store.EvalFile{
		Node: "F-x5q1z8",
		Criteria: []store.EvalCriterion{
			{
				ID:        "C1",
				Statement: "A reviewer confirms the rendered PDF matches the source form layout",
				Polarity:  store.PolarityIntroduce,
				Required:  true,
				Binding:   store.EvalBinding{Type: store.BindingManual},
			},
			{
				ID:        "C2",
				Statement: "Multi-zone calculations produce correct per-zone sampling counts",
				Polarity:  store.PolarityIntroduce,
				Required:  true,
				Binding:   store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/calc/multi_zone_spec.rb"}},
			},
			{
				ID:        "C3",
				Statement: "Parent + child render to a single PDF with continuous page numbers",
				Polarity:  store.PolarityIntroduce,
				Required:  false,
				Binding:   store.EvalBinding{Type: store.BindingCommand, Run: "bundle exec rspec spec/pdf/cert_golden_spec.rb"},
			},
			{
				ID:        "C4",
				Statement: "The rendered summary reads naturally to a human reviewer",
				Polarity:  store.PolarityIntroduce,
				Required:  true,
				Binding:   store.EvalBinding{Type: store.BindingEval, Run: "Grade whether the summary reads naturally, 1-5."},
			},
			{
				ID:        "C5",
				Statement: "Existing single-zone projects compute identical results to pre-change",
				Polarity:  store.PolarityPreserve,
				Required:  true,
				Binding:   store.EvalBinding{Type: store.BindingCommand, Run: "bundle exec rspec spec/calc/single_zone_characterization_spec.rb"},
				Baseline: &store.EvalBaseline{
					Surface: "app/models/calc",
					CapturedAt: store.RevisionRef{
						Kind:       store.KindGitTree,
						Value:      "git:3f6a9d2",
						Path:       "app/models/calc",
						ObservedAt: time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC),
					},
					Artifact: "spec/fixtures/calc/single_zone_2026-07-01.json",
				},
			},
		},
	}
}

// TestEvalContractOfMapsCriteriaInFileOrder pins the whole contract mapping
// byte-for-byte in JSON terms: the path and node passed straight through,
// every binding family's exact fields (a manual binding carries neither ref
// nor run, a test binding carries ref, a command and an eval binding each
// carry run), a not-required criterion rendering "required":false rather than
// omitting the key, and the required preserve criterion's full baseline with
// its captured_at revision reference (kind, value, path, and an RFC3339 UTC
// observed_at). The JSONEq comparison also rules out a stray "baseline" key on
// the four criteria that carry none, since an extra key fails the match.
func TestEvalContractOfMapsCriteriaInFileOrder(t *testing.T) {
	ef := evalFixture()
	got := EvalContractOf(".tiller/epics/calc/features/multi-zone/feature.eval.md", ef)

	require.Equal(t, ".tiller/epics/calc/features/multi-zone/feature.eval.md", got.Path)
	require.Equal(t, "F-x5q1z8", got.Node)
	require.Len(t, got.Criteria, 5)

	ids := make([]string, len(got.Criteria))
	for i, c := range got.Criteria {
		ids[i] = c.ID
	}
	require.Equal(t, []string{"C1", "C2", "C3", "C4", "C5"}, ids, "criteria stay in authored file order")

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	want := `{
	  "path": ".tiller/epics/calc/features/multi-zone/feature.eval.md",
	  "node": "F-x5q1z8",
	  "criteria": [
	    {
	      "id": "C1",
	      "statement": "A reviewer confirms the rendered PDF matches the source form layout",
	      "polarity": "introduce",
	      "required": true,
	      "binding": { "type": "manual" }
	    },
	    {
	      "id": "C2",
	      "statement": "Multi-zone calculations produce correct per-zone sampling counts",
	      "polarity": "introduce",
	      "required": true,
	      "binding": { "type": "test", "ref": "spec/calc/multi_zone_spec.rb" }
	    },
	    {
	      "id": "C3",
	      "statement": "Parent + child render to a single PDF with continuous page numbers",
	      "polarity": "introduce",
	      "required": false,
	      "binding": { "type": "command", "run": "bundle exec rspec spec/pdf/cert_golden_spec.rb" }
	    },
	    {
	      "id": "C4",
	      "statement": "The rendered summary reads naturally to a human reviewer",
	      "polarity": "introduce",
	      "required": true,
	      "binding": { "type": "eval", "run": "Grade whether the summary reads naturally, 1-5." }
	    },
	    {
	      "id": "C5",
	      "statement": "Existing single-zone projects compute identical results to pre-change",
	      "polarity": "preserve",
	      "required": true,
	      "binding": { "type": "command", "run": "bundle exec rspec spec/calc/single_zone_characterization_spec.rb" },
	      "baseline": {
	        "surface": "app/models/calc",
	        "captured_at": {
	          "kind": "git_tree",
	          "value": "git:3f6a9d2",
	          "path": "app/models/calc",
	          "observed_at": "2026-06-23T00:00:00Z"
	        },
	        "artifact": "spec/fixtures/calc/single_zone_2026-07-01.json"
	      }
	    }
	  ]
	}`
	require.JSONEq(t, want, string(raw))
}

// TestEvalContractErrorViewShape pins the union's error state: a node whose
// eval file failed to load, parse, or validate renders {path, error} in place
// of {path, node, criteria}, so a client can tell "no contract" (null) apart
// from "an authored contract that is not well-formed" (this) apart from a
// valid one (EvalContractView).
func TestEvalContractErrorViewShape(t *testing.T) {
	v := EvalContractErrorView{
		Path:  ".tiller/epics/form-engine/features/schema-engine/feature.eval.md",
		Error: "feature.eval.md has no criteria; an acceptance contract needs at least one",
	}
	got, err := json.Marshal(v)
	require.NoError(t, err)
	require.JSONEq(t, `{
	  "path": ".tiller/epics/form-engine/features/schema-engine/feature.eval.md",
	  "error": "feature.eval.md has no criteria; an acceptance contract needs at least one"
	}`, string(got))
}

// TestNodePageEvalFieldMarshalsNull pins the pure projection's contract: an
// eval file is never read here (NodePageOf performs no I/O), so Eval stays
// nil, and with no omitempty on its tag that renders "eval":null rather than
// omitting the key, so a renderer sees the same key on every node page
// regardless of whether one was ever authored.
func TestNodePageEvalFieldMarshalsNull(t *testing.T) {
	page := NodePage{
		Node: NodeView{Noun: "feature", ID: "F-000001", Status: "draft"},
		Ref:  "form-engine/schema-engine (F-000001)",
	}
	require.Nil(t, page.Eval)

	got, err := json.Marshal(page)
	require.NoError(t, err)
	require.Contains(t, string(got), `"eval":null`)
}

// A binding that names one file emits ref only; two or more emit refs and no
// ref, so readers of the single-ref shape see nothing change.
func TestEvalBindingViewRefShapes(t *testing.T) {
	one := evalBindingOf(store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/a_spec.rb"}})
	raw, err := json.Marshal(one)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"test","ref":"spec/a_spec.rb"}`, string(raw))

	two := evalBindingOf(store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/a_spec.rb", "spec/b_spec.rb"}})
	raw, err = json.Marshal(two)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"test","refs":["spec/a_spec.rb","spec/b_spec.rb"]}`, string(raw))
}
