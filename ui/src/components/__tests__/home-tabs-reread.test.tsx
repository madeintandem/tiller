// A generation bump (the topbar refresh, and later a window regaining focus)
// refetches every read Home holds. A tab whose presence or count comes from
// fetched data must stay put while that read is pending, so the reader keeps
// their place. Split from home.test.tsx, which is well past the size ceiling.

import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ListResult, ProjectSummary } from "../../api";
import { bumpFetchGeneration } from "../../api";
import Home from "../../views/Home";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

const summary: ProjectSummary = {
  project: { name: "Acme Platform", key: "acme", kind: "greenfield" },
  contributors: [{ id: "jc", name: "JC Grubbs" }],
  counts: { epics: 0, features: 0, tasks: 0, questions: 0 },
  whose_move: { human: 0, agent: 0 },
};

function emptyList(noun: string): ListResult {
  return { noun, count: 0, nodes: [] };
}

function routes(extra: Record<string, unknown>): Record<string, unknown> {
  return {
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
    "/api/drift": { compared: 0, uncovered: [] },
    "/api/execution": { observations: [], diagnostics: [] },
    ...extra,
  };
}

// A stub whose next read of one path is held pending until the test settles
// it: the shape of a refetch in flight after a generation bump.
function stubHeld(table: Record<string, unknown>, heldPath: string) {
  let pending: ((body: unknown) => void) | null = null;
  let armed = false;
  vi.stubGlobal("fetch", vi.fn((input: unknown) => {
    const url = String(input);
    if (url === heldPath && armed) {
      armed = false;
      return new Promise((resolve) => {
        pending = (body) => resolve({ ok: true, json: () => Promise.resolve(body) });
      });
    }
    if (url in table) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(table[url]) });
    }
    return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
  }));
  return {
    refetch: () => {
      armed = true;
      act(() => bumpFetchGeneration());
    },
    settle: async (body: unknown) => {
      await waitFor(() => expect(pending).not.toBeNull());
      await act(async () => pending?.(body));
    },
  };
}

function renderHome(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Home summary={summary} summaryError={null} />
    </MemoryRouter>,
  );
}

function recoveredFeatures(title: string): ListResult {
  return { noun: "feature", count: 1, nodes: [{
    id: "F-1", key: "billing/refunds", ref: "billing/refunds (F-1)", title, status: "active",
    recovered_review: {
      pr: 155, branch: "tiller/reviews/features/billing/refunds", head: "git:head",
      merge_commit: "git:merge", integration_branch: "main", basis: "ancestry",
      recovered_by: "jc", recovered_at: "2026-10-07T12:00:00Z", reason: "Authorized recovery",
      tasks: [], policy_snapshots: [],
    },
    current_acceptance: { node_id: "F-1", contract_status: "valid", acceptance_ready: true },
  }] };
}

describe("Home tabs while the page rereads", () => {
  it("keeps the Recovered reviews tab and its records on screen until the new list lands", async () => {
    const held = stubHeld(routes({ "/api/ls?noun=feature": recoveredFeatures("Refund flow") }), "/api/ls?noun=feature");
    renderHome("/?tab=recovered");
    expect(await screen.findByText("Refund flow")).toBeInTheDocument();

    held.refetch();

    expect(screen.getByRole("tab", { name: /^Recovered reviews/ })).toBeInTheDocument();
    expect(screen.getByText("Refund flow")).toBeInTheDocument();

    await held.settle(recoveredFeatures("Refund flow, renamed"));
    expect(await screen.findByText("Refund flow, renamed")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /^Recovered reviews/ })).toBeInTheDocument();
  });

  it("keeps the Branches tab and its count on screen until the new read lands", async () => {
    const diagnostic = { id: "T-two", code: "branch_missing", message: "No branch for this task" };
    const held = stubHeld(routes({ "/api/execution": { observations: [], diagnostics: [diagnostic] } }), "/api/execution");
    renderHome("/?tab=branches");
    const rail = screen.getByRole("tablist", { name: "Home view" });
    expect(await within(rail).findByRole("tab", { name: "Branches · 1" })).toBeInTheDocument();

    held.refetch();

    expect(within(rail).getByRole("tab", { name: "Branches · 1" })).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByText(/Loading execution sources/)).not.toBeInTheDocument();

    await held.settle({ observations: [], diagnostics: [] });
    expect(await within(rail).findByRole("tab", { name: "Branches" })).toBeInTheDocument();
  });
});
