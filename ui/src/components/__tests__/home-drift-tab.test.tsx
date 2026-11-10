// Drift lives on Home as a tab (R4): the report that used to be its own page
// renders under Home's tab rail from the /api/drift response Home already
// holds for its Drift stat tile, and the old /drift address forwards there.
// Split from home.test.tsx, which is well past the file-size ceiling.

import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { DriftReportView, ProjectSummary } from "../../api";
import { bumpFetchGeneration } from "../../api";
import App from "../../App";
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

function routes(drift: DriftReportView): Record<string, unknown> {
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
    "/api/drift": drift,
    "/api/execution": { observations: [], diagnostics: [] },
    "/api/work-items": { open: [], closed: [], attachments: [] },
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

// A stub that never answers one path (a first read still in flight), or
// answers it with a server error.
function stubFetchHolding(table: Record<string, unknown>, heldPath: string) {
  vi.stubGlobal("fetch", vi.fn((input: unknown) => {
    const url = String(input);
    if (url === heldPath) return new Promise(() => {});
    if (url in table) return Promise.resolve({ ok: true, json: () => Promise.resolve(table[url]) });
    return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
  }));
}

function stubFetchFailing(table: Record<string, unknown>, failingPath: string) {
  vi.stubGlobal("fetch", vi.fn((input: unknown) => {
    const url = String(input);
    if (url === failingPath) {
      return Promise.resolve({ ok: false, status: 500, statusText: "Internal Server Error", text: () => Promise.resolve("disk on fire") });
    }
    if (url in table) return Promise.resolve({ ok: true, json: () => Promise.resolve(table[url]) });
    return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
  }));
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

function Where() {
  const location = useLocation();
  return <p data-testid="where">{`${location.pathname}${location.search}${location.hash}`}</p>;
}

function renderHome(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Home summary={summary()} summaryError={null} />
      <Where />
    </MemoryRouter>,
  );
}

const withCandidates: DriftReportView = {
  compared: 5,
  candidates: {
    active_basis: [{ marker: "active_basis", noun: "feature", id: "F-1", title: "Audit export", detail: "feature.md changed" }],
    missing_surface: [{ marker: "missing_surface", noun: "task", id: "T-1", title: "Legacy calc", detail: "gone" }],
  },
  uncovered: [{ marker: "stale_spec", noun: "feature", id: "F-2", title: "Form engine", reason: "no_completion_record" }],
};

const uncoveredOnly: DriftReportView = {
  compared: 5,
  uncovered: [{ marker: "stale_spec", noun: "feature", id: "F-2", title: "Form engine", reason: "no_completion_record" }],
};

describe("Home Drift tab", () => {
  it("counts candidates plus uncovered and tints the count when candidates exist", async () => {
    stubFetch(routes(withCandidates));
    renderHome("/");

    const tab = await screen.findByRole("tab", { name: "Drift · 3, needs attention" });
    expect(tab.querySelector(".tab-count-warning")).not.toBeNull();
  });

  it("leaves the count untinted when only uncovered rows exist", async () => {
    stubFetch(routes(uncoveredOnly));
    renderHome("/");

    const tab = await screen.findByRole("tab", { name: "Drift · 1" });
    expect(tab.querySelector(".tab-count-warning")).toBeNull();
    expect(tab.querySelector(".tab-count-mark")).toBeNull();
    expect(tab.querySelector(".tab-count")).not.toBeNull();
  });

  it("is absent when the sweep found nothing", async () => {
    stubFetch(routes({ compared: 4, uncovered: [] }));
    renderHome("/");

    await screen.findByRole("tab", { name: "Overview" });
    await waitFor(() => expect(screen.getByRole("tab", { name: "Branches" })).toBeInTheDocument());
    expect(screen.queryByRole("tab", { name: /^Drift/ })).not.toBeInTheDocument();
  });

  it("renders the report from the response Home already holds, without a second request", async () => {
    const spy = stubFetch(routes(withCandidates));
    renderHome("/?tab=drift");

    expect(await screen.findByText(/Work rests on an older basis \(1\)/)).toBeInTheDocument();
    // The title sits above the rail on every tab, so the page's one h1 is
    // Home's title and the report adds none.
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    const driftCalls = spy.mock.calls.filter(([url]) => String(url) === "/api/drift");
    expect(driftCalls).toHaveLength(1);
  });

  it("selecting the tab rides ?tab=drift", async () => {
    stubFetch(routes(withCandidates));
    renderHome("/");

    const tab = await screen.findByRole("tab", { name: "Drift · 3, needs attention" });
    tab.click();
    await waitFor(() => expect(screen.getByTestId("where")).toHaveTextContent("/?tab=drift"));
  });

  it("opens the tab on ?tab=drift with nothing to show, and says the sweep came back clean", async () => {
    stubFetch(routes({ compared: 4, uncovered: [] }));
    renderHome("/?tab=drift");

    expect(await screen.findByText(/nothing moved/)).toBeInTheDocument();
    const rail = screen.getByRole("tablist", { name: "Home view" });
    expect(within(rail).getByRole("tab", { name: "Drift" })).toHaveAttribute("aria-selected", "true");
  });

  it("opens the tab on ?tab=drift while the sweep is still loading", async () => {
    stubFetchHolding(routes(withCandidates), "/api/drift");
    renderHome("/?tab=drift");

    expect(await screen.findByText("Sweeping the working tree...")).toBeInTheDocument();
    const rail = screen.getByRole("tablist", { name: "Home view" });
    expect(within(rail).getByRole("tab", { name: "Drift" })).toHaveAttribute("aria-selected", "true");
  });

  it("opens the tab on ?tab=drift when the sweep failed, and shows the error", async () => {
    stubFetchFailing(routes(withCandidates), "/api/drift");
    renderHome("/?tab=drift");

    expect(await screen.findByText(/disk on fire/)).toBeInTheDocument();
    const rail = screen.getByRole("tablist", { name: "Home view" });
    expect(within(rail).getByRole("tab", { name: "Drift" })).toHaveAttribute("aria-selected", "true");
  });

  it("the Drift stat tile links to the tab", async () => {
    stubFetch(routes(withCandidates));
    renderHome("/");

    const link = await screen.findByRole("link", { name: /drift/i });
    expect(link).toHaveAttribute("href", "/?tab=drift");
  });
});

describe("Home Drift tab while the page rereads", () => {
  it("keeps the tab, its count, and the report on screen until the new answer lands", async () => {
    const held = stubHeld(routes(withCandidates), "/api/drift");
    renderHome("/?tab=drift");
    expect(await screen.findByText(/Work rests on an older basis \(1\)/)).toBeInTheDocument();

    held.refetch();

    expect(screen.getByRole("tab", { name: "Drift · 3, needs attention" })).toBeInTheDocument();
    expect(screen.getByText(/Work rests on an older basis \(1\)/)).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent("/?tab=drift");

    await held.settle(uncoveredOnly);
    expect(await screen.findByRole("tab", { name: "Drift · 1" })).toBeInTheDocument();
    expect(screen.queryByText(/Work rests on an older basis/)).not.toBeInTheDocument();
  });

  it("keeps the tab under a reader on it when a reread comes back clean", async () => {
    const held = stubHeld(routes(withCandidates), "/api/drift");
    renderHome("/");
    (await screen.findByRole("tab", { name: "Drift · 3, needs attention" })).click();
    await waitFor(() => expect(screen.getByTestId("where")).toHaveTextContent("/?tab=drift"));

    held.refetch();
    await held.settle({ compared: 4, uncovered: [] });
    expect(await screen.findByText(/nothing moved/)).toBeInTheDocument();
    const rail = screen.getByRole("tablist", { name: "Home view" });
    expect(within(rail).getByRole("tab", { name: "Drift" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByTestId("where")).toHaveTextContent("/?tab=drift");
  });
});

describe("App: the old /drift address", () => {
  it.each([
    ["/drift", "/?tab=drift"],
    ["/drift#anything", "/?tab=drift#anything"],
  ])("%s lands on %s", async (from, to) => {
    stubFetch(routes(withCandidates));
    render(
      <MemoryRouter initialEntries={[from]}>
        <App />
        <Where />
      </MemoryRouter>,
    );

    await waitFor(() => expect(screen.getByTestId("where")).toHaveTextContent(to));
    expect(await screen.findByText(/Work rests on an older basis \(1\)/)).toBeInTheDocument();
    expect(screen.queryByText(/No such page/i)).not.toBeInTheDocument();
  });

  it("the top nav has no Drift entry", async () => {
    stubFetch(routes(withCandidates));
    render(
      <MemoryRouter initialEntries={["/"]}>
        <App />
      </MemoryRouter>,
    );
    await screen.findByRole("tab", { name: "Drift · 3, needs attention" });
    expect(screen.queryByRole("link", { name: /^Drift$/ })).not.toBeInTheDocument();
  });
});
