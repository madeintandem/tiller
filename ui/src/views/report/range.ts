// What the Report page does to a served /api/report before it draws (R8): trim
// the weekly series to the range the reader chose, and order the epics. Both
// are display only. views.ReportOf stays the one place a Project metric is
// defined, so nothing here recomputes a figure: a series is cut, a headline
// KPI computed over the whole project is left alone, and the server's rolling
// medians travel with the features they were computed for.

import type { Report, ReportEpic } from "../../api";

export type Range = number | "all";

// The ranges the control offers, in its order. The first is the default.
export const RANGES: { value: string; label: string }[] = [
  { value: "8", label: "Last 8 weeks" },
  { value: "26", label: "Last 26 weeks" },
  { value: "all", label: "All" },
];
export const DEFAULT_RANGE: Range = 8;

// parseRange reads ?weeks=. Anything that is not one of the offered values
// reads as the default, as an unknown ?tab= reads as Project.
export function parseRange(raw: string | null): Range {
  const hit = RANGES.find((r) => r.value === raw);
  if (!hit) return DEFAULT_RANGE;
  return hit.value === "all" ? "all" : Number(hit.value);
}

// rangeValue is the ?weeks= text for a range.
export function rangeValue(range: Range): string {
  return String(range);
}

// weeksNote is the line a weekly chart carries to say what it shows ("" when
// it shows no week, so the frame draws no note).
export function weeksNote(weeks: number): string {
  if (weeks < 1) return "";
  return weeks === 1 ? "Showing the last week" : `Showing the last ${weeks} weeks`;
}

function tail<T>(items: T[], n: number): T[] {
  return items.slice(Math.max(items.length - n, 0));
}

// sliceWeeks keeps the last `weeks` weeks of every per-week series: weeks,
// week_boundaries (one longer than weeks, so it keeps one more), burn_up,
// throughput, bugs, and the Open questions sparkline. A feature_times row is
// an event placed on the date axis, not a weekly value, so rows completed
// before the first shown week are dropped (the axis would clamp them onto its
// left edge); the rows kept carry the server's rolling medians untouched.
// Every other KPI, and the epics, describe the whole project and are left
// alone. A project with fewer weeks than asked keeps what it has.
export function sliceWeeks(report: Report, weeks: Range): Report {
  if (weeks === "all" || !(weeks >= 1) || report.weeks.length <= weeks) return report;
  const boundaries = tail(report.week_boundaries, weeks + 1);
  const from = Date.parse(boundaries[0]);
  return {
    ...report,
    weeks: tail(report.weeks, weeks),
    week_boundaries: boundaries,
    burn_up: tail(report.burn_up, weeks),
    throughput: tail(report.throughput, weeks),
    bugs: tail(report.bugs, weeks),
    feature_times: report.feature_times.filter((f) => !(Date.parse(f.completed_at) < from)),
    kpis: { ...report.kpis, open_questions_by_week: tail(report.kpis.open_questions_by_week, weeks) },
  };
}

const IN_PROGRESS = 0;
const NOT_STARTED = 1;
const PLANNED = 2;
const FINISHED = 3;

// epicClass buckets an epic by the counts the payload serves: a draft epic is
// planned; an approved epic with every feature done is finished; one with a
// feature in flight, or some done and some not, is in progress; the rest
// (nothing started, or no features yet) are not started.
function epicClass(epic: ReportEpic): number {
  if (epic.planned) return PLANNED;
  if (epic.done > 0 && epic.in_progress === 0 && epic.not_started === 0) return FINISHED;
  if (epic.in_progress > 0 || epic.done > 0) return IN_PROGRESS;
  return NOT_STARTED;
}

// sortEpics puts unfinished epics first: in progress, then not started, then
// planned (draft) ones; finished epics come back apart. Order within each
// class is the order served.
export function sortEpics(epics: ReportEpic[]): { open: ReportEpic[]; finished: ReportEpic[] } {
  const open = epics.filter((e) => epicClass(e) !== FINISHED);
  // Array.prototype.sort is stable, so the served order holds within a class.
  open.sort((a, b) => epicClass(a) - epicClass(b));
  return { open, finished: epics.filter((e) => epicClass(e) === FINISHED) };
}
