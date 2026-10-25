// Dependency reach is computed on node identities, then projected onto the
// visible cards. Collapsing a branch must not connect unrelated siblings.
import { describe, expect, it } from "vitest";
import type { MapTreeView } from "../../../api";
import { chainOf } from "./chain";
import { SHELF_BUG } from "./layout";

const view: MapTreeView = {
  nodes: [
    { noun: "epic", id: "E-1", title: "A", status: "ready" },
    { noun: "feature", id: "F-1", title: "a1", status: "ready", parent: "E-1" },
    { noun: "feature", id: "F-2", title: "a2", status: "draft", parent: "E-1" },
    { noun: "epic", id: "E-2", title: "B", status: "draft" },
    { noun: "feature", id: "F-3", title: "b1", status: "draft", parent: "E-2" },
    { noun: "feature", id: "F-4", title: "b2", status: "draft", parent: "E-2" },
    { noun: "bug", id: "B-1", title: "bug", status: "proposed" },
  ],
  edges: [
    { from: "F-2", to: "F-1", kind: "depends_on" },
    { from: "F-3", to: "F-2", kind: "depends_on" },
    { from: "F-4", to: "F-3", kind: "depends_on" },
    { from: "B-1", to: "F-2", kind: "attached_to" },
    { from: "B-1", to: "F-1", kind: "resolved_by" },
  ],
};

describe("chainOf", () => {
  it("does not borrow edges from siblings of a hidden selection or an intermediate endpoint", () => {
    const graph: MapTreeView = {
      nodes: [
        ...view.nodes,
        { noun: "epic", id: "E-3", title: "C", status: "draft" },
        { noun: "feature", id: "F-5", title: "c1", status: "draft", parent: "E-3" },
      ],
      edges: [
        { from: "F-1", to: "F-3", kind: "depends_on" },
        { from: "F-2", to: "F-4", kind: "depends_on" },
        { from: "F-4", to: "F-5", kind: "depends_on" },
        { from: "B-1", to: "F-2", kind: "attached_to" },
      ],
    };
    const chain = chainOf(graph, new Set(), "F-1");
    expect([...chain.up]).toEqual(["E-2"]);
    expect([...chain.near]).toEqual(["E-2"]);
    expect([...chain.down]).toEqual([]);
    expect([...chain.linked]).toEqual([]);
  });

  it("walks dependencies transitively in both directions, excluding the node itself", () => {
    const c = chainOf(view, new Set(["E-1", "E-2"]), "F-2");
    expect([...c.up].sort()).toEqual(["F-1"]);
    expect([...c.down].sort()).toEqual(["F-3", "F-4"]);
  });

  it("names the direct dependencies and dependents apart from the rest of the chain", () => {
    // near is what the canvas lights and draws arcs for; up and down are
    // the transitive reach that merely stays undimmed.
    const c = chainOf(view, new Set(["E-1", "E-2"]), "F-2");
    expect([...c.near].sort()).toEqual(["F-1", "F-3"]);
    expect(c.near.has("F-4")).toBe(false);
    // Lifted to the visible card like everything else: with E-2 shut, the
    // dependent next door is the epic.
    expect([...chainOf(view, new Set(["E-1"]), "F-2").near].sort()).toEqual(["E-2", "F-1"]);
  });

  it("lights a closed container when the chain passes through it", () => {
    const c = chainOf(view, new Set(["E-1"]), "F-2");
    expect([...c.down].sort()).toEqual(["E-2"]);
  });

  it("does not follow attached_to or resolved_by", () => {
    const c = chainOf(view, new Set(["E-1", "E-2"]), "F-2");
    expect(c.down.has("B-1")).toBe(false);
    expect(c.up.has("B-1")).toBe(false);
  });

  it("is empty for a node with no chain", () => {
    const c = chainOf(view, new Set(["E-1", "E-2"]), "B-1");
    expect(c.up.size).toBe(0);
    expect(c.down.size).toBe(0);
  });

  it("names the far end of an attached_to or a resolved_by, in either direction", () => {
    // A bug is not on anybody's critical path, so its edges are still not
    // followed. But a selected bug that lights no card at all reads as a
    // node with nothing to do with the project it was filed against: the two
    // nodes at the other end of its own edges are named so the canvas can
    // draw them and leave them undimmed.
    const open = new Set(["E-1", "E-2", SHELF_BUG]);
    expect([...chainOf(view, open, "B-1").linked].sort()).toEqual(["F-1", "F-2"]);
    expect([...chainOf(view, open, "F-2").linked]).toEqual(["B-1"]);
    expect([...chainOf(view, open, "F-4").linked]).toEqual([]);
  });

  it("lifts a shelf edge's far end to the card standing for it", () => {
    // With the Bugs branch shut, the bug's own edges belong to the branch
    // card, exactly as the drawn arcs do.
    const c = chainOf(view, new Set(["E-1", "E-2"]), "F-2");
    expect([...c.linked]).toEqual([SHELF_BUG]);
  });
});
