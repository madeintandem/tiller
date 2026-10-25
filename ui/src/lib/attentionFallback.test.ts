// Ported from lib/attention.test.ts (deleted at d5c5e13; `git show
// d5c5e13^:ui/src/lib/attention.test.ts` has the original), unchanged in
// substance: this is the same derivation, reconstructed as a fallback
// behind the queue join rather than the primary source (task-17-review
// fix wave, Important finding 4).

import { describe, expect, it } from "vitest";

import type { NodeView, Relationships } from "../api";
import { needsAttentionFallback } from "./attentionFallback";

function node(overrides: Partial<NodeView> = {}): NodeView {
  return {
    noun: "feature",
    id: "F-1",
    key: "foundation/app-skeleton-ci-boot",
    status: "specced",
    ...overrides,
  };
}

describe("needsAttentionFallback: needs_attestation supersedes next_owner", () => {
  it("fires the attest command for a needs_attestation criterion even when next_owner is agent", () => {
    const action = needsAttentionFallback(
      node({
        status: "in_review",
        next_owner: "agent",
        gates: {
          evals: {
            required: true,
            status: "pending",
            criteria: {
              C1: { status: "pass" },
              C6: { status: "needs_attestation" },
            },
          },
        },
      }),
    );

    expect(action).toEqual({
      label: "Awaiting your attestation",
      command: "tiller attest feature foundation/app-skeleton-ci-boot --criterion C6 --status pass",
    });
  });

  it("leads with the lowest sorted criterion id when several await attestation, label unchanged", () => {
    const action = needsAttentionFallback(
      node({
        status: "in_review",
        next_owner: "agent",
        gates: {
          evals: {
            required: true,
            criteria: {
              C6: { status: "needs_attestation" },
              C3: { status: "needs_attestation" },
            },
          },
        },
      }),
    );

    expect(action).toEqual({
      label: "Awaiting your attestation",
      command: "tiller attest feature foundation/app-skeleton-ci-boot --criterion C3 --status pass",
    });
  });

  it("ignores a needs_attestation criterion on a gate that is not itself required", () => {
    const action = needsAttentionFallback(
      node({
        status: "in_review",
        next_owner: "agent",
        gates: {
          "code-review": {
            required: false,
            criteria: { C1: { status: "needs_attestation" } },
          },
        },
      }),
    );

    expect(action).toBeNull();
  });
});

describe("needsAttentionFallback: plain next_owner gates", () => {
  it("returns null when next_owner is agent and nothing needs attestation", () => {
    expect(needsAttentionFallback(node({ status: "ready", next_owner: "agent" }))).toBeNull();
  });

  it("returns null when next_owner is empty (terminal)", () => {
    expect(needsAttentionFallback(node({ status: "done", next_owner: "" }))).toBeNull();
  });

  // Issue #152: the queue owns approve_feature and hides it exactly when the
  // approve would refuse (an open question on the feature or any of its
  // tasks), so a queue silent about a specced feature never gets a
  // recomputed approve from here.
  it("never offers approve feature for a specced feature", () => {
    const action = needsAttentionFallback(node({ status: "specced", next_owner: "human" }));
    expect(action).toEqual({
      label: "Needs your attention next",
      command: "tiller show feature foundation/app-skeleton-ci-boot",
    });
  });

  it("names the approve command for a draft epic, addressed by key", () => {
    const action = needsAttentionFallback(
      node({ noun: "epic", key: "foundation", id: "E-1", status: "draft", next_owner: "human" }),
    );
    expect(action).toEqual({
      label: "Awaiting your approval",
      command: "tiller approve epic foundation",
    });
  });

  it("names the approve command for a proposed task, addressed by its canonical key", () => {
    const action = needsAttentionFallback(
      node({
        noun: "task",
        key: "foundation/app-skeleton-ci-boot/boot-the-app",
        id: "T-br7z1r",
        status: "proposed",
        next_owner: "human",
      }),
    );
    expect(action).toEqual({
      label: "Awaiting your approval",
      command: "tiller approve task foundation/app-skeleton-ci-boot/boot-the-app",
    });
  });

  it("falls back to the id for a task the tree could not place", () => {
    const action = needsAttentionFallback(
      node({ noun: "task", key: undefined, id: "T-br7z1r", status: "proposed", next_owner: "human" }),
    );
    expect(action).toEqual({
      label: "Awaiting your approval",
      command: "tiller approve task T-br7z1r",
    });
  });

  it("names the holding question's answer, not approve task, for a proposed task a question holds", () => {
    const relationships: Relationships = {
      blocked_by_questions: [
        {
          noun: "question",
          id: "Q-aaaaaa",
          ref: "Q-aaaaaa",
          status: "open",
          title: "Canonical schema version",
          statement: "Which schema version is canonical?",
        },
      ],
    };
    const action = needsAttentionFallback(
      node({
        noun: "task",
        key: "foundation/app-skeleton-ci-boot/boot-the-app",
        id: "T-br7z1r",
        status: "proposed",
        next_owner: "human",
      }),
      relationships,
    );
    expect(action).toEqual({
      label: "Awaiting your answer · Canonical schema version (Q-aaaaaa)",
      command: 'tiller answer question Q-aaaaaa --answer "..."',
    });
  });

  it("names the holding question's answer for a specced feature a question names", () => {
    const action = needsAttentionFallback(node({ status: "specced", next_owner: "human" }), {
      blocked_by_questions: [{ noun: "question", id: "Q-bbbbbb", ref: "Q-bbbbbb", status: "open" }],
    });
    expect(action).toEqual({
      label: "Awaiting your answer · Q-bbbbbb",
      command: 'tiller answer question Q-bbbbbb --answer "..."',
    });
  });

  it("still names approve task for a proposed task no question holds", () => {
    const action = needsAttentionFallback(
      node({ noun: "task", key: "foundation/app-skeleton-ci-boot/boot-the-app", status: "proposed", next_owner: "human" }),
      {},
    );
    expect(action).toEqual({
      label: "Awaiting your approval",
      command: "tiller approve task foundation/app-skeleton-ci-boot/boot-the-app",
    });
  });

  it("never offers approve task under a retired ancestor", () => {
    const action = needsAttentionFallback(
      node({
        noun: "task",
        key: "foundation/app-skeleton-ci-boot/boot-the-app",
        status: "proposed",
        next_owner: "human",
        held: { noun: "feature", id: "F-1", ref: "foundation/app-skeleton-ci-boot (F-1)", state: "cancelled" },
      }),
    );
    expect(action).toEqual({
      label: "Needs your attention next",
      command: "tiller show task foundation/app-skeleton-ci-boot/boot-the-app",
    });
  });

  // The case this fallback exists for: a ready epic's only legal verb is
  // cancel, which the queue never tracks as a row (ui-surface.md's action
  // queue never names it), so this is the one derivation left to say
  // anything on the page at all.
  it("degrades to a safe show command for a human gate this task names no specific verb for", () => {
    const action = needsAttentionFallback(
      node({ noun: "epic", key: "foundation", status: "ready", next_owner: "human" }),
    );
    expect(action).toEqual({
      label: "Needs your attention next",
      command: "tiller show epic foundation",
    });
  });
});
