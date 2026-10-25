// The navigator's "Hide cancelled" toggle (JC, 2026-10-18), persisted so a
// reload keeps cancelled work out of the tree. A FLAT key, the same call
// splitStorage.ts makes for the pane width: whether to show cancelled rows is
// a reading preference, not project content, so nothing can leak between
// projects that share the console's origin.

export const HIDE_CANCELLED_STORAGE_KEY = "tiller-explore-hide-cancelled";

// Only the exact string "true" turns the toggle on; absent, hand-edited, or
// unreadable storage all fall back to showing everything, today's default.
export function readHideCancelled(): boolean {
  try {
    return window.localStorage.getItem(HIDE_CANCELLED_STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

export function writeHideCancelled(hide: boolean): void {
  try {
    window.localStorage.setItem(HIDE_CANCELLED_STORAGE_KEY, String(hide));
  } catch {
    // A refused write still leaves the toggle working for this session;
    // only persistence across a reload is lost.
  }
}

// visibleRows is the one filter every tier and shelf applies, so a cancelled
// node disappears the same way wherever it sits. A hidden node takes its own
// children with it, the way a collapsed branch does.
export function visibleRows<T extends { status: string }>(rows: T[], hideCancelled: boolean): T[] {
  return hideCancelled ? rows.filter((row) => row.status !== "cancelled") : rows;
}
