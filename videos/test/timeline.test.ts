import { describe, expect, it } from "vitest";
import type { Step } from "../src/check/diff";
import { spreadWords } from "../src/narration/words";
import type { Beat, Script } from "../src/script/schema";
import { buildTimeline, TooLongError, type Narration } from "../src/timeline/build";
import { toSrt } from "../src/timeline/captions";
import { chapters } from "../src/timeline/chapters";

const INTRO = "Hi, I'm JC. This video walks you from an empty repo to a feature merged with Tiller.";

const beats: Beat[] = [
  {
    id: "open",
    say: "Tiller keeps the plan in your repo, next to the code, so your agent and you read the same thing every time.",
    show: { kind: "card", kicker: "Tiller how-to", heading: "Plan in the repo" },
  },
  {
    id: "init",
    chapter: "Set up",
    say: "First we set the project up. One command writes the ledger and tells you what to do next, step by step, so nothing is guessed.",
    show: { kind: "term", stage: "s1", from: "s1-init", to: "s1-add" },
  },
  {
    id: "board",
    say: "The console shows the same plan as a board. Every card is a feature, and its column says where the work stands right now.",
    show: { kind: "console", capture: "board" },
  },
  {
    id: "bye",
    chapter: "Wrap up",
    say: "That is the whole loop. Next time we write the first feature together and watch the agent pick it up.",
    show: { kind: "outro" },
  },
];

const script = (bs: Beat[] = beats): Script => ({
  id: "v-test",
  title: "Test",
  description: "Test",
  tags: [],
  intro: { transcript: INTRO },
  captures: [{ id: "board", mark: "s1-end", path: "/build", seconds: 8 }],
  beats: bs,
});

const steps: Step[] = [
  { id: "s1-version", actor: "you", kind: "cmd", cmd: "tiller --version", out: ["0.22.0"] },
  { id: "s1-init", actor: "you", kind: "cmd", cmd: "tiller init", out: ["ok"] },
  { id: "s1-say", actor: "agent", kind: "say", text: "Done." },
  { id: "s1-add", actor: "you", kind: "cmd", cmd: "tiller add contributor jc", out: ["ok"] },
  { id: "s1-after", actor: "you", kind: "cmd", cmd: "tiller guide", out: ["ok"] },
];

// Fake narration: 150 wpm, words evenly spread, as NARRATION_FAKE produces.
function narrationFor(bs: Beat[], secondsEach?: number): Record<string, Narration> {
  const out: Record<string, Narration> = {};
  for (const b of bs) {
    const duration = secondsEach ?? Math.max(1, (b.say.split(/\s+/).length / 150) * 60);
    out[b.id] = { audio: `narration/${b.id}.mp3`, duration, words: spreadWords(b.say, duration) };
  }
  return out;
}

const intro = { path: "intro.mp4", duration: 6, words: spreadWords(INTRO, 6) };
const build = (s = script(), n = narrationFor(s.beats)) =>
  buildTimeline(s, intro, n, { board: "captures/board.webm" }, { s1: steps });

const norm = (s: string) => s.replace(/\s+/g, " ").trim();

type Cue = { start: number; end: number; lines: string[] };
function parseSrt(srt: string): Cue[] {
  const t = (s: string) => {
    const [h, m, rest] = s.split(":");
    const [sec, ms] = rest.split(",");
    return +h * 3600 + +m * 60 + +sec + +ms / 1000;
  };
  return srt
    .trim()
    .split(/\n\n+/)
    .map((block) => {
      const [, times, ...lines] = block.split("\n");
      const [a, b] = times.split(" --> ");
      return { start: t(a), end: t(b), lines };
    });
}

describe("buildTimeline", () => {
  it("adds intro, gaps, segments, and outro up to the duration", () => {
    const tl = build();
    const narr = narrationFor(beats);
    const spoken = beats.reduce((n, b) => n + narr[b.id].duration, 0);
    const expected = intro.duration + 0.4 + spoken + 0.35 * (beats.length - 1) + 6;
    expect(tl.duration).toBeCloseTo(expected, 6);
    expect(tl.outro.end).toBeCloseTo(tl.duration, 6);
    expect(tl.outro.end - tl.outro.start).toBeCloseTo(6, 6);
    expect(tl.segments[0].start).toBeCloseTo(6.4, 6);
    for (let i = 1; i < tl.segments.length; i++) {
      expect(tl.segments[i].start - tl.segments[i - 1].end).toBeCloseTo(0.35, 6);
    }
  });

  it("carries the term steps from show.from to show.to inclusive", () => {
    const seg = build().segments.find((s) => s.beatId === "init")!;
    expect(seg.steps!.map((s) => s.id)).toEqual(["s1-init", "s1-say", "s1-add"]);
  });

  it("carries the capture path for console beats", () => {
    const seg = build().segments.find((s) => s.beatId === "board")!;
    expect(seg.capturePath).toBe("captures/board.webm");
  });

  it("names the beat when a term step id is missing", () => {
    const bad = beats.map((b) => (b.id === "init" ? { ...b, show: { kind: "term" as const, stage: "s1", from: "s1-init", to: "nope" } } : b));
    expect(() => build(script(bad))).toThrow(/init/);
  });

  it("throws TooLongError with m:ss past 8:00", () => {
    // intro 6 + gap 0.4 + 4 beats + 3 gaps of 0.35 + outro 6 = 481 when the beats total 467.55.
    const each = (481 - 6 - 0.4 - 0.35 * 3 - 6) / 4;
    const s = script();
    let err: unknown;
    try {
      build(s, narrationFor(s.beats, each));
    } catch (e) {
      err = e;
    }
    expect(err).toBeInstanceOf(TooLongError);
    expect((err as Error).message).toContain("8:01");
  });
});

describe("toSrt", () => {
  it("captions exactly the spoken text: intro transcript then each beat", () => {
    const cues = parseSrt(toSrt(build()));
    const text = norm(cues.map((c) => c.lines.join(" ")).join(" "));
    expect(text).toBe(norm([INTRO, ...beats.map((b) => b.say)].join(" ")));
  });

  it("keeps cues to 2 lines of 42 characters and 6 seconds", () => {
    // Slow narration stresses the 6 s limit; long beats stress the line limit.
    for (const tl of [build(), build(script(), narrationFor(beats, 30))]) {
      const cues = parseSrt(toSrt(tl));
      expect(cues.length).toBeGreaterThan(4);
      for (const c of cues) {
        expect(c.lines.length).toBeLessThanOrEqual(2);
        for (const l of c.lines) expect(l.length).toBeLessThanOrEqual(42);
        expect(c.end - c.start).toBeLessThanOrEqual(6.0005);
        expect(c.end).toBeGreaterThan(c.start);
      }
    }
  });

  it("never spans a beat boundary", () => {
    const tl = build();
    const cues = parseSrt(toSrt(tl));
    const bounds = [tl.intro.duration, ...tl.segments.map((s) => s.end)];
    for (const c of cues) {
      for (const b of bounds) expect(c.start < b - 0.001 && c.end > b + 0.001).toBe(false);
    }
  });
});

describe("chapters", () => {
  it("starts at 0:00 Intro and lists each chaptered beat at its start", () => {
    const tl = build();
    const list = chapters(tl, script());
    expect(list[0]).toBe("0:00 Intro");
    expect(list.length).toBeGreaterThanOrEqual(3);
    expect(list.map((l) => l.replace(/^\d+:\d\d /, ""))).toEqual(["Intro", "Set up", "Wrap up"]);
    const setUp = tl.segments.find((s) => s.beatId === "init")!;
    const m = Math.floor(setUp.start / 60);
    const s = Math.floor(setUp.start % 60);
    expect(list[1]).toBe(`${m}:${String(s).padStart(2, "0")} Set up`);
  });

  it("throws when two chapters are under 10 seconds apart", () => {
    // Every beat is 4 s, so chapters land under 10 s apart.
    const s = script();
    const tl = build(s, narrationFor(s.beats, 4));
    expect(() => chapters(tl, s)).toThrow(/10/);
  });

  it("throws when a chaptered first beat starts under 10 seconds after a short intro", () => {
    const s = script(beats.map((b) => (b.id === "open" ? { ...b, chapter: "Why Tiller" } : b)));
    expect(() => chapters(build(s), s)).toThrow(/Why Tiller/);
  });
});
