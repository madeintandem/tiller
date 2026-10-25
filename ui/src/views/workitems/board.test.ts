import { describe, expect, it } from "vitest";

import type { WorkItemsBoard } from "../../api";
import {
  readWorkItemFilters,
  setWorkItemFilter,
  SHELF_COLUMN_STATUSES,
  workItemColumns,
} from "./board";

const board: WorkItemsBoard = {
  open: [
    {
      noun: "bug",
      id: "B-z",
      key: "bugs/zulu",
      title: "Zulu",
      status: "ready",
      ref: "bugs/zulu (B-z)",
      attachment_ids: ["E-1", "F-1", "T-1"],
    },
    {
      noun: "bug",
      id: "B-a",
      key: "bugs/alpha",
      title: "Alpha",
      status: "proposed",
      ref: "bugs/alpha (B-a)",
    },
    {
      noun: "chore",
      id: "CH-m",
      key: "chores/mike",
      title: "Mike",
      status: "ready",
      ref: "chores/mike (CH-m)",
      attachment_ids: ["E-1", "F-1"],
    },
  ],
  closed: Array.from({ length: 7 }, (_, index) => ({
    noun: index % 2 === 0 ? ("bug" as const) : ("chore" as const),
    id: `C-${index}`,
    title: `Closed ${index}`,
    status: index === 1 ? "cancelled" : "done",
    ref: `closed-${index} (C-${index})`,
    closed_at: `2026-08-29T${String(20 - index).padStart(2, "0")}:00:00Z`,
    attachment_ids: ["E-1", "F-1"],
  })),
  attachments: [
    {
      noun: "feature",
      id: "F-1",
      key: "foundation/console",
      ref: "foundation/console (F-1)",
      status: "active",
      title: "Console",
    },
  ],
};

describe("work item board filters", () => {
  it("reads URL filters and degrades unknown noun and terminal values", () => {
    expect(readWorkItemFilters(new URLSearchParams("noun=bug&attached=F-1&terminal=all"))).toEqual({
      noun: "bug",
      attached: ["F-1"],
      showAllTerminal: true,
    });
    expect(readWorkItemFilters(new URLSearchParams("noun=unknown&terminal=recent"))).toEqual({
      noun: "all",
      attached: [],
      showAllTerminal: false,
    });
  });

  it("reads attached as a comma list, keeping legacy single-id and none links", () => {
    const read = (query: string) => readWorkItemFilters(new URLSearchParams(query)).attached;
    expect(read("attached=F-1")).toEqual(["F-1"]);
    expect(read("attached=none")).toEqual(["none"]);
    expect(read("attached=F-1,T-1")).toEqual(["F-1", "T-1"]);
    expect(read("attached=none%2CF-1")).toEqual(["none", "F-1"]);
    expect(read("attached=F-1,,F-1, T-1")).toEqual(["F-1", "T-1"]);
    expect(read("attached=")).toEqual([]);
  });

  it("toggles attachment members and clears the axis with null", () => {
    const one = setWorkItemFilter(new URLSearchParams("noun=bug"), "attached", "F-1");
    expect(one.toString()).toBe("noun=bug&attached=F-1");
    const two = setWorkItemFilter(one, "attached", "T-1");
    expect(readWorkItemFilters(two).attached).toEqual(["F-1", "T-1"]);
    const loose = setWorkItemFilter(two, "attached", "none");
    expect(readWorkItemFilters(loose).attached).toEqual(["F-1", "T-1", "none"]);
    expect(readWorkItemFilters(setWorkItemFilter(loose, "attached", "F-1")).attached).toEqual(["T-1", "none"]);
    expect(setWorkItemFilter(loose, "attached", null).toString()).toBe("noun=bug");
    expect(setWorkItemFilter(one, "attached", "F-1").toString()).toBe("noun=bug");
  });

  it("matches any selected attachment, with none as one OR member", () => {
    const tasked = { ...board, open: [...board.open, { noun: "chore" as const, id: "CH-t", ref: "CH-t", status: "ready", attachment_ids: ["T-2"] }] };
    const ready = (attached: string[]) =>
      workItemColumns(tasked, { noun: "all", attached, showAllTerminal: true }).ready.map((item) => item.id);
    expect(ready(["T-1"])).toEqual(["B-z"]);
    expect(ready(["T-1", "T-2"])).toEqual(["B-z", "CH-t"]);
    const proposed = (attached: string[]) =>
      workItemColumns(tasked, { noun: "all", attached, showAllTerminal: true }).proposed.map((item) => item.id);
    expect(proposed(["T-2"])).toEqual([]);
    expect(proposed(["none", "T-2"])).toEqual(["B-a"]);
    expect(ready(["none", "T-2"])).toEqual(["CH-t"]);
  });

  it("maps every shelf heading to its lifecycle palette state", () => {
    expect(SHELF_COLUMN_STATUSES).toEqual({
      proposed: "specced",
      ready: "ready",
      in_progress: "active",
      check: "active",
      in_review: "in_review",
      closed: "done",
    });
  });

  it("filters without reordering and keeps both terminal statuses in Closed", () => {
    const got = workItemColumns(board, { noun: "all", attached: ["F-1"], showAllTerminal: false });
    expect(got.ready.map((item) => item.id)).toEqual(["B-z", "CH-m"]);
    expect(got.closed).toHaveLength(5);
    expect(got.closed[1].status).toBe("cancelled");
  });

  it("supports unattached and all-terminal links", () => {
    const got = workItemColumns(board, { noun: "bug", attached: ["none"], showAllTerminal: true });
    expect(got.proposed.map((item) => item.id)).toEqual(["B-a"]);
    expect(got.closed).toHaveLength(0);
  });

  it("updates one URL axis without dropping the others", () => {
    const next = setWorkItemFilter(new URLSearchParams("noun=bug&attached=F-1"), "terminal", "all");
    expect(next.toString()).toBe("noun=bug&attached=F-1&terminal=all");
  });

  it("drops default or unknown values on untouched filter axes", () => {
    const next = setWorkItemFilter(
      new URLSearchParams("noun=bug&attached=&terminal=recent"),
      "noun",
      "chore",
    );
    expect(next.toString()).toBe("noun=bug%2Cchore");
  });

  it("toggles independent noun selections while preserving attachment and history filters", () => {
    const selected = setWorkItemFilter(new URLSearchParams("noun=bug&attached=F-1&terminal=all"), "noun", "tweak");
    expect(readWorkItemFilters(selected)).toEqual({ noun: "bug,tweak", attached: ["F-1"], showAllTerminal: true });
    const tweakOnly = setWorkItemFilter(selected, "noun", "bug");
    expect(readWorkItemFilters(tweakOnly).noun).toBe("tweak");
    expect(readWorkItemFilters(setWorkItemFilter(tweakOnly, "noun", "all")).noun).toBe("all");
  });

  it("places tweaks in each lifecycle column before filtering terminal history", () => {
    const statuses = ["proposed", "ready", "in_progress", "check", "in_review"];
    const tweaks = statuses.map((status) => ({
      noun: "tweak" as const, id: `TW-${status}`, ref: `TW-${status}`, title: status, status,
      attachment_ids: ["F-1"],
    }));
    const closed = Array.from({ length: 7 }, (_, index) => ({
      noun: "tweak" as const, id: `TW-closed-${index}`, ref: `TW-closed-${index}`,
      status: index === 0 ? "cancelled" : "done", attachment_ids: ["F-1"],
    }));
    const mixed = { ...board, open: [...board.open, ...tweaks], closed: [...closed, ...board.closed] };
    const got = workItemColumns(mixed, { noun: "tweak", attached: ["F-1"], showAllTerminal: false });
    for (const status of statuses) {
      expect(got[status as keyof typeof got].map((item) => item.id)).toEqual([`TW-${status}`]);
    }
    expect(got.closed).toHaveLength(5);
    expect(got.closed[0].status).toBe("cancelled");
    expect(workItemColumns(mixed, { noun: "tweak", attached: ["F-1"], showAllTerminal: true }).closed).toHaveLength(7);
    expect(workItemColumns(mixed, { noun: "bug,tweak", attached: ["F-1"], showAllTerminal: true }).ready.map((item) => item.id)).toEqual(["B-z", "TW-ready"]);
    expect(workItemColumns(mixed, { noun: "tweak", attached: ["none"], showAllTerminal: true }).closed).toEqual([]);
  });
});
