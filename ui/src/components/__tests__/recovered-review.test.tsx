import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";
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
  expect(within(panel).getByText(/tests: Unknown/)).toBeInTheDocument();
  expect(within(panel).getByText(/current acceptance: ready/i)).toBeInTheDocument();
  expect(within(panel).queryByText(/awaiting.*merge/i)).not.toBeInTheDocument();
});

it("does not present containment as an observed merge or pending acceptance as historical policy", () => {
  render(<RecoveredReview record={{ ...recovery, basis: "contained" }} acceptanceReady={false} outstanding={[{ kind: "gate", id: "build", status: "fail", actor: "agent" }]} />);
  expect(screen.getByText(/not an observed merge/i)).toBeInTheDocument();
  expect(screen.getByText(/current acceptance: unresolved/i)).toBeInTheDocument();
  expect(screen.getByText(/build: Fail/)).toBeInTheDocument();
  expect(screen.getByText(/evals: Pending/)).toBeInTheDocument();
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
  expect(screen.getByText(/F-second.*C2: Pending/)).toBeInTheDocument();
  expect(screen.getByText(/current acceptance: unresolved/i)).toBeInTheDocument();
});

it("keeps mixed-provenance recorded warning counts independent of current passing gates", () => {
  render(<RecoveredReview record={recovery} historicalViolations={1} crossingViolations={1}
    acceptanceReady={true} gates={{ evals: { required: true, status: "pass" } }} />);
  expect(screen.getByText(/source-pinned historical policy warnings: 1.*crossing-time policy warnings: 1/)).toBeInTheDocument();
  expect(screen.getByText(/evals: Pass/)).toBeInTheDocument();
  expect(screen.getByText(/evals: Pending/)).toBeInTheDocument();
  expect(screen.getByText(/current acceptance: ready/i)).toBeInTheDocument();
});

const riding: RecoveredReviewView = {
  ...recovery,
  tasks: [
    { id: "T-2il2d6", path: ".tiller/epics/a/features/b/tasks/c/task.yml", state: "git:submitted-state" },
    { id: "T-notitle", path: ".tiller/epics/a/features/b/tasks/d/task.yml", state: "git:other-state" },
  ],
  policy_snapshots: [{ node_id: "F-review", path: "feature.yml", source: "git:historical-policy",
    observations: [{ gate: "evals", status: "needs_attestation", evidence_refs: ["artifacts/out.txt", "artifacts/more.txt"] }] }],
};

describe("leads with what matters", () => {
  it("shows the reason, the PR, and riding tasks by title, with the rest in a closed Details", () => {
    const { container } = render(<MemoryRouter><RecoveredReview record={riding} titles={new Map([["T-2il2d6", "Extract params"]])} /></MemoryRouter>);
    const details = container.querySelector("details") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    expect(details.querySelector("summary")?.textContent).toBe("Details");
    const body = screen.getByRole("region", { name: /recovered historical review/i });
    expect(within(body).getByText(/Authorized recovery/).closest("details")).toBeNull();
    expect(within(body).getByText(/PR #155/).closest("details")).toBeNull();
    const title = within(body).getByRole("link", { name: "Extract params" });
    expect(title).toHaveAttribute("href", "/node/task/T-2il2d6");
    expect(title.closest("details")).toBeNull();
    const bare = within(body).getByRole("link", { name: "T-notitle" });
    expect(bare.closest("details")).toBeNull();
    for (const text of ["git:historical-head", "git:integration-result", "git:submitted-state", ".tiller/epics/a/features/b/tasks/c/task.yml", "git:historical-policy"]) {
      expect(within(details).getByText(text, { exact: false })).toBeInTheDocument();
    }
    expect(within(details).getByText(/Evidence basis/)).toBeInTheDocument();
    const outside = container.cloneNode(true) as HTMLElement;
    outside.querySelector("details")?.remove();
    expect(outside.textContent).not.toMatch(/git:submitted-state|task\.yml|git:historical-head|Evidence basis/);
  });

  it("reads evidence refs as labels that keep the ref on the href and title", () => {
    const { container } = render(<MemoryRouter><RecoveredReview record={riding} /></MemoryRouter>);
    const links = Array.from(container.querySelectorAll("details a")).filter((a) => a.textContent === "evidence");
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAttribute("href", "/api/evidence?ref=artifacts%2Fout.txt");
    expect(links[0]).toHaveAttribute("title", "artifacts/out.txt");
    expect(container.textContent).not.toMatch(/evidence: /);
  });

  it("prints no raw underscore token outside ids, paths, and branch names", () => {
    const { container } = render(<MemoryRouter><RecoveredReview record={{ ...riding, basis: "contained" }}
      outstanding={[{ node_id: "F-review", kind: "gate", id: "build", status: "in_review", actor: "agent" }]}
      currentAcceptance={[{ node_id: "F-review", contract_status: "needs_work", acceptance_ready: false }]}
      gates={{ evals: { required: true, status: "needs_attestation" } }} /></MemoryRouter>);
    const text = (container.textContent ?? "").replace(/\.tiller\/\S+|tiller\/\S+/g, "");
    expect(text).not.toMatch(/[a-z]+_[a-z_]+/);
  });
});
