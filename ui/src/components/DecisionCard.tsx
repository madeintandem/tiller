// DecisionCard is the desk's needs-you card, rebuilt to the console v2 mock's
// anatomy: one QUEUE ENTRY per card, its command
// always visible - the brief's own words, "every command rendered" - rather
// than the earlier pass's one-kind-per-card batch with a click-to-reveal
// disclosure. Home supplies cards already in gravity order (queueGroups.ts's
// groupQueueByGravity, flattened); this component renders one entry and
// stops, so nothing here re-sorts or re-groups what it is handed.
//
// kind, who, and why are all derived from QueueEntry alone, and every clause
// is a fact the entry actually carries rather than the mock's own
// illustrative prose. The card leads with the node's title, which the entry
// carries on every kind whose node has one (QueueEntry.title; rowTitle
// below), and falls back to the node's reference when there is none. On
// attest_criterion, QueueEntry.Prose is the criterion id, never a title, so
// that kind says which criterion in a trailing qualifier instead.

import { Link } from "react-router-dom";

import type { QueueEntry, QueuePlan } from "../api";
import { isAttestGate } from "../api";
import { has } from "../lib/has";
import { statusLabel } from "../lib/statusLabel";
import CopyCmd from "./CopyCmd";
import RefLink from "./RefLink";
import { ProseInline } from "./Prose";

// The mock's own four examples, verbatim ("Attest · eval criterion",
// "Approve · feature plan", "Answer · question", "Run · parked feature
// check"); the four kinds it does not show extend the same verb-then-noun
// grammar. Exported (task 17, console v2 node page rebuild): the node page's
// own attention stack renders the SAME queue entries this card does, filtered
// to one node, and reuses this map plus whyLine below rather than a second
// hand-rolled copy of "what does attest_criterion mean in words" that could
// drift from this one. rowQualifier is exported too (task 6, home burst
// redesign): its own "Criterion <id>" composes with WhoLine's leading title
// here, a sentence the node page's own box never builds (NodeDetail.tsx's
// askKindQualifier is its own, narrower version, the bare id alone), and it
// is exported now because the burst band's own rows compose the identical
// qualifier rather than a second hand-rolled copy of this one.
export const KIND_LABEL: Record<string, string> = {
  approve_amendment: "Approve · amendment",
  approve_epic: "Approve · epic decomposition",
  approve_feature: "Approve · feature plan",
  approve_tasks: "Approve · added tasks",
  approve_bug: "Approve · bug triage",
  approve_chore: "Approve · chore triage",
  approve_tweak: "Approve · tweak triage",
  answer_question: "Answer · question",
  attest_gate: "Attest · review gate",
  attest_criterion: "Attest · eval criterion",
  feature_check: "Run · parked feature check",
  workflow_closeout: "Shared closeout",
  carrier_cancelled: "Resolve · cancelled carrier",
  // The Review Grain build's own coarse-grain groups (views/queue.go's
  // queueMergeFeatures/queueMergeEpics): a reviewed feature or epic with its
  // PR/review_ref already recorded and not yet done, the merge itself the
  // one thing left. Command is always `tiller sync`, the one command that
  // observes any merge, coarse or task-grain alike.
  merge_feature: "Merge · feature PR",
  merge_epic: "Merge · epic PR",
};

// kindLabel is a queue kind's eyebrow words: KIND_LABEL's, or for a kind
// this build does not name, the token worded in sentence case (statusLabel's
// fallback), never printed as minted.
export function kindLabel(kind: string): string {
  return KIND_LABEL[kind] ?? statusLabel(kind);
}

export default function DecisionCard({ entry, tabbed = false }: { entry: QueueEntry; tabbed?: boolean }) {
  const why = whyLine(entry);
  const human = entry.actor !== "agent";
  // Under a Needs You tab the tab already names the kind, so the eyebrow
  // survives only to say the one thing the tab does not: an agent holds this.
  const eyebrow = tabbed
    ? human ? "" : "Agent"
    : `${kindLabel(entry.kind)}${entry.actor ? ` · ${entry.actor}` : ""}`;
  const plan = entry.plan !== undefined ? planLine(entry.plan) : "";
  return (
    <div className={`dcard${human ? "" : " warn"}`}>
      {eyebrow !== "" && <span className="kind">{eyebrow}</span>}
      <WhoLine entry={entry} />
      {entry.kind === "attest_criterion" && has(entry.statement) && (
        <Link className="dcard-statement" to={criterionLink(entry)}>
          {entry.statement}
        </Link>
      )}
      {plan !== "" && <p className="dcard-plan">{plan}</p>}
      {/* whyLine's own default case answers "" for a kind neither it nor
          KIND_LABEL names: an empty
          .why paragraph still painted its own margin/line-height, a blank
          beat between who and the command. Dropped rather than rendered
          empty; NodeDetail.tsx's AskBox carries the identical guard. */}
      {why !== "" && <p className="why"><ProseInline markdown={why} /></p>}
      {has(entry.command) && <CopyCmd cmd={entry.command} accent={human} />}
    </div>
  );
}

// criterionLink is the node page's evals tab with the one criterion marked
// (CriterionRow reads the criterion param and scrolls itself into view).
function criterionLink(entry: QueueEntry): string {
  const id = entry.criterion ?? entry.prose ?? "";
  return `/node/${entry.noun}/${entry.id}?tab=evals&criterion=${encodeURIComponent(id)}`;
}

function count(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

// planLine is the size of the plan an approve_feature card asks the human to
// approve; a part that is zero is left out, and an all-zero plan says nothing.
export function planLine(plan: QueuePlan): string {
  const parts: string[] = [];
  if (plan.tasks > 0) parts.push(count(plan.tasks, "task", "tasks"));
  if (plan.criteria > 0) parts.push(count(plan.criteria, "criterion", "criteria"));
  if (plan.dependencies > 0) parts.push(count(plan.dependencies, "dependency", "dependencies"));
  return parts.join(", ");
}

// rowTitle is the title a card or band row leads with: the node's own title
// where the entry carries one. Prose stands in only for a kind whose prose IS
// a title; attest_criterion's prose is the criterion id and a closeout's is a
// sentence, so those lead with the node's reference instead.
export function rowTitle(entry: QueueEntry): string | undefined {
  if (has(entry.title)) {
    return entry.title;
  }
  return entry.kind === "attest_criterion" || entry.kind === "workflow_closeout" ? undefined : entry.prose;
}

// WhoLine names the node the entry is about, and it is the card's one path
// to that node's own page (the earlier disclosure
// row linked its title to /node/{noun}/{id}, and the rebuild dropped it -
// ui-surface.md:59's "Each row opens the in-context review... with the
// action in reach" still holds for a card that never collapses). RefLink
// carries QueueEntry's own noun/id/ref straight through (structural typing;
// QueueEntry already satisfies RefLink's RefTarget), so the same
// dangling-edge guard every other reference-bearing row on the console
// shares applies here too, unmodified.
//
// A question leads with its bounded title and keeps its bare id beside it as
// the keyless noun's address. Every other kind leads with its title where it
// has one and lets its key follow in muted mono. Two kinds need a trailing
// qualifier because the card renders every row now, with no expand step left
// to reveal it: a node can queue up to four attest_gate rows, one per optional
// review gate, or several attest_criterion rows, and without the gate name or
// criterion id each card would name the same node identically.
//
// The clamp remains useful for any unusually long explicit title. It is a
// paint bound only: RefLink still carries the complete title to the node page.
function WhoLine({ entry }: { entry: QueueEntry }) {
  if (entry.noun === "question") {
    return (
      <div className="who question-who">
        <RefLink
          node={entry}
          label={has(entry.prose) ? entry.prose : entry.ref}
          className="question-title clamp"
        />{" "}
        <span className="mono">{entry.id}</span>
      </div>
    );
  }
  const title = rowTitle(entry);
  const qualifier = rowQualifier(entry);
  return (
    <div className="who clamp">
      <RefLink node={entry} label={has(title) ? title : entry.ref} className="" />
      {has(title) && has(entry.key) && <span className="mono"> {entry.key}</span>}
      {qualifier !== "" && <span className="mono muted"> · {qualifier}</span>}
    </div>
  );
}

export function rowQualifier(entry: QueueEntry): string {
  if (isAttestGate(entry)) {
    return entry.gate;
  }
  if (entry.kind === "attest_criterion") {
    const id = entry.criterion ?? entry.prose;
    return has(id) ? `Criterion ${id}` : "";
  }
  if (entry.kind === "workflow_closeout") {
    return entry.phase?.replaceAll("_", " ") ?? "";
  }
  return "";
}

// whyLine is the card's one context sentence, mechanically phrased per kind
// from fields the entry itself carries (QueueEntry's own doc comment plus
// queue.go's per-kind builders): never a guess at a fact the payload does not
// carry (the mock's "blocks intake/eligibility-rules" clause on its
// answer_question example needs a target QueueEntry has no field for -
// that lives on QuestionItem, the board's own shape, not the queue's).
export function whyLine(entry: QueueEntry): string {
  switch (entry.kind) {
    case "workflow_closeout":
      return entry.prose ?? "";
    case "attest_criterion":
      return has(entry.statement) ? "" : "This criterion needs your attestation.";
    case "attest_gate":
      return isAttestGate(entry)
        ? `Optional review gate, recorded ${statusLabel(entry.gate_status).toLowerCase()}, awaiting your attestation.`
        : "Optional review gate awaiting your attestation.";
    case "approve_epic":
      return "Decomposition proposed and ready for your review.";
    case "approve_feature":
      return "Spec written and waiting on your approval.";
    case "approve_tasks":
      return "Task breakdown ready for your approval.";
    case "approve_amendment":
      return "Amendment staged and awaiting your approval. A staged reshape can supersede or cancel what the rest of the queue is asking about.";
    case "approve_bug":
    case "approve_chore":
    case "approve_tweak":
      return "Captured work is waiting on your triage.";
    case "answer_question":
      return has(entry.due) ? `Due ${entry.due}.` : "Open, no due date recorded.";
    case "feature_check":
      return "Every task done; the required feature gate has no recorded result. The rollup is parked until this runs.";
    case "carrier_cancelled":
      return "The recorded carrier was cancelled; clear it or choose a replacement.";
    case "merge_feature":
      return "The feature PR is open; merging it is the done crossing.";
    case "merge_epic":
      return "The epic PR is open; merging it is the done crossing.";
    default:
      return "";
  }
}
