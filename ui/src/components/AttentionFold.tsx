import { useState, type ReactNode } from "react";
import "./attention-fold.css";

// AttentionFold shows the first of the attention stack's boxes and folds the
// rest behind a "N more" button, so a node with several things waiting on it
// reads one decision at a time. Which boxes exist, and their order, is the
// stack's call; this only decides how many show.
export default function AttentionFold({ boxes }: { boxes: ReactNode[] }) {
  const [open, setOpen] = useState(false);
  if (boxes.length === 0) {
    return null;
  }
  const rest = boxes.length - 1;
  return (
    <div className="attn">
      {boxes[0]}
      {rest > 0 && !open && (
        <button type="button" className="attn-more" aria-expanded="false" onClick={() => setOpen(true)}>
          {rest} more
        </button>
      )}
      {rest > 0 && open && boxes.slice(1)}
    </div>
  );
}
