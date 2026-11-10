import type { Recap as RecapModel, RecapWindow } from "../../api";
import { formatWhen } from "../../lib/time";

// The one-liner under the head (mock:1144's own line, adapted per window):
// what this window holds, and whether looking at it just marked the reader
// caught up. Only the visit window does (ui-surface.md's horizon rule, pinned
// server-side by internal/serve/visit_test.go's
// TestRecapVisitWindowHoldsHorizonAcrossRequestsWithinSessionGap): it is the
// one request in the console that advances the last-visit mark, and the two
// fixed windows never touch it no matter how often they are read. Copying the
// mock's own sentence verbatim regardless of which window is open would put
// "reading this view marks you caught up" under a 24h view that just did no
// such thing, so this states the truth for whichever window is actually
// mounted rather than reciting the sentence written for the default one.
//
// With a filter on, shown is the filtered count and the head reads "120 of
// 1000 events"; total stays the unfiltered count.
export function histSubFor(active: RecapWindow, recap: RecapModel, shown?: number): string {
  const count =
    shown !== undefined
      ? `${shown} of ${recap.total} events`
      : `${recap.total} ${recap.total === 1 ? "event" : "events"}`;
  if (active === "visit") {
    // since is empty on a first visit, which recaps nothing; a blank "since"
    // would read as a claim about an instant nobody has.
    const since = recap.since !== "" ? ` since ${formatWhen(recap.since, new Date())}` : "";
    return `${count}${since} · reading this view marks you caught up`;
  }
  const span = active === "24h" ? "in the last 24 hours" : "in the last 7 days";
  return `${count} ${span} · fixed windows never mark you caught up`;
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
