// The relationship hub, how the graph is walked one page at a time
// (ui-surface.md, "Node detail"): every RelatedNode group the payload carries,
// labeled, minus the groups the type-aware body already pulled forward. The
// spec assigns composition children to the body (a feature shows its tasks, an
// epic its features), so the hub holds the graph edges and never repeats a
// list the page above it already rendered.
//
// Task 2 (node-detail-tabs) lifted the two parent-climb groups, epic and
// feature, into the node header (NodeDetail.tsx's ParentChain, which reads
// these same Relationships fields directly) and dropped source provenance
// from the body entirely - the header's own "from <path>" caption is the one
// place a source now links into Explore's search. Neither group nor the
// source prop this component used to accept belongs here anymore: the hub
// holds graph edges a reader has to keep walking (dependencies, supersession,
// question blocks), not the identity chain the header already states.
//
// `only` is the node-detail tab shell's addition: the
// Dependencies tab wants exactly depends_on/depended_on_by, nothing else, so
// it passes `only` to get that narrow a read rather than the whole unclaimed
// set every other caller (today, just the flat question page) still gets by
// leaving it undefined. It composes with `claimed` rather than replacing it,
// so a caller can still carve a group away from both an `only`-scoped read
// and the wider one.

import type { RelatedNode, Relationships } from "../../api";
import InstrSection from "../../components/InstrSection";
import RelatedList from "./RelatedList";

export type GroupKey = keyof Pick<Relationships,
  "features" | "tasks" | "depends_on" | "depended_on_by" | "duplicate_of" |
  "supersedes" | "superseded_by" | "blocked_by_questions" | "blocks"
>;

// Fixed rendering order: membership, the two dependency directions, duplicate
// target, supersession both ways, and the question edges. Epic and Feature,
// the two parent-climb keys Relationships also carries, are deliberately
// absent here - they render in the node header now, never in the hub.
const groups: { key: GroupKey; label: string }[] = [
  { key: "features", label: "Features" },
  { key: "tasks", label: "Tasks" },
  { key: "depends_on", label: "Depends on" },
  { key: "depended_on_by", label: "Depended on by" },
  { key: "duplicate_of", label: "Duplicate of" },
  { key: "supersedes", label: "Supersedes" },
  { key: "superseded_by", label: "Superseded by" },
  { key: "blocked_by_questions", label: "Blocked by questions" },
  { key: "blocks", label: "Blocks" },
];

// A group arrives as a single link (a parent) or a list; both
// normalize to rows so one renderer covers the hub.
function rowsOf(value: RelatedNode | RelatedNode[] | undefined): RelatedNode[] {
  if (value === undefined) {
    return [];
  }
  return Array.isArray(value) ? value : [value];
}

// dependencyCount is the Dependencies tab's own label count,
// read straight from the payload rather than from whatever the hub itself
// renders, so the count stays exact regardless of which other groups a given
// call scopes in or out.
export function dependencyCount(relationships: Relationships): number {
  return (relationships.depends_on?.length ?? 0) + (relationships.depended_on_by?.length ?? 0);
}

export default function RelationshipHub({
  relationships,
  claimed,
  only,
}: {
  relationships: Relationships;
  claimed: GroupKey[];
  only?: GroupKey[];
}) {
  const present = groups
    .filter((g) => (only === undefined || only.includes(g.key)) && !claimed.includes(g.key))
    .map((g) => ({ ...g, rows: rowsOf(relationships[g.key]) }))
    .filter((g) => g.rows.length > 0);
  // A node with no remaining edges draws no hub at all. "(none recorded)"
  // under a heading was two lines saying the same thing the absence says, and
  // an isolated node is the ordinary case early in a tree's life, not an
  // anomaly worth a line of its own.
  if (present.length === 0) {
    return null;
  }
  return (
    <>
      {present.map((g) => (
        <InstrSection key={g.key} label={g.label} caption={`${g.rows.length}`}>
          <RelatedList items={g.rows} />
        </InstrSection>
      ))}
    </>
  );
}
