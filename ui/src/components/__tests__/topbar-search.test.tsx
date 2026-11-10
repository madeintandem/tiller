// The topbar search field (docs/spec/ui-surface.md, "Topbar"): one input on
// every page, focused by "/", submitting to Explore's search page, where it
// shows the current query. Rendered through <App/> so the route contract and
// the removal of the old navigator and Search-mode inputs are under test.

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SearchNodeResult, SearchResult } from "../../api";
import App from "../../App";
import { HIDE_CANCELLED_STORAGE_KEY } from "../../views/explore/hideCancelled";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
  document.body.innerHTML = "";
});

function stubFetch(routes: Record<string, unknown> = {}) {
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

function Where() {
  const location = useLocation();
  return <output data-testid="where">{location.pathname + location.search}</output>;
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
      <Where />
    </MemoryRouter>,
  );
}

function node(over: Partial<SearchNodeResult> & Pick<SearchNodeResult, "noun" | "id">): SearchNodeResult {
  return { key: undefined, title: "Untitled", status: "ready", ref: over.id, match: "title", count: 0, hits: [], ...over };
}

function result(nodes: SearchNodeResult[], query = "renewal"): SearchResult {
  const kinds = new Map<string, number>();
  for (const n of nodes) kinds.set(n.noun, (kinds.get(n.noun) ?? 0) + 1);
  return {
    query, count: nodes.length, truncated: false,
    kinds: [...kinds].map(([kind, count]) => ({ kind, count })), nodes, documents: [],
  };
}

const field = () => screen.getByRole("textbox", { name: "Search" });
const form = () => field().closest("form") as HTMLFormElement;

describe("topbar search field", () => {
  it.each(["/", "/build", "/fix", "/report", "/history", "/explore", "/explore?mode=search&q=x"])(
    "is the one Search input in the header on %s",
    (path) => {
      stubFetch();
      const { container } = renderAt(path);
      const header = container.querySelector("header.site-header") as HTMLElement;
      expect(within(header).getAllByRole("textbox", { name: "Search" })).toHaveLength(1);
      expect(screen.getAllByRole("textbox", { name: "Search" })).toHaveLength(1);
    },
  );

  it("shows a visible hint of the / shortcut", () => {
    stubFetch();
    const { container } = renderAt("/");
    expect(container.querySelector(".topbar-search kbd")).toHaveTextContent("/");
  });

  it("is focused by / from Home", () => {
    stubFetch();
    renderAt("/");
    fireEvent.keyDown(document.body, { key: "/" });
    expect(field()).toHaveFocus();
  });

  it("is not focused by / typed into an input, select, textarea, or contenteditable", () => {
    stubFetch();
    renderAt("/");
    const input = document.body.appendChild(document.createElement("input"));
    const select = document.body.appendChild(document.createElement("select"));
    const area = document.body.appendChild(document.createElement("textarea"));
    const editable = document.body.appendChild(document.createElement("div"));
    editable.setAttribute("contenteditable", "true");
    const inner = editable.appendChild(document.createElement("span"));
    for (const target of [input, select, area, editable, inner]) {
      fireEvent.keyDown(target, { key: "/" });
      expect(field()).not.toHaveFocus();
    }
  });

  it("is not focused by / inside Explore's filter combobox", async () => {
    stubFetch();
    renderAt("/explore?mode=filter");
    const combo = (await screen.findAllByRole("combobox"))[0];
    combo.focus();
    fireEvent.keyDown(combo, { key: "/" });
    expect(field()).not.toHaveFocus();
  });

  it("is not focused by / with a modifier held", () => {
    stubFetch();
    renderAt("/");
    fireEvent.keyDown(document.body, { key: "/", metaKey: true });
    fireEvent.keyDown(document.body, { key: "/", ctrlKey: true });
    fireEvent.keyDown(document.body, { key: "/", altKey: true });
    expect(field()).not.toHaveFocus();
  });

  it("swallows the / that focuses it, so the slash is not typed", () => {
    stubFetch();
    renderAt("/");
    const event = new KeyboardEvent("keydown", { key: "/", bubbles: true, cancelable: true });
    document.body.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
  });

  it("blurs on Escape", () => {
    stubFetch();
    renderAt("/");
    field().focus();
    fireEvent.keyDown(field(), { key: "Escape" });
    expect(field()).not.toHaveFocus();
  });

  it("submits a trimmed query to /explore?mode=search&q=renewal and shows it there", async () => {
    stubFetch({ "/api/search?q=renewal": result([node({ noun: "epic", id: "E-1", title: "Renewal flow" })]) });
    renderAt("/");
    fireEvent.change(field(), { target: { value: "  renewal  " } });
    fireEvent.submit(form());
    expect(await screen.findByRole("link", { name: "Renewal flow" })).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent("/explore?mode=search&q=renewal");
    expect(field()).toHaveValue("renewal");
  });

  it("fills from a direct load of the search URL", () => {
    stubFetch();
    renderAt("/explore?mode=search&q=x");
    expect(field()).toHaveValue("x");
  });

  it("follows the URL when the query changes under it", async () => {
    stubFetch({ "/api/search?q=y": result([], "y") });
    renderAt("/explore?mode=search&q=x");
    fireEvent.change(field(), { target: { value: "y" } });
    fireEvent.submit(form());
    expect(await screen.findByText("No matches.")).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent("q=y");
    expect(field()).toHaveValue("y");
  });

  it("shows the search page's empty prompt, not an error, when cleared and submitted", async () => {
    stubFetch();
    renderAt("/explore?mode=search&q=x");
    fireEvent.change(field(), { target: { value: "   " } });
    fireEvent.submit(form());
    expect(await screen.findByText("Type a keyword, id, or key and press Enter.")).toBeInTheDocument();
    expect(screen.queryByText(/failed/)).not.toBeInTheDocument();
  });

  it("leaves Search mode and Browse's navigator without a second input", async () => {
    stubFetch();
    const search = renderAt("/explore?mode=search&q=x");
    expect(screen.getAllByRole("textbox")).toHaveLength(1);
    search.unmount();
    renderAt("/explore");
    const nav = await screen.findByRole("navigation", { name: "Definition navigator" });
    expect(within(nav).queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.getAllByRole("textbox", { name: "Search" })).toHaveLength(1);
  });
});

describe("search results with Hide cancelled", () => {
  const rows = [
    node({ noun: "epic", id: "E-1", title: "Renewal flow", status: "active" }),
    node({ noun: "epic", id: "E-2", title: "Renewal old", status: "cancelled" }),
    node({ noun: "task", id: "T-1", title: "Renewal retired task", status: "cancelled" }),
  ];

  it("hides cancelled rows and says how many", async () => {
    window.localStorage.setItem(HIDE_CANCELLED_STORAGE_KEY, "true");
    stubFetch({ "/api/search?q=renewal": result(rows) });
    renderAt("/explore?mode=search&q=renewal");
    expect(await screen.findByRole("link", { name: "Renewal flow" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Renewal old" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Renewal retired task" })).not.toBeInTheDocument();
    expect(screen.getByText(/2 cancelled results hidden/)).toBeInTheDocument();
    expect(screen.getByText(/^1 result/)).toBeInTheDocument();
  });

  it("shows everything, and no note, with it off", async () => {
    stubFetch({ "/api/search?q=renewal": result(rows) });
    renderAt("/explore?mode=search&q=renewal");
    expect(await screen.findByRole("link", { name: "Renewal old" })).toBeInTheDocument();
    expect(screen.queryByText(/cancelled results? hidden/)).not.toBeInTheDocument();
  });

  it("says so when every result is hidden", async () => {
    window.localStorage.setItem(HIDE_CANCELLED_STORAGE_KEY, "true");
    stubFetch({ "/api/search?q=renewal": result([rows[1]]) });
    renderAt("/explore?mode=search&q=renewal");
    expect(await screen.findByText(/1 cancelled result hidden/)).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Renewal old" })).not.toBeInTheDocument();
  });
});
