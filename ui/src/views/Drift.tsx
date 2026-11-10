// Drift is the console's read of the exact sweep `tiller drift` renders
// (ui-surface.md, the header band sentence: "the same candidates-and-
// uncovered answer `tiller drift` gives, and it states both, never reading
// clean over nodes the sweep could not check"): GET /api/drift serves
// views.DriftReportOf unchanged, the identical projection the CLI's human
// and --json output both render, so the two surfaces cannot disagree about
// what a sweep found.
//
// This report is not a new surface grammar: rows
// reuse Explore's own list chrome (explore.css's rule-list and
// explore-result-* classes) rather than inventing a layout, and drift.css
// adds only what that flat, single-line row shape never needed, the
// multi-line detail/files/since/context stack a candidate row carries.

import { useEffect } from "react";
import { useLocation } from "react-router-dom";
import type { CandidateView, DriftReportView, UncoveredView } from "../api";
import EmptyNote from "../components/EmptyNote";
import { DriftCandidateDetails, DriftTargetLink } from "../components/DriftFinding";
import type { FetchState } from "../hooks";
import { groupUncovered, markerCopy, reasonCopy, uncoveredDetail } from "../lib/drift";
import vocabulary from "../lib/vocabulary.json";
import "./explore.css";
import "./drift.css";

// MARKER_ORDER mirrors drift.KnownMarkers (internal/core/drift/drift.go), the
// schema's own fixed order (artifact-schema.md, "Drift control hooks"): a
// section that has candidates renders in this order rather than the
// incidental order Object.entries would give a map that arrived over JSON
// (Go's encoding/json sorts map keys ALPHABETICALLY, which is a different
// order: stale_spec would render last, not first), matching the CLI's own
// section order exactly.
//
// Keep this order aligned with drift.KnownMarkers when adding a marker.
//
// The order is vocabulary.json's drift_markers, which a Go test pins against
// drift.KnownMarkers, so the two cannot drift apart. A marker from a newer
// server that this list lacks still renders, after the known ones.
const MARKER_ORDER: string[] = vocabulary.drift_markers;

// UNCOVERED_SECTION_ID is the Uncovered section's anchor. Every marker section
// is anchored by its marker token, so /?tab=drift#missing_surface (the node
// drift panel's link, and the old /drift#stale_spec redirect) lands on it.
export const UNCOVERED_SECTION_ID = "uncovered";

// DriftReport is the report as a Home tab (R4): Home already fetches
// /api/drift for its Drift stat tile, so the tab renders from that same state
// rather than asking again, and the page's own heading is the tab's name.
export function DriftReport({ state }: { state: FetchState<DriftReportView> }) {
  const { data, error, loading } = state;
  const { hash } = useLocation();
  // The sections exist only once the report arrives, after the browser's own
  // hash scroll has already run, so a hash naming a section scrolls to it
  // here.
  useEffect(() => {
    if (data === null || hash.length < 2) return;
    document.getElementById(decodeURIComponent(hash.slice(1)))?.scrollIntoView?.({ block: "start" });
  }, [data, hash]);
  return (
    <section className="drift-page">
      {error !== null && <p className="error">{error}</p>}
      {error === null && loading && <p className="muted">Sweeping the working tree...</p>}
      {data !== null && <DriftReportBody report={data} />}
    </section>
  );
}

// DriftReportBody is the report without its page heading, so a host that
// supplies its own heading can render the same body.
export function DriftReportBody({ report }: { report: DriftReportView }) {
  const present = Object.keys(report.candidates ?? {});
  const markers = [...MARKER_ORDER, ...present.filter((m) => !MARKER_ORDER.includes(m)).sort()];
  const sections = markers
    .map((marker) => ({ marker, rows: report.candidates?.[marker] ?? [] }))
    .filter((s) => s.rows.length > 0);
  const uncovered = report.uncovered ?? [];
  const found = sections.length > 0 || uncovered.length > 0;

  return (
    <>
      {sections.map((s) => (
        <MarkerSection key={s.marker} marker={s.marker} rows={s.rows} />
      ))}
      {uncovered.length > 0 && <UncoveredSection rows={uncovered} />}
      <DriftFooter report={report} found={found} />
    </>
  );
}

function MarkerSection({ marker, rows }: { marker: string; rows: CandidateView[] }) {
  const copy = markerCopy(marker);
  return (
    <section className="drift-section" id={marker}>
      <h2 className="eyebrow drift-section-head">
        {copy.name} ({rows.length})
      </h2>
      <p className="muted drift-section-meaning">{copy.meaning}</p>
      <ul className="rule-list drift-rows">
        {rows.map((c, i) => (
          <CandidateRow key={`${c.noun}-${c.id}-${i}`} candidate={c} />
        ))}
      </ul>
    </section>
  );
}

function CandidateRow({ candidate: c }: { candidate: CandidateView }) {
  return (
    <li className="drift-row">
      <DriftTargetLink row={c} />
      <DriftCandidateDetails candidate={c} />
    </li>
  );
}

function UncoveredSection({ rows }: { rows: UncoveredView[] }) {
  return (
    <section className="drift-section" id={UNCOVERED_SECTION_ID}>
      <h2 className="eyebrow drift-section-head">Uncovered ({rows.length})</h2>
      <p className="muted drift-section-meaning">
        These could not be checked, so this report says nothing about whether they drifted.
      </p>
      {groupUncovered(rows).map((group) => (
        <UncoveredGroup key={`${group.marker}/${group.reason}`} group={group} />
      ))}
    </section>
  );
}

// UncoveredGroup states a cause shared by several rows once. Uncovered is one
// flat list across every marker (drift.Uncovered's own doc comment: a node can
// land here under more than one (marker, reason) pair), so a group names what
// could not be checked (the marker's unchecked label, never its finding name,
// since no finding was established), then the cause and the fix, then one row
// per node.
function UncoveredGroup({ group }: { group: { marker: string; reason: string; rows: UncoveredView[] } }) {
  const marker = markerCopy(group.marker);
  const reason = reasonCopy(group.marker, group.reason);
  return (
    <div className="drift-group">
      <h3 className="drift-group-head">
        {marker.unchecked} ({group.rows.length})
      </h3>
      <p className="drift-group-cause">{reason.cause}</p>
      <p className="muted drift-group-fix">{reason.fix}</p>
      <ul className="rule-list explore-results">
        {group.rows.map((u, i) => (
          <UncoveredRow key={`${u.noun}-${u.id}-${u.key ?? ""}-${i}`} row={u} />
        ))}
      </ul>
    </div>
  );
}

// UncoveredRow stays a single flex line (Explore's own explore-result-row):
// unlike a candidate, an uncovered row carries no files/since/context stack,
// so its content genuinely fits the flat row shape Explore's filter and
// search rows already use, with no drift-specific block layout needed. Its
// detail shows only when it says something its group's cause does not.
function UncoveredRow({ row: u }: { row: UncoveredView }) {
  const detail = uncoveredDetail(u);
  return (
    <li className="explore-result-row drift-row-uncovered">
      <DriftTargetLink row={u} />
      {detail !== "" && <span className="muted">{detail}</span>}
    </li>
  );
}

// count writes a number with the singular or plural form of its noun.
function count(n: number, singular: string, plural: string): string {
  return `${n} ${n === 1 ? singular : plural}`;
}

// checksRunCount follows internal/cli/drift.go's own writeDriftFooter
// arithmetic: the known markers minus however many the manifest left
// disabled.
function checksRunCount(report: DriftReportView): number {
  // Reports from an older binary have neither the design count nor markers.
  const total = report.compared_design === undefined ? 5 : MARKER_ORDER.length;
  return total - (report.disabled?.length ?? 0);
}

// uncomparedDesignDocs counts the distinct design documents named under
// Uncovered. compared_design counts only documents every check could finish
// (design.go's comparedDesign), so these are the ones the compared count
// leaves out, and the footer says so beside it.
function uncomparedDesignDocs(report: DriftReportView): number {
  const keys = new Set((report.uncovered ?? []).filter((u) => u.noun === "design").map((u) => u.key ?? ""));
  return keys.size;
}

// comparedLine keeps the CLI's clean/found split: "nothing moved" only when
// the sweep found nothing, since that clause is the coverage assertion clean
// output makes, not a caption that belongs on a run that DID find something.
function comparedLine(report: DriftReportView, found: boolean): string {
  const suffix = found ? "" : "; nothing moved";
  const docs =
    report.compared_design === undefined ? "" : ` and ${count(report.compared_design, "design doc", "design docs")}`;
  const checks = count(checksRunCount(report), "check", "checks");
  const line = `Compared ${count(report.compared, "node", "nodes")}${docs} across ${checks}${suffix}.`;
  const uncompared = uncomparedDesignDocs(report);
  return uncompared === 0
    ? line
    : `${line} ${count(uncompared, "design doc", "design docs")} could not be fully compared (see Uncovered).`;
}

function exemptedEntries(report: DriftReportView): Array<[string, number]> {
  const exempted = report.exempted ?? {};
  return MARKER_ORDER.filter((m) => (exempted[m] ?? 0) > 0).map((m) => [m, exempted[m]] as [string, number]);
}

function coverageEntries(report: DriftReportView): Array<[string, Record<string, number>]> {
  const coverage = report.coverage ?? {};
  return MARKER_ORDER.filter((m) => coverage[m] !== undefined).map(
    (m) => [m, coverage[m]] as [string, Record<string, number>],
  );
}

// COVERAGE_BUCKETS is internal/cli/drift.go's coverageBucketOrder, each bucket
// worded for what drift.go counts in it, with its verb agreeing with the
// count:
//   checked           the comparison ran
//   unresolvable      a path, file, or history could not be read, or a test
//                     ref is not a path, so nothing was compared
//   out_of_scope      an orphan_eval criterion bound to a command, a manual
//                     check, or a rubric, which names no file to look for
//   not_yet_in_scope  an orphan_eval criterion whose polarity is checked
//                     only once the work is approved (preserve) or finished
//                     (introduce)
const COVERAGE_BUCKETS: Array<{ key: string; label: (n: number) => string }> = [
  { key: "checked", label: () => "checked" },
  { key: "unresolvable", label: () => "could not be compared" },
  { key: "out_of_scope", label: (n) => (n === 1 ? "has no file to check" : "have no file to check") },
  {
    key: "not_yet_in_scope",
    label: (n) => `${n === 1 ? "is" : "are"} checked only once the work reaches a later stage`,
  },
];

// Coverage keeps the engine's actual grain: design docs are not work nodes,
// and orphan_eval counts criteria (drift.Report's Coverage doc comment).
function coverageUnit(marker: string, n: number): string {
  if (marker === "stale_design" || marker === "shifted_design") return n === 1 ? "design doc" : "design docs";
  if (marker === "orphan_eval") return n === 1 ? "criterion" : "criteria";
  return n === 1 ? "node" : "nodes";
}

// coverageLine names the unit once, on the bucket that leads. A bucket absent
// or zero is dropped rather than printed as a zero clause.
function coverageLine(marker: string, buckets: Record<string, number>): string {
  const parts: string[] = [];
  for (const { key, label } of COVERAGE_BUCKETS) {
    const n = buckets[key] ?? 0;
    if (n === 0) {
      continue;
    }
    parts.push(parts.length === 0 ? `${n} ${coverageUnit(marker, n)} ${label(n)}` : `${n} ${label(n)}`);
  }
  return parts.length === 0 ? `0 ${coverageUnit(marker, 0)} checked` : parts.join(", ");
}

// DriftFooter always renders (compared, disabled, exempted, coverage): on a
// clean sweep, it is the WHOLE visible page, so its Compared line has to
// state the coverage assertion outright rather than leaving a reader to
// infer "nothing rendered" means "nothing to see".
function DriftFooter({ report, found }: { report: DriftReportView; found: boolean }) {
  const exempted = exemptedEntries(report);
  const coverage = coverageEntries(report);
  return (
    <div className="drift-footer">
      {found ? (
        <p className="muted">{comparedLine(report, found)}</p>
      ) : (
        <EmptyNote>{comparedLine(report, found)}</EmptyNote>
      )}
      {(report.disabled?.length ?? 0) > 0 && (
        <p className="muted">Disabled: {(report.disabled ?? []).map((m) => markerCopy(m).name).join(", ")}</p>
      )}
      {exempted.length > 0 && (
        <p className="muted">Exempted: {exempted.map(([m, n]) => `${markerCopy(m).name}: ${n}`).join(", ")}</p>
      )}
      {coverage.length > 0 && (
        <div className="drift-coverage muted">
          <p>Coverage:</p>
          <ul>
            {coverage.map(([m, buckets]) => (
              <li key={m}>
                {markerCopy(m).name}: {coverageLine(m, buckets)}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
