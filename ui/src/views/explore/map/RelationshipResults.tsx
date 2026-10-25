import { useMemo, useRef, useState } from "react";
import type { RelationshipDiagnostic, RelationshipEdge, RelationshipGraph, RelationshipQueryResult, RelationshipVertex } from "../../../api";
import { relationshipQueryPath } from "../../../api";
import CopyCmd from "../../../components/CopyCmd";
import { useFetch } from "../../../hooks";
import { relationshipTitle } from "../../../lib/relationshipTitle";
import ReferenceIssues from "./ReferenceIssues";
import { RelationshipEndpoint, RelationshipEvidence } from "./RelationshipDetails";
import { matchesRelationshipKind, relationshipLabel, relationshipQueryCommand } from "./relationshipPresentation";
import { WORK_LINK_PRESET } from "./relationships";

const PAGE_SIZE = 25;
const questions = [
  { label: "Prerequisites", follow: "depends_on", address: "work" },
  { label: "Recorded dependents", follow: "depended_on_by", address: "work" },
  { label: "Work using a source document", follow: "source_for", address: "document" },
  { label: "Declarations naming a code path", follow: "covered_by", address: "path" },
  { label: "Amendments concerning an item", follow: "amended_by", address: "work" },
  { label: "Documents mentioning an item", follow: "mentioned_by", address: "work" },
];

export interface MapRelationshipResults {
  edges: readonly RelationshipEdge[];
  vertices: readonly RelationshipVertex[];
  diagnostics: readonly RelationshipDiagnostic[];
  complete: boolean;
}

export default function RelationshipResults({ graph, issuesGraph = graph, selected, mode, onMode, onSelect, onShow, onClose }: {
  graph: RelationshipGraph;
  issuesGraph?: RelationshipGraph;
  selected: string | null;
  mode: "relationships" | "issues";
  onMode: (mode: "relationships" | "issues") => void;
  onSelect: (id: string, context?: MapRelationshipResults) => void;
  onShow: (results: MapRelationshipResults) => void;
  onClose: () => void;
}) {
  const [scope, setScope] = useState("project");
  const [kind, setKind] = useState("");
  const [direction, setDirection] = useState("both");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [question, setQuestion] = useState(0);
  const [address, setAddress] = useState(selected ?? "");
  const [follow, setFollow] = useState(questions[0].follow);
  const [query, setQuery] = useState<{ from: string; follow: string; submission: number } | null>(null);
  const result = useFetch<RelationshipQueryResult>(query ? relationshipQueryPath(query.from, query.follow) : null, { refreshKey: query ? String(query.submission) : undefined });
  const bodyRef = useRef<HTMLDivElement>(null);
  const resultGraph = scope === "query" ? result.data : graph;
  const byID = useMemo(() => new Map((resultGraph?.vertices ?? []).map((vertex) => [vertex.id, vertex])), [resultGraph]);
  const starts = useMemo(() => new Set(scope === "selected" ? selected ? [selected] : [] : scope === "query" ? result.data?.starts ?? [] : []), [scope, selected, result.data]);
  const rows = useMemo(() => {
    const text = search.trim().toLocaleLowerCase();
    return (resultGraph?.edges ?? []).filter((edge) => {
      if (!matchesRelationshipKind(edge, kind)) return false;
      if (scope === "selected" && !starts.has(edge.from) && !starts.has(edge.to)) return false;
      if (scope !== "project" && direction === "out" && !starts.has(edge.from)) return false;
      if (scope !== "project" && direction === "in" && !starts.has(edge.to)) return false;
      if (!text) return true;
      const from = byID.get(edge.from);
      const to = byID.get(edge.to);
      return [edge.from, edge.to, edge.kind, relationshipLabel(edge.kind), from && relationshipTitle(from), to && relationshipTitle(to), from?.path, to?.path, ...edge.sources.flatMap((source) => [source.file, source.field, source.raw, source.criterion])]
        .some((value) => value?.toLocaleLowerCase().includes(text));
    });
  }, [resultGraph, search, kind, scope, starts, direction, byID]);
  const currentPage = Math.min(page, Math.max(0, Math.ceil(rows.length / PAGE_SIZE) - 1));
  const visible = rows.slice(currentPage * PAGE_SIZE, (currentPage + 1) * PAGE_SIZE);
  const context = { edges: resultGraph?.edges ?? [], vertices: resultGraph?.vertices ?? [], diagnostics: resultGraph?.diagnostics ?? [], complete: resultGraph?.complete ?? false };
  const selectResult = (id: string) => onSelect(id, scope === "query" ? context : undefined);
  const changePage = (next: number) => {
    setPage(next);
    bodyRef.current?.scrollTo?.({ top: 0 });
  };
  const queryGraph: RelationshipGraph | null = result.data ? { ...result.data, catalog: graph.catalog } : null;
  if (scope === "selected" && selected === null) {
    setScope("project");
    setPage(0);
  }
  return <aside className="map-relationship-results" aria-label="Relationship investigation">
    <div className="map-results-head">
      <h2>Relationships</h2>
      <button type="button" className="map-dock-close" aria-label="Close relationship investigation" onClick={onClose}>×</button>
      <div className="map-results-tabs">
        <button type="button" aria-pressed={mode === "relationships"} onClick={() => onMode("relationships")}>Browse relationships</button>
        <button type="button" aria-pressed={mode === "issues"} onClick={() => onMode("issues")}>Project reference issues ({issuesGraph.diagnostics.length})</button>
      </div>
    </div>
    <div className="map-results-body" ref={bodyRef}>
      {mode === "issues" ? <ReferenceIssues graph={issuesGraph} selected={selected} scope="project" onSelect={onSelect} /> : <>
        <div className="map-result-filters">
          <label>Scope<select aria-label="Result scope" value={scope} onChange={(event) => { setScope(event.target.value); setPage(0); }}>
            <option value="project">Whole project</option><option value="selected" disabled={selected === null}>Selected item</option>{query && <option value="query">Read-only query</option>}
          </select></label>
          <label>Direction<select aria-label="Result direction" value={direction} disabled={scope === "project"} onChange={(event) => { setDirection(event.target.value); setPage(0); }}>
            <option value="both">Both directions</option><option value="out">From selected / query start</option><option value="in">To selected / query start</option>
          </select></label>
          <label>Kind<select aria-label="Result kind" value={kind} onChange={(event) => { setKind(event.target.value); setPage(0); }}>
            <option value="">All kinds (including coordinates)</option><option value={WORK_LINK_PRESET}>Work links</option>
            {graph.catalog.map((entry) => <option key={entry.name} value={entry.name}>{relationshipLabel(entry.name)} · {entry.name}</option>)}
          </select></label>
          <label>Search<input type="search" aria-label="Search relationships" placeholder="Title, ID, path, source, or criterion" value={search} onChange={(event) => { setSearch(event.target.value); setPage(0); }} /></label>
        </div>
        {scope === "selected" && <p className="map-relation-meta">Selected item: {selected ? byID.get(selected) ? relationshipTitle(byID.get(selected)!) : selected : "None"}. Direction follows recorded edges, not dependency arrow layout.</p>}
        <details className="map-query-presets">
          <summary>Engineering questions · read-only queries</summary>
          <form onSubmit={(event) => {
            event.preventDefault();
            if (!address.trim() || !follow) return;
            setQuery((previous) => ({ from: address.trim(), follow, submission: (previous?.submission ?? 0) + 1 })); setScope("query"); setKind(""); setDirection("both"); setSearch(""); setPage(0);
          }}>
            <label>Question<select aria-label="Engineering question" value={question} onChange={(event) => {
              const next = Number(event.target.value); const preset = questions[next];
              setQuestion(next); setFollow(preset.follow);
              const vertex = graph.vertices.find((entry) => entry.id === selected);
              setAddress(preset.address === "document" ? vertex?.kind === "document" ? vertex.id : "doc:" : preset.address === "path" ? vertex?.kind === "path" ? vertex.id : "path:" : selected ?? "");
            }}>{questions.map((entry, i) => <option key={entry.follow} value={i}>{entry.label}</option>)}</select></label>
            <label>Start address<input aria-label="Query start address" value={address} onChange={(event) => setAddress(event.target.value)} placeholder="Immutable ID, doc:path, or path:code/file" required /></label>
            <label>Follow<select aria-label="Query follow direction" value={follow} onChange={(event) => setFollow(event.target.value)}>
              {graph.catalog.flatMap((entry) => [<option key={entry.name} value={entry.name}>{entry.name}</option>, <option key={entry.inverse} value={entry.inverse}>{entry.inverse}</option>])}
            </select></label>
            {selected && <button type="button" onClick={() => setAddress(selected)}>Use selected ID</button>}
            <button type="submit">Run read-only query</button>
            {address.trim() && follow && <CopyCmd cmd={relationshipQueryCommand(address.trim(), follow)} />}
          </form>
          <p className="muted">Queries traverse one recorded hop. Code-path queries also match enclosing declared scopes; they do not verify implementation. Generic question edges are not effective holds.</p>
        </details>
        {scope === "query" && query && <p className="map-relation-meta">Query: {query.from} · {query.follow} · depth 1{result.data && ` · ${result.data.matched.length} matched endpoints`}</p>}
        {scope === "query" && result.loading && <p role="status">Reading relationships…</p>}
        {scope === "query" && result.error && <p role="alert">Query unavailable: {result.error}</p>}
        {scope === "query" && queryGraph && (!queryGraph.complete || queryGraph.diagnostics.length > 0) && <ReferenceIssues graph={queryGraph} selected={selected} scope="query" onSelect={selectResult} />}
        <p role="status" className="map-result-count">{scope === "project" ? "Whole project" : scope === "selected" ? "Selected item" : "Read-only query"} · {scope === "project" ? "Both directions" : direction === "out" ? "Outgoing" : direction === "in" ? "Incoming" : "Both directions"} · {kind ? kind === WORK_LINK_PRESET ? "Work links" : relationshipLabel(kind) : "All kinds"} · {rows.length} relationships{rows.length > 0 && ` · showing ${currentPage * PAGE_SIZE + 1}–${Math.min(rows.length, (currentPage + 1) * PAGE_SIZE)}`}</p>
        <button type="button" disabled={visible.length === 0} onClick={() => onShow({ ...context, edges: visible })}>Show results on map</button>
        <p className="map-relation-meta">Maps this page only (up to {PAGE_SIZE} relationships). Browsing and searching do not expand the graph. Full graph is a separate toolbar choice.</p>
        <ol className="map-result-list" start={currentPage * PAGE_SIZE + 1}>
          {visible.map((edge) => <li key={edge.id} className="map-result-row">
            <p className="map-result-kind">{relationshipLabel(edge.kind)} <span className="muted">· {edge.kind}</span></p>
            <div className="map-result-endpoints">{[edge.from, edge.to].map((id, index) => <div key={`${id}:${index}`}><small className="muted">{index === 0 ? "From" : "To"}</small>{byID.has(id) ? <RelationshipEndpoint vertex={byID.get(id)!} edge={edge} onSelect={selectResult} /> : <code>{id}</code>}</div>)}</div>
            <RelationshipEvidence edge={edge} />
          </li>)}
        </ol>
        {rows.length === 0 && !result.loading && <p>No matching relationships in this scope.</p>}
      </>}
    </div>
    {mode === "relationships" && <div className="map-result-pagination">
      <button type="button" disabled={currentPage === 0} onClick={() => changePage(currentPage - 1)}>Previous results</button>
      <span>Page {currentPage + 1} of {Math.max(1, Math.ceil(rows.length / PAGE_SIZE))}</span>
      <button type="button" disabled={(currentPage + 1) * PAGE_SIZE >= rows.length} onClick={() => changePage(currentPage + 1)}>Next results</button>
    </div>}
  </aside>;
}
