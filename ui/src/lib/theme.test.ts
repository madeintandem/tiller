// Vitest coverage for the theme module (Phase 0 task 3): the exact
// semantics the plan and the mock's own applyTheme name - auto removes
// data-theme, a pick sets it, every pick persists under the plan's literal
// key ("tiller-theme", not the mock's own "tiller-mock-theme") - plus the
// garbage-value fold that keeps a hand-edited localStorage value from
// crashing or wedging the control. ThemeToggle.test.tsx covers the same
// ground through the rendered control; this file pins the underlying
// functions directly, including bootTheme, the pre-paint entry point
// main.tsx calls (its own test would require a full module-load harness).

import { afterEach, describe, expect, it } from "vitest";

import {
  applyThemeAttribute,
  bootTheme,
  normalizeTheme,
  readStoredTheme,
  setTheme,
  THEME_STORAGE_KEY,
} from "./theme";

afterEach(() => {
  document.documentElement.removeAttribute("data-theme");
  window.localStorage.clear();
});

describe("normalizeTheme", () => {
  it.each([
    ["auto", "auto"],
    ["light", "light"],
    ["dark", "dark"],
  ] as const)("keeps a recognized value %s as itself", (value, expected) => {
    expect(normalizeTheme(value)).toBe(expected);
  });

  it("folds null to auto", () => {
    expect(normalizeTheme(null)).toBe("auto");
  });

  it("folds an empty string to auto", () => {
    expect(normalizeTheme("")).toBe("auto");
  });

  it("folds a garbage/hand-edited value to auto rather than crashing", () => {
    expect(normalizeTheme("solarized")).toBe("auto");
  });

  // An Object.prototype member name must not leak a prototype-chain hit
  // (the same discipline states.ts's FOLDED_STATUSES Map documents).
  it("folds an Object.prototype member name to auto", () => {
    expect(normalizeTheme("toString")).toBe("auto");
  });
});

describe("readStoredTheme", () => {
  it("returns auto when nothing is stored", () => {
    expect(readStoredTheme()).toBe("auto");
  });

  it("returns the stored pick", () => {
    window.localStorage.setItem(THEME_STORAGE_KEY, "dark");
    expect(readStoredTheme()).toBe("dark");
  });

  it("folds a garbage stored value to auto", () => {
    window.localStorage.setItem(THEME_STORAGE_KEY, "solarized");
    expect(readStoredTheme()).toBe("auto");
  });
});

describe("applyThemeAttribute", () => {
  it("sets data-theme for an explicit pick", () => {
    applyThemeAttribute("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("removes data-theme for auto", () => {
    document.documentElement.setAttribute("data-theme", "dark");
    applyThemeAttribute("auto");
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
  });
});

describe("setTheme", () => {
  it("applies the attribute and persists the pick", () => {
    setTheme("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");
  });

  it("removes the attribute and persists auto", () => {
    setTheme("dark");
    setTheme("auto");
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("auto");
  });
});

describe("bootTheme", () => {
  it("applies a stored pick without re-persisting it", () => {
    window.localStorage.setItem(THEME_STORAGE_KEY, "light");
    bootTheme();
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("leaves the attribute unset (auto) when nothing is stored", () => {
    bootTheme();
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
  });

  it("folds a garbage stored value to auto, never crashing or setting a stray attribute", () => {
    window.localStorage.setItem(THEME_STORAGE_KEY, "solarized");
    expect(() => bootTheme()).not.toThrow();
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
  });
});
