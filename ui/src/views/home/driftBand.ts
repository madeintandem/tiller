// The header band's drift slot composition (ui-surface.md, the header band
// sentence: "the drift count, clickable through to the list of what
// drifted... it states both, never reading clean over nodes the sweep could
// not check"). Mirrors stat.ts's own shape: a pure derivation over the
// fetched payload, page-composed so the component stays a thin renderer.
//
// The slot's number is candidates+uncovered together (one glanceable total),
// but the two must never collapse into a single undifferentiated count
// anywhere a reader could mistake a coverage gap (uncovered) for an actual
// finding (a candidate): text always states both, "N drifted, M unchecked",
// even when one half is zero, so a reader can never read partial coverage as
// a clean sweep by a clause's mere absence.

import type { DriftReportView } from "../../api";

export interface DriftBand {
  total: number;
  text: string;
}

// composeDriftBand returns null on an all-zero report (no candidates, no
// uncovered): the band slot is absent then, not a zero tile, the same rule
// StatTile's dash-vs-zero distinction draws for a fetch that has not
// resolved, applied here to a report that resolved clean.
export function composeDriftBand(report: DriftReportView): DriftBand | null {
  const candidates = candidateCount(report);
  const uncovered = report.uncovered?.length ?? 0;
  const total = candidates + uncovered;
  if (total === 0) {
    return null;
  }
  return { total, text: `${candidates} drifted, ${uncovered} unchecked` };
}

function candidateCount(report: DriftReportView): number {
  if (report.candidates === undefined) {
    return 0;
  }
  let n = 0;
  for (const rows of Object.values(report.candidates)) {
    n += rows.length;
  }
  return n;
}
