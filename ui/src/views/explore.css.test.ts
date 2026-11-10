// Pins the navigator's two-line task title (Task 17). jsdom has no layout
// engine, so the wrap is read from the raw stylesheet, the pattern the other
// css tests use. The rule lives in explore-nav.css because explore.css is
// past the file-length ceiling.

import { describe, expect, it } from "vitest";

import css from "./explore-nav.css?raw";

describe("explore-nav.css: .explore-task-title", () => {
  const rule = /\.explore-task-title\s*\{([^}]*)\}/.exec(css)?.[1] ?? "";

  it("clamps to two lines and never forces one", () => {
    expect(rule).toMatch(/-webkit-line-clamp:\s*2/);
    expect(rule).not.toMatch(/white-space:\s*nowrap/);
  });
});
