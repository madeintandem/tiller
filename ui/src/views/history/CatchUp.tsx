import { useRef, useState } from "react";
import { markCaughtUp } from "../../api";

// The visit horizon moves only when the reader presses this button. The press
// is a one-shot marked GET (api.ts's markCaughtUp), never a header on the
// page's own read, so no re-render, filter change, window change, or refetch
// can send the marker. The list on screen is left exactly as it was: the new
// horizon applies on the next load, and the confirmation replaces the button
// until the reader leaves the page.

type Phase = "idle" | "saving" | "done" | "failed";

export default function CatchUp() {
  const [phase, setPhase] = useState<Phase>("idle");
  const inFlight = useRef(false);

  function press() {
    // The disabled attribute is the visible guard; the ref holds even if two
    // clicks land before React re-renders.
    if (inFlight.current || phase === "done") return;
    inFlight.current = true;
    setPhase("saving");
    markCaughtUp().then(
      () => setPhase("done"),
      () => setPhase("failed"),
    ).finally(() => {
      inFlight.current = false;
    });
  }

  if (phase === "done") {
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
