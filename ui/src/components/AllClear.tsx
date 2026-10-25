// AllClear is the desk's needs-you column when /api/queue serves nothing
// (mock round 3's second frame): the console names whose move it is instead
// of going blank. The all-clear rule (task C1's hard-won context, adjudicated):
// NodeView.next_owner reports "agent" for an in-review node, because resume
// and sync are the legal Tiller verbs from there and merge is not one Tiller
// can see; a human's PR review and merge is very much still outstanding. So
// this block never claims a blanket "nothing needs you" - the headline scopes
// itself to the mechanical queue, the ownership line is a plain whose_move
// tally rather than a narrated guess, and any in-review node is enumerated by
// name, never folded into a silent "agent" bucket.

import { Link } from "react-router-dom";
import type { RecoveredReviewView, WhoseMoveCounts } from "../api";
import RecoveredReview from "./RecoveredReview";

export interface AllClearReviewItem {
  noun: string;
  id: string;
  label: string;
  recovery?: RecoveredReviewView;
  reviewMissing?: boolean;
}

// Both clauses keep noun+verb agreement in both numbers ("1 node sits" /
// "3 nodes sit"), parallel to each other; the human clause once dropped its
// noun and read "3 sit on a human gate".
function ownershipLine(counts: WhoseMoveCounts): string {
  const agentClause =
    counts.agent === 1
      ? "1 node moves with the agent next"
      : `${counts.agent} nodes move with the agent next`;
  const humanClause =
    counts.human === 1
      ? "1 node sits on a human gate elsewhere in the tree"
      : `${counts.human} nodes sit on a human gate elsewhere in the tree`;
  return `${agentClause} · ${humanClause}.`;
}

function reviewLeadText(count: number): string {
  const noun = count === 1 ? "node" : "nodes";
  const verb = count === 1 ? "sits" : "sit";
  return `${count} ${noun} still ${verb} in review, waiting on your merge:`;
}

export default function AllClear({
  whoseMove,
  inReview,
}: {
  whoseMove: WhoseMoveCounts;
  inReview: AllClearReviewItem[];
}) {
  return (
    <div className="all-clear">
      <span className="all-clear-tick" aria-hidden="true">
        ✓
      </span>
      <div className="all-clear-body">
        <p className="all-clear-headline">
          <b>All clear — nothing waiting in the queue.</b>
        </p>
        <p className="all-clear-ownership muted">{ownershipLine(whoseMove)}</p>
        {inReview.length > 0 && (
          <div className="all-clear-review">
            <p className="all-clear-review-lead">{inReview.some((item) => item.recovery !== undefined || item.reviewMissing === true) ? `${inReview.length} nodes remain in review; inspect their recorded review state:` : reviewLeadText(inReview.length)}</p>
            <ul className="all-clear-review-list">
              {inReview.map((item) => (
                <li key={`${item.noun}:${item.id}`}>
                  <Link to={`/node/${item.noun}/${item.id}`}>{item.label}</Link>
                  {item.recovery !== undefined && <RecoveredReview record={item.recovery} />}
                  {item.reviewMissing === true && <p>Aggregate review is not recorded; sync has not inspected its merge state. If unmerged, complete ordinary submission; if already merged, explicitly authorized historical recovery is required.</p>}
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </div>
  );
}
