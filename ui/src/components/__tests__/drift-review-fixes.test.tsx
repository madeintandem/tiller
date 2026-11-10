// Review fixes on the walk D-F drift pass: one uncovered line shared by the
// node panel and a design file's baseline panel, coverage words true for every
// count they sit over, marker names true for every node, and a report that
// scrolls to its hash once rather than on every reread.

import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { DriftReportView } from "../../api";
import type { FetchState } from "../../hooks";
import { MARKER_COPY, reasonCopy } from "../../lib/drift";
import { DriftReport, DriftReportBody } from "../../views/Drift";
import { UncoveredLine } from "../DriftFinding";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("UncoveredLine", () => {
  it("names what could not be checked and states an unconfirmed doc's cause once", () => {
    render(
      <UncoveredLine
        row={{ marker: "stale_design", noun: "design", id: "", key: "a.md", reason: "unconfirmed", detail: "no human confirmation of the described code is recorded" }}
      />,
    );
    expect(screen.getByText("Design doc not checked against the code.")).toBeInTheDocument();
    expect(screen.queryByText(/no human confirmation of the described code/)).not.toBeInTheDocument();
  });

  it("keeps a detail that names something specific to the row", () => {
    const { container } = render(
      <UncoveredLine row={{ marker: "design", noun: "design", id: "", key: "a.md", reason: "comparison_error", detail: "Unknown drift check" }} />,
    );
    expect(container.textContent).toContain("(Unknown drift check)");
  });
});

describe("Marker copy is true for every row it heads", () => {
  it("drops shifted_design's history_diverged detail, which only restates the cause", () => {
    expect(reasonCopy("shifted_design", "history_diverged").detailRestatesCause).toBe(true);
  });

  it("labels stale_spec's unchecked rows by the whole completion record, not the spec alone", () => {
    expect(MARKER_COPY.stale_spec.unchecked).toBe("Finished work not checked against its completion record");
  });

  it("names active_basis without claiming the work started", () => {
    expect(MARKER_COPY.active_basis.name).toBe("Work rests on an older basis");
  });
});

function renderReport(report: DriftReportView) {
  return render(
    <MemoryRouter>
      <DriftReportBody report={report} />
    </MemoryRouter>,
  );
}

describe("Coverage words", () => {
  it("says an eval reference could not be checked, and words criteria the sweep skips truly", () => {
    renderReport({ compared: 1, coverage: { orphan_eval: { checked: 2, unresolvable: 1, not_yet_in_scope: 1 } } });
    expect(
      screen.getByText(
        "Eval points at something missing: 2 criteria checked, 1 could not be checked, 1 is not checked at this stage of the work",
      ),
    ).toBeInTheDocument();
  });

  it("keeps could not be compared for a marker that compares", () => {
    renderReport({ compared: 1, coverage: { active_basis: { checked: 1, unresolvable: 2 } } });
    expect(screen.getByText("Work rests on an older basis: 1 node checked, 2 could not be compared")).toBeInTheDocument();
  });
});

describe("Drift report hash scroll", () => {
  const report: DriftReportView = {
    compared: 1,
    candidates: { missing_surface: [{ marker: "missing_surface", noun: "feature", id: "F-1", detail: "x" }] },
  };
  const state = (data: DriftReportView): FetchState<DriftReportView> => ({ data, error: null, loading: false });

  it("scrolls to the hash section once, not again on a reread with the same hash", () => {
    const scroll = vi.spyOn(Element.prototype, "scrollIntoView").mockImplementation(() => undefined);
    const { rerender } = render(
      <MemoryRouter initialEntries={["/?tab=drift#missing_surface"]}>
        <DriftReport state={state(report)} />
      </MemoryRouter>,
    );
    expect(scroll).toHaveBeenCalledTimes(1);
    rerender(
      <MemoryRouter initialEntries={["/?tab=drift#missing_surface"]}>
        <DriftReport state={state({ ...report })} />
      </MemoryRouter>,
    );
    expect(scroll).toHaveBeenCalledTimes(1);
  });
});
