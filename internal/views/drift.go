package views

// This file projects drift.Report onto the read envelope `tiller drift` and
// Task 11's serve endpoint both render (cli-surface.md, the drift paragraph;
// task-10-brief.md, resolution 3). It is a pure struct mapping: drift.Run
// already did every git read and filesystem stat the sweep needed, so this
// reshapes what it found and does no I/O of its own, matching this package's
// own rule (node.go's package comment: "no reasoning, no mutation, no
// rendering").

import (
	"strings"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// DriftReportView is drift.Report's read projection, the one shape `tiller
// drift` and Task 11's serve endpoint both render, so the CLI and the console
// cannot disagree about what a sweep found. Candidates groups by marker,
// because a marker's whole section is what a reader (and the CLI's human
// renderer) wants at once; Uncovered stays a flat list across every marker,
// since a node can land there under one of several (Marker, Reason) pairs
// that only read sensibly beside its own Marker (drift.Uncovered's own doc
// comment). Compared always carries the node total the engine counted, never
// omitted even when it is zero, because a clean report's whole point is to
// state that count rather than stay silent about it.
type DriftReportView struct {
	Candidates     map[string][]CandidateView `json:"candidates,omitempty"`
	Uncovered      []UncoveredView            `json:"uncovered,omitempty"`
	Disabled       []string                   `json:"disabled,omitempty"`
	Exempted       map[string]int             `json:"exempted,omitempty"`
	Coverage       map[string]map[string]int  `json:"coverage,omitempty"`
	Compared       int                        `json:"compared"`
	ComparedDesign int                        `json:"compared_design"`
}

// CandidateView is one drifted node in the read envelope, the JSON face of
// drift.Candidate: the marker that found it, the node's structured noun/id/
// key (never a composed label; the global constraint that id and key ride as
// separate fields), the human Detail sentence, the Files that moved, Since
// the record's commit anchor (projected structurally, nil when there is none
// to date by: a pre-anchor record, or answered_question's own case where the
// answer revision is a worktree fingerprint rather than a commit,
// drift.Candidate's own doc comment), and any Context lines riding beside
// Detail.
type CandidateView struct {
	Marker   string              `json:"marker"`
	Noun     string              `json:"noun"`
	ID       string              `json:"id"`
	Key      string              `json:"key,omitempty"`
	Detail   string              `json:"detail,omitempty"`
	Files    []string            `json:"files,omitempty"`
	Since    *RevisionRefView    `json:"since,omitempty"`
	Until    *RevisionRefView    `json:"until,omitempty"`
	Features []DesignFeatureView `json:"features,omitempty"`
	Context  []string            `json:"context,omitempty"`

	// Remedy is the clearing move as one runnable command, composed per
	// marker where the recorded data suffices (candidateRemedy) and empty
	// where no single command clears it (stale_spec, orphan_eval). A done
	// feature's stale_spec eval leg and its orphan_eval clear through an
	// approved correct-eval amendment, which takes an edit, a staged
	// operation, and a human approval rather than one command. The console
	// renders what exists and invents nothing, per ui-surface.md's "reported on that
	// node's page with its clearing move beside it" and its renders-output-
	// only non-negotiable (JC's drift-panel ask, 2026-08-06).
	Remedy string `json:"remedy,omitempty"`

	// Ref is the node's human reference ("<key-path> (<id>)", gen.Ref) for the
	// plain-text renderer only; the JSON payload carries id/key structurally
	// instead (house pattern, node.go's own NodeView.Ref).
	Ref string `json:"-"`
}

// DesignFeatureView preserves the actual tracked feature and its crossing.
type DesignFeatureView struct {
	ID     string          `json:"id"`
	Key    string          `json:"key"`
	Spec   string          `json:"spec"`
	Commit RevisionRefView `json:"commit"`
}

// UncoveredView is one node a marker's sweep could not compare, the JSON face
// of drift.Uncovered. Reason is scoped per (Marker, Reason) pair, never
// globally unique across markers (drift.Uncovered's own doc comment
// enumerates which marker emits which), so a consumer keys on the pair rather
// than on Reason alone; carrying Marker on every row, in this flat list, is
// what makes that pair readable without cross-referencing anything else.
type UncoveredView struct {
	Marker string `json:"marker"`
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`

	// Ref, same house pattern as CandidateView.Ref above.
	Ref string `json:"-"`
}

// DriftReportOf projects one drift.Run outcome onto the read envelope. t is
// the same tree the sweep ran against, consulted only to build each row's
// human Ref (gen.RefByID), the one thing a Candidate/Uncovered's own id/key
// pair does not already carry as a rendered label.
func DriftReportOf(t *store.Tree, rep drift.Report) DriftReportView {
	v := DriftReportView{
		Disabled:       rep.Disabled,
		Exempted:       rep.Exempted,
		Coverage:       rep.Coverage,
		Compared:       rep.Compared,
		ComparedDesign: rep.ComparedDesign,
	}
	for _, c := range rep.Candidates {
		if v.Candidates == nil {
			v.Candidates = map[string][]CandidateView{}
		}
		v.Candidates[c.Marker] = append(v.Candidates[c.Marker], candidateViewOf(t, c))
	}
	for _, u := range rep.Uncovered {
		v.Uncovered = append(v.Uncovered, uncoveredViewOf(t, u))
	}
	return v
}

// candidateViewOf projects one drift.Candidate, preserving every field
// verbatim except Since, which is reshaped structurally (sinceViewOf), plus
// the composed Remedy.
func candidateViewOf(t *store.Tree, c drift.Candidate) CandidateView {
	v := CandidateView{
		Marker:  c.Marker,
		Noun:    c.Noun,
		ID:      c.ID,
		Key:     c.Key,
		Detail:  c.Detail,
		Files:   c.Files,
		Since:   sinceViewOf(c.Since),
		Until:   sinceViewOf(c.Until),
		Context: c.Context,
		Remedy:  candidateRemedy(c),
		Ref:     gen.RefByID(t, c.ID),
	}
	if c.Noun == "design" {
		v.Ref = c.Key
	}
	for _, feature := range c.Features {
		row := DesignFeatureView{ID: feature.ID, Key: feature.Key, Spec: feature.Spec}
		if commit := revisionRefOf(feature.Commit); commit != nil {
			row.Commit = *commit
		}
		v.Features = append(v.Features, row)
	}
	return v
}

// candidateRemedy composes the candidate's clearing move where the recorded
// data suffices, and "" where no command exists. The addr is the candidate's
// own key falling back to its id, the same key-first addressing basisAddr
// resolves; the candidate recorded its key at sweep time, so no tree lookup
// is needed here. missing_surface's command carries a <path> hole
// deliberately: the path is the human's judgment, and the template is
// exactly what the mend-drift skill prints.
func candidateRemedy(c drift.Candidate) string {
	addr := c.Key
	if addr == "" {
		addr = c.ID
	}
	switch c.Marker {
	case drift.MarkerStaleDesign, drift.MarkerShiftedDesign:
		return "tiller approve design -- '" + strings.ReplaceAll(addr, "'", "'\"'\"'") + "'"
	case drift.MarkerActiveBasis:
		return RefreshBasisCommand(c.Noun, addr)
	case drift.MarkerAnsweredQuestion:
		if c.Question == "" {
			return ""
		}
		return ReviewedAssumptionCommand(addr, c.Question)
	case drift.MarkerMissingSurface:
		return "tiller link " + c.Noun + " " + addr + " --surface <path>"
	default:
		return ""
	}
}

// NodeDriftView is the drift panel's payload: one node page's candidates and
// uncovered rows at candidate grain, beside (never instead of) the
// one-per-marker badge list the chip renders from. The serve handler fills
// both from its one drift.ForNode call, so the chip and the panel it opens
// cannot disagree (JC's drift-panel ask, 2026-08-06; serve/node.go used to
// discard the uncovered rows for want of exactly this detail area).
type NodeDriftView struct {
	Candidates []CandidateView `json:"candidates,omitempty"`
	Uncovered  []UncoveredView `json:"uncovered,omitempty"`
}

// NodeDriftDetailOf projects one node's ForNode outcome onto the panel
// payload, nil when there is nothing at all to show: the wire stays quiet
// the same way the chip does, and omitempty keeps the block off every clean
// node's page.
func NodeDriftDetailOf(t *store.Tree, candidates []drift.Candidate, uncovered []drift.Uncovered) *NodeDriftView {
	if len(candidates) == 0 && len(uncovered) == 0 {
		return nil
	}
	v := &NodeDriftView{}
	for _, c := range candidates {
		v.Candidates = append(v.Candidates, candidateViewOf(t, c))
	}
	for _, u := range uncovered {
		v.Uncovered = append(v.Uncovered, uncoveredViewOf(t, u))
	}
	return v
}

// uncoveredViewOf projects one drift.Uncovered, preserving every field
// verbatim.
func uncoveredViewOf(t *store.Tree, u drift.Uncovered) UncoveredView {
	v := UncoveredView{
		Marker: u.Marker,
		Noun:   u.Noun,
		ID:     u.ID,
		Key:    u.Key,
		Reason: u.Reason,
		Detail: u.Detail,
		Ref:    gen.RefByID(t, u.ID),
	}
	if u.Noun == "design" {
		v.Ref = u.Key
	}
	return v
}

// sinceViewOf projects a candidate's optional commit anchor, nil in either
// of the two cases drift.Candidate.Since's own doc comment enumerates (a
// pre-anchor record, or answered_question's fingerprint-kind case), neither
// of which is evidence the sweep failed to compare the node. It reuses
// revisionRefOf (eval.go), the same structural kind/value/
// path/observed_at projection every other revision-bearing field in this
// package already uses (task-10-brief.md, resolution 4: "as existing views
// do for revision-bearing fields"); short-sha rendering is left to the human
// renderer alone, never to this pure projection.
func sinceViewOf(r *store.RevisionRef) *RevisionRefView {
	if r == nil {
		return nil
	}
	return revisionRefOf(*r)
}
