// The pulse's Just happened column (task-10-brief.md): a bounded taste of
// fetchRecap("24h")'s own groups, rendered as "the same collapsed-burst rows
// History draws" (ui-surface.md's own words for this section). Bursting runs
// PER GROUP, over each group's own already-newest-first entries
// (internal/views/recap.go), before the resulting Burst lists are
// concatenated in the API's own group order (alphabetical by epic key,
// project group last - History.tsx's own doc comment) and capped. This is
// deliberate, not an oversight: flattening the raw entries first and bursting
// the merged list would require re-sorting across groups by timestamp to get
// a coherent newest-first timeline back, which is exactly the client-side
// ordering this codebase's cross-cutting rule forbids ("any ordering shown to
// the user comes from the server's shared computation, never a client
// sort") - and it would risk COLLAPSING two unrelated bursts from different
// epics that only happen to land adjacent after a merge, which per-group
// bursting can never do. The one thing given up is a strictly global
// timeline: the column reads epic-group by epic-group rather than event by
// event, which is the same trade the API's own group ordering already makes.

import type { Recap, TargetRef } from "../../api";
import type { Burst } from "../../lib/bursts";
import { burstsOf } from "../../lib/bursts";
import { has } from "../../lib/has";

export interface JustHappened {
  // The bursts to render, already capped.
  bursts: Burst[];
  // How many events the window truly held beyond what the shown bursts
  // represent: recap.total is the API's own true pre-cap count (Recap's own
  // doc comment), so this never has to guess at what a capped or omitted
  // group is hiding.
  moreCount: number;
}

export function composeJustHappened(recap: Recap, cap: number): JustHappened {
  const allBursts = recap.groups.flatMap((g) => burstsOf(g.entries));
  const bursts = allBursts.slice(0, cap);
  const shownEntryCount = bursts.reduce((n, b) => n + b.entries.length, 0);
  return { bursts, moreCount: Math.max(0, recap.total - shownEntryCount) };
}

// targetLabel picks a burst target's own display text: the title where the
// event recorded one, else the key, else the ref (a dangling edge's bare
// id). History.tsx's own BurstRow applies the identical fallback ladder to
// the identical TargetRef shape; it is duplicated here rather than imported
// because this task's bounds keep History.tsx off limits ("No History/
// Explore/Build/Drift view changes"), and a three-line pure function is a
// smaller cost than a cross-view import for one helper.
export function targetLabel(t: TargetRef): string {
  if (has(t.title)) {
    return t.title;
  }
  return has(t.key) ? t.key : t.ref;
}

// formatClock is Just happened's own time voice: a bare clock reading ("2:
// 10p"), never a date, since the column's own window is fixed at 24h and a
// date would be redundant on every row. It reuses burstWhen's own
// ends-match-or-span rule (lib/bursts.ts) by being handed to it as the
// format function, the same way History.tsx's formatTime is; the two format
// functions differ only in precision (History spans days, this column
// never does), so the RULE stays shared even though the two views render
// different text.
//
// toLocaleTimeString's AM/PM marker is locale-dependent (a 24-hour locale
// renders none), so the trailing single-letter "a"/"p" is a best-effort
// compaction for the common case rather than a guarantee; an unparseable
// timestamp renders as-is; matches formatTime's own precedent of showing
// the raw field over hiding it.
export function formatClock(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return iso;
  }
  const s = d.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  return s.replace(/\s+/g, "").replace(/([ap])m$/i, "$1").toLowerCase();
}
