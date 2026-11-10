// Pins the ledgers' fixed header: on a desktop-width split the pane holds its
// title and status rail still and only the tab panel scrolls, so no list row
// ever passes under a pinned element (Safari left a one-pixel line of rows
// showing above the sticky rail this replaced).

import { describe, expect, it } from "vitest";

import ledgerCss from "./explore-ledgers.css?raw";

function wideRule(selector: string): string {
  const media = ledgerCss.match(/@media \(width > 48rem\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&").replace(/\s+/g, "\\s*");
  return media.match(new RegExp(`${escaped}\\s*\\{([^}]*)\\}`))?.[1] ?? "";
}

describe("ledger panes", () => {
  it("stack the header, rail, and panel in a pane that does not scroll itself", () => {
    const pane = wideRule(".explore-detail.explore-ledger");
    expect(pane).toMatch(/display:\s*flex/);
    expect(pane).toMatch(/flex-direction:\s*column/);
    expect(pane).toMatch(/overflow-y:\s*hidden/);
  });

  it("scroll only the tab panel, which fills the rest of the pane", () => {
    const panel = wideRule('.explore-ledger > [role="tabpanel"]');
    expect(panel).toMatch(/flex:\s*1/);
    expect(panel).toMatch(/min-height:\s*0/);
    expect(panel).toMatch(/overflow-y:\s*auto/);
  });

  it("keep the rail and header from shrinking", () => {
    expect(wideRule(".explore-ledger > *")).toMatch(/flex:\s*none/);
  });
});
