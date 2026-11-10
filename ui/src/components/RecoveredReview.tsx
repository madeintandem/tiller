import Time from "./Time";
import TitleRef from "./TitleRef";
import type { CurrentAcceptanceView, GateView, RecoveredReviewView, WorkflowWork } from "../api";
import { evidenceLabel } from "../lib/evidenceLabel";
import { recoveryBasisLabel } from "../lib/executionWords";
import { statusLabel } from "../lib/statusLabel";

// The PR reads as a link when the console knows its address (a GitHub origin);
// the label keeps the honest wording that the number was caller-supplied.
function prLabel(record: RecoveredReviewView) {
  if (record.pr === undefined) return "Local review ref";
  const label = `Caller-supplied PR #${record.pr}`;
  if (record.pr_url === undefined || record.pr_url === "") return label;
  return <a href={record.pr_url} target="_blank" rel="noreferrer" aria-label={`${label} (opens the pull request)`}>{label}</a>;
}

// Historical provenance is not current approval or a host observation.
// The body leads with the reason, the PR, and the riding tasks; the recorded
// head, integration result, basis, and policy snapshots sit in a closed
// Details disclosure (native <details>, so its open state is not stored).
// titles maps a riding task's id to its title; a task with no known title
// shows its id.
export default function RecoveredReview({ record, titles, acceptanceReady, outstanding, gates, crossingsPending, currentAcceptance, historicalViolations, crossingViolations }: {
  record: RecoveredReviewView;
  titles?: ReadonlyMap<string, string>;
  acceptanceReady?: boolean;
  outstanding?: WorkflowWork[];
  gates?: Record<string, GateView>;
  crossingsPending?: boolean;
  currentAcceptance?: CurrentAcceptanceView[];
  historicalViolations?: number;
  crossingViolations?: number;
}) {
  return (
    <section className="abox warn" aria-label="Recovered historical review">
      <span className="kind">Recovered historical review</span>
      <p>Recorded by {record.recovered_by} <Time iso={record.recovered_at} />: {record.reason}</p>
      <p>{prLabel(record)} · <span className="mono">{record.branch}</span></p>
      <div>
        <span className="kind">Historical riding tasks</span>
        <ul>{record.tasks.map((task) => (
          <li key={task.id}>
            <TitleRef node={{ noun: "task", id: task.id, ref: task.id }} title={titles?.get(task.id)} addr={task.id} />
          </li>
        ))}</ul>
      </div>
      <details className="recovered-fold">
        <summary>Details</summary>
        <p>Historical head: <code>{record.head}</code></p>
        <p>Integration result: <code>{record.merge_commit}</code> on {record.integration_branch}</p>
        <p>Evidence basis: {recoveryBasisLabel(record.basis)}{record.basis === "contained" && (crossingsPending === true
          ? "; not an observed merge; pending historical riders require the human fast-forward decision"
          : crossingsPending === false
            ? "; containment was not an observed merge; historical riders have crossed and need no further fast-forward assertion"
            : "; containment at recovery was not an observed merge; only still-pending historical riders need a human fast-forward assertion")}</p>
        <ul>{record.tasks.map((task) => (
          <li key={task.id}>{titles?.get(task.id) ?? task.id} · <span className="mono">{task.path}</span> · submitted state <code>{task.state}</code></li>
        ))}</ul>
        <span className="kind">Historical policy · recorded state, not a new check run</span>
        {record.policy_snapshots.map((snapshot) => (
          <div key={`${snapshot.node_id ?? "manifest"}:${snapshot.path}:${snapshot.source}`}>
            <p>{snapshot.node_id ?? "Manifest"} · <code>{snapshot.source}</code> · {snapshot.path}</p>
            {snapshot.observations.length === 0
              ? <p className="muted">No nonpassing required-gate observations recorded in this snapshot.</p>
              : <ul>{snapshot.observations.map((observation) => (
                <li key={observation.gate}>
                  {observation.gate}: {statusLabel(observation.status)}
                  {(observation.unresolved_criteria?.length ?? 0) > 0 && <>; unresolved criteria: {observation.unresolved_criteria?.join(", ")}</>}
                  {(observation.evidence_refs?.length ?? 0) > 0 && <>; {observation.evidence_refs?.map((ref, index) => (
                    <span key={ref}>{index > 0 && ", "}<a className="instr-evid" href={`/api/evidence?ref=${encodeURIComponent(ref)}`} title={ref} target="_blank" rel="noreferrer">{evidenceLabel({ ref })}</a></span>
                  ))}</>}
                </li>
              ))}</ul>}
          </div>
        ))}
      </details>
      {(historicalViolations !== undefined || crossingViolations !== undefined) && (
        <p>Recorded source-pinned historical policy warnings: {historicalViolations ?? 0}; independent crossing-time policy warnings: {crossingViolations ?? 0}. Neither count states the current gate result.</p>
      )}
      <p>Current acceptance: {acceptanceReady === undefined ? "not classified here; inspect the current contract and gates" : acceptanceReady ? "ready" : "unresolved"}. Historical recovery does not approve or clear current acceptance.</p>
      {(outstanding?.length ?? 0) > 0 && <ul>{outstanding?.map((work) => (
        <li key={`${work.node_id ?? ""}:${work.kind}:${work.id}`}>{work.node_id !== undefined && `${work.node_id} · `}{work.id}: {statusLabel(work.status)} · {work.actor}</li>
      ))}</ul>}
      {currentAcceptance !== undefined && <div>
        <span className="kind">Participating feature acceptance</span>
        <ul>{currentAcceptance.map((feature) => (
          <li key={feature.node_id}>{feature.node_id}: {feature.acceptance_ready ? "ready" : "unresolved"} · contract {statusLabel(feature.contract_status).toLowerCase()}</li>
        ))}</ul>
      </div>}
      {gates !== undefined && (
        <div>
          <span className="kind">Current recorded gates · independent of historical policy</span>
          <ul>{Object.entries(gates).map(([name, gate]) => (
            <li key={name}>{name}: {statusLabel(gate.status ?? "pending")}</li>
          ))}</ul>
        </div>
      )}
    </section>
  );
}
