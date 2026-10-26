// RecapSignal is Home's one line about the recap, sitting at the trailing edge
// of the needs-you eyebrow row: the whole phrase is the link, leading with its
// destination, so Home's two questions read on one line, "what needs you" in
// magenta on the left and "what happened" in slate on the right.
//
// The link reads "History" and targets /history (the view renamed 2026-08-10):
// this is chrome that names the view to the reader, so it updates with it, even
// though the component and its wire fetch below keep the recap name
// (RecapSignal.tsx, /api/recap/peek) unchanged.
//
// Three rules hold it honest.
//
// It never goes magenta. Events behind the horizon are not things that need
// you; the things that do are the decision cards two lines below.
//
// It states volume only, never the ask count. "32 need you" beside a card
// headed "32 Answer questions" is exactly the duplication that moved the recap
// off Home: the live ask count belongs to the queue, and the recap view says
// only how many ask EVENTS the window held, in the past tense, linking back
// here for the number that is still true. (The peek serves an ask count anyway
// because it is free and the view's own head wants it; Home drops it.)
//
// It reads /api/recap/peek, never /api/recap. The visit-window recap is the
// one request in the console that advances the last-visit horizon, so a Home
// that fetched it would spend a recap nobody opened, which is the bug this
// whole move fixes. And while the fetch is in flight or after it failed, the
// link renders bare: a claim is never rendered over data that did not load,
// the same discipline AllClearSlot holds one surface up.

import { Link } from "react-router-dom";
import type { RecapPeek } from "../api";
import { recapPeekPath } from "../api";
import { useFetch } from "../hooks";

export default function RecapSignal() {
  const { data } = useFetch<RecapPeek>(recapPeekPath);

  return (
    <Link to="/history" className="recap-signal">
      History
      {data !== null && <span className="muted"> · {volumeLine(data.total)}</span>}
    </Link>
  );
}

// The phrase after the separator, at every volume. "nothing new since your
// last visit" is lifted verbatim from the recap view's own empty text, so
// Home's empty and the view's empty are one sentence in one voice.
function volumeLine(total: number): string {
  if (total === 0) {
    return "nothing new since your last visit";
  }
  return `${total} ${total === 1 ? "event" : "events"} since your last visit`;
}
