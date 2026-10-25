// The Explore tree's expansion map, persisted so a reload restores the
// branches the reader had open (task 4, docs/plans/2026-08-14-explore-tree-
// state.md), the same selectionStorage.ts idiom this module mirrors: a
// per-project key, because every project serves on the same default port and
// thus the same browser origin, so a flat key would leak one project's open
// branches into another project's tab.
//
// The stored value is a JSON array of the map's own [id, boolean] entries
// (Array.from(expansion.entries())), not a re-encoding into some other
// shape: reading it back is exactly `new Map(parsed)` once every entry is
// confirmed to be a real [string, boolean] pair.
//
// Normalize-on-read is TOTAL, unlike selectionStorage's per-segment filter:
// any garbage anywhere in the stored value (invalid JSON, a non-array, an
// entry that is not a two-element array, a non-string id, a non-boolean
// value) folds the whole read to null - "nothing stored" - rather than
// salvaging the entries that do parse. A map is a set of exceptions to
// "closed" (expansion.ts's own doc comment); a partially-salvaged map could
// silently drop a hand-closed entry and reopen a branch the reader meant to
// keep shut, which is worse than falling back to the auto-open default.

import type { Expansion } from "./expansion";

const EXPANSION_STORAGE_PREFIX = "tiller-explore-open";

export function expansionStorageKey(projectKey: string): string {
  return `${EXPANSION_STORAGE_PREFIX}:${projectKey}`;
}

// Returns null both when nothing has been stored yet for this project and
// when what IS stored does not parse as a real expansion map - the caller's
// cue, either way, to fall through to today's default (an empty map, opened
// only by the auto-open effect) exactly as if this project had never been
// visited.
export function readExpansion(projectKey: string): Expansion | null {
  let raw: string | null;
  try {
    raw = window.localStorage.getItem(expansionStorageKey(projectKey));
  } catch {
    return null;
  }
  if (raw === null) {
    return null;
  }
  return parseExpansion(raw);
}

// Persists the exact map under this project's own key, including the empty
// map: close-all's own empty map is a real remembered state (expansion.ts's
// own closeAll doc comment), distinct from nothing ever having been stored,
// so writing it here must not be skipped just because it has no entries.
export function writeExpansion(projectKey: string, x: Expansion): void {
  try {
    window.localStorage.setItem(expansionStorageKey(projectKey), JSON.stringify(Array.from(x.entries())));
  } catch {
    // A refused write still leaves this session working; only persistence
    // across a reload is lost.
  }
}

function parseExpansion(raw: string): Expansion | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }
  if (!Array.isArray(parsed)) {
    return null;
  }
  const entries: [string, boolean][] = [];
  for (const entry of parsed) {
    if (!Array.isArray(entry) || entry.length !== 2) {
      return null;
    }
    const [id, open] = entry as [unknown, unknown];
    if (typeof id !== "string" || typeof open !== "boolean") {
      return null;
    }
    entries.push([id, open]);
  }
  return new Map(entries);
}
