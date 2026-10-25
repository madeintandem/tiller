// Vitest coverage for the resizable split's clamp (task 3,
// docs/plans/2026-08-14-explore-tree-state.md): the one pure rule the
// divider's drag, keyboard, and double-click handlers all fold every
// candidate width through before it ever reaches state, and the one rule
// Task 4's persistence restore folds a stored value through too. Total on
// garbage input by design (a corrupted localStorage value, an off-window
// pointer event, NaN arithmetic) - there is no candidate width this
// function can be handed that does not resolve to a number in
// [NAV_WIDTH_MIN, NAV_WIDTH_MAX].
//
// Three bands, not two (the plan's own ruling): NaN, +/-Infinity, and any
// negative number are not a real width at all - a negative offset means the
// pointer math (or a corrupted stored value) produced nonsense, not "just
// past the left edge" - so all three fold to NAV_WIDTH_DEFAULT. A finite,
// non-negative width below the minimum is a real width that is merely too
// small, so IT floors at NAV_WIDTH_MIN instead.

import { describe, expect, it } from "vitest";

import { clampNavWidth, NAV_WIDTH_DEFAULT, NAV_WIDTH_MAX, NAV_WIDTH_MIN } from "./splitSize";

describe("clampNavWidth: the bounds", () => {
  it("passes a width already inside the bounds through unchanged", () => {
    expect(clampNavWidth(300)).toBe(300);
    expect(clampNavWidth(NAV_WIDTH_MIN)).toBe(NAV_WIDTH_MIN);
    expect(clampNavWidth(NAV_WIDTH_MAX)).toBe(NAV_WIDTH_MAX);
  });

  it("floors a finite, non-negative width below the minimum at 200", () => {
    expect(clampNavWidth(199)).toBe(NAV_WIDTH_MIN);
    expect(clampNavWidth(0)).toBe(NAV_WIDTH_MIN);
  });

  it("ceilings above the maximum at 560", () => {
    expect(clampNavWidth(561)).toBe(NAV_WIDTH_MAX);
    expect(clampNavWidth(10000)).toBe(NAV_WIDTH_MAX);
  });
});

describe("clampNavWidth: garbage input folds to the default", () => {
  it("folds NaN to 300", () => {
    expect(clampNavWidth(NaN)).toBe(NAV_WIDTH_DEFAULT);
  });

  it("folds positive and negative Infinity to 300", () => {
    expect(clampNavWidth(Infinity)).toBe(NAV_WIDTH_DEFAULT);
    expect(clampNavWidth(-Infinity)).toBe(NAV_WIDTH_DEFAULT);
  });

  it("folds any negative width to 300, not to the minimum", () => {
    // A negative width is not a real too-small offset, it is nonsense (a
    // pointer math error, a corrupted stored value) - the plan's own
    // acceptance line names this explicitly: NaN/Infinity/negative all fold
    // to the default alike, distinct from a real small-but-valid width
    // (0 or 199 above), which floors at the minimum instead.
    expect(clampNavWidth(-50)).toBe(NAV_WIDTH_DEFAULT);
    expect(clampNavWidth(-1)).toBe(NAV_WIDTH_DEFAULT);
  });
});

describe("the exported bounds", () => {
  it("carry the exact locked numbers", () => {
    expect(NAV_WIDTH_MIN).toBe(200);
    expect(NAV_WIDTH_MAX).toBe(560);
    expect(NAV_WIDTH_DEFAULT).toBe(300);
  });
});
