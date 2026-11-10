// Usage: npm run build -- <video-id>
// Turns an approved script into videos/out/<id>.mp4, <id>.srt, <id>.jpg (thumbnail), and <id>.meta.json.
import { copyFileSync, cpSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";
import { buildDir, introPath, layoutFor } from "../build/layout.js";
import { metadata, voiceProvenance } from "../build/meta.js";
import { clearOutputs } from "../build/outputs.js";
import { preflight } from "../build/preflight.js";
import { renderThumbnail, renderVideo } from "../build/render.js";
import { shoot } from "../capture/shoot.js";
import { checkSnapshots } from "../check/run.js";
import { readSnapshot, type Step } from "../check/diff.js";
import { loadConfig } from "../config.js";
import { probeDuration } from "../media/ffmpeg.js";
import { prepareIntro } from "../media/intro.js";
import { master } from "../media/master.js";
import { align, narrate } from "../narration/elevenlabs.js";
import { buildTimeline, openingEnd, type Narration } from "../timeline/build.js";
import { toSrt } from "../timeline/captions.js";
import { chapters as chapterLines } from "../timeline/chapters.js";

class Stop extends Error {}

// Runs one phase and prints one line: its name, an optional detail, and the elapsed time.
async function phase<T>(name: string, fn: () => T | Promise<T>, detail?: (r: T) => string): Promise<T> {
  const t0 = performance.now();
  const r = await fn();
  const s = ((performance.now() - t0) / 1000).toFixed(1);
  console.log(`${name.padEnd(10)} ${detail ? `${detail(r)} ` : ""}(${s} s)`);
  return r;
}

async function build(id: string): Promise<void> {
  const layout = layoutFor();

  // Free, local checks first: nothing paid or slow runs unless they all pass. The script and
  // its approved hash come from preflight's single read of the file.
  const { script, hash: approvedHash } = await phase("preflight", () => {
    const r = preflight(id, layout);
    for (const w of r.warnings) console.warn(`warning: ${w}`);
    if (!r.ok) throw new Stop(`preflight failed:\n${r.problems.map((p) => `  - ${p}`).join("\n")}`);
    return r;
  }, () => "ok");
  // A build that stops partway must not leave an older build's outputs looking current.
  clearOutputs(layout, id);
  await phase("check", () => {
    const r = checkSnapshots();
    if (!r.ok) throw new Stop(r.message);
    return r.message;
  }, (m) => m);

  const cfg = loadConfig(layout.config);
  const dir = buildDir(layout, id);
  mkdirSync(dir, { recursive: true });
  // The build dir is Remotion's publicDir, so the frame's brand assets go there too.
  cpSync(layout.publicDir, dir, { recursive: true });

  // Captures are local and free, so they run before the paid intro alignment and narration.
  const captures = await phase("captures", async () => {
    const out: Record<string, string> = {};
    for (const c of script.captures) out[c.id] = relative(dir, await shoot(c, join(dir, "captures")));
    return out;
  }, (c) => `${Object.keys(c).length} recorded`);

  // A script without an intro block opens on its first beat; there is no clip to prepare.
  const introTranscript = script.intro?.transcript;
  const intro = introTranscript === undefined ? undefined : await phase("intro", async () => {
    const prepared = await prepareIntro(introPath(layout, id), join(dir, "intro.mp4"));
    const words = await align(prepared.path, introTranscript, cfg.elevenlabs.apiKey, layout.cache);
    return { path: "intro.mp4", duration: prepared.duration, words };
  }, (i) => `${i.duration.toFixed(2)} s, ${i.words.length} words aligned`);

  const narrations = await phase("narration", async () => {
    const out: Record<string, Narration> = {};
    rmSync(join(dir, "narration"), { recursive: true, force: true });
    mkdirSync(join(dir, "narration"), { recursive: true });
    for (const beat of script.beats) {
      const n = await narrate(beat.say, cfg.elevenlabs, layout.cache);
      const audio = `narration/${beat.id}.mp3`;
      copyFileSync(n.audioPath, join(dir, audio));
      out[beat.id] = { audio, duration: n.duration, words: n.words };
    }
    return out;
  }, (n) => `${Object.keys(n).length} beats${process.env.NARRATION_FAKE === "1" ? " (fake)" : ""}`);

  const { timeline, chapters, srt } = await phase("timeline", () => {
    const stages = [...new Set(script.beats.flatMap((b) => (b.show.kind === "term" ? [b.show.stage] : [])))];
    const snapshots: Record<string, Step[]> = Object.fromEntries(stages.map((s) => [s, readSnapshot(s, layout.snapshots)]));
    const timeline = buildTimeline(script, intro, narrations, captures, snapshots);
    // Chapters are checked against YouTube's rules here, before the long render.
    const chapters = chapterLines(timeline, script);
    writeFileSync(join(dir, "timeline.json"), JSON.stringify(timeline, null, 2) + "\n");
    return { timeline, chapters, srt: toSrt(timeline) };
  }, (r) => `${r.timeline.duration.toFixed(2)} s, ${r.chapters.length} chapters`);

  const render = await phase("render", () => renderVideo(timeline, dir, join(dir, "render.mp4")), (p) => relative(layout.root, p));

  mkdirSync(layout.out, { recursive: true });
  const mp4 = join(layout.out, `${id}.mp4`);
  await phase("master", () => master(render, layout.music, timeline.outro, openingEnd(timeline), mp4), (p) => relative(layout.root, p));

  await phase("outputs", async () => {
    writeFileSync(join(layout.out, `${id}.srt`), srt);
    const fake = process.env.NARRATION_FAKE === "1";
    const provenance = { fake, scriptHash: approvedHash, voiceId: voiceProvenance(fake, cfg.elevenlabs.voiceId) };
    const meta = metadata(script, chapters, await probeDuration(mp4), provenance);
    writeFileSync(join(layout.out, `${id}.meta.json`), JSON.stringify(meta, null, 2) + "\n");
    return meta;
  }, (m) => `${id}.srt, ${id}.meta.json, ${m.durationSeconds.toFixed(2)} s`);

  await phase("thumbnail", () => renderThumbnail(script.title, dir, join(layout.out, `${id}.jpg`)), (p) => relative(layout.root, p));
}

const id = process.argv[2];
if (!id) {
  console.error("usage: npm run build -- <video-id>");
  process.exit(2);
}
const t0 = performance.now();
try {
  await build(id);
  console.log(`built ${id} in ${((performance.now() - t0) / 1000).toFixed(1)} s`);
} catch (e) {
  console.error(e instanceof Stop ? e.message : `build failed: ${(e as Error).message}`);
  process.exit(1);
}
