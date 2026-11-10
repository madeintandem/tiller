import { describe, expect, it } from "vitest";

import { displayStatus } from "./displayStatus";

describe("displayStatus", () => {
  it("draws a feature's board column over its stored status", () => {
    expect(displayStatus({ status: "ready", column: "active" })).toBe("active");
  });

  it("falls back to the stored status when no column is served", () => {
    expect(displayStatus({ status: "in_review" })).toBe("in_review");
  });

  it("takes a node whose status is optional, as a relationship vertex's is", () => {
    expect(displayStatus({ status: "ready", column: "active" })).toBe("active");
    expect(displayStatus({ column: "active" })).toBe("active");
    expect(displayStatus({ status: "in_review" })).toBe("in_review");
    expect(displayStatus({})).toBe("");
  });
});
