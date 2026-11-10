// Explore's reading pane with nothing selected: the nodes the reader opened
// lately, and two links to the places that usually come next, Home for what
// needs a decision and Build for what is moving. Neither count blocks the
// pane: while a read is loading (or has failed) its link renders without a
// number rather than a spinner, and the reads keep their data across a reread.

import { Link } from "react-router-dom";
import type { BuildBoard, Queue } from "../../api";
import { buildPath, queuePath } from "../../api";
import { useFetch } from "../../hooks";
import { readRecent } from "../../lib/recentNodes";
import { columnCards, visibleCards } from "../build/columns";
import { epicsParamValue } from "../build/selection";
import "../explore-empty.css";

// "In flight" is the number of feature cards in Build's Active and In review
// columns, across every epic: the board's own column assignment
// (BuildFeature.column, which puts a stored-ready feature with work in flight in
// Active) through the same visibleCards and columnCards Build runs. Tasks are
// not counted; Home's In flight column lists those.
export function inFlightCount(board: BuildBoard): number {
  const cards = visibleCards(board.epics);
  return columnCards(cards, "active").length + columnCards(cards, "in_review").length;
}

// The link opens Build with every epic selected, so the board shows the cards
// the number counts rather than Build's stored or default epic selection.
function buildHref(board: BuildBoard | null): string {
  const keys = board === null ? [] : board.epics.flatMap((epic) => (epic.key === undefined ? [] : [epic.key]));
  return keys.length === 0 ? "/build" : `/build?epics=${epicsParamValue(keys)}`;
}

function nounLabel(noun: string): string {
  return noun === "" ? noun : noun.charAt(0).toUpperCase() + noun.slice(1);
}

export default function ExploreEmpty({ projectKey }: { projectKey: string | undefined }) {
  const queue = useFetch<Queue>(queuePath, { retainData: true });
  const board = useFetch<BuildBoard>(buildPath, { retainData: true });
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
    flight = flying === 0 ? "Nothing in flight" : `${flying} ${flying === 1 ? "feature" : "features"} in flight`;
  }

  return (
    <div className="explore-detail">
      <p className="explore-empty-links">
        <Link to="/">{needs}</Link>
        <Link to={buildHref(board.data)} title="Features in Build's Active and In review columns, across all epics">
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
