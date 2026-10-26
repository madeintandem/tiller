// The mind map's layout, pure (docs/spec/ui-surface.md): the project at the
// center, epics alternating sides in wire order, children stacked beside their
// parent further out, Bugs and Chores as two last branches, branch connectors
// as the only lines at rest, dependency arcs lifted to the visible ancestor and
// deduped.
import { describe, expect, it } from "vitest";
import type { MapNodeView, MapTreeView } from "../../../api";
import { CARD, ROOT_ID, SHELF_BUG, SHELF_CHORE, arcPath, containers, layoutMap, parentMap, parentOf, sidesOf, visibleAncestor, visibleIds } from "./layout";

const view: MapTreeView = {
  nodes: [
    { noun: "epic", id: "E-1", title: "Foundation", status: "ready" },
    { noun: "feature", id: "F-1", title: "Skeleton", status: "ready", parent: "E-1" },
    { noun: "task", id: "T-1", title: "Init", status: "done", parent: "F-1" },
    { noun: "feature", id: "F-2", title: "CI", status: "draft", parent: "E-1", holds: 1 },
    { noun: "epic", id: "E-2", title: "Audit", status: "draft" },
    { noun: "feature", id: "F-3", title: "Trail", status: "draft", parent: "E-2" },
    { noun: "epic", id: "E-3", title: "Reporting", status: "draft" },
    { noun: "bug", id: "B-1", title: "Blank on reload", status: "proposed" },
  ],
  edges: [
    { from: "F-2", to: "F-1", kind: "depends_on" },
    { from: "F-3", to: "F-2", kind: "depends_on" },
    { from: "B-1", to: "F-2", kind: "attached_to" },
    { from: "B-1", to: "T-1", kind: "resolved_by" },
  ],
};
const allOpen = new Set(["E-1", "E-2", "E-3", "F-1", SHELF_BUG]);

// The same project with a chore on the shelf, which is what puts both shelf
// branches on the map at once: five root children rather than four.
const withChore: MapTreeView = {
  nodes: [...view.nodes, { noun: "chore", id: "CH-1", title: "Bump deps", status: "proposed" }],
  edges: view.edges,
};

describe("containers and parents", () => {
  it("names the epics and features with children plus a shelf branch that has something on it", () => {
    expect([...containers(view)].sort()).toEqual(["E-1", "E-2", "F-1", SHELF_BUG].sort());
  });

  it("keeps Tweaks on a distinct collapsible shelf and lifts their work links", () => {
    const tweaks: MapTreeView = {
      nodes: [...view.nodes, { noun: "tweak", id: "TW-abc123", title: "Button spacing", status: "ready" }],
      edges: [...view.edges, { from: "TW-abc123", to: "F-1", kind: "attached_to" }],
    };
    expect(parentOf(tweaks, "TW-abc123")).toBe("shelf:tweak");
    expect(containers(tweaks).has("shelf:tweak")).toBe(true);
    expect(visibleAncestor(tweaks, new Set(), "TW-abc123")).toBe("shelf:tweak");
    expect(layoutMap(tweaks, new Set()).boxes.has("TW-abc123")).toBe(false);
    expect(layoutMap(tweaks, new Set(["shelf:tweak"])).boxes.has("TW-abc123")).toBe(true);
  });
  it("gives a bug the Bugs branch as its parent and an epic none", () => {
    expect(parentOf(view, "B-1")).toBe(SHELF_BUG);
    expect(parentOf(view, "F-1")).toBe("E-1");
    expect(parentOf(view, "E-1")).toBeUndefined();
  });

  it("builds every virtual parent link in one pass, the shelf branches included", () => {
    // The canvas walks ancestry once per selected node and once per chain
    // member, and it re-renders on every pointermove of a pan: it takes the
    // whole map once rather than re-scanning the node list per link.
    const parents = parentMap(view);
    expect(parents.get("B-1")).toBe(SHELF_BUG);
    expect(parents.get("F-1")).toBe("E-1");
    expect(parents.get(SHELF_BUG)).toBe(ROOT_ID);
    expect(parents.get("E-1")).toBeUndefined();
    expect(parents.has(SHELF_CHORE)).toBe(false);
  });
});

describe("visibleIds", () => {
  it("shows every epic and every non-empty shelf branch with nothing open", () => {
    expect([...visibleIds(view, new Set())]).toEqual(["E-1", "E-2", "E-3", SHELF_BUG]);
  });

  it("shows an open branch's children in wire order, and a bug only when its shelf is open", () => {
    expect([...visibleIds(view, new Set(["E-1", SHELF_BUG]))]).toEqual(["E-1", "F-1", "F-2", "E-2", "E-3", SHELF_BUG, "B-1"]);
    expect([...visibleIds(view, new Set(["E-1", "F-1"]))]).toEqual(["E-1", "F-1", "T-1", "F-2", "E-2", "E-3", SHELF_BUG]);
  });
});

describe("visibleAncestor", () => {
  it("maps a hidden task to its closed feature, a hidden bug to the Bugs branch", () => {
    expect(visibleAncestor(view, new Set(["E-1"]), "T-1")).toBe("F-1");
    expect(visibleAncestor(view, new Set(), "T-1")).toBe("E-1");
    expect(visibleAncestor(view, new Set(), "B-1")).toBe(SHELF_BUG);
    expect(visibleAncestor(view, new Set([SHELF_BUG]), "B-1")).toBe("B-1");
  });
});

// Which side of the root each first-level branch takes: the side its
// recorded relationships already sit on, unless that side has run too far
// ahead in rows, a crossing arc being worth three rows of imbalance.
describe("sidesOf", () => {
  const epic = (id: string, features: number): MapNodeView[] => [
    { noun: "epic", id, title: id, status: "draft" },
    ...Array.from({ length: features }, (_, i) => ({ noun: "feature", id: `${id}/F-${i}`, title: `${id} ${i}`, status: "draft", parent: id }) as MapNodeView),
  ];
  const dep = (from: string, to: string) => ({ from, to, kind: "depends_on" as const });

  it("puts the first epic on the right and an unlinked second one on the lighter side", () => {
    const sides = sidesOf({ nodes: [...epic("E-1", 2), ...epic("E-2", 2)], edges: [] });
    expect(sides.get("E-1")).toBe("right");
    expect(sides.get("E-2")).toBe("left");
  });

  it("puts an epic beside the one its features depend on", () => {
    // Three arcs into E-1 outweigh the three rows the right side would then
    // stand ahead by; one arc would not, and two equal epics joined by one
    // arc still take opposite sides.
    const sides = sidesOf({
      nodes: [...epic("E-1", 2), ...epic("E-2", 2), ...epic("E-3", 2)],
      edges: [dep("E-2/F-0", "E-1/F-1"), dep("E-2/F-1", "E-1/F-1"), dep("E-2/F-1", "E-1/F-0")],
    });
    expect(sides.get("E-2")).toBe("right");
    expect(sides.get("E-3")).toBe("left");
    const oneArc = sidesOf({ nodes: [...epic("E-1", 2), ...epic("E-2", 2)], edges: [dep("E-2/F-0", "E-1/F-1")] });
    expect(oneArc.get("E-2")).toBe("left");
  });

  it("lets a heavy side give up a weakly linked epic and keep a strongly linked one", () => {
    // E-1 stands nine rows tall on the right. One arc is not worth seating
    // E-2 under it; four arcs are worth seating E-3 there.
    const sides = sidesOf({
      nodes: [...epic("E-1", 8), ...epic("E-2", 1), ...epic("E-3", 1)],
      edges: [
        dep("E-2/F-0", "E-1/F-0"),
        dep("E-3/F-0", "E-1/F-0"),
        dep("E-3/F-0", "E-1/F-1"),
        dep("E-3/F-0", "E-1/F-2"),
        dep("E-3/F-0", "E-1/F-3"),
      ],
    });
    expect(sides.get("E-2")).toBe("left");
    expect(sides.get("E-3")).toBe("right");
  });

  it("seats the Bugs branch beside the epic its bugs are filed against", () => {
    const sides = sidesOf({
      nodes: [...epic("E-1", 2), ...epic("E-2", 2), { noun: "bug", id: "B-1", title: "Blank", status: "proposed" }],
      edges: [{ from: "B-1", to: "E-2/F-0", kind: "attached_to" }],
    });
    expect(sides.get("E-2")).toBe("left");
    expect(sides.get(SHELF_BUG)).toBe("left");
  });
});

describe("layoutMap: the tree", () => {
  it("puts the root at the world origin with a branch on either side of it", () => {
    const l = layoutMap(view, new Set());
    const root = l.boxes.get(ROOT_ID)!;
    expect(root.side).toBe("center");
    expect(root.x + root.w / 2).toBeCloseTo(0);
    expect(root.y + root.h / 2).toBeCloseTo(0);
    expect(l.boxes.get("E-1")!.x).toBeGreaterThan(root.x + root.w);
    expect(l.boxes.get("E-2")!.x + l.boxes.get("E-2")!.w).toBeLessThan(root.x);
  });

  it("leaves the root box exactly where it was when a branch is toggled", () => {
    // World coordinates used to be re-derived per layout, so every chevron
    // click slid the whole map under the reader: on this fixture the root
    // travelled 332px on one toggle, and on Acme Expand all pushed it off
    // the frame. The root's center IS the origin now, so nothing moves it.
    const closed = layoutMap(view, new Set()).boxes.get(ROOT_ID)!;
    const opened = layoutMap(view, new Set(["E-1"])).boxes.get(ROOT_ID)!;
    const deep = layoutMap(view, allOpen).boxes.get(ROOT_ID)!;
    expect(opened.x).toBe(closed.x);
    expect(opened.y).toBe(closed.y);
    expect(deep.x).toBe(closed.x);
    expect(deep.y).toBe(closed.y);
  });

  it("seats the shelf branches like any other root child, and a chore hangs on its branch", () => {
    // Three epics, then Bugs, then Chores: the shelves are root children like
    // any other and take the side sidesOf hands them. Here the bug is filed
    // against E-1's feature, so Bugs sits with E-1; the chore has no link to
    // anything and goes to the lighter side.
    const l = layoutMap(withChore, new Set());
    expect(l.boxes.get(SHELF_BUG)!.side).toBe(l.boxes.get("E-1")!.side);
    expect(l.boxes.get(SHELF_CHORE)!.side).toBe("left");
    expect(parentOf(withChore, "CH-1")).toBe(SHELF_CHORE);
    expect(containers(withChore).has(SHELF_CHORE)).toBe(true);
    // The chore itself hangs on its branch, drawn only while that branch is
    // open.
    expect(l.boxes.has("CH-1")).toBe(false);
    expect(layoutMap(withChore, new Set([SHELF_CHORE])).boxes.get("CH-1")!.side).toBe("left");
  });

  it("gives every root child the same side whatever is open", () => {
    const closed = layoutMap(withChore, new Set());
    const opened = layoutMap(withChore, new Set([...allOpen, SHELF_CHORE, "F-3"]));
    for (const id of ["E-1", "E-2", "E-3", SHELF_BUG, SHELF_CHORE]) {
      expect(opened.boxes.get(id)!.side).toBe(closed.boxes.get(id)!.side);
    }
  });

  it("stacks a side's branches vertically in order without overlap, centered on the root", () => {
    // E-1 and the Bugs branch share the right side (the bug is filed against
    // E-1's feature), in that wire order.
    const l = layoutMap(view, new Set());
    const e1 = l.boxes.get("E-1")!;
    const bugs = l.boxes.get(SHELF_BUG)!;
    expect(bugs.side).toBe(e1.side);
    expect(bugs.y).toBeGreaterThanOrEqual(e1.y + e1.h);
    const root = l.boxes.get(ROOT_ID)!;
    const mid = (e1.y + bugs.y + bugs.h) / 2;
    expect(Math.abs(mid - (root.y + root.h / 2))).toBeLessThan(1);
  });

  it("stacks an open epic's features beside it on the same side, the epic centered on the stack", () => {
    const l = layoutMap(view, new Set(["E-1"]));
    const e1 = l.boxes.get("E-1")!;
    const f1 = l.boxes.get("F-1")!;
    const f2 = l.boxes.get("F-2")!;
    expect(f1.side).toBe("right");
    expect(f1.x).toBeGreaterThanOrEqual(e1.x + e1.w);
    expect(f2.y).toBeGreaterThanOrEqual(f1.y + f1.h);
    const stackMid = (f1.y + f2.y + f2.h) / 2;
    expect(Math.abs(stackMid - (e1.y + e1.h / 2))).toBeLessThan(1);
    expect(f1.w).toBe(CARD.feature.w);
  });

  it("centers a short stack on a parent whose own card is the taller of the two", () => {
    // One feature under one epic: the epic's card is 40 tall, the feature's
    // 28, so the block is the epic's and the stack has 12px of slack in it.
    // Started at the block's top, the lone feature hung above its parent and
    // the connector left the epic's edge climbing.
    const lone: MapTreeView = {
      nodes: [
        { noun: "epic", id: "E-9", title: "Lone", status: "draft" },
        { noun: "feature", id: "F-9", title: "Only", status: "done", parent: "E-9" },
      ],
      edges: [],
    };
    const l = layoutMap(lone, new Set(["E-9"]));
    const e9 = l.boxes.get("E-9")!;
    const f9 = l.boxes.get("F-9")!;
    expect(f9.y + f9.h / 2).toBeCloseTo(e9.y + e9.h / 2);
  });

  it("mirrors a left-side branch's children to the left", () => {
    const l = layoutMap(view, new Set(["E-2"]));
    const e2 = l.boxes.get("E-2")!;
    const f3 = l.boxes.get("F-3")!;
    expect(f3.side).toBe("left");
    expect(f3.x + f3.w).toBeLessThanOrEqual(e2.x);
  });

  it("draws one branch connector per visible parent-child pair and nothing else at rest", () => {
    const l = layoutMap(view, new Set(["E-1"]));
    const pairs = l.branches.map((b) => `${b.from}>${b.to}`).sort();
    expect(pairs).toEqual([`${ROOT_ID}>E-1`, `${ROOT_ID}>E-2`, `${ROOT_ID}>E-3`, `${ROOT_ID}>${SHELF_BUG}`, "E-1>F-1", "E-1>F-2"].sort());
    expect(l.branches[0].d).toMatch(/^M [\d.-]+ [\d.-]+ C /);
  });

  it("lifts dependency arcs to the visible ancestor, draws a pair once, and drops a pair that collapses", () => {
    const closed = layoutMap(view, new Set());
    const closedPairs = closed.edges.map((e) => `${e.from}>${e.to}:${e.kind}`);
    // F-3 -> F-2 lifts to E-2 -> E-1; F-2 -> F-1 collapses inside E-1; the bug's edges lift to the shelf.
    expect(closedPairs).toContain("E-1>E-2:depends_on");
    expect(closedPairs.filter((p) => p.startsWith("E-1>E-1"))).toHaveLength(0);
    expect(closedPairs).toContain(`${SHELF_BUG}>E-1:attached_to`);
    expect(closedPairs).toContain(`${SHELF_BUG}>E-1:resolved_by`);
    const open = layoutMap(view, allOpen);
    const openPairs = open.edges.map((e) => `${e.from}>${e.to}:${e.kind}`);
    expect(openPairs).toContain("F-1>F-2:depends_on");
    expect(openPairs).toContain("B-1>T-1:resolved_by");
    expect(open.edges[0].d).toMatch(/^M [\d.-]+ [\d.-]+ C /);
  });

  it("reports bounds that cover every box with padding, in the same world the boxes are in", () => {
    // The boxes sit around the origin, so the bounds are a rectangle in world
    // coordinates rather than a size read off zero: a fit reads x and y from
    // them, not just w and h.
    const l = layoutMap(view, allOpen);
    for (const b of l.boxes.values()) {
      expect(b.x).toBeGreaterThanOrEqual(l.bounds.x);
      expect(b.y).toBeGreaterThanOrEqual(l.bounds.y);
      expect(b.x + b.w).toBeLessThanOrEqual(l.bounds.x + l.bounds.w);
      expect(b.y + b.h).toBeLessThanOrEqual(l.bounds.y + l.bounds.h);
    }
    // width and height stay the bounds' size, which is what every caller that
    // only ever asked "how big is this" already meant.
    expect(l.width).toBe(l.bounds.w);
    expect(l.height).toBe(l.bounds.h);
    const empty = layoutMap({ nodes: [], edges: [] }, new Set());
    expect(empty.boxes.size).toBe(1);
    expect(empty.bounds.w).toBeGreaterThan(0);
    expect(empty.bounds.h).toBeGreaterThan(0);
    // One card centered on the origin, padded on all four sides.
    expect(empty.bounds.x).toBeCloseTo(-(CARD.root.w / 2 + 28));
    expect(empty.bounds.y).toBeCloseTo(-(CARD.root.h / 2 + 28));
  });

  it("omits a shelf branch when the shelf is empty", () => {
    const noShelf: MapTreeView = { nodes: view.nodes.filter((n) => n.noun !== "bug"), edges: [] };
    const l = layoutMap(noShelf, new Set());
    expect(l.boxes.has(SHELF_BUG)).toBe(false);
    expect(l.boxes.has(SHELF_CHORE)).toBe(false);
  });

  // arcPath routes by where the two cards sit, and every path is one cubic
  // whose ends leave and arrive horizontally, so the arrowhead at the end
  // points into a card's side rather than down onto its roof.
  describe("arcPath", () => {
    const box = (id: string, x: number, y: number, side: "left" | "right") => ({ id, noun: "feature", x, y, w: 200, h: 20, side });
    const cubic = (d: string) => {
      const m = d.match(/^M (\S+) (\S+) C (\S+) (\S+), (\S+) (\S+), (\S+) (\S+)$/);
      if (m === null) throw new Error(`not one cubic: ${d}`);
      const [x1, y1, c1x, c1y, c2x, c2y, x2, y2] = m.slice(1).map(Number);
      return { x1, y1, c1x, c1y, c2x, c2y, x2, y2 };
    };

    it("brackets two cards in one column on their outer edge, right side", () => {
      const a = box("a", 400, 0, "right");
      const b = box("b", 400, 100, "right");
      const c = cubic(arcPath(a, b));
      expect([c.x1, c.y1, c.x2, c.y2]).toEqual([600, 10, 600, 110]);
      expect(c.c1x).toBeGreaterThan(600);
      expect(c.c2x).toBeGreaterThan(600);
      expect([c.c1y, c.c2y]).toEqual([10, 110]);
    });

    it("brackets a left-side column on its left edge, and cards that merely overlap in x count as one column", () => {
      const a = box("a", -600, 0, "left");
      const b = { ...box("b", -560, 100, "left"), w: 216 };
      const c = cubic(arcPath(a, b));
      expect([c.x1, c.y1, c.x2, c.y2]).toEqual([-600, 10, -560, 110]);
      expect(c.c1x).toBeLessThan(-600);
      expect(c.c2x).toBeLessThan(-560);
    });

    it("bulges a bracket further the longer it spans, up to a cap", () => {
      const a = box("a", 400, 0, "right");
      const near = cubic(arcPath(a, box("b", 400, 32, "right")));
      const far = cubic(arcPath(a, box("c", 400, 300, "right")));
      const farther = cubic(arcPath(a, box("d", 400, 3000, "right")));
      expect(far.c1x).toBeGreaterThan(near.c1x);
      expect(farther.c1x).toEqual(far.c1x);
    });

    it("runs between the facing edges of two columns on the same side", () => {
      const a = box("a", 400, 0, "right");
      const b = box("b", 800, 100, "right");
      const out = cubic(arcPath(a, b));
      expect([out.x1, out.y1, out.x2, out.y2]).toEqual([600, 10, 800, 110]);
      expect([out.c1y, out.c2y]).toEqual([10, 110]);
      expect(out.c1x).toBeGreaterThan(600);
      expect(out.c2x).toBeLessThan(800);
      // And back the other way: from the outer card's inner edge to the
      // inner card's outer edge.
      const back = cubic(arcPath(b, a));
      expect([back.x1, back.x2]).toEqual([800, 600]);
    });

    it("crosses sides from inner edge to inner edge through the middle", () => {
      const a = box("a", 400, 0, "right");
      const b = box("b", -600, 100, "left");
      const c = cubic(arcPath(a, b));
      expect([c.x1, c.y1, c.x2, c.y2]).toEqual([400, 10, -400, 110]);
      expect([c.c1y, c.c2y]).toEqual([10, 110]);
      expect(c.c1x).toBeLessThan(400);
      expect(c.c2x).toBeGreaterThan(-400);
    });
  });
});
