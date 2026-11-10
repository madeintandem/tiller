import { describe, expect, it } from "vitest";

import { displayStatus } from "./displayStatus";

describe("displayStatus", () => {
  it("draws a feature's board column over its stored status", () => {
    expect(displayStatus({ status: "ready", column: "active" })).toBe("active");
  });

  it("falls back to the stored status when no column is served", () => {
    expect(displayStatus({ status: "in_review" })).toBe("in_review");
  });
});
