import type { Actor } from "../script/schema.js";
import type { Step } from "../check/diff.js";

export type TermLine = { actor: Actor; kind: "cmd" | "out" | "say"; text: string; typed: number };

export const TYPE_CPS = 40;
// A command never spends more than this share of its slot typing, so output has room to land.
const MAX_TYPE_SHARE = 0.7;

const weight = (s: Step): number => {
  const text = s.kind === "say" ? (s.text ?? "") : (s.cmd ?? "") + (s.out ?? []).join("");
  return Math.max(1, text.length);
};

// Steps share [startFrame, endFrame] in proportion to their text length. A cmd types at
// 40 chars/s (capped to its share) and then shows its output whole; a say appears whole.
// Markup (**bold**, !!fail!!, ++pass++) is kept verbatim in `text` for the renderer.
export function termState(steps: Step[], startFrame: number, endFrame: number, frame: number, fps: number): TermLine[] {
  if (frame <= startFrame || steps.length === 0) return [];
  const total = steps.reduce((n, s) => n + weight(s), 0);
  const span = endFrame - startFrame;
  const lines: TermLine[] = [];
  let at = startFrame;
  for (const s of steps) {
    const share = (span * weight(s)) / total;
    const t = (frame - at) / fps; // seconds into this step
    at += share;
    if (t <= 0) break;
    if (s.kind === "say") {
      const text = s.text ?? "";
      lines.push({ actor: s.actor, kind: "say", text, typed: text.length });
      continue;
    }
    const cmd = s.cmd ?? "";
    const typeSecs = Math.min(cmd.length / TYPE_CPS, (share / fps) * MAX_TYPE_SHARE);
    const done = typeSecs <= 0 || t >= typeSecs;
    const typed = done ? cmd.length : Math.min(cmd.length, Math.ceil((t / typeSecs) * cmd.length));
    lines.push({ actor: s.actor, kind: "cmd", text: cmd, typed });
    if (done) for (const o of s.out ?? []) lines.push({ actor: s.actor, kind: "out", text: o, typed: o.length });
  }
  return lines;
}
