// Vitest coverage for the header band's feature funnel and shelf-stat
// composition (task 8, settled decisions 8/9 of the home burst redesign),
// written before composeFunnel/composeShelfStat exist. The funnel reads
// Build's six columns as shape: composeFunnel always returns all six
// segments, zeros included, so a project with nothing in "active" still
// shows an active segment reading 0 rather than skipping it; anything
// outside those six (cancelled, archived) folds into one retired count
// instead of vanishing from the total. composeShelfStat gives the combined
// Bugs & Chores tile its open count the same way: done and cancelled are the
// only terminal states a shelf item carries, so open is everything else,
// with bugs and chores counted separately too since the tile's caption
// names both.

import { describe, expect, it } from "vitest";

import { BUILD_COLUMNS } from "../build/columns";
import { composeFunnel, composeShelfStat, FEATURE_FUNNEL } from "./funnel";

describe("FEATURE_FUNNEL", () => {
  it("is the six Build columns, in Build's own left-to-right order", () => {
    expect(FEATURE_FUNNEL).toEqual(["draft", "specced", "ready", "active", "in_review", "done"]);
  });

  it("stays the statuses Build's own columns render, never a second copy of them", () => {
    expect(FEATURE_FUNNEL).toEqual(BUILD_COLUMNS.map((c) => c.status));
  });
});

describe("composeFunnel", () => {
  it("buckets rows into the six segments, in FEATURE_FUNNEL order, with no retired", () => {
    const rows = [
      { status: "draft" },
      { status: "draft" },
      { status: "specced" },
      { status: "ready" },
      { status: "active" },
      { status: "active" },
      { status: "active" },
      { status: "in_review" },
      { status: "done" },
      { status: "done" },
    ];

    const { segments, retired } = composeFunnel(rows);

    expect(segments.map((s) => s.status)).toEqual(FEATURE_FUNNEL);
    expect(segments.map((s) => s.n)).toEqual([2, 1, 1, 3, 1, 2]);
    expect(retired).toBe(0);
  });

  it("keeps every segment present, zeros included, rather than dropping empty ones", () => {
    const { segments } = composeFunnel([{ status: "draft" }]);

    expect(segments).toEqual([
      { status: "draft", n: 1 },
      { status: "specced", n: 0 },
      { status: "ready", n: 0 },
      { status: "active", n: 0 },
      { status: "in_review", n: 0 },
      { status: "done", n: 0 },
    ]);
  });

  it("counts statuses outside the six funnel stages as retired rather than dropping them", () => {
    const rows = [
      { status: "draft" },
      { status: "cancelled" },
      { status: "archived" },
      { status: "cancelled" },
    ];

    const { segments, retired } = composeFunnel(rows);

    expect(retired).toBe(3);
    expect(segments.reduce((sum, s) => sum + s.n, 0)).toBe(1);
  });

  it("returns an all-zero funnel and no retired for no rows", () => {
    const { segments, retired } = composeFunnel([]);

    expect(segments.every((s) => s.n === 0)).toBe(true);
    expect(retired).toBe(0);
  });

  it("counts a row by its Build column when the server sent one", () => {
    const { segments } = composeFunnel([
      { status: "ready", column: "active" },
      { status: "ready", column: "ready" },
      { status: "draft" },
    ]);
    expect(segments.find((s) => s.status === "ready")?.n).toBe(1);
    expect(segments.find((s) => s.status === "active")?.n).toBe(1);
    expect(segments.find((s) => s.status === "draft")?.n).toBe(1);
  });
});

describe("composeShelfStat", () => {
  it("sums bugs and chores into one open count, excluding done and cancelled from each", () => {
    const bugs = [{ status: "draft" }, { status: "active" }, { status: "done" }];
    const chores = [
      { status: "ready" },
      { status: "cancelled" },
      { status: "active" },
      { status: "active" },
    ];

    const s = composeShelfStat(bugs, chores, [{ status: "ready" }, { status: "done" }, { status: "cancelled" }]);

    expect(s.bugs).toBe(2);
    expect(s.chores).toBe(3);
    expect(s.tweaks).toBe(1);
    expect(s.open).toBe(6);
  });

  it("treats every non-terminal status as open, not just a fixed allowlist", () => {
    const bugs = [{ status: "draft" }, { status: "in_review" }, { status: "blocked" }];

    const s = composeShelfStat(bugs, [], []);

    expect(s.bugs).toBe(3);
    expect(s.open).toBe(3);
  });

  it("returns all zero for two empty lists", () => {
    const s = composeShelfStat([], [], []);

    expect(s).toEqual({ open: 0, bugs: 0, chores: 0, tweaks: 0 });
  });
});
