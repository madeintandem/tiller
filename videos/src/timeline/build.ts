import type { Step } from "../check/diff.js";
import type { Word } from "../narration/words.js";
import type { Script, Show } from "../script/schema.js";
import { video } from "../theme.js";

// Asset paths (intro.path, audio, capturePath) are relative to the build directory,
// which the render step serves as Remotion's publicDir. Word times are relative to
// their own clip; `start` places the clip on the program timeline.
export type Segment = {
  beatId: string;
  start: number;
  end: number;
  audio: string;
  words: Word[];
  show: Show;
  // The beat's chapter name, shown as a lower third at the segment start.
  chapter?: string;
  capturePath?: string;
  // Length of the trimmed capture (the script's capture.seconds); past it the last frame holds.
  captureSeconds?: number;
  steps?: Step[];
};

export type Timeline = {
  id: string;
  fps: number;
  intro: { path: string; duration: number; words: Word[] };
  segments: Segment[];
  outro: { start: number; end: number };
  duration: number;
};

export type Narration = { audio: string; duration: number; words: Word[] };

export const INTRO_GAP = 0.4;
export const BEAT_GAP = 0.35;
export const OUTRO_SECONDS = 6;
export const MAX_SECONDS = 480;

// m:ss, rounding up so anything past 8:00 never reads as 8:00.
export const mss = (seconds: number, round: (n: number) => number = Math.floor): string => {
  const s = round(seconds);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
};

export class TooLongError extends Error {
  constructor(public readonly duration: number) {
    super(`video runs ${mss(duration, Math.ceil)}, over the ${mss(MAX_SECONDS)} ceiling`);
    this.name = "TooLongError";
  }
}

function termSteps(beatId: string, show: Extract<Show, { kind: "term" }>, snapshots: Record<string, Step[]>): Step[] {
  const stage = snapshots[show.stage];
  if (!stage) throw new Error(`beat ${beatId}: no snapshot for stage "${show.stage}"`);
  const from = stage.findIndex((s) => s.id === show.from);
  const to = stage.findIndex((s) => s.id === show.to);
  if (from < 0) throw new Error(`beat ${beatId}: stage ${show.stage} has no step "${show.from}"`);
  if (to < 0) throw new Error(`beat ${beatId}: stage ${show.stage} has no step "${show.to}"`);
  if (to < from) throw new Error(`beat ${beatId}: step "${show.to}" comes before "${show.from}"`);
  return stage.slice(from, to + 1);
}

// Intro, a short gap, each beat's narration back to back with a gap between, then the outro.
export function buildTimeline(
  script: Script,
  intro: { path: string; duration: number; words: Word[] },
  narrations: Record<string, Narration>,
  captures: Record<string, string>,
  snapshots: Record<string, Step[]>,
): Timeline {
  const segments: Segment[] = [];
  let at = intro.duration + INTRO_GAP;
  script.beats.forEach((beat, i) => {
    const n = narrations[beat.id];
    if (!n) throw new Error(`beat ${beat.id}: no narration`);
    if (i > 0) at += BEAT_GAP;
    const seg: Segment = { beatId: beat.id, start: at, end: at + n.duration, audio: n.audio, words: n.words, show: beat.show };
    if (beat.chapter) seg.chapter = beat.chapter;
    if (beat.show.kind === "console") {
      const id = beat.show.capture;
      const path = captures[id];
      if (!path) throw new Error(`beat ${beat.id}: no capture "${id}"`);
      seg.capturePath = path;
      const cap = script.captures.find((c) => c.id === id);
      if (cap) seg.captureSeconds = cap.seconds;
    }
    if (beat.show.kind === "term") seg.steps = termSteps(beat.id, beat.show, snapshots);
    segments.push(seg);
    at = seg.end;
  });
  const outro = { start: at, end: at + OUTRO_SECONDS };
  const duration = outro.end;
  if (duration > MAX_SECONDS) throw new TooLongError(duration);
  return { id: script.id, fps: video.fps, intro, segments, outro, duration };
}
