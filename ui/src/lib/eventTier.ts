// Which events a reader wants by default (ruling R3). History and Home's Just
// happened show milestones: a lifecycle crossing, a question, an amendment,
// anything that asked the reader something. Bookkeeping is the upkeep around
// that work (links, ordering, renames, roster changes, a node simply being
// added) and folds into one disclosure row instead of burying the milestones.
//
// The rule reads a type's verb, its first dot segment. A dotless type
// (amendment_opened, superseded) is its own verb. Both sets are exported so a
// test can pin that every verb in vocabulary.json event_types lands in one of
// them: a type the code starts to emit must be classified on purpose, not by
// falling through. A type neither set knows is still a milestone, because
// hiding an event nobody classified is the worse failure than showing it.

export type EventTier = "milestone" | "bookkeeping";

export const KNOWN_BOOKKEEPING_VERBS: ReadonlySet<string> = new Set([
  "link",
  "unlink",
  "order",
  "rename",
  "remove",
  "configure",
  "claim",
  "release",
  "reassign",
  "transfer",
  "move",
  "grade",
  "migrate",
  "contributor",
  "owner",
  // add is bookkeeping for every noun except question, handled in eventTier.
  "add",
]);

export const KNOWN_MILESTONE_VERBS: ReadonlySet<string> = new Set([
  "accept",
  "amendment_approved",
  "amendment_opened",
  "amendment_proposed",
  "amendment_staged",
  "amendment_withdrawn",
  "answer",
  "approve",
  "archive",
  "attest",
  "cancel",
  "check",
  "correct-eval",
  "eval",
  "recover_review",
  "resolve",
  "resume",
  "return",
  "review",
  "revise",
  "spec",
  "start",
  "submit",
  "superseded",
  "sync",
  "withdraw",
]);

export function eventTier(type: string, ask: boolean): EventTier {
  if (ask) {
    return "milestone";
  }
  const parts = type.split(".");
  const verb = parts[0];
  if (verb === "add" && parts[parts.length - 1] === "question") {
    return "milestone";
  }
  return KNOWN_BOOKKEEPING_VERBS.has(verb) ? "bookkeeping" : "milestone";
}
