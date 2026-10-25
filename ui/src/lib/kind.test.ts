// kindLabel is the word the node header's kind pill reads (node header pass,
// 2026-08-30): one per noun the console draws, and a capitalized fallback for
// a noun this build has never heard of, so the pill can never be blank.
import { describe, expect, it } from "vitest";
import { kindLabel } from "./kind";

describe("kindLabel", () => {
  it.each([
    ["epic", "Epic"],
    ["feature", "Feature"],
    ["task", "Task"],
    ["bug", "Bug"],
    ["chore", "Chore"],
    ["question", "Question"],
    ["amendment", "Amendment"],
  ])("names %s as %s", (noun, label) => {
    expect(kindLabel(noun)).toBe(label);
  });

  it("capitalizes a noun it does not know rather than going blank", () => {
    expect(kindLabel("decision")).toBe("Decision");
  });
});
