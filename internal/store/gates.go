package store

import "time"

// This file holds the stored gate block: the result of `tiller check` written
// onto a node, plus the human attestations and the merge-time policy
// violations that sit beside it (artifact-schema.md, Gates and The stored gate
// block). The manifest's gate configuration (which gates exist and how they
// run) is a separate shape and lives in manifest.go as GatesConfig/GateSpec.

// Gate status values (artifact-schema.md, The stored gate block).
const (
	GateStatusPass    = "pass"
	GateStatusFail    = "fail"
	GateStatusPending = "pending"
	GateStatusSkipped = "skipped"
	GateStatusStale   = "stale"
)

// Criterion status values. needs_attestation is unique to the per-criterion map:
// the machine-checkable part is settled, but a required manual or eval criterion
// still awaits human sign-off. stale is the reopen marker: work landed under a
// node whose criteria were already settled, so a prior result, including a
// human's attestation, no longer describes what is there now
// (artifact-schema.md, The eval model).
const (
	CriterionStatusPass             = "pass"
	CriterionStatusFail             = "fail"
	CriterionStatusPending          = "pending"
	CriterionStatusNeedsAttestation = "needs_attestation"
	CriterionStatusStale            = "stale"
)

// Gate kinds.
const (
	GateKindCommand     = "command"
	GateKindAttestation = "attestation"
)

// GatesBlock is the node-level gate storage: the per-gate results, the human
// attestations, and the policy violations recorded at merge. On disk these are
// the top-level siblings gates/attestations/policy_violations, so it is embedded
// inline on Feature and Task (artifact-schema.md, The stored gate block).
type GatesBlock struct {
	Gates            map[string]Gate   `yaml:"gates,omitempty"`
	Attestations     []Attestation     `yaml:"attestations,omitempty"`
	PolicyViolations []PolicyViolation `yaml:"policy_violations,omitempty"`
}

// Gate is one stored gate result. The evals gate additionally carries the
// per-criterion Criteria map and records its evidence there rather than in the
// gate-level Evidence list (artifact-schema.md, The stored gate block).
//
// Code and Config are the gate-reuse keys (design 2026-10-21-gate-reuse): the
// content identity of the code the command ran over and the hash of the
// command that ran. A later run on a node whose current code and config match
// a stored pass may reuse that record instead of rerunning. ReusedAt and
// ReusedFrom are provenance only: when a reuse happened, and the source node
// when it was another node. All four are omitempty, so a record written before
// reuse existed round-trips unchanged and an empty Code never matches.
type Gate struct {
	Required   bool                 `yaml:"required"`
	Kind       string               `yaml:"kind,omitempty"`
	Status     string               `yaml:"status,omitempty"`
	RunBy      string               `yaml:"run_by,omitempty"`
	RunAt      time.Time            `yaml:"run_at,omitempty"`
	Against    RevisionRef          `yaml:"against,omitempty"`
	Code       string               `yaml:"code,omitempty"`
	Config     string               `yaml:"config,omitempty"`
	ReusedAt   time.Time            `yaml:"reused_at,omitempty"`
	ReusedFrom string               `yaml:"reused_from,omitempty"`
	Evidence   []Evidence           `yaml:"evidence,omitempty"`
	Criteria   map[string]Criterion `yaml:"criteria,omitempty"`
}

// Criterion is the stored result of evaluating one eval criterion, kept under
// the evals gate's Criteria map keyed by criterion id (artifact-schema.md, The
// eval model). Code, Config, and ReusedAt carry the same gate-reuse meaning as
// on Gate; a criterion's source is always its own node, so it has no
// ReusedFrom.
type Criterion struct {
	Status   string     `yaml:"status,omitempty"`
	Code     string     `yaml:"code,omitempty"`
	Config   string     `yaml:"config,omitempty"`
	ReusedAt time.Time  `yaml:"reused_at,omitempty"`
	Evidence []Evidence `yaml:"evidence,omitempty"`
}

// Evidence is a typed pointer to a stored artifact: a command output, a rubric
// output, a diff, a test reference, and so on (artifact-schema.md, Gates).
type Evidence struct {
	Kind string `yaml:"kind,omitempty"`
	Ref  string `yaml:"ref,omitempty"`
}

// Attestation is one human sign-off, the entry that clears a criterion's or a
// gate's needs_attestation. Its id is the AT- class minted by the compact token
// rule (artifact-schema.md, The eval model).
type Attestation struct {
	ID          string            `yaml:"id"`
	Target      AttestationTarget `yaml:"target,omitempty"`
	Status      string            `yaml:"status,omitempty"`
	By          string            `yaml:"by,omitempty"`
	At          time.Time         `yaml:"at,omitempty"`
	EvidenceRef string            `yaml:"evidence_ref,omitempty"`
}

// AttestationTarget names what an attestation attests: a criterion or a gate
// (artifact-schema.md, The eval model).
type AttestationTarget struct {
	Kind string `yaml:"kind,omitempty"` // criterion | gate
	ID   string `yaml:"id,omitempty"`
}

// PolicyViolation is the record of a required gate that was red at merge under
// advisory enforcement: the gate, the observed revision, its status, and the
// evidence (artifact-schema.md, Gates; cli-surface.md, The done crossing). The
// schema does not pin the field names; these mirror the prose description.
type PolicyViolation struct {
	Gate               string      `yaml:"gate,omitempty"`
	Observed           RevisionRef `yaml:"observed,omitempty"`
	Status             string      `yaml:"status,omitempty"`
	EvidenceRef        string      `yaml:"evidence_ref,omitempty"`
	SourceRevision     RevisionRef `yaml:"source_revision,omitempty"`
	UnresolvedCriteria []string    `yaml:"unresolved_criteria,omitempty"`
	EvidenceRefs       []string    `yaml:"evidence_refs,omitempty"`
}
