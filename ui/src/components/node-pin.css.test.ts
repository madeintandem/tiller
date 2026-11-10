// Pins the node page's pinned tab rail and one-line header, layout facts
// jsdom cannot observe.

import { describe, expect, it } from "vitest";

import pinCss from "./node-pin.css?raw";

describe("node pin", () => {
  it("sticks the rail above the reading pane's own 22px top padding", () => {
    const rule = pinCss.match(/\.node-pin\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(rule).toMatch(/position:\s*sticky/);
    expect(rule).toMatch(/top:\s*-22px/);
    expect(rule).toMatch(/z-index:\s*2/);
    expect(rule).toMatch(/background:\s*var\(--paper\)/);
  });

  it("keeps the compact header on one non-wrapping line with an ellipsized title", () => {
    const row = pinCss.match(/\.node-compact\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(row).toMatch(/display:\s*flex/);
    expect(row).toMatch(/flex-wrap:\s*nowrap/);
    const title = pinCss.match(/\.node-compact-title\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(title).toMatch(/text-overflow:\s*ellipsis/);
    expect(title).toMatch(/white-space:\s*nowrap/);
    expect(title).toMatch(/overflow:\s*hidden/);
  });
});
