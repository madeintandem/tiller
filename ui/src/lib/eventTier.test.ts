import { describe, expect, it } from "vitest";

import { KNOWN_BOOKKEEPING_VERBS, KNOWN_MILESTONE_VERBS, eventTier } from "./eventTier";
import vocabulary from "./vocabulary.json";

function verbOf(type: string): string {
  return type.split(".")[0];
}

describe("eventTier", () => {
  it("classifies every verb in the event vocabulary without reaching the unknown branch", () => {
    const known = new Set([...KNOWN_BOOKKEEPING_VERBS, ...KNOWN_MILESTONE_VERBS]);
    const missing = vocabulary.event_types.map(verbOf).filter((v) => !known.has(v));
    expect(missing).toEqual([]);
  });

  it("keeps the two verb sets disjoint", () => {
    for (const v of KNOWN_BOOKKEEPING_VERBS) {
      expect(KNOWN_MILESTONE_VERBS.has(v)).toBe(false);
    }
  });

  it("reads the ruled bookkeeping verbs as bookkeeping", () => {
    for (const type of [
      "link.feature",
      "unlink.task",
      "order.features",
      "rename.title.feature",
      "remove.draft.epic",
      "configure.project",
      "claim.task",
      "release.task",
      "reassign.task",
      "transfer.task",
      "grade.bug",
      "migrate.schema",
      "contributor.add",
      "owner.remove",
      "add.feature",
      "add.task",
    ]) {
      expect(eventTier(type, false), type).toBe("bookkeeping");
    }
  });

  it("reads lifecycle crossings, questions, and amendments as milestones", () => {
    for (const type of [
      "approve.feature",
      "submit.task",
      "sync.task",
      "add.question",
      "answer.question",
      "spec.feature",
      "amendment_opened",
      "superseded",
      "approve.amendment",
    ]) {
      expect(eventTier(type, false), type).toBe("milestone");
    }
  });

  it("makes an ask a milestone whatever the type", () => {
    expect(eventTier("link.feature", true)).toBe("milestone");
    expect(eventTier("add.task", true)).toBe("milestone");
  });

  it("reads an unknown type as a milestone", () => {
    expect(eventTier("mystery.thing", false)).toBe("milestone");
    expect(eventTier("", false)).toBe("milestone");
  });
});
