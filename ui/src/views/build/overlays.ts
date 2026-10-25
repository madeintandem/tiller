// The board's two condition overlays (task-13-brief.md item 4), each a join
// by node id against a payload /api/build deliberately excludes: the queue,
// for the ask flag (server-side reasoning: BuildFeature carries no queue
// data, the same reason StreamsOf carries none for /api/board), and
// /api/drift's candidates, for the neutral drift flag (task-12-report.md's
// own call: a drift boolean on the projection would mean running the git-
// plus-stat sweep on every /api/build request, so the client joins instead,
// the identical overlay idiom Home already runs for its own queue and drift
// reads). Both joins are keyed on the bare node id and scoped to noun ===
// "feature", since a board card is always a feature - a queue entry or drift
// candidate naming a task or an epic has no card here to land on.

import { isAttestGate } from "../../api";
import type { DriftReportView, Queue, QueueEntry } from "../../api";
import { has } from "../../lib/has";

// askFlagLabel is the small ask flag's own word, derived from the queue
// entry's own kind (and, where the kind alone is ambiguous, the field that
// disambiguates it - the mock's own "attest C3" reads that way because the
// underlying entry is an attest_criterion whose Prose IS the criterion id,
// DecisionCard.tsx's own doc comment on why that one kind cannot lead with
// a title). Never a guess at a word the entry does not carry.
export function askFlagLabel(entry: QueueEntry): string {
  switch (entry.kind) {
    case "approve_amendment":
    case "approve_epic":
    case "approve_feature":
    case "approve_tasks":
      return "approve";
    case "answer_question":
      return "answer";
    case "attest_gate":
      return isAttestGate(entry) ? `attest ${entry.gate}` : "attest";
    case "attest_criterion":
      return has(entry.prose) ? `attest ${entry.prose}` : "attest";
    case "feature_check":
      return "check";
    case "workflow_closeout":
      return `${entry.actor ?? "team"} · ${entry.phase?.replaceAll("_", " ") ?? "closeout"}`;
    default:
      return "ask";
  }
}

// askLabelsFrom maps every feature-noun queue entry onto its flag label, one
// per feature id. A feature that queues more than one entry keeps the
// FIRST one's label, in served order (queue.go's own build order), rather
// than the last. This is not "no ranking rule exists" (Home's gravity very
// much ranks kinds, queueGroups.ts's groupQueueByGravity, attest ahead of
// approve ahead of answer); it is that gravity re-ranks KIND buckets only
// and leaves a kind's own members in server order (that file's own
// contract), so within one kind this pick and Home's are the identical
// entry either way. The two surfaces can only actually diverge when ONE
// feature queues TWO DIFFERENT kinds at once - gravity would rank one
// ahead of the other, and this flag still just takes whichever queue.go
// listed first - which is the one case a single small flag cannot resolve
// without becoming a ranked stack of its own, the same call the mock's
// one-word flag already made.
export function askLabelsFrom(queue: Queue | null): Map<string, string> {
  const labels = new Map<string, string>();
  if (queue === null) {
    return labels;
  }
  for (const entry of queue.entries) {
    if (entry.noun !== "feature") {
      continue;
    }
    if (!labels.has(entry.id)) {
      labels.set(entry.id, askFlagLabel(entry));
    }
  }
  return labels;
}

// driftIdsFrom collects every feature-noun candidate id out of the drift
// report's per-marker groups, flat, since the card's own drift flag only
// ever asks "is this feature a candidate at all", not which marker or how
// many.
export function driftIdsFrom(report: DriftReportView | null): Set<string> {
  const ids = new Set<string>();
  if (report === null || report.candidates === undefined) {
    return ids;
  }
  for (const group of Object.values(report.candidates)) {
    for (const candidate of group) {
      if (candidate.noun === "feature") {
        ids.add(candidate.id);
      }
    }
  }
  return ids;
}
