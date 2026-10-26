// needsAttentionFallback reconstructs the pre-task-17 single-trigger
// attention derivation (lib/attention.ts's attentionAction, deleted at
// d5c5e13 when the queue join became the node page's primary attention
// source: `git show d5c5e13^:ui/src/lib/attention.ts` has the deleted file
// this reconstructs, verbatim in logic - only the export name and this
// file's own framing changed).
//
// The queue join alone does
// not cover every state a plain human next-owner can sit at. The two states
// it DOES cover in the ordinary case - a needs_attestation criterion on a
// required gate (attest_criterion), and a pre-work approval (approve_epic /
// approve_feature / approve_tasks) - this function recomputes too, as
// defense in depth, never as the primary path for them. The one gap the
// queue leaves genuinely silent is the old strip's own third tier: a plain
// human next-owner naming no specific verb the queue tracks at all - a
// ready epic, whose only legal verb is cancel; a done epic, whose only
// legal verb is archive, left on the CLI for v1. AttentionStack only calls
// this when the node's own queue entries are empty (any kind, including
// ones the stack itself chooses not to render), so this derivation and the
// queue join can never both fire for the same node and disagree.
//
// Settle questions (item 1): approve feature and approve task
// refuse while an open question holds the plan, and approve_tasks rows are
// keyed to the feature, so a task page always reaches this fallback. The
// recomputed approve is therefore never offered for a feature (the queue's
// approve_feature row is the only honest source), nor for a task the page
// knows a question holds, which names that question's answer instead.

import type { GateView, NodeView, Relationships } from "../api";
import { addrOf } from "./addr";

export interface AttentionFallback {
  label: string;
  command: string;
}

interface PendingAttestation {
  gate: string;
  criterion: string;
}

// pendingAttestations scans every required gate's criteria map (only the
// evals gate carries one today - store's own schema comment says so - but
// nothing here hardcodes that name) for a needs_attestation result, sorted by
// gate then criterion id so the "first" one this fallback names is
// deterministic rather than riding whatever order the JSON happened to parse
// into.
function pendingAttestations(gates: Record<string, GateView> | undefined): PendingAttestation[] {
  const record = gates ?? {};
  const out: PendingAttestation[] = [];
  for (const gateName of Object.keys(record).sort()) {
    const gate = record[gateName];
    if (!gate.required) {
      continue;
    }
    const criteria = gate.criteria ?? {};
    for (const criterionId of Object.keys(criteria).sort()) {
      if (criteria[criterionId].status === "needs_attestation") {
        out.push({ gate: gateName, criterion: criterionId });
      }
    }
  }
  return out;
}

const PRE_WORK_STATUSES = new Set(["draft", "specced", "proposed"]);

export function needsAttentionFallback(
  node: NodeView,
  relationships?: Relationships,
): AttentionFallback | null {
  const pending = pendingAttestations(node.gates);
  if (pending.length > 0) {
    const first = pending[0];
    // COMMAND GRAMMAR: byte-parity with the server's own attest_criterion
    // queue entry, internal/views/queue.go queueAttestCriteria (the
    // canonical source; mirror it exactly, never re-derive). A
    // criterion-level attest is `--criterion <id> --status pass`, NEVER
    // combined with --gate.
    return {
      label: "Awaiting your attestation",
      command: `tiller attest ${node.noun} ${addrOf(node)} --criterion ${first.criterion} --status pass`,
    };
  }

  if (node.next_owner !== "human") {
    return null;
  }

  if (PRE_WORK_STATUSES.has(node.status)) {
    // A plan gate refuses while an open question holds the plan,
    // so a feature or task the page knows is held names the question's answer
    // rather than an approve that exits 1 when copied. blocked_by_questions is
    // the node's own blocker set, ancestors included (views.blockingQuestions
    // over gen's blocker predicates), sorted by ref.
    const question = relationships?.blocked_by_questions?.[0];
    if ((node.noun === "feature" || node.noun === "task") && question !== undefined) {
      return {
        label: `Awaiting your answer · ${question.title ? `${question.title} (${question.id})` : question.id}`,
        command: `tiller answer question ${question.id} --answer "..."`,
      };
    }
    // The queue owns a specced feature's approve_feature row and hides it
    // exactly when the approve would refuse, including for a question on one
    // of its tasks, which the feature's own blocker set does not carry. A
    // queue silent about a specced feature is that hold, so the fallback
    // never offers the approve. A node under a retired ancestor (held) is
    // refused the same way (mutate.guardParentState), so it never does either.
    if (node.noun !== "feature" && node.held === undefined) {
      return {
        label: "Awaiting your approval",
        command: `tiller approve ${node.noun} ${addrOf(node)}`,
      };
    }
  }

  return {
    label: "Needs your attention next",
    command: `tiller show ${node.noun} ${addrOf(node)}`,
  };
}
