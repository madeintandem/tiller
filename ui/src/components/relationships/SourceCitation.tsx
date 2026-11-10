import { Link } from "react-router-dom";
import type { RelationshipSource } from "../../api";
import { evidenceLabel } from "../../lib/evidenceLabel";
import { relationshipFileHref } from "../../lib/relationshipHref";
import {
  sourceBasisLabel,
  sourceBindingLabel,
  sourceClassificationLabel,
  sourceFieldLabel,
  sourceRevisionKindLabel,
  sourceRoleLabel,
  sourceScopeLabel,
  sourceStateLabel,
} from "../../lib/sourceWords";
import CopyCmd from "../CopyCmd";
import Time from "../Time";
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
  // Every recorded token reads as words (lib/sourceWords.ts); paths and ids
  // pass through as they are.
  const details = [
    source.field && sourceFieldLabel(source.field),
    source.criterion && `criterion ${source.criterion}`,
    source.basis && sourceBasisLabel(source.basis),
    source.binding_type && sourceBindingLabel(source.binding_type),
    source.classification && sourceClassificationLabel(source.classification),
    source.operation !== undefined && `operation ${source.operation}`,
    source.role && sourceRoleLabel(source.role),
    source.state && sourceStateLabel(source.state),
    source.scope && sourceScopeLabel(source.scope),
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
          Pinned {sourceRevisionKindLabel(source.revision.kind)}: <span className="mono">{source.revision.value}</span>
          {source.revision.path && <> · <span className="mono">{source.revision.path}</span></>}
          {source.revision.observed_at && <> · <span className="mono"><Time iso={source.revision.observed_at} /></span></>}
          {(href !== null || citationHref !== null) && " · File link opens current contents, not the pinned revision."}
        </div>
      )}
      {/* An evidence ref reads as a label like every other on the node
          page; the full ref rides only on the href and the title. */}
      {source.evidence?.map((ref) => (
        <div className="muted" key={ref}>
          <a
            className="instr-evid"
            href={`/api/evidence?ref=${encodeURIComponent(ref)}`}
            title={ref}
            target="_blank"
            rel="noreferrer"
          >
            {evidenceLabel({ ref, criterion: source.criterion })}
          </a>
        </div>
      ))}
    </li>
  );
}
