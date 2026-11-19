// A batch's members, one row each in the related-row grammar: the member's
// noun, its title as the link, its key in muted mono, a bug's severity, and
// the status the member itself stores. The batch carries the lifecycle, so a
// member reads ready while the batch works; a proposed one is the member that
// still waits on triage before the batch can start.

import type { BatchMemberView } from "../../api";
import Chip from "../../components/Chip";
import EmptyNote from "../../components/EmptyNote";
import InstrSection from "../../components/InstrSection";
import RefLink from "../../components/RefLink";
import SeverityChip from "../../components/SeverityChip";
import { has } from "../../lib/has";

export default function BatchMembersPanel({ members }: { members: BatchMemberView[] }) {
  return (
    <InstrSection label="Members" caption={`${members.length}`}>
      {members.length === 0 ? (
        <EmptyNote>This batch has no members.</EmptyNote>
      ) : (
        <ul className="related-list batch-members" aria-label="Batch members">
          {members.map((member) => (
            <li key={member.id}>
              <span className="batch-member-noun mono">{member.noun}</span>
              <RefLink
                node={{ noun: member.noun, id: member.id, ref: member.key ?? member.id }}
                label={member.title}
                className="related-title"
              />
              {has(member.key) && <span className="mono muted related-key">{member.key}</span>}
              {member.noun === "bug" && <SeverityChip severity={member.severity} />}
              <Chip status={member.status} />
            </li>
          ))}
        </ul>
      )}
    </InstrSection>
  );
}
