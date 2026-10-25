import { useMemo, useState } from "react";
import type { MapView, RelationshipDiagnostic } from "../../../api";
import type { MapRelationshipResults } from "./RelationshipResults";

// Incomplete describes extraction impact, not diagnostic identity.
function diagnosticKey(issue: RelationshipDiagnostic): string {
  return JSON.stringify([issue.code, issue.facet, issue.message, issue.source, issue.subjects]);
}

export function useRelationshipContext(view: MapView | null) {
  const [context, setContext] = useState<{
    base: MapView | null;
    mapped: MapRelationshipResults | null;
    query: MapRelationshipResults | null;
    cleared: boolean;
  }>({ base: view, mapped: null, query: null, cleared: false });

  // A successful refresh replaces the graph. Do not merge a query from the
  // previous working tree into it, or keep unreachable snapshots in state.
  // Failed refreshes retain the same view and therefore retain the context.
  if (context.base !== view) {
    setContext({ base: view, mapped: null, query: null, cleared: context.cleared || context.mapped !== null || context.query !== null });
  }

  const inspectionView = useMemo(() => {
    if (view === null || context.base !== view || (!context.mapped && !context.query)) return view;
    const vertices = new Map(view.relationships.vertices.map((vertex) => [vertex.id, vertex]));
    const edges = new Map(view.relationships.edges.map((edge) => [edge.id, edge]));
    const diagnostics = new Map(view.relationships.diagnostics.map((issue) => [diagnosticKey(issue), issue]));
    let complete = view.relationships.complete;
    for (const results of [context.mapped, context.query]) {
      if (!results) continue;
      for (const vertex of results.vertices) vertices.set(vertex.id, vertex);
      for (const edge of results.edges) edges.set(edge.id, edge);
      for (const issue of results.diagnostics) {
        const key = diagnosticKey(issue);
        diagnostics.set(key, { ...issue, incomplete: issue.incomplete || diagnostics.get(key)?.incomplete });
      }
      complete = complete && results.complete;
    }
    return { ...view, relationships: { ...view.relationships, vertices: [...vertices.values()], edges: [...edges.values()], diagnostics: [...diagnostics.values()], complete } };
  }, [view, context]);

  return {
    inspectionView,
    mappedResults: context.mapped,
    clearedOnRefresh: context.cleared,
    selectContext: (query: MapRelationshipResults) => setContext((previous) => ({ ...previous, query, cleared: false })),
    mapResults: (results: MapRelationshipResults) => setContext({ base: view, mapped: results, query: results, cleared: false }),
    clearMappedResults: () => setContext((previous) => ({ ...previous, mapped: null })),
  };
}
