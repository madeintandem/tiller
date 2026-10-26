// Vitest coverage for the lifted expansion map (task 1,
// docs/spec/ui-surface.md): every exported function,
// including the three rules the rest of the tree depends on verbatim -
// autoOpen skips an id held closed by hand, openAll overrides a held-closed
// entry, and closeAll returns to the empty map rather than an all-false one
// - plus the immutability every caller relies on (Explore.tsx holds the
// previous map in a setState updater and expects the input untouched).

import { describe, expect, it } from "vitest";

import {
  autoOpen,
  closeAll,
  emptyExpansion,
  isOpen,
  mergeRestored,
  openAll,
  toggle,
  type Expansion,
} from "./expansion";

describe("emptyExpansion", () => {
  it("is empty, and every id reads as closed against it", () => {
    const x = emptyExpansion();
    expect(x.size).toBe(0);
    expect(isOpen(x, "E-1")).toBe(false);
  });
});

describe("isOpen", () => {
  it("is true only when the entry is exactly true", () => {
    const x: Expansion = new Map([
      ["E-open", true],
      ["E-closed", false],
    ]);
    expect(isOpen(x, "E-open")).toBe(true);
    expect(isOpen(x, "E-closed")).toBe(false);
  });

  it("treats an absent id the same as one held false", () => {
    const x: Expansion = new Map([["E-closed", false]]);
    expect(isOpen(x, "E-missing")).toBe(false);
    expect(isOpen(x, "E-closed")).toBe(false);
  });
});

describe("toggle", () => {
  it("records an explicit open", () => {
    const x = toggle(emptyExpansion(), "E-1", true);
    expect(isOpen(x, "E-1")).toBe(true);
  });

  it("records an explicit close, overriding a prior open", () => {
    const opened = toggle(emptyExpansion(), "E-1", true);
    const closed = toggle(opened, "E-1", false);
    expect(isOpen(closed, "E-1")).toBe(false);
  });

  it("does not mutate the map it was given", () => {
    const before = emptyExpansion();
    toggle(before, "E-1", true);
    expect(before.size).toBe(0);
  });

  it("returns the same map reference when the value already matches (identity-stable)", () => {
    const opened = toggle(emptyExpansion(), "E-1", true);
    expect(toggle(opened, "E-1", true)).toBe(opened);

    const closed = toggle(emptyExpansion(), "E-1", false);
    expect(toggle(closed, "E-1", false)).toBe(closed);
  });
});

describe("autoOpen", () => {
  it("opens every id not already held false", () => {
    const x = autoOpen(emptyExpansion(), ["E-1", "F-1"]);
    expect(isOpen(x, "E-1")).toBe(true);
    expect(isOpen(x, "F-1")).toBe(true);
  });

  it("skips an id held false - the hand-closed rule", () => {
    const closed = toggle(emptyExpansion(), "E-1", false);
    const after = autoOpen(closed, ["E-1", "F-1"]);
    expect(isOpen(after, "E-1")).toBe(false);
    expect(isOpen(after, "F-1")).toBe(true);
  });

  it("leaves ids outside the given list untouched", () => {
    const seeded = toggle(emptyExpansion(), "E-other", true);
    const after = autoOpen(seeded, ["E-1"]);
    expect(isOpen(after, "E-other")).toBe(true);
    expect(isOpen(after, "E-1")).toBe(true);
  });

  it("does not mutate the map it was given", () => {
    const before = toggle(emptyExpansion(), "E-1", false);
    autoOpen(before, ["E-1", "F-1"]);
    expect(isOpen(before, "E-1")).toBe(false);
    expect(before.has("F-1")).toBe(false);
  });

  it("returns the same map reference when every id is already true-or-blocked (identity-stable)", () => {
    // E-1 explicitly held open, F-1 explicitly held closed by hand: neither
    // entry changes when autoOpen runs over the same two ids again, so the
    // input map itself comes back rather than an equal-but-new copy - the
    // guard that keeps a tree-refetch's fresh ids array from firing a
    // persist effect keyed on the expansion map's own identity.
    const x = toggle(toggle(emptyExpansion(), "E-1", true), "F-1", false);
    expect(autoOpen(x, ["E-1", "F-1"])).toBe(x);
  });

  it("still returns a new map when at least one given id actually changes", () => {
    const x = toggle(emptyExpansion(), "E-1", true);
    const after = autoOpen(x, ["E-1", "F-2"]);
    expect(after).not.toBe(x);
    expect(isOpen(after, "F-2")).toBe(true);
  });
});

describe("openAll", () => {
  it("opens every given id, including one held false", () => {
    const closed = toggle(emptyExpansion(), "E-1", false);
    const after = openAll(closed, ["E-1", "F-1"]);
    expect(isOpen(after, "E-1")).toBe(true);
    expect(isOpen(after, "F-1")).toBe(true);
  });

  it("leaves ids outside the given list untouched", () => {
    const seeded = toggle(emptyExpansion(), "E-other", false);
    const after = openAll(seeded, ["E-1"]);
    expect(isOpen(after, "E-other")).toBe(false);
  });

  it("does not mutate the map it was given", () => {
    const before = toggle(emptyExpansion(), "E-1", false);
    openAll(before, ["E-1"]);
    expect(isOpen(before, "E-1")).toBe(false);
  });
});

describe("closeAll", () => {
  it("returns the empty map, not every known id set to false", () => {
    const x = closeAll();
    expect(x.size).toBe(0);
  });

  it("lets a future selection auto-open normally (not wedged shut by a false entry)", () => {
    const after = autoOpen(closeAll(), ["E-1"]);
    expect(isOpen(after, "E-1")).toBe(true);
  });
});

// mergeRestored (task 4's restore effect, Explore.tsx): extracted to its own
// pure function later specifically so this
// precedence rule is pinned by a unit test directly, at a grain where
// reachability is irrelevant - the settled gate (holding auto-open until the
// restore has run) makes `live` provably empty at the one production call
// site's own merge time, so no integration test could ever exercise a real
// conflict there. A precedence rule guarded only by a comment about current
// reachability rots the day a future caller constructs a non-empty `live`
// before restore runs; these tests hold regardless of who calls it or when.
describe("mergeRestored", () => {
  it("fills in an id only stored knows about", () => {
    const stored: Expansion = new Map([["E-1", true]]);
    const merged = mergeRestored(stored, emptyExpansion());
    expect(isOpen(merged, "E-1")).toBe(true);
  });

  it("keeps an id only live knows about", () => {
    const live: Expansion = new Map([["E-1", true]]);
    const merged = mergeRestored(emptyExpansion(), live);
    expect(isOpen(merged, "E-1")).toBe(true);
  });

  it("a conflicting id resolves to live's value: stored true, live false stays false", () => {
    const stored: Expansion = new Map([["E-1", true]]);
    const live: Expansion = new Map([["E-1", false]]);
    const merged = mergeRestored(stored, live);
    expect(isOpen(merged, "E-1")).toBe(false);
  });

  it("a conflicting id resolves to live's value: stored false, live true stays true", () => {
    const stored: Expansion = new Map([["E-1", false]]);
    const live: Expansion = new Map([["E-1", true]]);
    const merged = mergeRestored(stored, live);
    expect(isOpen(merged, "E-1")).toBe(true);
  });

  it("returns the stored map itself when live is empty (identity-stable)", () => {
    const stored: Expansion = new Map([["E-1", true]]);
    expect(mergeRestored(stored, emptyExpansion())).toBe(stored);
  });

  it("does not mutate either input", () => {
    const stored: Expansion = new Map([["E-1", true]]);
    const live: Expansion = new Map([["E-1", false], ["F-1", true]]);
    mergeRestored(stored, live);
    expect(stored.get("E-1")).toBe(true);
    expect(live.get("E-1")).toBe(false);
    expect(live.get("F-1")).toBe(true);
  });
});
