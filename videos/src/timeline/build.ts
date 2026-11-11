import type { Step } from "../check/diff.js";
import type { Word } from "../narration/words.js";
import { cueWord, phraseIndex, type Point, type Script, type Show } from "../script/schema.js";
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
  // When each of a slide's points comes up, in seconds from the start of the narration.
  pointTimes?: number[];
  // For the "cli" graphic: the tiller commands and skills this video uses, to highlight.
  seen?: string[];
  // On a chapter card: every chapter of the video and which one this is.
  track?: { names: string[]; index: number };
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
  // The filmed intro clip, when the script has one; otherwise the program starts on its first beat.
  intro?: { path: string; duration: number; words: Word[] };
  segments: Segment[];
  outro: { start: number; end: number };
  duration: number;
};

export type Narration = { audio: string; duration: number; words: Word[] };

export const INTRO_GAP = 0.4;
export const BEAT_GAP = 0.35;
export const OUTRO_SECONDS = 6;
// The series' ceiling. A script may set its own `maxSeconds` when it has a reason to run longer.
export const MAX_SECONDS = 480;
export const ceilingFor = (script: { maxSeconds?: number }): number => script.maxSeconds ?? MAX_SECONDS;

// m:ss, rounding up so anything past 8:00 never reads as 8:00.
export const mss = (seconds: number, round: (n: number) => number = Math.floor): string => {
  const s = round(seconds);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
};

/** Where the opening music bed ends: the intro clip's end, or without one the end of the first beat. */
export const openingEnd = (t: Timeline): number => t.intro?.duration ?? t.segments[0]?.end ?? 0;

export class TooLongError extends Error {
  constructor(public readonly duration: number, max: number = MAX_SECONDS) {
    // Round to the millisecond first so float noise (481.0000001) never reads as a second more.
    const ceilMs = (s: number) => Math.ceil(Math.round(s * 1000) / 1000);
    super(`video runs ${mss(duration, ceilMs)}, over the ${mss(max)} ceiling`);
    this.name = "TooLongError";
  }
}

/**
 * When each point of a slide comes up, in seconds into the beat's narration: as its `on`
 * phrase starts being spoken, or (with no cue, or a cue the aligned words lack) at its
 * share of an even spread. Never earlier than the point before it.
 */
export function pointTimes(points: Point[], n: Narration): number[] {
  // Aligned words that normalize to nothing (a stray dash) are dropped along with their times.
  const spoken = n.words.map((w) => ({ w: cueWord(w.text), start: w.start })).filter((x) => x.w);
  let floor = 0;
  return points.map((p, i) => {
    const at = p.on === undefined ? -1 : phraseIndex(spoken.map((x) => x.w), p.on);
    const t = Math.max(floor, at >= 0 ? spoken[at].start : (n.duration * (i + 1)) / (points.length + 1));
    floor = t;
    return t;
  });
}

/**
 * The tiller commands and skills a script puts on screen, for the "cli" graphic to highlight:
 * the subcommand of every `tiller <subcommand>` its term beats run, a skill named as that
 * command's first argument (`tiller readout derive-baseline`), and `serve` when the script
 * shows the console.
 */
export function seenCommands(script: Script, snapshots: Record<string, Step[]>): string[] {
  const seen = new Set<string>();
  for (const beat of script.beats) {
    if (beat.show.kind !== "term") continue;
    for (const step of termSteps(beat.id, beat.show, snapshots)) {
      const m = step.kind === "cmd" ? /^tiller ([a-z][a-z-]*)(?: ([a-z][a-z-]*))?/.exec(step.cmd ?? "") : null;
      if (!m) continue;
      seen.add(m[1]);
      if (m[2] && (m[1] === "guide" || m[1] === "readout")) seen.add(`skill:${m[2]}`);
    }
  }
  if (script.captures.length > 0) seen.add("serve");
  return [...seen].sort();
}

// The steps a term beat shows, from its `from` to its `to` step inclusive.
export function termSteps(beatId: string, show: Extract<Show, { kind: "term" }>, snapshots: Record<string, Step[]>): Step[] {
  const stage = snapshots[show.stage];
  if (!stage) throw new Error(`beat ${beatId}: no snapshot for stage "${show.stage}"`);
  const from = stage.findIndex((s) => s.id === show.from);
  const to = stage.findIndex((s) => s.id === show.to);
  if (from < 0) throw new Error(`beat ${beatId}: stage ${show.stage} has no step "${show.from}"`);
  if (to < 0) throw new Error(`beat ${beatId}: stage ${show.stage} has no step "${show.to}"`);
  if (to < from) throw new Error(`beat ${beatId}: step "${show.to}" comes before "${show.from}"`);
  return stage.slice(from, to + 1);
}

// The intro clip and a short gap (when there is one), each beat's narration back to back with a gap between, then the outro.
export function buildTimeline(
  script: Script,
  intro: { path: string; duration: number; words: Word[] } | undefined,
  narrations: Record<string, Narration>,
  captures: Record<string, string>,
  snapshots: Record<string, Step[]>,
): Timeline {
  const segments: Segment[] = [];
  const chapterNames = script.beats.flatMap((b) => (b.chapter ? [b.chapter] : []));
  let at = intro ? intro.duration + INTRO_GAP : 0;
  script.beats.forEach((beat, i) => {
    const n = narrations[beat.id];
    if (!n) throw new Error(`beat ${beat.id}: no narration`);
    if (i > 0) at += BEAT_GAP;
    const seg: Segment = { beatId: beat.id, start: at, end: at + n.duration, audio: n.audio, words: n.words, show: beat.show };
    if (beat.chapter) {
      if (beat.show.kind !== "card") throw new Error(`beat ${beat.id}: chapter is only allowed on a card beat`);
      seg.chapter = beat.chapter;
      seg.track = { names: chapterNames, index: chapterNames.indexOf(beat.chapter) };
    }
    if (beat.show.kind === "card" && beat.show.art === "cli") seg.seen = seenCommands(script, snapshots);
    if (beat.show.kind === "card" && beat.show.points) seg.pointTimes = pointTimes(beat.show.points, n);
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
  if (duration > ceilingFor(script)) throw new TooLongError(duration, ceilingFor(script));
  return { id: script.id, fps: video.fps, ...(intro ? { intro } : {}), segments, outro, duration };
}
