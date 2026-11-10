import type { Recap as RecapModel, RecapWindow } from "../../api";

// The one-liner under the head: what this window holds. The visit window reads
// "1000 events since <time>" (History.tsx renders the since time through
// <Time>, so it shares the console's formatter and carries the absolute time
// on hover) beside the `Mark caught up` button; looking at the page never
// moves the horizon, only the press does (history/CatchUp.tsx). A fixed window
// is not framed by the horizon at all, so it names itself instead.
//
// With a filter on, shown is the filtered count and the head reads "120 of
// 1000 events"; total stays the unfiltered count.
export function countFor(recap: RecapModel, shown?: number): string {
  return shown !== undefined
    ? `${shown} of ${recap.total} events`
    : `${recap.total} ${recap.total === 1 ? "event" : "events"}`;
}

// The fixed windows' line. The visit window composes its own in History.tsx.
export function fixedSubFor(active: RecapWindow, recap: RecapModel, shown?: number): string {
  const span = active === "24h" ? "in the last 24 hours" : "in the last 7 days";
  return `${countFor(recap, shown)} ${span}`;
}

// The empty text does not repeat the horizon: the hist-sub line above already
// states that instant, and saying the same fact twice sixty pixels apart is
// the defect the original recap rewrite was about, at the smallest scale it
// has. Keeping the sentence bare also keeps it verbatim identical to Home's
// own recap signal, so the two surfaces stay one sentence in one voice.
export function emptyTextFor(window: string): string {
  switch (window) {
    case "24h":
      return "Nothing recorded in the last 24 hours.";
    case "7d":
      return "Nothing recorded in the last 7 days.";
    default:
      return "Nothing new since your last visit.";
  }
}
