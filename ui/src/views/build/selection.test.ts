// Vitest coverage for the Build board's `?epics=` URL semantics,
// written before Build.tsx consumes it:
// absent, present-empty, and present-non-empty are three distinct states,
// and collapsing any two of them is the bug this table exists to prevent.

import { describe, expect, it } from "vitest";

import { epicsParamValue, selectedEpicKeys, toggleEpicKey } from "./selection";

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
