package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// A greenfield eval file parses its criteria with polarity and binding
// (artifact-schema.md, The eval model).
func TestLoadEvalFile_Greenfield(t *testing.T) {
	path := filepath.Join(".tiller", "epics", "form-engine",
		"features", "nrca-form-loading", "feature.eval.md")

	ef, err := LoadEvalFile(fixtureTreeRoot, path)
	require.NoError(t, err)

	assert.Equal(t, "F-79dh3q", ef.Node)
	require.Len(t, ef.Criteria, 3)

	c1 := ef.Criteria[0]
	assert.Equal(t, "C1", c1.ID)
	assert.Equal(t, PolarityIntroduce, c1.Polarity)
	assert.Equal(t, BindingTest, c1.Binding.Type)
	assert.Equal(t, BindingRefs{"spec/form_definitions/nrca_lti_02a_spec.rb"}, c1.Binding.Ref)
	assert.True(t, c1.Required)

	c2 := ef.Criteria[1]
	assert.Equal(t, BindingCommand, c2.Binding.Type)
	assert.Equal(t, "bundle exec rspec spec/pdf/nrca_golden_spec.rb", c2.Binding.Run)

	c3 := ef.Criteria[2]
	assert.Equal(t, BindingManual, c3.Binding.Type)
}

// A brownfield preserve criterion carries a baseline: surface, captured-at
// revision, and the artifact embodying the old behavior.
func TestParseEvalFile_PreserveBaseline(t *testing.T) {
	src := []byte(`---
node: F-x5q1z8
criteria:
  - id: C2
    statement: Existing single-zone projects compute identical results to pre-change
    polarity: preserve
    binding: { type: command, run: "bundle exec rspec spec/calc/single_zone_characterization_spec.rb" }
    baseline:
      surface: "app/models/calc"
      captured_at: { kind: git_tree, value: "git:3f6a9d2", path: app/models/calc, observed_at: "2026-06-23T00:00:00Z" }
      artifact: "spec/fixtures/calc/single_zone_2026-07-01.json"
    required: true
---

Body prose after the frontmatter.
`)

	ef, err := ParseEvalFile(src)
	require.NoError(t, err)
	require.Len(t, ef.Criteria, 1)

	c := ef.Criteria[0]
	assert.Equal(t, PolarityPreserve, c.Polarity)
	require.NotNil(t, c.Baseline)
	assert.Equal(t, "app/models/calc", c.Baseline.Surface)
	assert.Equal(t, KindGitTree, c.Baseline.CapturedAt.Kind)
	assert.Equal(t, "git:3f6a9d2", c.Baseline.CapturedAt.Value)
	assert.Equal(t, "spec/fixtures/calc/single_zone_2026-07-01.json", c.Baseline.Artifact)
}

// wellFormedContract is a feature.eval.md whose three criteria each carry an id,
// a statement, a recognized binding, and the field that binding requires. It is
// the contract ValidateContract must accept.
const wellFormedContract = `---
node: F-79dh3q
criteria:
  - id: C1
    statement: The form validates against its JSON Schema
    polarity: introduce
    binding: { type: test, ref: "spec/form_spec.rb" }
    required: true
  - id: C2
    statement: Parent and child render to a single PDF
    polarity: introduce
    binding: { type: command, run: "bundle exec rspec spec/pdf_spec.rb" }
    required: true
  - id: C3
    statement: A reviewer confirms the rendered PDF matches the source layout
    polarity: introduce
    binding: { type: manual }
    required: true
---

# Acceptance
`

// TestValidateContract_WellFormed confirms a correctly authored acceptance
// contract passes validation.
func TestValidateContract_WellFormed(t *testing.T) {
	ef, err := ParseEvalFile([]byte(wellFormedContract))
	require.NoError(t, err)
	require.NoError(t, ef.ValidateContract())
}

// TestValidateContract_Rejections walks the structurally-wrong contracts the
// dry run surfaced: each must be refused, the message must name the offending
// criterion, and the error must classify as misuse (exit 2).
func TestValidateContract_Rejections(t *testing.T) {
	cases := []struct {
		name      string
		fm        string
		wantNamed string // substring the message must carry (the offending criterion)
	}{
		{
			name:      "no criteria at all",
			fm:        "node: F-1\ncriteria: []\n",
			wantNamed: "no criteria",
		},
		{
			name: "criterion is just prose (no binding)",
			fm: "node: F-1\ncriteria:\n" +
				"  - id: C1\n    statement: The form should feel nice\n",
			wantNamed: "C1",
		},
		{
			name: "unknown binding type",
			fm: "node: F-1\ncriteria:\n" +
				"  - id: C1\n    statement: It works\n    binding: { type: smoke }\n",
			wantNamed: "C1",
		},
		{
			name: "command binding with no run",
			fm: "node: F-1\ncriteria:\n" +
				"  - id: C2\n    statement: The suite passes\n    binding: { type: command }\n",
			wantNamed: "C2",
		},
		{
			name: "test binding with no ref or run",
			fm: "node: F-1\ncriteria:\n" +
				"  - id: C3\n    statement: The spec passes\n    binding: { type: test }\n",
			wantNamed: "C3",
		},
		{
			name: "eval binding with no rubric",
			fm: "node: F-1\ncriteria:\n" +
				"  - id: C4\n    statement: The copy reads naturally\n    binding: { type: eval }\n",
			wantNamed: "C4",
		},
		{
			name: "criterion missing a statement",
			fm: "node: F-1\ncriteria:\n" +
				"  - id: C5\n    binding: { type: manual }\n",
			wantNamed: "C5",
		},
		{
			name: "criterion missing an id",
			fm: "node: F-1\ncriteria:\n" +
				"  - statement: It works\n    binding: { type: manual }\n",
			wantNamed: "criterion",
		},
		{
			name: "required preserve criterion with no baseline",
			fm: "node: F-1\ncriteria:\n" +
				"  - id: C6\n    statement: Existing behavior is unchanged\n" +
				"    polarity: preserve\n    binding: { type: command, run: \"rspec\" }\n    required: true\n",
			wantNamed: "C6",
		},
		{
			name: "repeated criterion id hides the first result",
			fm: "node: F-1\ncriteria:\n" +
				"  - id: C1\n    statement: First\n    binding: { type: manual }\n" +
				"  - id: C1\n    statement: Second\n    binding: { type: manual }\n",
			wantNamed: "C1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ef, err := ParseEvalFile([]byte("---\n" + tc.fm + "---\n\nbody\n"))
			require.NoError(t, err, "the YAML itself parses; the contract is what is wrong")

			err = ef.ValidateContract()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantNamed)
			require.Contains(t, err.Error(), "tiller guide specify", "the message points at the in-project shape")

			var coder ExitCoder
			require.ErrorAs(t, err, &coder)
			require.Equal(t, 2, coder.ExitCode(), "a malformed contract is misuse (exit 2)")
			var r interface{ Reason() string }
			require.ErrorAs(t, err, &r)
			require.Equal(t, "eval_contract_invalid", r.Reason())
		})
	}
}

// bindingFrontmatter wraps one binding body into a one-criterion eval file.
func bindingFrontmatter(binding string) []byte {
	return []byte("---\nnode: F-1\ncriteria:\n  - id: C1\n    statement: It works\n    binding:\n" + binding + "---\n\nbody\n")
}

// A test binding's ref reads as a scalar or as a list of paths.
func TestParseEvalFile_BindingRefsShapes(t *testing.T) {
	cases := []struct {
		name    string
		binding string
		want    BindingRefs
	}{
		{"scalar", "      type: test\n      ref: spec/a_spec.rb\n", BindingRefs{"spec/a_spec.rb"}},
		{"flow list", "      type: test\n      ref: [spec/a_spec.rb, spec/b_spec.rb]\n", BindingRefs{"spec/a_spec.rb", "spec/b_spec.rb"}},
		{"block list", "      type: test\n      ref:\n        - spec/a_spec.rb\n        - spec/b_spec.rb\n", BindingRefs{"spec/a_spec.rb", "spec/b_spec.rb"}},
		{"absent", "      type: test\n      run: make test\n", nil},
		{"blank scalar", "      type: test\n      ref: \"\"\n      run: make test\n", nil},
		{"null", "      type: test\n      ref:\n      run: make test\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ef, err := ParseEvalFile(bindingFrontmatter(tc.binding))
			require.NoError(t, err)
			require.Len(t, ef.Criteria, 1)
			assert.Equal(t, tc.want, ef.Criteria[0].Binding.Ref)
		})
	}
}

// A mapping (or a list holding a non-scalar) under ref is not a path list and
// refuses at parse.
func TestParseEvalFile_BindingRefsRejectsNonScalars(t *testing.T) {
	for name, binding := range map[string]string{
		"mapping":        "      type: test\n      ref: { path: spec/a_spec.rb }\n",
		"nested mapping": "      type: test\n      ref:\n        - { path: spec/a_spec.rb }\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseEvalFile(bindingFrontmatter(binding))
			require.Error(t, err)
		})
	}
}

// One ref stays a scalar on disk, so single-file contracts do not churn; two or
// more write as a sequence; none writes nothing.
func TestEvalBinding_MarshalRefs(t *testing.T) {
	one, err := yaml.Marshal(EvalBinding{Type: BindingTest, Ref: BindingRefs{"spec/a_spec.rb"}})
	require.NoError(t, err)
	assert.Equal(t, "type: test\nref: spec/a_spec.rb\n", string(one))

	two, err := yaml.Marshal(EvalBinding{Type: BindingTest, Ref: BindingRefs{"spec/a_spec.rb", "spec/b_spec.rb"}})
	require.NoError(t, err)
	assert.Equal(t, "type: test\nref:\n    - spec/a_spec.rb\n    - spec/b_spec.rb\n", string(two))

	none, err := yaml.Marshal(EvalBinding{Type: BindingTest, Run: "make test"})
	require.NoError(t, err)
	assert.Equal(t, "type: test\nrun: make test\n", string(none))

	var back EvalBinding
	require.NoError(t, yaml.Unmarshal(two, &back))
	assert.Equal(t, BindingRefs{"spec/a_spec.rb", "spec/b_spec.rb"}, back.Ref)
}

func TestBindingRefs_JoinedAndEmpty(t *testing.T) {
	assert.Equal(t, "./a ./b", BindingRefs{"./a", "./b"}.Joined())
	assert.Equal(t, "", BindingRefs(nil).Joined())
	assert.True(t, BindingRefs(nil).Empty())
	assert.True(t, BindingRefs{"", "  "}.Empty())
	assert.False(t, BindingRefs{"", "a"}.Empty())
}

// Several refs are valid on a test binding, with or without a run; a blank item
// and a multi-ref eval binding are refused.
func TestValidateContract_MultiRef(t *testing.T) {
	parse := func(binding string) *EvalFile {
		ef, err := ParseEvalFile(bindingFrontmatter(binding))
		require.NoError(t, err)
		return ef
	}
	require.NoError(t, parse("      type: test\n      ref: [spec/a_spec.rb, spec/b_spec.rb]\n").ValidateContract())

	err := parse("      type: test\n      ref: [spec/a_spec.rb, \"\"]\n").ValidateContract()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "criterion C1 has a blank ref entry")

	err = parse("      type: eval\n      ref: [rubrics/a.md, rubrics/b.md]\n").ValidateContract()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "criterion C1 has an eval binding with 2 refs; a rubric ref names one file")

	err = parse("      type: test\n      ref: []\n").ValidateContract()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "test binding with no ref or run")
}

func TestSplitFrontmatter(t *testing.T) {
	fm, body, err := splitFrontmatter([]byte("---\nnode: F-1\n---\nhello body\n"))
	require.NoError(t, err)
	assert.Equal(t, "node: F-1", string(fm))
	assert.Equal(t, "hello body\n", string(body))
}

func TestSplitFrontmatter_Missing(t *testing.T) {
	_, _, err := splitFrontmatter([]byte("no frontmatter here\n"))
	assert.Error(t, err)
}

func TestSplitFrontmatter_Unterminated(t *testing.T) {
	_, _, err := splitFrontmatter([]byte("---\nnode: F-1\nnever closes\n"))
	assert.Error(t, err)
}
