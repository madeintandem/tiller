// Vitest coverage for the board's two condition overlays: the ask flag joins
// /api/queue onto cards by node id, scoped to feature-noun entries only; the
// drift flag does the identical join against /api/drift's per-marker candidate
// groups.

import { describe, expect, it } from "vitest";

import type { AttestGateEntry, DriftReportView, Queue, QueueEntry } from "../../api";
import { askFlagLabel, askLabelsFrom, driftIdsFrom } from "./overlays";

function entry(overrides: Partial<QueueEntry> & Pick<QueueEntry, "kind" | "id" | "noun">): QueueEntry {
  return {
    ref: `${overrides.id} (${overrides.id})`,
    command: "tiller approve feature " + overrides.id,
    ...overrides,
  };
}

describe("askFlagLabel", () => {
  it("labels every approve_* kind 'approve'", () => {
    for (const kind of ["approve_amendment", "approve_epic", "approve_feature", "approve_tasks"]) {
      expect(askFlagLabel(entry({ kind, id: "F-1", noun: "feature" }))).toBe("approve");
    }
  });

  it("labels answer_question 'answer'", () => {
    expect(askFlagLabel(entry({ kind: "answer_question", id: "Q-1", noun: "question" }))).toBe("answer");
  });

  it("labels feature_check 'check'", () => {
    expect(askFlagLabel(entry({ kind: "feature_check", id: "F-1", noun: "feature" }))).toBe("check");
  });

  it("names the gate for a well-formed attest_gate entry (the mock's own 'attest C3' grammar)", () => {
    const gateEntry: AttestGateEntry = {
      kind: "attest_gate",
      noun: "feature",
      id: "F-1",
      ref: "F-1 (F-1)",
      command: "tiller attest feature F-1 --gate build --status pass",
      gate: "build",
      gate_status: "pending",
    };
    expect(askFlagLabel(gateEntry)).toBe("attest build");
  });

  it("names the criterion id for attest_criterion, whose prose IS the criterion id", () => {
    expect(askFlagLabel(entry({ kind: "attest_criterion", id: "F-1", noun: "feature", prose: "C3" }))).toBe(
      "attest C3",
    );
  });

  it("falls back to the bare verb when a qualifying field is absent, never a crash", () => {
    expect(askFlagLabel(entry({ kind: "attest_criterion", id: "F-1", noun: "feature" }))).toBe("attest");
  });

  it("falls back to 'ask' for a kind this build has never named", () => {
    expect(askFlagLabel(entry({ kind: "some_future_kind", id: "F-1", noun: "feature" }))).toBe("ask");
  });
});

describe("askLabelsFrom", () => {
  it("maps only feature-noun entries, ignoring a task or question entry by the same-looking id", () => {
    const queue: Queue = {
      entries: [
        entry({ kind: "approve_feature", id: "F-1", noun: "feature" }),
        entry({ kind: "attest_gate", id: "T-1", noun: "task" }),
        entry({ kind: "answer_question", id: "Q-1", noun: "question" }),
      ],
    };
    const labels = askLabelsFrom(queue);
    expect(labels.get("F-1")).toBe("approve");
    expect(labels.has("T-1")).toBe(false);
    expect(labels.has("Q-1")).toBe(false);
  });

  it("keeps the FIRST entry's label for a feature with more than one queued ask, in served order", () => {
    const queue: Queue = {
      entries: [
        entry({ kind: "feature_check", id: "F-1", noun: "feature" }),
        entry({ kind: "approve_feature", id: "F-1", noun: "feature" }),
      ],
    };
    expect(askLabelsFrom(queue).get("F-1")).toBe("check");
  });

  it("returns an empty map for a null (not-yet-loaded) queue rather than throwing", () => {
    expect(askLabelsFrom(null).size).toBe(0);
  });

  it("returns an empty map for an empty queue", () => {
    expect(askLabelsFrom({ entries: [] }).size).toBe(0);
  });
});

describe("driftIdsFrom", () => {
  it("collects feature-noun candidate ids across every marker group", () => {
    const report: DriftReportView = {
      compared: 4,
      candidates: {
        stale_spec: [{ marker: "stale_spec", noun: "feature", id: "F-1" }],
        active_basis: [
          { marker: "active_basis", noun: "feature", id: "F-2" },
          { marker: "active_basis", noun: "task", id: "T-9" },
        ],
      },
    };
    const ids = driftIdsFrom(report);
    expect(ids.has("F-1")).toBe(true);
    expect(ids.has("F-2")).toBe(true);
    expect(ids.has("T-9")).toBe(false);
    expect(ids.size).toBe(2);
  });

  it("returns an empty set when candidates is absent (a clean sweep)", () => {
    expect(driftIdsFrom({ compared: 0 }).size).toBe(0);
  });

  it("returns an empty set for a null (not-yet-loaded) report", () => {
    expect(driftIdsFrom(null).size).toBe(0);
  });
});
