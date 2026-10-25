// The desk's card grouping (task C1): /api/queue serves entries kind by kind
// in views/queue.go's own fixed build order, but the mock's desk displays
// gravity order instead - amendment, attest, approval, answer, parked work,
// then merge asks (docs/plans/2026-07-23-console-chart-table-mocks.html,
// round 3 notes: "Gravity order for multiple cards").
// groupQueueByGravity re-ranks the KIND
// buckets only; the members within one kind stay in the exact order the API
// served them (issue #27's dependency order for the homogeneous-noun kinds,
// due-first for questions), since that is the order a batch command must join
// them in.
//
// approve_amendment leads every other kind, ahead of attest, because it leads
// the server's own queue for the same reason it leads Route: a staged reshape
// can supersede or cancel the very node another row is asking about
// (ui-surface.md, "it leads the queue rather than sorting in among the
// approve gates"). It was missing from this table until task-10's rebuild
// consumed groupQueueByGravity's output directly for Home's card order (task
// 6 added the kind to the queue and to DecisionCard's own KIND_TITLE but
// never to this one), which sorted it dead last under the unrecognized-kind
// fallback below - the exact opposite of the settled rule.
//
// There is no acceptance rank between attest and approve because there is no
// acceptance kind: epic done is a rollup `sync` records rather than a gate
// anyone waits on (artifact-schema.md, Epics States), so nothing arrives here
// to rank. An unrecognized kind still sorts last rather than being dropped,
// which is what keeps a console older than its server honest.
//
// feature_check stays explicitly ranked rather than falling through to the
// unrecognized-kind default. Merge asks sit after it as the latest-stage
// actions; giving them a shared rank preserves the server's order between
// epic- and feature-grain asks. Unknown kinds still follow every known rank.

import type { QueueEntry } from "../../api";

export interface QueueCardGroup {
  kind: string;
  entries: QueueEntry[];
}

const KIND_GRAVITY: Record<string, number> = {
  approve_amendment: 0,
  attest_gate: 1,
  attest_criterion: 2,
  approve_epic: 3,
  approve_feature: 4,
  approve_tasks: 5,
  approve_bug: 6,
  approve_chore: 7,
  approve_tweak: 8,
  answer_question: 9,
  carrier_cancelled: 10,
  feature_check: 11,
  workflow_closeout: 11,
  merge_epic: 12,
  merge_feature: 12,
};

export function groupQueueByGravity(entries: QueueEntry[]): QueueCardGroup[] {
  const byKind = new Map<string, QueueEntry[]>();
  const kindsSeen: string[] = [];
  for (const entry of entries) {
    let bucket = byKind.get(entry.kind);
    if (bucket === undefined) {
      bucket = [];
      byKind.set(entry.kind, bucket);
      kindsSeen.push(entry.kind);
    }
    bucket.push(entry);
  }
  kindsSeen.sort((a, b) => (KIND_GRAVITY[a] ?? 99) - (KIND_GRAVITY[b] ?? 99));
  return kindsSeen.map((kind) => ({ kind, entries: byKind.get(kind) ?? [] }));
}
