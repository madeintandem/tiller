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

// The topbar must stay one row at the supported 1024px minimum whatever the
// project key's length; jsdom cannot lay it out, so the rules are pinned.
describe("project key in the topbar", () => {
  const rule = stylesCss.match(/\.project-key\s*\{([^}]*)\}/)?.[1] ?? "";

  it("truncates with an ellipsis instead of growing the row", () => {
    expect(rule).toMatch(/overflow:\s*hidden/);
    expect(rule).toMatch(/text-overflow:\s*ellipsis/);
    expect(rule).toMatch(/white-space:\s*nowrap/);
    expect(rule).toMatch(/min-width:\s*0/);
  });

  it("is capped by the room the row leaves it, measured from the row itself", () => {
    expect(rule).toMatch(/max-width:[^;]*cqw/);
    expect(stylesCss).toMatch(/\.header-row\s*\{[^}]*container-type:\s*inline-size/);
  });
});
