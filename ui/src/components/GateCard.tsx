// GateCard renders one recorded gate (mock round 4's .gate): its name, a
// required/review tag, and a derived state line (lib/gateDisplay.ts), over
// either a criterion-by-criterion breakdown (the evals gate, decorated with
// the authored eval contract so criteria read before they run) or, for a
// gate with no per-criterion breakdown (build, tests, an attestation-backed
// review gate), its own recorded evidence. The console reads these; it never
// computes or grades them.
//
// GateCardList is the page-level entry point every noun's body calls with its
// whole gates record and eval payload: it renders the evals card (contract-
// decorated when the contract is well-formed, the served {path, error}
// verbatim in a quiet warning block when it is not - never hiding a malformed
// contract - a bare EmptyNote when the contract is well-formed but authored
// with zero criteria (JC's ruling, 2026-08-11: emptiness is information) - or
// a flat card when a result exists with no contract at all), then every
// other recorded gate in name order.

import { isEvalError } from "../api";
import type {
  AttestationView,
  EvalContractError,
  EvalCriterionView,
  EvalPayload,
  GateView,
  EvidenceView,
  NodeView,
} from "../api";
import { deriveEvalsGateState, deriveFlatGateState } from "../lib/gateDisplay";
import { has } from "../lib/has";
import CriterionRow from "./CriterionRow";
import EmptyNote from "./EmptyNote";
import StateDot from "./StateDot";

function GateHeader({
  name,
  required,
  dotStatus,
  label,
}: {
  name: string;
  required: boolean;
  dotStatus: string;
  label: string;
}) {
  return (
    <div className="instr-gate-head">
      {/* The gate name is a name, not a key, a path, or a command, so it is
          not mono: the mock's own .gname is 13.5px sans at 650 (mock:154),
          which instrument.css already reproduces exactly. */}
      <span className="instr-gate-name">{name}</span>
      <span className="instr-gate-tag">{required ? "required gate" : "review gate"}</span>
      {/* The derived state paints the line as well as the dot: quiet slate
          unless something here is the human's (mock note 5, "Green is quiet,
          amber is yours"). instrument.css holds the mapping. */}
      <span className="instr-gate-state" data-state={dotStatus}>
        <StateDot status={dotStatus} size={8} />
        {label}
      </span>
    </div>
  );
}

// EvidenceRow is a gate-level evidence entry (a command gate's own captured
// output, not a per-criterion one): the same verbatim-ref, new-tab link
// CriterionRow's evidence fact uses, kept to one shared shape across both
// grains.
function EvidenceRow({ kind, ref, url, unavailable }: EvidenceView) {
  if (ref === undefined || ref === "") {
    return null;
  }
  return (
    <div className="instr-gate-evidence">
      <span className="muted">{kind ?? "evidence"}</span>
      {unavailable !== undefined ? (
        <span className="mono">{ref} · Evidence unavailable: {unavailable}</span>
      ) : (
      <a
        className="instr-evid mono"
        href={url ?? `/api/evidence?ref=${encodeURIComponent(ref)}`}
        target="_blank"
        rel="noreferrer"
      >
        {ref}
      </a>
      )}
    </div>
  );
}


// GateCard's criteria branch decorates the authored contract's criteria
// (file order, never re-sorted) with the recorded per-criterion result,
// looked up by id rather than iterating the gate's own criteria map, so the
// contract - statement, binding, required - always reads even when nothing
// has run yet and the map is empty or absent.
export default function GateCard({
  name,
  gate,
  criteria,
  attestations = [],
}: {
  name: string;
  gate?: GateView;
  criteria?: EvalCriterionView[];
  attestations?: AttestationView[];
}) {
  if (criteria !== undefined) {
    const { dotStatus, label } = deriveEvalsGateState(criteria, gate);
    const required = gate?.required ?? true;
    return (
      <div className="instr-gate">
        <GateHeader name={name} required={required} dotStatus={dotStatus} label={label} />
        {has(gate?.against) && (
          <div className="instr-gate-against">
            <span className="muted">checked against</span>
            <span className="mono">{gate.against}</span>
          </div>
        )}
        {/* .criteria is the contract's own grid (task 17, the mock's .crow
            rows), the same class the Contract tab reaches for whether it
            arrives through this card's own criteria branch or nowhere at
            all - one grammar for a criterion row, never two. */}
        <div className="criteria">
          {criteria.map((c) => (
            <CriterionRow
              key={c.id}
              criterion={c}
              result={gate?.criteria?.[c.id]}
              attestations={attestations}
            />
          ))}
        </div>
      </div>
    );
  }

  const { dotStatus, label } = deriveFlatGateState(gate);
  const required = gate?.required ?? false;
  const evidence = gate?.evidence ?? [];
  return (
    <div className="instr-gate">
      <GateHeader name={name} required={required} dotStatus={dotStatus} label={label} />
      {has(gate?.against) && (
        <div className="instr-gate-against">
          <span className="muted">checked against</span>
          <span className="mono">{gate.against}</span>
        </div>
      )}
      {evidence.length > 0 && (
        <div className="instr-gate-evidence-list">
          {evidence.map((e, i) => (
            <EvidenceRow key={i} {...e} />
          ))}
        </div>
      )}
    </div>
  );
}

// EvalContractWarning is the malformed-contract degradation (never hide a
// spec-validation failure): the served path and error rendered verbatim,
// nothing reinterpreted or partially shown.
export function EvalContractWarning({ error }: { error: EvalContractError }) {
  return (
    <div className="instr-warning">
      <p className="mono">{error.path}</p>
      <p className="error">{error.error}</p>
    </div>
  );
}

function otherGateNames(gates: Record<string, GateView> | undefined): string[] {
  return Object.keys(gates ?? {})
    .filter((n) => n !== "evals")
    .sort();
}

export function GateCardList({ node, evalPayload }: { node: NodeView; evalPayload: EvalPayload }) {
  const gates = node.gates;
  const evalsGate = gates?.evals;
  const others = otherGateNames(gates);

  // A fresh draft with no authored contract and no recorded gate draws
  // nothing. The note that used to stand here was a bare <p> with no section
  // and no heading of its own, so on a stub feature it landed directly under
  // the prose body's last heading and read as that heading's content: an
  // empty-state line masquerading as spec text. Absence states this better
  // than a parenthetical, and a gate that is required but unmet is NOT this
  // case: it arrives as a real GateView and renders its own pending card.
  if (evalPayload === null && evalsGate === undefined && others.length === 0) {
    return null;
  }

  // A well-formed contract with zero criteria (JC's ruling, 2026-08-11): the
  // Contract tab now stays visible for this case (NodeDetail.tsx's
  // contractCount), so this is the tab's own honest empty-state body rather
  // than routing an empty criteria array through GateCard, whose state line
  // speaks about criteria a contract with nothing to check does not have.
  const wellFormed = !isEvalError(evalPayload) && evalPayload !== null;
  const authoredEmpty = wellFormed && evalPayload.criteria.length === 0;

  return (
    <>
      {isEvalError(evalPayload) && <EvalContractWarning error={evalPayload} />}
      {authoredEmpty && <EmptyNote>The contract is authored but carries no criteria.</EmptyNote>}
      {wellFormed && !authoredEmpty && (
        <GateCard
          name="evals"
          gate={evalsGate}
          criteria={evalPayload.criteria}
          attestations={node.attestations ?? []}
        />
      )}
      {!isEvalError(evalPayload) && evalPayload === null && evalsGate !== undefined && (
        <GateCard name="evals" gate={evalsGate} />
      )}
      {others.map((name) => (
        <GateCard key={name} name={name} gate={gates?.[name]} />
      ))}
    </>
  );
}
