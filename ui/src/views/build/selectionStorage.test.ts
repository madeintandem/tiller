// Vitest coverage for the Build board's epic-selection persistence (page
// reload restores the epics the developer was focused on), mirroring
// panelStorage.test.ts's own read/write/round-trip template for the sibling
// storage module.

import { afterEach, describe, expect, it } from "vitest";

import { readSelectionKeys, selectionStorageKey, writeSelectionKeys } from "./selectionStorage";

afterEach(() => {
  window.localStorage.clear();
});

describe("readSelectionKeys / writeSelectionKeys", () => {
  it("round-trips a written selection", () => {
    writeSelectionKeys("acme", ["intake", "form-engine"]);
    expect(readSelectionKeys("acme")).toEqual(["intake", "form-engine"]);
  });

  it("returns null when nothing has been stored yet for this project", () => {
    expect(readSelectionKeys("acme")).toBeNull();
  });

  it("an explicit empty selection round-trips as an empty array, distinct from no entry at all", () => {
    writeSelectionKeys("acme", []);
    expect(readSelectionKeys("acme")).toEqual([]);
    // Not the same thing as never having stored anything:
    expect(readSelectionKeys("acme")).not.toBeNull();
  });

  it("normalizes a garbage stored value the same way selectedEpicKeys parses a malformed URL param", () => {
    window.localStorage.setItem(selectionStorageKey("acme"), "a,,b,");
    expect(readSelectionKeys("acme")).toEqual(["a", "b"]);
  });

  it("writes under a key namespaced by project, not a flat shared key", () => {
    writeSelectionKeys("acme", ["intake"]);
    expect(window.localStorage.getItem(selectionStorageKey("acme"))).toBe("intake");
    expect(window.localStorage.getItem("tiller-build-epics")).toBeNull();
  });

  it("keeps one project's stored selection from bleeding into another's", () => {
    writeSelectionKeys("acme", ["intake", "form-engine"]);
    writeSelectionKeys("lantern", ["ios-shell"]);

    expect(readSelectionKeys("acme")).toEqual(["intake", "form-engine"]);
    expect(readSelectionKeys("lantern")).toEqual(["ios-shell"]);

    // A third project that never wrote anything sees neither.
    expect(readSelectionKeys("other-project")).toBeNull();
  });
});
