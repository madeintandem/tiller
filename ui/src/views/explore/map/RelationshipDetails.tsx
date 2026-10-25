import type { ReactElement, ReactNode } from "react";
import { useState } from "react";
import { Link } from "react-router-dom";
import type { RelationshipEdge, RelationshipGraph, RelationshipVertex } from "../../../api";
import Chip from "../../../components/Chip";
import Contributors from "../../../components/Contributors";
import CopyCmd from "../../../components/CopyCmd";
import SourceCitation from "../../../components/relationships/SourceCitation";
import { kindLabel } from "../../../lib/kind";
import { relationshipHref } from "../../../lib/relationshipHref";
import { relationshipTitle } from "../../../lib/relationshipTitle";
import { PersonName } from "../../../people";
import { counterpartHref, relationshipLabel, relationshipQueryCommand, selectedRelationshipGroups } from "./relationshipPresentation";
import type { SelectedRelationshipGroup } from "./relationshipPresentation";
import { relationshipStroke, WORK_LINK_PRESET } from "./relationships";

export function RelationshipEvidence({ edge }: { edge: RelationshipEdge }): ReactElement {
  const [expanded, setExpanded] = useState(false);
  return <div className="map-relation-evidence">
    <button type="button" className="map-card-link" aria-expanded={expanded} onClick={() => setExpanded(!expanded)}>Why linked?</button>
    {expanded && <div className="map-relation">
      <p className="mono">{edge.from} → {edge.kind} → {edge.to}</p>
      <p>{edge.derived ? "Derived match" : "Recorded relationship"}{edge.declared_scope ? ` · Declared scope: ${edge.declared_scope}` : ""}</p>
      {(edge.kind === "covers" || edge.kind === "preserves") && <p>Recorded declaration, not verified implementation or file existence. The source identifies its work surface, baseline, or acceptance binding.</p>}
      {edge.kind === "blocks" && <p>Recorded question target, not an effective hold. Current work status determines operative holds.</p>}
      {edge.kind === "amends" && <p>Operation state, evidence, and direct or ancestor-summary scope are recorded below. Proposed intent is not an applied change.</p>}
      <ul className="map-relation-sources">{edge.sources.map((source, i) => <SourceCitation key={i} source={source} />)}</ul>
      <CopyCmd cmd={relationshipQueryCommand(edge.from, edge.kind)} />
    </div>}
  </div>;
}

export function RelationshipEndpoint({ vertex, edge, onSelect }: {
  vertex: RelationshipVertex;
  edge: RelationshipEdge;
  onSelect: (id: string) => void;
}): ReactElement {
  const href = counterpartHref(vertex, edge);
  return <>
    <div className="map-relation-title">
      <span>{relationshipTitle(vertex)}{vertex.unresolved ? " (unresolved)" : ""}</span>
      {!vertex.unresolved && vertex.status && <Chip status={vertex.status} />}
    </div>
    {vertex.kind === "document" && <p className="map-relation-meta">{vertex.generated ? "Generated document" : "Authored document"}{vertex.path && ` · ${vertex.path}`}</p>}
    <div className="map-relation-actions">
      <button type="button" className="map-card-link" aria-label={`Inspect on map: ${relationshipTitle(vertex)}`} onClick={() => onSelect(vertex.id)}>Inspect on map</button>
      {href !== null && <Link to={href}>Open{vertex.destination?.kind === "file" ? " source" : ""}</Link>}
    </div>
  </>;
}

function RelationshipGroup({ group, selected, byID, onSelect }: {
  group: SelectedRelationshipGroup;
  selected: string;
  byID: ReadonlyMap<string, RelationshipVertex>;
  onSelect: (id: string) => void;
}): ReactElement {
  const [limit, setLimit] = useState(25);
  const statuses = new Map<string, number>();
  for (const edge of group.edges) {
    const vertex = byID.get(edge.from === selected ? edge.to : edge.from);
    if (vertex?.status && !vertex.unresolved) statuses.set(vertex.status, (statuses.get(vertex.status) ?? 0) + 1);
  }
  return <section className="map-dock-group" aria-label={group.label}>
    <div className="map-dock-label"><h3 className="eyebrow">{group.label}</h3><span className="map-dock-count">{group.edges.length}</span></div>
    {group.kind === "depends_on" && statuses.size > 0 && <p className="map-relation-meta">{[...statuses].map(([status, count]) => `${count} ${status}`).join(" · ")}</p>}
    <ul className="map-dock-tree">
      {group.edges.slice(0, limit).map((edge) => {
        const id = edge.from === selected ? edge.to : edge.from;
        const vertex = byID.get(id);
        return <li key={edge.id}><div className="map-relation-row">
          {vertex ? <RelationshipEndpoint vertex={vertex} edge={edge} onSelect={onSelect} /> : <code>{id}</code>}
          <RelationshipEvidence edge={edge} />
        </div></li>;
      })}
    </ul>
    {group.edges.length > limit && <button type="button" onClick={() => setLimit(limit + 25)}>Show next 25 of {group.edges.length - limit} remaining</button>}
  </section>;
}

export function SelectedRelationships({ selected, edges, byID, kind = "", onSelect }: {
  selected: string;
  edges: readonly RelationshipEdge[];
  byID: ReadonlyMap<string, RelationshipVertex>;
  kind?: string;
  onSelect: (id: string) => void;
}): ReactElement {
  const groups = selectedRelationshipGroups(edges, selected, kind);
  const count = groups.reduce((sum, group) => sum + group.edges.length, 0);
  return <div className="map-selected-relations">
    <p className="map-relation-scope">Selected item · {kind === WORK_LINK_PRESET ? "Work links" : kind ? relationshipLabel(kind) : "All engineering kinds"} · {count} relationships</p>
    {groups.length === 0 && <p className="muted">No matching relationships.</p>}
    {groups.map((group) => <RelationshipGroup key={`${selected}:${kind}:${group.kind}:${group.inverse}`} group={group} selected={selected} byID={byID} onSelect={onSelect} />)}
  </div>;
}

export function RelationshipLegend({ graph, edges, kind, scope, onKind }: {
  graph: RelationshipGraph;
  edges: readonly RelationshipEdge[];
  kind: string;
  scope: string;
  onKind: (kind: string) => void;
}): ReactElement {
  const counts = new Map<string, number>();
  for (const edge of edges) counts.set(edge.kind, (counts.get(edge.kind) ?? 0) + 1);
  return <aside className="map-relationship-legend" aria-label="Map key">
    <details>
      <summary>Map key</summary>
      <p className="map-relation-meta">{scope} · {edges.length} relationships, not wire or source counts</p>
      <button type="button" className="map-card-link" aria-pressed={kind === ""} onClick={() => onKind("")}>All kinds</button>
      <ul className="map-legend-kinds">
        {graph.catalog.filter((entry) => counts.has(entry.name) || entry.name === kind || entry.name === "depends_on" || entry.name === "attached_to").map((entry) => {
          const stroke = relationshipStroke(entry.name, graph.catalog);
          const label = entry.name === "depends_on" ? "Prerequisite → dependent" : entry.name === "attached_to" ? "Found in (undirected)" : entry.name === "contains" ? "Parent → child (tree)" : relationshipLabel(entry.name);
          return <li key={entry.name}><button type="button" aria-pressed={kind === entry.name} onClick={() => onKind(kind === entry.name ? "" : entry.name)}>
            <svg width="40" height="12" aria-hidden="true"><path d="M 0 6 H 35" stroke={stroke.color} strokeDasharray={stroke.dash} />{entry.name !== "attached_to" && <path d="m 30 2 6 4 -6 4" fill="none" stroke={stroke.color} />}</svg>
            <span>{label} ({counts.get(entry.name) ?? 0})</span>
          </button></li>;
        })}
      </ul>
      <p className="muted">Choose a line to filter the map and inspector. Other arrows follow recorded direction; collapsed endpoints land on their visible ancestor.</p>
    </details>
  </aside>;
}

export function RelationshipCard({ vertex, edges, byID, kind, onSelect, onClose, children }: {
  vertex: RelationshipVertex;
  edges: readonly RelationshipEdge[];
  byID: ReadonlyMap<string, RelationshipVertex>;
  kind: string;
  onSelect: (id: string) => void;
  onClose: () => void;
  children?: ReactNode;
}): ReactElement {
  const href = relationshipHref(vertex);
  return <aside className="map-dock map-endpoint-dock" aria-label="Selected relationship endpoint">
    <div className="map-dock-head">
      <div className="kind-line"><span className="kind-pill">{kindLabel(vertex.kind)}</span>{!vertex.unresolved && vertex.status && <Chip status={vertex.status} />}</div>
      <button type="button" className="map-dock-close" aria-label="Close" onClick={onClose}>×</button>
    </div>
    <div className="map-dock-body">
      <h2 className="map-dock-title">{relationshipTitle(vertex)}</h2>
      {vertex.unresolved && <p>Unresolved reference. No current destination is known.</p>}
      {vertex.generated && <p className="muted">Generated document</p>}
      {!vertex.unresolved && (vertex.kind === "epic" || vertex.kind === "feature" ? <p>Contributors: <Contributors people={vertex.contributors ?? []} /></p> : vertex.owner && <p>Owner: <PersonName id={vertex.owner} /></p>)}
      <CopyCmd cmd={vertex.path || vertex.raw || vertex.id} />
      {vertex.kind === "external_path" && <p>External coordinate; not served by the repository file reader.</p>}
      {vertex.kind === "path" && <p>Recorded repository coordinate; existence and implementation coverage are not verified.</p>}
      {href !== null && <Link className="map-dock-open" to={href}>Open {vertex.destination?.kind === "file" ? "document" : "node"}</Link>}
      <SelectedRelationships selected={vertex.id} edges={edges} byID={byID} kind={kind} onSelect={onSelect} />
      {children}
    </div>
  </aside>;
}
