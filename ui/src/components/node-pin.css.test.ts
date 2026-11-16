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

  it("covers the reading pane's top edge from outside the pane, so no row of scrolled content shows above the band", () => {
    // Safari paints one device-pixel row of scrolled content at the pane's
    // top edge when that edge lands on a fractional pixel (B-un831z). A cover
    // inside the pane did not hide it, so this one belongs to the split.
    const media = pinCss.match(/@media \(width > 48rem\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
    const wide = (selector: string) => {
      const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      return media.match(new RegExp(`${escaped}\\s*\\{([^}]*)\\}`))?.[1] ?? "";
    };
    expect(wide(".explore-split")).toMatch(/position:\s*relative/);
    const cover = wide(".explore-split::before");
    expect(cover).toMatch(/content:\s*""/);
    expect(cover).toMatch(/position:\s*absolute/);
    // It starts a pixel above the edge and ends a pixel below it, so the
    // partial row is covered whichever way the edge rounds.
    expect(cover).toMatch(/top:\s*-1px/);
    expect(cover).toMatch(/height:\s*2px/);
    expect(cover).toMatch(/background:\s*var\(--paper\)/);
    expect(cover).toMatch(/z-index:\s*3/);
    expect(cover).toMatch(/pointer-events:\s*none/);
    // The divider's seam runs up through the same pixels and stays on top.
    const divider = wide(".explore-divider");
    expect(divider).toMatch(/position:\s*relative/);
    expect(divider).toMatch(/z-index:\s*4/);
  });
});
