// The Explore navigator's status legibility pass (JC, 2026-10-18): every row
// draws a StateGlyph, finished rows step back, blocked rows stand out, and a
// "Hide cancelled" toggle drops cancelled rows from every tier and shelf. The
// navigator is rendered directly with its fetch states in hand, since none
// of this depends on Explore's routing or fetch timing.

import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ListResult, ListRow, ProjectSummary, TreeModel, TreeNode } from "../../api";
import type { FetchState } from "../../hooks";
import { HIDE_CANCELLED_STORAGE_KEY } from "../../views/explore/hideCancelled";
import Navigator from "../Navigator";

afterEach(() => {
  cleanup();
  window.localStorage.clear();
  vi.useRealTimers();
});

function loaded<T>(data: T): FetchState<T> {
  return { data, error: null, loading: false };
}

function node(noun: string, id: string, title: string, status: string, children: TreeNode[] = []): TreeNode {
  return { noun, id, title, status, path: "", files: [], children };
}

function shelf(noun: string, rows: ListRow[]): ListResult {
  return { noun, count: rows.length, nodes: rows };
}

const tree: TreeModel = {
  project: "acme",
  nodes: [
    node("epic", "E-a", "Checkout", "active", [
      node("feature", "F-done", "Cart persistence", "done"),
      node("feature", "F-active", "Payment intents", "active", [
        node("task", "T-done", "Stripe client wrapper", "done"),
        node("task", "T-blocked", "Retry policy", "blocked"),
        node("task", "T-cancelled", "Legacy gateway shim", "cancelled"),
      ]),
      node("feature", "F-cancelled", "Gift cards", "cancelled"),
    ]),
  ],
};

const bugs = shelf("bug", [
  { id: "B-open", ref: "B-open", key: "bugs/blank-on-reload", title: "Blank on reload", status: "ready" },
  { id: "B-cancelled", ref: "B-cancelled", title: "Duplicate of reload", status: "cancelled" },
]);

function renderNavigator() {
  // Every branch open, so each tier's rows are on screen without clicking.
  const expansion = new Map([
    ["E-a", true],
    ["F-active", true],
    ["shelf:bug", true],
  ]);
  return render(
    <MemoryRouter>
      <Navigator
        tree={loaded(tree)}
        questions={loaded(shelf("question", []))}
        bugs={loaded(bugs)}
        chores={loaded(shelf("chore", []))}
        tweaks={loaded(shelf("tweak", []))}
        summary={loaded({} as ProjectSummary)}
        selected={null}
        expansion={expansion}
        onToggleBranch={() => {}}
        onOpenAll={() => {}}
        onCloseAll={() => {}}
      />
    </MemoryRouter>,
  );
}

function rowOf(title: string): HTMLElement {
  const link = screen.getByRole("link", { name: new RegExp(title) });
  return link.closest(".explore-epic-row, .explore-feat, .explore-task, .explore-shelf-node") as HTMLElement;
}

describe("Navigator: status glyphs", () => {
  it("draws a named status glyph on every tier and shelf row", () => {
    renderNavigator();
    expect(within(rowOf("Checkout")).getByRole("img", { name: "Active" })).toBeInTheDocument();
    expect(within(rowOf("Cart persistence")).getByRole("img", { name: "Done" })).toBeInTheDocument();
    expect(within(rowOf("Retry policy")).getByRole("img", { name: "Blocked" })).toBeInTheDocument();
    expect(within(rowOf("Blank on reload")).getByRole("img", { name: "Ready" })).toBeInTheDocument();
    expect(document.querySelector(".explore-nav-tree .state-dot")).toBeNull();
  });

  it("lets finished rows step back and makes blocked rows stand out", () => {
    renderNavigator();
    expect(rowOf("Cart persistence")).toHaveClass("is-settled");
    expect(rowOf("Stripe client wrapper")).toHaveClass("is-settled");
    expect(rowOf("Gift cards")).toHaveClass("is-settled");
    expect(rowOf("Retry policy")).toHaveClass("is-blocked");
    expect(rowOf("Payment intents")).not.toHaveClass("is-settled");
    expect(rowOf("Payment intents")).not.toHaveClass("is-blocked");
    expect(rowOf("Blank on reload")).not.toHaveClass("is-settled");
  });
});

describe("Navigator: status glyph tooltips", () => {
  it("shows a glyph's status label on hover, and hides it on leave", () => {
    vi.useFakeTimers();
    renderNavigator();
    const glyph = within(rowOf("Checkout")).getByRole("img", { name: "Active" });
    fireEvent.pointerEnter(glyph, { pointerType: "mouse" });
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(screen.getByRole("tooltip")).toHaveTextContent(/^Active$/);
    fireEvent.pointerLeave(glyph, { pointerType: "mouse" });
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("keeps a shelf row's key on its title text, off the glyph, so the two tooltips never stack", () => {
    renderNavigator();
    const row = rowOf("Blank on reload");
    const glyph = within(row).getByRole("img", { name: "Ready" });
    expect(glyph.closest("[title]")).toBeNull();
    expect(within(row).getByText("Blank on reload")).toHaveAttribute("title", "bugs/blank-on-reload");
  });
});

describe("Navigator: hide cancelled", () => {
  it("shows cancelled rows until the toggle is pressed, then drops them from every tier and shelf", () => {
    renderNavigator();
    const toggle = screen.getByRole("button", { name: "Hide cancelled" });
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("link", { name: /Gift cards/ })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Legacy gateway shim/ })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Duplicate of reload/ })).toBeInTheDocument();

    fireEvent.click(toggle);

    expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByRole("link", { name: /Gift cards/ })).toBeNull();
    expect(screen.queryByRole("link", { name: /Legacy gateway shim/ })).toBeNull();
    expect(screen.queryByRole("link", { name: /Duplicate of reload/ })).toBeNull();
    expect(screen.getByRole("link", { name: /Cart persistence/ })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Blank on reload/ })).toBeInTheDocument();
  });

  it("counts only the rows it still shows", () => {
    renderNavigator();
    expect(rowOf("Checkout").querySelector(".explore-epic-count")).toHaveTextContent("3");
    expect(rowOf("Payment intents").querySelector(".explore-feat-count")).toHaveTextContent("3");
    expect(within(screen.getByRole("region", { name: "Bugs" })).getByText("2")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Hide cancelled" }));

    expect(rowOf("Checkout").querySelector(".explore-epic-count")).toHaveTextContent("2");
    expect(rowOf("Payment intents").querySelector(".explore-feat-count")).toHaveTextContent("2");
    expect(within(screen.getByRole("region", { name: "Bugs" })).getByText("1")).toBeInTheDocument();
  });

  it("remembers the choice across a reload", () => {
    renderNavigator();
    fireEvent.click(screen.getByRole("button", { name: "Hide cancelled" }));
    expect(window.localStorage.getItem(HIDE_CANCELLED_STORAGE_KEY)).toBe("true");
    cleanup();

    renderNavigator();
    expect(screen.getByRole("button", { name: "Hide cancelled" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByRole("link", { name: /Gift cards/ })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Hide cancelled" }));
    expect(window.localStorage.getItem(HIDE_CANCELLED_STORAGE_KEY)).toBe("false");
  });
});
