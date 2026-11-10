import { describe, expect, it } from "vitest";
import vocabulary from "./vocabulary.json";
import { eventPhrase } from "./eventPhrase";

describe("eventPhrase", () => {
  it("has plain-English words for every event type the code can mint", () => {
    expect(vocabulary.event_types.length).toBeGreaterThan(0);
    for (const type of vocabulary.event_types) {
      for (const count of [1, 3]) {
        const p = eventPhrase(type, count);
        expect(p.subject, `${type} subject`).toMatch(/^[a-z][a-z ]*$/);
        expect(p.verb, `${type} verb`).toMatch(/^[a-z][a-z ]*$/);
      }
    }
  });

  it.each([
    ["rename.title.task", 1, "task retitled", "retitled"],
    ["rename.title.task", 3, "tasks retitled", "retitled"],
    ["rename.key.feature", 1, "feature rekeyed", "rekeyed"],
    ["link.attach.bug", 1, "bug attached", "attached"],
    ["unlink.attach.bug", 1, "bug detached", "detached"],
    ["remove.proposed.task", 1, "task removed", "removed"],
    ["remove.draft.feature", 2, "features removed", "removed"],
    ["order.features", 1, "features reordered", "reordered"],
    ["order.features", 3, "features reordered", "reordered"],
    ["amendment_opened", 1, "amendment opened", "amendment opened"],
    ["correct-eval.feature", 1, "feature eval corrected", "eval corrected"],
    ["recover_review.epic", 1, "epic review recovered", "review recovered"],
    ["migrate.schema", 1, "schema migrated", "schema migrated"],
    ["link.surface.feature", 3, "surfaces recorded", "recorded"],
    ["owner.add", 2, "owners added", "added"],
    ["add.question", 1, "question asked", "asked"],
    ["add.criterion", 4, "criteria added", "added"],
  ])("phrases %s at %i as %s", (type, count, subject, verb) => {
    expect(eventPhrase(type, count)).toEqual({ subject, verb });
  });

  it("humanizes an unknown type instead of printing it raw", () => {
    expect(eventPhrase("frobnicate.widget", 1)).toEqual({ subject: "widget frobnicate", verb: "frobnicate" });
    expect(eventPhrase("frobnicate.widget", 3)).toEqual({ subject: "widgets frobnicate", verb: "frobnicate" });
    expect(eventPhrase("odd_thing", 1)).toEqual({ subject: "odd thing", verb: "odd thing" });
    expect(eventPhrase("odd_thing", 5)).toEqual({ subject: "odd thing", verb: "odd thing" });
  });
});
