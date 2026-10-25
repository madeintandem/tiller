// Walk dependencies on node identities, then project the reached nodes onto
// visible cards. Collapsing siblings onto one card must not join their chains.
// Attachments and carriers name direct neighbors but are never traversed.
import type { MapTreeView } from "../../../api";
import { parentMap, visibleAncestorIn, visibleIds } from "./layout";

export interface Chain {
  up: Set<string>;
  down: Set<string>;
  // The cards the selected card's own depends_on edges reach, in either
  // direction: its dependencies and its dependents, one step away.
  near: Set<string>;
  // The far ends of the selected card's own attached_to and resolved_by
  // edges, in either direction: the bug's targets when a bug is selected,
  // the bugs and chores filed against it when a tree node is.
  linked: Set<string>;
}

export function chainOf(view: MapTreeView, open: ReadonlySet<string>, selected: string): Chain {
  const visible = visibleIds(view, open);
  const parents = parentMap(view);
  const selectedCard = visibleAncestorIn(visible, parents, selected);
  const addCard = (cards: Set<string>, id: string) => {
    const card = visibleAncestorIn(visible, parents, id);
    if (card !== undefined && card !== selectedCard) cards.add(card);
  };
  const deps = new Map<string, Set<string>>();
  const dependents = new Map<string, Set<string>>();
  const near = new Set<string>();
  const linked = new Set<string>();
  for (const { from, to, kind } of view.edges) {
    if (from === to) continue;
    const neighbors = kind === "depends_on" ? near : linked;
    if (from === selected) addCard(neighbors, to);
    if (to === selected) addCard(neighbors, from);
    if (kind !== "depends_on") continue;
    (deps.get(from) ?? deps.set(from, new Set()).get(from)!).add(to);
    (dependents.get(to) ?? dependents.set(to, new Set()).get(to)!).add(from);
  }
  const walk = (adjacent: Map<string, Set<string>>): Set<string> => {
    const seen = new Set([selected]);
    const cards = new Set<string>();
    const queue = [selected];
    for (let i = 0; i < queue.length; i++) {
      for (const next of adjacent.get(queue[i]) ?? []) {
        if (seen.has(next)) continue;
        seen.add(next);
        addCard(cards, next);
        queue.push(next);
      }
    }
    return cards;
  };
  return { up: walk(deps), down: walk(dependents), near, linked };
}
