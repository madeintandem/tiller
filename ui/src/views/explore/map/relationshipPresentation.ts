import type { RelationshipDiagnostic, RelationshipEdge, RelationshipGraph, RelationshipVertex } from "../../../api";
import { relationshipFileHref, relationshipHref } from "../../../lib/relationshipHref";
import { WORK_LINK_KINDS, WORK_LINK_PRESET } from "./relationships";

const labels: Record<string, readonly [string, string]> = {
  depends_on: ["Depends on", "Required by"],
  contains: ["Children", "Parent"],
  has_document: ["Work documents", "Document of"],
  sourced_from: ["Sources", "Source for"],
  derived_from: ["Derived from", "Derivation of"],
  mentions: ["References to work", "Referenced in documents"],
  attached_to: ["Found in", "Attached work"],
  resolved_by: ["Carried by", "Carries"],
  duplicate_of: ["Duplicate of", "Recorded duplicates"],
  blocks: ["Recorded question targets", "Recorded questions"],
  assumes: ["Recorded assumptions", "Assumed by"],
  amends: ["Amendment targets", "Amendments"],
  supersedes: ["Replaces", "Replaced by"],
  triggered_by: ["Prompted by", "Prompted amendments"],
  amends_design: ["Design updates", "Design amendments"],
  implements: ["Recorded ADR references", "Referenced by work"],
  covers: ["Declared code scope", "Declared by"],
  preserves: ["Preservation context", "Preserved by"],
  at_path: ["Document coordinate", "Documents at this coordinate"],
};

export function relationshipLabel(kind: string, inverse = false): string {
  return labels[kind]?.[inverse ? 1 : 0] ?? kind.replaceAll("_", " ");
}

export function matchesRelationshipKind(edge: RelationshipEdge, kind: string): boolean {
  return kind === WORK_LINK_PRESET
    ? WORK_LINK_KINDS[edge.kind] === true
    : kind === "" || edge.kind === kind;
}

export interface SelectedRelationshipGroup {
  kind: string;
  inverse: boolean;
  label: string;
  edges: RelationshipEdge[];
}

export function selectedRelationshipGroups(edges: readonly RelationshipEdge[], selected: string, kind: string): SelectedRelationshipGroup[] {
  const groups = new Map<string, SelectedRelationshipGroup>();
  for (const edge of edges) {
    if ((edge.from !== selected && edge.to !== selected) || !matchesRelationshipKind(edge, kind)) continue;
    // Coordinates live in document metadata unless explicitly requested.
    if (edge.kind === "at_path" && kind === "") continue;
    const inverse = edge.from !== selected;
    const key = `${edge.kind}:${inverse}`;
    let group = groups.get(key);
    if (!group) {
      group = { kind: edge.kind, inverse, label: relationshipLabel(edge.kind, inverse), edges: [] };
      groups.set(key, group);
    }
    group.edges.push(edge);
  }
  const order = Object.keys(labels);
  return [...groups.values()].sort((a, b) => order.indexOf(a.kind) - order.indexOf(b.kind) || Number(a.inverse) - Number(b.inverse));
}

// A citation can be more precise than the document's default destination. It
// still needs the server-granted document capability and a safe file locator.
export function counterpartHref(vertex: RelationshipVertex, edge: RelationshipEdge): string | null {
  const href = relationshipHref(vertex);
  if (href === null || vertex.destination?.kind !== "file") return href;
  const path = vertex.destination.path;
  for (const source of edge.sources) {
    if (source.citation_path === path) return relationshipFileHref(path!, source.citation_line ?? 1) ?? href;
    if (source.file === path && source.line !== undefined) return relationshipFileHref(path, source.line) ?? href;
  }
  return href;
}

export function scopedReferenceIssues(graph: RelationshipGraph, selected: string | null): { local: RelationshipDiagnostic[]; global: RelationshipDiagnostic[] } {
  const subjects = new Set(selected === null ? [] : [selected]);
  if (selected !== null) {
    for (const vertex of graph.vertices) if (vertex.document_owner === selected) subjects.add(vertex.id);
    for (const edge of graph.edges) if (edge.kind === "has_document" && edge.from === selected) subjects.add(edge.to);
  }
  return {
    local: graph.diagnostics.filter((issue) => issue.subjects?.some((id) => subjects.has(id))),
    // Do not reimplement the backend's extraction-impact classification.
    global: graph.diagnostics.filter((issue) => issue.incomplete || !issue.subjects?.length),
  };
}

function shellArgument(value: string): string {
  return /^[A-Za-z0-9_./:-]+$/.test(value) ? value : `'${value.replaceAll("'", `'"'"'`)}'`;
}

export function relationshipQueryCommand(from: string, follow: string, depth?: number): string {
  return `tiller query --from ${shellArgument(from)} --follow ${shellArgument(follow)}${depth !== undefined ? ` --depth ${depth}` : ""} --json`;
}
