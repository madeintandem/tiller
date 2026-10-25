// The resizable split's one clamp (task 3, docs/plans/2026-08-14-explore-
// tree-state.md): every candidate navigator width, wherever it comes from -
// a mouse drag's pointer offset, a keyboard nudge, a double-click reset, or
// (Task 4) a value pulled back out of localStorage - folds through here
// before it ever reaches state, so `navWidth` in Explore.tsx is always a
// finite number inside [NAV_WIDTH_MIN, NAV_WIDTH_MAX] no matter what
// produced the candidate. Pure and React-free by design, the same posture
// expansion.ts (task 1) took for the same reason: the rule is testable with
// no render involved, and every caller (a pointermove handler, a keydown
// handler, a storage reader) shares the identical one.
//
// Two failure bands, not one: NaN, +/-Infinity, and any negative number are
// not a real width at all - they mean the arithmetic that produced the
// candidate broke (or a stored value was hand-edited into nonsense), not
// "the pointer is just past the left edge" - so all three fold straight to
// NAV_WIDTH_DEFAULT. A finite, non-negative width that is merely too small
// or too large is a real width that only needs floor/ceiling, so it clamps
// to NAV_WIDTH_MIN/NAV_WIDTH_MAX instead of collapsing all the way back to
// the default.

export const NAV_WIDTH_MIN = 200;
export const NAV_WIDTH_MAX = 560;
export const NAV_WIDTH_DEFAULT = 300;

export function clampNavWidth(px: number): number {
  if (!Number.isFinite(px) || px < 0) {
    return NAV_WIDTH_DEFAULT;
  }
  if (px < NAV_WIDTH_MIN) {
    return NAV_WIDTH_MIN;
  }
  if (px > NAV_WIDTH_MAX) {
    return NAV_WIDTH_MAX;
  }
  return px;
}
