// Shared derivation for how a gate or criterion result paints and reads,
// factored out so GateCard's own header line (mock round 4's gstate: "pending
// — runs at check during execute" before any check has run, "1 criterion
// awaits your attestation" once one does) and CriterionRow's per-row dot agree
// on one rule rather than two hand-rolled ones.
//
// This is a DISPLAY rule, layered on top of lib/states.ts rather than folded
// into it: statusColor's fold table is scoped to the wire NODE lifecycle
// vocabulary (proposed, open, answered, and the rest of NodeView.status), while
// a gate or criterion result (pass, fail, pending, needs_attestation) is a
// different vocabulary entirely, store.GateStatus* / store.CriterionStatus*,
// never a node's own status. Mapping each result onto the chart-state WORD it
// best represents, then handing that word to StateDot/Chip, reuses their
// existing total statusColor mapping (B1 contract: any wire status string, no
// casts) without teaching lib/states.ts a second, unrelated vocabulary.

import type { EvalCriterionView, GateView, NodeStatus } from "../api";

// resultStatus reads a criterion's recorded status, defaulting to "pending"
// the same way the pass-1 GatesBlock's own CriterionRow always did: a
// criterion with no recorded result yet, because check has never run, is
// exactly pending, never a blank dot.
export function resultStatus(status: string | undefined): string {
  return status ?? "pending";
}

// dotStateFor maps a gate or criterion result word onto the chart-state word
// StateDot/Chip should paint it as: a pass reads done (green), a fail reads
// blocked (red), needs_attestation reads in_review (amber, the one thing
// waiting on a human), and anything else (pending, skipped, stale, a result
// this build has never seen) reads draft (quiet slate) rather than guessing.
export function dotStateFor(status: string): NodeStatus {
  switch (status) {
    case "pass":
      return "done";
    case "fail":
      return "blocked";
    case "needs_attestation":
      return "in_review";
    default:
      return "draft";
  }
}

export interface DerivedGateState {
  dotStatus: NodeStatus;
  label: string;
}

// deriveCriteriaState computes a criteria-bearing gate's own header line from
// every criterion's decorated status (already defaulted through
// resultStatus). Priority order, most urgent first: a failure is never buried
// behind a quieter result; needs_attestation is the human's own next action
// and outranks a stale sibling, since attesting is a decision only the human
// can make while a fresh check is a mechanical rerun; stale (new work landed
// beneath a done feature, so whatever this criterion certified no longer
// covers what's there now) gets its own line rather than falling through to
// the pre-check default, matching deriveFlatGateState's stale branch below;
// an all-pass contract reads done; anything else (no check has run, or a mix
// of pending and pass with nothing yet failed, stale, or awaiting
// attestation) is the same quiet pre-check line the mock's draft-state
// instrument shows.
function deriveCriteriaState(statuses: string[]): DerivedGateState {
  const failed = statuses.filter((s) => s === "fail").length;
  if (failed > 0) {
    return {
      dotStatus: "blocked",
      label: `${failed} ${failed === 1 ? "criterion" : "criteria"} failed`,
    };
  }

  const attesting = statuses.filter((s) => s === "needs_attestation").length;
  if (attesting > 0) {
    return {
      dotStatus: "in_review",
      label:
        attesting === 1
          ? "1 criterion awaits your attestation"
          : `${attesting} criteria await your attestation`,
    };
  }

  const stale = statuses.filter((s) => s === "stale").length;
  if (stale > 0) {
    return {
      dotStatus: "draft",
      label: `${stale} ${stale === 1 ? "criterion" : "criteria"} stale — needs a fresh check`,
    };
  }

  if (statuses.length > 0 && statuses.every((s) => s === "pass")) {
    return { dotStatus: "done", label: "all criteria satisfied" };
  }

  return { dotStatus: "draft", label: "pending — runs at check during execute" };
}

// deriveFlatGateState is the same header line for a gate that carries no
// per-criterion breakdown (build, tests, an attestation-backed review gate):
// its own recorded status stands in for a criteria count.
export function deriveFlatGateState(gate: GateView | undefined): DerivedGateState {
  switch (gate?.status) {
    case "pass":
      return { dotStatus: "done", label: "passed" };
    case "fail":
      return { dotStatus: "blocked", label: "failed" };
    case "skipped":
      return { dotStatus: "draft", label: "skipped" };
    case "stale":
      return { dotStatus: "draft", label: "stale — needs a fresh check" };
    default:
      return { dotStatus: "draft", label: "pending — runs at check during execute" };
  }
}

export interface DerivedEvalsGateState extends DerivedGateState {
  // The required criterion the blocked or in-review state stands for: a fail
  // before a needs_attestation, file order. Undefined when nothing required is
  // failing or awaiting attestation.
  blocking?: string;
}

// deriveEvalsGateState is the evals gate's one readiness reading, shared by the
// node header chip and GateCard so the two never disagree. Once Go has recorded
// the gate, its stored status is the fact submit judges: pass
// reads done and anything else does not, whatever the per-criterion statuses
// say now (a required criterion added since the last check, or a stored stale
// over an all-optional contract). The required-criteria projection, which
// mirrors gates.GateStatusFromCriteria (only required criteria gate
// completion), supplies the label and the blocking qualifier, and stands in
// for readiness only when no gate status was ever recorded.
export function deriveEvalsGateState(
  criteria: EvalCriterionView[],
  gate: GateView | undefined,
): DerivedEvalsGateState {
  const projected = projectRequiredCriteria(criteria, gate);
  if (gate?.status === undefined) {
    return projected;
  }
  if (gate.status === "pass") {
    return projected.dotStatus === "done"
      ? { dotStatus: "done", label: projected.label }
      : { dotStatus: "done", label: "passed at last check" };
  }
  if (projected.dotStatus === "blocked" || projected.dotStatus === "in_review") {
    return projected;
  }
  return deriveFlatGateState(gate);
}

// projectRequiredCriteria reads readiness from the required criteria alone.
// Optional criteria still render their own rows, but cannot make a passing
// required set read blocked, and a contract with no required criteria is
// nonblocking.
function projectRequiredCriteria(
  criteria: EvalCriterionView[],
  gate: GateView | undefined,
): DerivedEvalsGateState {
  const required = criteria.filter((c) => c.required);
  if (required.length === 0) {
    return { dotStatus: "done", label: "no required criteria — nonblocking" };
  }
  const statuses = required.map((c) => resultStatus(gate?.criteria?.[c.id]?.status));
  const state = deriveCriteriaState(statuses);
  if (state.dotStatus === "done" && required.length < criteria.length) {
    return { dotStatus: "done", label: "required criteria satisfied" };
  }
  const failedIdx = statuses.indexOf("fail");
  const attestIdx = statuses.indexOf("needs_attestation");
  const blockingIdx = failedIdx !== -1 ? failedIdx : attestIdx;
  return blockingIdx === -1 ? state : { ...state, blocking: required[blockingIdx].id };
}
