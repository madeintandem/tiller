// The header band's stat composition (task C1): each of the four tiles
// (Epics, Features, Tasks, Questions) needs a StateBar mix plus a
// caption naming the literal wire-status breakdown ("63 draft . 6 specced .
// 1 ready"), derived from a flat list of nodes with their raw status. This is
// page-composed on purpose (StateBar.tsx's own doc comment: "the mix is
// typed NodeStatus, bars are page-composed"), and it folds through
// lib/states.ts's foldStatus rather than re-deriving the fold table, so a
// bar segment and its caption word can never disagree about which of the
// seven legend buckets a status belongs to.

import type { NodeStatus } from "../../api";
import type { StateMixEntry } from "../../components/StateBar";
import { foldStatus } from "../../lib/states";

const LEGEND_ORDER: NodeStatus[] = [
  "draft",
  "specced",
  "ready",
  "active",
  "in_review",
  "done",
  "blocked",
];

const LEGEND_RANK: Record<NodeStatus, number> = LEGEND_ORDER.reduce(
  (acc, state, i) => ({ ...acc, [state]: i }),
  {} as Record<NodeStatus, number>,
);

export interface ComposedStat {
  mix: StateMixEntry[];
  caption: string;
}

// composeStat groups rows by their literal wire status (so the caption reads
// the words that were actually observed, a question's "open"/"answered"
// included), orders those groups by the legend rank of the bucket each folds
// to, and separately sums the same rows into the seven-state mix StateBar
// paints. Both orderings and both groupings share one fold, so "open" (->
// in_review) always sits before "answered" (-> done) in the caption, the
// same order the bar's segments carry.
export function composeStat(rows: Array<{ status: string }>): ComposedStat {
  const wordCounts = new Map<string, number>();
  for (const row of rows) {
    wordCounts.set(row.status, (wordCounts.get(row.status) ?? 0) + 1);
  }

  const words = Array.from(wordCounts.keys()).sort((a, b) => {
    const rank = LEGEND_RANK[foldStatus(a)] - LEGEND_RANK[foldStatus(b)];
    return rank !== 0 ? rank : a.localeCompare(b);
  });
  const caption = words.map((word) => `${wordCounts.get(word) ?? 0} ${word}`).join(" · ");

  const bucketTotals = new Map<NodeStatus, number>();
  for (const [word, n] of wordCounts) {
    const bucket = foldStatus(word);
    bucketTotals.set(bucket, (bucketTotals.get(bucket) ?? 0) + n);
  }
  // Every legend bucket is emitted, zeros included: StateBar itself filters
  // n === 0 segments, so composeStat does not need to.
  const mix: StateMixEntry[] = LEGEND_ORDER.map((state) => ({
    state,
    n: bucketTotals.get(state) ?? 0,
  }));

  return { mix, caption };
}
