// Pins the topbar to the viewport, a layout fact jsdom cannot observe: Home,
// Report, and History scroll the page, and the nav stays in reach on a long
// one.

import { describe, expect, it } from "vitest";

import stylesCss from "./styles.css?raw";

describe("site header", () => {
  it("sticks to the top of the viewport", () => {
    const rule = stylesCss.match(/\.site-header\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(rule).toMatch(/position:\s*sticky/);
    expect(rule).toMatch(/top:\s*0/);
    expect(rule).toMatch(/z-index:/);
  });
});
