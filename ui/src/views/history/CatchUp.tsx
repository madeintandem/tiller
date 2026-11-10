import { useRef, useState } from "react";
import { markCaughtUp } from "../../api";

// The visit horizon moves only when the reader presses this button, at once,
// to the instant the digest on screen was computed (through), so nothing that
// arrived after the page loaded is swallowed. The press is a one-shot marked
// GET (api.ts's markCaughtUp), never a header on the page's own read, so no
// re-render, filter change, window change, or refetch can send the marker.
// The list on screen is left exactly as it was.
//
// Whether the press already happened is the parent's to hold (History keeps it
// above the fetch), so a refresh or window round trip on the same page visit
// does not put the button back over a horizon that already moved.

type Phase = "idle" | "saving" | "failed";

export default function CatchUp({
  through,
  done,
  onDone,
}: {
  through: string | undefined;
  done: boolean;
  onDone: () => void;
}) {
  const [phase, setPhase] = useState<Phase>("idle");
  const inFlight = useRef(false);

  function press() {
    // The disabled attribute is the visible guard; the ref holds even if two
    // clicks land before React re-renders.
    if (inFlight.current || done) return;
    inFlight.current = true;
    setPhase("saving");
    markCaughtUp(through)
      .then(
        () => onDone(),
        () => setPhase("failed"),
      )
      .finally(() => {
        inFlight.current = false;
      });
  }

  if (done) {
    return (
      <p className="catch-up muted" role="status">
        Caught up. Your next visit starts here.
      </p>
    );
  }
  return (
    <p className="catch-up">
      <button
        type="button"
        className="catch-up-btn"
        disabled={phase === "saving"}
        onClick={press}
        title="Marks the whole window caught up, not only the rows shown under the current filters."
      >
        Mark caught up
      </button>
      {phase === "failed" && (
        <span className="catch-up-error" role="alert">
          That did not save. Try again.
        </span>
      )}
    </p>
  );
}
