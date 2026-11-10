import { describe, expect, it } from "vitest";

import type { UncoveredView } from "../api";
import { groupUncovered, markerCopy, MARKER_COPY, reasonCopy, REASON_COPY } from "./drift";
import vocabulary from "./vocabulary.json";

describe("drift copy", () => {
  it.each(vocabulary.drift_markers)("marker %s has a plain name, a meaning, and a next step", (marker) => {
    const copy = MARKER_COPY[marker];
    expect(copy).toBeDefined();
    for (const text of [copy.name, copy.meaning, copy.next]) {
      expect(text.trim()).not.toBe("");
      expect(text).not.toContain("_");
      expect(text).not.toContain("—");
      expect(text.toLowerCase()).not.toContain("fork");
    }
  });

  it.each(vocabulary.drift_reasons)("reason %s has a cause and a fix", (reason) => {
    const copy = REASON_COPY[reason];
    expect(copy).toBeDefined();
    for (const text of [copy.cause, copy.fix]) {
      expect(text.trim()).not.toBe("");
      expect(text).not.toContain("_");
      expect(text).not.toContain("—");
      expect(text.toLowerCase()).not.toContain("fork");
    }
  });

  it("keeps no copy for a token the vocabulary does not list", () => {
    expect(Object.keys(MARKER_COPY).sort()).toEqual([...vocabulary.drift_markers].sort());
    expect(Object.keys(REASON_COPY).sort()).toEqual([...vocabulary.drift_reasons].sort());
  });

  it("names the markers the brief fixes", () => {
    expect(MARKER_COPY.stale_spec.name).toBe("Spec changed after the work");
    expect(MARKER_COPY.active_basis.name).toBe("Work started from an older basis");
    expect(MARKER_COPY.answered_question.name).toBe("An answered question was not reviewed");
    expect(MARKER_COPY.missing_surface.name).toBe("No surface recorded");
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
    const reason = reasonCopy("widget_jammed");
    expect(reason.cause).not.toContain("_");
    expect(reason.fix).not.toContain("_");
    expect(markerCopy("stale_spec")).toBe(MARKER_COPY.stale_spec);
    expect(reasonCopy("ref_unreadable")).toBe(REASON_COPY.ref_unreadable);
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
