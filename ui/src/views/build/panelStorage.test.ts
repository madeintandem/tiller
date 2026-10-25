// Vitest coverage for the epic panel's collapsed-state persistence
// (task-13-brief.md: "Panel collapsed state: localStorage, key in the house
// style"), mirroring lib/theme.test.ts's own read/write/normalize coverage
// for the sibling THEME_STORAGE_KEY this file's own doc comment cites.

import { afterEach, describe, expect, it } from "vitest";

import {
  BUILD_PANEL_STORAGE_KEY,
  normalizePanelState,
  readPanelState,
  writePanelState,
} from "./panelStorage";

afterEach(() => {
  window.localStorage.clear();
});

describe("normalizePanelState", () => {
  it("passes through the two known values", () => {
    expect(normalizePanelState("open")).toBe("open");
    expect(normalizePanelState("closed")).toBe("closed");
  });

  it("folds anything else, null included, to open rather than wedging the panel shut", () => {
    expect(normalizePanelState(null)).toBe("open");
    expect(normalizePanelState("")).toBe("open");
    expect(normalizePanelState("1")).toBe("open");
  });
});

describe("readPanelState / writePanelState", () => {
  it("round-trips a written state", () => {
    writePanelState("closed");
    expect(readPanelState()).toBe("closed");
    writePanelState("open");
    expect(readPanelState()).toBe("open");
  });

  it("defaults to open when nothing has been stored yet", () => {
    expect(readPanelState()).toBe("open");
  });

  it("writes under the house-style flat key, not a per-section prefix", () => {
    writePanelState("closed");
    expect(window.localStorage.getItem(BUILD_PANEL_STORAGE_KEY)).toBe("closed");
  });
});
