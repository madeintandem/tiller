// The feature body (task-1-brief.md: the node detail tab shell), decomposed
// into the three panels the feature tab set names - spec/evals/tasks, the
// other two of its five tabs (dependencies, history) live in NodeDetail.tsx,
// the same as every other noun's: the spec prose; the gate cards (the
// authored eval contract decorated with recorded results, plus
// build/tests/any review gates), the provenance line, and integrity notes;
// and the tasks. The eval contract renders before the recorded gates so the
// human reads what was authored before what was checked against it. Tasks
// are this body's composition edge, so the Dependencies tab's hub skips
// them (NodeDetail's claimedGroups, unchanged by this split).

import type { NodePage } from "../../api";
import { GateCardList } from "../../components/GateCard";
import IntegrityNotes from "../../components/IntegrityNotes";
import ProvenanceLine from "../../components/ProvenanceLine";
import TaskList from "../../components/TaskList";
import ProseSection from "./ProseSection";

export function FeatureSpecPanel({ page }: { page: NodePage }) {
  return <ProseSection prose={page.prose} label="Spec" node={page.node} diagnostics={page.backlinks.diagnostics} />;
}

export function FeatureEvalsPanel({ page }: { page: NodePage }) {
  const node = page.node;
  return (
    <>
      <GateCardList node={node} evalPayload={page.eval} />
      <ProvenanceLine
        against={node.gates?.evals?.against}
        pr={node.pr}
        prUrl={node.pr_url}
        reviewRef={node.review_ref}
        violations={node.policy_violations ?? []}
      />
      <IntegrityNotes attestations={node.attestations ?? []} violations={node.policy_violations ?? []} />
    </>
  );
}

export function FeatureTasksPanel({ page }: { page: NodePage }) {
  const tasks = page.relationships.tasks ?? [];
  return <TaskList tasks={tasks} />;
}
