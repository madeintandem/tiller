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
// and semantics; this component owns only the two-row structure.

import type { CSSProperties, ReactNode } from "react";
import { Fragment } from "react";
import type { BoardScrollMode } from "../lib/boardScroll";
import "./board.css";

export interface BoardLane {
  key: string;
  head: ReactNode;
  body: ReactNode;
}

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
  const style = { "--board-columns": lanes.length } as CSSProperties;
  return (
    <div className={`${className} board-scroll`} data-scroll-mode={scrollMode} style={style}>
      <div className="board-heads">
        {lanes.map((lane) => (
          <Fragment key={lane.key}>{lane.head}</Fragment>
        ))}
      </div>
      <div className="board-body">
        {lanes.map((lane) => (
          <Fragment key={lane.key}>{lane.body}</Fragment>
        ))}
      </div>
    </div>
  );
}
