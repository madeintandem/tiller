package gates

// This file holds the eval handoff: the portable contract between Tiller and the
// agent that grades an `eval`-binding acceptance criterion (artifact-schema.md,
// "The eval model"; cli-surface.md, "Gates, evals, and attestation"). Tiller does
// not reason and does not grade. It reads the criteria, runs the mechanical ones,
// emits a rubric packet for the AI-graded ones, validates the agent's recorded
// output, and records that output as evidence that still needs human attestation.
//
// SIGNAL (spec gap): the rubric packet and rubric output JSON shapes below were a
// Tier-2 "resolve while building" item and are NOT pinned in artifact-schema.md as
// of this writing. They are introduced here as the minimal portable contract, with
// stable schema ids (tiller.rubric_packet.v1 / tiller.rubric_output.v1), the same
// way the JSON envelope and `init --owner` were surfaced from the build and folded
// back into the specs. They want codifying into artifact-schema.md ("The eval
// model").
//
// Layering: this package imports only internal/store. The orchestration that reads
// a node's eval file, records the gates block, and writes an event lives in
// internal/core/mutate; the CLI shell is internal/cli.

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// Schema identifiers for the eval handoff payloads. A consumer branches on these
// rather than guessing the shape, and a future revision bumps the trailing
// version rather than mutating fields.
const (
	RubricPacketSchema = "tiller.rubric_packet.v1"
	RubricOutputSchema = "tiller.rubric_output.v1"
)

// Rubric verdicts the agent may record. A verdict is the agent's machine grade,
// not an approval: a pass verdict moves a required criterion to needs_attestation
// (awaiting a human), never straight to pass; a fail verdict is recorded as a red
// criterion the human can see.
const (
	RubricVerdictPass = "pass"
	RubricVerdictFail = "fail"
)

// evalEvidenceDir is the repo-relative directory rubric output lands in, matching
// the schema's example evidence ref artifacts/tiller/evals/F-79dh3q-C3.json
// (artifact-schema.md, "The eval model").
const evalEvidenceDir = "artifacts/tiller/evals"

// RubricPacket is the portable handoff `tiller eval --emit-rubrics` writes for the
// agent to grade with its harness model. It names the node, the time it was
// generated, and one entry per `eval`-binding criterion. The agent grades each
// criterion, writes a RubricOutput to the criterion's OutputPath, and records it
// with `tiller eval <noun> <node> --criterion <id> --record-rubric-output <file>`.
type RubricPacket struct {
	Schema      string            `json:"schema"`
	Node        string            `json:"node"`
	Noun        string            `json:"noun"`
	GeneratedAt time.Time         `json:"generated_at"`
	Criteria    []RubricCriterion `json:"criteria"`
}

// RubricCriterion is one criterion the agent must grade. Rubric carries the
// grading guidance from the binding (its inline `run` text, or its `ref`),
// OutputPath is where the agent writes the graded RubricOutput so the record step
// can find it.
type RubricCriterion struct {
	ID         string `json:"id"`
	Statement  string `json:"statement,omitempty"`
	Polarity   string `json:"polarity,omitempty"`
	Required   bool   `json:"required"`
	Rubric     string `json:"rubric,omitempty"`
	OutputPath string `json:"output_path"`
}

// RubricOutput is the agent's graded result for one eval criterion, the payload
// `--record-rubric-output` validates and stores as evidence. It is evidence, never
// approval: it can move a required criterion to needs_attestation but never to
// pass. Only a human `tiller attest --criterion` clears that.
type RubricOutput struct {
	Schema    string    `json:"schema"`
	Node      string    `json:"node,omitempty"`
	Criterion string    `json:"criterion"`
	Verdict   string    `json:"verdict"`
	Rationale string    `json:"rationale,omitempty"`
	GradedBy  string    `json:"graded_by,omitempty"`
	GradedAt  time.Time `json:"graded_at,omitempty"`
}

// EvidencePath is the repo-relative path a node's rubric output for one criterion
// is written to: artifacts/tiller/evals/<node-id>-<criterion-id>.json.
func EvidencePath(nodeID, critID string) string {
	return evalEvidenceDir + "/" + nodeID + "-" + critID + ".json"
}

// BuildPacket assembles the rubric packet for a node from its eval file, emitting
// one entry per `eval`-binding criterion in the file's order. A node with no eval
// criteria yields a packet with an empty (non-nil) criteria list, so the JSON
// renders `"criteria": []` rather than null.
func BuildPacket(noun, nodeID string, ef *store.EvalFile, at time.Time) RubricPacket {
	crits := make([]RubricCriterion, 0)
	if ef != nil {
		for _, c := range ef.Criteria {
			if c.Binding.Type != store.BindingEval {
				continue
			}
			crits = append(crits, RubricCriterion{
				ID:         c.ID,
				Statement:  c.Statement,
				Polarity:   c.Polarity,
				Required:   c.Required,
				Rubric:     rubricText(c.Binding),
				OutputPath: EvidencePath(nodeID, c.ID),
			})
		}
	}
	return RubricPacket{
		Schema:      RubricPacketSchema,
		Node:        nodeID,
		Noun:        noun,
		GeneratedAt: at.UTC(),
		Criteria:    crits,
	}
}

// rubricText pulls the grading guidance out of an eval binding: the inline `run`
// text if present, otherwise the `ref` pointing at a rubric file.
func rubricText(b store.EvalBinding) string {
	if b.Run != "" {
		return b.Run
	}
	return b.Ref.Joined()
}

// ParseRubricOutput decodes a recorded rubric output document, rejecting malformed
// JSON as a misuse (exit 2) the agent must fix.
func ParseRubricOutput(data []byte) (RubricOutput, error) {
	var out RubricOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return RubricOutput{}, &RubricOutputError{
			reason: "rubric_output_malformed",
			msg:    fmt.Sprintf("rubric output is not valid JSON: %v", err),
		}
	}
	return out, nil
}

// ValidateRubricOutput checks a recorded rubric output against the schema and the
// node's eval contract: the schema id must match, the named criterion must be an
// `eval`-binding criterion on this node, the node field (when present) must match,
// and the verdict must be pass or fail. A bad output is a misuse (exit 2).
func ValidateRubricOutput(out RubricOutput, nodeID string, ef *store.EvalFile) error {
	if out.Schema != RubricOutputSchema {
		return &RubricOutputError{
			reason: "rubric_output_schema",
			msg:    fmt.Sprintf("rubric output schema %q is not %q", out.Schema, RubricOutputSchema),
		}
	}
	if out.Node != "" && out.Node != nodeID {
		return &RubricOutputError{
			reason: "rubric_output_node",
			msg:    fmt.Sprintf("rubric output names node %q, not %q", out.Node, nodeID),
		}
	}
	if out.Verdict != RubricVerdictPass && out.Verdict != RubricVerdictFail {
		return &RubricOutputError{
			reason: "rubric_output_verdict",
			msg:    fmt.Sprintf("rubric output verdict %q must be pass or fail", out.Verdict),
		}
	}
	c, ok := findCriterion(ef, out.Criterion)
	if !ok {
		return &RubricOutputError{
			reason: "rubric_output_criterion",
			msg:    fmt.Sprintf("no criterion %q on node %s", out.Criterion, nodeID),
		}
	}
	if c.Binding.Type != store.BindingEval {
		return &RubricOutputError{
			reason: "rubric_output_criterion",
			msg:    fmt.Sprintf("criterion %s is a %s binding, not an eval binding", out.Criterion, c.Binding.Type),
		}
	}
	return nil
}

// RecordedCriterionStatus maps a recorded rubric verdict onto the stored criterion
// status. A pass verdict lands as needs_attestation, because an agent-recorded eval
// result never satisfies a required criterion on its own; only a human attest flips
// it to pass. A fail verdict is recorded as a red criterion.
func RecordedCriterionStatus(verdict string) string {
	if verdict == RubricVerdictPass {
		return store.CriterionStatusNeedsAttestation
	}
	return store.CriterionStatusFail
}

// CriterionSatisfied reports whether a stored criterion is clean: only an explicit
// pass satisfies it. pending, needs_attestation, stale, and fail are all unsatisfied.
func CriterionSatisfied(c store.Criterion) bool {
	return c.Status == store.CriterionStatusPass
}

// CriteriaRun is what one EvaluateCriteria call needs beyond the contract:
// the short revision the run is pinned to (it names evidence files), the
// current code identity, whether a passed command or test criterion may be
// reused (the caller folds --rerun and the evals gate's manifest `reuse:` into
// it), the clock instant a reused criterion is stamped with, and the optional
// progress sink.
type CriteriaRun struct {
	Against  string
	Code     string
	Reuse    bool
	At       time.Time
	Progress Progress
}

// EvaluateCriteria runs the mechanical part of every criterion in the eval file
// and returns the per-criterion status map for the node's `evals` gate. command
// and test bindings are run via RunGate and recorded only after their evidence is
// durable. A persistence error stops evaluation without an entry for that
// criterion. Manual and eval bindings retain their existing behavior.
// run.Progress is the same optional progress sink RunGate takes; each
// eval-binding criterion reports under its GateConfig name (eval-<criterion id>),
// so the line names the criterion for free.
//
// A command or test criterion whose prior record is a pass over the same code
// identity under the same resolved command is reused instead of run: the copy
// keeps its original evidence and adds reused_at (design
// 2026-10-21-gate-reuse). A fresh run records the code identity and config hash
// it ran under. Reuse needs run.Reuse and a non-empty run.Code; an empty
// identity never matches. Manual and eval criteria are never reused or re-keyed.
func EvaluateCriteria(
	ef *store.EvalFile,
	prior map[string]store.Criterion,
	m *store.Manifest,
	root string,
	run CriteriaRun,
) (map[string]store.Criterion, error) {
	out := map[string]store.Criterion{}
	if ef == nil {
		return out, nil
	}
	testsRunner := requiredTestsRunner(m)

	for _, c := range ef.Criteria {
		switch c.Binding.Type {
		case store.BindingCommand, store.BindingTest:
			cmd := criterionCommand(c.Binding, testsRunner)
			if cmd == "" {
				// Nothing runnable (a bare test ref with no suite runner): leave it
				// pending for the whole-suite tests gate to exercise, noting the ref.
				out[c.ID] = store.Criterion{
					Status:   store.CriterionStatusPending,
					Evidence: refEvidence(c.Binding),
				}
				continue
			}
			config := ConfigHash(store.GateKindCommand, cmd)
			if hit, ok := reusableCriterion(prior[c.ID], run, config); ok {
				hit.ReusedAt = run.At
				out[c.ID] = hit
				continue
			}
			res, err := RunGate(GateConfig{Name: "eval-" + c.ID, Run: cmd, Against: run.Against}, root, run.Progress)
			if err != nil {
				return out, err
			}
			out[c.ID] = store.Criterion{
				Status:   res.Status,
				Code:     run.Code,
				Config:   config,
				Evidence: []store.Evidence{{Kind: "command_output", Ref: res.EvidenceRef}},
			}
		case store.BindingManual:
			if keepPrior(prior, c.ID) {
				out[c.ID] = prior[c.ID]
				continue
			}
			out[c.ID] = store.Criterion{Status: manualPending(c.Required)}
		case store.BindingEval:
			if keepPrior(prior, c.ID) {
				out[c.ID] = prior[c.ID]
				continue
			}
			out[c.ID] = store.Criterion{Status: store.CriterionStatusPending}
		default:
			out[c.ID] = store.Criterion{Status: store.CriterionStatusPending}
		}
	}
	return out, nil
}

// CriteriaCurrent reports whether a recorded criteria map still holds under the
// reuse key (code identity plus config hash): every command or test criterion
// in ef that is runnable here and recorded pass carries a non-empty code
// identity equal to code and the config hash of the command it would resolve to
// now. A test binding resolves through the manifest's tests runner, so moving
// the runner retires its pass even on unchanged code. Criteria recorded other
// than pass, and manual or eval bindings, are not keyed and do not decide. A
// check that holds the evals gate out under --gate counts its record only when
// this holds, so it never stands on a pass EvaluateCriteria would not reuse.
func CriteriaCurrent(ef *store.EvalFile, criteria map[string]store.Criterion, m *store.Manifest, code string) bool {
	if ef == nil {
		return true
	}
	testsRunner := requiredTestsRunner(m)
	for _, c := range ef.Criteria {
		if c.Binding.Type != store.BindingCommand && c.Binding.Type != store.BindingTest {
			continue
		}
		cmd := criterionCommand(c.Binding, testsRunner)
		if cmd == "" {
			continue
		}
		rec, ok := criteria[c.ID]
		if !ok || rec.Status != store.CriterionStatusPass {
			continue
		}
		if _, reusable := reusableCriterion(rec, CriteriaRun{Code: code, Reuse: true}, ConfigHash(store.GateKindCommand, cmd)); !reusable {
			return false
		}
	}
	return true
}

// StaleRerunCriteria returns a copy of criteria with stale set on exactly the
// criteria a full check under run would rerun: each command or test criterion
// in ef that resolves to a command here and is not reusable (reusableCriterion).
// Manual and eval-kind criteria, unrunnable bindings, and records the contract
// no longer names keep their status, as EvaluateCriteria keeps a human-settled
// verdict, so a check that holds a non-current evals gate out under --gate
// stales the gate without discarding a human's acceptance.
func StaleRerunCriteria(ef *store.EvalFile, criteria map[string]store.Criterion, m *store.Manifest, run CriteriaRun) map[string]store.Criterion {
	if criteria == nil {
		return nil
	}
	out := make(map[string]store.Criterion, len(criteria))
	for id, c := range criteria {
		out[id] = c
	}
	if ef == nil {
		return out
	}
	testsRunner := requiredTestsRunner(m)
	for _, c := range ef.Criteria {
		if c.Binding.Type != store.BindingCommand && c.Binding.Type != store.BindingTest {
			continue
		}
		cmd := criterionCommand(c.Binding, testsRunner)
		rec, ok := out[c.ID]
		if cmd == "" || !ok {
			continue
		}
		if _, reusable := reusableCriterion(rec, run, ConfigHash(store.GateKindCommand, cmd)); reusable {
			continue
		}
		rec.Status = store.CriterionStatusStale
		out[c.ID] = rec
	}
	return out
}

// reusableCriterion returns the prior criterion when it may stand in for a
// fresh run: reuse is on, the current code identity is non-empty, and the prior
// record is a pass whose stored code identity and config hash both equal the
// current ones. A record written before this build carries neither field and
// never matches.
func reusableCriterion(prior store.Criterion, run CriteriaRun, config string) (store.Criterion, bool) {
	if !run.Reuse || run.Code == "" {
		return store.Criterion{}, false
	}
	if prior.Status != store.CriterionStatusPass || prior.Code != run.Code || prior.Config != config {
		return store.Criterion{}, false
	}
	return prior, true
}

// GateStatusFromCriteria derives the `evals` gate's roll-up status from its
// per-criterion map over the required criteria: any required criterion that failed
// makes the gate fail; all required criteria satisfied makes it pass; anything in
// between (a pending, needs_attestation, or stale required criterion) is pending.
// A stale criterion dropping the gate to pending rather than pass is what keeps a
// reopen from certifying itself on a result that predates the new work. Only the
// required criteria gate completion (artifact-schema.md, "The eval model").
func GateStatusFromCriteria(ef *store.EvalFile, criteria map[string]store.Criterion) string {
	if ef == nil {
		return store.GateStatusPending
	}
	allPass := true
	for _, c := range ef.Criteria {
		if !c.Required {
			continue
		}
		st := criteria[c.ID].Status
		if st == store.CriterionStatusFail {
			return store.GateStatusFail
		}
		if st != store.CriterionStatusPass {
			allPass = false
		}
	}
	if allPass {
		return store.GateStatusPass
	}
	return store.GateStatusPending
}

// RequiredFailed reports whether any required criterion failed, the signal the eval
// command turns into a blocked exit 1. A pending, needs_attestation, or stale
// required criterion is "not yet", not "red", so it does not block.
func RequiredFailed(ef *store.EvalFile, criteria map[string]store.Criterion) bool {
	if ef == nil {
		return false
	}
	for _, c := range ef.Criteria {
		if c.Required && criteria[c.ID].Status == store.CriterionStatusFail {
			return true
		}
	}
	return false
}

// AllRequiredSatisfied reports whether every required criterion is an explicit
// pass, the honest signal the eval command turns into a passed:true / exit 0. A
// required criterion that is pending, needs_attestation, stale, or failed leaves
// the run unsatisfied: acceptance is not met until a human attests it. A nil
// contract has no required criteria and is vacuously satisfied, the clean no-op
// an unspecced feature gets. This is the positive complement of RequiredFailed:
// passing requires all-green, not merely the absence of red.
func AllRequiredSatisfied(ef *store.EvalFile, criteria map[string]store.Criterion) bool {
	if ef == nil {
		return true
	}
	for _, c := range ef.Criteria {
		if !c.Required {
			continue
		}
		if criteria[c.ID].Status != store.CriterionStatusPass {
			return false
		}
	}
	return true
}

// findCriterion returns the named criterion from the eval file.
func findCriterion(ef *store.EvalFile, id string) (store.EvalCriterion, bool) {
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

// requiredTestsRunner returns the manifest's required `tests` gate command, the
// runner a bare test-binding ref is composed onto. Empty when no such gate exists.
func requiredTestsRunner(m *store.Manifest) string {
	if m == nil {
		return ""
	}
	if spec, ok := m.Gates.Required["tests"]; ok {
		return spec.Run
	}
	return ""
}

// criterionCommand returns the shell command that exercises a command or test
// binding: the binding's own `run` when present, otherwise the project tests
// runner composed with the test `ref`. Empty means nothing is runnable here.
func criterionCommand(b store.EvalBinding, testsRunner string) string {
	if b.Run != "" {
		return b.Run
	}
	if b.Type == store.BindingTest && !b.Ref.Empty() && testsRunner != "" {
		return testsRunner + " " + b.Ref.Joined()
	}
	return ""
}

// refEvidence records each of a test binding's refs as pending evidence when
// Tiller has no way to run them on its own.
func refEvidence(b store.EvalBinding) []store.Evidence {
	evidence := make([]store.Evidence, 0, len(b.Ref))
	for _, ref := range b.Ref {
		evidence = append(evidence, store.Evidence{Kind: "test", Ref: ref})
	}
	if len(evidence) == 0 {
		return nil
	}
	return evidence
}

// manualPending is the default status for a manual criterion: needs_attestation
// when required (a human must sign off before the node is clean), pending
// otherwise.
func manualPending(required bool) string {
	if required {
		return store.CriterionStatusNeedsAttestation
	}
	return store.CriterionStatusPending
}

// keepPrior reports whether a re-evaluation should preserve a prior criterion
// status rather than recompute it: a human-settled pass or a recorded
// needs_attestation is not downgraded.
func keepPrior(prior map[string]store.Criterion, id string) bool {
	c, ok := prior[id]
	if !ok {
		return false
	}
	return c.Status == store.CriterionStatusPass || c.Status == store.CriterionStatusNeedsAttestation
}

// RubricOutputError is a malformed-or-mismatched rubric output: a misuse (exit 2)
// the agent fixes by regrading. It carries a stable reason for the JSON envelope,
// surfaced by the CLI classifier through the ExitCoder and reasoner interfaces.
type RubricOutputError struct {
	reason string
	msg    string
}

func (e *RubricOutputError) Error() string  { return e.msg }
func (e *RubricOutputError) ExitCode() int  { return 2 }
func (e *RubricOutputError) Reason() string { return e.reason }
