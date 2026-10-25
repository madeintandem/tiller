// Drift is the console's read of the exact sweep `tiller drift` renders
// (ui-surface.md, the header band sentence: "the same candidates-and-
// uncovered answer `tiller drift` gives, and it states both, never reading
// clean over nodes the sweep could not check"): GET /api/drift serves
// views.DriftReportOf unchanged, the identical projection the CLI's human
// and --json output both render, so the two surfaces cannot disagree about
// what a sweep found.
//
// This page is a report, not a new surface grammar (task-11-brief.md): rows
// reuse Explore's own list chrome (explore.css's rule-list and
// explore-result-* classes) rather than inventing a layout, and drift.css
// adds only what that flat, single-line row shape never needed, the
// multi-line detail/files/since/context stack a candidate row carries.

import type { CandidateView, DriftReportView, UncoveredView } from "../api";
import { driftPath } from "../api";
import EmptyNote from "../components/EmptyNote";
import { DriftCandidateDetails, DriftTargetLink } from "../components/DriftFinding";
import { titleOf, useDocumentTitle, useFetch } from "../hooks";
import { has } from "../lib/has";
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
const MARKER_ORDER = [
  "stale_spec",
  "active_basis",
  "answered_question",
  "missing_surface",
  "orphan_eval",
  "stale_design",
  "shifted_design",
];

export default function Drift() {
  useDocumentTitle(titleOf("Drift"));
  const { data, error, loading } = useFetch<DriftReportView>(driftPath);
  return (
    <section className="drift-page">
      <h1>Drift</h1>
      {error !== null && <p className="error">{error}</p>}
      {error === null && loading && <p className="muted">Sweeping the working tree...</p>}
      {data !== null && <DriftBody report={data} />}
    </section>
  );
}

function DriftBody({ report }: { report: DriftReportView }) {
  const sections = MARKER_ORDER.map((marker) => ({
    marker,
    rows: report.candidates?.[marker] ?? [],
  })).filter((s) => s.rows.length > 0);
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
  return (
    <section className="drift-section">
      <h2 className="eyebrow drift-section-head">
        {marker} ({rows.length})
      </h2>
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
    <section className="drift-section">
      <h2 className="eyebrow drift-section-head">Uncovered ({rows.length})</h2>
      <ul className="rule-list explore-results">
        {rows.map((u, i) => (
          <UncoveredRow key={`${u.marker}-${u.noun}-${u.id}-${i}`} row={u} />
        ))}
      </ul>
    </section>
  );
}

// UncoveredRow stays a single flex line (Explore's own explore-result-row):
// unlike a candidate, an uncovered row carries no files/since/context stack,
// so its content genuinely fits the flat row shape Explore's filter and
// search rows already use, with no drift-specific block layout needed.
function UncoveredRow({ row: u }: { row: UncoveredView }) {
  return (
    <li className="explore-result-row drift-row-uncovered">
      {/* Uncovered is one flat list across every marker (drift.Uncovered's
          own doc comment: a node can land here under more than one (marker,
          reason) pair), so each row has to name its own marker rather than
          relying on section context the way a candidate row can. */}
      <span className="mono muted drift-uncovered-marker">[{u.marker}]</span>
      <DriftTargetLink row={u} />
      <span className="drift-uncovered-reason">{u.reason}</span>
      {has(u.detail) && <span className="muted">{u.detail}</span>}
    </li>
  );
}

// checksRunCount mirrors internal/cli/drift.go's own writeDriftFooter: the
// known markers minus however many the manifest left disabled, so the
// footer's own check count agrees with the CLI's identical arithmetic.
function checksRunCount(report: DriftReportView): number {
  // Reports from an older binary have neither the design count nor markers.
  const total = report.compared_design === undefined ? 5 : MARKER_ORDER.length;
  return total - (report.disabled?.length ?? 0);
}

// comparedLine mirrors internal/cli/drift.go's own writeDriftFooter clean/
// found split verbatim: "nothing moved" only when the sweep found nothing,
// since that clause is the coverage assertion clean output makes, not a
// caption that belongs on a run that DID find something.
function comparedLine(report: DriftReportView, found: boolean): string {
  const suffix = found ? "" : "; nothing moved";
  const docs = report.compared_design === undefined ? "" : ` and ${report.compared_design} design doc(s)`;
  return `Compared ${report.compared} node(s)${docs} across ${checksRunCount(report)} check(s)${suffix}.`;
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

// COVERAGE_BUCKETS mirrors internal/cli/drift.go's own coverageBucketOrder:
// the fixed display order of the four bucket names every marker's Coverage
// map uses, paired with the human label each renders as.
const COVERAGE_BUCKETS: Array<{ key: string; label: string }> = [
  { key: "checked", label: "checked" },
  { key: "unresolvable", label: "unresolvable" },
  { key: "out_of_scope", label: "out of scope" },
  { key: "not_yet_in_scope", label: "not yet in scope" },
];

// Coverage keeps the engine's actual grain: design docs are not work nodes.
function coverageUnit(marker: string): string {
  if (marker === "stale_design" || marker === "shifted_design") return "doc(s)";
  return marker === "orphan_eval" ? "criteria" : "node(s)";
}

// coverageLine mirrors internal/cli/drift.go's own coverageLine: the unit
// names itself once, on the bucket that leads, so the CLI and the console
// cannot drift into disagreeing prose about the same counts. A bucket absent
// or zero is dropped rather than printed as a zero clause.
function coverageLine(marker: string, buckets: Record<string, number>): string {
  const unit = coverageUnit(marker);
  const parts: string[] = [];
  for (const { key, label } of COVERAGE_BUCKETS) {
    const n = buckets[key] ?? 0;
    if (n === 0) {
      continue;
    }
    parts.push(parts.length === 0 ? `${n} ${unit} ${label}` : `${n} ${label}`);
  }
  return parts.length === 0 ? `0 ${unit} checked` : parts.join(", ");
}

// DriftFooter always renders (compared, disabled, exempted, coverage): on a
// clean sweep, it is the WHOLE visible page, so its Compared line has to
// state the coverage assertion outright rather than leaving a reader to
// infer "nothing rendered" means "nothing to see" (task-11-brief.md: "Empty
// state states what was compared, mirroring the CLI's coverage assertion").
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
        <p className="muted">Disabled: {(report.disabled ?? []).join(", ")}</p>
      )}
      {exempted.length > 0 && (
        <p className="muted">Exempted: {exempted.map(([m, n]) => `${m}: ${n}`).join(", ")}</p>
      )}
      {coverage.length > 0 && (
        <div className="drift-coverage muted">
          <p>Coverage:</p>
          <ul>
            {coverage.map(([m, buckets]) => (
              <li key={m}>
                {m}: {coverageLine(m, buckets)}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
