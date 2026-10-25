import { describe, expect, it } from "vitest";

import type { ListResult, ListRow } from "../../api";
import { expandableShelfIDs, selectedShelfBranch } from "./shelves";

const bugRow: ListRow = {
  id: "B-a",
  key: "bugs/a",
  title: "Blank on reload",
  status: "ready",
  ref: "bugs/a (B-a)",
};

function listResult(noun: string, nodes: ListRow[]): ListResult {
  return { noun, count: nodes.length, nodes };
}

describe("Explore shelf helpers", () => {
  it("maps shelf selections to their virtual navigator branch", () => {
    expect(selectedShelfBranch("bug")).toEqual(["shelf:bug"]);
    expect(selectedShelfBranch("chore")).toEqual(["shelf:chore"]);
    expect(selectedShelfBranch("tweak")).toEqual(["shelf:tweak"]);
    expect(selectedShelfBranch("task")).toEqual([]);
  });

  it("opens only shelves that have rows", () => {
    expect(expandableShelfIDs(listResult("bug", [bugRow]), listResult("chore", []), listResult("tweak", []))).toEqual([
      "shelf:bug",
    ]);
  });
  it("opens a populated Tweak shelf without opening empty shelves", () => {
    expect(expandableShelfIDs(listResult("bug", []), listResult("chore", []), listResult("tweak", [{ ...bugRow, id: "TW-abc123", key: "tweaks/button" }]))).toEqual(["shelf:tweak"]);
  });
});
