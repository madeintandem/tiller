// Home's stat band: every tile links to the list it counts, the title sits
// above the tab rail on every tab, and a tile that renders from a retained
// read says so when its reread fails. Split from home.test.tsx, which is well
// past the size ceiling.

import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { DriftReportView, ListResult, ListRow, ProjectSummary } from "../../api";
import { bumpFetchGeneration } from "../../api";
import FilterMode from "../../views/explore/FilterMode";
import { activeLedgerStatus, ledgerStatusTabs } from "../../views/explore/ledgerStatus";
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

function row(id: string, status: string, column?: string): ListRow {
  return { id, key: id.toLowerCase(), ref: `${id.toLowerCase()} (${id})`, title: `Title ${id}`, status, column } as ListRow;
}

function list(noun: string, nodes: ListRow[]): ListResult {
  return { noun, count: nodes.length, nodes };
}

const drift: DriftReportView = {
  compared: 5,
  uncovered: [{ marker: "stale_spec", noun: "feature", id: "F-2", title: "Form engine", reason: "no_completion_record" }],
};

function table(extra: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    "/api/queue": { entries: [] },
    "/api/ls?noun=epic": list("epic", [row("E-1", "active"), row("E-2", "draft"), row("E-3", "done")]),
    "/api/ls?noun=feature": list("feature", [
      row("F-1", "ready", "active"), row("F-2", "active"), row("F-3", "draft"), row("F-4", "done"),
    ]),
    "/api/ls?noun=task": list("task", [row("T-1", "active")]),
    "/api/ls?noun=question": list("question", [row("Q-1", "open"), row("Q-2", "answered")]),
    "/api/ls?noun=bug": list("bug", []),
    "/api/ls?noun=chore": list("chore", []),
    "/api/ls?noun=tweak": list("tweak", []),
    "/api/recap/peek": { since: "2026-07-24T00:00:00Z", reader: "jc", total: 0, asks: 0 },
    "/api/recap?window=24h": { since: "", window: "24h", reader: "jc", total: 0, groups: [] },
    "/api/board": { contributors: [] },
    "/api/next": { reader: "jc", caught_up: true, candidates: [] },
    "/api/drift": drift,
    "/api/execution": { observations: [], diagnostics: [] },
    ...extra,
  };
}

// A stub whose listed paths fail (500) while `failing` holds them.
function stubFetch(data: Record<string, unknown>) {
  const failing = new Set<string>();
  const spy = vi.fn((input: unknown) => {
    const url = String(input);
    if (failing.has(url)) {
      return Promise.resolve({ ok: false, status: 500, statusText: "Server Error", text: () => Promise.resolve("") });
    }
    if (url in data) return Promise.resolve({ ok: true, json: () => Promise.resolve(data[url]) });
    return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
  });
  vi.stubGlobal("fetch", spy);
  return { spy, failing, reread: () => act(() => bumpFetchGeneration()) };
}

function renderHome(path = "/") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Home summary={summary} summaryError={null} />
    </MemoryRouter>,
  );
}

describe("Home stat tiles link to the list they count", () => {
  it("links Epics to the epic list with a name that says what it opens", async () => {
    stubFetch(table());
    renderHome();
    const link = await screen.findByRole("link", { name: "3 epics, open the epic list" });
    expect(link).toHaveAttribute("href", "/explore?mode=filter&noun=epics");
    expect(link).toHaveTextContent("3");
    expect(link).toHaveTextContent("Epics");
  });

  it("links Questions to the open questions ledger, which reads that same status", async () => {
    stubFetch(table());
    renderHome();
    const link = await screen.findByRole("link", { name: "2 questions, open the questions list" });
    const href = link.getAttribute("href") as string;
    expect(href).toBe("/questions?status=open");
    const rows = [row("Q-1", "open"), row("Q-2", "answered")];
    const tabs = ledgerStatusTabs(rows, ["open", "answered", "cancelled"]);
    expect(activeLedgerStatus(new URL(href, "http://x").searchParams, tabs)).toBe("open");
  });

  it("links a tile with a zero count", async () => {
    stubFetch(table({ "/api/ls?noun=epic": list("epic", []) }));
    renderHome();
    const link = await screen.findByRole("link", { name: "0 epics, open the epic list" });
    expect(link).toHaveAttribute("href", "/explore?mode=filter&noun=epics");
  });

  it("links each funnel segment to the features list at that status, zeros included", async () => {
    stubFetch(table());
    const { container } = renderHome();
    await screen.findByRole("link", { name: "3 epics, open the epic list" });
    const segs = [...container.querySelectorAll(".funnel-row .fseg")];
    expect(segs.map((el) => el.getAttribute("href"))).toEqual(
      ["draft", "specced", "ready", "active", "in_review", "done"].map(
        (s) => `/explore?mode=filter&noun=features&status=${s}`,
      ),
    );
    // F-1 reads active through its column, F-2 is active, F-3 draft, F-4 done.
    expect(screen.getByRole("link", { name: "2 Active features, open the list" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "0 Specified features, open the list" })).toBeInTheDocument();
  });

  it("holds no interactive element inside a link tile", async () => {
    stubFetch(table());
    const { container } = renderHome();
    await screen.findByRole("link", { name: "3 epics, open the epic list" });
    const tiles = container.querySelectorAll("a.stat, a.fseg");
    expect(tiles.length).toBeGreaterThan(5);
    for (const tile of tiles) {
      expect(tile.querySelector("a, button, input, select, textarea")).toBeNull();
    }
  });

  it("opens a funnel number as a list of exactly that many rows", async () => {
    const features = table()["/api/ls?noun=feature"] as ListResult;
    const active = features.nodes.filter((r) => (r.column ?? r.status) === "active");
    const { spy } = stubFetch(table({
      "/api/ls?noun=features&status=active": {
        ...list("feature", active),
        facets: [{ axis: "status", options: [{ value: "active", label: "active", count: active.length, selected: true }] }],
      },
    }));
    render(
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route path="/" element={<Home summary={summary} summaryError={null} />} />
          <Route path="/explore" element={<FilterMode />} />
        </Routes>
      </MemoryRouter>,
    );
    const seg = await screen.findByRole("link", { name: "2 Active features, open the list" });
    fireEvent.click(seg);
    expect(await screen.findByText("2 results")).toBeInTheDocument();
    const urls = spy.mock.calls.map(([u]) => String(u));
    expect(urls).toContain("/api/ls?noun=features&status=active");
    expect(urls).not.toContain("/api/ls?noun=features");
  });
});

describe("Home title sits above the tab rail", () => {
  const withRecovered = table({
    "/api/ls?noun=feature": list("feature", [{
      ...row("F-1", "active"),
      recovered_review: {
        pr: 1, branch: "b", head: "h", merge_commit: "m", integration_branch: "main", basis: "ancestry",
        recovered_by: "jc", recovered_at: "2026-10-07T12:00:00Z", reason: "r", tasks: [], policy_snapshots: [],
      },
    } as ListRow]),
  });

  for (const tab of ["", "?tab=branches", "?tab=recovered", "?tab=drift"]) {
    it(`renders one h1 and the meta line above the rail on ${tab || "overview"}`, async () => {
      stubFetch(withRecovered);
      renderHome(`/${tab}`);
      const rail = await screen.findByRole("navigation", { name: "Home view" });
      await screen.findByRole("button", { name: /^Drift/ });
      const h1s = screen.getAllByRole("heading", { level: 1 });
      expect(h1s).toHaveLength(1);
      expect(h1s[0]).toHaveTextContent("Acme Platform");
      expect(screen.getByText("acme", { selector: ".mono" })).toBeInTheDocument();
      expect(screen.getByText(/Contributors:/)).toBeInTheDocument();
      expect(h1s[0].compareDocumentPosition(rail) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });
  }

  it("keeps the stat band on Overview only", async () => {
    stubFetch(table());
    renderHome("/?tab=branches");
    await screen.findByRole("navigation", { name: "Home view" });
    expect(screen.queryByRole("link", { name: /open the epic list/ })).not.toBeInTheDocument();
  });
});

describe("Home tiles from retained reads say when a reread failed", () => {
  it("keeps the last numbers, notes the error, and clears it on a good reread", async () => {
    const s = stubFetch(table());
    const { container } = renderHome();
    await screen.findByRole("link", { name: "3 epics, open the epic list" });
    expect(container.querySelectorAll(".stat-stale")).toHaveLength(0);

    for (const p of ["/api/ls?noun=epic", "/api/ls?noun=feature", "/api/drift"]) s.failing.add(p);
    s.reread();

    await waitFor(() => expect(container.querySelectorAll(".stat-stale")).toHaveLength(3));
    // The numbers stay.
    expect(screen.getByRole("link", { name: "3 epics, open the epic list" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "2 Active features, open the list" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^1 drift/i })).toBeInTheDocument();
    for (const note of container.querySelectorAll(".stat-stale")) {
      expect(note.textContent).toMatch(/out of date/i);
      expect(note.textContent).toMatch(/500/);
    }
    // A tile that is not from a retained read is not marked.
    expect(within(screen.getByRole("link", { name: "2 questions, open the questions list" })).queryByText(/out of date/i))
      .not.toBeInTheDocument();

    s.failing.clear();
    s.reread();
    await waitFor(() => expect(container.querySelectorAll(".stat-stale")).toHaveLength(0));
    expect(screen.getByRole("link", { name: "3 epics, open the epic list" })).toBeInTheDocument();
  });
});
