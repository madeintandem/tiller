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
import QuestionsPane from "../../views/explore/QuestionsPane";
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

  it("links Questions to the ledger's All view, stated in the URL, and shows as many rows as the tile counts", async () => {
    // Fixed data: three questions, two open and one answered.
    const questions = list("question", [row("Q-1", "open"), row("Q-2", "open"), row("Q-3", "answered")]);
    stubFetch(table({ "/api/ls?noun=question": questions }));
    const { container } = render(
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route path="/" element={<Home summary={summary} summaryError={null} />} />
          <Route path="/questions" element={<QuestionsPane />} />
        </Routes>
      </MemoryRouter>,
    );
    const link = await screen.findByRole("link", { name: "3 questions, open the questions list" });
    expect(link).toHaveAttribute("href", "/questions?status=all");
    fireEvent.click(link);
    const all = await screen.findByRole("button", { name: "All · 3" });
    expect(all).toHaveAttribute("aria-current", "true");
    expect(container.querySelectorAll(".qlist > *")).toHaveLength(3);
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

  function renderWithExplore() {
    return render(
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route path="/" element={<Home summary={summary} summaryError={null} />} />
          <Route path="/explore" element={<FilterMode />} />
        </Routes>
      </MemoryRouter>,
    );
  }

  it("opens the Epics number as a list of that many rows, cancelled and archived epics included", async () => {
    // Fixed data, the same five epics under the singular noun Home reads and
    // the plural noun Explore requests (the server maps both to one noun).
    const epics = list("epic", [
      row("E-1", "active"), row("E-2", "draft"), row("E-3", "done"), row("E-4", "cancelled"), row("E-5", "archived"),
    ]);
    const { spy } = stubFetch(table({
      "/api/ls?noun=epic": epics,
      "/api/ls?noun=epics": { ...epics, facets: [] },
    }));
    renderWithExplore();
    const link = await screen.findByRole("link", { name: "5 epics, open the epic list" });
    fireEvent.click(link);
    expect(await screen.findByText("5 results")).toBeInTheDocument();
    expect(spy.mock.calls.map(([u]) => String(u))).toContain("/api/ls?noun=epics");
  });

  // A stored-ready feature with a task in flight reads Active (column
  // active). Fixed responses per status, written by hand, not derived from the
  // rule the tile applies: the server filters on displayed status.
  const funnelFeatures = list("feature", [
    row("F-1", "ready", "active"), row("F-2", "ready"), row("F-3", "active"), row("F-4", "draft"),
  ]);
  const byStatus: Record<string, ListRow[]> = {
    ready: [row("F-2", "ready")],
    active: [row("F-1", "ready", "active"), row("F-3", "active")],
  };
  for (const [status, expected] of [["ready", 1], ["active", 2]] as const) {
    it(`opens the ${status} funnel number as a list of that many rows`, async () => {
      const facet = { axis: "status", options: [{ value: status, label: status, count: expected, selected: true }] };
      const { spy } = stubFetch(table({
        "/api/ls?noun=feature": funnelFeatures,
        [`/api/ls?noun=features&status=${status}`]: { ...list("feature", byStatus[status]), facets: [facet] },
      }));
      renderWithExplore();
      const label = status === "ready" ? "Ready" : "Active";
      const seg = await screen.findByRole("link", { name: `${expected} ${label} ${expected === 1 ? "feature" : "features"}, open the list` });
      expect(seg).toHaveAttribute("href", `/explore?mode=filter&noun=features&status=${status}`);
      fireEvent.click(seg);
      expect(await screen.findByText(`${expected} ${expected === 1 ? "result" : "results"}`)).toBeInTheDocument();
      expect(spy.mock.calls.map(([u]) => String(u))).toContain(`/api/ls?noun=features&status=${status}`);
    });
  }
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
    const epicsTile = screen.getByRole("link", { name: "3 epics, open the epic list" });
    expect(screen.getByRole("link", { name: "2 Active features, open the list" })).toBeInTheDocument();
    const driftTile = screen.getByRole("link", { name: /^1 drift/i });
    // Screen readers hear it: the link's description carries the note.
    expect(epicsTile).toHaveAccessibleDescription(/out of date.*500/i);
    expect(driftTile).toHaveAccessibleDescription(/out of date.*500/i);
    for (const note of container.querySelectorAll(".stat-stale")) {
      expect(note.textContent).toMatch(/out of date/i);
      expect(note.textContent).toMatch(/500/);
      expect(note.getAttribute("title")).toMatch(/500/);
    }
    // A tile that is not from a retained read is not marked.
    expect(within(screen.getByRole("link", { name: "2 questions, open the questions list" })).queryByText(/out of date/i))
      .not.toBeInTheDocument();

    s.failing.clear();
    s.reread();
    await waitFor(() => expect(container.querySelectorAll(".stat-stale")).toHaveLength(0));
    expect(screen.getByRole("link", { name: "3 epics, open the epic list" })).not.toHaveAccessibleDescription(/out of date/i);
  });
});

describe("the stat band is a named group", () => {
  it("names the scrolling box Project numbers", async () => {
    stubFetch(table());
    renderHome();
    const band = await screen.findByRole("group", { name: "Project numbers" });
    expect(within(band).getByRole("link", { name: "3 epics, open the epic list" })).toBeInTheDocument();
  });
});
