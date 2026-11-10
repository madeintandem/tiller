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

const COLLAPSED_HEAD = '.board-scroll > .board-heads > [data-collapsed="true"]';
const COLLAPSED_BODY = '.board-scroll > .board-body > [data-collapsed="true"]';

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

  function rule(selector: string): string {
    const start = boardCss.indexOf(`${selector} {`);
    expect(start).toBeGreaterThan(-1);
    return boardCss.slice(start, boardCss.indexOf("}", start));
  }

  it("keeps a collapsed head horizontal at the normal head height, label hidden", () => {
    // A vertical writing mode on the head would turn its flex column
    // sideways and let the label's length stretch the whole heads row.
    expect(rule(COLLAPSED_HEAD)).not.toMatch(/writing-mode|flex-direction:\s*column/);
    expect(rule(`${COLLAPSED_HEAD} .cn,\n${COLLAPSED_HEAD} .work-item-column-label`)).toMatch(/display:\s*none/);
  });

  // Fix's own sheet styles its heads and columns at two classes; the
  // collapsed rules must outrank them whatever order the sheets load in, or a
  // collapsed Fix head keeps its full padding and gap.
  it("outranks the Fix board's own head and column rules", () => {
    const specificity = (selector: string) => (selector.match(/\.[\w-]+|\[[^\]]+\]/g) ?? []).length;
    expect(rule(COLLAPSED_HEAD)).toMatch(/padding-left:\s*0/);
    expect(rule(COLLAPSED_HEAD)).toMatch(/gap:/);
    expect(rule(COLLAPSED_BODY)).toMatch(/padding-left:\s*0/);
    expect(workItemsCss).toContain(".work-items-page .work-item-column-heading {");
    expect(workItemsCss).toContain(".work-items-page .work-item-column {");
    expect(specificity(COLLAPSED_HEAD)).toBeGreaterThan(specificity(".work-items-page .work-item-column-heading"));
    expect(specificity(COLLAPSED_BODY)).toBeGreaterThan(specificity(".work-items-page .work-item-column"));
  });

  it("writes a collapsed lane's label vertically at the top of its body", () => {
    const label = rule(`${COLLAPSED_BODY}::before`);
    expect(label).toMatch(/content:\s*attr\(data-label\)/);
    expect(label).toMatch(/writing-mode:\s*vertical-rl/);
    expect(label).toMatch(/white-space:\s*nowrap/);
  });

  it("sizes the scroller's minimum width from BoardGrid's sum", () => {
    expect(boardCss).toMatch(/min-width:\s*var\(--board-min-width/);
  });

  it("leaves the Scroll control's buttons at the shared segmented-control size", () => {
    expect(boardCss).not.toMatch(/\.board-scroll-control \.segmented-control button\s*\{[^}]*font-size/);
  });
});
