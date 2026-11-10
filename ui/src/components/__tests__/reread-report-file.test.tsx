// Report (both tabs) and the file page while the page rereads: the figures
// and the file stay until the new answer lands, a failed reread keeps them
// and says they are out of date, and the file page does not scroll back to
// its line on a reread.

import { screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ProseFile, Report, UsageReport } from "../../api";
import { expectNoLoading, renderApp, stubReread, summary, watchLoading, type Routes, settleReads } from "./rereadHarness";

afterEach(async () => {
  await settleReads();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
  Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
});

const WEEKS = ["2026-09-28", "2026-10-05", "2026-10-12", "2026-10-19"];
const WEEK_BOUNDARIES = [...WEEKS, "2026-10-26"].map((day) => `${day}T00:00:00Z`);

function report(project: string): Report {
  return {
    project,
    as_of: "2026-10-20T15:00:00Z",
    agent_use: true,
    weeks: WEEKS,
    week_boundaries: WEEK_BOUNDARIES,
    kpis: {
      features_done: 0, features_total: 0, shipped_last_7_days: 0, median_cycle_days: null, median_lead_days: null,
      completed_features: 0, open_bugs: 0, open_questions: 0, answered_questions: 0, open_questions_by_week: WEEKS.map(() => 0),
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

function reportRoutes(): Routes {
  return { "/api/status": summary(), "/api/report": report("Acme Platform"), "/api/report/usage": usage("Claude Code") };
}

const FILE = "/api/prose?path=docs%2Fnotes.md&line=3";

function file(text: string): ProseFile {
  return { path: "docs/notes.md", line: 3, line_text: text, content: `# Notes\n\nOne\n\n${text}\n` };
}

describe("Report while the page rereads", () => {
  it("keeps the Project tab, then updates it in place", async () => {
    const stub = stubReread(reportRoutes());
    renderApp("/report");
    expect(await screen.findByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();

    const watch = watchLoading();
    stub.reread();
    expect(screen.getByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();

    await stub.release({ "/api/report": report("Acme Platform Two") });
    expect(await screen.findByRole("heading", { level: 1, name: "Acme Platform Two" })).toBeInTheDocument();
    expectNoLoading(watch);
  });

  it("keeps the Agent use tab, then updates it in place", async () => {
    const stub = stubReread(reportRoutes());
    renderApp("/report?tab=agent-use");
    expect(await screen.findByText(/Measured: Claude Code since/)).toBeInTheDocument();

    const watch = watchLoading();
    stub.reread();
    expect(screen.getByText(/Measured: Claude Code since/)).toBeInTheDocument();

    await stub.release({ "/api/report/usage": usage("Claude Code CLI") });
    expect(await screen.findByText(/Measured: Claude Code CLI since/)).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent("/report?tab=agent-use");
    expectNoLoading(watch);
  });

  it("keeps both tabs' figures and says they are out of date when the reread fails", async () => {
    const stub = stubReread(reportRoutes());
    renderApp("/report?tab=agent-use");
    expect(await screen.findByText(/Measured: Claude Code since/)).toBeInTheDocument();

    stub.reread();
    await stub.fail();

    expect(screen.getByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();
    expect(screen.getByText(/Measured: Claude Code since/)).toBeInTheDocument();
    expect(screen.getByText(/Out of date: GET \/api\/report failed/)).toBeInTheDocument();
    expect(screen.getByText(/Out of date: GET \/api\/report\/usage failed/)).toBeInTheDocument();
  });
});

describe("The file page while the page rereads", () => {
  it("keeps the file, does not scroll to its line again, then updates it in place", async () => {
    const scrolls = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", { configurable: true, value: scrolls });
    const stub = stubReread({ "/api/status": summary(), [FILE]: file("The line itself") });
    renderApp("/file/docs/notes.md#L3");
    expect(await screen.findByText("The line itself", { selector: "code" })).toBeInTheDocument();
    const arrived = scrolls.mock.calls.length;
    expect(arrived).toBeGreaterThan(0);

    const watch = watchLoading();
    stub.reread();
    expect(screen.getByText("The line itself", { selector: "code" })).toBeInTheDocument();

    await stub.release({ [FILE]: file("The line, edited") });
    expect(await screen.findByText("The line, edited", { selector: "code" })).toBeInTheDocument();
    expect(scrolls.mock.calls.length).toBe(arrived);
    expect(screen.getByTestId("where")).toHaveTextContent("/file/docs/notes.md#L3");
    expectNoLoading(watch);
  });

  it("keeps the file and says it is out of date when the reread fails", async () => {
    const stub = stubReread({ "/api/status": summary(), [FILE]: file("The line itself") });
    renderApp("/file/docs/notes.md#L3");
    expect(await screen.findByText("The line itself", { selector: "code" })).toBeInTheDocument();

    stub.reread();
    await stub.fail();

    expect(screen.getByText("The line itself", { selector: "code" })).toBeInTheDocument();
    expect(screen.getByText(/Out of date: GET \/api\/prose/)).toBeInTheDocument();
  });
});
