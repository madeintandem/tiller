// IntegrityNotes renders the two rarer integrity records a node can carry
// once a human has actually acted on it: recorded attestations (a human's own
// sign-off on a review gate or a criterion) and merge-time policy violations
// (a required gate that was not green when the node crossed to done under
// advisory enforcement). Both are a straight readout of the working tree,
// same as everything else on this page, and each renders only when its list
// is non-empty - a fresh draft carries neither.

import type { AttestationView, PolicyViolationView } from "../api";
import { has } from "../lib/has";
import { PersonName } from "../people";

// Mono carries the four things it is for on these rows and nothing else: the
// attestation's id, its target id, and the evidence ref, which are addresses.
// The recorded time, the status word, the gate's name, and the observed status
// are read, not copied, so they take the sans voice the mock gives a gate name
// (.gname, mock:154) and its caption text (.gfoot, mock:167).
function AttestationRow({ attestation: a }: { attestation: AttestationView }) {
  const target = [a.target_kind, a.target_id].filter((p) => has(p)).join(" ");
  return (
    <li className="instr-integrity-row">
      <span className="mono">{a.id}</span>
      {target !== "" && <span className="mono muted">{target}</span>}
      <span>{a.status ?? ""}</span>
      {has(a.by) && <span className="muted">by <PersonName id={a.by} /></span>}
      {has(a.at) && <span className="muted integrity-at">{a.at}</span>}
      {has(a.evidence_ref) && <span className="mono muted">{a.evidence_ref}</span>}
    </li>
  );
}

function ViolationRow({ violation: v }: { violation: PolicyViolationView }) {
  return (
    <li className="instr-integrity-row">
      <span className="instr-integrity-name">{v.gate ?? ""}</span>
      <span>{v.status ?? ""}</span>
      {has(v.observed) && <span className="muted">observed {v.observed}</span>}
      {has(v.evidence_ref) && <span className="mono muted">{v.evidence_ref}</span>}
      {has(v.source_revision) && <span className="mono muted">historical source {v.source_revision}</span>}
      {(v.unresolved_criteria?.length ?? 0) > 0 && <span>unresolved criteria: {v.unresolved_criteria?.join(", ")}</span>}
      {(v.evidence_refs?.length ?? 0) > 0 && <span className="mono muted">evidence: {v.evidence_refs?.join(", ")}</span>}
    </li>
  );
}

export default function IntegrityNotes({
  attestations,
  violations,
}: {
  attestations: AttestationView[];
  violations: PolicyViolationView[];
}) {
  if (attestations.length === 0 && violations.length === 0) {
    return null;
  }
  return (
    <div className="instr-integrity">
      {attestations.length > 0 && (
        <>
          <h3>Attestations</h3>
          <ul className="instr-integrity-list">
            {attestations.map((a) => (
              <AttestationRow key={a.id} attestation={a} />
            ))}
          </ul>
        </>
      )}
      {violations.length > 0 && (
        <>
          <h3>Policy violations</h3>
          <ul className="instr-integrity-list">
            {violations.map((v, i) => (
              <ViolationRow key={i} violation={v} />
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
