// Vitest coverage for the header band's drift slot composition (task-11-
// brief.md, Step 2), written before composeDriftBand exists. The slot's own
// rule: the big number is candidates+uncovered together, but the visible
// text must distinguish them ("2 drifted, 1 unchecked") so a reader can never
// mistake a coverage gap for a candidate, and an all-zero report composes to
// null (absent), never a zero tile.

import { describe, expect, it } from "vitest";

import type { DriftReportView } from "../../api";
import { composeDriftBand } from "./driftBand";

function report(overrides: Partial<DriftReportView> = {}): DriftReportView {
  return { compared: 0, ...overrides };
}

describe("composeDriftBand", () => {
  it("sums candidates across every marker plus uncovered into one total", () => {
    const band = composeDriftBand(
      report({
        candidates: {
          active_basis: [{ marker: "active_basis", noun: "feature", id: "F-1" }],
          stale_spec: [{ marker: "stale_spec", noun: "feature", id: "F-2" }],
        },
        uncovered: [{ marker: "stale_spec", noun: "feature", id: "F-3", reason: "no_completion_record" }],
      }),
    );

    expect(band).not.toBeNull();
    expect(band?.total).toBe(3);
    // The visible text distinguishes the two counts; it must never collapse
    // them into one undifferentiated number a reader could misread as "3
    // things drifted" when only 2 actually did.
    expect(band?.text).toBe("2 drifted, 1 unchecked");
  });

  it("composes to null on an all-zero report, never a zero tile", () => {
    expect(composeDriftBand(report())).toBeNull();
    expect(composeDriftBand(report({ candidates: {}, uncovered: [] }))).toBeNull();
  });

  it("never drops either number to zero silently when only one side is zero", () => {
    // All candidates, no uncovered: still states "0 unchecked" rather than
    // omitting the clause, so a reader can never read partial coverage as
    // full coverage by the clause's mere absence.
    const allCandidates = composeDriftBand(
      report({ candidates: { active_basis: [{ marker: "active_basis", noun: "feature", id: "F-1" }] } }),
    );
    expect(allCandidates?.text).toBe("1 drifted, 0 unchecked");

    // All uncovered, no candidates: the inverse case, "0 drifted" stated
    // plainly rather than reading as a clean sweep.
    const allUncovered = composeDriftBand(
      report({ uncovered: [{ marker: "stale_spec", noun: "feature", id: "F-1", reason: "no_completion_record" }] }),
    );
    expect(allUncovered?.text).toBe("0 drifted, 1 unchecked");
  });
});
