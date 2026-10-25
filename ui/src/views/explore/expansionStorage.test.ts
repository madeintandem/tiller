// Vitest coverage for the Explore tree's expansion persistence (task 4,
// docs/plans/2026-08-14-explore-tree-state.md), mirroring
// selectionStorage.test.ts's own read/write/round-trip template for the
// sibling per-project storage module.

import { afterEach, describe, expect, it } from "vitest";

import { emptyExpansion, toggle, type Expansion } from "./expansion";
import { expansionStorageKey, readExpansion, writeExpansion } from "./expansionStorage";

afterEach(() => {
  window.localStorage.clear();
});

function mapOf(entries: [string, boolean][]): Expansion {
  return new Map(entries);
}

describe("readExpansion / writeExpansion", () => {
  it("round-trips a written map, entries and all", () => {
    const x = mapOf([
      ["E-1", true],
      ["F-1", false],
    ]);
    writeExpansion("acme", x);
    expect(readExpansion("acme")).toEqual(x);
  });

  it("returns null when nothing has been stored yet for this project", () => {
    expect(readExpansion("acme")).toBeNull();
  });

  it("round-trips the empty map as a real (empty) map, distinct from nothing stored", () => {
    writeExpansion("acme", emptyExpansion());
    const read = readExpansion("acme");
    expect(read).not.toBeNull();
    expect(read?.size).toBe(0);
  });

  it("writes under a key namespaced by project, not a flat shared key", () => {
    writeExpansion("acme", mapOf([["E-1", true]]));
    expect(window.localStorage.getItem(expansionStorageKey("acme"))).not.toBeNull();
    expect(window.localStorage.getItem("tiller-explore-open")).toBeNull();
  });

  it("keeps one project's stored expansion from bleeding into another's", () => {
    writeExpansion("acme", mapOf([["E-1", true]]));
    writeExpansion("lantern", mapOf([["E-lantern", false]]));

    expect(readExpansion("acme")).toEqual(mapOf([["E-1", true]]));
    expect(readExpansion("lantern")).toEqual(mapOf([["E-lantern", false]]));
    // A third project that never wrote anything sees neither.
    expect(readExpansion("other-project")).toBeNull();
  });

  it("persists a hand-closed entry the same as a hand-opened one", () => {
    const closed = toggle(emptyExpansion(), "E-1", false);
    writeExpansion("acme", closed);
    expect(readExpansion("acme")).toEqual(closed);
  });
});

describe("readExpansion: total normalize on garbage", () => {
  it("folds invalid JSON to null", () => {
    window.localStorage.setItem(expansionStorageKey("acme"), "not json{");
    expect(readExpansion("acme")).toBeNull();
  });

  it("folds a JSON value that is not an array to null", () => {
    window.localStorage.setItem(expansionStorageKey("acme"), JSON.stringify({ E1: true }));
    expect(readExpansion("acme")).toBeNull();
  });

  it("folds an entry that is not a two-element array to null, the whole read", () => {
    window.localStorage.setItem(
      expansionStorageKey("acme"),
      JSON.stringify([["E-1", true], ["F-1"]]),
    );
    expect(readExpansion("acme")).toBeNull();
  });

  it("folds a non-string id to null, the whole read", () => {
    window.localStorage.setItem(expansionStorageKey("acme"), JSON.stringify([[1, true]]));
    expect(readExpansion("acme")).toBeNull();
  });

  it("folds a non-boolean value to null, the whole read", () => {
    window.localStorage.setItem(expansionStorageKey("acme"), JSON.stringify([["E-1", "open"]]));
    expect(readExpansion("acme")).toBeNull();
  });

  it("does not salvage the entries that do parse when one entry is malformed", () => {
    // Total normalize, not per-entry filtering: one bad entry folds the
    // whole read to null rather than keeping E-1 and dropping the bad one.
    window.localStorage.setItem(
      expansionStorageKey("acme"),
      JSON.stringify([["E-1", true], ["F-1", "not a boolean"]]),
    );
    expect(readExpansion("acme")).toBeNull();
  });
});
