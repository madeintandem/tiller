package views

// This file derives the read side's authored eval contract (artifact-schema.md,
// "The eval model"): the acceptance criteria carried in a *.eval.md file's
// frontmatter, projected onto a stable JSON shape. It is a pure struct mapping
// over an already-parsed store.EvalFile; reading the file from disk and
// validating it as a well-formed contract are the caller's job
// (store.LoadEvalFile, EvalFile.ValidateContract), not this file's. The eval
// criteria are the authored contract; the recorded results of evaluating them
// live separately, in the node's gates block (views.GateView's Criteria map).

import (
	"github.com/madeintandem/tiller/internal/store"
)

// EvalContractView is a node's authored acceptance contract, the JSON face of
// a parsed and validated store.EvalFile: the repo-relative path it was read
// from (the same drill target gen.EvalPath derives), the node it grounds, and
// its criteria in file order.
type EvalContractView struct {
	Path     string              `json:"path"`
	Node     string              `json:"node"`
	Criteria []EvalCriterionView `json:"criteria"`
}

// EvalCriterionView is one acceptance criterion in the read envelope, the JSON
// face of store.EvalCriterion: its polarity, whether it is required, the gate
// it binds to, and its pre-change baseline when it carries one (a required
// preserve criterion always does; every other criterion carries none).
type EvalCriterionView struct {
	ID        string            `json:"id"`
	Statement string            `json:"statement"`
	Polarity  string            `json:"polarity"`
	Required  bool              `json:"required"`
	Binding   EvalBindingView   `json:"binding"`
	Baseline  *EvalBaselineView `json:"baseline,omitempty"`
}

// EvalBindingView connects a criterion to a gate in the read envelope, the
// JSON face of store.EvalBinding: a test file or id, a command to run, a bare
// manual human verification, or an AI-graded rubric. Ref and Run are omitted
// together for a manual binding, which needs neither. Ref carries a single
// ref; a test binding that names two or more files sets Refs instead and
// leaves Ref empty, so readers of single refs are unchanged.
type EvalBindingView struct {
	Type string   `json:"type"`
	Ref  string   `json:"ref,omitempty"`
	Refs []string `json:"refs,omitempty"`
	Run  string   `json:"run,omitempty"`
}

// evalBindingOf projects a stored binding onto the read envelope.
func evalBindingOf(b store.EvalBinding) EvalBindingView {
	view := EvalBindingView{Type: b.Type, Run: b.Run}
	switch len(b.Ref) {
	case 0:
	case 1:
		view.Ref = b.Ref[0]
	default:
		view.Refs = append([]string(nil), b.Ref...)
	}
	return view
}

// EvalBaselineView is a required preserve criterion's pre-change baseline in
// the read envelope, the JSON face of store.EvalBaseline: the source surface,
// the revision it was captured at, and the artifact that embodies the old
// behavior.
type EvalBaselineView struct {
	Surface    string           `json:"surface,omitempty"`
	CapturedAt *RevisionRefView `json:"captured_at,omitempty"`
	Artifact   string           `json:"artifact,omitempty"`
}

// RevisionRefView is a stored revision reference in the read envelope, the
// JSON face of store.RevisionRef: what Kind and Value address (a git blob,
// commit, tree, or content hash), the Path the ref was taken against, and
// ObservedAt rendered RFC3339 in UTC.
type RevisionRefView struct {
	Kind       string `json:"kind"`
	Value      string `json:"value"`
	Path       string `json:"path,omitempty"`
	ObservedAt string `json:"observed_at,omitempty"`
}

// EvalContractErrorView reports that a node's eval file failed to load,
// parse, or validate as a well-formed acceptance contract: the path it was
// read from and the error naming why, standing in for the criteria a
// well-formed contract would carry.
type EvalContractErrorView struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// EvalContractOf projects a parsed, validated eval file onto the read
// envelope: the path it was read from, its node, and its criteria, preserving
// file order.
func EvalContractOf(path string, ef *store.EvalFile) EvalContractView {
	return EvalContractView{
		Path:     path,
		Node:     ef.Node,
		Criteria: evalCriteriaOf(ef.Criteria),
	}
}

// evalCriteriaOf projects a file's criteria onto the read envelope, preserving
// their authored order, the order a human reads the contract in.
func evalCriteriaOf(cs []store.EvalCriterion) []EvalCriterionView {
	out := make([]EvalCriterionView, len(cs))
	for i, c := range cs {
		out[i] = EvalCriterionView{
			ID:        c.ID,
			Statement: c.Statement,
			Polarity:  c.Polarity,
			Required:  c.Required,
			Binding:   evalBindingOf(c.Binding),
			Baseline:  evalBaselineOf(c.Baseline),
		}
	}
	return out
}

// evalBaselineOf projects a criterion's optional pre-change baseline onto the
// read envelope, nil when the criterion carries none.
func evalBaselineOf(b *store.EvalBaseline) *EvalBaselineView {
	if b == nil {
		return nil
	}
	return &EvalBaselineView{
		Surface:    b.Surface,
		CapturedAt: revisionRefOf(b.CapturedAt),
		Artifact:   b.Artifact,
	}
}

// revisionRefOf projects a stored revision reference onto the read envelope,
// nil for the zero value, the same convention BasisOf and VerifiedAgainstOf
// use elsewhere in this package for an unset revision-bearing field.
func revisionRefOf(r store.RevisionRef) *RevisionRefView {
	if r.Kind == "" && r.Value == "" && r.Path == "" && r.ObservedAt.IsZero() {
		return nil
	}
	return &RevisionRefView{
		Kind:       r.Kind,
		Value:      r.Value,
		Path:       r.Path,
		ObservedAt: formatTime(r.ObservedAt),
	}
}
