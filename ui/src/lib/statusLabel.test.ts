import { describe, expect, it } from "vitest";

import { statusLabel } from "./statusLabel";
import vocabulary from "./vocabulary.json";

describe("statusLabel", () => {
  it.each(vocabulary.statuses)("reads %s as plain sentence-case words", (status) => {
    expect(statusLabel(status)).toMatch(/^[A-Z][a-z]+( [a-z]+)*$/);
  });

  it("applies the named overrides", () => {
    expect(statusLabel("specced")).toBe("Specified");
    expect(statusLabel("needs_attestation")).toBe("Needs attestation");
    expect(statusLabel("in_review")).toBe("In review");
    expect(statusLabel("in_progress")).toBe("In progress");
  });

  it("falls back to sentence case for a status it has not seen", () => {
    expect(statusLabel("some_new_state")).toBe("Some new state");
  });
});
