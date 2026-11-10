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
  // Phrase verbs ("specced"), plus "asked" first when any entry asked.
  kinds: string[];
  epics: { id: string; title: string }[];
}

export type FilteredRecap = Recap & { shown: number };

export const ASKED = "asked";

// An entry's kind is the verb of its event phrase, never the raw type. The
// special kind "asked" also matches any entry whose ask flag is set.
function kindMatches(e: RecapEntry, kind: string): boolean {
  if (kind === ASKED && e.ask) {
    return true;
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
  verbs.delete(ASKED);
  const kinds = [...verbs].sort();
  if (asked || hasAskedVerb(recap)) {
    kinds.unshift(ASKED);
  }
  return {
    actors: [...actors].sort(),
    kinds,
    epics: [...epics].map(([id, title]) => ({ id, title })),
  };
}

function hasAskedVerb(recap: Recap): boolean {
  return recap.groups.some((g) => g.entries.some((e) => eventPhrase(e.type, 1).verb === ASKED));
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
