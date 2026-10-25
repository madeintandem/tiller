import { afterEach, describe, expect, it, vi } from "vitest";

import type { EvalPayload, QueueEntry } from "./api";
import {
  fetchJSON,
  fetchWorkItems,
  getLastFetchAt,
  isAttestGate,
  isEvalError,
  subscribeLastFetch,
} from "./api";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function okJSON(body: unknown) {
  return vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(body) });
}

describe("isEvalError", () => {
  it("narrows an EvalContractError on its error field", () => {
    const payload: EvalPayload = { path: "features/foo.eval.md", error: "malformed frontmatter" };
    expect(isEvalError(payload)).toBe(true);
    if (isEvalError(payload)) {
      // Narrowed without a cast: error is only reachable on the failure case.
      expect(payload.error).toBe("malformed frontmatter");
    }
  });

  it("rejects a well-formed EvalContractView", () => {
    const payload: EvalPayload = { path: "features/foo.eval.md", node: "feature-1", criteria: [] };
    expect(isEvalError(payload)).toBe(false);
  });

  it("rejects null", () => {
    const payload: EvalPayload = null;
    expect(isEvalError(payload)).toBe(false);
  });
});

describe("isAttestGate", () => {
  it("accepts a well-formed attest_gate entry", () => {
    const entry: QueueEntry = {
      kind: "attest_gate",
      noun: "feature",
      id: "feature-1",
      ref: "epic-1/feature-1 (feature-1)",
      command: "tiller attest feature epic-1/feature-1 --gate security_review --status pass",
      gate: "security_review",
      gate_status: "pending",
    };
    expect(isAttestGate(entry)).toBe(true);
  });

  it("rejects an approve_epic entry", () => {
    const entry: QueueEntry = {
      kind: "approve_epic",
      noun: "epic",
      id: "epic-1",
      ref: "epic-1 (epic-1)",
      command: "tiller approve epic epic-1",
    };
    expect(isAttestGate(entry)).toBe(false);
  });

  it("accepts a well-formed feature_check entry", () => {
    // The queue's newest kind (Task 7): an active feature with every task
    // done and a required feature-grain gate unrecorded. It carries no gate
    // or gate_status (those ride attest_gate only), so it must type as a
    // plain QueueEntry and read as not-attest_gate.
    const entry: QueueEntry = {
      kind: "feature_check",
      noun: "feature",
      id: "F-dhzdb2",
      ref: "epic-1/feature-1 (F-dhzdb2)",
      prose: "Feature 1",
      command: "tiller check feature F-dhzdb2",
    };
    expect(entry.kind).toBe("feature_check");
    expect(isAttestGate(entry)).toBe(false);
  });
});

it("fetches the work-items projection from its stable path", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(JSON.stringify({ open: [], closed: [], attachments: [] }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );

  await fetchWorkItems();

  expect(fetchMock).toHaveBeenCalledWith("/api/work-items");
});

// The topbar's asof affordance (AsOf.tsx) tracks this subscribable, which
// fetchJSON updates on every successful read - the one choke point every
// fetchXxx helper and useFetch funnel through, so a failed read must never
// move it. lastFetchAt is genuine module-level state (there is no test-only
// reset), so these tests only ever assert what a fresh success DOES to it,
// never what its value was beforehand.
describe("fetchJSON's last-fetch subscribable", () => {
  it("advances getLastFetchAt and notifies subscribers on a successful read", async () => {
    vi.stubGlobal("fetch", okJSON({ ok: true }));
    const seen: Date[] = [];
    const unsubscribe = subscribeLastFetch((at) => seen.push(at));

    const before = new Date();
    await fetchJSON("/api/status");
    const after = new Date();

    expect(seen).toHaveLength(1);
    expect(getLastFetchAt()).toEqual(seen[0]);
    expect(seen[0].getTime()).toBeGreaterThanOrEqual(before.getTime());
    expect(seen[0].getTime()).toBeLessThanOrEqual(after.getTime());

    unsubscribe();
  });

  it("never notifies subscribers, and never advances the mark, on a failed read", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: false, status: 500, statusText: "Internal Server Error", text: () => Promise.resolve("") }),
    );
    const seen: Date[] = [];
    const unsubscribe = subscribeLastFetch((at) => seen.push(at));
    const before = getLastFetchAt();

    await expect(fetchJSON("/api/status")).rejects.toThrow();

    expect(seen).toHaveLength(0);
    expect(getLastFetchAt()).toEqual(before);

    unsubscribe();
  });

  it("stops notifying once unsubscribed", async () => {
    vi.stubGlobal("fetch", okJSON({ ok: true }));
    const seen: Date[] = [];
    const unsubscribe = subscribeLastFetch((at) => seen.push(at));
    unsubscribe();

    await fetchJSON("/api/status");

    expect(seen).toHaveLength(0);
  });
});
