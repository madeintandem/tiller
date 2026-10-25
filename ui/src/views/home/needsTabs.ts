// The desk's scale-aware tabs (JC, 2026-09-01). Past ROW_CAP decisions, and
// with more than one kind on the queue, the needs-you body shows one kind at
// a time behind a rail that names every kind with its count: the page is as
// tall as one kind, the counts keep every kind on the queue in view, and
// the members are one click away rather than a page-scroll away. At the cap
// or under, or with a single kind, there is no rail and the body is what it
// was, so a small desk stays the settled grid pixel for pixel. The active
// tab is the kind the URL names when that kind is on the queue, and the
// first (highest-gravity) kind otherwise, the same fallback NodeTabs applies
// to a stale `?tab=`.
import { ROW_CAP } from "../../lib/disclosure";
import type { BandRun } from "./bands";

export interface NeedsTabs {
  // The kinds on the rail in run order; empty when there is no rail.
  kinds: string[];
  // The kind whose members render; null when there is no rail.
  active: string | null;
}

export function runKind(run: BandRun): string {
  return run.type === "band" ? run.group.kind : run.kind;
}

export function runCount(run: BandRun): number {
  return run.type === "band" ? run.group.entries.length : run.entries.length;
}

export function needsTabs(runs: BandRun[], total: number, wanted: string | null): NeedsTabs {
  if (total <= ROW_CAP || runs.length < 2) {
    return { kinds: [], active: null };
  }
  const kinds = runs.map(runKind);
  const active = wanted !== null && kinds.includes(wanted) ? wanted : kinds[0];
  return { kinds, active };
}
