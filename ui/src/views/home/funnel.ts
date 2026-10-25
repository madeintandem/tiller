// The header band's feature funnel and shelf-stat composition (task 8,
// settled decisions 8/9 of the home burst redesign): the funnel replaces the
// old Features/Tasks stat tiles with the six columns Build itself reads as
// shape, so composeFunnel always returns all six segments in FEATURE_FUNNEL
// order, zeros included, and folds anything outside that set (cancelled,
// archived) into one retired count rather than dropping it silently.
// composeShelfStat counts nonterminal shelf work for Fix and retains each
// noun's open count for the caption.

import { BUILD_COLUMNS } from "../build/columns";

// Derived from BUILD_COLUMNS rather than restated: the funnel is DEFINED as
// the shape Build's own six columns read (settled decision 8), so the two
// cannot be allowed to drift apart. BUILD_COLUMNS is the fixed literal the
// board owns; this is its status sequence, in column order.
export const FEATURE_FUNNEL: string[] = BUILD_COLUMNS.map((c) => c.status);

export interface FunnelSegment {
  status: string;
  n: number;
}

export interface ComposedFunnel {
  segments: FunnelSegment[];
  retired: number;
}

// composeFunnel counts each row by its Build column when the server sent one
// (a feature row's column, gen.FeatureBoardStatus: a ready feature with a
// task in flight sits in active), so the funnel and the board agree.
export function composeFunnel(rows: Array<{ status: string; column?: string }>): ComposedFunnel {
  const counts = new Map<string, number>();
  let retired = 0;
  for (const row of rows) {
    const column = row.column ?? row.status;
    if (FEATURE_FUNNEL.includes(column)) {
      counts.set(column, (counts.get(column) ?? 0) + 1);
    } else {
      retired += 1;
    }
  }
  const segments = FEATURE_FUNNEL.map((status) => ({ status, n: counts.get(status) ?? 0 }));
  return { segments, retired };
}

export interface ComposedShelfStat {
  open: number;
  bugs: number;
  chores: number;
  tweaks: number;
}

function openCount(rows: Array<{ status: string }>): number {
  return rows.filter((row) => row.status !== "done" && row.status !== "cancelled").length;
}

export function composeShelfStat(
  bugs: Array<{ status: string }>,
  chores: Array<{ status: string }>,
  tweaks: Array<{ status: string }>,
): ComposedShelfStat {
  const bugsOpen = openCount(bugs);
  const choresOpen = openCount(chores);
  const tweaksOpen = openCount(tweaks);
  return { open: bugsOpen + choresOpen + tweaksOpen, bugs: bugsOpen, chores: choresOpen, tweaks: tweaksOpen };
}
