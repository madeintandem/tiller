// Explore's empty reading pane (task 32): recently opened nodes plus the two
// useful links, and the recorder that feeds them. Rendered through <App/> for
// the same reason explore.test.tsx is: the layout route is the contract.

import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useNavigate } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { NodePage, ProjectSummary, Queue, QueueEntry, Streams } from "../../api";
import { bumpFetchGeneration } from "../../api";
import App from "../../App";
import { readRecent, recentKey } from "../../lib/recentNodes";

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

const emptyList = (noun: string) => ({ noun, count: 0, nodes: [] });

function stubFetch(routes: Record<string, unknown>) {
  const mock = vi.fn((input: unknown) => {
    const url = String(input);
    if (url in routes) {
      const body = routes[url];
      if (body instanceof Promise) return body;
      return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
    }
    return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

function baseRoutes(extra: Record<string, unknown> = {}) {
  return {
    "/api/status": summary,
    "/api/tree": { project: "acme", nodes: [] },
    "/api/ls?noun=question": emptyList("question"),
    "/api/ls?noun=bug": emptyList("bug"),
    "/api/ls?noun=chore": emptyList("chore"),
    "/api/ls?noun=tweak": emptyList("tweak"),
    ...extra,
  };
}

const entry = (n: number): QueueEntry => ({
  kind: "approve_epic", noun: "epic", id: `E-${n}`, ref: `E-${n}`, command: "tiller approve",
});
const queue = (n: number): Queue => ({ entries: Array.from({ length: n }, (_, i) => entry(i)) });

function streams(active: number, review: number, blocked: number, unclaimed = 0): Streams {
  const item = (i: number) => ({ noun: "task", id: `T-${i}`, ref: `T-${i}`, status: "active", display: "active" });
  return {
    contributors: [{
      contributor: { id: "jc", name: "JC Grubbs" },
      active: Array.from({ length: active }, (_, i) => item(i)),
      blocked: Array.from({ length: blocked }, (_, i) => ({ ...item(100 + i), blocked_by: [] })),
      gates_in_review: Array.from({ length: review }, (_, i) => ({ ...item(200 + i), gates: [] })),
      open_questions: [],
    }],
    unclaimed: Array.from({ length: unclaimed }, (_, i) => ({
      noun: "task", id: `T-u${i}`, ref: `T-u${i}`, status: "ready", feature_id: "F-1",
    })),
  } as unknown as Streams;
}

function renderAt(path = "/explore") {
  return render(<MemoryRouter initialEntries={[path]}><App /></MemoryRouter>);
}

const SENTENCE = "Select a node from the navigator to see its detail.";

function storeRecent(list: unknown) {
  window.localStorage.setItem(recentKey("acme"), JSON.stringify(list));
}

describe("Explore empty reading pane", () => {
  it("lists recently opened nodes title first, key in mono after, noun as text", async () => {
    storeRecent([
      { noun: "feature", id: "F-1", key: "pay/checkout", title: "Checkout flow" },
      { noun: "question", id: "Q-9", title: "Which rounding mode" },
    ]);
    stubFetch(baseRoutes({ "/api/queue": queue(2), "/api/board": streams(2, 1, 1) }));
    renderAt();
    const region = await screen.findByRole("region", { name: "Recently opened" });
    const links = within(region).getAllByRole("link");
    expect(links.map((l) => l.textContent)).toEqual(["Checkout flow", "Which rounding mode"]);
    expect(links[0]).toHaveAttribute("href", "/node/feature/F-1");
    const row = links[0].closest("li") as HTMLElement;
    const key = within(row).getByText("pay/checkout");
    expect(key.className).toContain("mono");
    expect(links[0].compareDocumentPosition(key) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(row).getByText("Feature")).toBeInTheDocument();
    const second = links[1].closest("li") as HTMLElement;
    expect(within(second).getByText("Q-9").className).toContain("mono");
    expect(screen.queryByText(SENTENCE)).not.toBeInTheDocument();
  });

  it("links to Home with the decision count and to Build with the in-flight count", async () => {
    stubFetch(baseRoutes({ "/api/queue": queue(3), "/api/board": streams(2, 1, 4, 5) }));
    renderAt();
    const needs = await screen.findByRole("link", { name: "3 decisions need you" });
    expect(needs).toHaveAttribute("href", "/");
    // In flight is the active and in-review work the Home column shows, not
    // the blocked register and not the unclaimed pool.
    const flight = await screen.findByRole("link", { name: "3 in flight" });
    expect(flight).toHaveAttribute("href", "/build");
  });

  it("with no history shows the two links and the old sentence", async () => {
    stubFetch(baseRoutes({ "/api/queue": queue(1), "/api/board": streams(1, 0, 0) }));
    renderAt();
    expect(await screen.findByText(SENTENCE)).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Recently opened" })).not.toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "1 decision needs you" })).toHaveAttribute("href", "/");
    expect(await screen.findByRole("link", { name: "1 in flight" })).toHaveAttribute("href", "/build");
  });

  it("says nothing needs you and nothing is in flight at zero, still as links", async () => {
    stubFetch(baseRoutes({ "/api/queue": queue(0), "/api/board": streams(0, 0, 2) }));
    renderAt();
    expect(await screen.findByRole("link", { name: "Nothing needs you right now" })).toHaveAttribute("href", "/");
    expect(await screen.findByRole("link", { name: "Nothing in flight" })).toHaveAttribute("href", "/build");
  });

  it("renders the links without counts, and no spinner, while loading or after a failed read", async () => {
    stubFetch(baseRoutes({ "/api/queue": new Promise(() => {}), "/api/board": new Promise(() => {}) }));
    renderAt();
    const needs = await screen.findByRole("link", { name: "Decisions that need you" });
    expect(needs).toHaveAttribute("href", "/");
    expect(screen.getByRole("link", { name: "Work in flight" })).toHaveAttribute("href", "/build");
    expect(screen.queryByText(/Loading/)).not.toBeInTheDocument();
  });

  it("falls back to the count-free links when the reads fail", async () => {
    stubFetch(baseRoutes());
    renderAt();
    expect(await screen.findByRole("link", { name: "Decisions that need you" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Work in flight" })).toBeInTheDocument();
  });

  it.each([
    ["corrupt JSON", "{nope"],
    ["a wrong shape", '{"a":1}'],
  ])("treats %s in storage as no history", async (_n, raw) => {
    window.localStorage.setItem(recentKey("acme"), raw);
    stubFetch(baseRoutes({ "/api/queue": queue(0), "/api/board": streams(0, 0, 0) }));
    renderAt();
    expect(await screen.findByText(SENTENCE)).toBeInTheDocument();
  });

  it("does not show another project's history", async () => {
    window.localStorage.setItem(
      recentKey("other"), JSON.stringify([{ noun: "epic", id: "E-1", title: "Elsewhere" }]),
    );
    stubFetch(baseRoutes({ "/api/queue": queue(0), "/api/board": streams(0, 0, 0) }));
    renderAt();
    expect(await screen.findByText(SENTENCE)).toBeInTheDocument();
    expect(screen.queryByText("Elsewhere")).not.toBeInTheDocument();
  });
});

function page(noun: string, id: string, title: string, key?: string): NodePage {
  return {
    node: { noun, id, key, title, status: "ready" },
    ref: key ?? id,
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
  } as unknown as NodePage;
}

describe("recording recently opened nodes", () => {
  it("records a node page that loaded, with its title as served, once", async () => {
    stubFetch(baseRoutes({
      "/api/node/epic/E-u2w9wv?history=integration": page("epic", "E-u2w9wv", "Foundation", "foundation"),
      "/api/queue": queue(0),
    }));
    renderAt("/node/epic/E-u2w9wv");
    await screen.findByRole("heading", { level: 1, name: "Foundation" });
    await waitFor(() => expect(readRecent("acme")).toEqual([
      { noun: "epic", id: "E-u2w9wv", title: "Foundation", key: "foundation" },
    ]));
  });

  it("records nothing for a node that failed to load", async () => {
    stubFetch(baseRoutes({ "/api/queue": queue(0) }));
    renderAt("/node/epic/E-gone");
    await screen.findByRole("heading", { name: "No such node" });
    expect(readRecent("acme")).toEqual([]);
  });

  it("records once per visit: a fetch-generation bump (reread) does not record again", async () => {
    stubFetch(baseRoutes({
      "/api/node/epic/E-u2w9wv?history=integration": page("epic", "E-u2w9wv", "Foundation", "foundation"),
      "/api/queue": queue(0),
    }));
    renderAt("/node/epic/E-u2w9wv");
    await screen.findByRole("heading", { level: 1, name: "Foundation" });
    await waitFor(() => expect(readRecent("acme")).toHaveLength(1));
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    act(() => bumpFetchGeneration());
    await screen.findByRole("heading", { level: 1, name: "Foundation" });
    await waitFor(() => expect(screen.queryByText("Loading node...")).not.toBeInTheDocument());
    expect(setItem.mock.calls.filter(([key]) => key === recentKey("acme"))).toHaveLength(0);
  });

  it("records a different node when navigating to it", async () => {
    stubFetch(baseRoutes({
      "/api/node/epic/E-1?history=integration": page("epic", "E-1", "First", "first"),
      "/api/node/epic/E-2?history=integration": page("epic", "E-2", "Second", "second"),
      "/api/queue": queue(0),
    }));
    render(<MemoryRouter initialEntries={["/node/epic/E-1"]}><Go /><App /></MemoryRouter>);
    await screen.findByRole("heading", { level: 1, name: "First" });
    await waitFor(() => expect(readRecent("acme")).toHaveLength(1));
    fireEvent.click(screen.getByRole("button", { name: "go-second" }));
    await screen.findByRole("heading", { level: 1, name: "Second" });
    await waitFor(() => expect(readRecent("acme").map((r) => r.id)).toEqual(["E-2", "E-1"]));
  });

  it("records exactly once, under the right key, when the project key arrives late", async () => {
    let release: (value: unknown) => void = () => {};
    const status = new Promise((resolve) => { release = resolve; });
    stubFetch(baseRoutes({
      "/api/status": status,
      "/api/node/epic/E-u2w9wv?history=integration": page("epic", "E-u2w9wv", "Foundation", "foundation"),
      "/api/queue": queue(0),
    }));
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    renderAt("/node/epic/E-u2w9wv");
    await new Promise((r) => setTimeout(r, 50));
    expect(setItem.mock.calls.filter(([key]) => String(key).startsWith("tiller-explore-recent"))).toHaveLength(0);
    await act(async () => { release({ ok: true, json: () => Promise.resolve(summary) }); });
    await screen.findByRole("heading", { level: 1, name: "Foundation" });
    await waitFor(() => expect(readRecent("acme")).toHaveLength(1));
    expect(setItem.mock.calls.filter(([key]) => String(key).startsWith("tiller-explore-recent"))
      .map(([key]) => key)).toEqual([recentKey("acme")]);
  });
});

function Go() {
  const navigate = useNavigate();
  return <button type="button" onClick={() => navigate("/node/epic/E-2")}>go-second</button>;
}
