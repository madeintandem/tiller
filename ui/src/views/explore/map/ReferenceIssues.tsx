import { useState } from "react";
import { Link } from "react-router-dom";
import type { RelationshipDiagnostic, RelationshipGraph, RelationshipVertex } from "../../../api";
import CopyCmd from "../../../components/CopyCmd";
import SourceCitation from "../../../components/relationships/SourceCitation";
import { relationshipFileHref, relationshipHref } from "../../../lib/relationshipHref";
import { relationshipTitle } from "../../../lib/relationshipTitle";
import { relationshipQueryCommand, scopedReferenceIssues } from "./relationshipPresentation";

// These are presentation causes, not severities or extraction-impact rules.
function issueCause(issue: RelationshipDiagnostic): { label: string; next: string } {
  if (issue.code === "unresolved_reference" && issue.facet === "provenance") return {
    label: "Unsupported citation format",
    next: "Tiller could not interpret this recorded value as a supported locator. Inspect the original reference; this does not establish that a file is missing.",
  };
  if (issue.code === "unresolved_amendment_endpoint" || issue.code === "amendment_history_incomplete") return {
    label: "Unresolved historical context",
    next: "Inspect the amendment operation and recorded evidence. Historical keys are not rebound to their current holders; this is not an operative work hold.",
  };
  if (issue.code === "ambiguous_reference" || issue.code === "conflicting_supersession") return {
    label: "Ambiguous reference",
    next: "Compare the recorded identities and source occurrences. Tiller has not chosen a target or repaired the relationship.",
  };
  if (["unresolved_reference", "unresolved_mention", "unresolved_document"].includes(issue.code)) return {
    label: "Missing target",
    next: "Inspect the affected work and the exact source before changing the recorded target. No automatic repair is available.",
  };
  if (["corpus_unreadable", "document_unreadable", "contract_unavailable", "contract_path_unavailable"].includes(issue.code)) return {
    label: "Input could not be read",
    next: "Inspect the named input and the read failure. Unexamined input is not evidence of a missing target.",
  };
  if (issue.code === "contract_invalid" || issue.code === "contract_owner_mismatch") return {
    label: "Invalid acceptance contract",
    next: "Inspect the contract's syntax, owning feature, and criteria. Omitted contract relationships do not prove absent coverage or completed work.",
  };
  if (issue.code === "unresolved_citation" || issue.code === "invalid_path") return {
    label: "Unavailable citation coordinate",
    next: "Inspect the original path and line coordinate. Tiller has not granted a destination for this reference.",
  };
  return { label: "Other reference issues", next: "Inspect the source and reported limitation. Extraction completeness does not mean every reference resolved or the project is healthy." };
}

function IssueOccurrence({ issue, byID, onSelect }: {
  issue: RelationshipDiagnostic;
  byID: ReadonlyMap<string, RelationshipVertex>;
  onSelect: (id: string) => void;
}) {
  const source = issue.source;
  const sourceVertex = source ? byID.get(`doc:${source.file}`) : undefined;
  const citationVertex = source?.citation_path ? byID.get(`doc:${source.citation_path}`) : undefined;
  const allowSourceNavigation = sourceVertex !== undefined && relationshipHref(sourceVertex) !== null && sourceVertex.destination?.kind === "file";
  const allowCitationNavigation = issue.code !== "unresolved_citation" && citationVertex !== undefined && relationshipHref(citationVertex) !== null && citationVertex.destination?.kind === "file";
  const sourceHref = source && allowSourceNavigation ? relationshipFileHref(source.file, source.line ?? 1) : null;
  const subjects = (issue.subjects ?? []).map((id) => byID.get(id) ?? { id, kind: "unresolved", unresolved: true });
  const querySubject = subjects.find((vertex) => vertex.destination?.kind === "node") ?? subjects.find((vertex) => byID.has(vertex.id));
  return <li className="map-reference-occurrence">
    <p>{issue.message}</p>
    {subjects.length === 0 ? <p className="muted">Project-wide input</p> : <ul className="map-issue-subjects">{subjects.map((vertex) => {
      const href = relationshipHref(vertex);
      return <li key={vertex.id}><span>{relationshipTitle(vertex)}{vertex.unresolved ? " (unresolved)" : ""}</span>
        {byID.has(vertex.id) && <button type="button" className="map-card-link" onClick={() => onSelect(vertex.id)}>Inspect affected item</button>}
        {href !== null && <Link to={href}>Open affected {vertex.destination?.kind === "file" ? "document" : "work"}</Link>}
      </li>;
    })}</ul>}
    {source && <>
      <ul className="map-relation-sources"><SourceCitation source={source} allowSourceNavigation={allowSourceNavigation} allowCitationNavigation={allowCitationNavigation} /></ul>
      {sourceHref !== null && <Link to={sourceHref}>Open source document</Link>}
      <div><span className="map-copy-label">Copy source path and field</span><CopyCmd cmd={`${source.file}${source.line ? `#L${source.line}` : ""}${source.field ? ` · ${source.field}` : ""}${source.operation !== undefined ? ` · operation ${source.operation}` : ""}`} /></div>
      {source.raw && <div><span className="map-copy-label">Copy original reference</span><CopyCmd cmd={source.raw} /></div>}
    </>}
    {querySubject && <div><span className="map-copy-label">Inspect subject and diagnostics (read-only, no traversal)</span><CopyCmd cmd={relationshipQueryCommand(querySubject.id, "mentioned_by", 0)} /></div>}
    <small className="muted">{issue.facet} · {issue.code}{issue.incomplete ? " · Extraction incomplete" : ""}</small>
  </li>;
}

function IssueGroup({ label, next, issues, byID, onSelect }: {
  label: string;
  next: string;
  issues: RelationshipDiagnostic[];
  byID: ReadonlyMap<string, RelationshipVertex>;
  onSelect: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [page, setPage] = useState(0);
  const current = Math.min(page, Math.max(0, Math.ceil(issues.length / 20) - 1));
  return <section className="map-issue-group" aria-label={label}>
    <button type="button" className="map-card-link" aria-expanded={expanded} onClick={() => setExpanded(!expanded)}>{label} ({issues.length} occurrences)</button>
    {expanded && <>
      <p>{next}</p>
      <ol start={current * 20 + 1}>{issues.slice(current * 20, current * 20 + 20).map((issue, i) => <IssueOccurrence key={`${current}:${i}`} issue={issue} byID={byID} onSelect={onSelect} />)}</ol>
      {issues.length > 20 && <div className="map-result-pagination">
        <button type="button" disabled={current === 0} onClick={() => setPage(current - 1)}>Previous occurrences</button>
        <span>{current * 20 + 1}–{Math.min(issues.length, current * 20 + 20)} of {issues.length}</span>
        <button type="button" disabled={(current + 1) * 20 >= issues.length} onClick={() => setPage(current + 1)}>Next occurrences</button>
      </div>}
    </>}
  </section>;
}

export default function ReferenceIssues({ graph, selected, scope = "selected", onSelect }: {
  graph: RelationshipGraph;
  selected: string | null;
  scope?: "selected" | "project" | "query";
  onSelect: (id: string) => void;
}) {
  const scoped = scope === "selected" ? scopedReferenceIssues(graph, selected) : null;
  const issues = scoped ? scoped.local : graph.diagnostics;
  const byID = new Map(graph.vertices.map((vertex) => [vertex.id, vertex]));
  const groups = new Map<string, { next: string; issues: RelationshipDiagnostic[] }>();
  for (const issue of issues) {
    const { label, next } = issueCause(issue);
    const group = groups.get(label);
    if (group) group.issues.push(issue);
    else groups.set(label, { next, issues: [issue] });
  }
  return <section className="map-reference-issues" aria-label={scope === "query" ? "Query reference issues" : scope === "project" ? "Project reference issues" : "Selected reference issues"}>
    <h3 className="eyebrow">Reference issues ({issues.length})</h3>
    <p className="map-relation-meta">{scope === "query" ? "Query subjects and scan-wide warnings" : scope === "project" ? "Whole project" : "Selected item and its owned documents"} · Counts source occurrences, not affected nodes</p>
    {scope !== "selected" && !graph.complete && <p role="status">Extraction is incomplete. Unread or omitted input anywhere in the project can conceal incoming relationships.</p>}
    {scope !== "selected" && graph.complete && <p className="muted">Extraction completed. This does not mean every reference resolved or the project is healthy.</p>}
    {issues.length === 0 && <p className="muted">No recorded reference issues in this scope.</p>}
    {[...groups].map(([label, group]) => <IssueGroup key={`${selected}:${scope}:${label}`} label={label} next={group.next} issues={group.issues} byID={byID} onSelect={onSelect} />)}
    {scoped && scoped.global.length > 0 && <p className="muted">{scoped.global.length} scan-wide warnings remain in Project reference issues. They are not counted as this item's issues.</p>}
  </section>;
}
