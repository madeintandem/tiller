package store

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// This file holds the eval model: the acceptance contract carried in the
// frontmatter of a *.eval.md file (artifact-schema.md, The eval model). The
// stored results of evaluating these criteria do not live here; they land in
// the node's gates block (gates.go).

// Criterion polarity values (artifact-schema.md, The eval model). introduce
// asserts new behavior is present; preserve asserts existing behavior is
// unchanged.
const (
	PolarityIntroduce = "introduce"
	PolarityPreserve  = "preserve"
)

// Eval binding types (artifact-schema.md, The eval model).
const (
	BindingTest    = "test"
	BindingCommand = "command"
	BindingManual  = "manual"
	BindingEval    = "eval"
)

// EvalFile is the parsed frontmatter of a *.eval.md file: the node it grounds
// and its ordered acceptance criteria. The prose body is the human elaboration
// and is not modeled here.
type EvalFile struct {
	Node     string          `yaml:"node"`
	Criteria []EvalCriterion `yaml:"criteria,omitempty"`
}

// EvalCriterion is one acceptance criterion: a polarity, a binding to a gate,
// and an optional pre-change baseline that a required preserve criterion must
// carry (artifact-schema.md, The eval model).
type EvalCriterion struct {
	ID        string        `yaml:"id"`
	Statement string        `yaml:"statement,omitempty"`
	Polarity  string        `yaml:"polarity,omitempty"`
	Binding   EvalBinding   `yaml:"binding,omitempty"`
	Baseline  *EvalBaseline `yaml:"baseline,omitempty"`
	Required  bool          `yaml:"required,omitempty"`
}

// EvalBinding connects a criterion to a gate: a test file or id, a command to
// run, a manual human verification, or an AI-graded rubric (artifact-schema.md,
// The eval model).
type EvalBinding struct {
	Type string      `yaml:"type,omitempty"`
	Ref  BindingRefs `yaml:"ref,omitempty"`
	Run  string      `yaml:"run,omitempty"`
}

// BindingRefs is the ref of a binding: one repo-relative path or test id, or a
// list of them. A test binding may name several files; an eval binding's rubric
// ref names one. It reads from YAML as a scalar or a sequence of scalars and
// writes a single item back as a scalar, so single-file contracts stay as they
// were written.
type BindingRefs []string

// UnmarshalYAML accepts a scalar (one ref), a sequence of scalars, or null. A
// blank scalar reads as no ref, as an empty string field always has; only a
// blank item inside a list is refused (by ValidateContract).
func (r *BindingRefs) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" || strings.TrimSpace(node.Value) == "" {
			*r = nil
			return nil
		}
		*r = BindingRefs{node.Value}
		return nil
	case yaml.SequenceNode:
		refs := make(BindingRefs, 0, len(node.Content))
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: a binding ref list holds paths, not nested values", item.Line)
			}
			refs = append(refs, item.Value)
		}
		*r = refs
		return nil
	default:
		return fmt.Errorf("line %d: a binding ref is a path or a list of paths", node.Line)
	}
}

// MarshalYAML writes one item as a scalar and several as a sequence. An empty
// list is omitted by the field's omitempty tag before this is reached.
func (r BindingRefs) MarshalYAML() (any, error) {
	if len(r) == 1 {
		return r[0], nil
	}
	return []string(r), nil
}

// Joined is the refs separated by single spaces, the form a runner command and
// a display line use.
func (r BindingRefs) Joined() string {
	return strings.Join(r, " ")
}

// Empty reports whether every item is blank, including a list with no items.
func (r BindingRefs) Empty() bool {
	for _, ref := range r {
		if strings.TrimSpace(ref) != "" {
			return false
		}
	}
	return true
}

// hasBlankEntry reports whether any item is blank.
func (r BindingRefs) hasBlankEntry() bool {
	for _, ref := range r {
		if strings.TrimSpace(ref) == "" {
			return true
		}
	}
	return false
}

// EvalBaseline is the pre-change baseline a required preserve criterion carries:
// the source surface, the revision it was captured at, and the artifact that
// embodies the old behavior (artifact-schema.md, The eval model).
type EvalBaseline struct {
	Surface    string      `yaml:"surface,omitempty"`
	CapturedAt RevisionRef `yaml:"captured_at,omitempty"`
	Artifact   string      `yaml:"artifact,omitempty"`
}

// docPointer is the breadcrumb every contract rejection ends on, so an agent
// that authored a malformed eval.md is sent at the canonical shape rather than
// left to reverse-engineer it. The breadcrumb stays inside the project per the
// self-contained rule: the shape comes out of the binary via `tiller guide
// specify`, never from a doc the project does not carry.
const docPointer = "run `tiller guide specify` for the contract shape"

// EvalContractError reports that a feature.eval.md parsed as YAML but is not a
// well-formed acceptance contract: it carries no criteria, or a criterion is
// missing a required field or names an unrecognized binding. It is a misuse
// (exit 2) the agent fixes by rewriting the contract to the documented shape,
// the same class as a malformed rubric output. Criterion names the offending
// criterion (empty for a file-level fault).
type EvalContractError struct {
	Criterion string
	Msg       string
}

func (e *EvalContractError) Error() string  { return e.Msg }
func (e *EvalContractError) ExitCode() int  { return 2 }
func (e *EvalContractError) Reason() string { return "eval_contract_invalid" }

// ValidateContract checks a parsed eval file is a well-formed acceptance contract
// (artifact-schema.md, The eval model). The contract must carry at least one
// criterion, and each criterion must have an id, a statement, a recognized
// binding type, the field that binding requires (a command needs a run, a test
// needs a ref or run, an eval needs a rubric in run or ref), and a baseline when
// it is a required preserve criterion. It returns an EvalContractError naming the
// first offending criterion. Criterion ids must be unique, since results are keyed
// by id and a repeat would hide one criterion behind another. A malformed
// contract is never silently accepted; the error's Reason() is
// "eval_contract_invalid".
func (ef *EvalFile) ValidateContract() error {
	if ef == nil || len(ef.Criteria) == 0 {
		return &EvalContractError{
			Msg: "feature.eval.md has no criteria; an acceptance contract needs at least one (" + docPointer + ")",
		}
	}
	seen := make(map[string]int, len(ef.Criteria))
	for i, c := range ef.Criteria {
		if err := validateCriterion(i, c); err != nil {
			return err
		}
		// Results are keyed by criterion id, so a repeated id would let one
		// criterion's result overwrite or hide another's, including a failure.
		if first, dup := seen[c.ID]; dup {
			return contractErr(c.ID, fmt.Sprintf("criterion id %s appears more than once (criteria #%d and #%d); every criterion needs a unique id", c.ID, first+1, i+1))
		}
		seen[c.ID] = i
	}
	return nil
}

// validateCriterion checks one criterion against the contract shape, returning a
// named EvalContractError on the first defect. The position index names a
// criterion that lacks its own id.
func validateCriterion(i int, c EvalCriterion) error {
	if strings.TrimSpace(c.ID) == "" {
		return contractErr("", fmt.Sprintf("criterion #%d has no id", i+1))
	}
	if strings.TrimSpace(c.Statement) == "" {
		return contractErr(c.ID, fmt.Sprintf("criterion %s has no statement", c.ID))
	}

	switch c.Binding.Type {
	case BindingCommand:
		if strings.TrimSpace(c.Binding.Run) == "" {
			return contractErr(c.ID, fmt.Sprintf("criterion %s has a command binding with no run", c.ID))
		}
	case BindingTest:
		if c.Binding.Ref.hasBlankEntry() {
			return contractErr(c.ID, fmt.Sprintf("criterion %s has a blank ref entry", c.ID))
		}
		if c.Binding.Ref.Empty() && strings.TrimSpace(c.Binding.Run) == "" {
			return contractErr(c.ID, fmt.Sprintf("criterion %s has a test binding with no ref or run", c.ID))
		}
	case BindingEval:
		if c.Binding.Ref.hasBlankEntry() {
			return contractErr(c.ID, fmt.Sprintf("criterion %s has a blank ref entry", c.ID))
		}
		if len(c.Binding.Ref) > 1 {
			return contractErr(c.ID, fmt.Sprintf("criterion %s has an eval binding with %d refs; a rubric ref names one file", c.ID, len(c.Binding.Ref)))
		}
		if strings.TrimSpace(c.Binding.Run) == "" && c.Binding.Ref.Empty() {
			return contractErr(c.ID, fmt.Sprintf("criterion %s has an eval binding with no rubric (set run or ref)", c.ID))
		}
	case BindingManual:
		// A manual binding needs no extra field; a human verifies it.
	default:
		return contractErr(c.ID, fmt.Sprintf("criterion %s has an unrecognized binding type %q (want test, command, manual, or eval)", c.ID, c.Binding.Type))
	}

	if c.Required && c.Polarity == PolarityPreserve && c.Baseline == nil {
		return contractErr(c.ID, fmt.Sprintf("required preserve criterion %s has no baseline (surface, captured_at, artifact)", c.ID))
	}
	return nil
}

// contractErr builds a named EvalContractError, appending the documentation
// pointer so every rejection sends the agent at the canonical shape.
func contractErr(criterion, msg string) *EvalContractError {
	return &EvalContractError{Criterion: criterion, Msg: msg + " (" + docPointer + ")"}
}

// LoadEvalFile reads and parses a repository-relative *.eval.md file.
func LoadEvalFile(root, relative string) (*EvalFile, error) {
	data, err := ReadRepositoryFile(root, relative)
	if err != nil {
		return nil, err
	}
	return ParseEvalFile(data)
}

// ParseEvalFile parses the YAML frontmatter of a *.eval.md document into an
// EvalFile, discarding the markdown body.
func ParseEvalFile(data []byte) (*EvalFile, error) {
	fm, _, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	var ef EvalFile
	if err := yaml.Unmarshal(fm, &ef); err != nil {
		return nil, fmt.Errorf("eval: parse frontmatter: %w", err)
	}
	return &ef, nil
}

// splitFrontmatter separates a leading `---` delimited YAML frontmatter block
// from the markdown body, returning each. It accepts CRLF or LF line endings and
// a leading UTF-8 BOM.
func splitFrontmatter(data []byte) (frontmatter, body []byte, err error) {
	s := strings.TrimPrefix(string(data), "\ufeff")
	if !strings.HasPrefix(s, "---") {
		return nil, nil, fmt.Errorf("eval: missing opening frontmatter delimiter")
	}
	rest := strings.TrimPrefix(s[3:], "\r")
	rest = strings.TrimPrefix(rest, "\n")

	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, nil, fmt.Errorf("eval: unterminated frontmatter")
	}
	fm := rest[:idx]
	after := rest[idx+len("\n---"):]
	after = strings.TrimPrefix(after, "\r")
	after = strings.TrimPrefix(after, "\n")
	return []byte(fm), []byte(after), nil
}
