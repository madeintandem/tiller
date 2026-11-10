// Home's Branches and Recovered reviews tabs lead with what matters: task
// titles, plain words for every code, and the detail folded away. Split from
// home.test.tsx, which is well past the file-size ceiling.

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ProjectSummary } from "../../api";
import Home from "../../views/Home";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function summary(): ProjectSummary {
  return {
    project: { name: "Acme Platform", key: "acme", kind: "greenfield" },
    contributors: [{ id: "jc", name: "JC Grubbs" }],
    counts: { epics: 0, features: 0, tasks: 0, questions: 0 },
    whose_move: { human: 0, agent: 0 },
  };
}

function emptyList(noun: string) {
  return { noun, count: 0, nodes: [] };
}

function routes(extra: Record<string, unknown>): Record<string, unknown> {
  return {
    "/api/status": summary(),
    "/api/queue": { entries: [] },
    "/api/ls?noun=epic": emptyList("epic"),
    "/api/ls?noun=feature": emptyList("feature"),
    "/api/ls?noun=task": emptyList("task"),
    "/api/ls?noun=question": emptyList("question"),
    "/api/ls?noun=bug": emptyList("bug"),
    "/api/ls?noun=chore": emptyList("chore"),
    "/api/ls?noun=tweak": emptyList("tweak"),
    "/api/recap/peek": { since: "2026-07-24T00:00:00Z", reader: "jc", total: 0, asks: 0 },
    "/api/recap?window=24h": { since: "", window: "24h", reader: "jc", total: 0, groups: [] },
    "/api/board": { contributors: [] },
    "/api/next": { reader: "jc", caught_up: true, candidates: [] },
    "/api/drift": { compared: 0 },
    "/api/execution": { observations: [], diagnostics: [] },
    "/api/work-items": { open: [], closed: [], attachments: [] },
    ...extra,
  };
}

function stubFetch(table: Record<string, unknown>) {
  const spy = vi.fn((input: unknown) => {
    const url = String(input);
    if (url in table) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(table[url]) });
    }
    return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
  });
  vi.stubGlobal("fetch", spy);
  return spy;
}

function renderHome(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Home summary={summary()} summaryError={null} />
    </MemoryRouter>,
  );
}

const recovered = {
  pr: 155, branch: "tiller/reviews/features/billing/refunds", head: "git:done-head",
  merge_commit: "git:done-result", integration_branch: "main", basis: "contained",
  recovered_by: "jc", recovered_at: "2026-10-07T12:00:00Z", reason: "Authorized recovery",
  tasks: [
    { id: "T-2il2d6", path: ".tiller/epics/a/features/b/tasks/c/task.yml", state: "git:submitted" },
    { id: "T-untitled", path: ".tiller/epics/a/features/b/tasks/d/task.yml", state: "git:submitted-two" },
  ],
  policy_snapshots: [],
};

// Only node keys, ids, paths, and branch names may carry an underscore.
const underscoreToken = /[a-z]+_[a-z_]+/;
const pathsAndBranches = /\.tiller\/\S+|tiller\/\S+/g;

describe("Home Branches tab", () => {
  it("leads a diagnostic with the task title and plain words, never the raw code", async () => {
    stubFetch(routes({
      "/api/execution": { observations: [], diagnostics: [
        { id: "T-ir6f27", title: "Extract params", branch: "tiller/foundation/routing-layer/extract-params", code: "branch_missing", message: "recorded execution branch cannot be resolved locally" },
      ] },
    }));
    renderHome("/?tab=branches");
    const region = await screen.findByRole("region", { name: "Visibility diagnostics" });
    expect(within(region).getByRole("link", { name: "Extract params" })).toHaveAttribute("href", "/node/task/T-ir6f27");
    expect(region).toHaveTextContent("Branch not found");
    expect(region).toHaveTextContent("recorded execution branch cannot be resolved locally");
    expect(region.textContent).not.toContain("branch_missing");
    const tab = screen.getByRole("region", { name: "Execution sources" });
    expect((tab.textContent ?? "").replace(pathsAndBranches, "")).not.toMatch(underscoreToken);
  });
});

describe("Home Recovered reviews tab", () => {
  const features = { noun: "feature", count: 1, nodes: [{
    id: "F-1", key: "billing/refunds", ref: "billing/refunds (F-1)", title: "Refunds", status: "done", recovered_review: recovered,
    current_acceptance: { node_id: "F-1", contract_status: "valid", acceptance_ready: true },
    recovered_crossings_pending: false,
  }] };
  const tasks = { noun: "task", count: 1, nodes: [{ id: "T-2il2d6", key: "a/b/c", ref: "a/b/c (T-2il2d6)", title: "Extract params", status: "done" }] };

  async function openTab() {
    fireEvent.click(await within(screen.getByRole("navigation", { name: "Home view" })).findByRole("button", { name: /Recovered reviews/ }));
    return screen.findByRole("region", { name: /recovered historical review/i });
  }

  it("shows riding tasks by title from the task list Home already read, with the id when no title is known", async () => {
    const spy = stubFetch(routes({ "/api/ls?noun=feature": features, "/api/ls?noun=task": tasks }));
    renderHome("/");
    const panel = await openTab();
    expect(within(panel).getByRole("link", { name: "Extract params" })).toHaveAttribute("href", "/node/task/T-2il2d6");
    expect(within(panel).getByRole("link", { name: "T-untitled" })).toBeInTheDocument();
    expect(spy.mock.calls.filter((call) => String(call[0]).startsWith("/api/node/task"))).toHaveLength(0);
    expect(panel.querySelector("details")).not.toHaveAttribute("open");
    expect((panel.textContent ?? "").replace(pathsAndBranches, "")).not.toMatch(underscoreToken);
  });
});
