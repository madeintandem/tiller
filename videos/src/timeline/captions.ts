import type { Word } from "../narration/words.js";
import type { Timeline } from "./build.js";

export const LINE_CHARS = 42;
export const MAX_LINES = 2;
export const MAX_CUE_SECONDS = 6;

type Cue = { start: number; end: number; lines: string[] };

// Greedy line fill: append the word to the last line, or open a new line when it would pass 42.
function place(lines: string[], text: string): string[] {
  if (lines.length === 0) return [text];
  const last = lines[lines.length - 1];
  if (last.length + 1 + text.length <= LINE_CHARS) return [...lines.slice(0, -1), `${last} ${text}`];
  return [...lines, text];
}

// One clip's cues; a clip is the intro or one beat, so a cue never crosses a beat boundary.
function clipCues(words: Word[], offset: number): Cue[] {
  const cues: Cue[] = [];
  let cur: Cue | undefined;
  for (const w of words) {
    const start = offset + w.start;
    const end = offset + w.end;
    if (cur) {
      const lines = place(cur.lines, w.text);
      if (lines.length <= MAX_LINES && end - cur.start <= MAX_CUE_SECONDS) {
        cur.lines = lines;
        cur.end = end;
        continue;
      }
    }
    cur = { start, end, lines: [w.text] };
    cues.push(cur);
  }
  return cues;
}

const stamp = (t: number): string => {
  const ms = Math.max(0, Math.round(t * 1000));
  const pad = (n: number, w = 2) => String(n).padStart(w, "0");
  return `${pad(Math.floor(ms / 3_600_000))}:${pad(Math.floor(ms / 60_000) % 60)}:${pad(Math.floor(ms / 1000) % 60)},${pad(ms % 1000, 3)}`;
};

// SubRip captions from the spoken words only: the intro transcript, then each beat's say.
export function toSrt(timeline: Timeline): string {
  const cues = [
    ...clipCues(timeline.intro.words, 0),
    ...timeline.segments.flatMap((s) => clipCues(s.words, s.start)),
  ];
  return cues.map((c, i) => `${i + 1}\n${stamp(c.start)} --> ${stamp(c.end)}\n${c.lines.join("\n")}\n`).join("\n");
}
