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

// formatTokens abbreviates a token count to about three significant figures:
// "6.03B", "186M", "8.9M", "412K", "950".
export function formatTokens(n: number): string {
  if (n >= 1e9) return `${(n / 1e9).toFixed(2)}B`;
  if (n >= 1e6) {
    const m = n / 1e6;
    if (m >= 999.5) return `${(n / 1e9).toFixed(2)}B`;
    return m >= 99.5 ? `${Math.round(m)}M` : `${m.toFixed(1)}M`;
  }
  if (n >= 1e3) {
    const k = Math.round(n / 1e3);
    return k >= 1000 ? "1.0M" : `${k}K`;
  }
  return String(Math.round(n));
}

// formatPercent is part of whole to one decimal, "94.9%"; "0%" of nothing.
export function formatPercent(part: number, whole: number): string {
  return whole > 0 ? `${((part / whole) * 100).toFixed(1)}%` : "0%";
}

// formatAsOf is the header's "As of" time in the reader's locale.
export function formatAsOf(iso: string): string {
  return new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}
