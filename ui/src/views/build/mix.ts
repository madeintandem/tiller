// composeMix turns one epic's feature_counts (views.BuildEpic, keyed by the
// feature's board column) into the StateBar mix the panel's epic row
// paints, in the board's own fixed six-column order - the identical
// LEGEND_ORDER.map technique views/home/stat.ts's composeStat already uses
// for the header tiles, so no .sort() is needed here either: the order is a
// hardcoded literal (BUILD_COLUMNS), not a computed one. Every column is
// emitted, zeros included, since StateBar itself filters n === 0 segments.

import type { AmendmentHoldView, NodeStatus } from "../../api";
import type { StateMixEntry } from "../../components/StateBar";
import { foldStatus } from "../../lib/states";
import { statusLabel } from "../../lib/statusLabel";
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
// Every status word goes through statusLabel, lower-cased the way
// taskMixCaption words them: the caption is also the rail bar's hover title,
// so "1 in review · 6 done", never "1 in_review".
export function composeMixCaption(mix: StateMixEntry[], counts?: Record<string, number>): string {
  const visible = mix.filter((s) => s.n > 0).map((s) => `${String(s.n)} ${statusLabel(s.state).toLowerCase()}`);
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
      parts.push(`+${String(n)} ${statusLabel(status).toLowerCase()}`);
    }
  }
  return parts;
}

// The task statuses in lifecycle order, the order the caption names them in
// after the done count. A fixed literal like BUILD_COLUMNS, so no sort runs.
// A live status this list does not know is still counted, after these, in
// the order the tasks arrived.
const TASK_ORDER: readonly string[] = ["proposed", "ready", "in_progress", "check", "in_review", "blocked", "draft", "specced", "active"];

// The chart states in lifecycle order, the order the bar paints its segments.
const BAR_ORDER: readonly NodeStatus[] = ["draft", "specced", "ready", "active", "in_review", "blocked", "done"];

// Cancelled tasks and amendment-held drafts do not contribute to progress.
type ProgressTask = { status: string; amendment_hold?: AmendmentHoldView };

function liveStatuses(tasks: readonly ProgressTask[]): string[] {
  return tasks.filter((t) => t.status !== "cancelled" && t.amendment_hold === undefined).map((t) => t.status);
}

// taskMix folds a feature's live tasks onto the StateBar's chart states
// (a task's proposed, in_progress, and check fold through lib/states.ts like
// every other bar), in lifecycle order, zeros included.
export function taskMix(tasks: readonly ProgressTask[]): StateMixEntry[] {
  const live = liveStatuses(tasks);
  return BAR_ORDER.map((state) => ({ state, n: live.filter((s) => foldStatus(s) === state).length }));
}

// taskMixCaption states the bar in words, so it never rests on color:
// "N of M done" first (M counts live tasks only), then each other status
// with its count in lifecycle order, lower-cased through statusLabel:
// "0 of 7 done, 7 in review".
export function taskMixCaption(tasks: readonly ProgressTask[]): string {
  const live = liveStatuses(tasks);
  if (live.length === 0) {
    return "no tasks yet";
  }
  const counts = new Map<string, number>();
  for (const status of live) {
    if (status !== "done") {
      counts.set(status, (counts.get(status) ?? 0) + 1);
    }
  }
  const known = TASK_ORDER.filter((s) => counts.has(s));
  const unknown = [...counts.keys()].filter((s) => !TASK_ORDER.includes(s));
  const parts = [...known, ...unknown].map((s) => `${String(counts.get(s))} ${statusLabel(s).toLowerCase()}`);
  const done = live.length - [...counts.values()].reduce((a, b) => a + b, 0);
  return [`${String(done)} of ${String(live.length)} done`, ...parts].join(", ");
}
