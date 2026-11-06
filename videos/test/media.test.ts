import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { ffmpeg, probeDuration, run } from "../src/media/ffmpeg.js";
import { hdrFilter, prepareIntro } from "../src/media/intro.js";
import { hasFilter } from "../src/media/ffmpeg.js";
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

async function regionLufs(file: string, from: number, len: number): Promise<number> {
  const { stderr } = await run("ffmpeg", ["-hide_banner", "-nostats", "-ss", String(from), "-t", String(len), "-i", file, "-vn", "-af", "ebur128", "-f", "null", "-"]);
  const summary = stderr.slice(stderr.lastIndexOf("Summary:"));
  return parseFloat(/I:\s+(-?[\d.]+) LUFS/.exec(summary)![1]);
}

describe("prepareIntro", () => {
  it("trims leading and trailing silence, keeping 120 ms of pad at each end", async () => {
    const r = await prepareIntro(p("intro.mp4"), p("intro-out.mp4"));
    expect(r.path).toBe(p("intro-out.mp4"));
    expect(Math.abs(r.duration - 3.24)).toBeLessThan(0.1);
  }, 60_000);

  it("clamps the pad to the clip when speech runs to the edges", async () => {
    await ffmpeg([
      "-f", "lavfi", "-i", "color=c=white:s=640x360:r=30:d=3",
      "-f", "lavfi", "-i", "sine=f=440:d=3",
      "-t", "3", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", p("edge.mp4"),
    ]);
    const r = await prepareIntro(p("edge.mp4"), p("edge-out.mp4"));
    expect(Math.abs(r.duration - 3)).toBeLessThan(0.1);
  }, 60_000);

  it("normalizes a quiet intro to -16 LUFS", async () => {
    await ffmpeg([
      "-f", "lavfi", "-i", "color=c=white:s=640x360:r=30:d=4",
      "-f", "lavfi", "-i", "sine=f=440:d=4,volume=-9dB",
      "-t", "4", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", p("quiet.mp4"),
    ]);
    expect(Math.abs((await integratedLufs(p("quiet.mp4"))) - -30)).toBeLessThan(2);
    const r = await prepareIntro(p("quiet.mp4"), p("quiet-out.mp4"));
    expect(Math.abs((await integratedLufs(r.path)) - -16)).toBeLessThan(1);
  }, 60_000);

  it("refuses or tone-maps an HDR (HLG) clip, never passing HDR through", async () => {
    await ffmpeg([
      "-f", "lavfi", "-i", "testsrc2=s=640x360:r=30:d=3",
      "-f", "lavfi", "-i", "sine=f=440:d=3",
      "-t", "3", "-vf", "setparams=color_primaries=bt2020:color_trc=arib-std-b67:colorspace=bt2020nc,format=yuv420p10le",
      "-c:v", "libx264", "-c:a", "aac", p("hlg.mp4"),
    ]);
    if (!(await hasFilter("zscale"))) {
      await expect(prepareIntro(p("hlg.mp4"), p("hlg-out.mp4"))).rejects.toThrow(/Settings > Camera > Formats/);
      return;
    }
    const r = await prepareIntro(p("hlg.mp4"), p("hlg-out.mp4"));
    const { stdout } = await run("ffprobe", ["-v", "error", "-select_streams", "v:0", "-show_entries", "stream=color_transfer", "-of", "csv=p=0", r.path]);
    expect(stdout.trim()).toBe("bt709");
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

describe("hdrFilter", () => {
  it("passes SDR through untouched", () => {
    expect(hdrFilter("bt709", false)).toBe("");
    expect(hdrFilter(undefined, false)).toBe("");
  });

  it("tone-maps HLG and PQ to BT.709 when zscale is available", () => {
    for (const trc of ["arib-std-b67", "smpte2084"]) expect(hdrFilter(trc, true)).toMatch(/zscale.*tonemap.*bt709/);
  });

  it("refuses HLG and PQ without zscale, naming the camera setting", () => {
    for (const trc of ["arib-std-b67", "smpte2084"]) expect(() => hdrFilter(trc, false)).toThrow(/turn off HDR Video.*Settings > Camera > Formats/);
  });
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

  it("sets the music bed 18 LU under a -16 LUFS voice", async () => {
    // 16 s render: a -16 LUFS "voice" tone under the intro (0 to 4 s), silent after.
    await ffmpeg([
      "-f", "lavfi", "-i", "color=c=black:s=640x360:r=30:d=16",
      "-f", "lavfi", "-i", "sine=f=300:d=4,apad=whole_dur=16",
      "-t", "16", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "pcm_s16le", p("voice-raw.mov"),
    ]);
    const raw = await integratedLufs(p("voice-raw.mov"));
    await ffmpeg(["-i", p("voice-raw.mov"), "-c:v", "copy", "-af", `volume=${(-16 - raw).toFixed(2)}dB`, "-c:a", "aac", "-b:a", "256k", p("voice.mp4")]);
    const out = await master(p("voice.mp4"), p("music.wav"), { start: 10 }, 4, p("master-bed.mp4"));
    const voiceAndMusic = await regionLufs(out, 1.2, 1.6); // music at full under the voice
    const musicOnly = await regionLufs(out, 11.5, 3); // music alone under the outro
    expect(Math.abs(voiceAndMusic - musicOnly - 18)).toBeLessThan(1.5);
  }, 120_000);

  it("carries no music between introEnd and outro.start", async () => {
    const out = await master(p("silent.mp4"), p("music.wav"), { start: 10 }, 3, p("master-silent.mp4"));
    expect(await maxVolume(out, 0.2, 2.5)).toBeGreaterThan(-40); // music under the intro
    expect(await maxVolume(out, 10.5, 1)).toBeGreaterThan(-40); // music under the outro
    expect(await maxVolume(out, 3.1, 6.8)).toBeLessThan(-60); // narration region
  }, 120_000);
});
