// index.html links the console's favicon: a row of tabs is
// easier to pick out with a mark than with the browser's default globe. Read
// from the literal source the same way theme.indexHtml.test.ts does.

import { describe, expect, it } from "vitest";

import indexHtml from "../../index.html?raw";

describe("index.html links the favicon", () => {
  it("names /favicon.svg as an SVG icon", () => {
    expect(indexHtml).toMatch(/<link rel="icon" type="image\/svg\+xml" href="\/favicon\.svg"\s*\/?>/);
  });
});
