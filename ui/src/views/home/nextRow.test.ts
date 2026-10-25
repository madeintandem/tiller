// Vitest coverage for an Up next row's words: the link label a ranked move's
// node shows, and the reason beside it.

import { describe, expect, it } from "vitest";

import type { NextCandidate } from "../../api";
import { nextLabel, nextReason } from "./nextRow";

const question = { noun: "question", id: "Q-aaaaa0", ref: "Q-aaaaa0", title: "Which database?" };

function candidate(over: Partial<NextCandidate>): NextCandidate {
  return { actor: "human", kind: "answer_question", action: "answer", reason: "", tier: 1, ...over };
}

describe("nextLabel", () => {
  it("names a question by its title", () => {
    expect(nextLabel(question)).toBe("Which database?");
  });

  it("falls back to the address for an untitled question and for other nouns", () => {
    expect(nextLabel({ ...question, title: undefined })).toBe("Q-aaaaa0");
    expect(nextLabel({ noun: "task", id: "T-aaaaa0", key: "e/f/t", ref: "e/f/t (T-aaaaa0)", title: "Wire it" })).toBe("e/f/t");
  });
});

describe("nextReason", () => {
  it("drops the question title the link already shows", () => {
    expect(nextReason(candidate({ node: question, reason: "due tomorrow; Which database?" }))).toBe("Due tomorrow.");
    expect(nextReason(candidate({ node: question, reason: "Which database?" }))).toBe("");
  });

  it("keeps a reason that does not end on the title", () => {
    expect(nextReason(candidate({ node: { ...question, title: undefined }, reason: "due tomorrow; Which database?" }))).toBe(
      "Due tomorrow; Which database?",
    );
  });

  it("names the branch a move continues on", () => {
    const own = candidate({
      actor: "agent",
      kind: "continue",
      action: "continue",
      node: { noun: "task", id: "T-aaaaa0", ref: "e/f/t (T-aaaaa0)" },
      reason: "your task, in flight on its branch",
      branch: "tiller/e/f/t",
    });
    expect(nextReason(own)).toBe("Your task, in flight on its branch. Continue it on `tiller/e/f/t`.");
  });
});
