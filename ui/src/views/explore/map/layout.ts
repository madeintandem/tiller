// The Explore map's layout, a balanced horizontal mind map, pure and
// dependency-free (docs/plans/2026-08-30-explore-map-design.md, "The layout",
// amended 2026-08-31). The project is the root at the center; its children
// (epics in wire order, then the Bugs and Chores branches when their shelf
// holds anything) are seated right or left by sidesOf, beside what they
// depend on unless that side has run too far ahead; every child stacks
// beside its parent further out on the same side; a branch is as tall as its
// open descendants and sits at the vertical center of its stack. Branch
// connectors are the only lines at rest; dependency arcs are computed here
// too, lifted to the visible ancestor, deduped, and routed by where their
// two cards sit, but the canvas draws them only for a selection or on the
// Work links preset.
//
// The root card's center is the world origin, and everything is placed around
// it: what is left of the root or above it is at negative coordinates, which
// the world transform the canvas's two layers share maps like any other
// point, and no toggle can move the root on screen. The drawing's own
// rectangle is reported as `bounds`, which is what a fit reads.
import type { MapEdgeKind, MapNodeView, MapTreeView } from "../../../api";

export const ROOT_ID = "root";
export const SHELF_BUG = "shelf:bug";
export const SHELF_CHORE = "shelf:chore";
export const SHELF_TWEAK = "shelf:tweak";

export type Side = "left" | "right" | "center";

export interface Box {
  id: string;
  noun: string;
  x: number;
  y: number;
  w: number;
  h: number;
  side: Side;
}

export interface BranchPath {
  key: string;
  from: string;
  to: string;
  d: string;
}

export interface EdgePath {
  key: string;
  from: string;
  to: string;
  kind: MapEdgeKind;
  d: string;
}

// The world rectangle the drawing occupies: the boxes' extent padded on all
// four sides. It carries an origin as well as a size, because the root's
// center is the world origin and everything left of it or above it is at
// negative coordinates.
export interface Bounds {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface MapLayout {
  boxes: Map<string, Box>;
  branches: BranchPath[];
  edges: EdgePath[];
  bounds: Bounds;
  // The bounds' size under their old names, for every caller that only ever
  // asked how big the drawing is.
  width: number;
  height: number;
}

// Closed-card sizes per noun; a container's card never grows, since its
// children stack beside it rather than inside it.
export const CARD: Record<string, { w: number; h: number }> = {
  root: { w: 160, h: 44 },
  epic: { w: 240, h: 40 },
  feature: { w: 232, h: 28 },
  task: { w: 216, h: 26 },
  bug: { w: 216, h: 26 },
  chore: { w: 216, h: 26 },
  tweak: { w: 216, h: 26 },
  shelf: { w: 200, h: 34 },
};
const GAP_ROOT = 140;
const GAP_BRANCH = 100;
const GAP_LEAF = 90;
const ROW_GAP_LEAF = 4;
const ROW_GAP_BRANCH = 12;
const PAD = 28;
// How far a same-column bracket bows out from the column's edge: enough for
// an adjacent pair to read as a curve, capped so a long span stays a flat
// bracket rather than a wide bay.
const BRACKET_MIN = 20;
const BRACKET_MAX = 72;

function isShelfNode(n: MapNodeView): boolean {
  return n.noun === "bug" || n.noun === "chore" || n.noun === "tweak";
}

function shelfOf(n: MapNodeView): string {
  return n.noun === "bug" ? SHELF_BUG : n.noun === "chore" ? SHELF_CHORE : SHELF_TWEAK;
}

function sizeOf(noun: string): { w: number; h: number } {
  return CARD[noun] ?? CARD.task;
}

// The virtual tree: the root's children are the epics in wire order, then
// each non-empty shelf; a shelf's children are its bugs or chores in wire
// order; every other node's children come from the wire's parent field.
function childrenMap(view: MapTreeView): Map<string, MapNodeView[]> {
  const out = new Map<string, MapNodeView[]>();
  const push = (parent: string, n: MapNodeView) => {
    const list = out.get(parent) ?? [];
    list.push(n);
    out.set(parent, list);
  };
  for (const n of view.nodes) {
    if (isShelfNode(n)) {
      push(shelfOf(n), n);
    } else if (n.parent !== undefined) {
      push(n.parent, n);
    } else {
      push(ROOT_ID, n);
    }
  }
  if (out.has(SHELF_BUG)) {
    push(ROOT_ID, { noun: "shelf", id: SHELF_BUG, title: "Bugs", status: "draft" });
  }
  if (out.has(SHELF_CHORE)) {
    push(ROOT_ID, { noun: "shelf", id: SHELF_CHORE, title: "Chores", status: "draft" });
  }
  if (out.has(SHELF_TWEAK)) {
    push(ROOT_ID, { noun: "shelf", id: SHELF_TWEAK, title: "Tweaks", status: "draft" });
  }
  return out;
}

// parentMap is every virtual child-to-parent link in one pass: a bug or a
// chore hangs off its shelf, a shelf that carries anything off the root, and
// everything else off the parent the wire names (undefined for an epic, which
// hangs off the root without saying so). The canvas walks ancestry once per
// selected node and once per member of its chain, and it re-renders on every
// pointermove of a pan, so it takes the whole map once and reads it rather
// than scanning the node list per link.
export function parentMap(view: MapTreeView): Map<string, string | undefined> {
  const out = new Map<string, string | undefined>();
  for (const n of view.nodes) {
    if (isShelfNode(n)) {
      const shelf = shelfOf(n);
      out.set(n.id, shelf);
      out.set(shelf, ROOT_ID);
    } else {
      out.set(n.id, n.parent);
    }
  }
  return out;
}

export function parentOf(view: MapTreeView, id: string): string | undefined {
  return parentMap(view).get(id);
}

// What one dependency arc across the root costs when a branch is seated,
// in rows of imbalance: a branch follows its relationships to the heavier
// side until that side stands this many rows further ahead per arc.
const ROWS_PER_CROSSING = 3;

// The first-level branch a node hangs under: an epic for itself and for
// everything in its tree, a shelf for a bug or a chore.
function branchOf(parents: ReadonlyMap<string, string | undefined>, id: string): string {
  let current = id;
  for (;;) {
    const parent = parents.get(current);
    if (parent === undefined || parent === ROOT_ID) {
      return current;
    }
    current = parent;
  }
}

// sidesOf seats every first-level branch (an epic, or a shelf that holds
// something) on the right or the left of the root. Strict alternation was
// the first rule, and on Lantern it seated Device Readiness on the left while
// everything it depends on sat on the right, so four of the map's five
// cross-side arcs were the alternation's own doing (2026-08-31). Each root
// child, in wire order, now takes the side that costs less: every recorded
// relationship between it and a branch already seated on the other side is a
// crossing, worth ROWS_PER_CROSSING rows, and the rows the chosen side would
// then stand ahead of the other by are the rest. Wire order is dependency
// order, so a branch's dependencies are seated before it is. Rows are the
// height at Expand all (the branch card plus its direct children), which
// never changes with the open set, so a chevron never moves a branch across
// the root. A tie goes right, which is where the first epic lands.
export function sidesOf(view: MapTreeView): Map<string, "left" | "right"> {
  const kids = childrenMap(view);
  const parents = parentMap(view);
  const links = new Map<string, Map<string, number>>();
  const link = (a: string, b: string) => {
    const row = links.get(a) ?? links.set(a, new Map()).get(a)!;
    row.set(b, (row.get(b) ?? 0) + 1);
  };
  for (const e of view.edges) {
    if (!parents.has(e.from) || !parents.has(e.to)) {
      continue;
    }
    const a = branchOf(parents, e.from);
    const b = branchOf(parents, e.to);
    if (a !== b) {
      link(a, b);
      link(b, a);
    }
  }
  const seated = new Map<string, "left" | "right">();
  const rows = { left: 0, right: 0 };
  for (const child of kids.get(ROOT_ID) ?? []) {
    const weight = 1 + (kids.get(child.id) ?? []).length;
    const crossingsIfOn = (side: "left" | "right"): number => {
      let n = 0;
      for (const [other, count] of links.get(child.id) ?? []) {
        const there = seated.get(other);
        if (there !== undefined && there !== side) {
          n += count;
        }
      }
      return n;
    };
    const cost = (side: "left" | "right", other: "left" | "right") =>
      crossingsIfOn(side) * ROWS_PER_CROSSING + Math.max(0, rows[side] + weight - rows[other]);
    const side = cost("left", "right") < cost("right", "left") ? "left" : "right";
    seated.set(child.id, side);
    rows[side] += weight;
  }
  return seated;
}

export function containers(view: MapTreeView): Set<string> {
  const kids = childrenMap(view);
  const out = new Set<string>();
  for (const [id, list] of kids) {
    if (id !== ROOT_ID && list.length > 0) {
      out.add(id);
    }
  }
  return out;
}

// visibleIds walks the virtual tree from the root: a root child is always
// visible; deeper nodes only while every branch above them is open. Wire
// order is preserved within a stack.
export function visibleIds(view: MapTreeView, open: ReadonlySet<string>): Set<string> {
  const kids = childrenMap(view);
  const out = new Set<string>();
  const walk = (n: MapNodeView) => {
    out.add(n.id);
    if (open.has(n.id)) {
      for (const child of kids.get(n.id) ?? []) {
        walk(child);
      }
    }
  };
  for (const child of kids.get(ROOT_ID) ?? []) {
    walk(child);
  }
  return out;
}

// The card a node lands on: itself while it is visible, else the nearest
// open container above it, else nothing at all. It takes the visible set and
// the parent links rather than the map, because every caller asks it about
// both ends of every edge in the project and rebuilding those two per
// question was the whole cost of routing the arcs.
export function visibleAncestorIn(
  visible: ReadonlySet<string>,
  parents: ReadonlyMap<string, string | undefined>,
  id: string,
): string | undefined {
  let current: string | undefined = id;
  while (current !== undefined && !visible.has(current)) {
    current = current === SHELF_BUG || current === SHELF_CHORE || current === SHELF_TWEAK ? undefined : parents.get(current);
  }
  return current;
}

// The same question asked once, for a caller with nothing built yet.
export function visibleAncestor(
  view: MapTreeView,
  open: ReadonlySet<string>,
  id: string,
): string | undefined {
  return visibleAncestorIn(visibleIds(view, open), parentMap(view), id);
}

// arcPath is a dependency arc, routed by where its two cards sit. Every
// route is one cubic that leaves and arrives horizontally, so the arrowhead
// on its end points into the side of a card and sits in the open beside it
// rather than on the card's roof, under whatever is stacked above.
//
// Three cases. Two cards whose x-ranges overlap share a column (siblings, or
// the features of two epics on one side), and the arc brackets them on the
// column's OUTER edge, away from the root, bulging out a little further the
// longer it spans: a chain down a stack reads like an arc diagram beside it,
// and with the tasks closed that margin is empty. Two cards in different
// columns on one side get an S-curve between their facing edges, the branch
// connector's own shape. Two cards on opposite sides get the same S-curve
// between their INNER edges, across the middle, where the root and the epic
// column are mostly open space. The old rule, top edge to top edge with an
// upward bow, sent every same-column arc straight down behind the cards
// between its ends and lifted every cross-side arc into whichever epic card
// the bow rose to; on Lantern that was 20 of 24 arcs hidden and the other 4
// apparently leaving cards they never touched (2026-08-31).
export function arcPath(a: Box, b: Box): string {
  const ay = a.y + a.h / 2;
  const by = b.y + b.h / 2;
  const sameColumn = a.x < b.x + b.w && b.x < a.x + a.w;
  if (sameColumn) {
    const out = a.side === "left" ? -1 : 1;
    const x1 = out > 0 ? a.x + a.w : a.x;
    const x2 = out > 0 ? b.x + b.w : b.x;
    const bulge = Math.min(BRACKET_MAX, BRACKET_MIN + Math.abs(by - ay) / 5);
    return `M ${x1} ${ay} C ${x1 + out * bulge} ${ay}, ${x2 + out * bulge} ${by}, ${x2} ${by}`;
  }
  const bRight = b.x > a.x;
  const x1 = bRight ? a.x + a.w : a.x;
  const x2 = bRight ? b.x : b.x + b.w;
  const mid = (x1 + x2) / 2;
  return `M ${x1} ${ay} C ${mid} ${ay}, ${mid} ${by}, ${x2} ${by}`;
}

interface Placed {
  box: Box;
  block: number;
}

// The root card's name is not a layout concern: CARD.root is a fixed size
// whatever the project is called, and no Box carries a title. The canvas
// takes the name as a prop of its own instead.
export function layoutMap(view: MapTreeView, open: ReadonlySet<string>): MapLayout {
  const kids = childrenMap(view);
  const boxes = new Map<string, Box>();
  const branches: BranchPath[] = [];

  const gapAfter = (noun: string) => (noun === "root" ? GAP_ROOT : noun === "feature" ? GAP_LEAF : GAP_BRANCH);
  const rowGap = (noun: string) => (noun === "feature" || noun === "task" || noun === "bug" || noun === "chore" || noun === "tweak" ? ROW_GAP_LEAF : ROW_GAP_BRANCH);

  // The height a node's whole open subtree needs on its side.
  const blockOf = (n: MapNodeView): number => {
    const own = sizeOf(n.noun).h;
    const children = open.has(n.id) ? (kids.get(n.id) ?? []) : [];
    if (children.length === 0) {
      return own;
    }
    let sum = 0;
    children.forEach((c, i) => {
      sum += blockOf(c) + (i > 0 ? rowGap(c.noun) : 0);
    });
    return Math.max(own, sum);
  };

  // The height one stack of siblings occupies, their blocks plus the gaps
  // between them.
  const stackHeight = (list: MapNodeView[]): number =>
    list.reduce((sum, c, i) => sum + blockOf(c) + (i > 0 ? rowGap(c.noun) : 0), 0);

  // Place n with its block's top at y, its card at the block's center, and
  // its open children stacked beside it further out on the same side.
  const place = (n: MapNodeView, side: Side, xInner: number, y: number): Placed => {
    const size = sizeOf(n.noun);
    const block = blockOf(n);
    const x = side === "left" ? xInner - size.w : xInner;
    const box: Box = { id: n.id, noun: n.noun, x, y: y + block / 2 - size.h / 2, w: size.w, h: size.h, side };
    boxes.set(n.id, box);
    const children = open.has(n.id) ? (kids.get(n.id) ?? []) : [];
    // The stack is centered in the block rather than started at its top,
    // because the block is the taller of the card and the stack: when the
    // card wins (a lone feature under an epic), the slack is the card's, and
    // hanging the children off the top of it left them riding above the
    // parent they belong to with the connector climbing out of its edge.
    let cy = y + block / 2 - stackHeight(children) / 2;
    for (const c of children) {
      const childInner = side === "left" ? x - gapAfter(n.noun) : x + size.w + gapAfter(n.noun);
      const placed = place(c, side, childInner, cy);
      branches.push({ key: `${n.id}>${c.id}`, from: n.id, to: c.id, d: connector(box, placed.box) });
      cy += placed.block + rowGap(c.noun);
    }
    return { box, block };
  };

  // Lay both sides out from x = 0 in their own direction, then translate
  // everything so the root's own center is the world origin.
  const rootSize = sizeOf("root");
  const rootChildren = kids.get(ROOT_ID) ?? [];
  const sides: Record<"right" | "left", MapNodeView[]> = { right: [], left: [] };
  const seat = sidesOf(view);
  rootChildren.forEach((c) => sides[seat.get(c.id) ?? "right"].push(c));
  const heights = { right: stackHeight(sides.right), left: stackHeight(sides.left) };
  const rootBox: Box = { id: ROOT_ID, noun: "root", x: 0, y: -rootSize.h / 2, w: rootSize.w, h: rootSize.h, side: "center" };
  boxes.set(ROOT_ID, rootBox);
  for (const side of ["right", "left"] as const) {
    let y = -heights[side] / 2;
    for (const c of sides[side]) {
      const inner = side === "right" ? rootSize.w + GAP_ROOT : -GAP_ROOT;
      const placed = place(c, side, inner, y);
      branches.push({ key: `${ROOT_ID}>${c.id}`, from: ROOT_ID, to: c.id, d: connector(rootBox, placed.box) });
      y += placed.block + ROW_GAP_BRANCH;
    }
  }
  // The root's center IS the origin, so opening or closing a branch never
  // moves the root on screen. World coordinates used to be re-derived from
  // the drawing's own extent per layout, which meant every chevron click slid
  // the whole map sideways under the reader and Expand all could push the
  // root card out of the frame entirely (fix round 2). Negative coordinates
  // are fine: the viewport is a world point and a zoom, and the transform
  // both canvas layers carry moves that point to the frame's top left
  // whatever its sign.
  const dx = -(rootBox.x + rootBox.w / 2);
  const dy = -(rootBox.y + rootBox.h / 2);
  for (const b of boxes.values()) {
    b.x += dx;
    b.y += dy;
  }
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const b of boxes.values()) {
    minX = Math.min(minX, b.x);
    minY = Math.min(minY, b.y);
    maxX = Math.max(maxX, b.x + b.w);
    maxY = Math.max(maxY, b.y + b.h);
  }
  const bounds: Bounds = { x: minX - PAD, y: minY - PAD, w: maxX - minX + 2 * PAD, h: maxY - minY + 2 * PAD };
  // Branches and arcs are drawn from the boxes' final coordinates, so the
  // translation above reaches every line without touching a path string.
  for (const br of branches) {
    br.d = connector(boxes.get(br.from)!, boxes.get(br.to)!);
  }

  return { boxes, branches, edges: routeEdges(view, open, boxes), bounds, width: bounds.w, height: bounds.h };
}

// connector is the branch line: from the parent's outer edge to the child's
// inner edge, by the child's side, as one cubic curve.
function connector(parent: Box, child: Box): string {
  const goRight = child.side !== "left";
  const x1 = goRight ? parent.x + parent.w : parent.x;
  const x2 = goRight ? child.x : child.x + child.w;
  const y1 = parent.y + parent.h / 2;
  const y2 = child.y + child.h / 2;
  const dx = Math.abs(x2 - x1) / 2;
  const sx = goRight ? 1 : -1;
  return `M ${x1} ${y1} C ${x1 + sx * dx} ${y1}, ${x2 - sx * dx} ${y2}, ${x2} ${y2}`;
}

// routeEdges lifts every wire edge to the cards its ends land on, draws a
// depends_on from the dependency to the dependent and a shelf edge from the
// shelf node to its target, drops a pair that collapses onto one card, and
// draws a repeated pair once.
//
// The visible set and the parent links are built once for the whole pass
// rather than once per edge end. Acme records 298 edges, so the old shape
// walked the node list nearly six hundred times per layout, and a layout runs
// on every toggle.
function routeEdges(view: MapTreeView, open: ReadonlySet<string>, boxes: Map<string, Box>): EdgePath[] {
  const out: EdgePath[] = [];
  const seen = new Set<string>();
  const visible = visibleIds(view, open);
  const parents = parentMap(view);
  for (const e of view.edges) {
    const [srcID, dstID] = e.kind === "depends_on" ? [e.to, e.from] : [e.from, e.to];
    const src = visibleAncestorIn(visible, parents, srcID);
    const dst = visibleAncestorIn(visible, parents, dstID);
    if (src === undefined || dst === undefined || src === dst) {
      continue;
    }
    const key = `${src}>${dst}:${e.kind}`;
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    const a = boxes.get(src);
    const b = boxes.get(dst);
    if (a === undefined || b === undefined) {
      continue;
    }
    out.push({ key, from: src, to: dst, kind: e.kind, d: arcPath(a, b) });
  }
  return out;
}
