import { describe, expect, it } from "vitest";

import { evidenceLabel } from "./evidenceLabel";

describe("evidenceLabel", () => {
  it("names the criterion when one is known", () => {
    expect(evidenceLabel({ ref: "artifacts/x.txt", kind: "rubric_output", criterion: "C1" })).toBe("eval C1 output");
  });

  it("names the kind in plain words when only a kind is known", () => {
    expect(evidenceLabel({ ref: "artifacts/x.txt", kind: "command_output" })).toBe("command output evidence");
    expect(evidenceLabel({ ref: "artifacts/x.txt", kind: "code_review" })).toBe("code review evidence");
    expect(evidenceLabel({ ref: "artifacts/x.txt", kind: "test" })).toBe("test evidence");
  });

  it("falls back to a bare label", () => {
    expect(evidenceLabel({ ref: "artifacts/x.txt" })).toBe("evidence");
    expect(evidenceLabel({ ref: "artifacts/x.txt", kind: "" })).toBe("evidence");
  });
});
