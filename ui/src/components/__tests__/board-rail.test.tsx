import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import BoardGrid, { type BoardLane } from "../BoardGrid";

afterEach(cleanup);

function lane(key: string, empty: boolean): BoardLane {
  return {
    key,
    empty,
    label: key,
    head: <div className="col-head">{key}</div>,
    body: <div className="col board-column">{empty ? null : <p>card</p>}</div>,
  };
}

describe("BoardGrid collapsed rails", () => {
  it("sums the scroller's minimum width from rails, open lanes, and gaps", () => {
    const { container } = render(
      <BoardGrid className="board" scrollMode="board" lanes={[lane("a", true), lane("b", false), lane("c", true), lane("d", false)]} />,
    );
    const scroller = container.querySelector(".board-scroll") as HTMLElement;
    expect(scroller.style.getPropertyValue("--board-min-width")).toBe(
      "calc(2 * 44px + 2 * var(--board-col-min) + 3 * var(--board-gap))",
    );
  });

  it("names the collapsed head and hands the body its label, leaving open lanes unmarked", () => {
    const { container } = render(
      <BoardGrid className="board" scrollMode="board" lanes={[lane("Draft", true), lane("Ready", false)]} />,
    );
    const heads = container.querySelectorAll(".board-heads > *");
    const bodies = container.querySelectorAll(".board-body > *");
    expect(heads[0]).toHaveAttribute("aria-label", "Draft, 0");
    expect(bodies[0]).toHaveAttribute("data-label", "Draft");
    expect(heads[1]).not.toHaveAttribute("data-collapsed");
    expect(bodies[1]).not.toHaveAttribute("data-collapsed");
  });
});
