// One phrase for every event type, shared by History, Home's Pulse, and the
// node History tab. The console already maps machine kinds to English in
// static tables; this is that table one level down, for the event stream's
// `type` strings. No raw type reaches the screen: a type this build has never
// seen is humanized (separators become spaces) rather than printed as minted.
//
// Resolution order: a full-type override, then verb.qualifier.noun (the
// qualifier table), then verb.noun, then the humanizing fallback. The set of
// types the code can mint is listed in vocabulary.json (event_types) and
// pinned against the lifecycle table by a Go test.

import { tokenWords } from "./statusLabel";

export interface EventPhrase {
  // The count-led subject a collapsed row carries: "features linked", which
  // the view prints after the count numeral.
  subject: string;
  // The participle alone, for a run of length one, which names its node
  // instead of a count: "Audit Engine added".
  verb: string;
}

// One participle per verb the event stream can mint. attest and eval come
// through mutate's saveEvalTarget, approve/archive/cancel/check/resume/return/
// spec/start/submit/sync/withdraw through the lifecycle table, and the rest
// directly from internal/core/mutate. accept cannot be minted now (done above
// a task is a rollup sync computes) but history still carries accept rows.
const PARTICIPLE: Record<string, string> = {
  accept: "accepted",
  add: "added",
  answer: "answered",
  approve: "approved",
  propose: "proposed",
  archive: "archived",
  attest: "attested",
  cancel: "cancelled",
  check: "checked",
  claim: "claimed",
  configure: "configured",
  // An amendment's approve mints revise.<noun> and correct-eval.feature
  // itself; correct-eval's participle names its object so a single row reads
  // "Fast logging eval corrected".
  "correct-eval": "eval corrected",
  eval: "evaluated",
  grade: "graded",
  link: "linked",
  migrate: "migrated",
  move: "moved",
  order: "reordered",
  reassign: "reassigned",
  recover_review: "review recovered",
  release: "released",
  remove: "removed",
  rename: "renamed",
  resolve: "resolved",
  resume: "resumed",
  return: "returned",
  review: "reviewed",
  revise: "revised",
  spec: "specced",
  start: "started",
  submit: "submitted",
  superseded: "superseded",
  sync: "synced",
  transfer: "transferred",
  unlink: "unlinked",
  withdraw: "withdrawn",
};

// Nouns already plural in the type itself (order.features), never pluralized
// again and the same at any count.
const PLURAL_NOUNS = new Set(["features", "tasks", "bugs", "chores", "tweaks"]);

// Irregular plurals: criterion -> criteria, the plural the decision cards'
// plan line also uses.
const PLURAL: Record<string, string> = {
  criterion: "criteria",
};

// Type-scoped participles for a verb.noun type: composing add.question gives
// "questions added", while the recap's ask story hangs on the word ask.
const TYPE_PARTICIPLE: Record<string, string> = {
  "add.question": "asked",
};

// verb.qualifier.noun types: the participle for verb.qualifier. The
// qualifier says how (a title or a key, an attachment, a proposed or draft
// node), the noun is still the last segment.
const QUALIFIER_PARTICIPLE: Record<string, string> = {
  "rename.title": "retitled",
  "rename.key": "rekeyed",
  "link.attach": "attached",
  "unlink.attach": "detached",
  "remove.proposed": "removed",
  "remove.draft": "removed",
};

// Full-phrase overrides for a type that is not plain verb.noun. Both
// surface-recording types report the noun "surface", not the grain word
// (epic/feature) their third segment carries; the grain still keeps its own
// burst because bursts group on the full type string. Roster events keep the
// legacy owner vocabulary alongside contributor.
const TYPE_PHRASE: Record<string, { noun: string; participle: string }> = {
  "link.surface.epic": { noun: "surface", participle: "recorded" },
  "link.surface.feature": { noun: "surface", participle: "recorded" },
  "unlink.surface.epic": { noun: "surface", participle: "removed" },
  "unlink.surface.feature": { noun: "surface", participle: "removed" },
  "owner.add": { noun: "owner", participle: "added" },
  "owner.remove": { noun: "owner", participle: "removed" },
  "contributor.add": { noun: "contributor", participle: "added" },
  "contributor.remove": { noun: "contributor", participle: "removed" },
};

// Whole-phrase overrides: both slots carry the same fixed words.
const FIXED_PHRASE: Record<string, string> = {
  amendment_opened: "amendment opened",
  amendment_staged: "amendment staged",
  amendment_approved: "amendment approved",
  amendment_proposed: "amendment proposed",
  amendment_withdrawn: "amendment withdrawn",
  design_intake: "design landed",
  superseded: "superseded",
  "migrate.schema": "schema migrated",
};

export function eventPhrase(type: string, count: number): EventPhrase {
  const fixed = FIXED_PHRASE[type];
  if (fixed !== undefined) {
    return { subject: fixed, verb: fixed };
  }
  const override = TYPE_PHRASE[type];
  if (override !== undefined) {
    return { subject: `${pluralize(override.noun, count)} ${override.participle}`, verb: override.participle };
  }
  const parts = type.split(".");
  if (parts.length === 3 && parts.every((p) => p !== "")) {
    const participle = QUALIFIER_PARTICIPLE[`${parts[0]}.${parts[1]}`];
    if (participle !== undefined) {
      return { subject: `${pluralize(parts[2], count)} ${participle}`, verb: participle };
    }
  }
  if (parts.length === 2 && parts[0] !== "" && parts[1] !== "") {
    const [verb, noun] = parts;
    const participle = TYPE_PARTICIPLE[type] ?? PARTICIPLE[verb];
    if (participle !== undefined) {
      return { subject: `${pluralize(noun, count)} ${participle}`, verb: participle };
    }
    return { subject: `${pluralize(noun, count)} ${tokenWords(verb)}`, verb: tokenWords(verb) };
  }
  const words = tokenWords(type);
  return { subject: words, verb: words };
}

// pluralize also words the noun (statusLabel.ts's tokenWords), so a noun
// this build has never seen ("work_item") reads "work items", never as
// minted. It never guesses at a participle.
function pluralize(noun: string, count: number): string {
  if (count === 1 || PLURAL_NOUNS.has(noun)) {
    return tokenWords(noun);
  }
  return tokenWords(PLURAL[noun] ?? `${noun}s`);
}
