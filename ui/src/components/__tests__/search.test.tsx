// Explore search (ui-surface.md, "Explore"): results arrive grouped, one row
// per node and one per unowned document, nodes ranked by how they matched.
// The page puts an exact id or key match first, groups the rest by kind under
// tabs that carry each kind's count, and keeps each row to its title, key,
// status, and first few matching lines. These render through <App/> so the
// route and query-string contract is under test, not a copy of it.

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SearchDocumentResult, SearchNodeResult, SearchResult } from "../../api";
import App from "../../App";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function stubFetch(routes: Record<string, unknown>) {
  const fetchMock = vi.fn((input: unknown) => {
    const url = String(input);
    if (url in routes) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[url]) });
    }
    return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  );
}

function node(over: Partial<SearchNodeResult> & Pick<SearchNodeResult, "noun" | "id">): SearchNodeResult {
  return {
    key: undefined,
    title: "Untitled",
    status: "ready",
    ref: over.id,
    match: "text",
    count: 0,
    hits: [],
    ...over,
  };
}

function doc(path: string, lines: number[]): SearchDocumentResult {
  return {
    path,
    count: lines.length,
    hits: lines.map((line) => ({ path, line, snippet: `payment note ${String(line)}` })),
  };
}

function result(nodes: SearchNodeResult[], documents: SearchDocumentResult[] = [], query = "payment"): SearchResult {
  const kinds = new Map<string, number>();
  for (const n of nodes) kinds.set(n.noun, (kinds.get(n.noun) ?? 0) + 1);
  if (documents.length > 0) kinds.set("document", documents.length);
  return {
    query,
    count: nodes.length + documents.length,
    truncated: false,
    kinds: [...kinds].map(([kind, count]) => ({ kind, count })),
    nodes,
    documents,
  };
}

const bug = node({
  noun: "bug", id: "B-aki9u3", key: "bugs/admin-filter-alignment", title: "Admin filter alignment",
  status: "proposed", ref: "bugs/admin-filter-alignment (B-aki9u3)", match: "id",
});

describe("Explore search: exact matches", () => {
  it("puts the node an id names first, in its own Exact match section with its kind", async () => {
    const other = node({
      noun: "feature", id: "F-1", key: "admin/filters", title: "Filters", match: "text", count: 1,
      hits: [{ path: ".tiller/epics/admin/features/filters/feature.md", line: 4, snippet: "see B-aki9u3" }],
    });
    stubFetch({ "/api/search?q=B-aki9u3": result([bug, other], [], "B-aki9u3") });

    renderAt("/explore?mode=search&q=B-aki9u3");

    const exact = await screen.findByRole("region", { name: "Exact match" });
    const link = within(exact).getByRole("link", { name: "Admin filter alignment" });
    expect(link).toHaveAttribute("href", "/node/bug/B-aki9u3");
    expect(within(exact).getByText("Bug")).toBeInTheDocument();
    expect(within(exact).getByText("ID match")).toBeInTheDocument();
    // The exact node is not repeated in its kind's section below.
    expect(screen.getAllByRole("link", { name: "Admin filter alignment" })).toHaveLength(1);
    expect(screen.getByRole("region", { name: "Features" })).toBeInTheDocument();
  });
});

describe("Explore search: grouped by kind", () => {
  it("offers one tab per kind with its count, and an All tab with the total", async () => {
    stubFetch({
      "/api/search?q=payment": result(
        [node({ noun: "epic", id: "E-1", title: "Payments", match: "title" }), node({ noun: "task", id: "T-1" }), node({ noun: "task", id: "T-2" })],
        [doc(".tiller/design/billing.md", [3])],
      ),
    });

    renderAt("/explore?mode=search&q=payment");

    const tabs = await screen.findByRole("navigation", { name: "Result kind" });
    expect(within(tabs).getAllByRole("button").map((b) => b.textContent)).toEqual([
      "All · 4", "Epics · 1", "Tasks · 2", "Documents · 1",
    ]);
    expect(screen.getByRole("region", { name: "Epics" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Tasks" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Documents" })).toBeInTheDocument();
  });

  it("caps each section on All and links to the kind's own tab for the rest", async () => {
    const tasks = Array.from({ length: 8 }, (_, i) => node({ noun: "task", id: `T-${String(i)}`, title: `Task ${String(i)}` }));
    const fetchMock = stubFetch({
      "/api/search?q=payment": result(tasks),
      "/api/search?q=payment&kind=task": { ...result(tasks), kind: "task" },
    });

    renderAt("/explore?mode=search&q=payment");

    const section = await screen.findByRole("region", { name: "Tasks" });
    expect(within(section).getAllByRole("listitem")).toHaveLength(5);
    fireEvent.click(within(section).getByRole("button", { name: "All 8 tasks ▸" }));

    expect(await screen.findByText("Task 7")).toBeInTheDocument();
    expect(fetchMock.mock.calls.map((c) => String(c[0]))).toContain("/api/search?q=payment&kind=task");
    const tabs = screen.getByRole("navigation", { name: "Result kind" });
    expect(within(tabs).getByRole("button", { name: "Tasks · 8" })).toHaveAttribute("aria-current", "true");
  });

  it("reads the kind tab from the URL and asks the server for that kind", async () => {
    const fetchMock = stubFetch({
      "/api/search?q=payment&kind=bug": { ...result([bug], [], "payment"), kind: "bug" },
    });

    renderAt("/explore?mode=search&q=payment&kind=bug");

    expect(await screen.findByRole("link", { name: "Admin filter alignment" })).toBeInTheDocument();
    expect(fetchMock.mock.calls.map((c) => String(c[0]))).toContain("/api/search?q=payment&kind=bug");
  });
});

describe("Explore search: a result row", () => {
  it("on a kind tab, leads with the highlighted title, then key, status, and the node's matching lines", async () => {
    const feature = node({
      noun: "feature", id: "F-pueftm", key: "foundation/app-boot", title: "App boot", status: "active",
      ref: "foundation/app-boot (F-pueftm)", match: "title", count: 5,
      hits: [
        { path: ".tiller/epics/foundation/features/app-boot/feature.md", line: 3, snippet: "the app boots against Postgres" },
        { path: ".tiller/epics/foundation/features/app-boot/feature.md", line: 9, snippet: "boot order" },
        { path: ".tiller/epics/foundation/features/app-boot/feature.md", line: 12, snippet: "reboot" },
      ],
    });
    stubFetch({ "/api/search?q=boot&kind=feature": { ...result([feature], [], "boot"), kind: "feature" } });
    const { container } = renderAt("/explore?mode=search&q=boot&kind=feature");

    const title = await screen.findByRole("link", { name: "App boot" });
    expect(title).toHaveAttribute("href", "/node/feature/F-pueftm");
    expect(title.querySelector("mark")?.textContent).toBe("boot");
    const row = title.closest("li") as HTMLElement;
    expect(within(row).getByText("foundation/app-boot")).toBeInTheDocument();
    expect(within(row).getByText("active")).toBeInTheDocument();
    expect(within(row).getByText("feature.md:3")).toBeInTheDocument();
    expect(within(row).getByText("+2 more matches")).toBeInTheDocument();
    expect(container.querySelectorAll(".search-result .hit-snippet mark")[0]?.textContent).toBe("boot");
  });

  it("on All, keeps a row to one matching line and puts its total on the head line", async () => {
    const feature = node({
      noun: "feature", id: "F-1", key: "foundation/app-boot", title: "App boot", match: "key_fragment", count: 5,
      hits: [
        { path: ".tiller/epics/foundation/features/app-boot/feature.md", line: 3, snippet: "boot one" },
        { path: ".tiller/epics/foundation/features/app-boot/feature.md", line: 9, snippet: "boot two" },
      ],
    });
    stubFetch({ "/api/search?q=boot": result([feature], [], "boot") });
    renderAt("/explore?mode=search&q=boot");

    const row = (await screen.findByRole("link", { name: "App boot" })).closest("li") as HTMLElement;
    expect(within(row).getByText("Key contains · 5 matches")).toBeInTheDocument();
    expect(within(row).getByText("feature.md:3")).toBeInTheDocument();
    expect(within(row).queryByText("feature.md:9")).not.toBeInTheDocument();
    expect(within(row).queryByText(/more match/)).not.toBeInTheDocument();
  });

  it("labels a question by title and Q-id and names the ledger field that matched", async () => {
    const question = node({
      noun: "question", id: "Q-h2n7m5", title: "Audit selection unit", status: "open", match: "text", count: 1,
      hits: [{ path: ".tiller/decisions/open-questions.yml", line: 0, snippet: "Which multi-year unit?", field: "statement" }],
    });
    stubFetch({ "/api/search?q=multi-year": result([question], [], "multi-year") });

    renderAt("/explore?mode=search&q=multi-year");

    const title = await screen.findByRole("link", { name: "Audit selection unit" });
    const row = title.closest("li") as HTMLElement;
    expect(within(row).getByText("Q-h2n7m5").className).toContain("mono");
    expect(within(row).getByText("statement")).toBeInTheDocument();
  });

  it("links an unowned document and each of its lines to the file page, segment-encoded", async () => {
    const path = ".tiller/design/loose plans/#1 follow-up.md";
    stubFetch({ "/api/search?q=payment&kind=document": { ...result([], [doc(path, [27, 40])]), kind: "document" } });

    renderAt("/explore?mode=search&q=payment&kind=document");

    const file = await screen.findByRole("link", { name: "design/loose plans/#1 follow-up.md" });
    expect(file).toHaveAttribute("href", "/file/.tiller/design/loose%20plans/%231%20follow-up.md#L27");
    expect(screen.getByRole("link", { name: "line 40" })).toHaveAttribute(
      "href", "/file/.tiller/design/loose%20plans/%231%20follow-up.md#L40",
    );
  });
});

describe("Explore search: totals", () => {
  it("states the result total and says when the page holds fewer", async () => {
    stubFetch({
      "/api/search?q=payment": { ...result([node({ noun: "task", id: "T-1" })]), count: 260, truncated: true },
    });

    renderAt("/explore?mode=search&q=payment");

    expect(await screen.findByText("260 results · showing 1; narrow the query or pick a kind to see the rest")).toBeInTheDocument();
  });

  it("answers an empty result plainly", async () => {
    stubFetch({ "/api/search?q=zz": result([], [], "zz") });

    renderAt("/explore?mode=search&q=zz");

    expect(await screen.findByText("No matches.")).toBeInTheDocument();
  });
});
