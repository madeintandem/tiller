import { describe, expect, it } from "vitest";

import type { Recap, RecapEntry, RecapGroup } from "../../api";
import { filterRecap, historyFacets } from "./filter";

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
      entry("approve.feature", "allen"),
      entry("add.question", "jc", true),
    ]),
    group({ id: "E-2", key: "billing" }, [entry("spec.feature", "allen"), entry("check.feature", "jc", true)]),
    group(null, [entry("add.epic", "jc")]),
  ]);

describe("historyFacets", () => {
  it("lists contributors, phrase verbs with asked first, and epics by title", () => {
    const f = historyFacets(sample());
    expect(f.actors.slice().sort()).toEqual(["allen", "jc"]);
    expect(f.kinds[0]).toBe("asked");
    expect(f.kinds.slice(1)).toEqual(f.kinds.slice(1).slice().sort());
    expect(f.kinds).toContain("specced");
    expect(f.kinds).toContain("approved");
    expect(f.kinds.filter((k) => k === "asked")).toHaveLength(1);
    expect(f.kinds.some((k) => k.includes("_") || k.includes("."))).toBe(false);
    expect(f.epics).toEqual([
      { id: "E-1", title: "Audit Engine" },
      { id: "E-2", title: "billing" },
    ]);
  });

  it("has no asked option when nothing asked", () => {
    const f = historyFacets(recap([group(null, [entry("spec.feature", "jc")])]));
    expect(f.kinds).toEqual(["specced"]);
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
    const out = filterRecap(sample(), { actor: "allen" });
    expect(out.groups.map((g) => g.entries.length)).toEqual([1, 1]);
    expect(out.shown).toBe(2);
    expect(out.total).toBe(1000);
  });

  it("filters by kind, asked matching the ask flag", () => {
    expect(filterRecap(sample(), { kind: "specced" }).shown).toBe(2);
    const asked = filterRecap(sample(), { kind: "asked" });
    expect(asked.shown).toBe(2);
    expect(asked.groups.every((g) => g.entries.every((e) => e.ask))).toBe(true);
  });

  it("filters by epic", () => {
    const out = filterRecap(sample(), { epic: "E-2" });
    expect(out.groups).toHaveLength(1);
    expect(out.shown).toBe(2);
  });

  it("composes filters", () => {
    expect(filterRecap(sample(), { actor: "jc", kind: "specced", epic: "E-1" }).shown).toBe(1);
    expect(filterRecap(sample(), { actor: "allen", epic: "E-1", kind: "asked" }).groups).toEqual([]);
  });
});
