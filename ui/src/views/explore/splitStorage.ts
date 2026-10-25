// The Explore split's navigator width, persisted so a reload restores the
// divider where the reader left it (task 4, docs/plans/2026-08-14-explore-
// tree-state.md). A FLAT key, unlike expansionStorage.ts's per-project one:
// an expansion map names actual project node ids, so one project's stored
// branches bleeding into another's tab would be visibly wrong, but a pane
// width is a plain viewport preference with no such content to leak - the
// same reasoning panelStorage.ts's own flat BUILD_PANEL_STORAGE_KEY already
// applies to the epic panel's open/closed state.

import { clampNavWidth, NAV_WIDTH_DEFAULT } from "./splitSize";

export const SPLIT_STORAGE_KEY = "tiller-explore-split";

// Total over any stored string: absent, hand-edited, or otherwise
// non-numeric content all reach clampNavWidth's own NaN band and fold to
// NAV_WIDTH_DEFAULT there, so this function owns no normalization logic of
// its own beyond turning the raw string into a number.
export function readNavWidth(): number {
  let raw: string | null;
  try {
    raw = window.localStorage.getItem(SPLIT_STORAGE_KEY);
  } catch {
    return NAV_WIDTH_DEFAULT;
  }
  if (raw === null) {
    return NAV_WIDTH_DEFAULT;
  }
  return clampNavWidth(Number(raw));
}

export function writeNavWidth(px: number): void {
  try {
    window.localStorage.setItem(SPLIT_STORAGE_KEY, String(px));
  } catch {
    // A refused write still leaves the divider working for this session;
    // only persistence across a reload is lost.
  }
}
