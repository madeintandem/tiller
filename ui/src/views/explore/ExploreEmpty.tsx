// Explore's reading pane with nothing selected: the nodes the reader opened
// lately, and two links to the places that usually come next, Home for what
// needs a decision and Build for what is moving. Neither count blocks the
// pane: while a read is loading (or has failed) its link renders without a
// number rather than a spinner, and the reads keep their data across a reread.

import { Link } from "react-router-dom";
import type { Queue, Streams } from "../../api";
import { boardPath, queuePath } from "../../api";
import { useFetch } from "../../hooks";
import { readRecent } from "../../lib/recentNodes";
import "../explore-empty.css";

// "In flight" counts what Home's In flight column calls moving work: each
// task an owner has active or in review, once. The blocked register and the
// unclaimed pool are waiting, not in flight, so they are not counted; Build
// shows the same tasks in its Active and In review columns.
export function inFlightCount(streams: Streams): number {
  const ids = new Set<string>();
  for (const stream of streams.contributors) {
    for (const item of stream.active) ids.add(item.id);
    for (const item of stream.gates_in_review) ids.add(item.id);
  }
  return ids.size;
}

function nounLabel(noun: string): string {
  return noun === "" ? noun : noun.charAt(0).toUpperCase() + noun.slice(1);
}

export default function ExploreEmpty({ projectKey }: { projectKey: string | undefined }) {
  const queue = useFetch<Queue>(queuePath, { retainData: true });
  const board = useFetch<Streams>(boardPath, { retainData: true });
  const recent = projectKey === undefined ? [] : readRecent(projectKey);

  const decisions = queue.data?.entries.length;
  const flying = board.data === null ? undefined : inFlightCount(board.data);

  let needs = "Decisions that need you";
  if (decisions !== undefined) {
    needs =
      decisions === 0
        ? "Nothing needs you right now"
        : `${decisions} ${decisions === 1 ? "decision needs" : "decisions need"} you`;
  }
  let flight = "Work in flight";
  if (flying !== undefined) {
    flight = flying === 0 ? "Nothing in flight" : `${flying} in flight`;
  }

  return (
    <div className="explore-detail">
      <p className="explore-empty-links">
        <Link to="/">{needs}</Link>
        <Link to="/build" title="Tasks active or in review, as Build's Active and In review columns show them">
          {flight}
        </Link>
      </p>
      {recent.length === 0 ? (
        <p className="muted">Select a node from the navigator to see its detail.</p>
      ) : (
        <section aria-labelledby="explore-recent-heading" className="explore-recent">
          <h2 id="explore-recent-heading">Recently opened</h2>
          <ul className="rule-list">
            {recent.map((node) => (
              <li key={node.id} className="explore-recent-row">
                <Link to={`/node/${node.noun}/${node.id}`}>{node.title}</Link>
                <span className="mono muted explore-recent-key">{node.key ?? node.id}</span>
                <span className="muted explore-recent-noun">{nounLabel(node.noun)}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
