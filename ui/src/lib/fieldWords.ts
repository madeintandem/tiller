// Plain words for the small machine vocabularies a node page prints as a
// field value: an amendment's staged operation and its precondition names, a
// shelf item's resolution, and a feature's change type. Each table covers
// every value the Go code defines for that field; a value from a newer
// server is humanized (separators become spaces, sentence case), never
// printed raw.

import { eventPhrase } from "./eventPhrase";

function humanize(token: string): string {
  const spaced = token.replace(/[-_.]+/g, " ").trim();
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

function sentenceCase(words: string): string {
  return words.charAt(0).toUpperCase() + words.slice(1);
}

// An amendment operation (store/amendment.go's AmendmentOp*) reads as the
// phrase History prints for the event its approval mints, so the page and
// the timeline name one change one way: correct-eval approves as
// correct-eval.feature, "Feature eval corrected". The verb is the event's
// verb segment; ops that add or move name their own noun, and the others
// take their target's noun.
const OP_EVENT: Record<string, { verb: string; noun?: string }> = {
  revise: { verb: "revise" },
  supersede: { verb: "superseded" },
  cancel: { verb: "cancel" },
  "add-epic": { verb: "add", noun: "epic" },
  "add-feature": { verb: "add", noun: "feature" },
  "add-task": { verb: "add", noun: "task" },
  "add-question": { verb: "add", noun: "question" },
  "move-feature": { verb: "move", noun: "feature" },
  "correct-eval": { verb: "correct-eval" },
};

export function amendmentOpLabel(op: string, targetNoun?: string): string {
  const event = OP_EVENT[op];
  if (event === undefined) {
    return humanize(op);
  }
  const noun = event.noun ?? targetNoun;
  if (noun === undefined || noun === "") {
    // No noun to name: the participle alone ("Revised").
    return sentenceCase(eventPhrase(`${event.verb}.target`, 1).verb);
  }
  return sentenceCase(eventPhrase(`${event.verb}.${noun}`, 1).subject);
}

// The precondition names a staged operation may carry (mutate/amendment.go).
const PRECONDITION: Record<string, string> = {
  target_status: "Target status",
  target_revision: "Target revision",
  key_absent: "Key not yet in use",
  spec_revision: "Spec revision",
  eval_revision: "Eval revision",
};

export function preconditionLabel(name: string): string {
  return PRECONDITION[name] ?? humanize(name);
}

// A cancelled shelf item's resolution (store/workitem.go's Resolution*).
const RESOLUTION: Record<string, string> = {
  wontfix: "Won't fix",
  invalid: "Not valid",
  duplicate: "Duplicate",
  transferred: "Transferred",
};

export function resolutionLabel(resolution: string): string {
  return RESOLUTION[resolution] ?? humanize(resolution);
}

// A feature's change type, the four values `tiller add feature
// --change-type` accepts, worded from artifact-schema.md's own gloss: create
// writes new code, modify changes existing paths, replace rewrites a
// surface, migrate moves one from one pattern to another.
const CHANGE_TYPE: Record<string, string> = {
  create: "New code",
  modify: "Changes existing code",
  replace: "Rewrites existing code",
  migrate: "Moves code to a new pattern",
};

export function changeTypeLabel(changeType: string): string {
  return CHANGE_TYPE[changeType] ?? humanize(changeType);
}
