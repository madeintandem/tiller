import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { ExecutionRecord, ExecutionSnapshot, NodePage } from "../../api";
import type { FetchState } from "../../hooks";
import ExecutionSources from "../ExecutionSources";
import GateCard from "../GateCard";
import CriterionRow from "../CriterionRow";
import NodeDetail from "../NodeDetail";

import HistorySection from "../../views/node/HistorySection";
import { ExecutionProvenance } from "../ExecutionSources";
const observation: ExecutionRecord = {
  id: "T-one", key: "epic/feature/task", branch: "origin/task-one", source: "remote_tracking",
  revision: "abcdef123456", observed_at: "2026-10-06T12:00:00Z", uncommitted: false,
  status: "in_review", owner: "developer", integration_status: "ready",
};

afterEach(() => { cleanup(); });

// The body of Home's Branches tab. Home owns the fetch (home.test.tsx pins
// the tab, its count, and the URL); these tests hand the component a settled
// fetch state directly.
function settled(snapshot: ExecutionSnapshot): FetchState<ExecutionSnapshot> {
  return { data: snapshot, error: null, loading: false };
}

describe("Execution sources", () => {
  it("shows each observation's provenance and the visibility diagnostics, with no disclosure to open", () => {
    render(<MemoryRouter><ExecutionSources state={settled({
      observations: [observation, { ...observation, id: "T-two", branch: "task-two", source: "local_branch", uncommitted: true }, { ...observation, id: "T-three", source: "working_tree" }],
      diagnostics: [{ branch: "task-missing", code: "state_unreadable", message: "Task state could not be read" }],
    })} /></MemoryRouter>);
    expect(screen.getByText("Task state could not be read", { exact: false })).toBeVisible();
    expect(screen.getByText("Visibility diagnostics · 1")).toBeVisible();
    expect(screen.getByText("Task observations · 3")).toBeVisible();
    expect(screen.getByText(/last fetched observation, not remote current/)).toBeVisible();
    expect(screen.getByText("local branch")).toBeVisible();
    expect(screen.getByText("working tree")).toBeVisible();
    expect(screen.getByText("uncommitted Tiller state")).toBeVisible();
    expect(screen.getByText(/The console does not fetch Git remotes/)).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: observation.key })[0]).toHaveAttribute("href", "/node/task/T-one");
  });

  it("leads an observation with the task title and follows it with the key; an untitled one shows its key once", () => {
    render(<MemoryRouter><ExecutionSources state={settled({
      observations: [{ ...observation, title: "Wire the form" }, { ...observation, id: "T-two", key: "epic/feature/other", branch: "origin/two" }],
      diagnostics: [],
    })} /></MemoryRouter>);
    const titled = screen.getByRole("link", { name: "Wire the form" });
    expect(titled).toHaveAttribute("href", "/node/task/T-one");
    expect(titled.closest("li")?.querySelector(".title-ref-key")?.textContent).toBe("epic/feature/task");
    const bare = screen.getByRole("link", { name: "epic/feature/other" }).closest("li") as HTMLElement;
    expect(bare.querySelector(".title-ref-key")).toBeNull();
    expect(bare.textContent?.split("epic/feature/other")).toHaveLength(2);
  });

  it("names which related tip is behind on the observation itself", () => {
    render(<MemoryRouter><ExecutionSources state={settled({
      observations: [{ ...observation, source: "local_branch", freshness: "local_branch_behind" }],
      diagnostics: [],
    })} /></MemoryRouter>);
    expect(screen.getByText("local branch is behind its tracking ref")).toBeVisible();
    expect(screen.queryByRole("region", { name: "Visibility diagnostics" })).not.toBeInTheDocument();
  });

  it("caps visibility diagnostics and reveals the rest on request", () => {
    const diagnostics = Array.from({ length: 11 }, (_, i) => ({ id: `T-${i}`, code: "branch_missing", message: `diagnostic ${i}` }));
    render(<MemoryRouter><ExecutionSources state={settled({ observations: [], diagnostics })} /></MemoryRouter>);
    const list = screen.getByRole("region", { name: "Visibility diagnostics" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(8);
    fireEvent.click(within(list).getByRole("button", { name: /3 more diagnostics/ }));
    expect(within(list).getAllByRole("listitem")).toHaveLength(11);
    expect(screen.getByText("No task execution observations discovered.")).toBeVisible();
  });

  it("keeps an observed task's open question without its command", () => {
    const page: NodePage = {
      node: { noun: "task", id: "T-one", status: "in_progress", next_owner: "human" },
      ref: "T-one", prose: null, relationships: {}, backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [], eval: null, execution: { ...observation, source: "working_tree", status: "in_progress" },
    };
    const queue = { entries: [{ kind: "answer_question", noun: "task", id: "T-one", ref: "T-one", command: "tiller answer Q-one --text <answer>", actor: "human" }] };
    render(<MemoryRouter><NodeDetail page={page} queue={queue} /></MemoryRouter>);
    expect(screen.getByText(/Answer · question/)).toBeVisible();
    expect(screen.queryByText("tiller answer Q-one --text <answer>")).not.toBeInTheDocument();
  });

  it("names execution discovery request failures", () => {
    render(<MemoryRouter><ExecutionSources state={{ data: null, error: "offline", loading: false }} /></MemoryRouter>);
    expect(screen.getByRole("alert")).toHaveTextContent("Execution sources unavailable: offline");
  });

  it.each(["working_tree", "local_branch", "remote_tracking"] as const)("does not suggest integration commands from %s observed task state", (source) => {
    const page: NodePage = {
      node: { noun: "task", id: "T-one", status: "in_review", next_owner: "human" },
      ref: "T-one", prose: null, relationships: {}, backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [], eval: null, execution: { ...observation, source },
      workflow: { phase: "acceptance", actor: "human", action: "attest", reason: "Approve", command: "tiller attest feature F-one --criterion C1 --status pass", review_id: "F-one", contract_status: "valid", acceptance_ready: false, submission_recorded: false },
    };
    render(<MemoryRouter><NodeDetail page={page} queue={{ entries: [] }} /></MemoryRouter>);
    if (source !== "working_tree") {
      expect(screen.getByText(/does not make lifecycle commands safe to run on the served checkout/)).toBeVisible();
    }
    expect(screen.queryByText("tiller attest feature F-one --criterion C1 --status pass")).not.toBeInTheDocument();
  });
});

describe("Branches tab leads with what matters", () => {
  const rows = [
    { ...observation, title: "Wire the form" },
    { ...observation, id: "T-two", key: "epic/feature/other", branch: "origin/two", observed_at: "2026-10-06T12:00:05Z" },
  ];

  it("states when it observed once, above the list, and no row repeats it", () => {
    const { container } = render(<MemoryRouter><ExecutionSources state={settled({ observations: rows, diagnostics: [] })} /></MemoryRouter>);
    const observed = container.querySelectorAll("time");
    expect(observed).toHaveLength(1);
    expect(screen.getByText(/^Observed/)).toBeVisible();
    const line = screen.getByText(/^Observed/);
    const list = within(screen.getByRole("region", { name: "Task observations" })).getAllByRole("list")[0];
    expect(line.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(list).queryByText(/observed/i)).toBeNull();
    expect(observed[0]).toHaveAttribute("title");
  });

  it("folds the two explanatory paragraphs into a closed disclosure", () => {
    const { container } = render(<MemoryRouter><ExecutionSources state={settled({ observations: rows, diagnostics: [] })} /></MemoryRouter>);
    const details = container.querySelector("details") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    expect(details.querySelector("summary")?.textContent).toBe("How branch observation works");
    expect(details).toHaveTextContent(/does not fetch Git remotes/);
    expect(details).toHaveTextContent(/Global History shows integration history/);
  });

  it("keeps the node page's provenance observed time by default and drops it on request", () => {
    const { rerender } = render(<MemoryRouter><ExecutionProvenance observation={observation} /></MemoryRouter>);
    expect(screen.getByText(/observed/)).toBeInTheDocument();
    rerender(<MemoryRouter><ExecutionProvenance observation={observation} showObserved={false} /></MemoryRouter>);
    expect(screen.queryByText(/observed/)).toBeNull();
  });

  it("leads a diagnostic with the task title, the branch in mono, and plain words with no code", () => {
    render(<MemoryRouter><ExecutionSources state={settled({
      observations: [],
      diagnostics: [
        { id: "T-ir6f27", title: "Extract params", branch: "tiller/foundation/extract-params", code: "branch_missing", message: "recorded execution branch cannot be resolved locally" },
        { id: "T-bare", branch: "tiller/bare", code: "brand_new_code", message: "something new" },
        { code: "discovery_unavailable", message: "cannot list refs" },
      ],
    })} /></MemoryRouter>);
    const region = screen.getByRole("region", { name: "Visibility diagnostics" });
    const [first, second, third] = within(region).getAllByRole("listitem");
    expect(within(first).getByRole("link", { name: "Extract params" })).toHaveAttribute("href", "/node/task/T-ir6f27");
    expect(first.querySelector(".title-ref-key")?.textContent).toBe("T-ir6f27");
    expect(first.querySelector(".mono:not(.title-ref-key)")?.textContent).toBe("tiller/foundation/extract-params");
    expect(first).toHaveTextContent("Branch not found");
    expect(first).toHaveTextContent("recorded execution branch cannot be resolved locally");
    expect(within(second).getByRole("link", { name: "T-bare" })).toBeInTheDocument();
    expect(second).toHaveTextContent("Brand new code");
    expect(third).toHaveTextContent("Branch discovery unavailable");
    expect(region.textContent).not.toMatch(/[a-z]+_[a-z]+/);
  });

  it("shows no raw underscore token on the tab outside ids, paths, and branch names", () => {
    const { container } = render(<MemoryRouter><ExecutionSources state={settled({
      observations: [{ ...observation, source: "local_branch", freshness: "local_branch_behind" }, { ...observation, id: "T-b", source: "remote_tracking", freshness: "remote_tracking_behind" }],
      diagnostics: [{ id: "T-x", code: "divergent_refs", message: "local and tracking diverge" }],
    })} /></MemoryRouter>);
    expect(container.textContent).not.toMatch(/[a-z]+_[a-z_]+/);
  });
});

describe("Observed execution history", () => {
  it("shows source history read failures even without loaded events", () => {
    render(<HistorySection history={[]} executionError="events unreadable at abcdef123456" />);
    expect(screen.getByRole("alert")).toHaveTextContent("Execution history unavailable: events unreadable at abcdef123456");
  });
});

describe("Source-qualified evidence", () => {

  it("renders an unavailable diagnostic instead of linking gate and criterion evidence", () => {
    const evidence = [{ ref: "artifacts/missing.txt", url: "/wrong", unavailable: "Not present at observed revision" }];
    const { container } = render(<><GateCard name="tests" gate={{ required: true, evidence }} /><CriterionRow criterion={{ id: "C1", statement: "It works", polarity: "positive", required: true, binding: { type: "test" } }} result={{ evidence }} /></>);
    expect(within(container).queryByRole("link")).not.toBeInTheDocument();
    expect(screen.getByText("evidence · Evidence unavailable: Not present at observed revision")).toHaveAttribute("title", "artifacts/missing.txt");
    expect(screen.getByText("eval C1 output · Evidence unavailable: Not present at observed revision")).toHaveAttribute("title", "artifacts/missing.txt");
  });
});
