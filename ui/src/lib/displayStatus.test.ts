import { describe, expect, it } from "vitest";

import { displayStatus } from "./displayStatus";

describe("displayStatus", () => {
  it("draws a feature's board column over its stored status", () => {
    expect(displayStatus({ status: "ready", column: "active" })).toBe("active");
  });

  it("falls back to the stored status when no column is served", () => {
    expect(displayStatus({ status: "in_review" })).toBe("in_review");
  });

  // A member of a live batch is approved work its batch is doing, so it reads
  // batched and never ready. A proposed member still waits on triage, and a
  // crossed one is done, so both keep their stored status.
  it("reads a ready member of a live batch as batched", () => {
    expect(displayStatus({ status: "ready", activity: "batched" })).toBe("batched");
    expect(displayStatus({ status: "proposed", activity: "batched" })).toBe("proposed");
    expect(displayStatus({ status: "done", activity: "batched" })).toBe("done");
    expect(displayStatus({ status: "ready", activity: "carried" })).toBe("ready");
  });

  it("takes a node whose status is optional, as a relationship vertex's is", () => {
    expect(displayStatus({ status: "ready", column: "active" })).toBe("active");
    expect(displayStatus({ column: "active" })).toBe("active");
    expect(displayStatus({ status: "in_review" })).toBe("in_review");
    expect(displayStatus({})).toBe("");
  });
});
