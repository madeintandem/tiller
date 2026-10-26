// The Build board's epic selection, persisted to localStorage so a page
// reload or bare-route arrival restores the epics the developer was focused
// on. Storage only bootstraps an absent URL selection; the URL stays the
// single source of truth downstream, as selection.ts's doc comment
// frames it. Namespaced per project key because every project serves
// on the same default port (4400), one browser origin, so a flat key (the
// panelStorage.ts shape) would leak one project's selection into another
// project's tab.
//
// The stored value mirrors the URL's own serialization exactly
// (epicsParamValue / selectedEpicKeys, selection.ts) rather than inventing a
// second one: a comma-joined string, with the present-but-empty form ("")
// read back distinctly from no entry at all (`null`) - the same
// absent/present-empty/present-non-empty table selection.ts documents for
// the URL param, mirrored into storage so an explicit "deselect everything"
// survives a reload instead of reviving the server default.

import { epicsParamValue, selectedEpicKeys } from "./selection";

const SELECTION_STORAGE_PREFIX = "tiller-build-epics";

export function selectionStorageKey(projectKey: string): string {
  return `${SELECTION_STORAGE_PREFIX}:${projectKey}`;
}

// Returns null when this project has no stored selection yet - the caller's
// cue to fall through to the server default (active_epics) exactly as an
// absent URL param would. A stored entry, even the present-but-empty one,
// comes back as a real (possibly empty) array, parsed with the exact same
// segment-filtering selectedEpicKeys already applies to the URL param, so a
// hand-edited or stray-comma value normalizes the same way in both places
// rather than growing a second parser.
export function readSelectionKeys(projectKey: string): string[] | null {
  let raw: string | null;
  try {
    raw = window.localStorage.getItem(selectionStorageKey(projectKey));
  } catch {
    return null;
  }
  if (raw === null) {
    return null;
  }
  return selectedEpicKeys(raw, []);
}

// Persists the exact selection under this project's own key. An empty
// selection still writes the present-but-empty form (epicsParamValue's own
// choice, mirrored here): writing nothing would make the next read see
// `null` again and silently fall back to the default the human just
// deselected away from.
export function writeSelectionKeys(projectKey: string, selected: string[]): void {
  try {
    window.localStorage.setItem(selectionStorageKey(projectKey), epicsParamValue(selected));
  } catch {
    // A refused write still leaves this session working; only persistence
    // across a reload is lost.
  }
}
