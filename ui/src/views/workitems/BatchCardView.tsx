// A live batch on the Fix board: one card in the batch's own status column,
// standing for the bugs, chores, and tweaks it reviews on one branch and one
// PR. It reads like the cards around it (noun chip, title, owner, date,
// severity) and adds the member count and the members themselves, each a link
// to its own page. A member that is not yet approved says so, because the
// batch cannot start until it is.

import RefLink from "../../components/RefLink";
import SeverityChip from "../../components/SeverityChip";
import { formatAdded } from "../../lib/added";
import { has } from "../../lib/has";
import { statusLabel } from "../../lib/statusLabel";
import { formatAbsolute } from "../../lib/time";
import { PersonName } from "../../people";
import type { BatchMemberView } from "../../api";
import type { BatchEntry } from "./board";

export default function BatchCardView({ batch }: { batch: BatchEntry }) {
  const title = has(batch.title) ? batch.title : batch.ref;
  const count = batch.member_list.length;
  return (
    <article className="work-item-card">
      <span className="work-item-noun-chip" data-noun="batch">batch</span>
      <RefLink node={batch} label={title} className="work-item-card-title" />
      {has(batch.owner) && (
        <span className="work-item-owner">Owner: <PersonName id={batch.owner} name={batch.owner_name} /></span>
      )}
      <p className="work-item-attachment">{count} {count === 1 ? "member" : "members"}</p>
      <ul className="work-item-batch-members" aria-label={`Members of ${title}`}>
        {batch.member_list.map((member) => (
          <BatchMemberRow key={member.id} member={member} />
        ))}
      </ul>
      {has(batch.added_at) && (
        <p className="work-item-added">
          <time dateTime={batch.added_at} title={formatAbsolute(batch.added_at)}>
            {formatAdded(batch.added_at, new Date())}
          </time>
        </p>
      )}
      {batch.severity !== undefined && (
        <div className="work-item-card-chips">
          <SeverityChip severity={batch.severity} />
        </div>
      )}
    </article>
  );
}

// A ready member is the expected case and says nothing; any other stored
// status (proposed, most often) is named in words beside the link.
function BatchMemberRow({ member }: { member: BatchMemberView }) {
  return (
    <li>
      <span className="work-item-noun-chip" data-noun={member.noun}>{member.noun}</span>
      <RefLink
        node={{ noun: member.noun, id: member.id, ref: member.key ?? member.id }}
        label={member.title}
        className=""
      />
      {member.status !== "ready" && <span className="work-item-batch-member-status">{statusLabel(member.status)}</span>}
    </li>
  );
}
