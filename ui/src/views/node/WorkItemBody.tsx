import type { NodePage } from "../../api";
import { GateCardList } from "../../components/GateCard";
import IntegrityNotes from "../../components/IntegrityNotes";
import ProvenanceLine from "../../components/ProvenanceLine";
import ProseSection from "./ProseSection";

export function WorkItemSpecPanel({ page }: { page: NodePage }) {
  const label = `${page.node.noun[0].toUpperCase()}${page.node.noun.slice(1)} contract`;
  return <ProseSection prose={page.prose} label={label} node={page.node} diagnostics={page.backlinks.diagnostics} />;
}

export function WorkItemGatesPanel({ page }: { page: NodePage }) {
  const node = page.node;
  return (
    <>
      <GateCardList node={node} evalPayload={null} />
      <ProvenanceLine
        against={undefined}
        pr={node.pr}
        prUrl={node.pr_url}
        reviewRef={node.review_ref}
        violations={node.policy_violations ?? []}
      />
      <IntegrityNotes attestations={node.attestations ?? []} violations={node.policy_violations ?? []} />
    </>
  );
}
