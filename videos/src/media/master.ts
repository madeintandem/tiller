import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { ffmpeg, probeDuration, run } from "./ffmpeg.js";

const TARGET_I = -14;
const TARGET_TP = -1;
const TARGET_LRA = 11;
const MUSIC_LUFS = -24;
const FADE = 1;

/** Music gain: 0 everywhere except 0..introEnd and outroStart..end, with FADE-second ramps at each edge. */
export function musicGain(introEnd: number, outroStart: number, end: number): string {
  const ramp = (a: number, b: number) => `clip(min((t-${a})/${FADE},(${b}-t)/${FADE}),0,1)`;
  return `${ramp(0, introEnd)}+${ramp(outroStart, end)}`;
}

/**
 * Mix music under the intro and outro only, loudness-normalize the whole program
 * (two-pass loudnorm), and encode H.264 High / AAC 320k with faststart.
 */
export async function master(
  renderPath: string,
  musicPath: string,
  outro: { start: number },
  introEnd: number,
  outPath: string,
): Promise<string> {
  const end = await probeDuration(renderPath);
  const dir = mkdtempSync(join(tmpdir(), "master-"));
  try {
    const mixed = join(dir, "mix.wav");
    const gain = musicGain(introEnd, outro.start, end);
    await ffmpeg([
      "-i", renderPath, "-stream_loop", "-1", "-i", musicPath,
      "-filter_complex",
      `[1:a]aformat=sample_rates=48000:channel_layouts=stereo,loudnorm=I=${MUSIC_LUFS}:TP=-2:LRA=${TARGET_LRA},` +
        `volume='${gain}':eval=frame[m];` +
        `[0:a]aformat=sample_rates=48000:channel_layouts=stereo[v];` +
        `[v][m]amix=inputs=2:duration=first:normalize=0[a]`,
      "-map", "[a]", "-t", String(end), "-c:a", "pcm_f32le", mixed,
    ]);

    const norm = `loudnorm=I=${TARGET_I}:TP=${TARGET_TP}:LRA=${TARGET_LRA}`;
    const { stderr } = await run("ffmpeg", ["-hide_banner", "-nostats", "-i", mixed, "-af", `${norm}:print_format=json`, "-f", "null", "-"]);
    const m = JSON.parse(stderr.slice(stderr.lastIndexOf("{"), stderr.lastIndexOf("}") + 1));
    const pass2 =
      `${norm}:measured_I=${m.input_i}:measured_TP=${m.input_tp}:measured_LRA=${m.input_lra}:` +
      `measured_thresh=${m.input_thresh}:offset=${m.target_offset}:linear=true`;

    await ffmpeg([
      "-i", renderPath, "-i", mixed, "-map", "0:v:0", "-map", "1:a:0",
      "-af", `${pass2},aresample=48000`,
      "-c:v", "libx264", "-profile:v", "high", "-pix_fmt", "yuv420p", "-crf", "18", "-preset", "medium",
      "-c:a", "aac", "-b:a", "320k", "-movflags", "+faststart", "-t", String(end), outPath,
    ]);
    return outPath;
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}
