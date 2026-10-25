import type { MapEdgeKind, MapView, RelationshipEdge, RelationshipVertex } from "../../../api";
import type { MapLayout } from "./layout";
import { arcPath, parentMap, visibleAncestorIn } from "./layout";

export interface RelationshipArc {
  edge: RelationshipEdge;
  from: string;
  to: string;
  d: string;
}

export interface RelationshipOverlay {
  layout: MapLayout;
  edges: RelationshipEdge[];
  arcs: RelationshipArc[];
  cards: RelationshipVertex[];
  byID: ReadonlyMap<string, RelationshipVertex>;
  contextTop: number | null;
}

export const WORK_LINK_PRESET = "work_links";
export const WORK_LINK_KINDS: Readonly<Record<string, boolean>> = {
  depends_on: true,
  attached_to: true,
  resolved_by: true,
} satisfies Record<MapEdgeKind, true>;

// The display tree is immutable input. Semantic endpoints occupy their own
// region below it; they never participate in side affinity, sibling order,
// display ancestry, or the dependency chain.
export function relationshipOverlay(
  view: MapView,
  tree: MapLayout,
  selected: string | null,
  showAll: boolean,
  kind: string,
  results?: { edges: readonly RelationshipEdge[]; vertices: readonly RelationshipVertex[] } | null,
): RelationshipOverlay {
  const byID = new Map(view.relationships.vertices.map((vertex) => [vertex.id, vertex]));
  for (const vertex of results?.vertices ?? []) byID.set(vertex.id, vertex);
  const work = new Set(view.nodes.map((node) => node.id));
  const visible = new Set(tree.boxes.keys());
  const parents = parentMap(view);
  const representative = (id: string): string | undefined =>
    work.has(id) ? visibleAncestorIn(visible, parents, id) : id;
  const edges = (results?.edges ?? view.relationships.edges).filter((edge) => {
    if (kind === WORK_LINK_PRESET ? WORK_LINK_KINDS[edge.kind] !== true : kind !== "" && edge.kind !== kind) return false;
    if (results) return true;
    if (!showAll && kind === "" && edge.kind === "at_path") return false;
    // Choose relationships by identity before projecting their endpoints.
    // A collapsed ancestor stands for the selection, not for a wider scope.
    return showAll || (selected !== null && (edge.from === selected || edge.to === selected));
  });
  const endpointIDs = new Set<string>();
  for (const edge of edges) {
    if (!work.has(edge.from)) endpointIDs.add(edge.from);
    if (!work.has(edge.to)) endpointIDs.add(edge.to);
  }
  if (selected !== null && !work.has(selected)) endpointIDs.add(selected);
  const cards = [...byID.values()].filter((vertex) => endpointIDs.has(vertex.id));
  const boxes = new Map(tree.boxes);
  const contextTop = cards.length === 0 ? null : tree.bounds.y + tree.bounds.h + 64;
  const columns = Math.min(3, cards.length);
  const cardWidth = 280;
  const cardHeight = 72;
  const gap = 20;
  const left = -(columns * (cardWidth + gap) - gap) / 2;
  if (contextTop !== null) {
    cards.forEach((vertex, i) => {
      boxes.set(vertex.id, {
        id: vertex.id,
        noun: vertex.kind,
        side: "right",
        x: left + (i % columns) * (cardWidth + gap),
        y: contextTop + Math.floor(i / columns) * (cardHeight + gap),
        w: cardWidth,
        h: cardHeight,
      });
    });
  }
  const arcs: RelationshipArc[] = [];
  for (const edge of edges) {
    // Containment already has a structural connector. Keep its provenance in
    // the inspector without drawing a second wire over the tree.
    if (edge.kind === "contains") continue;
    const from = representative(edge.from);
    const to = representative(edge.to);
    if (from === undefined || to === undefined || from === to) continue;
    const a = boxes.get(from);
    const b = boxes.get(to);
    if (a === undefined || b === undefined) continue;
    // Preserve dependency-flow arrows; all other relations follow their
    // recorded direction. The inspector always states canonical direction.
    const d = edge.kind === "depends_on" ? arcPath(b, a) : arcPath(a, b);
    arcs.push({ edge, from, to, d });
  }
  let bounds = tree.bounds;
  if (contextTop !== null) {
    const right = Math.max(bounds.x + bounds.w, left + columns * (cardWidth + gap) - gap + 28);
    const bottom = contextTop + Math.ceil(cards.length / columns) * (cardHeight + gap) - gap + 28;
    const x = Math.min(bounds.x, left - 28);
    bounds = { x, y: bounds.y, w: right - x, h: bottom - bounds.y };
  }
  return {
    layout: { ...tree, boxes, bounds, width: bounds.w, height: bounds.h },
    edges,
    arcs,
    cards,
    byID,
    contextTop,
  };
}

const RELATION_COLORS = ["var(--ink)", "var(--ink-2)", "var(--ink-3)"];
const RELATION_DASHES = ["none", "7 4", "2 4", "9 3 2 3", "12 5", "4 3 1 3", "1 2"];

// Arcs and their legend share a deterministic visual key. Text labels remain
// authoritative; color is never the only way to identify a relationship.
export function relationshipStroke(kind: string, catalog: readonly { name: string }[]): { color: string; dash: string } {
  if (kind === "contains") return { color: "color-mix(in srgb, var(--ink-3) 55%, var(--paper))", dash: "none" };
  const index = Math.max(0, catalog.findIndex((entry) => entry.name === kind));
  return { color: RELATION_COLORS[index % RELATION_COLORS.length], dash: RELATION_DASHES[Math.floor(index / RELATION_COLORS.length) % RELATION_DASHES.length] };
}
