// The map's per-project memory (docs/plans/2026-08-30-explore-map-design.md,
// "Persistence"): which cards are open and where the viewport sits, keyed
// by project so two projects never share a picture; a restored open set is
// reconciled against the live tree so a removed node never lingers.
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { MapTreeView } from "../../../api";
import { mapStorageKey, readMapState, reconcileOpen, writeMapState } from "./mapStorage";

const view: MapTreeView = {
  nodes: [
    { noun: "epic", id: "E-1", title: "A", status: "ready" },
    { noun: "feature", id: "F-1", title: "a1", status: "ready", parent: "E-1" },
  ],
  edges: [],
};

describe("mapStorage", () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => localStorage.clear());

  it("keys by project", () => {
    expect(mapStorageKey("lantern")).toBe("tiller.map.lantern");
  });

  it("round-trips the open set and the viewport", () => {
    writeMapState("lantern", { open: ["E-1"], viewport: { x: 10, y: 20, k: 1.5 } });
    expect(readMapState("lantern")).toEqual({ open: ["E-1"], viewport: { x: 10, y: 20, k: 1.5 } });
    expect(readMapState("other")).toBeNull();
  });

  it("keeps an open set stored with no viewport", () => {
    writeMapState("lantern", { open: ["E-1"], viewport: null });
    expect(readMapState("lantern")).toEqual({ open: ["E-1"], viewport: null });
  });

  it("stamps the schema it wrote and refuses a record that predates it", () => {
    // Build 1's viewports were measured in a world whose origin moved with
    // the open set, so a stored one now names a place that does not exist.
    // An unmarked record reads as nothing at all, which gets that reader the
    // first-visit fit instead of a window onto empty paper.
    writeMapState("lantern", { open: ["E-1"], viewport: { x: 10, y: 20, k: 1.5 } });
    expect(JSON.parse(localStorage.getItem("tiller.map.lantern")!).v).toBe(2);
    localStorage.setItem("tiller.map.lantern", JSON.stringify({ open: ["E-1"], viewport: { x: 10, y: 20, k: 1.5 } }));
    expect(readMapState("lantern")).toBeNull();
    localStorage.setItem("tiller.map.lantern", JSON.stringify({ v: 1, open: ["E-1"], viewport: null }));
    expect(readMapState("lantern")).toBeNull();
  });

  it("reads bad JSON as nothing rather than throwing", () => {
    localStorage.setItem("tiller.map.lantern", "{not json");
    expect(readMapState("lantern")).toBeNull();
  });

  it("reconciles a stored open set against the live tree, keeping only containers that still exist", () => {
    expect([...reconcileOpen(["E-1", "E-gone", "F-1"], view)]).toEqual(["E-1"]);
  });
});
