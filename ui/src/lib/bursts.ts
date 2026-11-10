// Burst collapsing, the recap view's answer to a wall of identical rows.
//
// The measurement that motivates it, taken from the live Acme tree's 7d
// window: 167 events, one owner, two minutes, and four bursts. 10 add.epic and
// 64 add.feature at 21:37, 32 add.question at 21:37, 61 link.feature at 21:42.
// A row-per-event rail therefore printed the event type, the timestamp and the
// owner 167 times each to carry 167 subjects. Those three columns are not
// noise that happens to repeat, they are the burst's own identity, so
// collapsing a run promotes them from repetition into the row's subject rather
// than hiding anything. What is collapsed stays reachable: the view expands a
// burst in place, DecisionCard's grammar.
//
// Two rules keep the collapse honest, and both were chosen against the data
// rather than tuned.
//
// CONSECUTIVE, not "all matching". A run is broken by any intervening event of
// a different type, so a lane whose activity genuinely interleaves keeps its
// interleaving instead of being re-sorted into piles. This is only sound
// because the payload is one newest-first timeline (internal/views/recap.go),
// which is what the same pass fixed there.
//
// NO TIME TOLERANCE. A tolerance constant would be a magic number with no
// measurement behind it, and the payload is already time-ordered, so adjacency
// in the list IS adjacency in time. What replaces it is a display rule that
// cannot lie: burstWhen states one timestamp when the run's ends render
// identically at the view's own minute precision, and "earliest to latest"
// when they do not. The precision decides, not a constant.

import type { RecapEntry } from "../api";
import { eventPhrase } from "./eventPhrase";
import { eventTier } from "./eventTier";

// A run of consecutive entries sharing one (type, contributor). The actor is in the key
// even though a group is already one (epic, stream) cell and its entries
// therefore share a contributor by construction: the key states the rule the
// grammar depends on rather than leaning on a coincidence upstream.
export interface Burst {
  type: string;
  contributor: string;
  entries: RecapEntry[];
  // How many of the run's entries created an ask for the reader. The one thing
  // on this page that needs the human, so it is the one thing rendered loud.
  asks: number;
}

export function burstsOf(entries: RecapEntry[]): Burst[] {
  const out: Burst[] = [];
  for (const entry of entries) {
    const open = out.length > 0 ? out[out.length - 1] : undefined;
    if (open !== undefined && open.type === entry.type && open.contributor === entry.contributor) {
      open.entries.push(entry);
      if (entry.ask) {
        open.asks += 1;
      }
      continue;
    }
    out.push({
      type: entry.type,
      contributor: entry.contributor,
      entries: [entry],
      asks: entry.ask ? 1 : 0,
    });
  }
  return out;
}

// A maximal run of adjacent bookkeeping bursts, folded into one row by the
// milestones-by-default view. count is EVENTS (the sum of the bursts' sizes),
// verbs the participles seen, deduplicated in first-seen order.
export interface BookkeepingRun<T = Burst> {
  kind: "bookkeeping";
  // The folded items: plain Bursts, or whatever wrapper the caller tiered
  // (flat mode tiers a burst plus its origin).
  bursts: T[];
  count: number;
  verbs: string[];
}

// tierBursts folds each maximal run of adjacent bookkeeping bursts (see
// eventTier.ts) into one BookkeepingRun. A burst holding any ask is a
// milestone burst and ends a run. showAll returns the input unchanged. The
// caller passes one day group's bursts at a time, so a run never spans a day
// heading.
//
// A caller whose items carry more than a burst (flat mode's burst plus its
// origin) passes burstOf to read the burst out of an item, and runKey to name
// the recap group an item came from: a run also ends where the key changes, so
// a fold never spans an epic. Without them items are plain bursts sharing one
// key.
export interface TierOptions<T> {
  burstOf: (item: T) => Burst;
  runKey?: (item: T) => string | number;
}

export function tierBursts(bursts: Burst[], showAll: boolean): (Burst | BookkeepingRun)[];
export function tierBursts<T>(
  items: T[],
  showAll: boolean,
  options: TierOptions<T>,
): (T | BookkeepingRun<T>)[];
export function tierBursts<T>(
  items: T[],
  showAll: boolean,
  options?: TierOptions<T>,
): (T | BookkeepingRun<T>)[] {
  if (showAll) {
    return items;
  }
  const burstOf = options?.burstOf ?? ((item: T) => item as unknown as Burst);
  const out: (T | BookkeepingRun<T>)[] = [];
  let open: BookkeepingRun<T> | undefined;
  let openKey: string | number | undefined;
  for (const item of items) {
    const burst = burstOf(item);
    if (eventTier(burst.type, burst.asks > 0) === "milestone") {
      open = undefined;
      out.push(item);
      continue;
    }
    const key = options?.runKey?.(item);
    if (open !== undefined && key !== openKey) {
      open = undefined;
    }
    if (open === undefined) {
      open = { kind: "bookkeeping", bursts: [], count: 0, verbs: [] };
      openKey = key;
      out.push(open);
    }
    open.bursts.push(item);
    open.count += burst.entries.length;
    const verb = eventPhrase(burst.type, 1).verb;
    if (!open.verbs.includes(verb)) {
      open.verbs.push(verb);
    }
  }
  return out;
}

// burstWhen is the run's time, stated at the view's own precision. One stamp
// when the ends render identically there, "earliest to latest" when they do
// not, so the row can never claim a span happened at an instant. Verified on
// the live tree: add.feature spans 21:37:02 to 21:37:04 and reads as one
// stamp, while a week of done.task in one lane would read "Jul 20, 9:12 AM to
// Jul 25, 4:40 PM", which is the correct thing to say about it.
//
// entries arrive newest first, so the head is the latest and the tail the
// earliest. format is injected because it is the caller's display precision
// that decides, and the two have to be the same function or the rule is a lie.
export function burstWhen(entries: RecapEntry[], format: (iso: string) => string): string {
  if (entries.length === 0) {
    return "";
  }
  const latest = format(entries[0].at);
  const earliest = format(entries[entries.length - 1].at);
  return latest === earliest ? latest : `${earliest} to ${latest}`;
}

// leafKey drops the owning group's key prefix from a node's key, so a row
// under the form-engine head reads "audit-overlay-rendering" rather than
// "form-engine/audit-overlay-rendering". This is the mock's own decision at
// mocks.html:304, where a decision row prints the leaf and lets its container
// carry the path. A key that IS the group's key (the epic's own add.epic row)
// collapses to nothing rather than repeating the head one line below it.
export function leafKey(key: string, groupKey: string): string {
  if (key === "" || key === groupKey) {
    return "";
  }
  if (groupKey !== "" && key.startsWith(`${groupKey}/`)) {
    return key.slice(groupKey.length + 1);
  }
  return key;
}
