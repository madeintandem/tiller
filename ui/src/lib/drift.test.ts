import { describe, expect, it } from "vitest";

import type { UncoveredView } from "../api";
import { groupUncovered, markerCopy, MARKER_COPY, reasonCopy, REASON_COPY, REASON_COPY_BY_MARKER } from "./drift";
import vocabulary from "./vocabulary.json";

describe("drift copy", () => {
  it.each([...vocabulary.drift_markers, ...vocabulary.drift_extra_markers])("marker %s has a plain name, a meaning, and a next step", (marker) => {
    const copy = MARKER_COPY[marker];
    expect(copy).toBeDefined();
    for (const text of [copy.name, copy.meaning, copy.next]) {
      expect(text.trim()).not.toBe("");
      expect(text).not.toContain("_");
      expect(text).not.toContain("\u2014");
      expect(text.toLowerCase()).not.toContain("fork");
    }
  });

  it.each(vocabulary.drift_reasons)("reason %s has a cause and a fix", (reason) => {
    const copy = REASON_COPY[reason];
    expect(copy).toBeDefined();
    for (const text of [copy.cause, copy.fix]) {
      expect(text.trim()).not.toBe("");
      expect(text).not.toContain("_");
      expect(text).not.toContain("\u2014");
      expect(text.toLowerCase()).not.toContain("fork");
    }
  });

  it("keeps no copy for a token the vocabulary does not list", () => {
    expect(Object.keys(MARKER_COPY).sort()).toEqual(
      [...vocabulary.drift_markers, ...vocabulary.drift_extra_markers].sort(),
    );
    expect(Object.keys(REASON_COPY).sort()).toEqual([...vocabulary.drift_reasons].sort());
  });

  it("names the markers the brief fixes", () => {
    expect(MARKER_COPY.stale_spec.name).toBe("Spec changed after the work");
    expect(MARKER_COPY.active_basis.name).toBe("Work started from an older basis");
    expect(MARKER_COPY.answered_question.name).toBe("An answered question was not reviewed");
    expect(MARKER_COPY.missing_surface.name).toBe("Recorded file is gone");
    expect(MARKER_COPY.orphan_eval.name).toBe("Eval no longer matches its spec");
    expect(MARKER_COPY.stale_design.name).toBe("Design doc is behind the code");
    expect(MARKER_COPY.shifted_design.name).toBe("Design doc may have shifted");
    expect(REASON_COPY.ref_unreadable.cause).toBe(
      "The eval's ref could not be read as a file. A ref that names several files must be a YAML list, not one space-separated string.",
    );
    expect(REASON_COPY.ref_unreadable.fix).toBe("Rewrite ref as a list, then run the check again.");
  });

  it("humanizes a marker or reason from a newer server instead of printing it raw", () => {
    const marker = markerCopy("stale_widget");
    expect(marker.name).toBe("Stale widget");
    expect(marker.meaning).not.toContain("_");
    expect(marker.next).not.toContain("_");
    const reason = reasonCopy("stale_widget", "widget_jammed");
    expect(reason.cause).not.toContain("_");
    expect(reason.fix).not.toContain("_");
    expect(markerCopy("stale_spec")).toBe(MARKER_COPY.stale_spec);
    expect(reasonCopy("orphan_eval", "ref_unreadable")).toBe(REASON_COPY.ref_unreadable);
  });

  it("never shows the newer-version fallback for the design marker", () => {
    const copy = markerCopy("design");
    expect(copy).toBe(MARKER_COPY.design);
    expect(`${copy.name} ${copy.meaning} ${copy.next}`).not.toContain("newer version");
  });
});

// Every (marker, reason) pair the sweeps emit, from drift.go, design.go, and
// serve/prose.go. A reason's sentence must be true under each marker.
const EMITTED_PAIRS: Array<[string, string]> = [
  ["stale_spec", "no_completion_record"],
  ["stale_spec", "spec_unreadable"],
  ["stale_spec", "eval_unreadable"],
  ["stale_spec", "surface_unreadable"],
  ["active_basis", "basis_unreadable"],
  ["answered_question", "unknown_question"],
  ["missing_surface", "surface_unreadable"],
  ["orphan_eval", "eval_unreadable"],
  ["orphan_eval", "ref_unreadable"],
  ["stale_design", "unconfirmed"],
  ["stale_design", "surface_unreadable"],
  ["stale_design", "history_unavailable"],
  ["stale_design", "history_diverged"],
  ["shifted_design", "unconfirmed"],
  ["shifted_design", "surface_unrecorded"],
  ["shifted_design", "surface_unreadable"],
  ["shifted_design", "history_unavailable"],
  ["shifted_design", "history_diverged"],
  ["design", "comparison_error"],
];

describe("reason copy is keyed by marker and reason", () => {
  it.each(EMITTED_PAIRS)("%s / %s has its own true, plain copy", (marker, reason) => {
    const copy = reasonCopy(marker, reason);
    const fallback = reasonCopy("no_such_marker", "no_such_reason");
    expect(copy).not.toEqual(fallback);
    for (const text of [copy.cause, copy.fix]) {
      expect(text.trim()).not.toBe("");
      expect(text).not.toContain("_");
      expect(text).not.toContain("—");
      expect(text).not.toContain("newer version");
    }
  });

  it("covers every reason in the vocabulary by at least one emitted pair", () => {
    for (const reason of vocabulary.drift_reasons) {
      expect(EMITTED_PAIRS.some(([, r]) => r === reason)).toBe(true);
    }
  });

  it("lists overrides only for pairs that are emitted", () => {
    for (const [marker, reasons] of Object.entries(REASON_COPY_BY_MARKER)) {
      for (const reason of Object.keys(reasons)) {
        expect(EMITTED_PAIRS).toContainEqual([marker, reason]);
      }
    }
  });

  it("says a different, true thing for eval_unreadable under each marker", () => {
    const orphan = reasonCopy("orphan_eval", "eval_unreadable");
    const stale = reasonCopy("stale_spec", "eval_unreadable");
    expect(orphan.cause).toContain("valid contract");
    expect(stale.cause).toContain("recorded when the work finished");
    expect(stale.cause).not.toContain("criteria");
    expect(orphan).not.toEqual(stale);
  });

  it("says a different, true thing for history_diverged under each design marker", () => {
    const stale = reasonCopy("stale_design", "history_diverged");
    const shifted = reasonCopy("shifted_design", "history_diverged");
    expect(stale.cause).toContain("finished");
    expect(shifted.cause).toContain("current commit");
    expect(shifted.cause).not.toContain("finished");
  });

  it("says a different, true thing for surface_unreadable under each marker", () => {
    const causes = ["stale_spec", "missing_surface", "stale_design", "shifted_design"].map(
      (m) => reasonCopy(m, "surface_unreadable").cause,
    );
    expect(new Set(causes).size).toBe(3);
    expect(causes[2]).toBe(causes[3]);
    expect(causes[2]).toContain("document");
  });
});

describe("groupUncovered", () => {
  const row = (id: string, marker: string, reason: string): UncoveredView => ({
    marker,
    noun: "feature",
    id,
    reason,
  });

  it("groups rows by marker and reason together, in first-seen order", () => {
    const rows = [
      row("F-1", "orphan_eval", "ref_unreadable"),
      row("F-2", "stale_spec", "eval_unreadable"),
      row("F-3", "orphan_eval", "ref_unreadable"),
      row("F-4", "orphan_eval", "eval_unreadable"),
      row("F-5", "stale_spec", "eval_unreadable"),
    ];
    const groups = groupUncovered(rows);
    expect(groups.map((g) => [g.marker, g.reason, g.rows.map((r) => r.id)])).toEqual([
      ["orphan_eval", "ref_unreadable", ["F-1", "F-3"]],
      ["stale_spec", "eval_unreadable", ["F-2", "F-5"]],
      ["orphan_eval", "eval_unreadable", ["F-4"]],
    ]);
  });

  it("returns no groups for no rows", () => {
    expect(groupUncovered([])).toEqual([]);
  });
});
