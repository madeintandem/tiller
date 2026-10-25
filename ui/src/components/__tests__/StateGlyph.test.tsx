// StateGlyph is the Explore tree's status mark (JC, 2026-10-18: the 6-8px
// dots were too small and too close in color to read with color blindness).
// The contract under test is that shape alone carries the state: each of the
// seven chart states draws its own shape, a folded wire status draws its
// chart sibling's, and the status label rides along as the accessible name.
// Hovering with a mouse shows a styled tooltip carrying the same label,
// which replaced the native SVG <title> so two tooltips never show at once.

import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import StateGlyph from "../StateGlyph";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function shapeOf(status: string): string | null {
  const { container } = render(<StateGlyph status={status} />);
  const shape = container.querySelector(".state-glyph")?.getAttribute("data-shape") ?? null;
  cleanup();
  return shape;
}

describe("StateGlyph", () => {
  it("draws a different shape for each of the seven chart states", () => {
    const states = ["draft", "specced", "ready", "active", "in_review", "done", "blocked"];
    const shapes = states.map(shapeOf);
    expect(shapes.every((shape) => shape !== null)).toBe(true);
    expect(new Set(shapes).size).toBe(states.length);
  });

  it("draws a folded wire status as its chart sibling", () => {
    expect(shapeOf("proposed")).toBe(shapeOf("specced"));
    expect(shapeOf("in_progress")).toBe(shapeOf("active"));
    expect(shapeOf("check")).toBe(shapeOf("active"));
    expect(shapeOf("open")).toBe(shapeOf("in_review"));
    expect(shapeOf("answered")).toBe(shapeOf("done"));
  });

  it("rests an unknown status at the draft shape rather than drawing nothing", () => {
    expect(shapeOf("cancelled")).toBe(shapeOf("draft"));
    expect(shapeOf("someday")).toBe(shapeOf("draft"));
  });

  it("names the status label for assistive tech", () => {
    render(<StateGlyph status="in_review" />);
    const glyph = screen.getByRole("img", { name: "In Review" });
    expect(glyph).toHaveAttribute("data-status", "in_review");
  });

  it("labels each status in Title Case, specced as Specified", () => {
    const labels: Record<string, string> = {
      draft: "Draft",
      specced: "Specified",
      proposed: "Proposed",
      ready: "Ready",
      active: "Active",
      in_progress: "In Progress",
      check: "Check",
      in_review: "In Review",
      done: "Done",
      blocked: "Blocked",
      cancelled: "Cancelled",
      archived: "Archived",
      open: "Open",
      answered: "Answered",
      someday_maybe_later: "Someday Maybe Later",
    };
    for (const [status, label] of Object.entries(labels)) {
      render(<StateGlyph status={status} />);
      expect(screen.getByRole("img"), status).toHaveAccessibleName(label);
      cleanup();
    }
  });

  it("draws no native SVG title and carries no description, so only the label tooltip ever shows", () => {
    const { container } = render(<StateGlyph status="in_review" />);
    expect(container.querySelector("title")).toBeNull();
    expect(screen.getByRole("img")).not.toHaveAttribute("aria-describedby");
  });

  it("shows the label shortly after a mouse hover, and hides on leave", () => {
    vi.useFakeTimers();
    render(<StateGlyph status="in_review" />);
    const glyph = screen.getByRole("img", { name: "In Review" });

    hover(glyph, "mouse");
    expect(screen.queryByRole("tooltip")).toBeNull();
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(screen.getByRole("tooltip")).toHaveTextContent(/^In Review$/);

    fireEvent.pointerLeave(glyph, { pointerType: "mouse" });
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("never shows a tooltip for a hover that leaves before the delay", () => {
    vi.useFakeTimers();
    render(<StateGlyph status="done" />);
    const glyph = screen.getByRole("img", { name: "Done" });
    hover(glyph, "mouse");
    fireEvent.pointerLeave(glyph, { pointerType: "mouse" });
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("ignores a touch, so a tap never leaves a stuck tooltip", () => {
    vi.useFakeTimers();
    render(<StateGlyph status="done" />);
    hover(screen.getByRole("img", { name: "Done" }), "touch");
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("sits above the glyph, centred on it", () => {
    vi.useFakeTimers();
    render(<StateGlyph status="done" />);
    const glyph = screen.getByRole("img", { name: "Done" });
    stubRect(glyph, { left: 40, top: 100, width: 14, height: 14 });
    stubTooltipRect({ width: 60, height: 24 });
    showTip(glyph);
    const tip = screen.getByRole("tooltip");
    expect(tip.style.left).toBe("17px");
    expect(tip.style.top).toBe("68px");
  });

  it("flips below the glyph when there is no room above", () => {
    vi.useFakeTimers();
    render(<StateGlyph status="done" />);
    const glyph = screen.getByRole("img", { name: "Done" });
    stubRect(glyph, { left: 40, top: 10, width: 14, height: 14 });
    stubTooltipRect({ width: 60, height: 24 });
    showTip(glyph);
    const tip = screen.getByRole("tooltip");
    expect(tip.style.left).toBe("17px");
    expect(tip.style.top).toBe("32px");
  });

  it("stays inside the viewport's left edge", () => {
    vi.useFakeTimers();
    render(<StateGlyph status="done" />);
    const glyph = screen.getByRole("img", { name: "Done" });
    stubRect(glyph, { left: 0, top: 100, width: 14, height: 14 });
    stubTooltipRect({ width: 60, height: 24 });
    showTip(glyph);
    expect(screen.getByRole("tooltip").style.left).toBe("8px");
  });

  it("stays inside the viewport's right edge", () => {
    vi.useFakeTimers();
    vi.stubGlobal("innerWidth", 400);
    render(<StateGlyph status="done" />);
    const glyph = screen.getByRole("img", { name: "Done" });
    stubRect(glyph, { left: 390, top: 100, width: 14, height: 14 });
    stubTooltipRect({ width: 60, height: 24 });
    showTip(glyph);
    expect(screen.getByRole("tooltip").style.left).toBe("332px");
  });

  it("dismisses the tooltip on any scroll", () => {
    vi.useFakeTimers();
    render(<StateGlyph status="done" />);
    showTip(screen.getByRole("img", { name: "Done" }));
    expect(screen.getByRole("tooltip")).toBeInTheDocument();
    act(() => {
      document.body.dispatchEvent(new Event("scroll"));
    });
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("unmounts cleanly with a tooltip still pending", () => {
    vi.useFakeTimers();
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    const { unmount } = render(<StateGlyph status="done" />);
    hover(screen.getByRole("img", { name: "Done" }), "mouse");
    unmount();
    expect(vi.getTimerCount()).toBe(0);
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(errors).not.toHaveBeenCalled();
    expect(screen.queryByRole("tooltip")).toBeNull();
  });
});

function hover(glyph: Element, pointerType: string) {
  fireEvent.pointerEnter(glyph, { pointerType });
}

function showTip(glyph: Element) {
  hover(glyph, "mouse");
  act(() => {
    vi.advanceTimersByTime(200);
  });
}

function stubRect(el: Element, { left, top, width, height }: { left: number; top: number; width: number; height: number }) {
  el.getBoundingClientRect = () => new DOMRect(left, top, width, height);
}

// The tooltip mounts on hover, so its size is stubbed on the prototype its
// element will have; the glyph's own stub (an SVG element) is unaffected.
function stubTooltipRect({ width, height }: { width: number; height: number }) {
  vi.spyOn(HTMLDivElement.prototype, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, width, height));
}
