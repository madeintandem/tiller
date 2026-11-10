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

  it("never leaves a dotted type's verb as the raw or humanized token", () => {
    // A verb with no participle falls to humanize(), which still passes the
    // character check above ("tasks claim"). Dotless types (amendment_opened,
    // superseded) are whole fixed phrases, so they are exempt.
    for (const type of vocabulary.event_types.filter((t) => t.includes("."))) {
      const first = type.split(".")[0];
      const { verb } = eventPhrase(type, 1);
      expect(verb, `${type} verb`).not.toBe(first);
      expect(verb, `${type} verb`).not.toBe(first.replace(/[_-]+/g, " "));
    }
  });

  it.each([
    ["withdraw.feature", 1, "feature withdrawn", "withdrawn"],
    ["attest.feature", 4, "features attested", "attested"],
    ["accept.epic", 1, "epic accepted", "accepted"],
    ["revise.feature", 2, "features revised", "revised"],
    ["link.feature", 6, "features linked", "linked"],
    ["cancel.task", 3, "tasks cancelled", "cancelled"],
    ["unlink.feature", 1, "feature unlinked", "unlinked"],
  ])("pins lifecycle verb %s at %i as %s", (type, count, subject, verb) => {
    expect(eventPhrase(type, count)).toEqual({ subject, verb });
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
