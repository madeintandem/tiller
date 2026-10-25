import { Link } from "react-router-dom";
import type { RelationshipSource } from "../../api";
import { relationshipFileHref } from "../../lib/relationshipHref";
import CopyCmd from "../CopyCmd";
import "./sourceCitation.css";

export default function SourceCitation({ source, allowSourceNavigation = true, allowCitationNavigation = true }: {
  source: RelationshipSource;
  allowSourceNavigation?: boolean;
  allowCitationNavigation?: boolean;
}) {
  const location = `${source.file}${source.line !== undefined ? `#L${source.line}` : ""}`;
  const href = allowSourceNavigation ? relationshipFileHref(source.file, source.line ?? 1) : null;
  const citationLocation = source.citation_path
    ? `${source.citation_path}${source.citation_line !== undefined ? `#L${source.citation_line}` : ""}`
    : null;
  const citationHref = source.citation_path && allowCitationNavigation
    ? relationshipFileHref(source.citation_path, source.citation_line ?? 1)
    : null;
  const details = [
    source.field,
    source.criterion && `criterion ${source.criterion}`,
    source.basis,
    source.binding_type && `binding ${source.binding_type}`,
    source.classification,
    source.operation !== undefined && `operation ${source.operation}`,
    source.role,
    source.state,
    source.scope,
  ].filter(Boolean);
  return (
    <li className="relationship-source-citation">
      {source.file && (href === null
        ? <CopyCmd cmd={location} />
        : <Link className="mono" to={href}>{location}</Link>)}
      {details.length > 0 && <span className="muted"> · {details.join(" · ")}</span>}
      {source.raw && <div className="mono muted">Recorded: {source.raw}</div>}
      {citationLocation !== null && (
        <div>
          Cites: {citationHref === null
            ? <CopyCmd cmd={citationLocation} />
            : <Link className="mono" to={citationHref}>{citationLocation}</Link>}
        </div>
      )}
      {source.revision && (
        <div className="muted">
          Pinned {source.revision.kind}: <span className="mono">{source.revision.value}</span>
          {source.revision.path && <> · <span className="mono">{source.revision.path}</span></>}
          {source.revision.observed_at && <> · <span className="mono">{source.revision.observed_at}</span></>}
          {(href !== null || citationHref !== null) && " · File link opens current contents, not the pinned revision."}
        </div>
      )}
      {source.evidence?.map((ref) => <div className="mono muted" key={ref}>Evidence: {ref}</div>)}
    </li>
  );
}
