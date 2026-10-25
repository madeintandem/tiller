// Vitest coverage for the header band's stat composition (task C1), written
// before composeStat exists. composeStat turns a flat list of
// wire statuses into StateBar's seven-state mix plus a caption in the
// literal wire words, both ordered by the legend rank the status FOLDS to
// (lib/states.ts's foldStatus), so a question's "open" (-> in_review) sorts
// before "answered" (-> done) the same way the mock's "10 open . 2 answered"
// does, even though "open"/"answered" are not chart states themselves.

import { describe, expect, it } from "vitest";

import { composeStat } from "./stat";

describe("composeStat", () => {
  it("buckets by folded legend rank and captions in the literal wire words", () => {
    const rows = [
      ...Array<{ status: string }>(63).fill({ status: "draft" }),
      ...Array<{ status: string }>(6).fill({ status: "specced" }),
      { status: "ready" },
    ];

    const { mix, caption } = composeStat(rows);

    expect(caption).toBe("63 draft · 6 specced · 1 ready");
    // Every one of the seven legend buckets is present (StateBar itself
    // drops the zero entries), in legend order, draft first.
    expect(mix.map((m) => m.state)).toEqual([
      "draft",
      "specced",
      "ready",
      "active",
      "in_review",
      "done",
      "blocked",
    ]);
    expect(mix.find((m) => m.state === "draft")?.n).toBe(63);
    expect(mix.find((m) => m.state === "specced")?.n).toBe(6);
    expect(mix.find((m) => m.state === "ready")?.n).toBe(1);
    expect(mix.find((m) => m.state === "active")?.n).toBe(0);
  });

  it("orders a question's open/answered by their folded bucket rank, not alphabetically", () => {
    const rows = [
      ...Array<{ status: string }>(10).fill({ status: "open" }),
      ...Array<{ status: string }>(2).fill({ status: "answered" }),
    ];

    const { mix, caption } = composeStat(rows);

    // "answered" is alphabetically first but folds to done (rank 5), after
    // open's in_review (rank 4): the mock's own order, "open" before
    // "answered".
    expect(caption).toBe("10 open · 2 answered");
    expect(mix.find((m) => m.state === "in_review")?.n).toBe(10);
    expect(mix.find((m) => m.state === "done")?.n).toBe(2);
  });

  it("returns an empty caption and an all-zero mix for no rows", () => {
    const { mix, caption } = composeStat([]);
    expect(caption).toBe("");
    expect(mix.every((m) => m.n === 0)).toBe(true);
  });
});
