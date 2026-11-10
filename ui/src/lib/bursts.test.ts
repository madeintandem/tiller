// Vitest coverage for the recap's burst collapsing (the recap-page pass). The
// numbers in the first describe are the live Acme tree's own: 167 events in
// two minutes by one owner, which the rail rendered as 167 rows carrying 167
// subjects and 501 repetitions of three constants.

import { describe, expect, it } from "vitest";

import type { RecapEntry } from "../api";
import { burstWhen, burstsOf, leafKey, tierBursts } from "./bursts";
import type { BookkeepingRun } from "./bursts";

function entry(overrides: Partial<RecapEntry> = {}): RecapEntry {
  return {
    at: "2026-07-25T21:37:02Z",
    type: "add.feature",
    contributor: "jc",
    command: "add feature x",
    ask: false,
    targets: [],
    ...overrides,
  };
}

describe("burstsOf", () => {
  it("collapses a run of one type into one row and keeps every entry under it", () => {
    const entries = Array.from({ length: 32 }, (_, i) =>
      entry({ type: "add.question", ask: true, at: `2026-07-25T21:37:0${i % 10}Z` }),
    );

    const bursts = burstsOf(entries);

    expect(bursts).toHaveLength(1);
    expect(bursts[0].entries).toHaveLength(32);
    expect(bursts[0].asks).toBe(32);
  });

  it("splits on type, so the four bursts of the live window read as four rows", () => {
    const entries = [
      ...Array.from({ length: 8 }, () => entry({ type: "link.feature" })),
      ...Array.from({ length: 8 }, () => entry({ type: "add.feature" })),
      entry({ type: "add.epic" }),
    ];

    expect(burstsOf(entries).map((b) => [b.type, b.entries.length])).toEqual([
      ["link.feature", 8],
      ["add.feature", 8],
      ["add.epic", 1],
    ]);
  });

  it("keeps a surface recording apart from a dependency edge, even back to back", () => {
    // `tiller link <feature> --depends-on X --surface Y` records
    // one link.feature event and one link.surface.feature event, adjacent in
    // the stream. They must render as two rows, not collapse into one just
    // because both came from the same command in the same call.
    const entries = [entry({ type: "link.surface.feature" }), entry({ type: "link.feature" })];

    expect(burstsOf(entries).map((b) => b.type)).toEqual(["link.surface.feature", "link.feature"]);
  });

  it("groups CONSECUTIVE runs only, so genuinely interleaved activity stays interleaved", () => {
    // The whole reason the API had to stop partitioning asks ahead of the
    // rest: a re-sorted list would collapse these three into one row and
    // claim a burst that never happened.
    const entries = [
      entry({ type: "done.task" }),
      entry({ type: "start.task" }),
      entry({ type: "done.task" }),
    ];

    expect(burstsOf(entries).map((b) => b.entries.length)).toEqual([1, 1, 1]);
  });

  it("breaks a run when the acting contributor changes, never merging two streams", () => {
    const entries = [
      entry({ contributor: "jc" }),
      entry({ contributor: "agent-1" }),
      entry({ contributor: "agent-1" }),
    ];

    expect(burstsOf(entries).map((b) => [b.contributor, b.entries.length])).toEqual([
      ["jc", 1],
      ["agent-1", 2],
    ]);
  });

  it("counts a mixed burst's asks without rounding either way", () => {
    const entries = [
      entry({ type: "spec.feature", ask: true }),
      entry({ type: "spec.feature", ask: false }),
      entry({ type: "spec.feature", ask: true }),
    ];

    const [burst] = burstsOf(entries);
    expect(burst.entries).toHaveLength(3);
    expect(burst.asks).toBe(2);
  });

  it("returns nothing for nothing", () => {
    expect(burstsOf([])).toEqual([]);
  });
});

describe("burstWhen", () => {
  const minutes = (iso: string) => iso.slice(11, 16);

  it("states ONE time when the run's ends render identically at the display precision", () => {
    // Measured on the live tree: add.feature spans 21:37:02 to 21:37:04, which
    // is one moment at the minute precision the view prints.
    const entries = [
      entry({ at: "2026-07-25T21:37:04Z" }),
      entry({ at: "2026-07-25T21:37:03Z" }),
      entry({ at: "2026-07-25T21:37:02Z" }),
    ];

    expect(burstWhen(entries, minutes)).toBe("21:37");
  });

  it("states the span when they do not, so a week in one lane can never read as an instant", () => {
    const entries = [
      entry({ at: "2026-07-25T16:40:00Z" }),
      entry({ at: "2026-07-20T09:12:00Z" }),
    ];

    expect(burstWhen(entries, minutes)).toBe("09:12 to 16:40");
  });

  it("reads the ends off a newest-first list, earliest named first", () => {
    const entries = [entry({ at: "2026-07-25T16:40:00Z" }), entry({ at: "2026-07-25T09:12:00Z" })];
    expect(burstWhen(entries, minutes)).toBe("09:12 to 16:40");
  });

  it("says nothing about nothing", () => {
    expect(burstWhen([], minutes)).toBe("");
  });
});

describe("leafKey", () => {
  it("drops the group's prefix, because the group head already carries it", () => {
    expect(leafKey("form-engine/audit-overlay-rendering", "form-engine")).toBe(
      "audit-overlay-rendering",
    );
  });

  it("collapses a key that IS the group's key, rather than repeating the head", () => {
    expect(leafKey("audit-engine", "audit-engine")).toBe("");
  });

  it("keeps a key that does not live under the group, never mangling it", () => {
    expect(leafKey("checkout/cart", "form-engine")).toBe("checkout/cart");
    // A prefix match that is not a path boundary is not a prefix.
    expect(leafKey("form-engine-2/thing", "form-engine")).toBe("form-engine-2/thing");
  });

  it("passes a key through whole in the project group, which has no key", () => {
    expect(leafKey("form-engine/thing", "")).toBe("form-engine/thing");
    expect(leafKey("", "")).toBe("");
  });
});

describe("tierBursts", () => {
  const bk = (type: string, n = 1) => burstsOf(Array.from({ length: n }, () => entry({ type })));

  it("folds adjacent bookkeeping bursts into one run, counting events and listing verbs once", () => {
    const bursts = [...bk("link.feature", 8), ...bk("order.features", 4), ...bk("rename.title.task", 2)];
    const out = tierBursts(bursts, false);
    expect(out).toHaveLength(1);
    const run = out[0] as BookkeepingRun;
    expect(run.kind).toBe("bookkeeping");
    expect(run.count).toBe(14);
    expect(run.bursts).toHaveLength(3);
    expect(run.verbs).toEqual(["linked", "reordered", "retitled"]);
  });

  it("dedupes verbs in first-seen order", () => {
    const e = (type: string, contributor: string) => entry({ type, contributor });
    const bursts = burstsOf([e("link.feature", "a"), e("order.tasks", "a"), e("link.task", "b")]);
    const run = tierBursts(bursts, false)[0] as BookkeepingRun;
    expect(run.verbs).toEqual(["linked", "reordered"]);
    expect(run.count).toBe(3);
  });

  it("a milestone between two bookkeeping bursts gives two runs", () => {
    const bursts = [...bk("link.feature"), ...bk("approve.feature"), ...bk("order.tasks")];
    const out = tierBursts(bursts, false);
    expect(out.map((o) => ("kind" in o ? "run" : "burst"))).toEqual(["run", "burst", "run"]);
  });

  it("a burst holding an ask stays a milestone burst and splits a run", () => {
    const asking = burstsOf([entry({ type: "link.feature", ask: true })]);
    const out = tierBursts([...bk("order.tasks"), ...asking, ...bk("order.bugs")], false);
    expect(out).toHaveLength(3);
    expect("kind" in out[1]).toBe(false);
  });

  it("returns the input unchanged when showAll is set", () => {
    const bursts = [...bk("link.feature"), ...bk("order.tasks")];
    expect(tierBursts(bursts, true)).toBe(bursts);
  });
});
