// Vitest coverage for sentence (lib/sentence.ts), the console's copy of the
// server's views.Sentence (internal/views/next_text.go). The table is the Go
// test's own (TestSentenceLeavesALeadingKeyAlone), so a ranked next reason
// reads the same on Home as it does in `tiller next`.

import { describe, expect, it } from "vitest";

import { sentence } from "./sentence";

describe("sentence", () => {
  it.each([
    ["no work is planned yet", "No work is planned yet."],
    ["in review, waiting on your merge.", "In review, waiting on your merge."],
    ["checkout/alpha/t1 (T-x) is ready", "checkout/alpha/t1 (T-x) is ready."],
    ["rides-alone review", "rides-alone review."],
    ["2 more remain..", "2 more remain."],
    ["holds it; Which schema?", "Holds it; Which schema?"],
    ["really!", "Really!"],
    ["   ", ""],
  ])("words %j as %j", (input, want) => {
    expect(sentence(input)).toBe(want);
  });

  it("never alters a leading token that opens with a parenthesis", () => {
    expect(sentence("(T-x) waits")).toBe("(T-x) waits.");
  });
});
