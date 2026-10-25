// Burst collapsing, the recap view's answer to a wall of identical rows.
//
// The measurement that motivates it, taken from the live Acme tree's 7d
// window: 167 events, one owner, two minutes, and four bursts. 10 add.epic and
// 64 add.feature at 21:37, 32 add.question at 21:37, 61 link.feature at 21:42.
// A row-per-event rail therefore printed the event type, the timestamp and the
// owner 167 times each to carry 167 subjects. Those three columns are not
// noise that happens to repeat, they are the burst's own identity, so
// collapsing a run promotes them from repetition into the row's subject rather
// than hiding anything. What is collapsed stays reachable: the view expands a
// burst in place, DecisionCard's grammar.
//
// Two rules keep the collapse honest, and both were chosen against the data
// rather than tuned.
//
// CONSECUTIVE, not "all matching". A run is broken by any intervening event of
// a different type, so a lane whose activity genuinely interleaves keeps its
// interleaving instead of being re-sorted into piles. This is only sound
// because the payload is one newest-first timeline (internal/views/recap.go),
// which is what the same pass fixed there.
//
// NO TIME TOLERANCE. A tolerance constant would be a magic number with no
// measurement behind it, and the payload is already time-ordered, so adjacency
// in the list IS adjacency in time. What replaces it is a display rule that
// cannot lie: burstWhen states one timestamp when the run's ends render
// identically at the view's own minute precision, and "earliest to latest"
// when they do not. The precision decides, not a constant.

import type { RecapEntry } from "../api";

// A run of consecutive entries sharing one (type, contributor). The actor is in the key
// even though a group is already one (epic, stream) cell and its entries
// therefore share a contributor by construction: the key states the rule the
// grammar depends on rather than leaning on a coincidence upstream.
export interface Burst {
  type: string;
  contributor: string;
  entries: RecapEntry[];
  // How many of the run's entries created an ask for the reader. The one thing
  // on this page that needs the human, so it is the one thing rendered loud.
  asks: number;
}

export function burstsOf(entries: RecapEntry[]): Burst[] {
  const out: Burst[] = [];
  for (const entry of entries) {
    const open = out.length > 0 ? out[out.length - 1] : undefined;
    if (open !== undefined && open.type === entry.type && open.contributor === entry.contributor) {
      open.entries.push(entry);
      if (entry.ask) {
        open.asks += 1;
      }
      continue;
    }
    out.push({
      type: entry.type,
      contributor: entry.contributor,
      entries: [entry],
      asks: entry.ask ? 1 : 0,
    });
  }
  return out;
}

// An event type is verb.noun, built as such at the one place events are minted
// (internal/core/mutate, `in.Verb + "." + noun`), so the row's phrase is a
// lexical transform of a token the row already printed rather than narration:
// plural(noun, n) followed by participle(verb). ui-surface.md's "mechanical,
// never narrative" bars inventing sentences the read model does not carry, and
// the console already maps machine kinds to English in a static table
// (DecisionCard's KIND_TITLE). Every map below is that table one level down,
// with KIND_TITLE's own `?? kind` fallback discipline: an unknown token
// renders raw rather than being guessed at.
//
// link.surface.epic / link.surface.feature are the one exception to
// verb.noun (internal/core/mutate/surface.go, SetSurface): --depends-on and
// --surface both mint through tiller link, and for one round they shared
// link.epic/link.feature on the reasoning that they are the same command's
// output. That reasoning did not survive contact with this file: two facts
// under one type collapsed into one burst and one phrase, "features linked"
// for a call that only recorded a surface. TYPE_PHRASE below is checked
// before the verb.noun split even runs, so it does not disturb the generic
// path or the "not verb.noun renders raw" fallback (weird.type.thing, a
// three-segment type nobody has sanctioned, still renders raw).

// The participles, one per verb the event stream can mint, and the map is only
// worth having if it is exhaustive: the fallback below prints the raw verb, and
// a raw verb reads as broken English on a row a human is meant to scan. There
// are THREE minting paths, not two, which is how attest and eval were missed
// on the first pass and rendered as "features attest" and "Feature One eval" on
// a real tree: add and answer and link mint their type directly in
// internal/core/mutate; approve, archive, cancel, check, resume, return, spec,
// start, submit, sync and withdraw come through commitTransition from the
// lifecycle table; and attest and eval come through that SAME commitTransition
// from mutate's saveEvalTarget, which passes its own verb rather than a
// lifecycle one. claim and release never reach here, the recap filters them as
// churn. Anything unlisted renders its verb raw.
const PARTICIPLE: Record<string, string> = {
  // accept is NOT in the list above and cannot be minted any more: done above a
  // task is a rollup sync computes and the verb left the lifecycle table with it
  // (artifact-schema.md, "Done above a task is computed, and accept is gone").
  // The entry stays because the recap reads HISTORY, and a tree that predates
  // the change still carries accept.epic rows whose English should not degrade
  // to "epic accept" now that the verb is gone.
  accept: "accepted",
  add: "added",
  answer: "answered",
  approve: "approved",
  archive: "archived",
  attest: "attested",
  cancel: "cancelled",
  check: "checked",
  // An amendment's approve mints revise.<noun> and correct-eval.feature itself
  // (internal/core/mutate/amendment_approve.go). correct-eval's participle
  // names its object so a single row reads "Fast logging eval corrected".
  "correct-eval": "eval corrected",
  eval: "evaluated",
  link: "linked",
  unlink: "unlinked",
  resume: "resumed",
  return: "returned",
  revise: "revised",
  spec: "specced",
  start: "started",
  submit: "submitted",
  sync: "synced",
  withdraw: "withdrawn",
};

// The irregular plurals. criterion -> criteria is lifted from
// DecisionCard.contextLine, which had to solve the same problem for the same
// vocabulary. No noun the store mints today pluralizes irregularly, so this
// map is carried defensively rather than exercised by a live event type.
const PLURAL: Record<string, string> = {
  criterion: "criteria",
};

// Exactly one type-scoped override for a verb.noun type, and it earns itself:
// composing add.question gives "questions added", while the recap's entire
// ask story hangs on the word ask, and the page head and the burst rows both
// say "asked you". Everything else in verb.noun shape composes.
const TYPE_PARTICIPLE: Record<string, string> = {
  "add.question": "asked",
};

// Full-phrase overrides for a type that is NOT verb.noun, checked ahead of
// the split in burstPhrase so they never touch the generic path. Both
// surface-recording types report the noun "surface", not the grain word
// (epic/feature) their own third segment carries: what a reader scans for is
// that a surface got recorded, and the grain still keeps its own burst
// without needing its own word in the phrase, because burstsOf groups on the
// full type string and link.surface.epic still differs from
// link.surface.feature there.
// Roster events use person.action order; retain the legacy owner's vocabulary.
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

export interface BurstPhrase {
  // The count-led subject a collapsed row carries: "features linked", which
  // the view prints after the count numeral.
  subject: string;
  // The participle alone, for the run of length one, which names its node
  // instead of a count: "Audit Engine added".
  verb: string;
}

export function burstPhrase(type: string, count: number): BurstPhrase {
  const override = TYPE_PHRASE[type];
  if (override !== undefined) {
    return { subject: `${pluralize(override.noun, count)} ${override.participle}`, verb: override.participle };
  }
  const parts = type.split(".");
  // A type that is not exactly verb.noun (a bare "sync", or something a later
  // build mints in a shape this one has never seen) renders raw in both slots.
  // Printing the token the payload actually carries beats guessing at it.
  if (parts.length !== 2 || parts[0] === "" || parts[1] === "") {
    return { subject: type, verb: type };
  }
  const [verb, noun] = parts;
  const participle = TYPE_PARTICIPLE[type] ?? PARTICIPLE[verb] ?? verb;
  return { subject: `${pluralize(noun, count)} ${participle}`, verb: participle };
}

function pluralize(noun: string, count: number): string {
  if (count === 1) {
    return noun;
  }
  return PLURAL[noun] ?? `${noun}s`;
}

// burstWhen is the run's time, stated at the view's own precision. One stamp
// when the ends render identically there, "earliest to latest" when they do
// not, so the row can never claim a span happened at an instant. Verified on
// the live tree: add.feature spans 21:37:02 to 21:37:04 and reads as one
// stamp, while a week of done.task in one lane would read "Jul 20, 9:12 AM to
// Jul 25, 4:40 PM", which is the correct thing to say about it.
//
// entries arrive newest first, so the head is the latest and the tail the
// earliest. format is injected because it is the caller's display precision
// that decides, and the two have to be the same function or the rule is a lie.
export function burstWhen(entries: RecapEntry[], format: (iso: string) => string): string {
  if (entries.length === 0) {
    return "";
  }
  const latest = format(entries[0].at);
  const earliest = format(entries[entries.length - 1].at);
  return latest === earliest ? latest : `${earliest} to ${latest}`;
}

// leafKey drops the owning group's key prefix from a node's key, so a row
// under the form-engine head reads "audit-overlay-rendering" rather than
// "form-engine/audit-overlay-rendering". This is the mock's own decision at
// mocks.html:304, where a decision row prints the leaf and lets its container
// carry the path. A key that IS the group's key (the epic's own add.epic row)
// collapses to nothing rather than repeating the head one line below it.
export function leafKey(key: string, groupKey: string): string {
  if (key === "" || key === groupKey) {
    return "";
  }
  if (groupKey !== "" && key.startsWith(`${groupKey}/`)) {
    return key.slice(groupKey.length + 1);
  }
  return key;
}
