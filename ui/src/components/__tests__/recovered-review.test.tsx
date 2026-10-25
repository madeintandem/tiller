import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { RecoveredReviewView } from "../../api";
import RecoveredReview from "../RecoveredReview";

afterEach(cleanup);

const recovery: RecoveredReviewView = {
  pr: 155, branch: "tiller/reviews/features/review", head: "git:historical-head",
  merge_commit: "git:integration-result", integration_branch: "main", basis: "ancestry",
  recovered_by: "jc", recovered_at: "2026-10-07T12:00:00Z", reason: "Authorized recovery", tasks: [],
  policy_snapshots: [{ node_id: "F-review", path: "feature.yml", source: "git:historical-policy",
    observations: [{ gate: "evals", status: "pending", unresolved_criteria: ["C8"] }, { gate: "tests", status: "unknown" }] }],
};

it("keeps historical policy and current acceptance independently visible", () => {
  render(<RecoveredReview record={recovery} acceptanceReady={true} />);
  const panel = screen.getByRole("region", { name: /recovered historical review/i });
  expect(within(panel).getByText(/155/)).toBeInTheDocument();
  expect(within(panel).getByText("git:historical-head")).toBeInTheDocument();
  expect(within(panel).getByText("git:integration-result")).toBeInTheDocument();
  expect(within(panel).getByText(/C8/)).toBeInTheDocument();
  expect(within(panel).getByText(/tests: unknown/)).toBeInTheDocument();
  expect(within(panel).getByText(/current acceptance: ready/i)).toBeInTheDocument();
  expect(within(panel).queryByText(/awaiting.*merge/i)).not.toBeInTheDocument();
});

it("does not present containment as an observed merge or pending acceptance as historical policy", () => {
  render(<RecoveredReview record={{ ...recovery, basis: "contained" }} acceptanceReady={false} outstanding={[{ kind: "gate", id: "build", status: "fail", actor: "agent" }]} />);
  expect(screen.getByText(/not an observed merge/i)).toBeInTheDocument();
  expect(screen.getByText(/current acceptance: unresolved/i)).toBeInTheDocument();
  expect(screen.getByText(/build: fail/)).toBeInTheDocument();
  expect(screen.getByText(/evals: pending/)).toBeInTheDocument();
});

it("makes completed contained crossings historical rather than requesting another assertion", () => {
  render(<RecoveredReview record={{ ...recovery, basis: "contained" }} crossingsPending={false} acceptanceReady={false} />);
  expect(screen.getByText(/need no further fast-forward assertion/i)).toBeInTheDocument();
  expect(screen.queryByText(/pending historical riders require/i)).not.toBeInTheDocument();
});

it("retains every participating feature's independently classified acceptance", () => {
  render(<RecoveredReview record={recovery} acceptanceReady={false}
    currentAcceptance={[
      { node_id: "F-first", contract_status: "valid", acceptance_ready: true },
      { node_id: "F-second", contract_status: "valid", acceptance_ready: false },
    ]}
    outstanding={[{ node_id: "F-second", kind: "criterion", id: "C2", status: "pending", actor: "human" }]} />);
  expect(screen.getByText(/F-first: ready/)).toBeInTheDocument();
  expect(screen.getByText(/F-second: unresolved/)).toBeInTheDocument();
  expect(screen.getByText(/F-second.*C2: pending/)).toBeInTheDocument();
  expect(screen.getByText(/current acceptance: unresolved/i)).toBeInTheDocument();
});

it("keeps mixed-provenance recorded warning counts independent of current passing gates", () => {
  render(<RecoveredReview record={recovery} historicalViolations={1} crossingViolations={1}
    acceptanceReady={true} gates={{ evals: { required: true, status: "pass" } }} />);
  expect(screen.getByText(/source-pinned historical policy warnings: 1.*crossing-time policy warnings: 1/)).toBeInTheDocument();
  expect(screen.getByText(/evals: pass/)).toBeInTheDocument();
  expect(screen.getByText(/evals: pending/)).toBeInTheDocument();
  expect(screen.getByText(/current acceptance: ready/i)).toBeInTheDocument();
});
