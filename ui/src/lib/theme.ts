// Theme control (Phase 0 task 3): Auto/Light/Dark, wired to the data-theme
// attribute tokens.css already reads (task 2's light-dark() layer, the
// forced-theme :root[data-theme='light'|'dark'] overrides and the --tint
// media-query fallback all key off this attribute; see that file's own
// header comment). This module is the one place that reads/writes it, so
// both the pre-paint boot call (main.tsx) and the control itself (
// ThemeToggle.tsx) apply the exact same semantics as the mock's own
// applyTheme (the retired console mock):
// "auto" removes the attribute and lets prefers-color-scheme decide, a
// specific pick sets it.
//
// The plan names the storage key verbatim: "tiller-theme" - NOT the mock's
// own "tiller-mock-theme", which is mock-only scaffolding.

// index.html carries an inline pre-paint twin of this key and the
// 'light'/'dark' literals below (see that file's own header comment); an
// edit to either side must be mirrored in the other or they silently drift.
export const THEME_STORAGE_KEY = "tiller-theme";

export type ThemePick = "auto" | "light" | "dark";

const VALID_PICKS: ReadonlySet<string> = new Set<ThemePick>(["auto", "light", "dark"]);

function isThemePick(value: string | null): value is ThemePick {
  return value !== null && VALID_PICKS.has(value);
}

// Total over any stored string: a value this build never wrote (hand-edited
// localStorage, an older/newer build's vocabulary) folds to "auto" rather
// than crashing or leaving the control wedged on a pick with no matching
// button.
export function normalizeTheme(value: string | null): ThemePick {
  return isThemePick(value) ? value : "auto";
}

// Wrapped in try/catch the same way RailSection's persistence is
// (components/RailSection.tsx): a refused read (private mode, a disabled
// store) degrades to "auto" rather than throwing during module load.
export function readStoredTheme(): ThemePick {
  try {
    return normalizeTheme(window.localStorage.getItem(THEME_STORAGE_KEY));
  } catch {
    return "auto";
  }
}

// Sets or removes data-theme on the root element. This is the only DOM
// write - no localStorage access - so the pre-paint boot call can apply a
// stored pick without also re-persisting it on every load.
export function applyThemeAttribute(pick: ThemePick): void {
  const root = document.documentElement;
  if (pick === "auto") {
    root.removeAttribute("data-theme");
  } else {
    root.setAttribute("data-theme", pick);
  }
}

function persistTheme(pick: ThemePick): void {
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, pick);
  } catch {
    // A refused write still leaves the attribute (and thus the theme)
    // applied for this session; only persistence across a reload is lost.
  }
}

// The control's own entry point: apply the attribute and persist the pick,
// in that order, so a localStorage failure never leaves the visible theme
// out of sync with what was clicked.
export function setTheme(pick: ThemePick): void {
  applyThemeAttribute(pick);
  persistTheme(pick);
}

// main.tsx's pre-paint entry point (see that file's own comment for why it
// runs where it does): read whatever is stored - normalizing a garbage
// value to "auto" - and apply it, without writing back to localStorage.
export function bootTheme(): void {
  applyThemeAttribute(readStoredTheme());
}
