import { describe, expect, it } from "vitest";

import type { Recap, RecapEntry } from "../../api";
import { composeJustHappened } from "./justHappened";

function entry(overrides: Partial<RecapEntry> & Pick<RecapEntry, "type" | "contributor">): RecapEntry {
  return { at: "2026-08-10T12:00:00Z", command: "tiller add task", ask: false, targets: [], ...overrides };
}

describe("composeJustHappened", () => {
  it("bursts within a group but never merges two groups' bursts into one", () => {
    const recap: Recap = {
      since: "",
      window: "24h",
      reader: "jc",
      total: 4,
      groups: [
        {
          epic: { key: "form-engine", id: "E-1", ref: "form-engine (E-1)" },
          stream: "jc",
          entries: [entry({ type: "done.task", contributor: "jc" }), entry({ type: "done.task", contributor: "jc" })],
          omitted: 0,
        },
        {
          epic: { key: "intake", id: "E-2", ref: "intake (E-2)" },
          stream: "jc",
          entries: [entry({ type: "done.task", contributor: "jc" }), entry({ type: "done.task", contributor: "jc" })],
          omitted: 0,
        },
      ],
    };

    const { bursts, moreCount } = composeJustHappened(recap, 5);

    // Two groups of two identical-looking done.task/jc entries stay two
    // bursts, not one: a merge-then-burst would have collapsed all four into
    // a single "4 tasks done" row, which would be claiming one node's work
    // spanned two different epics.
    expect(bursts).toHaveLength(2);
    expect(bursts[0].entries).toHaveLength(2);
    expect(bursts[1].entries).toHaveLength(2);
    expect(moreCount).toBe(0);
  });

  it("caps at the given number of burst rows and reports what the cap left out", () => {
    const recap: Recap = {
      since: "",
      window: "24h",
      reader: "jc",
      total: 3,
      groups: [
        {
          epic: null,
          stream: "jc",
          entries: [
            entry({ type: "add.epic", contributor: "jc" }),
            entry({ type: "add.feature", contributor: "jc" }),
            entry({ type: "add.question", contributor: "jc" }),
          ],
          omitted: 0,
        },
      ],
    };

    // Three distinct types in a row never collapse into one burst (bursting
    // only joins CONSECUTIVE same-type-same-owner runs), so this group is
    // three one-entry bursts; capping at 2 rows leaves one entry unshown.
    const { bursts, moreCount } = composeJustHappened(recap, 2);
    expect(bursts).toHaveLength(2);
    expect(moreCount).toBe(1);
  });

  it("counts moreCount against the API's true total, not just what groups served", () => {
    // recap.total is the true pre-cap count across the whole window (Recap's
    // own doc comment); a group can carry fewer served entries than its own
    // true share (its own `omitted`), and the true total still has to be
    // what moreCount measures against.
    const recap: Recap = {
      since: "",
      window: "24h",
      reader: "jc",
      total: 50,
      groups: [
        { epic: null, stream: "jc", entries: [entry({ type: "add.feature", contributor: "jc" })], omitted: 9 },
      ],
    };
    const { bursts, moreCount } = composeJustHappened(recap, 5);
    expect(bursts).toHaveLength(1);
    expect(moreCount).toBe(49);
  });

  it("returns no bursts and zero more on an empty window", () => {
    const recap: Recap = { since: "", window: "24h", reader: "jc", total: 0, groups: [] };
    const { bursts, moreCount } = composeJustHappened(recap, 6);
    expect(bursts).toHaveLength(0);
    expect(moreCount).toBe(0);
  });
});
