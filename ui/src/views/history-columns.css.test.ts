// Pins the History row columns from the browser walk of batches D to F
// (E2): the toggle column holds one fixed width on every row, so "who" sits
// at one x, and below 48rem every row wraps one way, the title staying on the
// first line and "who" dropping beneath it. jsdom has no layout engine, so
// the raw rules are pinned here and the geometry was measured in a browser.

import { describe, expect, it } from "vitest";

import historyCss from "./history.css?raw";

describe("history.css: one toggle column on every row", () => {
  it("gives .burst-toggle a fixed width", () => {
    expect(historyCss).toMatch(/\.burst-toggle\s*\{[^}]*flex:\s*none;[^}]*width:\s*7ch;/);
  });
});

describe("history.css: one wrap shape below 48rem", () => {
  const media = historyCss.slice(historyCss.indexOf("@media (max-width: 48rem)"));

  it("keeps the title cell on the first line from a zero basis", () => {
    expect(media).toMatch(/\.hrow > \.what\s*\{\s*flex:\s*1 1 0;/);
  });

  it("drops who to a line beneath, beside the toggle column", () => {
    expect(media).toMatch(/\.hrow > \.who\s*\{[^}]*flex:\s*1 0 calc\(100% - 7ch/);
  });
});
