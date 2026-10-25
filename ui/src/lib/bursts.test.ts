// Vitest coverage for the recap's burst collapsing (the recap-page pass). The
// numbers in the first describe are the live Acme tree's own: 167 events in
// two minutes by one owner, which the rail rendered as 167 rows carrying 167
// subjects and 501 repetitions of three constants.

import { describe, expect, it } from "vitest";

import type { RecapEntry } from "../api";
import { burstPhrase, burstWhen, burstsOf, leafKey } from "./bursts";

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
    // Fix round 2: `tiller link <feature> --depends-on X --surface Y` records
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

describe("burstPhrase", () => {
  it.each([
    ["owner.add", "owners added", "added"],
    ["owner.remove", "owners removed", "removed"],
    ["contributor.add", "contributors added", "added"],
    ["contributor.remove", "contributors removed", "removed"],
  ])("renders roster event %s without changing historical vocabulary", (type, subject, verb) => {
    expect(burstPhrase(type, 2)).toEqual({ subject, verb });
  });

  it.each([
    ["add.feature", 8, "features added", "added"],
    ["add.feature", 1, "feature added", "added"],
    ["link.feature", 6, "features linked", "linked"],
    ["add.epic", 1, "epic added", "added"],
    ["spec.feature", 2, "features specced", "specced"],
    ["submit.feature", 1, "feature submitted", "submitted"],
    ["cancel.task", 3, "tasks cancelled", "cancelled"],
    ["withdraw.feature", 1, "feature withdrawn", "withdrawn"],
    // attest and eval reach the stream through mutate's saveEvalTarget rather
    // than the lifecycle table, the third minting path, and were missed on the
    // first pass: a live tree rendered "4 features attest" and "Feature One
    // eval". They are the next two verbs a tree mints after its first approval,
    // so the fallback below is the wrong home for them.
    ["attest.feature", 4, "features attested", "attested"],
    ["eval.feature", 1, "feature evaluated", "evaluated"],
    // An amendment's approve mints revise.<noun> and correct-eval.feature
    // directly (internal/core/mutate/amendment_approve.go), outside the three
    // paths above, so both were rendering their raw verb.
    ["revise.feature", 2, "features revised", "revised"],
    ["correct-eval.feature", 1, "feature eval corrected", "eval corrected"],
  ])("composes %s at n=%i as %s", (type, count, subject, verb) => {
    expect(burstPhrase(type, count)).toEqual({ subject, verb });
  });

  it("has a participle for every verb the event stream can mint", () => {
    // The map is only worth having if it is exhaustive, so this is the guard
    // that fails when a new verb lands: the full set the stream can carry after
    // the recap's claim/release churn filter (internal/views/recap.go), read off
    // internal/core/mutate's three minting paths and the lifecycle table.
    //
    // accept is deliberately absent: it left the lifecycle table when done above
    // a task became a rollup sync computes, so nothing can mint it now. The
    // participle for it survives for history's sake, pinned on its own below.
    const VERBS = [
      "add", "answer", "approve", "archive", "attest", "cancel",
      "check", "eval", "link", "resume", "return", "spec", "start",
      "submit", "sync", "withdraw", "revise", "correct-eval",
    ];
    const raw = VERBS.filter((v) => burstPhrase(`${v}.feature`, 1).verb === v);
    expect(raw).toEqual([]);
  });

  it("still reads a historical accept row as English, though nothing mints one now", () => {
    // The recap reads HISTORY. A tree that predates the rollup change carries
    // accept.epic rows, and dropping the participle with the verb would degrade
    // them to "epic accept" on a page a human is meant to scan.
    expect(burstPhrase("accept.epic", 1)).toEqual({ subject: "epic accepted", verb: "accepted" });
  });

  it("says asked, not added, for the one type the recap's whole story hangs on", () => {
    expect(burstPhrase("add.question", 32)).toEqual({ subject: "questions asked", verb: "asked" });
    expect(burstPhrase("add.question", 1)).toEqual({ subject: "question asked", verb: "asked" });
  });

  it("says surfaces recorded for link.surface.<grain>, not depends-on's linked", () => {
    // Fix round 2: SetSurface mints link.surface.epic / link.surface.feature
    // (internal/core/mutate/surface.go) rather than sharing --depends-on's
    // link.epic / link.feature. Both grains report "surface", not the grain
    // word, since a reader scans for the fact that a surface got recorded;
    // the grain still keeps its own burst (the previous test) without
    // needing its own word here.
    expect(burstPhrase("link.surface.feature", 3)).toEqual({ subject: "surfaces recorded", verb: "recorded" });
    expect(burstPhrase("link.surface.feature", 1)).toEqual({ subject: "surface recorded", verb: "recorded" });
    expect(burstPhrase("link.surface.epic", 1)).toEqual({ subject: "surface recorded", verb: "recorded" });
  });

  it("says surfaces removed for unlink.surface.<grain>, and unlinked for unlink", () => {
    expect(burstPhrase("unlink.surface.feature", 2)).toEqual({ subject: "surfaces removed", verb: "removed" });
    expect(burstPhrase("unlink.surface.epic", 1)).toEqual({ subject: "surface removed", verb: "removed" });
    expect(burstPhrase("unlink.feature", 1)).toEqual({ subject: "feature unlinked", verb: "unlinked" });
  });

  it("takes the irregular plurals rather than +s", () => {
    // criterion -> criteria is carried defensively, lifted from
    // DecisionCard.contextLine, which solved it for the same vocabulary. No
    // verb mints a *.criterion event today; the map is here so the first one
    // that does is not spelled "criterions".
    expect(burstPhrase("add.criterion", 4).subject).toBe("criteria added");
    // Everything the stream actually mints pluralizes with a plain +s.
    expect(burstPhrase("add.feature", 2).subject).toBe("features added");
  });

  it("renders an unknown verb raw rather than guessing at a participle", () => {
    // KIND_TITLE's `?? kind` discipline, one level down: a build that mints a
    // verb this one has never seen prints the token the payload carries.
    expect(burstPhrase("frobnicate.feature", 3)).toEqual({
      subject: "features frobnicate",
      verb: "frobnicate",
    });
  });

  it("takes +s on an unknown noun", () => {
    expect(burstPhrase("add.widget", 3).subject).toBe("widgets added");
  });

  it.each(["sync", "weird.type.thing", "", ".feature", "add."])(
    "renders %s raw, because it is not verb.noun",
    (type) => {
      expect(burstPhrase(type, 6)).toEqual({ subject: type, verb: type });
    },
  );
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
