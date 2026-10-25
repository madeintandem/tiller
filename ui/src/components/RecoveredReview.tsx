import type { CurrentAcceptanceView, GateView, RecoveredReviewView, WorkflowWork } from "../api";

// Historical provenance is not current approval or a host observation.
export default function RecoveredReview({ record, acceptanceReady, outstanding, gates, crossingsPending, currentAcceptance, historicalViolations, crossingViolations }: {
  record: RecoveredReviewView;
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
      <p>{record.pr === undefined ? "Local review ref" : `Caller-supplied PR #${record.pr}`} · {record.branch}</p>
      <p>Historical head: <code>{record.head}</code></p>
      <p>Integration result: <code>{record.merge_commit}</code> on {record.integration_branch}</p>
      <p>Evidence basis: {record.basis}{record.basis === "contained" && (crossingsPending === true
        ? "; not an observed merge; pending historical riders require the human fast-forward decision"
        : crossingsPending === false
          ? "; containment was not an observed merge; historical riders have crossed and need no further fast-forward assertion"
          : "; containment at recovery was not an observed merge; only still-pending historical riders need a human fast-forward assertion")}</p>
      <p>Recorded by {record.recovered_by} at {record.recovered_at}: {record.reason}</p>
      <div>
        <span className="kind">Historical riding tasks</span>
        <ul>{record.tasks.map((task) => (
          <li key={task.id}>{task.id} · {task.path} · submitted state <code>{task.state}</code></li>
        ))}</ul>
      </div>
      <div>
        <span className="kind">Historical policy · recorded state, not a new check run</span>
        {record.policy_snapshots.map((snapshot) => (
          <div key={`${snapshot.node_id ?? "manifest"}:${snapshot.path}:${snapshot.source}`}>
            <p>{snapshot.node_id ?? "Manifest"} · <code>{snapshot.source}</code> · {snapshot.path}</p>
            {snapshot.observations.length === 0
              ? <p className="muted">No nonpassing required-gate observations recorded in this snapshot.</p>
              : <ul>{snapshot.observations.map((observation) => (
                <li key={observation.gate}>
                  {observation.gate}: {observation.status}
                  {(observation.unresolved_criteria?.length ?? 0) > 0 && <>; unresolved criteria: {observation.unresolved_criteria?.join(", ")}</>}
                  {(observation.evidence_refs?.length ?? 0) > 0 && <>; evidence: {observation.evidence_refs?.join(", ")}</>}
                </li>
              ))}</ul>}
          </div>
        ))}
      </div>
      {(historicalViolations !== undefined || crossingViolations !== undefined) && (
        <p>Recorded source-pinned historical policy warnings: {historicalViolations ?? 0}; independent crossing-time policy warnings: {crossingViolations ?? 0}. Neither count states the current gate result.</p>
      )}
      <p>Current acceptance: {acceptanceReady === undefined ? "not classified here; inspect the current contract and gates" : acceptanceReady ? "ready" : "unresolved"}. Historical recovery does not approve or clear current acceptance.</p>
      {(outstanding?.length ?? 0) > 0 && <ul>{outstanding?.map((work) => (
        <li key={`${work.node_id ?? ""}:${work.kind}:${work.id}`}>{work.node_id !== undefined && `${work.node_id} · `}{work.id}: {work.status} · {work.actor}</li>
      ))}</ul>}
      {currentAcceptance !== undefined && <div>
        <span className="kind">Participating feature acceptance</span>
        <ul>{currentAcceptance.map((feature) => (
          <li key={feature.node_id}>{feature.node_id}: {feature.acceptance_ready ? "ready" : "unresolved"} · contract {feature.contract_status}</li>
        ))}</ul>
      </div>}
      {gates !== undefined && (
        <div>
          <span className="kind">Current recorded gates · independent of historical policy</span>
          <ul>{Object.entries(gates).map(([name, gate]) => (
            <li key={name}>{name}: {gate.status ?? "pending"}</li>
          ))}</ul>
        </div>
      )}
    </section>
  );
}
