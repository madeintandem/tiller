// gateChipsFor derives the node header's compact gate-chip row (the mock's
// .gate-chip, the retired console mock; "meta row with owner, claim, branch,
// PR, and gate chips from node.gates"). One chip per RECORDED gate only -
// node.gates, never a gate that merely has an authored contract and has not run
// - in a fixed display order: every other gate alphabetically, evals always
// trailing, since it is the acceptance gate the Contract tab details further
// and reads last, after the mechanical CI gates a reader checks first.
//
// Each chip is binary, passed or not (the mock's own two registers, .g.pass
// / .g.pend - a failed or stale gate still reads pend, the same two-state
// ceiling GateCard's own header line already accepts for everything but a
// criteria-bearing gate's richer label). The evals chip alone can carry a
// qualifier: the one required criterion its pend register stands for, a fail
// before a needs_attestation, file order. Pass/pend and qualifier both come
// from lib/gateDisplay.ts's deriveEvalsGateState, the same reading GateCard's
// own header line uses, so this chip and that line can never disagree: a
// recorded gate status decides pass/pend, and the required-criteria projection
// only names the qualifier (or decides when nothing was recorded). Optional
// criteria never hold the chip at pend. The qualifier is undefined
// once nothing required is failing or awaiting attestation, or the contract
// itself is null or malformed - a reader taps into the Contract tab for the
// rest either way.

import { isEvalError } from "../api";
import type { EvalPayload, GateView } from "../api";
import { deriveEvalsGateState, deriveFlatGateState } from "./gateDisplay";

export interface GateChip {
  name: string;
  pass: boolean;
  qualifier?: string;
}

function order(names: string[]): string[] {
  const rest = names.filter((n) => n !== "evals").sort();
  return names.includes("evals") ? [...rest, "evals"] : rest;
}

function evalsChip(evalPayload: EvalPayload, gate: GateView | undefined): GateChip {
  if (evalPayload === null || isEvalError(evalPayload)) {
    return { name: "evals", pass: deriveFlatGateState(gate).dotStatus === "done" };
  }
  const state = deriveEvalsGateState(evalPayload.criteria, gate);
  return { name: "evals", pass: state.dotStatus === "done", qualifier: state.blocking };
}

export function gateChipsFor(
  gates: Record<string, GateView> | undefined,
  evalPayload: EvalPayload,
): GateChip[] {
  const record = gates ?? {};
  return order(Object.keys(record)).map((name) =>
    name === "evals"
      ? evalsChip(evalPayload, record[name])
      : { name, pass: deriveFlatGateState(record[name]).dotStatus === "done" },
  );
}
