// Vitest coverage for the board's own ordering guarantee:
// visibleCards flattens the selection in served order, columnCards is a
// pure status filter over it, and BUILD_COLUMNS names the fixed six.

import { describe, expect, it } from "vitest";

import type { BuildEpic, BuildFeature } from "../../api";
import { BUILD_COLUMNS, columnCards, isColumnStatus, renderedFeatureCount, visibleCards } from "./columns";

function feature(id: string, status: string): BuildFeature {
  return {
    id,
    status,
    column: status,
    ref: `${id} (${id})`,
    open_work_items: 0,
    task_done: 0,
    task_total: 0,
    tasks: [],
    contributors: [],
  };
}

function epic(id: string, key: string, features: BuildFeature[]): BuildEpic {
  return { noun: "epic", id, key, ref: `${key} (${id})`, status: "active", contributors: [], features };
}

describe("BUILD_COLUMNS", () => {
  it("names the six stored feature states, in the board's own fixed order", () => {
    expect(BUILD_COLUMNS.map((c) => c.status)).toEqual([
      "draft",
      "specced",
      "ready",
      "active",
      "in_review",
      "done",
    ]);
  });
});

describe("visibleCards", () => {
  it("flattens epics-then-features in the exact order handed to it, never re-sorted", () => {
    const epics = [
      epic("E-2", "z-epic", [feature("F-3", "draft"), feature("F-4", "done")]),
      epic("E-1", "a-epic", [feature("F-1", "draft")]),
    ];

    const cards = visibleCards(epics);

    // z-epic leads here even though "a-epic" would sort first alphabetically
    // - proof this is a flatten, not a sort, of whatever order it is handed.
    expect(cards.map((c) => c.feature.id)).toEqual(["F-3", "F-4", "F-1"]);
    expect(cards.map((c) => c.epic.id)).toEqual(["E-2", "E-2", "E-1"]);
  });

  it("returns no cards for no epics", () => {
    expect(visibleCards([])).toEqual([]);
  });
});

describe("columnCards", () => {
  it("filters by status, preserving the served sequence exactly", () => {
    const epics = [epic("E-1", "intake", [feature("F-3", "draft"), feature("F-1", "done"), feature("F-2", "draft")])];
    const cards = visibleCards(epics);

    expect(columnCards(cards, "draft").map((c) => c.feature.id)).toEqual(["F-3", "F-2"]);
    expect(columnCards(cards, "done").map((c) => c.feature.id)).toEqual(["F-1"]);
    expect(columnCards(cards, "active")).toEqual([]);
  });

  it("places a card by its served column, not its stored status", () => {
    const inFlight = { ...feature("F-4", "ready"), column: "active" };
    const cards = visibleCards([epic("E-1", "intake", [feature("F-5", "ready"), inFlight])]);

    expect(columnCards(cards, "ready").map((c) => c.feature.id)).toEqual(["F-5"]);
    expect(columnCards(cards, "active").map((c) => c.feature.id)).toEqual(["F-4"]);
  });
});

// Review finding 1: a cancelled feature is real, server-served data
// (BuildEpic.Features and feature_counts both include it - the server does
// not filter it out) but has no column of the fixed six to land in.
// isColumnStatus/renderedFeatureCount are the counting side that has to
// agree with that fact rather than with epic.features.length.
describe("isColumnStatus", () => {
  it("is true for exactly the six BUILD_COLUMNS statuses", () => {
    for (const col of BUILD_COLUMNS) {
      expect(isColumnStatus(col.status)).toBe(true);
    }
  });

  it("is false for cancelled, and for an unrecognized status", () => {
    expect(isColumnStatus("cancelled")).toBe(false);
    expect(isColumnStatus("some_future_status")).toBe(false);
  });
});

describe("renderedFeatureCount", () => {
  it("counts only the features that land in one of the six columns", () => {
    const features = [feature("F-1", "draft"), feature("F-2", "cancelled"), feature("F-3", "done")];
    expect(renderedFeatureCount(features)).toBe(2);
  });

  it("agrees with the sum of every column's own columnCards for the same features", () => {
    const epics = [
      epic("E-1", "intake", [feature("F-1", "draft"), feature("F-2", "cancelled"), feature("F-3", "done")]),
    ];
    const cards = visibleCards(epics);
    const columnTotal = BUILD_COLUMNS.reduce((n, col) => n + columnCards(cards, col.status).length, 0);
    expect(renderedFeatureCount(epics[0].features)).toBe(columnTotal);
  });

  it("returns 0 for no features, and counts all of them when none are cancelled", () => {
    expect(renderedFeatureCount([])).toBe(0);
    expect(renderedFeatureCount([feature("F-1", "draft"), feature("F-2", "active")])).toBe(2);
  });
});
