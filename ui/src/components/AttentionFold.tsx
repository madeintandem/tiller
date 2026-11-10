import { useState, type ReactNode } from "react";
import "./attention-fold.css";

// AttentionFold shows the first of the attention stack's boxes and folds the
// rest behind a disclosure, so a node with several things waiting on it reads
// one decision at a time. Which boxes exist, and their order, is the stack's
// call; this only decides how many show. The control is the node page's own
// "N more" disclosure (.instr-more, as EpicBody and HistorySection use it),
// with the console-wide focus ring, dressed as a control rather than a
// caption: a verb ("Show 2 more"), a disclosure chevron, and aria-expanded.
// Open, it stays at the foot of the stack as "Show fewer" and folds the
// followers back up.
export default function AttentionFold({ boxes }: { boxes: ReactNode[] }) {
  const [open, setOpen] = useState(false);
  if (boxes.length === 0) {
    return null;
  }
  const rest = boxes.length - 1;
  return (
    <div className="attn">
      {boxes[0]}
      {rest > 0 && open && boxes.slice(1)}
      {rest > 0 && (
        <button
          type="button"
          className="instr-more instr-more-toggle"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
        >
          <span className="instr-more-chevron" aria-hidden="true">
            {open ? "▾" : "▸"}
          </span>
          {open ? "Show fewer" : `Show ${rest} more`}
        </button>
      )}
    </div>
  );
}
