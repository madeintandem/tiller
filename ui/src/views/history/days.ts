import type { Burst } from "../../lib/bursts";
import { formatDayHeading } from "../../lib/time";

// A burst's OWN timestamp for bucketing/sorting purposes: entries[0] - the
// newest end, since a burst keeps its entries in the group's own
// newest-first order (lib/bursts.ts). Shared by both modes' own composition
// over dayGroupsOf below, so "which instant does a burst row itself live at"
// is answered in exactly one place.
export function burstAt(burst: Burst): string {
  return burst.entries.length > 0 ? burst.entries[0].at : "";
}

// One (calendar day, its own items) bucket, in the order items arrived
// (newest day first, since the payload is one newest-first timeline). Generic
// over T rather than fixed to Burst: epic mode buckets plain Bursts, flat
// mode buckets FlatBursts (a burst plus the origin it merged in from), and
// the bucketing walk itself does not care which - only atOf, supplied by the
// caller, does.
export interface DayGroup<T> {
  key: string;
  label: string;
  bursts: T[];
}

// Buckets an already-collapsed, already-ordered item list by the LOCAL
// calendar day atOf reports for each item - not a per-entry split: an item
// that spans several days still reads as ONE row (its clock is its latest
// event) and lives under the day its own atOf instant fell on, exactly where a reader
// scanning newest-first would look for it first. Because the source list
// is already newest-first and a day boundary can only be crossed walking
// down it, never re-crossed, one linear walk buckets the whole list; no
// re-sorting - which holds for flat mode's own call site too, since
// flatBurstsOf below does its own sorting BEFORE this function ever runs,
// not the other way around.
//
// Both callers give every bucket its day heading, a one-day group included:
// rows carry the clock only, so the date has to live somewhere
// (RecapGroupBlock and FlatBody). This function only ever answers "how many
// distinct calendar days did these items touch, and which fell on which".
export function dayGroupsOf<T>(items: T[], atOf: (item: T) => string): DayGroup<T>[] {
  const out: DayGroup<T>[] = [];
  for (const item of items) {
    const at = atOf(item);
    const key = dayKey(at);
    const open = out.length > 0 ? out[out.length - 1] : undefined;
    if (open !== undefined && open.key === key) {
      open.bursts.push(item);
      continue;
    }
    out.push({ key, label: dayLabel(at), bursts: [item] });
  }
  return out;
}

// A bucketing key, local calendar date only (year-month-day), so two
// timestamps on the same local day always fold together regardless of their
// time-of-day. An unparseable value gets its own key rather than colliding
// with a real day's bucket (mirrors formatTime's own as-is fallback).
function dayKey(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return iso;
  }
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

// The eyebrow text a day heading reads: "Monday · Nov 9", with the year added
// for a day outside the current year. There is no "Today" special case: that
// reading depends on the wall clock rather than the entries themselves.
function dayLabel(iso: string): string {
  return formatDayHeading(iso, new Date());
}
