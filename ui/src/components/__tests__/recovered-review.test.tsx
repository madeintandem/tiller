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
  it("keeps current-state items visible and folds only historical record detail into a closed Details", () => {
    const { container } = render(<MemoryRouter><RecoveredReview record={{ ...riding, basis: "contained" }}
      titles={new Map([["T-2il2d6", "Extract params"]])}
      acceptanceReady={false} crossingsPending={true} historicalViolations={1} crossingViolations={2}
      outstanding={[{ node_id: "F-review", kind: "criterion", id: "C2", status: "pending", actor: "human" }]}
      currentAcceptance={[{ node_id: "F-review", contract_status: "valid", acceptance_ready: false }]}
      gates={{ lint: { required: true, status: "pass" } }} /></MemoryRouter>);
    const details = container.querySelector("details") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    expect(details.querySelector("summary")?.textContent).toBe("Details");
    const outside = container.cloneNode(true) as HTMLElement;
    outside.querySelector("details")?.remove();
    const visible = outside.textContent ?? "";
    const folded = details.textContent ?? "";
    // Visible: the reason, the PR, riding tasks by title (id when unknown),
    // and every current-state item.
    for (const text of ["Extract params", "T-notitle"]) expect(visible).toContain(text);
    for (const text of ["Authorized recovery", "PR #155", "Current acceptance: unresolved",
      "C2: Pending", "Participating feature acceptance", "F-review: unresolved", "Current recorded gates", "lint: Pass",
      "source-pinned historical policy warnings: 1", "crossing-time policy warnings: 2"]) {
      expect(visible).toContain(text);
      expect(folded).not.toContain(text);
    }
    // Folded: the historical record detail, and none of it visible.
    for (const text of ["git:historical-head", "git:integration-result", "Evidence basis: containment", "not an observed merge",
      "task.yml", "git:submitted-state", "git:other-state", "git:historical-policy", "feature.yml", "evals: Needs attestation"]) {
      expect(folded).toContain(text);
      expect(visible).not.toContain(text);
    }
    expect(Array.from(details.querySelectorAll("a")).filter((a) => a.textContent === "evidence")).toHaveLength(2);
  });

  it("links the PR when its address is known, saying so in the accessible name, and reads as text when it is not", () => {
    const { rerender } = render(<MemoryRouter><RecoveredReview record={{ ...riding, pr_url: "https://github.com/acme/app/pull/155" }} /></MemoryRouter>);
    const link = screen.getByRole("link", { name: "Caller-supplied PR #155 (opens the pull request)" });
    expect(link).toHaveAttribute("href", "https://github.com/acme/app/pull/155");
    expect(link).toHaveTextContent("Caller-supplied PR #155");
    rerender(<MemoryRouter><RecoveredReview record={riding} /></MemoryRouter>);
    expect(screen.queryByRole("link", { name: /pull request/ })).toBeNull();
    expect(screen.getByText(/Caller-supplied PR #155/)).toBeInTheDocument();
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
