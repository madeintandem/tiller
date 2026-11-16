// Arriving at Home, Report, or Explore again while the page is already
// showing: the topbar link for the open page, or the browser's back and
// forward between two addresses of the same page. The page stays mounted, so
// the arrival itself has to reread the page's own reads, once each, and keep
// what is on screen until the answers land. The page's own URL changes (a
// tab, a range, a filter, a search kind, a map selection) are not arrivals.
// Build and Fix pin the same guarantees in reread-arrival.test.tsx.

import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BuildBoard, DriftReportView, ListResult, ListRow, MapView, NodePage, Report, SearchResult, TreeModel, UsageReport } from "../../api";
import App from "../../App";
import { emptyList, expectNoLoading, homeRoutes, settleReads, stubReread, summary, watchLoading, type Routes } from "./rereadHarness";

afterEach(async () => {
  await settleReads();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
  Reflect.deleteProperty(document, "visibilityState");
});

function Probe() {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <>
      <p data-testid="where">{`${location.pathname}${location.search}${location.hash}`}</p>
      <button onClick={() => navigate(-1)}>history back</button>
      <button onClick={() => navigate(1)}>history forward</button>
    </>
  );
}

// The console at the last of the given addresses, with the earlier ones
// behind it in history.
function renderConsole(...entries: string[]) {
  return render(
    <MemoryRouter initialEntries={entries}>
      <App />
      <Probe />
    </MemoryRouter>,
  );
}

const where = () => screen.getByTestId("where").textContent;

function topbarLink(name: string): HTMLElement {
  const nav = document.querySelector(".site-nav");
  if (nav === null) throw new Error("no primary navigation");
  return within(nav as HTMLElement).getByRole("link", { name });
}

type Stub = ReturnType<typeof stubReread>;
const reads = (stub: Stub, url: string) => stub.calls.filter((call) => call.url === url).length;
const counts = (stub: Stub, urls: string[]) => Object.fromEntries(urls.map((url) => [url, reads(stub, url)]));

// The reads issued since a count was taken, one entry per request.
const since = (stub: Stub, before: number) => stub.calls.slice(before).map((call) => call.url).sort();

// Lets any read a click might have started reach the stub before a count.
const settle = () => act(async () => { await Promise.resolve(); });

const back = () => fireEvent.click(screen.getByRole("button", { name: "history back" }));
const forward = () => fireEvent.click(screen.getByRole("button", { name: "history forward" }));

// One manual refresh, then one return to the window 31 seconds later: each
// rereads every url once and no more.
async function expectOneReadPerRefreshAndReturn(stub: Stub, urls: string[]) {
  const before = counts(stub, urls);
  fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
  await settle();
  for (const url of urls) expect(reads(stub, url) - before[url], `${url} on refresh`).toBe(1);

  await waitFor(() => expect(screen.getByRole("button", { name: "Refresh now" })).toBeEnabled());
  const later = Date.now() + 31_000;
  vi.spyOn(Date, "now").mockReturnValue(later);
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "visible" });
  act(() => {
    document.dispatchEvent(new Event("visibilitychange"));
    window.dispatchEvent(new Event("focus"));
  });
  await settle();
  for (const url of urls) expect(reads(stub, url) - before[url], `${url} on return`).toBe(2);
}

// ---------------------------------------------------------------- Home

// Every read the desk shows, the shell's project status among them: Home
// draws its header from that read.
const HOME_READS = Object.keys(homeRoutes()).filter((url) => url !== "/api/version");

function epics(status: string): ListResult {
  return { noun: "epic", count: 1, nodes: [{ id: "E-1", ref: "form (E-1)", title: "Form Engine", status }] };
}

const drift: DriftReportView = {
  compared: 5,
  candidates: { missing_surface: [{ marker: "missing_surface", noun: "task", id: "T-1", title: "Legacy calc", detail: "gone" }] },
  uncovered: [],
};

function deskRoutes(status: string): Routes {
  return { ...homeRoutes(), "/api/ls?noun=epic": epics(status), "/api/drift": drift };
}

async function openHome(stub: Stub, ...entries: string[]) {
  renderConsole(...(entries.length > 0 ? entries : ["/"]));
  expect(await screen.findByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();
  await waitFor(() => { for (const url of HOME_READS) expect(reads(stub, url), url).toBeGreaterThan(0); });
  await settle();
}

describe("arriving at Home while it is showing", () => {
  it("reads each of its resources once when it opens", async () => {
    const stub = stubReread(deskRoutes("active"));
    await openHome(stub);

    for (const url of HOME_READS) expect(reads(stub, url), url).toBe(1);
  });

  it("rereads the desk once on the topbar link and shows the changed status", async () => {
    const stub = stubReread(deskRoutes("active"));
    await openHome(stub);
    expect(await screen.findByRole("link", { name: /^1 epic,/ })).toHaveTextContent(/active/i);
    const before = counts(stub, HOME_READS);

    const watch = watchLoading();
    stub.hold();
    fireEvent.click(topbarLink("Home"));
    await settle();

    expect(where()).toBe("/");
    for (const url of HOME_READS) expect(reads(stub, url) - before[url], url).toBe(1);
    // The desk on screen stays until the answers land.
    expect(screen.getByRole("link", { name: /^1 epic,/ })).toHaveTextContent(/active/i);

    await stub.release({ "/api/ls?noun=epic": epics("done") });
    await waitFor(() => expect(screen.getByRole("link", { name: /^1 epic,/ })).toHaveTextContent(/done/i));
    expect(screen.getByRole("link", { name: /^1 epic,/ })).not.toHaveTextContent(/active/i);
    expectNoLoading(watch);
  });

  it("rereads once on browser back and once on forward", async () => {
    const stub = stubReread(deskRoutes("active"));
    await openHome(stub, "/", "/?needs=review");
    const before = counts(stub, HOME_READS);

    back();
    await waitFor(() => expect(where()).toBe("/"));
    await settle();
    for (const url of HOME_READS) expect(reads(stub, url) - before[url], url).toBe(1);

    forward();
    await waitFor(() => expect(where()).toBe("/?needs=review"));
    await settle();
    for (const url of HOME_READS) expect(reads(stub, url) - before[url], url).toBe(2);
  });

  it("reads nothing when a tab changes or the drift tile opens its tab", async () => {
    const stub = stubReread(deskRoutes("active"));
    await openHome(stub);
    const before = stub.calls.length;

    fireEvent.click(screen.getByRole("link", { name: /open the drift tab/ }));
    await waitFor(() => expect(where()).toBe("/?tab=drift"));
    fireEvent.click(screen.getByRole("tab", { name: /Branches/ }));
    await waitFor(() => expect(where()).toBe("/?tab=branches"));
    await settle();
    expect(since(stub, before)).toEqual([]);

    // Overview draws the History signal again, which reads its own count as
    // it mounts; none of the desk's reads is issued again.
    fireEvent.click(screen.getByRole("tab", { name: /Overview/ }));
    await waitFor(() => expect(where()).toBe("/"));
    await settle();
    expect(since(stub, before)).toEqual(["/api/recap/peek"]);
  });

  it("still rereads once per manual refresh and once per return to the window after an arrival", async () => {
    const stub = stubReread(deskRoutes("active"));
    await openHome(stub);
    fireEvent.click(topbarLink("Home"));
    await settle();

    await expectOneReadPerRefreshAndReturn(stub, HOME_READS);
  });

  it("keeps the desk and says it is out of date when the arrival's reread fails", async () => {
    const stub = stubReread(deskRoutes("active"));
    await openHome(stub);

    stub.hold();
    fireEvent.click(topbarLink("Home"));
    await waitFor(() => expect(stub.heldUrls()).toContain("/api/ls?noun=epic"));
    await stub.fail();

    expect(screen.getByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^1 epic,/ })).toHaveTextContent(/active/i);
    expect(screen.getAllByText(/out of date/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/Out of date: GET \/api\/status failed/)).toBeInTheDocument();
  });
});

// -------------------------------------------------------------- Report

const WEEKS = ["2026-09-28", "2026-10-05", "2026-10-12", "2026-10-19"];
const WEEK_BOUNDARIES = [...WEEKS, "2026-10-26"].map((day) => `${day}T00:00:00Z`);

function report(openBugs: number): Report {
  return {
    project: "Acme Platform",
    as_of: "2026-10-20T15:00:00Z",
    agent_use: true,
    weeks: WEEKS,
    week_boundaries: WEEK_BOUNDARIES,
    kpis: {
      features_done: 0, features_total: 0, shipped_last_7_days: 0, median_cycle_days: null, median_lead_days: null,
      completed_features: 0, open_bugs: openBugs, open_questions: 0, answered_questions: 0, open_questions_by_week: WEEKS.map(() => 0),
    },
    burn_up: WEEKS.map(() => ({ scope: 0, done: 0 })),
    throughput: WEEKS.map(() => ({ feature_work: 0, fixes: 0, upkeep: 0 })),
    bugs: WEEKS.map(() => ({ reported: 0, fixed: 0, open: 0 })),
    feature_times: [],
    epics: [],
  };
}

function usage(label: string): UsageReport {
  return {
    weeks: WEEKS,
    week_boundaries: WEEK_BOUNDARIES,
    harnesses: [{ harness: "claude-code", label, since: "2026-09-30" }],
    full_coverage: "2026-09-30",
    coverage: ["none", "full", "full", "full"],
    kpis: { total: 1000, cache_read: 500, input: 300, cache_write: 100, output: 100, median_per_item: null, items: 0, tied_share: 0 },
    weekly: WEEKS.map(() => ({ input: 0, cache_write: 0, cache_read: 0, output: 0 })),
    stages: [], models: [], model_stages: [], features: [], features_left_out: 0,
    item_bands: [], items_left_out: 0, items_unmeasured: 0,
  };
}

function reportRoutes(openBugs: number): Routes {
  return {
    "/api/status": summary(),
    "/api/version": { version: "0.0.0-test" },
    "/api/report": report(openBugs),
    "/api/report/usage": usage("Claude Code"),
  };
}

const REPORT_READS = ["/api/report", "/api/report/usage"];

const openBugsTile = () => screen.getByText("Open bugs").closest(".report-tile, .tile, div") as HTMLElement;

async function openReport(stub: Stub, ...entries: string[]) {
  renderConsole(...entries);
  expect(await screen.findByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();
  await settle();
  expect(reads(stub, "/api/report")).toBe(1);
}

describe("arriving at Report while it is showing", () => {
  it("reads each tab's resource once when it opens", async () => {
    const stub = stubReread(reportRoutes(3));
    await openReport(stub, "/report?tab=agent-use");
    expect(await screen.findByText(/Measured: Claude Code since/)).toBeInTheDocument();
    await settle();

    for (const url of REPORT_READS) expect(reads(stub, url), url).toBe(1);
  });

  it("rereads the Project figures once on the topbar link and shows the changed count", async () => {
    const stub = stubReread(reportRoutes(3));
    await openReport(stub, "/report?weeks=26");
    expect(within(openBugsTile()).getByText("3")).toBeInTheDocument();
    const before = stub.calls.length;

    const watch = watchLoading();
    stub.hold();
    fireEvent.click(topbarLink("Report"));
    await waitFor(() => expect(where()).toBe("/report"));
    await settle();

    expect(since(stub, before)).toEqual(["/api/report"]);
    expect(within(openBugsTile()).getByText("3")).toBeInTheDocument();

    await stub.release({ "/api/report": report(4) });
    expect(await within(openBugsTile()).findByText("4")).toBeInTheDocument();
    expectNoLoading(watch);
  });

  it("rereads both reads once on the topbar link when the address does not change at all", async () => {
    const stub = stubReread(reportRoutes(3));
    await openReport(stub, "/report");
    fireEvent.click(screen.getByRole("tab", { name: "Agent use" }));
    expect(await screen.findByText(/Measured: Claude Code since/)).toBeInTheDocument();
    await settle();
    expect(where()).toBe("/report?tab=agent-use");
    // The topbar link drops the tab, so walk to the bare address first.
    fireEvent.click(screen.getByRole("tab", { name: "Project" }));
    await waitFor(() => expect(where()).toBe("/report"));
    await settle();
    const before = stub.calls.length;

    fireEvent.click(topbarLink("Report"));
    await settle();

    expect(where()).toBe("/report");
    expect(since(stub, before)).toEqual(["/api/report"]);
  });

  it("rereads once on browser back and once on forward", async () => {
    const stub = stubReread(reportRoutes(3));
    await openReport(stub, "/report?tab=agent-use", "/report?tab=agent-use&weeks=26");
    expect(await screen.findByText(/Measured: Claude Code since/)).toBeInTheDocument();
    await settle();
    const before = counts(stub, REPORT_READS);

    back();
    await waitFor(() => expect(where()).toBe("/report?tab=agent-use"));
    await settle();
    for (const url of REPORT_READS) expect(reads(stub, url) - before[url], url).toBe(1);

    forward();
    await waitFor(() => expect(where()).toBe("/report?tab=agent-use&weeks=26"));
    await settle();
    for (const url of REPORT_READS) expect(reads(stub, url) - before[url], url).toBe(2);
  });

  it("reads nothing when the range changes, and only the new tab's own read when the tab changes", async () => {
    const stub = stubReread(reportRoutes(3));
    await openReport(stub, "/report");
    const before = stub.calls.length;

    fireEvent.change(screen.getByLabelText("Range"), { target: { value: "26" } });
    await waitFor(() => expect(where()).toBe("/report?weeks=26"));
    fireEvent.change(screen.getByLabelText("Range"), { target: { value: "all" } });
    await waitFor(() => expect(where()).toBe("/report?weeks=all"));
    await settle();
    expect(since(stub, before)).toEqual([]);

    // The Agent use tab reads its own figures when it first renders; the
    // Project read is not issued again for the tab change.
    fireEvent.click(screen.getByRole("tab", { name: "Agent use" }));
    expect(await screen.findByText(/Measured: Claude Code since/)).toBeInTheDocument();
    await settle();
    expect(since(stub, before)).toEqual(["/api/report/usage"]);
  });

  it("still rereads once per manual refresh and once per return to the window after an arrival", async () => {
    const stub = stubReread(reportRoutes(3));
    await openReport(stub, "/report");
    fireEvent.click(topbarLink("Report"));
    await settle();

    await expectOneReadPerRefreshAndReturn(stub, ["/api/report"]);
  });

  it("keeps the figures and says they are out of date when the arrival's reread fails", async () => {
    const stub = stubReread(reportRoutes(3));
    await openReport(stub, "/report");

    stub.hold();
    fireEvent.click(topbarLink("Report"));
    await waitFor(() => expect(stub.heldUrls()).toContain("/api/report"));
    await stub.fail();

    expect(within(openBugsTile()).getByText("3")).toBeInTheDocument();
    expect(screen.getByText(/Out of date: GET \/api\/report failed/)).toBeInTheDocument();
  });
});

// ------------------------------------------------------------- Explore

function list(noun: string, rows: ListRow[] = []): ListResult {
  return { noun, count: rows.length, nodes: rows };
}

function tree(status: string): TreeModel {
  return {
    project: "acme",
    nodes: [{
      noun: "epic", id: "E-1", key: "foundation", title: "Foundation", status, path: "", files: [],
      children: [{ noun: "feature", id: "F-1", key: "foundation/skeleton", title: "Skeleton", status: "ready", path: "", files: [], children: [] }],
    }],
  };
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

const bug = (title: string): ListRow => ({ id: "B-1", key: "bugs/one", title, status: "ready", ref: "bugs/one (B-1)" });

function searchAnswer(title: string): SearchResult {
  return {
    query: "payment", count: 1, truncated: false, kinds: [{ kind: "bug", count: 1 }],
    nodes: [{ noun: "bug", id: "B-1", key: "bugs/one", title, status: "ready", ref: "bugs/one (B-1)", match: "text", count: 1, hits: [] }],
    documents: [],
  };
}

function mapView(title: string): MapView {
  return {
    nodes: [{ noun: "epic", id: "E-1", key: "foundation", title, status: "ready" }],
    edges: [],
    relationships: { vertices: [], edges: [], catalog: [], diagnostics: [], complete: true },
  };
}

function exploreRoutes(status = "ready"): Routes {
  return {
    "/api/status": summary(),
    "/api/version": { version: "0.0.0-test" },
    "/api/tree": tree(status),
    "/api/ls?noun=question": list("question"),
    "/api/ls?noun=bug": list("bug", [bug("Blank on reload")]),
    "/api/ls?noun=chore": list("chore"),
    "/api/ls?noun=tweak": list("tweak"),
    "/api/ls?noun=epics": emptyList("epic"),
    "/api/queue": { entries: [] },
    "/api/build": board(1),
    "/api/search?q=payment": searchAnswer("Payment retries"),
    "/api/search?q=payment&kind=bug": searchAnswer("Payment retries"),
    "/api/map": mapView("Foundation map"),
  };
}

// Browse at /explore: the navigator's five reads, the page's own project
// status, and the empty pane's two.
const BROWSE_READS = [
  "/api/tree", "/api/ls?noun=question", "/api/ls?noun=bug", "/api/ls?noun=chore", "/api/ls?noun=tweak",
  "/api/status", "/api/queue", "/api/build",
];

const navigator = () => within(screen.getByRole("navigation", { name: "Definition navigator" }));

async function openBrowse(stub: Stub) {
  renderConsole("/explore");
  expect(await screen.findByRole("navigation", { name: "Definition navigator" })).toBeInTheDocument();
  expect(await navigator().findByRole("link", { name: "Foundation" })).toBeInTheDocument();
  expect(await screen.findByRole("link", { name: "1 feature in flight" })).toBeInTheDocument();
  await waitFor(() => { for (const url of BROWSE_READS) expect(reads(stub, url), url).toBeGreaterThan(0); });
  await settle();
}

describe("arriving at Explore while it is showing", () => {
  it("reads each of its resources once when it opens", async () => {
    const stub = stubReread(exploreRoutes());
    await openBrowse(stub);

    // The shell reads the project status too, for the topbar.
    for (const url of BROWSE_READS) expect(reads(stub, url), url).toBe(url === "/api/status" ? 2 : 1);
  });

  it("rereads Browse once on the topbar link and shows the changed status", async () => {
    const stub = stubReread(exploreRoutes("ready"));
    await openBrowse(stub);
    const row = () => navigator().getByRole("link", { name: "Foundation" }).closest("li, .nav-row, div") as HTMLElement;
    expect(row().innerHTML).toContain("ready");
    const before = stub.calls.length;

    const watch = watchLoading();
    stub.hold();
    fireEvent.click(topbarLink("Explore"));
    await settle();

    expect(where()).toBe("/explore");
    expect(since(stub, before)).toEqual([...BROWSE_READS].sort());
    expect(navigator().getByRole("link", { name: "Foundation" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "1 feature in flight" })).toBeInTheDocument();

    await stub.release({ "/api/tree": tree("done"), "/api/build": board(2) });
    expect(await screen.findByRole("link", { name: "2 features in flight" })).toBeInTheDocument();
    await waitFor(() => expect(row().innerHTML).toContain("done"));
    expectNoLoading(watch);
  });

  it("reads nothing when the tree opens or closes", async () => {
    const stub = stubReread(exploreRoutes());
    await openBrowse(stub);
    const before = stub.calls.length;

    fireEvent.click(navigator().getByRole("button", { name: "Open all" }));
    expect(await navigator().findByRole("link", { name: "Skeleton" })).toBeInTheDocument();
    fireEvent.click(navigator().getByRole("button", { name: "Close all" }));
    fireEvent.click(navigator().getByRole("button", { name: /^Expand Foundation/ }));
    expect(await navigator().findByRole("link", { name: "Skeleton" })).toBeInTheDocument();
    await settle();

    expect(where()).toBe("/explore");
    expect(since(stub, before)).toEqual([]);
  });

  it("rereads Filter's list, the question count, and the project status once on browser back and once on forward", async () => {
    const stub = stubReread(exploreRoutes());
    renderConsole("/explore?mode=filter&noun=bug", "/explore?mode=filter&noun=bug&x=1");
    expect(await screen.findByText("Blank on reload")).toBeInTheDocument();
    await settle();
    const urls = ["/api/ls?noun=bug", "/api/ls?noun=question", "/api/status"];
    const before = counts(stub, urls);
    const total = stub.calls.length;

    back();
    await waitFor(() => expect(where()).toBe("/explore?mode=filter&noun=bug"));
    await settle();
    for (const url of urls) expect(reads(stub, url) - before[url], url).toBe(1);
    expect(stub.calls.length - total).toBe(urls.length);

    forward();
    await waitFor(() => expect(where()).toBe("/explore?mode=filter&noun=bug&x=1"));
    await settle();
    for (const url of urls) expect(reads(stub, url) - before[url], url).toBe(2);
    expect(screen.getByText("Blank on reload")).toBeInTheDocument();
  });

  it("reads only the new list when a filter changes", async () => {
    const stub = stubReread(exploreRoutes());
    renderConsole("/explore?mode=filter&noun=bug");
    expect(await screen.findByText("Blank on reload")).toBeInTheDocument();
    await settle();
    const before = stub.calls.length;

    fireEvent.change(screen.getByRole("combobox", { name: "Noun" }), { target: { value: "epics" } });
    await waitFor(() => expect(where()).toBe("/explore?mode=filter&noun=epics"));
    await settle();

    // The list is a different resource, so it is read; nothing is reread.
    expect(since(stub, before)).toEqual(["/api/ls?noun=epics"]);
  });

  it("rereads the answer once when the same search is submitted again", async () => {
    const stub = stubReread(exploreRoutes());
    renderConsole("/explore?mode=search&q=payment");
    expect(await screen.findByRole("link", { name: "Payment retries" })).toBeInTheDocument();
    await settle();
    const before = reads(stub, "/api/search?q=payment");

    const watch = watchLoading();
    stub.hold();
    fireEvent.submit(screen.getByRole("search"));
    await waitFor(() => expect(stub.heldUrls()).toContain("/api/search?q=payment"));
    await settle();

    expect(where()).toBe("/explore?mode=search&q=payment");
    expect(reads(stub, "/api/search?q=payment") - before).toBe(1);
    expect(screen.getByRole("link", { name: "Payment retries" })).toBeInTheDocument();

    await stub.release({ "/api/search?q=payment": searchAnswer("Payment retries, capped") });
    expect(await screen.findByRole("link", { name: "Payment retries, capped" })).toBeInTheDocument();
    expectNoLoading(watch);
  });

  it("reads only the kind's answer when a search result tab changes", async () => {
    const stub = stubReread(exploreRoutes());
    renderConsole("/explore?mode=search&q=payment");
    expect(await screen.findByRole("link", { name: "Payment retries" })).toBeInTheDocument();
    await settle();
    const before = stub.calls.length;

    fireEvent.click(screen.getByRole("tab", { name: /Bugs/ }));
    await waitFor(() => expect(where()).toBe("/explore?mode=search&q=payment&kind=bug"));
    await settle();

    expect(since(stub, before)).toEqual(["/api/search?q=payment&kind=bug"]);
  });

  it("rereads the Map once on browser back and reads nothing when a card is selected", async () => {
    const stub = stubReread(exploreRoutes());
    renderConsole("/explore?mode=map&x=1", "/explore?mode=map");
    expect((await screen.findAllByText("Foundation map")).length).toBeGreaterThan(0);
    await settle();
    const quiet = stub.calls.length;

    fireEvent.click(screen.getByRole("button", { name: /Foundation map/ }));
    await waitFor(() => expect(where()).toBe("/explore?mode=map&sel=E-1"));
    await settle();
    expect(since(stub, quiet)).toEqual([]);

    const before = reads(stub, "/api/map");
    back();
    await waitFor(() => expect(where()).toBe("/explore?mode=map&x=1"));
    await settle();
    expect(reads(stub, "/api/map") - before).toBe(1);
  });

  it("reads nothing when a node page behind the same layout changes its tab", async () => {
    const page: NodePage = {
      node: { noun: "task", id: "T-h4k2wq", key: "foundation/skeleton/boot", title: "Boot the app", status: "active" },
      ref: "foundation/skeleton/boot (T-h4k2wq)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
    };
    const stub = stubReread({ ...exploreRoutes(), "/api/node/task/T-h4k2wq?history=integration": page });
    renderConsole("/node/task/T-h4k2wq");
    expect(await screen.findByRole("heading", { level: 1, name: /Boot the app/ })).toBeInTheDocument();
    expect(await navigator().findByRole("link", { name: "Foundation" })).toBeInTheDocument();
    await settle();
    const before = stub.calls.length;

    const tabs = screen.getAllByRole("tab");
    fireEvent.click(tabs[tabs.length - 1]);
    await waitFor(() => expect(where()).toMatch(/^\/node\/task\/T-h4k2wq\?tab=/));
    await settle();

    expect(since(stub, before)).toEqual([]);
  });

  it("still rereads once per manual refresh and once per return to the window after an arrival", async () => {
    const stub = stubReread(exploreRoutes());
    await openBrowse(stub);
    fireEvent.click(topbarLink("Explore"));
    await settle();

    // A refresh rereads every mounted read, the shell's status with them.
    const before = counts(stub, BROWSE_READS);
    fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
    await settle();
    for (const url of BROWSE_READS) expect(reads(stub, url) - before[url], url).toBe(url === "/api/status" ? 2 : 1);

    await waitFor(() => expect(screen.getByRole("button", { name: "Refresh now" })).toBeEnabled());
    const later = Date.now() + 31_000;
    vi.spyOn(Date, "now").mockReturnValue(later);
    Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "visible" });
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
      window.dispatchEvent(new Event("focus"));
    });
    await settle();
    for (const url of BROWSE_READS) expect(reads(stub, url) - before[url], url).toBe(url === "/api/status" ? 4 : 2);
  });

  it("keeps the navigator and the empty pane and names the failure when the arrival's reread fails", async () => {
    const stub = stubReread(exploreRoutes());
    await openBrowse(stub);

    stub.hold();
    fireEvent.click(topbarLink("Explore"));
    await waitFor(() => expect(stub.heldUrls()).toContain("/api/tree"));
    await stub.fail();

    expect(navigator().getByRole("link", { name: "Foundation" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "1 feature in flight" })).toBeInTheDocument();
    expect(within(screen.getByRole("status", { name: "Navigator refresh" })).getAllByText(/refresh failed/).length).toBeGreaterThan(0);
  });
});
