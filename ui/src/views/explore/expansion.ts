// Explore's tree expansion, lifted out of each row's own useState (task 1,
// docs/spec/ui-surface.md): a branch's openness stops
// being derived from the current selection at render time and becomes an
// explicit record of what the reader (or a selection landing inside it) has
// opened, kept until something closes it. That inversion is what lets a
// branch opened by an earlier selection outlive that selection moving
// elsewhere: nothing in this module ever closes an id on its own, only
// toggle (a chevron click) and closeAll (task 2) do.
//
// The map holds only exceptions to "closed": true means held open, false
// means held closed by hand, and an absent id defaults to closed the same
// way every row's default did before this map existed. Keyed by node id, not
// key: an id persists on disk across a rename and a key does not
// (ui/src/api.ts's TreeNode, internal/gen/tree.go).
//
// Pure and React-free by design (Task 1's brief): Explore.tsx owns the
// useState and the effect that calls into this module, so the map's own
// rules stay testable with no render involved.

export type Expansion = ReadonlyMap<string, boolean>;

export function emptyExpansion(): Expansion {
  return new Map();
}

// Absent and false read identically: both mean "not open." Only an entry
// that is exactly true counts, which is what lets closeAll's empty map and a
// branch closed by hand collapse to the same rendered state.
export function isOpen(x: Expansion, id: string): boolean {
  return x.get(id) === true;
}

// A chevron's own write: the reader's explicit choice for this one id,
// replacing whatever the map held for it. Identity-stable when the value
// already matches (task 4's fold-in from Task 1's review): returning the
// SAME map reference when nothing actually changes is what lets a caller's
// setState bail out of a re-render instead of firing on a no-op write, which
// otherwise turned every re-derived toggle call into persist-effect churn.
export function toggle(x: Expansion, id: string, open: boolean): Expansion {
  if (x.get(id) === open) {
    return x;
  }
  const next = new Map(x);
  next.set(id, open);
  return next;
}

// Materializes a selection's ancestry as opens, skipping any id already held
// false - the rule that a branch closed by hand stays closed even when the
// selection lands on it again. Every other given id is set true regardless
// of its prior state, so a selection can only ever open a branch, never
// close one.
//
// Identity-stable when nothing actually changes (task 4's fold-in from Task
// 1's review): when every id is already true, or already held false and so
// skipped, the input map comes back unchanged rather than a new-but-equal
// copy. Explore.tsx's auto-open effect re-runs on every tree refetch (its
// own dependency is tree.data, a fresh object each time even when the
// content is identical), so without this every refetch would trigger a
// pointless re-render (React only bails out of one when a setState call
// returns the exact previous reference) - and, since Explore.tsx's own
// persist effect depends on this very map's identity, a pointless render
// would also have meant a pointless localStorage write on every refetch.
// This earns its keep on the render side unconditionally; the write side
// still depends on that effect's own dependency array actually keying off
// the map alone (Explore.tsx's own persist-effect comment has the detail
// and the fix-round finding that made this distinction matter).
export function autoOpen(x: Expansion, ids: readonly string[]): Expansion {
  let next: Map<string, boolean> | null = null;
  for (const id of ids) {
    if (x.get(id) === true || x.get(id) === false) {
      continue;
    }
    if (next === null) {
      next = new Map(x);
    }
    next.set(id, true);
  }
  return next ?? x;
}

// Every given id set true, including one the reader held closed by hand: an
// explicit "open everything" command outranks a remembered close the same
// way a fresh click on that one row's own chevron would.
export function openAll(x: Expansion, ids: readonly string[]): Expansion {
  const next = new Map(x);
  for (const id of ids) {
    next.set(id, true);
  }
  return next;
}

// The empty map, not every known id set to false: an empty map means
// "nothing held open or closed," so a selection landing somewhere new after
// a close-all auto-opens its ancestry exactly as a first visit would, rather
// than staying wedged shut by a blanket false entry.
export function closeAll(): Expansion {
  return new Map();
}

// Composes a freshly-read stored map with the live session state
// materialized so far, LIVE winning any id both name (task 4's restore
// effect, Explore.tsx): whatever this session has already opened via
// auto-open or a hand click outranks a disk-stored value for that same id,
// and STORED only fills in what live does not yet know about - the same
// precedence toggle's own hand-closed rule already gives an explicit
// choice over an inferred one.
//
// Extracted to its own pure function (fix round 3, re-review) rather than
// left inline at its one call site: round 2's fix (holding auto-open until
// the restore has run) makes `live` provably empty every time that call
// site actually invokes this, so no integration test can exercise a
// genuine conflict there - a precedence rule with no test but a comment
// explaining why none can exist rots the moment a future caller constructs
// a non-empty `live` before restore runs. expansion.test.ts pins the
// precedence direction directly, independent of whether any current caller
// can reach a real conflict.
//
// Identity-stable when live is empty: returns STORED itself rather than a
// copy, since there is nothing for live to override in that case. This is
// the ordinary case at the one production call site (live is the session's
// own expansion state, which round 2's fix keeps empty until this merge
// runs), so the fast path is not a theoretical nicety.
export function mergeRestored(stored: Expansion, live: Expansion): Expansion {
  if (live.size === 0) {
    return stored;
  }
  const merged = new Map(stored);
  for (const [id, open] of live) {
    merged.set(id, open);
  }
  return merged;
}
