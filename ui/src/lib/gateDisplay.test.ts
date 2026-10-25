// Vitest coverage for the acceptance instrument's shared gate/criterion
// display derivation (task C3). Written before GateCard/CriterionRow reach
// for it, per the branch's TDD discipline; run once red, then made to pass.

import { describe, expect, it } from "vitest";

import type { GateView } from "../api";
import {
  deriveEvalsGateState,
  deriveFlatGateState,
  dotStateFor,
  resultStatus,
} from "./gateDisplay";

describe("resultStatus", () => {
  it("defaults an unrecorded criterion to pending", () => {
    expect(resultStatus(undefined)).toBe("pending");
  });

  it("passes through a recorded status verbatim", () => {
    expect(resultStatus("pass")).toBe("pass");
  });
});

describe("dotStateFor", () => {
  it.each([
    ["pass", "done"],
    ["fail", "blocked"],
    ["needs_attestation", "in_review"],
    ["pending", "draft"],
    ["skipped", "draft"],
    ["some-future-result", "draft"],
  ])("maps %s to %s", (status, expected) => {
    expect(dotStateFor(status)).toBe(expected);
  });
});

// The criteria projection is module-private; it is reached here the way every
// caller reaches it, through deriveEvalsGateState with an all-required contract
// and no recorded gate status.
function criteriaState(statuses: string[]) {
  const criteria = statuses.map((_, i) => ({
    id: `C${i + 1}`,
    statement: "s",
    polarity: "introduce",
    required: true,
    binding: { type: "manual" },
  }));
  const gate: GateView = {
    required: true,
    criteria: Object.fromEntries(statuses.map((status, i) => [`C${i + 1}`, { status }])),
  };
  const { dotStatus, label } = deriveEvalsGateState(criteria, gate);
  return { dotStatus, label };
}

describe("deriveEvalsGateState: the criteria projection with no recorded gate status", () => {
  it("reads pending before any check has run", () => {
    const state = criteriaState(["pending", "pending", "pending"]);
    expect(state).toEqual({
      dotStatus: "draft",
      label: "pending — runs at check during execute",
    });
  });

  it("names one criterion awaiting attestation, singular", () => {
    const state = criteriaState(["pass", "pass", "needs_attestation"]);
    expect(state).toEqual({
      dotStatus: "in_review",
      label: "1 criterion awaits your attestation",
    });
  });

  it("names several criteria awaiting attestation, plural", () => {
    const state = criteriaState(["needs_attestation", "needs_attestation", "pass"]);
    expect(state).toEqual({
      dotStatus: "in_review",
      label: "2 criteria await your attestation",
    });
  });

  it("reads all criteria satisfied once every one has passed", () => {
    const state = criteriaState(["pass", "pass"]);
    expect(state).toEqual({ dotStatus: "done", label: "all criteria satisfied" });
  });

  it("puts a failure ahead of a needs_attestation sibling, never buried", () => {
    const state = criteriaState(["fail", "needs_attestation"]);
    expect(state).toEqual({ dotStatus: "blocked", label: "1 criterion failed" });
  });

  it("pluralizes a failure count", () => {
    const state = criteriaState(["fail", "fail", "pass"]);
    expect(state).toEqual({ dotStatus: "blocked", label: "2 criteria failed" });
  });

  it("reads stale as needing a fresh check, not as never-run pending", () => {
    const state = criteriaState(["stale", "stale"]);
    expect(state).toEqual({
      dotStatus: "draft",
      label: "2 criteria stale — needs a fresh check",
    });
  });

  it("names one stale criterion, singular", () => {
    const state = criteriaState(["pass", "stale"]);
    expect(state).toEqual({
      dotStatus: "draft",
      label: "1 criterion stale — needs a fresh check",
    });
  });

  it("puts a needs_attestation sibling ahead of stale: attesting is the human's job, a fresh check is not", () => {
    const state = criteriaState(["stale", "needs_attestation"]);
    expect(state).toEqual({
      dotStatus: "in_review",
      label: "1 criterion awaits your attestation",
    });
  });
});

describe("deriveFlatGateState", () => {
  function gate(overrides: Partial<GateView> = {}): GateView {
    return { required: true, ...overrides };
  }

  it("reads pending when the gate has never recorded a result", () => {
    expect(deriveFlatGateState(undefined)).toEqual({
      dotStatus: "draft",
      label: "pending — runs at check during execute",
    });
  });

  it("reads passed", () => {
    expect(deriveFlatGateState(gate({ status: "pass" }))).toEqual({
      dotStatus: "done",
      label: "passed",
    });
  });

  it("reads failed", () => {
    expect(deriveFlatGateState(gate({ status: "fail" }))).toEqual({
      dotStatus: "blocked",
      label: "failed",
    });
  });

  it("reads skipped", () => {
    expect(deriveFlatGateState(gate({ status: "skipped" }))).toEqual({
      dotStatus: "draft",
      label: "skipped",
    });
  });

  it("reads stale as needing a fresh check", () => {
    expect(deriveFlatGateState(gate({ status: "stale" }))).toEqual({
      dotStatus: "draft",
      label: "stale — needs a fresh check",
    });
  });
});
