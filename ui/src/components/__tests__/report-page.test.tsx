// Vitest/RTL coverage for the Report page (Task 8 of the report page plan):
// the header, the off-branch notice, the KPI strip, and the page's charts
// composed from one /api/report read. Fetch stubbing follows drift.test.tsx's
// stubFetch-keyed-by-URL convention (no MSW in this harness).

import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Report } from "../../api";
import ReportPage from "../../views/Report";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// Matches drift.test.tsx's own stubFetch: an unmapped path 404s the way
// fetchJSON expects, so a missing fixture fails loudly instead of silently
// resolving to undefined.
function stubFetch(routes: Record<string, unknown>) {
  const spy = vi.fn((input: unknown) => {
    const url = String(input);
    if (url in routes) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[url]) });
    }
    return Promise.resolve({
      ok: false,
      status: 404,
      statusText: "Not Found",
      text: () => Promise.resolve(""),
    });
  });
  vi.stubGlobal("fetch", spy);
  return spy;
}

function renderReport(entry = "/report") {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <ReportPage />
    </MemoryRouter>,
  );
}

const WEEKS = ["2026-09-28", "2026-10-05", "2026-10-12", "2026-10-19"];
const WEEK_BOUNDARIES = [...WEEKS, "2026-10-26"].map((day) => `${day}T00:00:00Z`);

// Four weeks with every series populated, the same shape report-charts.test
// draws from.
function fixtureReport(overrides: Partial<Report> = {}): Report {
  return {
    project: "Acme Platform",
    as_of: "2026-10-20T15:00:00Z",
    agent_use: true,
    weeks: WEEKS,
    week_boundaries: WEEK_BOUNDARIES,
    kpis: {
      features_done: 3,
      features_total: 8,
      shipped_last_7_days: 4,
      median_cycle_days: 4.83,
      median_lead_days: 10.95,
      completed_features: 2,
      open_bugs: 1,
      open_questions: 2,
      answered_questions: 5,
      open_questions_by_week: [1, 3, 2, 2],
    },
    burn_up: [
      { scope: 5, done: 0 },
      { scope: 8, done: 1 },
      { scope: 8, done: 3 },
      { scope: 8, done: 3 },
    ],
    throughput: [
      { feature_work: 0, fixes: 0, upkeep: 0 },
      { feature_work: 3, fixes: 1, upkeep: 1 },
      { feature_work: 2, fixes: 1, upkeep: 0 },
      { feature_work: 1, fixes: 0, upkeep: 1 },
    ],
    bugs: [
      { reported: 1, fixed: 0, open: 1 },
      { reported: 2, fixed: 1, open: 2 },
      { reported: 0, fixed: 1, open: 1 },
      { reported: 1, fixed: 1, open: 1 },
    ],
    feature_times: [
      {
        title: "Checkout flow",
        completed_at: "2026-10-07T10:00:00Z",
        cycle_days: 6.46,
        lead_days: 9.5,
        cycle_median_days: 6.46,
        lead_median_days: 9.5,
      },
      {
        title: "Invoice export",
        completed_at: "2026-10-14T16:00:00Z",
        cycle_days: 3.2,
        lead_days: 12.4,
        cycle_median_days: 4.83,
        lead_median_days: 10.95,
      },
    ],
    epics: [
      { title: "Payments", done: 3, in_progress: 2, not_started: 3, planned: false, planned_features: 0 },
      { title: "Reporting", done: 0, in_progress: 0, not_started: 0, planned: false, planned_features: 0 },
    ],
    ...overrides,
  };
}

// The zero-event project: four weeks, every series zero, nothing completed.
function emptyReport(): Report {
  const zeros = WEEKS.map(() => 0);
  return {
    project: "Acme Platform",
    as_of: "2026-10-20T15:00:00Z",
    agent_use: true,
    weeks: WEEKS,
    week_boundaries: WEEK_BOUNDARIES,
    kpis: {
      features_done: 0,
      features_total: 0,
      shipped_last_7_days: 0,
      median_cycle_days: null,
      median_lead_days: null,
      completed_features: 0,
      open_bugs: 0,
      open_questions: 0,
      answered_questions: 0,
      open_questions_by_week: zeros,
    },
    burn_up: WEEKS.map(() => ({ scope: 0, done: 0 })),
    throughput: WEEKS.map(() => ({ feature_work: 0, fixes: 0, upkeep: 0 })),
    bugs: WEEKS.map(() => ({ reported: 0, fixed: 0, open: 0 })),
    feature_times: [],
    epics: [],
  };
}

// The tile whose label is `label`: its <dt>'s grouping <div>.
function tile(label: string): HTMLElement {
  const term = screen.getAllByRole("term").find((t) => t.textContent === label);
  if (!term) throw new Error(`no KPI tile labeled ${label}`);
  return term.parentElement!;
}

const emptyUsage = {
  weeks: [],
  week_boundaries: [],
  harnesses: [],
  full_coverage: "",
  coverage: [],
  kpis: { total: 0, cache_read: 0, input: 0, cache_write: 0, output: 0, median_per_item: null, items: 0, tied_share: 0 },
  weekly: [],
  stages: [],
  models: [],
  model_stages: [],
  features: [],
  features_left_out: 0,
  item_bands: [],
  items_left_out: 0,
  items_unmeasured: 0,
};

describe("Report page", () => {
  it("names Report while loading", () => {
    vi.stubGlobal("fetch", vi.fn(() => new Promise(() => {})));
    renderReport();
    expect(screen.getByRole("heading", { name: "Report" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Loading");
  });

  it("renders the header without branch or tiller vocabulary", async () => {
    stubFetch({ "/api/report": fixtureReport() });
    renderReport();

    expect(await screen.findByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();
    expect(screen.getByText("Project report")).toBeInTheDocument();
    expect(screen.getByText(/^As of /)).toBeInTheDocument();
    expect(document.title).toBe("Report · Tiller");
    expect(document.body.textContent).not.toMatch(/\b[EFTBQ]-[a-z0-9]{6}\b/);
    expect(screen.queryByRole("note")).toBeNull();
  });

  it("shows the off-branch notice without blocking the charts", async () => {
    stubFetch({
      "/api/report": fixtureReport({ off_branch: { branch: "feature/x", integration: "main" } }),
    });
    renderReport();

    expect(
      await screen.findByText("Showing branch feature/x, not main. Numbers include unmerged work."),
    ).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "3 of 8 approved features done" })).toBeInTheDocument();
  });

  it("renders the five KPI tiles in order", async () => {
    stubFetch({ "/api/report": fixtureReport() });
    renderReport();

    await screen.findByRole("heading", { level: 1, name: "Acme Platform" });
    expect(screen.getAllByRole("term").map((t) => t.textContent)).toEqual([
      "Features done",
      "Shipped, last 7 days",
      "Median cycle time",
      "Open bugs",
      "Open questions",
    ]);
    expect(tile("Features done")).toHaveTextContent("3 of 8");
    expect(tile("Shipped, last 7 days")).toHaveTextContent("4");
    expect(tile("Median cycle time")).toHaveTextContent("4.8 d");
    expect(tile("Median cycle time")).toHaveTextContent("lead 11 d · 2 features");
    expect(tile("Open bugs")).toHaveTextContent("1");
    expect(tile("Open questions")).toHaveTextContent("2");
    expect(tile("Open questions")).toHaveTextContent("5 answered");
    expect(screen.getByRole("img", { name: "Open questions by week" })).toBeInTheDocument();
  });

  it("shows Not yet for an empty median", async () => {
    stubFetch({
      "/api/report": fixtureReport({
        kpis: { ...fixtureReport().kpis, median_cycle_days: null, median_lead_days: null, completed_features: 0 },
      }),
    });
    renderReport();

    await screen.findByRole("heading", { level: 1, name: "Acme Platform" });
    expect(tile("Median cycle time")).toHaveTextContent("Not yet");
    expect(tile("Median cycle time")).not.toHaveTextContent("lead");
    expect(document.body.innerHTML).not.toMatch(/NaN/);
  });

  it("renders every empty state from an empty report", async () => {
    stubFetch({ "/api/report": emptyReport() });
    renderReport();

    await screen.findByRole("heading", { level: 1, name: "Acme Platform" });
    // Burn-up and epic progress share the no-scope copy.
    expect(screen.getAllByText("No approved scope yet")).toHaveLength(2);
    expect(screen.getByText("Nothing shipped yet")).toBeInTheDocument();
    expect(screen.getByText("No bugs reported yet")).toBeInTheDocument();
    expect(screen.getByText("No features completed yet")).toBeInTheDocument();
    expect(tile("Features done")).toHaveTextContent("0 of 0");
    expect(tile("Median cycle time")).toHaveTextContent("Not yet");
    expect(document.body.innerHTML).not.toMatch(/NaN|Infinity/);
  });

  it("shows draft epics as ghost rows while burn-up and the KPI keep approved scope", async () => {
    const report = emptyReport();
    report.epics = [{ title: "Billing", done: 0, in_progress: 0, not_started: 0, planned: true, planned_features: 2 }];
    stubFetch({ "/api/report": report });
    renderReport();

    await screen.findByRole("heading", { level: 1, name: "Acme Platform" });
    // Only the burn-up keeps the no-scope copy; epic progress draws the ghost.
    expect(screen.getAllByText("No approved scope yet")).toHaveLength(1);
    expect(screen.getByText("Billing")).toBeInTheDocument();
    expect(screen.getByText("2 features planned")).toBeInTheDocument();
    expect(tile("Features done")).toHaveTextContent("0 of 0");
  });

  it("puts an Enlarge button on each chart with data and none on an empty one", async () => {
    stubFetch({ "/api/report": fixtureReport() });
    renderReport();

    await screen.findByRole("heading", { level: 1, name: "Acme Platform" });
    expect(screen.getAllByRole("button", { name: /^Enlarge / }).map((b) => b.getAttribute("aria-label"))).toEqual([
      "Enlarge Feature burn-up",
      "Enlarge Throughput",
      "Enlarge Bugs reported vs fixed",
      "Enlarge Lead and cycle time",
      "Enlarge Epic progress",
    ]);
    cleanup();

    stubFetch({ "/api/report": emptyReport() });
    renderReport();
    await screen.findByRole("heading", { level: 1, name: "Acme Platform" });
    expect(screen.queryByRole("button", { name: /^Enlarge / })).toBeNull();
  });

  it("states a failed read", async () => {
    stubFetch({});
    renderReport();

    expect(await screen.findByText(/404/)).toHaveClass("error");
  });

  describe("tabs", () => {
    it("shows the Project tab by default and does not fetch usage", async () => {
      const spy = stubFetch({ "/api/report": fixtureReport(), "/api/report/usage": emptyUsage });
      renderReport();

      expect(await screen.findByRole("img", { name: "3 of 8 approved features done" })).toBeInTheDocument();
      expect(screen.getByRole("navigation", { name: "Report view" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Project" })).toHaveClass("active");
      expect(spy.mock.calls.map((c) => String(c[0]))).not.toContain("/api/report/usage");
    });

    it("opens Agent use from ?tab=agent-use", async () => {
      stubFetch({ "/api/report": fixtureReport(), "/api/report/usage": emptyUsage });
      renderReport("/report?tab=agent-use");

      expect(await screen.findByText("No agent usage recorded yet")).toBeInTheDocument();
      expect(screen.getByRole("heading", { level: 1, name: "Acme Platform" })).toBeInTheDocument();
      expect(screen.queryByRole("img", { name: "3 of 8 approved features done" })).toBeNull();
    });

    it("shows no tabs and never reads usage with capture off", async () => {
      const spy = stubFetch({ "/api/report": fixtureReport({ agent_use: false }), "/api/report/usage": emptyUsage });
      renderReport("/report?tab=agent-use");

      expect(await screen.findByRole("img", { name: "3 of 8 approved features done" })).toBeInTheDocument();
      expect(screen.queryByRole("navigation", { name: "Report view" })).toBeNull();
      expect(screen.queryByRole("button", { name: "Agent use" })).toBeNull();
      expect(spy.mock.calls.map((c) => String(c[0]))).not.toContain("/api/report/usage");
    });

    it("reads an unknown tab as Project", async () => {
      const spy = stubFetch({ "/api/report": fixtureReport(), "/api/report/usage": emptyUsage });
      renderReport("/report?tab=nonsense");

      expect(await screen.findByRole("img", { name: "3 of 8 approved features done" })).toBeInTheDocument();
      expect(spy.mock.calls.map((c) => String(c[0]))).not.toContain("/api/report/usage");
    });
  });
});
