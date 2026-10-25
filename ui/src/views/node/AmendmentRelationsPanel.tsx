import type { AmendmentEffect, AmendmentRelation, Relationships } from "../../api";
import Chip from "../../components/Chip";
import InstrSection from "../../components/InstrSection";
import RefLink from "../../components/RefLink";

const roleLabels: Record<AmendmentEffect["role"], string> = {
  target: "target",
  replacement: "replacement",
  created: "created node",
  addition_parent: "addition parent",
  question_block: "question-block target",
  move_source_parent: "move source epic",
  move_destination_parent: "move destination epic",
};
const scopeLabels: Record<AmendmentEffect["scope"], string> = {
  direct: "direct",
  descendant: "descendant",
  historical_parent: "historical parent",
};

export function uniqueAmendments(relations: AmendmentRelation[] = []): AmendmentRelation[] {
  const seen = new Set<string>();
  return relations.filter(({ amendment }) => {
    if (seen.has(amendment.id)) return false;
    seen.add(amendment.id);
    return true;
  });
}

export function ProposedAmendmentNotice({ relationships }: { relationships: Relationships }) {
  const proposed = uniqueAmendments(relationships.amendments).filter(({ amendment }) => amendment.status === "proposed");
  if (proposed.length === 0) return null;
  return (
    <aside className="instr-warning" aria-label="Proposed amendments">
      <p>
        {relationships.amendment_history_warning ? "At least " : ""}
        {proposed.length} proposed {proposed.length === 1 ? "amendment concerns" : "amendments concern"} this work.
        {" "}Proposed effects are not approved changes.
      </p>
      <ul>
        {proposed.map(({ amendment }) => (
          <li key={amendment.id}>
            <RefLink node={amendment} label={amendment.title} className="" />
          </li>
        ))}
      </ul>
    </aside>
  );
}

export default function AmendmentRelationsPanel({ relationships }: { relationships: Relationships }) {
  const relations = uniqueAmendments(relationships.amendments);
  return (
    <InstrSection label="Related amendments">
      {relationships.amendment_history_warning && (
        <p className="instr-warning" role="status">{relationships.amendment_history_warning}</p>
      )}
      <ul className="rule-list">
        {relations.map((relation) => (
          <li key={relation.amendment.id} className="instr-amendop">
            <div className="instr-drift instr-amendop-head">
              <RefLink node={relation.amendment} label={relation.amendment.title} className="" />
              <Chip status={relation.amendment.status} />
              {relation.amendment.key && <span className="mono muted">{relation.amendment.key}</span>}
            </div>
            <div className="instr-amendop-detail">
              {relation.proposed_at && <p>Proposed <time dateTime={relation.proposed_at}>{relation.proposed_at}</time></p>}
              {relation.approved_at && <p>Approved <time dateTime={relation.approved_at}>{relation.approved_at}</time></p>}
              {relation.withdrawn_at && <p>Withdrawn <time dateTime={relation.withdrawn_at}>{relation.withdrawn_at}</time></p>}
              <ul>
                {relation.effects.map((effect, index) => (
                  <li key={index}>
                    {effect.operation_index !== undefined && <>Operation {effect.operation_index} · </>}
                    {effect.operation} · {roleLabels[effect.role]} · {scopeLabels[effect.scope]}
                    {effect.node && <> · <RefLink node={effect.node} /></>}
                    {!effect.node && effect.reference && <> · <span className="mono">{effect.reference}</span></>}
                    {effect.note && <> — {effect.note}</>}
                  </li>
                ))}
              </ul>
            </div>
          </li>
        ))}
      </ul>
    </InstrSection>
  );
}
