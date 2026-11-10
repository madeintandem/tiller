import type { Recap, RecapEntry } from "../../api";
import { eventPhrase } from "../../lib/eventPhrase";

// The three client-side filters over the recap History already fetched. Pure:
// no fetch, no router. Order of operations in the view is filter first, then
// the milestone tier folds what is left.

export interface HistoryFilter {
  actor?: string;
  kind?: string;
  epic?: string;
}

export interface HistoryFacets {
  // Contributor ids present; the view shows each by its display name.
  actors: string[];
  // Phrase verbs ("specced"), plus ASKED_YOU first when any entry asked the
  // reader.
  kinds: string[];
  epics: { id: string; title: string }[];
}

export type FilteredRecap = Recap & { shown: number };

// The special kind for the events that asked the reader: exactly the entries
// whose ask flag is set, the same set the head's `N asked you` tally and a
// row's `asked you` link count. It is not the verb "asked", which an added
// question carries whether or not it asked the reader; those are found under
// that verb, a kind of its own. Its URL value is `asked-you`; it reads
// `asked you` on screen (kindLabel).
export const ASKED_YOU = "asked-you";

export function kindLabel(kind: string): string {
  return kind === ASKED_YOU ? "asked you" : kind;
}

// An entry's kind is the verb of its event phrase, never the raw type, or
// ASKED_YOU, which matches the ask flag alone.
function kindMatches(e: RecapEntry, kind: string): boolean {
  if (kind === ASKED_YOU) {
    return e.ask;
  }
  return eventPhrase(e.type, 1).verb === kind;
}

// An event belongs to the epic its group is served under (RecapGroup.epic), so
// no server change is needed to tie an event to its epic.
export function historyFacets(recap: Recap): HistoryFacets {
  const actors = new Set<string>();
  const verbs = new Set<string>();
  const epics = new Map<string, string>();
  let asked = false;
  for (const g of recap.groups) {
    if (g.entries.length > 0 && g.epic !== null && !epics.has(g.epic.id)) {
      epics.set(g.epic.id, g.epic.title ?? g.epic.key);
    }
    for (const e of g.entries) {
      if (e.contributor !== "") {
        actors.add(e.contributor);
      }
      verbs.add(eventPhrase(e.type, 1).verb);
      asked = asked || e.ask;
    }
  }
  const kinds = [...verbs].sort();
  if (asked) {
    kinds.unshift(ASKED_YOU);
  }
  return {
    actors: [...actors].sort(),
    kinds,
    epics: [...epics].map(([id, title]) => ({ id, title })),
  };
}

// Recomputes each group's entries and drops empty groups. total stays the
// unfiltered count; shown is the filtered count of served entries.
export function filterRecap(recap: Recap, filter: HistoryFilter): FilteredRecap {
  const groups = recap.groups
    .filter((g) => filter.epic === undefined || g.epic?.id === filter.epic)
    .map((g) => ({
      ...g,
      entries: g.entries.filter(
        (e) =>
          (filter.actor === undefined || e.contributor === filter.actor) &&
          (filter.kind === undefined || kindMatches(e, filter.kind)),
      ),
    }))
    .filter((g) => g.entries.length > 0);
  const shown = groups.reduce((n, g) => n + g.entries.length, 0);
  return { ...recap, groups, shown };
}
