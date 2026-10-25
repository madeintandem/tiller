// The epic panel's collapsed state (task-13-brief.md: "Panel collapsed
// state: localStorage, key in the house style (`tiller-build-panel` shape;
// see `tiller-theme`)"): one flat key, read/written the same normalize-on-
// read shape lib/theme.ts's THEME_STORAGE_KEY establishes, rather than
// RailSection.tsx's per-section-id prefix scheme - Build has exactly one
// panel, so there is no id to key it by.

export const BUILD_PANEL_STORAGE_KEY = "tiller-build-panel";

export type PanelState = "open" | "closed";

function isPanelState(value: string | null): value is PanelState {
  return value === "open" || value === "closed";
}

// Total over any stored string: a value this build never wrote (hand-edited
// localStorage, an older/newer build's vocabulary) folds to "open" rather
// than crashing or wedging the panel closed with no visible way to reopen
// it - the same normalize-on-read posture readStoredTheme takes.
export function normalizePanelState(value: string | null): PanelState {
  return isPanelState(value) ? value : "open";
}

// Wrapped in try/catch the same way RailSection's and theme.ts's own
// persistence are: a refused read (private mode, a disabled store) degrades
// to "open" rather than throwing during render.
export function readPanelState(key = BUILD_PANEL_STORAGE_KEY): PanelState {
  try {
    return normalizePanelState(window.localStorage.getItem(key));
  } catch {
    return "open";
  }
}

export function writePanelState(state: PanelState, key = BUILD_PANEL_STORAGE_KEY): void {
  try {
    window.localStorage.setItem(key, state);
  } catch {
    // A refused write still leaves the toggle working for this session;
    // only persistence across a reload is lost.
  }
}
