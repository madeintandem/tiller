// Text formats for the Report page: durations in calendar days, week labels,
// and the header's read time.

import { localMidnight } from "./scale";

// formatDays shows one decimal below 10 days and whole days from 10 up, so
// 9.96 reads "10 d" rather than "10.0 d". Anything under 0.05, zero included,
// reads "< 0.1 d" rather than a "0.0 d" that looks like no time at all.
export function formatDays(d: number): string {
  if (d < 0.05) return "< 0.1 d";
  const tenths = Math.round(d * 10) / 10;
  return tenths < 10 ? `${tenths.toFixed(1)} d` : `${Math.round(d)} d`;
}

// formatWeek names a week by its Monday, "Oct 5".
export function formatWeek(monday: string): string {
  return new Date(localMidnight(monday)).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

// formatAsOf is the header's "As of" time in the reader's locale.
export function formatAsOf(iso: string): string {
  return new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}
