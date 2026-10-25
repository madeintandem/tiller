// The task body. A task is a folder node with an implementation spec of its own
// now (artifact-schema.md, Tasks), so the body opens with that spec and then
// carries what it always did: the recorded gates (build/tests; evals only when
// the manifest scopes it to task grain), provenance, and integrity notes. That
// is the order ui-surface.md's node detail names for this noun, "a task shows
// its implementation spec, its gates and evidence, and its PR", and the same
// order the feature body reads in, spec before what was checked against it.
//
// task-1-brief.md's tab shell split this file into the panels the task tab
// set names - spec/gates, moved here JSX for JSX from the one body function
// they used to share. It carried a third piece too, TaskFeaturePanel, the
// parent-feature climb, mounted as an interim bridge inside the Dependencies
// tab until task 2 (node-detail-tabs) had somewhere better for it: the node
// header's own parent climb (components/NodeDetail.tsx), which reads
// relationships.feature/epic directly and states the same dangling-parent
// note ("Parent feature not found") this panel used to. That retires the
// bridge, so this file no longer carries the climb at all. Task 17 (console
// v2, node page rebuild) reshaped that climb again, from a plain parent list
// (ParentChain) into the breadcrumb chain (Crumbs), same data, same
// dangling-parent guard, ancestor order flipped to epic-then-feature. The
// other two of a task's four tabs (dependencies' own depends_on/
// depended_on_by read, history) live in NodeDetail.tsx, same as every other
// noun's.

import type { NodePage } from "../../api";
import { GateCardList } from "../../components/GateCard";
import IntegrityNotes from "../../components/IntegrityNotes";
import ProvenanceLine from "../../components/ProvenanceLine";
import ProseSection from "./ProseSection";

export function TaskSpecPanel({ page }: { page: NodePage }) {
  return <ProseSection prose={page.prose} label="Implementation spec" node={page.node} diagnostics={page.backlinks.diagnostics} />;
}

export function TaskGatesPanel({ page }: { page: NodePage }) {
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
