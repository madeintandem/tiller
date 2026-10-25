// Vitest coverage for the Explore split's width persistence (task 4,
// docs/plans/2026-08-14-explore-tree-state.md), mirroring panelStorage.test.ts's
// own read/write/round-trip template for the sibling flat-key module.

import { afterEach, describe, expect, it } from "vitest";

import { NAV_WIDTH_DEFAULT, NAV_WIDTH_MAX, NAV_WIDTH_MIN } from "./splitSize";
import { readNavWidth, SPLIT_STORAGE_KEY, writeNavWidth } from "./splitStorage";

afterEach(() => {
  window.localStorage.clear();
});

describe("readNavWidth / writeNavWidth", () => {
  it("round-trips a written width", () => {
    writeNavWidth(420);
    expect(readNavWidth()).toBe(420);
  });

  it("defaults to NAV_WIDTH_DEFAULT when nothing has been stored yet", () => {
    expect(readNavWidth()).toBe(NAV_WIDTH_DEFAULT);
  });

  it("writes under the house-style flat key, not a per-project one", () => {
    writeNavWidth(400);
    expect(window.localStorage.getItem(SPLIT_STORAGE_KEY)).toBe("400");
  });

  it("restores a stored value through the clamp: 9999 restores as the 560 ceiling", () => {
    window.localStorage.setItem(SPLIT_STORAGE_KEY, "9999");
    expect(readNavWidth()).toBe(NAV_WIDTH_MAX);
  });

  it("restores a too-small stored value through the clamp floor", () => {
    window.localStorage.setItem(SPLIT_STORAGE_KEY, "10");
    expect(readNavWidth()).toBe(NAV_WIDTH_MIN);
  });

  it("folds a non-numeric stored value to the default", () => {
    window.localStorage.setItem(SPLIT_STORAGE_KEY, "not-a-number");
    expect(readNavWidth()).toBe(NAV_WIDTH_DEFAULT);
  });
});
