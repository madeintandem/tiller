import { describe, expect, it } from "vitest";

import type { Report, ReportEpic } from "../../api";
import { DEFAULT_RANGE, parseRange, rangeWords, sliceWeeks, sortEpics, weeksNote } from "./range";

const DAY = 24 * 3600 * 1000;
const START = Date.parse("2026-04-06T00:00:00Z");

// n weeks of distinguishable series: week i carries the value i everywhere.
function reportOf(n: number): Report {
  const iso = (i: number) => new Date(START + i * 7 * DAY).toISOString();
  return {
    project: "Acme",
    as_of: iso(n),
    agent_use: false,
    weeks: Array.from({ length: n }, (_, i) => iso(i).slice(0, 10)),
    week_boundaries: Array.from({ length: n + 1 }, (_, i) => iso(i)),
    kpis: {
      features_done: 3,
      features_total: 8,
      shipped_last_7_days: 4,
      median_cycle_days: 4.8,
      median_lead_days: 11,
      completed_features: 2,
      open_bugs: 1,
      open_questions: 2,
      answered_questions: 5,
      open_questions_by_week: Array.from({ length: n }, (_, i) => i),
    },
    burn_up: Array.from({ length: n }, (_, i) => ({ scope: i, done: i })),
    throughput: Array.from({ length: n }, (_, i) => ({ feature_work: i, fixes: i, upkeep: i })),
    bugs: Array.from({ length: n }, (_, i) => ({ reported: i, fixed: i, open: i })),
    feature_times: Array.from({ length: n }, (_, i) => ({
      title: `Feature ${i}`,
      completed_at: new Date(START + i * 7 * DAY + DAY).toISOString(),
      cycle_days: 1,
      lead_days: 2,
      cycle_median_days: 1,
      lead_median_days: 2,
    })),
    epics: [],
  };
}

describe("parseRange", () => {
  it("reads the three offered ranges", () => {
    expect(parseRange("8")).toBe(8);
    expect(parseRange("26")).toBe(26);
    expect(parseRange("all")).toBe("all");
  });

  it("reads a missing, unknown, or malformed value as the default", () => {
    for (const raw of [null, "", "12", "0", "-3", "8.5", "ALL", "abc", "8 "]) {
      expect(parseRange(raw)).toBe(DEFAULT_RANGE);
    }
    expect(DEFAULT_RANGE).toBe(8);
  });
});

describe("sliceWeeks", () => {
  it("keeps the last N weeks of every weekly series, aligned", () => {
    const full = reportOf(30);
    const sliced = sliceWeeks(full, 8);
    const last8 = [22, 23, 24, 25, 26, 27, 28, 29];
    expect(sliced.weeks).toHaveLength(8);
    expect(sliced.weeks[0]).toBe(full.weeks[22]);
    expect(sliced.week_boundaries).toHaveLength(9);
    expect(sliced.week_boundaries[0]).toBe(full.week_boundaries[22]);
    expect(sliced.week_boundaries[8]).toBe(full.week_boundaries[30]);
    expect(sliced.burn_up.map((w) => w.scope)).toEqual(last8);
    expect(sliced.throughput.map((w) => w.upkeep)).toEqual(last8);
    expect(sliced.bugs.map((w) => w.open)).toEqual(last8);
  });

  it("leaves every KPI alone, the Open questions sparkline included, and the epics", () => {
    const full = reportOf(30);
    full.epics = [{ title: "A", done: 1, in_progress: 0, not_started: 0, planned: false, planned_features: 0 }];
    const sliced = sliceWeeks(full, 8);
    expect(sliced.kpis).toBe(full.kpis);
    expect(sliced.epics).toBe(full.epics);
    expect(sliced.project).toBe(full.project);
    expect(sliced.as_of).toBe(full.as_of);
  });

  it("drops completed features that fall before the first shown week, keeping their server medians", () => {
    const sliced = sliceWeeks(reportOf(30), 8);
    expect(sliced.feature_times.map((f) => f.title)).toEqual([22, 23, 24, 25, 26, 27, 28, 29].map((i) => `Feature ${i}`));
  });

  it("does not mutate its input", () => {
    const full = reportOf(30);
    sliceWeeks(full, 8);
    expect(full.weeks).toHaveLength(30);
    expect(full.throughput).toHaveLength(30);
  });

  it("shows everything for all, and for a project shorter than the range", () => {
    const full = reportOf(5);
    expect(sliceWeeks(full, "all")).toEqual(full);
    expect(sliceWeeks(full, 26)).toEqual(full);
    expect(sliceWeeks(full, 5)).toEqual(full);
  });

  it("copes with an empty project", () => {
    const sliced = sliceWeeks(reportOf(0), 8);
    expect(sliced.weeks).toEqual([]);
    expect(sliced.throughput).toEqual([]);
  });
});

describe("weeksNote", () => {
  it("says how many weeks a chart shows", () => {
    expect(weeksNote(8)).toBe("Showing the last 8 weeks");
    expect(weeksNote(26, 26)).toBe("Showing the last 26 weeks");
    expect(weeksNote(1)).toBe("Showing the last week");
    expect(weeksNote(0)).toBe("");
  });

  it("says all N weeks when All is selected", () => {
    expect(weeksNote(30, "all")).toBe("Showing all 30 weeks");
    expect(weeksNote(1, "all")).toBe("Showing all 1 week");
  });
});

describe("rangeWords", () => {
  it("names the range in words", () => {
    expect(rangeWords(8)).toBe("the last 8 weeks");
    expect(rangeWords(26)).toBe("the last 26 weeks");
  });
});

function epic(title: string, done: number, inProgress: number, notStarted: number, planned = false): ReportEpic {
  return { title, done, in_progress: inProgress, not_started: notStarted, planned, planned_features: planned ? 2 : 0 };
}

describe("sortEpics", () => {
  it("orders in progress, not started, planned, and sets finished apart", () => {
    const { open, finished } = sortEpics([
      epic("Plan A", 0, 0, 0, true),
      epic("Fresh", 0, 0, 3),
      epic("Done 1", 4, 0, 0),
      epic("Active", 1, 1, 1),
      epic("Between", 2, 0, 2),
      epic("Empty", 0, 0, 0),
      epic("Plan B", 0, 0, 0, true),
      epic("Active 2", 0, 2, 0),
      epic("Done 2", 1, 0, 0),
    ]);
    expect(open.map((e) => e.title)).toEqual(["Active", "Between", "Active 2", "Fresh", "Empty", "Plan A", "Plan B"]);
    expect(finished.map((e) => e.title)).toEqual(["Done 1", "Done 2"]);
  });

  it("returns empty groups for no epics", () => {
    expect(sortEpics([])).toEqual({ open: [], finished: [] });
  });
});
