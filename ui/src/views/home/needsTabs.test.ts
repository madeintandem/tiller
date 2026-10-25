// The desk's scale-aware tabs (JC, 2026-09-01): past ROW_CAP decisions and
// with more than one kind on the queue, the needs-you body shows one kind at
// a time behind a tab rail that names every kind with its count; at the cap
// or under, or with one kind, there is no rail and the body is what it was.
// The active tab is the one the URL names when that kind is present, and the
// first (highest-gravity) kind otherwise.
import { describe, expect, it } from "vitest";

import type { QueueEntry } from "../../api";
import { ROW_CAP } from "../../lib/disclosure";
import type { BandRun } from "./bands";
import { needsTabs, runKind } from "./needsTabs";

let nextId = 0;

function entry(kind: string): QueueEntry {
  nextId += 1;
  return { kind, noun: "feature", id: `X-${nextId}`, ref: `X-${nextId}`, command: `tiller approve ${kind} X-${nextId}` };
}

function cards(kind: string, count: number): BandRun {
  return { type: "cards", kind, entries: Array.from({ length: count }, () => entry(kind)) };
}

function band(kind: string, count: number): BandRun {
  return { type: "band", group: { kind, entries: Array.from({ length: count }, () => entry(kind)) } };
}

const total = (runs: BandRun[]) => runs.reduce((n, r) => n + (r.type === "band" ? r.group.entries.length : r.entries.length), 0);

describe("needsTabs", () => {
  it("tabs the kinds in run order past the cap, the first one active", () => {
    const runs = [cards("approve_amendment", 1), band("approve_epic", ROW_CAP + 1), cards("answer_question", 2)];
    const tabs = needsTabs(runs, total(runs), null);
    expect(tabs.kinds).toEqual(["approve_amendment", "approve_epic", "answer_question"]);
    expect(tabs.active).toBe("approve_amendment");
  });

  it("opens the kind the URL names when it is on the queue, and the first kind when it is not", () => {
    const runs = [cards("approve_amendment", 1), cards("approve_epic", 3), cards("answer_question", 2)];
    expect(needsTabs(runs, total(runs), "answer_question").active).toBe("answer_question");
    expect(needsTabs(runs, total(runs), "approve_bug").active).toBe("approve_amendment");
  });

  it("shows no rail at the cap or under, however many kinds there are", () => {
    const runs = [cards("approve_amendment", 1), cards("approve_epic", 2), cards("answer_question", 2)];
    const tabs = needsTabs(runs, total(runs), "approve_epic");
    expect(tabs.kinds).toEqual([]);
    expect(tabs.active).toBeNull();
  });

  it("shows no rail for one kind, however many entries it has", () => {
    const runs = [band("approve_epic", ROW_CAP + 12)];
    expect(needsTabs(runs, total(runs), null).kinds).toEqual([]);
  });

  it("names a run's kind whether it is cards or a band", () => {
    expect(runKind(cards("approve_bug", 1))).toBe("approve_bug");
    expect(runKind(band("approve_chore", 7))).toBe("approve_chore");
  });
});
