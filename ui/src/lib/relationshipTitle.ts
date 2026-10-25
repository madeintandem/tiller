import type { RelationshipVertex } from "../api";

export function relationshipTitle(vertex: RelationshipVertex): string {
  return vertex.title || vertex.key || vertex.path || vertex.raw || vertex.id;
}
