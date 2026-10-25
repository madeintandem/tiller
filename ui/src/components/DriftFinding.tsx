import { Link } from "react-router-dom";
import type { CandidateView, UncoveredView } from "../api";
import { refOf, shortSha } from "../lib/drift";
import { has } from "../lib/has";
import CopyCmd from "./CopyCmd";
import RefLink from "./RefLink";

// Ledger paths are literal filenames, not citations: encode before appending
// the locator so a '#' or '%2F' in a name cannot become route syntax.
export function driftFileHref(path: string): string {
  return `/file/${path.split("/").map(encodeURIComponent).join("/")}#L1`;
}

export function DriftTargetLink({ row }: { row: CandidateView | UncoveredView }) {
  if (row.noun === "design") {
    return has(row.key) ? (
      <Link to={driftFileHref(`.tiller/design/${row.key}`)} className="explore-result-title mono">
        design/{row.key}
      </Link>
    ) : <span className="mono">design</span>;
  }
  return <RefLink node={{ noun: row.noun, id: row.id, ref: refOf(row.key, row.id) }} className="explore-result-title mono" />;
}

// Shared by the project report and registered design-file disclosure. The
// server supplies findings and commands; the UI never recomputes drift.
export function DriftCandidateDetails({ candidate: c }: { candidate: CandidateView }) {
  return (
    <>
      {has(c.detail) && <p className="drift-row-detail">{c.detail}</p>}
      {(c.files?.length ?? 0) > 0 && (
        <p className="muted drift-row-line">
          <span className="drift-row-label">files</span>{" "}
          <span className="mono">{c.files?.join(", ")}</span>
        </p>
      )}
      {c.since !== undefined && (
        <p className="muted drift-row-line">
          <span className="drift-row-label">since</span>{" "}
          <span className="mono" title={c.since.value}>{shortSha(c.since)}</span>
          {c.until !== undefined && <> through <span className="mono" title={c.until.value}>{shortSha(c.until)}</span></>}
        </p>
      )}
      {(c.features?.length ?? 0) > 0 && (
        <ul className="drift-row-context">
          {c.features?.map((feature) => (
            <li key={feature.id}>
              <RefLink node={{ noun: "feature", id: feature.id, ref: refOf(feature.key, feature.id) }} />
              {" at "}<span className="mono" title={feature.commit.value}>{shortSha(feature.commit)}</span>
              {" · "}<Link to={driftFileHref(feature.spec)}>feature spec</Link>
            </li>
          ))}
        </ul>
      )}
      {(c.context?.length ?? 0) > 0 && (
        <ul className="drift-row-context muted">
          {c.context?.map((line, i) => <li key={i}>{line}</li>)}
        </ul>
      )}
      {has(c.remedy) && (
        <div className="drift-row-line">
          {c.noun === "design" && <p>After human review against current code, reconcile the document and confirm it:</p>}
          <CopyCmd cmd={c.remedy} />
        </div>
      )}
    </>
  );
}
