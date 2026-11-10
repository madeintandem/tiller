// Vitest coverage for the comma-idiom phase suffix (Task 8), in
// gateDisplay.test.ts's idiom: the direct analog for a small, total display
// derivation with no companion component to exercise it through.

import { describe, expect, it } from "vitest";

import { withActivityPhrase } from "./activityPhrase";

describe("withActivityPhrase", () => {
  it("appends the awaiting-check phase, comma idiom", () => {
    expect(withActivityPhrase({ status: "active", activity: "awaiting_check" })).toBe(
      "Active, awaiting check",
    );
  });

  it("appends the awaiting-sync phase, comma idiom", () => {
    expect(withActivityPhrase({ status: "active", activity: "awaiting_sync" })).toBe(
      "Active, awaiting sync",
    );
  });

  it("falls through to the bare status when activity is absent", () => {
    expect(withActivityPhrase({ status: "draft" })).toBe("Draft");
  });

  it("falls through to the bare status when activity is empty", () => {
    expect(withActivityPhrase({ status: "specced", activity: "" })).toBe("Specified");
  });

  it("falls through to the bare status for an epic's plain active/idle activity", () => {
    // An epic's derived working status rides the same wire field
    // (gen.EpicActivity) but is never one of the two feature phases, so it
    // must never grow a suffix here.
    expect(withActivityPhrase({ status: "ready", activity: "active" })).toBe("Ready");
    expect(withActivityPhrase({ status: "ready", activity: "idle" })).toBe("Ready");
  });

  it("names a node's board column over its stored status", () => {
    expect(withActivityPhrase({ status: "ready", column: "active" })).toBe("Active");
  });

  it("never shows a wire word", () => {
    expect(withActivityPhrase({ status: "in_progress" })).toBe("In progress");
  });
});
