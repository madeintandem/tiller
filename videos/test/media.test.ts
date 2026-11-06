import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { ffmpeg, probeDuration, run } from "../src/media/ffmpeg.js";
import { prepareIntro } from "../src/media/intro.js";
import { master } from "../src/media/master.js";

let dir: string;
const p = (n: string) => join(dir, n);

async function integratedLufs(file: string): Promise<number> {
  const { stderr } = await run("ffmpeg", ["-hide_banner", "-nostats", "-i", file, "-af", "ebur128", "-f", "null", "-"]);
  const summary = stderr.slice(stderr.lastIndexOf("Summary:"));
  return parseFloat(/I:\s+(-?[\d.]+) LUFS/.exec(summary)![1]);
}

async function maxVolume(file: string, from: number, len: number): Promise<number> {
  const { stderr } = await run("ffmpeg", ["-hide_banner", "-nostats", "-ss", String(from), "-t", String(len), "-i", file, "-af", "volumedetect", "-vn", "-f", "null", "-"]);
  return parseFloat(/max_volume: (-?[\d.inf]+) dB/.exec(stderr)![1]);
}

async function streams(file: string): Promise<string[]> {
  const { stdout } = await run("ffprobe", ["-v", "error", "-show_entries", "stream=codec_name", "-of", "csv=p=0", file]);
  return stdout.trim().split("\n").sort();
}

/** 1 s silence + 3 s tone + 1 s silence audio, over a color source of the given size. */
async function makeClip(out: string, size: string, fps: number) {
  await ffmpeg([
    "-f", "lavfi", "-i", `color=c=white:s=${size}:r=${fps}:d=5`,
    "-f", "lavfi", "-i", "sine=f=440:d=3,adelay=1000|1000,apad=whole_dur=5",
    "-t", "5", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", out,
  ]);
}

beforeAll(async () => {
  dir = mkdtempSync(join(tmpdir(), "media-"));
  await makeClip(p("intro.mp4"), "1280x720", 24);
  await makeClip(p("portrait.mp4"), "720x1280", 60);
  // 12 s render: tone "voice" from 4 to 8 s, silent otherwise.
  await ffmpeg([
    "-f", "lavfi", "-i", "color=c=black:s=640x360:r=30:d=12",
    "-f", "lavfi", "-i", "sine=f=300:d=12,volume='between(t,4,8)':eval=frame",
    "-t", "12", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", p("render.mp4"),
  ]);
  await ffmpeg([
    "-f", "lavfi", "-i", "color=c=black:s=640x360:r=30:d=12",
    "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo",
    "-t", "12", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", p("silent.mp4"),
  ]);
  await ffmpeg(["-f", "lavfi", "-i", "sine=f=880:d=5", "-c:a", "pcm_s16le", p("music.wav")]);
}, 120_000);

afterAll(() => rmSync(dir, { recursive: true, force: true }));

describe("prepareIntro", () => {
  it("trims leading and trailing silence", async () => {
    const r = await prepareIntro(p("intro.mp4"), p("intro-out.mp4"));
    expect(r.path).toBe(p("intro-out.mp4"));
    expect(Math.abs(r.duration - 3)).toBeLessThan(0.15);
  }, 60_000);

  it("scales a portrait 60 fps clip to 1920x1080 at 30 fps without stretching", async () => {
    const r = await prepareIntro(p("portrait.mp4"), p("portrait-out.mp4"));
    const { stdout } = await run("ffprobe", ["-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height,r_frame_rate", "-of", "csv=p=0", r.path]);
    expect(stdout.trim()).toBe("1920,1080,30/1");
    // Pillarboxed: the non-black picture is a 9:16 column, not the full frame.
    const { stderr } = await run("ffmpeg", ["-hide_banner", "-nostats", "-i", r.path, "-vf", "cropdetect=limit=40:round=2", "-frames:v", "20", "-f", "null", "-"]);
    const [w, h] = [...stderr.matchAll(/crop=(\d+):(\d+):\d+:\d+/g)].pop()!.slice(1).map(Number);
    expect(h).toBe(1080);
    expect(Math.abs(w - 608)).toBeLessThan(8);
  }, 60_000);
});

describe("master", () => {
  it("normalizes to -14 LUFS and outputs one h264 and one aac stream", async () => {
    const out = await master(p("render.mp4"), p("music.wav"), { start: 10 }, 3, p("master.mp4"));
    expect(Math.abs((await integratedLufs(out)) - -14)).toBeLessThan(1);
    expect(await streams(out)).toEqual(["aac", "h264"]);
    const { stdout } = await run("ffprobe", ["-v", "error", "-select_streams", "v:0", "-show_entries", "stream=profile,pix_fmt", "-of", "csv=p=0", out]);
    expect(stdout.trim()).toBe("High,yuv420p");
    expect(Math.abs((await probeDuration(out)) - 12)).toBeLessThan(0.2);
  }, 120_000);

  it("carries no music between introEnd and outro.start", async () => {
    const out = await master(p("silent.mp4"), p("music.wav"), { start: 10 }, 3, p("master-silent.mp4"));
    expect(await maxVolume(out, 0.2, 2.5)).toBeGreaterThan(-40); // music under the intro
    expect(await maxVolume(out, 10.5, 1)).toBeGreaterThan(-40); // music under the outro
    expect(await maxVolume(out, 3.1, 6.8)).toBeLessThan(-60); // narration region
  }, 120_000);
});
