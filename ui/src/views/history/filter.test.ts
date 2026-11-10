import { describe, expect, it } from "vitest";

import type { Recap, RecapEntry, RecapGroup } from "../../api";
import { ASKED_YOU, filterRecap, historyFacets, kindLabel } from "./filter";

function entry(type: string, contributor: string, ask = false): RecapEntry {
  return { at: "2026-07-24T10:00:00Z", type, contributor, command: "x", ask, targets: [] };
}

function group(epic: { id: string; key: string; title?: string } | null, entries: RecapEntry[]): RecapGroup {
  return {
    epic: epic === null ? null : { ...epic, ref: `${epic.key} (${epic.id})` },
    stream: "jc",
    entries,
    omitted: 0,
  };
}

function recap(groups: RecapGroup[], total = 1000): Recap {
  return { since: "", window: "7d", reader: "jc", total, groups };
}

const sample = () =>
  recap([
    group({ id: "E-1", key: "audit", title: "Audit Engine" }, [
      entry("spec.feature", "jc"),
      entry("approve.feature", "dana"),
      entry("add.question", "jc", true),
    ]),
    group({ id: "E-2", key: "billing" }, [entry("spec.feature", "dana"), entry("check.feature", "jc", true)]),
    group(null, [entry("add.epic", "jc")]),
  ]);

describe("historyFacets", () => {
  it("lists contributors, phrase verbs with asked you first, and epics by title", () => {
    const f = historyFacets(sample());
    expect(f.actors.slice().sort()).toEqual(["dana", "jc"]);
    expect(f.kinds[0]).toBe(ASKED_YOU);
    expect(kindLabel(f.kinds[0])).toBe("asked you");
    expect(f.kinds.slice(1)).toEqual(f.kinds.slice(1).slice().sort());
    expect(f.kinds).toContain("specced");
    expect(f.kinds).toContain("approved");
    // add.question's own verb is a kind of its own, apart from asked you.
    expect(f.kinds).toContain("asked");
    expect(kindLabel("asked")).toBe("asked");
    expect(f.kinds.filter((k) => k === ASKED_YOU)).toHaveLength(1);
    expect(f.kinds.some((k) => k.includes("_") || k.includes("."))).toBe(false);
    expect(f.epics).toEqual([
      { id: "E-1", title: "Audit Engine" },
      { id: "E-2", title: "billing" },
    ]);
  });

  it("has no asked you option when nothing asked the reader", () => {
    const f = historyFacets(recap([group(null, [entry("spec.feature", "jc")])]));
    expect(f.kinds).toEqual(["specced"]);
    const question = historyFacets(recap([group(null, [entry("add.question", "jc")])]));
    expect(question.kinds).toEqual(["asked"]);
  });
});

describe("filterRecap", () => {
  it("passes everything through with no filter, shown equal to the served count", () => {
    const out = filterRecap(sample(), {});
    expect(out.groups).toHaveLength(3);
    expect(out.shown).toBe(6);
    expect(out.total).toBe(1000);
  });

  it("filters by actor and drops empty groups", () => {
    const out = filterRecap(sample(), { actor: "dana" });
    expect(out.groups.map((g) => g.entries.length)).toEqual([1, 1]);
    expect(out.shown).toBe(2);
    expect(out.total).toBe(1000);
  });

  it("filters by kind, asked you matching exactly the ask flag the tally counts", () => {
    expect(filterRecap(sample(), { kind: "specced" }).shown).toBe(2);
    const withQuestion = recap([group(null, [entry("add.question", "dana"), entry("check.feature", "jc", true), entry("add.question", "jc", true)])]);
    const askedYou = filterRecap(withQuestion, { kind: ASKED_YOU });
    const tally = withQuestion.groups.flatMap((g) => g.entries).filter((e) => e.ask).length;
    expect(askedYou.shown).toBe(tally);
    expect(askedYou.groups.every((g) => g.entries.every((e) => e.ask))).toBe(true);
    // A question that did not ask the reader is found under its own verb.
    const asked = filterRecap(withQuestion, { kind: "asked" });
    expect(asked.shown).toBe(2);
    expect(asked.groups.every((g) => g.entries.every((e) => e.type === "add.question"))).toBe(true);
  });

  it("filters by epic", () => {
    const out = filterRecap(sample(), { epic: "E-2" });
    expect(out.groups).toHaveLength(1);
    expect(out.shown).toBe(2);
  });

  it("composes filters", () => {
    expect(filterRecap(sample(), { actor: "jc", kind: "specced", epic: "E-1" }).shown).toBe(1);
    expect(filterRecap(sample(), { actor: "dana", epic: "E-1", kind: ASKED_YOU }).groups).toEqual([]);
  });
});
