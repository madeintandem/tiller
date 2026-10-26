// Vitest coverage for the burst desk's band composition,
// written before bands.ts exists. bandRuns folds any kind group over ROW_CAP
// into a band run and merges the adjacent uncompressed groups around it into
// one cards run so they still share a single grid (settled decisions 2-4).
// batchCommand exists only for queue kinds backed by a multi-node CLI verb.
// approve_tasks is absent because --feature excludes positional nodes, and
// approve_amendment is absent because the CLI accepts one amendment address.
// sharedWhy answers a band's one shared sentence only when whyLine agrees
// across every entry, never a majority or a first-wins guess.

import { describe, expect, it } from "vitest";

import type { QueueEntry } from "../../api";
import { ROW_CAP } from "../../lib/disclosure";
import { bandRuns, batchCommand, sharedWhy } from "./bands";
import type { QueueCardGroup } from "./queueGroups";

let nextId = 0;

function entry(kind: string, overrides: Partial<QueueEntry> = {}): QueueEntry {
  nextId += 1;
  const id = overrides.id ?? `X-${nextId}`;
  return {
    kind,
    noun: "feature",
    id,
    ref: `${id} (${id})`,
    command: `tiller approve ${kind} ${id}`,
    ...overrides,
  };
}

function group(kind: string, count: number): QueueCardGroup {
  return { kind, entries: Array.from({ length: count }, () => entry(kind)) };
}

describe("bandRuns", () => {
  it("gives every kind its own run in gravity order, cards or band, never merging neighbours", () => {
    // Sizes 1, 2, 7, 3 relative to the settled cap: only the third group
    // exceeds it and folds into a band. Each run is one column of the desk
    // (JC, 2026-08-31: three walls of rows wanted columns), so two small
    // kinds side by side stay two runs rather than pooling into one grid.
    const groups = [
      group("attest_gate", 1),
      group("approve_epic", 2),
      group("approve_feature", 7),
      group("approve_bug", 3),
    ];

    const runs = bandRuns(groups);

    expect(runs).toEqual([
      { type: "cards", kind: "attest_gate", entries: groups[0].entries },
      { type: "cards", kind: "approve_epic", entries: groups[1].entries },
      { type: "band", group: groups[2] },
      { type: "cards", kind: "approve_bug", entries: groups[3].entries },
    ]);
  });

  it("keeps a group of exactly ROW_CAP as cards but compresses ROW_CAP + 1", () => {
    const atCap = group("approve_feature", ROW_CAP);
    const overCap = group("approve_bug", ROW_CAP + 1);

    expect(bandRuns([atCap])).toEqual([{ type: "cards", kind: "approve_feature", entries: atCap.entries }]);
    expect(bandRuns([overCap])).toEqual([{ type: "band", group: overCap }]);
  });

  it("keeps amendment bursts as cards so every single-address command remains visible", () => {
    const amendments = group("approve_amendment", ROW_CAP + 1);

    expect(bandRuns([amendments])).toEqual([
      { type: "cards", kind: "approve_amendment", entries: amendments.entries },
    ]);
  });
});

describe("bandRuns: review gate attestations", () => {
  it("keeps attest_gate bursts as cards so every gate's own command remains runnable", () => {
    const attests = group("attest_gate", ROW_CAP + 1);

    expect(bandRuns([attests])).toEqual([{ type: "cards", kind: "attest_gate", entries: attests.entries }]);
  });
});

describe("batchCommand", () => {
  it("joins approve_epic entries by their addrOf address", () => {
    const entries = [
      entry("approve_epic", { key: "platform-foundation" }),
      entry("approve_epic", { key: "ui-foundation" }),
    ];

    expect(batchCommand("approve_epic", entries)).toBe(
      "tiller approve epic platform-foundation ui-foundation",
    );
  });

  it("falls back to id where key is absent, addrOf's own rule", () => {
    const entries = [
      entry("approve_epic", { key: "platform-foundation" }),
      entry("approve_epic", { id: "E-9" }),
    ];

    expect(batchCommand("approve_epic", entries)).toBe(
      "tiller approve epic platform-foundation E-9",
    );
  });

  it("returns null for approve_tasks: its --feature flag is mutually exclusive with positional nodes", () => {
    const entries = [entry("approve_tasks"), entry("approve_tasks")];

    expect(batchCommand("approve_tasks", entries)).toBeNull();
  });

  it("returns null for approve_amendment because the CLI accepts one address", () => {
    const entries = [
      entry("approve_amendment", { key: "iphone-only" }),
      entry("approve_amendment", { key: "desktop-only" }),
    ];

    expect(batchCommand("approve_amendment", entries)).toBeNull();
  });

  it("returns null for answer_question: no batch verb exists for this kind", () => {
    const entries = [entry("answer_question"), entry("answer_question")];

    expect(batchCommand("answer_question", entries)).toBeNull();
  });

  it("returns null for a batch of one entry, the row's own single", () => {
    expect(batchCommand("approve_chore", [entry("approve_chore")])).toBeNull();
  });
});

describe("sharedWhy", () => {
  it("returns the common why-line for two approve_epic entries", () => {
    const entries = [entry("approve_epic"), entry("approve_epic")];

    expect(sharedWhy(entries)).toBe("Decomposition proposed and ready for your review.");
  });

  it("returns empty when why-lines differ: two answer_question entries with different due dates", () => {
    const entries = [
      entry("answer_question", { due: "2026-09-01" }),
      entry("answer_question", { due: "2026-09-05" }),
    ];

    expect(sharedWhy(entries)).toBe("");
  });
});
