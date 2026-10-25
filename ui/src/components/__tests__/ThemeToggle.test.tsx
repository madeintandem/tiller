// Vitest coverage for ThemeToggle (Phase 0 task 3): the topbar's
// Auto/Light/Dark segmented control. lib/theme.test.ts pins the underlying
// functions directly; this file exercises them through the rendered
// control, the way primitives.test.tsx exercises the other shared chrome.

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { THEME_STORAGE_KEY } from "../../lib/theme";
import ThemeToggle from "../ThemeToggle";

afterEach(() => {
  cleanup();
  document.documentElement.removeAttribute("data-theme");
  window.localStorage.clear();
});

describe("ThemeToggle", () => {
  it("renders all three picks, defaulting to Auto active with no stored value", () => {
    render(<ThemeToggle />);
    expect(screen.getByRole("group", { name: "Theme" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Auto" })).toHaveClass("active");
    expect(screen.getByRole("button", { name: "Light" })).not.toHaveClass("active");
    expect(screen.getByRole("button", { name: "Dark" })).not.toHaveClass("active");
  });

  it("picking Dark sets data-theme on the root and persists it under the plan's key", () => {
    render(<ThemeToggle />);

    fireEvent.click(screen.getByRole("button", { name: "Dark" }));

    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");
    expect(screen.getByRole("button", { name: "Dark" })).toHaveClass("active");
    expect(screen.getByRole("button", { name: "Auto" })).not.toHaveClass("active");
  });

  it("picking Auto after Dark removes data-theme and persists auto", () => {
    render(<ThemeToggle />);

    fireEvent.click(screen.getByRole("button", { name: "Dark" }));
    expect(document.documentElement.hasAttribute("data-theme")).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "Auto" }));

    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("auto");
    expect(screen.getByRole("button", { name: "Auto" })).toHaveClass("active");
  });

  it("picking Light sets data-theme to light and persists it", () => {
    render(<ThemeToggle />);

    fireEvent.click(screen.getByRole("button", { name: "Light" }));

    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("light");
  });

  it("a garbage stored value (hand-edited localStorage) renders as Auto, not a crash or a wedged control", () => {
    window.localStorage.setItem(THEME_STORAGE_KEY, "solarized");

    render(<ThemeToggle />);

    expect(screen.getByRole("button", { name: "Auto" })).toHaveClass("active");
    expect(screen.getByRole("button", { name: "Light" })).not.toHaveClass("active");
    expect(screen.getByRole("button", { name: "Dark" })).not.toHaveClass("active");
  });
});
