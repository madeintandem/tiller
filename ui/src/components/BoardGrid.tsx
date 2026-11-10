// BoardGrid lays out a board's columns as two rows: the column heads, which
// never scroll vertically, over the column bodies, which do. Both rows sit in
// one horizontal scroller, so panning moves heads and bodies together.
//
// The heads used to stick to the top of a single scroller that held whole
// columns. Layout placed them exactly, but Safari's asynchronous scrolling
// composited the sticky heads a few pixels away from that place, so scrolled
// cards showed in a strip above each head on Build and Fix. A head that is
// not inside the scroller has nothing to drift against.
//
// Each lane supplies its own head and body elements with their own classes
// and semantics; this component owns only the two-row structure and the
// column template. A lane with no cards collapses to a slim rail so the
// columns that hold work get the width. The rail's head keeps the heads
// row's normal height (dot and count only) and its body carries the label
// written vertically. A board where every lane is empty
// collapses nothing: a row of rails would read as a broken board, so each
// lane stays full width. The collapsed head stays in the same static heads
// row, and keeps an accessible name of its label and count because its text
// is written vertically.

import type { CSSProperties, ReactNode } from "react";
import { Fragment, cloneElement, isValidElement } from "react";
import type { BoardScrollMode } from "../lib/boardScroll";
import "./board.css";

// What BoardGrid adds to a collapsed lane's head and body elements; a lane's
// own components spread it onto their root element.
export interface LaneMark {
  "data-collapsed"?: string;
  "aria-label"?: string;
  "data-label"?: string;
}

export interface BoardLane {
  key: string;
  head: ReactNode;
  body: ReactNode;
  empty: boolean;
  label: string;
}

const RAIL_WIDTH = "44px";
const OPEN_WIDTH = "minmax(var(--board-col-min), 1fr)";

export default function BoardGrid({
  className,
  scrollMode,
  lanes,
}: {
  className: string;
  scrollMode: BoardScrollMode;
  lanes: BoardLane[];
}) {
  // The column count rides a custom property so both rows (board.css) share
  // one template however many lanes a board has.
  const allEmpty = lanes.every((lane) => lane.empty);
  const collapsed = lanes.map((lane) => lane.empty && !allEmpty);
  const railCount = collapsed.filter(Boolean).length;
  const openCount = lanes.length - railCount;
  const style = {
    "--board-columns": lanes.length,
    "--board-min-width": `calc(${railCount} * ${RAIL_WIDTH} + ${openCount} * var(--board-col-min) + ${Math.max(lanes.length - 1, 0)} * var(--board-gap))`,
  } as CSSProperties;
  const rowStyle = { gridTemplateColumns: collapsed.map((c) => (c ? RAIL_WIDTH : OPEN_WIDTH)).join(" ") };
  // A lane's own head and body elements carry the collapsed mark, so the
  // two-row grid keeps its direct-child structure.
  // The head keeps the heads row's normal height, showing only the dot and
  // count; the body writes the label vertically, where height is free.
  const mark = (node: ReactNode, on: boolean, extra: Record<string, string>): ReactNode =>
    on && isValidElement<Record<string, unknown>>(node)
      ? cloneElement(node, { "data-collapsed": "true", ...extra })
      : node;
  return (
    <div className={`${className} board-scroll`} data-scroll-mode={scrollMode} style={style}>
      <div className="board-heads" style={rowStyle}>
        {lanes.map((lane, i) => (
          <Fragment key={lane.key}>{mark(lane.head, collapsed[i], { "aria-label": `${lane.label}, 0` })}</Fragment>
        ))}
      </div>
      <div className="board-body" style={rowStyle}>
        {lanes.map((lane, i) => (
          <Fragment key={lane.key}>{mark(lane.body, collapsed[i], { "data-label": lane.label })}</Fragment>
        ))}
      </div>
    </div>
  );
}
