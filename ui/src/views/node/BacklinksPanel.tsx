import { Link } from "react-router-dom";
import type { BacklinksView, RelationshipVertex } from "../../api";
import CopyCmd from "../../components/CopyCmd";
import InstrSection from "../../components/InstrSection";
import SourceCitation from "../../components/relationships/SourceCitation";
import { relationshipHref } from "../../lib/relationshipHref";
import { relationshipTitle } from "../../lib/relationshipTitle";
import "./backlinks.css";

function VertexReference({ vertex }: { vertex: RelationshipVertex }) {
  const label = relationshipTitle(vertex);
  const href = relationshipHref(vertex);
  if (vertex.unresolved) return <span>{label} <span className="muted">(unresolved)</span></span>;
  if (vertex.kind === "path" || vertex.kind === "external_path") {
    return <CopyCmd cmd={vertex.path || vertex.raw || vertex.id} />;
  }
  return href === null ? <span>{label}</span> : <Link to={href}>{label}</Link>;
}


function SourceGroup({ kind, sources, generated }: {
  kind: string;
  sources: BacklinksView["groups"][number]["sources"];
  generated: boolean;
}) {
  const label = kind.replaceAll("_", " ");
  const heading = `${generated ? "Generated · " : ""}${label.charAt(0).toUpperCase()}${label.slice(1)}`;
  return (
    <section aria-label={heading} className="backlinks-group">
      <h3>{heading} <span className="muted">· {sources.length}</span></h3>
      <ul className="rule-list">
        {sources.map(({ vertex, owner_vertex, edges }) => (
          <li className="backlinks-source" key={vertex.id}>
            <div className="backlinks-source-head"><VertexReference vertex={vertex} /></div>
            {owner_vertex && (
              <div className="backlinks-owner muted">Owned by <VertexReference vertex={owner_vertex} /></div>
            )}
            {edges.map((edge) => (
              <div key={edge.id}>
                {edge.derived && <p className="muted">Derived relationship{edge.declared_scope && <> · declared scope <span className="mono">{edge.declared_scope}</span></>}</p>}
                <ul className="backlinks-citations">
                  {edge.sources.map((source) => <SourceCitation key={JSON.stringify(source)} source={source} />)}
                </ul>
              </div>
            ))}
          </li>
        ))}
      </ul>
    </section>
  );
}

export default function BacklinksPanel({ backlinks }: { backlinks: BacklinksView }) {
  if (backlinks.count === 0 && backlinks.complete && backlinks.diagnostics.length === 0) return null;
  return (
    <InstrSection label="Backlinks" caption={`${backlinks.complete ? "" : "at least "}${backlinks.count} ${backlinks.count === 1 ? "source" : "sources"}`}>
      {!backlinks.complete && (
        <p className="instr-warning" role="status">Backlinks are incomplete. Unavailable relationship inputs may hide other sources.</p>
      )}
      {backlinks.diagnostics.length > 0 && (
        <ul className="backlinks-diagnostics" aria-label="Relationship diagnostics">
          {backlinks.diagnostics.map((diagnostic) => (
            <li key={JSON.stringify(diagnostic)}>
              <span>{diagnostic.facet}: {diagnostic.message}</span>
              {diagnostic.source && <ul className="backlinks-citations"><SourceCitation
                source={diagnostic.source}
                allowSourceNavigation={diagnostic.code !== "unresolved_document" && diagnostic.code !== "document_unreadable"}
                allowCitationNavigation={diagnostic.code !== "unresolved_citation"}
              /></ul>}
            </li>
          ))}
        </ul>
      )}
      {backlinks.groups.map((group) => {
        const authored = group.sources.filter(({ vertex }) => group.kind !== "mentioned_by" || !vertex.generated);
        const generated = group.sources.filter(({ vertex }) => group.kind === "mentioned_by" && vertex.generated);
        return (
          <div key={group.kind}>
            {authored.length > 0 && <SourceGroup kind={group.kind} sources={authored} generated={false} />}
            {generated.length > 0 && <SourceGroup kind={group.kind} sources={generated} generated />}
          </div>
        );
      })}
    </InstrSection>
  );
}
