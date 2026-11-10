// Vitest coverage for the Build board's `?epics=` URL semantics,
// written before Build.tsx consumes it:
// absent, present-empty, and present-non-empty are three distinct states,
// and collapsing any two of them is the bug this table exists to prevent.

import { describe, expect, it } from "vitest";

import type { BuildEpic } from "../../api";
import {
  epicsParamValue,
  isAllEpics,
  resolveSelection,
  selectedEpicKeys,
  shouldStoreSelection,
  showableEpics,
  splitEpics,
  toggleEpicKey,
} from "./selection";

describe("selectedEpicKeys", () => {
  it("absent (null) reads as the default selection", () => {
    expect(selectedEpicKeys(null, ["intake", "form-engine"])).toEqual(["intake", "form-engine"]);
  });

  it("present but empty reads as nothing selected, never the default", () => {
    expect(selectedEpicKeys("", ["intake", "form-engine"])).toEqual([]);
  });

  it("present and non-empty reads as exactly those keys, ignoring the default entirely", () => {
    expect(selectedEpicKeys("a,b", ["intake", "form-engine"])).toEqual(["a", "b"]);
  });

  it("drops stray empty segments from a malformed list rather than crashing", () => {
    expect(selectedEpicKeys("a,,b,", [])).toEqual(["a", "b"]);
  });

  it("a default selection is never consulted once the param is present", () => {
    // The empty-default case is the one most likely to accidentally read as
    // "fall through to default" if the null check is ever written loosely
    // (e.g. `raw || defaultKeys`, which "" also satisfies).
    expect(selectedEpicKeys("x", [])).toEqual(["x"]);
  });
});

describe("toggleEpicKey", () => {
  it("appends a key that was not selected", () => {
    expect(toggleEpicKey(["a"], "b")).toEqual(["a", "b"]);
  });

  it("removes a key that was selected", () => {
    expect(toggleEpicKey(["a", "b"], "a")).toEqual(["b"]);
  });

  it("deselecting the last key returns an empty array, not the default", () => {
    expect(toggleEpicKey(["a"], "a")).toEqual([]);
  });

  // Deliberate-but-undocumented until now (toggleEpicKey's own comment states
  // the rationale): an unmatched key already in the selection - one that
  // names no epic the board actually loaded, e.g. a stale `?epics=bogus,
  // intake` param - survives a click on some other key untouched, since it
  // may simply name an epic that has not arrived yet.
  it("preserves a selected key that names no known epic when a different key is toggled", () => {
    expect(toggleEpicKey(["bogus", "intake"], "form-engine")).toEqual([
      "bogus",
      "intake",
      "form-engine",
    ]);
  });
});

describe("epicsParamValue", () => {
  it("joins a non-empty selection with commas", () => {
    expect(epicsParamValue(["a", "b"])).toBe("a,b");
  });

  it("serializes an empty selection to the present-but-empty form, not omitted", () => {
    // This is what keeps a round trip honest: writing "" here is what makes
    // the NEXT selectedEpicKeys(params.get("epics"), ...) read null-vs-""
    // correctly rather than silently reviving the default.
    expect(epicsParamValue([])).toBe("");
  });
});

describe("splitEpics", () => {
  const e = (key: string, status: string) => ({ key, status }) as never;

  it("puts done and archived epics in finished and keeps served order in both lists", () => {
    const epics = [e("a", "active"), e("b", "done"), e("c", "ready"), e("d", "archived"), e("f", "draft")];
    const { live, finished } = splitEpics(epics);
    expect(live.map((x) => x.key)).toEqual(["a", "c", "f"]);
    expect(finished.map((x) => x.key)).toEqual(["b", "d"]);
  });

  it("does not treat a cancelled epic as finished", () => {
    const { live, finished } = splitEpics([e("x", "cancelled")]);
    expect(live).toHaveLength(1);
    expect(finished).toHaveLength(0);
  });
});

describe("epics=all", () => {
  const epics = [
    { id: "E-1", key: "intake" },
    { id: "E-2" },
    { id: "E-3", key: "ops" },
  ] as unknown as BuildEpic[];

  it("showableEpics drops epics with no key", () => {
    expect(showableEpics(epics).map((e) => e.id)).toEqual(["E-1", "E-3"]);
  });

  it("selects every showable epic when it is the whole parameter", () => {
    expect(resolveSelection("all", ["intake"], epics)).toEqual(["intake", "ops"]);
  });

  it("is not stored, while an explicit list and the empty selection are", () => {
    expect(isAllEpics("all")).toBe(true);
    expect(shouldStoreSelection("all")).toBe(false);
    expect(shouldStoreSelection("a,b")).toBe(true);
    expect(shouldStoreSelection("")).toBe(true);
    expect(shouldStoreSelection(null)).toBe(false);
  });

  it("leaves an explicit list, the empty value, and the default exactly as before", () => {
    expect(resolveSelection("a,b", ["x"], epics)).toEqual(["a", "b"]);
    expect(resolveSelection("", ["x"], epics)).toEqual([]);
    expect(resolveSelection(null, ["x"], epics)).toEqual(["x"]);
  });

  it("reads a key literally named all as a key whenever it is not the whole parameter", () => {
    expect(isAllEpics("all,ops")).toBe(false);
    expect(resolveSelection("ops,all", [], epics)).toEqual(["ops", "all"]);
    // A selection that is just that one key serializes so it cannot be read as the special value.
    expect(epicsParamValue(["all"])).toBe("all,");
    expect(isAllEpics(epicsParamValue(["all"]))).toBe(false);
    expect(selectedEpicKeys(epicsParamValue(["all"]), [])).toEqual(["all"]);
  });

  it("a reader's change from all becomes an explicit stored list", () => {
    const next = toggleEpicKey(resolveSelection("all", [], epics), "ops");
    expect(next).toEqual(["intake"]);
    expect(epicsParamValue(next)).toBe("intake");
    expect(shouldStoreSelection(epicsParamValue(next))).toBe(true);
  });
});
