import { describe, expect, it } from "vitest";

import type { EvalContractView, EvalCriterionView, GateView } from "../api";
import { gateChipsFor } from "./gateChips";

function criterion(overrides: Partial<EvalCriterionView> = {}): EvalCriterionView {
  return {
    id: "C1",
    statement: "A valid link redeems exactly once.",
    polarity: "introduce",
    required: true,
    binding: { type: "test", ref: "spec/redeem_spec.rb" },
    ...overrides,
  };
}

function contract(criteria: EvalCriterionView[]): EvalContractView {
  return { path: "features/x/feature.eval.md", node: "F-1", criteria };
}

describe("gateChipsFor: order", () => {
  it("orders every other gate alphabetically with evals always trailing", () => {
    const gates: Record<string, GateView> = {
      evals: { required: true, status: "pending" },
      tests: { required: true, status: "pass" },
      build: { required: true, status: "pass" },
    };
    const names = gateChipsFor(gates, null).map((c) => c.name);
    expect(names).toEqual(["build", "tests", "evals"]);
  });

  it("renders no chip at all with no recorded gates, even with an authored contract", () => {
    expect(gateChipsFor(undefined, contract([criterion()]))).toEqual([]);
  });
});

describe("gateChipsFor: flat gates (build, tests, review gates)", () => {
  it("reads pass from the recorded status", () => {
    const chips = gateChipsFor({ build: { required: true, status: "pass" } }, null);
    expect(chips).toEqual([{ name: "build", pass: true }]);
  });

  it("reads pend for anything short of pass: pending, fail, or stale alike", () => {
    for (const status of ["pending", "fail", "stale", "skipped"]) {
      const chips = gateChipsFor({ tests: { required: true, status } }, null);
      expect(chips[0].pass).toBe(false);
    }
  });
});

describe("gateChipsFor: the evals chip", () => {
  it("passes only once every criterion has", () => {
    const criteria = [criterion({ id: "C1" }), criterion({ id: "C2" })];
    const gate: GateView = {
      required: true,
      criteria: { C1: { status: "pass" }, C2: { status: "pass" } },
    };
    const chips = gateChipsFor({ evals: gate }, contract(criteria));
    expect(chips).toEqual([{ name: "evals", pass: true, qualifier: undefined }]);
  });

  it("names the criterion awaiting attestation as its qualifier when pending", () => {
    const criteria = [
      criterion({ id: "C1" }),
      criterion({ id: "C2" }),
      criterion({ id: "C3", binding: { type: "manual" } }),
    ];
    const gate: GateView = {
      required: true,
      criteria: { C1: { status: "pass" }, C2: { status: "pass" }, C3: { status: "needs_attestation" } },
    };
    const chips = gateChipsFor({ evals: gate }, contract(criteria));
    expect(chips[0]).toEqual({ name: "evals", pass: false, qualifier: "C3" });
  });

  it("names a failed criterion over a needs_attestation one, the same priority the gate's own header line ranks by", () => {
    const criteria = [
      criterion({ id: "C1" }),
      criterion({ id: "C2", binding: { type: "manual" } }),
      criterion({ id: "C3" }),
    ];
    const gate: GateView = {
      required: true,
      criteria: {
        C1: { status: "fail" },
        C2: { status: "needs_attestation" },
        C3: { status: "pass" },
      },
    };
    const chips = gateChipsFor({ evals: gate }, contract(criteria));
    expect(chips[0].qualifier).toBe("C1");
  });

  it("carries no qualifier before any check has run", () => {
    const chips = gateChipsFor({ evals: { required: true } }, contract([criterion()]));
    expect(chips[0]).toEqual({ name: "evals", pass: false, qualifier: undefined });
  });

  it("falls back to the flat gate state, no qualifier, when the contract failed to load", () => {
    const chips = gateChipsFor(
      { evals: { required: true, status: "pending" } },
      { path: "x", error: "malformed" },
    );
    expect(chips).toEqual([{ name: "evals", pass: false }]);
  });

  it("falls back to the flat gate state when a result exists with no contract at all", () => {
    const chips = gateChipsFor({ evals: { required: true, status: "pass" } }, null);
    expect(chips).toEqual([{ name: "evals", pass: true }]);
  });
});

describe("gateChipsFor: only required criteria gate the evals chip", () => {
  it("passes when every required criterion passed, whatever optional criteria read", () => {
    for (const optional of ["fail", "pending", "needs_attestation", undefined]) {
      const criteria = [criterion({ id: "C1" }), criterion({ id: "C2", required: false })];
      const gate: GateView = {
        required: true,
        status: "pass",
        criteria: { C1: { status: "pass" }, ...(optional ? { C2: { status: optional } } : {}) },
      };
      const chips = gateChipsFor({ evals: gate }, contract(criteria));
      expect(chips[0]).toEqual({ name: "evals", pass: true, qualifier: undefined });
    }
  });

  it("still blocks on a required criterion that failed, pends, or went stale", () => {
    for (const required of ["fail", "pending", "needs_attestation", "stale"]) {
      const criteria = [criterion({ id: "C1" }), criterion({ id: "C2", required: false })];
      const gate: GateView = {
        required: true,
        criteria: { C1: { status: required }, C2: { status: "pass" } },
      };
      expect(gateChipsFor({ evals: gate }, contract(criteria))[0].pass).toBe(false);
    }
  });

  it("names the failing required criterion, never an optional one, as its qualifier", () => {
    const criteria = [
      criterion({ id: "C1", required: false }),
      criterion({ id: "C2", binding: { type: "manual" } }),
    ];
    const gate: GateView = {
      required: true,
      criteria: { C1: { status: "fail" }, C2: { status: "needs_attestation" } },
    };
    expect(gateChipsFor({ evals: gate }, contract(criteria))[0]).toEqual({
      name: "evals",
      pass: false,
      qualifier: "C2",
    });
  });

  it("reads an all-optional contract as nonblocking", () => {
    const criteria = [criterion({ id: "C1", required: false })];
    const gate: GateView = { required: true, criteria: { C1: { status: "fail" } } };
    expect(gateChipsFor({ evals: gate }, contract(criteria))[0].pass).toBe(true);
  });
});
