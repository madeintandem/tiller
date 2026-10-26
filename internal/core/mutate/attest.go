package mutate

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/gates"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// This file holds the attestation mutation the `tiller attest` verb drives
// (cli-surface.md, "Gates, evals, and attestation"). Attestation is the human
// judgment Tiller cannot produce: it sets an attestation-backed optional gate, or
// signs off a manual or required eval criterion that `tiller eval` reported pending.
// It appends one entry to the node's `attestations` list (an AT- id minted by the
// compact token rule) and satisfies the target by setting its status.
//
// This is the boundary the eval handoff defends: a required eval criterion is never
// satisfied by the agent's recorded rubric alone. `tiller eval --record-rubric-output`
// leaves it needs_attestation; only a human `tiller attest --criterion ... --status
// pass` here flips it to pass and clears that.

// AttestInput is the attest request: exactly one of Gate or Criterion names the
// target, Status is the human judgment (pass or fail), and Reason is the optional
// note recorded on the event.
type AttestInput struct {
	Gate      string
	Criterion string
	Status    string
	Reason    string
}

// AttestResult is the outcome the CLI renders: the node, the unchanged lifecycle
// status, the attested target and status, the minted attestation id, and whether
// the target is now satisfied (a pass attestation).
type AttestResult struct {
	Noun          string
	ID            string
	Key           string
	Status        string
	TargetKind    string
	TargetID      string
	AttestStatus  string
	AttestationID string
	Satisfied     bool
	Changed       bool
	// Warnings are advisory notes on a recorded attestation, today only an
	// attested gate key the manifest does not declare.
	Warnings []string
}

// Attestation target kinds (artifact-schema.md, The eval model).
const (
	attestTargetCriterion = "criterion"
	attestTargetGate      = "gate"
)

// Attest records a human attestation against a node and satisfies its target. It
// validates the request, resolves the acting contributor (the exit-3 refusal every
// mutation shares), loads the node's gates block, sets the target's status, appends
// the attestation entry, and writes the node with one event under the mutation lock.
func Attest(tree *store.Tree, deps Deps, node store.Node, in AttestInput) (AttestResult, error) {
	if err := validateAttestInput(in); err != nil {
		return AttestResult{}, err
	}
	if err := refuseCommandGateAttestation(tree.Manifest, in.Gate); err != nil {
		return AttestResult{}, err
	}
	if in.Criterion != "" && store.IsWorkNoun(node.Noun()) {
		return AttestResult{}, &usageError{
			reason: "unsupported_noun",
			msg:    fmt.Sprintf("attest --criterion does not run on a %s because a shelf node has no eval contract", node.Noun()),
		}
	}
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return AttestResult{}, err
	}
	if feature, ok := node.(*store.Feature); ok {
		if err := guardAttestBranch(tree, deps.rev(tree.Root), feature); err != nil {
			return AttestResult{}, err
		}
	}
	target, err := loadAttestTarget(tree, node)
	if err != nil {
		return AttestResult{}, err
	}

	att := store.Attestation{
		// AT ids are appended to the node's attestation list, not keyed into
		// the tree's id maps, so there is nothing to retry against
		// (artifact-schema.md, The eval model: the AT- and EV- classes are
		// separate non-node identifiers).
		ID:     deps.mintRaw("AT"),
		Status: in.Status,
		By:     owner.ID,
		At:     deps.clock().Now().UTC(),
	}

	var targetKind, targetID string
	var warnings []string
	if in.Criterion != "" {
		targetKind, targetID = attestTargetCriterion, in.Criterion
		if _, ok := findEvalCriterion(target.evalFile, in.Criterion); !ok {
			return AttestResult{}, &usageError{
				reason: "unknown_criterion",
				msg:    fmt.Sprintf("no criterion %q on %s %s", in.Criterion, target.noun, target.id),
			}
		}
		att.Target = store.AttestationTarget{Kind: attestTargetCriterion, ID: in.Criterion}
		att.EvidenceRef = criterionEvidenceRef(target.gates, in.Criterion)
		setCriterionStatus(target.gates, evalsGateRequired(tree.Manifest), in.Criterion, in.Status, owner.ID, att.At, target.evalFile)
	} else {
		targetKind, targetID = attestTargetGate, in.Gate
		att.Target = store.AttestationTarget{Kind: attestTargetGate, ID: in.Gate}
		setGateAttestationStatus(target.gates, in.Gate, in.Status, owner.ID, att.At, optionalGateRequired(tree.Manifest, in.Gate))
		if w := undeclaredGateWarning(tree.Manifest, in.Gate); w != "" {
			warnings = append(warnings, w)
		}
	}

	target.gates.Attestations = append(target.gates.Attestations, att)

	if err := saveEvalTarget(tree, deps, owner, target, "attest"); err != nil {
		return AttestResult{}, err
	}

	return AttestResult{
		Noun: target.noun, ID: target.id, Key: target.key, Status: target.status,
		TargetKind: targetKind, TargetID: targetID, AttestStatus: in.Status,
		AttestationID: att.ID, Satisfied: in.Status == store.GateStatusPass, Changed: true,
		Warnings: warnings,
	}, nil
}

func loadAttestTarget(tree *store.Tree, node store.Node) (*evalTarget, error) {
	work, ok := store.AsWorkNode(node)
	if !ok {
		return loadEvalTarget(tree, node)
	}
	doc, ymlRel, snapshot, err := loadWorkItemFile(tree, work)
	if err != nil {
		return nil, err
	}
	body := doc.Body()
	return &evalTarget{
		noun: doc.Noun(), id: body.ID, key: deref(body.Key), status: body.Status,
		gates: &body.GatesBlock, ymlRel: ymlRel, before: snapshot,
		save: func() ([]byte, error) { return yaml.Marshal(doc) },
	}, nil
}

// validateAttestInput rejects a missing or doubled target and a status outside the
// pass/fail set as misuse, before any work.
func validateAttestInput(in AttestInput) error {
	if (in.Gate == "") == (in.Criterion == "") {
		return &usageError{
			reason: "attest_target",
			msg:    "attest needs exactly one of --gate or --criterion",
		}
	}
	// The evals gate has no status of its own to attest: it is DERIVED from the
	// per-criterion map (gates.GateStatusFromCriteria), so hand-setting it writes
	// a value the next check recomputes away, over the top of the criteria that
	// are the real record. Sign the criteria instead.
	if in.Gate == "evals" {
		return &usageError{
			reason: "derived_gate",
			msg:    "the evals gate status is derived from its criteria: attest --criterion <C-id> instead",
		}
	}
	if in.Status != store.GateStatusPass && in.Status != store.GateStatusFail {
		return &usageError{
			reason: "attest_status",
			msg:    fmt.Sprintf("attest --status %q must be pass or fail", in.Status),
		}
	}
	return nil
}

// refuseCommandGateAttestation rejects a gate attestation aimed at a gate the
// current manifest declares as command-backed. Such a gate's result is the
// evidence `tiller check` records from a run; an attestation would replace a
// recorded failure with a verdict and flip the stored kind, so the run could
// never be told apart from a human's word. Attestation gates, and gate keys
// the manifest does not declare (recorded with undeclaredGateWarning), keep
// their behavior.
func refuseCommandGateAttestation(m *store.Manifest, name string) error {
	if m == nil || name == "" {
		return nil
	}
	spec, ok := m.Gates.Required[name]
	if !ok {
		spec, ok = m.Gates.Optional[name]
	}
	if !ok || spec.Kind != store.GateKindCommand {
		return nil
	}
	return &usageError{
		reason: "command_gate",
		msg:    fmt.Sprintf("gate %q is command-backed in tiller.yml: its result comes from `tiller check`, not an attestation", name),
	}
}

// setCriterionStatus sets a criterion's status inside the evals gate, preserving its
// recorded evidence (the rubric output stays as the trail behind the human's
// sign-off), creating the gate or criterion entry when attest reaches one eval never
// ran on, and recomputing the gate roll-up.
func setCriterionStatus(gb *store.GatesBlock, required bool, critID, status, runBy string, at time.Time, ef *store.EvalFile) {
	if gb.Gates == nil {
		gb.Gates = map[string]store.Gate{}
	}
	g, ok := gb.Gates["evals"]
	if !ok {
		g = store.Gate{Required: required, Kind: store.GateKindCommand}
	}
	if g.Criteria == nil {
		g.Criteria = map[string]store.Criterion{}
	}
	c := g.Criteria[critID]
	c.Status = status
	g.Criteria[critID] = c
	g.RunBy = runBy
	g.RunAt = at
	g.Status = gates.GateStatusFromCriteria(ef, g.Criteria)
	gb.Gates["evals"] = g
}

// setGateAttestationStatus sets an attestation-backed gate's status, creating the
// gate entry when this attestation is its first signal (Tiller never runs the
// reviewer behind it).
func setGateAttestationStatus(gb *store.GatesBlock, name, status, by string, at time.Time, required bool) {
	if gb.Gates == nil {
		gb.Gates = map[string]store.Gate{}
	}
	g := gb.Gates[name]
	g.Required = required
	g.Kind = store.GateKindAttestation
	g.Status = status
	g.RunBy = by
	g.RunAt = at
	gb.Gates[name] = g
}

// criterionEvidenceRef returns the rubric evidence already recorded on a criterion
// so the attestation points back at what the human reviewed. It returns "" when no
// artifact exists (a manual criterion, or an eval criterion whose rubric output was
// never recorded): an attestation never references a file that was not created, so a
// manual sign-off carries no evidence_ref rather than a dangling rubric-output path
// (cli-surface.md, "Gates, evals, and attestation").
func criterionEvidenceRef(gb *store.GatesBlock, critID string) string {
	if gb.Gates != nil {
		if g, ok := gb.Gates["evals"]; ok {
			if c, ok := g.Criteria[critID]; ok && len(c.Evidence) > 0 {
				return c.Evidence[0].Ref
			}
		}
	}
	return ""
}

// findEvalCriterion reports whether the eval file carries the named criterion.
func findEvalCriterion(ef *store.EvalFile, id string) (store.EvalCriterion, bool) {
	if ef == nil {
		return store.EvalCriterion{}, false
	}
	for _, c := range ef.Criteria {
		if c.ID == id {
			return c, true
		}
	}
	return store.EvalCriterion{}, false
}

// optionalGateRequired reports whether the named gate is a required gate in the
// manifest; an attestation-backed gate is normally optional, so this is false for
// the review gates, but a manifest may mark one required.
func optionalGateRequired(m *store.Manifest, name string) bool {
	if m == nil {
		return false
	}
	_, ok := m.Gates.Required[name]
	return ok
}

// undeclaredGateWarning names an attested gate key the manifest does not
// declare, with the declared keys beside it, or returns "" for a declared gate.
// The attestation is still recorded, since unknown gates are deliberately
// preserved, but a misspelling such as code-review for the seeded code_review
// otherwise records a second gate nobody configured while the declared one stays
// unattested. The derived evals gate is left out of the list because attest
// refuses it.
func undeclaredGateWarning(m *store.Manifest, name string) string {
	if m == nil {
		return ""
	}
	if _, ok := m.Gates.Optional[name]; ok {
		return ""
	}
	if _, ok := m.Gates.Required[name]; ok {
		return ""
	}
	var declared []string
	for k := range m.Gates.Optional {
		declared = append(declared, k)
	}
	for k := range m.Gates.Required {
		if k != "evals" {
			declared = append(declared, k)
		}
	}
	if len(declared) == 0 {
		return fmt.Sprintf("gate %q is not declared in tiller.yml, which declares no attestable gates; recorded anyway", name)
	}
	sort.Strings(declared)
	return fmt.Sprintf("gate %q is not declared in tiller.yml (declared: %s); recorded anyway", name, strings.Join(declared, ", "))
}
