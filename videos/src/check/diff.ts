import { existsSync, readFileSync } from "node:fs";
import { VIDEOS_DIR } from "../script/load.js";
import type { Actor } from "../script/schema.js";
import { normalize } from "./normalize.js";

export type Step = {
  id: string;
  actor: Actor;
  // "edit" is one file written or changed: `removed` and `added` are its changed lines (for a
  // `new` file, every line is added).
  kind: "cmd" | "say" | "edit";
  cmd?: string;
  out?: string[];
  text?: string;
  exit?: number;
  file?: string;
  new?: boolean;
  removed?: string[];
  added?: string[];
};

export function readSnapshot(stage: string, dir = `${VIDEOS_DIR}snapshots`): Step[] {
  const path = `${dir}/${stage}.jsonl`;
  if (!existsSync(path)) throw new Error(`no snapshot at ${path}`);
  return readFileSync(path, "utf8")
    .split("\n")
    .filter((l) => l.trim() !== "")
    .map((l) => JSON.parse(l) as Step);
}

const norm = (s: string | undefined) => (s === undefined ? s : normalize(s));

// Human-readable differences between two runs; empty means they match.
export function diffSnapshots(committed: Step[], fresh: Step[]): string[] {
  const out: string[] = [];
  const freshById = new Map(fresh.map((s) => [s.id, s]));
  const known = new Set(committed.map((s) => s.id));
  for (const c of committed) {
    const f = freshById.get(c.id);
    if (!f) {
      out.push(`${c.id}: step is missing from the fresh run`);
      continue;
    }
    if (c.actor !== f.actor) out.push(`${c.id}: actor changed from ${c.actor} to ${f.actor}`);
    if (c.kind !== f.kind) out.push(`${c.id}: kind changed from ${c.kind} to ${f.kind}`);
    if (norm(c.cmd) !== norm(f.cmd)) out.push(`${c.id}: command changed\n  was: ${c.cmd}\n  now: ${f.cmd}`);
    if (norm(c.text) !== norm(f.text)) out.push(`${c.id}: text changed\n  was: ${c.text}\n  now: ${f.text}`);
    if (c.file !== f.file) out.push(`${c.id}: file changed from ${c.file} to ${f.file}`);
    for (const side of ["removed", "added"] as const) {
      const was = (c[side] ?? []).map(normalize);
      const now = (f[side] ?? []).map(normalize);
      if (was.length !== now.length || was.some((l, i) => l !== now[i])) out.push(`${c.id}: the ${side} lines of ${c.file} changed`);
    }
    if (c.exit !== f.exit) out.push(`${c.id}: exit changed from ${c.exit} to ${f.exit}`);
    const a = (c.out ?? []).map(normalize);
    const b = (f.out ?? []).map(normalize);
    if (a.length !== b.length || a.some((l, i) => l !== b[i])) {
      const n = Math.max(a.length, b.length);
      let i = 0;
      while (i < n && a[i] === b[i]) i++;
      out.push(`${c.id}: output changed at line ${i + 1}\n  was: ${a[i] ?? "(none)"}\n  now: ${b[i] ?? "(none)"}`);
    }
  }
  for (const f of fresh) if (!known.has(f.id)) out.push(`${f.id}: step is new in the fresh run`);
  return out;
}
