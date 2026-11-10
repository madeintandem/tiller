// Explore while the page rereads: the navigator, the empty pane, the node
// page (both steps of its two-step read), Filter, Search, Map, and the two
// ledgers keep what they show until the new answer lands, then update in
// place. The node page must not show "Reading branch history..." over rows
// it already had, nor record the visit a second time.

import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BuildBoard, ListResult, ListRow, MapView, NodePage, SearchResult, TreeModel, TreeNode } from "../../api";
import * as recent from "../../lib/recentNodes";
import { expectNoLoading, renderApp, stubReread, summary, watchLoading, type Routes } from "./rereadHarness";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function list(noun: string, rows: ListRow[] = []): ListResult {
  return { noun, count: rows.length, nodes: rows };
}

function treeNode(noun: string, overrides: Partial<TreeNode>): TreeNode {
  return { noun, id: "x", status: "ready", path: "", files: [], children: [], ...overrides };
}

function tree(epicTitle: string): TreeModel {
  return { project: "acme", nodes: [treeNode("epic", { id: "E-1", key: "foundation", title: epicTitle })] };
}

function board(features: number): BuildBoard {
  return {
    epics: [{
      noun: "epic", id: "E-1", key: "foundation", title: "Foundation", ref: "foundation (E-1)", status: "active",
      contributors: [], feature_counts: { active: features },
      features: Array.from({ length: features }, (_, i) => ({
        id: `F-${i}`, title: `Feature ${i}`, status: "active", column: "active", ref: `F-${i}`,
        open_work_items: 0, task_done: 0, task_total: 0, tasks: [], contributors: [],
      })),
    }],
  };
}

function exploreRoutes(): Routes {
  return {
    "/api/status": summary(),
    "/api/tree": tree("Foundation"),
    "/api/ls?noun=question": list("question"),
    "/api/ls?noun=bug": list("bug"),
    "/api/ls?noun=chore": list("chore"),
    "/api/ls?noun=tweak": list("tweak"),
    "/api/queue": { entries: [] },
    "/api/build": board(1),
  };
}

const integrationEvent = { at: "2026-10-06T11:00:00Z", type: "add.task", contributor: "", command: "tiller add task boot-the-app" };
const branchEvent = { at: "2026-10-06T12:00:00Z", type: "start.task", contributor: "", command: "tiller start task T-h4k2wq" };
const laterEvent = { at: "2026-10-06T13:00:00Z", type: "submit.task", contributor: "", command: "tiller submit task T-h4k2wq" };

function taskPage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: { noun: "task", id: "T-h4k2wq", key: "foundation/skeleton/boot", title: "Boot the app", status: "active" },
    ref: "foundation/skeleton/boot (T-h4k2wq)",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

const FIRST = "/api/node/task/T-h4k2wq?history=integration";
const MERGED = "/api/node/task/T-h4k2wq";

describe("Explore while the page rereads", () => {
  it("keeps the navigator and the empty pane, then updates both in place", async () => {
    const stub = stubReread(exploreRoutes());
    renderApp("/explore");
    const nav = within(await screen.findByRole("navigation", { name: "Definition navigator" }));
    expect(await nav.findByRole("link", { name: "Foundation" })).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "1 feature in flight" })).toBeInTheDocument();

    const watch = watchLoading();
    stub.reread();
    expect(nav.getByRole("link", { name: "Foundation" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "1 feature in flight" })).toBeInTheDocument();

    await stub.release({ "/api/tree": tree("Foundation Two"), "/api/build": board(2) });
    expect(await nav.findByRole("link", { name: "Foundation Two" })).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "2 features in flight" })).toBeInTheDocument();
    expectNoLoading(watch);
  });

  it("keeps the node page and its branch history through both reads, and records the visit once", async () => {
    const record = vi.spyOn(recent, "recordRecent");
    const stub = stubReread({
      ...exploreRoutes(),
      [FIRST]: taskPage({ history: [integrationEvent], history_pending: true }),
      [MERGED]: taskPage({ history: [branchEvent, integrationEvent] }),
    });
    renderApp("/node/task/T-h4k2wq?tab=history");
    expect(await screen.findByTitle("tiller start task T-h4k2wq")).toBeInTheDocument();
    expect(record).toHaveBeenCalledTimes(1);

    const watch = watchLoading();
    stub.reread();
    expect(stub.heldUrls()).toEqual(expect.arrayContaining([FIRST, MERGED]));
    expect(screen.getByTitle("tiller start task T-h4k2wq")).toBeInTheDocument();
    expect(screen.getByTitle("tiller add task boot-the-app")).toBeInTheDocument();

    await stub.release({ [MERGED]: taskPage({ history: [laterEvent, branchEvent, integrationEvent] }) });
    expect(await screen.findByTitle("tiller submit task T-h4k2wq")).toBeInTheDocument();
    expect(screen.getByTitle("tiller start task T-h4k2wq")).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent("/node/task/T-h4k2wq?tab=history");
    expect(record).toHaveBeenCalledTimes(1);
    expectNoLoading(watch);
  });

  it("keeps the node page and says it is out of date when the reread fails", async () => {
    const stub = stubReread({ ...exploreRoutes(), [FIRST]: taskPage() });
    renderApp("/node/task/T-h4k2wq");
    expect(await screen.findByRole("heading", { level: 1, name: /Boot the app/ })).toBeInTheDocument();

    stub.reread();
    await stub.fail();

    expect(screen.getByRole("heading", { level: 1, name: /Boot the app/ })).toBeInTheDocument();
    expect(screen.getByText(/Out of date: GET \/api\/node\/task\/T-h4k2wq\?history=integration failed/)).toBeInTheDocument();
  });

  it("keeps Filter's list, then updates it in place", async () => {
    const bug = (title: string): ListRow => ({ id: "B-1", key: "bugs/one", title, status: "ready", ref: "bugs/one (B-1)" });
    const stub = stubReread({ ...exploreRoutes(), "/api/ls?noun=bug": list("bug", [bug("Blank on reload")]) });
    renderApp("/explore?mode=filter&noun=bug");
    expect(await screen.findByText("Blank on reload")).toBeInTheDocument();

    const watch = watchLoading();
    stub.reread();
    expect(screen.getByText("Blank on reload")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Noun" })).toBeInTheDocument();

    await stub.release({ "/api/ls?noun=bug": list("bug", [bug("Blank on every reload")]) });
    expect(await screen.findByText("Blank on every reload")).toBeInTheDocument();
    expectNoLoading(watch);
  });

  it("keeps Search's results, then updates them in place", async () => {
    const answer = (title: string): SearchResult => ({
      query: "payment", count: 1, truncated: false, kinds: [{ kind: "bug", count: 1 }],
      nodes: [{ noun: "bug", id: "B-1", key: "bugs/one", title, status: "ready", ref: "bugs/one (B-1)", match: "text", count: 1, hits: [] }],
      documents: [],
    });
    const stub = stubReread({ ...exploreRoutes(), "/api/search?q=payment": answer("Payment retries") });
    renderApp("/explore?mode=search&q=payment");
    expect(await screen.findByRole("link", { name: "Payment retries" })).toBeInTheDocument();

    const watch = watchLoading();
    stub.reread();
    expect(screen.getByRole("link", { name: "Payment retries" })).toBeInTheDocument();

    await stub.release({ "/api/search?q=payment": answer("Payment retries, capped") });
    expect(await screen.findByRole("link", { name: "Payment retries, capped" })).toBeInTheDocument();
    expectNoLoading(watch);
  });

  it("keeps the Map, then updates it in place", async () => {
    const view = (title: string): MapView => ({
      nodes: [{ noun: "epic", id: "E-1", key: "foundation", title, status: "ready" }],
      edges: [],
      relationships: { vertices: [], edges: [], catalog: [], diagnostics: [], complete: true },
    });
    const stub = stubReread({ ...exploreRoutes(), "/api/map": view("Foundation map") });
    renderApp("/explore?mode=map");
    expect((await screen.findAllByText("Foundation map")).length).toBeGreaterThan(0);

    const watch = watchLoading();
    stub.reread();
    expect(screen.getAllByText("Foundation map").length).toBeGreaterThan(0);

    await stub.release({ "/api/map": view("Foundation map, renamed") });
    expect((await screen.findAllByText("Foundation map, renamed")).length).toBeGreaterThan(0);
    expectNoLoading(watch);
  });

  it("keeps the Questions ledger, then updates it in place", async () => {
    const question = (title: string): ListRow => ({ id: "Q-1", ref: "Q-1", title, status: "open" });
    const stub = stubReread({ ...exploreRoutes(), "/api/ls?noun=question": list("question", [question("Which currency?")]) });
    renderApp("/questions");
    const pane = () => screen.getByRole("heading", { level: 1, name: "Questions" }).closest(".explore-detail") as HTMLElement;
    expect(await screen.findByRole("heading", { level: 1, name: "Questions" })).toBeInTheDocument();
    expect(await within(pane()).findByText("Which currency?")).toBeInTheDocument();

    const watch = watchLoading();
    stub.reread();
    expect(within(pane()).getByText("Which currency?")).toBeInTheDocument();

    await stub.release({ "/api/ls?noun=question": list("question", [question("Which currencies?")]) });
    expect(await within(pane()).findByText("Which currencies?")).toBeInTheDocument();
    expectNoLoading(watch);
  });

  it("keeps the Questions ledger and says it is out of date when the reread fails", async () => {
    const stub = stubReread({ ...exploreRoutes(), "/api/ls?noun=question": list("question", [{ id: "Q-1", ref: "Q-1", title: "Which currency?", status: "open" }]) });
    renderApp("/questions");
    expect(await screen.findByRole("heading", { level: 1, name: "Questions" })).toBeInTheDocument();

    stub.reread();
    await stub.fail();

    expect(screen.getByRole("heading", { level: 1, name: "Questions" })).toBeInTheDocument();
    expect(screen.getByText(/Out of date: GET \/api\/ls\?noun=question failed/)).toBeInTheDocument();
  });

  it("keeps the Amendments ledger, then updates it in place", async () => {
    const amendment = (title: string): ListRow => ({ id: "A-1", ref: "A-1", title, status: "proposed" });
    const stub = stubReread({ ...exploreRoutes(), "/api/ls?noun=amendments": list("amendment", [amendment("Split billing")]) });
    renderApp("/amendments");
    expect(await screen.findByText("Split billing")).toBeInTheDocument();

    const watch = watchLoading();
    stub.reread();
    expect(screen.getByText("Split billing")).toBeInTheDocument();

    await stub.release({ "/api/ls?noun=amendments": list("amendment", [amendment("Split billing in two")]) });
    expect(await screen.findByText("Split billing in two")).toBeInTheDocument();
    expectNoLoading(watch);
  });
});
