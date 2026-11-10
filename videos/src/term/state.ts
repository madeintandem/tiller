import type { Actor } from "../script/schema.js";
import type { Step } from "../check/diff.js";

// `current` marks the lines of the step in progress (the last one shown). The renderer keeps
// the head of that block (a command) in view while its output scrolls beneath it.
export type TermLine = {
  actor: Actor;
  kind: "cmd" | "out" | "say" | "edit";
  // An edit's `text` is its file path; its changed lines are in `edit`.
  text: string;
  typed: number;
  current?: true;
  edit?: EditView;
};

// What an edit shows: the lines it removed and added, cut to the first few, and how many
// added lines were left out (a new file shows its opening lines, not all of them).
export type EditView = { isNew: boolean; removed: string[]; added: string[]; more: number };
export const EDIT_MAX_LINES = 6;

export function editView(s: Step): EditView {
  const added = s.added ?? [];
  // Blank lines would spend the few lines shown on nothing.
  const shown = (s.new ? added.filter((l) => l.trim() !== "") : added).slice(0, EDIT_MAX_LINES);
  return { isNew: s.new === true, removed: (s.removed ?? []).slice(0, EDIT_MAX_LINES), added: shown, more: added.length - shown.length };
}

export const TYPE_CPS = 40;
// A command never spends more than this share of its slot typing, so output has room to land.
const MAX_TYPE_SHARE = 0.7;
// Output lines come out over this share of the time left after typing; the rest holds the whole output.
const REVEAL_SHARE = 0.85;

const weight = (s: Step): number => {
  if (s.kind === "edit") {
    const v = editView(s);
    return Math.max(1, (s.file ?? "").length + [...v.removed, ...v.added].join("").length);
  }
  const text = s.kind === "say" ? (s.text ?? "") : (s.cmd ?? "") + (s.out ?? []).join("");
  return Math.max(1, text.length);
};

// Steps share [startFrame, endFrame] in proportion to their text length. A cmd types at
// 40 chars/s (capped to its share), then reveals its output a line at a time over the rest
// of its share, so a long output scrolls while the narration speaks; a say or an edit appears whole.
// Markup (**bold**, !!fail!!, ++pass++) is kept verbatim in `text` for the renderer.
export function termState(steps: Step[], startFrame: number, endFrame: number, frame: number, fps: number): TermLine[] {
  if (frame <= startFrame || steps.length === 0) return [];
  const total = steps.reduce((n, s) => n + weight(s), 0);
  const span = endFrame - startFrame;
  const lines: TermLine[] = [];
  let at = startFrame;
  let currentFrom = 0;
  for (const s of steps) {
    const share = (span * weight(s)) / total;
    const t = (frame - at) / fps; // seconds into this step
    at += share;
    if (t <= 0) break;
    currentFrom = lines.length;
    if (s.kind === "edit") {
      const file = s.file ?? "";
      lines.push({ actor: s.actor, kind: "edit", text: file, typed: file.length, edit: editView(s) });
      continue;
    }
    if (s.kind === "say") {
      const text = s.text ?? "";
      lines.push({ actor: s.actor, kind: "say", text, typed: text.length });
      continue;
    }
    const cmd = s.cmd ?? "";
    const shareSecs = share / fps;
    const typeSecs = Math.min(cmd.length / TYPE_CPS, shareSecs * MAX_TYPE_SHARE);
    const done = typeSecs <= 0 || t >= typeSecs;
    const typed = done ? cmd.length : Math.min(cmd.length, Math.ceil((t / typeSecs) * cmd.length));
    lines.push({ actor: s.actor, kind: "cmd", text: cmd, typed });
    if (!done) continue;
    const out = s.out ?? [];
    const revealSecs = (shareSecs - typeSecs) * REVEAL_SHARE;
    const shown = revealSecs <= 0 ? out.length : Math.min(out.length, Math.max(1, Math.ceil(((t - typeSecs) / revealSecs) * out.length)));
    for (const o of out.slice(0, shown)) lines.push({ actor: s.actor, kind: "out", text: o, typed: o.length });
  }
  for (let i = currentFrom; i < lines.length; i++) lines[i].current = true;
  return lines;
}
