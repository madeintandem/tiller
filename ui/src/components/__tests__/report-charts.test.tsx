// Vitest/RTL coverage for the Report page's hand-written SVG charts (Task 7
// of the report page plan): each chart draws from a Report payload, owns its
// empty state, hatches the current partial week, and never leaks NaN into the
// SVG when a series is all zero.

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Report } from "../../api";
import BugsChart from "../../views/report/BugsChart";
import BurnUpChart from "../../views/report/BurnUpChart";
import ChartCard from "../../views/report/ChartCard";
import EpicProgressChart from "../../views/report/EpicProgressChart";
import LeadCycleChart from "../../views/report/LeadCycleChart";
import Sparkline from "../../views/report/Sparkline";
import ThroughputChart from "../../views/report/ThroughputChart";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const WEEKS = ["2026-09-28", "2026-10-05", "2026-10-12", "2026-10-19"];
const WEEK_BOUNDARIES = [...WEEKS, "2026-10-26"].map((day) => `${day}T00:00:00Z`);

// Four weeks with every series populated: two completed features, one epic
// with features and one approved epic still empty.
function fixtureReport(): Report {
  return {
    project: "Acme",
    as_of: "2026-10-20T15:00:00Z",
    agent_use: false,
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
  };
}

// The zero-event project: four weeks, every series zero, nothing completed.
function emptyReport(): Report {
  const zeros = WEEKS.map(() => 0);
  return {
    project: "Acme",
    as_of: "2026-10-20T15:00:00Z",
    agent_use: false,
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

// A draft epic, not yet approved: a ghost row carrying only its planned
// feature count.
function planned(title: string, features = 0) {
  return { title, done: 0, in_progress: 0, not_started: 0, planned: true, planned_features: features };
}

// The fixture with a not-yet-approved epic between the two approved ones, in
// planned order.
function ghostReport(): Report {
  const report = fixtureReport();
  report.epics = [report.epics[0], planned("Billing", 2), report.epics[1]];
  return report;
}

// Only draft epics: nothing approved yet, but future work to show.
function plannedOnlyReport(): Report {
  return { ...emptyReport(), epics: [planned("Billing"), planned("Reporting")] };
}

const CHARTS = [BurnUpChart, ThroughputChart, BugsChart, LeadCycleChart, EpicProgressChart];
const WEEKLY = [BurnUpChart, ThroughputChart, BugsChart, LeadCycleChart];

describe("report charts", () => {
  it("renders the burn-up with labeled scope and done lines", () => {
    render(<BurnUpChart report={fixtureReport()} />);
    const img = screen.getByRole("img", { name: "3 of 8 approved features done" });
    const labels = Array.from(img.querySelectorAll("text")).map((t) => t.textContent);
    expect(labels).toContain("Scope 8");
    expect(labels).toContain("Done 3");
    const legend = within(screen.getByRole("list"));
    expect(legend.getByText("Scope")).toBeInTheDocument();
    expect(legend.getByText("Done")).toBeInTheDocument();
  });

  it("marks the current week as partial in every weekly chart", () => {
    for (const Chart of WEEKLY) {
      const { container, unmount } = render(<Chart report={fixtureReport()} />);
      expect(container.querySelectorAll('[data-partial="true"]')).toHaveLength(1);
      const band = container.querySelector('[data-partial="true"]')!;
      const pattern = container.querySelector('pattern[id^="report-partial-"]')!;
      expect(pattern).not.toBeNull();
      expect(band.getAttribute("fill")).toBe(`url(#${pattern.id})`);
      unmount();
    }
  });

  it("adds Week so far to the current week's tooltip only", () => {
    const { container } = render(<ThroughputChart report={fixtureReport()} />);
    fireEvent.pointerMove(container.querySelector('[data-week="1"]')!);
    const earlier = screen.getByRole("tooltip");
    expect(earlier).toHaveTextContent("Week of Oct 5");
    expect(earlier).toHaveTextContent("Feature work 3");
    expect(earlier).not.toHaveTextContent("Week so far");

    fireEvent.pointerMove(container.querySelector('[data-week="3"]')!);
    expect(screen.getByRole("tooltip")).toHaveTextContent("Week so far");
    fireEvent.pointerLeave(container.querySelector('[data-week="3"]')!);
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("renders every chart's empty state", () => {
    const copy: [typeof BurnUpChart, string][] = [
      [BurnUpChart, "No approved scope yet"],
      [ThroughputChart, "Nothing shipped yet"],
      [BugsChart, "No bugs reported yet"],
      [LeadCycleChart, "No features completed yet"],
      [EpicProgressChart, "No approved scope yet"],
    ];
    for (const [Chart, text] of copy) {
      const { container, unmount } = render(<Chart report={emptyReport()} />);
      expect(within(container).getAllByText(text)).toHaveLength(1);
      expect(container.querySelector("svg[role='img']")).toBeNull();
      unmount();
    }
  });

  it("draws one dumbbell per completed feature", () => {
    const report = fixtureReport();
    const { container } = render(<LeadCycleChart report={report} />);
    expect(container.querySelectorAll("[data-dumbbell]")).toHaveLength(report.feature_times.length);
  });

  it.each([
    {
      zone: "UTC",
      boundaries: WEEK_BOUNDARIES,
      completedAt: "2026-10-18T17:30:00-07:00", // Monday UTC, Sunday for a remote reader
      hoursIntoWeek: 0.5,
      weekHours: 168,
    },
    {
      zone: "Tokyo",
      boundaries: [...WEEKS, "2026-10-26"].map((day) => `${day}T00:00:00+09:00`),
      completedAt: "2026-10-18T15:30:00Z", // Monday in the server's timezone
      hoursIntoWeek: 0.5,
      weekHours: 168,
    },
    {
      zone: "New York across spring DST",
      weeks: ["2026-02-09", "2026-02-16", "2026-02-23", "2026-03-02"],
      boundaries: [
        "2026-02-09T00:00:00-05:00",
        "2026-02-16T00:00:00-05:00",
        "2026-02-23T00:00:00-05:00",
        "2026-03-02T00:00:00-05:00",
        "2026-03-09T00:00:00-04:00",
      ],
      completedAt: "2026-03-09T03:30:00Z",
      hoursIntoWeek: 166.5,
      weekHours: 167,
    },
  ])("positions completion dots and median lines in $zone server bands", (scenario) => {
    const report = fixtureReport();
    report.weeks = scenario.weeks ?? WEEKS;
    report.week_boundaries = scenario.boundaries;
    report.as_of = scenario.completedAt;
    report.feature_times = [{ ...report.feature_times[0], completed_at: scenario.completedAt }];
    const { container } = render(<LeadCycleChart report={report} />);
    const partial = container.querySelector('[data-partial="true"]')!;
    const start = Number(partial.getAttribute("x"));
    const band = Number(partial.getAttribute("width"));
    const expected = start + band * scenario.hoursIntoWeek / scenario.weekHours;
    for (const series of ["cycle", "lead"]) {
      const dot = container.querySelector(`circle[data-series="${series}"]`)!;
      const x = Number(dot.getAttribute("cx"));
      expect(x).toBeGreaterThan(start);
      expect(x).toBeLessThan(start + band);
      expect(x).toBeCloseTo(expected, 8);
      const median = container.querySelector(`polyline[data-series="${series}-median"]`)!;
      expect(Number(median.getAttribute("points")!.split(",")[0])).toBeCloseTo(expected, 8);
    }
  });

  it("shows the feature name and both durations on hover", () => {
    const { container } = render(<LeadCycleChart report={fixtureReport()} />);
    fireEvent.pointerMove(container.querySelectorAll("[data-dumbbell]")[0]);
    const tip = screen.getByRole("tooltip");
    expect(tip).toHaveTextContent("Checkout flow");
    expect(tip).toHaveTextContent("Cycle 6.5 d");
    expect(tip).toHaveTextContent("Lead 9.5 d");
  });

  it("labels an empty approved epic", () => {
    render(<EpicProgressChart report={fixtureReport()} />);
    expect(screen.getByText("3 of 8 features")).toBeInTheDocument();
    expect(screen.getByText("No features yet")).toBeInTheDocument();
  });

  it("draws a not-yet-approved epic as an empty dotted track", () => {
    const { container } = render(<EpicProgressChart report={ghostReport()} />);
    const rows = container.querySelectorAll("[data-planned='true']");
    expect(rows).toHaveLength(1);
    const ghost = rows[0];
    expect(ghost).toHaveTextContent("Billing");
    expect(within(ghost as HTMLElement).getByText("2 features planned")).toBeInTheDocument();
    expect(ghost.querySelector("text")).toHaveClass("report-row-title", "report-row-planned");
    const track = ghost.querySelector(".report-ghost-track")!;
    expect(track).not.toBeNull();
    // Round dots: a dash array with a round cap.
    expect(track.getAttribute("stroke-dasharray")).toBeTruthy();
    expect(track.getAttribute("stroke-linecap")).toBe("round");
    // Only the hover target and the dotted track: no segments, no filled track.
    expect(Array.from(ghost.querySelectorAll("rect")).map((r) => r.getAttribute("class"))).toEqual([
      "report-hit",
      "report-ghost-track",
    ]);
    // The approved rows still draw as before, in order around the ghost.
    const titles = Array.from(container.querySelectorAll(".report-row-title")).map((t) => t.textContent);
    expect(titles).toEqual(["Payments", "Billing", "Reporting"]);
    expect(screen.getByText("3 of 8 features")).toBeInTheDocument();
    expect(screen.getByText("No features yet")).toBeInTheDocument();
  });

  it("labels a ghost row by its planned feature count", () => {
    const report = fixtureReport();
    report.epics = [planned("Billing", 1), planned("Reporting", 0)];
    render(<EpicProgressChart report={report} />);
    expect(screen.getByText("1 feature planned")).toBeInTheDocument();
    expect(screen.getByText("Not yet approved", { selector: "text" })).toBeInTheDocument();
  });

  it("names a ghost row in a one-row tooltip", () => {
    const { container } = render(<EpicProgressChart report={ghostReport()} />);
    fireEvent.pointerMove(container.querySelector("[data-planned='true']")!);
    const rows = screen.getByRole("tooltip").querySelectorAll(".report-tooltip-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("Billing");
    expect(rows[0]).toHaveTextContent("2 features planned");
  });

  it("draws the ghost track and its legend swatch with the same round dots", () => {
    const { container } = render(<EpicProgressChart report={ghostReport()} />);
    const track = container.querySelector(".report-ghost-track")!;
    const swatch = container.querySelector("[data-swatch='outline'] rect")!;
    for (const attr of ["stroke-dasharray", "stroke-linecap", "stroke-width"]) {
      expect(swatch.getAttribute(attr)).toBe(track.getAttribute(attr));
    }
  });

  it("shows the Not yet approved legend entry only with a ghost row", () => {
    const { container, unmount } = render(<EpicProgressChart report={fixtureReport()} />);
    expect(within(screen.getByRole("list")).queryByText("Not yet approved")).toBeNull();
    expect(container.querySelector("[data-planned='true']")).toBeNull();
    unmount();

    render(<EpicProgressChart report={ghostReport()} />);
    const entry = within(screen.getByRole("list")).getByText("Not yet approved").closest("li")!;
    expect(entry.querySelector("[data-swatch='outline']")).not.toBeNull();
  });

  it("counts ghost rows in the chart summary", () => {
    render(<EpicProgressChart report={ghostReport()} />);
    expect(
      screen.getByRole("img", { name: "3 of 8 features done across 2 epics and 1 epic not yet approved" }),
    ).toBeInTheDocument();
  });

  it("draws ghost rows instead of the empty state when nothing is approved yet", () => {
    const { container } = render(<EpicProgressChart report={plannedOnlyReport()} />);
    expect(screen.queryByText("No approved scope yet")).toBeNull();
    expect(container.querySelectorAll("[data-planned='true']")).toHaveLength(2);
    expect(screen.getAllByText("Not yet approved", { selector: "text" })).toHaveLength(2);
    expect(screen.getByRole("img", { name: "2 epics not yet approved" })).toBeInTheDocument();
  });

  it("names each throughput and bug series in client language", () => {
    render(<ThroughputChart report={fixtureReport()} />);
    for (const label of ["Feature work", "Fixes", "Upkeep"]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
    cleanup();
    render(<BugsChart report={fixtureReport()} />);
    for (const label of ["Reported", "Fixed", "Open"]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
  });

  it("draws a labeled sparkline", () => {
    const { container } = render(<Sparkline values={[1, 3, 2, 2]} label="Open questions by week" />);
    const svg = screen.getByRole("img", { name: "Open questions by week" });
    expect(svg).toHaveAttribute("width", "64");
    expect(svg).toHaveAttribute("height", "20");
    expect(container.querySelector("polyline")?.getAttribute("points")?.split(" ")).toHaveLength(4);
  });

  it("never renders NaN", () => {
    for (const report of [fixtureReport(), emptyReport(), ghostReport(), plannedOnlyReport()]) {
      render(
        <>
          {CHARTS.map((Chart, i) => (
            <Chart key={i} report={report} />
          ))}
          <Sparkline values={report.kpis.open_questions_by_week} label="Open questions by week" />
          <Sparkline values={[]} label="No weeks" />
        </>,
      );
      expect(document.body.innerHTML).not.toMatch(/NaN|Infinity/);
      cleanup();
    }
  });

  it("gives every pattern on the page its own id", () => {
    const { container } = render(
      <>
        {CHARTS.map((Chart, i) => (
          <Chart key={i} report={fixtureReport()} />
        ))}
      </>,
    );
    const ids = Array.from(container.querySelectorAll("pattern")).map((p) => p.id);
    expect(ids.filter((id) => id.startsWith("report-partial-"))).toHaveLength(WEEKLY.length);
    expect(ids.every((id) => /^[A-Za-z0-9-]+$/.test(id))).toBe(true);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("raises the tooltip above the pointer in the plot's lower half", () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      left: 0, top: 0, right: 640, bottom: 240, width: 640, height: 240, x: 0, y: 0, toJSON: () => ({}),
    });
    const { container } = render(<ThroughputChart report={fixtureReport()} />);
    const week = container.querySelector('[data-week="1"]')!;
    fireEvent.pointerMove(week, { clientX: 100, clientY: 60 });
    expect(screen.getByRole("tooltip")).not.toHaveClass("is-raised");
    fireEvent.pointerMove(week, { clientX: 100, clientY: 200 });
    expect(screen.getByRole("tooltip")).toHaveClass("is-raised");
  });

  it("keeps a count of 1 visible beside a large week", () => {
    const report = fixtureReport();
    report.throughput = [
      { feature_work: 400, fixes: 1, upkeep: 1 },
      { feature_work: 0, fixes: 0, upkeep: 0 },
      { feature_work: 0, fixes: 0, upkeep: 0 },
      { feature_work: 0, fixes: 0, upkeep: 0 },
    ];
    const { container } = render(<ThroughputChart report={report} />);
    const fixes = container.querySelectorAll('[data-kind="fixes"]');
    expect(fixes).toHaveLength(1);
    expect(Number(fixes[0].getAttribute("height"))).toBeGreaterThanOrEqual(2);
    expect(container.querySelectorAll('[data-kind="upkeep"]')).toHaveLength(1);
  });

  // narrowBugBars renders n weeks of bugs at the 640px default width and
  // returns each Reported and Fixed bar's edges with its week's band.
  function narrowBugBars(n: number) {
    const report = fixtureReport();
    const start = Date.UTC(2016, 11, 5);
    report.week_boundaries = Array.from({ length: n + 1 }, (_, i) => new Date(start + i * 7 * 86400000).toISOString());
    report.weeks = report.week_boundaries.slice(0, -1).map((boundary) => boundary.slice(0, 10));
    report.bugs = report.weeks.map(() => ({ reported: 2, fixed: 1, open: 1 }));
    const { container } = render(<BugsChart report={report} />);
    expect(container.innerHTML).not.toMatch(/NaN|Infinity/);
    const band = (640 - 40 - 72) / n;
    return ["reported", "fixed"].flatMap((series) => {
      const bars = Array.from(container.querySelectorAll(`[data-series="${series}"]`));
      expect(bars).toHaveLength(n);
      return bars.map((bar, i) => {
        // columnPath starts at the bar's left edge and its last curve ends on
        // the right edge.
        const d = bar.getAttribute("d")!;
        const left = parseFloat(d.slice(1));
        const right = parseFloat(d.slice(d.lastIndexOf("Q") + 1));
        return { left, right, weekLeft: 40 + band * i, weekRight: 40 + band * (i + 1) };
      });
    });
  }

  it("keeps narrow bug bars at least 1px and inside their week", () => {
    // 200 weeks leaves about 2.6px per week.
    for (const { left, right, weekLeft, weekRight } of narrowBugBars(200)) {
      expect(right - left).toBeGreaterThanOrEqual(1 - 1e-9);
      expect(left).toBeGreaterThanOrEqual(weekLeft - 1e-9);
      expect(right).toBeLessThanOrEqual(weekRight + 1e-9);
    }
  });

  it("keeps bug bars inside their week under 2px per week", () => {
    // 520 weeks leaves about 1px per week, too narrow for two 1px bars.
    for (const { left, right, weekLeft, weekRight } of narrowBugBars(520)) {
      expect(right).toBeGreaterThan(left);
      expect(left).toBeGreaterThanOrEqual(weekLeft - 1e-9);
      expect(right).toBeLessThanOrEqual(weekRight + 1e-9);
    }
  });

  // JC is color blind: each chart's series must differ by more than hue.
  describe("non-hue encodings", () => {
    it("dashes the burn-up's Scope line and not its Done line", () => {
      const { container } = render(<BurnUpChart report={fixtureReport()} />);
      expect(container.querySelector('[data-series="scope"]')).toHaveAttribute("stroke-dasharray");
      expect(container.querySelector('[data-series="done"]')).not.toHaveAttribute("stroke-dasharray");
    });

    it("dashes the lead median and draws lead as a hollow dot", () => {
      const { container } = render(<LeadCycleChart report={fixtureReport()} />);
      expect(container.querySelector('[data-series="lead-median"]')).toHaveAttribute("stroke-dasharray");
      expect(container.querySelector('[data-series="cycle-median"]')).not.toHaveAttribute("stroke-dasharray");
      for (const dot of container.querySelectorAll('[data-series="lead"]')) {
        expect(dot).toHaveClass("report-dot-hollow");
      }
      for (const dot of container.querySelectorAll('[data-series="cycle"]')) {
        expect(dot).not.toHaveClass("report-dot-hollow");
      }
    });

    it("hatches throughput's Fixes segments and their legend swatch", () => {
      const { container } = render(<ThroughputChart report={fixtureReport()} />);
      const hatched = (el: Element) => {
        const id = /^url\(#(.+)\)$/.exec(el.getAttribute("fill") ?? "")?.[1];
        return id !== undefined && id.startsWith("report-hatch-") && container.querySelector(`pattern[id="${id}"]`) !== null;
      };
      const fixes = Array.from(container.querySelectorAll('[data-kind="fixes"]'));
      expect(fixes.length).toBeGreaterThan(0);
      expect(fixes.every(hatched)).toBe(true);
      expect(Array.from(container.querySelectorAll('[data-kind="feature_work"]')).some(hatched)).toBe(false);

      const legend = within(screen.getByRole("list"));
      const swatch = legend.getByText("Fixes").parentElement!.querySelector("[data-swatch]")!;
      expect(swatch).toHaveAttribute("data-swatch", "hatched");
      expect(hatched(swatch.querySelector(":scope > rect")!)).toBe(true);
    });

    it("draws bugs Open as a line and Reported and Fixed as bars", () => {
      const { container } = render(<BugsChart report={fixtureReport()} />);
      expect(container.querySelector('[data-series="open"]')?.tagName).toBe("polyline");
      for (const series of ["reported", "fixed"]) {
        const bars = container.querySelectorAll(`[data-series="${series}"]`);
        expect(bars.length).toBeGreaterThan(0);
        for (const bar of bars) expect(bar.tagName).toBe("path");
      }
    });
  });

  // The enlarge control (ChartCard): a magnifier on each chart with data
  // opens the same chart, live and taller, in a native modal <dialog>.
  // jsdom has no showModal or close, so they are stubbed to set open and
  // fire "close" the way a browser does.
  describe("enlarge", () => {
    const TITLED: [typeof BurnUpChart, string][] = [
      [BurnUpChart, "Feature burn-up"],
      [ThroughputChart, "Throughput"],
      [BugsChart, "Bugs reported vs fixed"],
      [LeadCycleChart, "Lead and cycle time"],
      [EpicProgressChart, "Epic progress"],
    ];
    const native = {
      showModal: Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype, "showModal"),
      close: Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype, "close"),
    };

    beforeEach(() => {
      Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
        configurable: true,
        value(this: HTMLDialogElement) {
          this.open = true;
        },
      });
      Object.defineProperty(HTMLDialogElement.prototype, "close", {
        configurable: true,
        value(this: HTMLDialogElement) {
          if (!this.open) return;
          this.open = false;
          this.dispatchEvent(new Event("close"));
        },
      });
    });

    afterEach(() => {
      for (const [name, descriptor] of Object.entries(native)) {
        if (descriptor) Object.defineProperty(HTMLDialogElement.prototype, name, descriptor);
        else Reflect.deleteProperty(HTMLDialogElement.prototype, name);
      }
    });

    function renderCard(Chart: typeof BurnUpChart, report = fixtureReport()) {
      return render(
        <ChartCard>
          <Chart report={report} />
        </ChartCard>,
      );
    }

    it("puts an Enlarge button on every chart with data", () => {
      for (const [Chart, title] of TITLED) {
        const { unmount } = renderCard(Chart);
        const button = screen.getByRole("button", { name: `Enlarge ${title}` });
        expect(button).toHaveAttribute("title", "Enlarge");
        unmount();
      }
    });

    it("gives an empty chart no Enlarge button", () => {
      for (const [Chart] of TITLED) {
        const { unmount } = renderCard(Chart, emptyReport());
        expect(screen.queryByRole("button", { name: /^Enlarge/ })).toBeNull();
        unmount();
      }
    });

    it("draws no Enlarge button on a chart outside a card", () => {
      render(<BurnUpChart report={fixtureReport()} />);
      expect(screen.queryByRole("button", { name: /^Enlarge/ })).toBeNull();
    });

    it("opens a second, taller copy of the chart in a dialog", () => {
      for (const [Chart, title] of TITLED) {
        const { unmount } = renderCard(Chart);
        expect(screen.queryByRole("dialog")).toBeNull();
        fireEvent.click(screen.getByRole("button", { name: `Enlarge ${title}` }));

        const dialog = screen.getByRole("dialog", { name: title });
        expect(within(dialog).getByRole("heading", { name: title })).toBeInTheDocument();
        expect(within(dialog).queryByRole("button", { name: /^Enlarge/ })).toBeNull();
        expect(screen.getAllByRole("heading", { name: title })).toHaveLength(2);

        const [page, enlarged] = screen.getAllByRole("img").filter((svg) => svg.closest(".report-plot"));
        expect(dialog.contains(page)).toBe(false);
        expect(dialog.contains(enlarged)).toBe(true);
        // A weekly chart grows from 240 to 1.5x; Epic progress sets its own
        // height (two 44px rows) and keeps it.
        const [before, after] = Chart === EpicProgressChart ? ["92", "92"] : ["240", "360"];
        expect(page).toHaveAttribute("height", before);
        expect(enlarged).toHaveAttribute("height", after);
        unmount();
      }
    });

    it("draws ghost rows in the enlarged copy too", () => {
      renderCard(EpicProgressChart, plannedOnlyReport());
      fireEvent.click(screen.getByRole("button", { name: "Enlarge Epic progress" }));
      const dialog = screen.getByRole("dialog", { name: "Epic progress" });
      expect(dialog.querySelectorAll("[data-planned='true']")).toHaveLength(2);
      expect(within(dialog).getByRole("list")).toHaveTextContent("Not yet approved");
    });

    it("keeps tooltips working inside the dialog", () => {
      renderCard(ThroughputChart);
      fireEvent.click(screen.getByRole("button", { name: "Enlarge Throughput" }));
      const dialog = screen.getByRole("dialog");
      fireEvent.pointerMove(dialog.querySelector('[data-week="1"]')!);
      expect(within(dialog).getByRole("tooltip")).toHaveTextContent("Feature work 3");
    });

    it("closes from the Close button and returns focus to the opener", () => {
      renderCard(BugsChart);
      const opener = screen.getByRole("button", { name: "Enlarge Bugs reported vs fixed" });
      fireEvent.click(opener);
      fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Close" }));
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(screen.getAllByRole("heading", { name: "Bugs reported vs fixed" })).toHaveLength(1);
      expect(opener).toHaveFocus();
    });

    it("unmounts the enlarged copy and refocuses the opener when the browser closes the dialog on Escape", () => {
      renderCard(BurnUpChart);
      const opener = screen.getByRole("button", { name: "Enlarge Feature burn-up" });
      fireEvent.click(opener);
      const dialog = screen.getByRole("dialog") as HTMLDialogElement;
      expect(screen.getAllByRole("heading", { name: "Feature burn-up" })).toHaveLength(2);
      // The browser's own Escape handling: "cancel", then close(), which
      // fires "close". Only the app's close handler can unmount the copy and
      // move focus, since the stub does neither.
      fireEvent(dialog, new Event("cancel", { cancelable: true }));
      dialog.close();
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(screen.getAllByRole("heading", { name: "Feature burn-up" })).toHaveLength(1);
      expect(opener).toHaveFocus();
    });

    // The open dialog's panel sits at (100, 100)-(500, 400). A press on
    // ::backdrop targets the dialog element but lies outside that box; a
    // press on the panel's own scrollbar targets it inside the box.
    function openLeadCycle() {
      vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (this: Element) {
        const panel = this instanceof HTMLDialogElement;
        return panel
          ? { left: 100, top: 100, right: 500, bottom: 400, width: 400, height: 300, x: 100, y: 100, toJSON: () => ({}) }
          : { left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0, x: 0, y: 0, toJSON: () => ({}) };
      });
      renderCard(LeadCycleChart);
      fireEvent.click(screen.getByRole("button", { name: "Enlarge Lead and cycle time" }));
      return screen.getByRole("dialog");
    }
    const BACKDROP = { clientX: 20, clientY: 20 };

    it("closes on a press and release on the backdrop", () => {
      const dialog = openLeadCycle();
      fireEvent.pointerDown(dialog, BACKDROP);
      fireEvent.click(dialog, BACKDROP);
      expect(screen.queryByRole("dialog")).toBeNull();
    });

    it("stays open on a click inside the chart", () => {
      const dialog = openLeadCycle();
      const heading = within(dialog).getByRole("heading", { name: "Lead and cycle time" });
      fireEvent.pointerDown(heading, { clientX: 200, clientY: 150 });
      fireEvent.click(heading, { clientX: 200, clientY: 150 });
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    it("stays open when a press inside the panel is released on the backdrop", () => {
      const dialog = openLeadCycle();
      fireEvent.pointerDown(within(dialog).getByRole("heading", { name: "Lead and cycle time" }), { clientX: 200, clientY: 150 });
      // The click of a drag that ends outside lands on the common ancestor,
      // the dialog.
      fireEvent.click(dialog, BACKDROP);
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    it("stays open on a click on the panel's own scrollbar", () => {
      const dialog = openLeadCycle();
      fireEvent.pointerDown(dialog, { clientX: 495, clientY: 250 });
      fireEvent.click(dialog, { clientX: 495, clientY: 250 });
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    it("draws the enlarged copy at the open dialog's width from its first render", () => {
      // A plot measures 640 on the page, 0 inside a closed (display: none)
      // dialog, and 1000 inside an open one.
      vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (this: Element) {
        const width = this.closest("dialog") ? (this.closest("dialog[open]") ? 1000 : 0) : 640;
        return { left: 0, top: 0, right: width, bottom: 0, width, height: 0, x: 0, y: 0, toJSON: () => ({}) };
      });
      renderCard(BurnUpChart);
      fireEvent.click(screen.getByRole("button", { name: "Enlarge Feature burn-up" }));
      const dialog = screen.getByRole("dialog");
      expect(dialog.querySelector(".report-plot > svg")).toHaveAttribute("width", "1000");
      expect(within(dialog).getByRole("button", { name: "Close" })).toHaveFocus();
    });

    it("keeps pattern ids unique and finite numbers with a dialog open", () => {
      for (const [Chart, title] of TITLED) {
        const { container, unmount } = render(
          <>
            {TITLED.map(([C], i) => (
              <ChartCard key={i}>
                <C report={fixtureReport()} />
              </ChartCard>
            ))}
          </>,
        );
        fireEvent.click(screen.getByRole("button", { name: `Enlarge ${title}` }));
        const ids = Array.from(document.querySelectorAll("pattern")).map((p) => p.id);
        const partials = ids.filter((id) => id.startsWith("report-partial-")).length;
        expect(partials).toBe(WEEKLY.length + (Chart === EpicProgressChart ? 0 : 1));
        expect(new Set(ids).size).toBe(ids.length);
        expect(container.innerHTML).not.toMatch(/NaN|Infinity/);
        unmount();
      }
    });
  });
});
