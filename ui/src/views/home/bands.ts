// The desk's burst compression (home burst redesign, settled decisions 2-4):
// a kind with more than ROW_CAP entries folds its repeated chrome into one
// band, unless its single-node command cannot be safely dropped from the
// desk. Amendment approval accepts one address and lives only on its queue
// card; a review gate or eval criterion attestation names one node and one
// gate or criterion and has no batch verb; a workflow closeout carries its
// own phase command. Those kinds always remain individual command cards. Every kind is its own
// run and column; adjacent small kinds never pool into one grid. The batch
// foot exists only where cli-surface.md has a multi-node approve verb.
// approve_tasks is absent because --feature excludes positionals, and
// approve_amendment is absent because the CLI accepts exactly one address.
import type { QueueEntry } from "../../api";
import { whyLine } from "../../components/DecisionCard";
import { addrOf } from "../../lib/addr";
import { ROW_CAP } from "../../lib/disclosure";
import type { QueueCardGroup } from "./queueGroups";

export type BandRun =
  | { type: "cards"; kind: string; entries: QueueEntry[] }
  | { type: "band"; group: QueueCardGroup };

const NEVER_COMPRESSED = new Set(["approve_amendment", "attest_gate", "attest_criterion", "workflow_closeout"]);

export function bandRuns(groups: QueueCardGroup[]): BandRun[] {
  return groups.map((group) =>
    !NEVER_COMPRESSED.has(group.kind) && group.entries.length > ROW_CAP
      ? { type: "band", group }
      : { type: "cards", kind: group.kind, entries: [...group.entries] },
  );
}

const BATCH_NOUN: Record<string, string> = {
  approve_epic: "epic",
  approve_feature: "feature",
  approve_bug: "bug",
  approve_chore: "chore",
  approve_tweak: "tweak",
};

export function batchCommand(kind: string, entries: QueueEntry[]): string | null {
  const noun = BATCH_NOUN[kind];
  if (noun === undefined || entries.length < 2) return null;
  return `tiller approve ${noun} ${entries.map((e) => addrOf(e)).join(" ")}`;
}

// queueEntryKey is the React key every rendered queue entry uses, in the flat
// card grid (Home.tsx) and inside a band's rows (DecisionBand.tsx) alike.
// entry.id alone would collide for attest_criterion, whose id names the
// FEATURE rather than the criterion, so several criteria on one feature share
// it; the gate or prose that distinguishes them rides in the key. One
// definition, because two copies of a key rule can silently drift into two
// different collision behaviors.
export function queueEntryKey(entry: QueueEntry): string {
  return `${entry.kind}:${entry.id}:${entry.gate ?? entry.prose ?? ""}`;
}

export function sharedWhy(entries: QueueEntry[]): string {
  const first = entries.length > 0 ? whyLine(entries[0]) : "";
  if (first === "") return "";
  return entries.every((e) => whyLine(e) === first) ? first : "";
}
