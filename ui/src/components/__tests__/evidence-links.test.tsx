// Evidence links read as short labels (console UI plan, task 15): the raw ref
// stays in the href and the title, never on the screen.

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { EvalCriterionView } from "../../api";
import CriterionRow from "../CriterionRow";
import GateCard from "../GateCard";

afterEach(() => {
  cleanup();
});

const criterion: EvalCriterionView = {
  id: "C1",
  statement: "Output matches the rubric.",
  polarity: "introduce",
  required: true,
  binding: { type: "eval" },
};

describe("evidence links read as labels", () => {
  it("labels a criterion row's evidence link and keeps the ref in href and title", () => {
    const ref = "artifacts/tiller/gates/evals-C1-rubric.txt";
    render(
      <CriterionRow
        criterion={criterion}
        result={{ status: "needs_attestation", evidence: [{ kind: "rubric_output", ref }] }}
      />,
    );
    const link = screen.getByRole("link", { name: "eval C1 output" });
    expect(link).toHaveAttribute("title", ref);
    expect(link).toHaveAttribute("href", `/api/evidence?ref=${encodeURIComponent(ref)}`);
    expect(document.body.textContent ?? "").not.toContain(ref);
  });

  it("labels a flat gate's evidence by its kind in plain words", () => {
    const ref = "artifacts/tiller/gates/tests-zzz.txt";
    render(
      <GateCard
        name="tests"
        gate={{ required: true, status: "pass", evidence: [{ kind: "command_output", ref }] }}
      />,
    );
    const link = screen.getByRole("link", { name: "command output evidence" });
    expect(link).toHaveAttribute("title", ref);
    expect(link).toHaveAttribute("href", `/api/evidence?ref=${encodeURIComponent(ref)}`);
    expect(document.body.textContent ?? "").not.toContain("_");
  });
});
