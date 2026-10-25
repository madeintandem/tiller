// Vitest coverage for the desk's card-grouping logic (task C1), written
// before groupQueueByGravity exists: the queue arrives from /api/queue kind
// by kind in the API's own fixed build order (approvals, then answers, then
// attestations last - views/queue.go's QueueOf), but the desk displays gravity
// order instead (attest, then approve, then answer - closest to done first, the
// mock's round 3 notes). Grouping must re-rank the KIND buckets without ever
// re-sorting the members within one, which stay in #27's served dependency
// order.

import { describe, expect, it } from "vitest";

import type { QueueEntry } from "../../api";
import { groupQueueByGravity } from "./queueGroups";

function entry(overrides: Partial<QueueEntry> & Pick<QueueEntry, "kind" | "id">): QueueEntry {
  return {
    noun: "feature",
    ref: `${overrides.id} (${overrides.id})`,
    command: "tiller approve feature " + overrides.id,
    ...overrides,
  };
}

describe("groupQueueByGravity", () => {
  it("orders kind groups attest* -> approve* -> answer_question -> feature_check regardless of array order", () => {
    // Deliberately scrambled relative to both gravity AND the API's own
    // build order, so passing proves real re-ranking, not a lucky pass-through.
    // feature_check leads the array here specifically to prove its last
    // place is a real rank (KIND_GRAVITY's explicit entry), not just an
    // accident of appearing after everything else already in the fixture.
    const entries: QueueEntry[] = [
      entry({ kind: "feature_check", id: "F-5" }),
      entry({ kind: "answer_question", id: "Q-1" }),
      entry({ kind: "approve_feature", id: "F-1" }),
      entry({ kind: "attest_criterion", id: "F-2" }),
      entry({ kind: "approve_epic", id: "E-2" }),
      entry({ kind: "attest_gate", id: "F-3" }),
      entry({ kind: "approve_tasks", id: "F-4" }),
    ];

    const groups = groupQueueByGravity(entries);

    expect(groups.map((g) => g.kind)).toEqual([
      "attest_gate",
      "attest_criterion",
      "approve_epic",
      "approve_feature",
      "approve_tasks",
      "answer_question",
      "feature_check",
    ]);
  });

  it("places shelf triage with approvals and a cancelled carrier before parked checks", () => {
    const kinds = groupQueueByGravity([
      entry({ kind: "feature_check", id: "F-1" }),
      entry({ kind: "carrier_cancelled", id: "B-2" }),
      entry({ kind: "approve_chore", id: "CH-1" }),
      entry({ kind: "answer_question", id: "Q-1" }),
      entry({ kind: "approve_bug", id: "B-1" }),
      entry({ kind: "attest_gate", id: "T-1" }),
    ]).map((group) => group.kind);

    expect(kinds).toEqual([
      "attest_gate",
      "approve_bug",
      "approve_chore",
      "answer_question",
      "carrier_cancelled",
      "feature_check",
    ]);
  });

  it("keeps merge asks after parked checks and ahead of unknown kinds", () => {
    const kinds = groupQueueByGravity([
      entry({ kind: "server_added_kind", id: "X-1" }),
      entry({ kind: "merge_epic", id: "E-1" }),
      entry({ kind: "merge_feature", id: "F-1" }),
      entry({ kind: "feature_check", id: "F-2" }),
    ]).map((group) => group.kind);

    expect(kinds).toEqual([
      "feature_check",
      "merge_epic",
      "merge_feature",
      "server_added_kind",
    ]);
  });

  // The three gravity ranks are attest, approve, answer, and nothing sits
  // between them: epic done is a rollup `sync` records, not a gate anyone waits
  // on (artifact-schema.md, Epics States), so the acceptance kind the desk once
  // ranked second no longer arrives. A kind this build does not know still
  // sorts last rather than vanishing, which is what keeps a console older than
  // its server honest about what it cannot place.
  it("sorts an unknown kind last rather than dropping it", () => {
    const groups = groupQueueByGravity([
      entry({ kind: "accept_epic", id: "E-1" }),
      entry({ kind: "answer_question", id: "Q-1" }),
      entry({ kind: "attest_gate", id: "T-1" }),
    ]);

    expect(groups.map((g) => g.kind)).toEqual(["attest_gate", "answer_question", "accept_epic"]);
  });

  it("keeps members within a kind in served order, never re-sorted", () => {
    const entries: QueueEntry[] = [
      entry({ kind: "approve_feature", id: "F-9", key: "z/last" }),
      entry({ kind: "approve_feature", id: "F-1", key: "a/first" }),
    ];

    const groups = groupQueueByGravity(entries);

    expect(groups).toHaveLength(1);
    expect(groups[0].entries.map((e) => e.id)).toEqual(["F-9", "F-1"]);
  });

  it("emits one group per distinct kind present, omitting kinds with no entries", () => {
    const entries: QueueEntry[] = [entry({ kind: "approve_feature", id: "F-1" })];

    const groups = groupQueueByGravity(entries);

    expect(groups).toHaveLength(1);
    expect(groups[0]).toEqual({ kind: "approve_feature", entries: [entries[0]] });
  });

  it("returns no groups for an empty queue", () => {
    expect(groupQueueByGravity([])).toEqual([]);
  });

  // ui-surface.md: a proposed amendment "leads the queue rather than sorting
  // in among the approve gates, because a staged reshape can supersede or
  // cancel the very node another row is asking about." KIND_GRAVITY carried
  // no entry for this kind until this pin, which sorted it dead last under
  // the unrecognized-kind fallback - the opposite of the settled rule, and
  // reachable the moment Home's card grid started consuming this function's
  // order directly (task-10-brief.md).
  it("sorts approve_amendment ahead of attest, the previous leader", () => {
    const groups = groupQueueByGravity([
      entry({ kind: "attest_gate", id: "T-1" }),
      entry({ kind: "approve_amendment", id: "A-1" }),
    ]);

    expect(groups.map((g) => g.kind)).toEqual(["approve_amendment", "attest_gate"]);
  });
});
