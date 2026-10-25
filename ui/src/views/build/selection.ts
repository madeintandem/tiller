// The Build board's own selection state (task-13-brief.md item 3): which
// epics' features the board's columns show, riding the `?epics=` URL param
// rather than component state, so a board configuration is a link (the
// phase acceptance line: "a pasted board URL reproduces the selection").
//
// The URL semantics table is exact, and the three functions below are its
// whole implementation:
//
//   - absent (`params.get("epics") === null`)      -> the default selection
//   - present, empty (`?epics=`)                    -> nothing selected
//   - present, non-empty (`?epics=a,b`)              -> exactly those keys
//
// "Absent" and "present but empty" read as two different states on
// purpose - collapsing them (e.g. treating "" as "no override") is exactly
// the bug this table exists to prevent, since deselecting the last epic
// must stay deselected on reload rather than silently reverting to the
// default. selectedEpicKeys is the one place that distinction is decided;
// every other piece of Build.tsx reads the result, never the raw param.

export function selectedEpicKeys(raw: string | null, defaultKeys: string[]): string[] {
  if (raw === null) {
    return defaultKeys;
  }
  if (raw === "") {
    return [];
  }
  // A stray comma ("a,,b" or a trailing "a,") splits to an empty segment;
  // filtering it out is defensive parsing, not a second selection state -
  // there is no way to name "the empty key" as a real epic key.
  return raw.split(",").filter((k) => k !== "");
}

// toggleEpicKey returns the next selection after one epic row's click:
// removed if it was present, appended otherwise. The result may come back
// empty, which is itself the legal explicit "nothing selected" state, never
// re-substituted with the default - the caller writes it straight back onto
// the URL via epicsParamValue below.
//
// Deliberate, not an oversight: an unmatched key already in `selected` (one
// that names no epic this board knows about) rides through untouched by a
// click on some other key, since it may simply name an epic that arrives later.
export function toggleEpicKey(selected: string[], key: string): string[] {
  return selected.includes(key) ? selected.filter((k) => k !== key) : [...selected, key];
}

// epicsParamValue serializes a selection back onto the URL. An empty
// selection still serializes to the present-but-empty form (""), never
// omitted: writing nothing here would mean the next parse reads the param
// as absent again, silently reviving the default selection the human just
// clicked away from.
export function epicsParamValue(selected: string[]): string {
  return selected.join(",");
}
