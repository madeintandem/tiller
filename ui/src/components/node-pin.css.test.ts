// Pins the node page's pinned tab rail and one-line header, layout facts
// jsdom cannot observe. The browser measurement that motivates them (no
// jump when the compact header shows) is in the walk fix report; these pins
// keep the rules that make it hold.

import { describe, expect, it } from "vitest";

import pinCss from "./node-pin.css?raw";

function rule(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&").replace(/\s+/g, "\\s*");
  return pinCss.match(new RegExp(`(?:^|\\n)${escaped}\\s*\\{([^}]*)\\}`))?.[1] ?? "";
}

describe("node pin", () => {
  it("sticks the rail one compact band below the pane's top edge, opaque", () => {
    const pin = rule(".node-pin");
    expect(pin).toMatch(/position:\s*sticky/);
    // The pane (.explore-detail) has 22px of top padding the sticky offset
    // is measured from; the band is the compact header's reserved height.
    expect(pin).toMatch(/top:\s*calc\(var\(--node-pin-band\)\s*-\s*22px\)/);
    expect(pin).toMatch(/z-index:\s*2/);
    expect(pin).toMatch(/background:\s*var\(--paper\)/);
  });

  it("keeps an opaque gap under the rail so scrolled text is not cut against it", () => {
    expect(rule(".node-pin")).toMatch(/padding-bottom:\s*8px/);
  });

  it("overlays the compact header in the band above the rail, out of flow", () => {
    const row = rule(".node-compact");
    expect(row).toMatch(/position:\s*absolute/);
    expect(row).toMatch(/bottom:\s*100%/);
    expect(row).toMatch(/height:\s*var\(--node-pin-band\)/);
    expect(row).toMatch(/background:\s*var\(--paper\)/);
  });

  it("places the sentinel one band above the rail, out of flow", () => {
    expect(rule(".node-pinned")).toMatch(/position:\s*relative/);
    const sentinel = rule(".node-pin-sentinel");
    expect(sentinel).toMatch(/position:\s*absolute/);
    expect(sentinel).toMatch(/top:\s*calc\(-1\s*\*\s*var\(--node-pin-band\)\)/);
  });

  it("keeps the compact header on one non-wrapping line with an ellipsized title", () => {
    const row = rule(".node-compact");
    expect(row).toMatch(/display:\s*flex/);
    expect(row).toMatch(/flex-wrap:\s*nowrap/);
    const title = rule(".node-compact-title");
    expect(title).toMatch(/text-overflow:\s*ellipsis/);
    expect(title).toMatch(/white-space:\s*nowrap/);
    expect(title).toMatch(/overflow:\s*hidden/);
  });
});
