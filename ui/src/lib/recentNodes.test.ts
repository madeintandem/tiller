import { afterEach, describe, expect, it, vi } from "vitest";
import { readRecent, recentKey, recordRecent } from "./recentNodes";

afterEach(() => {
  window.localStorage.clear();
  vi.restoreAllMocks();
});

const node = (n: number) => ({ noun: "task", id: `T-${n}`, title: `Task ${n}` });

describe("recentNodes", () => {
  it("reads an empty list when nothing is stored", () => {
    expect(readRecent("acme")).toEqual([]);
  });

  it("records newest first under a per-project key", () => {
    recordRecent("acme", node(1));
    recordRecent("acme", node(2));
    expect(readRecent("acme").map((r) => r.id)).toEqual(["T-2", "T-1"]);
    expect(window.localStorage.getItem("tiller-explore-recent:acme")).not.toBeNull();
    expect(recentKey("acme")).toBe("tiller-explore-recent:acme");
    expect(readRecent("other")).toEqual([]);
  });

  it("moves a revisited node to the front without duplicating it, taking the newer title", () => {
    recordRecent("acme", node(1));
    recordRecent("acme", node(2));
    recordRecent("acme", { ...node(1), title: "Renamed" });
    expect(readRecent("acme")).toEqual([
      { noun: "task", id: "T-1", title: "Renamed" },
      { noun: "task", id: "T-2", title: "Task 2" },
    ]);
  });

  it("keeps an optional key", () => {
    recordRecent("acme", { noun: "feature", id: "F-1", key: "pay/checkout", title: "Checkout" });
    expect(readRecent("acme")[0].key).toBe("pay/checkout");
  });

  it("caps the list at eight", () => {
    for (let n = 1; n <= 10; n++) recordRecent("acme", node(n));
    const ids = readRecent("acme").map((r) => r.id);
    expect(ids).toHaveLength(8);
    expect(ids[0]).toBe("T-10");
    expect(ids[7]).toBe("T-3");
  });

  it.each([
    ["corrupt JSON", "{nope"],
    ["a non-array", '{"id":"x"}'],
    ["a wrong-shaped entry", '[{"noun":"task","id":"T-1","title":"ok"},{"id":3}]'],
  ])("reads an empty list from %s", (_name, raw) => {
    window.localStorage.setItem(recentKey("acme"), raw);
    expect(readRecent("acme")).toEqual([]);
  });

  it("recovers by overwriting a corrupt value", () => {
    window.localStorage.setItem(recentKey("acme"), "{nope");
    recordRecent("acme", node(1));
    expect(readRecent("acme")).toHaveLength(1);
  });

  it("never throws when storage does", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(readRecent("acme")).toEqual([]);
    expect(() => recordRecent("acme", node(1))).not.toThrow();
  });
});
