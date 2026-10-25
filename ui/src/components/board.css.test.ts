// Pins two board facts jsdom cannot observe. Column heads sit in their own
// row above the board's scroller rather than sticking inside it: Safari's
// asynchronous scrolling composited a sticky head a few pixels away from
// where layout put it, so scrolled cards showed in a strip above the head on
// both Build and Fix (seen on 0.18.1; an overpaint of the strip in 0.17.7 did
// not hold). And the Scroll control inherits the shared segmented control's
// type size instead of overriding it, so it matches the topbar's theme
// selector.

import { describe, expect, it } from "vitest";

import buildCss from "../views/build.css?raw";
import workItemsCss from "../views/workitems.css?raw";
import boardCss from "./board.css?raw";

describe("board.css", () => {
  it("scrolls only the body row, never the heads", () => {
    expect(boardCss).toMatch(/\.board-scroll\[data-scroll-mode="board"\] > \.board-body\s*\{[^}]*overflow-y:\s*auto/);
    expect(boardCss).not.toMatch(/\.board-heads[^{]*\{[^}]*overflow/);
  });

  it("lets board-mode columns grow to the tallest one rather than the body's height", () => {
    expect(boardCss).toMatch(/\[data-scroll-mode="board"\] > \.board-body > \.board-column\s*\{\s*min-height:\s*auto;/);
  });

  it("pins no column head with position: sticky on either board", () => {
    for (const css of [boardCss, buildCss, workItemsCss]) {
      expect(css).not.toMatch(/position:\s*sticky/);
    }
  });

  it("leaves the Scroll control's buttons at the shared segmented-control size", () => {
    expect(boardCss).not.toMatch(/\.board-scroll-control \.segmented-control button\s*\{[^}]*font-size/);
  });
});
