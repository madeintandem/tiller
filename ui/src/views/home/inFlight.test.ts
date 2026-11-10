import { describe, expect, it } from "vitest";
import { featureKeyOf, groupByFeature, taskKeyTail } from "./inFlight";

const row = (id: string, featureId?: string, featureTitle?: string) => ({
  id,
  feature_id: featureId,
  feature_title: featureTitle,
});

describe("groupByFeature", () => {
  it("joins items of one feature into the first group seen, adjacent or not", () => {
    const groups = groupByFeature([
      row("a", "F-1", "One"),
      row("b", "F-2", "Two"),
      row("c", "F-1", "One"),
    ]);
    expect(groups.map((g) => [g.featureId, g.items.map((i) => i.id)])).toEqual([
      ["F-1", ["a", "c"]],
      ["F-2", ["b"]],
    ]);
    expect(groups[0].featureTitle).toBe("One");
  });

  it("keeps groups in first-seen order and items in input order", () => {
    const groups = groupByFeature([row("a", "F-2", "Two"), row("b", "F-1", "One"), row("c", "F-2", "Two")]);
    expect(groups.map((g) => g.featureId)).toEqual(["F-2", "F-1"]);
    expect(groups[0].items.map((i) => i.id)).toEqual(["a", "c"]);
  });

  it("puts items with no feature in one group of their own, in place", () => {
    const groups = groupByFeature([row("a", "F-1", "One"), row("b"), row("c", "F-1", "One"), row("d")]);
    expect(groups.map((g) => [g.featureId, g.featureTitle, g.items.map((i) => i.id)])).toEqual([
      ["F-1", "One", ["a", "c"]],
      [null, null, ["b", "d"]],
    ]);
  });

  it("returns no groups for no items", () => {
    expect(groupByFeature([])).toEqual([]);
  });
});

describe("feature and task key helpers", () => {
  it("names the feature key as the task key without its last segment", () => {
    expect(featureKeyOf("reviewed-audits/selection-and-queues/pick-rows")).toBe("reviewed-audits/selection-and-queues");
    expect(featureKeyOf("pick-rows")).toBeUndefined();
    expect(featureKeyOf(undefined)).toBeUndefined();
  });

  it("shortens a task key to its last segment", () => {
    expect(taskKeyTail("reviewed-audits/selection-and-queues/pick-rows")).toBe("pick-rows");
    expect(taskKeyTail("pick-rows")).toBe("pick-rows");
  });
});
