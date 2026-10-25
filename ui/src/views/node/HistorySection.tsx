// The lightweight history timeline (ui-surface.md, "Node detail"): the node's
// events exactly as the payload orders them (the API serves newest first), the
// recorded RFC3339 time rendered plainly, the event type, the actor, and the
// literal command. The console never reaches out to GitHub for depth, and
// nothing in a history entry is a URL, so every field here is text.
//
// A node with no events draws nothing: every node has at least its own add
// event, so an empty timeline means the event file failed to load, which the
// page states where it happens rather than as an empty section here.
//
// The list is capped the way every other list on the console is capped
// (DecisionCard's disclosure): the head carries the true count, the rows
// expand in place with no refetch. A day-old feature has two events and a
// six-month one has fifty, and the payload is uncapped, so this is the cap
// that keeps a mature node's page from ending in a scroll of its own past.

import { useState } from "react";
import type { HistoryEntry } from "../../api";
import InstrSection from "../../components/InstrSection";
import { PersonName } from "../../people";

const ROW_CAP = 8;

export default function HistorySection({ history, executionError }: { history: HistoryEntry[]; executionError?: string }) {
  const [expanded, setExpanded] = useState(false);
  if (history.length === 0 && executionError === undefined) {
    return null;
  }
  const shown = expanded ? history : history.slice(0, ROW_CAP);
  const hidden = history.length - shown.length;
  return (
    <InstrSection label={`History · ${history.length}`} caption="newest first">
      {executionError !== undefined && <p role="alert">Execution history unavailable: {executionError}</p>}
      <ol className="history">
        {/* Entries carry no id and the list is static per fetch, so the
            index is a stable key. */}
        {shown.map((e, i) => (
          <li key={i} className="history-entry">
            {/* A timestamp is not a key, an id, a path, or a command, which
                are mono's whole list. It keeps its column by tabular figures
                (node.css) instead. */}
            <span className="muted history-at">{e.at}</span>
            <span className="history-type">{e.type}</span>
            {e.contributor !== "" && <span className="muted"><PersonName id={e.contributor} /></span>}
            {e.command !== "" && <code className="history-command mono muted">{e.command}</code>}
          </li>
        ))}
      </ol>
      {hidden > 0 && (
        <button type="button" className="instr-more" onClick={() => setExpanded(true)}>
          {hidden} more {hidden === 1 ? "event" : "events"} ▸
        </button>
      )}
    </InstrSection>
  );
}
