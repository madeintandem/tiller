// The pulse's Just happened column: a bounded taste of
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
