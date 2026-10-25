package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// wellFormedAmendment is the worked example from the amendment build's task-1
// brief: one operation of each of the five staged kinds, so a single fixture
// exercises every field this type carries.
const wellFormedAmendment = `id: A-d6w3f9
key: iphone-only
title: Remove native iPad support from v1
status: proposed
implements: ""
triggered_by: [design/04-platform-and-constraints.md]
amends_design: [design/01-overview.md]
base_revision: git:78d2166
operations:
  - op: revise
    target: F-z8e13q
    preconditions: { spec_revision: "git:aaa...", eval_revision: "git:bbb..." }
  - op: supersede
    target: F-79dh3q
    by: F-x5q1z8
    preconditions: { target_status: done, target_revision: "git:7bc..." }
  - op: cancel
    target: T-p7kknm
    reason: "superseded under the iPhone-only decision"
    superseded_by: T-newone
    preconditions: { target_status: check }
  - op: add-feature
    key: form-engine/table-backed-forms
    change_type: replace
    preconditions: { key_absent: form-engine/table-backed-forms }
  - op: add-task
    key: app-experience/fast-logging/iphone-acceptance
    title: iPhone-only acceptance
    preconditions: { key_absent: app-experience/fast-logging/iphone-acceptance }
`

// parseWellFormedAmendment unmarshals a fresh copy of wellFormedAmendment, so
// each rejection case in TestValidateAmendment_Rejections mutates its own
// value rather than a shared one.
func parseWellFormedAmendment(t *testing.T) Amendment {
	t.Helper()
	var a Amendment
	require.NoError(t, yaml.Unmarshal([]byte(wellFormedAmendment), &a))
	return a
}

// TestAmendment_RoundTrip pins every field of the on-disk shape task-1-brief.md
// specifies verbatim: it parses, every operation's fields land where expected,
// and marshaling it back out and re-parsing yields the same value.
func TestAmendment_RoundTrip(t *testing.T) {
	a := parseWellFormedAmendment(t)

	assert.Equal(t, "A-d6w3f9", a.ID)
	require.NotNil(t, a.Key, "an amendment is a folder-backed... err, file-backed node: it carries a key")
	assert.Equal(t, "iphone-only", *a.Key)
	assert.Equal(t, "Remove native iPad support from v1", a.Title)
	assert.Equal(t, AmendmentStatusProposed, a.Status)
	assert.Equal(t, "", a.Implements, "implements may be empty: no ADR reference yet")
	assert.Equal(t, []string{"design/04-platform-and-constraints.md"}, a.TriggeredBy)
	assert.Equal(t, []string{"design/01-overview.md"}, a.AmendsDesign)
	assert.Equal(t, "git:78d2166", a.BaseRevision)
	require.Len(t, a.Operations, 5)

	revise := a.Operations[0]
	assert.Equal(t, AmendmentOpRevise, revise.Op)
	assert.Equal(t, "F-z8e13q", revise.Target)
	assert.Equal(t, map[string]string{"spec_revision": "git:aaa...", "eval_revision": "git:bbb..."}, revise.Preconditions)

	supersede := a.Operations[1]
	assert.Equal(t, AmendmentOpSupersede, supersede.Op)
	assert.Equal(t, "F-79dh3q", supersede.Target)
	assert.Equal(t, "F-x5q1z8", supersede.By)
	assert.Equal(t, map[string]string{"target_status": "done", "target_revision": "git:7bc..."}, supersede.Preconditions)

	cancel := a.Operations[2]
	assert.Equal(t, AmendmentOpCancel, cancel.Op)
	assert.Equal(t, "T-p7kknm", cancel.Target)
	assert.Equal(t, "superseded under the iPhone-only decision", cancel.Reason)
	assert.Equal(t, "T-newone", cancel.SupersededBy)
	assert.Equal(t, map[string]string{"target_status": "check"}, cancel.Preconditions)

	addFeature := a.Operations[3]
	assert.Equal(t, AmendmentOpAddFeature, addFeature.Op)
	assert.Equal(t, "form-engine/table-backed-forms", addFeature.Key)
	assert.Equal(t, "replace", addFeature.ChangeType)
	assert.Equal(t, map[string]string{"key_absent": "form-engine/table-backed-forms"}, addFeature.Preconditions)

	addTask := a.Operations[4]
	assert.Equal(t, AmendmentOpAddTask, addTask.Op)
	assert.Equal(t, "app-experience/fast-logging/iphone-acceptance", addTask.Key)
	assert.Equal(t, "iPhone-only acceptance", addTask.Title)
	assert.Equal(t, map[string]string{"key_absent": "app-experience/fast-logging/iphone-acceptance"}, addTask.Preconditions)

	require.NoError(t, ValidateAmendment(&a), "the worked example is the well-formed shape ValidateAmendment must accept")

	out, err := yaml.Marshal(&a)
	require.NoError(t, err)
	var roundTripped Amendment
	require.NoError(t, yaml.Unmarshal(out, &roundTripped))
	assert.Equal(t, a, roundTripped, "marshal then unmarshal must reproduce the same value")
}

// TestAmendment_NodeInterface pins Amendment against the common Node view every
// addressable artifact implements, the same shape the resolver and read
// commands hold without switching on a concrete type.
func TestAmendment_NodeInterface(t *testing.T) {
	key := "iphone-only"
	a := &Amendment{ID: "A-d6w3f9", Key: &key, Status: AmendmentStatusProposed}

	var n Node = a
	assert.Equal(t, "A-d6w3f9", n.NodeID())
	require.NotNil(t, n.NodeKey())
	assert.Equal(t, "iphone-only", *n.NodeKey())
	assert.Equal(t, NounAmendment, n.Noun())
	assert.Equal(t, AmendmentStatusProposed, n.NodeStatus())
}

// TestValidateAmendment_WellFormed confirms the worked example, unmutated,
// passes validation.
func TestValidateAmendment_WellFormed(t *testing.T) {
	a := parseWellFormedAmendment(t)
	require.NoError(t, ValidateAmendment(&a))
}

// TestValidateAmendment_Rejections walks every shape defect ValidateAmendment
// is asked to catch: an unrecognized status, an unrecognized op, a field
// illegal for its op, and a field missing that its op requires. Each case
// starts from the well-formed example and mutates exactly one thing, so the
// rejection is attributable to that one change.
func TestValidateAmendment_Rejections(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(a *Amendment)
		wantMsg string
		wantOp  int // OpIndex the error names; -1 for a file-level fault
	}{
		{
			name:    "unrecognized status",
			mutate:  func(a *Amendment) { a.Status = "active" },
			wantMsg: `"active"`,
			wantOp:  -1,
		},
		{
			name:    "unrecognized op",
			mutate:  func(a *Amendment) { a.Operations[0].Op = "rewrite" },
			wantMsg: "operation 0",
			wantOp:  0,
		},
		{
			name:    "superseded_by set on a non-cancel op",
			mutate:  func(a *Amendment) { a.Operations[0].SupersededBy = "T-abc123" },
			wantMsg: "operation 0",
			wantOp:  0,
		},
		{
			name:    "by set on a non-supersede op",
			mutate:  func(a *Amendment) { a.Operations[0].By = "F-abc123" },
			wantMsg: "operation 0",
			wantOp:  0,
		},
		{
			name:    "blocks set on a non-add-question op",
			mutate:  func(a *Amendment) { a.Operations[0].Blocks = []string{"F-abc123"} },
			wantMsg: "operation 0",
			wantOp:  0,
		},
		{
			name:    "add-feature with no key",
			mutate:  func(a *Amendment) { a.Operations[3].Key = "" },
			wantMsg: "operation 3",
			wantOp:  3,
		},
		{
			name:    "add-task with no key",
			mutate:  func(a *Amendment) { a.Operations[4].Key = "" },
			wantMsg: "operation 4",
			wantOp:  4,
		},
		{
			name:    "revise with no target",
			mutate:  func(a *Amendment) { a.Operations[0].Target = "" },
			wantMsg: "operation 0",
			wantOp:  0,
		},
		{
			name:    "supersede with no target",
			mutate:  func(a *Amendment) { a.Operations[1].Target = "" },
			wantMsg: "operation 1",
			wantOp:  1,
		},
		{
			name:    "cancel with no target",
			mutate:  func(a *Amendment) { a.Operations[2].Target = "" },
			wantMsg: "operation 2",
			wantOp:  2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := parseWellFormedAmendment(t)
			tc.mutate(&a)

			err := ValidateAmendment(&a)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)

			var shapeErr *AmendmentShapeError
			require.ErrorAs(t, err, &shapeErr)
			assert.Equal(t, tc.wantOp, shapeErr.OpIndex)

			var coder ExitCoder
			require.ErrorAs(t, err, &coder)
			assert.Equal(t, 2, coder.ExitCode(), "a malformed amendment is misuse (exit 2)")

			var r interface{ Reason() string }
			require.ErrorAs(t, err, &r)
			assert.Equal(t, "amendment_shape_invalid", r.Reason())
		})
	}
}

// TestValidateAmendment_NilIsRefused guards the entry point against a nil
// pointer rather than panicking, the same defensiveness ValidateContract shows
// for a nil EvalFile.
func TestValidateAmendment_NilIsRefused(t *testing.T) {
	err := ValidateAmendment(nil)
	require.Error(t, err)
}

// TestValidateAmendment_AddEpic pins add-epic's own shape (task-2-brief.md):
// a bare epic key, no parent segment, and no target the way revise/supersede/
// cancel need one. It requires a non-blank Key exactly as add-feature and
// add-task do.
func TestValidateAmendment_AddEpic(t *testing.T) {
	key := "iphone-only"
	a := Amendment{
		ID: "A-d6w3f9", Key: &key, Status: AmendmentStatusProposed,
		Operations: []AmendmentOp{
			{Op: AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"},
		},
	}
	require.NoError(t, ValidateAmendment(&a))

	a.Operations[0].Key = ""
	err := ValidateAmendment(&a)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation 0")

	var shapeErr *AmendmentShapeError
	require.ErrorAs(t, err, &shapeErr)
	assert.Equal(t, 0, shapeErr.OpIndex)
}

// TestValidateAmendment_AddQuestion pins add-question's two-field shape:
// Title is the required one-line scanning title and Statement is the required
// full question. Statement is illegal on every other operation kind.
func TestValidateAmendment_AddQuestion(t *testing.T) {
	key := "iphone-only"
	valid := Amendment{
		ID: "A-d6w3f9", Key: &key, Status: AmendmentStatusProposed,
		Operations: []AmendmentOp{
			{
				Op:        AmendmentOpAddQuestion,
				Title:     "Authentication provider",
				Statement: "Which auth provider?",
				Blocks:    []string{"F-abc123"},
			},
		},
	}
	require.NoError(t, ValidateAmendment(&valid))

	out, err := yaml.Marshal(&valid)
	require.NoError(t, err)
	assert.Contains(t, string(out), "title: Authentication provider")
	assert.Contains(t, string(out), "statement: Which auth provider?")

	tests := []struct {
		name    string
		op      AmendmentOp
		wantMsg string
	}{
		{
			name:    "missing title",
			op:      AmendmentOp{Op: AmendmentOpAddQuestion, Statement: "Which auth provider?"},
			wantMsg: "has no title",
		},
		{
			name:    "missing statement",
			op:      AmendmentOp{Op: AmendmentOpAddQuestion, Title: "Authentication provider"},
			wantMsg: "has no statement",
		},
		{
			name:    "line feed in title",
			op:      AmendmentOp{Op: AmendmentOpAddQuestion, Title: "Authentication\nprovider", Statement: "Which auth provider?"},
			wantMsg: "title must be one line",
		},
		{
			name:    "carriage return in title",
			op:      AmendmentOp{Op: AmendmentOpAddQuestion, Title: "Authentication\rprovider", Statement: "Which auth provider?"},
			wantMsg: "title must be one line",
		},
		{
			name:    "statement on another operation",
			op:      AmendmentOp{Op: AmendmentOpAddEpic, Key: "hardening", Statement: "Which auth provider?"},
			wantMsg: "sets statement",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := valid
			a.Operations = []AmendmentOp{tt.op}
			err := ValidateAmendment(&a)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)

			var shapeErr *AmendmentShapeError
			require.ErrorAs(t, err, &shapeErr)
			assert.Equal(t, 0, shapeErr.OpIndex)
			assert.Equal(t, "amendment_shape_invalid", shapeErr.Reason())
		})
	}
}

// TestAmendmentOp_SourceRoundTrip pins AmendmentOp.Source (task-1-brief.md):
// free text under the "source" key, round-tripping through yaml unchanged.
func TestAmendmentOp_SourceRoundTrip(t *testing.T) {
	raw := `id: A-d6w3f9
key: iphone-only
status: proposed
operations:
  - op: revise
    target: F-z8e13q
    source: "docs/feedback/opus-review.md#L12-18"
`
	var a Amendment
	require.NoError(t, yaml.Unmarshal([]byte(raw), &a))
	require.Len(t, a.Operations, 1)
	assert.Equal(t, "docs/feedback/opus-review.md#L12-18", a.Operations[0].Source)
	require.NoError(t, ValidateAmendment(&a))

	out, err := yaml.Marshal(&a)
	require.NoError(t, err)
	assert.Contains(t, string(out), "source: docs/feedback/opus-review.md#L12-18")

	var roundTripped Amendment
	require.NoError(t, yaml.Unmarshal(out, &roundTripped))
	assert.Equal(t, a, roundTripped)
}

// TestAmendmentOp_SourceLegalOnEveryOpKind pins that validateAmendmentOp
// carries no per-kind rule for source (task-1-brief.md: "legal on every op
// kind"): the worked example, with a source citation stapled onto each of its
// five differently-shaped operations, still validates clean.
func TestAmendmentOp_SourceLegalOnEveryOpKind(t *testing.T) {
	a := parseWellFormedAmendment(t)
	for i := range a.Operations {
		a.Operations[i].Source = "docs/feedback/opus-review.md#L1-2"
	}
	require.NoError(t, ValidateAmendment(&a))
}

// TestAmendment_SourceRoundTrip pins Amendment.Source (task-1-brief.md): nil
// when the amendment carries none (every hand-staged amendment `tiller add
// amendment` opens today), and Path/Revision round-tripping through yaml when
// present, the shape a later task's `tiller intake` writes.
func TestAmendment_SourceRoundTrip(t *testing.T) {
	a := parseWellFormedAmendment(t)
	assert.Nil(t, a.Source, "a hand amendment carries no source")

	raw := `id: A-d6w3f9
key: iphone-only
status: proposed
source:
  path: .tiller/feedback/opus-review.md
  revision: "git:9f2a1c0"
`
	var withSource Amendment
	require.NoError(t, yaml.Unmarshal([]byte(raw), &withSource))
	require.NotNil(t, withSource.Source)
	assert.Equal(t, ".tiller/feedback/opus-review.md", withSource.Source.Path)
	assert.Equal(t, "git:9f2a1c0", withSource.Source.Revision)
	require.NoError(t, ValidateAmendment(&withSource))

	out, err := yaml.Marshal(&withSource)
	require.NoError(t, err)
	var roundTripped Amendment
	require.NoError(t, yaml.Unmarshal(out, &roundTripped))
	assert.Equal(t, withSource, roundTripped)
}

// TestValidateAmendment_CorrectEval pins correct-eval's shape: a target and a
// reason are required, and retire is legal on this op alone with non-blank,
// unique criterion ids.
func TestValidateAmendment_CorrectEval(t *testing.T) {
	key := "eval-bindings"
	amendmentWith := func(op AmendmentOp) *Amendment {
		return &Amendment{
			ID: "A-d6w3f9", Key: &key, Status: AmendmentStatusProposed,
			Operations: []AmendmentOp{op},
		}
	}
	valid := AmendmentOp{
		Op: AmendmentOpCorrectEval, Target: "F-z8e13q", Reason: "a binding file moved",
		Retire:        []string{"C7", "C8"},
		Preconditions: map[string]string{"eval_revision": "git:aaa"},
	}

	t.Run("a well-formed correct-eval is accepted", func(t *testing.T) {
		require.NoError(t, ValidateAmendment(amendmentWith(valid)))
	})

	t.Run("retire is optional", func(t *testing.T) {
		op := valid
		op.Retire = nil
		require.NoError(t, ValidateAmendment(amendmentWith(op)))
	})

	t.Run("retire round-trips through yaml", func(t *testing.T) {
		data, err := yaml.Marshal(amendmentWith(valid))
		require.NoError(t, err)
		assert.Contains(t, string(data), "retire:")
		var back Amendment
		require.NoError(t, yaml.Unmarshal(data, &back))
		assert.Equal(t, []string{"C7", "C8"}, back.Operations[0].Retire)
	})

	refusals := []struct {
		name    string
		op      AmendmentOp
		wantMsg string
	}{
		{"no target", AmendmentOp{Op: AmendmentOpCorrectEval, Reason: "r"}, "has no target"},
		{"blank reason", AmendmentOp{Op: AmendmentOpCorrectEval, Target: "F-z8e13q", Reason: "  "}, "has no reason"},
		{"missing reason", AmendmentOp{Op: AmendmentOpCorrectEval, Target: "F-z8e13q"}, "has no reason"},
		{"blank retire id", AmendmentOp{Op: AmendmentOpCorrectEval, Target: "F-z8e13q", Reason: "r", Retire: []string{"C7", " "}}, "blank"},
		{"duplicate retire ids", AmendmentOp{Op: AmendmentOpCorrectEval, Target: "F-z8e13q", Reason: "r", Retire: []string{"C7", "C7"}}, "twice"},
		{"duplicate retire ids differing only in whitespace", AmendmentOp{Op: AmendmentOpCorrectEval, Target: "F-z8e13q", Reason: "r", Retire: []string{"C7", " C7 "}}, "retires C7 twice"},
		{"retire on a revise", AmendmentOp{Op: AmendmentOpRevise, Target: "F-z8e13q", Retire: []string{"C7"}}, "retire"},
		{"retire on a cancel", AmendmentOp{Op: AmendmentOpCancel, Target: "F-z8e13q", Reason: "r", Retire: []string{"C7"}}, "retire"},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAmendment(amendmentWith(tc.op))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
			var shapeErr *AmendmentShapeError
			require.ErrorAs(t, err, &shapeErr)
			assert.Equal(t, 0, shapeErr.OpIndex)
		})
	}
}
