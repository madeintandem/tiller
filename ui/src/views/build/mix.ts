// composeMix turns one epic's feature_counts (views.BuildEpic, keyed by the
// feature's board column) into the StateBar mix the panel's epic row
// paints, in the board's own fixed six-column order - the identical
// LEGEND_ORDER.map technique views/home/stat.ts's composeStat already uses
// for the header tiles, so no .sort() is needed here either: the order is a
// hardcoded literal (BUILD_COLUMNS), not a computed one. Every column is
// emitted, zeros included, since StateBar itself filters n === 0 segments.

import type { NodeStatus } from "../../api";
import type { StateMixEntry } from "../../components/StateBar";
import { BUILD_COLUMNS, isColumnStatus } from "./columns";

export function composeMix(counts: Record<string, number> | undefined): StateMixEntry[] {
  return BUILD_COLUMNS.map((col) => ({
    // Every BUILD_COLUMNS status is one of the six literal feature states,
    // which is a subset of NodeStatus by construction (columns.ts's own
    // comment); the cast names that fact rather than re-deriving it.
    state: col.status as NodeStatus,
    n: counts?.[col.status] ?? 0,
  }));
}

// composeMixCaption is the epic row's mix bar accessible name (StateBar's
// own caption prop, its one non-color-alone requirement): the literal
// status words and their counts, in the same fixed column order the bar
// itself paints in, so the announced text and the visible bar can never
// disagree about ordering. counts is the SAME feature_counts composeMix
// above read; passing it here too is what lets the caption name a
// "cancelled" (or any future) status the six columns exclude, as a
// trailing "+N status" clause, rather than silently under-reporting the
// epic while the visible segments' own sum agrees with the board's actual
// card count (review finding 1: the mix already only ever read the six
// known keys, so it never rendered a phantom column - it just had nothing
// to say about what those six keys leave out).
export function composeMixCaption(mix: StateMixEntry[], counts?: Record<string, number>): string {
  const visible = mix.filter((s) => s.n > 0).map((s) => `${String(s.n)} ${s.state}`);
  const parts = [...visible, ...excludedParts(counts)];
  return parts.length === 0 ? "no features yet" : parts.join(" · ");
}

// excludedParts reads feature_counts for any key BUILD_COLUMNS does not
// cover ("cancelled" is the one that exists in the store's vocabulary
// today; written generically over whatever key the wire actually sends
// rather than hardcoding that one word, so an unrecognized future status
// still gets named instead of silently vanishing from the count). Object.
// entries here walks the object in the order JSON.parse gave it, which is
// alphabetical because Go's encoding/json sorts map keys on the wire
// (Drift.tsx's own comment on the identical fact) - not a client sort, no
// comparator, just reading the order already on the wire.
function excludedParts(counts: Record<string, number> | undefined): string[] {
  if (counts === undefined) {
    return [];
  }
  const parts: string[] = [];
  for (const [status, n] of Object.entries(counts)) {
    if (n > 0 && !isColumnStatus(status)) {
      parts.push(`+${String(n)} ${status}`);
    }
  }
  return parts;
}
